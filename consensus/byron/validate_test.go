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

package byron

import (
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// Test constants matching mainnet Byron parameters
const (
	testByronSlotsPerEpoch        = 21600
	testByronSlotDuration         = 20 * time.Second
	testByronSecurityParam        = 2160
	testByronProtocolMagicMainnet = 764824073
	testByronProtocolMagicTestnet = 1097911063
)

// testByronConfig returns a ByronConfig for testing with mainnet-like parameters.
func testByronConfig() ByronConfig {
	return ByronConfig{
		ProtocolMagic:  testByronProtocolMagicMainnet,
		SlotsPerEpoch:  testByronSlotsPerEpoch,
		SlotDuration:   testByronSlotDuration,
		SecurityParam:  testByronSecurityParam,
		NumGenesisKeys: 7, // Mainnet had 7 genesis delegates
	}
}

func TestNewHeaderValidator(t *testing.T) {
	config := testByronConfig()
	validator := NewHeaderValidator(config)

	if validator == nil {
		t.Fatal("expected non-nil validator")
	}

	if validator.config.ProtocolMagic != testByronProtocolMagicMainnet {
		t.Errorf("expected protocol magic %d, got %d",
			testByronProtocolMagicMainnet, validator.config.ProtocolMagic)
	}
}

func TestValidateSlotOrdering(t *testing.T) {
	config := testByronConfig()
	validator := NewHeaderValidator(config)

	tests := []struct {
		name        string
		input       *ValidateHeaderInput
		expectError bool
	}{
		{
			name: "valid slot increase",
			input: &ValidateHeaderInput{
				Slot:     100,
				PrevSlot: 50,
				IsEBB:    false,
			},
			expectError: false,
		},
		{
			name: "slot not increasing",
			input: &ValidateHeaderInput{
				Slot:     50,
				PrevSlot: 100,
				IsEBB:    false,
			},
			expectError: true,
		},
		{
			name: "slot equal (invalid for non-EBB)",
			input: &ValidateHeaderInput{
				Slot:     100,
				PrevSlot: 100,
				IsEBB:    false,
			},
			expectError: true,
		},
		{
			name: "EBB can have equal slot",
			input: &ValidateHeaderInput{
				Slot:     testByronSlotsPerEpoch,
				PrevSlot: testByronSlotsPerEpoch,
				IsEBB:    true,
			},
			expectError: false,
		},
		{
			name: "EBB at non-epoch-boundary (invalid)",
			input: &ValidateHeaderInput{
				Slot:     100, // Not at epoch boundary
				PrevSlot: 50,
				IsEBB:    true,
			},
			expectError: true,
		},
		{
			name: "EBB slot decreasing (invalid)",
			input: &ValidateHeaderInput{
				Slot:     50,
				PrevSlot: 100,
				IsEBB:    true,
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.validateSlotOrdering(tc.input)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateBlockNumber(t *testing.T) {
	config := testByronConfig()
	validator := NewHeaderValidator(config)

	tests := []struct {
		name        string
		input       *ValidateHeaderInput
		expectError bool
	}{
		{
			name: "valid block number",
			input: &ValidateHeaderInput{
				BlockNumber:     101,
				PrevBlockNumber: 100,
				IsEBB:           false,
			},
			expectError: false,
		},
		{
			name: "block number not incremented",
			input: &ValidateHeaderInput{
				BlockNumber:     100,
				PrevBlockNumber: 100,
				IsEBB:           false,
			},
			expectError: true,
		},
		{
			name: "block number skipped",
			input: &ValidateHeaderInput{
				BlockNumber:     103,
				PrevBlockNumber: 100,
				IsEBB:           false,
			},
			expectError: true,
		},
		{
			name: "EBB can have same block number",
			input: &ValidateHeaderInput{
				BlockNumber:     100,
				PrevBlockNumber: 100,
				IsEBB:           true,
			},
			expectError: false,
		},
		{
			name: "EBB can have next block number",
			input: &ValidateHeaderInput{
				BlockNumber:     101,
				PrevBlockNumber: 100,
				IsEBB:           true,
			},
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.validateBlockNumber(tc.input)
			if tc.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestValidatePrevHash(t *testing.T) {
	config := testByronConfig()
	validator := NewHeaderValidator(config)

	prevHash := make([]byte, 32)
	for i := range prevHash {
		prevHash[i] = byte(i)
	}

	tests := []struct {
		name        string
		input       *ValidateHeaderInput
		expectError bool
	}{
		{
			name: "matching prev hash",
			input: &ValidateHeaderInput{
				PrevHash:       prevHash,
				PrevHeaderHash: prevHash,
			},
			expectError: false,
		},
		{
			name: "mismatched prev hash",
			input: &ValidateHeaderInput{
				PrevHash:       prevHash,
				PrevHeaderHash: make([]byte, 32),
			},
			expectError: true,
		},
		{
			name: "no prev header hash (genesis)",
			input: &ValidateHeaderInput{
				PrevHash:       prevHash,
				PrevHeaderHash: nil,
			},
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.validatePrevHash(tc.input)
			if tc.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestValidateProtocolMagic(t *testing.T) {
	config := testByronConfig()
	validator := NewHeaderValidator(config)

	tests := []struct {
		name        string
		input       *ValidateHeaderInput
		expectError bool
	}{
		{
			name: "matching protocol magic",
			input: &ValidateHeaderInput{
				ProtocolMagic: testByronProtocolMagicMainnet,
			},
			expectError: false,
		},
		{
			name: "wrong protocol magic",
			input: &ValidateHeaderInput{
				ProtocolMagic: testByronProtocolMagicTestnet,
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.validateProtocolMagic(tc.input)
			if tc.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestValidateBlockSignature(t *testing.T) {
	config := testByronConfig()
	validator := NewHeaderValidator(config)
	// Enable fallback since we're using raw test data, not real CBOR headers
	validator.AllowSignatureFallback = true

	// Generate test key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey failed: %v", err)
	}
	message := []byte("test header body")
	signature := ed25519.Sign(privKey, message)

	tests := []struct {
		name        string
		input       *ValidateHeaderInput
		expectError bool
	}{
		{
			name: "valid signature",
			input: &ValidateHeaderInput{
				IssuerPubKey:   pubKey,
				BlockSignature: signature,
				HeaderCbor:     message,
			},
			expectError: false,
		},
		{
			name: "invalid signature",
			input: &ValidateHeaderInput{
				IssuerPubKey:   pubKey,
				BlockSignature: make([]byte, ed25519.SignatureSize),
				HeaderCbor:     message,
			},
			expectError: true,
		},
		{
			name: "wrong message",
			input: &ValidateHeaderInput{
				IssuerPubKey:   pubKey,
				BlockSignature: signature,
				HeaderCbor:     []byte("different message"),
			},
			expectError: true,
		},
		{
			name: "missing header cbor",
			input: &ValidateHeaderInput{
				IssuerPubKey:   pubKey,
				BlockSignature: signature,
				HeaderCbor:     nil,
			},
			expectError: true,
		},
		{
			name: "wrong pub key size",
			input: &ValidateHeaderInput{
				IssuerPubKey:   []byte("short"),
				BlockSignature: signature,
				HeaderCbor:     message,
			},
			expectError: true,
		},
		{
			name: "wrong signature size",
			input: &ValidateHeaderInput{
				IssuerPubKey:   pubKey,
				BlockSignature: []byte("short"),
				HeaderCbor:     message,
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.validateBlockSignature(tc.input)
			if tc.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestValidateGenesisDelegate(t *testing.T) {
	// Generate test key
	pubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey failed: %v", err)
	}
	keyHash := common.Blake2b224Hash(pubKey).Bytes()

	// Config with known genesis key
	config := testByronConfig()
	config.GenesisKeyHashes = [][]byte{keyHash}
	validator := NewHeaderValidator(config)

	tests := []struct {
		name        string
		input       *ValidateHeaderInput
		expectError bool
	}{
		{
			name: "valid genesis delegate",
			input: &ValidateHeaderInput{
				IssuerPubKey: pubKey,
			},
			expectError: false,
		},
		{
			name: "unknown delegate",
			input: &ValidateHeaderInput{
				IssuerPubKey: make([]byte, ed25519.PublicKeySize),
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.validateGenesisDelegate(tc.input)
			if tc.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}

	// Test with no genesis keys configured (should skip check)
	configNoKeys := testByronConfig()
	validatorNoKeys := NewHeaderValidator(configNoKeys)
	input := &ValidateHeaderInput{
		IssuerPubKey: make([]byte, ed25519.PublicKeySize),
	}
	err = validatorNoKeys.validateGenesisDelegate(input)
	if err != nil {
		t.Errorf(
			"expected no error when no genesis keys configured, got: %v",
			err,
		)
	}
}

func TestValidateSlotLeader(t *testing.T) {
	// Generate 3 test keys to simulate genesis delegates
	keys := make([]ed25519.PublicKey, 3)
	keyHashes := make([][]byte, 3)
	for i := 0; i < 3; i++ {
		pubKey, _, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatalf("ed25519.GenerateKey failed: %v", err)
		}
		keys[i] = pubKey
		keyHashes[i] = common.Blake2b224Hash(pubKey).Bytes()
	}

	// Config with 3 genesis delegates in order
	config := testByronConfig()
	config.GenesisKeyHashes = keyHashes
	config.NumGenesisKeys = 3
	validator := NewHeaderValidator(config)

	tests := []struct {
		name        string
		slot        uint64
		issuerKey   ed25519.PublicKey
		expectError bool
	}{
		{
			name:        "slot 0 expects delegate 0",
			slot:        0,
			issuerKey:   keys[0],
			expectError: false,
		},
		{
			name:        "slot 1 expects delegate 1",
			slot:        1,
			issuerKey:   keys[1],
			expectError: false,
		},
		{
			name:        "slot 2 expects delegate 2",
			slot:        2,
			issuerKey:   keys[2],
			expectError: false,
		},
		{
			name:        "slot 3 expects delegate 0 (round-robin)",
			slot:        3,
			issuerKey:   keys[0],
			expectError: false,
		},
		{
			name:        "slot 4 expects delegate 1 (round-robin)",
			slot:        4,
			issuerKey:   keys[1],
			expectError: false,
		},
		{
			name:        "slot 100 expects delegate 1 (100 % 3 = 1)",
			slot:        100,
			issuerKey:   keys[1],
			expectError: false,
		},
		{
			name:        "wrong delegate for slot 0",
			slot:        0,
			issuerKey:   keys[1], // Should be keys[0]
			expectError: true,
		},
		{
			name:        "wrong delegate for slot 1",
			slot:        1,
			issuerKey:   keys[0], // Should be keys[1]
			expectError: true,
		},
		{
			name:        "wrong delegate for slot 5",
			slot:        5,
			issuerKey:   keys[0], // Should be keys[2] (5 % 3 = 2)
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := &ValidateHeaderInput{
				Slot:         tc.slot,
				IssuerPubKey: tc.issuerKey,
			}
			err := validator.validateSlotLeader(input)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}

	// Test with no genesis keys configured (should skip check)
	configNoKeys := testByronConfig()
	validatorNoKeys := NewHeaderValidator(configNoKeys)
	inputNoKeys := &ValidateHeaderInput{
		Slot:         0,
		IssuerPubKey: make([]byte, ed25519.PublicKeySize),
	}
	err := validatorNoKeys.validateSlotLeader(inputNoKeys)
	require.NoError(t, err, "expected no error when no genesis keys configured")
}

func TestSlotLeader(t *testing.T) {
	// Generate 5 test key hashes
	keyHashes := make([][]byte, 5)
	for i := 0; i < 5; i++ {
		keyHashes[i] = make([]byte, 28) // Blake2b224 hash size
		keyHashes[i][0] = byte(i)       // Make each unique
	}

	config := ByronConfig{
		GenesisKeyHashes: keyHashes,
	}

	tests := []struct {
		slot          uint64
		expectedIndex int
	}{
		{0, 0},
		{1, 1},
		{2, 2},
		{3, 3},
		{4, 4},
		{5, 0}, // 5 % 5 = 0
		{6, 1}, // 6 % 5 = 1
		{10, 0},
		{11, 1},
		{100, 0}, // 100 % 5 = 0
		{101, 1}, // 101 % 5 = 1
		{102, 2}, // 102 % 5 = 2
	}

	for _, tc := range tests {
		index, keyHash := config.SlotLeader(tc.slot)
		require.Equal(t, tc.expectedIndex, index, "SlotLeader(%d) index mismatch", tc.slot)
		require.NotNil(t, keyHash, "SlotLeader(%d) returned nil key hash", tc.slot)
		require.Equal(t, byte(tc.expectedIndex), keyHash[0], "SlotLeader(%d) returned wrong key hash", tc.slot)
	}

	// Test with no genesis keys
	emptyConfig := ByronConfig{}
	index, keyHash := emptyConfig.SlotLeader(0)
	require.Equal(t, -1, index, "SlotLeader with no keys should return -1")
	require.Nil(t, keyHash, "SlotLeader with no keys should return nil hash")
}

func TestValidateHeaderFull(t *testing.T) {
	config := testByronConfig()
	validator := NewHeaderValidator(config)
	// Enable fallback since we're using raw test data, not real CBOR headers
	validator.AllowSignatureFallback = true

	// Generate test key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey failed: %v", err)
	}
	message := []byte("test header body")
	signature := ed25519.Sign(privKey, message)
	prevHash := make([]byte, 32)

	// Valid main block header
	input := &ValidateHeaderInput{
		Slot:            100,
		BlockNumber:     50,
		PrevHash:        prevHash,
		ProtocolMagic:   testByronProtocolMagicMainnet,
		IssuerPubKey:    pubKey,
		BlockSignature:  signature,
		HeaderCbor:      message,
		PrevSlot:        50,
		PrevBlockNumber: 49,
		PrevHeaderHash:  prevHash,
		IsEBB:           false,
	}

	result := validator.ValidateHeader(input)
	if !result.Valid {
		t.Errorf("expected valid header, got errors: %v", result.Errors)
	}
}

func TestValidateEBBHeader(t *testing.T) {
	config := testByronConfig()
	validator := NewHeaderValidator(config)

	prevHash := make([]byte, 32)

	// Valid EBB header
	input := &ValidateHeaderInput{
		Slot:            testByronSlotsPerEpoch,
		BlockNumber:     100,
		PrevHash:        prevHash,
		ProtocolMagic:   testByronProtocolMagicMainnet,
		PrevSlot:        testByronSlotsPerEpoch - 1,
		PrevBlockNumber: 100,
		PrevHeaderHash:  prevHash,
		IsEBB:           true,
	}

	result := validator.ValidateHeader(input)
	if !result.Valid {
		t.Errorf("expected valid EBB header, got errors: %v", result.Errors)
	}
}

func TestSlotToEpoch(t *testing.T) {
	config := testByronConfig()
	tests := []struct {
		slot     uint64
		expected uint64
	}{
		{0, 0},
		{1, 0},
		{testByronSlotsPerEpoch - 1, 0},
		{testByronSlotsPerEpoch, 1},
		{testByronSlotsPerEpoch + 1, 1},
		{2 * testByronSlotsPerEpoch, 2},
	}

	for _, tc := range tests {
		result := config.SlotToEpoch(tc.slot)
		if result != tc.expected {
			t.Errorf(
				"SlotToEpoch(%d) = %d, want %d",
				tc.slot,
				result,
				tc.expected,
			)
		}
	}
}

func TestEpochFirstSlot(t *testing.T) {
	config := testByronConfig()
	tests := []struct {
		epoch    uint64
		expected uint64
	}{
		{0, 0},
		{1, testByronSlotsPerEpoch},
		{2, 2 * testByronSlotsPerEpoch},
	}

	for _, tc := range tests {
		result := config.EpochFirstSlot(tc.epoch)
		if result != tc.expected {
			t.Errorf(
				"EpochFirstSlot(%d) = %d, want %d",
				tc.epoch,
				result,
				tc.expected,
			)
		}
	}
}

func TestIsEpochBoundarySlot(t *testing.T) {
	config := testByronConfig()
	tests := []struct {
		slot     uint64
		expected bool
	}{
		{0, true},
		{1, false},
		{testByronSlotsPerEpoch - 1, false},
		{testByronSlotsPerEpoch, true},
		{2 * testByronSlotsPerEpoch, true},
	}

	for _, tc := range tests {
		result := config.IsEpochBoundarySlot(tc.slot)
		if result != tc.expected {
			t.Errorf(
				"IsEpochBoundarySlot(%d) = %v, want %v",
				tc.slot,
				result,
				tc.expected,
			)
		}
	}
}

func TestValidateBodyHash_NilBlock(t *testing.T) {
	err := ValidateBodyHash(nil)
	if err == nil {
		t.Error("expected error for nil block")
	}
	valErr, ok := err.(*common.ValidationError)
	if !ok {
		t.Errorf("expected ValidationError, got %T", err)
	}
	if valErr.Type != common.ValidationErrorTypeBodyHash {
		t.Errorf("expected body_hash error type, got %s", valErr.Type)
	}
}

func TestValidateEBBBodyHash_NilBlock(t *testing.T) {
	err := ValidateEBBBodyHash(nil)
	if err == nil {
		t.Error("expected error for nil block")
	}
	valErr, ok := err.(*common.ValidationError)
	if !ok {
		t.Errorf("expected ValidationError, got %T", err)
	}
	if valErr.Type != common.ValidationErrorTypeBodyHash {
		t.Errorf("expected body_hash error type, got %s", valErr.Type)
	}
}

func TestComputeMerkleRoot_Empty(t *testing.T) {
	// Empty list should produce hash of empty bytes
	root := computeMerkleRoot(nil)
	expected := common.Blake2b256Hash(nil)
	if root != expected {
		t.Errorf("empty merkle root mismatch: got %s, want %s", root.String(), expected.String())
	}
}

func TestComputeMerkleRoot_SingleItem(t *testing.T) {
	// Single item: hash(0x00 || item)
	item := []byte("test data")
	root := computeMerkleRoot([][]byte{item})

	// Expected: hash(0x00 || item)
	leafData := append([]byte{0x00}, item...)
	expected := common.Blake2b256Hash(leafData)
	if root != expected {
		t.Errorf("single item merkle root mismatch: got %s, want %s", root.String(), expected.String())
	}
}

func TestComputeMerkleRoot_TwoItems(t *testing.T) {
	// Two items should create a branch
	item1 := []byte("item1")
	item2 := []byte("item2")
	root := computeMerkleRoot([][]byte{item1, item2})

	// Expected: hash(0x01 || hash(0x00 || item1) || hash(0x00 || item2))
	leaf1 := common.Blake2b256Hash(append([]byte{0x00}, item1...))
	leaf2 := common.Blake2b256Hash(append([]byte{0x00}, item2...))

	branchData := make([]byte, 1+64)
	branchData[0] = 0x01
	copy(branchData[1:33], leaf1[:])
	copy(branchData[33:65], leaf2[:])
	expected := common.Blake2b256Hash(branchData)

	if root != expected {
		t.Errorf("two item merkle root mismatch: got %s, want %s", root.String(), expected.String())
	}
}

func TestComputeMerkleRoot_ThreeItems(t *testing.T) {
	// Three items: last one gets duplicated for padding
	item1 := []byte("item1")
	item2 := []byte("item2")
	item3 := []byte("item3")
	root := computeMerkleRoot([][]byte{item1, item2, item3})

	// Build expected tree
	leaf1 := common.Blake2b256Hash(append([]byte{0x00}, item1...))
	leaf2 := common.Blake2b256Hash(append([]byte{0x00}, item2...))
	leaf3 := common.Blake2b256Hash(append([]byte{0x00}, item3...))

	// Level 1: combine (leaf1, leaf2), (leaf3, leaf3)
	branch1Data := make([]byte, 1+64)
	branch1Data[0] = 0x01
	copy(branch1Data[1:33], leaf1[:])
	copy(branch1Data[33:65], leaf2[:])
	branch1 := common.Blake2b256Hash(branch1Data)

	branch2Data := make([]byte, 1+64)
	branch2Data[0] = 0x01
	copy(branch2Data[1:33], leaf3[:])
	copy(branch2Data[33:65], leaf3[:]) // duplicated
	branch2 := common.Blake2b256Hash(branch2Data)

	// Level 2: combine branch1, branch2
	rootData := make([]byte, 1+64)
	rootData[0] = 0x01
	copy(rootData[1:33], branch1[:])
	copy(rootData[33:65], branch2[:])
	expected := common.Blake2b256Hash(rootData)

	if root != expected {
		t.Errorf("three item merkle root mismatch: got %s, want %s", root.String(), expected.String())
	}
}

func TestToUint32(t *testing.T) {
	tests := []struct {
		name        string
		input       any
		expected    uint32
		expectError bool
	}{
		{"uint64 valid", uint64(100), 100, false},
		{"uint64 overflow", uint64(0x100000000), 0, true},
		{"uint32", uint32(200), 200, false},
		{"uint", uint(300), 300, false},
		{"int valid", int(400), 400, false},
		{"int negative", int(-1), 0, true},
		{"int64 valid", int64(500), 500, false},
		{"int64 negative", int64(-1), 0, true},
		{"string invalid", "abc", 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := toUint32(tc.input)
			if tc.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tc.expected {
					t.Errorf("got %d, want %d", result, tc.expected)
				}
			}
		})
	}
}

func TestParseByronBodyProof_InvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input any
	}{
		{"nil", nil},
		{"not a slice", "string"},
		{"wrong slice length", []any{1, 2, 3}},
		{"invalid txProof", []any{"not a slice", []any{}, []byte{}, []byte{}}},
		{"invalid txProof length", []any{[]any{1, 2}, []any{}, []byte{}, []byte{}}},
		{"invalid dlgProof type", []any{[]any{uint64(0), make([]byte, 32), make([]byte, 32)}, []any{}, "not bytes", []byte{}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseByronBodyProof(tc.input)
			if err == nil {
				t.Error("expected error but got nil")
			}
		})
	}
}

func TestParseByronBodyProof_ValidInput(t *testing.T) {
	txBodyRoot := make([]byte, 32)
	txWitRoot := make([]byte, 32)
	sscHash1 := make([]byte, 32)
	sscHash2 := make([]byte, 32)
	dlgProof := make([]byte, 32)
	updProof := make([]byte, 32)

	for i := range txBodyRoot {
		txBodyRoot[i] = byte(i)
		txWitRoot[i] = byte(i + 32)
		sscHash1[i] = byte(i + 64)
		sscHash2[i] = byte(i + 96)
		dlgProof[i] = byte(i + 128)
		updProof[i] = byte(i + 160)
	}

	// Test type 0 (CommitmentsProof) with two hashes
	input := []any{
		[]any{uint64(5), txBodyRoot, txWitRoot}, // txProof
		[]any{uint64(0), sscHash1, sscHash2},    // sscProof: type 0 (Commitments) needs two hashes
		dlgProof,                                // dlgProof
		updProof,                                // updProof
	}

	proof, err := parseByronBodyProof(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if proof.TxProof.TxCount != 5 {
		t.Errorf("TxCount = %d, want 5", proof.TxProof.TxCount)
	}
	if proof.TxProof.TxBodyMerkleRoot != common.Blake2b256(txBodyRoot) {
		t.Error("TxBodyMerkleRoot mismatch")
	}
	if proof.TxProof.TxWitnessMerkleRoot != common.Blake2b256(txWitRoot) {
		t.Error("TxWitnessMerkleRoot mismatch")
	}
	if proof.SscProof.Type != 0 {
		t.Errorf("SscProof.Type = %d, want 0", proof.SscProof.Type)
	}
	if proof.SscProof.Hash1 != common.Blake2b256(sscHash1) {
		t.Error("SscProof.Hash1 mismatch")
	}
	if proof.SscProof.Hash2 == nil {
		t.Error("SscProof.Hash2 should not be nil for type 0")
	} else if *proof.SscProof.Hash2 != common.Blake2b256(sscHash2) {
		t.Error("SscProof.Hash2 mismatch")
	}
	if proof.DlgProof != common.Blake2b256(dlgProof) {
		t.Error("DlgProof mismatch")
	}
	if proof.UpdProof != common.Blake2b256(updProof) {
		t.Error("UpdProof mismatch")
	}
}

func TestParseByronBodyProof_CertificatesProof(t *testing.T) {
	// Test type 3 (CertificatesProof) with only one hash
	txBodyRoot := make([]byte, 32)
	txWitRoot := make([]byte, 32)
	sscHash := make([]byte, 32)
	dlgProof := make([]byte, 32)
	updProof := make([]byte, 32)

	for i := range txBodyRoot {
		txBodyRoot[i] = byte(i)
		txWitRoot[i] = byte(i + 32)
		sscHash[i] = byte(i + 64)
		dlgProof[i] = byte(i + 96)
		updProof[i] = byte(i + 128)
	}

	input := []any{
		[]any{uint64(5), txBodyRoot, txWitRoot}, // txProof
		[]any{uint64(3), sscHash},               // sscProof: type 3 (Certificates) needs one hash
		dlgProof,                                // dlgProof
		updProof,                                // updProof
	}

	proof, err := parseByronBodyProof(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if proof.SscProof.Type != 3 {
		t.Errorf("SscProof.Type = %d, want 3", proof.SscProof.Type)
	}
	if proof.SscProof.Hash1 != common.Blake2b256(sscHash) {
		t.Error("SscProof.Hash1 mismatch")
	}
	if proof.SscProof.Hash2 != nil {
		t.Error("SscProof.Hash2 should be nil for type 3")
	}
}

func TestParseSscProof_InvalidInputs(t *testing.T) {
	tests := []struct {
		name  string
		input any
	}{
		{"nil", nil},
		{"not a slice", "string"},
		{"empty slice", []any{}},
		{"only type", []any{uint64(0)}},
		{"invalid type", []any{uint64(4), make([]byte, 32)}},
		{"type 0 missing second hash", []any{uint64(0), make([]byte, 32)}},
		{"invalid hash1 length", []any{uint64(3), make([]byte, 31)}},
		{"invalid hash2 length", []any{uint64(0), make([]byte, 32), make([]byte, 31)}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSscProof(tc.input)
			if err == nil {
				t.Error("expected error but got nil")
			}
		})
	}
}

func TestParseSscProof_AllTypes(t *testing.T) {
	hash1 := make([]byte, 32)
	hash2 := make([]byte, 32)
	for i := range hash1 {
		hash1[i] = byte(i)
		hash2[i] = byte(i + 32)
	}

	tests := []struct {
		name        string
		input       []any
		expectType  uint64
		expectHash2 bool
	}{
		{"type 0 (Commitments)", []any{uint64(0), hash1, hash2}, 0, true},
		{"type 1 (Openings)", []any{uint64(1), hash1, hash2}, 1, true},
		{"type 2 (Shares)", []any{uint64(2), hash1, hash2}, 2, true},
		{"type 3 (Certificates)", []any{uint64(3), hash1}, 3, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			proof, err := parseSscProof(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if proof.Type != tc.expectType {
				t.Errorf("Type = %d, want %d", proof.Type, tc.expectType)
			}

			if tc.expectHash2 {
				if proof.Hash2 == nil {
					t.Error("Hash2 should not be nil")
				}
			} else {
				if proof.Hash2 != nil {
					t.Error("Hash2 should be nil")
				}
			}
		})
	}
}

// Real mainnet Byron block from cexplorer.io
// https://cexplorer.io/block/1451a0dbf16cfeddf4991a838961df1b08a68f43a19c0eb3b36cc4029c77a2d8
// slot:4471207
// hash:1451a0dbf16cfeddf4991a838961df1b08a68f43a19c0eb3b36cc4029c77a2d8
var testByronMainBlockHex = "83851a2d964a09582025df38df102b89ec25a432a2972993d2fa8cc1f597a73e6260b2f07e79501eb084830258200f284bc22f5b96228ee0687b7bb87c56132f77df4235c78a1595729ccfce2001582019fb988d02ec920a6de5ac71c5d5e75f8b73d7ed8e8abea7773e28859983206e82035820d36a2619a672494604e11bb447cbcf5231e9f2ba25c2169177edc941bd50ad6c5820afc0da64183bf2664f3d4eec7238d524ba607faeeab24fc100eb861dba69971b58204e66280cd94d591072349bec0a3090a53aa945562efb6d08d56e53654b0e4098848218cf0758401bc97a2fe02c297880ce8ecfd997fe4c1ec09ee10feeee9f686760166b05281d6283468ffd93becb0c956ccddd642df9b1244c915911185fa49355f6f22bfab9811a004430ed820282840058401bc97a2fe02c297880ce8ecfd997fe4c1ec09ee10feeee9f686760166b05281d6283468ffd93becb0c956ccddd642df9b1244c915911185fa49355f6f22bfab9584061261a95b7613ee6bf2067dad77b70349729b0c50d57bc1cf30de0db4a1e73a885d0054af7c23fc6c37919dba41c602a57e2d0f9329a7954b867338d6fb2c9455840e03e62f083df5576360e60a32e22bbb07b3c8df4fcab8079f1d6f61af3954d242ba8a06516c395939f24096f3df14e103a7d9c2b80a68a9363cf1f27c7a4e3075840325068a2307397703c4eebb1de1ecab0b23c24a5e80c985e0f7546bb6571ee9eb94069708fc25ec67a4a5753a0d49ab5e536131c19c7f9dd4fd32532fd0f71028483010000826a63617264616e6f2d736c01a058204ba92aa320c60acc9ad7b9a64f2eda55c4d2ec28e604faf186708b4f0c4e8edf849f82839f8200d8185824825820b0a7782d21f37e9d98f4cbdc23bf2677d93eca1ac0fb3f79923863a698d53f8f018200d81858248258205bd3e8385d2ecdd17d3b602263e8a5e7aa0edb4dd00221f369c2720f7d85940d008200d81858248258201e4a77f8375548e5bc409a518dbcb4a8437b539682f4e840f4a1056f01cea566008200d81858248258205e83b53253f705c214d904f65fdaaa2f153db59a229a9cee1da6c329b543236100ff9f8282d818584283581ca1932430cb1ad6482a1b67964d778c18b574674fda151cdfa73c63cda101581e581cfc8a0b5477e819a27a34910e6c174b50b871192e95cca1a711bbceb3001abcb52f6d1b000000013446d5718282d818584283581c093916f7e775fba80eaa65cded085d985f7f9e4982cddd2bb7c476aea101581e581c83d3e2df30edf90acf198b85a7327d964f9d92fd739d0c986a914f6c001a27a611b61a000c48cbffa0848200d8185885825840a781c060f2b32d116ef79bb1823d4a25ea36f6c130b3a182713739ea1819e32d261db3dce30b15c29db81d1c284d3fe350d12241e8ccc65cdf08adba90e0ad4558408eb4c9549a6a044d687d6c04fdee2240994f43966ef113ebb3e76a756e39472badb137c3e0268d34ce6042f76c2534220cc1e061a1a29cce065faf486184cf078200d818588582584085dc150754227f68d1640887f8fa57c93e4cad3499f2cb7b5e8258b0b367dcceaa42bf9ea1cfff73fd0fab44d9e0a36ef61bc5d0f294365316a4e0ed12b40a135840f1233519fa85f3ecbb2deaa9dff2d7e943156d49a7a33603381f2c1779b7f65ea0d39a8dcdd227f5d69b9355ab35df0c43c2abb751c6dd24b107a2c7ac51f5088200d81858858258403559467e9b4a4e47af0388e7224358197e5d39c57c71c391db4a7d480f297d8b86b0746de21dc5dfca2bd8b8fa817c1fa1c3bd3eeaddbfd7a6b270564e416d0c5840b0e33544dcb1895b592a612f5be81242a88226d0612da76099b653f89ce7c5641af14fad696ccd44b58744915291240224fd83a26f103c0717752ea256b4af0b8200d8185885825840572c3ea039ded80f19b0d6841e9ad0d0d1b73242ac98538affbec6e7356192f48eba0291ea1b174f9c42e139ba85ce75656a036ba0993dda605d5a62956dba6558406257e3a27a896268cade4d5371537ed606d3004d6269f87ebe6056b6eff737a2a9ef82d27ba1f9b642ffc622ec27b38e69ed41e272d3de0767cad860d50fa10d82839f8200d8185824825820779a319e0d64b80eaff5ed13d08062b8672fc71ac27e7b30574c1c7972764de202ff9f8282d818582183581c2c0dd53d4e6001e006729fc09d74c5a799d5f93c9f4b74748412a823a0001a1abd89081a769cfd808282d818582183581c05b073f36ee030589a31148838cd47e8d8c8f82fec9fe091c7d53cd8a0001a0c5f26b11b00000016bc0c4c47ffa0818200d8185885825840f129f07bbfd87fd1d3ff5fb32e9a5566e02208f89518e9994048add22074f433424e682a392581268c7544e34e9c54378a8820bdcf7dddce30490bbb2d363b4b5840709a2e70d3803554a15d788235bf56c9567407102be375be5071fa81d4c137047743b5f5abefdbab6b2781822474995dff917213c962ecd111619d75b8534f0aff8203d90102809fff82809fff81a0"

func TestValidateBodyHash_RealMainnetBlock(t *testing.T) {
	// Decode the hex string
	blockBytes, err := hex.DecodeString(testByronMainBlockHex)
	if err != nil {
		t.Fatalf("failed to decode block hex: %v", err)
	}

	// Parse the block
	block, err := byron.NewByronMainBlockFromCbor(
		blockBytes,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	if err != nil {
		t.Fatalf("failed to parse block: %v", err)
	}

	// Test that the body proof can be parsed
	proof, err := parseByronBodyProof(block.BlockHeader.BodyProof)
	if err != nil {
		t.Fatalf("failed to parse body proof: %v", err)
	}

	// Verify transaction count matches
	txCount := len(block.Body.TxPayload)
	if uint32(txCount) != proof.TxProof.TxCount {
		t.Errorf(
			"transaction count mismatch: got %d, header says %d",
			txCount,
			proof.TxProof.TxCount,
		)
	}

	t.Logf("Block has %d transactions", txCount)
	t.Logf("TxBodyMerkleRoot: %s", proof.TxProof.TxBodyMerkleRoot.String())
	t.Logf("TxWitnessMerkleRoot: %s", proof.TxProof.TxWitnessMerkleRoot.String())
	t.Logf("DlgProof: %s", proof.DlgProof.String())
	t.Logf("UpdProof: %s", proof.UpdProof.String())

	// Validate full body hash including tx body merkle root, witness hash,
	// delegation proof, and update proof
	err = ValidateBodyHash(block)
	if err != nil {
		t.Errorf("ValidateBodyHash failed: %v", err)
	}

	// Verify SSC proof parsing works correctly
	t.Logf("SSC Proof Type: %d", proof.SscProof.Type)
	t.Logf("SSC Proof Hash1: %s", proof.SscProof.Hash1.String())
	if proof.SscProof.Hash2 != nil {
		t.Logf("SSC Proof Hash2: %s", proof.SscProof.Hash2.String())
	} else {
		t.Logf("SSC Proof Hash2: nil (correct for type 3)")
	}

	// Verify SSC proof type matches payload type
	sscPayloadVal := block.Body.SscPayload.Value()
	if sscPayload, ok := sscPayloadVal.([]any); ok && len(sscPayload) > 0 {
		if payloadType, ok := sscPayload[0].(uint64); ok {
			if proof.SscProof.Type != payloadType {
				t.Errorf("SSC proof type mismatch: got %d, want %d", proof.SscProof.Type, payloadType)
			} else {
				t.Logf("SSC proof type matches payload type: %d", payloadType)
			}
		}
	}
}

// Test genesis config similar to mainnet for NewByronConfigFromGenesis test
const testByronGenesisJSON = `{
    "avvmDistr": {},
    "blockVersionData": {
        "heavyDelThd": "300000000000",
        "maxBlockSize": "2000000",
        "maxHeaderSize": "2000000",
        "maxProposalSize": "700",
        "maxTxSize": "4096",
        "mpcThd": "20000000000000",
        "scriptVersion": 0,
        "slotDuration": "20000",
        "softforkRule": {
            "initThd": "900000000000000",
            "minThd": "600000000000000",
            "thdDecrement": "50000000000000"
        },
        "txFeePolicy": {
            "multiplier": "43946000000",
            "summand": "155381000000000"
        },
        "unlockStakeEpoch": "18446744073709551615",
        "updateImplicit": "10000",
        "updateProposalThd": "100000000000000",
        "updateVoteThd": "1000000000000"
    },
    "ftsSeed": "76617361206f7061736120736b6f766f726f64612047677572646120626f726f64612070726f766f6461",
    "protocolConsts": {
        "k": 2160,
        "protocolMagic": 764824073,
        "vssMinTTL": 2,
        "vssMaxTTL": 6
    },
    "startTime": 1506203091,
    "bootStakeholders": {
        "af2800c124e599d6dec188a75f8bfde397ebb778163a18240371f2d1": 1
    },
    "heavyDelegation": {
        "1deb82908402c7ee3efeb16f369d97fba316ee621d09b32b8969e54b":{"cert":"c8b39f094dc00608acb2d20ff274cb3e0c022ccb0ce558ea7c1a2d3a32cd54b42cc30d32406bcfbb7f2f86d05d2032848be15b178e3ad776f8b1bc56a671400d","delegatePk":"6MA6A8Cy3b6kGVyvOfQeZp99JR7PIh+7LydcCl1+BdGQ3MJG9WyOM6wANwZuL2ZN2qmF6lKECCZDMI3eT1v+3w==","issuerPk":"UHMxYf2vtsjLb64OJb35VVEFs2eO+wjxd1uekN5PXHe8yM7/+NkBHLJ4so/dyG2bqwmWVtd6eFbHYZEIy/ZXUg==","omega":0},
        "65904a89e6d0e5f881513d1736945e051b76f095eca138ee869d543d":{"cert":"552741f728196e62f218047b944b24ce4d374300d04b9b281426f55aa000d53ded66989ad5ea0908e6ff6492001ff18ece6c7040a934060759e9ae09863bf203","delegatePk":"X93u2t4nFNbbL54RBHQ9LY2Bjs3cMG4XYQjbFMqt1EG0V9WEDGD4hAuZyPeMKQriKdT4Qx5ni6elRcNWB7lN2w==","issuerPk":"C9sfXvPZlAN1k/ImYlXxNKVkZYuy34FLO5zvuW2jT6nIiFkchbdw/TZybV89mRxmiCiv/Hu+CHL9aZE25mTZ2A==","omega":0},
        "5411c7bf87c252609831a337a713e4859668cba7bba70a9c3ef7c398":{"cert":"c946fd596bdb31949aa435390de19a549c9698cad1813e34ff2431bc06190188188f4e84001380713e3f916c7526096e7c4855904bff40385007b81e1e657d0e","delegatePk":"i1Mgdin5ow5LIBUETzN8AXNavmckPBlHDJ2ujHtzJ5gJiHufQg1vcO4enVDBYFKHjlRLZctdRL1pF1ayhM2Cmw==","issuerPk":"mm+jQ8jGw23ho1Vv60Eb/fhwjVr4jehibQ/Gv6Tuu22Zq4PZDWZTGtkSLT+ctLydBbJkSGclMqaNp5b5MoQx/Q==","omega":0}
    },
    "nonAvvmBalances": {},
    "vssCerts": {}
}`

func TestNewByronConfigFromGenesis(t *testing.T) {
	genesis, err := byron.NewByronGenesisFromReader(strings.NewReader(testByronGenesisJSON))
	if err != nil {
		t.Fatalf("Failed to parse genesis: %v", err)
	}

	config, err := NewByronConfigFromGenesis(&genesis)
	if err != nil {
		t.Fatalf("NewByronConfigFromGenesis failed: %v", err)
	}

	// Verify protocol magic
	if config.ProtocolMagic != 764824073 {
		t.Errorf("ProtocolMagic: got %d, want 764824073", config.ProtocolMagic)
	}

	// Verify slots per epoch (10 * K = 10 * 2160 = 21600)
	if config.SlotsPerEpoch != 21600 {
		t.Errorf("SlotsPerEpoch: got %d, want 21600", config.SlotsPerEpoch)
	}

	// Verify slot duration (20000ms = 20s)
	expectedDuration := 20 * time.Second
	if config.SlotDuration != expectedDuration {
		t.Errorf("SlotDuration: got %v, want %v", config.SlotDuration, expectedDuration)
	}

	// Verify security parameter
	if config.SecurityParam != 2160 {
		t.Errorf("SecurityParam: got %d, want 2160", config.SecurityParam)
	}

	// Verify number of genesis keys
	if config.NumGenesisKeys != 3 {
		t.Errorf("NumGenesisKeys: got %d, want 3", config.NumGenesisKeys)
	}

	// Verify key hashes are populated
	if len(config.GenesisKeyHashes) != 3 {
		t.Errorf("GenesisKeyHashes length: got %d, want 3", len(config.GenesisKeyHashes))
	}

	// Verify each key hash is 28 bytes
	for i, h := range config.GenesisKeyHashes {
		if len(h) != 28 {
			t.Errorf("GenesisKeyHashes[%d] length: got %d, want 28", i, len(h))
		}
	}

	// Verify slot leader calculation works
	for slot := uint64(0); slot < 6; slot++ {
		expectedIndex := int(slot % 3)
		index, keyHash := config.SlotLeader(slot)
		if index != expectedIndex {
			t.Errorf("SlotLeader(%d) index: got %d, want %d", slot, index, expectedIndex)
		}
		if keyHash == nil {
			t.Errorf("SlotLeader(%d) returned nil key hash", slot)
		}
	}

	t.Logf("Config created successfully:")
	t.Logf("  ProtocolMagic: %d", config.ProtocolMagic)
	t.Logf("  SlotsPerEpoch: %d", config.SlotsPerEpoch)
	t.Logf("  SlotDuration: %v", config.SlotDuration)
	t.Logf("  SecurityParam: %d", config.SecurityParam)
	t.Logf("  NumGenesisKeys: %d", config.NumGenesisKeys)
}

func TestByronTxFeePolicy_CalculateMinFee(t *testing.T) {
	// Use mainnet-like fee policy values
	// summand: 155381000000000 (scaled by 10^9)
	// multiplier: 43946000000 (scaled by 10^9)
	// Actual values: base fee = 155381 lovelace, per-byte = 43.946 lovelace
	policy := ByronTxFeePolicy{
		Summand:    155381000000000,
		Multiplier: 43946000000,
	}

	tests := []struct {
		name        string
		txSize      uint64
		expectedFee uint64
	}{
		{
			name:        "zero size tx",
			txSize:      0,
			expectedFee: 155381, // Just the base fee (155381000000000 / 10^9)
		},
		{
			name:   "100 byte tx",
			txSize: 100,
			// fee = ceiling((155381000000000 + 43946000000 * 100) / 10^9)
			// fee = ceiling(159775600000000 / 10^9)
			// fee = ceiling(159775.6) = 159776
			expectedFee: 159776,
		},
		{
			name:   "200 byte tx",
			txSize: 200,
			// fee = ceiling((155381000000000 + 43946000000 * 200) / 10^9)
			// fee = ceiling(164170200000000 / 10^9)
			// fee = ceiling(164170.2) = 164171
			expectedFee: 164171,
		},
		{
			name:   "500 byte tx",
			txSize: 500,
			// fee = ceiling((155381000000000 + 43946000000 * 500) / 10^9)
			// fee = ceiling(177354000000000 / 10^9)
			// fee = 177354 (exact, no rounding needed)
			expectedFee: 177354,
		},
		{
			name:   "1000 byte tx",
			txSize: 1000,
			// fee = ceiling((155381000000000 + 43946000000 * 1000) / 10^9)
			// fee = ceiling(199327000000000 / 10^9)
			// fee = 199327
			expectedFee: 199327,
		},
		{
			name:   "4096 byte tx (max Byron tx size)",
			txSize: 4096,
			// fee = ceiling((155381000000000 + 43946000000 * 4096) / 10^9)
			// fee = ceiling((155381000000000 + 180002816000000) / 10^9)
			// fee = ceiling(335383816000000 / 10^9)
			// fee = ceiling(335383.816) = 335384
			expectedFee: 335384,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fee := policy.CalculateMinFee(tc.txSize)
			if fee != tc.expectedFee {
				t.Errorf("CalculateMinFee(%d) = %d, want %d", tc.txSize, fee, tc.expectedFee)
			}
		})
	}
}

func TestByronTxFeePolicy_ZeroPolicy(t *testing.T) {
	// Zero policy should return zero fee
	policy := ByronTxFeePolicy{}
	fee := policy.CalculateMinFee(1000)
	if fee != 0 {
		t.Errorf("CalculateMinFee with zero policy = %d, want 0", fee)
	}
}

func TestByronConfig_CalculateMinFee(t *testing.T) {
	config := ByronConfig{
		TxFeePolicy: ByronTxFeePolicy{
			Summand:    155381000000000,
			Multiplier: 43946000000,
		},
	}

	// Verify the config method delegates correctly
	fee := config.CalculateMinFee(200)
	expected := uint64(164171)
	if fee != expected {
		t.Errorf("config.CalculateMinFee(200) = %d, want %d", fee, expected)
	}
}

func TestNewByronConfigFromGenesis_FeePolicy(t *testing.T) {
	genesis, err := byron.NewByronGenesisFromReader(strings.NewReader(testByronGenesisJSON))
	if err != nil {
		t.Fatalf("Failed to parse genesis: %v", err)
	}

	config, err := NewByronConfigFromGenesis(&genesis)
	if err != nil {
		t.Fatalf("NewByronConfigFromGenesis failed: %v", err)
	}

	// Verify fee policy was extracted
	if config.TxFeePolicy.Summand != 155381000000000 {
		t.Errorf("TxFeePolicy.Summand = %d, want 155381000000000", config.TxFeePolicy.Summand)
	}
	if config.TxFeePolicy.Multiplier != 43946000000 {
		t.Errorf("TxFeePolicy.Multiplier = %d, want 43946000000", config.TxFeePolicy.Multiplier)
	}

	// Verify fee calculation works
	fee := config.CalculateMinFee(200)
	if fee != 164171 {
		t.Errorf("CalculateMinFee(200) = %d, want 164171", fee)
	}

	t.Logf("Fee policy loaded from genesis:")
	t.Logf("  Summand: %d (%.6f ADA base fee)", config.TxFeePolicy.Summand,
		float64(config.TxFeePolicy.Summand)/float64(ByronFeeDivisor)/1_000_000)
	t.Logf("  Multiplier: %d (%.6f lovelace/byte)", config.TxFeePolicy.Multiplier,
		float64(config.TxFeePolicy.Multiplier)/float64(ByronFeeDivisor))
	t.Logf("  Fee for 200-byte tx: %d lovelace (%.6f ADA)", fee, float64(fee)/1_000_000)
}
