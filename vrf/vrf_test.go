// Copyright 2024 Cardano Foundation
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

package vrf

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/blake2b"
)

// Test seed (exactly 32 bytes)
var testSeed = []byte("test_seed_for_vrf_testing!!!_32!")

func TestKeyGen(t *testing.T) {
	pk, sk, err := KeyGen(testSeed)
	if err != nil {
		t.Fatalf("KeyGen failed: %v", err)
	}

	if len(pk) != PublicKeySize {
		t.Errorf(
			"expected public key of %d bytes, got %d",
			PublicKeySize,
			len(pk),
		)
	}
	if len(sk) != SeedSize {
		t.Errorf("expected secret key of %d bytes, got %d", SeedSize, len(sk))
	}
}

func TestKeyGenInvalidSeed(t *testing.T) {
	_, _, err := KeyGen([]byte("short"))
	if err == nil {
		t.Error("expected error for short seed")
	}
}

func TestProveAndVerify(t *testing.T) {
	pk, sk, err := KeyGen(testSeed)
	if err != nil {
		t.Fatalf("KeyGen failed: %v", err)
	}

	alpha := []byte("test input message")
	proof, output, err := Prove(sk, alpha)
	if err != nil {
		t.Fatalf("Prove failed: %v", err)
	}

	if len(proof) != ProofSize {
		t.Errorf("expected proof of %d bytes, got %d", ProofSize, len(proof))
	}
	if len(output) != OutputSize {
		t.Errorf("expected output of %d bytes, got %d", OutputSize, len(output))
	}

	// Verify
	verifiedOutput, err := VerifyAndHash(pk, proof, alpha)
	if err != nil {
		t.Fatalf("VerifyAndHash failed: %v", err)
	}

	if !bytes.Equal(output, verifiedOutput) {
		t.Error("output mismatch between Prove and VerifyAndHash")
	}
}

func TestVerify(t *testing.T) {
	pk, sk, err := KeyGen(testSeed)
	if err != nil {
		t.Fatalf("KeyGen failed: %v", err)
	}

	alpha := []byte("test input")
	proof, output, err := Prove(sk, alpha)
	if err != nil {
		t.Fatalf("Prove failed: %v", err)
	}

	// Verify with correct output
	valid, err := Verify(pk, proof, output, alpha)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if !valid {
		t.Error("expected valid verification")
	}

	// Verify with wrong output
	wrongOutput := make([]byte, OutputSize)
	valid, err = Verify(pk, proof, wrongOutput, alpha)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if valid {
		t.Error("expected invalid verification with wrong output")
	}
}

func TestProofDeterminism(t *testing.T) {
	_, sk, err := KeyGen(testSeed)
	if err != nil {
		t.Fatalf("KeyGen failed: %v", err)
	}

	alpha := []byte("determinism test")

	proof1, output1, err := Prove(sk, alpha)
	if err != nil {
		t.Fatalf("first Prove failed: %v", err)
	}

	proof2, output2, err := Prove(sk, alpha)
	if err != nil {
		t.Fatalf("second Prove failed: %v", err)
	}

	if !bytes.Equal(proof1, proof2) {
		t.Error("proofs are not deterministic")
	}
	if !bytes.Equal(output1, output2) {
		t.Error("outputs are not deterministic")
	}
}

func TestMkInputVrf(t *testing.T) {
	slot := int64(12345)
	eta0 := make([]byte, 32)
	for i := range eta0 {
		eta0[i] = byte(i)
	}

	input, err := MkInputVrf(slot, eta0)
	if err != nil {
		t.Fatalf("MkInputVrf failed: %v", err)
	}
	if len(input) != 32 {
		t.Errorf("expected 32-byte input, got %d", len(input))
	}

	// Same inputs should produce same output
	input2, err := MkInputVrf(slot, eta0)
	if err != nil {
		t.Fatalf("MkInputVrf failed: %v", err)
	}
	if !bytes.Equal(input, input2) {
		t.Error("MkInputVrf not deterministic")
	}

	// Different slot should produce different output
	input3, err := MkInputVrf(slot+1, eta0)
	if err != nil {
		t.Fatalf("MkInputVrf failed: %v", err)
	}
	if bytes.Equal(input, input3) {
		t.Error("expected different outputs for different slots")
	}
}

