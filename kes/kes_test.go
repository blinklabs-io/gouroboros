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

package kes

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test seed (exactly 32 bytes)
// Note: "lenght" typo is intentional - matches canonical test vectors from input-output-hk/kes
var testSeed = []byte("test string of 32 byte of lenght")

func TestKeyGen(t *testing.T) {
	sk, pk, err := KeyGen(CardanoKesDepth, testSeed)
	require.NoError(t, err, "KeyGen failed")
	require.NotNil(t, sk, "secret key is nil")
	require.Len(t, pk, PublicKeySize, "public key wrong size")
	require.EqualValues(t, CardanoKesDepth, sk.Depth, "wrong depth")
	require.Equal(t, uint64(0), sk.Period, "initial period should be 0")
}

func TestKeyGenInvalidSeed(t *testing.T) {
	_, _, err := KeyGen(CardanoKesDepth, []byte("short"))
	require.Error(t, err, "expected error for short seed")
}

func TestSignAndVerify(t *testing.T) {
	sk, pk, err := KeyGen(CardanoKesDepth, testSeed)
	require.NoError(t, err, "KeyGen failed")

	message := []byte("test message")
	sig, err := Sign(sk, 0, message)
	require.NoError(t, err, "Sign failed")
	require.Len(t, sig, CardanoKesSignatureSize, "signature wrong size")

	// Verify
	kesSig, err := NewSumKesFromBytes(CardanoKesDepth, sig)
	require.NoError(t, err, "NewSumKesFromBytes failed")
	require.True(
		t,
		kesSig.Verify(0, pk, message),
		"signature verification failed",
	)
}

func TestSignWrongPeriod(t *testing.T) {
	sk, _, err := KeyGen(CardanoKesDepth, testSeed)
	require.NoError(t, err, "KeyGen failed")

	// Try to sign at period 1 while key is at period 0
	_, err = Sign(sk, 1, []byte("test"))
	require.Error(t, err, "expected error when signing at wrong period")
}

func TestUpdate(t *testing.T) {
	sk, pk, err := KeyGen(CardanoKesDepth, testSeed)
	require.NoError(t, err, "KeyGen failed")

	// Update to period 1
	sk1, err := Update(sk)
	require.NoError(t, err, "Update failed")
	require.Equal(t, uint64(1), sk1.Period, "expected period 1")

	// Verify we can sign at period 1
	message := []byte("test message at period 1")
	sig, err := Sign(sk1, 1, message)
	require.NoError(t, err, "Sign at period 1 failed")

	kesSig, err := NewSumKesFromBytes(CardanoKesDepth, sig)
	require.NoError(t, err, "NewSumKesFromBytes failed")
	require.True(
		t,
		kesSig.Verify(1, pk, message),
		"signature verification at period 1 failed",
	)
}

func TestPublicKeyExtraction(t *testing.T) {
	sk, pk, err := KeyGen(CardanoKesDepth, testSeed)
	require.NoError(t, err, "KeyGen failed")

	extractedPk := PublicKey(sk)
	require.True(
		t,
		bytes.Equal(pk, extractedPk),
		"extracted public key does not match generated public key",
	)
}

func TestPublicKeyAfterUpdate(t *testing.T) {
	sk, pk, err := KeyGen(CardanoKesDepth, testSeed)
	require.NoError(t, err, "KeyGen failed")

	// Update key
	sk1, err := Update(sk)
	require.NoError(t, err, "Update failed")

	// Public key should remain the same
	extractedPk := PublicKey(sk1)
	require.True(
		t,
		bytes.Equal(pk, extractedPk),
		"public key changed after update",
	)
}

func TestMaxPeriod(t *testing.T) {
	expected := uint64(1 << CardanoKesDepth) // 64 for depth 6
	require.Equal(
		t,
		expected,
		MaxPeriod(CardanoKesDepth),
		"max period mismatch",
	)
}

func TestVerifySignedKES(t *testing.T) {
	sk, pk, err := KeyGen(CardanoKesDepth, testSeed)
	require.NoError(t, err, "KeyGen failed")

	message := []byte("test message")
	sig, err := Sign(sk, 0, message)
	require.NoError(t, err, "Sign failed")
	require.True(
		t,
		VerifySignedKES(pk, 0, message, sig),
		"VerifySignedKES failed",
	)
}

func TestHashPair(t *testing.T) {
	left := make([]byte, PublicKeySize)
	right := make([]byte, PublicKeySize)
	for i := range left {
		left[i] = byte(i)
		right[i] = byte(i + 32)
	}

	hash := HashPair(left, right)
	require.Len(t, hash, PublicKeySize, "hash wrong size")

	// Same inputs should produce same hash
	hash2 := HashPair(left, right)
	require.True(t, bytes.Equal(hash, hash2), "HashPair not deterministic")
}
