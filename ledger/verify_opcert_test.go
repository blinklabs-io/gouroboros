// Copyright 2024 Cardano Foundation
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

package ledger

import (
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/kes"
)

// Test seed for deterministic key generation (exactly 32 bytes)
var opCertTestSeed = []byte("test_seed_for_opcert_validate!!X")

// TestCreateAndVerifyOpCert tests creating and verifying an OpCert
func TestCreateAndVerifyOpCert(t *testing.T) {
	// Generate cold key pair
	coldPrivateKey := ed25519.NewKeyFromSeed(opCertTestSeed)
	coldPublicKey := coldPrivateKey.Public().(ed25519.PublicKey)

	// Generate KES key pair
	kesSeed := []byte("kes_seed_for_testing_purposes!!X")
	_, kesVkey, err := kes.KeyGen(kes.CardanoKesDepth, kesSeed)
	if err != nil {
		t.Fatalf("failed to generate KES key: %v", err)
	}

	// Create OpCert
	opCert, err := CreateOpCert(kesVkey, 1, 100, opCertTestSeed)
	if err != nil {
		t.Fatalf("failed to create OpCert: %v", err)
	}

	// Verify OpCert signature
	err = VerifyOpCertSignature(opCert, coldPublicKey)
	if err != nil {
		t.Errorf("OpCert signature verification failed: %v", err)
	}

	// Verify fields
	if opCert.IssueNumber != 1 {
		t.Errorf("expected issue number 1, got %d", opCert.IssueNumber)
	}
	if opCert.KesPeriod != 100 {
		t.Errorf("expected KES period 100, got %d", opCert.KesPeriod)
	}
	if len(opCert.ColdSignature) != 64 {
		t.Errorf(
			"expected 64-byte signature, got %d bytes",
			len(opCert.ColdSignature),
		)
	}
}

// TestVerifyOpCertSignatureWithFullKey tests verification with 64-byte private key
func TestVerifyOpCertSignatureWithFullKey(t *testing.T) {
	// Generate cold key pair
	coldPrivateKey := ed25519.NewKeyFromSeed(opCertTestSeed)
	coldPublicKey := coldPrivateKey.Public().(ed25519.PublicKey)

	// Generate KES key pair
	kesSeed := []byte("kes_seed_for_testing_purposes!!X")
	_, kesVkey, err := kes.KeyGen(kes.CardanoKesDepth, kesSeed)
	if err != nil {
		t.Fatalf("failed to generate KES key: %v", err)
	}

	// Create OpCert with full 64-byte private key
	opCert, err := CreateOpCert(kesVkey, 2, 200, coldPrivateKey)
	if err != nil {
		t.Fatalf("failed to create OpCert: %v", err)
	}

	// Verify OpCert signature
	err = VerifyOpCertSignature(opCert, coldPublicKey)
	if err != nil {
		t.Errorf("OpCert signature verification failed: %v", err)
	}
}

// TestVerifyOpCertSignatureInvalid tests that invalid signatures fail
func TestVerifyOpCertSignatureInvalid(t *testing.T) {
	// Generate cold key pair
	coldPrivateKey := ed25519.NewKeyFromSeed(opCertTestSeed)
	coldPublicKey := coldPrivateKey.Public().(ed25519.PublicKey)

	// Generate different cold key (exactly 32 bytes)
	differentSeed := []byte("different_seed_for_wrong_key!!XX")
	differentPrivateKey := ed25519.NewKeyFromSeed(differentSeed)
	differentPublicKey := differentPrivateKey.Public().(ed25519.PublicKey)

	// Generate KES key pair
	kesSeed := []byte("kes_seed_for_testing_purposes!!X")
	_, kesVkey, err := kes.KeyGen(kes.CardanoKesDepth, kesSeed)
	if err != nil {
		t.Fatalf("failed to generate KES key: %v", err)
	}

	// Create OpCert signed with one key
	opCert, err := CreateOpCert(kesVkey, 1, 100, opCertTestSeed)
	if err != nil {
		t.Fatalf("failed to create OpCert: %v", err)
	}

	// Verify with correct key should pass
	err = VerifyOpCertSignature(opCert, coldPublicKey)
	if err != nil {
		t.Errorf(
			"OpCert signature verification with correct key failed: %v",
			err,
		)
	}

	// Verify with different key should fail
	err = VerifyOpCertSignature(opCert, differentPublicKey)
	if err == nil {
		t.Error("expected signature verification to fail with wrong key")
	}
}

