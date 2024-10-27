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

package babbage

import (
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

// BabbageProtocolParameters represents the current Babbage protocol parameters as seen in local-state-query
type BabbageProtocolParameters struct {
	cbor.StructAsArray
	MinFeeA              uint
	MinFeeB              uint
	MaxBlockBodySize     uint
	MaxTxSize            uint
	MaxBlockHeaderSize   uint
	KeyDeposit           uint
	PoolDeposit          uint
	MaxEpoch             uint
	NOpt                 uint
	A0                   *cbor.Rat
	Rho                  *cbor.Rat
	Tau                  *cbor.Rat
	ProtocolMajor        uint
	ProtocolMinor        uint
	MinPoolCost          uint64
	AdaPerUtxoByte       uint64
	CostModels           map[uint][]int64
	ExecutionCosts       common.ExUnitPrice
	MaxTxExUnits         common.ExUnit
	MaxBlockExUnits      common.ExUnit
	MaxValueSize         uint
	CollateralPercentage uint
	MaxCollateralInputs  uint
}

func (p *BabbageProtocolParameters) Update(
	paramUpdate *BabbageProtocolParameterUpdate,
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
	if paramUpdate.ProtocolVersion != nil {
		p.ProtocolMajor = paramUpdate.ProtocolVersion.Major
		p.ProtocolMinor = paramUpdate.ProtocolVersion.Minor
	}
	if paramUpdate.MinPoolCost != nil {
		p.MinPoolCost = *paramUpdate.MinPoolCost
	}
	if paramUpdate.AdaPerUtxoByte != nil {
		p.AdaPerUtxoByte = *paramUpdate.AdaPerUtxoByte
	}
	if paramUpdate.CostModels != nil {
		p.CostModels = paramUpdate.CostModels
	}
	if paramUpdate.ExecutionCosts != nil {
		p.ExecutionCosts = *paramUpdate.ExecutionCosts
	}
	if paramUpdate.MaxTxExUnits != nil {
		p.MaxTxExUnits = *paramUpdate.MaxTxExUnits
	}
	if paramUpdate.MaxBlockExUnits != nil {
		p.MaxBlockExUnits = *paramUpdate.MaxBlockExUnits
	}
	if paramUpdate.MaxValueSize != nil {
		p.MaxValueSize = *paramUpdate.MaxValueSize
	}
	if paramUpdate.CollateralPercentage != nil {
		p.CollateralPercentage = *paramUpdate.CollateralPercentage
	}
	if paramUpdate.MaxCollateralInputs != nil {
		p.MaxCollateralInputs = *paramUpdate.MaxCollateralInputs
	}
}

type BabbageProtocolParameterUpdate struct {
	cbor.DecodeStoreCbor
	MinFeeA              *uint                                     `cbor:"0,keyasint"`
	MinFeeB              *uint                                     `cbor:"1,keyasint"`
	MaxBlockBodySize     *uint                                     `cbor:"2,keyasint"`
	MaxTxSize            *uint                                     `cbor:"3,keyasint"`
	MaxBlockHeaderSize   *uint                                     `cbor:"4,keyasint"`
	KeyDeposit           *uint                                     `cbor:"5,keyasint"`
	PoolDeposit          *uint                                     `cbor:"6,keyasint"`
	MaxEpoch             *uint                                     `cbor:"7,keyasint"`
	NOpt                 *uint                                     `cbor:"8,keyasint"`
	A0                   *cbor.Rat                                 `cbor:"9,keyasint"`
	Rho                  *cbor.Rat                                 `cbor:"10,keyasint"`
	Tau                  *cbor.Rat                                 `cbor:"11,keyasint"`
	ProtocolVersion      *common.ProtocolParametersProtocolVersion `cbor:"14,keyasint"`
	MinPoolCost          *uint64                                   `cbor:"16,keyasint"`
	AdaPerUtxoByte       *uint64                                   `cbor:"17,keyasint"`
	CostModels           map[uint][]int64                          `cbor:"18,keyasint"`
	ExecutionCosts       *common.ExUnitPrice                       `cbor:"19,keyasint"`
	MaxTxExUnits         *common.ExUnit                            `cbor:"20,keyasint"`
	MaxBlockExUnits      *common.ExUnit                            `cbor:"21,keyasint"`
	MaxValueSize         *uint                                     `cbor:"22,keyasint"`
	CollateralPercentage *uint                                     `cbor:"23,keyasint"`
	MaxCollateralInputs  *uint                                     `cbor:"24,keyasint"`
}

func (BabbageProtocolParameterUpdate) IsProtocolParameterUpdate() {}

func (u *BabbageProtocolParameterUpdate) UnmarshalCBOR(data []byte) error {
	return u.UnmarshalCbor(data, u)
}

func (p *BabbageProtocolParameters) Utxorpc() *cardano.PParams {
	// TODO: Implement the conversion logic to cardano.PParams
	return &cardano.PParams{}
}
