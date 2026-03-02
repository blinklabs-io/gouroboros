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

// ErrMissingEta0Provider is returned when validation is enabled but no Eta0Provider is configured.
var ErrMissingEta0Provider = errors.New("pipeline: validation enabled but Eta0Provider not configured")

// closedResultsChan is a closed channel returned by Results() before Start() is called.
// This prevents callers from blocking indefinitely on a nil channel.
var closedResultsChan = func() <-chan *BlockItem {
	ch := make(chan *BlockItem)
	close(ch)
	return ch
}()

// newNotStartedErrorsChan creates a fresh channel that yields ErrPipelineNotStarted once.
// Each call creates a new channel to ensure all callers receive the error.
func newNotStartedErrorsChan() <-chan error {
	ch := make(chan error, 1)
	ch <- ErrPipelineNotStarted
	close(ch)
	return ch
}

// BlockPipeline orchestrates the block processing pipeline.
type BlockPipeline struct {
	config PipelineConfig

	// Stages
	decodeStage   *DecodeStage
	validateStage *ValidateStage
	applyStage    *ApplyStage

	// Worker pools and runners
	decodePool   *StageWorkerPool
	validatePool *StageWorkerPool
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

// NewBlockPipeline creates a new BlockPipeline using functional options.
// Use With* options to customize the pipeline configuration.
//
// Example:
//
//	p := NewBlockPipeline(
//	    WithDecodeWorkers(4),
//	    WithApplyFunc(myApplyFunc),
//	)
func NewBlockPipeline(opts ...PipelineOption) *BlockPipeline {
	config := DefaultPipelineConfig()
	for _, opt := range opts {
		opt(&config)
	}
	return &BlockPipeline{
		config:  config,
		metrics: NewPipelineMetrics(config.MetricsWindowSize),
	}
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

	// Validate configuration
	validationEnabled := p.config.ValidateWorkers > 0
	if validationEnabled && p.config.Eta0Provider == nil {
		return ErrMissingEta0Provider
	}

	// Create cancellable context
	p.ctx, p.cancel = context.WithCancel(ctx)

	// Create channels
	bufSize := p.config.PrefetchBufferSize
	p.submitChan = make(chan *BlockItem, bufSize)
	p.decodedChan = make(chan *BlockItem, bufSize)
	p.resultsChan = make(chan *BlockItem, bufSize)
	p.errorsChan = make(chan error, bufSize)

	// Create decode stage
	p.decodeStage = NewDecodeStage(p.config.SkipBodyHashValidation)
	p.applyStage = NewApplyStage(p.config.ApplyFunc, p.config.MaxPendingBlocks)

	// Create decode worker pool
	p.decodePool = NewStageWorkerPool(StageWorkerPoolConfig{
		Stage:         p.decodeStage,
		NumWorkers:    p.config.DecodeWorkers,
		Input:         p.submitChan,
		Output:        p.decodedChan,
		Errors:        p.errorsChan,
		RecordMetrics: DecodeMetricsRecorder(p.metrics),
	})

	// Determine input channel for apply stage
	var applyInput <-chan *BlockItem

	if validationEnabled {
		// Create validation stage
		p.validatedChan = make(chan *BlockItem, bufSize)
		p.validateStage = NewValidateStage(ValidateStageConfig{
			Eta0Provider:      p.config.Eta0Provider,
			SlotsPerKesPeriod: p.config.SlotsPerKesPeriod,
			VerifyConfig:      p.config.VerifyConfig,
		})
		p.validatePool = NewStageWorkerPool(StageWorkerPoolConfig{
			Stage:         p.validateStage,
			NumWorkers:    p.config.ValidateWorkers,
			Input:         p.decodedChan,
			Output:        p.validatedChan,
			Errors:        p.errorsChan,
			RecordMetrics: ValidateMetricsRecorder(p.metrics),
			ShouldRecord:  RecordIfDecoded,
		})
		applyInput = p.validatedChan
	} else {
		// Skip validation - decoded blocks go directly to apply
		applyInput = p.decodedChan
	}

	p.applyRunner = NewApplyStageRunner(
		p.applyStage,
		applyInput,
		p.resultsChan,
		p.errorsChan,
		bufSize, // Deprecated: pendingQueueSize is no longer used (kept for API compatibility)
	)
	p.applyRunner.SetMetrics(p.metrics)

	// Start all stages
	// Note: p.ctx is derived from the passed ctx via context.WithCancel above
	p.decodePool.Start(p.ctx) //nolint:contextcheck
	if validationEnabled {
		p.validatePool.Start(p.ctx) //nolint:contextcheck
	}
	p.applyRunner.Start(p.ctx) //nolint:contextcheck

	// Start metrics collection goroutine
	p.wg.Add(1)
	go p.metricsCollector()

	p.started.Store(true)
	return nil
}

// Submit submits a new block for processing.
// This method is safe to call concurrently with Stop().
// The context allows callers to handle timeouts or cancellations when the
// pipeline is full and applying backpressure.
func (p *BlockPipeline) Submit(ctx context.Context, blockType uint, rawCbor []byte, tip pcommon.Tip) error {
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

	// Allocate sequence number only once, then send.
	// We use a single blocking select to avoid sequence gaps that would occur
	// if we allocated in a non-blocking attempt that failed.
	item := NewBlockItem(blockType, rawCbor, tip, atomic.AddUint64(&p.sequenceCounter, 1)-1)

	select {
	case p.submitChan <- item:
		p.metrics.RecordSubmit()
		return nil
	case <-ctx.Done():
		// Context cancelled while waiting - sequence gap is acceptable
		// because this typically means shutdown.
		return ctx.Err()
	case <-p.ctx.Done():
		return ErrPipelineStopped
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
		return newNotStartedErrorsChan()
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

	// Wait for validate workers to finish (if validation is enabled)
	if p.validatePool != nil {
		p.validatePool.Stop()
		close(p.validatedChan)
	}

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
