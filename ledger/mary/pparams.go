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

package mary

import (
	"errors"
	"math"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

type MaryProtocolParameters struct {
	cbor.StructAsArray
	MinFeeA            uint
	MinFeeB            uint
	MaxBlockBodySize   uint
	MaxTxSize          uint
	MaxBlockHeaderSize uint
	KeyDeposit         uint
	PoolDeposit        uint
	MaxEpoch           uint
	NOpt               uint
	A0                 *cbor.Rat
	Rho                *cbor.Rat
	Tau                *cbor.Rat
	Decentralization   *cbor.Rat
	ExtraEntropy       common.Nonce
	ProtocolMajor      uint
	ProtocolMinor      uint
	MinUtxoValue       uint
	MinPoolCost        uint64
}

func (p *MaryProtocolParameters) Update(
	paramUpdate *MaryProtocolParameterUpdate,
) {
	if paramUpdate.MinFeeA != nil {
		p.MinFeeA = *paramUpdate.MinFeeA
	}
	if paramUpdate.MinFeeB != nil {
		p.MinFeeB = *paramUpdate.MinFeeB
	}
	if paramUpdate.MaxBlockBodySize != nil {
		p.MaxBlockBodySize = *paramUpdate.MaxBlockBodySize
	}
	if paramUpdate.MaxTxSize != nil {
		p.MaxTxSize = *paramUpdate.MaxTxSize
	}
	if paramUpdate.MaxBlockHeaderSize != nil {
		p.MaxBlockHeaderSize = *paramUpdate.MaxBlockHeaderSize
	}
	if paramUpdate.KeyDeposit != nil {
		p.KeyDeposit = *paramUpdate.KeyDeposit
	}
	if paramUpdate.PoolDeposit != nil {
		p.PoolDeposit = *paramUpdate.PoolDeposit
	}
	if paramUpdate.MaxEpoch != nil {
		p.MaxEpoch = *paramUpdate.MaxEpoch
	}
	if paramUpdate.NOpt != nil {
		p.NOpt = *paramUpdate.NOpt
	}
	if paramUpdate.A0 != nil {
		p.A0 = &cbor.Rat{Rat: new(big.Rat).Set(paramUpdate.A0.Rat)}
	}
	if paramUpdate.Rho != nil {
		p.Rho = &cbor.Rat{Rat: new(big.Rat).Set(paramUpdate.Rho.Rat)}
	}
	if paramUpdate.Tau != nil {
		p.Tau = &cbor.Rat{Rat: new(big.Rat).Set(paramUpdate.Tau.Rat)}
	}
	if paramUpdate.Decentralization != nil {
		p.Decentralization = &cbor.Rat{
			Rat: new(big.Rat).Set(paramUpdate.Decentralization.Rat),
		}
	}
	if paramUpdate.ProtocolVersion != nil {
		p.ProtocolMajor = paramUpdate.ProtocolVersion.Major
		p.ProtocolMinor = paramUpdate.ProtocolVersion.Minor
	}
	if paramUpdate.ExtraEntropy != nil {
		p.ExtraEntropy = *paramUpdate.ExtraEntropy
	}
	if paramUpdate.MinUtxoValue != nil {
		p.MinUtxoValue = *paramUpdate.MinUtxoValue
	}
	if paramUpdate.MinPoolCost != nil {
		p.MinPoolCost = *paramUpdate.MinPoolCost
	}
}

type MaryProtocolParameterUpdate struct {
	cbor.DecodeStoreCbor
	MinFeeA            *uint                                     `cbor:"0,keyasint"`
	MinFeeB            *uint                                     `cbor:"1,keyasint"`
	MaxBlockBodySize   *uint                                     `cbor:"2,keyasint"`
	MaxTxSize          *uint                                     `cbor:"3,keyasint"`
	MaxBlockHeaderSize *uint                                     `cbor:"4,keyasint"`
	KeyDeposit         *uint                                     `cbor:"5,keyasint"`
	PoolDeposit        *uint                                     `cbor:"6,keyasint"`
	MaxEpoch           *uint                                     `cbor:"7,keyasint"`
	NOpt               *uint                                     `cbor:"8,keyasint"`
	A0                 *cbor.Rat                                 `cbor:"9,keyasint"`
	Rho                *cbor.Rat                                 `cbor:"10,keyasint"`
	Tau                *cbor.Rat                                 `cbor:"11,keyasint"`
	Decentralization   *cbor.Rat                                 `cbor:"12,keyasint"`
	ExtraEntropy       *common.Nonce                             `cbor:"13,keyasint"`
	ProtocolVersion    *common.ProtocolParametersProtocolVersion `cbor:"14,keyasint"`
	MinUtxoValue       *uint                                     `cbor:"15,keyasint"`
	MinPoolCost        *uint64                                   `cbor:"16,keyasint"`
}

func (*MaryProtocolParameterUpdate) IsProtocolParameterUpdate() {}

func (u *MaryProtocolParameterUpdate) UnmarshalCBOR(cborData []byte) error {
	type tMaryProtocolParameterUpdate MaryProtocolParameterUpdate
	var tmp tMaryProtocolParameterUpdate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*u = MaryProtocolParameterUpdate(tmp)
	u.SetCbor(cborData)
	return nil
}

func (p *MaryProtocolParameterUpdate) MarshalCBOR() ([]byte, error) {
	// Return the stored CBOR if available
	if p.Cbor() != nil {
		return p.Cbor(), nil
	}
	// Otherwise, encode generically
	return cbor.EncodeGeneric(p)
}

func (p *MaryProtocolParameters) Utxorpc() (*utxorpc.PParams, error) {
	// sanity check
	if p.A0 == nil ||
		p.A0.Num().Int64() < math.MinInt32 ||
		p.A0.Num().Int64() > math.MaxInt32 ||
		p.A0.Denom().Int64() < 0 ||
		p.A0.Denom().Int64() > math.MaxUint32 {
		return nil, errors.New("invalid A0 rational number values")
	}
	if p.Rho == nil ||
		p.Rho.Num().Int64() < math.MinInt32 ||
		p.Rho.Num().Int64() > math.MaxInt32 ||
		p.Rho.Denom().Int64() < 0 ||
		p.Rho.Denom().Int64() > math.MaxUint32 {
		return nil, errors.New("invalid Rho rational number values")
	}
	if p.Tau == nil ||
		p.Tau.Num().Int64() < math.MinInt32 ||
		p.Tau.Num().Int64() > math.MaxInt32 ||
		p.Tau.Denom().Int64() < 0 ||
		p.Tau.Denom().Int64() > math.MaxUint32 {
		return nil, errors.New("invalid Tau rational number values")
	}
	// #nosec G115
	return &utxorpc.PParams{
		MaxTxSize:                uint64(p.MaxTxSize),
		MinFeeCoefficient:        common.ToUtxorpcBigInt(uint64(p.MinFeeA)),
		MinFeeConstant:           common.ToUtxorpcBigInt(uint64(p.MinFeeB)),
		MaxBlockBodySize:         uint64(p.MaxBlockBodySize),
		MaxBlockHeaderSize:       uint64(p.MaxBlockHeaderSize),
		StakeKeyDeposit:          common.ToUtxorpcBigInt(uint64(p.KeyDeposit)),
		PoolDeposit:              common.ToUtxorpcBigInt(uint64(p.PoolDeposit)),
		MinPoolCost:              common.ToUtxorpcBigInt(p.MinPoolCost),
		PoolRetirementEpochBound: uint64(p.MaxEpoch),
		DesiredNumberOfPools:     uint64(p.NOpt),
		PoolInfluence: &utxorpc.RationalNumber{
			Numerator:   int32(p.A0.Num().Int64()),
			Denominator: uint32(p.A0.Denom().Int64()),
		},
		MonetaryExpansion: &utxorpc.RationalNumber{
			Numerator:   int32(p.Rho.Num().Int64()),
			Denominator: uint32(p.Rho.Denom().Int64()),
		},
		TreasuryExpansion: &utxorpc.RationalNumber{
			Numerator:   int32(p.Tau.Num().Int64()),
			Denominator: uint32(p.Tau.Denom().Int64()),
		},
		ProtocolVersion: &utxorpc.ProtocolVersion{
			Major: uint32(p.ProtocolMajor),
			Minor: uint32(p.ProtocolMinor),
		},
	}, nil
}

func UpgradePParams(
	prevPParams shelley.ShelleyProtocolParameters,
) MaryProtocolParameters {
	return MaryProtocolParameters{
		MinFeeA:            prevPParams.MinFeeA,
		MinFeeB:            prevPParams.MinFeeB,
		MaxBlockBodySize:   prevPParams.MaxBlockBodySize,
		MaxTxSize:          prevPParams.MaxTxSize,
		MaxBlockHeaderSize: prevPParams.MaxBlockHeaderSize,
		KeyDeposit:         prevPParams.KeyDeposit,
		PoolDeposit:        prevPParams.PoolDeposit,
		MaxEpoch:           prevPParams.MaxEpoch,
		NOpt:               prevPParams.NOpt,
		A0:                 prevPParams.A0,
		Rho:                prevPParams.Rho,
		Tau:                prevPParams.Tau,
		Decentralization:   prevPParams.Decentralization,
		ExtraEntropy:       prevPParams.ExtraEntropy,
		ProtocolMajor:      prevPParams.ProtocolMajor,
		ProtocolMinor:      prevPParams.ProtocolMinor,
		MinUtxoValue:       prevPParams.MinUtxoValue,
		MinPoolCost:        340000000, // initial MinPoolCost at Mary on mainnet
	}
}
