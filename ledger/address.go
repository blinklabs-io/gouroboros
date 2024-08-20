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

package ledger

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/base58"
	"github.com/blinklabs-io/gouroboros/bech32"
	"github.com/blinklabs-io/gouroboros/cbor"
)

const (
	AddressHeaderTypeMask    = 0xF0
	AddressHeaderNetworkMask = 0x0F
	AddressHashSize          = 28

	AddressNetworkTestnet = 0
	AddressNetworkMainnet = 1

	AddressTypeKeyKey        = 0b0000
	AddressTypeScriptKey     = 0b0001
	AddressTypeKeyScript     = 0b0010
	AddressTypeScriptScript  = 0b0011
	AddressTypeKeyPointer    = 0b0100
	AddressTypeScriptPointer = 0b0101
	AddressTypeKeyNone       = 0b0110
	AddressTypeScriptNone    = 0b0111
	AddressTypeByron         = 0b1000
	AddressTypeNoneKey       = 0b1110
	AddressTypeNoneScript    = 0b1111
)

type AddrKeyHash Blake2b224

type Address struct {
	addressType    uint8
	networkId      uint8
	paymentAddress []byte
	stakingAddress []byte
	extraData      []byte
}

// NewAddress returns an Address based on the provided bech32 address string
func NewAddress(addr string) (Address, error) {
	_, data, err := bech32.DecodeNoLimit(addr)
	if err != nil {
		return Address{}, err
	}
	decoded, err := bech32.ConvertBits(data, 5, 8, false)
	if err != nil {
		return Address{}, err
	}
	a := Address{}
	err = a.populateFromBytes(decoded)
	if err != nil {
		return Address{}, err
	}
	return a, nil
}

// NewAddressFromParts returns an Address based on the individual parts of the address that are provided
func NewAddressFromParts(
	addrType uint8,
	networkId uint8,
	paymentAddr []byte,
	stakingAddr []byte,
) (Address, error) {
	if len(paymentAddr) != AddressHashSize {
		return Address{}, fmt.Errorf(
			"invalid payment address hash length: %d",
			len(paymentAddr),
		)
	}
	if len(stakingAddr) > 0 && len(stakingAddr) != AddressHashSize {
		return Address{}, fmt.Errorf(
			"invalid staking address hash length: %d",
			len(stakingAddr),
		)
	}
	return Address{
		addressType:    addrType,
		networkId:      networkId,
		paymentAddress: paymentAddr[:],
		stakingAddress: stakingAddr[:],
	}, nil
}

func (a *Address) populateFromBytes(data []byte) error {
	// Extract header info
	header := data[0]
	a.addressType = (header & AddressHeaderTypeMask) >> 4
	a.networkId = header & AddressHeaderNetworkMask
	// Check length
	// We exclude a few address types without fixed sizes that we don't properly support yet
	if a.addressType != AddressTypeByron &&
		a.addressType != AddressTypeKeyPointer &&
		a.addressType != AddressTypeScriptPointer {
		dataLen := len(data)
		// Addresses must be at least the address hash size plus header byte
		if dataLen < (AddressHashSize + 1) {
			return fmt.Errorf("invalid address length: %d", dataLen)
		}
		// Check bounds of second part if the address type is supposed to have one
		if a.addressType != AddressTypeKeyNone &&
			a.addressType != AddressTypeScriptNone {
			if dataLen > (AddressHashSize + 1) {
				if dataLen < (AddressHashSize + AddressHashSize + 1) {
					return fmt.Errorf("invalid address length: %d", dataLen)
				}
			}
		}
	}
	// Extract payload
	// NOTE: this is definitely incorrect for Byron
	payload := data[1:]
	a.paymentAddress = payload[:AddressHashSize]
	payload = payload[AddressHashSize:]
	if a.addressType != AddressTypeKeyNone &&
		a.addressType != AddressTypeScriptNone {
		if len(payload) >= AddressHashSize {
			a.stakingAddress = payload[:AddressHashSize]
			payload = payload[AddressHashSize:]
		}
	}
	// Store any extra address data
	// This is needed to handle the case describe in:
	// https://github.com/IntersectMBO/cardano-ledger/issues/2729
	if len(payload) > 0 {
		a.extraData = payload[:]
	}
	// Adjust stake addresses
	if a.addressType == AddressTypeNoneKey ||
		a.addressType == AddressTypeNoneScript {
		a.stakingAddress = a.paymentAddress[:]
		a.paymentAddress = make([]byte, 0)
	}
	return nil
}

