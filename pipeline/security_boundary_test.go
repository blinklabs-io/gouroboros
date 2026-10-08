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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func acceptSecurityTestChainContext(context.Context, *BlockItem) error {
	return nil
}

func TestBlockPipelineRequiresExplicitSourcePolicy(t *testing.T) {
	t.Run("default rejects missing validation", func(t *testing.T) {
		p := NewBlockPipeline()
		require.ErrorIs(t, p.Start(t.Context()), ErrBlockValidationRequired)
	})

	t.Run("normal requires chain context", func(t *testing.T) {
		p := NewBlockPipeline(
			WithValidateWorkers(1),
			WithEta0("00"),
		)
		require.ErrorIs(t, p.Start(t.Context()), ErrMissingChainContextValidator)
	})

	t.Run("normal rejects body hash bypass", func(t *testing.T) {
		p := NewBlockPipeline(
			WithValidateWorkers(1),
			WithEta0("00"),
			WithChainContextValidator(acceptSecurityTestChainContext),
			WithApplyFunc(func(*BlockItem) error { return nil }),
			WithSkipBodyHashValidation(true),
		)
		require.ErrorIs(t, p.Start(t.Context()), ErrBodyHashValidationRequired)
	})

	t.Run("normal requires authoritative apply", func(t *testing.T) {
		p := NewBlockPipeline(
			WithValidateWorkers(1),
			WithEta0("00"),
			WithChainContextValidator(acceptSecurityTestChainContext),
		)
		require.ErrorIs(t, p.Start(t.Context()), ErrMissingApplyFunc)
	})

	t.Run("normal requires KES period", func(t *testing.T) {
		p := NewBlockPipeline(
			WithValidateWorkers(1),
			WithEta0("00"),
			WithChainContextValidator(acceptSecurityTestChainContext),
			WithApplyFunc(func(*BlockItem) error { return nil }),
		)
		require.ErrorIs(t, p.Start(t.Context()), ErrInvalidSlotsPerKesPeriod)
	})

	t.Run("normal requires block type resolver", func(t *testing.T) {
		p := NewBlockPipeline(
			WithValidateWorkers(1),
			WithEta0("00"),
			WithSlotsPerKesPeriod(1),
			WithChainContextValidator(acceptSecurityTestChainContext),
			WithApplyFunc(func(*BlockItem) error { return nil }),
		)
		require.ErrorIs(t, p.Start(t.Context()), ErrMissingBlockTypeResolver)
	})

	t.Run("trusted mode is decode only", func(t *testing.T) {
		p := NewBlockPipeline(
			WithTrustedDecodeOnly(),
			WithApplyFunc(func(*BlockItem) error { return nil }),
		)
		require.ErrorIs(t, p.Start(t.Context()), ErrTrustedDecodeOnlyApply)
	})

	t.Run("trusted mode rejects chain context validation", func(t *testing.T) {
		p := NewBlockPipeline(
			WithTrustedDecodeOnly(),
			WithChainContextValidator(acceptSecurityTestChainContext),
		)
		err := p.Start(t.Context())
		require.ErrorIs(t, err, ErrTrustedDecodeOnlyChainContext)
		var configErr *TrustedDecodeOnlyChainContextError
		require.ErrorAs(t, err, &configErr)
	})

	t.Run("trusted mode rejects block type resolver", func(t *testing.T) {
		p := NewBlockPipeline(
			WithTrustedDecodeOnly(),
			WithBlockTypeResolver(resolveTestBlockType),
		)
		require.ErrorIs(
			t,
			p.Start(t.Context()),
			ErrTrustedDecodeOnlyBlockTypeResolver,
		)
	})
}

