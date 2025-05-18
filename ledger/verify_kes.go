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

// This file is taken almost verbatim (including comments) from
// https://github.com/cardano-foundation/cardano-ibc-incubator

package ledger

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"math"

	"github.com/blinklabs-io/gouroboros/cbor"
	"golang.org/x/crypto/blake2b"
)

// This module inspired by https://github.com/input-output-hk/kes,
// special thanks to https://github.com/iquerejeta, who helped me a lot on this journey

const (
	SIGMA_SIZE      = 64
	PUBLIC_KEY_SIZE = 32
	Sum0KesSig_SIZE = 64
)

type SumXKesSig struct {
	Depth                  uint64
	Sigma                  any
	LeftHandSidePublicKey  ed25519.PublicKey
	RightHandSidePublicKey ed25519.PublicKey
}

func NewSumKesFromByte(depth uint64, fromByte []byte) SumXKesSig {
	kesSize := SIGMA_SIZE + depth*(PUBLIC_KEY_SIZE*2)
	if kesSize > math.MaxInt {
		panic("kes size too large")
	}
	if len(fromByte) != int(kesSize) {
		panic("length not match")
	}
	nextKesSize := SIGMA_SIZE + (depth-1)*(PUBLIC_KEY_SIZE*2)
	var sigma any
	if depth == 1 {
		sigma = Sum0KesSigFromByte(fromByte)
	} else {
		sigma = NewSumKesFromByte(depth-1, fromByte[0:nextKesSize])
	}
	return SumXKesSig{
		depth,
		sigma,
		fromByte[nextKesSize : nextKesSize+PUBLIC_KEY_SIZE],
		fromByte[nextKesSize+PUBLIC_KEY_SIZE : nextKesSize+PUBLIC_KEY_SIZE*2],
	}
}

func (s SumXKesSig) Verify(
	period uint64,
	pubKey ed25519.PublicKey,
	msg []byte,
) bool {
	pk2 := HashPair(s.LeftHandSidePublicKey, s.RightHandSidePublicKey)
	if !bytes.Equal(pk2, pubKey) {
		return false
	}

	nextDepth := uint64(math.Pow(2, float64(s.Depth)-1))
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

func Sum0KesSigFromByte(sigBytes []byte) Sum0KesSig {
	return sigBytes[0:Sum0KesSig_SIZE]
}

type Sum0KesSig []byte

func (s Sum0KesSig) Verify(
	_ uint64,
	pubKey ed25519.PublicKey,
	msg []byte,
) bool {
	return ed25519.Verify(pubKey, msg, s)
}

// TODO: make this work on anything from Shelley onward (#845)
func VerifyKes(
	header *BabbageBlockHeader,
	slotsPerKesPeriod uint64,
) (bool, error) {
	// Ref: https://github.com/IntersectMBO/ouroboros-consensus/blob/de74882102236fdc4dd25aaa2552e8b3e208448c/ouroboros-consensus-cardano/src/shelley/Ouroboros/Consensus/Shelley/Protocol/Praos.hs#L125
	// Ref: https://github.com/IntersectMBO/cardano-ledger/blob/master/libs/cardano-protocol-tpraos/src/Cardano/Protocol/TPraos/BHeader.hs#L189
	msgBytes, err := cbor.Encode(header.Body)
	if err != nil {
		return false, err
	}
	opCert := header.Body.OpCert
	opCertVkHotBytes := header.Body.OpCert.HotVkey
	startOfKesPeriod := uint64(opCert.KesPeriod)
	currentSlot := header.Body.Slot
	currentKesPeriod := currentSlot / slotsPerKesPeriod
	t := uint64(0)
	if currentKesPeriod >= startOfKesPeriod {
		t = currentKesPeriod - startOfKesPeriod
	}
	return verifySignedKES(opCertVkHotBytes, t, msgBytes, header.Signature), nil
}

func verifySignedKES(vkey []byte, period uint64, msg []byte, sig []byte) bool {
	proof := NewSumKesFromByte(6, sig)
	isValid := proof.Verify(period, vkey, msg)
	return isValid
}
