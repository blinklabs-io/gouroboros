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

// Package bench provides allocation regression tests and benchmarks for
// gouroboros.
package bench

import (
	"math/big"
	"os"
	"strconv"
	"testing"

	"github.com/blinklabs-io/gouroboros/consensus"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/vrf"
	"github.com/stretchr/testify/require"
)

// Pre-computed test data for allocation regression tests.
// These are initialized once to avoid including setup allocations in tests.
var (
	// VRF test data
	vrfSeed       = []byte("test_seed_for_vrf_testing!!!_32!")
	vrfPK         []byte
	vrfSK         []byte
	vrfProof      []byte
	vrfOutput     []byte
	vrfAlpha      = []byte("vrf test message for regression")
	vrfEpochNonce = make([]byte, 32)

	// KES test data
	kesSeed = []byte(
		"test string of 32 byte of lenght",
	) // Matches canonical test vector
	kesSK      *kes.SecretKey
	kesPK      []byte
	kesSig     []byte
	kesMessage = []byte("kes test message for regression")

	// Consensus test data
	poolStake       = uint64(1_000_000_000)   // 1B lovelace
	totalStake      = uint64(100_000_000_000) // 100B lovelace
	activeSlotCoeff = big.NewRat(1, 20)       // 0.05

	// Block test data (loaded lazily)
	conwayBlockCbor []byte
)

func init() {
	var err error

	// Initialize VRF test data
	vrfPK, vrfSK, err = vrf.KeyGen(vrfSeed)
	if err != nil {
		panic("failed to generate VRF keys: " + err.Error())
	}
	vrfProof, vrfOutput, err = vrf.Prove(vrfSK, vrfAlpha)
	if err != nil {
		panic("failed to generate VRF proof: " + err.Error())
	}
	for i := range vrfEpochNonce {
		vrfEpochNonce[i] = byte(i)
	}

	// Initialize KES test data
	kesSK, kesPK, err = kes.KeyGen(kes.CardanoKesDepth, kesSeed)
	if err != nil {
		panic("failed to generate KES keys: " + err.Error())
	}
	kesSig, err = kes.Sign(kesSK, 0, kesMessage)
	if err != nil {
		panic("failed to generate KES signature: " + err.Error())
	}

	// Initialize block test data from testdata package
	blocks := testdata.GetTestBlocks()
	for _, block := range blocks {
		if block.Name == "Conway" {
			conwayBlockCbor = block.Cbor
			break
		}
	}
	if conwayBlockCbor == nil {
		panic("failed to find Conway block in testdata")
	}
}

// getThresholdMultiplier returns the allocation threshold multiplier from
// environment. Default is 1.0 (no adjustment). Set
// GOUROBOROS_ALLOC_THRESHOLD_MULTIPLIER to override.
func getThresholdMultiplier() float64 {
	if v := os.Getenv("GOUROBOROS_ALLOC_THRESHOLD_MULTIPLIER"); v != "" {
		if m, err := strconv.ParseFloat(v, 64); err == nil && m > 0 {
			return m
		}
	}
	return 1.0
}

// TestAllocationRegression tests that key paths don't exceed allocation limits.
// This helps catch allocation regressions before they are merged.
//
// To adjust thresholds temporarily (e.g., during optimization work):
//
//	GOUROBOROS_ALLOC_THRESHOLD_MULTIPLIER=1.5 go test
//
// -run=TestAllocationRegression ./internal/bench/...
func TestAllocationRegression(t *testing.T) {
	multiplier := getThresholdMultiplier()

	tests := []struct {
		name      string
		fn        func() any
		maxAllocs int64
	}{
		// Current baselines with 10% headroom:
		// VRFVerify: 11 allocs (measured 2026-02-10)
		{"VRFVerify", vrfVerifyOnce, 12},
		// KESVerify: 12 allocs (measured 2026-02-10)
		{"KESVerify", kesVerifyOnce, 14},
		// LeaderCheck: 1225 allocs - includes Taylor series for threshold calc
		// (measured 2026-02-10)
		{"LeaderCheck", leaderCheckOnce, 1350},
		// CBORBlockDecode (Conway): 2652 allocs (measured 2026-02-10)
		{"CBORBlockDecode", cborBlockDecodeOnce, 3000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Run the function once to warm up (cache effects, lazy init)
			_ = tc.fn()

			// Measure allocations over 100 runs
			allocs := testing.AllocsPerRun(100, func() {
				_ = tc.fn()
			})

			adjustedLimit := float64(tc.maxAllocs) * multiplier
			if allocs > adjustedLimit {
				t.Errorf(
					"%s: %.0f allocs > %.0f limit (base: %d, multiplier: %.2f)",
					tc.name,
					allocs,
					adjustedLimit,
					tc.maxAllocs,
					multiplier,
				)
			} else {
				t.Logf("%s: %.0f allocs (limit: %.0f)", tc.name, allocs, adjustedLimit)
			}
		})
	}
}

