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
	"time"
)

// ErrPendingLimitExceeded is returned when the apply stage's pending buffer is full.
var ErrPendingLimitExceeded = errors.New("pipeline: pending block limit exceeded")

// ApplyFunc is a function that applies a block to some state.
// It is called in sequence order (by SequenceNumber).
type ApplyFunc func(*BlockItem) error

// ApplyStage buffers validated blocks and applies them in sequence order.
//
// Thread-safety: While ApplyStage uses internal locking for state management,
// ProcessWithStatus must be called from a single goroutine to guarantee ordered
// execution of ApplyFunc. The ApplyStageRunner provides this guarantee.
type ApplyStage struct {
	applyFunc  ApplyFunc
	maxPending int
	mu         sync.Mutex
	// pending holds out-of-order items waiting to be applied
	pending map[uint64]*BlockItem
	// nextSequence is the next sequence number to apply
	nextSequence uint64
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
func (s *ApplyStage) ProcessWithStatus(ctx context.Context, item *BlockItem) ([]*BlockItem, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	s.mu.Lock()

	// Check if this is the next item to apply
	if item.SequenceNumber() == s.nextSequence {
		s.nextSequence++
		s.mu.Unlock()
		// Apply if valid (no decode or validation errors)
		if item.DecodeError() == nil && item.ValidationError() == nil {
			s.applyItem(ctx, item)
		}
		// Try to apply any buffered items that are now in order
		buffered := s.applyPending(ctx)
		// Return the input item plus any buffered items
		processed := make([]*BlockItem, 0, 1+len(buffered))
		processed = append(processed, item)
		processed = append(processed, buffered...)
		return processed, nil
	}

	// Buffer for later - always add to preserve sequence ordering
	s.pending[item.SequenceNumber()] = item
	pendingCount := len(s.pending)
	s.mu.Unlock()

	// Check pending limit after buffering - return error to signal backpressure
	// but the item is still buffered to prevent sequence gaps
	if s.maxPending > 0 && pendingCount > s.maxPending {
		return nil, ErrPendingLimitExceeded
	}
	return nil, nil
}

// applyItem applies a single item without holding the lock.
func (s *ApplyStage) applyItem(ctx context.Context, item *BlockItem) {
	select {
	case <-ctx.Done():
		item.SetApplied(false, ctx.Err(), 0)
		return
	default:
	}

	start := time.Now()
	var err error
	if s.applyFunc != nil {
		err = s.applyFunc(item)
	}
	duration := time.Since(start)

	if err != nil {
		item.SetApplied(false, err, duration)
	} else {
		item.SetApplied(true, nil, duration)
	}
}

// applyPending applies any pending items that are now in order.
// This method acquires and releases the lock as needed to avoid holding it during applyFunc.
// Returns a slice of all items that were processed from the pending buffer.
func (s *ApplyStage) applyPending(ctx context.Context) []*BlockItem {
	var processed []*BlockItem
	for {
		select {
		case <-ctx.Done():
			return processed
		default:
		}

		s.mu.Lock()
		item, ok := s.pending[s.nextSequence]
		if !ok {
			s.mu.Unlock()
			return processed
		}
		delete(s.pending, s.nextSequence)
		s.nextSequence++
		s.mu.Unlock()

		// Apply if valid, otherwise just advance (sequence already incremented)
		if item.DecodeError() == nil && item.ValidationError() == nil {
			s.applyItem(ctx, item)
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
	return len(s.pending)
}

// ApplyStageRunner runs the apply stage as a single goroutine.
type ApplyStageRunner struct {
	stage   *ApplyStage
	input   <-chan *BlockItem
	output  chan<- *BlockItem
	errors  chan<- error
	metrics *PipelineMetrics
	done    chan struct{}
	running bool
	mu      sync.Mutex
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
// until completion; it does not signal the runner to stop.
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

			processed, err := r.stage.ProcessWithStatus(ctx, item)
			if err != nil {
				select {
				case r.errors <- err:
				case <-ctx.Done():
					return
				}
				continue
			}

			// Forward all processed items (includes input item + any buffered items
			// that became ready). This eliminates the data loss vulnerability from
			// the previous callback-based approach where items could be dropped if
			// the pending queue overflowed.
			for _, p := range processed {
				r.forwardItem(ctx, p)
			}
		}
	}
}

// forwardItem sends an item to output and reports any apply errors.
func (r *ApplyStageRunner) forwardItem(ctx context.Context, item *BlockItem) {
	// Record metrics for items that went through the apply stage (both success and failure).
	// Items with decode/validation errors are not applied and don't have apply metrics.
	if r.metrics != nil && item.DecodeError() == nil && item.ValidationError() == nil {
		r.metrics.RecordApply(item.ApplyDuration(), item.ApplyError())
		r.metrics.RecordPipelineLatency(item.TotalDuration())
	}

	select {
	case r.output <- item:
	case <-ctx.Done():
		return
	}

	// Report apply errors separately
	if applyErr := item.ApplyError(); applyErr != nil {
		select {
		case r.errors <- applyErr:
		case <-ctx.Done():
			return
		}
	}
}
