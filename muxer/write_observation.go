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
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// WritePoint is an immutable Cardano point. Origin has HashLength zero;
// a block point has HashLength 32. It retains no caller-owned byte slice.
type WritePoint struct {
	Slot       uint64
	Hash       [32]byte
	HashLength uint8
}

// WriteMetadata identifies a message independently of mux batching. Session
// distinguishes client lifetimes even when connection addresses are reused.
type WriteMetadata struct {
	Connection string
	Session    uint64
	ProtocolID uint16
	RequestID  uint64
	Start      WritePoint
	End        WritePoint
}

// WriteFailure identifies why a message did not complete its transport write.
// It describes delivery, not the peer's response or a ledger verdict.
type WriteFailure uint8

const (
	// WriteFailureNone means no failure has been recorded.
	WriteFailureNone WriteFailure = iota
	// WriteFailureTransport includes partial writes and deadline failures.
	WriteFailureTransport
	// WriteFailureCanceled means admission or enqueue was canceled.
	WriteFailureCanceled
	// WriteFailureAbandoned means shutdown or request retirement preceded
	// a complete write.
	WriteFailureAbandoned
	// WriteFailureEnqueue means the message could not enter the send queue.
	WriteFailureEnqueue
)

// WriteRecord contains connection-write timings, not physical NIC departure.
// WriteStartedAt is immediately before the first net.Conn.Write call covering
// the message; coalesced messages can share that timestamp. FirstPayloadOffset
// locates its first byte within that call's mux payload (after the 8-byte
// header). WriteCompletedAt follows the successful write of the final SDU.
// A record with Lost set is incomplete and must not enter measurements.
type WriteRecord struct {
	Metadata           WriteMetadata
	RequestedAt        time.Time
	EnqueueStartedAt   time.Time
	DequeuedAt         time.Time
	WriteStartedAt     time.Time
	WriteCompletedAt   time.Time
	FailedAt           time.Time
	Failure            WriteFailure
	FirstPayloadOffset int
	Lost               bool
}

// WriteRecorder retains a fixed number of message records without callbacks
// or export on a protocol or mux goroutine. It never overwrites a record.
// Create a new recorder for each measurement window; Snapshot does not reset
// capacity. A zero-value recorder rejects every observation.
type WriteRecorder struct {
	slots    []WriteTrace
	next     atomic.Uint64
	rejected atomic.Uint64
	lost     atomic.Uint64
}

// NewWriteRecorder allocates capacity records up front. The bound includes
// pending and completed messages and must be between 1 and 1,000,000.
func NewWriteRecorder(capacity int) (*WriteRecorder, error) {
	if capacity < 1 || capacity > 1_000_000 {
		return nil, errors.New("write recorder capacity must be 1..1000000")
	}
	return &WriteRecorder{slots: make([]WriteTrace, capacity)}, nil
}

// Begin reserves one record. Nil, exhausted, or invalid recorders return nil
// without changing transport behavior. Connection labels are limited to 1024
// bytes, and point hashes to origin or 32 bytes. Rejected reports lost attempts.
func (r *WriteRecorder) Begin(meta WriteMetadata) *WriteTrace {
	if r == nil {
		return nil
	}
	if len(meta.Connection) > 1024 ||
		(meta.Start.HashLength != 0 && meta.Start.HashLength != 32) ||
		(meta.End.HashLength != 0 && meta.End.HashLength != 32) {
		r.rejected.Add(1)
		return nil
	}
	i := r.next.Add(1) - 1
	if i >= uint64(len(r.slots)) {
		r.rejected.Add(1)
		return nil
	}
	t := &r.slots[i]
	t.recorder = r
	meta.Connection = strings.Clone(meta.Connection)
	t.record = WriteRecord{Metadata: meta, RequestedAt: time.Now()}
	t.ready.Store(true)
	return t
}

// Rejected reports attempts rejected by capacity or invalid metadata.
func (r *WriteRecorder) Rejected() uint64 { return r.rejected.Load() }

// Lost reports record updates discarded rather than waiting on a snapshot.
// Consumers must reject a measurement window when either loss count is nonzero.
func (r *WriteRecorder) Lost() uint64 { return r.lost.Load() }

// Snapshot copies initialized records in reservation order. It may run while
// traffic is active; record updates never wait for it. Final measurements need
// quiescent protocol/mux writers and zero Rejected and Lost counts.
func (r *WriteRecorder) Snapshot() []WriteRecord {
	ret := make([]WriteRecord, 0, min(r.next.Load(), uint64(len(r.slots))))
	for i := range r.slots {
		t := &r.slots[i]
		if !t.ready.Load() {
			continue
		}
		t.mu.Lock()
		record := t.record
		record.Lost = t.lost.Load()
		t.mu.Unlock()
		ret = append(ret, record)
	}
	return ret
}

