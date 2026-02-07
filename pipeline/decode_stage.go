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

	block, err := ledger.NewBlockFromCbor(item.BlockType, item.RawCbor, config)
	duration := time.Since(start)

	if err != nil {
		item.SetDecodeError(err, duration)
		return err
	}

	item.SetBlock(block, duration)
	return nil
}

// DecodeStageWorkerPool runs multiple decode workers in parallel.
type DecodeStageWorkerPool struct {
	stage      *DecodeStage
	numWorkers int
	input      <-chan *BlockItem
	output     chan<- *BlockItem
	errors     chan<- error
	metrics    *PipelineMetrics
	wg         sync.WaitGroup
	started    atomic.Bool
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
	if numWorkers <= 0 {
		numWorkers = 1
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
func (p *DecodeStageWorkerPool) SetMetrics(metrics *PipelineMetrics) {
	p.metrics = metrics
}

// Start starts the worker pool. Call Stop to wait for completion.
// This method is idempotent - calling it multiple times has no effect.
func (p *DecodeStageWorkerPool) Start(ctx context.Context) {
	if p.started.Swap(true) {
		return // Already started
	}
	for i := 0; i < p.numWorkers; i++ {
		p.wg.Add(1)
		go p.worker(ctx)
	}
}

// Stop waits for all workers to complete.
func (p *DecodeStageWorkerPool) Stop() {
	p.wg.Wait()
}

func (p *DecodeStageWorkerPool) worker(ctx context.Context) {
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

			// Record metrics
			if p.metrics != nil {
				p.metrics.RecordDecode(item.DecodeDuration(), err)
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
