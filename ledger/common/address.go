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
	"errors"
	"fmt"
	"hash/crc32"
	"strings"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/btcsuite/btcd/btcutil/base58"
	"github.com/btcsuite/btcd/btcutil/bech32"
	"golang.org/x/crypto/sha3"
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

	ByronAddressTypePubkey = 0
	ByronAddressTypeScript = 1
	ByronAddressTypeRedeem = 2
)

type AddrKeyHash = Blake2b224

type Address struct {
	addressType      uint8
	networkId        uint8
	paymentAddress   []byte
	stakingAddress   []byte
	extraData        []byte
	byronAddressType uint64
	byronAddressAttr ByronAddressAttributes
}

// NewAddress returns an Address based on the provided bech32/base58 address string
// It detects if the string has mixed case assumes it is a base58 encoded address
// otherwise, it assumes it is bech32 encoded
func NewAddress(addr string) (Address, error) {
	var decoded []byte
	var err error

	if strings.ToLower(addr) != addr {
		// Mixed case detected: Assume Base58 encoding (e.g., Byron addresses)
		decoded = base58.Decode(addr)
	} else {
		_, data, err := bech32.DecodeNoLimit(addr)
		if err != nil {
			return Address{}, err
		}
		decoded, err = bech32.ConvertBits(data, 5, 8, false)
		if err != nil {
			return Address{}, err
		}
	}
	a := Address{}
	err = a.populateFromBytes(decoded)
	if err != nil {
		return Address{}, err
	}
	return a, nil
}

// NewAddressFromBytes returns an Address based on the raw bytes provided
func NewAddressFromBytes(addrBytes []byte) (Address, error) {
	var ret Address
	if err := ret.populateFromBytes(addrBytes); err != nil {
		return Address{}, err
	}
	return ret, nil
}

// NewAddressFromParts returns an Address based on the individual parts of the address that are provided
func NewAddressFromParts(
	addrType uint8,
	networkId uint8,
	paymentAddr []byte,
	stakingAddr []byte,
) (Address, error) {
	// Validate network ID
	if networkId != AddressNetworkTestnet && networkId != AddressNetworkMainnet {
		return Address{}, errors.New("invalid network ID")
	}

	// Handle stake-only addresses
	if addrType == AddressTypeNoneKey || addrType == AddressTypeNoneScript {
		if len(paymentAddr) > 0 {
			return Address{}, errors.New("payment address must be empty for stake-only addresses")
		}
		if len(stakingAddr) != AddressHashSize {
			return Address{}, fmt.Errorf("staking key must be exactly %d bytes", AddressHashSize)
		}
		return Address{
			addressType:    addrType,
			networkId:      networkId,
			stakingAddress: stakingAddr,
		}, nil
	}

	// Handle regular addresses
	if len(paymentAddr) != AddressHashSize {
		return Address{}, fmt.Errorf("payment address must be exactly %d bytes", AddressHashSize)
	}

	if len(stakingAddr) > 0 && len(stakingAddr) != AddressHashSize {
		return Address{}, fmt.Errorf("staking address must be empty or exactly %d bytes", AddressHashSize)
	}

	return Address{
		addressType:    addrType,
		networkId:      networkId,
		paymentAddress: paymentAddr,
		stakingAddress: stakingAddr,
	}, nil
}

func NewByronAddressFromParts(
	byronAddrType uint64,
	paymentAddr []byte,
	attr ByronAddressAttributes,
) (Address, error) {
	if len(paymentAddr) != AddressHashSize {
		return Address{}, fmt.Errorf(
			"invalid payment address hash length: %d",
			len(paymentAddr),
		)
	}
	return Address{
		addressType:      AddressTypeByron,
		paymentAddress:   paymentAddr,
		byronAddressType: byronAddrType,
		byronAddressAttr: attr,
	}, nil
}

