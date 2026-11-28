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
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// panicOnSecondCall panics on its second call and counts the others. The
// decode and validate stages run one worker here, so the second call is the
// block with sequence number 1.
type panicOnSecondCall struct {
	calls atomic.Int64
}

func (c *panicOnSecondCall) next() {
	if c.calls.Add(1) == 2 {
		panic(stagePanicValue)
	}
}

// TestBlockPipelineStagePanicOutcome checks that a panic in any stage or
// consumer callback fails its own block, which is fatal, so:
//   - only the blocks before it are committed, in order, and none after it;
//   - Fence and WaitForDrain return the contained panic rather than success
//     or waiting for ever;
//   - Stop returns, every goroutine has exited, and the retained-CBOR budget
//     and active items are back to zero.
//
// It fails if the panicking item reaches ApplyFunc looking decoded, if the
// runner goes on to the next block, or if the worker dies holding the item
// so that Fence never returns.
func TestBlockPipelineStagePanicOutcome(t *testing.T) {
	const blocks = 3
	tests := []struct {
		name   string
		where  string
		inject func(*panicOnSecondCall) []PipelineOption
	}{
		{
			name:  "block type resolver",
			where: "decode stage",
			inject: func(c *panicOnSecondCall) []PipelineOption {
				return []PipelineOption{WithBlockTypeResolver(
					func(ctx context.Context, header []byte) (uint, error) {
						c.next()
						return resolveTestBlockType(ctx, header)
					},
				)}
			},
		},
		{
			name:  "eta0 provider",
			where: "validate stage",
			inject: func(c *panicOnSecondCall) []PipelineOption {
				return []PipelineOption{WithEta0Provider(
					func(uint64) (string, error) {
						c.next()
						return shelleyBlockEta0, nil
					},
				)}
			},
		},
		{
			name:  "chain-context validator",
			where: "chain-context validator",
			inject: func(c *panicOnSecondCall) []PipelineOption {
				return []PipelineOption{WithChainContextValidator(
					func(context.Context, *BlockItem) error {
						c.next()
						return nil
					},
				)}
			},
		},
		{
			name:  "verify config provider",
			where: "ordered validator",
			inject: func(c *panicOnSecondCall) []PipelineOption {
				config := validatingTestVerifyConfig(t)
				return []PipelineOption{WithVerifyConfigProvider(
					func(context.Context, *BlockItem) (common.VerifyConfig, error) {
						c.next()
						return config, nil
					},
				)}
			},
		},
		{
			name:  "apply function",
			where: "apply function",
			inject: func(*panicOnSecondCall) []PipelineOption {
				return nil
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ignore := goleak.IgnoreCurrent()
			var (
				mu      sync.Mutex
				applied []uint64
			)
			calls := &panicOnSecondCall{}
			apply := func(item *BlockItem) error {
				if test.name == "apply function" {
					calls.next()
				}
				mu.Lock()
				applied = append(applied, item.SequenceNumber())
				mu.Unlock()
				return nil
			}
			opts := append(
				validatedTestPipelineOptions(t, apply),
				test.inject(calls)...,
			)
			p := NewBlockPipeline(opts...)
			ctx, cancel := context.WithTimeout(
				context.Background(),
				10*time.Second,
			)
			defer cancel()
			require.NoError(t, p.Start(context.Background()))

			shelley := getValidShelleyBlock(t)
			for i := range blocks {
				err := p.Submit(
					ctx,
					shelley.BlockType,
					shelley.Cbor,
					createTestTip(uint64(1000+i), uint64(500+i)),
				)
				// A later Submit may see the pipeline already stopped by
				// the panic; it must then report that panic.
				if err != nil {
					require.ErrorIs(t, err, ErrStagePanic, "submit %d", i)
				}
			}

			err := p.Fence(ctx)
			require.ErrorIs(t, err, ErrStagePanic)
			require.ErrorContains(t, err, test.where)
			require.ErrorContains(t, err, stagePanicValue)
			require.ErrorIs(t, p.WaitForDrain(ctx), ErrStagePanic)

			mu.Lock()
			require.Equal(t, []uint64{0}, applied,
				"only the block before the panic commits")
			mu.Unlock()
			results := p.DrainResults()
			sequences := make([]uint64, 0, len(results))
			for _, item := range results {
				require.True(t, item.IsApplied())
				sequences = append(sequences, item.SequenceNumber())
				item.Release()
			}
			require.Equal(t, []uint64{0}, sequences,
				"Results holds the committed prefix only")

			require.NoError(t, p.Stop(ctx))
			p.rawBudgetMu.Lock()
			inUse := p.rawBytesInUse
			p.rawBudgetMu.Unlock()
			require.Zero(t, inUse, "retained CBOR budget after Stop")
			p.activeItemsMu.Lock()
			active := len(p.activeItems)
			p.activeItemsMu.Unlock()
			require.Zero(t, active, "items holding a reservation after Stop")
			require.NoError(t, goleak.Find(ignore))
		})
	}
}

