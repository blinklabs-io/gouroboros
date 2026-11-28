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

package protocol

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

func TestObservedMessagesCoalesceAndFragment(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	require.NoError(t, remote.SetDeadline(time.Now().Add(5*time.Second)))
	m := muxer.New(local)
	defer m.Stop(context.Background())
	state := NewState(1, "send")
	p := New(ProtocolConfig{
		Muxer: m, ProtocolId: 3, Role: ProtocolRoleClient,
		InitialState: state,
		StateMap: StateMap{state: {
			Agency:      AgencyClient,
			Transitions: []StateTransition{{MsgType: 0, NewState: state}},
		}},
	})
	defer p.Stop(context.Background())
	recorder, err := muxer.NewWriteRecorder(2)
	require.NoError(t, err)
	first := recorder.Begin(muxer.WriteMetadata{ProtocolID: 3, RequestID: 1})
	second := recorder.Begin(muxer.WriteMetadata{ProtocolID: 3, RequestID: 2})
	a := bytes.Repeat([]byte{0x01}, muxer.SegmentMaxPayloadLength-10)
	b := bytes.Repeat([]byte{0x02}, 64)
	for i, test := range []struct {
		data  []byte
		trace *muxer.WriteTrace
	}{
		{a, first}, {b, second},
	} {
		msg := &MessageBase{}
		msg.SetCbor(test.data)
		require.NoError(
			t,
			p.SendMessageContextObserved(context.Background(), msg, test.trace),
		)
		require.True(t, recorder.Snapshot()[i].WriteStartedAt.IsZero())
	}
	p.Start()
	readHeader := func() muxer.SegmentHeader {
		var h muxer.SegmentHeader
		require.NoError(t, binary.Read(remote, binary.BigEndian, &h))
		require.Equal(t, uint16(3), h.GetProtocolId())
		return h
	}
	h := readHeader()
	require.Equal(t, uint16(muxer.SegmentMaxPayloadLength), h.PayloadLength)
	payload := make([]byte, int(h.PayloadLength))
	_, err = io.ReadFull(remote, payload)
	require.NoError(t, err)
	// Reading the second header proves the first Send returned and the next
	// Write is blocked on its payload; snapshotting here cannot contend with
	// either boundary update.
	h = readHeader()
	records := recorder.Snapshot()
	require.False(t, records[0].WriteCompletedAt.IsZero())
	require.True(t, records[1].WriteCompletedAt.IsZero())
	require.Equal(t, records[0].WriteStartedAt, records[1].WriteStartedAt)
	require.Equal(t, len(a), records[1].FirstPayloadOffset)
	rest := make([]byte, int(h.PayloadLength))
	_, err = io.ReadFull(remote, rest)
	require.NoError(t, err)
	require.True(t, p.WaitSendQueueDrained(time.Second))
	require.Equal(t, append(a, b...), append(payload, rest...))
	records = recorder.Snapshot()
	for i, record := range records {
		require.Equal(t, uint64(i+1), record.Metadata.RequestID)
		require.False(t, record.DequeuedAt.Before(record.EnqueueStartedAt))
		require.False(t, record.WriteStartedAt.Before(record.DequeuedAt))
		require.False(t, record.WriteCompletedAt.Before(record.WriteStartedAt))
		require.True(t, record.FailedAt.IsZero())
	}
	require.Zero(t, recorder.Lost())
	require.Zero(t, recorder.Rejected())
}

func TestDisabledSendObservationAllocations(t *testing.T) {
	state := NewState(1, "send")
	p := New(
		ProtocolConfig{
			InitialState: state,
			StateMap:     StateMap{state: {Agency: AgencyClient}},
		},
	)
	msg := &MessageBase{}
	msg.SetCbor([]byte{0x80})
	var sendErr error
	allocations := testing.AllocsPerRun(1000, func() {
		sendErr = p.SendMessage(msg)
		<-p.sendQueueChan
		p.pendingSendBytes = 0
	})
	require.NoError(t, sendErr)
	require.Zero(
		t,
		allocations,
		"disabled queue path must not allocate observation state",
	)
}

func TestObservedPipelinedDequeuePrecedesAgencyWait(t *testing.T) {
	p, _, _, _, _, _ := newHeldMessageProtocol(t)
	recorder, err := muxer.NewWriteRecorder(1)
	require.NoError(t, err)
	trace := recorder.Begin(muxer.WriteMetadata{})
	dequeued := make(chan struct{})
	proceed := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(proceed) }) }
	t.Cleanup(release)
	p.pipelinedDequeueHook = func() { close(dequeued); <-proceed }
	require.NoError(
		t,
		p.SendMessageContextObserved(
			context.Background(),
			&MessageBase{MessageType: 4},
			trace,
		),
	)
	select {
	case <-dequeued:
	case <-time.After(time.Second):
		t.Fatal("message did not reach the pipelined dequeue")
	}
	record := recorder.Snapshot()[0]
	require.False(
		t,
		record.DequeuedAt.IsZero(),
		"dequeue must be recorded before waiting for agency",
	)
	require.True(t, record.WriteStartedAt.IsZero())
	release()
	agencyAt := time.Now()
	require.NoError(t, p.transitionState(&MessageBase{MessageType: 3}))
	require.True(t, p.WaitSendQueueDrained(time.Second))
	final := recorder.Snapshot()[0]
	require.Equal(t, record.DequeuedAt, final.DequeuedAt)
	require.False(t, final.WriteStartedAt.Before(agencyAt))
	require.False(t, final.WriteCompletedAt.IsZero())
	require.Zero(t, recorder.Lost())
}