func NewByronAddressRedeem(
	pubkey []byte,
	attr ByronAddressAttributes,
) (Address, error) {
	if len(pubkey) != 32 {
		return Address{}, fmt.Errorf(
			"invalid redeem pubkey length: %d",
			len(pubkey),
		)
	}
	addrRoot := []any{
		ByronAddressTypeRedeem,
		[]any{
			ByronAddressTypeRedeem,
			pubkey,
		},
		attr,
	}
	addrRootBytes, err := cbor.Encode(addrRoot)
	if err != nil {
		return Address{}, err
	}
	sha3Sum := sha3.Sum256(addrRootBytes)
	addrHash := Blake2b224Hash(sha3Sum[:])
	return Address{
		addressType:      AddressTypeByron,
		paymentAddress:   addrHash.Bytes(),
		byronAddressType: ByronAddressTypeRedeem,
		byronAddressAttr: attr,
	}, nil
}

func (a *Address) populateFromBytes(data []byte) error {
	// Extract header info
	header := data[0]
	a.addressType = (header & AddressHeaderTypeMask) >> 4
	a.networkId = header & AddressHeaderNetworkMask
	// Byron Addresses
	if a.addressType == AddressTypeByron {
		var rawAddr byronAddress
		if _, err := cbor.Decode(data, &rawAddr); err != nil {
			return err
		}
		payloadBytes, ok := rawAddr.Payload.Content.([]byte)
		if !ok || rawAddr.Payload.Number != 24 {
			return errors.New(
				"invalid Byron address data: unexpected payload content",
			)
		}
		payloadChecksum := crc32.ChecksumIEEE(payloadBytes)
		if rawAddr.Checksum != payloadChecksum {
			return errors.New(
				"invalid Byron address data: checksum does not match",
			)
		}
		var byronAddr byronAddressPayload
		if _, err := cbor.Decode(payloadBytes, &byronAddr); err != nil {
			return err
		}
		if len(byronAddr.Hash) != AddressHashSize {
			return errors.New(
				"invalid Byron address data: hash is not expected length",
			)
		}
		a.byronAddressType = byronAddr.AddrType
		a.byronAddressAttr = byronAddr.Attr
		a.paymentAddress = byronAddr.Hash
		return nil
	}
	// Check length
	// We exclude a few address types without fixed sizes that we don't properly support yet
	if a.addressType != AddressTypeKeyPointer &&
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
	addrBytes, err := a.Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to get address bytes: %w", err)
	}
	return cbor.Encode(addrBytes)
}

func (a *Address) ToPlutusData() data.PlutusData {
	if a.addressType == AddressTypeByron {
		// There is no PlutusData representation for Byron addresses
		return nil
	}
	// Build payment part
	var paymentPd data.PlutusData
	switch a.addressType {
	case AddressTypeKeyKey, AddressTypeKeyScript, AddressTypeKeyPointer, AddressTypeKeyNone:
		paymentPd = data.NewConstr(
			0,
			data.NewByteString(a.paymentAddress),
		)
	case AddressTypeScriptKey, AddressTypeScriptScript, AddressTypeScriptPointer, AddressTypeScriptNone:
		paymentPd = data.NewConstr(
			1,
			data.NewByteString(a.paymentAddress),
		)
	default:
		return nil
	}
	// Build stake part
	var stakePd data.PlutusData
	switch a.addressType {
	case AddressTypeKeyKey, AddressTypeScriptKey, AddressTypeNoneKey:
		tmpCred := &Credential{
			CredType:   CredentialTypeAddrKeyHash,
			Credential: NewBlake2b224(a.stakingAddress),
		}
		stakePd = data.NewConstr(
			0,
			tmpCred.ToPlutusData(),
		)
	case AddressTypeKeyScript, AddressTypeScriptScript, AddressTypeNoneScript:
		tmpCred := &Credential{
			CredType:   CredentialTypeScriptHash,
			Credential: NewBlake2b224(a.stakingAddress),
		}
		stakePd = data.NewConstr(
			0,
			tmpCred.ToPlutusData(),
		)
	case AddressTypeKeyNone, AddressTypeScriptNone:
		stakePd = data.NewConstr(1)
	// TODO: add support for pointer addresses once we add it to Address
	default:
		return nil
	}
	return data.NewConstr(
		0,
		paymentPd,
		stakePd,
	)
}

