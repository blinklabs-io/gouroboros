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

// Package kes implements Key-Evolving Signatures (KES) as used in Cardano's
// Praos consensus protocol. KES provides forward-secure signatures where
// compromising the current secret key does not compromise past signatures.
//
// This implementation uses the MMM (Malkin-Micciancio-Miner) sum composition
// scheme with Ed25519 as the base signature scheme. Cardano uses depth 6
// (Sum6Kes), providing 64 time periods.
package kes

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"math"

	"golang.org/x/crypto/blake2b"
)

const (
	// SigmaSize is the size of an Ed25519 signature
	SigmaSize = 64

	// PublicKeySize is the size of an Ed25519 public key
	PublicKeySize = 32

	// Sum0KesSigSize is the size of a depth-0 KES signature (just Ed25519)
	Sum0KesSigSize = 64

	// CardanoKesDepth is the fixed KES tree depth used in Cardano's consensus protocol
	CardanoKesDepth = 6

	// CardanoKesSignatureSize is the expected KES signature size: 64 + depth*64
	CardanoKesSignatureSize = 448

	// CardanoKesSecretKeySize is the size of a KES secret key for depth 6
	// Layout: 32 (Ed25519 seed) + 6 * (32 seed + 32 left_pk + 32 right_pk) = 608
	CardanoKesSecretKeySize = 608

	// SeedSize is the size of a KES seed
	SeedSize = 32
)

// SumXKesSig represents a KES signature at a given depth
type SumXKesSig struct {
	Depth                  uint64
	Sigma                  any
	LeftHandSidePublicKey  ed25519.PublicKey
	RightHandSidePublicKey ed25519.PublicKey
}

// Sum0KesSig represents a depth-0 KES signature (just an Ed25519 signature)
type Sum0KesSig []byte

// NewSumKesFromBytes deserializes a KES signature from bytes
func NewSumKesFromBytes(depth uint64, fromByte []byte) (SumXKesSig, error) {
	if depth == 0 {
		return SumXKesSig{}, errors.New("depth must be at least 1")
	}
	kesSize := SigmaSize + depth*(PublicKeySize*2)
	if kesSize > math.MaxInt {
		return SumXKesSig{}, fmt.Errorf(
			"kes size too large for depth %d",
			depth,
		)
	}
	if len(fromByte) != int(kesSize) {
		return SumXKesSig{}, fmt.Errorf(
			"expected %d bytes, got %d",
			kesSize,
			len(fromByte),
		)
	}
	nextKesSize := SigmaSize + (depth-1)*(PublicKeySize*2)
	var sigma any
	if depth == 1 {
		sigma = Sum0KesSigFromBytes(fromByte)
	} else {
		var err error
		sigma, err = NewSumKesFromBytes(depth-1, fromByte[0:nextKesSize])
		if err != nil {
			return SumXKesSig{}, err
		}
	}
	return SumXKesSig{
		depth,
		sigma,
		fromByte[nextKesSize : nextKesSize+PublicKeySize],
		fromByte[nextKesSize+PublicKeySize : nextKesSize+PublicKeySize*2],
	}, nil
}

// Sum0KesSigFromBytes creates a Sum0KesSig from bytes.
// Returns nil if sigBytes is too short.
func Sum0KesSigFromBytes(sigBytes []byte) Sum0KesSig {
	if len(sigBytes) < Sum0KesSigSize {
		return nil
	}
	return sigBytes[0:Sum0KesSigSize]
}

// Verify verifies a KES signature.
// Returns false if the period is out of range for this signature's depth.
func (s SumXKesSig) Verify(
	period uint64,
	pubKey ed25519.PublicKey,
	msg []byte,
) bool {
	// Reject out-of-range periods - max valid period is 2^depth - 1
	maxPeriod := uint64(1) << s.Depth
	if period >= maxPeriod {
		return false
	}

	pk2 := HashPair(s.LeftHandSidePublicKey, s.RightHandSidePublicKey)
	if !bytes.Equal(pk2, pubKey) {
		return false
	}

	nextDepth := uint64(1) << (s.Depth - 1)
	sigma := s.Sigma
	nextPeriod := period
	nextPk := s.LeftHandSidePublicKey
	if period >= nextDepth {
		nextPeriod = period - nextDepth
		nextPk = s.RightHandSidePublicKey
	}

	switch sumX := sigma.(type) {
	case SumXKesSig:
		return sumX.Verify(nextPeriod, nextPk, msg)
	case Sum0KesSig:
		return sumX.Verify(nextPeriod, nextPk, msg)
	default:
		return false
	}
}

// Verify verifies a depth-0 KES signature (Ed25519)
func (s Sum0KesSig) Verify(
	_ uint64,
	pubKey ed25519.PublicKey,
	msg []byte,
) bool {
	return ed25519.Verify(pubKey, msg, s)
}

// HashPair computes the Blake2b-256 hash of two public keys concatenated
func HashPair(l ed25519.PublicKey, r ed25519.PublicKey) ed25519.PublicKey {
	h, err := blake2b.New(32, nil)
	if err != nil {
		panic(
			fmt.Sprintf(
				"unexpected error creating empty blake2b hash: %s",
				err,
			),
		)
	}
	h.Write(l[:])
	h.Write(r[:])
	return h.Sum(nil)
}

// SignatureSize returns the size of a KES signature for a given depth.
// Returns -1 if the result would overflow int (for extremely large depths).
func SignatureSize(depth uint64) int {
	// Check for overflow before conversion
	// Each level adds 2 * PublicKeySize (64 bytes)
	bytesPerLevel := uint64(PublicKeySize * 2) // 64
	additionalBytes := depth * bytesPerLevel
	// Check if multiplication overflowed
	if depth > 0 && additionalBytes/depth != bytesPerLevel {
		return -1
	}
	totalBytes := uint64(SigmaSize) + additionalBytes
	// Check if addition overflowed or exceeds max int
	if totalBytes < uint64(SigmaSize) || totalBytes > uint64(math.MaxInt) {
		return -1
	}
	return int(totalBytes)
}

// MaxPeriod returns the maximum period for a given depth
func MaxPeriod(depth uint64) uint64 {
	return uint64(1) << depth
}

// VerifySignedKES verifies a KES signature at the Cardano depth (6)
func VerifySignedKES(vkey []byte, period uint64, msg []byte, sig []byte) bool {
	proof, err := NewSumKesFromBytes(CardanoKesDepth, sig)
	if err != nil {
		return false
	}
	return proof.Verify(period, vkey, msg)
}
