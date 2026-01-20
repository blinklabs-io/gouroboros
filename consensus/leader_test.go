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

package consensus

import (
	"math/big"
	"testing"
)

// Test seed (exactly 32 bytes)
var testVRFSeed = []byte("test_vrf_seed_for_consensus!!!!!")

func TestNewSimpleVRFSigner(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("NewSimpleVRFSigner failed: %v", err)
	}

	if len(signer.PublicKey()) != 32 {
		t.Errorf("expected 32-byte public key, got %d", len(signer.PublicKey()))
	}
}

func TestSimpleVRFSignerProve(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("NewSimpleVRFSigner failed: %v", err)
	}

	input := []byte("test input")
	proof, output, err := signer.Prove(input)
	if err != nil {
		t.Fatalf("Prove failed: %v", err)
	}

	if len(proof) != 80 {
		t.Errorf("expected 80-byte proof, got %d", len(proof))
	}
	if len(output) != 64 {
		t.Errorf("expected 64-byte output, got %d", len(output))
	}
}

func TestIsSlotLeader(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("NewSimpleVRFSigner failed: %v", err)
	}

	epochNonce := make([]byte, 32)
	for i := range epochNonce {
		epochNonce[i] = byte(i)
	}

	// Test with mainnet-like parameters (f = 0.05)
	activeSlotCoeff := big.NewRat(1, 20)

	// A pool with 100% stake should always be eligible
	result, err := IsSlotLeader(
		1000, // slot
		epochNonce,
		1000000000000, // poolStake = 100% of total
		1000000000000, // totalStake
		activeSlotCoeff,
		signer,
	)
	if err != nil {
		t.Fatalf("IsSlotLeader failed: %v", err)
	}

	// With 100% stake, probability ≈ 0.05, so not guaranteed but likely
	// We just check that the function runs correctly
	if result.Threshold == nil {
		t.Error("expected non-nil threshold")
	}
	if result.Eligible && (result.Proof == nil || result.Output == nil) {
		t.Error("if eligible, proof and output should not be nil")
	}
}

func TestIsSlotLeaderZeroStake(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("NewSimpleVRFSigner failed: %v", err)
	}

	epochNonce := make([]byte, 32)
	activeSlotCoeff := big.NewRat(1, 20)

	// Zero pool stake should never be eligible
	result, err := IsSlotLeader(
		1000,
		epochNonce,
		0,             // poolStake = 0
		1000000000000, // totalStake
		activeSlotCoeff,
		signer,
	)
	if err != nil {
		t.Fatalf("IsSlotLeader failed: %v", err)
	}

	if result.Eligible {
		t.Error("pool with zero stake should not be eligible")
	}
}

func TestIsSlotLeaderZeroTotalStake(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("NewSimpleVRFSigner failed: %v", err)
	}

	epochNonce := make([]byte, 32)
	activeSlotCoeff := big.NewRat(1, 20)

	// Zero total stake should return not eligible (edge case)
	result, err := IsSlotLeader(
		1000,
		epochNonce,
		1000,
		0, // totalStake = 0
		activeSlotCoeff,
		signer,
	)
	if err != nil {
		t.Fatalf("IsSlotLeader failed: %v", err)
	}

	if result.Eligible {
		t.Error("should not be eligible with zero total stake")
	}
}

func TestIsSlotLeaderNilSigner(t *testing.T) {
	epochNonce := make([]byte, 32)
	activeSlotCoeff := big.NewRat(1, 20)

	_, err := IsSlotLeader(
		1000,
		epochNonce,
		1000,
		10000,
		activeSlotCoeff,
		nil,
	)
	if err == nil {
		t.Error("expected error for nil signer")
	}
}

func TestIsSlotLeaderEmptyNonce(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("NewSimpleVRFSigner failed: %v", err)
	}

	activeSlotCoeff := big.NewRat(1, 20)

	_, err = IsSlotLeader(
		1000,
		[]byte{}, // empty nonce
		1000,
		10000,
		activeSlotCoeff,
		signer,
	)
	if err == nil {
		t.Error("expected error for empty epoch nonce")
	}
}

func TestIsSlotLeaderNilActiveSlotCoeff(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("NewSimpleVRFSigner failed: %v", err)
	}

	epochNonce := make([]byte, 32)

	_, err = IsSlotLeader(
		1000,
		epochNonce,
		1000,
		10000,
		nil, // nil activeSlotCoeff
		signer,
	)
	if err == nil {
		t.Error("expected error for nil activeSlotCoeff")
	}
}

