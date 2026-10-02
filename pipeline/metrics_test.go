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
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stageRecorder lets one test body run against the decode, validate and apply
// stages by naming how to record into, and read back from, each of them.
type stageRecorder struct {
	name    string
	record  func(m *PipelineMetrics, d time.Duration, err error)
	timings func(s PipelineStats) StageTimings
}

var stageRecorders = []stageRecorder{
	{
		name: "decode",
		record: func(m *PipelineMetrics, d time.Duration, err error) {
			m.RecordDecode(d, err)
		},
		timings: func(s PipelineStats) StageTimings { return s.DecodeTimings },
	},
	{
		name: "validate",
		record: func(m *PipelineMetrics, d time.Duration, err error) {
			m.RecordValidate(d, err)
		},
		timings: func(s PipelineStats) StageTimings { return s.ValidateTimings },
	},
	{
		name: "apply",
		record: func(m *PipelineMetrics, d time.Duration, err error) {
			m.RecordApply(d, err)
		},
		timings: func(s PipelineStats) StageTimings { return s.ApplyTimings },
	},
}

// TestPipelineMetricsStageTimings checks that each Record* method keeps the
// durations it is given. Before the change they were dropped, so only the
// success and error counters could change.
func TestPipelineMetricsStageTimings(t *testing.T) {
	for _, stage := range stageRecorders {
		t.Run(stage.name, func(t *testing.T) {
			m := NewPipelineMetrics(0)
			assert.Equal(t, StageTimings{}, stage.timings(m.Stats()))

			for _, d := range []time.Duration{
				10 * time.Millisecond,
				30 * time.Millisecond,
				20 * time.Millisecond,
			} {
				stage.record(m, d, nil)
			}

			got := stage.timings(m.Stats())
			assert.Equal(t, uint64(3), got.Count)
			assert.Equal(t, 60*time.Millisecond, got.Total)
			assert.Equal(t, 30*time.Millisecond, got.Max)
			assert.Equal(t, 20*time.Millisecond, got.Mean())
		})
	}
}

// TestPipelineMetricsStageTimingsIgnoreFailures checks that a failed item does
// not enter the aggregate. A failed stage can report a partial or zero
// duration, which would skew Total, Max and Mean.
func TestPipelineMetricsStageTimingsIgnoreFailures(t *testing.T) {
	for _, stage := range stageRecorders {
		t.Run(stage.name, func(t *testing.T) {
			m := NewPipelineMetrics(0)
			stage.record(m, 5*time.Millisecond, nil)
			stage.record(m, time.Hour, errors.New("stage failed"))

			got := stage.timings(m.Stats())
			assert.Equal(t, uint64(1), got.Count)
			assert.Equal(t, 5*time.Millisecond, got.Total)
			assert.Equal(t, 5*time.Millisecond, got.Max)
		})
	}
}

// TestPipelineMetricsStageTimingsAreIndependent checks that each stage writes
// to its own aggregate, so a mix-up between the three Record* methods fails.
func TestPipelineMetricsStageTimingsAreIndependent(t *testing.T) {
	m := NewPipelineMetrics(0)
	m.RecordDecode(time.Millisecond, nil)
	m.RecordValidate(2*time.Millisecond, nil)
	m.RecordApply(3*time.Millisecond, nil)

	stats := m.Stats()
	assert.Equal(t, time.Millisecond, stats.DecodeTimings.Total)
	assert.Equal(t, 2*time.Millisecond, stats.ValidateTimings.Total)
	assert.Equal(t, 3*time.Millisecond, stats.ApplyTimings.Total)
}

// TestPipelineMetricsStageTimingsClampNegativeDurations checks that a negative
// duration, as a clock adjustment could produce, still counts as an item but
// cannot reduce Total or raise Max.
func TestPipelineMetricsStageTimingsClampNegativeDurations(t *testing.T) {
	m := NewPipelineMetrics(0)
	m.RecordDecode(-time.Second, nil)

	got := m.Stats().DecodeTimings
	assert.Equal(t, uint64(1), got.Count)
	assert.Zero(t, got.Total)
	assert.Zero(t, got.Max)
}

// TestStageTimingsMeanWithoutSamples checks that Mean does not divide by zero
// before anything has been recorded.
func TestStageTimingsMeanWithoutSamples(t *testing.T) {
	assert.Zero(t, StageTimings{}.Mean())
}