// TestDecodeStageRejectsEveryWrongDeclaredType checks the decode stage's
// block-type check against every real era fixture: whatever the wire claims,
// a block whose authoritative type differs is rejected with
// BlockTypeMismatchError before any typed decoder runs, and the true type
// decodes. Shelley, Allegra and Mary share a wire format, so for those the
// claimed era's decoder would accept the bytes; the check is then the only
// thing that stops a block being processed under the wrong era's rules.
func TestDecodeStageRejectsEveryWrongDeclaredType(t *testing.T) {
	declarable := []uint{
		ledger.BlockTypeByronEbb,
		ledger.BlockTypeByronMain,
		ledger.BlockTypeShelley,
		ledger.BlockTypeAllegra,
		ledger.BlockTypeMary,
		ledger.BlockTypeAlonzo,
		ledger.BlockTypeBabbage,
		ledger.BlockTypeConway,
		ledger.BlockTypeDijkstra,
	}
	acceptedByWrongDecoder := 0
	for _, fixture := range testdata.GetTestBlocks() {
		resolver := func(context.Context, []byte) (uint, error) {
			return fixture.BlockType, nil
		}
		stage := NewDecodeStage(false)
		stage.SetBlockTypeResolver(resolver)
		for _, declared := range declarable {
			item := NewBlockItem(
				declared,
				fixture.Cbor,
				createTestTip(1, 1),
				0,
			)
			err := stage.Process(t.Context(), item)
			if declared == fixture.BlockType {
				require.NoError(t, err, fixture.Name)
				require.NotNil(t, item.Block(), fixture.Name)
				require.Equal(t, int(fixture.BlockType), item.Block().Type())
				continue
			}
			var mismatch *BlockTypeMismatchError
			require.ErrorAs(t, err, &mismatch, "%s declared %d",
				fixture.Name, declared)
			require.Equal(t, fixture.BlockType, mismatch.Expected)
			require.Equal(t, declared, mismatch.Actual)
			require.ErrorIs(t, item.DecodeError(), ErrBlockTypeMismatch)
			require.Nil(t, item.Block(),
				"%s declared %d reached a typed decoder", fixture.Name, declared)
			if _, decodeErr := ledger.NewBlockFromCbor(
				declared,
				fixture.Cbor,
			); decodeErr == nil {
				acceptedByWrongDecoder++
			}
		}
	}
	require.Positive(t, acceptedByWrongDecoder,
		"no fixture decodes under a wrong era, so the test proves nothing")
}

// TestBlockPipelineResultsMatchSubmittedCbor checks that no stage changes a
// decoded block behind its preserved CBOR. A block decoded from the wire
// keeps the original bytes and MarshalCBOR returns them unchanged, so a
// stage that mutated a field without clearing them would emit the old bytes
// with a different value in memory. For every era fixture the delivered
// block must re-encode to the submitted bytes, and those bytes must decode
// to the same block hash, transaction ids and validity flags the delivered
// value reports.
func TestBlockPipelineResultsMatchSubmittedCbor(t *testing.T) {
	fixtures := testdata.GetTestBlocks()
	p := NewBlockPipeline(
		WithTrustedDecodeOnly(),
		WithDecodeWorkers(2),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, p.Start(ctx))
	t.Cleanup(func() { require.NoError(t, p.Stop(context.Background())) })

	submitErr := make(chan error, 1)
	go func() {
		for i, fixture := range fixtures {
			raw := slices.Clone(fixture.Cbor)
			if err := p.Submit(
				ctx,
				fixture.BlockType,
				raw,
				createTestTip(uint64(i), uint64(i)),
			); err != nil {
				submitErr <- err
				return
			}
		}
		submitErr <- nil
	}()
	for i, fixture := range fixtures {
		var item *BlockItem
		select {
		case item = <-p.Results():
		case <-ctx.Done():
			t.Fatalf("no result for %s", fixture.Name)
		}
		require.Equal(t, uint64(i), item.SequenceNumber())
		require.NoError(t, item.DecodeError(), fixture.Name)
		block := item.Block()
		require.NotNil(t, block, fixture.Name)
		require.True(t, bytes.Equal(fixture.Cbor, block.Cbor()),
			"%s: preserved CBOR differs from the submitted bytes", fixture.Name)
		encoded, err := cbor.Encode(block)
		require.NoError(t, err, fixture.Name)
		require.True(t, bytes.Equal(fixture.Cbor, encoded),
			"%s: MarshalCBOR differs from the submitted bytes", fixture.Name)

		fresh, err := ledger.NewBlockFromCbor(fixture.BlockType, encoded)
		require.NoError(t, err, fixture.Name)
		require.Equal(t, fresh.Hash(), block.Hash(), fixture.Name)
		got := block.Transactions()
		want := fresh.Transactions()
		require.Len(t, got, len(want), fixture.Name)
		for j := range want {
			require.Equal(t, want[j].Hash(), got[j].Hash(),
				"%s transaction %d", fixture.Name, j)
			require.Equal(t, want[j].IsValid(), got[j].IsValid(),
				"%s transaction %d", fixture.Name, j)
		}
		item.Release()
	}
	require.NoError(t, <-submitErr)
	require.False(t, errors.Is(p.Fence(ctx), ErrStagePanic))
}