func TestBlockPipelineRejectsVerifyConfigValidationBypasses(t *testing.T) {
	tests := []struct {
		name   string
		flag   string
		config common.VerifyConfig
	}{
		{
			name:   "header validation",
			flag:   "SkipHeaderValidation",
			config: common.VerifyConfig{SkipHeaderValidation: true},
		},
		{
			name:   "body hash validation",
			flag:   "SkipBodyHashValidation",
			config: common.VerifyConfig{SkipBodyHashValidation: true},
		},
		{
			name:   "transaction validation",
			flag:   "SkipTransactionValidation",
			config: common.VerifyConfig{SkipTransactionValidation: true},
		},
		{
			name:   "stake pool validation",
			flag:   "SkipStakePoolValidation",
			config: common.VerifyConfig{SkipStakePoolValidation: true},
		},
		{
			name:   "block limits validation",
			flag:   "SkipBlockLimitsValidation",
			config: common.VerifyConfig{SkipBlockLimitsValidation: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := NewBlockPipeline(
				WithValidateWorkers(1),
				WithEta0("00"),
				WithChainContextValidator(acceptSecurityTestChainContext),
				WithApplyFunc(func(*BlockItem) error { return nil }),
				WithVerifyConfig(test.config),
			)
			err := p.Start(t.Context())
			require.ErrorIs(t, err, ErrValidationBypassConfigured)
			var bypassErr *ValidationBypassError
			require.ErrorAs(t, err, &bypassErr)
			require.Equal(t, test.flag, bypassErr.Flag)
		})
	}
}

func TestRealAdjacentEraHeadersRequireEnvelopeType(t *testing.T) {
	wanted := map[string]bool{"Allegra": true, "Mary": true, "Alonzo": true}
	for _, fixture := range testdata.GetTestBlocks() {
		if !wanted[fixture.Name] {
			continue
		}
		block, err := ledger.NewBlockFromCbor(
			fixture.BlockType,
			fixture.Cbor,
			common.VerifyConfig{SkipBodyHashValidation: true},
		)
		require.NoError(t, err, fixture.Name)

		blockType, err := ledger.DetermineBlockType(block.Header().Cbor())
		require.Zero(t, blockType, fixture.Name)
		require.ErrorIs(t, err, ledger.ErrAmbiguousBlockType, fixture.Name)
	}
}

func TestBlockTypeMismatchRejectedBeforeTypedDecodeAndValidation(t *testing.T) {
	var allegra testdata.TestBlock
	for _, fixture := range testdata.GetTestBlocks() {
		if fixture.Name == "Allegra" {
			allegra = fixture
			break
		}
	}
	require.NotEmpty(t, allegra.Cbor)
	var blockFields []cbor.RawMessage
	_, err := cbor.Decode(allegra.Cbor, &blockFields)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(blockFields), 2)
	blockFields[1] = cbor.RawMessage{0x00}
	malformedBodyBlock, err := cbor.Encode(blockFields)
	require.NoError(t, err)
	headerCbor, err := ledger.ExtractBlockHeaderCbor(malformedBodyBlock)
	require.NoError(t, err)
	require.Equal(t, []byte(blockFields[0]), headerCbor)
	_, err = ledger.NewBlockFromCbor(
		ledger.BlockTypeMary,
		malformedBodyBlock,
	)
	require.Error(t, err, "the claimed typed decoder must reject the fixture")

	resolver := func(context.Context, []byte) (uint, error) {
		return uint(ledger.BlockTypeAllegra), nil
	}
	item := NewBlockItem(
		uint(ledger.BlockTypeMary),
		malformedBodyBlock,
		createTestTip(1, 1),
		0,
	)
	decodeStage := NewDecodeStage(false)
	decodeStage.SetBlockTypeResolver(resolver)
	err = decodeStage.Process(t.Context(), item)
	require.ErrorIs(t, err, ErrBlockTypeMismatch)
	require.Nil(t, item.Block(), "typed decoder must not run after a mismatch")

	eta0Calls := 0
	chainContextCalls := 0
	applyCalls := 0
	p := NewBlockPipeline(
		WithDecodeWorkers(1),
		WithValidateWorkers(1),
		WithBlockTypeResolver(resolver),
		WithEta0Provider(func(uint64) (string, error) {
			eta0Calls++
			return shelleyBlockEta0, nil
		}),
		WithSlotsPerKesPeriod(129600),
		WithVerifyConfig(validatingTestVerifyConfig(t)),
		WithChainContextValidator(func(context.Context, *BlockItem) error {
			chainContextCalls++
			return nil
		}),
		WithApplyFunc(func(*BlockItem) error {
			applyCalls++
			return nil
		}),
	)
	require.NoError(t, p.Start(t.Context()))
	require.NoError(t, p.Submit(
		t.Context(),
		uint(ledger.BlockTypeMary),
		malformedBodyBlock,
		createTestTip(1, 1),
	))
	err = p.Fence(t.Context())
	require.ErrorIs(t, err, ErrBlockTypeMismatch)
	require.NoError(t, p.Stop())
	require.Zero(t, eta0Calls)
	require.Zero(t, chainContextCalls)
	require.Zero(t, applyCalls)
}

