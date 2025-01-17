// Copyright 2024 Blink Labs Software
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
	"fmt"
	"slices"

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

	n.Type = uint(nonceType)

	switch nonceType {
	case NonceTypeNeutral:
		// Value uses default value
	case NonceTypeNonce:
		if err := cbor.DecodeGeneric(data, n); err != nil {
			return err
		}
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
		return fmt.Errorf("did not find expected key 'tag' for nonce")
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

// CalculateRollingNonce calculates a rolling nonce (eta_v) value from the previous block's eta_v value and the current
// block's VRF result
func CalculateRollingNonce(prevBlockNonce []byte, blockVrf []byte) (Blake2b256, error) {
	if len(blockVrf) != 32 && len(blockVrf) != 64 {
		return Blake2b256{}, fmt.Errorf("invalid block VRF length: %d, expected 32 or 64", len(blockVrf))
	}
	blockVrfHash := Blake2b256Hash(blockVrf)
	tmpData := slices.Concat(prevBlockNonce, blockVrfHash.Bytes())
	return Blake2b256Hash(tmpData), nil
}

// CalculateEpochNonce calculates an epoch nonce from the rolling nonce (eta_v) value of the block immediately before the stability
// window and the block hash of the first block from the previous epoch.
func CalculateEpochNonce(stableBlockNonce []byte, prevEpochFirstBlockHash []byte, extraEntropy []byte) (Blake2b256, error) {
	tmpData := slices.Concat(
		stableBlockNonce,
		prevEpochFirstBlockHash,
	)
	tmpDataHash := Blake2b256Hash(tmpData)
	if len(extraEntropy) > 0 {
		tmpData2 := slices.Concat(
			tmpDataHash.Bytes(),
			extraEntropy,
		)
		tmpDataHash = Blake2b256Hash(tmpData2)
	}
	return tmpDataHash, nil
}
