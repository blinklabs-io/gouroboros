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

//go:build profile

// Package bench provides profiling integration tests for gouroboros.
//
// These tests are designed to generate pprof profiles for analysis of CPU and
// memory usage in key validation paths. They are guarded by the "profile" build
// tag so they don't run in normal test suites.
//
// Usage:
//
//	# Generate CPU profile for block validation
//	go test -tags=profile -run=TestProfileBlockValidation \
//	    -cpuprofile=cpu_block.prof ./internal/bench/...
//
//	# Generate memory profile for CBOR decode
//	go test -tags=profile -run=TestProfileCBORDecode \
//	    -memprofile=mem_cbor.prof ./internal/bench/...
//
//	# Analyze profiles
//	go tool pprof -http=localhost:8080 cpu_block.prof
//	go tool pprof -http=localhost:8081 mem_cbor.prof
package bench

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/consensus"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/vrf"
)

const (
	// profileIterations is the number of iterations to run for each profile
	// test.
	// Higher values produce more accurate profiles but take longer to run.
	profileIterations = 1000
)

// TestProfileBlockValidation generates CPU/memory profiles for block
// validation.
// Run with -cpuprofile and/or -memprofile flags to capture profiles.
//
// Example:
//
//	go test -tags=profile -run=TestProfileBlockValidation \
//	    -cpuprofile=cpu_block.prof -memprofile=mem_block.prof \
//	    ./internal/bench/...
func TestProfileBlockValidation(t *testing.T) {
	blocks := testdata.GetTestBlocks()

	// Create a config that skips validation steps requiring ledger state
	// This focuses profiling on the core decode + hash validation path
	config := common.VerifyConfig{
		SkipTransactionValidation: true,
		SkipStakePoolValidation:   true,
		// Keep body hash validation enabled - this is a key validation path
		SkipBodyHashValidation: false,
	}

	// Pre-decode all blocks to ensure they're valid
	decodedBlocks := make([]ledger.Block, len(blocks))
	for i, block := range blocks {
		decoded, err := ledger.NewBlockFromCbor(block.BlockType, block.Cbor)
		if err != nil {
			t.Fatalf("failed to pre-decode %s block: %v", block.Name, err)
		}
		decodedBlocks[i] = decoded
	}

	// Dummy epoch nonce (hex encoded) for VRF verification
	eta0Hex := hex.EncodeToString(make([]byte, 32))
	slotsPerKes := uint64(129600) // Mainnet value

	t.Run("AllEras", func(t *testing.T) {
		for i := 0; i < profileIterations; i++ {
			for j, block := range decodedBlocks {
				// Skip Byron blocks - they use Ouroboros Classic, not Praos
				if blocks[j].BlockType == ledger.BlockTypeByronMain ||
					blocks[j].BlockType == ledger.BlockTypeByronEbb {
					continue
				}

				// VerifyBlock includes VRF/KES verification, body hash, etc.
				// With our config, transaction validation is skipped.
				// Errors are expected since we use a dummy epoch nonce for VRF.
				_, _, _, _, err := ledger.VerifyBlock(
					block,
					eta0Hex,
					slotsPerKes,
					config,
				)
				if err != nil && i == 0 {
					// Log once per block to avoid flooding output
					t.Logf(
						"VerifyBlock error for %s (expected with dummy VRF input): %v",
						blocks[j].Name,
						err,
					)
				}
			}
		}
	})

	// Also run per-era to isolate performance characteristics
	for idx, block := range blocks {
		// Skip Byron for Praos-era validation
		if block.BlockType == ledger.BlockTypeByronMain ||
			block.BlockType == ledger.BlockTypeByronEbb {
			continue
		}

		t.Run(block.Name, func(t *testing.T) {
			decoded := decodedBlocks[idx]
			for i := 0; i < profileIterations; i++ {
				// Errors are expected since we use a dummy epoch nonce for VRF.
				_, _, _, _, err := ledger.VerifyBlock(
					decoded,
					eta0Hex,
					slotsPerKes,
					config,
				)
				if err != nil && i == 0 {
					// Log once to avoid flooding output
					t.Logf(
						"VerifyBlock error (expected with dummy VRF input): %v",
						err,
					)
				}
			}
		})
	}
}

