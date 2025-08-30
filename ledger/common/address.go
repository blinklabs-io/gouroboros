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
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"math/big"
	"slices"
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
	paymentPayload   AddressPayload
	stakingPayload   AddressPayload
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
	if networkId != AddressNetworkTestnet &&
		networkId != AddressNetworkMainnet {
		return Address{}, errors.New("invalid network ID")
	}
	// Build address bytes
	buf := bytes.NewBuffer(nil)
	header := (addrType << 4) | (networkId & AddressHeaderNetworkMask)
	if err := buf.WriteByte(header); err != nil {
		return Address{}, err
	}
	if _, err := buf.Write(paymentAddr); err != nil {
		return Address{}, err
	}
	if _, err := buf.Write(stakingAddr); err != nil {
		return Address{}, err
	}
	return NewAddressFromBytes(buf.Bytes())
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
		addressType: AddressTypeByron,
		paymentPayload: AddressPayloadKeyHash{
			Hash: AddrKeyHash(paymentAddr),
		},
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
		addressType: AddressTypeByron,
		paymentPayload: AddressPayloadKeyHash{
			Hash: AddrKeyHash(addrHash.Bytes()),
		},
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
		a.paymentPayload = AddressPayloadKeyHash{
			Hash: AddrKeyHash(byronAddr.Hash),
		}
		return nil
	}
	// Payment payload
	payload := data[1:]
	switch a.addressType {
	case AddressTypeKeyKey, AddressTypeKeyScript, AddressTypeKeyPointer, AddressTypeKeyNone:
		if len(payload) < AddressHashSize {
			return errors.New("invalid payment payload: key hash too small")
		}
		a.paymentPayload = AddressPayloadKeyHash{
			Hash: AddrKeyHash(payload[0:AddressHashSize]),
		}
		payload = payload[AddressHashSize:]
	case AddressTypeScriptKey, AddressTypeScriptScript, AddressTypeScriptPointer, AddressTypeScriptNone:
		if len(payload) < AddressHashSize {
			return errors.New("invalid payment payload: script hash too small")
		}
		a.paymentPayload = AddressPayloadScriptHash{
			Hash: ScriptHash(payload[0:AddressHashSize]),
		}
		payload = payload[AddressHashSize:]
	}
	// Staking payload
	switch a.addressType {
	case AddressTypeKeyKey, AddressTypeScriptKey, AddressTypeNoneKey:
		if len(payload) < AddressHashSize {
			return errors.New("invalid staking payload: key hash too small")
		}
		a.stakingPayload = AddressPayloadKeyHash{
			Hash: AddrKeyHash(payload[0:AddressHashSize]),
		}
		payload = payload[AddressHashSize:]
	case AddressTypeKeyScript, AddressTypeScriptScript, AddressTypeNoneScript:
		if len(payload) < AddressHashSize {
			return errors.New("invalid staking payload: script hash too small")
		}
		a.stakingPayload = AddressPayloadScriptHash{
			Hash: ScriptHash(payload[0:AddressHashSize]),
		}
		payload = payload[AddressHashSize:]
	case AddressTypeKeyPointer, AddressTypeScriptPointer:
		var tmpPointer AddressPayloadPointer
		n, err := tmpPointer.decode(payload)
		if err != nil {
			return err
		}
		a.stakingPayload = tmpPointer
		payload = payload[n:]
	}
	// Store any extra address data
	// This is needed to handle the case describe in:
	// https://github.com/IntersectMBO/cardano-ledger/issues/2729
	if len(payload) > 0 {
		a.extraData = payload[:]
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
	switch p := a.paymentPayload.(type) {
	case AddressPayloadKeyHash:
		paymentPd = data.NewConstr(
			0,
			data.NewByteString(p.Hash[:]),
		)
	case AddressPayloadScriptHash:
		paymentPd = data.NewConstr(
			1,
			data.NewByteString(p.Hash[:]),
		)
	default:
		return nil
	}
	// Build stake part
	var stakePd data.PlutusData
	if a.stakingPayload == nil {
		stakePd = data.NewConstr(1)
	} else {
		switch p := a.stakingPayload.(type) {
		case AddressPayloadKeyHash:
			tmpCred := &Credential{
				CredType:   CredentialTypeAddrKeyHash,
				Credential: NewBlake2b224(p.Hash[:]),
			}
			stakePd = data.NewConstr(
				0,
				data.NewConstr(
					0,
					tmpCred.ToPlutusData(),
				),
			)
		case AddressPayloadScriptHash:
			tmpCred := &Credential{
				CredType:   CredentialTypeScriptHash,
				Credential: NewBlake2b224(p.Hash[:]),
			}
			stakePd = data.NewConstr(
				0,
				data.NewConstr(
					0,
					tmpCred.ToPlutusData(),
				),
			)
		case AddressPayloadPointer:
			stakePd = data.NewConstr(
				1,
				data.NewInteger(
					new(big.Int).SetUint64(p.Slot),
				),
				data.NewInteger(
					new(big.Int).SetUint64(p.TxIndex),
				),
				data.NewInteger(
					new(big.Int).SetUint64(p.CertIndex),
				),
			)
		default:
			return nil
		}
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
		paymentPayload: a.paymentPayload,
	}
	return newAddr
}

