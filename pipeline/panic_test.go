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
	"sync/atomic"
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

func TestStageWorkerPoolContainsMetricsCallbackPanic(t *testing.T) {
	for _, test := range []struct {
		name         string
		shouldRecord ShouldRecordMetrics
		record       MetricsRecorder
	}{
		{
			name: "predicate",
			shouldRecord: func(item *BlockItem) bool {
				if item.SequenceNumber() == 0 {
					panic(stagePanicValue)
				}
				return true
			},
			record: func(*BlockItem, error) {},
		},
		{
			name: "recorder",
			record: func(item *BlockItem, _ error) {
				if item.SequenceNumber() == 0 {
					panic(stagePanicValue)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := make(chan *BlockItem, 2)
			output := make(chan *BlockItem, 2)
			errorChan := make(chan error, 2)
			input <- newPanicTestItem(0)
			input <- newPanicTestItem(1)
			close(input)
			pool := NewStageWorkerPool(StageWorkerPoolConfig{
				Stage: NewStageFunc(
					"decode",
					func(context.Context, *BlockItem) error { return nil },
				),
				NumWorkers:    1,
				Input:         input,
				Output:        output,
				Errors:        errorChan,
				RecordMetrics: test.record,
				ShouldRecord:  test.shouldRecord,
			})
			pool.Start(context.Background())
			pool.Stop()
			close(output)
			close(errorChan)

			items := make([]*BlockItem, 0, 2)
			for item := range output {
				items = append(items, item)
			}
			require.Len(t, items, 2)
			require.Equal(t, uint64(0), items[0].SequenceNumber())
			require.Equal(t, uint64(1), items[1].SequenceNumber())
			require.NoError(t, items[0].DecodeError())

			errs := make([]error, 0, 1)
			for err := range errorChan {
				errs = append(errs, err)
			}
			require.Len(t, errs, 1)
			requireContainedStagePanic(t, errs[0])
		})
	}
}

// TestApplyFuncPanicBecomesApplyError covers the consumer-callback boundary in
// the apply stage. The panic must become a fatal item error and prevent the
// buffered item behind it from applying.
func TestApplyFuncPanicBecomesApplyError(t *testing.T) {
	applied := []uint64{}
	stage := newTestApplyStage(func(item *BlockItem) error {
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
	require.ErrorIs(t, err, ErrPipelineItemRejected)
	require.Empty(t, processed)
	require.Empty(t, applied)
	require.Equal(t, 1, stage.PendingCount())
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
	input := make(chan *BlockItem, 2)
	output := make(chan *BlockItem, 2)
	errorChan := make(chan error, 2)
	processedChan := make(chan uint64, 2)
	fatalChan := make(chan struct{}, 1)
	runner := NewApplyStageRunner(nil, input, output, errorChan, 0)
	runner.SetProcessedFunc(func(sequence uint64) {
		processedChan <- sequence
	})
	runner.setFatalFunc(func() { fatalChan <- struct{}{} })

	runner.Start(context.Background())
	outOfOrder := newPanicTestItem(1)
	input <- outOfOrder
	input <- newPanicTestItem(0)

	select {
	case err := <-errorChan:
		require.ErrorIs(t, err, ErrStagePanic)
		require.ErrorContains(t, err, "apply stage")
	case <-time.After(5 * time.Second):
		t.Fatal("the apply runner did not report the contained panic")
	}
	runner.Stop()
	require.ErrorIs(t, outOfOrder.ApplyError(), ErrStagePanic)
	select {
	case completed := <-processedChan:
		t.Fatalf("stage panic advanced completion to %d across an ordering gap", completed)
	default:
	}
	select {
	case item := <-output:
		t.Fatalf("stage panic forwarded out-of-order item %d", item.SequenceNumber())
	default:
	}
	select {
	case <-fatalChan:
	default:
		t.Fatal("stage panic did not trigger fatal pipeline cancellation")
	}
}

func TestApplyStageRunnerCancelsBeforeReportingFatalError(t *testing.T) {
	input := make(chan *BlockItem, 2)
	output := make(chan *BlockItem, 1)
	errorChan := make(chan error, 1)
	blockedError := errors.New("error channel is full")
	errorChan <- blockedError
	fatalChan := make(chan struct{})
	runner := NewApplyStageRunner(
		newTestApplyStage(nil, 1), input, output, errorChan, 0,
	)
	runner.setFatalFunc(func() { close(fatalChan) })
	ctx, cancel := context.WithCancel(context.Background())
	runner.Start(ctx)
	t.Cleanup(func() {
		cancel()
		runner.Stop()
	})

	input <- newPanicTestItem(1)
	input <- newPanicTestItem(2)
	select {
	case <-fatalChan:
	case <-time.After(time.Second):
		t.Fatal("a full error channel prevented fatal pipeline cancellation")
	}

	runner.Stop()
	require.Same(t, blockedError, <-errorChan)
	select {
	case err := <-errorChan:
		require.ErrorIs(t, err, ErrPendingLimitExceeded)
	default:
	}
}

func TestApplyStageRunnerContainsProcessedCallbackPanic(t *testing.T) {
	input := make(chan *BlockItem, 1)
	output := make(chan *BlockItem, 1)
	errorChan := make(chan error, 1)
	fatalChan := make(chan struct{}, 1)
	runner := NewApplyStageRunner(
		newTestApplyStage(nil, 0), input, output, errorChan, 0,
	)
	runner.SetProcessedFunc(func(uint64) { panic(stagePanicValue) })
	runner.setFatalFunc(func() { fatalChan <- struct{}{} })
	runner.Start(context.Background())
	input <- newPanicTestItem(0)

	select {
	case err := <-errorChan:
		requireContainedStagePanic(t, err)
		require.ErrorContains(t, err, "apply processed callback")
	case <-time.After(5 * time.Second):
		t.Fatal("the processed callback panic was not reported")
	}
	select {
	case item := <-output:
		require.Equal(t, uint64(0), item.SequenceNumber())
		require.True(t, item.IsApplied())
	case <-time.After(5 * time.Second):
		t.Fatal("the completed item was not forwarded")
	}
	runner.Stop()
	select {
	case <-fatalChan:
	default:
		t.Fatal("processed callback panic did not trigger fatal cancellation")
	}
}

type panicAfterNameStage struct {
	processStarted atomic.Bool
}

func (s *panicAfterNameStage) Name() string {
	if s.processStarted.Load() {
		panic("stage name panic")
	}
	return "decode"
}

func (s *panicAfterNameStage) Process(context.Context, *BlockItem) error {
	s.processStarted.Store(true)
	panic(stagePanicValue)
}

// TestStageWorkerPoolRecoveryDoesNotCallStageName proves that recovery does
// not execute consumer code. Name succeeds before Process starts and panics
// afterward, so calling it from the deferred handler would replace the
// original panic and escape the worker goroutine.
func TestStageWorkerPoolRecoveryDoesNotCallStageName(t *testing.T) {
	item := newPanicTestItem(0)
	gotItems, gotErrors := runPanicTestPool(
		t, &panicAfterNameStage{}, []*BlockItem{item},
	)

	require.Len(t, gotErrors, 1)
	requireContainedStagePanic(t, gotErrors[0])
	require.NotContains(t, gotErrors[0].Error(), "stage name panic")
	require.Len(t, gotItems, 1)
	require.ErrorIs(t, gotItems[0].DecodeError(), ErrStagePanic)
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
		stage := newTestApplyStage(func(*BlockItem) error { return nil }, 0)
		processed, err := stage.ProcessWithStatus(
			context.Background(), newPanicTestItem(0),
		)
		require.NoError(t, err)
		require.Len(t, processed, 1)
		require.True(t, processed[0].IsApplied())
		require.NoError(t, processed[0].ApplyError())
	})

	t.Run("apply function error", func(t *testing.T) {
		stage := newTestApplyStage(func(*BlockItem) error {
			return errInjectedStage
		}, 0)
		processed, err := stage.ProcessWithStatus(
			context.Background(), newPanicTestItem(0),
		)
		require.ErrorIs(t, err, ErrPipelineItemRejected)
		require.Empty(t, processed)
	})

	t.Run("nil apply function", func(t *testing.T) {
		stage := newTestApplyStage(nil, 0)
		processed, err := stage.ProcessWithStatus(
			context.Background(), newPanicTestItem(0),
		)
		require.NoError(t, err)
		require.Len(t, processed, 1)
		require.True(t, processed[0].IsApplied())
		require.NoError(t, processed[0].ApplyError())
	})
}
