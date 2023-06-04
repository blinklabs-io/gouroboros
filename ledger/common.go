// Copyright 2023 Blink Labs, LLC.
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

package ledger

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/base58"
	"github.com/blinklabs-io/gouroboros/internal/bech32"
)

// MultiAsset represents a collection of policies, assets, and quantities. It's used for
// TX outputs (uint64) and TX asset minting (int64 to allow for negative values for burning)
type MultiAsset[T int64 | uint64] struct {
	data map[Blake2b224]map[cbor.ByteString]T
}

func (m *MultiAsset[T]) UnmarshalCBOR(data []byte) error {
	_, err := cbor.Decode(data, &(m.data))
	return err
}

func (m *MultiAsset[T]) MarshalCBOR() ([]byte, error) {
	return cbor.Encode(&(m.data))
}

func (m *MultiAsset[T]) Policies() []Blake2b224 {
	var ret []Blake2b224
	for policyId := range m.data {
		ret = append(ret, policyId)
	}
	return ret
}

func (m *MultiAsset[T]) Assets(policyId Blake2b224) [][]byte {
	assets, ok := m.data[policyId]
	if !ok {
		return nil
	}
	var ret [][]byte
	for assetName := range assets {
		ret = append(ret, assetName.Bytes())
	}
	return ret
}

func (m *MultiAsset[T]) Asset(policyId Blake2b224, assetName []byte) T {
	policy, ok := m.data[policyId]
	if !ok {
		return 0
	}
	return policy[cbor.ByteString(assetName)]
}

const (
	addressHeaderTypeMask    = 0xF0
	addressHeaderNetworkMask = 0x0F
	addressHashSize          = 28

	addressTypeKeyKey        = 0b0000
	addressTypeScriptKey     = 0b0001
	addressTypeKeyScript     = 0b0010
	addressTypeScriptScript  = 0b0011
	addressTypeKeyPointer    = 0b0100
	addressTypeScriptPointer = 0b0101
	addressTypeKeyNone       = 0b0110
	addressTypeScriptNone    = 0b0111
	addressTypeByron         = 0b1000
	addressTypeNoneKey       = 0b1110
	addressTypeNoneScript    = 0b1111
)

type Address struct {
	addressType    uint8
	networkId      uint8
	paymentAddress []byte
	stakingAddress []byte
	hrp            string
}

func (a *Address) UnmarshalCBOR(data []byte) error {
	// Decode bytes from CBOR
	tmpData := []byte{}
	if _, err := cbor.Decode(data, &tmpData); err != nil {
		return err
	}
	// Extract header info
	header := tmpData[0]
	a.addressType = (header & addressHeaderTypeMask) >> 4
	a.networkId = header & addressHeaderNetworkMask
	// Extract payload
	// NOTE: this is probably incorrect for Byron
	payload := tmpData[1:]
	a.paymentAddress = payload[:addressHashSize]
	a.stakingAddress = payload[addressHashSize:]
	// Generate human readable part of address for output
	a.hrp = a.generateHRP()
	return nil
}

func (a *Address) MarshalCBOR() ([]byte, error) {
	return cbor.Encode(a.Bytes())
}

func (a *Address) generateHRP() string {
	var ret string
	if a.addressType == addressTypeNoneKey || a.addressType == addressTypeNoneScript {
		ret = "stake"
	} else {
		ret = "addr"
	}
	// Add test_ suffix if not mainnet
	if a.networkId != 1 {
		ret += "_test"
	}
	return ret
}

func (a *Address) Bytes() []byte {
	ret := []byte{}
	ret = append(ret, (byte(a.addressType)<<4)|(byte(a.networkId)&addressHeaderNetworkMask))
	ret = append(ret, a.paymentAddress...)
	ret = append(ret, a.stakingAddress...)
	return ret
}

func (a Address) String() string {
	data := a.Bytes()
	if a.addressType == addressTypeByron {
		// Encode data to base58
		encoded := base58.Encode(data)
		return encoded
	} else {
		// Convert data to base32 and encode as bech32
		convData, err := bech32.ConvertBits(data, 8, 5, true)
		if err != nil {
			panic(fmt.Sprintf("unexpected error converting data to base32: %s", err))
		}
		encoded, err := bech32.Encode(a.hrp, convData)
		if err != nil {
			panic(fmt.Sprintf("unexpected error encoding data as bech32: %s", err))
		}
		return encoded
	}
}
