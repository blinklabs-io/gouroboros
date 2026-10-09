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
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// ErrPipelineStopped is returned when trying to submit to a stopped pipeline.
var ErrPipelineStopped = errors.New("pipeline is stopped")

// ErrPipelineNotStarted is returned when trying to use a pipeline that hasn't been started.
var ErrPipelineNotStarted = errors.New("pipeline not started")

// ErrMissingEta0Provider is returned when validation is enabled but no Eta0Provider is configured.
var ErrMissingEta0Provider = errors.New(
	"pipeline: validation enabled but Eta0Provider not configured",
)

// ErrMissingBlockTypeResolver is returned when normal processing cannot bind
// a signed block header to the authoritative active era before typed decode.
var ErrMissingBlockTypeResolver = errors.New(
	"pipeline: block type resolver not configured",
)

// ErrInvalidSlotsPerKesPeriod is returned when validation cannot calculate a
// KES period because its configured slot interval is zero.
var ErrInvalidSlotsPerKesPeriod = errors.New(
	"pipeline: SlotsPerKesPeriod must be greater than zero",
)

// ErrMissingChainContextValidator is returned when ordered application lacks
// an authoritative chain-context validator.
var ErrMissingChainContextValidator = errors.New(
	"pipeline: chain-context validator not configured",
)

// ErrMissingApplyFunc is returned when ordered validation has no authoritative
// state commit function.
var ErrMissingApplyFunc = errors.New(
	"pipeline: apply function not configured",
)

// ErrBlockValidationRequired is returned when a non-decode-only pipeline has
// no block-local validation workers.
var ErrBlockValidationRequired = errors.New(
	"pipeline: block-local validation is required",
)

// ErrTrustedDecodeOnlyApply is returned when trusted decode-only mode is
// combined with an ApplyFunc.
var ErrTrustedDecodeOnlyApply = errors.New(
	"pipeline: trusted decode-only mode cannot apply blocks",
)

// ErrTrustedDecodeOnlyValidation is returned when trusted decode-only mode is
// combined with validation workers.
var ErrTrustedDecodeOnlyValidation = errors.New(
	"pipeline: trusted decode-only mode cannot run validation workers",
)

// ErrTrustedDecodeOnlyChainContext is returned when trusted decode-only mode
// is combined with a chain-context validator.
var ErrTrustedDecodeOnlyChainContext = errors.New(
	"pipeline: trusted decode-only mode cannot validate chain context",
)

// ErrTrustedDecodeOnlyBlockTypeResolver is returned when trusted decode-only
// mode is combined with an authoritative block type resolver.
var ErrTrustedDecodeOnlyBlockTypeResolver = errors.New(
	"pipeline: trusted decode-only mode cannot resolve authoritative block types",
)

// TrustedDecodeOnlyChainContextError identifies an invalid trusted decode-only
// configuration that also supplies a chain-context validator.
type TrustedDecodeOnlyChainContextError struct{}

func (*TrustedDecodeOnlyChainContextError) Error() string {
	return ErrTrustedDecodeOnlyChainContext.Error()
}

func (*TrustedDecodeOnlyChainContextError) Unwrap() error {
	return ErrTrustedDecodeOnlyChainContext
}

// ErrBodyHashValidationRequired is returned when a normal pipeline attempts
// to skip block body binding during decode.
var ErrBodyHashValidationRequired = errors.New(
	"pipeline: body hash validation is required",
)

// ErrValidationBypassConfigured is returned when normal mode configures a
// block-validation bypass.
var ErrValidationBypassConfigured = errors.New(
	"pipeline: block-validation bypass configured",
)

// ValidationBypassError identifies the VerifyConfig flag that would bypass a
// required normal-mode validation.
type ValidationBypassError struct {
	Flag string
}

func (e *ValidationBypassError) Error() string {
	return fmt.Sprintf("%s: VerifyConfig.%s", ErrValidationBypassConfigured, e.Flag)
}

func (*ValidationBypassError) Unwrap() error {
	return ErrValidationBypassConfigured
}