// WriteTrace correlates one message through queuing, batching and fragments.
// Only a recorder creates traces. Callers must retire an abandoned queued
// message with Abandon; canceling an enqueue context after enqueue does not
// retract bytes that may already be owned by the muxer.
type WriteTrace struct {
	mu               sync.Mutex
	ready            atomic.Bool
	lost             atomic.Bool
	recorder         *WriteRecorder
	record           WriteRecord
	writing          bool
	abandonRequested bool
}

func (t *WriteTrace) lock() bool {
	if t == nil || !t.ready.Load() {
		return false
	}
	if !t.mu.TryLock() {
		t.lost.Store(true)
		t.recorder.lost.Add(1)
		return false
	}
	return true
}

// SetRequestID binds the protocol's assigned request identity before enqueue.
func (t *WriteTrace) SetRequestID(id uint64) {
	if t == nil || !t.lock() {
		return
	}
	if t.record.Metadata.RequestID == 0 && t.record.EnqueueStartedAt.IsZero() {
		t.record.Metadata.RequestID = id
	}
	t.mu.Unlock()
}

// Enqueue marks the start of offering the message to the protocol queue.
// It includes any wait for queue capacity; it is not a transport timestamp.
func (t *WriteTrace) Enqueue() {
	if t == nil || !t.lock() {
		return
	}
	if t.record.EnqueueStartedAt.IsZero() {
		t.record.EnqueueStartedAt = time.Now()
	}
	t.mu.Unlock()
}

// Dequeue marks the first removal from the protocol queue. Agency and mux
// scheduling can still delay the actual connection write.
func (t *WriteTrace) Dequeue() {
	if t == nil || !t.lock() {
		return
	}
	if t.record.DequeuedAt.IsZero() {
		t.record.DequeuedAt = time.Now()
	}
	t.mu.Unlock()
}

// Abandon retires an unowned message immediately. A segment dispatched to
// Muxer.Send retains its transport outcome: successful final-SDU completion
// wins over concurrent request retirement, while an incomplete message is
// abandoned after that segment finishes. It never retracts queued bytes.
func (t *WriteTrace) Abandon() {
	if t == nil || !t.lock() {
		return
	}
	if t.record.WriteCompletedAt.IsZero() && t.record.FailedAt.IsZero() {
		if t.writing {
			t.abandonRequested = true
		} else {
			t.record.FailedAt = time.Now()
			t.record.Failure = WriteFailureAbandoned
		}
	}
	t.mu.Unlock()
}

func (t *WriteTrace) claim() {
	if t == nil || !t.lock() {
		return
	}
	if t.record.WriteCompletedAt.IsZero() && t.record.FailedAt.IsZero() {
		t.writing = true
	}
	t.mu.Unlock()
}

// Fail records an enqueue or transport failure. Request owners use Abandon
// so retirement cannot override an in-flight final write. A completed write
// remains completed even if the peer later refuses the range.
func (t *WriteTrace) Fail(reason WriteFailure) {
	if t == nil || !t.lock() {
		return
	}
	t.writing = false
	if t.record.WriteCompletedAt.IsZero() && t.record.FailedAt.IsZero() {
		t.record.FailedAt = time.Now()
		t.record.Failure = reason
	}
	t.mu.Unlock()
}

type writeSpan struct {
	next   *writeSpan
	trace  *WriteTrace
	first  bool
	last   bool
	offset int
}

func (s writeSpan) started(at time.Time) {
	if !s.first || s.trace == nil || !s.trace.lock() {
		return
	}
	if s.trace.record.FailedAt.IsZero() &&
		s.trace.record.WriteStartedAt.IsZero() {
		s.trace.record.WriteStartedAt = at
		s.trace.record.FirstPayloadOffset = s.offset
	}
	s.trace.mu.Unlock()
}

func (s writeSpan) completed(at time.Time) {
	if s.trace == nil || !s.trace.lock() {
		return
	}
	s.trace.writing = false
	if s.trace.record.FailedAt.IsZero() {
		if s.last {
			s.trace.record.WriteCompletedAt = at
		} else if s.trace.abandonRequested {
			s.trace.record.FailedAt = at
			s.trace.record.Failure = WriteFailureAbandoned
		}
	}
	s.trace.mu.Unlock()
}