func TestIsSlotLeaderFromComponents(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20)

	// Test with a VRF output that should definitely be below threshold for 100% stake
	// (output of all zeros would be 0, which is below any positive threshold)
	zeroOutput := make([]byte, 64)

	eligible := IsSlotLeaderFromComponents(
		zeroOutput,
		1000000000000,
		1000000000000,
		activeSlotCoeff,
	)

	if !eligible {
		t.Error("output of zeros with 100% stake should be eligible")
	}

	// Test with maximum output (all 0xFF bytes)
	maxOutput := make([]byte, 64)
	for i := range maxOutput {
		maxOutput[i] = 0xFF
	}

	eligible = IsSlotLeaderFromComponents(
		maxOutput,
		1, // very small stake
		1000000000000,
		activeSlotCoeff,
	)

	// Maximum VRF output (2^512 - 1) with tiny stake (1/1e12) should NOT be eligible.
	// IsSlotLeaderFromComponents is deterministic: the threshold calculation
	// for 1/1e12 stake with f=0.05 gives T ≈ 2^512 * 5e-14, which is much less
	// than 2^512 - 1. So the max output should never be below this threshold.
	if eligible {
		t.Error("maximum VRF output with tiny stake should not be eligible")
	}
}

func TestIsSlotLeaderDeterminism(t *testing.T) {
	signer1, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("failed to create signer1: %v", err)
	}
	signer2, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("failed to create signer2: %v", err)
	}

	epochNonce := make([]byte, 32)
	for i := range epochNonce {
		epochNonce[i] = byte(i)
	}
	activeSlotCoeff := big.NewRat(1, 20)

	result1, err := IsSlotLeader(
		12345,
		epochNonce,
		1000000,
		10000000,
		activeSlotCoeff,
		signer1,
	)
	if err != nil {
		t.Fatalf("IsSlotLeader failed: %v", err)
	}

	result2, err := IsSlotLeader(
		12345,
		epochNonce,
		1000000,
		10000000,
		activeSlotCoeff,
		signer2,
	)
	if err != nil {
		t.Fatalf("IsSlotLeader failed: %v", err)
	}

	// Results should be identical for same inputs
	if result1.Eligible != result2.Eligible {
		t.Error("leader election should be deterministic")
	}
	if result1.Threshold.Cmp(result2.Threshold) != 0 {
		t.Error("thresholds should be identical for same stake parameters")
	}
}

func TestFindNextSlotLeadership(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("NewSimpleVRFSigner failed: %v", err)
	}

	epochNonce := make([]byte, 32)
	for i := range epochNonce {
		epochNonce[i] = byte(i * 3)
	}
	activeSlotCoeff := big.NewRat(
		9,
		10,
	) // 90% active slot coefficient for faster tests

	// With 100% stake and f=0.9, probability per slot ≈ 0.9
	// We should find eligibility very quickly
	slot, proof, output, err := FindNextSlotLeadership(
		1,   // start slot
		100, // max slot (reduced from 1000)
		epochNonce,
		10000000000, // poolStake = 100%
		10000000000, // totalStake
		activeSlotCoeff,
		signer,
	)
	if err != nil {
		t.Fatalf("FindNextSlotLeadership failed: %v", err)
	}

	if slot == 0 {
		// This is probabilistically unlikely but possible
		t.Log("no slot leadership found in range (may be valid but unlikely)")
	} else {
		if proof == nil || output == nil {
			t.Error("if slot found, proof and output should not be nil")
		}
		if slot < 1 || slot > 100 {
			t.Errorf("found slot %d outside search range", slot)
		}
	}
}

func TestFindNextSlotLeadershipNoEligibility(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	if err != nil {
		t.Fatalf("NewSimpleVRFSigner failed: %v", err)
	}

	epochNonce := make([]byte, 32)
	activeSlotCoeff := big.NewRat(1, 20)

	// With zero stake, should never find eligibility
	slot, proof, output, err := FindNextSlotLeadership(
		1,
		100,
		epochNonce,
		0,          // zero stake
		1000000000, // total stake
		activeSlotCoeff,
		signer,
	)
	if err != nil {
		t.Fatalf("FindNextSlotLeadership failed: %v", err)
	}

	if slot != 0 || proof != nil || output != nil {
		t.Error("should not find eligibility with zero stake")
	}
}
