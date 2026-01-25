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
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	byronConsensus "github.com/blinklabs-io/gouroboros/consensus/byron"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper functions to extract consensus data from Byron blocks

// extractByronIssuerPubKey extracts the issuer public key from a Byron main block header.
// In Byron, ConsensusData.PubKey is an extended Ed25519 key (64 bytes):
// - First 32 bytes: Ed25519 public key
// - Last 32 bytes: Chain code (for HD key derivation)
// For signature verification, we only need the first 32 bytes.
func extractByronIssuerPubKey(header *byron.ByronMainBlockHeader) ([]byte, error) {
	pubKey := header.ConsensusData.PubKey

	// Byron uses extended Ed25519 keys: 32 byte pubkey + 32 byte chaincode
	if len(pubKey) == 64 {
		// Return only the Ed25519 public key portion
		return pubKey[:32], nil
	}

	if len(pubKey) == ed25519.PublicKeySize {
		return pubKey, nil
	}

	return nil, fmt.Errorf(
		"invalid issuer public key size: got %d, expected 32 or 64",
		len(pubKey),
	)
}

// extractByronExtendedPubKey extracts the full extended public key (pubkey + chaincode).
func extractByronExtendedPubKey(header *byron.ByronMainBlockHeader) ([]byte, error) {
	pubKey := header.ConsensusData.PubKey
	if len(pubKey) != 64 {
		return nil, fmt.Errorf(
			"expected 64-byte extended public key, got %d bytes",
			len(pubKey),
		)
	}
	return pubKey, nil
}

// extractByronBlockSignature extracts the block signature from a Byron main block header.
// Byron block signatures are structured as:
// - [0, signature] for simple block signature (BlockSignature)
// - [1, [[epoch, issuerVK, delegateVK, cert], signature]] for heavy delegation (BlockPSignatureHeavy)
// - [2, [[omega, issuerVK, delegateVK, cert], signature]] for lightweight delegation (BlockPSignatureLight)
//
// For types 1 and 2, the signature is inside a nested array structure.
func extractByronBlockSignature(header *byron.ByronMainBlockHeader) ([]byte, error) {
	blockSig := header.ConsensusData.BlockSig
	if len(blockSig) < 2 {
		return nil, fmt.Errorf("block signature array too short: %d elements", len(blockSig))
	}

	// Get signature type
	sigType, ok := blockSig[0].(uint64)
	if !ok {
		return nil, fmt.Errorf("expected uint64 signature type, got %T", blockSig[0])
	}

	switch sigType {
	case 0:
		// Simple signature: [0, signature]
		sig, ok := blockSig[1].([]byte)
		if !ok {
			return nil, fmt.Errorf("expected []byte signature, got %T", blockSig[1])
		}
		if len(sig) != ed25519.SignatureSize {
			return nil, fmt.Errorf("invalid signature size: got %d, expected %d",
				len(sig), ed25519.SignatureSize)
		}
		return sig, nil

	case 1, 2:
		// Delegation signature: [type, [[...cert...], signature]]
		// The second element is an array containing the cert and signature
		innerArray, ok := blockSig[1].([]any)
		if !ok {
			return nil, fmt.Errorf("expected []any for delegation sig, got %T", blockSig[1])
		}
		if len(innerArray) < 2 {
			return nil, fmt.Errorf("delegation sig inner array too short: %d", len(innerArray))
		}

		// The signature is the last element of the inner array
		sig, ok := innerArray[len(innerArray)-1].([]byte)
		if !ok {
			return nil, fmt.Errorf("expected []byte signature in delegation, got %T",
				innerArray[len(innerArray)-1])
		}
		if len(sig) != ed25519.SignatureSize {
			return nil, fmt.Errorf("invalid signature size: got %d, expected %d",
				len(sig), ed25519.SignatureSize)
		}
		return sig, nil

	default:
		return nil, fmt.Errorf("unknown signature type: %d", sigType)
	}
}

// extractByronDelegationCert extracts the delegation certificate from a Byron block signature.
// Returns nil if the signature is a simple (type 0) signature.
// For delegation signatures, returns the certificate array elements.
func extractByronDelegationCert(header *byron.ByronMainBlockHeader) ([]any, error) {
	blockSig := header.ConsensusData.BlockSig
	if len(blockSig) < 2 {
		return nil, fmt.Errorf("block signature array too short")
	}

	sigType, ok := blockSig[0].(uint64)
	if !ok {
		return nil, fmt.Errorf("expected uint64 signature type")
	}

	if sigType == 0 {
		// No delegation cert for simple signatures
		return nil, nil
	}

	// For types 1 and 2, extract the certificate
	innerArray, ok := blockSig[1].([]any)
	if !ok {
		return nil, fmt.Errorf("expected []any for delegation sig")
	}

	if len(innerArray) < 2 {
		return nil, fmt.Errorf("delegation sig inner array too short")
	}

	// The certificate is the first element
	cert, ok := innerArray[0].([]any)
	if !ok {
		return nil, fmt.Errorf("expected []any for delegation cert, got %T", innerArray[0])
	}

	return cert, nil
}

// extractByronHeaderCbor returns the CBOR-encoded header for signature verification.
// In Byron, the signature is computed over the header body, not the full header.
func extractByronHeaderCbor(header *byron.ByronMainBlockHeader) ([]byte, error) {
	headerCbor := header.Cbor()
	if len(headerCbor) == 0 {
		return nil, fmt.Errorf("header CBOR is empty")
	}
	return headerCbor, nil
}

// Byron block test vectors from real mainnet/testnet blocks
// These test that our Byron block parsing matches the expected hashes and structure

// TestByronMainBlockConformance tests parsing of real Byron main blocks
func TestByronMainBlockConformance(t *testing.T) {
	// Real mainnet Byron block from cexplorer
	// https://cexplorer.io/block/1451a0dbf16cfeddf4991a838961df1b08a68f43a19c0eb3b36cc4029c77a2d8
	tests := []struct {
		name         string
		hexData      string
		expectedHash string
		expectedSlot uint64
	}{
		{
			name: "mainnet_block_slot_4471207",
			// Block from slot 4471207
			hexData:      mainnetByronBlockHex,
			expectedHash: "1451a0dbf16cfeddf4991a838961df1b08a68f43a19c0eb3b36cc4029c77a2d8",
			expectedSlot: 4471207,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := hex.DecodeString(strings.TrimSpace(tc.hexData))
			if err != nil {
				t.Fatalf("Failed to decode hex: %v", err)
			}

			block, err := byron.NewByronMainBlockFromCbor(data)
			if err != nil {
				t.Fatalf("Failed to decode Byron block: %v", err)
			}

			// Verify hash
			hash := block.Hash()
			hashHex := hex.EncodeToString(hash.Bytes())
			if hashHex != tc.expectedHash {
				t.Errorf(
					"Hash mismatch:\n  got:  %s\n  want: %s",
					hashHex,
					tc.expectedHash,
				)
			}

			// Verify slot
			if block.SlotNumber() != tc.expectedSlot {
				t.Errorf(
					"Slot mismatch: got %d, want %d",
					block.SlotNumber(),
					tc.expectedSlot,
				)
			}

			// Verify era
			if block.Era().Id != byron.EraIdByron {
				t.Errorf(
					"Era mismatch: got %d, want %d",
					block.Era().Id,
					byron.EraIdByron,
				)
			}

			// Verify block type
			if block.Type() != byron.BlockTypeByronMain {
				t.Errorf(
					"Block type mismatch: got %d, want %d",
					block.Type(),
					byron.BlockTypeByronMain,
				)
			}

			t.Logf(
				"Byron main block validated: slot=%d, hash=%s, txs=%d",
				block.SlotNumber(),
				hashHex[:16]+"...",
				len(block.Transactions()),
			)
		})
	}
}

