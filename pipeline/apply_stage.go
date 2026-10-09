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
	"time"

	"github.com/blinklabs-io/gouroboros/internal/panics"
)

// ErrPendingLimitExceeded is returned when the apply stage's pending buffer is full.
var ErrPendingLimitExceeded = errors.New(
	"pipeline: pending block limit exceeded",
)

// ErrBlockNotValidated is returned when the apply stage requires validation
// but receives a block that was never validated. This guards against blocks
// reaching apply without passing through the validate stage.
var ErrBlockNotValidated = errors.New(
	"pipeline: block reached apply stage without validation",
)

// ErrChainContextValidation is returned when authoritative chain-context
// validation rejects a block.
var ErrChainContextValidation = errors.New(
	"pipeline: chain-context validation failed",
)

// ErrBlockApply is returned when ApplyFunc fails to commit a validated block.
var ErrBlockApply = errors.New("pipeline: block apply failed")

// ErrPipelineItemRejected marks a failure that makes ordered application
// unsafe to continue.
var ErrPipelineItemRejected = errors.New("pipeline: ordered block rejected")

// ApplyFunc is a function that applies a block to some state.
// It is called in sequence order after ChainContextValidator succeeds. It must
// commit state atomically: returning an error cancels ordered processing.
type ApplyFunc func(*BlockItem) error

// ChainContextValidator validates a decoded block against the consumer's
// current authoritative chain state. It is called in sequence order and must
// be read-only; ApplyFunc alone commits state. The consumer owns rollback of
// that authoritative state.
type ChainContextValidator func(context.Context, *BlockItem) error

// ApplyStage buffers validated blocks and applies them in sequence order.
//
// Thread-safety: While ApplyStage uses internal locking for state management,
// ProcessWithStatus must be called from a single goroutine to guarantee ordered
// execution of ApplyFunc. The ApplyStageRunner provides this guarantee.
type ApplyStage struct {
	applyFunc             ApplyFunc
	chainContextValidator ChainContextValidator
	trustedDecodeOnly     bool
	// requireValidation requires items to have passed validation (IsValid)
	// before applying. ValidationError alone cannot distinguish "validation
	// passed" from "validation never ran" (both are nil).
	requireValidation bool
	maxPending        int
	mu                sync.Mutex
	// pending holds out-of-order items waiting to be applied
	pending map[uint64]*BlockItem
	// nextSequence is the next sequence number to apply
	nextSequence uint64
	inFlight     int
}

// NewApplyStage creates a new ApplyStage with the given apply function.
// maxPending limits the number of out-of-order blocks that can be buffered.
// Use 0 for unlimited (not recommended in production).
func NewApplyStage(applyFunc ApplyFunc, maxPending int) *ApplyStage {
	return &ApplyStage{
		applyFunc:    applyFunc,
		maxPending:   maxPending,
		pending:      make(map[uint64]*BlockItem),
		nextSequence: 0,
	}
}

// SetRequireValidation sets whether items must have passed validation before
// being applied. When enabled, an unvalidated item is not applied and its
// apply error is set to ErrBlockNotValidated. Enable this whenever the
// pipeline runs with validation so that blocks which bypass the validate
// stage cannot be applied. Must be called before processing begins to avoid
// data races.
func (s *ApplyStage) SetRequireValidation(require bool) {
	s.requireValidation = require
}

// SetChainContextValidator sets the authoritative validator used immediately
// before ApplyFunc. It must be called before processing begins.
func (s *ApplyStage) SetChainContextValidator(validator ChainContextValidator) {
	s.chainContextValidator = validator
}

func (s *ApplyStage) setTrustedDecodeOnly(trusted bool) {
	s.trustedDecodeOnly = trusted
}

// Name returns the stage name.
func (s *ApplyStage) Name() string {
	return "apply"
}

// Process buffers the item and applies any items that are now in order.
// Returns nil even if the item is buffered (not yet applied).
// The actual apply error is stored in the item and sent to the error channel.
func (s *ApplyStage) Process(ctx context.Context, item *BlockItem) error {
	_, err := s.ProcessWithStatus(ctx, item)
	return err
}

// ProcessWithStatus processes an item and returns all items that were processed.
// If the item is next in sequence, it is processed immediately along with any
// buffered items that become ready. If the item is out of order, it is buffered
// and the returned slice will be nil.
//
// This design eliminates data loss that could occur with callback-based approaches
// when many buffered items are released at once.
func (s *ApplyStage) ProcessWithStatus(
	ctx context.Context,
	item *BlockItem,
) ([]*BlockItem, error) {
	processed, _, err := s.processWithStatus(ctx, item)
	return processed, err
}

