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
	"sync/atomic"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

func TestBlockPipelineOrderedParametersRefresh(t *testing.T) {
	block := getValidShelleyBlock(t)
	var applied atomic.Uint64
	var configured atomic.Uint64
	options := validatedTestPipelineOptions(t, func(*BlockItem) error {
		applied.Add(1)
		return nil
	})
	config := validatingTestVerifyConfig(t)
	options = append(
		options,
		WithValidateWorkers(2),
		WithVerifyConfigProvider(
			func(_ context.Context, item *BlockItem) (common.VerifyConfig, error) {
				if applied.Load() != item.SequenceNumber() {
					return common.VerifyConfig{}, fmt.Errorf(
						"state read before predecessor applied",
					)
				}
				configured.Add(1)
				current := config
				if item.SequenceNumber() == 1 {
					params := *config.ProtocolParameters.(*shelley.ShelleyProtocolParameters)
					params.MaxBlockHeaderSize = 1
					current.ProtocolParameters = &params
				}
				return current, nil
			},
		),
	)
	p := NewBlockPipeline(options...)
	require.NoError(t, p.Start(t.Context()))
	defer p.Stop()
	for range 2 {
		require.NoError(t, p.Submit(t.Context(), block.BlockType,
			block.Cbor, createTestTip(1, 1)))
	}
	require.ErrorContains(
		t,
		p.Fence(t.Context()),
		"block header size exceeds protocol maximum",
	)
	require.Equal(
		t,
		uint64(2),
		configured.Load(),
		"refresh state for every ordered block",
	)
	require.Equal(
		t,
		uint64(1),
		applied.Load(),
		"new parameters must reject the next block before apply",
	)
}

func TestBlockPipelineHeaderOnlyReturnsValidatedOrderedResults(t *testing.T) {
	block := getValidShelleyBlock(t)
	p := NewBlockPipeline(WithHeaderOnly(), WithValidateWorkers(2),
		WithBlockTypeResolver(resolveTestBlockType), WithEta0(shelleyBlockEta0),
		WithSlotsPerKesPeriod(129600))
	require.NoError(t, p.Start(t.Context()))
	defer p.Stop()
	require.NoError(t, p.Submit(t.Context(), block.BlockType,
		block.Cbor, createTestTip(1, 1)))
	require.NoError(t, p.Fence(t.Context()))
	item := <-p.Results()
	require.True(t, item.IsValid())
	require.False(t, item.IsApplied())
	require.False(t, p.ValidatesPeerBlocks())
	require.NoError(t, p.Submit(t.Context(), block.BlockType,
		[]byte{0xff}, createTestTip(2, 2)))
	require.ErrorIs(t, p.Fence(t.Context()), ErrPipelineItemRejected)
	select {
	case item := <-p.Results():
		require.Nil(t, item, "malformed preparation must not emit a result")
	default:
	}
}

func TestOrderedProviderPanicPreservesBufferedPrefix(t *testing.T) {
	stage := newTestApplyStage(func(*BlockItem) error { return nil }, 0)
	stage.orderedValidator = func(_ context.Context, item *BlockItem) error {
		if item.SequenceNumber() == 1 {
			panic("state unavailable")
		}
		return nil
	}
	second := newDecodedTestItem(t, 1)
	first := newDecodedTestItem(t, 0)
	_, err := stage.ProcessWithStatus(t.Context(), second)
	require.NoError(t, err)
	processed, err := stage.ProcessWithStatus(t.Context(), first)
	require.ErrorIs(t, err, ErrStagePanic)
	require.True(
		t,
		first.IsApplied(),
		"committed predecessor survives provider panic",
	)
	require.Contains(
		t,
		processed,
		first,
		"committed prefix remains available for forwarding",
	)
	require.False(t, second.IsApplied())
}

func TestOrderedProviderRejectsMissingStateAndBypasses(t *testing.T) {
	for _, test := range []struct {
		name    string
		change  func(*common.VerifyConfig)
		message string
	}{
		{"state", func(c *common.VerifyConfig) { c.LedgerState = nil }, "requires ledger state"},
		{"parameters", func(c *common.VerifyConfig) { c.ProtocolParameters = nil }, "requires protocol parameters"},
		{"transactions", func(c *common.VerifyConfig) { c.SkipTransactionValidation = true }, "SkipTransactionValidation"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var applied atomic.Uint64
			options := validatedTestPipelineOptions(t, func(*BlockItem) error {
				applied.Add(1)
				return nil
			})
			config := validatingTestVerifyConfig(t)
			test.change(&config)
			options = append(
				options,
				WithVerifyConfigProvider(
					func(context.Context, *BlockItem) (common.VerifyConfig, error) {
						return config, nil
					},
				),
			)
			p := NewBlockPipeline(options...)
			require.NoError(t, p.Start(t.Context()))
			defer p.Stop()
			block := getValidShelleyBlock(t)
			require.NoError(
				t,
				p.Submit(
					t.Context(),
					block.BlockType,
					block.Cbor,
					createTestTip(1, 1),
				),
			)
			require.ErrorContains(t, p.Fence(t.Context()), test.message)
			require.Zero(t, applied.Load())
		})
	}
}
