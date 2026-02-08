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
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// Eta0Provider is a function that returns the epoch nonce (eta0) for a given slot.
// This allows dynamic lookup of the correct nonce for each block's epoch,
// which is essential for VRF validation across epoch boundaries.
//
// Parameters:
//   - slot: The slot number of the block being validated
//
// Returns:
//   - string: The epoch nonce in hex format (64 hex characters = 32 bytes)
//   - error: An error if the nonce cannot be determined for the given slot
type Eta0Provider func(slot uint64) (string, error)

// ValidateStageConfig holds configuration for the validate stage.
type ValidateStageConfig struct {
	// Eta0Provider dynamically provides the epoch nonce for each block's slot.
	// This is required for VRF validation since the epoch nonce changes every epoch.
	// For simple test cases, use StaticEta0Provider to wrap a constant value.
	Eta0Provider Eta0Provider
	// SlotsPerKesPeriod is the number of slots per KES period.
	SlotsPerKesPeriod uint64
	// VerifyConfig contains verification options.
	VerifyConfig common.VerifyConfig
}

// StaticEta0Provider returns an Eta0Provider that always returns the same nonce.
// This is useful for tests or scenarios where all blocks are from the same epoch.
func StaticEta0Provider(eta0 string) Eta0Provider {
	return func(slot uint64) (string, error) {
		return eta0, nil
	}
}

// ValidateStage validates decoded blocks using VRF, KES, and other checks.
type ValidateStage struct {
	config ValidateStageConfig
}

// NewValidateStage creates a new ValidateStage with the given configuration.
func NewValidateStage(config ValidateStageConfig) *ValidateStage {
	return &ValidateStage{
		config: config,
	}
}

// Name returns the stage name.
func (s *ValidateStage) Name() string {
	return "validate"
}

// Process validates the block in the item.
func (s *ValidateStage) Process(ctx context.Context, item *BlockItem) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Skip validation if decode failed - the decode stage already reported
	// the error, so return nil to avoid generating a spurious duplicate error.
	if !item.IsDecoded() {
		return nil
	}

	start := time.Now()

	block := item.Block()
	slot := block.SlotNumber()

	// Get the epoch nonce from the provider
	if s.config.Eta0Provider == nil {
		duration := time.Since(start)
		configErr := fmt.Errorf("eta0 provider not configured for slot %d", slot)
		item.SetValidation(false, "", configErr, duration)
		return configErr
	}

	eta0, err := s.config.Eta0Provider(slot)
	if err != nil {
		duration := time.Since(start)
		providerErr := fmt.Errorf("eta0 provider error for slot %d: %w", slot, err)
		item.SetValidation(false, "", providerErr, duration)
		return providerErr
	}

	isValid, vrfOutput, _, _, err := ledger.VerifyBlock(
		block,
		eta0,
		s.config.SlotsPerKesPeriod,
		s.config.VerifyConfig,
	)

	duration := time.Since(start)

	if err != nil {
		item.SetValidation(false, "", err, duration)
		return err
	}

	if !isValid {
		validationErr := fmt.Errorf("block validation failed at slot %d", slot)
		item.SetValidation(false, vrfOutput, validationErr, duration)
		return validationErr
	}

	item.SetValidation(true, vrfOutput, nil, duration)
	return nil
}

// ValidateStageWorkerPool runs multiple validate workers in parallel.
type ValidateStageWorkerPool struct {
	stage      *ValidateStage
	numWorkers int
	input      <-chan *BlockItem
	output     chan<- *BlockItem
	errors     chan<- error
	metrics    *PipelineMetrics
	wg         sync.WaitGroup
	started    atomic.Bool
}

// NewValidateStageWorkerPool creates a new worker pool for the validate stage.
//
// Parameters:
//   - stage: The validate stage to use for processing (required, panics if nil)
//   - numWorkers: Number of parallel workers; defaults to 1 if <= 0
//   - input: Channel to receive block items from (required for processing)
//   - output: Channel to send processed items to (required for forwarding)
//   - errors: Channel to send errors to; may be nil (errors will be dropped)
//
// Note: If input or output channels are nil, workers will block indefinitely
// when attempting to receive or send items.
func NewValidateStageWorkerPool(
	stage *ValidateStage,
	numWorkers int,
	input <-chan *BlockItem,
	output chan<- *BlockItem,
	errors chan<- error,
) *ValidateStageWorkerPool {
	if stage == nil {
		panic(ErrNilStage)
	}
	if numWorkers <= 0 {
		numWorkers = 1
	}
	return &ValidateStageWorkerPool{
		stage:      stage,
		numWorkers: numWorkers,
		input:      input,
		output:     output,
		errors:     errors,
	}
}

// SetMetrics sets the metrics collector for the worker pool.
func (p *ValidateStageWorkerPool) SetMetrics(metrics *PipelineMetrics) {
	p.metrics = metrics
}

// Start starts the worker pool.
// This method is idempotent - calling it multiple times has no effect.
func (p *ValidateStageWorkerPool) Start(ctx context.Context) {
	if p.started.Swap(true) {
		return // Already started
	}
	for i := 0; i < p.numWorkers; i++ {
		p.wg.Add(1)
		go p.worker(ctx)
	}
}

// Stop waits for all workers to complete.
func (p *ValidateStageWorkerPool) Stop() {
	p.wg.Wait()
}

func (p *ValidateStageWorkerPool) worker(ctx context.Context) {
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

			// Record metrics (only for decoded items that were actually validated)
			if p.metrics != nil && item.IsDecoded() {
				p.metrics.RecordValidate(item.ValidateDuration(), err)
			}

			if err != nil && p.errors != nil {
				select {
				case p.errors <- err:
				case <-ctx.Done():
					return
				}
			}

			// Forward to next stage
			select {
			case p.output <- item:
			case <-ctx.Done():
				return
			}
		}
	}
}
