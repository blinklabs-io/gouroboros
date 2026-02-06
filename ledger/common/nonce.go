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
		type tNonce Nonce
		var tmp tNonce
		if _, err := cbor.Decode(data, &tmp); err != nil {
			return err
		}
		*n = Nonce(tmp)
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

// CalculateRollingNonce calculates a rolling nonce (eta_v) value from the previous block's eta_v value and the current
// block's VRF result
func CalculateRollingNonce(
	prevBlockNonce []byte,
	blockVrf []byte,
) (Blake2b256, error) {
	if len(blockVrf) != 32 && len(blockVrf) != 64 {
		return Blake2b256{}, fmt.Errorf(
			"invalid block VRF length: %d, expected 32 or 64",
			len(blockVrf),
		)
	}
	if len(prevBlockNonce) != 32 {
		return Blake2b256{}, fmt.Errorf(
			"invalid prev block nonce length: %d, expected 32",
			len(prevBlockNonce),
		)
	}
	blockVrfHash := Blake2b256Hash(blockVrf)
	var buf [64]byte
	copy(buf[:32], prevBlockNonce)
	copy(buf[32:], blockVrfHash.Bytes())
	return Blake2b256Hash(buf[:]), nil
}

// CalculateEpochNonce calculates an epoch nonce from the rolling nonce (eta_v) value of the block immediately before the stability
// window and the block hash of the first block from the previous epoch.
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
	var buf [64]byte
	copy(buf[:32], stableBlockNonce)
	copy(buf[32:], prevEpochFirstBlockHash)
	tmpDataHash := Blake2b256Hash(buf[:])
	if len(extraEntropy) > 0 {
		buf2 := make([]byte, 32+len(extraEntropy))
		copy(buf2[:32], tmpDataHash.Bytes())
		copy(buf2[32:], extraEntropy)
		tmpDataHash = Blake2b256Hash(buf2)
	}
	return tmpDataHash, nil
}
