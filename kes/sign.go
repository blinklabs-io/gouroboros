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

package kes

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"math"

	"golang.org/x/crypto/blake2b"
)

const (
	// Internal constants for key generation
	kesSeedSize       = 32
	kesEd25519KeySize = 32
)

// SecretKey represents a KES secret key for a given depth.
// For depth 6 (Cardano), the key is 608 bytes.
type SecretKey struct {
	Depth  uint64
	Period uint64
	Data   []byte // The raw key bytes
}

// secretKeySize returns the size of a KES secret key for a given depth
func secretKeySize(depth uint64) int {
	if depth == 0 {
		return kesEd25519KeySize
	}
	// Each level adds: seed (32) + left_pk (32) + right_pk (32) = 96 bytes
	// Bounds check to prevent overflow
	if depth > math.MaxInt/96 {
		panic("depth too large")
	}
	return kesEd25519KeySize + int(depth)*96 // #nosec G115
}

// KeyGen generates a KES keypair from a 32-byte seed.
// depth: tree depth (6 for Cardano)
// seed: 32-byte random seed
// Returns (secretKey, publicKey, error)
func KeyGen(depth uint64, seed []byte) (*SecretKey, []byte, error) {
	if len(seed) != kesSeedSize {
		return nil, nil, fmt.Errorf(
			"seed must be %d bytes, got %d",
			kesSeedSize,
			len(seed),
		)
	}

	keySize := secretKeySize(depth)
	data := make([]byte, keySize)

	pubKey, err := keyGenInternal(depth, seed, data)
	if err != nil {
		return nil, nil, err
	}

	sk := &SecretKey{
		Depth:  depth,
		Period: 0,
		Data:   data,
	}

	return sk, pubKey, nil
}

// keyGenInternal recursively generates the secret key structure
// Returns the public key for this subtree
func keyGenInternal(depth uint64, seed []byte, data []byte) ([]byte, error) {
	if depth == 0 {
		// Base case: Ed25519 key
		// Store the seed (which we use to derive the signing key when needed)
		copy(data[0:kesEd25519KeySize], seed)

		// Generate the public key
		privateKey := ed25519.NewKeyFromSeed(seed)
		pubKey := privateKey.Public().(ed25519.PublicKey)
		return pubKey, nil
	}

	// Expand seed into left and right seeds using Blake2b with domain separators
	leftSeed := expandSeed(seed, 0x01)
	rightSeed := expandSeed(seed, 0x02)

	// Calculate sizes for child key
	childKeySize := secretKeySize(depth - 1)

	// Generate left subtree key (this will be the active subtree initially)
	leftPubKey, err := keyGenInternal(depth-1, leftSeed, data[0:childKeySize])
	if err != nil {
		return nil, err
	}

	// For the right subtree, we only need the public key now
	// Store the seed for later when we evolve to the right subtree
	rightPubKey, err := publicKeyFromSeed(depth-1, rightSeed)
	if err != nil {
		return nil, err
	}

	// Layout after child key:
	// [childKeySize:+32] = right seed (to generate right subtree later)
	// [childKeySize+32:+32] = left public key
	// [childKeySize+64:+32] = right public key
	offset := childKeySize
	copy(data[offset:offset+32], rightSeed)
	offset += 32
	copy(data[offset:offset+32], leftPubKey)
	offset += 32
	copy(data[offset:offset+32], rightPubKey)

	// Compute combined public key
	pubKey := HashPair(leftPubKey, rightPubKey)
	return pubKey, nil
}

// publicKeyFromSeed generates only the public key from a seed (no secret key stored)
func publicKeyFromSeed(depth uint64, seed []byte) ([]byte, error) {
	if depth == 0 {
		privateKey := ed25519.NewKeyFromSeed(seed)
		return privateKey.Public().(ed25519.PublicKey), nil
	}

	leftSeed := expandSeed(seed, 0x01)
	rightSeed := expandSeed(seed, 0x02)

	leftPubKey, err := publicKeyFromSeed(depth-1, leftSeed)
	if err != nil {
		return nil, err
	}

	rightPubKey, err := publicKeyFromSeed(depth-1, rightSeed)
	if err != nil {
		return nil, err
	}

	return HashPair(leftPubKey, rightPubKey), nil
}

// expandSeed expands a seed using Blake2b with a domain separator
// The domain separator is prepended to match the Haskell/Rust implementations
func expandSeed(seed []byte, separator byte) []byte {
	h, err := blake2b.New256(nil)
	if err != nil {
		panic(fmt.Sprintf("unexpected error creating blake2b hash: %s", err))
	}
	h.Write([]byte{separator})
	h.Write(seed)
	return h.Sum(nil)
}

