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

package pipeline_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/pipeline"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// BenchmarkDecodeStage benchmarks CBOR decode throughput for different eras.
func BenchmarkDecodeStage(b *testing.B) {
	blocks := testdata.GetTestBlocks()
	stage := pipeline.NewDecodeStage(true) // Skip body hash validation for pure decode perf

	for _, block := range blocks {
		b.Run(block.Name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(block.Cbor)))

			ctx := context.Background()

			b.ResetTimer()
			for b.Loop() {
				item := pipeline.NewBlockItem(block.BlockType, block.Cbor, pcommon.Tip{}, 0)
				err := stage.Process(ctx, item)
				if err != nil {
					b.Fatalf("decode %s block error: %v", block.Name, err)
				}
			}
		})
	}

	// Also run a combined benchmark for all eras
	b.Run("AllEras", func(b *testing.B) {
		b.ReportAllocs()
		totalBytes := int64(0)
		for _, block := range blocks {
			totalBytes += int64(len(block.Cbor))
		}
		// b.SetBytes expects bytes per operation; use average block size
		b.SetBytes(totalBytes / int64(len(blocks)))

		ctx := context.Background()
		blockCount := len(blocks)

		b.ResetTimer()
		for i := 0; b.Loop(); i++ {
			block := blocks[i%blockCount]
			item := pipeline.NewBlockItem(block.BlockType, block.Cbor, pcommon.Tip{}, 0)
			err := stage.Process(ctx, item)
			if err != nil {
				b.Fatalf("decode %s block error: %v", block.Name, err)
			}
		}
	})
}

// BenchmarkStageWorkerPool benchmarks parallel decode with different worker counts.
func BenchmarkStageWorkerPool(b *testing.B) {
	blocks := testdata.GetTestBlocks()
	workerCounts := []int{1, 2, 4, 8}

	for _, numWorkers := range workerCounts {
		b.Run(numWorkerName(numWorkers), func(b *testing.B) {
			b.ReportAllocs()

			// Calculate average bytes per iteration
			totalBytes := int64(0)
			for _, block := range blocks {
				totalBytes += int64(len(block.Cbor))
			}
			// b.SetBytes expects bytes per operation; use average block size
			b.SetBytes(totalBytes / int64(len(blocks)))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Use fixed-size buffers to avoid OOM when b.N is large
			const bufferSize = 100
			input := make(chan *pipeline.BlockItem, bufferSize)
			output := make(chan *pipeline.BlockItem, bufferSize)
			errors := make(chan error, bufferSize)

			// Create worker pool using the generic StageWorkerPool
			stage := pipeline.NewDecodeStage(true)
			pool := pipeline.NewStageWorkerPool(pipeline.StageWorkerPoolConfig{
				Stage:      stage,
				NumWorkers: numWorkers,
				Input:      input,
				Output:     output,
				Errors:     errors,
			})
			pool.Start(ctx)
			defer pool.Stop()

			b.ResetTimer()

			// Feeder goroutine to avoid blocking on full channel
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < b.N; i++ {
					block := blocks[i%len(blocks)]
					item := pipeline.NewBlockItem(block.BlockType, block.Cbor, pcommon.Tip{}, uint64(i))
					select {
					case input <- item:
					case <-ctx.Done():
						return
					}
				}
				close(input)
			}()

			// Drain output
			received := 0
		receiveLoop:
			for received < b.N {
				select {
				case _, ok := <-output:
					if !ok {
						break receiveLoop
					}
					received++
				case err := <-errors:
					b.Fatalf("decode error: %v", err)
				case <-ctx.Done():
					break receiveLoop
				}
			}

			b.StopTimer()
			pool.Stop()
			wg.Wait()
		})
	}
}

func numWorkerName(n int) string {
	return fmt.Sprintf("Workers%d", n)
}

// BenchmarkBlockPipeline benchmarks the full BlockPipeline end-to-end throughput.
func BenchmarkBlockPipeline(b *testing.B) {
	blocks := testdata.GetTestBlocks()

	b.Run("EndToEnd", func(b *testing.B) {
		b.ReportAllocs()

		// Calculate average bytes per operation
		totalBytes := int64(0)
		for _, block := range blocks {
			totalBytes += int64(len(block.Cbor))
		}
		b.SetBytes(totalBytes / int64(len(blocks)))

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create full pipeline (decode + apply, no validation for benchmark)
		p := pipeline.NewBlockPipeline(
			pipeline.WithDecodeWorkers(4),
			pipeline.WithValidateWorkers(0), // Skip validation for throughput benchmark
			pipeline.WithSkipBodyHashValidation(true),
			pipeline.WithApplyFunc(func(item *pipeline.BlockItem) error {
				return nil // No-op apply for benchmark
			}),
		)
		if err := p.Start(ctx); err != nil {
			b.Fatalf("failed to start pipeline: %v", err)
		}
		defer func() { _ = p.Stop() }()

		b.ResetTimer()

		// Submit blocks
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < b.N; i++ {
				blk := blocks[i%len(blocks)]
				tip := pcommon.Tip{BlockNumber: uint64(i)}
				if err := p.Submit(ctx, blk.BlockType, blk.Cbor, tip); err != nil {
					return
				}
			}
		}()

		// Receive results
		received := 0
	receiveLoop:
		for received < b.N {
			select {
			case _, ok := <-p.Results():
				if !ok {
					break receiveLoop
				}
				received++
			case err := <-p.Errors():
				b.Fatalf("pipeline error: %v", err)
			case <-ctx.Done():
				break receiveLoop
			}
		}

		b.StopTimer()
		_ = p.Stop()
		wg.Wait()
	})
}

// BenchmarkBlockDecode provides a baseline comparison with direct ledger decode.
func BenchmarkBlockDecode(b *testing.B) {
	blocks := testdata.GetTestBlocks()

	for _, block := range blocks {
		b.Run(block.Name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(block.Cbor)))

			b.ResetTimer()
			for b.Loop() {
				_, err := ledger.NewBlockFromCbor(block.BlockType, block.Cbor)
				if err != nil {
					b.Fatalf("decode %s block error: %v", block.Name, err)
				}
			}
		})
	}
}