// TestProfileCBORDecode generates profiles for CBOR decoding of blocks.
// This isolates the CBOR deserialization overhead from validation logic.
//
// Example:
//
//	go test -tags=profile -run=TestProfileCBORDecode \
//	    -cpuprofile=cpu_cbor.prof -memprofile=mem_cbor.prof \
//	    ./internal/bench/...
func TestProfileCBORDecode(t *testing.T) {
	blocks := testdata.GetTestBlocks()

	t.Run("AllEras", func(t *testing.T) {
		for i := 0; i < profileIterations; i++ {
			for _, block := range blocks {
				_, err := ledger.NewBlockFromCbor(block.BlockType, block.Cbor)
				if err != nil {
					t.Fatalf("decode %s block error: %v", block.Name, err)
				}
			}
		}
	})

	// Per-era profiling
	for _, block := range blocks {
		t.Run(block.Name, func(t *testing.T) {
			for i := 0; i < profileIterations; i++ {
				_, err := ledger.NewBlockFromCbor(block.BlockType, block.Cbor)
				if err != nil {
					t.Fatalf("decode %s block error: %v", block.Name, err)
				}
			}
		})
	}
}

// TestProfileVRF generates profiles for VRF operations.
// This focuses on the cryptographic primitives used in leader election.
//
// Example:
//
//	go test -tags=profile -run=TestProfileVRF \
//	    -cpuprofile=cpu_vrf.prof -memprofile=mem_vrf.prof \
//	    ./internal/bench/...
func TestProfileVRF(t *testing.T) {
	// Generate VRF keys for testing (seed must be exactly 32 bytes)
	seed := []byte("test_seed_for_vrf_profiling!32!!")
	pk, sk, err := vrf.KeyGen(seed)
	if err != nil {
		t.Fatalf("VRF KeyGen failed: %v", err)
	}

	// Test message (simulates slot + nonce input)
	alpha := make([]byte, 64)
	for i := range alpha {
		alpha[i] = byte(i)
	}

	// Generate a proof for verification tests
	proof, hash, err := vrf.Prove(sk, alpha)
	if err != nil {
		t.Fatalf("VRF Prove failed: %v", err)
	}

	t.Run("Prove", func(t *testing.T) {
		for i := 0; i < profileIterations; i++ {
			_, _, err := vrf.Prove(sk, alpha)
			if err != nil {
				t.Fatalf("VRF Prove failed: %v", err)
			}
		}
	})

	t.Run("Verify", func(t *testing.T) {
		for i := 0; i < profileIterations; i++ {
			valid, err := vrf.Verify(pk, proof, hash, alpha)
			if err != nil {
				t.Fatalf("VRF Verify failed: %v", err)
			}
			if !valid {
				t.Fatal("VRF Verify returned false for valid proof")
			}
		}
	})

	t.Run("MkInputVrf", func(t *testing.T) {
		eta0 := make([]byte, 32)
		for i := range eta0 {
			eta0[i] = byte(i * 7)
		}
		slot := int64(50_000_000)

		for i := 0; i < profileIterations; i++ {
			_ = vrf.MkInputVrf(slot, eta0)
		}
	})

	t.Run("FullLeaderCheck", func(t *testing.T) {
		eta0 := make([]byte, 32)
		for i := range eta0 {
			eta0[i] = byte(i * 7)
		}

		for i := 0; i < profileIterations; i++ {
			slot := int64(50_000_000 + i)
			vrfInput := vrf.MkInputVrf(slot, eta0)

			proof, _, err := vrf.Prove(sk, vrfInput)
			if err != nil {
				t.Fatalf("VRF Prove failed: %v", err)
			}

			_, err = vrf.VerifyAndHash(pk, proof, vrfInput)
			if err != nil {
				t.Fatalf("VRF VerifyAndHash failed: %v", err)
			}
		}
	})
}

// TestProfileConsensus generates profiles for consensus operations.
// This focuses on leader election and threshold calculations.
//
// Example:
//
//	go test -tags=profile -run=TestProfileConsensus \
//	    -cpuprofile=cpu_consensus.prof -memprofile=mem_consensus.prof \
//	    ./internal/bench/...
func TestProfileConsensus(t *testing.T) {
	// Import consensus package dynamically to avoid circular dependencies
	// For now, we test the VRF components directly

	t.Run("LeaderValueComputation", func(t *testing.T) {
		// Generate VRF proof output (seed must be exactly 32 bytes)
		seed := []byte("consensus_profile_test_seed_32!!")
		_, sk, err := vrf.KeyGen(seed)
		if err != nil {
			t.Fatalf("VRF KeyGen failed: %v", err)
		}

		eta0 := make([]byte, 32)
		for i := range eta0 {
			eta0[i] = byte(i * 3)
		}

		for i := 0; i < profileIterations; i++ {
			slot := int64(100_000_000 + i)
			vrfInput := vrf.MkInputVrf(slot, eta0)

			_, hash, err := vrf.Prove(sk, vrfInput)
			if err != nil {
				t.Fatalf("VRF Prove failed: %v", err)
			}

			// Compute leader value from VRF output
			_ = consensus.VrfLeaderValue(hash)
		}
	})
}

