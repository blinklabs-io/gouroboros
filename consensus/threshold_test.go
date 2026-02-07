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
	"bytes"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"
)

func TestCertifiedNatThresholdZeroStake(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20) // 0.05

	// Zero pool stake
	threshold := CertifiedNatThreshold(0, 1000000, activeSlotCoeff)
	if threshold.Cmp(big.NewInt(0)) != 0 {
		t.Error("expected zero threshold for zero pool stake")
	}

	// Zero total stake
	threshold = CertifiedNatThreshold(1000, 0, activeSlotCoeff)
	if threshold.Cmp(big.NewInt(0)) != 0 {
		t.Error("expected zero threshold for zero total stake")
	}
}

func TestCertifiedNatThresholdFullStake(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20) // 0.05

	// Pool with 100% stake
	threshold := CertifiedNatThreshold(1000000, 1000000, activeSlotCoeff)

	// With 100% stake and f=0.05, threshold should be approximately 0.05 * 2^256
	// The threshold should be positive and substantial
	if threshold.Sign() <= 0 {
		t.Error("expected positive threshold for full stake")
	}

	// Check it's in the right ballpark: should be roughly 5% of 2^256
	// 2^256 is a 257-bit number, 5% of it should be around 252-256 bits
	bitLen := threshold.BitLen()
	if bitLen < 248 || bitLen > 256 {
		t.Errorf(
			"threshold bit length %d not in expected range [248, 256]",
			bitLen,
		)
	}
}

func TestCertifiedNatThresholdPartialStake(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20) // 0.05

	// 10% stake pool
	threshold10 := CertifiedNatThreshold(100000, 1000000, activeSlotCoeff)

	// 50% stake pool
	threshold50 := CertifiedNatThreshold(500000, 1000000, activeSlotCoeff)

	// Higher stake should have higher threshold (more likely to be leader)
	if threshold50.Cmp(threshold10) <= 0 {
		t.Error("higher stake should result in higher threshold")
	}
}

func TestCertifiedNatThresholdMonotonicity(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20)
	totalStake := uint64(1000000000)

	var prevThreshold *big.Int
	// Test 10 evenly spaced stake values for monotonicity
	for stake := uint64(100000000); stake <= totalStake; stake += 100000000 {
		threshold := CertifiedNatThreshold(stake, totalStake, activeSlotCoeff)

		if prevThreshold != nil && threshold.Cmp(prevThreshold) < 0 {
			t.Errorf("threshold should be monotonically increasing with stake")
		}
		prevThreshold = threshold
	}
}

func TestCertifiedNatThresholdStakeExceedsTotal(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20)

	// Pool stake > total stake (should be capped to total)
	threshold := CertifiedNatThreshold(2000000, 1000000, activeSlotCoeff)

	// Should be the same as 100% stake
	threshold100 := CertifiedNatThreshold(1000000, 1000000, activeSlotCoeff)

	if threshold.Cmp(threshold100) != 0 {
		t.Error("stake exceeding total should be capped")
	}
}

func TestVRFOutputToInt(t *testing.T) {
	// Test with zeros
	zeroOutput := make([]byte, 64)
	zeroInt := VRFOutputToInt(zeroOutput)
	if zeroInt.Cmp(big.NewInt(0)) != 0 {
		t.Error("zero output should convert to zero integer")
	}

	// Test with 0x01 at the end (little-endian interpretation would be 1)
	oneOutput := make([]byte, 64)
	oneOutput[63] = 0x01
	oneInt := VRFOutputToInt(oneOutput)
	if oneInt.Cmp(big.NewInt(1)) != 0 {
		t.Errorf("expected 1, got %s", oneInt.String())
	}

	// Test with 0x01 at the start (big-endian interpretation)
	bigOutput := make([]byte, 64)
	bigOutput[0] = 0x01
	bigInt := VRFOutputToInt(bigOutput)
	expected := new(
		big.Int,
	).Exp(big.NewInt(2), big.NewInt(504), nil)
	// 2^(63*8)
	if bigInt.Cmp(expected) != 0 {
		t.Errorf("expected 2^504, got %s", bigInt.String())
	}
}

