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
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// MetricsRecorder is a function that records metrics for a processed block item.
// It receives the item that was processed and the error (if any) from processing.
// The function should extract the appropriate duration from the item based on
// which stage recorded the timing (e.g., DecodeDuration, ValidateDuration).
type MetricsRecorder func(item *BlockItem, err error)

// ShouldRecordMetrics is a function that determines whether metrics should be
// recorded for a given block item. This allows stages to skip metric recording
// for items that weren't actually processed (e.g., validation skipping items
// that failed decode).
type ShouldRecordMetrics func(item *BlockItem) bool

// StageWorkerPool runs multiple workers in parallel for a given stage.
// This is a generic worker pool that can be used with any stage implementing
// the Stage interface.
type StageWorkerPool struct {
	stage         Stage
	numWorkers    int
	input         <-chan *BlockItem
	output        chan<- *BlockItem
	errors        chan<- error
	recordMetrics MetricsRecorder
	shouldRecord  ShouldRecordMetrics
	wg            sync.WaitGroup
	started       atomic.Bool
}

// StageWorkerPoolConfig holds configuration for creating a StageWorkerPool.
type StageWorkerPoolConfig struct {
	// Stage is the processing stage to use (required, panics if nil).
	Stage Stage
	// NumWorkers is the number of parallel workers; defaults to 1 if <= 0.
	NumWorkers int
	// Input is the channel to receive block items from.
	Input <-chan *BlockItem
	// Output is the channel to send processed items to.
	Output chan<- *BlockItem
	// Errors is the channel to send errors to; may be nil.
	Errors chan<- error
	// RecordMetrics is called after processing to record metrics.
	// If nil, no metrics are recorded.
	RecordMetrics MetricsRecorder
	// ShouldRecord determines whether to record metrics for an item.
	// If nil, metrics are recorded for all items.
	ShouldRecord ShouldRecordMetrics
}

// NewStageWorkerPool creates a new worker pool for the given stage.
//
// Parameters:
//   - config: Configuration for the worker pool (Stage is required)
//
// Note: If input or output channels are nil, workers will block indefinitely
// when attempting to receive or send items.
func NewStageWorkerPool(config StageWorkerPoolConfig) *StageWorkerPool {
	if config.Stage == nil {
		panic(ErrNilStage)
	}
	numWorkers := config.NumWorkers
	if numWorkers <= 0 {
		numWorkers = 1
	}
	return &StageWorkerPool{
		stage:         config.Stage,
		numWorkers:    numWorkers,
		input:         config.Input,
		output:        config.Output,
		errors:        config.Errors,
		recordMetrics: config.RecordMetrics,
		shouldRecord:  config.ShouldRecord,
	}
}

// Start starts the worker pool. Call Stop to wait for completion.
// This method is idempotent - calling it multiple times has no effect.
func (p *StageWorkerPool) Start(ctx context.Context) {
	if p.started.Swap(true) {
		return // Already started
	}
	for i := 0; i < p.numWorkers; i++ {
		p.wg.Add(1)
		go p.worker(ctx)
	}
}

// Stop waits for all workers to complete.
func (p *StageWorkerPool) Stop() {
	p.wg.Wait()
}

func (p *StageWorkerPool) worker(ctx context.Context) {
	defer p.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case item, ok := <-p.input:
			if !ok {
				return
			}

			err := p.stage.Process(ctx, item)

			// Record metrics only for actual processing attempts (not context cancellation)
			// and only if the shouldRecord check passes (or is nil)
			if p.recordMetrics != nil &&
				!errors.Is(err, context.Canceled) &&
				!errors.Is(err, context.DeadlineExceeded) &&
				(p.shouldRecord == nil || p.shouldRecord(item)) {
				p.recordMetrics(item, err)
			}

			if err != nil && p.errors != nil {
				// Send error but still forward item for tracking
				select {
				case p.errors <- err:
				case <-ctx.Done():
					return
				}
			}

			// Forward to next stage (even on error, for stats tracking)
			select {
			case p.output <- item:
			case <-ctx.Done():
				return
			}
		}
	}
}

// DecodeMetricsRecorder returns a MetricsRecorder for the decode stage.
func DecodeMetricsRecorder(metrics *PipelineMetrics) MetricsRecorder {
	if metrics == nil {
		return nil
	}
	return func(item *BlockItem, err error) {
		metrics.RecordDecode(item.DecodeDuration(), err)
	}
}

// ValidateMetricsRecorder returns a MetricsRecorder for the validate stage.
func ValidateMetricsRecorder(metrics *PipelineMetrics) MetricsRecorder {
	if metrics == nil {
		return nil
	}
	return func(item *BlockItem, err error) {
		metrics.RecordValidate(item.ValidateDuration(), err)
	}
}

// ApplyMetricsRecorder returns a MetricsRecorder for the apply stage.
func ApplyMetricsRecorder(metrics *PipelineMetrics) MetricsRecorder {
	if metrics == nil {
		return nil
	}
	return func(item *BlockItem, err error) {
		metrics.RecordApply(item.ApplyDuration(), err)
	}
}

// AlwaysRecordMetrics is a ShouldRecordMetrics that always returns true.
func AlwaysRecordMetrics(item *BlockItem) bool {
	return true
}

// RecordIfDecoded is a ShouldRecordMetrics that only records metrics
// if the item was successfully decoded.
func RecordIfDecoded(item *BlockItem) bool {
	return item.IsDecoded()
}
