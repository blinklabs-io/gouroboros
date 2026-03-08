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
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/gouroboros/vrf"
	"github.com/stretchr/testify/require"
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
		require.NoError(t, err, "failed to pre-decode %s block", block.Name)
		decodedBlocks[i] = decoded
	}

	// Dummy epoch nonce (hex encoded) for VRF verification
	eta0Hex := hex.EncodeToString(make([]byte, 32))
	slotsPerKes := uint64(129600) // Mainnet value

	t.Run("AllEras", func(t *testing.T) {
		// Pre-loop: call VerifyBlock once per block to log whether each
		// succeeds or fails. Errors are expected because we use a dummy
		// epoch nonce, so VRF verification will fail.
		for j, block := range decodedBlocks {
			if blocks[j].BlockType == ledger.BlockTypeByronMain ||
				blocks[j].BlockType == ledger.BlockTypeByronEbb {
				continue
			}
			_, _, _, _, err := ledger.VerifyBlock(
				block,
				eta0Hex,
				slotsPerKes,
				config,
			)
			if err != nil {
				t.Logf(
					"VerifyBlock error for %s (expected with dummy VRF input): %v",
					blocks[j].Name,
					err,
				)
			}
		}

		// Hot loop: errors are expected (dummy VRF nonce), and we still
		// exercise the code path up to the point of failure, which is
		// the target of this profile. We capture the first error to
		// confirm we are hitting the expected code path.
		var firstErr error
		for i := 0; i < profileIterations; i++ {
			for j, block := range decodedBlocks {
				if blocks[j].BlockType == ledger.BlockTypeByronMain ||
					blocks[j].BlockType == ledger.BlockTypeByronEbb {
					continue
				}
				_, _, _, _, err := ledger.VerifyBlock(
					block,
					eta0Hex,
					slotsPerKes,
					config,
				)
				if err != nil && firstErr == nil {
					firstErr = err
				}
			}
		}
		if firstErr != nil {
			t.Logf(
				"VerifyBlock error in hot loop (expected): %v",
				firstErr,
			)
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

			// Pre-loop: single call to log whether verification succeeds or
			// fails. Errors are expected with the dummy epoch nonce.
			_, _, _, _, err := ledger.VerifyBlock(
				decoded,
				eta0Hex,
				slotsPerKes,
				config,
			)
			if err != nil {
				t.Logf(
					"VerifyBlock error (expected with dummy VRF input): %v",
					err,
				)
			}

			// Hot loop: errors are expected (dummy VRF nonce) and we
			// capture the first to confirm the expected code path.
			var loopErr error
			for i := 0; i < profileIterations; i++ {
				_, _, _, _, err = ledger.VerifyBlock(
					decoded,
					eta0Hex,
					slotsPerKes,
					config,
				)
				if err != nil && loopErr == nil {
					loopErr = err
				}
			}
			if loopErr != nil {
				t.Logf(
					"VerifyBlock error in hot loop (expected): %v",
					loopErr,
				)
			}
		})
	}
}

