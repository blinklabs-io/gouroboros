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

package muxer

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWriteObservationWaitsForConnectionWrite(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	m := newTestMuxer(t, local)
	recorder, err := NewWriteRecorder(1)
	require.NoError(t, err)
	trace := recorder.Begin(WriteMetadata{ProtocolID: 3, RequestID: 17})
	require.NotNil(t, trace)
	trace.Enqueue()
	trace.Dequeue()
	otherDone := make(chan error, 1)
	go func() { otherDone <- m.Send(NewSegment(4, []byte{99}, false)) }()
	header := make([]byte, 8)
	_, err = io.ReadFull(remote, header)
	require.NoError(t, err)
	segment := NewSegment(3, []byte{1, 2, 3}, false)
	require.NotNil(t, segment)
	segment.AddWriteSpan(trace, true, true, 0)
	done := make(chan error, 1)
	go func() { done <- m.Send(segment) }()
	require.Eventually(t, func() bool { return m.egress.waiting() == 1 },
		time.Second, time.Millisecond)
	record := recorder.Snapshot()[0]
	require.False(t, record.EnqueueStartedAt.IsZero())
	require.True(
		t,
		record.WriteStartedAt.IsZero(),
		"mux arbitration must precede write start",
	)
	releaseAt := time.Now()
	payload := make([]byte, 1)
	_, err = io.ReadFull(remote, payload)
	require.NoError(t, err)
	require.NoError(t, <-otherDone)
	_, err = io.ReadFull(remote, header)
	require.NoError(t, err)
	record = recorder.Snapshot()[0]
	require.False(t, record.WriteStartedAt.Before(releaseAt))
	require.True(
		t,
		record.WriteCompletedAt.IsZero(),
		"Write is still blocked on payload",
	)
	payload = make([]byte, 3)
	_, err = io.ReadFull(remote, payload)
	require.NoError(t, err)
	require.NoError(t, <-done)
	record = recorder.Snapshot()[0]
	require.Equal(t, []byte{1, 2, 3}, payload)
	require.False(t, record.WriteCompletedAt.Before(record.WriteStartedAt))
	require.True(t, record.FailedAt.IsZero())
	require.Zero(t, recorder.Lost())
}

func TestWriteObservationPartialWriteFails(t *testing.T) {
	local, remote := net.Pipe()
	m := newTestMuxer(t, local)
	recorder, err := NewWriteRecorder(1)
	require.NoError(t, err)
	trace := recorder.Begin(WriteMetadata{ProtocolID: 3})
	require.NotNil(t, trace)
	segment := NewSegment(3, bytes.Repeat([]byte{7}, 100), false)
	require.NotNil(t, segment)
	segment.AddWriteSpan(trace, true, true, 0)
	done := make(chan error, 1)
	go func() { done <- m.Send(segment) }()
	prefix := make([]byte, 12)
	_, err = io.ReadFull(remote, prefix)
	require.NoError(t, err)
	require.NoError(t, remote.Close())
	require.Error(t, <-done)
	record := recorder.Snapshot()[0]
	require.False(t, record.WriteStartedAt.IsZero())
	require.True(t, record.WriteCompletedAt.IsZero())
	require.Equal(t, WriteFailureTransport, record.Failure)
	require.False(t, record.FailedAt.IsZero())
}

func TestWriteObservationShutdownBeforeArbitration(t *testing.T) {
	conn := newGateConn(0)
	m := newTestMuxer(t, conn)
	recorder, err := NewWriteRecorder(1)
	require.NoError(t, err)
	trace := recorder.Begin(WriteMetadata{ProtocolID: 3})
	require.NotNil(t, trace)
	segment := NewSegment(3, []byte{1}, false)
	require.NotNil(t, segment)
	segment.AddWriteSpan(trace, true, true, 0)
	_, err = m.egress.acquire(newTestSegment(4), nil)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- m.Send(segment) }()
	require.Eventually(t, func() bool { return m.egress.waiting() == 1 },
		time.Second, time.Millisecond)
	require.NoError(t, m.Stop(context.Background()))
	require.Error(t, <-done)
	m.egress.release()
	record := recorder.Snapshot()[0]
	require.True(t, record.WriteStartedAt.IsZero())
	require.True(t, record.WriteCompletedAt.IsZero())
	require.False(t, record.FailedAt.IsZero())
}

func TestWriteRecorderBoundedLossAndImmutableMetadata(t *testing.T) {
	recorder, err := NewWriteRecorder(1)
	require.NoError(t, err)
	meta := WriteMetadata{Connection: "connection", RequestID: 8}
	meta.Start.HashLength = 32
	meta.Start.Hash[0] = 42
	trace := recorder.Begin(meta)
	require.NotNil(t, trace)
	meta.Start.Hash[0] = 99
	require.Nil(t, recorder.Begin(meta))
	require.Equal(t, uint64(1), recorder.Rejected())
	trace.mu.Lock()
	trace.Enqueue()
	trace.mu.Unlock()
	record := recorder.Snapshot()[0]
	require.Equal(t, byte(42), record.Metadata.Start.Hash[0])
	require.True(t, record.Lost)
	require.Equal(t, uint64(1), recorder.Lost())
	record.Metadata.Start.Hash[0] = 88
	require.Equal(t, byte(42), recorder.Snapshot()[0].Metadata.Start.Hash[0])
}

func TestWriteRecorderConcurrentReservation(t *testing.T) {
	recorder, err := NewWriteRecorder(64)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for id := range uint64(128) {
		wg.Go(func() {
			trace := recorder.Begin(WriteMetadata{RequestID: id})
			trace.Enqueue()
			trace.Fail(WriteFailureCanceled)
		})
	}
	wg.Wait()
	records := recorder.Snapshot()
	require.Len(t, records, 64)
	require.Equal(t, uint64(64), recorder.Rejected())
	require.Zero(t, recorder.Lost())
	seen := make(map[uint64]bool)
	for _, record := range records {
		require.False(t, seen[record.Metadata.RequestID])
		seen[record.Metadata.RequestID] = true
		require.Equal(t, WriteFailureCanceled, record.Failure)
		require.True(t, record.WriteCompletedAt.IsZero())
	}
}

func TestWriteObservationDisabledAllocations(t *testing.T) {
	segment := NewSegment(3, []byte{1}, false)
	require.NotNil(t, segment)
	var trace *WriteTrace
	require.Zero(t, testing.AllocsPerRun(1000, func() {
		trace.Enqueue()
		trace.Dequeue()
		trace.SetRequestID(1)
		trace.Fail(WriteFailureAbandoned)
		segment.AddWriteSpan(trace, true, true, 0)
	}))
	require.Nil(t, segment.writeSpans)
}