// TestVerifyOpCertSignatureNil tests nil OpCert handling
func TestVerifyOpCertSignatureNil(t *testing.T) {
	coldPublicKey := make([]byte, 32)

	err := VerifyOpCertSignature(nil, coldPublicKey)
	if err == nil {
		t.Error("expected error for nil OpCert")
	}
}

// TestVerifyOpCertSignatureInvalidSizes tests validation of field sizes
func TestVerifyOpCertSignatureInvalidSizes(t *testing.T) {
	coldPublicKey := make([]byte, 32)

	testCases := []struct {
		name    string
		opCert  *OpCert
		coldKey []byte
		errMsg  string
	}{
		{
			name: "invalid kes_vkey size",
			opCert: &OpCert{
				KesVkey:       make([]byte, 16), // wrong size
				ColdSignature: make([]byte, 64),
			},
			coldKey: coldPublicKey,
			errMsg:  "kes_vkey",
		},
		{
			name: "invalid signature size",
			opCert: &OpCert{
				KesVkey:       make([]byte, 32),
				ColdSignature: make([]byte, 32), // wrong size
			},
			coldKey: coldPublicKey,
			errMsg:  "cold_signature",
		},
		{
			name: "invalid cold_vkey size",
			opCert: &OpCert{
				KesVkey:       make([]byte, 32),
				ColdSignature: make([]byte, 64),
			},
			coldKey: make([]byte, 16), // wrong size
			errMsg:  "cold_vkey",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyOpCertSignature(tc.opCert, tc.coldKey)
			if err == nil {
				t.Error("expected error for invalid field size")
			}
			if opCertErr, ok := err.(*OpCertError); ok {
				if opCertErr.Field != tc.errMsg {
					t.Errorf(
						"expected error field %s, got %s",
						tc.errMsg,
						opCertErr.Field,
					)
				}
			} else {
				t.Errorf("expected *OpCertError, got %T", err)
			}
		})
	}
}

// TestValidateKesPeriod tests KES period validation
func TestValidateKesPeriod(t *testing.T) {
	const slotsPerKesPeriod = 129600 // mainnet value
	const maxKesEvolutions = 62      // mainnet value

	testCases := []struct {
		name              string
		opCertKesPeriod   uint64
		currentSlot       uint64
		expectedEvolution uint64
		expectError       bool
		errorContains     string
	}{
		{
			name:              "valid current period equals opcert period",
			opCertKesPeriod:   10,
			currentSlot:       10 * slotsPerKesPeriod,
			expectedEvolution: 0,
			expectError:       false,
		},
		{
			name:              "valid one evolution",
			opCertKesPeriod:   10,
			currentSlot:       11 * slotsPerKesPeriod,
			expectedEvolution: 1,
			expectError:       false,
		},
		{
			name:              "valid at max evolution minus one",
			opCertKesPeriod:   10,
			currentSlot:       (10 + 61) * slotsPerKesPeriod,
			expectedEvolution: 61,
			expectError:       false,
		},
		{
			name:            "invalid future certificate",
			opCertKesPeriod: 100,
			currentSlot:     50 * slotsPerKesPeriod,
			expectError:     true,
			errorContains:   "future",
		},
		{
			name:            "invalid expired certificate",
			opCertKesPeriod: 10,
			currentSlot:     (10 + 62) * slotsPerKesPeriod, // exactly at max
			expectError:     true,
			errorContains:   "expired",
		},
		{
			name:            "invalid well expired certificate",
			opCertKesPeriod: 10,
			currentSlot:     (10 + 100) * slotsPerKesPeriod,
			expectError:     true,
			errorContains:   "expired",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			evolution, err := ValidateKesPeriod(
				tc.opCertKesPeriod,
				tc.currentSlot,
				slotsPerKesPeriod,
				maxKesEvolutions,
			)

			if tc.expectError {
				if err == nil {
					t.Error("expected error but got none")
				} else if tc.errorContains != "" {
					if opCertErr, ok := err.(*OpCertError); ok {
						if !strings.Contains(opCertErr.Error(), tc.errorContains) {
							t.Errorf("expected error to contain %q, got %q", tc.errorContains, opCertErr.Error())
						}
					} else {
						t.Errorf("expected *OpCertError, got %T", err)
					}
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if evolution != tc.expectedEvolution {
					t.Errorf("expected evolution %d, got %d", tc.expectedEvolution, evolution)
				}
			}
		})
	}
}

