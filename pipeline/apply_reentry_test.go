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
)

// An ApplyFunc that drains during Stop must not wait for the apply runner,
// which is the goroutine running it: that wait would never end, and Stop,
// which waits for the runner, would never return either.
func TestBlockPipelineWaitForDrainFromApplyFuncDuringStop(t *testing.T) {
	var p *BlockPipeline
	entered := make(chan struct{})
	drained := make(chan error, 1)
	p = NewBlockPipeline(validatedTestPipelineOptions(t,
		func(item *BlockItem) error {
			close(entered)
			<-p.ctx.Done()
			drained <- p.WaitForDrain(item.ApplyContext())
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
	<-entered

	stopDone := make(chan error, 1)
	go func() { stopDone <- p.Stop(context.Background()) }()
	select {
	case err := <-drained:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForDrain from ApplyFunc during Stop did not return")
	}
	select {
	case err := <-stopDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return")
	}
}

// Only this pipeline's ApplyContext skips the wait: another pipeline's, or a
// plain context, still waits for the apply runner during Stop.
func TestBlockPipelineWaitForDrainOtherContextWaitsDuringStop(t *testing.T) {
	other := NewBlockPipeline()
	foreign := NewBlockItem(0, nil, createTestTip(1, 1), 0)
	foreign.applyCtx = other.applyCtx
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
	<-applyStarted
	stopDone := make(chan error, 1)
	go func() { stopDone <- p.Stop(context.Background()) }()
	<-p.ctx.Done()

	for _, ctx := range []context.Context{
		foreign.ApplyContext(),
		context.Background(),
	} {
		waitCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		err := p.WaitForDrain(waitCtx)
		cancel()
		require.ErrorIs(t, err, context.DeadlineExceeded)
	}

	close(releaseApply)
	select {
	case err := <-stopDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return after ApplyFunc returned")
	}
}
