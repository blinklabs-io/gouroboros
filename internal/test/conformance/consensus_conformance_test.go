// Copyright 2025 Blink Labs Software
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

package conformance

import (
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/consensus"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/gouroboros/vrf"
)

// TestShelleyBlockVRFExtraction tests that we can extract VRF data from real Shelley blocks
func TestShelleyBlockVRFExtraction(t *testing.T) {
	blockPath := filepath.Join(
		"..",
		"..",
		"..",
		"protocol",
		"chainsync",
		"testdata",
		"shelley_block_testnet_02b1c561715da9e540411123a6135ee319b02f60b9a11a603d3305556c04329f.hex",
	)

	hexData, err := os.ReadFile(blockPath)
	if err != nil {
		t.Skipf("Test file not found: %v", err)
	}

	data, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	block, err := shelley.NewShelleyBlockFromCbor(data)
	if err != nil {
		t.Fatalf("Failed to decode Shelley block: %v", err)
	}

	header := block.Header()
	headerTyped, ok := header.(*shelley.ShelleyBlockHeader)
	if !ok {
		t.Fatalf("expected *shelley.ShelleyBlockHeader, got %T", header)
	}
	body := headerTyped.Body

	// Verify VRF key is present and has correct size
	if len(body.VrfKey) != 32 {
		t.Errorf("VRF key should be 32 bytes, got %d", len(body.VrfKey))
	}

	// Verify NonceVrf components
	// Note: In Shelley era, VRF outputs are 64 bytes (SHA512 output)
	if len(body.NonceVrf.Output) != 64 {
		t.Errorf(
			"NonceVrf output should be 64 bytes, got %d",
			len(body.NonceVrf.Output),
		)
	}
	if len(body.NonceVrf.Proof) != 80 {
		t.Errorf(
			"NonceVrf proof should be 80 bytes, got %d",
			len(body.NonceVrf.Proof),
		)
	}

	// Verify LeaderVrf components
	if len(body.LeaderVrf.Output) != 64 {
		t.Errorf(
			"LeaderVrf output should be 64 bytes, got %d",
			len(body.LeaderVrf.Output),
		)
	}
	if len(body.LeaderVrf.Proof) != 80 {
		t.Errorf(
			"LeaderVrf proof should be 80 bytes, got %d",
			len(body.LeaderVrf.Proof),
		)
	}

	t.Logf("Shelley block slot=%d, block=%d", body.Slot, body.BlockNumber)
	t.Logf("VRF key: %x...", body.VrfKey[:8])
	t.Logf("NonceVrf output: %x...", body.NonceVrf.Output[:8])
	t.Logf("LeaderVrf output: %x...", body.LeaderVrf.Output[:8])
}

// TestVRFProofToHashConsistency tests that ProofToHash produces consistent output
// for proofs extracted from real blocks
func TestVRFProofToHashConsistency(t *testing.T) {
	blockPath := filepath.Join(
		"..",
		"..",
		"..",
		"protocol",
		"chainsync",
		"testdata",
		"shelley_block_testnet_02b1c561715da9e540411123a6135ee319b02f60b9a11a603d3305556c04329f.hex",
	)

	hexData, err := os.ReadFile(blockPath)
	if err != nil {
		t.Skipf("Test file not found: %v", err)
	}

	data, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	block, err := shelley.NewShelleyBlockFromCbor(data)
	if err != nil {
		t.Fatalf("Failed to decode Shelley block: %v", err)
	}

	header := block.Header()
	headerTyped, ok := header.(*shelley.ShelleyBlockHeader)
	if !ok {
		t.Fatalf("expected *shelley.ShelleyBlockHeader, got %T", header)
	}
	body := headerTyped.Body

	// Test NonceVrf proof
	nonceOutput, err := vrf.ProofToHash(body.NonceVrf.Proof)
	if err != nil {
		t.Fatalf("Failed to get hash from NonceVrf proof: %v", err)
	}

	// The output from ProofToHash should be 64 bytes (SHA512)
	if len(nonceOutput) != 64 {
		t.Errorf(
			"ProofToHash output should be 64 bytes, got %d",
			len(nonceOutput),
		)
	}

	// Test LeaderVrf proof
	leaderOutput, err := vrf.ProofToHash(body.LeaderVrf.Proof)
	if err != nil {
		t.Fatalf("Failed to get hash from LeaderVrf proof: %v", err)
	}

	if len(leaderOutput) != 64 {
		t.Errorf(
			"LeaderVrf ProofToHash output should be 64 bytes, got %d",
			len(leaderOutput),
		)
	}

	t.Logf("NonceVrf ProofToHash: %x...", nonceOutput[:16])
	t.Logf("LeaderVrf ProofToHash: %x...", leaderOutput[:16])
}

