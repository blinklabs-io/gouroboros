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
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A Stop after the parent context's end has already torn the pipeline down
// still records that Stop has begun: Start then refuses the pipeline, and
// WaitForDrain succeeds as documented for a pipeline being stopped.
func TestBlockPipelineStopAfterParentCancelTeardownIsRecorded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := NewBlockPipeline(WithTrustedDecodeOnly(), WithDecodeWorkers(1))
	require.NoError(t, p.Start(ctx))
	cancel()
	select {
	case <-p.teardownDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the parent cancel did not tear the pipeline down")
	}

	require.NoError(t, p.Stop(context.Background()))
	require.ErrorIs(t, p.Start(context.Background()), ErrPipelineStopped)
	require.NoError(t, p.WaitForDrain(context.Background()))
}

// A fatal processing error cancels the pipeline context but not the context
// given to Start, so the pipeline keeps the committed prefix for a Results
// reader that arrives after the failure, until Stop.
func TestBlockPipelineFatalErrorKeepsCommittedResultsUntilStop(t *testing.T) {
	p := NewBlockPipeline(validatedTestPipelineOptions(t,
		func(item *BlockItem) error {
			if item.SequenceNumber() == 1 {
				return errors.New("apply failed")
			}
			return nil
		},
	)...)
	require.NoError(t, p.Start(context.Background()))
	block := getValidShelleyBlock(t)
	for i := range 2 {
		require.NoError(t, p.Submit(
			context.Background(),
			block.BlockType,
			block.Cbor,
			createTestTip(uint64(1000+i), uint64(500+i)),
		))
	}
	select {
	case <-p.ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the failed apply did not cancel the pipeline")
	}
	// A teardown started by the fatal cancel would discard the committed
	// result; give one the time to start before reading.
	require.Never(t, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.teardownStarted
	}, 200*time.Millisecond, 5*time.Millisecond)

	select {
	case item, ok := <-p.Results():
		require.True(t, ok, "Results closed before Stop")
		require.Equal(t, uint64(0), item.SequenceNumber())
	case <-time.After(5 * time.Second):
		t.Fatal("the committed result was not delivered")
	}
	require.NoError(t, p.Stop(context.Background()))
}