// TestAllocationBaselines runs extended allocation measurements and reports
// the actual allocation counts for each operation. This is useful for
// establishing new baselines or investigating allocation changes.
func TestAllocationBaselines(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping baseline test in short mode")
	}

	tests := []struct {
		name string
		fn   func() any
	}{
		{"VRFVerify", vrfVerifyOnce},
		{"VRFProve", vrfProveOnce},
		{"VRFMkInputVrf", vrfMkInputVrfOnce},
		{"KESVerify", kesVerifyOnce},
		{"LeaderCheck", leaderCheckOnce},
		{"ThresholdCalc", thresholdCalcOnce},
		{"CBORBlockDecode", cborBlockDecodeOnce},
	}

	t.Log("Allocation baselines (1000 iterations):")
	for _, tc := range tests {
		// Warm up
		_ = tc.fn()

		allocs := testing.AllocsPerRun(1000, func() {
			_ = tc.fn()
		})

		t.Logf("  %s: %.2f allocs/op", tc.name, allocs)
	}
}

// vrfVerifyOnce performs a single VRF verification.
func vrfVerifyOnce() any {
	valid, err := vrf.Verify(vrfPK, vrfProof, vrfOutput, vrfAlpha)
	if err != nil || !valid {
		panic("VRF verification failed")
	}
	return valid
}

// vrfProveOnce performs a single VRF proof generation.
func vrfProveOnce() any {
	proof, output, err := vrf.Prove(vrfSK, vrfAlpha)
	if err != nil {
		panic("VRF proof generation failed: " + err.Error())
	}
	return [][]byte{proof, output}
}

// vrfMkInputVrfOnce performs a single VRF input creation.
func vrfMkInputVrfOnce() any {
	return vrf.MkInputVrf(50_000_000, vrfEpochNonce)
}

// kesVerifyOnce performs a single KES signature verification.
func kesVerifyOnce() any {
	valid := kes.VerifySignedKES(kesPK, 0, kesMessage, kesSig)
	if !valid {
		panic("KES verification failed")
	}
	return valid
}

// leaderCheckOnce performs a single leader election check using pre-computed
// components (no VRF signing, just threshold calculation and comparison).
func leaderCheckOnce() any {
	return consensus.IsSlotLeaderFromComponents(
		vrfOutput,
		poolStake,
		totalStake,
		activeSlotCoeff,
	)
}

// thresholdCalcOnce performs a single threshold calculation.
func thresholdCalcOnce() any {
	return consensus.CertifiedNatThreshold(
		poolStake,
		totalStake,
		activeSlotCoeff,
	)
}

// cborBlockDecodeOnce performs a single Conway block CBOR decode.
func cborBlockDecodeOnce() any {
	block, err := ledger.NewBlockFromCbor(
		ledger.BlockTypeConway,
		conwayBlockCbor,
	)
	if err != nil {
		panic("block decode failed: " + err.Error())
	}
	return block
}