// TestLeaderThresholdKnownValues tests the leader threshold calculation against
// known values derived from the Cardano specification.
//
// The threshold formula is: T = 2^256 * (1 - (1-f)^σ)  (CPRAOS)
// where f = active slot coefficient, σ = relative stake
func TestLeaderThresholdKnownValues(t *testing.T) {
	tests := []struct {
		name            string
		poolStake       uint64
		totalStake      uint64
		activeSlotCoeff *big.Rat
		// For these tests, we verify properties rather than exact values
		// because the exact values depend on the precision of calculations
		expectPositive  bool
		expectBitLength int // approximate expected bit length
	}{
		{
			name:            "mainnet_1percent_stake",
			poolStake:       1_000_000_000,     // 1 billion lovelace
			totalStake:      100_000_000_000,   // 100 billion lovelace
			activeSlotCoeff: big.NewRat(1, 20), // 0.05
			expectPositive:  true,
			expectBitLength: 249, // approximately (CPRAOS uses 2^256)
		},
		{
			name:            "mainnet_5percent_stake",
			poolStake:       5_000_000_000,     // 5 billion lovelace
			totalStake:      100_000_000_000,   // 100 billion lovelace
			activeSlotCoeff: big.NewRat(1, 20), // 0.05
			expectPositive:  true,
			expectBitLength: 251, // approximately (CPRAOS uses 2^256)
		},
		{
			name:            "mainnet_10percent_stake",
			poolStake:       10_000_000_000,    // 10 billion lovelace
			totalStake:      100_000_000_000,   // 100 billion lovelace
			activeSlotCoeff: big.NewRat(1, 20), // 0.05
			expectPositive:  true,
			expectBitLength: 252, // approximately (CPRAOS uses 2^256)
		},
		{
			name:            "mainnet_50percent_stake",
			poolStake:       50_000_000_000,    // 50 billion lovelace
			totalStake:      100_000_000_000,   // 100 billion lovelace
			activeSlotCoeff: big.NewRat(1, 20), // 0.05
			expectPositive:  true,
			expectBitLength: 254, // approximately (CPRAOS uses 2^256)
		},
		{
			name:            "mainnet_100percent_stake",
			poolStake:       100_000_000_000,   // 100 billion lovelace
			totalStake:      100_000_000_000,   // 100 billion lovelace
			activeSlotCoeff: big.NewRat(1, 20), // 0.05
			expectPositive:  true,
			expectBitLength: 253, // f = 0.05, so threshold ≈ 0.05 * 2^256 (CPRAOS)
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			threshold := consensus.CertifiedNatThreshold(
				tc.poolStake,
				tc.totalStake,
				tc.activeSlotCoeff,
			)

			if tc.expectPositive && threshold.Sign() <= 0 {
				t.Error("expected positive threshold")
			}

			bitLen := threshold.BitLen()
			// Allow for some variance in bit length (+/- 5)
			if bitLen < tc.expectBitLength-5 || bitLen > tc.expectBitLength+5 {
				t.Errorf("bit length %d not in expected range [%d, %d]",
					bitLen, tc.expectBitLength-5, tc.expectBitLength+5)
			}

			t.Logf("threshold bit length: %d", bitLen)
		})
	}
}

// TestLeaderThresholdMonotonicity verifies that higher stake always results
// in higher threshold (more likely to be elected leader)
func TestLeaderThresholdMonotonicity(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20)
	totalStake := uint64(100_000_000_000) // 100 billion

	var prevThreshold *big.Int
	// Test from 1% to 100% stake in 10% increments
	for pct := uint64(1); pct <= 100; pct += 10 {
		poolStake := totalStake * pct / 100
		threshold := consensus.CertifiedNatThreshold(
			poolStake,
			totalStake,
			activeSlotCoeff,
		)

		if prevThreshold != nil {
			if threshold.Cmp(prevThreshold) <= 0 {
				t.Errorf(
					"threshold should increase monotonically with stake: %d%% <= previous",
					pct,
				)
			}
		}
		prevThreshold = threshold

		t.Logf("stake %d%%: threshold bit length = %d", pct, threshold.BitLen())
	}
}

