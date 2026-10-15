// Copyright 2026 Blink Labs Software
// SPDX-License-Identifier: Apache-2.0

package consensus

import (
	"bytes"
	"math/big"
	"testing"

	"golang.org/x/crypto/blake2b"
)

func TestLeaderEligibilityUtilityVRFOutputToInt(t *testing.T) {
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
func TestLeaderEligibilityUtilityVRFOutputConversionRoundTrip(t *testing.T) {
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
func TestLeaderEligibilityUtilityVRFOutputOrder(t *testing.T) {
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
func TestLeaderEligibilityUtilityVrfLeaderValueReturns32Bytes(t *testing.T) {
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
				t.Errorf(
					"VrfLeaderValue should return 32 bytes, got %d",
					len(result),
				)
			}
		})
	}
}
func TestLeaderEligibilityUtilityVrfLeaderValueAppliesLPrefix(t *testing.T) {
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
		t.Errorf(
			"VrfLeaderValue did not apply 'L' (0x4C) prefix correctly\nexpected: %x\ngot: %x",
			expected,
			result,
		)
	}
}
func TestLeaderEligibilityUtilityVrfLeaderValueKnownVector(t *testing.T) {
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
		t.Errorf(
			"VrfLeaderValue failed for known vector\nexpected: %x\ngot: %x",
			expected,
			result,
		)
	}

	// The result should be deterministic
	result2 := VrfLeaderValue(zeroInput)
	if !bytes.Equal(result, result2) {
		t.Error("VrfLeaderValue should be deterministic")
	}
}
func TestLeaderEligibilityUtilityVrfLeaderValueDifferentInputs(t *testing.T) {
	input1 := make([]byte, 64)
	input2 := make([]byte, 64)
	input2[0] = 0x01 // Single bit difference

	result1 := VrfLeaderValue(input1)
	result2 := VrfLeaderValue(input2)

	if bytes.Equal(result1, result2) {
		t.Error(
			"different inputs should produce different VrfLeaderValue outputs",
		)
	}
}
func TestLeaderEligibilityUtilityVrfLeaderValueEmptyInput(t *testing.T) {
	// Empty input should still work - BLAKE2b-256(0x4C)
	result := VrfLeaderValue(nil)
	if len(result) != 32 {
		t.Errorf(
			"VrfLeaderValue(nil) should return 32 bytes, got %d",
			len(result),
		)
	}

	// Also test empty slice
	result2 := VrfLeaderValue([]byte{})
	if len(result2) != 32 {
		t.Errorf(
			"VrfLeaderValue(empty) should return 32 bytes, got %d",
			len(result2),
		)
	}

	// nil and empty slice should produce the same result
	if !bytes.Equal(result, result2) {
		t.Error("VrfLeaderValue(nil) and VrfLeaderValue(empty) should be equal")
	}
}
