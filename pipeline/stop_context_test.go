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

func submitOneTestBlock(t *testing.T, p *BlockPipeline) {
	t.Helper()
	block := getValidShelleyBlock(t)
	require.NoError(t, p.Submit(
		context.Background(),
		block.BlockType,
		block.Cbor,
		createTestTip(1000, 500),
	))
}

func stopWithin(t *testing.T, p *BlockPipeline) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, p.Stop(ctx))
}

// An ApplyFunc that stops its own pipeline with its item's ApplyContext gets
// control back at once; with any other context, including another
// pipeline's, Stop waits for the runner that is running the call. A later
// Stop joins everything.
func TestBlockPipelineStopFromApplyFunc(t *testing.T) {
	ignore := goleak.IgnoreCurrent()
	other := NewBlockPipeline()
	var p *BlockPipeline
	otherErr := make(chan error, 1)
	selfErr := make(chan error, 1)
	p = NewBlockPipeline(validatedTestPipelineOptions(t,
		func(item *BlockItem) error {
			ctx, cancel := context.WithTimeout(other.applyCtx, 50*time.Millisecond)
			otherErr <- p.Stop(ctx)
			cancel()
			selfErr <- p.Stop(item.ApplyContext())
			return nil
		},
	)...)
	require.NoError(t, p.Start(context.Background()))
	submitOneTestBlock(t, p)

	select {
	case err := <-otherErr:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(5 * time.Second):
		t.Fatal("Stop with another pipeline's context did not honour its deadline")
	}
	select {
	case err := <-selfErr:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stop with the item's ApplyContext waited for its own runner")
	}
	stopWithin(t, p)
	require.NoError(t, goleak.Find(ignore))
}

// An external Stop waits for an ApplyFunc call in progress, or returns
// ctx.Err() when its context ends first; the pipeline still stops.
func TestBlockPipelineStopWaitsForApplyFuncOrContext(t *testing.T) {
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
	submitOneTestBlock(t, p)
	<-applyStarted

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	require.ErrorIs(t, p.Stop(ctx), context.DeadlineExceeded)
	cancel()
	select {
	case <-p.ctx.Done():
	default:
		t.Fatal("a Stop that gave up did not cancel the pipeline")
	}

	stopDone := make(chan error, 1)
	go func() { stopDone <- p.Stop(context.Background()) }()
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

// A drain from ApplyFunc with its item's ApplyContext never waits for its
// own runner: before Stop it reports that it cannot, during Stop it succeeds.
func TestBlockPipelineWaitForDrainFromApplyFunc(t *testing.T) {
	var p *BlockPipeline
	before := make(chan error, 1)
	during := make(chan error, 1)
	p = NewBlockPipeline(validatedTestPipelineOptions(t,
		func(item *BlockItem) error {
			before <- p.WaitForDrain(item.ApplyContext())
			_ = p.Stop(item.ApplyContext())
			during <- p.WaitForDrain(item.ApplyContext())
			return nil
		},
	)...)
	require.NoError(t, p.Start(context.Background()))
	submitOneTestBlock(t, p)
	for name, ch := range map[string]chan error{"before": before, "during": during} {
		select {
		case err := <-ch:
			if name == "before" {
				require.ErrorIs(t, err, ErrWaitForDrainInApplyFunc)
			} else {
				require.NoError(t, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("WaitForDrain %s Stop did not return", name)
		}
	}
	stopWithin(t, p)
}
