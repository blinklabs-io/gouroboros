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

// A caller that uses Results as its data path expects one result per
// submitted block. In lossless mode a reader slower than the pipeline still
// receives every committed block, in order, and nothing is dropped.
func TestLosslessResultsDeliversEveryCommittedBlock(t *testing.T) {
	const blocks = 20
	p := NewBlockPipeline(append(
		validatedTestPipelineOptions(t, func(*BlockItem) error { return nil }),
		WithPrefetchBufferSize(1),
		WithLosslessResults(),
	)...)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, p.Start(ctx))
	t.Cleanup(func() { require.NoError(t, p.Stop(context.Background())) })

	shelley := getValidShelleyBlock(t)
	submitErr := make(chan error, 1)
	go func() {
		for range blocks {
			if err := p.Submit(
				ctx, shelley.BlockType, shelley.Cbor, createTestTip(1000, 500),
			); err != nil {
				submitErr <- err
				return
			}
		}
		submitErr <- nil
	}()
	// Read nothing until every block is committed or committing has stopped
	// to wait for the reader, so a mode that drops results has dropped them.
	submitted := false
	select {
	case err := <-submitErr:
		require.NoError(t, err)
		submitted = true
		require.NoError(t, p.Fence(ctx))
	case <-time.After(500 * time.Millisecond):
	}
	for want := range uint64(blocks) {
		select {
		case item := <-p.Results():
			require.Equal(t, want, item.SequenceNumber(),
				"a committed block was not delivered")
			item.Release()
		case <-time.After(2 * time.Second):
			t.Fatalf("result %d never delivered", want)
		}
	}
	if !submitted {
		require.NoError(t, <-submitErr)
	}
	require.Zero(t, p.Stats().ResultsDropped)
}

// With no reader, lossless mode holds committing back once the queue is
// full, rather than dropping a result; reading releases it.
func TestLosslessResultsHoldsCommitsForTheReader(t *testing.T) {
	p := NewBlockPipeline(append(
		validatedTestPipelineOptions(t, func(*BlockItem) error { return nil }),
		WithPrefetchBufferSize(1),
		WithLosslessResults(),
	)...)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, p.Start(ctx))
	t.Cleanup(func() { require.NoError(t, p.Stop(context.Background())) })

	shelley := getValidShelleyBlock(t)
	submit := func() {
		require.NoError(t, p.Submit(
			ctx, shelley.BlockType, shelley.Cbor, createTestTip(1000, 500),
		))
	}
	// Fill the forwarder's queue and the runner's handoff, then one more.
	for range 4 {
		submit()
	}
	fenceCtx, fenceCancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer fenceCancel()
	require.Error(t, p.Fence(fenceCtx), "commit did not wait for the reader")
	require.Zero(t, p.Stats().ResultsDropped)

	go func() {
		for item := range p.Results() {
			item.Release()
		}
	}()
	require.NoError(t, p.Fence(ctx))
	require.Zero(t, p.Stats().ResultsDropped)
}

// Stop must not wait for a reader that never comes, even with the lossless
// queue full.
func TestLosslessResultsStopWithoutReader(t *testing.T) {
	p := NewBlockPipeline(append(
		validatedTestPipelineOptions(t, func(*BlockItem) error { return nil }),
		WithPrefetchBufferSize(1),
		WithLosslessResults(),
	)...)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, p.Start(ctx))

	shelley := getValidShelleyBlock(t)
	for range 4 {
		require.NoError(t, p.Submit(
			ctx, shelley.BlockType, shelley.Cbor, createTestTip(1000, 500),
		))
	}
	stopped := make(chan error, 1)
	go func() { stopped <- p.Stop(context.Background()) }()
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("Stop waited for a Results reader")
	}
}

// Header-only and trusted decode-only pipelines have no ApplyFunc, so Results
// is their only output and must not drop a committed block for a slow reader,
// even without WithLosslessResults.
func TestDecodeOnlyResultsAreLosslessByDefault(t *testing.T) {
	const blocks = 8
	shelley := getValidShelleyBlock(t)
	modes := map[string][]PipelineOption{
		"trusted decode-only": {WithTrustedDecodeOnly()},
		"header-only": {
			WithHeaderOnly(),
			WithValidateWorkers(1),
			WithBlockTypeResolver(resolveTestBlockType),
			WithEta0Provider(StaticEta0Provider(shelleyBlockEta0)),
			WithSlotsPerKesPeriod(129600),
		},
	}
	for name, opts := range modes {
		t.Run(name, func(t *testing.T) {
			p := NewBlockPipeline(append(opts, WithPrefetchBufferSize(1))...)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			require.NoError(t, p.Start(ctx))
			t.Cleanup(func() { require.NoError(t, p.Stop(context.Background())) })

			go func() {
				for range blocks {
					if p.Submit(
						ctx, shelley.BlockType, shelley.Cbor, createTestTip(1000, 500),
					) != nil {
						return
					}
				}
			}()
			// Let a dropping pipeline commit and drop before reading.
			time.Sleep(300 * time.Millisecond)
			for want := range uint64(blocks) {
				select {
				case item := <-p.Results():
					require.Equal(t, want, item.SequenceNumber(),
						"a committed block was not delivered")
					item.Release()
				case <-time.After(2 * time.Second):
					t.Fatalf("result %d never delivered", want)
				}
			}
			require.Zero(t, p.Stats().ResultsDropped)
		})
	}
}
