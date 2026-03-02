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

// DefaultMaxPendingBlocks is the default limit for out-of-order blocks buffered
// in the apply stage. This matches the Cardano security parameter (k=2160) which
// defines the immutability window.
const DefaultMaxPendingBlocks = 2160

// PipelineConfig holds configuration for a BlockPipeline.
type PipelineConfig struct {
	// DecodeWorkers is the number of parallel decode workers.
	DecodeWorkers int
	// ValidateWorkers is the number of parallel validate workers.
	ValidateWorkers int
	// PrefetchBufferSize is the buffer size for inter-stage channels.
	PrefetchBufferSize int
	// MaxPendingBlocks limits out-of-order blocks buffered in the apply stage.
	// This prevents unbounded memory growth when blocks arrive out of order.
	// Default is 2160 (Cardano security parameter k).
	MaxPendingBlocks int
	// Eta0Provider dynamically provides the epoch nonce for each block's slot.
	// This is required for VRF validation since the epoch nonce changes every epoch.
	// For simple test cases, use StaticEta0Provider to wrap a constant value.
	Eta0Provider Eta0Provider
	// SlotsPerKesPeriod is the number of slots per KES period.
	SlotsPerKesPeriod uint64
	// VerifyConfig contains verification options.
	VerifyConfig common.VerifyConfig
	// ApplyFunc is the function called to apply blocks in order.
	ApplyFunc ApplyFunc
	// MetricsWindowSize is the number of samples to keep for latency metrics.
	MetricsWindowSize int
	// SkipBodyHashValidation disables body hash validation during decode.
	SkipBodyHashValidation bool
}

// DefaultPipelineConfig returns a PipelineConfig with sensible defaults.
// Validation is disabled by default (ValidateWorkers = 0) to prevent nil-pointer
// panics when Eta0Provider is not configured. To enable validation, use
// WithValidateWorkers() and ensure Eta0Provider and SlotsPerKesPeriod are configured.
func DefaultPipelineConfig() PipelineConfig {
	numCPU := runtime.NumCPU()

	// Scale decode workers with CPU count (decode is faster than validate)
	decodeWorkers := numCPU / 4
	if decodeWorkers < 2 {
		decodeWorkers = 2
	}

	return PipelineConfig{
		DecodeWorkers:      decodeWorkers,
		ValidateWorkers:    0,                       // Validation is opt-in; requires Eta0Provider
		PrefetchBufferSize: 1000,                    // Large enough for typical chain gaps
		MaxPendingBlocks:   DefaultMaxPendingBlocks, // Cardano security parameter k
		MetricsWindowSize:  1000,
	}
}

// PipelineOption is a functional option for configuring a BlockPipeline.
type PipelineOption func(*PipelineConfig)

// WithConfig applies a complete PipelineConfig, replacing all default values.
// This is useful when migrating from the old config-based constructor pattern
// or when you have a pre-configured PipelineConfig struct.
//
// Note: Options applied after WithConfig will still override the config values.
//
// Example:
//
//	config := DefaultPipelineConfig()
//	config.DecodeWorkers = 8
//	p := NewBlockPipeline(WithConfig(config))
func WithConfig(config PipelineConfig) PipelineOption {
	return func(c *PipelineConfig) {
		*c = config
	}
}

// WithDecodeWorkers sets the number of decode workers.
func WithDecodeWorkers(n int) PipelineOption {
	return func(c *PipelineConfig) {
		if n > 0 {
			c.DecodeWorkers = n
		}
	}
}

// WithValidateWorkers sets the number of validate workers.
// Set to 0 to disable validation entirely (useful for trusted block sources).
func WithValidateWorkers(n int) PipelineOption {
	return func(c *PipelineConfig) {
		if n >= 0 {
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

// WithMaxPendingBlocks sets the limit for out-of-order blocks in the apply stage.
// This prevents unbounded memory growth. Default is 2160 (Cardano security parameter).
func WithMaxPendingBlocks(n int) PipelineOption {
	return func(c *PipelineConfig) {
		if n > 0 {
			c.MaxPendingBlocks = n
		}
	}
}

// WithApplyFunc sets the apply function.
// A nil function is ignored (the pipeline will use a no-op apply).
func WithApplyFunc(fn ApplyFunc) PipelineOption {
	return func(c *PipelineConfig) {
		if fn != nil {
			c.ApplyFunc = fn
		}
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
		c.Eta0Provider = StaticEta0Provider(eta0)
	}
}

// WithEta0Provider sets a dynamic epoch nonce provider for validation.
// The provider is called for each block with its slot number and must return
// the correct epoch nonce for that slot's epoch. This is required for
// production use since the epoch nonce changes every epoch.
//
// Example:
//
//	pipeline := NewBlockPipeline(
//	    WithEta0Provider(func(slot uint64) (string, error) {
//	        epoch := slot / slotsPerEpoch // Calculate epoch from slot
//	        nonce, err := ledgerState.EpochNonce(epoch)
//	        return hex.EncodeToString(nonce), err
//	    }),
//	)
func WithEta0Provider(provider Eta0Provider) PipelineOption {
	return func(c *PipelineConfig) {
		c.Eta0Provider = provider
	}
}

// WithSlotsPerKesPeriod sets the slots per KES period for validation.
func WithSlotsPerKesPeriod(slots uint64) PipelineOption {
	return func(c *PipelineConfig) {
		c.SlotsPerKesPeriod = slots
	}
}

// WithVerifyConfig sets the verify config for validation.
func WithVerifyConfig(config common.VerifyConfig) PipelineOption {
	return func(c *PipelineConfig) {
		c.VerifyConfig = config
	}
}
