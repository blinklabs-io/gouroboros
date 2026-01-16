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
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger/common"
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
			if tc.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
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

func TestValidateHeaderFull(t *testing.T) {
	config := testByronConfig()
	validator := NewHeaderValidator(config)

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