func (a *Address) UnmarshalCBOR(data []byte) error {
	// Try to unwrap as bytestring (Shelley and forward)
	tmpData := []byte{}
	if _, err := cbor.Decode(data, &tmpData); err == nil {
		err := a.populateFromBytes(tmpData)
		if err != nil {
			return err
		}
	} else {
		// Probably a Byron address
		if err := a.populateFromBytes(data); err != nil {
			return err
		}
	}
	return nil
}

func (a *Address) MarshalCBOR() ([]byte, error) {
	return cbor.Encode(a.Bytes())
}

// PaymentAddress returns a new Address with only the payment address portion. This will return nil for anything other than payment and script addresses
func (a Address) PaymentAddress() *Address {
	var addrType uint8
	if a.addressType == AddressTypeKeyKey ||
		a.addressType == AddressTypeKeyNone {
		addrType = AddressTypeKeyNone
	} else if a.addressType == AddressTypeScriptKey ||
		a.addressType == AddressTypeScriptNone ||
		a.addressType == AddressTypeScriptScript {
		addrType = AddressTypeScriptNone
	} else {
		// Unsupported address type
		return nil
	}
	newAddr := &Address{
		addressType:    addrType,
		networkId:      a.networkId,
		paymentAddress: a.paymentAddress[:],
	}
	return newAddr
}

// PaymentKeyHash returns a new Blake2b224 hash of the payment key
func (a *Address) PaymentKeyHash() Blake2b224 {
	if len(a.paymentAddress) != AddressHashSize {
		// Return empty hash
		return Blake2b224([AddressHashSize]byte{})
	}
	return Blake2b224(a.paymentAddress[:])
}

// StakeAddress returns a new Address with only the stake key portion. This will return nil if the address is not a payment/staking key pair
func (a Address) StakeAddress() *Address {
	var addrType uint8
	if a.addressType == AddressTypeKeyKey ||
		a.addressType == AddressTypeScriptKey {
		addrType = AddressTypeNoneKey
	} else if a.addressType == AddressTypeScriptScript ||
		a.addressType == AddressTypeNoneScript {
		addrType = AddressTypeNoneScript
	} else {
		// Unsupported address type
		return nil
	}
	newAddr := &Address{
		addressType:    addrType,
		networkId:      a.networkId,
		stakingAddress: a.stakingAddress[:],
	}
	return newAddr
}

// StakeKeyHash returns a new Blake2b224 hash of the stake key
func (a *Address) StakeKeyHash() Blake2b224 {
	if len(a.stakingAddress) != AddressHashSize {
		// Return empty hash
		return Blake2b224([AddressHashSize]byte{})
	}
	return Blake2b224(a.stakingAddress[:])
}

func (a Address) generateHRP() string {
	var ret string
	if a.addressType == AddressTypeNoneKey ||
		a.addressType == AddressTypeNoneScript {
		ret = "stake"
	} else {
		ret = "addr"
	}
	// Add test_ suffix if not mainnet
	if a.networkId != AddressNetworkMainnet {
		ret += "_test"
	}
	return ret
}

// Bytes returns the underlying bytes for the address
func (a Address) Bytes() []byte {
	ret := []byte{}
	ret = append(
		ret,
		(byte(a.addressType)<<4)|(byte(a.networkId)&AddressHeaderNetworkMask),
	)
	ret = append(ret, a.paymentAddress...)
	ret = append(ret, a.stakingAddress...)
	ret = append(ret, a.extraData...)
	return ret
}

// String returns the bech32-encoded version of the address
func (a Address) String() string {
	data := a.Bytes()
	if a.addressType == AddressTypeByron {
		// Encode data to base58
		encoded := base58.Encode(data)
		return encoded
	} else {
		// Convert data to base32 and encode as bech32
		convData, err := bech32.ConvertBits(data, 8, 5, true)
		if err != nil {
			panic(fmt.Sprintf("unexpected error converting data to base32: %s", err))
		}
		// Generate human readable part of address for output
		hrp := a.generateHRP()
		encoded, err := bech32.Encode(hrp, convData)
		if err != nil {
			panic(fmt.Sprintf("unexpected error encoding data as bech32: %s", err))
		}
		return encoded
	}
}

func (a Address) MarshalJSON() ([]byte, error) {
	return []byte(`"` + a.String() + `"`), nil
}
