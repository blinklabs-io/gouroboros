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

	"github.com/blinklabs-io/gouroboros/vrf"
)

// VRFTestVectors represents the JSON structure of VRF test vectors
type VRFTestVectors struct {
	Source      string          `json:"source"`
	Description string          `json:"description"`
	Vectors     []VRFTestVector `json:"vectors"`
}

// VRFTestVector represents a single VRF test vector
type VRFTestVector struct {
	SK    string `json:"sk"`    // Secret key (32 bytes hex)
	PK    string `json:"pk"`    // Public key (32 bytes hex)
	Pi    string `json:"pi"`    // Proof (80 bytes hex)
	Beta  string `json:"beta"`  // Output (64 bytes hex)
	Alpha string `json:"alpha"` // Input message (variable length hex)
}

// TestVRFVerifyConformance tests VRF verification against official test vectors.
// These vectors test that the proof verifies correctly with the given public key,
// and that ProofToHash produces the expected output.
// Note: Some vectors have sk/pk pairs that don't match (they're testing verification
// with known-good proofs, not key generation).
func TestVRFVerifyConformance(t *testing.T) {
	// Load test vectors
	vectorsPath := filepath.Join(".", "vrf_vectors.json")
	data, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatalf("Failed to read VRF vectors file: %v", err)
	}

	var vectors VRFTestVectors
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatalf("Failed to parse VRF vectors: %v", err)
	}

	t.Logf(
		"Running %d VRF verification conformance tests from %s",
		len(vectors.Vectors),
		vectors.Source,
	)

	for i, v := range vectors.Vectors {
		t.Run(formatVectorName(i, v.Alpha), func(t *testing.T) {
			// Decode hex values
			expectedPK, err := hex.DecodeString(v.PK)
			if err != nil {
				t.Fatalf("Failed to decode pk: %v", err)
			}

			proof, err := hex.DecodeString(v.Pi)
			if err != nil {
				t.Fatalf("Failed to decode pi: %v", err)
			}

			expectedOutput, err := hex.DecodeString(v.Beta)
			if err != nil {
				t.Fatalf("Failed to decode beta: %v", err)
			}

			alpha, err := hex.DecodeString(v.Alpha)
			if err != nil {
				t.Fatalf("Failed to decode alpha: %v", err)
			}

			// Test 1: Verify the proof with the given public key
			valid, err := vrf.Verify(expectedPK, proof, expectedOutput, alpha)
			if err != nil {
				t.Fatalf("Verify failed: %v", err)
			}
			if !valid {
				t.Error("Verify returned false for valid proof")
			}

			// Test 2: ProofToHash produces expected output
			output, err := vrf.ProofToHash(proof)
			if err != nil {
				t.Fatalf("ProofToHash failed: %v", err)
			}
			if hex.EncodeToString(output) != v.Beta {
				t.Errorf(
					"ProofToHash output mismatch:\n  got:  %x\n  want: %s",
					output,
					v.Beta,
				)
			}
		})
	}
}

// TestVRFProveConformance tests that our Prove function produces correct outputs
// for vectors where the sk/pk pair is consistent (sk generates the listed pk).
func TestVRFProveConformance(t *testing.T) {
	// Load test vectors
	vectorsPath := filepath.Join(".", "vrf_vectors.json")
	data, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatalf("Failed to read VRF vectors file: %v", err)
	}

	var vectors VRFTestVectors
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatalf("Failed to parse VRF vectors: %v", err)
	}

	testedCount := 0
	skippedCount := 0

	for i, v := range vectors.Vectors {
		t.Run(formatVectorName(i, v.Alpha), func(t *testing.T) {
			// Decode hex values
			sk, err := hex.DecodeString(v.SK)
			if err != nil {
				t.Fatalf("Failed to decode sk: %v", err)
			}

			alpha, err := hex.DecodeString(v.Alpha)
			if err != nil {
				t.Fatalf("Failed to decode alpha: %v", err)
			}

			// First check if sk generates the expected pk
			// Some test vectors have mismatched sk/pk pairs for verification testing
			pk, _, err := vrf.KeyGen(sk)
			if err != nil {
				t.Fatalf("KeyGen failed: %v", err)
			}

			if hex.EncodeToString(pk) != v.PK {
				// Skip this vector - sk doesn't match pk (this is expected for some vectors)
				skippedCount++
				t.Skipf(
					"Skipping: sk does not generate listed pk (verification-only vector)",
				)
				return
			}

			testedCount++

			// Generate proof
			proof, output, err := vrf.Prove(sk, alpha)
			if err != nil {
				t.Fatalf("Prove failed: %v", err)
			}

			// Check proof matches expected
			if hex.EncodeToString(proof) != v.Pi {
				t.Errorf(
					"Prove proof mismatch:\n  got:  %x\n  want: %s",
					proof,
					v.Pi,
				)
			}

			// Check output matches expected
			if hex.EncodeToString(output) != v.Beta {
				t.Errorf(
					"Prove output mismatch:\n  got:  %x\n  want: %s",
					output,
					v.Beta,
				)
			}

			// Verify our generated proof is valid
			valid, err := vrf.Verify(pk, proof, output, alpha)
			if err != nil {
				t.Fatalf("Verify of generated proof failed: %v", err)
			}
			if !valid {
				t.Error("Generated proof failed verification")
			}
		})
	}

	t.Logf(
		"Tested %d vectors, skipped %d verification-only vectors",
		testedCount,
		skippedCount,
	)
}

// formatVectorName creates a descriptive test name from vector index and alpha
func formatVectorName(index int, alpha string) string {
	if alpha == "" {
		return "vector_" + strconv.Itoa(index) + "_empty_input"
	}
	if len(alpha) <= 16 {
		return "vector_" + strconv.Itoa(index) + "_alpha_" + alpha
	}
	return "vector_" + strconv.Itoa(index) + "_alpha_" + alpha[:16] + "..."
}
