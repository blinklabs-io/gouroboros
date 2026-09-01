// Copyright 2026 Blink Labs Software
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

package common

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// CIP-0008 defines message signing using COSE (RFC 8152) structures.
// The core signing primitive is ed25519 via VerifyVKeySignature.
//
// COSE_Sign1 = [protected, unprotected, payload, signature]
//
// The actual signed data is a Sig_structure1:
//   ["Signature1", protected, external_aad, payload]
//
// Protected headers contain at minimum: {1: -8} (alg: EdDSA)
// and optionally {"address": bstr} for Cardano address binding.

const (
	// coseAlgEdDSA is the COSE algorithm identifier for EdDSA (-8).
	coseAlgEdDSA = -8
	// coseHeaderAlg is the COSE header label for algorithm.
	coseHeaderAlg = 1
)

// buildProtectedHeaders builds CBOR-encoded protected headers per CIP-0008.
// At minimum: {1: -8} (algorithm = EdDSA).
// If address is non-nil, also includes {"address": address}.
func buildProtectedHeaders(t *testing.T, address []byte) []byte {
	t.Helper()
	headers := map[any]any{
		coseHeaderAlg: coseAlgEdDSA,
	}
	if len(address) > 0 {
		headers["address"] = address
	}
	encoded, err := cbor.Encode(headers)
	if err != nil {
		t.Fatalf("failed to encode protected headers: %v", err)
	}
	return encoded
}

// buildSigStructure1 builds the Sig_structure1 per RFC 8152 Section 4.4:
//
//	["Signature1", body_protected, external_aad, payload]
func buildSigStructure1(
	t *testing.T,
	protectedHeaders []byte,
	externalAad []byte,
	payload []byte,
) []byte {
	t.Helper()
	if externalAad == nil {
		externalAad = []byte{}
	}
	sigStructure := []any{
		"Signature1",
		protectedHeaders,
		externalAad,
		payload,
	}
	encoded, err := cbor.Encode(sigStructure)
	if err != nil {
		t.Fatalf("failed to encode Sig_structure1: %v", err)
	}
	return encoded
}

// generateKeyPair generates a fresh ed25519 key pair for testing.
func generateKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key pair: %v", err)
	}
	return pub, priv
}

// signCoseSign1 builds a CIP-0008 COSE_Sign1 message and returns
// (protected, signature, sigStructure) for verification tests.
func signCoseSign1(
	t *testing.T,
	priv ed25519.PrivateKey,
	payload []byte,
	address []byte,
) (protectedHeaders []byte, signature []byte, sigStructure []byte) {
	t.Helper()
	protectedHeaders = buildProtectedHeaders(t, address)
	sigStructure = buildSigStructure1(t, protectedHeaders, nil, payload)
	signature = ed25519.Sign(priv, sigStructure)
	return protectedHeaders, signature, sigStructure
}

// TestCIP0008SignWithPaymentKey tests signing and verifying a message
// using an ed25519 payment key, following CIP-0008 COSE_Sign1 structure.
func TestCIP0008SignWithPaymentKey(t *testing.T) {
	pub, priv := generateKeyPair(t)
	message := []byte("Hello, Cardano!")

	// Simulate a Shelley enterprise payment address (type 0x60 | network 0x00)
	paymentAddr := make([]byte, 29)
	paymentAddr[0] = 0x60 // enterprise address, testnet
	hash := Blake2b224Hash(pub)
	copy(paymentAddr[1:], hash[:])

	_, sig, sigStructure := signCoseSign1(t, priv, message, paymentAddr)

	// Verify signature using VerifyVKeySignature (the core CIP-0008 primitive)
	if err := VerifyVKeySignature(pub, sig, sigStructure); err != nil {
		t.Fatalf(
			"valid payment key signature should verify: %v",
			err,
		)
	}
}

// TestCIP0008SignWithStakeKey tests signing and verifying a message
// using an ed25519 stake key, following CIP-0008 COSE_Sign1 structure.
func TestCIP0008SignWithStakeKey(t *testing.T) {
	pub, priv := generateKeyPair(t)
	message := []byte("Stake key signing test")

	// Simulate a Shelley reward/stake address (type 0xE0 | network 0x00)
	stakeAddr := make([]byte, 29)
	stakeAddr[0] = 0xE0 // reward address, testnet
	hash := Blake2b224Hash(pub)
	copy(stakeAddr[1:], hash[:])

	_, sig, sigStructure := signCoseSign1(t, priv, message, stakeAddr)

	if err := VerifyVKeySignature(pub, sig, sigStructure); err != nil {
		t.Fatalf(
			"valid stake key signature should verify: %v",
			err,
		)
	}
}

