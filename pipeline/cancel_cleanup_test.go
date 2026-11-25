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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func rawBytesInUse(p *BlockPipeline) uint64 {
	p.rawBudgetMu.Lock()
	defer p.rawBudgetMu.Unlock()
	return p.rawBytesInUse
}

func activeItemCount(p *BlockPipeline) int {
	p.activeItemsMu.Lock()
	defer p.activeItemsMu.Unlock()
	return len(p.activeItems)
}

// Cancelling the context given to Start ends the pipeline as Stop would:
// every goroutine it started returns, Results is closed, and every retained
// CBOR reservation is returned, without anyone calling Stop. A drain then
// reports the cancellation, and Stop afterwards is a no-op.
func TestBlockPipelineParentCancelCleansUpWithoutStop(t *testing.T) {
	ignore := goleak.IgnoreCurrent()
	ctx, cancel := context.WithCancel(context.Background())
	p := NewBlockPipeline(
		WithTrustedDecodeOnly(),
		WithDecodeWorkers(1),
		WithPrefetchBufferSize(1),
	)
	require.NoError(t, p.Start(ctx))
	block := getValidShelleyBlock(t)
	for i := range 2 {
		require.NoError(t, p.Submit(
			context.Background(),
			block.BlockType,
			block.Cbor,
			createTestTip(uint64(1000+i), uint64(500+i)),
		))
	}
	require.NotZero(t, rawBytesInUse(p))
	cancel()

	require.NoError(t, goleak.Find(ignore), "goroutines left after the parent context ended")
	require.Zero(t, rawBytesInUse(p), "retained CBOR budget after the parent context ended")
	require.Zero(t, activeItemCount(p))
	for range p.Results() {
	}
	require.ErrorIs(t, p.WaitForDrain(context.Background()), ErrPipelineStopped)

	require.NoError(t, p.Stop(context.Background()))
	require.NoError(t, p.Stop(context.Background()))
	require.NoError(t, goleak.Find(ignore))
}

// Stop racing the parent context's end tears the pipeline down once and
// returns only after the cleanup goroutine has returned.
func TestBlockPipelineStopRacingParentCancel(t *testing.T) {
	ignore := goleak.IgnoreCurrent()
	for range 20 {
		ctx, cancel := context.WithCancel(context.Background())
		p := NewBlockPipeline(WithTrustedDecodeOnly(), WithDecodeWorkers(1))
		require.NoError(t, p.Start(ctx))
		block := getValidShelleyBlock(t)
		require.NoError(t, p.Submit(
			context.Background(),
			block.BlockType,
			block.Cbor,
			createTestTip(1000, 500),
		))
		done := make(chan error, 2)
		go func() { cancel(); done <- nil }()
		go func() { done <- p.Stop(context.Background()) }()
		for range 2 {
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("Stop racing a parent cancel did not return")
			}
		}
		require.Zero(t, rawBytesInUse(p))
	}
	require.NoError(t, goleak.Find(ignore))
}