func TestIsVRFOutputBelowThreshold(t *testing.T) {
	// Note: IsVRFOutputBelowThreshold now uses CPRAOS algorithm which
	// hashes the VRF output with VrfLeaderValue first (BLAKE2b-256 with "L" prefix)
	// before comparing against the threshold.

	// Create a VRF output and compute its leader value
	vrfOutput := make([]byte, 64)
	vrfOutput[0] = 0x42 // Some arbitrary data
	leaderValue := VrfLeaderValue(vrfOutput)
	leaderValueInt := new(big.Int).SetBytes(leaderValue)

	// Test with threshold just above the leader value
	thresholdAbove := new(big.Int).Add(leaderValueInt, big.NewInt(1))
	if !IsVRFOutputBelowThreshold(vrfOutput, thresholdAbove) {
		t.Error("VRF output should be below threshold when threshold > leader value")
	}

	// Test with threshold equal to the leader value (should not be below)
	if IsVRFOutputBelowThreshold(vrfOutput, leaderValueInt) {
		t.Error("VRF output should not be below threshold when threshold == leader value")
	}

	// Test with threshold below the leader value
	if leaderValueInt.Sign() > 0 {
		thresholdBelow := new(big.Int).Sub(leaderValueInt, big.NewInt(1))
		if IsVRFOutputBelowThreshold(vrfOutput, thresholdBelow) {
			t.Error("VRF output should not be below threshold when threshold < leader value")
		}
	}

	// Test with nil threshold
	if IsVRFOutputBelowThreshold(vrfOutput, nil) {
		t.Error("should return false for nil threshold")
	}

	// Test with empty output
	if IsVRFOutputBelowThreshold([]byte{}, big.NewInt(1000000)) {
		t.Error("should return false for empty output")
	}
}

func TestDifferentActiveSlotCoefficients(t *testing.T) {
	totalStake := uint64(1000000000)
	poolStake := uint64(500000000) // 50%

	// f = 0.05 (mainnet)
	f005 := big.NewRat(1, 20)
	threshold005 := CertifiedNatThreshold(poolStake, totalStake, f005)

	// f = 0.10
	f010 := big.NewRat(1, 10)
	threshold010 := CertifiedNatThreshold(poolStake, totalStake, f010)

	// Higher f should result in higher threshold (more slots filled)
	if threshold010.Cmp(threshold005) <= 0 {
		t.Error("higher active slot coefficient should increase threshold")
	}
}

func TestLnOneMinusSmallValues(t *testing.T) {
	// Test that ln(1-x) works correctly for small x
	// ln(1-0.05) ≈ -0.05129...
	x := big.NewRat(1, 20) // 0.05
	result := lnOneMinus(x)

	// Result should be negative
	if result.Sign() >= 0 {
		t.Error("ln(1-0.05) should be negative")
	}

	// Result should be approximately -0.05129 (within some tolerance)
	// We check that -0.055 < result < -0.05
	lowerBound := big.NewRat(-55, 1000) // -0.055
	upperBound := big.NewRat(-50, 1000) // -0.050

	if result.Cmp(lowerBound) < 0 || result.Cmp(upperBound) > 0 {
		t.Errorf(
			"ln(1-0.05) = %s, expected approximately -0.0513",
			result.FloatString(4),
		)
	}
}

