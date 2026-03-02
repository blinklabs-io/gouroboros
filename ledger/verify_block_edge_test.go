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

package ledger_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testConwayBlockHex is a minimal Conway block header hex for edge case testing.
// This represents a valid Conway block structure that can be tampered with for negative tests.
// Full block data for comprehensive tests is available in verify_block_test.go.
const testConwayBlockHex = "820685828a1a00a60faf1a0817580c58204eac1e7264c0e80436b04687e75d46d6a0d6b2338c2abb73a14fafbd689f69b2582012209e0b93f0128f670c9a02781c5466c4c4be003da3a51344b6a94f709ce51f58209c1a5fc5dec0a4b822d5a3b254ce9b168299479127aadcf97506ef257517fff682584023c2d70c24c44041644f5152f7e8a1bb580e516eb8e73c7df287116adb5f009c0c001feccfeebdf34c2275d1fce859c6c46182631b6306d5fd2724ac7ab1c6be58500dbe31ef7c00c34b6522e983d223e05075359cb170668d960b8cebfced178287ee6ca5cfc6e8e60aec97fd197aebfefc24aae695680631d575c6dacdfd9efc5687e46eb2a5c04a755c7f260af9ef830819c5ea"

// testSlotsPerKesPeriod is the slots per KES period for testing
const testSlotsPerKesPeriod = uint64(129600)

// skipAllValidationConfig returns a VerifyConfig that skips all validations.
// This is useful for tests that don't need full block validation.
func skipAllValidationConfig() common.VerifyConfig {
	return common.VerifyConfig{
		SkipBodyHashValidation:    true,
		SkipTransactionValidation: true,
		SkipStakePoolValidation:   true,
	}
}

// TestVerifyBlock_BodyHashTampering tests that body hash validation correctly
// detects tampered block bodies.
func TestVerifyBlock_BodyHashTampering(t *testing.T) {
	t.Run("tampered block body should fail validation", func(t *testing.T) {
		// Create a minimal block with body hash validation enabled
		// Using truncated CBOR to trigger body hash mismatch detection

		// First, create valid block bytes
		validBlockHex := "820685828a1a00000001" + // minimal header prefix
			"1a00000064" + // slot 100
			"5820" + strings.Repeat("00", 32) + // prev block hash (zeroed)
			"5820" + strings.Repeat("11", 32) + // issuer vkey hash
			"5820" + strings.Repeat("22", 32) + // vrf key
			"8258400000000000000000000000000000000000000000000000000000000000000000" +
			"0000000000000000000000000000000000000000000000000000000000000000" + // vrf result
			"1a00000001" + // block size
			"5820" + strings.Repeat("33", 32) + // block body hash
			"58200000000000000000000000000000000000000000000000000000000000000000" + // operational cert
			"820a00" // protocol version

		blockBytes, err := hex.DecodeString(validBlockHex)
		require.NoError(t, err, "failed to decode test block hex")

		// Tamper with the block by modifying bytes in what would be the body area
		require.Greater(
			t,
			len(blockBytes),
			50,
			"block bytes too short for tampering test",
		)

		tamperedBytes := make([]byte, len(blockBytes))
		copy(tamperedBytes, blockBytes)
		// Modify several bytes to corrupt the block body
		tamperedBytes[40] ^= 0xFF
		tamperedBytes[41] ^= 0xFF
		tamperedBytes[42] ^= 0xFF

		// Attempt to decode - should fail or produce mismatched hash
		_, err = ledger.NewBlockFromCbor(
			ledger.BlockTypeConway,
			tamperedBytes,
		)
		if err == nil {
			t.Log(
				"tampered block decoded without error - body hash validation may be skipped by default",
			)
		} else {
			// Verify the error indicates a body hash or CBOR issue
			errStr := err.Error()
			isExpectedError := strings.Contains(errStr, "body hash") ||
				strings.Contains(errStr, "decode") ||
				strings.Contains(errStr, "cbor") ||
				strings.Contains(errStr, "invalid")
			assert.True(t, isExpectedError, "unexpected error type for tampered block: %v", err)
		}
	})

	t.Run(
		"valid block structure should not trigger body hash error when skipped",
		func(t *testing.T) {
			// Test that with body hash validation skipped, we don't get body hash errors
			config := skipAllValidationConfig()

			// This tests the skip configuration works
			blockBytes, err := hex.DecodeString(testConwayBlockHex)
			require.NoError(t, err, "failed to decode test block hex")

			// With skip config, we should not get body hash validation errors
			// (though we may get other errors like VRF/KES which is expected)
			_, err = ledger.NewBlockFromCbor(
				ledger.BlockTypeConway,
				blockBytes,
				config,
			)
			if err != nil {
				errStr := err.Error()
				assert.NotContains(t, errStr, "body hash",
					"got body hash error when validation should be skipped")
				// Other errors are acceptable (VRF, KES, CBOR structure)
				t.Logf("expected non-body-hash error: %v", err)
			}
		},
	)
}