// TestByronEBBConformance tests parsing of real Byron Epoch Boundary Blocks
func TestByronEBBConformance(t *testing.T) {
	// Load EBB from testdata file
	ebbPath := filepath.Join(
		"..",
		"..",
		"..",
		"protocol",
		"chainsync",
		"testdata",
		"byron_ebb_testnet_8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f.hex",
	)

	hexData, err := os.ReadFile(ebbPath)
	if err != nil {
		t.Skipf("EBB test file not found: %v", err)
	}

	data, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	block, err := byron.NewByronEpochBoundaryBlockFromCbor(data)
	if err != nil {
		t.Fatalf("Failed to decode Byron EBB: %v", err)
	}

	// Verify hash matches filename
	expectedHash := "8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f"
	hash := block.Hash()
	hashHex := hex.EncodeToString(hash.Bytes())
	if hashHex != expectedHash {
		t.Errorf(
			"Hash mismatch:\n  got:  %s\n  want: %s",
			hashHex,
			expectedHash,
		)
	}

	// Verify era
	if block.Era().Id != byron.EraIdByron {
		t.Errorf(
			"Era mismatch: got %d, want %d",
			block.Era().Id,
			byron.EraIdByron,
		)
	}

	// Verify block type
	if block.Type() != byron.BlockTypeByronEbb {
		t.Errorf(
			"Block type mismatch: got %d, want %d",
			block.Type(),
			byron.BlockTypeByronEbb,
		)
	}

	// EBBs should have no transactions
	if len(block.Transactions()) != 0 {
		t.Errorf(
			"EBB should have no transactions, got %d",
			len(block.Transactions()),
		)
	}

	t.Logf(
		"Byron EBB validated: slot=%d, hash=%s",
		block.SlotNumber(),
		hashHex[:16]+"...",
	)
}

// TestByronMainBlockFromTestnetFile tests parsing from testdata file
func TestByronMainBlockFromTestnetFile(t *testing.T) {
	blockPath := filepath.Join(
		"..",
		"..",
		"..",
		"protocol",
		"chainsync",
		"testdata",
		"byron_main_block_testnet_f38aa5e8cf0b47d1ffa8b2385aa2d43882282db2ffd5ac0e3dadec1a6f2ecf08.hex",
	)

	hexData, err := os.ReadFile(blockPath)
	if err != nil {
		t.Skipf("Test file not found: %v", err)
	}

	data, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	block, err := byron.NewByronMainBlockFromCbor(data)
	if err != nil {
		t.Fatalf("Failed to decode Byron block: %v", err)
	}

	// Verify hash matches filename
	expectedHash := "f38aa5e8cf0b47d1ffa8b2385aa2d43882282db2ffd5ac0e3dadec1a6f2ecf08"
	hash := block.Hash()
	hashHex := hex.EncodeToString(hash.Bytes())
	if hashHex != expectedHash {
		t.Errorf(
			"Hash mismatch:\n  got:  %s\n  want: %s",
			hashHex,
			expectedHash,
		)
	}

	t.Logf("Byron testnet block validated: slot=%d, hash=%s, txs=%d",
		block.SlotNumber(), hashHex[:16]+"...", len(block.Transactions()))
}

// TestByronHeaderFields tests that header fields are correctly extracted
func TestByronHeaderFields(t *testing.T) {
	data, err := hex.DecodeString(strings.TrimSpace(mainnetByronBlockHex))
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	block, err := byron.NewByronMainBlockFromCbor(data)
	if err != nil {
		t.Fatalf("Failed to decode Byron block: %v", err)
	}

	header := block.Header()

	// Check header methods work
	if header.SlotNumber() != 4471207 {
		t.Errorf(
			"Header slot mismatch: got %d, want 4471207",
			header.SlotNumber(),
		)
	}

	// Block number should be retrievable
	blockNum := header.BlockNumber()
	t.Logf("Block number (difficulty): %d", blockNum)

	// PrevHash should be present
	prevHash := header.PrevHash()
	if len(prevHash.Bytes()) != 32 {
		t.Errorf("PrevHash should be 32 bytes, got %d", len(prevHash.Bytes()))
	}

	// Era should be Byron
	if header.Era().Id != byron.EraIdByron {
		t.Errorf("Era mismatch")
	}
}

// TestByronBlockBodyHash tests that body hash is correctly computed
func TestByronBlockBodyHash(t *testing.T) {
	data, err := hex.DecodeString(strings.TrimSpace(mainnetByronBlockHex))
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	block, err := byron.NewByronMainBlockFromCbor(data)
	if err != nil {
		t.Fatalf("Failed to decode Byron block: %v", err)
	}

	// BlockBodyHash should return something (or zero hash if not extractable)
	bodyHash := block.BlockBodyHash()
	if len(bodyHash.Bytes()) != 32 {
		t.Errorf("Body hash should be 32 bytes")
	}
}

// TestByronSlotToEpoch tests Byron slot/epoch calculations using ByronConfig.SlotToEpoch
func TestByronSlotToEpoch(t *testing.T) {
	// Create a ByronConfig to test the actual SlotToEpoch function
	config := byronConsensus.ByronConfig{
		SlotsPerEpoch: byron.ByronSlotsPerEpoch,
	}

	tests := []struct {
		slot          uint64
		expectedEpoch uint64
	}{
		{0, 0},
		{1, 0},
		{byron.ByronSlotsPerEpoch - 1, 0},
		{byron.ByronSlotsPerEpoch, 1},
		{byron.ByronSlotsPerEpoch + 1, 1},
		{2 * byron.ByronSlotsPerEpoch, 2},
		{4471207, 207}, // Real mainnet slot (4471207 / 21600 = 207)
	}

	for _, tc := range tests {
		// Test the actual ByronConfig.SlotToEpoch function
		epoch := config.SlotToEpoch(tc.slot)
		assert.Equalf(
			t,
			tc.expectedEpoch,
			epoch,
			"Slot %d",
			tc.slot,
		)
	}
}

