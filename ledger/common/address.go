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
	"bytes"
	"crypto/sha3"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math/big"
	"strings"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/btcsuite/btcd/btcutil/base58"
	"github.com/btcsuite/btcd/btcutil/bech32"
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

	AddressTypeScriptBit = 0x01

	ByronAddressTypePubkey = 0
	ByronAddressTypeScript = 1
	ByronAddressTypeRedeem = 2
)

// init registers a bech32/base58 address formatter with the cbor package so
// AnnotateAddresses and the Cardano-aware diagnostic formatters can render
// raw address byte strings in their human-readable form. The hook is
// best-effort: it only fires for byte strings that successfully parse as an
// Address; anything else falls through unannotated.
func init() {
	cbor.RegisterAddressFormatter(func(b []byte) (string, bool) {
		addr, err := NewAddressFromBytes(b)
		if err != nil {
			return "", false
		}
		s := addr.String()
		if s == "" {
			return "", false
		}
		return s, true
	})
}

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

// NewAddress returns an Address based on the provided bech32/base58 address
// string.
func NewAddress(addr string) (Address, error) {
	var decoded []byte
	hrp, data, err := bech32.DecodeNoLimit(addr)
	isBech32 := err == nil
	if err == nil {
		decoded, err = bech32.ConvertBits(data, 5, 8, false)
		if err != nil {
			return Address{}, err
		}
	} else {
		// A string with a known Shelley HRP was intended to be bech32. Do not
		// reinterpret a checksum or mixed-case failure as a Byron address.
		if hasShelleyAddressHRP(addr) {
			return Address{}, err
		}
		// bech32 failed — try base58 (Byron addresses)
		decoded = base58.Decode(addr)
		if len(decoded) == 0 {
			return Address{}, err
		}
	}
	a := Address{}
	err = a.populateFromBytes(decoded)
	if err != nil {
		return Address{}, err
	}
	if isBech32 {
		if a.addressType == AddressTypeByron {
			return Address{}, errors.New(
				"byron addresses must use base58 encoding",
			)
		}
		expectedHRP := a.generateHRP()
		if !strings.EqualFold(hrp, expectedHRP) {
			return Address{}, fmt.Errorf(
				"address HRP %q does not match expected HRP %q",
				hrp,
				expectedHRP,
			)
		}
	}
	return a, nil
}

func hasShelleyAddressHRP(addr string) bool {
	return hasFoldedPrefix(addr, "addr1") ||
		hasFoldedPrefix(addr, "addr_test1") ||
		hasFoldedPrefix(addr, "stake1") ||
		hasFoldedPrefix(addr, "stake_test1")
}