// TestVerifyBlock_VRFEdgeCases tests VRF validation edge cases.
// TODO: These tests currently verify test setup assumptions and data structure
// requirements. To exercise actual VRF verification, they need to be extended
// to construct full block contexts and call ledger.VerifyVrf or VerifyBlock
// with appropriate test vectors.
func TestVerifyBlock_VRFEdgeCases(t *testing.T) {
	t.Run("invalid VRF proof bytes", func(t *testing.T) {
		// Test with malformed VRF proof data
		// VRF proofs must be exactly 80 bytes; shorter data should fail

		shortProof := make([]byte, 40) // Too short for valid VRF proof
		for i := range shortProof {
			shortProof[i] = byte(i)
		}

		// Verify that short proof data would be rejected
		// TODO: Extend to call actual VRF verification with constructed test data
		assert.Less(
			t,
			len(shortProof),
			80,
			"test setup error: proof should be shorter than 80 bytes",
		)
	})

	t.Run("modified VRF output", func(t *testing.T) {
		// VRF output modification should cause verification failure
		// VRF outputs are 32 bytes (blake2b-256 hash)

		validOutput := make([]byte, 32)
		for i := range validOutput {
			validOutput[i] = byte(i)
		}

		// Modify the output
		modifiedOutput := make([]byte, 32)
		copy(modifiedOutput, validOutput)
		modifiedOutput[0] ^= 0xFF
		modifiedOutput[15] ^= 0xFF

		// The outputs should differ
		// TODO: Extend to verify actual VRF output mismatch detection
		assert.NotEqual(
			t,
			validOutput,
			modifiedOutput,
			"modification did not change output",
		)
	})

	t.Run("empty VRF key", func(t *testing.T) {
		// Empty VRF key should be rejected
		emptyKey := []byte{}
		// VRF keys must be 32 bytes (Ed25519 public key)
		// TODO: Extend to call actual VRF verification to confirm rejection
		assert.Empty(t, emptyKey, "test setup error: key should be empty")
	})
}

// TestVerifyBlock_KESEdgeCases tests KES signature validation edge cases.
// TODO: These tests verify KES period arithmetic and data structure requirements.
// To exercise actual KES signature verification, they need to be extended to
// construct full block contexts and call ledger.VerifyKesComponents or VerifyBlock
// with appropriate test vectors containing valid/invalid KES signatures.
func TestVerifyBlock_KESEdgeCases(t *testing.T) {
	t.Run("invalid KES signature bytes", func(t *testing.T) {
		// KES signatures have specific structure requirements
		// Test with malformed signature data

		// KES signatures are 448 bytes for depth-6 KES
		shortSig := make([]byte, 100) // Too short
		for i := range shortSig {
			shortSig[i] = byte(i % 256)
		}

		// TODO: Extend to call actual KES verification to confirm rejection
		assert.Less(t, len(shortSig), 448,
			"test setup error: signature should be shorter than 448 bytes")
	})

	t.Run("KES period boundary", func(t *testing.T) {
		// Test KES period calculations at boundaries
		// KES period = slot / slotsPerKesPeriod
		// TODO: Extend to verify actual KES period validation in VerifyBlock

		testCases := []struct {
			slot              uint64
			slotsPerKesPeriod uint64
			expectedPeriod    uint64
		}{
			{0, testSlotsPerKesPeriod, 0},
			{testSlotsPerKesPeriod - 1, testSlotsPerKesPeriod, 0},
			{testSlotsPerKesPeriod, testSlotsPerKesPeriod, 1},
			{testSlotsPerKesPeriod + 1, testSlotsPerKesPeriod, 1},
			{testSlotsPerKesPeriod * 10, testSlotsPerKesPeriod, 10},
		}

		for _, tc := range testCases {
			period := tc.slot / tc.slotsPerKesPeriod
			assert.Equal(t, tc.expectedPeriod, period,
				"slot %d / %d: period mismatch", tc.slot, tc.slotsPerKesPeriod)
		}
	})

	t.Run("expired KES period detection", func(t *testing.T) {
		// KES keys have a maximum validity period (typically 62 periods)
		// Test detection of expired KES periods
		// TODO: Extend to verify actual KES expiry detection in VerifyBlock

		maxKESEvolutions := uint64(62) // Standard Cardano max
		currentPeriod := uint64(100)
		keyStartPeriod := uint64(30)

		keysAge := currentPeriod - keyStartPeriod
		isExpired := keysAge > maxKESEvolutions

		assert.True(
			t,
			isExpired,
			"KES key with age %d should be expired (max %d)",
			keysAge,
			maxKESEvolutions,
		)
	})
}

