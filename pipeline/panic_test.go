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

	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

const stagePanicValue = "injected stage panic"

var errInjectedStage = errors.New("injected stage error")

func newPanicTestItem(seq uint64) *BlockItem {
	return NewBlockItem(0, []byte{0x80}, pcommon.Tip{}, seq)
}

// requireContainedStagePanic asserts the pipeline handed the caller a
// contained panic rather than losing the worker goroutine to it.
func requireContainedStagePanic(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStagePanic)
	require.ErrorContains(t, err, stagePanicValue)
	require.ErrorContains(t, err, "panic_test.go")
}

// runPanicTestPool starts a single-worker pool over the given stage and feeds
// it the supplied items, returning every item and error it produced.
func runPanicTestPool(
	t *testing.T,
	stage Stage,
	items []*BlockItem,
) ([]*BlockItem, []error) {
	t.Helper()

	input := make(chan *BlockItem, len(items))
	output := make(chan *BlockItem, len(items))
	errorChan := make(chan error, len(items))
	for _, item := range items {
		input <- item
	}
	close(input)

	pool := NewStageWorkerPool(StageWorkerPoolConfig{
		Stage:      stage,
		NumWorkers: 1,
		Input:      input,
		Output:     output,
		Errors:     errorChan,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool.Start(ctx)
	pool.Stop()

	close(output)
	close(errorChan)
	gotItems := make([]*BlockItem, 0, len(items))
	for item := range output {
		gotItems = append(gotItems, item)
	}
	gotErrors := make([]error, 0, len(items))
	for err := range errorChan {
		gotErrors = append(gotErrors, err)
	}
	return gotItems, gotErrors
}

// TestStageWorkerPoolContainsStagePanic covers the worker-pool boundary: a
// panicking stage must fail its own item and leave the worker able to take
// the next one, because a lost worker stalls the pipeline permanently.
func TestStageWorkerPoolContainsStagePanic(t *testing.T) {
	stage := NewStageFunc(
		"decode",
		func(_ context.Context, item *BlockItem) error {
			if item.SequenceNumber() == 0 {
				panic(stagePanicValue)
			}
			item.SetDecodeError(errInjectedStage, 0)
			return errInjectedStage
		},
	)
	items := []*BlockItem{newPanicTestItem(0), newPanicTestItem(1)}
	gotItems, gotErrors := runPanicTestPool(t, stage, items)

	require.Len(t, gotErrors, 2)
	requireContainedStagePanic(t, gotErrors[0])
	require.ErrorIs(
		t, gotErrors[1], errInjectedStage,
		"the worker did not survive the panic to process the next item",
	)

	require.Len(t, gotItems, 2)
	// The panicking stage never recorded an outcome, so without the
	// containment marking one the item would reach the apply stage looking
	// like a decoded block with a nil Block.
	require.ErrorIs(t, gotItems[0].DecodeError(), ErrStagePanic)
	require.False(t, gotItems[0].IsDecoded())
}

// TestApplyFuncPanicBecomesApplyError covers the consumer-callback boundary in
// the apply stage. The panic must become the item's apply error without
// disturbing ordered application of the items behind it.
func TestApplyFuncPanicBecomesApplyError(t *testing.T) {
	applied := []uint64{}
	stage := NewApplyStage(func(item *BlockItem) error {
		if item.SequenceNumber() == 0 {
			panic(stagePanicValue)
		}
		applied = append(applied, item.SequenceNumber())
		return nil
	}, 0)

	ctx := context.Background()
	// Submit out of order so the second item is buffered and only released
	// by the first, which is the path a panic could have unwound out of.
	buffered, err := stage.ProcessWithStatus(ctx, newPanicTestItem(1))
	require.NoError(t, err)
	require.Empty(t, buffered)

	processed, err := stage.ProcessWithStatus(ctx, newPanicTestItem(0))
	require.NoError(t, err)
	require.Len(
		t, processed, 2,
		"the buffered item was dropped when the apply function panicked",
	)

	requireContainedStagePanic(t, processed[0].ApplyError())
	require.False(t, processed[0].IsApplied())
	require.True(t, processed[1].IsApplied())
	require.Equal(t, []uint64{1}, applied)
	require.Zero(t, stage.PendingCount())
}

// TestApplyStageRunnerContainsStagePanic covers the runner backstop: the
// runner is the pipeline's only apply goroutine, so losing it would leave
// WaitForDrain waiting forever.
//
// The panic is injected through a nil stage, which NewApplyStageRunner accepts
// without complaint where NewStageWorkerPool rejects one outright. The
// consumer's own ApplyFunc is guarded closer in by callApplyFunc, so this
// backstop only ever sees a fault in the stage itself.
func TestApplyStageRunnerContainsStagePanic(t *testing.T) {
	input := make(chan *BlockItem, 1)
	output := make(chan *BlockItem, 1)
	errorChan := make(chan error, 1)
	runner := NewApplyStageRunner(nil, input, output, errorChan, 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	input <- newPanicTestItem(0)

	select {
	case err := <-errorChan:
		require.Error(t, err)
		require.ErrorIs(t, err, ErrStagePanic)
	case <-time.After(5 * time.Second):
		t.Fatal("the apply runner reported nothing and is no longer running")
	}
	cancel()
	runner.Stop()
}

// TestStagePipelineUnaffectedByContainment covers the cases that must not
// change: a stage that succeeds and a stage that fails normally.
func TestStagePipelineUnaffectedByContainment(t *testing.T) {
	t.Run("successful stage", func(t *testing.T) {
		stage := NewStageFunc(
			"decode",
			func(context.Context, *BlockItem) error { return nil },
		)
		gotItems, gotErrors := runPanicTestPool(
			t, stage, []*BlockItem{newPanicTestItem(0)},
		)
		require.Empty(t, gotErrors)
		require.Len(t, gotItems, 1)
		require.NoError(t, gotItems[0].DecodeError())
	})

	t.Run("stage error", func(t *testing.T) {
		stage := NewStageFunc(
			"decode",
			func(_ context.Context, item *BlockItem) error {
				item.SetDecodeError(errInjectedStage, time.Millisecond)
				return errInjectedStage
			},
		)
		gotItems, gotErrors := runPanicTestPool(
			t, stage, []*BlockItem{newPanicTestItem(0)},
		)
		require.Len(t, gotErrors, 1)
		require.ErrorIs(t, gotErrors[0], errInjectedStage)
		require.NotErrorIs(t, gotErrors[0], ErrStagePanic)
		require.Len(t, gotItems, 1)
		require.ErrorIs(t, gotItems[0].DecodeError(), errInjectedStage)
		require.Equal(t, time.Millisecond, gotItems[0].DecodeDuration())
	})

	t.Run("successful apply function", func(t *testing.T) {
		stage := NewApplyStage(func(*BlockItem) error { return nil }, 0)
		processed, err := stage.ProcessWithStatus(
			context.Background(), newPanicTestItem(0),
		)
		require.NoError(t, err)
		require.Len(t, processed, 1)
		require.True(t, processed[0].IsApplied())
		require.NoError(t, processed[0].ApplyError())
	})

	t.Run("apply function error", func(t *testing.T) {
		stage := NewApplyStage(func(*BlockItem) error {
			return errInjectedStage
		}, 0)
		processed, err := stage.ProcessWithStatus(
			context.Background(), newPanicTestItem(0),
		)
		require.NoError(t, err)
		require.Len(t, processed, 1)
		require.False(t, processed[0].IsApplied())
		require.ErrorIs(t, processed[0].ApplyError(), errInjectedStage)
		require.NotErrorIs(t, processed[0].ApplyError(), ErrStagePanic)
	})

	t.Run("nil apply function", func(t *testing.T) {
		stage := NewApplyStage(nil, 0)
		processed, err := stage.ProcessWithStatus(
			context.Background(), newPanicTestItem(0),
		)
		require.NoError(t, err)
		require.Len(t, processed, 1)
		require.True(t, processed[0].IsApplied())
		require.NoError(t, processed[0].ApplyError())
	})
}
