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

	// Queue tracking (requires mutex)
	mu                sync.RWMutex
	currentQueueDepth int
	peakQueueDepth    int

	// Timing
	lastBlockTime time.Time
	startTime     time.Time
}

// NewPipelineMetrics creates a new PipelineMetrics.
// The windowSize parameter is ignored (kept for API compatibility).
func NewPipelineMetrics(windowSize int) *PipelineMetrics {
	return &PipelineMetrics{
		startTime: time.Now(),
	}
}

// RecordSubmit increments the submitted counter.
func (m *PipelineMetrics) RecordSubmit() {
	m.blocksSubmitted.Add(1)
}

// RecordDecode records a decode result.
func (m *PipelineMetrics) RecordDecode(duration time.Duration, err error) {
	if err != nil {
		m.decodeErrors.Add(1)
	} else {
		m.blocksDecoded.Add(1)
	}
}

// RecordValidate records a validation result.
func (m *PipelineMetrics) RecordValidate(duration time.Duration, err error) {
	if err != nil {
		m.validationErrors.Add(1)
	} else {
		m.blocksValidated.Add(1)
	}
}

// RecordApply records an apply result.
func (m *PipelineMetrics) RecordApply(duration time.Duration, err error) {
	if err != nil {
		m.applyErrors.Add(1)
	} else {
		m.blocksApplied.Add(1)
		m.mu.Lock()
		m.lastBlockTime = time.Now()
		m.mu.Unlock()
	}
}

// RecordPipelineLatency records end-to-end pipeline latency.
// This is a no-op since we removed latency tracking.
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

	m.mu.Lock()
	m.currentQueueDepth = 0
	m.peakQueueDepth = 0
	m.lastBlockTime = time.Time{}
	m.startTime = time.Now()
	m.mu.Unlock()
}
