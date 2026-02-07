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

// ErrPendingQueueFull is returned when the pending queue is full and an item cannot be enqueued.
var ErrPendingQueueFull = errors.New("apply stage: pending queue full, item dropped")

// ApplyFunc is a function that applies a block to some state.
// It is called in sequence order (by SequenceNumber).
type ApplyFunc func(*BlockItem) error

// ApplyStage buffers validated blocks and applies them in sequence order.
type ApplyStage struct {
	applyFunc ApplyFunc
	mu        sync.Mutex
	// pending holds out-of-order items waiting to be applied
	pending map[uint64]*BlockItem
	// nextSequence is the next sequence number to apply
	nextSequence uint64
	// onProcessed is called when a buffered item is processed (used for forwarding)
	onProcessed func(*BlockItem)
}

// NewApplyStage creates a new ApplyStage with the given apply function.
func NewApplyStage(applyFunc ApplyFunc) *ApplyStage {
	return &ApplyStage{
		applyFunc:    applyFunc,
		pending:      make(map[uint64]*BlockItem),
		nextSequence: 0,
	}
}

// SetOnProcessed sets a callback that is invoked when buffered items are processed.
// This is used by the runner to forward buffered items to the output channel.
func (s *ApplyStage) SetOnProcessed(fn func(*BlockItem)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onProcessed = fn
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

// ProcessWithStatus is like Process but also returns whether the item was processed
// immediately (true) or buffered for later processing (false).
// The runner uses this to avoid forwarding buffered items twice.
func (s *ApplyStage) ProcessWithStatus(ctx context.Context, item *BlockItem) (immediate bool, err error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	s.mu.Lock()

	// Skip items that failed validation
	if item.DecodeError() != nil || item.ValidationError() != nil {
		// Still need to track sequence for ordering
		if item.SequenceNumber == s.nextSequence {
			s.nextSequence++
			s.mu.Unlock()
			// Try to apply any buffered items
			s.applyPending(ctx)
			return true, nil
		} else {
			s.pending[item.SequenceNumber] = item
			s.mu.Unlock()
			return false, nil
		}
	}

	// Check if this is the next item to apply
	if item.SequenceNumber == s.nextSequence {
		s.nextSequence++
		s.mu.Unlock()
		// Apply without holding lock to avoid blocking other processing
		s.applyItem(ctx, item)
		// Try to apply any buffered items that are now in order
		s.applyPending(ctx)
		return true, nil
	} else {
		// Buffer for later
		s.pending[item.SequenceNumber] = item
		s.mu.Unlock()
		return false, nil
	}
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
func (s *ApplyStage) applyPending(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		s.mu.Lock()
		item, ok := s.pending[s.nextSequence]
		if !ok {
			s.mu.Unlock()
			return
		}
		delete(s.pending, s.nextSequence)
		s.nextSequence++
		onProcessed := s.onProcessed
		s.mu.Unlock()

		// Apply if valid, otherwise just advance (sequence already incremented)
		if item.DecodeError() == nil && item.ValidationError() == nil {
			s.applyItem(ctx, item)
		}

		// Notify runner to forward this buffered item to output
		if onProcessed != nil {
			onProcessed(item)
		}
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
	stage        *ApplyStage
	input        <-chan *BlockItem
	output       chan<- *BlockItem
	errors       chan<- error
	metrics      *PipelineMetrics
	done         chan struct{}
	running      bool
	mu           sync.Mutex
	pendingQueue chan *BlockItem // queue for items processed from pending buffer
}

// NewApplyStageRunner creates a new runner for the apply stage.
//
// Parameters:
//   - stage: The apply stage to use for processing
//   - input: Channel to receive block items from
//   - output: Channel to send processed items to
//   - errors: Channel to send errors to
//   - pendingQueueSize: Size of the pending queue buffer; should match PrefetchBufferSize
//     to avoid data loss when large gaps in sequence numbers are filled
func NewApplyStageRunner(
	stage *ApplyStage,
	input <-chan *BlockItem,
	output chan<- *BlockItem,
	errors chan<- error,
	pendingQueueSize int,
) *ApplyStageRunner {
	if pendingQueueSize <= 0 {
		pendingQueueSize = 1000 // default fallback
	}
	r := &ApplyStageRunner{
		stage:        stage,
		input:        input,
		output:       output,
		errors:       errors,
		done:         make(chan struct{}),
		pendingQueue: make(chan *BlockItem, pendingQueueSize),
	}
	// Set up callback to queue pending items for output
	stage.SetOnProcessed(func(item *BlockItem) {
		select {
		case r.pendingQueue <- item:
		default:
			// Queue full - report error to prevent silent data loss
			select {
			case r.errors <- ErrPendingQueueFull:
			default:
				// Error channel also full, item is dropped
			}
		}
	})
	return r
}

// SetMetrics sets the metrics collector for the runner.
func (r *ApplyStageRunner) SetMetrics(metrics *PipelineMetrics) {
	r.mu.Lock()
	defer r.mu.Unlock()
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

// Stop waits for the runner to complete.
func (r *ApplyStageRunner) Stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()

	<-r.done
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
		case pendingItem := <-r.pendingQueue:
			// Forward pending items that were processed from buffer
			r.forwardItem(ctx, pendingItem)
		case item, ok := <-r.input:
			if !ok {
				// Drain pending queue before exiting
				r.drainPendingQueue(ctx)
				return
			}

			immediate, err := r.stage.ProcessWithStatus(ctx, item)
			if err != nil {
				select {
				case r.errors <- err:
				case <-ctx.Done():
					return
				}
			}

			// Forward to output (results channel) only if processed immediately.
			// Buffered items will be forwarded later via pendingQueue when they are
			// processed by applyPending(). This prevents double-forwarding of items
			// that arrive out of order.
			if immediate {
				r.forwardItem(ctx, item)
			}

			// Also drain any pending items that were just unblocked
			r.drainPendingQueue(ctx)
		}
	}
}

// forwardItem sends an item to output and reports any apply errors.
func (r *ApplyStageRunner) forwardItem(ctx context.Context, item *BlockItem) {
	// Record metrics for applied items
	if r.metrics != nil && item.IsApplied() {
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

// drainPendingQueue forwards all items currently in the pending queue.
func (r *ApplyStageRunner) drainPendingQueue(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case pendingItem := <-r.pendingQueue:
			r.forwardItem(ctx, pendingItem)
		default:
			return
		}
	}
}