// TestProfileTxValidation generates profiles for transaction validation rules.
// This exercises the per-era UtxoValidationRules against transactions extracted
// from test blocks.
//
// Example:
//
//	go test -tags=profile -run=TestProfileTxValidation \
//	    -cpuprofile=cpu_txval.prof -memprofile=mem_txval.prof \
//	    ./internal/bench/...
func TestProfileTxValidation(t *testing.T) {
	blocks := testdata.GetTestBlocks()

	// BenchLedgerState provides a minimal mock ledger state suitable for
	// profiling. We use the bench helper rather than MockLedgerState from
	// internal/test/ledger because profiling tests need a lightweight,
	// self-contained state without full test framework dependencies.
	ls := BenchLedgerState()
	pp := BenchProtocolParams()

	for _, block := range blocks {
		// Skip Byron - no UTXO validation rules for Byron era
		if block.BlockType == ledger.BlockTypeByronMain ||
			block.BlockType == ledger.BlockTypeByronEbb {
			continue
		}

		decoded, err := ledger.NewBlockFromCbor(block.BlockType, block.Cbor)
		require.NoError(t, err, "failed to decode %s block", block.Name)

		txs := decoded.Transactions()
		if len(txs) == 0 {
			t.Logf("skipping %s: no transactions in block", block.Name)
			continue
		}

		// Select the era-appropriate validation rules
		var rules []common.UtxoValidationRuleFunc
		switch decoded.Era().Id {
		case shelley.EraShelley.Id:
			rules = shelley.UtxoValidationRules
		case allegra.EraAllegra.Id:
			rules = allegra.UtxoValidationRules
		case mary.EraMary.Id:
			rules = mary.UtxoValidationRules
		case alonzo.EraAlonzo.Id:
			rules = alonzo.UtxoValidationRules
		case babbage.EraBabbage.Id:
			rules = babbage.UtxoValidationRules
		case conway.EraConway.Id:
			rules = conway.UtxoValidationRules
		default:
			t.Logf(
				"skipping %s: unsupported era for validation rules",
				block.Name,
			)
			continue
		}

		t.Run(block.Name, func(t *testing.T) {
			tx := txs[0]
			slot := decoded.SlotNumber()

			// Pre-loop: log whether validation passes or fails. Errors are
			// expected because the mock ledger state does not contain the
			// UTXOs referenced by real mainnet transactions.
			err := common.VerifyTransaction(tx, slot, ls, pp, rules)
			if err != nil {
				t.Logf(
					"VerifyTransaction error (expected with mock state): %v",
					err,
				)
			}

			// Hot loop: exercise the validation rule code path.
			// Errors are expected and discarded for consistent profiling.
			for i := 0; i < profileIterations; i++ {
				_ = common.VerifyTransaction(tx, slot, ls, pp, rules)
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
	require.NoError(t, err, "VRF KeyGen failed")

	// Test message (simulates slot + nonce input)
	alpha := make([]byte, 64)
	for i := range alpha {
		alpha[i] = byte(i)
	}

	// Generate a proof for verification tests
	proof, hash, err := vrf.Prove(sk, alpha)
	require.NoError(t, err, "VRF Prove failed")

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
			_, err := vrf.MkInputVrf(slot, eta0)
			if err != nil {
				t.Fatalf("VRF MkInputVrf failed: %v", err)
			}
		}
	})

	t.Run("FullLeaderCheck", func(t *testing.T) {
		eta0 := make([]byte, 32)
		for i := range eta0 {
			eta0[i] = byte(i * 7)
		}

		for i := 0; i < profileIterations; i++ {
			slot := int64(50_000_000 + i)
			vrfInput, err := vrf.MkInputVrf(slot, eta0)
			if err != nil {
				t.Fatalf("VRF MkInputVrf failed: %v", err)
			}

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
	t.Run("LeaderValueComputation", func(t *testing.T) {
		// Generate VRF proof output (seed must be exactly 32 bytes)
		seed := []byte("consensus_profile_test_seed_32!!")
		_, sk, err := vrf.KeyGen(seed)
		require.NoError(t, err, "VRF KeyGen failed")

		eta0 := make([]byte, 32)
		for i := range eta0 {
			eta0[i] = byte(i * 3)
		}

		for i := 0; i < profileIterations; i++ {
			slot := int64(100_000_000 + i)
			vrfInput, err := vrf.MkInputVrf(slot, eta0)
			if err != nil {
				t.Fatalf("VRF MkInputVrf failed: %v", err)
			}

			_, hash, err := vrf.Prove(sk, vrfInput)
			if err != nil {
				t.Fatalf("VRF Prove failed: %v", err)
			}

			// Compute leader value from VRF output
			_ = consensus.VrfLeaderValue(hash)
		}
	})
}

// TestProfileBodyHash generates profiles for block decode and body hash access.
// This profiles CBOR decoding and the BlockBodyHash() accessor; it does NOT
// profile full body-hash validation (which is done by ledger.VerifyBlock).
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
			// Profile CBOR decode + body hash access.
			// NewBlockFromCbor only decodes CBOR; it does not validate the
			// body hash. Full body-hash validation is performed by
			// ledger.VerifyBlock. This loop profiles decode + accessing
			// BlockBodyHash() (compute/access, not verify).
			for i := 0; i < profileIterations; i++ {
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