// TestRegressionVRF verifies VRF allocation regression with detailed breakdown.
func TestRegressionVRF(t *testing.T) {
	multiplier := getThresholdMultiplier()

	t.Run("Verify", func(t *testing.T) {
		// Validate operation works before measuring allocations
		valid, err := vrf.Verify(vrfPK, vrfProof, vrfOutput, vrfAlpha)
		require.NoError(t, err)
		require.True(t, valid, "VRF verification should succeed")

		allocs := testing.AllocsPerRun(100, func() {
			_, _ = vrf.Verify(vrfPK, vrfProof, vrfOutput, vrfAlpha)
		})
		limit := 12.0 * multiplier // Baseline: 11 allocs
		require.LessOrEqualf(t, allocs, limit,
			"VRF Verify: %.0f allocs exceeds limit %.0f", allocs, limit)
		t.Logf("VRF Verify: %.0f allocs (limit: %.0f)", allocs, limit)
	})

	t.Run("VerifyAndHash", func(t *testing.T) {
		// Validate operation works before measuring allocations
		output, err := vrf.VerifyAndHash(vrfPK, vrfProof, vrfAlpha)
		require.NoError(t, err)
		require.NotNil(t, output, "VRF VerifyAndHash should return output")

		allocs := testing.AllocsPerRun(100, func() {
			_, _ = vrf.VerifyAndHash(vrfPK, vrfProof, vrfAlpha)
		})
		limit := 12.0 * multiplier // Baseline: 11 allocs
		require.LessOrEqualf(t, allocs, limit,
			"VRF VerifyAndHash: %.0f allocs exceeds limit %.0f", allocs, limit)
		t.Logf("VRF VerifyAndHash: %.0f allocs (limit: %.0f)", allocs, limit)
	})

	t.Run("Prove", func(t *testing.T) {
		// Validate operation works before measuring allocations
		proof, output, err := vrf.Prove(vrfSK, vrfAlpha)
		require.NoError(t, err)
		require.NotNil(t, proof, "VRF Prove should return proof")
		require.NotNil(t, output, "VRF Prove should return output")

		allocs := testing.AllocsPerRun(100, func() {
			_, _, _ = vrf.Prove(vrfSK, vrfAlpha)
		})
		limit := 15.0 * multiplier // Baseline: 13 allocs
		require.LessOrEqualf(t, allocs, limit,
			"VRF Prove: %.0f allocs exceeds limit %.0f", allocs, limit)
		t.Logf("VRF Prove: %.0f allocs (limit: %.0f)", allocs, limit)
	})

	t.Run("MkInputVrf", func(t *testing.T) {
		allocs := testing.AllocsPerRun(100, func() {
			_ = vrf.MkInputVrf(50_000_000, vrfEpochNonce)
		})
		limit := 5.0 * multiplier // Baseline: 4 allocs
		require.LessOrEqualf(t, allocs, limit,
			"VRF MkInputVrf: %.0f allocs exceeds limit %.0f", allocs, limit)
		t.Logf("VRF MkInputVrf: %.0f allocs (limit: %.0f)", allocs, limit)
	})
}

// TestRegressionKES verifies KES allocation regression with detailed breakdown.
func TestRegressionKES(t *testing.T) {
	multiplier := getThresholdMultiplier()

	t.Run("VerifySignedKES", func(t *testing.T) {
		// Validate operation works before measuring allocations
		valid := kes.VerifySignedKES(kesPK, 0, kesMessage, kesSig)
		require.True(t, valid, "KES verification should succeed")

		allocs := testing.AllocsPerRun(100, func() {
			_ = kes.VerifySignedKES(kesPK, 0, kesMessage, kesSig)
		})
		limit := 12.0 * multiplier
		require.LessOrEqualf(
			t,
			allocs,
			limit,
			"KES VerifySignedKES: %.0f allocs exceeds limit %.0f",
			allocs,
			limit,
		)
		t.Logf("KES VerifySignedKES: %.0f allocs (limit: %.0f)", allocs, limit)
	})

	t.Run("NewSumKesFromBytes", func(t *testing.T) {
		// Validate deserialization succeeds before measuring allocations
		_, err := kes.NewSumKesFromBytes(kes.CardanoKesDepth, kesSig)
		require.NoError(t, err, "NewSumKesFromBytes should succeed")

		allocs := testing.AllocsPerRun(100, func() {
			_, _ = kes.NewSumKesFromBytes(kes.CardanoKesDepth, kesSig)
		})
		limit := 8.0 * multiplier
		require.LessOrEqualf(
			t,
			allocs,
			limit,
			"KES NewSumKesFromBytes: %.0f allocs exceeds limit %.0f",
			allocs,
			limit,
		)
		t.Logf(
			"KES NewSumKesFromBytes: %.0f allocs (limit: %.0f)",
			allocs,
			limit,
		)
	})
}