func TestTrustedDecodeOnlyPreservesEnvelopeWithoutApplying(t *testing.T) {
	p := NewBlockPipeline(WithTrustedDecodeOnly())
	require.NoError(t, p.Start(t.Context()))
	t.Cleanup(func() { require.NoError(t, p.Stop()) })

	const blockType = uint(ledger.BlockTypeConway)
	require.NoError(t, p.Submit(
		t.Context(),
		blockType,
		getValidBlockCbor(t),
		createTestTip(1000, 500),
	))

	select {
	case item := <-p.Results():
		require.True(t, item.IsDecoded())
		require.Equal(t, blockType, item.BlockType())
		require.False(t, item.IsApplied())
	case <-time.After(time.Second):
		t.Fatal("trusted decoded item was not returned")
	}
}

func TestApplyStageValidatesContextImmediatelyBeforeCommit(t *testing.T) {
	var calls []string
	stage := NewApplyStage(func(*BlockItem) error {
		calls = append(calls, "apply")
		return nil
	}, 0)
	stage.SetRequireValidation(true)
	stage.SetChainContextValidator(func(_ context.Context, item *BlockItem) error {
		require.Equal(t, uint(ledger.BlockTypeMary), item.BlockType())
		calls = append(calls, "validate")
		return nil
	})
	item := NewBlockItem(uint(ledger.BlockTypeMary), nil, createTestTip(1, 1), 0)
	item.SetValidation(true, "", nil, 0)

	require.NoError(t, stage.Process(t.Context(), item))
	require.Equal(t, []string{"validate", "apply"}, calls)
	require.True(t, item.IsApplied())
}

func TestApplyStagePendingCountIncludesChainContextValidation(t *testing.T) {
	validationStarted := make(chan struct{})
	releaseValidation := make(chan struct{})
	stage := NewApplyStage(func(*BlockItem) error { return nil }, 0)
	stage.SetRequireValidation(true)
	stage.SetChainContextValidator(func(context.Context, *BlockItem) error {
		close(validationStarted)
		<-releaseValidation
		return nil
	})
	item := NewBlockItem(0, nil, createTestTip(1, 1), 0)
	item.SetValidation(true, "", nil, 0)
	done := make(chan error, 1)
	go func() { done <- stage.Process(t.Context(), item) }()
	<-validationStarted
	require.Equal(t, 1, stage.PendingCount())
	close(releaseValidation)
	require.NoError(t, <-done)
	require.Zero(t, stage.PendingCount())
}

func TestStoppingPipelinePreservesFatalCause(t *testing.T) {
	p := NewBlockPipeline(WithTrustedDecodeOnly())
	require.NoError(t, p.Start(t.Context()))
	fatalErr := errors.New("fatal validation failure")
	p.setFatalError(fatalErr)
	p.stopping.Store(true)

	require.ErrorIs(t, p.Submit(t.Context(), 0, nil, createTestTip(0, 0)), fatalErr)
	require.ErrorIs(t, p.Fence(t.Context()), fatalErr)

	p.stopping.Store(false)
	require.NoError(t, p.Stop())
}

func TestApplyFailureCancelsWithoutAdvancingAuthoritativeContext(t *testing.T) {
	input := make(chan *BlockItem, 2)
	output := make(chan *BlockItem, 2)
	errorChan := make(chan error, 1)
	errorChan <- errors.New("occupied")
	canceled := make(chan struct{})

	contextPosition := uint64(0)
	validationCalls := 0
	stage := NewApplyStage(func(item *BlockItem) error {
		if item.SequenceNumber() == 0 {
			return errors.New("commit failed")
		}
		contextPosition++
		return nil
	}, 0)
	stage.SetRequireValidation(true)
	stage.SetChainContextValidator(func(_ context.Context, item *BlockItem) error {
		validationCalls++
		if item.SequenceNumber() != contextPosition {
			return errors.New("context advanced")
		}
		return nil
	})
	runner := NewApplyStageRunner(stage, input, output, errorChan, 0)
	runner.setFatalFunc(func() { close(canceled) })
	runner.Start(t.Context())

	for seq := range uint64(2) {
		item := NewBlockItem(0, nil, createTestTip(seq, seq), seq)
		item.SetValidation(true, "", nil, 0)
		input <- item
	}
	close(input)

	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("fatal apply failure did not cancel the pipeline")
	}
	runner.Stop()
	require.Equal(t, uint64(0), contextPosition)
	require.Equal(t, 1, validationCalls)
	require.Empty(t, output)
	require.Len(t, errorChan, 1, "fatal reporting must not block on a full channel")
}