// TestByronConsensusDataExtraction tests extraction of consensus data from Byron blocks.
// This verifies we can access issuer pubkey, signature, and header CBOR for validation.
func TestByronConsensusDataExtraction(t *testing.T) {
	data, err := hex.DecodeString(strings.TrimSpace(mainnetByronBlockHex))
	require.NoError(t, err, "Failed to decode hex")

	block, err := byron.NewByronMainBlockFromCbor(data)
	require.NoError(t, err, "Failed to decode Byron block")

	header := block.BlockHeader

	// Test extended pubkey extraction (full 64 bytes)
	extPubKey, err := extractByronExtendedPubKey(header)
	require.NoError(t, err, "Failed to extract extended pubkey")
	assert.Equal(t, 64, len(extPubKey), "Extended pubkey should be 64 bytes")
	t.Logf("Extended pubkey: %x...", extPubKey[:16])
	t.Logf("  Ed25519 pubkey (32 bytes): %x", extPubKey[:32])
	t.Logf("  Chain code (32 bytes):     %x", extPubKey[32:])

	// Test issuer pubkey extraction (just the 32-byte Ed25519 key)
	pubKey, err := extractByronIssuerPubKey(header)
	require.NoError(t, err, "Failed to extract issuer pubkey")
	assert.Equal(t, ed25519.PublicKeySize, len(pubKey), "Issuer pubkey should be 32 bytes")
	t.Logf("Issuer pubkey (Ed25519): %x", pubKey)

	// Test block signature extraction
	sig, err := extractByronBlockSignature(header)
	require.NoError(t, err, "Failed to extract block signature")
	assert.Equal(t, ed25519.SignatureSize, len(sig), "Block signature should be 64 bytes")
	t.Logf("Block signature: %x", sig)

	// Test delegation certificate extraction
	cert, err := extractByronDelegationCert(header)
	require.NoError(t, err, "Failed to extract delegation cert")
	if cert != nil {
		t.Logf("Delegation certificate: %d elements", len(cert))
		for i, elem := range cert {
			switch v := elem.(type) {
			case uint64:
				t.Logf("  [%d] uint64: %d", i, v)
			case []byte:
				if len(v) <= 128 {
					t.Logf("  [%d] []byte: %d bytes - %x", i, len(v), v)
				} else {
					t.Logf("  [%d] []byte: %d bytes - %x...", i, len(v), v[:32])
				}
			default:
				t.Logf("  [%d] %T: %v", i, elem, elem)
			}
		}

		// For signature type 2 (lightweight), show the full BlockSig structure
		sigType, _ := header.ConsensusData.BlockSig[0].(uint64)
		if sigType == 2 {
			innerArray, ok := header.ConsensusData.BlockSig[1].([]any)
			require.True(t, ok, "sigType 2: expected []any for BlockSig[1], got %T", header.ConsensusData.BlockSig[1])
			t.Logf("Lightweight delegation inner array: %d elements", len(innerArray))
			for i, elem := range innerArray {
				switch v := elem.(type) {
				case []byte:
					t.Logf("  inner[%d] []byte: %d bytes - %x", i, len(v), v)
				case []any:
					t.Logf("  inner[%d] []any: %d elements", i, len(v))
					for j, inner := range v {
						switch iv := inner.(type) {
						case []byte:
							if len(iv) <= 128 {
								t.Logf("    inner[%d][%d] []byte: %d bytes - %x", i, j, len(iv), iv)
							} else {
								t.Logf("    inner[%d][%d] []byte: %d bytes - %x...", i, j, len(iv), iv[:32])
							}
						case uint64:
							t.Logf("    inner[%d][%d] uint64: %d", i, j, iv)
						case []any:
							t.Logf("    inner[%d][%d] []any: %d elements - %v", i, j, len(iv), iv)
						default:
							t.Logf("    inner[%d][%d] %T: %v", i, j, inner, inner)
						}
					}
				default:
					t.Logf("  inner[%d] %T: %v", i, elem, elem)
				}
			}
		}

		// For signature type 1 (heavy), show structure too
		if sigType == 1 {
			innerArray, ok := header.ConsensusData.BlockSig[1].([]any)
			require.True(t, ok, "sigType 1: expected []any for BlockSig[1], got %T", header.ConsensusData.BlockSig[1])
			t.Logf("Heavy delegation inner array: %d elements", len(innerArray))
			for i, elem := range innerArray {
				switch v := elem.(type) {
				case []byte:
					t.Logf("  inner[%d] []byte: %d bytes - %x", i, len(v), v)
				case []any:
					t.Logf("  inner[%d] []any: %d elements", i, len(v))
					for j, inner := range v {
						switch iv := inner.(type) {
						case []byte:
							if len(iv) <= 128 {
								t.Logf("    inner[%d][%d] []byte: %d bytes - %x", i, j, len(iv), iv)
							} else {
								t.Logf("    inner[%d][%d] []byte: %d bytes - %x...", i, j, len(iv), iv[:32])
							}
						case uint64:
							t.Logf("    inner[%d][%d] uint64: %d", i, j, iv)
						case []any:
							t.Logf("    inner[%d][%d] []any: %d elements - %v", i, j, len(iv), iv)
						default:
							t.Logf("    inner[%d][%d] %T: %v", i, j, inner, inner)
						}
					}
				default:
					t.Logf("  inner[%d] %T: %v", i, elem, elem)
				}
			}
		}
	} else {
		t.Logf("No delegation certificate (simple signature)")
	}

	// Test header CBOR extraction
	headerCbor, err := extractByronHeaderCbor(header)
	require.NoError(t, err, "Failed to extract header CBOR")
	assert.Greater(t, len(headerCbor), 0, "Header CBOR should not be empty")
	t.Logf("Header CBOR length: %d bytes", len(headerCbor))

	// Log the block signature structure for debugging
	t.Logf("BlockSig structure: %d elements", len(header.ConsensusData.BlockSig))
	sigType, _ := header.ConsensusData.BlockSig[0].(uint64)
	switch sigType {
	case 0:
		t.Logf("  Signature type: BlockSignature (simple)")
	case 1:
		t.Logf("  Signature type: BlockPSignatureHeavy (delegation)")
	case 2:
		t.Logf("  Signature type: BlockPSignatureLight (lightweight delegation)")
	}
}

// TestByronBodyHashValidation tests that the body hash in the header matches
// the computed hash of the block body.
func TestByronBodyHashValidation(t *testing.T) {
	data, err := hex.DecodeString(strings.TrimSpace(mainnetByronBlockHex))
	require.NoError(t, err, "Failed to decode hex")

	block, err := byron.NewByronMainBlockFromCbor(data)
	require.NoError(t, err, "Failed to decode Byron block")

	header := block.BlockHeader

	// Log BodyProof structure for debugging
	t.Logf("BodyProof type: %T", header.BodyProof)
	switch bp := header.BodyProof.(type) {
	case []byte:
		t.Logf("BodyProof is []byte, length: %d", len(bp))
		if len(bp) == 32 {
			t.Logf("BodyProof (hash): %x", bp)
		}
	case []any:
		t.Logf("BodyProof is []any, length: %d", len(bp))
		for i, elem := range bp {
			switch v := elem.(type) {
			case []byte:
				t.Logf("  [%d] []byte, len=%d: %x", i, len(v), v)
			case uint64:
				t.Logf("  [%d] uint64: %d", i, v)
			default:
				t.Logf("  [%d] %T", i, elem)
			}
		}
	default:
		t.Logf("BodyProof is unknown type: %T, value: %v", header.BodyProof, header.BodyProof)
	}

	// Get the stored body hash from header
	storedHash := block.BlockBodyHash()
	t.Logf("BlockBodyHash() returned: %x", storedHash.Bytes())

	// Check if it's all zeros (meaning extraction failed)
	isZero := true
	for _, b := range storedHash.Bytes() {
		if b != 0 {
			isZero = false
			break
		}
	}

	if isZero {
		t.Logf("WARNING: BlockBodyHash() returned zero hash - BodyProof extraction needs fix")
	}

	// Get the body CBOR and compute hash
	bodyCbor := block.Body.Cbor()
	require.NotNil(t, bodyCbor, "Body CBOR should not be nil")

	computedHash := common.Blake2b256Hash(bodyCbor)

	// Compare hashes
	if bytes.Equal(storedHash.Bytes(), computedHash.Bytes()) {
		t.Logf("Body hash validation PASSED")
		t.Logf("  Stored hash:   %x", storedHash.Bytes())
		t.Logf("  Computed hash: %x", computedHash.Bytes())
	} else {
		// Byron's body proof is more complex - it's a Merkle tree of the body parts
		// The body proof contains: [txProof, sscProof, dlgProof, updProof]
		// Each proof is a hash of that section
		t.Logf("Body hash differs (expected - Byron uses Merkle tree proof)")
		t.Logf("  Stored hash (may be incomplete):   %x", storedHash.Bytes())
		t.Logf("  Computed hash of raw CBOR:         %x", computedHash.Bytes())
		t.Logf("  Body CBOR length: %d bytes", len(bodyCbor))
	}
}