func TestMkInputVrfInvalidEta0Length(t *testing.T) {
	tests := []struct {
		name string
		eta0 []byte
	}{
		{name: "ZeroLength", eta0: []byte{}},
		{name: "Short", eta0: make([]byte, 31)},
		{name: "Oversized", eta0: make([]byte, 33)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := MkInputVrf(12345, tc.eta0)
			assert.Error(t, err)
		})
	}
}

func TestMkSeedTPraos(t *testing.T) {
	slot := int64(12345)
	eta0 := make([]byte, 32)
	for i := range eta0 {
		eta0[i] = byte(i)
	}

	// MkSeedTPraos with zero seed constant should equal MkInputVrf
	// (XOR with zero is identity)
	zeroSeed := make([]byte, 32)
	tpraosWithZero, err := MkSeedTPraos(slot, eta0, zeroSeed)
	assert.NoError(t, err)
	praos, err := MkInputVrf(slot, eta0)
	assert.NoError(t, err)
	assert.Equal(t, praos, tpraosWithZero, "MkSeedTPraos with zero seed should equal MkInputVrf")

	// MkSeedTPraos with SeedEta should differ from MkInputVrf
	tpraosSeed, err := MkSeedTPraos(slot, eta0, SeedEta())
	assert.NoError(t, err)
	assert.NotEqual(t, praos, tpraosSeed, "TPraos seed should differ from CPraos input")

	// MkSeedTPraos should be deterministic
	tpraosSeed2, err := MkSeedTPraos(slot, eta0, SeedEta())
	assert.NoError(t, err)
	assert.Equal(t, tpraosSeed, tpraosSeed2, "MkSeedTPraos not deterministic")

	// Different seed constants should produce different results
	tpraosL, err := MkSeedTPraos(slot, eta0, SeedL())
	assert.NoError(t, err)
	assert.NotEqual(t, tpraosSeed, tpraosL, "SeedEta and SeedL should produce different results")

	// Verify SeedEta is blake2b-256 of uint64be(0)
	seedEtaVal := SeedEta()
	var uint64beBytes [8]byte
	binary.BigEndian.PutUint64(uint64beBytes[:], 0)
	expected := blake2b.Sum256(uint64beBytes[:])
	assert.Equal(t, expected[:], seedEtaVal, "SeedEta should be blake2b-256(uint64be(0))")

	// Verify XOR property: base XOR seed XOR seed = base
	recovered := make([]byte, len(tpraosSeed))
	for i := range tpraosSeed {
		recovered[i] = tpraosSeed[i] ^ seedEtaVal[i]
	}
	assert.Equal(t, praos, recovered, "XOR inverse should recover base")
}

func TestProofToHash(t *testing.T) {
	_, sk, err := KeyGen(testSeed)
	if err != nil {
		t.Fatalf("KeyGen failed: %v", err)
	}

	alpha := []byte("test")
	proof, output, err := Prove(sk, alpha)
	if err != nil {
		t.Fatalf("Prove failed: %v", err)
	}

	hash, err := ProofToHash(proof)
	if err != nil {
		t.Fatalf("ProofToHash failed: %v", err)
	}

	if !bytes.Equal(output, hash) {
		t.Error("ProofToHash output doesn't match Prove output")
	}
}

func TestProofToHashInvalidProof(t *testing.T) {
	_, err := ProofToHash([]byte("short"))
	if err == nil {
		t.Error("expected error for short proof")
	}
}

// TestWithCardanoVectors tests against known Cardano test vectors
func TestWithCardanoVectors(t *testing.T) {
	// Known test vector from Cardano
	seedHex := "9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60"
	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		t.Fatalf("failed to decode seed hex: %v", err)
	}

	pk, sk, err := KeyGen(seed)
	if err != nil {
		t.Fatalf("KeyGen failed: %v", err)
	}

	// The public key should be deterministic
	expectedPkHex := "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a"
	expectedPk, err := hex.DecodeString(expectedPkHex)
	if err != nil {
		t.Fatalf("failed to decode expected public key hex: %v", err)
	}
	if !bytes.Equal(pk, expectedPk) {
		t.Errorf(
			"public key mismatch:\nexpected: %x\ngot:      %x",
			expectedPk,
			pk,
		)
	}

	// Test proving with empty alpha
	alpha := []byte("")
	proof, _, err := Prove(sk, alpha)
	if err != nil {
		t.Fatalf("Prove failed: %v", err)
	}

	// Verify the proof
	_, err = VerifyAndHash(pk, proof, alpha)
	if err != nil {
		t.Errorf("Verification failed: %v", err)
	}
}