func TestFatalCauseSurvivesBeforeFenceAcquiresSubmitGate(t *testing.T) {
	rejected := errors.New("wrong active era")
	p := NewBlockPipeline(validatedTestPipelineOptions(t,
		func(*BlockItem) error { return nil },
	)...)
	p.config.ChainContextValidator = func(context.Context, *BlockItem) error {
		return rejected
	}
	require.NoError(t, p.Start(t.Context()))
	t.Cleanup(func() { require.NoError(t, p.Stop()) })

	shelley := getValidShelleyBlock(t)
	require.NoError(t, p.Submit(
		t.Context(),
		shelley.BlockType,
		shelley.Cbor,
		createTestTip(1000, 500),
	))
	select {
	case <-p.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("ordered rejection did not cancel the pipeline")
	}

	err := p.Fence(t.Context())
	require.ErrorIs(t, err, ErrChainContextValidation)
	require.ErrorIs(t, err, rejected)
}

func TestProcessedCallbackPanicCancelsWithFullErrorChannel(t *testing.T) {
	occupied := errors.New("occupied")
	p := NewBlockPipeline(
		WithTrustedDecodeOnly(),
		WithPrefetchBufferSize(1),
	)
	p.testProcessedFunc = func(uint64) { panic("processed callback") }
	require.NoError(t, p.Start(t.Context()))
	t.Cleanup(func() { require.NoError(t, p.Stop()) })
	p.errorsChan <- occupied

	require.NoError(t, p.Submit(
		t.Context(),
		uint(ledger.BlockTypeConway),
		getValidBlockCbor(t),
		createTestTip(1000, 500),
	))
	select {
	case <-p.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("a full error channel prevented callback panic cancellation")
	}

	err := p.Fence(t.Context())
	require.ErrorIs(t, err, ErrStagePanic)
	require.ErrorContains(t, err, "apply processed callback")
	require.Same(t, occupied, <-p.errorsChan)
}

func newValidatedSecurityTestItem(sequence uint64) *BlockItem {
	item := NewBlockItem(0, nil, createTestTip(sequence, sequence), sequence)
	item.SetValidation(true, "", nil, 0)
	return item
}

func TestFatalRejectionCancelsBeforeFullResultDelivery(t *testing.T) {
	const rejectedSequence = uint64(4)
	input := make(chan *BlockItem, rejectedSequence+1)
	output := make(chan *BlockItem, 2)
	errorChan := make(chan error, 1)
	fatalCause := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())

	stage := NewApplyStage(func(item *BlockItem) error {
		if item.SequenceNumber() == rejectedSequence {
			return errors.New("buffered commit failed")
		}
		return nil
	}, 0)
	stage.SetRequireValidation(true)
	stage.SetChainContextValidator(acceptSecurityTestChainContext)
	runner := NewApplyStageRunner(stage, input, output, errorChan, 0)
	runner.setFatalErrorFunc(func(err error) { fatalCause <- err })
	runner.setFatalFunc(cancel)
	runner.Start(ctx)
	defer func() {
		cancel()
		runner.Stop()
	}()

	for sequence := uint64(1); sequence <= rejectedSequence; sequence++ {
		input <- newValidatedSecurityTestItem(sequence)
	}
	input <- newValidatedSecurityTestItem(0)
	close(input)

	select {
	case err := <-fatalCause:
		require.ErrorIs(t, err, ErrBlockApply)
	case <-time.After(time.Second):
		t.Fatal("fatal cause waited for the full results channel")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("fatal cancellation was not published before results drained")
	}

	for want := uint64(0); want < rejectedSequence; want++ {
		select {
		case item := <-output:
			require.Equal(t, want, item.SequenceNumber())
		case <-time.After(time.Second):
			t.Fatalf("successful prefix item %d was not forwarded", want)
		}
	}
	runner.Stop()
	select {
	case item := <-output:
		t.Fatalf("unexpected result for sequence %d", item.SequenceNumber())
	default:
	}
}