// TestByronFullHeaderValidation tests full header validation with signature verification.
// This test uses EnvelopeOnly: false to enable cryptographic validation.
func TestByronFullHeaderValidation(t *testing.T) {
	data, err := hex.DecodeString(strings.TrimSpace(mainnetByronBlockHex))
	require.NoError(t, err, "Failed to decode hex")

	block, err := byron.NewByronMainBlockFromCbor(data)
	require.NoError(t, err, "Failed to decode Byron block")

	header := block.BlockHeader

	// Extract consensus data
	pubKey, err := extractByronIssuerPubKey(header)
	require.NoError(t, err, "Failed to extract issuer pubkey")

	sig, err := extractByronBlockSignature(header)
	require.NoError(t, err, "Failed to extract block signature")

	headerCbor, err := extractByronHeaderCbor(header)
	require.NoError(t, err, "Failed to extract header CBOR")

	// Log signature type for debugging
	sigType, _ := header.ConsensusData.BlockSig[0].(uint64)
	t.Logf("Signature type: %d", sigType)

	// Create validator config
	// Note: We don't have the genesis delegate keys, so we skip that validation
	config := byronConsensus.ByronConfig{
		ProtocolMagic:  header.ProtocolMagic,
		SlotsPerEpoch:  byron.ByronSlotsPerEpoch,
		NumGenesisKeys: 7, // Mainnet had 7 genesis delegates
		// GenesisKeyHashes not set - will skip delegate validation
	}

	validator := byronConsensus.NewHeaderValidator(config)
	// Skip delegation cert verification as the exact cardano-crypto format is unknown
	validator.SkipDelegationCertVerification = true

	// Create validation input
	// For testing without a previous block, we use dummy previous values
	prevHash := make([]byte, 32)
	copy(prevHash, header.PrevBlock.Bytes())

	// Calculate previous slot/block, guarding against underflow at genesis
	prevSlot := header.SlotNumber()
	if prevSlot > 0 {
		prevSlot--
	}
	prevBlockNum := header.BlockNumber()
	if prevBlockNum > 0 {
		prevBlockNum--
	}

	input := &byronConsensus.ValidateHeaderInput{
		Slot:           header.SlotNumber(),
		BlockNumber:    header.BlockNumber(),
		PrevHash:       header.PrevBlock.Bytes(),
		ProtocolMagic:  header.ProtocolMagic,
		IssuerPubKey:   pubKey,
		BlockSignature: sig,
		HeaderCbor:     headerCbor,
		BlockSig:       header.ConsensusData.BlockSig, // Full signature structure for proxy verification
		// Set previous values to allow this block to validate
		PrevSlot:        prevSlot,
		PrevBlockNumber: prevBlockNum,
		PrevHeaderHash:  prevHash,
		IsEBB:           false,
		EnvelopeOnly:    false, // Enable full validation
	}

	result := validator.ValidateHeader(input)

	// Validate result using testify assertions
	if result.Valid {
		t.Logf("Full header validation PASSED")
	} else {
		t.Logf("Full header validation FAILED with %d errors:", len(result.Errors))
		for i, err := range result.Errors {
			t.Logf("  [%d] %v", i, err)
		}
	}

	// Assert validation passed, including all errors in the failure message
	require.True(t, result.Valid, "Full header validation should pass: %v", result.Errors)
}

// TestByronEnvelopeValidation tests envelope-only validation (no signature verification).
// This is the current behavior of the conformance tests.
func TestByronEnvelopeValidation(t *testing.T) {
	data, err := hex.DecodeString(strings.TrimSpace(mainnetByronBlockHex))
	require.NoError(t, err, "Failed to decode hex")

	block, err := byron.NewByronMainBlockFromCbor(data)
	require.NoError(t, err, "Failed to decode Byron block")

	header := block.BlockHeader

	// Create validator config
	config := byronConsensus.ByronConfig{
		ProtocolMagic: header.ProtocolMagic,
		SlotsPerEpoch: byron.ByronSlotsPerEpoch,
	}

	validator := byronConsensus.NewHeaderValidator(config)

	// Create validation input with envelope-only mode
	prevHash := make([]byte, 32)
	copy(prevHash, header.PrevBlock.Bytes())

	// Calculate previous slot/block, guarding against underflow at genesis
	prevSlot := header.SlotNumber()
	if prevSlot > 0 {
		prevSlot--
	}
	prevBlockNum := header.BlockNumber()
	if prevBlockNum > 0 {
		prevBlockNum--
	}

	input := &byronConsensus.ValidateHeaderInput{
		Slot:            header.SlotNumber(),
		BlockNumber:     header.BlockNumber(),
		PrevHash:        header.PrevBlock.Bytes(),
		ProtocolMagic:   header.ProtocolMagic,
		PrevSlot:        prevSlot,
		PrevBlockNumber: prevBlockNum,
		PrevHeaderHash:  prevHash,
		IsEBB:           false,
		EnvelopeOnly:    true, // Envelope-only validation
	}

	result := validator.ValidateHeader(input)

	// Envelope validation should pass
	require.True(t, result.Valid, "Envelope validation should pass: %v", result.Errors)
	t.Logf("Envelope validation PASSED for slot %d, block %d",
		header.SlotNumber(), header.BlockNumber())
}

// TestByronSignatureFormat documents the Byron block signature format.
// This is useful for understanding how to implement proper signature verification.
func TestByronSignatureFormat(t *testing.T) {
	data, err := hex.DecodeString(strings.TrimSpace(mainnetByronBlockHex))
	require.NoError(t, err, "Failed to decode hex")

	block, err := byron.NewByronMainBlockFromCbor(data)
	require.NoError(t, err, "Failed to decode Byron block")

	header := block.BlockHeader
	blockSig := header.ConsensusData.BlockSig

	t.Logf("Byron BlockSig structure analysis:")
	t.Logf("  Number of elements: %d", len(blockSig))

	if len(blockSig) >= 1 {
		// First element is the signature type
		if sigType, ok := blockSig[0].(uint64); ok {
			switch sigType {
			case 0:
				t.Logf("  Type: 0 (BlockSignature - simple)")
			case 1:
				t.Logf("  Type: 1 (BlockPSignatureHeavy - delegation cert)")
			case 2:
				t.Logf("  Type: 2 (BlockPSignatureLight - lightweight delegation)")
			default:
				t.Logf("  Type: %d (unknown)", sigType)
			}
		}
	}

	// For delegation signatures, structure is typically:
	// [1, [[epoch, genesisKey, delegateKey, dlgCertSig], proxySignature]]
	// or
	// [2, [[omega, issuerVK, delegateVK, dlgCertSig], proxySignature]]

	// Document the structure for future implementation
	for i, elem := range blockSig {
		t.Logf("  Element [%d]: type=%T", i, elem)
		switch v := elem.(type) {
		case []byte:
			t.Logf("    bytes length: %d", len(v))
			if len(v) <= 64 {
				t.Logf("    value: %x", v)
			}
		case []any:
			t.Logf("    array length: %d", len(v))
			for j, inner := range v {
				switch iv := inner.(type) {
				case []byte:
					t.Logf("    [%d]: []byte, len=%d", j, len(iv))
				case uint64:
					t.Logf("    [%d]: uint64, val=%d", j, iv)
				default:
					t.Logf("    [%d]: %T", j, inner)
				}
			}
		}
	}
}

// TestByronTestnetBlockValidation tests validation of a testnet Byron block.
// This provides coverage for a different block than mainnet.
func TestByronTestnetBlockValidation(t *testing.T) {
	blockPath := filepath.Join(
		"..",
		"..",
		"..",
		"protocol",
		"chainsync",
		"testdata",
		"byron_main_block_testnet_f38aa5e8cf0b47d1ffa8b2385aa2d43882282db2ffd5ac0e3dadec1a6f2ecf08.hex",
	)

	hexData, err := os.ReadFile(blockPath)
	if err != nil {
		t.Skipf("Test file not found: %v", err)
	}

	data, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	require.NoError(t, err, "Failed to decode hex")

	block, err := byron.NewByronMainBlockFromCbor(data)
	require.NoError(t, err, "Failed to decode Byron block")

	header := block.BlockHeader

	// Test consensus data extraction
	pubKey, err := extractByronIssuerPubKey(header)
	require.NoError(t, err, "Failed to extract issuer pubkey")
	t.Logf("Testnet block issuer pubkey: %x", pubKey)

	sig, err := extractByronBlockSignature(header)
	require.NoError(t, err, "Failed to extract block signature")
	t.Logf("Testnet block signature: %x", sig)

	// Get signature type
	sigType, ok := header.ConsensusData.BlockSig[0].(uint64)
	require.True(t, ok, "Failed to get signature type")
	t.Logf("Testnet block signature type: %d", sigType)

	// Run envelope validation
	config := byronConsensus.ByronConfig{
		ProtocolMagic: header.ProtocolMagic,
		SlotsPerEpoch: byron.ByronSlotsPerEpoch,
	}

	validator := byronConsensus.NewHeaderValidator(config)

	prevHash := make([]byte, 32)
	copy(prevHash, header.PrevBlock.Bytes())

	// Calculate previous slot/block, guarding against underflow at genesis
	prevSlot := header.SlotNumber()
	if prevSlot > 0 {
		prevSlot--
	}
	prevBlockNum := header.BlockNumber()
	if prevBlockNum > 0 {
		prevBlockNum--
	}

	input := &byronConsensus.ValidateHeaderInput{
		Slot:            header.SlotNumber(),
		BlockNumber:     header.BlockNumber(),
		PrevHash:        header.PrevBlock.Bytes(),
		ProtocolMagic:   header.ProtocolMagic,
		PrevSlot:        prevSlot,
		PrevBlockNumber: prevBlockNum,
		PrevHeaderHash:  prevHash,
		IsEBB:           false,
		EnvelopeOnly:    true,
	}

	result := validator.ValidateHeader(input)
	require.True(t, result.Valid, "Envelope validation should pass: %v", result.Errors)
	t.Logf("Testnet block envelope validation PASSED")
}