func validationBypassFlag(config lcommon.VerifyConfig) string {
	switch {
	case config.SkipHeaderValidation:
		return "SkipHeaderValidation"
	case config.SkipBodyHashValidation:
		return "SkipBodyHashValidation"
	case config.SkipTransactionValidation:
		return "SkipTransactionValidation"
	case config.SkipStakePoolValidation:
		return "SkipStakePoolValidation"
	case config.SkipBlockLimitsValidation:
		return "SkipBlockLimitsValidation"
	default:
		return ""
	}
}

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
	sequenceCounter   atomic.Uint64
	completedSequence atomic.Uint64
	ctx               context.Context
	cancel            context.CancelFunc
	started           atomic.Bool
	stopping          atomic.Bool
	stopped           atomic.Bool
	wg                sync.WaitGroup
	mu                sync.Mutex // protects Start/Stop
	fatalErrMu        sync.RWMutex
	fatalErr          error
	// submitGate serializes successful submissions and Fence boundaries.
	// Its channel form lets Fence stop waiting when its context is canceled.
	submitGate     chan struct{}
	completionMu   sync.Mutex
	completionChan chan struct{}
	// Test hooks must be installed before Start. They make blocked Submit/Fence
	// interleavings deterministic without changing production behavior.
	testSubmitLocked func()
	// testSubmitReady is called after Submit's cancellation and stopping checks,
	// immediately before its enqueue select. It is nil in production and lets
	// tests synchronize cancellation at the backpressure boundary.
	testSubmitReady   func()
	testFenceBoundary func(uint64)
	testProcessedFunc func(uint64)
}

// NewBlockPipeline creates a new BlockPipeline using functional options.
// Use With* options to customize the pipeline configuration.
//
// Example:
//
//	p := NewBlockPipeline(
//	    WithDecodeWorkers(4),
//	    WithTrustedDecodeOnly(),
//	)
func NewBlockPipeline(opts ...PipelineOption) *BlockPipeline {
	config := DefaultPipelineConfig()
	for _, opt := range opts {
		opt(&config)
	}
	p := &BlockPipeline{
		config:     config,
		metrics:    NewPipelineMetrics(config.MetricsWindowSize),
		submitGate: make(chan struct{}, 1),
	}
	p.submitGate <- struct{}{}
	return p
}

func (p *BlockPipeline) lockSubmit(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.ctx.Err() != nil {
		return p.stoppedError()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.ctx.Done():
		return p.stoppedError()
	case <-p.submitGate:
	}

	if err := ctx.Err(); err != nil {
		p.unlockSubmit()
		return err
	}
	if p.ctx.Err() != nil {
		p.unlockSubmit()
		return p.stoppedError()
	}
	return nil
}

func (p *BlockPipeline) unlockSubmit() {
	p.submitGate <- struct{}{}
}

// waitForPendingCapacity applies backpressure before Submit assigns a sequence.
// One additional accepted sequence is allowed beyond the pending limit so the
// missing earliest sequence can be in flight while MaxPendingBlocks later
// sequences wait in ApplyStage.
func (p *BlockPipeline) waitForPendingCapacity(ctx context.Context) error {
	if p.config.MaxPendingBlocks <= 0 {
		return nil
	}
	maxPending := uint64(p.config.MaxPendingBlocks)
	for {
		p.completionMu.Lock()
		submitted := p.sequenceCounter.Load()
		completed := p.completedSequence.Load()
		if submitted <= completed || submitted-completed <= maxPending {
			p.completionMu.Unlock()
			return nil
		}
		completionChan := p.completionChan
		p.completionMu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.ctx.Done():
			return p.stoppedError()
		case <-completionChan:
		}
	}
}

