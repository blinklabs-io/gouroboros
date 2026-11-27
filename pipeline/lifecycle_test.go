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

// After the parent context is cancelled, Stop returns with every
// reservation returned and no goroutine left behind.
func TestBlockPipelineStopAfterParentCancelReleasesEverything(t *testing.T) {
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
	cancel()

	require.NoError(t, p.Stop(context.Background()))

	p.rawBudgetMu.Lock()
	inUse := p.rawBytesInUse
	p.rawBudgetMu.Unlock()
	require.Zero(t, inUse, "retained CBOR budget after Stop")
	p.activeItemsMu.Lock()
	active := len(p.activeItems)
	p.activeItemsMu.Unlock()
	require.Zero(t, active, "items still holding a reservation after Stop")
	require.NoError(t, goleak.Find(ignore))
}

// Cancellation cannot interrupt a call of ApplyFunc in progress. Stop must
// wait for it to return, and then for every goroutine the pipeline started.
func TestBlockPipelineStopWaitsForApplyFuncInProgress(t *testing.T) {
	ignore := goleak.IgnoreCurrent()
	applyStarted := make(chan struct{})
	releaseApply := make(chan struct{})
	p := NewBlockPipeline(validatedTestPipelineOptions(t,
		func(*BlockItem) error {
			close(applyStarted)
			<-releaseApply
			return nil
		},
	)...)
	require.NoError(t, p.Start(context.Background()))
	block := getValidShelleyBlock(t)
	require.NoError(t, p.Submit(
		context.Background(),
		block.BlockType,
		block.Cbor,
		createTestTip(1000, 500),
	))
	select {
	case <-applyStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("ApplyFunc did not start")
	}

	stopDone := make(chan error, 1)
	go func() { stopDone <- p.Stop(context.Background()) }()
	// Stop cancels the pipeline context before it waits for the runner.
	select {
	case <-p.ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not cancel the pipeline")
	}
	select {
	case err := <-stopDone:
		t.Fatalf("Stop returned %v while ApplyFunc was still running", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseApply)
	select {
	case err := <-stopDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return after ApplyFunc returned")
	}
	require.NoError(t, goleak.Find(ignore))
}