func TestExpRational(t *testing.T) {
	// Test exp(0) = 1
	zero := big.NewRat(0, 1)
	result := expRational(zero)
	if result.Cmp(big.NewRat(1, 1)) != 0 {
		t.Errorf("exp(0) should be 1, got %s", result.FloatString(4))
	}

	// Test exp(1) ≈ 2.71828...
	one := big.NewRat(1, 1)
	result = expRational(one)

	// Check that 2.71 < result < 2.72
	lowerBound := big.NewRat(271, 100)
	upperBound := big.NewRat(272, 100)

	if result.Cmp(lowerBound) < 0 || result.Cmp(upperBound) > 0 {
		t.Errorf(
			"exp(1) = %s, expected approximately 2.718",
			result.FloatString(4),
		)
	}

	// Test exp(-1) ≈ 0.36788...
	negOne := big.NewRat(-1, 1)
	result = expRational(negOne)

	// Check that 0.36 < result < 0.37
	lowerBound = big.NewRat(36, 100)
	upperBound = big.NewRat(37, 100)

	if result.Cmp(lowerBound) < 0 || result.Cmp(upperBound) > 0 {
		t.Errorf(
			"exp(-1) = %s, expected approximately 0.368",
			result.FloatString(4),
		)
	}
}

func TestThresholdDeterminism(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20)
	poolStake := uint64(123456789)
	totalStake := uint64(987654321)

	threshold1 := CertifiedNatThreshold(poolStake, totalStake, activeSlotCoeff)
	threshold2 := CertifiedNatThreshold(poolStake, totalStake, activeSlotCoeff)

	if threshold1.Cmp(threshold2) != 0 {
		t.Error("threshold calculation should be deterministic")
	}
}

func TestVRFOutputConversionRoundTrip(t *testing.T) {
	// Create a known big integer
	original := new(big.Int)
	original.SetString("123456789012345678901234567890", 10)

	// Convert to 64-byte output
	outputBytes := original.Bytes()
	vrfOutput := make([]byte, 64)
	copy(vrfOutput[64-len(outputBytes):], outputBytes)

	// Convert back
	recovered := VRFOutputToInt(vrfOutput)

	if original.Cmp(recovered) != 0 {
		t.Errorf(
			"round-trip failed: original=%s, recovered=%s",
			original,
			recovered,
		)
	}
}

func TestZeroOutputLeaderValue(t *testing.T) {
	// With CPRAOS, the VRF output is hashed with VrfLeaderValue before comparison.
	// Zero VRF output produces a specific leader value from the hash.
	// Test that the leader value is computed correctly and can be compared.

	zeroOutput := make([]byte, 64)
	leaderValue := VrfLeaderValue(zeroOutput)

	// The leader value should be 32 bytes
	if len(leaderValue) != 32 {
		t.Errorf("expected 32-byte leader value, got %d bytes", len(leaderValue))
	}

	// The leader value should be deterministic
	leaderValue2 := VrfLeaderValue(zeroOutput)
	if !bytes.Equal(leaderValue, leaderValue2) {
		t.Error("VrfLeaderValue should be deterministic")
	}

	// Test that IsSlotLeaderFromComponents uses the hashed value
	activeSlotCoeff := big.NewRat(1, 20)
	leaderValueInt := new(big.Int).SetBytes(leaderValue)

	// With 100% stake, the threshold is about 5% of 2^256
	// Check if the zero output's leader value is below or above
	threshold := CertifiedNatThreshold(1000000000, 1000000000, activeSlotCoeff)
	eligible := leaderValueInt.Cmp(threshold) < 0

	// Verify IsSlotLeaderFromComponents matches
	eligibleFromFunc := IsSlotLeaderFromComponents(
		zeroOutput,
		1000000000,
		1000000000,
		activeSlotCoeff,
	)

	if eligible != eligibleFromFunc {
		t.Error("IsSlotLeaderFromComponents result should match manual comparison")
	}
}

