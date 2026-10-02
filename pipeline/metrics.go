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

package pipeline

import (
	"sync"
	"sync/atomic"
	"time"
)

// stageTimer accumulates the durations of successfully processed items.
type stageTimer struct {
	count   atomic.Uint64
	totalNs atomic.Int64
	maxNs   atomic.Int64
}

// record adds one duration. Negative durations are clamped to zero.
func (t *stageTimer) record(d time.Duration) {
	ns := max(int64(d), 0)
	t.count.Add(1)
	t.totalNs.Add(ns)
	for {
		current := t.maxNs.Load()
		if ns <= current || t.maxNs.CompareAndSwap(current, ns) {
			return
		}
	}
}

func (t *stageTimer) snapshot() StageTimings {
	return StageTimings{
		Count: t.count.Load(),
		Total: time.Duration(t.totalNs.Load()),
		Max:   time.Duration(t.maxNs.Load()),
	}
}

func (t *stageTimer) reset() {
	t.count.Store(0)
	t.totalNs.Store(0)
	t.maxNs.Store(0)
}

// PipelineMetrics tracks metrics for the entire pipeline.
// Uses atomic counters for thread-safe operation.
type PipelineMetrics struct {
	// Counters (atomic)
	blocksSubmitted  atomic.Uint64
	blocksDecoded    atomic.Uint64
	blocksValidated  atomic.Uint64
	blocksApplied    atomic.Uint64
	decodeErrors     atomic.Uint64
	validationErrors atomic.Uint64
	applyErrors      atomic.Uint64

	// Per-stage durations of successful items (atomic)
	decodeTimer   stageTimer
	validateTimer stageTimer
	applyTimer    stageTimer

	// Queue tracking (requires mutex)
	mu                sync.RWMutex
	currentQueueDepth int
	peakQueueDepth    int

	// Timing
	lastBlockTime time.Time
	startTime     time.Time
}

// NewPipelineMetrics creates a new PipelineMetrics.
// The windowSize parameter is intentionally ignored (kept for API
// compatibility); stage timings are cumulative, not windowed.
func NewPipelineMetrics(windowSize int) *PipelineMetrics {
	return &PipelineMetrics{
		startTime: time.Now(),
	}
}

// RecordSubmit increments the submitted counter.
func (m *PipelineMetrics) RecordSubmit() {
	m.blocksSubmitted.Add(1)
}

// RecordDecode records a decode result. The duration is retained only for
// successful decodes.
func (m *PipelineMetrics) RecordDecode(duration time.Duration, err error) {
	if err != nil {
		m.decodeErrors.Add(1)
	} else {
		m.blocksDecoded.Add(1)
		m.decodeTimer.record(duration)
	}
}

// RecordValidate records a validation result. The duration is retained only
// for successful validations.
func (m *PipelineMetrics) RecordValidate(duration time.Duration, err error) {
	if err != nil {
		m.validationErrors.Add(1)
	} else {
		m.blocksValidated.Add(1)
		m.validateTimer.record(duration)
	}
}

// RecordApply records an apply result. The duration is retained only for
// successful applies.
func (m *PipelineMetrics) RecordApply(duration time.Duration, err error) {
	if err != nil {
		m.applyErrors.Add(1)
	} else {
		m.blocksApplied.Add(1)
		m.applyTimer.record(duration)
		m.mu.Lock()
		m.lastBlockTime = time.Now()
		m.mu.Unlock()
	}
}

// RecordPipelineLatency records end-to-end pipeline latency.
// It is intentionally a no-op: per-stage durations are tracked by
// RecordDecode, RecordValidate and RecordApply instead.
func (m *PipelineMetrics) RecordPipelineLatency(duration time.Duration) {
	// No-op: latency tracking removed
}

// UpdateQueueDepth updates the queue depth tracking.
func (m *PipelineMetrics) UpdateQueueDepth(depth int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentQueueDepth = depth
	if depth > m.peakQueueDepth {
		m.peakQueueDepth = depth
	}
}

// Stats returns a snapshot of the current metrics.
func (m *PipelineMetrics) Stats() PipelineStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return PipelineStats{
		BlocksSubmitted:   m.blocksSubmitted.Load(),
		BlocksDecoded:     m.blocksDecoded.Load(),
		BlocksValidated:   m.blocksValidated.Load(),
		BlocksApplied:     m.blocksApplied.Load(),
		DecodeErrors:      m.decodeErrors.Load(),
		ValidationErrors:  m.validationErrors.Load(),
		ApplyErrors:       m.applyErrors.Load(),
		DecodeTimings:     m.decodeTimer.snapshot(),
		ValidateTimings:   m.validateTimer.snapshot(),
		ApplyTimings:      m.applyTimer.snapshot(),
		CurrentQueueDepth: m.currentQueueDepth,
		PeakQueueDepth:    m.peakQueueDepth,
		LastBlockTime:     m.lastBlockTime,
		StartTime:         m.startTime,
	}
}

// Reset resets all metrics.
func (m *PipelineMetrics) Reset() {
	m.blocksSubmitted.Store(0)
	m.blocksDecoded.Store(0)
	m.blocksValidated.Store(0)
	m.blocksApplied.Store(0)
	m.decodeErrors.Store(0)
	m.validationErrors.Store(0)
	m.applyErrors.Store(0)
	m.decodeTimer.reset()
	m.validateTimer.reset()
	m.applyTimer.reset()

	m.mu.Lock()
	m.currentQueueDepth = 0
	m.peakQueueDepth = 0
	m.lastBlockTime = time.Time{}
	m.startTime = time.Now()
	m.mu.Unlock()
}
