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
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/stretchr/testify/require"
)

// Chain-sync submits one block and fences it before reading the next message,
// and it never reads Results. Committing must not wait for a reader.
func TestBlockPipelineFenceDoesNotWaitForResultsReader(t *testing.T) {
	const blocks = 10
	var applied atomic.Int64
	p := NewBlockPipeline(append(
		validatedTestPipelineOptions(t, func(*BlockItem) error {
			applied.Add(1)
			return nil
		}),
		WithPrefetchBufferSize(1),
	)...)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, p.Start(ctx))
	t.Cleanup(func() { require.NoError(t, p.Stop(context.Background())) })

	shelley := getValidShelleyBlock(t)
	for i := range blocks {
		require.NoError(t, p.Submit(
			ctx,
			shelley.BlockType,
			shelley.Cbor,
			createTestTip(1000, 500),
		), "submit %d", i)
		require.NoError(
			t,
			p.Fence(ctx),
			"fence after block %d with %d applied",
			i,
			applied.Load(),
		)
	}
	require.EqualValues(t, blocks, applied.Load())

	// A one-result buffer keeps only the latest commit for a late reader.
	// Fence returns at commit, so wait for the last handoff's drop.
	require.Eventually(t, func() bool {
		return p.Stats().ResultsDropped == blocks-1
	}, time.Second, time.Millisecond)
	select {
	case item := <-p.Results():
		require.Equal(t, uint64(blocks-1), item.SequenceNumber())
		require.NotNil(t, item.RawCbor())
		item.Release()
	case <-ctx.Done():
		t.Fatal("the latest result was not delivered to a late reader")
	}
}

// Undelivered results once kept their admission reservation, so with no
// reader the retained-CBOR budget filled and Submit waited for ever.
func TestBlockPipelineUnreadResultsDoNotHoldSubmitBudget(t *testing.T) {
	const blocks = 4
	shelley := getValidShelleyBlock(t)
	raw := shelley.Cbor
	p := NewBlockPipeline(append(
		validatedTestPipelineOptions(t, func(*BlockItem) error { return nil }),
		WithMaxRawCborBytes(2*uint64(len(raw))),
	)...)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, p.Start(ctx))
	t.Cleanup(func() { require.NoError(t, p.Stop(context.Background())) })

	for i := range blocks {
		require.NoError(t, p.Submit(
			ctx,
			shelley.BlockType,
			raw,
			createTestTip(uint64(i), uint64(i)),
		), "submit %d", i)
		require.NoError(t, p.Fence(ctx), "fence %d", i)
	}
	// Fence returns at commit, which can precede the handoff that returns
	// the admission reservation.
	require.Eventually(t, func() bool {
		p.rawBudgetMu.Lock()
		defer p.rawBudgetMu.Unlock()
		return p.rawBytesInUse == 0
	}, time.Second, time.Millisecond, "committed results return admission")
	require.Eventually(t, func() bool {
		return p.Stats().ResultsDropped == blocks-1
	}, time.Second, time.Millisecond)

	select {
	case item := <-p.Results():
		require.Equal(t, uint64(blocks-1), item.SequenceNumber())
		require.Equal(t, raw, item.RawCbor())
		item.Release()
	case <-ctx.Done():
		t.Fatal("the latest result was not delivered to a late reader")
	}
}

// A reader that never falls further behind than the result buffer receives
// every commit, in commit order, with its data intact.
func TestBlockPipelineResultsReaderThatKeepsUpSeesEveryResult(t *testing.T) {
	const blocks = 20
	raw := getValidBlockCbor(t)
	p := NewBlockPipeline(
		WithTrustedDecodeOnly(),
		WithPrefetchBufferSize(1),
		WithMaxRawCborBytes(2*uint64(len(raw))),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, p.Start(ctx))
	t.Cleanup(func() { require.NoError(t, p.Stop(context.Background())) })

	for i := range blocks {
		require.NoError(t, p.Submit(
			ctx,
			uint(ledger.BlockTypeConway),
			raw,
			createTestTip(uint64(i), uint64(i)),
		))
		require.NoError(t, p.Fence(ctx))
		select {
		case item := <-p.Results():
			require.Equal(t, uint64(i), item.SequenceNumber())
			require.Equal(t, raw, item.RawCbor())
			item.Release()
		case <-ctx.Done():
			t.Fatalf("result %d was not delivered", i)
		}
	}
}

// A reader that falls behind the result bound loses the oldest results, sees
// the gap in sequence numbers, and finds it counted in the stats.
func TestBlockPipelineResultsDropOldestWhenReaderFallsBehind(t *testing.T) {
	const (
		bound  = 3
		blocks = 8
	)
	shelley := getValidShelleyBlock(t)
	raw := shelley.Cbor
	p := NewBlockPipeline(append(
		validatedTestPipelineOptions(t, func(*BlockItem) error { return nil }),
		WithPrefetchBufferSize(bound),
	)...)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, p.Start(ctx))
	t.Cleanup(func() { require.NoError(t, p.Stop(context.Background())) })

	for i := range blocks {
		require.NoError(t, p.Submit(
			ctx,
			shelley.BlockType,
			raw,
			createTestTip(uint64(i), uint64(i)),
		))
	}
	require.NoError(t, p.Fence(ctx))
	require.Eventually(t, func() bool {
		return p.Stats().ResultsDropped == blocks-bound
	}, time.Second, time.Millisecond)

	for want := uint64(blocks - bound); want < blocks; want++ {
		select {
		case item := <-p.Results():
			require.Equal(t, want, item.SequenceNumber())
			item.Release()
		case <-ctx.Done():
			t.Fatalf("result %d was not delivered", want)
		}
	}
	require.Empty(t, p.DrainResults())
}