func TestMaxOutputNeverEligible(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20)

	// Maximum VRF output (all 0xFF bytes)
	maxOutput := make([]byte, 64)
	for i := range maxOutput {
		maxOutput[i] = 0xFF
	}

	// With CPRAOS, the VRF output is hashed via VrfLeaderValue (BLAKE2b-256 with "L" prefix)
	// before comparison. The resulting leader value is deterministic but unpredictable.
	// With f=0.05, threshold ≈ 0.05 * 2^256, so only ~5% of possible leader values are eligible.
	// The hash of all-0xFF input produces a specific value that happens to be ineligible.
	eligible := IsSlotLeaderFromComponents(
		maxOutput,
		1000000000,
		1000000000, // 100% stake
		activeSlotCoeff,
	)

	require.False(t, eligible,
		"maximum VRF output should not be eligible with the deterministic hash result")
}

func TestVRFOutputOrder(t *testing.T) {
	// Verify that big-endian byte order is used
	output1 := make([]byte, 64)
	output2 := make([]byte, 64)

	output1[0] = 0x01  // High byte set
	output2[63] = 0xFF // Low byte set

	int1 := VRFOutputToInt(output1)
	int2 := VRFOutputToInt(output2)

	// int1 should be much larger than int2 (big-endian)
	if int1.Cmp(int2) <= 0 {
		t.Error("VRF output should be interpreted as big-endian")
	}
}

func TestThresholdWithVerySmallStake(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20)

	// Very small stake ratio (1 lovelace out of 1 trillion)
	threshold := CertifiedNatThreshold(1, 1000000000000, activeSlotCoeff)

	// Threshold should be tiny but positive
	if threshold.Sign() <= 0 {
		t.Error("threshold should be positive for non-zero stake")
	}

	// Should be much smaller than full stake threshold
	fullThreshold := CertifiedNatThreshold(
		1000000000000,
		1000000000000,
		activeSlotCoeff,
	)
	ratio := new(big.Int).Div(fullThreshold, threshold)

	// The ratio should be very large (close to the stake ratio)
	if ratio.Cmp(big.NewInt(1000000)) < 0 {
		t.Errorf("threshold ratio seems too small: %s", ratio)
	}
}

func TestThresholdDeterministicBytes(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20)
	poolStake := uint64(500000000)
	totalStake := uint64(1000000000)

	// Verify threshold bytes are deterministic (same input produces same bytes)
	threshold1 := CertifiedNatThreshold(poolStake, totalStake, activeSlotCoeff)
	threshold2 := CertifiedNatThreshold(poolStake, totalStake, activeSlotCoeff)

	// Note: CertifiedNatThreshold doesn't take a slot parameter - it only depends
	// on stake and activeSlotCoeff. This test verifies byte-level determinism.
	if !bytes.Equal(threshold1.Bytes(), threshold2.Bytes()) {
		t.Error("threshold should be the same for same stake parameters")
	}
}

// =============================================================================
// CPRAOS Leader Election Tests
// =============================================================================

// TestVrfLeaderValueReturns32Bytes verifies that VrfLeaderValue returns exactly
// 32 bytes (BLAKE2b-256 output).
func TestVrfLeaderValueReturns32Bytes(t *testing.T) {
	// Test with various VRF output sizes
	testCases := []struct {
		name      string
		inputSize int
	}{
		{"64-byte VRF output", 64},
		{"32-byte input", 32},
		{"empty input", 0},
		{"single byte", 1},
		{"100-byte input", 100},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input := make([]byte, tc.inputSize)
			// Fill with some non-zero data for non-empty inputs
			for i := range input {
				input[i] = byte(i % 256)
			}

			result := VrfLeaderValue(input)
			if len(result) != 32 {
				t.Errorf("VrfLeaderValue should return 32 bytes, got %d", len(result))
			}
		})
	}
}