func TestBlockPipelineStopDrainsFatalCommittedPrefix(t *testing.T) {
	const rejectedSequence = uint64(4)
	p := NewBlockPipeline()
	p.ctx, p.cancel = context.WithCancel(context.Background())
	p.submitChan = make(chan *BlockItem, 1)
	p.decodedChan = make(chan *BlockItem, rejectedSequence+1)
	p.resultsChan = make(chan *BlockItem, 2)
	p.errorsChan = make(chan error, 1)
	p.decodePool = &StageWorkerPool{}
	p.applyStage = NewApplyStage(func(item *BlockItem) error {
		if item.SequenceNumber() == rejectedSequence {
			return errors.New("buffered commit failed")
		}
		return nil
	}, 0)
	p.applyStage.SetRequireValidation(true)
	p.applyStage.SetChainContextValidator(acceptSecurityTestChainContext)
	p.applyRunner = NewApplyStageRunner(
		p.applyStage,
		p.decodedChan,
		p.resultsChan,
		p.errorsChan,
		0,
	)
	p.applyRunner.setFatalErrorFunc(p.setFatalError)
	p.applyRunner.setFatalFunc(p.cancel)
	p.applyRunner.Start(p.ctx)
	p.started.Store(true)

	for sequence := uint64(1); sequence <= rejectedSequence; sequence++ {
		p.decodedChan <- newValidatedSecurityTestItem(sequence)
	}
	p.decodedChan <- newValidatedSecurityTestItem(0)
	select {
	case <-p.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("fatal rejection did not cancel the pipeline")
	}

	stopped := make(chan error, 1)
	go func() { stopped <- p.Stop() }()
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Stop remained blocked on undelivered committed results")
	}
}

func TestBufferedRejectionRecordsExactApplyMetrics(t *testing.T) {
	tests := []struct {
		name      string
		validator ChainContextValidator
		apply     ApplyFunc
	}{
		{
			name:      "chain context",
			validator: func(context.Context, *BlockItem) error { return errors.New("wrong chain context") },
			apply:     func(*BlockItem) error { return nil },
		},
		{
			name:      "apply",
			validator: acceptSecurityTestChainContext,
			apply:     func(*BlockItem) error { return errors.New("commit failed") },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := make(chan *BlockItem, 2)
			output := make(chan *BlockItem, 2)
			errorChan := make(chan error, 1)
			fatalCause := make(chan error, 1)
			ctx, cancel := context.WithCancel(context.Background())

			stage := NewApplyStage(func(item *BlockItem) error {
				if item.SequenceNumber() == 1 {
					return test.apply(item)
				}
				return nil
			}, 0)
			stage.SetRequireValidation(true)
			stage.SetChainContextValidator(func(ctx context.Context, item *BlockItem) error {
				if item.SequenceNumber() == 1 {
					return test.validator(ctx, item)
				}
				return nil
			})
			metrics := NewPipelineMetrics(1)
			runner := NewApplyStageRunner(stage, input, output, errorChan, 0)
			runner.SetMetrics(metrics)
			runner.setFatalErrorFunc(func(err error) { fatalCause <- err })
			runner.setFatalFunc(cancel)
			runner.Start(ctx)
			defer func() {
				cancel()
				runner.Stop()
			}()

			input <- newValidatedSecurityTestItem(1)
			input <- newValidatedSecurityTestItem(0)
			close(input)

			select {
			case <-fatalCause:
			case <-time.After(time.Second):
				t.Fatal("buffered rejection did not cancel the runner")
			}
			runner.Stop()

			stats := metrics.Stats()
			require.Equal(t, uint64(1), stats.BlocksApplied)
			require.Equal(t, uint64(1), stats.ApplyErrors)
			select {
			case item := <-output:
				require.Equal(t, uint64(0), item.SequenceNumber())
			case <-time.After(time.Second):
				t.Fatal("successful prefix was not forwarded")
			}
			select {
			case item := <-output:
				t.Fatalf("rejected sequence %d was forwarded", item.SequenceNumber())
			default:
			}
		})
	}
}
