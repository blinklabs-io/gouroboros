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

	// With 100% stake and f=0.05, threshold should be approximately 0.05 * 2^512
	// The threshold should be positive and substantial
	if threshold.Sign() <= 0 {
		t.Error("expected positive threshold for full stake")
	}

	// Check it's in the right ballpark: should be roughly 5% of 2^512
	// 2^512 is a 513-bit number, 5% of it should be around 509-510 bits
	bitLen := threshold.BitLen()
	if bitLen < 500 || bitLen > 512 {
		t.Errorf(
			"threshold bit length %d not in expected range [500, 512]",
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
	threshold := big.NewInt(1000000)

	// Output below threshold
	lowOutput := make([]byte, 64)
	lowOutput[63] = 0x01 // value = 1
	if !IsVRFOutputBelowThreshold(lowOutput, threshold) {
		t.Error("output 1 should be below threshold 1000000")
	}

	// Output above threshold (very large value)
	highOutput := make([]byte, 64)
	for i := range highOutput {
		highOutput[i] = 0xFF
	}
	if IsVRFOutputBelowThreshold(highOutput, threshold) {
		t.Error("maximum output should not be below small threshold")
	}

	// Output exactly at threshold (should not be below)
	exactOutput := make([]byte, 64)
	thresholdBytes := threshold.Bytes()
	copy(exactOutput[64-len(thresholdBytes):], thresholdBytes)
	if IsVRFOutputBelowThreshold(exactOutput, threshold) {
		t.Error("output equal to threshold should not be below")
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

func TestZeroOutputAlwaysEligible(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20)

	// Any pool with non-zero stake should be eligible with zero VRF output
	zeroOutput := make([]byte, 64)

	eligible := IsSlotLeaderFromComponents(
		zeroOutput,
		1,          // minimal stake
		1000000000, // large total
		activeSlotCoeff,
	)

	if !eligible {
		t.Error(
			"zero VRF output should always be eligible for any non-zero stake",
		)
	}
}

func TestMaxOutputNeverEligible(t *testing.T) {
	activeSlotCoeff := big.NewRat(1, 20)

	// Maximum VRF output (all 0xFF bytes)
	maxOutput := make([]byte, 64)
	for i := range maxOutput {
		maxOutput[i] = 0xFF
	}

	// Even with 100% stake, max output should not be eligible
	// because threshold is 1 - (1-f)^1 = f ≈ 0.05, and max output / 2^512 = ~1
	eligible := IsSlotLeaderFromComponents(
		maxOutput,
		1000000000,
		1000000000, // 100% stake
		activeSlotCoeff,
	)

	if eligible {
		t.Error(
			"maximum VRF output should not be eligible even with 100% stake",
		)
	}
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