// TestProfileBodyHash generates profiles specifically for body hash validation.
// This isolates the CBOR encoding and Blake2b hashing overhead by re-decoding
// blocks and accessing body hash related operations.
//
// Example:
//
//	go test -tags=profile -run=TestProfileBodyHash \
//	    -cpuprofile=cpu_bodyhash.prof -memprofile=mem_bodyhash.prof \
//	    ./internal/bench/...
func TestProfileBodyHash(t *testing.T) {
	blocks := testdata.GetTestBlocks()

	for _, block := range blocks {
		// Skip Byron - uses different body hash mechanism
		if block.BlockType == ledger.BlockTypeByronMain ||
			block.BlockType == ledger.BlockTypeByronEbb {
			continue
		}

		t.Run(block.Name, func(t *testing.T) {
			// Profile the decode path with body hash validation enabled
			// This exercises the full body hash validation code path
			for i := 0; i < profileIterations; i++ {
				// NewBlockFromCbor with default config validates body hash
				decoded, err := ledger.NewBlockFromCbor(
					block.BlockType,
					block.Cbor,
				)
				if err != nil {
					t.Fatalf("decode %s block error: %v", block.Name, err)
				}
				// Access body hash to ensure it's computed
				_ = decoded.BlockBodyHash()
			}
		})
	}
}

// TestProfileNativeScript generates profiles for native script evaluation.
// This tests the recursive script evaluation path using constructed scripts.
//
// Example:
//
//	go test -tags=profile -run=TestProfileNativeScript \
//	    -cpuprofile=cpu_script.prof -memprofile=mem_script.prof \
//	    ./internal/bench/...
func TestProfileNativeScript(t *testing.T) {
	// Key hashes map containing test keys
	keyHashes := map[common.Blake2b224]bool{
		testKeyHash1: true,
		testKeyHash2: true,
		testKeyHash3: true,
	}

	// Test slot and validity interval
	slot := uint64(100_000_000)
	validityStart := uint64(99_000_000)
	validityEnd := uint64(101_000_000)

	t.Run("ScriptType_Pubkey", func(t *testing.T) {
		script := createPubkeyScript(testKeyHash1)
		for i := 0; i < profileIterations; i++ {
			_ = script.Evaluate(slot, validityStart, validityEnd, keyHashes)
		}
	})

	t.Run("ScriptType_All", func(t *testing.T) {
		script := createAllScript(
			createPubkeyScript(testKeyHash1),
			createPubkeyScript(testKeyHash2),
			createPubkeyScript(testKeyHash3),
		)
		for i := 0; i < profileIterations; i++ {
			_ = script.Evaluate(slot, validityStart, validityEnd, keyHashes)
		}
	})

	t.Run("ScriptType_Any", func(t *testing.T) {
		script := createAnyScript(
			createPubkeyScript(testKeyHash1),
			createPubkeyScript(testKeyHash2),
			createPubkeyScript(testKeyHash3),
		)
		for i := 0; i < profileIterations; i++ {
			_ = script.Evaluate(slot, validityStart, validityEnd, keyHashes)
		}
	})

	t.Run("ScriptType_NofK", func(t *testing.T) {
		script := createNofKScript(2,
			createPubkeyScript(testKeyHash1),
			createPubkeyScript(testKeyHash2),
			createPubkeyScript(testKeyHash3),
		)
		for i := 0; i < profileIterations; i++ {
			_ = script.Evaluate(slot, validityStart, validityEnd, keyHashes)
		}
	})

	t.Run("ScriptType_Timelock", func(t *testing.T) {
		script := createAllScript(
			createInvalidBeforeScript(validityStart),
			createInvalidHereafterScript(validityEnd+1000),
			createPubkeyScript(testKeyHash1),
		)
		for i := 0; i < profileIterations; i++ {
			_ = script.Evaluate(slot, validityStart, validityEnd, keyHashes)
		}
	})

	t.Run("NestedDepth5", func(t *testing.T) {
		script := createNestedScript(5, "All")
		for i := 0; i < profileIterations; i++ {
			_ = script.Evaluate(slot, validityStart, validityEnd, keyHashes)
		}
	})
}
