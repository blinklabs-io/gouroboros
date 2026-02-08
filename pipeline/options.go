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
	"runtime"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// PipelineConfig holds configuration for a BlockPipeline.
type PipelineConfig struct {
	// DecodeWorkers is the number of parallel decode workers.
	DecodeWorkers int
	// ValidateWorkers is the number of parallel validate workers.
	ValidateWorkers int
	// PrefetchBufferSize is the buffer size for inter-stage channels.
	PrefetchBufferSize int
	// ValidateConfig holds validation configuration.
	ValidateConfig ValidateStageConfig
	// ApplyFunc is the function called to apply blocks in order.
	ApplyFunc ApplyFunc
	// MetricsWindowSize is the number of samples to keep for latency metrics.
	MetricsWindowSize int
	// SkipBodyHashValidation disables body hash validation during decode.
	SkipBodyHashValidation bool
}

// DefaultPipelineConfig returns a PipelineConfig with sensible defaults.
func DefaultPipelineConfig() PipelineConfig {
	numCPU := runtime.NumCPU()
	validateWorkers := numCPU / 2
	if validateWorkers < 1 {
		validateWorkers = 1
	}

	return PipelineConfig{
		DecodeWorkers:      2,
		ValidateWorkers:    validateWorkers,
		PrefetchBufferSize: 100,
		MetricsWindowSize:  1000,
	}
}

// PipelineOption is a functional option for configuring a BlockPipeline.
type PipelineOption func(*PipelineConfig)

// WithDecodeWorkers sets the number of decode workers.
func WithDecodeWorkers(n int) PipelineOption {
	return func(c *PipelineConfig) {
		if n > 0 {
			c.DecodeWorkers = n
		}
	}
}

// WithValidateWorkers sets the number of validate workers.
func WithValidateWorkers(n int) PipelineOption {
	return func(c *PipelineConfig) {
		if n > 0 {
			c.ValidateWorkers = n
		}
	}
}

// WithPrefetchBufferSize sets the buffer size for inter-stage channels.
func WithPrefetchBufferSize(size int) PipelineOption {
	return func(c *PipelineConfig) {
		if size > 0 {
			c.PrefetchBufferSize = size
		}
	}
}

// WithValidateConfig sets the validation configuration.
func WithValidateConfig(config ValidateStageConfig) PipelineOption {
	return func(c *PipelineConfig) {
		c.ValidateConfig = config
	}
}

// WithApplyFunc sets the apply function.
func WithApplyFunc(fn ApplyFunc) PipelineOption {
	return func(c *PipelineConfig) {
		c.ApplyFunc = fn
	}
}

// WithMetricsWindowSize sets the metrics window size.
func WithMetricsWindowSize(size int) PipelineOption {
	return func(c *PipelineConfig) {
		if size > 0 {
			c.MetricsWindowSize = size
		}
	}
}

// WithSkipBodyHashValidation sets whether to skip body hash validation.
func WithSkipBodyHashValidation(skip bool) PipelineOption {
	return func(c *PipelineConfig) {
		c.SkipBodyHashValidation = skip
	}
}

// WithEta0 sets a static epoch nonce (eta0) for validation.
// This is a convenience wrapper for simple test cases where all blocks
// are from the same epoch. For production use, prefer WithEta0Provider.
func WithEta0(eta0 string) PipelineOption {
	return func(c *PipelineConfig) {
		c.ValidateConfig.Eta0Provider = StaticEta0Provider(eta0)
	}
}

// WithEta0Provider sets a dynamic epoch nonce provider for validation.
// The provider is called for each block with its slot number and must return
// the correct epoch nonce for that slot's epoch. This is required for
// production use since the epoch nonce changes every epoch.
//
// Example:
//
//	pipeline := NewBlockPipelineWithOptions(
//	    WithEta0Provider(func(slot uint64) (string, error) {
//	        epoch := slot / slotsPerEpoch // Calculate epoch from slot
//	        nonce, err := ledgerState.EpochNonce(epoch)
//	        return hex.EncodeToString(nonce), err
//	    }),
//	)
func WithEta0Provider(provider Eta0Provider) PipelineOption {
	return func(c *PipelineConfig) {
		c.ValidateConfig.Eta0Provider = provider
	}
}

// WithSlotsPerKesPeriod sets the slots per KES period for validation.
func WithSlotsPerKesPeriod(slots uint64) PipelineOption {
	return func(c *PipelineConfig) {
		c.ValidateConfig.SlotsPerKesPeriod = slots
	}
}

// WithVerifyConfig sets the verify config for validation.
func WithVerifyConfig(config common.VerifyConfig) PipelineOption {
	return func(c *PipelineConfig) {
		c.ValidateConfig.VerifyConfig = config
	}
}
