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

package dijkstra

import (
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

type DijkstraProtocolParameters struct {
	conway.ConwayProtocolParameters
	// TODO: Wire reference-script size limits into Dijkstra validation rules.
	MaxRefScriptSizePerBlock uint32
	MaxRefScriptSizePerTx    uint32
	RefScriptCostStride      uint32
	RefScriptCostMultiplier  *cbor.Rat
}

type dijkstraProtocolParametersCbor struct {
	cbor.StructAsArray
	MinFeeA                    uint
	MinFeeB                    uint
	MaxBlockBodySize           uint
	MaxTxSize                  uint
	MaxBlockHeaderSize         uint
	KeyDeposit                 uint
	PoolDeposit                uint
	MaxEpoch                   uint
	NOpt                       uint
	A0                         *cbor.Rat
	Rho                        *cbor.Rat
	Tau                        *cbor.Rat
	ProtocolVersion            common.ProtocolParametersProtocolVersion
	MinPoolCost                uint64
	AdaPerUtxoByte             uint64
	CostModels                 map[uint][]int64
	ExecutionCosts             common.ExUnitPrice
	MaxTxExUnits               common.ExUnits
	MaxBlockExUnits            common.ExUnits
	MaxValueSize               uint
	CollateralPercentage       uint
	MaxCollateralInputs        uint
	PoolVotingThresholds       conway.PoolVotingThresholds
	DRepVotingThresholds       conway.DRepVotingThresholds
	MinCommitteeSize           uint
	CommitteeTermLimit         uint64
	GovActionValidityPeriod    uint64
	GovActionDeposit           uint64
	DRepDeposit                uint64
	DRepInactivityPeriod       uint64
	MinFeeRefScriptCostPerByte *cbor.Rat
	MaxRefScriptSizePerBlock   uint32
	MaxRefScriptSizePerTx      uint32
	RefScriptCostStride        uint32
	RefScriptCostMultiplier    *cbor.Rat
}

func (p *DijkstraProtocolParameters) UnmarshalCBOR(cborData []byte) error {
	var tmp dijkstraProtocolParametersCbor
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	p.ConwayProtocolParameters = conway.ConwayProtocolParameters{
		MinFeeA:                    tmp.MinFeeA,
		MinFeeB:                    tmp.MinFeeB,
		MaxBlockBodySize:           tmp.MaxBlockBodySize,
		MaxTxSize:                  tmp.MaxTxSize,
		MaxBlockHeaderSize:         tmp.MaxBlockHeaderSize,
		KeyDeposit:                 tmp.KeyDeposit,
		PoolDeposit:                tmp.PoolDeposit,
		MaxEpoch:                   tmp.MaxEpoch,
		NOpt:                       tmp.NOpt,
		A0:                         tmp.A0,
		Rho:                        tmp.Rho,
		Tau:                        tmp.Tau,
		ProtocolVersion:            tmp.ProtocolVersion,
		MinPoolCost:                tmp.MinPoolCost,
		AdaPerUtxoByte:             tmp.AdaPerUtxoByte,
		CostModels:                 tmp.CostModels,
		ExecutionCosts:             tmp.ExecutionCosts,
		MaxTxExUnits:               tmp.MaxTxExUnits,
		MaxBlockExUnits:            tmp.MaxBlockExUnits,
		MaxValueSize:               tmp.MaxValueSize,
		CollateralPercentage:       tmp.CollateralPercentage,
		MaxCollateralInputs:        tmp.MaxCollateralInputs,
		PoolVotingThresholds:       tmp.PoolVotingThresholds,
		DRepVotingThresholds:       tmp.DRepVotingThresholds,
		MinCommitteeSize:           tmp.MinCommitteeSize,
		CommitteeTermLimit:         tmp.CommitteeTermLimit,
		GovActionValidityPeriod:    tmp.GovActionValidityPeriod,
		GovActionDeposit:           tmp.GovActionDeposit,
		DRepDeposit:                tmp.DRepDeposit,
		DRepInactivityPeriod:       tmp.DRepInactivityPeriod,
		MinFeeRefScriptCostPerByte: tmp.MinFeeRefScriptCostPerByte,
	}
	p.MaxRefScriptSizePerBlock = tmp.MaxRefScriptSizePerBlock
	p.MaxRefScriptSizePerTx = tmp.MaxRefScriptSizePerTx
	p.RefScriptCostStride = tmp.RefScriptCostStride
	p.RefScriptCostMultiplier = tmp.RefScriptCostMultiplier
	return nil
}

func (p DijkstraProtocolParameters) MarshalCBOR() ([]byte, error) {
	return cbor.Encode(p.toCbor())
}

func (p DijkstraProtocolParameters) toCbor() dijkstraProtocolParametersCbor {
	return dijkstraProtocolParametersCbor{
		MinFeeA:                    p.MinFeeA,
		MinFeeB:                    p.MinFeeB,
		MaxBlockBodySize:           p.MaxBlockBodySize,
		MaxTxSize:                  p.MaxTxSize,
		MaxBlockHeaderSize:         p.MaxBlockHeaderSize,
		KeyDeposit:                 p.KeyDeposit,
		PoolDeposit:                p.PoolDeposit,
		MaxEpoch:                   p.MaxEpoch,
		NOpt:                       p.NOpt,
		A0:                         p.A0,
		Rho:                        p.Rho,
		Tau:                        p.Tau,
		ProtocolVersion:            p.ProtocolVersion,
		MinPoolCost:                p.MinPoolCost,
		AdaPerUtxoByte:             p.AdaPerUtxoByte,
		CostModels:                 p.CostModels,
		ExecutionCosts:             p.ExecutionCosts,
		MaxTxExUnits:               p.MaxTxExUnits,
		MaxBlockExUnits:            p.MaxBlockExUnits,
		MaxValueSize:               p.MaxValueSize,
		CollateralPercentage:       p.CollateralPercentage,
		MaxCollateralInputs:        p.MaxCollateralInputs,
		PoolVotingThresholds:       p.PoolVotingThresholds,
		DRepVotingThresholds:       p.DRepVotingThresholds,
		MinCommitteeSize:           p.MinCommitteeSize,
		CommitteeTermLimit:         p.CommitteeTermLimit,
		GovActionValidityPeriod:    p.GovActionValidityPeriod,
		GovActionDeposit:           p.GovActionDeposit,
		DRepDeposit:                p.DRepDeposit,
		DRepInactivityPeriod:       p.DRepInactivityPeriod,
		MinFeeRefScriptCostPerByte: p.MinFeeRefScriptCostPerByte,
		MaxRefScriptSizePerBlock:   p.MaxRefScriptSizePerBlock,
		MaxRefScriptSizePerTx:      p.MaxRefScriptSizePerTx,
		RefScriptCostStride:        p.RefScriptCostStride,
		RefScriptCostMultiplier:    p.RefScriptCostMultiplier,
	}
}

func (p *DijkstraProtocolParameters) Update(
	paramUpdate *DijkstraProtocolParameterUpdate,
) {
	if paramUpdate == nil {
		return
	}
	p.ConwayProtocolParameters.Update(paramUpdate.conwayUpdate())
	if paramUpdate.MaxRefScriptSizePerBlock != nil {
		p.MaxRefScriptSizePerBlock = *paramUpdate.MaxRefScriptSizePerBlock
	}
	if paramUpdate.MaxRefScriptSizePerTx != nil {
		p.MaxRefScriptSizePerTx = *paramUpdate.MaxRefScriptSizePerTx
	}
	if paramUpdate.RefScriptCostStride != nil {
		p.RefScriptCostStride = *paramUpdate.RefScriptCostStride
	}
	if paramUpdate.RefScriptCostMultiplier != nil {
		p.RefScriptCostMultiplier = paramUpdate.RefScriptCostMultiplier
	}
}

func (p *DijkstraProtocolParameters) Utxorpc() (*utxorpc.PParams, error) {
	return p.ConwayProtocolParameters.Utxorpc()
}

func (p *DijkstraProtocolParameters) KeyDepositAmount() *big.Int {
	return p.ConwayProtocolParameters.KeyDepositAmount()
}

func (p *DijkstraProtocolParameters) PoolDepositAmount() *big.Int {
	return p.ConwayProtocolParameters.PoolDepositAmount()
}

func (p *DijkstraProtocolParameters) MinPoolCostAmount() *big.Int {
	return p.ConwayProtocolParameters.MinPoolCostAmount()
}

func (p *DijkstraProtocolParameters) AdaPerUtxoByteAmount() *big.Int {
	return p.ConwayProtocolParameters.AdaPerUtxoByteAmount()
}

func (p *DijkstraProtocolParameters) GovActionDepositAmount() *big.Int {
	return p.ConwayProtocolParameters.GovActionDepositAmount()
}

func (p *DijkstraProtocolParameters) DRepDepositAmount() *big.Int {
	return p.ConwayProtocolParameters.DRepDepositAmount()
}

type DijkstraProtocolParameterUpdate struct {
	cbor.DecodeStoreCbor
	MinFeeA                    *uint                                     `cbor:"0,keyasint"`
	MinFeeB                    *uint                                     `cbor:"1,keyasint"`
	MaxBlockBodySize           *uint                                     `cbor:"2,keyasint"`
	MaxTxSize                  *uint                                     `cbor:"3,keyasint"`
	MaxBlockHeaderSize         *uint                                     `cbor:"4,keyasint"`
	KeyDeposit                 *uint                                     `cbor:"5,keyasint"`
	PoolDeposit                *uint                                     `cbor:"6,keyasint"`
	MaxEpoch                   *uint                                     `cbor:"7,keyasint"`
	NOpt                       *uint                                     `cbor:"8,keyasint"`
	A0                         *cbor.Rat                                 `cbor:"9,keyasint"`
	Rho                        *cbor.Rat                                 `cbor:"10,keyasint"`
	Tau                        *cbor.Rat                                 `cbor:"11,keyasint"`
	ProtocolVersion            *common.ProtocolParametersProtocolVersion `cbor:"14,keyasint"`
	MinPoolCost                *uint64                                   `cbor:"16,keyasint"`
	AdaPerUtxoByte             *uint64                                   `cbor:"17,keyasint"`
	CostModels                 map[uint][]int64                          `cbor:"18,keyasint"`
	ExecutionCosts             *common.ExUnitPrice                       `cbor:"19,keyasint"`
	MaxTxExUnits               *common.ExUnits                           `cbor:"20,keyasint"`
	MaxBlockExUnits            *common.ExUnits                           `cbor:"21,keyasint"`
	MaxValueSize               *uint                                     `cbor:"22,keyasint"`
	CollateralPercentage       *uint                                     `cbor:"23,keyasint"`
	MaxCollateralInputs        *uint                                     `cbor:"24,keyasint"`
	PoolVotingThresholds       *conway.PoolVotingThresholds              `cbor:"25,keyasint"`
	DRepVotingThresholds       *conway.DRepVotingThresholds              `cbor:"26,keyasint"`
	MinCommitteeSize           *uint                                     `cbor:"27,keyasint"`
	CommitteeTermLimit         *uint64                                   `cbor:"28,keyasint"`
	GovActionValidityPeriod    *uint64                                   `cbor:"29,keyasint"`
	GovActionDeposit           *uint64                                   `cbor:"30,keyasint"`
	DRepDeposit                *uint64                                   `cbor:"31,keyasint"`
	DRepInactivityPeriod       *uint64                                   `cbor:"32,keyasint"`
	MinFeeRefScriptCostPerByte *cbor.Rat                                 `cbor:"33,keyasint"`
	MaxRefScriptSizePerBlock   *uint32                                   `cbor:"34,keyasint"`
	MaxRefScriptSizePerTx      *uint32                                   `cbor:"35,keyasint"`
	RefScriptCostStride        *uint32                                   `cbor:"36,keyasint"`
	RefScriptCostMultiplier    *cbor.Rat                                 `cbor:"37,keyasint"`
}

func (DijkstraProtocolParameterUpdate) IsProtocolParameterUpdate() {}

func (u *DijkstraProtocolParameterUpdate) UnmarshalCBOR(cborData []byte) error {
	type tDijkstraProtocolParameterUpdate DijkstraProtocolParameterUpdate
	var tmp tDijkstraProtocolParameterUpdate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*u = DijkstraProtocolParameterUpdate(tmp)
	u.SetCbor(cborData)
	return nil
}

func (u DijkstraProtocolParameterUpdate) Cbor() []byte {
	return u.DecodeStoreCbor.Cbor()
}

func (u *DijkstraProtocolParameterUpdate) conwayUpdate() *conway.ConwayProtocolParameterUpdate {
	return &conway.ConwayProtocolParameterUpdate{
		MinFeeA:                    u.MinFeeA,
		MinFeeB:                    u.MinFeeB,
		MaxBlockBodySize:           u.MaxBlockBodySize,
		MaxTxSize:                  u.MaxTxSize,
		MaxBlockHeaderSize:         u.MaxBlockHeaderSize,
		KeyDeposit:                 u.KeyDeposit,
		PoolDeposit:                u.PoolDeposit,
		MaxEpoch:                   u.MaxEpoch,
		NOpt:                       u.NOpt,
		A0:                         u.A0,
		Rho:                        u.Rho,
		Tau:                        u.Tau,
		ProtocolVersion:            u.ProtocolVersion,
		MinPoolCost:                u.MinPoolCost,
		AdaPerUtxoByte:             u.AdaPerUtxoByte,
		CostModels:                 u.CostModels,
		ExecutionCosts:             u.ExecutionCosts,
		MaxTxExUnits:               u.MaxTxExUnits,
		MaxBlockExUnits:            u.MaxBlockExUnits,
		MaxValueSize:               u.MaxValueSize,
		CollateralPercentage:       u.CollateralPercentage,
		MaxCollateralInputs:        u.MaxCollateralInputs,
		PoolVotingThresholds:       u.PoolVotingThresholds,
		DRepVotingThresholds:       u.DRepVotingThresholds,
		MinCommitteeSize:           u.MinCommitteeSize,
		CommitteeTermLimit:         u.CommitteeTermLimit,
		GovActionValidityPeriod:    u.GovActionValidityPeriod,
		GovActionDeposit:           u.GovActionDeposit,
		DRepDeposit:                u.DRepDeposit,
		DRepInactivityPeriod:       u.DRepInactivityPeriod,
		MinFeeRefScriptCostPerByte: u.MinFeeRefScriptCostPerByte,
	}
}

var (
	_ common.ProtocolParameters      = (*DijkstraProtocolParameters)(nil)
	_ common.ProtocolParameterUpdate = (*DijkstraProtocolParameterUpdate)(nil)
)
