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
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/stretchr/testify/require"
)

// KESTestVectors represents the JSON structure of KES test vectors
type KESTestVectors struct {
	Source      string          `json:"source"`
	Description string          `json:"description"`
	Note        string          `json:"note"`
	Parameters  KESParameters   `json:"parameters"`
	Vectors     []KESTestVector `json:"vectors"`
}

// KESParameters contains the test parameters
type KESParameters struct {
	Depth         uint64 `json:"depth"`
	MaxPeriod     uint64 `json:"max_period"`
	SignatureSize int    `json:"signature_size"`
	Seed          string `json:"seed"`
	SeedHex       string `json:"seed_hex"`
	Message       string `json:"message"`
	MessageHex    string `json:"message_hex"`
}

// KESTestVector represents a single KES test vector
type KESTestVector struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Period      uint64 `json:"period"`
	Signature   string `json:"signature"`
}

// TestKESVerifyConformance tests KES signature verification against official test vectors
func TestKESVerifyConformance(t *testing.T) {
	// Load test vectors
	vectorsPath := filepath.Join(".", "kes_vectors.json")
	data, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatalf("Failed to read KES vectors file: %v", err)
	}

	var vectors KESTestVectors
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatalf("Failed to parse KES vectors: %v", err)
	}

	t.Logf(
		"Running %d KES verification conformance tests from %s",
		len(vectors.Vectors),
		vectors.Source,
	)

	// Parse parameters
	seed := []byte(vectors.Parameters.Seed)
	if len(seed) != 32 {
		t.Fatalf("Seed must be 32 bytes, got %d", len(seed))
	}

	message := []byte(vectors.Parameters.Message)

	// Generate key to get public key
	sk, pubKey, err := kes.KeyGen(vectors.Parameters.Depth, seed)
	if err != nil {
		t.Fatalf("KeyGen failed: %v", err)
	}
	_ = sk // We only need pubKey for verification

	for _, v := range vectors.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			// Decode expected signature
			expectedSig, err := hex.DecodeString(v.Signature)
			if err != nil {
				t.Fatalf("Failed to decode signature: %v", err)
			}

			if len(expectedSig) != vectors.Parameters.SignatureSize {
				t.Fatalf(
					"Signature size mismatch: got %d, want %d",
					len(expectedSig),
					vectors.Parameters.SignatureSize,
				)
			}

			// Verify the signature
			valid := kes.VerifySignedKES(pubKey, v.Period, message, expectedSig)
			if !valid {
				t.Errorf(
					"Signature verification failed for period %d",
					v.Period,
				)
			}
		})
	}
}

// TestKESSignConformance tests KES signature generation against official test vectors
func TestKESSignConformance(t *testing.T) {
	// Load test vectors
	vectorsPath := filepath.Join(".", "kes_vectors.json")
	data, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatalf("Failed to read KES vectors file: %v", err)
	}

	var vectors KESTestVectors
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatalf("Failed to parse KES vectors: %v", err)
	}

	// Parse parameters
	seed := []byte(vectors.Parameters.Seed)
	if len(seed) != 32 {
		t.Fatalf("Seed must be 32 bytes, got %d", len(seed))
	}

	message := []byte(vectors.Parameters.Message)

	for _, v := range vectors.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			// Generate fresh key for each test
			sk, pubKey, err := kes.KeyGen(vectors.Parameters.Depth, seed)
			if err != nil {
				t.Fatalf("KeyGen failed: %v", err)
			}

			// Evolve key to target period
			for i := uint64(0); i < v.Period; i++ {
				sk, err = kes.Update(sk)
				if err != nil {
					t.Fatalf("Update failed at period %d: %v", i, err)
				}
			}

			// Sign the message
			sig, err := kes.Sign(sk, v.Period, message)
			if err != nil {
				t.Fatalf("Sign failed: %v", err)
			}

			// Decode expected signature
			expectedSig, err := hex.DecodeString(v.Signature)
			if err != nil {
				t.Fatalf("Failed to decode expected signature: %v", err)
			}

			// Compare signatures
			if hex.EncodeToString(sig) != v.Signature {
				t.Errorf(
					"Signature mismatch at period %d:\n  got:  %x\n  want: %s",
					v.Period,
					sig,
					v.Signature,
				)
			}

			// Verify our generated signature
			valid := kes.VerifySignedKES(pubKey, v.Period, message, sig)
			if !valid {
				t.Error("Generated signature failed verification")
			}

			// Verify expected signature
			valid = kes.VerifySignedKES(pubKey, v.Period, message, expectedSig)
			if !valid {
				t.Error("Expected signature failed verification")
			}
		})
	}
}

// TestKESKeyEvolution tests that key evolution works correctly
func TestKESKeyEvolution(t *testing.T) {
	// Load test vectors
	vectorsPath := filepath.Join(".", "kes_vectors.json")
	data, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatalf("Failed to read KES vectors file: %v", err)
	}

	var vectors KESTestVectors
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatalf("Failed to parse KES vectors: %v", err)
	}

	seed := []byte(vectors.Parameters.Seed)
	message := []byte(vectors.Parameters.Message)

	// Generate initial key
	sk, pubKey, err := kes.KeyGen(vectors.Parameters.Depth, seed)
	if err != nil {
		t.Fatalf("KeyGen failed: %v", err)
	}

	// Test that public key remains constant after evolution
	initialPubKey := make([]byte, len(pubKey))
	copy(initialPubKey, pubKey)

	// Evolve through multiple periods and verify signatures
	for period := range uint64(10) {
		t.Run("period_"+strconv.Itoa(int(period)), func(t *testing.T) {
			// Sign at current period
			sig, err := kes.Sign(sk, period, message)
			require.NoErrorf(t, err, "Sign failed at period %d", period)

			// Verify signature
			valid := kes.VerifySignedKES(pubKey, period, message, sig)
			require.Truef(
				t,
				valid,
				"Signature verification failed at period %d",
				period,
			)

			// Verify public key hasn't changed
			currentPubKey := kes.PublicKey(sk)
			require.Equalf(
				t,
				hex.EncodeToString(initialPubKey),
				hex.EncodeToString(currentPubKey),
				"Public key changed after evolution to period %d",
				period,
			)
		})

		// Evolve to next period
		if period < 9 {
			var err error
			sk, err = kes.Update(sk)
			require.NoErrorf(t, err, "Update failed at period %d", period)
		}
	}
}

// TestKESSignatureSize verifies signature sizes match expected values
func TestKESSignatureSize(t *testing.T) {
	// Load test vectors
	vectorsPath := filepath.Join(".", "kes_vectors.json")
	data, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatalf("Failed to read KES vectors file: %v", err)
	}

	var vectors KESTestVectors
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatalf("Failed to parse KES vectors: %v", err)
	}

	// Verify expected signature size for depth 6
	expectedSize := kes.SignatureSize(vectors.Parameters.Depth)
	if expectedSize != vectors.Parameters.SignatureSize {
		t.Errorf(
			"Signature size mismatch: got %d, want %d",
			expectedSize,
			vectors.Parameters.SignatureSize,
		)
	}

	// Also verify against constant
	if expectedSize != kes.CardanoKesSignatureSize {
		t.Errorf(
			"Signature size doesn't match Cardano constant: got %d, want %d",
			expectedSize,
			kes.CardanoKesSignatureSize,
		)
	}
}
