// Copyright 2024 Blink Labs, LLC.
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

	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"

	"github.com/blinklabs-io/gouroboros/cbor"
)

const (
	CertificateTypeStakeRegistration    = 0
	CertificateTypeStakeDeregistration  = 1
	CertificateTypeStakeDelegation      = 2
	CertificateTypePoolRegistration     = 3
	CertificateTypePoolRetirement       = 4
	CertificateTypeGenesisKeyDelegation = 5
	CertificateTypeMoveInstantaneous    = 6
)

type Certificate interface {
	Cbor() []byte
	CertificateType() uint
	Data() *cbor.Value
	Utxorpc() *utxorpc.Certificate
}

const (
	StakeCredentialTypeAddrKeyHash = 0
	StakeCredentialTypeScriptHash  = 1
)

type StakeCredential struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	credType        uint
	StakeCredential []byte
}

func NewStakeCredentialFromCbor(credType uint, data []byte) (StakeCredential, error) {
	cred := StakeCredential{}
	switch credType {
	case StakeCredentialTypeAddrKeyHash:
		// TODO: do this
		return cred, nil
	case StakeCredentialTypeScriptHash:
		// TODO: do this
		return cred, nil
	}
	return cred, fmt.Errorf("unknown stake credential type: %d", credType)
}

type StakeRegistrationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	certType          uint
	StakeRegistration StakeCredential
}

func NewStakeRegistrationCertificateFromCbor(credType uint, data []byte) (StakeCredential, error) {
	cred := StakeCredential{
		credType: credType,
	}
	switch credType {
	case StakeCredentialTypeAddrKeyHash:
		// TODO: do this
		return cred, nil
	case StakeCredentialTypeScriptHash:
		// TODO: do this
		return cred, nil
	}
	return cred, fmt.Errorf("unknown stake credential type: %d", credType)
}

func (c *StakeRegistrationCertificate) UnmarshalCBOR(cborData []byte) error {
	return c.UnmarshalCbor(cborData, c)
}

func (c *StakeRegistrationCertificate) CertificateType() uint {
	return c.certType
}

type StakeDeregistrationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	certType            uint
	StakeDeregistration *StakeCredential
}

func (c *StakeDeregistrationCertificate) UnmarshalCBOR(cborData []byte) error {
	return c.UnmarshalCbor(cborData, c)
}

func (c *StakeDeregistrationCertificate) CertificateType() uint {
	return c.certType
}

type StakeDelegationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	// TODO: complete this
	// certType   uint
	// credential *StakeCredential
	// poolHash   PoolKeyHash
}

type AddrKeyHash Blake2b224
type PoolKeyHash Blake2b224
type PoolMetadataHash Blake2b256
type VrfKeyHash Blake2b256

type RationalNumber struct {
	Numerator   uint64
	Denominator uint64
}

type PoolMetadata struct {
	cbor.StructAsArray
	// TODO: complete this
	// url  string
	// hash PoolMetadataHash
}

type Relay struct {
	cbor.StructAsArray
	// TODO: create different relay shapes
	_ interface{}
}

type PoolRegistrationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	// certType      uint
	Operator      PoolKeyHash
	VrfKeyHash    VrfKeyHash
	Pledge        uint64
	Cost          uint64
	Margin        RationalNumber
	RewardAccount AddrKeyHash
	PoolOwners    []AddrKeyHash
	Relays        []Relay
	PoolMetadata  *PoolMetadata
}

type PoolRetirementCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	// certType    uint
	PoolKeyHash PoolKeyHash
	Epoch       uint64
}

type GenesisKeyDelegationCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	// certType            uint
	GenesisHash         []byte
	GenesisDelegateHash []byte
	VrfKeyHash          VrfKeyHash
}

type MirSource int32

const (
	MirSourceUnspecified MirSource = 0
	MirSourceReserves    MirSource = 1
	MirSourceTreasury    MirSource = 2
)

type MirTarget struct {
	cbor.StructAsArray
	StakeCredential *StakeCredential
	DeltaCoin       int64
}

type MoveInstantaneousCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	// certType uint
	From     MirSource
	To       []*MirTarget
	OtherPot uint64
}
