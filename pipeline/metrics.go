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
	"sort"
	"sync"
	"time"
)

// LatencyTracker tracks latency samples in a sliding window.
type LatencyTracker struct {
	mu       sync.Mutex
	samples  []time.Duration
	maxSize  int
	position int
	filled   bool
}

// NewLatencyTracker creates a new latency tracker with the given window size.
func NewLatencyTracker(windowSize int) *LatencyTracker {
	if windowSize <= 0 {
		windowSize = 1000
	}
	return &LatencyTracker{
		samples: make([]time.Duration, windowSize),
		maxSize: windowSize,
	}
}

// Add adds a latency sample.
func (t *LatencyTracker) Add(d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.samples[t.position] = d
	t.position++
	if t.position >= t.maxSize {
		t.position = 0
		t.filled = true
	}
}

// Percentile returns the p-th percentile of the samples.
// p should be between 0 and 100; values outside this range are clamped.
func (t *LatencyTracker) Percentile(p float64) time.Duration {
	// Clamp p to valid range
	if p < 0 {
		p = 0
	} else if p > 100 {
		p = 100
	}

	t.mu.Lock()
	count := t.count()
	if count == 0 {
		t.mu.Unlock()
		return 0
	}

	// Copy samples while holding lock
	sorted := make([]time.Duration, count)
	copy(sorted, t.samples[:count])
	t.mu.Unlock()

	// Sort outside the lock to avoid blocking Add calls
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	// Calculate percentile index
	idx := int(float64(count-1) * p / 100.0)
	if idx >= count {
		idx = count - 1
	}
	return sorted[idx]
}

// Max returns the maximum latency in the window.
func (t *LatencyTracker) Max() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	count := t.count()
	if count == 0 {
		return 0
	}

	var max time.Duration
	for i := 0; i < count; i++ {
		if t.samples[i] > max {
			max = t.samples[i]
		}
	}
	return max
}

// Avg returns the average latency in the window.
func (t *LatencyTracker) Avg() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	count := t.count()
	if count == 0 {
		return 0
	}

	var sum time.Duration
	for i := 0; i < count; i++ {
		sum += t.samples[i]
	}
	return sum / time.Duration(count)
}

// Stats returns a LatencyStats snapshot.
// This method acquires the lock once, copies the data, and computes all stats
// from the copy to avoid repeated lock acquisition and sorting.
func (t *LatencyTracker) Stats() LatencyStats {
	t.mu.Lock()
	count := t.count()
	if count == 0 {
		t.mu.Unlock()
		return LatencyStats{}
	}

	// Copy samples while holding lock
	samples := make([]time.Duration, count)
	copy(samples, t.samples[:count])
	t.mu.Unlock()

	// Sort once for all percentile calculations
	sort.Slice(samples, func(i, j int) bool {
		return samples[i] < samples[j]
	})

	// Calculate all stats from the sorted copy
	var sum time.Duration
	var max time.Duration
	for _, s := range samples {
		sum += s
		if s > max {
			max = s
		}
	}

	return LatencyStats{
		P50: samples[int(float64(count-1)*0.50)],
		P90: samples[int(float64(count-1)*0.90)],
		P99: samples[int(float64(count-1)*0.99)],
		Max: max,
		Avg: sum / time.Duration(count),
	}
}

// count returns the number of samples (must hold lock).
func (t *LatencyTracker) count() int {
	if t.filled {
		return t.maxSize
	}
	return t.position
}

// Reset clears all samples.
func (t *LatencyTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.position = 0
	t.filled = false
}

// PipelineMetrics tracks metrics for the entire pipeline.
type PipelineMetrics struct {
	mu sync.RWMutex

	// Counters
	blocksSubmitted  uint64
	blocksDecoded    uint64
	blocksValidated  uint64
	blocksApplied    uint64
	decodeErrors     uint64
	validationErrors uint64
	applyErrors      uint64

	// Latency trackers
	decodeLatency   *LatencyTracker
	validateLatency *LatencyTracker
	applyLatency    *LatencyTracker
	pipelineLatency *LatencyTracker

	// Queue tracking
	currentQueueDepth int
	peakQueueDepth    int

	// Timing
	lastBlockTime time.Time
	startTime     time.Time
}

// NewPipelineMetrics creates a new PipelineMetrics.
func NewPipelineMetrics(windowSize int) *PipelineMetrics {
	return &PipelineMetrics{
		decodeLatency:   NewLatencyTracker(windowSize),
		validateLatency: NewLatencyTracker(windowSize),
		applyLatency:    NewLatencyTracker(windowSize),
		pipelineLatency: NewLatencyTracker(windowSize),
		startTime:       time.Now(),
	}
}

// RecordSubmit increments the submitted counter.
func (m *PipelineMetrics) RecordSubmit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocksSubmitted++
}

// RecordDecode records a decode result.
func (m *PipelineMetrics) RecordDecode(duration time.Duration, err error) {
	m.mu.Lock()
	if err != nil {
		m.decodeErrors++
	} else {
		m.blocksDecoded++
	}
	m.mu.Unlock()
	m.decodeLatency.Add(duration)
}

// RecordValidate records a validation result.
func (m *PipelineMetrics) RecordValidate(duration time.Duration, err error) {
	m.mu.Lock()
	if err != nil {
		m.validationErrors++
	} else {
		m.blocksValidated++
	}
	m.mu.Unlock()
	m.validateLatency.Add(duration)
}

// RecordApply records an apply result.
func (m *PipelineMetrics) RecordApply(duration time.Duration, err error) {
	m.mu.Lock()
	if err != nil {
		m.applyErrors++
	} else {
		m.blocksApplied++
	}
	m.lastBlockTime = time.Now()
	m.mu.Unlock()
	m.applyLatency.Add(duration)
}

// RecordPipelineLatency records end-to-end pipeline latency.
func (m *PipelineMetrics) RecordPipelineLatency(duration time.Duration) {
	m.pipelineLatency.Add(duration)
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
	// Copy counter values while holding lock
	m.mu.RLock()
	stats := PipelineStats{
		BlocksSubmitted:   m.blocksSubmitted,
		BlocksDecoded:     m.blocksDecoded,
		BlocksValidated:   m.blocksValidated,
		BlocksApplied:     m.blocksApplied,
		DecodeErrors:      m.decodeErrors,
		ValidationErrors:  m.validationErrors,
		ApplyErrors:       m.applyErrors,
		CurrentQueueDepth: m.currentQueueDepth,
		PeakQueueDepth:    m.peakQueueDepth,
		LastBlockTime:     m.lastBlockTime,
		StartTime:         m.startTime,
	}
	m.mu.RUnlock()

	// Calculate latency stats outside the lock (each tracker has its own lock)
	stats.DecodeLatency = m.decodeLatency.Stats()
	stats.ValidateLatency = m.validateLatency.Stats()
	stats.ApplyLatency = m.applyLatency.Stats()
	stats.PipelineLatency = m.pipelineLatency.Stats()

	return stats
}

// Reset resets all metrics.
func (m *PipelineMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.blocksSubmitted = 0
	m.blocksDecoded = 0
	m.blocksValidated = 0
	m.blocksApplied = 0
	m.decodeErrors = 0
	m.validationErrors = 0
	m.applyErrors = 0
	m.currentQueueDepth = 0
	m.peakQueueDepth = 0
	m.lastBlockTime = time.Time{}
	m.startTime = time.Now()

	m.decodeLatency.Reset()
	m.validateLatency.Reset()
	m.applyLatency.Reset()
	m.pipelineLatency.Reset()
}