// TestVrfLeaderValueAppliesLPrefix verifies that VrfLeaderValue computes
// BLAKE2b-256 with "L" (0x4C) prefix as specified by CPRAOS.
func TestVrfLeaderValueAppliesLPrefix(t *testing.T) {
	// The VRF leader value is computed as:
	// BLAKE2b-256(0x4C || vrfOutput)
	// where 0x4C is ASCII "L"

	testData := []byte{0x01, 0x02, 0x03, 0x04}
	result := VrfLeaderValue(testData)

	// Compute expected result manually using golang.org/x/crypto/blake2b
	// with the "L" prefix (0x4C)
	prefixedData := append([]byte{0x4C}, testData...)
	hasher, err := blake2b.New256(nil)
	if err != nil {
		t.Fatalf("failed to create blake2b hasher: %v", err)
	}
	hasher.Write(prefixedData)
	expected := hasher.Sum(nil)

	if !bytes.Equal(result, expected) {
		t.Errorf("VrfLeaderValue did not apply 'L' (0x4C) prefix correctly\nexpected: %x\ngot: %x", expected, result)
	}
}

// TestVrfLeaderValueKnownVector tests VrfLeaderValue against a known test vector.
// The test vector is computed by manually calculating BLAKE2b-256(0x4C || input).
func TestVrfLeaderValueKnownVector(t *testing.T) {
	// Test vector: 64 bytes of zeros (typical VRF output size)
	zeroInput := make([]byte, 64)

	// Compute expected: BLAKE2b-256(0x4C || zeros(64))
	prefixedData := append([]byte{0x4C}, zeroInput...)
	hasher, err := blake2b.New256(nil)
	if err != nil {
		t.Fatalf("failed to create blake2b hasher: %v", err)
	}
	hasher.Write(prefixedData)
	expected := hasher.Sum(nil)

	result := VrfLeaderValue(zeroInput)
	if !bytes.Equal(result, expected) {
		t.Errorf("VrfLeaderValue failed for known vector\nexpected: %x\ngot: %x", expected, result)
	}

	// The result should be deterministic
	result2 := VrfLeaderValue(zeroInput)
	if !bytes.Equal(result, result2) {
		t.Error("VrfLeaderValue should be deterministic")
	}
}

// TestVrfLeaderValueDifferentInputsProduceDifferentOutputs verifies that
// different inputs produce different outputs (basic collision resistance check).
func TestVrfLeaderValueDifferentInputsProduceDifferentOutputs(t *testing.T) {
	input1 := make([]byte, 64)
	input2 := make([]byte, 64)
	input2[0] = 0x01 // Single bit difference

	result1 := VrfLeaderValue(input1)
	result2 := VrfLeaderValue(input2)

	if bytes.Equal(result1, result2) {
		t.Error("different inputs should produce different VrfLeaderValue outputs")
	}
}

// TestVrfLeaderValueEmptyInput tests handling of empty input.
func TestVrfLeaderValueEmptyInput(t *testing.T) {
	// Empty input should still work - BLAKE2b-256(0x4C)
	result := VrfLeaderValue(nil)
	if len(result) != 32 {
		t.Errorf("VrfLeaderValue(nil) should return 32 bytes, got %d", len(result))
	}

	// Also test empty slice
	result2 := VrfLeaderValue([]byte{})
	if len(result2) != 32 {
		t.Errorf("VrfLeaderValue(empty) should return 32 bytes, got %d", len(result2))
	}

	// nil and empty slice should produce the same result
	if !bytes.Equal(result, result2) {
		t.Error("VrfLeaderValue(nil) and VrfLeaderValue(empty) should be equal")
	}
}

// =============================================================================
// 256-bit Threshold Tests
// =============================================================================

// TestCertifiedNatThreshold256BitBased verifies that the threshold is now
// calculated using 2^256 instead of 2^512.
func TestCertifiedNatThreshold256BitBased(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20) // 0.05

	// Pool with 100% stake
	threshold := CertifiedNatThreshold(1000000, 1000000, activeSlotCoeff)

	// With 100% stake and f=0.05, threshold should be approximately 0.05 * 2^256
	// The threshold should be positive and substantial
	if threshold.Sign() <= 0 {
		t.Error("expected positive threshold for full stake")
	}

	// Check it's in the right ballpark: should be roughly 5% of 2^256
	// 2^256 is a 257-bit number, 5% of it should be around 252-256 bits
	bitLen := threshold.BitLen()
	if bitLen < 248 || bitLen > 256 {
		t.Errorf(
			"256-bit threshold bit length %d not in expected range [248, 256] for 100%% stake",
			bitLen,
		)
	}

	// Verify it's NOT in the 512-bit range
	if bitLen > 300 {
		t.Errorf("threshold appears to be 512-bit based, not 256-bit (bitLen=%d)", bitLen)
	}
}