// TestPipelineMetricsKeepCountersAndLastBlockTime checks that retaining
// durations did not change the existing success and error counters, or when
// LastBlockTime is set (only on a successful apply).
func TestPipelineMetricsKeepCountersAndLastBlockTime(t *testing.T) {
	m := NewPipelineMetrics(0)
	m.RecordDecode(time.Millisecond, nil)
	m.RecordDecode(time.Millisecond, errors.New("decode"))
	m.RecordValidate(time.Millisecond, nil)
	m.RecordValidate(time.Millisecond, errors.New("validate"))
	m.RecordApply(time.Millisecond, errors.New("apply"))

	stats := m.Stats()
	assert.Equal(t, uint64(1), stats.BlocksDecoded)
	assert.Equal(t, uint64(1), stats.DecodeErrors)
	assert.Equal(t, uint64(1), stats.BlocksValidated)
	assert.Equal(t, uint64(1), stats.ValidationErrors)
	assert.Equal(t, uint64(1), stats.ApplyErrors)
	assert.True(t, stats.LastBlockTime.IsZero())

	m.RecordApply(time.Millisecond, nil)
	assert.False(t, m.Stats().LastBlockTime.IsZero())
}

// TestPipelineMetricsResetClearsStageTimings checks that Reset clears the
// timings along with the counters, so a reused PipelineMetrics starts clean.
func TestPipelineMetricsResetClearsStageTimings(t *testing.T) {
	m := NewPipelineMetrics(0)
	m.RecordDecode(time.Millisecond, nil)
	m.RecordValidate(time.Millisecond, nil)
	m.RecordApply(time.Millisecond, nil)

	m.Reset()

	stats := m.Stats()
	assert.Equal(t, StageTimings{}, stats.DecodeTimings)
	assert.Equal(t, StageTimings{}, stats.ValidateTimings)
	assert.Equal(t, StageTimings{}, stats.ApplyTimings)
}

// TestPipelineMetricsStageTimingsConcurrent records from many goroutines while
// others read Stats, as the stage workers do. Every duration from 1us to
// workers*perWorker us is recorded exactly once, so the expected Count, Total
// and Max are exact and a lost update shows up as a mismatch. Run it with
// -race to catch unsynchronized access.
func TestPipelineMetricsStageTimingsConcurrent(t *testing.T) {
	const (
		workers   = 16
		perWorker = 500
	)
	m := NewPipelineMetrics(0)

	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perWorker {
				d := time.Duration(w*perWorker+i+1) * time.Microsecond
				m.RecordDecode(d, nil)
				m.RecordValidate(d, nil)
				m.RecordApply(d, nil)
				_ = m.Stats()
			}
		}()
	}
	wg.Wait()

	n := workers * perWorker
	wantTotal := time.Duration(n*(n+1)/2) * time.Microsecond
	wantMax := time.Duration(n) * time.Microsecond
	stats := m.Stats()
	for _, got := range []StageTimings{
		stats.DecodeTimings,
		stats.ValidateTimings,
		stats.ApplyTimings,
	} {
		assert.Equal(t, uint64(n), got.Count)
		assert.Equal(t, wantTotal, got.Total)
		assert.Equal(t, wantMax, got.Max)
	}
}

// runPipeline submits count copies of rawCbor to a started pipeline, waits for
// every item to come out of Results, stops the pipeline and returns its final
// stats. Any stage error leaves the item in Results, so the loop does not hang
// on a failing stage.
func runPipeline(
	t *testing.T,
	blockType uint,
	rawCbor []byte,
	count int,
	opts ...PipelineOption,
) PipelineStats {
	t.Helper()
	p := NewBlockPipeline(opts...)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, p.Start(ctx))

	for i := range count {
		tip := createTestTip(uint64(1000+i), uint64(i))
		require.NoError(
			t,
			p.Submit(ctx, blockType, rawCbor, tip),
		)
	}
	for range count {
		select {
		case <-p.Results():
		case <-ctx.Done():
			t.Fatal("timed out waiting for pipeline results")
		}
	}
	require.NoError(t, p.Stop())
	return p.Stats()
}

func validatingPipelineOptions(
	eta0 string,
	apply ApplyFunc,
) []PipelineOption {
	return []PipelineOption{
		WithDecodeWorkers(2),
		WithValidateWorkers(2),
		WithSkipBodyHashValidation(true),
		WithEta0Provider(StaticEta0Provider(eta0)),
		WithSlotsPerKesPeriod(129600),
		WithVerifyConfig(common.VerifyConfig{
			SkipBodyHashValidation:    true,
			SkipTransactionValidation: true,
			SkipStakePoolValidation:   true,
		}),
		WithApplyFunc(apply),
	}
}

