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
	"runtime"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// BlockTypeResolver resolves the exact block type that is active for a signed
// header. The input is the preserved header CBOR and does not include the
// untrusted wire discriminator. Decode workers may call the resolver
// concurrently and out of submission order, before earlier blocks reach
// ApplyFunc. Implementations must be concurrency-safe and resolve from an
// immutable or atomically read hard-fork schedule keyed by the signed header;
// they must not depend on state changes from earlier in-flight blocks.
type BlockTypeResolver func(context.Context, []byte) (uint, error)

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
	// Submit applies backpressure before assigning a sequence when accepting
	// another block could exceed this limit. One additional sequence may be in
	// flight to occupy the missing earliest position. Default is 2160 (Cardano
	// security parameter k); zero disables the limit.
	MaxPendingBlocks int
	// Eta0Provider dynamically provides the epoch nonce for each block's slot.
	// This is required for VRF validation since the epoch nonce changes every epoch.
	// For simple test cases, use StaticEta0Provider to wrap a constant value.
	Eta0Provider Eta0Provider
	// SlotsPerKesPeriod is the number of slots per KES period.
	SlotsPerKesPeriod uint64
	// VerifyConfig contains verification options.
	VerifyConfig common.VerifyConfig
	// BlockTypeResolver binds a signed header to the authoritative active era
	// before an era-specific decoder is selected.
	BlockTypeResolver BlockTypeResolver
	// ChainContextValidator validates each decoded block against authoritative
	// chain state immediately before ordered application.
	ChainContextValidator ChainContextValidator
	// ApplyFunc commits validated blocks to authoritative state in order. It is
	// required outside trusted decode-only mode.
	ApplyFunc ApplyFunc
	// MetricsWindowSize is the number of samples to keep for latency metrics.
	MetricsWindowSize int
	// SkipBodyHashValidation disables body hash validation during decode.
	SkipBodyHashValidation bool
	// TrustedDecodeOnly permits decoding without validation or application.
	// Callers must opt into this mode explicitly.
	TrustedDecodeOnly bool
}

// DefaultPipelineConfig returns a PipelineConfig with sensible sizing defaults.
// Normal processing also requires nonzero validation workers and
// SlotsPerKesPeriod, plus BlockTypeResolver, Eta0Provider,
// ChainContextValidator, and ApplyFunc. Callers that only decode trusted
// persisted data must opt into trusted decode-only mode instead.
func DefaultPipelineConfig() PipelineConfig {
	numCPU := runtime.NumCPU()

	// Scale decode workers with CPU count (decode is faster than validate)
	decodeWorkers := max(numCPU/4, 2)

	return PipelineConfig{
		DecodeWorkers:      decodeWorkers,
		ValidateWorkers:    0,
		PrefetchBufferSize: 1000,                    // Large enough for typical chain gaps
		MaxPendingBlocks:   DefaultMaxPendingBlocks, // Cardano security parameter k
		MetricsWindowSize:  1000,
	}
}

// WithTrustedDecodeOnly configures the pipeline to decode blocks and return
// them through Results without validation or application. It is intended for
// trusted persisted data whose consumer performs validation separately.
func WithTrustedDecodeOnly() PipelineOption {
	return func(c *PipelineConfig) {
		c.TrustedDecodeOnly = true
	}
}

// WithBlockTypeResolver sets the authoritative resolver used before typed
// block decoding. It must derive the active block type from the signed header
// and trusted hard-fork state.
func WithBlockTypeResolver(resolver BlockTypeResolver) PipelineOption {
	return func(c *PipelineConfig) {
		c.BlockTypeResolver = resolver
	}
}

// WithChainContextValidator sets the authoritative validator run in sequence
// immediately before ApplyFunc. The validator checks linkage and consensus
// state after BlockTypeResolver has bound the signed header to the active era.
func WithChainContextValidator(validator ChainContextValidator) PipelineOption {
	return func(c *PipelineConfig) {
		c.ChainContextValidator = validator
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

// WithValidateWorkers sets the number of block-local validate workers.
// Set to 0 only with WithTrustedDecodeOnly.
// When validation is enabled (n > 0), the apply stage refuses to apply any
// block that has not actually passed validation, reporting
// ErrBlockNotValidated on the errors channel.
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
// Submit applies backpressure before assigning a sequence when accepting another
// block could exceed this limit. Passing zero disables the limit. The default
// is 2160 (Cardano security parameter).
func WithMaxPendingBlocks(n int) PipelineOption {
	return func(c *PipelineConfig) {
		if n >= 0 {
			c.MaxPendingBlocks = n
		}
	}
}

// WithApplyFunc sets the authoritative state commit function. A nil function
// is ignored and causes Start to reject a normal pipeline.
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