// TestCertifiedNatThresholdWith32ByteLeaderValue verifies that a 32-byte leader
// value works correctly with the threshold.
func TestCertifiedNatThresholdWith32ByteLeaderValue(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20)

	// Create a 32-byte leader value (simulating VrfLeaderValue output)
	leaderValue := make([]byte, 32)

	// Zero leader value should always be below threshold for non-zero stake
	threshold := CertifiedNatThreshold(500000, 1000000, activeSlotCoeff)
	leaderValueInt := new(big.Int).SetBytes(leaderValue)

	if leaderValueInt.Cmp(threshold) >= 0 {
		t.Error("zero leader value should be below threshold for 50% stake")
	}

	// Maximum 32-byte value
	maxLeaderValue := make([]byte, 32)
	for i := range maxLeaderValue {
		maxLeaderValue[i] = 0xFF
	}
	maxLeaderValueInt := new(big.Int).SetBytes(maxLeaderValue)

	// Maximum leader value should not be below threshold (even with 100% stake at f=0.05)
	threshold100 := CertifiedNatThreshold(1000000, 1000000, activeSlotCoeff)
	if maxLeaderValueInt.Cmp(threshold100) < 0 {
		t.Error("maximum 32-byte leader value should not be below threshold")
	}
}

// TestThreshold256BitUpperBound verifies the threshold never exceeds 2^256.
func TestThreshold256BitUpperBound(t *testing.T) {
	// Even with 100% stake and high active slot coefficient, threshold should be < 2^256
	highActiveSlotCoeff := big.NewRat(9, 10) // 0.9 (90% - unrealistic but good for testing)

	threshold := CertifiedNatThreshold(1000000000, 1000000000, highActiveSlotCoeff)

	twoTo256 := new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil)
	if threshold.Cmp(twoTo256) >= 0 {
		t.Error("threshold should never exceed 2^256")
	}
}

// =============================================================================
// IsVRFOutputBelowThreshold with Hashing Tests
// =============================================================================

// TestIsVRFOutputBelowThresholdHashesFirst verifies that IsVRFOutputBelowThreshold
// hashes the 64-byte VRF output first before comparing.
func TestIsVRFOutputBelowThresholdHashesFirst(t *testing.T) {
	// When IsVRFOutputBelowThreshold receives a 64-byte VRF output,
	// it should first compute VrfLeaderValue (32 bytes) then compare

	// Create a 64-byte VRF output
	vrfOutput := make([]byte, 64)
	for i := range vrfOutput {
		vrfOutput[i] = byte(i)
	}

	// Compute what the leader value should be
	expectedLeaderValue := VrfLeaderValue(vrfOutput)
	leaderValueInt := new(big.Int).SetBytes(expectedLeaderValue)

	// Create a threshold just above the leader value
	thresholdAbove := new(big.Int).Add(leaderValueInt, big.NewInt(1))
	// Create a threshold just below the leader value
	thresholdBelow := new(big.Int).Sub(leaderValueInt, big.NewInt(1))

	// Should be below when threshold is above
	if !IsVRFOutputBelowThreshold(vrfOutput, thresholdAbove) {
		t.Error("VRF output should be below threshold when threshold > leader value")
	}

	// Should NOT be below when threshold is below
	if leaderValueInt.Sign() > 0 && IsVRFOutputBelowThreshold(vrfOutput, thresholdBelow) {
		t.Error("VRF output should not be below threshold when threshold < leader value")
	}
}