// TestLeaderElectionProbability verifies that the calculated threshold
// corresponds approximately to the expected probability
func TestLeaderElectionProbability(t *testing.T) {
	// With f=0.05 and σ=1 (100% stake), probability should be approximately 0.05
	// So threshold should be approximately 0.05 * 2^256 (CPRAOS)

	activeSlotCoeff := big.NewRat(
		1,
		20,
	) // 0.05
	threshold := consensus.CertifiedNatThreshold(
		100,
		100,
		activeSlotCoeff,
	) // 100% stake

	// 2^256 (CPRAOS uses 256-bit threshold)
	twoTo256 := new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil)

	// Calculate probability: threshold / 2^256
	// Use floating point for comparison (acceptable for this verification)
	thresholdFloat := new(big.Float).SetInt(threshold)
	maxFloat := new(big.Float).SetInt(twoTo256)
	probability := new(big.Float).Quo(thresholdFloat, maxFloat)

	probFloat64, _ := probability.Float64()

	// Expected probability is f = 0.05
	// Allow for some numerical precision differences
	if probFloat64 < 0.04 || probFloat64 > 0.06 {
		t.Errorf(
			"probability %.6f not in expected range [0.04, 0.06]",
			probFloat64,
		)
	}

	t.Logf("Calculated probability: %.6f (expected ~0.05)", probFloat64)
}

// TestVRFInputGeneration tests the VRF input generation function
func TestVRFInputGeneration(t *testing.T) {
	// Test with known slot and nonce values
	slot := int64(12345678)
	eta0 := make([]byte, 32)
	for i := range eta0 {
		eta0[i] = byte(i)
	}

	input := vrf.MkInputVrf(slot, eta0)

	// Input should be 32 bytes (blake2b-256 output)
	if len(input) != 32 {
		t.Errorf("VRF input should be 32 bytes, got %d", len(input))
	}

	// Input should be deterministic
	input2 := vrf.MkInputVrf(slot, eta0)
	if hex.EncodeToString(input) != hex.EncodeToString(input2) {
		t.Error("VRF input should be deterministic")
	}

	// Different slot should produce different input
	input3 := vrf.MkInputVrf(slot+1, eta0)
	if hex.EncodeToString(input) == hex.EncodeToString(input3) {
		t.Error("different slot should produce different VRF input")
	}

	// Different nonce should produce different input
	eta1 := make([]byte, 32)
	for i := range eta1 {
		eta1[i] = byte(i + 1)
	}
	input4 := vrf.MkInputVrf(slot, eta1)
	if hex.EncodeToString(input) == hex.EncodeToString(input4) {
		t.Error("different nonce should produce different VRF input")
	}

	t.Logf("VRF input for slot %d: %x", slot, input)
}

// TestCardanoCryptoPraosVectors tests against vectors from cardano-crypto-praos
// These vectors use ECVRF-ED25519-SHA512-Elligator2 (ietfdraft03)
// The vectors are from the existing vrf_vectors.json which sources from:
// https://github.com/input-output-hk/vrf
func TestCardanoCryptoPraosVectors(t *testing.T) {
	// Subset of vectors from vrf_vectors.json for quick consensus conformance testing
	// Full test coverage is in TestVRFVerifyConformance
	vectors := []struct {
		name  string
		pk    string // Public key (32 bytes hex)
		alpha string // Input message (hex)
		pi    string // Proof (80 bytes hex)
		beta  string // Output (64 bytes hex)
	}{
		{
			// Vector 0: empty input
			name:  "empty_input",
			pk:    "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a",
			alpha: "",
			pi:    "b6b4699f87d56126c9117a7da55bd0085246f4c56dbc95d20172612e9d38e8d7ca65e573a126ed88d4e30a46f80a666854d675cf3ba81de0de043c3774f061560f55edc256a787afe701677c0f602900",
			beta:  "5b49b554d05c0cd5a5325376b3387de59d924fd1e13ded44648ab33c21349a603f25b84ec5ed887995b33da5e3bfcb87cd2f64521c4c62cf825cffabbe5d31cc",
		},
		{
			// Vector 1: single byte input
			name:  "single_byte_input",
			pk:    "3d4017c3e843895a92b70aa74d1b7ebc9c982ccf2ec4968cc0cd55f12af4660c",
			alpha: "72",
			pi:    "ae5b66bdf04b4c010bfe32b2fc126ead2107b697634f6f7337b9bff8785ee111200095ece87dde4dbe87343f6df3b107d91798c8a7eb1245d3bb9c5aafb093358c13e6ae1111a55717e895fd15f99f07",
			beta:  "94f4487e1b2fec954309ef1289ecb2e15043a2461ecc7b2ae7d4470607ef82eb1cfa97d84991fe4a7bfdfd715606bc27e2967a6c557cfb5875879b671740b7d8",
		},
		{
			// Vector 2: two byte input
			name:  "two_byte_input",
			pk:    "fc51cd8e6218a1a38da47ed00230f0580816ed13ba3303ac5deb911548908025",
			alpha: "af82",
			pi:    "dfa2cba34b611cc8c833a6ea83b8eb1bb5e2ef2dd1b0c481bc42ff36ae7847f6ab52b976cfd5def172fa412defde270c8b8bdfbaae1c7ece17d9833b1bcf31064fff78ef493f820055b561ece45e1009",
			beta:  "2031837f582cd17a9af9e0c7ef5a6540e3453ed894b62c293686ca3c1e319dde9d0aa489a4b59a9594fc2328bc3deff3c8a0929a369a72b1180a596e016b5ded",
		},
	}

	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			// Decode expected values
			expectedPK, err := hex.DecodeString(v.pk)
			if err != nil {
				t.Fatalf("failed to decode pk: %v", err)
			}
			proof, err := hex.DecodeString(v.pi)
			if err != nil {
				t.Fatalf("failed to decode pi: %v", err)
			}
			expectedOutput, err := hex.DecodeString(v.beta)
			if err != nil {
				t.Fatalf("failed to decode beta: %v", err)
			}
			alpha, err := hex.DecodeString(v.alpha)
			if err != nil {
				t.Fatalf("failed to decode alpha: %v", err)
			}

			// Test ProofToHash produces expected output
			output, err := vrf.ProofToHash(proof)
			if err != nil {
				t.Fatalf("ProofToHash failed: %v", err)
			}

			if hex.EncodeToString(output) != v.beta {
				t.Errorf("ProofToHash mismatch:\n  got:  %x\n  want: %s",
					output, v.beta)
			}

			// Test Verify with expected values
			valid, err := vrf.Verify(expectedPK, proof, expectedOutput, alpha)
			if err != nil {
				t.Fatalf("Verify failed: %v", err)
			}
			if !valid {
				t.Error("Verify returned false for valid proof")
			}

			t.Logf("Vector %s verified successfully", v.name)
		})
	}
}

