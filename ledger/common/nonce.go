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
	"encoding/json"
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

const (
	NonceTypeNeutral = 0
	NonceTypeNonce   = 1
)

type Nonce struct {
	cbor.StructAsArray
	Type  uint
	Value [32]byte
}

func (n *Nonce) UnmarshalCBOR(data []byte) error {
	nonceType, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	// nonce type is known within uint range
	n.Type = uint(nonceType) // #nosec G115
	switch nonceType {
	case NonceTypeNeutral:
		// Value uses default value
	case NonceTypeNonce:
		var tmp struct {
			cbor.StructAsArray
			Type  uint
			Value Blake2b256
		}
		if _, err := cbor.Decode(data, &tmp); err != nil {
			return err
		}
		n.Type = tmp.Type
		copy(n.Value[:], tmp.Value[:])
	default:
		return fmt.Errorf("unsupported nonce type %d", nonceType)
	}
	return nil
}

func (n *Nonce) UnmarshalJSON(data []byte) error {
	var tmpData map[string]string
	if err := json.Unmarshal(data, &tmpData); err != nil {
		return err
	}
	tag, ok := tmpData["tag"]
	if !ok {
		return errors.New("did not find expected key 'tag' for nonce")
	}
	switch tag {
	case "NeutralNonce":
		n.Type = NonceTypeNeutral
	default:
		return fmt.Errorf("unsupported nonce tag: %s", tag)
	}
	return nil
}

func (n *Nonce) MarshalCBOR() ([]byte, error) {
	var tmpData []any
	switch n.Type {
	case NonceTypeNeutral:
		tmpData = []any{NonceTypeNeutral}
	case NonceTypeNonce:
		tmpData = []any{NonceTypeNonce, n.Value}
	}
	return cbor.Encode(tmpData)
}

// NonceFromTPraosVRF derives a nonce contribution from the raw 64-byte
// nonce VRF output (bheaderEta) used in Shelley through Alonzo. The separate
// leader VRF output (bheaderL) must not be used here.
// It implements Cardano.Ledger.BaseTypes.mkNonceFromOutputVRF: H(output).
func NonceFromTPraosVRF(output []byte) (Nonce, error) {
	if len(output) != 64 {
		return Nonce{}, fmt.Errorf(
			"invalid TPraos nonce VRF output length: %d, expected 64",
			len(output),
		)
	}
	return Nonce{Type: NonceTypeNonce, Value: Blake2b256Hash(output)}, nil
}

// NonceFromPraosVRF derives a nonce contribution from the raw 64-byte unified
// VRF output used from Babbage onward. It implements
// Cardano.Protocol.Praos.VRF.vrfNonceValue: H(H("N" || output)), with
// Blake2b-256 for both hashes.
func NonceFromPraosVRF(output []byte) (Nonce, error) {
	if len(output) != 64 {
		return Nonce{}, fmt.Errorf(
			"invalid Praos VRF output length: %d, expected 64", len(output),
		)
	}
	var input [65]byte
	input[0] = 'N'
	copy(input[1:], output)
	rangeExtended := Blake2b256Hash(input[:])
	return Nonce{
		Type: NonceTypeNonce, Value: Blake2b256Hash(rangeExtended[:]),
	}, nil
}

// CombineNonces implements the Cardano nonce combination operator (⭒).
// A NonceTypeNeutral is the identity on either side. Two NonceTypeNonce
// values combine as H(a || b), including when either hash is all zeros.
func CombineNonces(a, b Nonce) (Nonce, error) {
	for _, nonce := range []Nonce{a, b} {
		if nonce.Type != NonceTypeNeutral && nonce.Type != NonceTypeNonce {
			return Nonce{}, fmt.Errorf("unsupported nonce type %d", nonce.Type)
		}
	}
	if a.Type == NonceTypeNeutral {
		if b.Type == NonceTypeNeutral {
			return Nonce{Type: NonceTypeNeutral}, nil
		}
		return b, nil
	}
	if b.Type == NonceTypeNeutral {
		return a, nil
	}
	var input [64]byte
	copy(input[:32], a.Value[:])
	copy(input[32:], b.Value[:])
	return Nonce{Type: NonceTypeNonce, Value: Blake2b256Hash(input[:])}, nil
}