// TestIsVRFOutputBelowThresholdNilThreshold tests handling of nil threshold.
func TestIsVRFOutputBelowThresholdNilThreshold(t *testing.T) {
	vrfOutput := make([]byte, 64)

	result := IsVRFOutputBelowThreshold(vrfOutput, nil)
	if result {
		t.Error("IsVRFOutputBelowThreshold should return false for nil threshold")
	}
}

// TestIsVRFOutputBelowThresholdEmptyOutput tests handling of empty VRF output.
func TestIsVRFOutputBelowThresholdEmptyOutput(t *testing.T) {
	threshold := big.NewInt(1000000)

	// Empty output should return false (invalid input)
	result := IsVRFOutputBelowThreshold([]byte{}, threshold)
	if result {
		t.Error("IsVRFOutputBelowThreshold should return false for empty output")
	}

	// Nil output should also return false
	result = IsVRFOutputBelowThreshold(nil, threshold)
	if result {
		t.Error("IsVRFOutputBelowThreshold should return false for nil output")
	}
}

// TestIsVRFOutputBelowThresholdZeroThreshold tests handling of zero threshold.
func TestIsVRFOutputBelowThresholdZeroThreshold(t *testing.T) {
	vrfOutput := make([]byte, 64)

	// Zero threshold - nothing should be below it (leader value is always >= 0)
	result := IsVRFOutputBelowThreshold(vrfOutput, big.NewInt(0))
	if result {
		t.Error("IsVRFOutputBelowThreshold should return false for zero threshold")
	}
}

// TestIsVRFOutputBelowThresholdMaxThreshold tests with maximum 256-bit threshold.
func TestIsVRFOutputBelowThresholdMaxThreshold(t *testing.T) {
	vrfOutput := make([]byte, 64)
	for i := range vrfOutput {
		vrfOutput[i] = 0xFF // Max value
	}

	// Maximum 256-bit threshold (2^256 - 1)
	maxThreshold := new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil)
	maxThreshold.Sub(maxThreshold, big.NewInt(1))

	// With CPRAOS, the leader value is BLAKE2b-256("L" || vrfOutput), which produces
	// a 256-bit value. This value will always be < 2^256 - 1 (the max threshold),
	// so the result should always be true.
	result := IsVRFOutputBelowThreshold(vrfOutput, maxThreshold)
	require.True(t, result, "leader value should always be below max 256-bit threshold")
}

// TestIsVRFOutputBelowThresholdDeterminism verifies deterministic behavior.
func TestIsVRFOutputBelowThresholdDeterminism(t *testing.T) {
	vrfOutput := make([]byte, 64)
	for i := range vrfOutput {
		vrfOutput[i] = byte(i * 7 % 256)
	}
	threshold := big.NewInt(1000000000)

	result1 := IsVRFOutputBelowThreshold(vrfOutput, threshold)
	result2 := IsVRFOutputBelowThreshold(vrfOutput, threshold)

	if result1 != result2 {
		t.Error("IsVRFOutputBelowThreshold should be deterministic")
	}
}

// =============================================================================
// Integration Tests for CPRAOS Leader Election
// =============================================================================