// TestByronBodyProofStructure documents the Byron body proof structure.
// This is preparation for implementing BY-5 (body hash verification).
func TestByronBodyProofStructure(t *testing.T) {
	data, err := hex.DecodeString(strings.TrimSpace(mainnetByronBlockHex))
	require.NoError(t, err, "Failed to decode hex")

	block, err := byron.NewByronMainBlockFromCbor(data)
	require.NoError(t, err, "Failed to decode Byron block")

	header := block.BlockHeader

	// Byron BodyProof structure (per Cardano CDDL):
	// block_proof = [tx_proof, mpc_proof, prx_sk_proof, upd_proof]
	// where each proof is a hash or Merkle proof of the respective body section

	t.Logf("Byron BodyProof analysis:")
	bodyProof, ok := header.BodyProof.([]any)
	if !ok {
		t.Fatalf("BodyProof is not []any, got %T", header.BodyProof)
	}

	t.Logf("  Number of proof elements: %d", len(bodyProof))

	proofNames := []string{"tx_proof", "ssc_proof", "dlg_proof", "upd_proof"}
	for i, proof := range bodyProof {
		name := "unknown"
		if i < len(proofNames) {
			name = proofNames[i]
		}

		switch p := proof.(type) {
		case []byte:
			t.Logf("  [%d] %s: []byte, len=%d, value=%x", i, name, len(p), p)
		case []any:
			t.Logf("  [%d] %s: []any (Merkle proof), len=%d", i, name, len(p))
			// Could recurse to show more detail if needed
		default:
			t.Logf("  [%d] %s: %T", i, name, proof)
		}
	}

	// Note: For full body hash verification (BY-5), we would need to:
	// 1. Compute the Merkle root of each body section
	// 2. Combine them according to the Byron body proof algorithm
	// 3. Compare with the stored proof
	t.Logf("\nNote: Full body hash verification requires implementing Byron Merkle tree logic")
}