func (s *ApplyStage) processWithStatus(
	ctx context.Context,
	item *BlockItem,
) ([]*BlockItem, *BlockItem, error) {
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
	}

	s.mu.Lock()

	// Check if this is the next item to apply
	if item.SequenceNumber() == s.nextSequence {
		s.nextSequence++
		s.mu.Unlock()
		if err := s.maybeApply(ctx, item); err != nil {
			return nil, item, err
		}
		// Try to apply any buffered items that are now in order
		buffered, rejected, err := s.applyPending(ctx)
		processed := make([]*BlockItem, 0, 1+len(buffered))
		processed = append(processed, item)
		processed = append(processed, buffered...)
		if err != nil {
			return processed, rejected, err
		}
		// Return the input item plus any buffered items
		return processed, nil, nil
	}

	// Reject before buffering so callers can retry without leaving this sequence
	// in both their queue and the stage's pending map.
	if s.maxPending > 0 && len(s.pending) >= s.maxPending {
		s.mu.Unlock()
		return nil, item, ErrPendingLimitExceeded
	}
	s.pending[item.SequenceNumber()] = item
	s.mu.Unlock()
	return nil, nil, nil
}

// maybeApply validates and applies an ordered item. Any rejection outside
// trusted decode-only mode is fatal to ordered processing.
func (s *ApplyStage) maybeApply(ctx context.Context, item *BlockItem) error {
	if item.DecodeError() != nil {
		if s.trustedDecodeOnly {
			return nil
		}
		return fmt.Errorf(
			"%w: decode block: %w",
			ErrPipelineItemRejected,
			item.DecodeError(),
		)
	}
	if item.ValidationError() != nil {
		return fmt.Errorf(
			"%w: validate block: %w",
			ErrPipelineItemRejected,
			item.ValidationError(),
		)
	}
	if s.requireValidation && !item.IsValid() {
		item.SetApplied(false, ErrBlockNotValidated, 0)
		return fmt.Errorf(
			"%w: %w",
			ErrPipelineItemRejected,
			ErrBlockNotValidated,
		)
	}
	if s.trustedDecodeOnly {
		return nil
	}
	s.mu.Lock()
	s.inFlight++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.inFlight--
		s.mu.Unlock()
	}()
	if s.chainContextValidator == nil {
		return fmt.Errorf(
			"%w: %w",
			ErrPipelineItemRejected,
			ErrMissingChainContextValidator,
		)
	}
	if err := s.callChainContextValidator(ctx, item); err != nil {
		wrappedErr := fmt.Errorf("%w: %w", ErrChainContextValidation, err)
		item.SetApplied(false, wrappedErr, 0)
		return fmt.Errorf("%w: %w", ErrPipelineItemRejected, wrappedErr)
	}
	return s.applyItem(ctx, item)
}

// applyItem applies a single item without holding the lock.
func (s *ApplyStage) applyItem(ctx context.Context, item *BlockItem) error {
	select {
	case <-ctx.Done():
		item.SetApplied(false, ctx.Err(), 0)
		return ctx.Err()
	default:
	}

	start := time.Now()
	err := s.callApplyFunc(item)
	duration := time.Since(start)

	if err != nil {
		wrappedErr := fmt.Errorf("%w: %w", ErrBlockApply, err)
		item.SetApplied(false, wrappedErr, duration)
		return fmt.Errorf("%w: %w", ErrPipelineItemRejected, wrappedErr)
	} else {
		item.SetApplied(true, nil, duration)
	}
	return nil
}

func (s *ApplyStage) callChainContextValidator(
	ctx context.Context,
	item *BlockItem,
) (err error) {
	defer func() {
		if recovered := panics.New(
			ErrStagePanic,
			"chain-context validator",
			recover(),
		); recovered != nil {
			err = recovered
		}
	}()
	return s.chainContextValidator(ctx, item)
}

// callApplyFunc invokes the consumer's ApplyFunc for an item, containing any
// panic so that it becomes this item's apply error.
//
// Guarding the callback here rather than around the runner's loop is what
// keeps ordered application intact: applyPending has already advanced
// nextSequence past this item and holds items it has processed but not yet
// returned, so unwinding out of it would drop them and leave WaitForDrain
// waiting on blocks that will never be reported.
func (s *ApplyStage) callApplyFunc(item *BlockItem) (err error) {
	if s.applyFunc == nil {
		return nil
	}
	defer panics.Guard(ErrStagePanic, "apply function", &err)
	return s.applyFunc(item)
}