// TestCIP0008VerifyValidSignature tests that a correctly constructed
// COSE_Sign1 signature passes verification.
func TestCIP0008VerifyValidSignature(t *testing.T) {
	tests := []struct {
		name    string
		message []byte
	}{
		{"simple message", []byte("test message")},
		{"empty message", []byte{}},
		{"binary payload", []byte{0x00, 0xFF, 0xDE, 0xAD, 0xBE, 0xEF}},
		{"long message", make([]byte, 4096)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub, priv := generateKeyPair(t)
			_, sig, sigStructure := signCoseSign1(
				t,
				priv,
				tt.message,
				nil,
			)
			if err := VerifyVKeySignature(pub, sig, sigStructure); err != nil {
				t.Errorf("signature should verify for %q: %v", tt.name, err)
			}
		})
	}
}

// TestCIP0008RejectInvalidSignatures tests that tampered or incorrect
// signatures are properly rejected.
func TestCIP0008RejectInvalidSignatures(t *testing.T) {
	pub, priv := generateKeyPair(t)
	message := []byte("authentic message")
	_, sig, sigStructure := signCoseSign1(t, priv, message, nil)

	t.Run("wrong public key", func(t *testing.T) {
		otherPub, _ := generateKeyPair(t)
		err := VerifyVKeySignature(otherPub, sig, sigStructure)
		if err == nil {
			t.Error("should reject signature with wrong public key")
		}
	})

	t.Run("tampered message", func(t *testing.T) {
		tamperedMsg := []byte("tampered message")
		tamperedSigStructure := buildSigStructure1(
			t,
			buildProtectedHeaders(t, nil),
			nil,
			tamperedMsg,
		)
		// Original sig against tampered sig structure
		err := VerifyVKeySignature(pub, sig, tamperedSigStructure)
		if err == nil {
			t.Error("should reject signature with tampered message")
		}
	})

	t.Run("tampered signature bytes", func(t *testing.T) {
		tamperedSig := make([]byte, len(sig))
		copy(tamperedSig, sig)
		tamperedSig[0] ^= 0xFF
		err := VerifyVKeySignature(pub, tamperedSig, sigStructure)
		if err == nil {
			t.Error("should reject tampered signature")
		}
	})

	t.Run("invalid public key size", func(t *testing.T) {
		err := VerifyVKeySignature([]byte{0x01, 0x02}, sig, sigStructure)
		if err == nil {
			t.Error("should reject invalid public key size")
		}
	})

	t.Run("invalid signature size", func(t *testing.T) {
		err := VerifyVKeySignature(pub, []byte{0x01, 0x02}, sigStructure)
		if err == nil {
			t.Error("should reject invalid signature size")
		}
	})

	t.Run("empty public key", func(t *testing.T) {
		err := VerifyVKeySignature([]byte{}, sig, sigStructure)
		if err == nil {
			t.Error("should reject empty public key")
		}
	})

	t.Run("empty signature", func(t *testing.T) {
		err := VerifyVKeySignature(pub, []byte{}, sigStructure)
		if err == nil {
			t.Error("should reject empty signature")
		}
	})

	t.Run("signature from different message", func(t *testing.T) {
		otherMsg := []byte("other message")
		_, otherSig, _ := signCoseSign1(t, priv, otherMsg, nil)
		// Use otherSig against original sigStructure
		err := VerifyVKeySignature(pub, otherSig, sigStructure)
		if err == nil {
			t.Error(
				"should reject signature created for a different message",
			)
		}
	})
}

// TestCIP0008CoseStructureEncoding tests CBOR encoding/decoding of the
// COSE_Sign1 and Sig_structure1 structures used by CIP-0008.
func TestCIP0008CoseStructureEncoding(t *testing.T) {
	t.Run("Sig_structure1 encoding", func(t *testing.T) {
		protected := buildProtectedHeaders(t, nil)
		payload := []byte("test payload")
		externalAad := []byte{}

		sigStructure := []any{
			"Signature1",
			protected,
			externalAad,
			payload,
		}
		encoded, err := cbor.Encode(sigStructure)
		if err != nil {
			t.Fatalf("encode Sig_structure1: %v", err)
		}
		// Decode back as array
		var decoded []any
		if _, err := cbor.Decode(encoded, &decoded); err != nil {
			t.Fatalf("decode Sig_structure1: %v", err)
		}
		if len(decoded) != 4 {
			t.Fatalf(
				"Sig_structure1 should have 4 elements, got %d",
				len(decoded),
			)
		}
		context, ok := decoded[0].(string)
		if !ok {
			t.Fatalf("context should be string, got %T", decoded[0])
		}
		if context != "Signature1" {
			t.Errorf("context = %q, want %q", context, "Signature1")
		}
	})

	t.Run("COSE_Sign1 array encoding", func(t *testing.T) {
		pub, priv := generateKeyPair(t)
		message := []byte("encode test")
		protected, sig, _ := signCoseSign1(t, priv, message, nil)

		// Build COSE_Sign1 = [protected, unprotected, payload, signature]
		unprotected := map[any]any{
			"hashed": false,
		}
		coseSign1 := []any{
			protected,
			unprotected,
			message,
			sig,
		}
		encoded, err := cbor.Encode(coseSign1)
		if err != nil {
			t.Fatalf("encode COSE_Sign1: %v", err)
		}

		// Decode and verify structure
		var decoded []any
		if _, err := cbor.Decode(encoded, &decoded); err != nil {
			t.Fatalf("decode COSE_Sign1: %v", err)
		}
		if len(decoded) != 4 {
			t.Fatalf(
				"COSE_Sign1 should have 4 elements, got %d",
				len(decoded),
			)
		}

		// Extract and verify the signature from decoded structure
		decodedSig, ok := decoded[3].([]byte)
		if !ok {
			t.Fatalf("signature should be []byte, got %T", decoded[3])
		}
		sigStructure := buildSigStructure1(t, protected, nil, message)
		if err := VerifyVKeySignature(
			pub,
			decodedSig,
			sigStructure,
		); err != nil {
			t.Errorf(
				"signature from decoded COSE_Sign1 should verify: %v",
				err,
			)
		}
	})
}