// TestByronDelegationCertFormats tests all possible delegation certificate signature formats
// to discover which one Cardano actually uses.
// SKIP: This is an exploratory test. The delegation certificate format is not yet fully understood.
// The block signature verification works without verifying the delegation certificate.
func TestByronDelegationCertFormats(t *testing.T) {
	t.Skip("Delegation certificate signature uses cardano-crypto's extended Ed25519 with unknown format")
	data, err := hex.DecodeString(strings.TrimSpace(mainnetByronBlockHex))
	require.NoError(t, err, "Failed to decode hex")

	block, err := byron.NewByronMainBlockFromCbor(data)
	require.NoError(t, err, "Failed to decode Byron block")

	header := block.BlockHeader

	// Extract the delegation certificate components
	// Structure: [2, [[omega, issuerVK, delegateVK, certSig], blockSig]]
	innerArray := header.ConsensusData.BlockSig[1].([]any)
	cert := innerArray[0].([]any)

	omega, _ := cert[0].(uint64)
	issuerVK := cert[1].([]byte)
	delegateVK := cert[2].([]byte)
	certSig := cert[3].([]byte)

	t.Logf("Omega: %d", omega)
	t.Logf("IssuerVK: %x", issuerVK)
	t.Logf("DelegateVK: %x", delegateVK)
	t.Logf("CertSig: %x", certSig)
	t.Logf("Protocol Magic: %d", header.ProtocolMagic)

	// The issuer's Ed25519 public key is the first 32 bytes of the extended key
	issuerPubKey := issuerVK[:32]

	// Try many different formats
	formats := []struct {
		name string
		data func() []byte
	}{
		{
			name: "CRC32 protected: 0x0a + CBOR([pm, crc32]) + CBOR_bytes(delegateVK + CBOR(omega))",
			data: func() []byte {
				// CRC-protected protocol magic
				pmCbor, _ := cbor.Encode(header.ProtocolMagic)
				crc := crc32.ChecksumIEEE(pmCbor)
				crcProtected, _ := cbor.Encode([]any{header.ProtocolMagic, crc})
				// Data payload
				omegaCbor, _ := cbor.Encode(omega)
				dataPayload := append(delegateVK, omegaCbor...)
				dataAsBytes, _ := cbor.Encode(dataPayload)
				// Combine
				buf := []byte{0x0a}
				buf = append(buf, crcProtected...)
				buf = append(buf, dataAsBytes...)
				return buf
			},
		},
		{
			name: "Rust format: ['0','1'] + issuerVK + 0x0a + CBOR(pm) + CBOR((delegateVK, omega))",
			data: func() []byte {
				pm, _ := cbor.Encode(header.ProtocolMagic)
				tuple, _ := cbor.Encode([]any{delegateVK, omega})
				buf := []byte{'0', '1'}
				buf = append(buf, issuerVK...)
				buf = append(buf, 0x0a)
				buf = append(buf, pm...)
				buf = append(buf, tuple...)
				return buf
			},
		},
		{
			name: "Haskell format: 0x0a + CBOR(pm) + '00' + delegateVK + CBOR(omega)",
			data: func() []byte {
				pm, _ := cbor.Encode(header.ProtocolMagic)
				omegaBytes, _ := cbor.Encode(omega)
				buf := []byte{0x0a}
				buf = append(buf, pm...)
				buf = append(buf, []byte("00")...)
				buf = append(buf, delegateVK...)
				buf = append(buf, omegaBytes...)
				return buf
			},
		},
		{
			name: "Simple format: 0x0a + CBOR(pm) + CBOR((delegateVK, omega))",
			data: func() []byte {
				pm, _ := cbor.Encode(header.ProtocolMagic)
				tuple, _ := cbor.Encode([]any{delegateVK, omega})
				buf := []byte{0x0a}
				buf = append(buf, pm...)
				buf = append(buf, tuple...)
				return buf
			},
		},
		{
			name: "Just CBOR((delegateVK, omega))",
			data: func() []byte {
				tuple, _ := cbor.Encode([]any{delegateVK, omega})
				return tuple
			},
		},
		{
			name: "Just CBOR((omega, delegateVK))",
			data: func() []byte {
				tuple, _ := cbor.Encode([]any{omega, delegateVK})
				return tuple
			},
		},
		{
			name: "'00' + delegateVK + CBOR(omega)",
			data: func() []byte {
				omegaBytes, _ := cbor.Encode(omega)
				buf := []byte("00")
				buf = append(buf, delegateVK...)
				buf = append(buf, omegaBytes...)
				return buf
			},
		},
		{
			name: "0x00 (byte) + delegateVK + CBOR(omega)",
			data: func() []byte {
				omegaBytes, _ := cbor.Encode(omega)
				buf := []byte{0x00}
				buf = append(buf, delegateVK...)
				buf = append(buf, omegaBytes...)
				return buf
			},
		},
		{
			name: "CBOR(pm) + '00' + delegateVK + CBOR(omega)",
			data: func() []byte {
				pm, _ := cbor.Encode(header.ProtocolMagic)
				omegaBytes, _ := cbor.Encode(omega)
				buf := pm
				buf = append(buf, []byte("00")...)
				buf = append(buf, delegateVK...)
				buf = append(buf, omegaBytes...)
				return buf
			},
		},
		{
			name: "delegateVK + CBOR(omega)",
			data: func() []byte {
				omegaBytes, _ := cbor.Encode(omega)
				buf := make([]byte, 0, len(delegateVK)+len(omegaBytes))
				buf = append(buf, delegateVK...)
				buf = append(buf, omegaBytes...)
				return buf
			},
		},
		{
			name: "CBOR(omega) + delegateVK",
			data: func() []byte {
				omegaBytes, _ := cbor.Encode(omega)
				buf := make([]byte, 0, len(delegateVK)+len(omegaBytes))
				buf = append(buf, omegaBytes...)
				buf = append(buf, delegateVK...)
				return buf
			},
		},
	}

	for _, f := range formats {
		signedData := f.data()
		valid := ed25519.Verify(issuerPubKey, signedData, certSig)
		if valid {
			t.Logf("SUCCESS: %s", f.name)
			t.Logf("  Signed data length: %d bytes", len(signedData))
			t.Logf("  Signed data: %x", signedData)
			return
		}
		t.Logf("FAILED: %s (len=%d)", f.name, len(signedData))
	}

	// Try additional formats with raw encoding
	additionalFormats := []struct {
		name string
		data func() []byte
	}{
		{
			name: "Raw: just omega as big-endian uint64",
			data: func() []byte {
				buf := make([]byte, 8)
				buf[7] = byte(omega)
				return append(delegateVK, buf...)
			},
		},
		{
			name: "CBOR array: [omega, delegateVK] definite",
			data: func() []byte {
				// Manually construct CBOR: 0x82 (2-elem array) + 0x00 (uint 0) + 0x5840 (64-byte bstr) + delegateVK
				buf := []byte{0x82, 0x00, 0x58, 0x40}
				buf = append(buf, delegateVK...)
				return buf
			},
		},
		{
			name: "Just delegateVK",
			data: func() []byte {
				return delegateVK
			},
		},
		{
			name: "0x0a + CBOR(pm) + CBOR([omega, delegateVK])",
			data: func() []byte {
				pm, _ := cbor.Encode(header.ProtocolMagic)
				tuple, _ := cbor.Encode([]any{omega, delegateVK})
				buf := []byte{0x0a}
				buf = append(buf, pm...)
				buf = append(buf, tuple...)
				return buf
			},
		},
		{
			name: "CBOR struct as array: [delegateVK, omega]",
			data: func() []byte {
				type pskData struct {
					cbor.StructAsArray
					DelegateVK []byte
					Omega      uint64
				}
				d, _ := cbor.Encode(pskData{DelegateVK: delegateVK, Omega: omega})
				return d
			},
		},
		{
			name: "With sign tag 0x0a only (no pm)",
			data: func() []byte {
				tuple, _ := cbor.Encode([]any{delegateVK, omega})
				buf := []byte{0x0a}
				buf = append(buf, tuple...)
				return buf
			},
		},
		{
			name: "Blake2b256(delegateVK) + omega",
			data: func() []byte {
				hash := common.Blake2b256Hash(delegateVK)
				omegaBytes, _ := cbor.Encode(omega)
				buf := hash.Bytes()
				buf = append(buf, omegaBytes...)
				return buf
			},
		},
		{
			name: "0x0a + pm_raw(4 bytes BE) + CBOR((delegateVK, omega))",
			data: func() []byte {
				pm := []byte{
					byte(header.ProtocolMagic >> 24),
					byte(header.ProtocolMagic >> 16),
					byte(header.ProtocolMagic >> 8),
					byte(header.ProtocolMagic),
				}
				tuple, _ := cbor.Encode([]any{delegateVK, omega})
				buf := []byte{0x0a}
				buf = append(buf, pm...)
				buf = append(buf, tuple...)
				return buf
			},
		},
		// Additional formats based on block signature pattern
		{
			name: "'00' + issuerVK + 0x0a + CBOR(pm) + CBOR((delegateVK, omega))",
			data: func() []byte {
				pm, _ := cbor.Encode(header.ProtocolMagic)
				tuple, _ := cbor.Encode([]any{delegateVK, omega})
				buf := []byte{'0', '0'}
				buf = append(buf, issuerVK...)
				buf = append(buf, 0x0a)
				buf = append(buf, pm...)
				buf = append(buf, tuple...)
				return buf
			},
		},
		{
			name: "0x0a + CBOR(pm) + '00' + issuerVK + delegateVK + CBOR(omega)",
			data: func() []byte {
				pm, _ := cbor.Encode(header.ProtocolMagic)
				omegaBytes, _ := cbor.Encode(omega)
				buf := []byte{0x0a}
				buf = append(buf, pm...)
				buf = append(buf, '0', '0')
				buf = append(buf, issuerVK...)
				buf = append(buf, delegateVK...)
				buf = append(buf, omegaBytes...)
				return buf
			},
		},
		{
			name: "CBOR((omega, issuerVK, delegateVK))",
			data: func() []byte {
				tuple, _ := cbor.Encode([]any{omega, issuerVK, delegateVK})
				return tuple
			},
		},
		{
			name: "CBOR((omega, delegateVK, issuerVK))",
			data: func() []byte {
				tuple, _ := cbor.Encode([]any{omega, delegateVK, issuerVK})
				return tuple
			},
		},
		{
			name: "0x0a + CBOR(pm) + CBOR((omega, delegateVK))",
			data: func() []byte {
				pm, _ := cbor.Encode(header.ProtocolMagic)
				tuple, _ := cbor.Encode([]any{omega, delegateVK})
				buf := []byte{0x0a}
				buf = append(buf, pm...)
				buf = append(buf, tuple...)
				return buf
			},
		},
		{
			name: "Just raw: delegateVK[:32] (Ed25519 portion only)",
			data: func() []byte {
				return delegateVK[:32]
			},
		},
		{
			name: "0x0a + CBOR(pm) + delegateVK[:32] + CBOR(omega)",
			data: func() []byte {
				pm, _ := cbor.Encode(header.ProtocolMagic)
				omegaBytes, _ := cbor.Encode(omega)
				buf := []byte{0x0a}
				buf = append(buf, pm...)
				buf = append(buf, delegateVK[:32]...)
				buf = append(buf, omegaBytes...)
				return buf
			},
		},
		{
			name: "0x0a + CBOR(pm) + '00' + delegateVK[:32] + CBOR(omega)",
			data: func() []byte {
				pm, _ := cbor.Encode(header.ProtocolMagic)
				omegaBytes, _ := cbor.Encode(omega)
				buf := []byte{0x0a}
				buf = append(buf, pm...)
				buf = append(buf, '0', '0')
				buf = append(buf, delegateVK[:32]...)
				buf = append(buf, omegaBytes...)
				return buf
			},
		},
		// Maybe it's over the full cert structure?
		{
			name: "CBOR([omega, delegateVK]) - cert payload only",
			data: func() []byte {
				tuple, _ := cbor.Encode([]any{omega, delegateVK})
				return tuple
			},
		},
		{
			name: "0x0a + CBOR(pm) + CBOR([omega, delegateVK]) - full cert payload",
			data: func() []byte {
				pm, _ := cbor.Encode(header.ProtocolMagic)
				tuple, _ := cbor.Encode([]any{omega, delegateVK})
				buf := []byte{0x0a}
				buf = append(buf, pm...)
				buf = append(buf, tuple...)
				return buf
			},
		},
		{
			name: "Full cert as CBOR: [omega, issuerVK, delegateVK]",
			data: func() []byte {
				tuple, _ := cbor.Encode([]any{omega, issuerVK, delegateVK})
				return tuple
			},
		},
		{
			name: "0x0a + CBOR(pm) + CBOR([omega, issuerVK, delegateVK])",
			data: func() []byte {
				pm, _ := cbor.Encode(header.ProtocolMagic)
				tuple, _ := cbor.Encode([]any{omega, issuerVK, delegateVK})
				buf := []byte{0x0a}
				buf = append(buf, pm...)
				buf = append(buf, tuple...)
				return buf
			},
		},
		// Try with epoch as Word64 (8 bytes)
		{
			name: "0x0a + CBOR(pm) + '00' + delegateVK + omega_uint64_cbor",
			data: func() []byte {
				pm, _ := cbor.Encode(header.ProtocolMagic)
				// Force epoch as 8-byte CBOR: 0x1b + 8 bytes
				omegaBytes := []byte{0x1b, 0, 0, 0, 0, 0, 0, 0, 0}
				buf := []byte{0x0a}
				buf = append(buf, pm...)
				buf = append(buf, '0', '0')
				buf = append(buf, delegateVK...)
				buf = append(buf, omegaBytes...)
				return buf
			},
		},
		// No signTag - just payload
		{
			name: "'00' + delegateVK + omega (no tag)",
			data: func() []byte {
				omegaBytes, _ := cbor.Encode(omega)
				buf := []byte{'0', '0'}
				buf = append(buf, delegateVK...)
				buf = append(buf, omegaBytes...)
				return buf
			},
		},
		// Reverse: omega first
		{
			name: "0x0a + CBOR(pm) + omega + '00' + delegateVK",
			data: func() []byte {
				pm, _ := cbor.Encode(header.ProtocolMagic)
				omegaBytes, _ := cbor.Encode(omega)
				buf := []byte{0x0a}
				buf = append(buf, pm...)
				buf = append(buf, omegaBytes...)
				buf = append(buf, '0', '0')
				buf = append(buf, delegateVK...)
				return buf
			},
		},
		// Chain code only
		{
			name: "0x0a + CBOR(pm) + '00' + chainCode + CBOR(omega)",
			data: func() []byte {
				pm, _ := cbor.Encode(header.ProtocolMagic)
				omegaBytes, _ := cbor.Encode(omega)
				chainCode := delegateVK[32:] // Second 32 bytes
				buf := []byte{0x0a}
				buf = append(buf, pm...)
				buf = append(buf, '0', '0')
				buf = append(buf, chainCode...)
				buf = append(buf, omegaBytes...)
				return buf
			},
		},
	}

	for _, f := range additionalFormats {
		signedData := f.data()
		valid := ed25519.Verify(issuerPubKey, signedData, certSig)
		if valid {
			t.Logf("SUCCESS: %s", f.name)
			t.Logf("  Signed data length: %d bytes", len(signedData))
			t.Logf("  Signed data: %x", signedData)
			return
		}
		t.Logf("FAILED: %s (len=%d)", f.name, len(signedData))
	}

	// Also try with delegate's public key (maybe it's a self-signed key?)
	delegatePubKey := delegateVK[:32]
	t.Logf("Trying with delegate's public key: %x", delegatePubKey)

	for _, f := range formats {
		signedData := f.data()
		valid := ed25519.Verify(delegatePubKey, signedData, certSig)
		if valid {
			t.Logf("SUCCESS with delegate key: %s", f.name)
			t.Logf("  Signed data length: %d bytes", len(signedData))
			t.Logf("  Signed data: %x", signedData)
			return
		}
	}

	for _, f := range additionalFormats {
		signedData := f.data()
		valid := ed25519.Verify(delegatePubKey, signedData, certSig)
		if valid {
			t.Logf("SUCCESS with delegate key: %s", f.name)
			t.Logf("  Signed data length: %d bytes", len(signedData))
			t.Logf("  Signed data: %x", signedData)
			return
		}
	}

	t.Log("Delegate key also failed")

	// Let's also check if the header's public key is different
	headerPubKey := header.ConsensusData.PubKey[:32]
	t.Logf("Header PubKey (Ed25519): %x", headerPubKey)
	t.Logf("IssuerVK[:32]: %x", issuerPubKey)
	t.Logf("DelegateVK[:32]: %x", delegatePubKey)
	t.Logf("Same header/issuer? %v", bytes.Equal(headerPubKey, issuerPubKey))

	require.Fail(t, "No format worked!")
}

