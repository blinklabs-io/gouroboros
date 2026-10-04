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

// Package pipeline provides a concurrent block processing pipeline for Cardano blocks.
// It supports parallel decoding and validation with ordered application.
package pipeline

import (
	"context"
	"math"
	"time"

	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// Stage represents a processing stage in the block pipeline.
type Stage interface {
	// Name returns the name of the stage for logging and metrics.
	Name() string
	// Process processes a single block item. Returns an error if processing fails.
	Process(ctx context.Context, item *BlockItem) error
}

// StageFunc is an adapter that allows using ordinary functions as Stage implementations.
type StageFunc struct {
	name string
	fn   func(ctx context.Context, item *BlockItem) error
}

// NewStageFunc creates a new StageFunc with the given name and processing function.
func NewStageFunc(name string, fn func(ctx context.Context, item *BlockItem) error) *StageFunc {
	return &StageFunc{
		name: name,
		fn:   fn,
	}
}

// Name returns the name of the stage.
func (s *StageFunc) Name() string {
	return s.name
}

// Process calls the underlying function.
func (s *StageFunc) Process(ctx context.Context, item *BlockItem) error {
	return s.fn(ctx, item)
}

// Pipeline represents a block processing pipeline.
type Pipeline interface {
	// Start starts the pipeline processing.
	Start(ctx context.Context) error
	// Submit submits a new block for processing.
	// The context allows callers to handle timeouts or cancellations when the
	// pipeline is full and applying backpressure.
	Submit(ctx context.Context, blockType uint, rawCbor []byte, tip pcommon.Tip) error
	// Results returns a channel of successfully processed block items.
	Results() <-chan *BlockItem
	// Errors returns a channel of processing errors.
	Errors() <-chan error
	// Stop gracefully stops the pipeline.
	Stop() error
	// WaitForDrain waits for all submitted blocks to be processed.
	WaitForDrain(ctx context.Context) error
	// Stats returns the current pipeline statistics.
	Stats() PipelineStats
}

// StageTimings summarizes how long a pipeline stage took to process the items
// it completed successfully. Failed items are excluded because their recorded
// duration covers only part of the work.
//
// The three fields are read from independent atomic counters, so a snapshot
// taken while items are being recorded can pair a Count with a Total from a
// slightly later item. The skew is bounded by the items in flight and does not
// accumulate.
type StageTimings struct {
	// Count is the number of successful items recorded.
	Count uint64
	// Total is the sum of the recorded durations.
	Total time.Duration
	// Max is the longest recorded duration.
	Max time.Duration
}

// Mean returns the average recorded duration, or zero when nothing has been
// recorded.
func (t StageTimings) Mean() time.Duration {
	if t.Count == 0 || t.Count > math.MaxInt64 {
		return 0
	}
	return t.Total / time.Duration(t.Count)
}

// PipelineStats contains statistics about pipeline performance.
type PipelineStats struct {
	// BlocksSubmitted is the total number of blocks submitted to the pipeline.
	BlocksSubmitted uint64
	// BlocksDecoded is the total number of blocks successfully decoded.
	BlocksDecoded uint64
	// BlocksValidated is the total number of blocks successfully validated.
	BlocksValidated uint64
	// BlocksApplied is the total number of blocks successfully applied.
	BlocksApplied uint64
	// DecodeErrors is the total number of decode errors.
	DecodeErrors uint64
	// ValidationErrors is the total number of validation errors.
	ValidationErrors uint64
	// ApplyErrors is the total number of apply errors.
	ApplyErrors uint64

	// DecodeTimings summarizes the durations of successful decodes.
	DecodeTimings StageTimings
	// ValidateTimings summarizes the durations of successful validations.
	ValidateTimings StageTimings
	// ApplyTimings summarizes the durations of successful applies.
	ApplyTimings StageTimings

	// CurrentQueueDepth is the current number of blocks in the pipeline.
	CurrentQueueDepth int
	// PeakQueueDepth is the maximum queue depth observed.
	PeakQueueDepth int

	// LastBlockTime is the time the last block was processed.
	LastBlockTime time.Time
	// StartTime is when the pipeline was started.
	StartTime time.Time
}