// TestValidateKesPeriodZeroSlotsPerPeriod tests edge case of zero slots per period
func TestValidateKesPeriodZeroSlotsPerPeriod(t *testing.T) {
	_, err := ValidateKesPeriod(10, 1000, 0, 62)
	if err == nil {
		t.Error("expected error for zero slotsPerKesPeriod")
	}
}

// TestValidateOpCert tests full OpCert validation
func TestValidateOpCert(t *testing.T) {
	const slotsPerKesPeriod = 129600
	const maxKesEvolutions = 62

	// Generate cold key pair
	coldPrivateKey := ed25519.NewKeyFromSeed(opCertTestSeed)
	coldPublicKey := coldPrivateKey.Public().(ed25519.PublicKey)

	// Generate KES key pair
	kesSeed := []byte("kes_seed_for_testing_purposes!!X")
	_, kesVkey, err := kes.KeyGen(kes.CardanoKesDepth, kesSeed)
	if err != nil {
		t.Fatalf("failed to generate KES key: %v", err)
	}

	// Create OpCert at period 10
	opCert, err := CreateOpCert(kesVkey, 1, 10, opCertTestSeed)
	if err != nil {
		t.Fatalf("failed to create OpCert: %v", err)
	}

	testCases := []struct {
		name              string
		currentSlot       uint64
		expectedEvolution uint64
		expectError       bool
	}{
		{
			name:              "valid at creation period",
			currentSlot:       10 * slotsPerKesPeriod,
			expectedEvolution: 0,
			expectError:       false,
		},
		{
			name:              "valid one period later",
			currentSlot:       11 * slotsPerKesPeriod,
			expectedEvolution: 1,
			expectError:       false,
		},
		{
			name:              "valid mid-way through period",
			currentSlot:       10*slotsPerKesPeriod + 50000,
			expectedEvolution: 0,
			expectError:       false,
		},
		{
			name:        "invalid before creation",
			currentSlot: 5 * slotsPerKesPeriod,
			expectError: true,
		},
		{
			name:        "invalid expired",
			currentSlot: (10 + 62) * slotsPerKesPeriod,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			evolution, err := ValidateOpCert(
				opCert,
				coldPublicKey,
				tc.currentSlot,
				slotsPerKesPeriod,
				maxKesEvolutions,
			)

			if tc.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if evolution != tc.expectedEvolution {
					t.Errorf("expected evolution %d, got %d", tc.expectedEvolution, evolution)
				}
			}
		})
	}
}