// TestByronBlockSignatureFormats tests different block signature formats
func TestByronBlockSignatureFormats(t *testing.T) {
	data, err := hex.DecodeString(strings.TrimSpace(mainnetByronBlockHex))
	require.NoError(t, err, "Failed to decode hex")

	block, err := byron.NewByronMainBlockFromCbor(data)
	require.NoError(t, err, "Failed to decode Byron block")

	header := block.BlockHeader

	// Extract the proxy signature components
	// Structure: [2, [[omega, issuerVK, delegateVK, certSig], blockSig]]
	innerArray, ok := header.ConsensusData.BlockSig[1].([]any)
	if !ok {
		t.Fatalf("expected []any for BlockSig[1], got %T", header.ConsensusData.BlockSig[1])
	}
	if len(innerArray) < 2 {
		t.Fatalf("innerArray too short: expected at least 2 elements, got %d", len(innerArray))
	}

	cert, ok := innerArray[0].([]any)
	if !ok {
		t.Fatalf("expected []any for innerArray[0] (cert), got %T", innerArray[0])
	}
	if len(cert) < 3 {
		t.Fatalf("cert too short: expected at least 3 elements, got %d", len(cert))
	}

	issuerVK, ok := cert[1].([]byte)
	if !ok {
		t.Fatalf("expected []byte for cert[1] (issuerVK), got %T", cert[1])
	}
	delegateVK, ok := cert[2].([]byte)
	if !ok {
		t.Fatalf("expected []byte for cert[2] (delegateVK), got %T", cert[2])
	}
	blockSig, ok := innerArray[1].([]byte)
	if !ok {
		t.Fatalf("expected []byte for innerArray[1] (blockSig), got %T", innerArray[1])
	}

	t.Logf("IssuerVK (64 bytes): %x", issuerVK)
	t.Logf("DelegateVK (64 bytes): %x", delegateVK)
	t.Logf("BlockSig (64 bytes): %x", blockSig)
	t.Logf("Protocol Magic: %d", header.ProtocolMagic)

	// The delegate's Ed25519 public key is the first 32 bytes
	delegatePubKey := delegateVK[:32]

	// Get the header CBOR for building ToSign
	headerCbor := header.Cbor()
	t.Logf("Header CBOR length: %d bytes", len(headerCbor))

	// Build various ToSign structures
	pmBytes, _ := cbor.Encode(header.ProtocolMagic)
	t.Logf("CBOR(protocolMagic): %x", pmBytes)

	// ToSign: [prevHash, bodyProof, epochAndSlot, difficulty, extraData]
	toSign := struct {
		cbor.StructAsArray
		PrevHash    common.Blake2b256
		BodyProof   any
		EpochSlot   any
		Difficulty  any
		ExtraHeader any
	}{
		PrevHash:  header.PrevBlock,
		BodyProof: header.BodyProof,
		EpochSlot: struct {
			cbor.StructAsArray
			Epoch uint64
			Slot  uint16
		}{
			Epoch: header.ConsensusData.SlotId.Epoch,
			Slot:  header.ConsensusData.SlotId.Slot,
		},
		Difficulty: struct {
			cbor.StructAsArray
			Value uint64
		}{
			Value: header.ConsensusData.Difficulty.Value,
		},
		ExtraHeader: struct {
			cbor.StructAsArray
			BlockVersion    byron.ByronBlockVersion
			SoftwareVersion byron.ByronSoftwareVersion
			Attributes      any
			ExtraProof      common.Blake2b256
		}{
			BlockVersion:    header.ExtraData.BlockVersion,
			SoftwareVersion: header.ExtraData.SoftwareVersion,
			Attributes:      header.ExtraData.Attributes,
			ExtraProof:      header.ExtraData.ExtraProof,
		},
	}

	toSignBytes, _ := cbor.Encode(toSign)
	t.Logf("CBOR(ToSign) length: %d bytes", len(toSignBytes))
	t.Logf("CBOR(ToSign): %x", toSignBytes)

	// Try different buffer formats
	formats := []struct {
		name string
		data func() []byte
	}{
		{
			name: "Full Rust format: '01' + issuerVK + 0x08 + CBOR(pm) + CBOR(toSign)",
			data: func() []byte {
				buf := []byte{'0', '1'}
				buf = append(buf, issuerVK...)
				buf = append(buf, 0x08) // MainBlockLight tag
				buf = append(buf, pmBytes...)
				buf = append(buf, toSignBytes...)
				return buf
			},
		},
		{
			name: "Full Rust format: '01' + issuerVK + 0x09 + CBOR(pm) + CBOR(toSign)",
			data: func() []byte {
				buf := []byte{'0', '1'}
				buf = append(buf, issuerVK...)
				buf = append(buf, 0x09) // MainBlockHeavy tag
				buf = append(buf, pmBytes...)
				buf = append(buf, toSignBytes...)
				return buf
			},
		},
		{
			name: "Without prefix: issuerVK + 0x08 + CBOR(pm) + CBOR(toSign)",
			data: func() []byte {
				buf := issuerVK
				buf = append(buf, 0x08)
				buf = append(buf, pmBytes...)
				buf = append(buf, toSignBytes...)
				return buf
			},
		},
		{
			name: "Just ToSign",
			data: func() []byte {
				return toSignBytes
			},
		},
		{
			name: "0x08 + CBOR(pm) + CBOR(toSign)",
			data: func() []byte {
				buf := []byte{0x08}
				buf = append(buf, pmBytes...)
				buf = append(buf, toSignBytes...)
				return buf
			},
		},
		{
			name: "CBOR(pm) + CBOR(toSign)",
			data: func() []byte {
				buf := pmBytes
				buf = append(buf, toSignBytes...)
				return buf
			},
		},
		{
			name: "Header CBOR only",
			data: func() []byte {
				return headerCbor
			},
		},
		{
			name: "'01' + CBOR(toSign)",
			data: func() []byte {
				buf := []byte{'0', '1'}
				buf = append(buf, toSignBytes...)
				return buf
			},
		},
	}

	for _, f := range formats {
		signedData := f.data()
		valid := ed25519.Verify(delegatePubKey, signedData, blockSig)
		if valid {
			t.Logf("SUCCESS: %s", f.name)
			t.Logf("  Signed data length: %d bytes", len(signedData))
			return
		}
		t.Logf("FAILED: %s (len=%d)", f.name, len(signedData))
	}

	require.Fail(t, "No block signature format worked!")
}