// Start starts the pipeline processing.
func (p *BlockPipeline) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopping.Load() || p.stopped.Load() {
		return ErrPipelineStopped
	}

	if p.started.Load() {
		return nil // Already started
	}

	// Validate configuration.
	trustedDecodeOnly := p.config.TrustedDecodeOnly
	validationEnabled := p.config.ValidateWorkers > 0
	if trustedDecodeOnly {
		if p.config.ApplyFunc != nil {
			return ErrTrustedDecodeOnlyApply
		}
		if validationEnabled {
			return ErrTrustedDecodeOnlyValidation
		}
		if p.config.ChainContextValidator != nil {
			return &TrustedDecodeOnlyChainContextError{}
		}
		if p.config.BlockTypeResolver != nil {
			return ErrTrustedDecodeOnlyBlockTypeResolver
		}
	} else {
		if !validationEnabled {
			return ErrBlockValidationRequired
		}
		if p.config.Eta0Provider == nil {
			return ErrMissingEta0Provider
		}
		if p.config.ChainContextValidator == nil {
			return ErrMissingChainContextValidator
		}
		if p.config.ApplyFunc == nil {
			return ErrMissingApplyFunc
		}
		if p.config.SkipBodyHashValidation {
			return ErrBodyHashValidationRequired
		}
		if flag := validationBypassFlag(p.config.VerifyConfig); flag != "" {
			return &ValidationBypassError{Flag: flag}
		}
		if p.config.SlotsPerKesPeriod == 0 {
			return ErrInvalidSlotsPerKesPeriod
		}
		if p.config.BlockTypeResolver == nil {
			return ErrMissingBlockTypeResolver
		}
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
	p.decodeStage.SetBlockTypeResolver(p.config.BlockTypeResolver)
	p.applyStage = NewApplyStage(p.config.ApplyFunc, p.config.MaxPendingBlocks)
	p.applyStage.SetChainContextValidator(p.config.ChainContextValidator)
	p.applyStage.setTrustedDecodeOnly(trustedDecodeOnly)
	// When validation is enabled, require items to have actually passed
	// validation before apply; ValidationError alone cannot distinguish
	// "passed" from "never ran"
	p.applyStage.SetRequireValidation(validationEnabled)

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
	p.completionMu.Lock()
	p.completionChan = make(chan struct{})
	p.completedSequence.Store(0)
	p.completionMu.Unlock()
	processedFunc := p.markProcessed
	if p.testProcessedFunc != nil {
		processedFunc = p.testProcessedFunc
	}
	p.applyRunner.SetProcessedFunc(processedFunc)
	p.applyRunner.setFatalErrorFunc(p.setFatalError)
	p.applyRunner.setFatalFunc(p.cancel)

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
func (p *BlockPipeline) Submit(
	ctx context.Context,
	blockType uint,
	rawCbor []byte,
	tip pcommon.Tip,
) error {
	// Early checks for common cases (before acquiring lock)
	if !p.started.Load() {
		return ErrPipelineNotStarted
	}

	// The gate prevents Stop from closing submitChan while a submission is in
	// flight. It also serializes the enqueue and sequence commit, so canceled
	// submissions cannot leave a sequence gap that would block ordered apply.
	if err := p.lockSubmit(ctx); err != nil {
		return err
	}
	defer p.unlockSubmit()
	if p.testSubmitLocked != nil {
		p.testSubmitLocked()
	}
	if err := p.waitForPendingCapacity(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.ctx.Err() != nil || p.stopping.Load() || p.stopped.Load() {
		return p.stoppedError()
	}

	// Commit a sequence number only after its item was successfully enqueued.
	// Keeping the provisional ID under submitGate makes successful IDs unique and
	// contiguous in queue order, while canceled or backpressured submissions do
	// not create positions that Fence must wait for.
	sequence := p.sequenceCounter.Load()
	item := NewBlockItem(blockType, rawCbor, tip, sequence)

	// Preserve cancellation precedence over an immediately writable input.
	// These checks are repeated under submitGate so cancellation that occurred
	// while acquiring the gate cannot lose a ready select tie to the enqueue.
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.ctx.Err() != nil {
		return p.stoppedError()
	}
	// Check stopping under the gate to ensure we don't race with Stop. This
	// follows the context checks so caller cancellation keeps precedence.
	if p.stopping.Load() || p.stopped.Load() {
		return p.stoppedError()
	}
	if p.testSubmitReady != nil {
		p.testSubmitReady()
	}

	select {
	case p.submitChan <- item:
		p.sequenceCounter.Add(1)
		p.metrics.RecordSubmit()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-p.ctx.Done():
		return p.stoppedError()
	}
}

// Fence waits until every block submitted before the fence is installed has
// completed ordered processing. It does not wait for blocks submitted after
// the fence, allowing callers to establish an exact ordering boundary without
// draining later work.
func (p *BlockPipeline) Fence(ctx context.Context) error {
	if !p.started.Load() {
		return ErrPipelineNotStarted
	}

	// The gate excludes in-flight submissions while we capture the sequence
	// boundary. Submit commits each sequence number under this gate only after
	// enqueue, so every position at or below target is waitable.
	if err := p.lockSubmit(ctx); err != nil {
		return err
	}
	if p.stopping.Load() || p.stopped.Load() {
		p.unlockSubmit()
		return p.stoppedError()
	}
	target := p.sequenceCounter.Load()
	p.unlockSubmit()
	if p.testFenceBoundary != nil {
		p.testFenceBoundary(target)
	}
	if target == 0 {
		return nil
	}

	for {
		if p.completedSequence.Load() >= target {
			return nil
		}
		p.completionMu.Lock()
		if p.completedSequence.Load() >= target {
			p.completionMu.Unlock()
			return nil
		}
		completionChan := p.completionChan
		p.completionMu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.ctx.Done():
			return p.stoppedError()
		case <-completionChan:
		}
	}
}

func (p *BlockPipeline) setFatalError(err error) {
	p.fatalErrMu.Lock()
	if p.fatalErr == nil {
		p.fatalErr = err
	}
	p.fatalErrMu.Unlock()
}

func (p *BlockPipeline) stoppedError() error {
	p.fatalErrMu.RLock()
	err := p.fatalErr
	p.fatalErrMu.RUnlock()
	if err != nil {
		return err
	}
	return ErrPipelineStopped
}

func (p *BlockPipeline) markProcessed(sequence uint64) {
	p.completedSequence.Store(sequence)
	p.completionMu.Lock()
	close(p.completionChan)
	p.completionChan = make(chan struct{})
	p.completionMu.Unlock()
}

// Results returns a channel of successfully processed block items.
// If the pipeline has not been started, it returns a closed channel. Results
// are observational: Stop may consume undelivered notifications while waiting
// for the apply runner. Callers that need every notification must receive them
// before invoking Stop; delivery is not guaranteed after Stop begins. ApplyFunc
// remains the authoritative commit sink.
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

// Stop gracefully stops the pipeline. It drains undelivered observational
// results while waiting for the apply runner so shutdown cannot depend on a
// Results consumer.
func (p *BlockPipeline) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started.Load() || p.stopped.Load() {
		return nil
	}

	// Publish the explicit stop before canceling the pipeline context so drain
	// waiters can distinguish Stop from cancellation inherited from Start.
	p.stopping.Store(true)
	// Cancel before acquiring submitGate to avoid deadlock: Submit holds the
	// gate while blocked on a channel send and needs ctx.Done to unblock.
	p.cancel()

	// Now acquire the gate to ensure no Submit() calls are in progress.
	// Any Submit() blocked on channel send will now return via ctx.Done().
	<-p.submitGate
	p.stopped.Store(true)
	// Close input channel to signal shutdown
	close(p.submitChan)
	p.unlockSubmit()

	// Wait for decode workers to finish
	p.decodePool.Stop()
	close(p.decodedChan)

	// Wait for validate workers to finish (if validation is enabled)
	if p.validatePool != nil {
		p.validatePool.Stop()
		close(p.validatedChan)
	}

	// The fatal path preserves its committed prefix with blocking result sends.
	// Drain those observational notifications while waiting so a caller that
	// does not consume Results cannot prevent shutdown.
	applyDone := make(chan struct{})
	go func() {
		p.applyRunner.Stop()
		close(applyDone)
	}()
	for {
		select {
		case <-applyDone:
			goto applyStopped
		case <-p.resultsChan:
		}
	}

applyStopped:

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

// PendingCount returns an observational snapshot of the approximate number of
// items still being processed. It includes items in inter-stage channels and
// items buffered or active in the apply stage, but not work executing in other
// stages. It must not be used as a completion barrier; use Fence or
// WaitForDrain instead.
func (p *BlockPipeline) PendingCount() int {
	if !p.started.Load() {
		return 0
	}
	channelDepth := len(
		p.submitChan,
	) + len(
		p.decodedChan,
	) + len(
		p.validatedChan,
	)
	applyPending := 0
	if p.applyStage != nil {
		applyPending = p.applyStage.PendingCount()
	}
	return channelDepth + applyPending
}

// WaitForDrain blocks until every block accepted before the drain boundary has
// completed ordered processing, or the context is cancelled. This is useful
// before handling rollbacks to ensure no earlier block is applied afterward.
func (p *BlockPipeline) WaitForDrain(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !p.started.Load() {
		return ErrPipelineNotStarted
	}
	// Preserve the historical lifecycle contract: explicit shutdown makes the
	// drain vacuous, including while Stop waits for the submission gate.
	if p.stopping.Load() || p.stopped.Load() {
		return nil
	}
	err := p.Fence(ctx)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if p.stopping.Load() || p.stopped.Load() {
		return nil
	}
	if p.ctx.Err() != nil {
		return p.stoppedError()
	}
	return err
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
			depth := len(
				p.submitChan,
			) + len(
				p.decodedChan,
			) + len(
				p.validatedChan,
			)
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
