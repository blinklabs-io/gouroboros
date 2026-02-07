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

// BenchmarkDecodeStageWorkerPool benchmarks parallel decode with different worker counts.
func BenchmarkDecodeStageWorkerPool(b *testing.B) {
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

			// Create channels
			input := make(chan *pipeline.BlockItem, b.N)
			output := make(chan *pipeline.BlockItem, b.N)
			errors := make(chan error, 100)

			// Create worker pool (don't start yet)
			stage := pipeline.NewDecodeStage(true)
			pool := pipeline.NewDecodeStageWorkerPool(stage, numWorkers, input, output, errors)

			// Pre-populate input channel before starting workers
			for i := 0; i < b.N; i++ {
				block := blocks[i%len(blocks)]
				item := pipeline.NewBlockItem(block.BlockType, block.Cbor, pcommon.Tip{}, uint64(i))
				input <- item
			}
			close(input)

			b.ResetTimer()
			// Start workers after timer reset to exclude setup time
			pool.Start(ctx)

			// Drain output
			received := 0
			for received < b.N {
				select {
				case <-output:
					received++
				case err := <-errors:
					b.Fatalf("decode error: %v", err)
				}
			}

			b.StopTimer()
			pool.Stop()
		})
	}
}

func numWorkerName(n int) string {
	switch n {
	case 1:
		return "Workers1"
	case 2:
		return "Workers2"
	case 4:
		return "Workers4"
	case 8:
		return "Workers8"
	default:
		return "Workers"
	}
}

// BenchmarkBlockPipeline benchmarks the full pipeline end-to-end throughput.
func BenchmarkBlockPipeline(b *testing.B) {
	blocks := testdata.GetTestBlocks()

	b.Run("EndToEnd", func(b *testing.B) {
		b.ReportAllocs()

		// Calculate average bytes per operation
		totalBytes := int64(0)
		for _, block := range blocks {
			totalBytes += int64(len(block.Cbor))
		}
		// b.SetBytes expects bytes per operation; use average block size
		b.SetBytes(totalBytes / int64(len(blocks)))

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create pipeline stages
		decodeStage := pipeline.NewDecodeStage(true)

		// For now, we just test the decode stage since the full pipeline
		// implementation is pending in PIPE-6
		input := make(chan *pipeline.BlockItem, 100)
		output := make(chan *pipeline.BlockItem, 100)
		errors := make(chan error, 100)

		pool := pipeline.NewDecodeStageWorkerPool(decodeStage, 4, input, output, errors)
		pool.Start(ctx)

		b.ResetTimer()

		// Submit blocks
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

		// Receive results
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

// BenchmarkPipelineScaling benchmarks how throughput scales with different worker counts.
func BenchmarkPipelineScaling(b *testing.B) {
	blocks := testdata.GetTestBlocks()
	workerCounts := []int{1, 2, 4, 8}

	// Pre-compute average bytes per operation
	totalBytes := int64(0)
	for _, block := range blocks {
		totalBytes += int64(len(block.Cbor))
	}
	avgBytes := totalBytes / int64(len(blocks))

	for _, numWorkers := range workerCounts {
		b.Run(numWorkerName(numWorkers), func(b *testing.B) {
			b.ReportAllocs()
			// b.SetBytes expects bytes per operation; use average block size
			b.SetBytes(avgBytes)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Create pipeline with specified worker count
			decodeStage := pipeline.NewDecodeStage(true)
			input := make(chan *pipeline.BlockItem, 100)
			output := make(chan *pipeline.BlockItem, 100)
			errors := make(chan error, 100)

			pool := pipeline.NewDecodeStageWorkerPool(
				decodeStage,
				numWorkers,
				input,
				output,
				errors,
			)
			pool.Start(ctx)

			b.ResetTimer()

			// Submit and receive in parallel
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < b.N; i++ {
					block := blocks[i%len(blocks)]
					item := pipeline.NewBlockItem(
						block.BlockType,
						block.Cbor,
						pcommon.Tip{},
						uint64(i),
					)
					select {
					case input <- item:
					case <-ctx.Done():
						return
					}
				}
				close(input)
			}()

			// Receive results
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
