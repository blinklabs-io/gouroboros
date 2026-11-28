// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package blockfetch

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

func TestRangeWriteObservationAdmissionAndCancellation(t *testing.T) {
	recorder, err := muxer.NewWriteRecorder(3)
	require.NoError(t, err)
	c := newStartedQueueTestClient(t, &Config{
		WriteRecorder: recorder, RequestPipelining: true, MaxInFlightBytes: 1,
		RangeDoneFunc: func(CallbackContext, error) error { return nil },
	})
	first, err := c.RequestRange(
		context.Background(),
		RangeRequest{ExpectedBytes: 1},
	)
	require.NoError(t, err)
	require.True(t, c.ProtocolInstance().WaitSendQueueDrained(time.Second))
	waiting := make(chan struct{})
	c.beforeInFlightWait = func() { close(waiting) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := c.RequestRange(ctx, RangeRequest{ExpectedBytes: 1})
		result <- err
	}()
	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("request did not wait for byte admission")
	}
	records := recorder.Snapshot()
	require.Len(t, records, 2)
	require.Equal(t, first, records[0].Metadata.RequestID)
	require.False(t, records[0].WriteCompletedAt.IsZero())
	require.Zero(t, records[1].Metadata.RequestID)
	require.True(t, records[1].EnqueueStartedAt.IsZero())
	require.True(t, records[1].WriteStartedAt.IsZero())
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	record := recorder.Snapshot()[1]
	require.Equal(t, muxer.WriteFailureCanceled, record.Failure)
	require.False(t, record.FailedAt.IsZero())
	require.True(t, record.WriteCompletedAt.IsZero())
	require.Zero(t, recorder.Lost())
}

func TestRangeWriteObservationAdmissionRelease(t *testing.T) {
	recorder, err := muxer.NewWriteRecorder(2)
	require.NoError(t, err)
	c := newStartedQueueTestClient(t, &Config{
		WriteRecorder: recorder, RequestPipelining: true, MaxInFlightBytes: 1,
		RangeDoneFunc: func(CallbackContext, error) error { return nil },
	})
	_, err = c.RequestRange(
		context.Background(),
		RangeRequest{ExpectedBytes: 1},
	)
	require.NoError(t, err)
	require.True(t, c.ProtocolInstance().WaitSendQueueDrained(time.Second))
	waiting := make(chan struct{})
	c.beforeInFlightWait = func() { close(waiting) }
	result := make(chan error, 1)
	go func() {
		_, err := c.RequestRange(
			context.Background(),
			RangeRequest{ExpectedBytes: 1},
		)
		result <- err
	}()
	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("request did not wait for admission")
	}
	releaseAt := time.Now()
	c.queueMutex.Lock()
	c.removeLocked(c.queue[0])
	c.queueMutex.Unlock()
	require.NoError(t, <-result)
	require.True(t, c.ProtocolInstance().WaitSendQueueDrained(time.Second))
	record := recorder.Snapshot()[1]
	require.False(t, record.RequestedAt.After(releaseAt))
	require.False(t, record.WriteStartedAt.Before(releaseAt))
	require.False(t, record.WriteCompletedAt.IsZero())
	require.Equal(t, uint64(2), record.Metadata.RequestID)
	require.Zero(t, recorder.Lost())
}

func TestRangeWriteObservationImmutablePointsAndSessions(t *testing.T) {
	recorder, err := muxer.NewWriteRecorder(2)
	require.NoError(t, err)
	cfg := &Config{WriteRecorder: recorder}
	a := newQueueTestClient(cfg)
	defer a.pipelineCancel()
	b := newQueueTestClient(cfg)
	defer b.pipelineCancel()
	start := pcommon.Point{Slot: 9, Hash: make([]byte, 32)}
	start.Hash[0] = 42
	end := pcommon.Point{Slot: 11, Hash: make([]byte, 32)}
	end.Hash[0] = 81
	a.observeRange(start, end)
	b.observeRange(start, end)
	start.Hash[0] = 99
	end.Hash[0] = 100
	records := recorder.Snapshot()
	require.Equal(t, byte(42), records[0].Metadata.Start.Hash[0])
	require.Equal(t, uint64(11), records[0].Metadata.End.Slot)
	require.Equal(t, byte(81), records[0].Metadata.End.Hash[0])
	require.Equal(t, uint8(32), records[0].Metadata.End.HashLength)
	require.Equal(t, ProtocolId, records[0].Metadata.ProtocolID)
	require.NotEmpty(t, records[0].Metadata.Connection)
	require.NotEqual(
		t,
		records[0].Metadata.Session,
		records[1].Metadata.Session,
	)
	require.Equal(t, records[0].Metadata.Session, a.WriteObservationSession())
	require.Equal(t, records[1].Metadata.Session, b.WriteObservationSession())
	a.initProtocol()
	defer a.pipelineCancel()
	require.Equal(t, records[0].Metadata.Session, a.WriteObservationSession())
	disabled := newQueueTestClient(&Config{})
	defer disabled.pipelineCancel()
	require.Zero(t, disabled.WriteObservationSession())
	require.Zero(
		t,
		testing.AllocsPerRun(
			1000,
			func() { disabled.observeRange(start, start) },
		),
	)
}

type retireAfterWriteConn struct {
	net.Conn
	retire func()
	once   sync.Once
}

func (c *retireAfterWriteConn) Write(data []byte) (int, error) {
	n, err := c.Conn.Write(data)
	if err == nil {
		c.once.Do(c.retire)
	}
	return n, err
}

func TestRangeWriteCompletionSurvivesRetirementDuringWrite(t *testing.T) {
	local, remote := net.Pipe()
	accepted := make(chan struct{})
	var release sync.Once
	allowRetirement := func() { release.Do(func() { close(accepted) }) }
	defer allowRetirement()
	wrapper := &retireAfterWriteConn{Conn: local}
	m := muxer.New(wrapper)
	recorder, err := muxer.NewWriteRecorder(1)
	require.NoError(t, err)
	cfg, err := NewConfig(
		WithWriteRecorder(recorder),
		WithRequestPipelining(true),
		WithRangeDoneFunc(func(CallbackContext, error) error { return nil }),
	)
	require.NoError(t, err)
	c := NewClient(
		protocol.ProtocolOptions{
			Muxer: m,
			Mode:  protocol.ProtocolModeNodeToNode,
		},
		&cfg,
	)
	wrapper.retire = func() {
		<-accepted
		c.failOutstandingForProtocol(
			c.ProtocolInstance(),
			errors.New("request retired after transport accepted bytes"),
		)
	}
	drained := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, remote); close(drained) }()
	c.Start()
	t.Cleanup(func() {
		allowRetirement()
		_ = m.Stop(context.Background())
		_ = c.Stop(context.Background())
		_ = remote.Close()
		<-drained
	})
	id, err := c.RequestRange(context.Background(), RangeRequest{})
	require.NoError(t, err)
	allowRetirement()
	require.True(t, c.ProtocolInstance().WaitSendQueueDrained(time.Second))
	record := recorder.Snapshot()[0]
	require.Equal(t, id, record.Metadata.RequestID)
	require.False(
		t,
		record.WriteCompletedAt.IsZero(),
		"successful final-SDU write must survive request retirement",
	)
	require.True(t, record.FailedAt.IsZero())
	require.Equal(t, muxer.WriteFailureNone, record.Failure)
	require.Zero(t, recorder.Lost())
}