func hasFoldedPrefix(value, prefix string) bool {
	return len(value) >= len(prefix) &&
		strings.EqualFold(value[:len(prefix)], prefix)
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

	ret := Address{
		addressType: addrType,
		networkId:   networkId,
	}

	switch addrType {
	case AddressTypeKeyKey, AddressTypeKeyScript, AddressTypeKeyNone:
		if len(paymentAddr) != AddressHashSize {
			return Address{}, fmt.Errorf(
				"invalid payment address hash length: %d",
				len(paymentAddr),
			)
		}
		ret.paymentPayload = AddressPayloadKeyHash{
			Hash: AddrKeyHash(paymentAddr),
		}
	case AddressTypeScriptKey, AddressTypeScriptScript, AddressTypeScriptNone:
		if len(paymentAddr) != AddressHashSize {
			return Address{}, fmt.Errorf(
				"invalid payment address hash length: %d",
				len(paymentAddr),
			)
		}
		ret.paymentPayload = AddressPayloadScriptHash{
			Hash: ScriptHash(paymentAddr),
		}
	case AddressTypeNoneKey, AddressTypeNoneScript:
		if len(paymentAddr) != 0 {
			return Address{}, fmt.Errorf(
				"unexpected payment payload length: %d",
				len(paymentAddr),
			)
		}
	case AddressTypeKeyPointer, AddressTypeScriptPointer:
		// Preserve pointer-address behavior via the existing byte path so
		// extra trailing data continues to round-trip unchanged.
		fallthrough
	default:
		addrBytes := make([]byte, 1+len(paymentAddr)+len(stakingAddr))
		addrBytes[0] = (addrType << 4) | (networkId & AddressHeaderNetworkMask)
		offset := 1 + copy(addrBytes[1:], paymentAddr)
		copy(addrBytes[offset:], stakingAddr)
		return NewAddressFromBytes(addrBytes)
	}

	switch addrType {
	case AddressTypeKeyKey, AddressTypeScriptKey, AddressTypeNoneKey:
		if len(stakingAddr) != AddressHashSize {
			return Address{}, fmt.Errorf(
				"invalid staking address hash length: %d",
				len(stakingAddr),
			)
		}
		ret.stakingPayload = AddressPayloadKeyHash{
			Hash: AddrKeyHash(stakingAddr),
		}
	case AddressTypeKeyScript, AddressTypeScriptScript, AddressTypeNoneScript:
		if len(stakingAddr) != AddressHashSize {
			return Address{}, fmt.Errorf(
				"invalid staking address hash length: %d",
				len(stakingAddr),
			)
		}
		ret.stakingPayload = AddressPayloadScriptHash{
			Hash: ScriptHash(stakingAddr),
		}
	case AddressTypeKeyNone, AddressTypeScriptNone:
		if len(stakingAddr) != 0 {
			return Address{}, fmt.Errorf(
				"unexpected staking payload length: %d",
				len(stakingAddr),
			)
		}
	}

	return ret, nil
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
			Hash: AddrKeyHash(addrHash),
		},
		byronAddressType: ByronAddressTypeRedeem,
		byronAddressAttr: attr,
	}, nil
}

func (a *Address) populateFromBytes(data []byte) error {
	if len(data) == 0 {
		return errors.New("invalid address data: empty byte slice")
	}
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
			Hash: AddrKeyHash(NewBlake2b224(byronAddr.Hash)),
		}
		return nil
	}
	// Validate network ID for non-Byron addresses
	if a.networkId != AddressNetworkTestnet &&
		a.networkId != AddressNetworkMainnet {
		return fmt.Errorf("invalid network ID: %d", a.networkId)
	}
	// Validate that the address type is one of the known CIP-0019 types.
	// Types 9-13 are reserved and must be rejected rather than silently
	// decoding as an address with no payment/staking payload
	switch a.addressType {
	case AddressTypeKeyKey,
		AddressTypeScriptKey,
		AddressTypeKeyScript,
		AddressTypeScriptScript,
		AddressTypeKeyPointer,
		AddressTypeScriptPointer,
		AddressTypeKeyNone,
		AddressTypeScriptNone,
		AddressTypeNoneKey,
		AddressTypeNoneScript:
		// Known address type
	default:
		return fmt.Errorf("invalid address type: %d", a.addressType)
	}
	// Payment payload
	payload := data[1:]
	switch a.addressType {
	case AddressTypeKeyKey,
		AddressTypeKeyScript,
		AddressTypeKeyPointer,
		AddressTypeKeyNone:
		if len(payload) < AddressHashSize {
			return errors.New("invalid payment payload: key hash too small")
		}
		a.paymentPayload = AddressPayloadKeyHash{
			Hash: AddrKeyHash(payload[0:AddressHashSize]),
		}
		payload = payload[AddressHashSize:]
	case AddressTypeScriptKey,
		AddressTypeScriptScript,
		AddressTypeScriptPointer,
		AddressTypeScriptNone:
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
	// A well-formed address of a given type has an exact, computable
	// length, so nothing should remain in payload at this point. However,
	// a small, fixed set of addresses were minted on Cardano mainnet with
	// extra trailing bytes due to a historical wallet/ledger bug (see
	// https://github.com/IntersectMBO/cardano-ledger/issues/2729 and
	// https://github.com/blinklabs-io/gouroboros/issues/519). Those
	// addresses are permanently part of the chain, so we special-case the
	// exact trailing byte sequences known to have appeared on mainnet
	// (mirroring the TRAILING_WHITELIST approach taken by
	// cardano-multiplatform-lib) to allow them to keep decoding, while
	// rejecting any other unexpected trailing data outright. The known
	// malformed addresses are all mainnet addresses, so we only consult
	// the whitelist for mainnet; a testnet address is never exempted, even
	// if its trailing bytes happen to collide with a whitelisted sequence.
	if len(payload) > 0 {
		if a.networkId != AddressNetworkMainnet ||
			!isKnownMalformedAddressTrailer(payload) {
			return fmt.Errorf(
				"invalid address data: %d unexpected trailing byte(s)",
				len(payload),
			)
		}
		a.extraData = payload[:]
	}
	return nil
}

