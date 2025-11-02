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

package shelley

import (
	"errors"
	"math"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	cardano "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

type ShelleyProtocolParameters struct {
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
}

func (p *ShelleyProtocolParameters) Update(
	paramUpdate *ShelleyProtocolParameterUpdate,
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
		p.A0 = paramUpdate.A0
	}
	if paramUpdate.Rho != nil {
		p.Rho = paramUpdate.Rho
	}
	if paramUpdate.Tau != nil {
		p.Tau = paramUpdate.Tau
	}
	if paramUpdate.Decentralization != nil {
		p.Decentralization = paramUpdate.Decentralization
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
}

func (p *ShelleyProtocolParameters) UpdateFromGenesis(
	genesis *ShelleyGenesis,
) error {
	if genesis == nil {
		return nil
	}
	genesisParams := genesis.ProtocolParameters
	p.MinFeeA = genesisParams.MinFeeA
	p.MinFeeB = genesisParams.MinFeeB
	p.MaxBlockBodySize = genesisParams.MaxBlockBodySize
	p.MaxTxSize = genesisParams.MaxTxSize
	p.MaxBlockHeaderSize = genesisParams.MaxBlockHeaderSize
	p.KeyDeposit = genesisParams.KeyDeposit
	p.PoolDeposit = genesisParams.PoolDeposit
	p.MaxEpoch = genesisParams.MaxEpoch
	p.NOpt = genesisParams.NOpt
	if genesisParams.A0 != nil {
		p.A0 = &cbor.Rat{Rat: new(big.Rat).Set(genesisParams.A0.Rat)}
	}
	if genesisParams.Rho != nil {
		p.Rho = &cbor.Rat{Rat: new(big.Rat).Set(genesisParams.Rho.Rat)}
	}
	if genesisParams.Tau != nil {
		p.Tau = &cbor.Rat{Rat: new(big.Rat).Set(genesisParams.Tau.Rat)}
	}
	if genesisParams.Decentralization != nil {
		p.Decentralization = &cbor.Rat{
			Rat: new(big.Rat).Set(genesisParams.Decentralization.Rat),
		}
	}
	p.ExtraEntropy = genesisParams.ExtraEntropy
	p.ProtocolMajor = genesisParams.ProtocolVersion.Major
	p.ProtocolMinor = genesisParams.ProtocolVersion.Minor
	p.MinUtxoValue = genesisParams.MinUtxoValue
	return nil
}

type ShelleyProtocolParameterUpdate struct {
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
}

func (ShelleyProtocolParameterUpdate) IsProtocolParameterUpdate() {}

func (u *ShelleyProtocolParameterUpdate) UnmarshalCBOR(cborData []byte) error {
	type tShelleyProtocolParameterUpdate ShelleyProtocolParameterUpdate
	var tmp tShelleyProtocolParameterUpdate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*u = ShelleyProtocolParameterUpdate(tmp)
	u.SetCbor(cborData)
	return nil
}

func (p *ShelleyProtocolParameters) Utxorpc() (*cardano.PParams, error) {
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
	return &cardano.PParams{
		MaxTxSize:                uint64(p.MaxTxSize),
		MinFeeCoefficient:        uint64(p.MinFeeA),
		MinFeeConstant:           uint64(p.MinFeeB),
		MaxBlockBodySize:         uint64(p.MaxBlockBodySize),
		MaxBlockHeaderSize:       uint64(p.MaxBlockHeaderSize),
		StakeKeyDeposit:          uint64(p.KeyDeposit),
		PoolDeposit:              uint64(p.PoolDeposit),
		PoolRetirementEpochBound: uint64(p.MaxEpoch),
		DesiredNumberOfPools:     uint64(p.NOpt),
		PoolInfluence: &cardano.RationalNumber{
			Numerator:   int32(p.A0.Num().Int64()),
			Denominator: uint32(p.A0.Denom().Int64()),
		},
		MonetaryExpansion: &cardano.RationalNumber{
			Numerator:   int32(p.Rho.Num().Int64()),
			Denominator: uint32(p.Rho.Denom().Int64()),
		},
		TreasuryExpansion: &cardano.RationalNumber{
			Numerator:   int32(p.Tau.Num().Int64()),
			Denominator: uint32(p.Tau.Denom().Int64()),
		},
		ProtocolVersion: &cardano.ProtocolVersion{
			Major: uint32(p.ProtocolMajor),
			Minor: uint32(p.ProtocolMinor),
		},
	}, nil
}

func UpgradePParams(prevPParams any) ShelleyProtocolParameters {
	// No upgrade from Byron
	return ShelleyProtocolParameters{}
}