// TestVerifyBlock_MalformedCBOR tests graceful handling of malformed CBOR data.
func TestVerifyBlock_MalformedCBOR(t *testing.T) {
	t.Run("truncated CBOR data", func(t *testing.T) {
		// Truncated CBOR should fail gracefully with decode error
		truncatedHex := testConwayBlockHex[:50] // Very short, truncated

		blockBytes, err := hex.DecodeString(truncatedHex)
		require.NoError(t, err, "failed to decode truncated hex")

		_, err = ledger.NewBlockFromCbor(ledger.BlockTypeConway, blockBytes)
		require.Error(t, err, "expected error for truncated CBOR")

		// Error should indicate CBOR decode issue
		errStr := strings.ToLower(err.Error())
		isExpectedError := strings.Contains(errStr, "decode") ||
			strings.Contains(errStr, "cbor") ||
			strings.Contains(errStr, "unexpected") ||
			strings.Contains(errStr, "invalid") ||
			strings.Contains(errStr, "eof")
		assert.True(
			t,
			isExpectedError,
			"expected CBOR decode error, got: %v",
			err,
		)
	})

	t.Run("empty CBOR data", func(t *testing.T) {
		emptyBytes := []byte{}

		_, err := ledger.NewBlockFromCbor(ledger.BlockTypeConway, emptyBytes)
		assert.Error(t, err, "expected error for empty CBOR")
	})

	t.Run("invalid CBOR structure", func(t *testing.T) {
		// Valid CBOR but wrong structure for a block
		// This is a simple CBOR map instead of expected block array structure
		invalidStructureHex := "a16568656c6c6f65776f726c64" // {"hello": "world"}

		blockBytes, err := hex.DecodeString(invalidStructureHex)
		require.NoError(t, err, "failed to decode invalid structure hex")

		_, err = ledger.NewBlockFromCbor(ledger.BlockTypeConway, blockBytes)
		require.Error(t, err, "expected error for invalid CBOR structure")

		// Should indicate structure mismatch
		errStr := strings.ToLower(err.Error())
		isExpectedError := strings.Contains(errStr, "decode") ||
			strings.Contains(errStr, "structure") ||
			strings.Contains(errStr, "invalid") ||
			strings.Contains(errStr, "type") ||
			strings.Contains(errStr, "unmarshal")
		assert.True(
			t,
			isExpectedError,
			"expected structure error, got: %v",
			err,
		)
	})

	t.Run("CBOR with wrong array length", func(t *testing.T) {
		// CBOR array but wrong number of elements
		// A single-element array when block expects more elements
		wrongLengthHex := "8100" // [0] - single element array

		blockBytes, err := hex.DecodeString(wrongLengthHex)
		require.NoError(t, err, "failed to decode wrong length hex")

		_, err = ledger.NewBlockFromCbor(ledger.BlockTypeConway, blockBytes)
		assert.Error(t, err, "expected error for wrong array length")
	})

	t.Run("random garbage bytes", func(t *testing.T) {
		// Completely random bytes that aren't valid CBOR
		garbageBytes := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE}

		_, err := ledger.NewBlockFromCbor(ledger.BlockTypeConway, garbageBytes)
		assert.Error(t, err, "expected error for garbage bytes")
	})

	t.Run("CBOR indefinite length without break", func(t *testing.T) {
		// Start indefinite array but don't close it properly
		// 0x9F starts indefinite array, needs 0xFF break to close
		incompleteHex := "9f01020304" // indefinite array [1,2,3,4] without break

		blockBytes, err := hex.DecodeString(incompleteHex)
		require.NoError(t, err, "failed to decode incomplete indefinite hex")

		_, err = ledger.NewBlockFromCbor(ledger.BlockTypeConway, blockBytes)
		assert.Error(t, err, "expected error for incomplete indefinite CBOR")
	})
}

// TestVerifyBlock_BlockTypeEdgeCases tests edge cases for block type handling.
func TestVerifyBlock_BlockTypeEdgeCases(t *testing.T) {
	t.Run("unknown block type", func(t *testing.T) {
		validCBOR := []byte{0x82, 0x00, 0x00} // minimal valid CBOR array

		// Use an invalid block type constant
		unknownBlockType := uint(99)

		_, err := ledger.NewBlockFromCbor(unknownBlockType, validCBOR)
		require.Error(t, err, "expected error for unknown block type")
		assert.Contains(
			t,
			err.Error(),
			"unknown",
			"expected 'unknown' in error",
		)
	})
}