// CalculateRollingNonce combines two nonneutral 32-byte nonce hashes.
// blockNonce must already be derived with NonceFromTPraosVRF or
// NonceFromPraosVRF. Raw 64-byte outputs are rejected because their
// derivation depends on the consensus family. An all-zero hash is a
// nonneutral nonce, not the identity.
//
// Deprecated: use CombineNonces to represent neutrality explicitly.
func CalculateRollingNonce(
	prevBlockNonce []byte,
	blockNonce []byte,
) (Blake2b256, error) {
	if len(blockNonce) != 32 {
		return Blake2b256{}, fmt.Errorf(
			"invalid nonce contribution length: %d, expected 32; "+
				"derive raw outputs with NonceFromTPraosVRF or NonceFromPraosVRF",
			len(blockNonce),
		)
	}
	if len(prevBlockNonce) != 32 {
		return Blake2b256{}, fmt.Errorf(
			"invalid prev block nonce length: %d, expected 32",
			len(prevBlockNonce),
		)
	}
	a := Nonce{Type: NonceTypeNonce}
	b := Nonce{Type: NonceTypeNonce}
	copy(a.Value[:], prevBlockNonce)
	copy(b.Value[:], blockNonce)
	nonce, err := CombineNonces(a, b)
	return Blake2b256(nonce.Value), err
}

// EpochNonceFromNonces returns the nonce a new epoch starts with,
// candidate ⭒ lab ⭒ extraEntropy, as the reference's TICKN rule
// (Cardano.Protocol.TPraos.Rules.Tickn) computes it. lab is the
// last-epoch-block nonce, which is NonceTypeNeutral for the first two epoch
// changes after a chain's start. Praos has no extra entropy, so Praos callers
// pass NonceTypeNeutral for it. A NonceTypeNeutral operand is skipped, as in
// CombineNonces.
func EpochNonceFromNonces(candidate, lab, extraEntropy Nonce) (Nonce, error) {
	nonce, err := CombineNonces(candidate, lab)
	if err != nil {
		return Nonce{}, err
	}
	return CombineNonces(nonce, extraEntropy)
}

// CalculateEpochNonce calculates an epoch nonce:
//
//	epochNonce = blake2b_256(candidateNonce || prevEpochFirstBlockHash)
//
// When extraEntropy is non-empty (must be exactly 32 bytes):
//
//	epochNonce = blake2b_256(epochNonce || extraEntropy)
//
// A 32-byte input is always an ordinary nonce, so it cannot express a
// NeutralNonce candidate or last-epoch-block nonce.
//
// Deprecated: use EpochNonceFromNonces to represent neutrality explicitly.
func CalculateEpochNonce(
	stableBlockNonce []byte,
	prevEpochFirstBlockHash []byte,
	extraEntropy []byte,
) (Blake2b256, error) {
	if len(stableBlockNonce) != 32 || len(prevEpochFirstBlockHash) != 32 {
		return Blake2b256{}, fmt.Errorf(
			"invalid epoch nonce inputs: stable=%d, prevEpochFirstBlockHash=%d, expected 32 each",
			len(stableBlockNonce),
			len(prevEpochFirstBlockHash),
		)
	}
	if len(extraEntropy) != 0 && len(extraEntropy) != 32 {
		return Blake2b256{}, fmt.Errorf(
			"invalid extraEntropy length: %d, expected 0 or 32",
			len(extraEntropy),
		)
	}
	var buf [64]byte
	copy(buf[:32], stableBlockNonce)
	copy(buf[32:], prevEpochFirstBlockHash)
	tmpDataHash := Blake2b256Hash(buf[:])
	if len(extraEntropy) == 32 {
		copy(buf[:32], tmpDataHash.Bytes())
		copy(buf[32:], extraEntropy)
		tmpDataHash = Blake2b256Hash(buf[:])
	}
	return tmpDataHash, nil
}