func (a Address) NetworkId() uint {
	if a.addressType == AddressTypeByron {
		// Use Shelley network ID convention
		if a.byronAddressAttr.Network == nil {
			// Return mainnet if no network ID is present in address
			return AddressNetworkMainnet
		}
		// Return testnet, since the convention says we only include network ID on testnets
		return AddressNetworkTestnet
	} else {
		return uint(a.networkId)
	}
}

func (a Address) Type() uint8 {
	return a.addressType
}

func (a Address) ByronType() uint64 {
	return a.byronAddressType
}

// PaymentAddress returns a new Address with only the payment address portion. This will return nil for anything other than payment and script addresses
func (a Address) PaymentAddress() *Address {
	var addrType uint8
	switch a.addressType {
	case AddressTypeKeyKey, AddressTypeKeyNone:
		addrType = AddressTypeKeyNone
	case AddressTypeScriptKey, AddressTypeScriptNone, AddressTypeScriptScript:
		addrType = AddressTypeScriptNone
	default:
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
	switch a.addressType {
	case AddressTypeKeyKey, AddressTypeScriptKey:
		addrType = AddressTypeNoneKey
	case AddressTypeScriptScript, AddressTypeNoneScript:
		addrType = AddressTypeNoneScript
	default:
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

func (a *Address) ByronAttr() ByronAddressAttributes {
	return a.byronAddressAttr
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
func (a Address) Bytes() ([]byte, error) {
	if a.addressType == AddressTypeByron {
		tmpPayload := []any{
			a.paymentAddress,
			a.byronAddressAttr,
			a.byronAddressType,
		}
		rawPayload, err := cbor.Encode(tmpPayload)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to encode Byron address payload: %w",
				err,
			)
		}
		tmpData := []any{
			cbor.Tag{
				Number:  24,
				Content: rawPayload,
			},
			crc32.ChecksumIEEE(rawPayload),
		}
		ret, err := cbor.Encode(tmpData)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to encode Byron address data: %w",
				err,
			)
		}
		return ret, nil
	}
	ret := []byte{}
	ret = append(
		ret,
		(a.addressType<<4)|(a.networkId&AddressHeaderNetworkMask),
	)
	ret = append(ret, a.paymentAddress...)
	ret = append(ret, a.stakingAddress...)
	ret = append(ret, a.extraData...)
	return ret, nil
}

// String returns the bech32-encoded version of the address
func (a Address) String() string {
	data, err := a.Bytes()
	if err != nil {
		panic(fmt.Sprintf("failed to get address bytes: %v", err))
	}
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

type byronAddress struct {
	cbor.StructAsArray
	Payload  cbor.Tag
	Checksum uint32
}

type byronAddressPayload struct {
	cbor.StructAsArray
	Hash     []byte
	Attr     ByronAddressAttributes
	AddrType uint64
}

type ByronAddressAttributes struct {
	Payload []byte
	Network *uint32
}

func (a *ByronAddressAttributes) UnmarshalCBOR(data []byte) error {
	var tmpData struct {
		Payload    []byte `cbor:"1,keyasint,omitempty"`
		NetworkRaw []byte `cbor:"2,keyasint,omitempty"`
	}
	if _, err := cbor.Decode(data, &tmpData); err != nil {
		return err
	}
	a.Payload = tmpData.Payload
	if len(tmpData.NetworkRaw) > 0 {
		var tmpNetwork uint32
		if _, err := cbor.Decode(tmpData.NetworkRaw, &tmpNetwork); err != nil {
			return err
		}
		a.Network = &tmpNetwork
	}
	return nil
}

func (a *ByronAddressAttributes) MarshalCBOR() ([]byte, error) {
	tmpData := make(map[int]any)
	if len(a.Payload) > 0 {
		tmpData[1] = a.Payload
	}
	if a.Network != nil {
		networkRaw, err := cbor.Encode(a.Network)
		if err != nil {
			return nil, err
		}
		tmpData[2] = networkRaw
	}
	return cbor.Encode(tmpData)
}
