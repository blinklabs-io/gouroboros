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

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// ErrNilStage is returned when a nil stage is passed to a worker pool.
var ErrNilStage = errors.New("pipeline: nil stage")

// DecodeStage decodes raw block CBOR into Block objects.
type DecodeStage struct {
	// SkipBodyHashValidation disables body hash validation during decode.
	SkipBodyHashValidation bool
}

// NewDecodeStage creates a new DecodeStage.
func NewDecodeStage(skipBodyHashValidation bool) *DecodeStage {
	return &DecodeStage{
		SkipBodyHashValidation: skipBodyHashValidation,
	}
}

// Name returns the stage name.
func (s *DecodeStage) Name() string {
	return "decode"
}

// Process decodes the raw CBOR in the block item.
func (s *DecodeStage) Process(ctx context.Context, item *BlockItem) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	start := time.Now()

	config := common.VerifyConfig{
		SkipBodyHashValidation: s.SkipBodyHashValidation,
	}

	block, err := ledger.NewBlockFromCbor(item.BlockType(), item.RawCbor(), config)
	duration := time.Since(start)

	if err != nil {
		item.SetDecodeError(err, duration)
		return err
	}

	item.SetBlock(block, duration)
	return nil
}

// DecodeStageWorkerPool runs multiple decode workers in parallel.
// Deprecated: Use StageWorkerPool with DecodeMetricsRecorder instead.
// This type is kept for backward compatibility.
type DecodeStageWorkerPool struct {
	stage      *DecodeStage
	numWorkers int
	input      <-chan *BlockItem
	output     chan<- *BlockItem
	errors     chan<- error
	metrics    *PipelineMetrics
	pool       *StageWorkerPool
	startOnce  sync.Once
}

// NewDecodeStageWorkerPool creates a new worker pool for the decode stage.
//
// Parameters:
//   - stage: The decode stage to use for processing (required, panics if nil)
//   - numWorkers: Number of parallel workers; defaults to 1 if <= 0
//   - input: Channel to receive block items from (required for processing)
//   - output: Channel to send processed items to (required for forwarding)
//   - errors: Channel to send errors to; may be nil (errors will be dropped)
//
// Note: If input or output channels are nil, workers will block indefinitely
// when attempting to receive or send items.
//
// Deprecated: Use NewStageWorkerPool with DecodeMetricsRecorder instead.
func NewDecodeStageWorkerPool(
	stage *DecodeStage,
	numWorkers int,
	input <-chan *BlockItem,
	output chan<- *BlockItem,
	errors chan<- error,
) *DecodeStageWorkerPool {
	if stage == nil {
		panic(ErrNilStage)
	}
	return &DecodeStageWorkerPool{
		stage:      stage,
		numWorkers: numWorkers,
		input:      input,
		output:     output,
		errors:     errors,
	}
}

// SetMetrics sets the metrics collector for the worker pool.
// Must be called before Start() to avoid data races.
func (p *DecodeStageWorkerPool) SetMetrics(metrics *PipelineMetrics) {
	p.metrics = metrics
}

// Start starts the worker pool. Call Stop to wait for completion.
// This method is idempotent - calling it multiple times has no effect.
func (p *DecodeStageWorkerPool) Start(ctx context.Context) {
	p.startOnce.Do(func() {
		// Create the underlying pool with metrics now that they've been set
		p.pool = NewStageWorkerPool(StageWorkerPoolConfig{
			Stage:         p.stage,
			NumWorkers:    p.numWorkers,
			Input:         p.input,
			Output:        p.output,
			Errors:        p.errors,
			RecordMetrics: DecodeMetricsRecorder(p.metrics),
		})
		p.pool.Start(ctx)
	})
}

// Stop waits for all workers to complete.
func (p *DecodeStageWorkerPool) Stop() {
	if p.pool != nil {
		p.pool.Stop()
	}
}