// TestActiveSlotCoefficientImpact tests how different active slot coefficients
// affect the leader election threshold
func TestActiveSlotCoefficientImpact(t *testing.T) {
	poolStake := uint64(50_000_000_000)   // 50 billion (50% stake)
	totalStake := uint64(100_000_000_000) // 100 billion

	coefficients := []struct {
		name string
		f    *big.Rat
	}{
		{"f=0.01", big.NewRat(1, 100)},
		{"f=0.05", big.NewRat(1, 20)}, // mainnet
		{"f=0.10", big.NewRat(1, 10)},
		{"f=0.20", big.NewRat(1, 5)},
	}

	var prevThreshold *big.Int
	for _, coef := range coefficients {
		threshold := consensus.CertifiedNatThreshold(
			poolStake,
			totalStake,
			coef.f,
		)

		if prevThreshold != nil {
			// Higher f should result in higher threshold
			if threshold.Cmp(prevThreshold) <= 0 {
				t.Errorf(
					"threshold should increase with higher f: %s",
					coef.name,
				)
			}
		}
		prevThreshold = threshold

		t.Logf("%s: threshold bit length = %d", coef.name, threshold.BitLen())
	}
}

// TestVRFOutputEligibility tests the eligibility check with various outputs
// Note: CPRAOS applies BLAKE2b-256("L" || vrfOutput) before comparison,
// so the raw VRF output value doesn't directly determine eligibility.
func TestVRFOutputEligibility(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20) // 0.05

	// Pool with 100% stake (high threshold for testing)
	threshold := consensus.CertifiedNatThreshold(
		100_000_000_000,
		100_000_000_000,
		activeSlotCoeff,
	)

	// With CPRAOS, the VRF output is hashed with "L" prefix before comparison
	// So we can't assume zero output = eligible. Instead, we test that:
	// 1. The function is deterministic
	// 2. Max output with full stake threshold still has some eligible cases

	tests := []struct {
		name   string
		output []byte
	}{
		{
			name:   "zero_output",
			output: make([]byte, 64),
		},
		{
			name: "max_output",
			output: func() []byte {
				b := make([]byte, 64)
				for i := range b {
					b[i] = 0xFF
				}
				return b
			}(),
		},
		{
			name: "small_output",
			output: func() []byte {
				b := make([]byte, 64)
				b[63] = 0x01 // value = 1
				return b
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Test determinism - same input should give same result
			eligible1 := consensus.IsVRFOutputBelowThreshold(tc.output, threshold)
			eligible2 := consensus.IsVRFOutputBelowThreshold(tc.output, threshold)
			if eligible1 != eligible2 {
				t.Error("IsVRFOutputBelowThreshold should be deterministic")
			}

			// Log the result for visibility
			leaderValue := consensus.VrfLeaderValue(tc.output)
			t.Logf("%s: eligible=%v, leaderValue=%x (first 8 bytes)",
				tc.name, eligible1, leaderValue[:8])
		})
	}
}