// TestRegressionConsensus verifies consensus allocation regression.
func TestRegressionConsensus(t *testing.T) {
	multiplier := getThresholdMultiplier()

	t.Run("CertifiedNatThreshold", func(t *testing.T) {
		allocs := testing.AllocsPerRun(100, func() {
			_ = consensus.CertifiedNatThreshold(
				poolStake,
				totalStake,
				activeSlotCoeff,
			)
		})
		// Baseline: 1220 allocs - uses Taylor series with big.Rat operations
		limit := 1350.0 * multiplier
		require.LessOrEqualf(
			t,
			allocs,
			limit,
			"CertifiedNatThreshold: %.0f allocs exceeds limit %.0f",
			allocs,
			limit,
		)
		t.Logf(
			"CertifiedNatThreshold: %.0f allocs (limit: %.0f)",
			allocs,
			limit,
		)
	})

	t.Run("IsSlotLeaderFromComponents", func(t *testing.T) {
		allocs := testing.AllocsPerRun(100, func() {
			_ = consensus.IsSlotLeaderFromComponents(
				vrfOutput, poolStake, totalStake, activeSlotCoeff)
		})
		// Baseline: 1225 allocs - includes threshold calculation
		limit := 1350.0 * multiplier
		require.LessOrEqualf(
			t,
			allocs,
			limit,
			"IsSlotLeaderFromComponents: %.0f allocs exceeds limit %.0f",
			allocs,
			limit,
		)
		t.Logf(
			"IsSlotLeaderFromComponents: %.0f allocs (limit: %.0f)",
			allocs,
			limit,
		)
	})

	t.Run("VrfLeaderValue", func(t *testing.T) {
		allocs := testing.AllocsPerRun(100, func() {
			_ = consensus.VrfLeaderValue(vrfOutput)
		})
		limit := 5.0 * multiplier // Baseline: 4 allocs
		require.LessOrEqualf(t, allocs, limit,
			"VrfLeaderValue: %.0f allocs exceeds limit %.0f", allocs, limit)
		t.Logf("VrfLeaderValue: %.0f allocs (limit: %.0f)", allocs, limit)
	})
}

// TestRegressionCBOR verifies CBOR decode allocation regression.
func TestRegressionCBOR(t *testing.T) {
	multiplier := getThresholdMultiplier()
	blocks := testdata.GetTestBlocks()

	// Expected allocation limits by era (measured 2026-02-10 + 10% headroom)
	limits := map[string]int64{
		"Byron":   550,  // Baseline: 500
		"Shelley": 485,  // Baseline: 441
		"Allegra": 1180, // Baseline: 1072
		"Mary":    1230, // Baseline: 1116
		"Alonzo":  1500, // Baseline: 1362
		"Babbage": 6260, // Baseline: 5688
		"Conway":  2920, // Baseline: 2652
	}

	for _, block := range blocks {
		t.Run(block.Name, func(t *testing.T) {
			limit, ok := limits[block.Name]
			if !ok {
				t.Skipf("no limit defined for %s", block.Name)
			}

			// Validate decode works before measuring allocations
			_, err := ledger.NewBlockFromCbor(block.BlockType, block.Cbor)
			require.NoError(
				t,
				err,
				"%s block decode should succeed",
				block.Name,
			)

			allocs := testing.AllocsPerRun(100, func() {
				_, _ = ledger.NewBlockFromCbor(block.BlockType, block.Cbor)
			})

			adjustedLimit := float64(limit) * multiplier
			require.LessOrEqualf(
				t,
				allocs,
				adjustedLimit,
				"%s block decode: %.0f allocs exceeds limit %.0f",
				block.Name,
				allocs,
				adjustedLimit,
			)
			t.Logf(
				"%s: %.0f allocs (limit: %.0f)",
				block.Name,
				allocs,
				adjustedLimit,
			)
		})
	}
}