// shelleyBlockEta0 is the epoch nonce under which the Shelley test block's VRF
// proofs verify. The Conway test block has no known nonce, so this is the real
// block the repository can validate successfully end to end.
const shelleyBlockEta0 = "829749cb2701843214ae3aee67ae12ec9bdb3502e060ac0b75275d0f52af349c"

// TestBlockPipelineStageTimingsAllStages runs real blocks through decode, a
// real validate stage that succeeds, and apply, and checks that each stage
// records into its own timings. It is the only test that reaches the success
// path of the validate stage as the pipeline wires it.
func TestBlockPipelineStageTimingsAllStages(t *testing.T) {
	var shelley testdata.TestBlock
	for _, block := range testdata.GetTestBlocks() {
		if block.Name == "Shelley" {
			shelley = block
		}
	}
	require.NotEmpty(t, shelley.Cbor, "Shelley test block not found")

	const numBlocks = 4
	stats := runPipeline(
		t,
		uint(shelley.BlockType),
		shelley.Cbor,
		numBlocks,
		validatingPipelineOptions(
			shelleyBlockEta0,
			func(*BlockItem) error { return nil },
		)...,
	)

	require.Equal(t, uint64(numBlocks), stats.BlocksDecoded)
	require.Equal(t, uint64(numBlocks), stats.BlocksValidated,
		"validation must succeed for this test to exercise the success path")
	require.Equal(t, uint64(numBlocks), stats.BlocksApplied)
	for name, timings := range map[string]StageTimings{
		"decode":   stats.DecodeTimings,
		"validate": stats.ValidateTimings,
		"apply":    stats.ApplyTimings,
	} {
		assert.Equal(t, uint64(numBlocks), timings.Count, name)
		assert.Positive(t, timings.Total, name)
		assert.Positive(t, timings.Max, name)
		assert.GreaterOrEqual(t, timings.Total, timings.Max, name)
	}
}

// TestBlockPipelineStageTimingsExcludeValidationFailures makes every
// validation fail and checks that the failed validations count as errors and
// add nothing to ValidateTimings, while the stages that succeeded still do.
func TestBlockPipelineStageTimingsExcludeValidationFailures(t *testing.T) {
	const numBlocks = 3
	stats := runPipeline(
		t,
		uint(ledger.BlockTypeConway),
		getValidBlockCbor(t),
		numBlocks,
		validatingPipelineOptions(
			// An all-zero nonce does not match the block's VRF proof.
			"0000000000000000000000000000000000000000000000000000000000000000",
			func(*BlockItem) error { return nil },
		)...,
	)

	require.Equal(t, uint64(numBlocks), stats.ValidationErrors)
	assert.Equal(t, StageTimings{}, stats.ValidateTimings)
	assert.Equal(t, uint64(numBlocks), stats.DecodeTimings.Count)
	assert.Zero(t, stats.ApplyTimings.Count,
		"unvalidated items are not applied when validation is enabled")
}

// TestBlockPipelineStageTimingsExcludeDecodeFailures submits bytes that
// cannot decode and checks that they count as decode errors and add nothing
// to DecodeTimings.
func TestBlockPipelineStageTimingsExcludeDecodeFailures(t *testing.T) {
	const numBlocks = 3
	stats := runPipeline(
		t,
		uint(ledger.BlockTypeConway),
		[]byte{0xff, 0x00, 0x01},
		numBlocks,
		WithDecodeWorkers(2),
		WithValidateWorkers(0),
		WithSkipBodyHashValidation(true),
		WithApplyFunc(func(*BlockItem) error { return nil }),
	)

	require.Equal(t, uint64(numBlocks), stats.DecodeErrors)
	assert.Equal(t, StageTimings{}, stats.DecodeTimings)
	assert.Zero(t, stats.ApplyTimings.Count)
}