// PaymentKeyHash returns a new Blake2b224 hash of the payment key
func (a *Address) PaymentKeyHash() Blake2b224 {
	if a.paymentPayload == nil {
		// Return empty hash
		return Blake2b224([AddressHashSize]byte{})
	}
	switch p := a.paymentPayload.(type) {
	case AddressPayloadKeyHash:
		return Blake2b224(p.Hash[:])
	case AddressPayloadScriptHash:
		return Blake2b224(p.Hash[:])
	default:
		// Return empty hash
		return Blake2b224([AddressHashSize]byte{})
	}
}

// PaymentPayload returns the payment payload
func (a *Address) PayloadPayload() AddressPayload {
	return a.paymentPayload
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
		stakingPayload: a.stakingPayload,
	}
	return newAddr
}

// StakeKeyHash returns a new Blake2b224 hash of the stake key
func (a *Address) StakeKeyHash() Blake2b224 {
	if a.stakingPayload == nil {
		// Return empty hash
		return Blake2b224([AddressHashSize]byte{})
	}
	switch p := a.stakingPayload.(type) {
	case AddressPayloadKeyHash:
		return Blake2b224(p.Hash[:])
	case AddressPayloadScriptHash:
		return Blake2b224(p.Hash[:])
	default:
		// Return empty hash
		return Blake2b224([AddressHashSize]byte{})
	}
}

// StakingPayload returns the staking payload
func (a *Address) StakingPayload() AddressPayload {
	return a.stakingPayload
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
			a.paymentPayload.(AddressPayloadKeyHash).Hash.Bytes(),
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
	buf := bytes.NewBuffer(nil)
	header := (a.addressType << 4) | (a.networkId & AddressHeaderNetworkMask)
	if err := buf.WriteByte(header); err != nil {
		return nil, err
	}
	if a.paymentPayload != nil {
		var paymentPayload []byte
		switch p := a.paymentPayload.(type) {
		case AddressPayloadKeyHash:
			paymentPayload = p.Hash.Bytes()
		case AddressPayloadScriptHash:
			paymentPayload = p.Hash.Bytes()
		}
		if _, err := buf.Write(paymentPayload); err != nil {
			return nil, err
		}
	}
	if a.stakingPayload != nil {
		var stakingPayload []byte
		switch p := a.stakingPayload.(type) {
		case AddressPayloadKeyHash:
			stakingPayload = p.Hash.Bytes()
		case AddressPayloadScriptHash:
			stakingPayload = p.Hash.Bytes()
		case AddressPayloadPointer:
			var err error
			stakingPayload, err = p.encode()
			if err != nil {
				return nil, err
			}
		}
		if _, err := buf.Write(stakingPayload); err != nil {
			return nil, err
		}
	}
	if _, err := buf.Write(a.extraData); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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

type AddressPayload interface {
	isAddressPayload()
}

type AddressPayloadKeyHash struct {
	Hash AddrKeyHash
}

func (AddressPayloadKeyHash) isAddressPayload() {}

type AddressPayloadScriptHash struct {
	Hash ScriptHash
}

func (AddressPayloadScriptHash) isAddressPayload() {}

type AddressPayloadPointer struct {
	Slot      uint64
	TxIndex   uint64
	CertIndex uint64
}

func (AddressPayloadPointer) isAddressPayload() {}

func (a *AddressPayloadPointer) decode(data []byte) (int, error) {
	readVarUint := func(buf *bytes.Reader) (uint64, error) {
		var ret uint64
		for {
			byt, err := buf.ReadByte()
			if err != nil {
				return 0, err
			}
			ret = (ret << 7) | uint64(byt&0x7F)
			if (byt & 0x80) == 0 {
				return ret, nil
			}
		}
	}
	buf := bytes.NewReader(data)
	var err error
	a.Slot, err = readVarUint(buf)
	if err != nil {
		return 0, err
	}
	a.TxIndex, err = readVarUint(buf)
	if err != nil {
		return 0, err
	}
	a.CertIndex, err = readVarUint(buf)
	if err != nil {
		return 0, err
	}
	return buf.Len(), nil
}

func (a *AddressPayloadPointer) encode() ([]byte, error) {
	writeVarUint := func(buf *bytes.Buffer, val uint64) error {
		data := []byte{
			byte(val & 0x7F),
		}
		val /= 128
		for val > 0 {
			data = append(
				data,
				byte((val&0x7F)|0x80),
			)
			val /= 128
		}
		slices.Reverse(data)
		if _, err := buf.Write(data); err != nil {
			return err
		}
		return nil
	}
	buf := bytes.NewBuffer(nil)
	if err := writeVarUint(buf, a.Slot); err != nil {
		return nil, err
	}
	if err := writeVarUint(buf, a.TxIndex); err != nil {
		return nil, err
	}
	if err := writeVarUint(buf, a.CertIndex); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
