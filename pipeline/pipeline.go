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
	"time"

	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// ErrPipelineStopped is returned when trying to submit to a stopped pipeline.
var ErrPipelineStopped = errors.New("pipeline is stopped")

// ErrPipelineNotStarted is returned when trying to use a pipeline that hasn't been started.
var ErrPipelineNotStarted = errors.New("pipeline not started")

// closedResultsChan is a closed channel returned by Results() before Start() is called.
// This prevents callers from blocking indefinitely on a nil channel.
var closedResultsChan = func() <-chan *BlockItem {
	ch := make(chan *BlockItem)
	close(ch)
	return ch
}()

// notStartedErrorsChan is a channel that yields ErrPipelineNotStarted once, then closes.
// This is returned by Errors() before Start() is called.
var notStartedErrorsChan = func() <-chan error {
	ch := make(chan error, 1)
	ch <- ErrPipelineNotStarted
	close(ch)
	return ch
}()

// BlockPipeline orchestrates the block processing pipeline.
type BlockPipeline struct {
	config PipelineConfig

	// Stages
	decodeStage   *DecodeStage
	validateStage *ValidateStage
	applyStage    *ApplyStage

	// Worker pools and runners
	decodePool   *DecodeStageWorkerPool
	validatePool *ValidateStageWorkerPool
	applyRunner  *ApplyStageRunner

	// Channels
	submitChan    chan *BlockItem
	decodedChan   chan *BlockItem
	validatedChan chan *BlockItem
	resultsChan   chan *BlockItem
	errorsChan    chan error

	// Metrics
	metrics *PipelineMetrics

	// State
	sequenceCounter uint64
	ctx             context.Context
	cancel          context.CancelFunc
	started         atomic.Bool
	stopped         atomic.Bool
	wg              sync.WaitGroup
	mu              sync.Mutex   // protects Start/Stop
	submitMu        sync.RWMutex // protects Submit against concurrent Stop
}

// NewBlockPipeline creates a new BlockPipeline with the given configuration.
func NewBlockPipeline(config PipelineConfig) *BlockPipeline {
	return &BlockPipeline{
		config:  config,
		metrics: NewPipelineMetrics(config.MetricsWindowSize),
	}
}

// NewBlockPipelineWithOptions creates a new BlockPipeline using functional options.
func NewBlockPipelineWithOptions(opts ...PipelineOption) *BlockPipeline {
	config := DefaultPipelineConfig()
	for _, opt := range opts {
		opt(&config)
	}
	return NewBlockPipeline(config)
}

// Start starts the pipeline processing.
func (p *BlockPipeline) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopped.Load() {
		return ErrPipelineStopped
	}

	if p.started.Load() {
		return nil // Already started
	}

	// Create cancellable context
	p.ctx, p.cancel = context.WithCancel(ctx)

	// Create channels
	bufSize := p.config.PrefetchBufferSize
	p.submitChan = make(chan *BlockItem, bufSize)
	p.decodedChan = make(chan *BlockItem, bufSize)
	p.validatedChan = make(chan *BlockItem, bufSize)
	p.resultsChan = make(chan *BlockItem, bufSize)
	p.errorsChan = make(chan error, bufSize)

	// Create stages
	p.decodeStage = NewDecodeStage(p.config.SkipBodyHashValidation)
	p.validateStage = NewValidateStage(p.config.ValidateConfig)
	p.applyStage = NewApplyStage(p.config.ApplyFunc)

	// Create worker pools
	p.decodePool = NewDecodeStageWorkerPool(
		p.decodeStage,
		p.config.DecodeWorkers,
		p.submitChan,
		p.decodedChan,
		p.errorsChan,
	)
	p.decodePool.SetMetrics(p.metrics)

	p.validatePool = NewValidateStageWorkerPool(
		p.validateStage,
		p.config.ValidateWorkers,
		p.decodedChan,
		p.validatedChan,
		p.errorsChan,
	)
	p.validatePool.SetMetrics(p.metrics)

	p.applyRunner = NewApplyStageRunner(
		p.applyStage,
		p.validatedChan,
		p.resultsChan,
		p.errorsChan,
		bufSize, // Use same buffer size for pending queue to avoid data loss
	)
	p.applyRunner.SetMetrics(p.metrics)

	// Start all stages
	// Note: p.ctx is derived from the passed ctx via context.WithCancel above
	p.decodePool.Start(p.ctx)   //nolint:contextcheck
	p.validatePool.Start(p.ctx) //nolint:contextcheck
	p.applyRunner.Start(p.ctx)  //nolint:contextcheck

	// Start metrics collection goroutine
	p.wg.Add(1)
	go p.metricsCollector()

	p.started.Store(true)
	return nil
}