// TestBlockPipelineStageTimingsExcludeApplyFailures makes the apply function
// fail and checks that the failures count as apply errors and add nothing to
// ApplyTimings, exercising the direct RecordApply call in the apply stage.
func TestBlockPipelineStageTimingsExcludeApplyFailures(t *testing.T) {
	const numBlocks = 3
	stats := runPipeline(
		t,
		uint(ledger.BlockTypeConway),
		getValidBlockCbor(t),
		numBlocks,
		WithDecodeWorkers(2),
		WithValidateWorkers(0),
		WithSkipBodyHashValidation(true),
		WithApplyFunc(func(*BlockItem) error {
			return errors.New("apply failed")
		}),
	)

	require.Equal(t, uint64(numBlocks), stats.ApplyErrors)
	assert.Equal(t, StageTimings{}, stats.ApplyTimings)
	assert.Equal(t, uint64(numBlocks), stats.DecodeTimings.Count)
}

// TestStageMetricsRecordersFeedMatchingTimings runs a stub stage through a
// StageWorkerPool with each stage's real metrics recorder, as the pipeline
// does. The stub reports a known duration, and fails every third item with a
// very large one, so the exact Count, Total and Max pin down both what each
// recorder reads from the item and that failures are left out.
func TestStageMetricsRecordersFeedMatchingTimings(t *testing.T) {
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		getValidBlockCbor(t),
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	cases := []struct {
		name         string
		recorder     func(*PipelineMetrics) MetricsRecorder
		shouldRecord ShouldRecordMetrics
		setDuration  func(*BlockItem, time.Duration, error)
		timings      func(PipelineStats) StageTimings
		errors       func(PipelineStats) uint64
	}{
		{
			name:         "decode",
			recorder:     DecodeMetricsRecorder,
			shouldRecord: AlwaysRecordMetrics,
			setDuration: func(item *BlockItem, d time.Duration, err error) {
				if err != nil {
					item.SetDecodeError(err, d)
					return
				}
				item.SetBlock(block, d)
			},
			timings: func(s PipelineStats) StageTimings { return s.DecodeTimings },
			errors:  func(s PipelineStats) uint64 { return s.DecodeErrors },
		},
		{
			name:         "validate",
			recorder:     ValidateMetricsRecorder,
			shouldRecord: RecordIfDecoded,
			setDuration: func(item *BlockItem, d time.Duration, err error) {
				item.SetValidation(err == nil, "", err, d)
			},
			timings: func(s PipelineStats) StageTimings { return s.ValidateTimings },
			errors:  func(s PipelineStats) uint64 { return s.ValidationErrors },
		},
		{
			name:         "apply",
			recorder:     ApplyMetricsRecorder,
			shouldRecord: AlwaysRecordMetrics,
			setDuration: func(item *BlockItem, d time.Duration, err error) {
				item.SetApplied(err == nil, err, d)
			},
			timings: func(s PipelineStats) StageTimings { return s.ApplyTimings },
			errors:  func(s PipelineStats) uint64 { return s.ApplyErrors },
		},
	}

	const numItems = 9
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metrics := NewPipelineMetrics(0)
			input := make(chan *BlockItem, numItems)
			output := make(chan *BlockItem, numItems)

			// The stub gives item n (1-based) a duration of n milliseconds and
			// fails items 3, 6 and 9 with a duration that would dominate every
			// aggregate if it were recorded.
			stage := NewStageFunc(
				tc.name,
				func(_ context.Context, item *BlockItem) error {
					n := int(item.SequenceNumber())
					if n%3 == 0 {
						err := errors.New("stage failed")
						tc.setDuration(item, time.Hour, err)
						return err
					}
					tc.setDuration(item, time.Duration(n)*time.Millisecond, nil)
					return nil
				},
			)
			pool := NewStageWorkerPool(StageWorkerPoolConfig{
				Stage:         stage,
				NumWorkers:    3,
				Input:         input,
				Output:        output,
				RecordMetrics: tc.recorder(metrics),
				ShouldRecord:  tc.shouldRecord,
			})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			pool.Start(ctx)

			for n := 1; n <= numItems; n++ {
				item := NewBlockItem(
					uint(ledger.BlockTypeConway),
					nil,
					createTestTip(uint64(n), uint64(n)),
					uint64(n),
				)
				// The validate recorder only counts items that decoded.
				item.SetBlock(block, time.Millisecond)
				input <- item
			}
			close(input)
			pool.Stop()

			stats := metrics.Stats()
			assert.Equal(t, uint64(3), tc.errors(stats))
			got := tc.timings(stats)
			assert.Equal(t, uint64(6), got.Count)
			// Successful items are 1, 2, 4, 5, 7 and 8 ms.
			assert.Equal(t, 27*time.Millisecond, got.Total)
			assert.Equal(t, 8*time.Millisecond, got.Max)
		})
	}
}