// knownMalformedAddressTrailers holds the exact trailing byte sequences of
// the small set of addresses known to exist on Cardano mainnet with extra
// bytes appended beyond their expected length, due to a historical
// wallet/ledger bug. See:
// https://github.com/IntersectMBO/cardano-ledger/issues/2729
// https://github.com/blinklabs-io/gouroboros/issues/519
// This list mirrors the TRAILING_WHITELIST constant maintained by
// cardano-multiplatform-lib, the canonical reference for these addresses.
var knownMalformedAddressTrailers = [][]byte{
	{
		203, 87, 175, 176, 179, 95, 200, 156, 99, 6, 28, 153, 20, 224, 85, 0,
		26, 81, 140, 117, 22,
	},
	{
		19, 213, 244, 163, 254, 4, 120, 178, 36, 30, 1, 104, 227, 203, 165, 0,
		26, 34, 193, 90, 17,
	},
	{0},
	{
		106, 51, 48, 102, 53, 97, 109, 107, 119, 104, 119, 113, 97, 52, 119,
		118, 102, 121, 106, 100, 101, 122, 121, 97, 101, 108, 109, 110, 110,
		103, 100, 54, 100, 52, 101,
	},
	{
		53, 97, 99, 121, 50, 114, 48, 101, 107, 114, 112, 113, 122, 113, 106,
		108, 113, 100, 107, 56, 108, 122, 113, 110, 53, 114, 52, 53, 110,
	},
	{
		6, 29, 7, 12, 13, 4, 27, 7, 2, 15, 11, 13, 11, 15, 2, 9, 18, 5, 29,
		28, 16, 9, 17, 4, 14, 31, 7, 19, 17, 3, 1, 0, 11, 16, 22, 0,
	},
	{
		18, 110, 119, 53, 51, 53, 103, 54, 118, 115, 112, 55, 120, 55, 102,
		104, 120, 112, 113, 50, 112, 116, 115, 104, 57, 103, 107, 114,
	},
	{44},
}

func isKnownMalformedAddressTrailer(trailer []byte) bool {
	for _, known := range knownMalformedAddressTrailers {
		if bytes.Equal(trailer, known) {
			return true
		}
	}
	return false
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
	// Stake-only address
	if a.paymentPayload == nil && a.stakingPayload != nil {
		switch p := a.stakingPayload.(type) {
		case AddressPayloadKeyHash:
			return data.NewConstr(
				0,
				data.NewByteString(p.Hash.Bytes()),
			)
		case AddressPayloadScriptHash:
			return data.NewConstr(
				1,
				data.NewByteString(p.Hash.Bytes()),
			)
		}
	}
	// Build payment part
	var paymentPd data.PlutusData
	switch p := a.paymentPayload.(type) {
	case AddressPayloadKeyHash:
		paymentPd = data.NewConstr(
			0,
			data.NewByteString(p.Hash.Bytes()),
		)
	case AddressPayloadScriptHash:
		paymentPd = data.NewConstr(
			1,
			data.NewByteString(p.Hash.Bytes()),
		)
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
				Credential: NewBlake2b224(p.Hash.Bytes()),
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
				Credential: NewBlake2b224(p.Hash.Bytes()),
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
				0,
				data.NewConstr(
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
				),
			)
		default:
			panic(fmt.Sprintf("unsupported staking payload type: %T", p))
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
		return p.Hash
	case AddressPayloadScriptHash:
		return p.Hash
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
		return p.Hash
	case AddressPayloadScriptHash:
		return p.Hash
	default:
		// Return empty hash
		return Blake2b224([AddressHashSize]byte{})
	}
}