// Submit submits a new block for processing.
// This method is safe to call concurrently with Stop().
func (p *BlockPipeline) Submit(blockType uint, rawCbor []byte, tip pcommon.Tip) error {
	// Early checks for common cases (before acquiring lock)
	if !p.started.Load() {
		return ErrPipelineNotStarted
	}

	// RLock allows concurrent submits while preventing races with Stop().
	// This is critical: between the stopped check and channel send, Stop() could
	// close submitChan causing a panic. The RLock ensures Stop() waits until
	// all in-flight submits complete before closing the channel.
	p.submitMu.RLock()
	defer p.submitMu.RUnlock()

	// Check stopped under lock to ensure we don't race with Stop()
	if p.stopped.Load() {
		return ErrPipelineStopped
	}

	seq := atomic.AddUint64(&p.sequenceCounter, 1) - 1
	item := NewBlockItem(blockType, rawCbor, tip, seq)

	p.metrics.RecordSubmit()

	select {
	case p.submitChan <- item:
		return nil
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
}

// Results returns a channel of successfully processed block items.
// If the pipeline has not been started, returns a closed channel to prevent blocking.
func (p *BlockPipeline) Results() <-chan *BlockItem {
	if !p.started.Load() {
		return closedResultsChan
	}
	return p.resultsChan
}

// Errors returns a channel of processing errors.
// If the pipeline has not been started, returns a channel that yields
// ErrPipelineNotStarted once and then closes.
func (p *BlockPipeline) Errors() <-chan error {
	if !p.started.Load() {
		return notStartedErrorsChan
	}
	return p.errorsChan
}

// Stop gracefully stops the pipeline.
func (p *BlockPipeline) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started.Load() || p.stopped.Load() {
		return nil
	}

	// Cancel context FIRST to unblock any Submit() calls waiting on channel send.
	// This must happen before acquiring submitMu.Lock() to avoid deadlock:
	// Submit() holds RLock while blocking on channel, and we need it to unblock
	// via ctx.Done() before we can acquire the write lock.
	p.cancel()

	// Now acquire write lock to ensure no Submit() calls are in progress.
	// Any Submit() blocked on channel send will now return via ctx.Done().
	p.submitMu.Lock()
	p.stopped.Store(true)
	// Close input channel to signal shutdown
	close(p.submitChan)
	p.submitMu.Unlock()

	// Wait for decode workers to finish
	p.decodePool.Stop()
	close(p.decodedChan)

	// Wait for validate workers to finish
	p.validatePool.Stop()
	close(p.validatedChan)

	// Wait for apply runner to finish
	p.applyRunner.Stop()

	// Close output channels
	close(p.resultsChan)
	close(p.errorsChan)

	// Wait for metrics collector
	p.wg.Wait()

	return nil
}

// Stats returns the current pipeline statistics.
func (p *BlockPipeline) Stats() PipelineStats {
	return p.metrics.Stats()
}

// PendingCount returns the approximate number of items still being processed.
// This includes items in inter-stage channels and items buffered in the apply stage.
// Useful for coordinating with rollback operations.
func (p *BlockPipeline) PendingCount() int {
	if !p.started.Load() {
		return 0
	}
	channelDepth := len(p.submitChan) + len(p.decodedChan) + len(p.validatedChan)
	applyPending := 0
	if p.applyStage != nil {
		applyPending = p.applyStage.PendingCount()
	}
	return channelDepth + applyPending
}

// WaitForDrain blocks until all currently submitted items have been processed
// or the context is cancelled. This is useful before handling rollbacks to
// ensure no blocks are applied after the rollback.
func (p *BlockPipeline) WaitForDrain(ctx context.Context) error {
	if !p.started.Load() {
		return ErrPipelineNotStarted
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if p.PendingCount() == 0 {
				return nil
			}
		}
	}
}

// metricsCollector collects metrics from processed items.
func (p *BlockPipeline) metricsCollector() {
	defer p.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			// Update queue depth
			depth := len(p.submitChan) + len(p.decodedChan) + len(p.validatedChan)
			p.metrics.UpdateQueueDepth(depth)
		}
	}
}

// DrainResults reads all available results without blocking.
// Useful for testing or cleanup.
func (p *BlockPipeline) DrainResults() []*BlockItem {
	var results []*BlockItem
	for {
		select {
		case item, ok := <-p.resultsChan:
			if !ok {
				return results
			}
			results = append(results, item)
		default:
			return results
		}
	}
}

// DrainErrors reads all available errors without blocking.
// Useful for testing or cleanup.
func (p *BlockPipeline) DrainErrors() []error {
	var errs []error
	for {
		select {
		case err, ok := <-p.errorsChan:
			if !ok {
				return errs
			}
			errs = append(errs, err)
		default:
			return errs
		}
	}
}