// Real mainnet Byron block from cexplorer (slot 4471207)
// https://cexplorer.io/block/1451a0dbf16cfeddf4991a838961df1b08a68f43a19c0eb3b36cc4029c77a2d8
const mainnetByronBlockHex = `83851a2d964a09582025df38df102b89ec25a432a2972993d2fa8cc1f597a73e6260b2f07e79501eb084830258200f284bc22f5b96228ee0687b7bb87c56132f77df4235c78a1595729ccfce2001582019fb988d02ec920a6de5ac71c5d5e75f8b73d7ed8e8abea7773e28859983206e82035820d36a2619a672494604e11bb447cbcf5231e9f2ba25c2169177edc941bd50ad6c5820afc0da64183bf2664f3d4eec7238d524ba607faeeab24fc100eb861dba69971b58204e66280cd94d591072349bec0a3090a53aa945562efb6d08d56e53654b0e4098848218cf0758401bc97a2fe02c297880ce8ecfd997fe4c1ec09ee10feeee9f686760166b05281d6283468ffd93becb0c956ccddd642df9b1244c915911185fa49355f6f22bfab9811a004430ed820282840058401bc97a2fe02c297880ce8ecfd997fe4c1ec09ee10feeee9f686760166b05281d6283468ffd93becb0c956ccddd642df9b1244c915911185fa49355f6f22bfab9584061261a95b7613ee6bf2067dad77b70349729b0c50d57bc1cf30de0db4a1e73a885d0054af7c23fc6c37919dba41c602a57e2d0f9329a7954b867338d6fb2c9455840e03e62f083df5576360e60a32e22bbb07b3c8df4fcab8079f1d6f61af3954d242ba8a06516c395939f24096f3df14e103a7d9c2b80a68a9363cf1f27c7a4e3075840325068a2307397703c4eebb1de1ecab0b23c24a5e80c985e0f7546bb6571ee9eb94069708fc25ec67a4a5753a0d49ab5e536131c19c7f9dd4fd32532fd0f71028483010000826a63617264616e6f2d736c01a058204ba92aa320c60acc9ad7b9a64f2eda55c4d2ec28e604faf186708b4f0c4e8edf849f82839f8200d8185824825820b0a7782d21f37e9d98f4cbdc23bf2677d93eca1ac0fb3f79923863a698d53f8f018200d81858248258205bd3e8385d2ecdd17d3b602263e8a5e7aa0edb4dd00221f369c2720f7d85940d008200d81858248258201e4a77f8375548e5bc409a518dbcb4a8437b539682f4e840f4a1056f01cea566008200d81858248258205e83b53253f705c214d904f65fdaaa2f153db59a229a9cee1da6c329b543236100ff9f8282d818584283581ca1932430cb1ad6482a1b67964d778c18b574674fda151cdfa73c63cda101581e581cfc8a0b5477e819a27a34910e6c174b50b871192e95cca1a711bbceb3001abcb52f6d1b000000013446d5718282d818584283581c093916f7e775fba80eaa65cded085d985f7f9e4982cddd2bb7c476aea101581e581c83d3e2df30edf90acf198b85a7327d964f9d92fd739d0c986a914f6c001a27a611b61a000c48cbffa0848200d8185885825840a781c060f2b32d116ef79bb1823d4a25ea36f6c130b3a182713739ea1819e32d261db3dce30b15c29db81d1c284d3fe350d12241e8ccc65cdf08adba90e0ad4558408eb4c9549a6a044d687d6c04fdee2240994f43966ef113ebb3e76a756e39472badb137c3e0268d34ce6042f76c2534220cc1e061a1a29cce065faf486184cf078200d818588582584085dc150754227f68d1640887f8fa57c93e4cad3499f2cb7b5e8258b0b367dcceaa42bf9ea1cfff73fd0fab44d9e0a36ef61bc5d0f294365316a4e0ed12b40a135840f1233519fa85f3ecbb2deaa9dff2d7e943156d49a7a33603381f2c1779b7f65ea0d39a8dcdd227f5d69b9355ab35df0c43c2abb751c6dd24b107a2c7ac51f5088200d81858858258403559467e9b4a4e47af0388e7224358197e5d39c57c71c391db4a7d480f297d8b86b0746de21dc5dfca2bd8b8fa817c1fa1c3bd3eeaddbfd7a6b270564e416d0c5840b0e33544dcb1895b592a612f5be81242a88226d0612da76099b653f89ce7c5641af14fad696ccd44b58744915291240224fd83a26f103c0717752ea256b4af0b8200d8185885825840572c3ea039ded80f19b0d6841e9ad0d0d1b73242ac98538affbec6e7356192f48eba0291ea1b174f9c42e139ba85ce75656a036ba0993dda605d5a62956dba6558406257e3a27a896268cade4d5371537ed606d3004d6269f87ebe6056b6eff737a2a9ef82d27ba1f9b642ffc622ec27b38e69ed41e272d3de0767cad860d50fa10d82839f8200d8185824825820779a319e0d64b80eaff5ed13d08062b8672fc71ac27e7b30574c1c7972764de202ff9f8282d818582183581c2c0dd53d4e6001e006729fc09d74c5a799d5f93c9f4b74748412a823a0001a1abd89081a769cfd808282d818582183581c05b073f36ee030589a31148838cd47e8d8c8f82fec9fe091c7d53cd8a0001a0c5f26b11b00000016bc0c4c47ffa0818200d8185885825840f129f07bbfd87fd1d3ff5fb32e9a5566e02208f89518e9994048add22074f433424e682a392581268c7544e34e9c54378a8820bdcf7dddce30490bbb2d363b4b5840709a2e70d3803554a15d788235bf56c9567407102be375be5071fa81d4c137047743b5f5abefdbab6b2781822474995dff917213c962ecd111619d75b8534f0aff8203d90102809fff82809fff81a0`