// TestCIP0008HashedPayload tests the CIP-0008 hashed payload mode where
// the Blake2b224 hash of the payload is signed instead of the raw payload.
func TestCIP0008HashedPayload(t *testing.T) {
	pub, priv := generateKeyPair(t)
	largePayload := make([]byte, 8192)
	for i := range largePayload {
		largePayload[i] = byte(i % 256)
	}

	// Hash the payload per CIP-0008 (Blake2b224)
	payloadHash := Blake2b224Hash(largePayload)

	// Sign the hash instead of the full payload
	protected := buildProtectedHeaders(t, nil)
	sigStructure := buildSigStructure1(t, protected, nil, payloadHash[:])
	sig := ed25519.Sign(priv, sigStructure)

	// Verify with the hashed payload
	if err := VerifyVKeySignature(pub, sig, sigStructure); err != nil {
		t.Fatalf("hashed payload signature should verify: %v", err)
	}

	// Verify it does NOT verify against the raw (unhashed) payload
	rawSigStructure := buildSigStructure1(t, protected, nil, largePayload)
	if err := VerifyVKeySignature(pub, sig, rawSigStructure); err == nil {
		t.Error(
			"hashed payload signature should NOT verify against raw payload",
		)
	}
}

// TestCIP0008ExternalAad tests that the external_aad field in
// Sig_structure1 affects signature verification as specified by RFC 8152.
func TestCIP0008ExternalAad(t *testing.T) {
	pub, priv := generateKeyPair(t)
	message := []byte("aad test")
	protected := buildProtectedHeaders(t, nil)
	externalAad := []byte("application-specific-context")

	sigStructure := buildSigStructure1(t, protected, externalAad, message)
	sig := ed25519.Sign(priv, sigStructure)

	// Verify with correct external_aad
	if err := VerifyVKeySignature(pub, sig, sigStructure); err != nil {
		t.Fatalf("should verify with correct external_aad: %v", err)
	}

	// Should fail with different external_aad
	wrongAad := buildSigStructure1(
		t,
		protected,
		[]byte("wrong-context"),
		message,
	)
	if err := VerifyVKeySignature(pub, sig, wrongAad); err == nil {
		t.Error("should reject signature with wrong external_aad")
	}

	// Should fail with empty external_aad when one was used
	noAad := buildSigStructure1(t, protected, nil, message)
	if err := VerifyVKeySignature(pub, sig, noAad); err == nil {
		t.Error("should reject signature when external_aad is missing")
	}
}

// TestCIP0008AddressBinding tests that embedding the address in the
// protected headers binds the signature to a specific address, preventing
// replay across different addresses.
func TestCIP0008AddressBinding(t *testing.T) {
	pub, priv := generateKeyPair(t)
	message := []byte("address-bound message")

	addr1 := make([]byte, 29)
	addr1[0] = 0x60 // enterprise, testnet
	hash := Blake2b224Hash(pub)
	copy(addr1[1:], hash[:])

	addr2 := make([]byte, 29)
	addr2[0] = 0x61 // enterprise, mainnet
	copy(addr2[1:], hash[:])

	// Sign with addr1 in protected headers
	_, sig, sigStructure := signCoseSign1(t, priv, message, addr1)

	// Verify against addr1 sig structure — should pass
	if err := VerifyVKeySignature(pub, sig, sigStructure); err != nil {
		t.Fatalf("should verify with correct address: %v", err)
	}

	// Verify against addr2 sig structure — should fail (different address)
	sigStructureAddr2 := buildSigStructure1(
		t,
		buildProtectedHeaders(t, addr2),
		nil,
		message,
	)
	if err := VerifyVKeySignature(pub, sig, sigStructureAddr2); err == nil {
		t.Error(
			"should reject signature when address differs in protected headers",
		)
	}
}