// TestCPRAOSLeaderElectionIntegration tests the full CPRAOS leader election flow.
func TestCPRAOSLeaderElectionIntegration(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20) // 0.05

	// Simulate a 64-byte VRF output
	vrfOutput := make([]byte, 64)
	// Use deterministic test data
	for i := range vrfOutput {
		vrfOutput[i] = byte(i)
	}

	// Step 1: Compute leader value (32 bytes)
	leaderValue := VrfLeaderValue(vrfOutput)
	if len(leaderValue) != 32 {
		t.Fatalf("leaderValue should be 32 bytes, got %d", len(leaderValue))
	}

	// Step 2: Compute threshold for various stake ratios
	for _, stakePercent := range []uint64{1, 10, 25, 50, 75, 100} {
		poolStake := stakePercent * 10000
		totalStake := uint64(1000000)

		threshold := CertifiedNatThreshold(poolStake, totalStake, activeSlotCoeff)

		// Threshold should be positive for non-zero stake
		if threshold.Sign() <= 0 {
			t.Errorf("threshold should be positive for %d%% stake", stakePercent)
		}

		// Threshold bit length should be in 256-bit range
		if threshold.BitLen() > 256 {
			t.Errorf("threshold bit length %d exceeds 256 for %d%% stake", threshold.BitLen(), stakePercent)
		}

		// Step 3: Check if leader value is below threshold
		leaderValueInt := new(big.Int).SetBytes(leaderValue)
		isLeader := leaderValueInt.Cmp(threshold) < 0

		// Verify IsVRFOutputBelowThreshold matches manual comparison
		isLeaderFunc := IsVRFOutputBelowThreshold(vrfOutput, threshold)
		if isLeader != isLeaderFunc {
			t.Errorf("IsVRFOutputBelowThreshold result mismatch for %d%% stake", stakePercent)
		}
	}
}

// TestCPRAOSZeroOutputLeaderValueComparison tests that comparing a zero VRF output's
// leader value against a threshold works correctly without panics.
func TestCPRAOSZeroOutputLeaderValueComparison(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20)

	// VRF output of all zeros
	vrfOutput := make([]byte, 64)

	// Any pool with non-zero stake should have a positive threshold
	threshold := CertifiedNatThreshold(1, 1000000000000, activeSlotCoeff)
	require.True(t, threshold.Sign() > 0, "threshold should be positive for tiny but non-zero stake")

	// The leader value for zero VRF output (deterministic hash result)
	leaderValue := VrfLeaderValue(vrfOutput)
	require.Len(t, leaderValue, 32, "leader value should be 32 bytes")

	leaderValueInt := new(big.Int).SetBytes(leaderValue)

	// Verify the comparison works without panic and produces a deterministic result
	cmpResult := leaderValueInt.Cmp(threshold)
	// Run again to verify determinism
	require.Equal(t, cmpResult, leaderValueInt.Cmp(threshold), "comparison should be deterministic")
}

// TestCPRAOSHighStakeIncreasesEligibility verifies higher stake increases election probability.
func TestCPRAOSHighStakeIncreasesEligibility(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20)
	totalStake := uint64(1000000000)

	// Generate many VRF outputs and count how many are eligible at different stake levels
	// Note: This is a statistical test - we use a deterministic seed for reproducibility

	lowStakeThreshold := CertifiedNatThreshold(100000000, totalStake, activeSlotCoeff)  // 10%
	highStakeThreshold := CertifiedNatThreshold(500000000, totalStake, activeSlotCoeff) // 50%

	// Higher stake should have higher threshold (more likely to be eligible)
	if highStakeThreshold.Cmp(lowStakeThreshold) <= 0 {
		t.Error("higher stake should result in higher threshold")
	}

	// Count eligible outputs
	lowStakeEligible := 0
	highStakeEligible := 0
	numSamples := 100

	for i := 0; i < numSamples; i++ {
		vrfOutput := make([]byte, 64)
		// Deterministic pseudo-random data based on index
		for j := range vrfOutput {
			vrfOutput[j] = byte((i*j + i + j) % 256)
		}

		leaderValue := VrfLeaderValue(vrfOutput)
		leaderValueInt := new(big.Int).SetBytes(leaderValue)

		if leaderValueInt.Cmp(lowStakeThreshold) < 0 {
			lowStakeEligible++
		}
		if leaderValueInt.Cmp(highStakeThreshold) < 0 {
			highStakeEligible++
		}
	}

	// High stake should have at least as many eligible slots as low stake
	if highStakeEligible < lowStakeEligible {
		t.Errorf("high stake eligibility (%d) should be >= low stake eligibility (%d)",
			highStakeEligible, lowStakeEligible)
	}
}