// StakingPayload returns the staking payload
func (a *Address) StakingPayload() AddressPayload {
	return a.stakingPayload
}

// StakeCredential returns the key or script credential carried by the
// address's staking payload. Pointer and absent staking payloads do not contain
// a credential.
func (a *Address) StakeCredential() (Credential, bool) {
	if a == nil {
		return Credential{}, false
	}
	switch payload := a.stakingPayload.(type) {
	case AddressPayloadKeyHash:
		return Credential{
			CredType:   CredentialTypeAddrKeyHash,
			Credential: NewBlake2b224(payload.Hash.Bytes()),
		}, true
	case AddressPayloadScriptHash:
		return Credential{
			CredType:   CredentialTypeScriptHash,
			Credential: NewBlake2b224(payload.Hash.Bytes()),
		}, true
	default:
		return Credential{}, false
	}
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

	var paymentPayload []byte
	if a.paymentPayload != nil {
		switch p := a.paymentPayload.(type) {
		case AddressPayloadKeyHash:
			paymentPayload = p.Hash.Bytes()
		case AddressPayloadScriptHash:
			paymentPayload = p.Hash.Bytes()
		}
	}

	var stakingPayload []byte
	if a.stakingPayload != nil {
		switch p := a.stakingPayload.(type) {
		case AddressPayloadKeyHash:
			stakingPayload = p.Hash.Bytes()
		case AddressPayloadScriptHash:
			stakingPayload = p.Hash.Bytes()
		case AddressPayloadPointer:
			stakingPayload = p.encode()
		}
	}

	ret := make(
		[]byte,
		1+len(paymentPayload)+len(stakingPayload)+len(a.extraData),
	)
	ret[0] = (a.addressType << 4) | (a.networkId & AddressHeaderNetworkMask)
	offset := 1 + copy(ret[1:], paymentPayload)
	offset += copy(ret[offset:], stakingPayload)
	copy(ret[offset:], a.extraData)
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

func (a Address) MarshalText() ([]byte, error) {
	return []byte(a.String()), nil
}

func (a *Address) UnmarshalText(text []byte) error {
	parsed, err := NewAddress(string(text))
	if err != nil {
		return err
	}
	*a = parsed
	return nil
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
	readVarUint := func(data []byte, offset int) (uint64, int, error) {
		var ret uint64
		for offset < len(data) {
			byt := data[offset]
			offset++
			ret = (ret << 7) | uint64(byt&0x7F)
			if (byt & 0x80) == 0 {
				return ret, offset, nil
			}
		}
		return 0, offset, io.ErrUnexpectedEOF
	}

	var offset int
	var err error
	a.Slot, offset, err = readVarUint(data, offset)
	if err != nil {
		return 0, err
	}
	a.TxIndex, offset, err = readVarUint(data, offset)
	if err != nil {
		return 0, err
	}
	a.CertIndex, offset, err = readVarUint(data, offset)
	if err != nil {
		return 0, err
	}
	return offset, nil
}

func (a *AddressPayloadPointer) encode() []byte {
	writeVarUint := func(dst []byte, val uint64) int {
		var tmp [10]byte
		i := len(tmp) - 1
		tmp[i] = byte(val & 0x7F)
		val /= 128
		for val > 0 {
			i--
			tmp[i] = byte((val & 0x7F) | 0x80) //nolint:gosec // masked to 7 bits, always fits byte
			val /= 128
		}
		return copy(dst, tmp[i:])
	}
	ret := make([]byte, 0, 30)
	ret = ret[:cap(ret)]
	offset := writeVarUint(ret, a.Slot)
	offset += writeVarUint(ret[offset:], a.TxIndex)
	offset += writeVarUint(ret[offset:], a.CertIndex)
	return ret[:offset]
}