// TestValidateOpCertWrongColdKey tests validation fails with wrong cold key
func TestValidateOpCertWrongColdKey(t *testing.T) {
	const slotsPerKesPeriod = 129600
	const maxKesEvolutions = 62

	// Generate different cold key (exactly 32 bytes)
	differentSeed := []byte("different_seed_for_wrong_key!!XX")
	differentPrivateKey := ed25519.NewKeyFromSeed(differentSeed)
	differentPublicKey := differentPrivateKey.Public().(ed25519.PublicKey)

	// Generate KES key pair
	kesSeed := []byte("kes_seed_for_testing_purposes!!X")
	_, kesVkey, err := kes.KeyGen(kes.CardanoKesDepth, kesSeed)
	if err != nil {
		t.Fatalf("failed to generate KES key: %v", err)
	}

	// Create OpCert with original key
	opCert, err := CreateOpCert(kesVkey, 1, 10, opCertTestSeed)
	if err != nil {
		t.Fatalf("failed to create OpCert: %v", err)
	}

	// Validate with wrong cold key
	_, err = ValidateOpCert(
		opCert,
		differentPublicKey,
		10*slotsPerKesPeriod,
		slotsPerKesPeriod,
		maxKesEvolutions,
	)

	if err == nil {
		t.Error("expected error when validating with wrong cold key")
	}
}

// TestCreateOpCertInvalidInputs tests CreateOpCert with invalid inputs
func TestCreateOpCertInvalidInputs(t *testing.T) {
	testCases := []struct {
		name    string
		kesVkey []byte
		coldKey []byte
		wantErr bool
	}{
		{
			name:    "invalid kesVkey size",
			kesVkey: make([]byte, 16),
			coldKey: make([]byte, 32),
			wantErr: true,
		},
		{
			name:    "invalid coldKey size",
			kesVkey: make([]byte, 32),
			coldKey: make([]byte, 48),
			wantErr: true,
		},
		{
			name:    "valid 32-byte seed",
			kesVkey: make([]byte, 32),
			coldKey: make([]byte, 32),
			wantErr: false,
		},
		{
			name:    "valid 64-byte private key",
			kesVkey: make([]byte, 32),
			coldKey: make([]byte, 64),
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateOpCert(tc.kesVkey, 1, 100, tc.coldKey)
			if tc.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestOpCertWithTestnetParameters tests with testnet-like parameters
func TestOpCertWithTestnetParameters(t *testing.T) {
	const slotsPerKesPeriod = 3600 // testnet value
	const maxKesEvolutions = 120   // testnet value

	// Generate cold key pair
	coldPrivateKey := ed25519.NewKeyFromSeed(opCertTestSeed)
	coldPublicKey := coldPrivateKey.Public().(ed25519.PublicKey)

	// Generate KES key pair
	kesSeed := []byte("kes_seed_for_testing_purposes!!X")
	_, kesVkey, err := kes.KeyGen(kes.CardanoKesDepth, kesSeed)
	if err != nil {
		t.Fatalf("failed to generate KES key: %v", err)
	}

	// Create OpCert at period 50
	opCert, err := CreateOpCert(kesVkey, 5, 50, opCertTestSeed)
	if err != nil {
		t.Fatalf("failed to create OpCert: %v", err)
	}

	// Validate at period 100 (50 evolutions)
	evolution, err := ValidateOpCert(
		opCert,
		coldPublicKey,
		100*slotsPerKesPeriod,
		slotsPerKesPeriod,
		maxKesEvolutions,
	)

	if err != nil {
		t.Errorf("validation failed: %v", err)
	}
	if evolution != 50 {
		t.Errorf("expected evolution 50, got %d", evolution)
	}

	// Should still be valid at period 169 (119 evolutions)
	evolution, err = ValidateOpCert(
		opCert,
		coldPublicKey,
		169*slotsPerKesPeriod,
		slotsPerKesPeriod,
		maxKesEvolutions,
	)

	if err != nil {
		t.Errorf("validation at max-1 evolution failed: %v", err)
	}
	if evolution != 119 {
		t.Errorf("expected evolution 119, got %d", evolution)
	}

	// Should be expired at period 170 (120 evolutions = max)
	_, err = ValidateOpCert(
		opCert,
		coldPublicKey,
		170*slotsPerKesPeriod,
		slotsPerKesPeriod,
		maxKesEvolutions,
	)

	if err == nil {
		t.Error("expected certificate to be expired at max evolutions")
	}
}