// Sign signs a message at the given period.
// Returns the signature (448 bytes for depth 6)
func Sign(sk *SecretKey, period uint64, message []byte) ([]byte, error) {
	if sk == nil {
		return nil, errors.New("secret key is nil")
	}

	maxPeriod := uint64(1) << sk.Depth
	if period >= maxPeriod {
		return nil, fmt.Errorf(
			"period %d exceeds maximum %d for depth %d",
			period,
			maxPeriod-1,
			sk.Depth,
		)
	}

	if period != sk.Period {
		return nil, fmt.Errorf(
			"key is at period %d, cannot sign at period %d",
			sk.Period,
			period,
		)
	}

	// Signature size: 64 (Ed25519) + depth * 64 (public key pairs)
	// Bounds check to prevent overflow (depth is limited by maxPeriod check above)
	sigSize := SigmaSize + int(sk.Depth)*PublicKeySize*2 // #nosec G115
	sig := make([]byte, sigSize)

	err := signInternal(sk.Depth, period, sk.Data, message, sig)
	if err != nil {
		return nil, err
	}

	return sig, nil
}

// signInternal recursively signs the message and builds the signature
func signInternal(
	depth uint64,
	period uint64,
	data []byte,
	message []byte,
	sig []byte,
) error {
	if depth == 0 {
		// Base case: Ed25519 signature
		seed := data[0:kesEd25519KeySize]
		privateKey := ed25519.NewKeyFromSeed(seed)
		signature := ed25519.Sign(privateKey, message)
		copy(sig[0:SigmaSize], signature)
		return nil
	}

	childKeySize := secretKeySize(depth - 1)
	halfPeriod := uint64(1) << (depth - 1)

	// Get the public keys stored at this level
	offset := childKeySize + 32 // Skip child key and seed
	leftPubKey := data[offset : offset+32]
	rightPubKey := data[offset+32 : offset+64]

	// Calculate where to put the public key pair in the signature
	// The signature is built from the leaf up, so child signature comes first
	// Note: depth is bounded by period validation in Sign
	childSigSize := SigmaSize + int(depth-1)*PublicKeySize*2 // #nosec G115
	pkPairOffset := childSigSize

	// Store the public key pair at this level
	copy(sig[pkPairOffset:pkPairOffset+32], leftPubKey)
	copy(sig[pkPairOffset+32:pkPairOffset+64], rightPubKey)

	// Recurse into the appropriate subtree
	if period < halfPeriod {
		// Left subtree
		return signInternal(
			depth-1,
			period,
			data[0:childKeySize],
			message,
			sig[0:childSigSize],
		)
	}
	// Right subtree
	return signInternal(
		depth-1,
		period-halfPeriod,
		data[0:childKeySize],
		message,
		sig[0:childSigSize],
	)
}

// Update evolves the secret key to the next period.
// Returns the updated key, or error if key is exhausted.
func Update(sk *SecretKey) (*SecretKey, error) {
	if sk == nil {
		return nil, errors.New("secret key is nil")
	}

	maxPeriod := uint64(1) << sk.Depth
	newPeriod := sk.Period + 1

	if newPeriod >= maxPeriod {
		return nil, fmt.Errorf(
			"key is exhausted at period %d (max %d)",
			sk.Period,
			maxPeriod-1,
		)
	}

	// Create a copy of the key data
	newData := make([]byte, len(sk.Data))
	copy(newData, sk.Data)

	err := updateInternal(sk.Depth, sk.Period, newData)
	if err != nil {
		return nil, err
	}

	return &SecretKey{
		Depth:  sk.Depth,
		Period: newPeriod,
		Data:   newData,
	}, nil
}

// updateInternal recursively updates the secret key
func updateInternal(depth uint64, period uint64, data []byte) error {
	if depth == 0 {
		// At depth 0, zero out the old key for forward security.
		// This ensures compromising future keys doesn't reveal past signing capability.
		for i := range kesEd25519KeySize {
			data[i] = 0
		}
		return nil
	}

	childKeySize := secretKeySize(depth - 1)
	halfPeriod := uint64(1) << (depth - 1)

	seedOffset := childKeySize
	seed := data[seedOffset : seedOffset+32]

	if period < halfPeriod-1 {
		// Still in left subtree, recurse
		return updateInternal(depth-1, period, data[0:childKeySize])
	} else if period == halfPeriod-1 {
		// Transitioning from left to right subtree
		// Generate the right subtree key from the stored seed
		rightSubtreeData := make([]byte, childKeySize)
		_, err := keyGenInternal(depth-1, seed, rightSubtreeData)
		if err != nil {
			return err
		}

		// Copy the new subtree key into place
		copy(data[0:childKeySize], rightSubtreeData)

		// Zero out the seed (it's been used)
		for i := range 32 {
			data[seedOffset+i] = 0
		}

		return nil
	} else {
		// In right subtree, recurse
		return updateInternal(depth-1, period-halfPeriod, data[0:childKeySize])
	}
}

// PublicKey extracts the public key from a secret key
func PublicKey(sk *SecretKey) []byte {
	if sk == nil {
		return nil
	}

	return publicKeyInternal(sk.Depth, sk.Data)
}

// publicKeyInternal recursively computes the public key
func publicKeyInternal(depth uint64, data []byte) []byte {
	if depth == 0 {
		seed := data[0:kesEd25519KeySize]
		privateKey := ed25519.NewKeyFromSeed(seed)
		return privateKey.Public().(ed25519.PublicKey)
	}

	childKeySize := secretKeySize(depth - 1)
	offset := childKeySize + 32 // Skip child key and seed
	leftPubKey := data[offset : offset+32]
	rightPubKey := data[offset+32 : offset+64]

	return HashPair(leftPubKey, rightPubKey)
}