// applyPending applies any pending items that are now in order.
// This method acquires and releases the lock as needed to avoid holding it during applyFunc.
// Returns a slice of all items that were processed from the pending buffer.
func (s *ApplyStage) applyPending(
	ctx context.Context,
) ([]*BlockItem, *BlockItem, error) {
	var processed []*BlockItem
	for {
		select {
		case <-ctx.Done():
			return processed, nil, ctx.Err()
		default:
		}

		s.mu.Lock()
		item, ok := s.pending[s.nextSequence]
		if !ok {
			s.mu.Unlock()
			return processed, nil, nil
		}
		delete(s.pending, s.nextSequence)
		s.nextSequence++
		s.mu.Unlock()

		// Apply if valid, otherwise just advance (sequence already incremented)
		if err := s.maybeApply(ctx, item); err != nil {
			return processed, item, err
		}

		processed = append(processed, item)
	}
}

// Reset resets the stage state for reuse.
func (s *ApplyStage) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = make(map[uint64]*BlockItem)
	s.nextSequence = 0
}

// PendingCount returns the number of items waiting to be applied.
func (s *ApplyStage) PendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending) + s.inFlight
}

// ApplyStageRunner runs the apply stage as a single goroutine.
type ApplyStageRunner struct {
	stage          *ApplyStage
	input          <-chan *BlockItem
	output         chan<- *BlockItem
	errors         chan<- error
	metrics        *PipelineMetrics
	processedFunc  func(uint64)
	fatalFunc      func()
	fatalErrorFunc func(error)
	done           chan struct{}
	running        bool
	mu             sync.Mutex
}

// NewApplyStageRunner creates a new runner for the apply stage.
//
// Parameters:
//   - stage: The apply stage to use for processing
//   - input: Channel to receive block items from
//   - output: Channel to send processed items to
//   - errors: Channel to send errors to
//   - pendingQueueSize: Deprecated, no longer used. The new implementation
//     returns processed items directly from ProcessWithStatus, eliminating
//     the data loss vulnerability that could occur when buffered items
//     exceeded the queue size.
func NewApplyStageRunner(
	stage *ApplyStage,
	input <-chan *BlockItem,
	output chan<- *BlockItem,
	errors chan<- error,
	pendingQueueSize int, //nolint:revive // kept for API compatibility
) *ApplyStageRunner {
	return &ApplyStageRunner{
		stage:  stage,
		input:  input,
		output: output,
		errors: errors,
		done:   make(chan struct{}),
	}
}

// SetMetrics sets the metrics collector for the runner.
// Must be called before Start() to avoid data races.
func (r *ApplyStageRunner) SetMetrics(metrics *PipelineMetrics) {
	r.metrics = metrics
}

// SetProcessedFunc sets a callback invoked after ordered processing completes
// through the supplied sequence number. It must be called before Start.
func (r *ApplyStageRunner) SetProcessedFunc(processedFunc func(uint64)) {
	r.processedFunc = processedFunc
}

// setFatalFunc sets the pipeline cancellation hook used when runner
// bookkeeping can no longer be trusted. It must be called before Start.
func (r *ApplyStageRunner) setFatalFunc(fatalFunc func()) {
	r.fatalFunc = fatalFunc
}

func (r *ApplyStageRunner) setFatalErrorFunc(fatalErrorFunc func(error)) {
	r.fatalErrorFunc = fatalErrorFunc
}

// Start starts the apply stage runner.
func (r *ApplyStageRunner) Start(ctx context.Context) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.done = make(chan struct{})
	r.mu.Unlock()

	go r.run(ctx)
}

// Stop waits for the runner to complete. The runner will exit when the context
// passed to Start is cancelled or the input channel is closed. This method blocks
// until completion; it does not signal the runner to stop. If the output is not
// consumed concurrently, callers must drain it until Stop returns.
func (r *ApplyStageRunner) Stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	// Capture done channel while holding lock to avoid race with concurrent Start()
	done := r.done
	r.mu.Unlock()

	<-done
}

