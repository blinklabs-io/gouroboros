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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test seed (exactly 32 bytes)
var testVRFSeed = []byte("test_vrf_seed_for_consensus!!!!!")

func TestNewSimpleVRFSigner(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	require.NoError(t, err, "NewSimpleVRFSigner failed")

	assert.Len(t, signer.PublicKey(), 32, "expected 32-byte public key")
}

func TestSimpleVRFSignerProve(t *testing.T) {
	signer, err := NewSimpleVRFSigner(testVRFSeed)
	require.NoError(t, err, "NewSimpleVRFSigner failed")

	input := []byte("test input")
	proof, output, err := signer.Prove(input)
	require.NoError(t, err, "Prove failed")

	assert.Len(t, proof, 80, "expected 80-byte proof")
	assert.Len(t, output, 64, "expected 64-byte output")
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

	// With CPRAOS, the VRF output is first hashed with VrfLeaderValue (BLAKE2b-256
	// with "L" prefix) before comparison against the threshold. This means the
	// leader value is deterministic but not directly related to the raw VRF output.

	// Test with zero VRF output - the leader value is a specific hash
	zeroOutput := make([]byte, 64)
	leaderValue := VrfLeaderValue(zeroOutput)
	leaderValueInt := new(big.Int).SetBytes(leaderValue)

	// With 100% stake, threshold is about 5% of 2^256
	threshold := CertifiedNatThreshold(1000000000000, 1000000000000, activeSlotCoeff)
	expectedEligible := leaderValueInt.Cmp(threshold) < 0

	eligible := IsSlotLeaderFromComponents(
		zeroOutput,
		1000000000000,
		1000000000000,
		activeSlotCoeff,
	)

	if eligible != expectedEligible {
		t.Errorf("IsSlotLeaderFromComponents result (%v) does not match expected (%v) for zero output",
			eligible, expectedEligible)
	}

	// Test with maximum VRF output (all 0xFF bytes)
	// The leader value is still a hash, so it won't necessarily be max value
	maxOutput := make([]byte, 64)
	for i := range maxOutput {
		maxOutput[i] = 0xFF
	}

	maxLeaderValue := VrfLeaderValue(maxOutput)
	maxLeaderValueInt := new(big.Int).SetBytes(maxLeaderValue)

	// With tiny stake (1/1e12) and f=0.05, threshold is very small
	tinyThreshold := CertifiedNatThreshold(1, 1000000000000, activeSlotCoeff)
	expectedEligibleMax := maxLeaderValueInt.Cmp(tinyThreshold) < 0

	eligible = IsSlotLeaderFromComponents(
		maxOutput,
		1, // very small stake
		1000000000000,
		activeSlotCoeff,
	)

	// The eligibility depends on the hash of maxOutput, not maxOutput itself
	if eligible != expectedEligibleMax {
		t.Errorf("IsSlotLeaderFromComponents result (%v) does not match expected (%v) for max output with tiny stake",
			eligible, expectedEligibleMax)
	}

	// Verify determinism - same inputs should give same results
	eligible2 := IsSlotLeaderFromComponents(
		zeroOutput,
		1000000000000,
		1000000000000,
		activeSlotCoeff,
	)
	if eligible2 != expectedEligible {
		t.Error("IsSlotLeaderFromComponents should be deterministic")
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