func (r *ApplyStageRunner) run(ctx context.Context) {
	defer func() {
		r.mu.Lock()
		r.running = false
		close(r.done)
		r.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case item, ok := <-r.input:
			if !ok {
				return
			}

			processed, rejected, processErr := r.process(ctx, item)
			processedErr := r.accountProcessed(processed)
			if processedErr != nil {
				fatalErr := errors.Join(processErr, processedErr)
				r.recordItemMetrics(rejected)
				r.fail(fatalErr)
				r.forwardCommitted(processed)
				select {
				case r.errors <- fatalErr:
				default:
				}
				return
			}
			if processErr != nil {
				fatal := errors.Is(processErr, ErrStagePanic) ||
					errors.Is(processErr, ErrPendingLimitExceeded) ||
					errors.Is(processErr, ErrPipelineItemRejected)
				if fatal {
					fatalErr := processErr
					r.recordItemMetrics(rejected)
					r.fail(fatalErr)
					r.forwardCommitted(processed)
					select {
					case r.errors <- fatalErr:
					default:
					}
					return
				}
				r.forwardProcessed(ctx, processed)
				select {
				case r.errors <- processErr:
				case <-ctx.Done():
					return
				}
				continue
			}
			r.forwardProcessed(ctx, processed)
		}
	}
}

// accountProcessed records every successfully committed item and advances the
// completion boundary without waiting for a Results consumer.
func (r *ApplyStageRunner) accountProcessed(
	processed []*BlockItem,
) error {
	if len(processed) == 0 {
		return nil
	}
	var err error
	if r.processedFunc != nil {
		err = r.callProcessedFunc(
			processed[len(processed)-1].SequenceNumber() + 1,
		)
	}
	for _, item := range processed {
		r.recordItemMetrics(item)
	}
	return err
}

func (r *ApplyStageRunner) forwardProcessed(
	ctx context.Context,
	processed []*BlockItem,
) {
	for _, item := range processed {
		if !r.forwardItem(ctx, item) {
			return
		}
	}
}

// forwardCommitted preserves the ordered successful prefix after a fatal
// cancellation. The runner retains ownership until the Results consumer has
// accepted every item, so callers must continue draining Results during Stop.
func (r *ApplyStageRunner) forwardCommitted(processed []*BlockItem) {
	for _, item := range processed {
		r.output <- item
	}
}

func (r *ApplyStageRunner) fail(err error) {
	if r.fatalErrorFunc != nil {
		r.fatalErrorFunc(err)
	}
	if r.fatalFunc != nil {
		r.fatalFunc()
	}
}

func (r *ApplyStageRunner) recordItemMetrics(item *BlockItem) {
	if r.metrics != nil && item != nil && item.DecodeError() == nil &&
		item.ValidationError() == nil &&
		(item.IsApplied() || item.ApplyError() != nil) {
		r.metrics.RecordApply(item.ApplyDuration(), item.ApplyError())
		r.metrics.RecordPipelineLatency(item.TotalDuration())
	}
}

// process contains apply-stage panics so the runner can cancel the pipeline
// and report the failure. The consumer's ApplyFunc is guarded closer in by
// callApplyFunc, so a panic reaching here comes from ordering bookkeeping.
func (r *ApplyStageRunner) process(
	ctx context.Context,
	item *BlockItem,
) (processed []*BlockItem, rejected *BlockItem, err error) {
	defer func() {
		if recovered := panics.New(ErrStagePanic, "apply stage", recover()); recovered != nil {
			err = recovered
			rejected = item
			if item != nil {
				item.SetApplied(false, recovered, 0)
			}
		}
	}()
	return r.stage.processWithStatus(ctx, item)
}

func (r *ApplyStageRunner) callProcessedFunc(sequence uint64) (err error) {
	defer func() {
		if recovered := panics.New(
			ErrStagePanic,
			"apply processed callback",
			recover(),
		); recovered != nil {
			err = recovered
		}
	}()
	r.processedFunc(sequence)
	return nil
}

// forwardItem sends an item to output and reports any apply errors. A ready
// output wins over cancellation so an already committed prefix fills available
// result capacity in order without delaying fatal shutdown on a full channel.
func (r *ApplyStageRunner) forwardItem(
	ctx context.Context,
	item *BlockItem,
) bool {
	select {
	case r.output <- item:
		return r.reportItemError(ctx, item)
	default:
	}
	select {
	case r.output <- item:
		return r.reportItemError(ctx, item)
	case <-ctx.Done():
		return false
	}
}

func (r *ApplyStageRunner) reportItemError(
	ctx context.Context,
	item *BlockItem,
) bool {
	// Report apply errors separately
	if applyErr := item.ApplyError(); applyErr != nil {
		select {
		case r.errors <- applyErr:
		case <-ctx.Done():
			return false
		}
	}
	return true
}
