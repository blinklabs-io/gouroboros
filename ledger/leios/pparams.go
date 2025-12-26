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

package leios

import (
	"errors"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
)

type LeiosProtocolParameters struct {
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
	A0                         cbor.Rat
	Rho                        cbor.Rat
	Tau                        cbor.Rat
	ProtocolVersion            common.ProtocolParametersProtocolVersion
	MinPoolCost                uint64
	AdaPerUtxoByte             uint64
	CostModels                 map[uint][]int64
	ExecutionCosts             common.ExUnitPrice
	MaxTxExUnits               common.ExUnits
	MaxBlockExUnits            common.ExUnits
	MaxValueSize               uint
	CollateralPercentage       uint
	PoolVotingThresholds       conway.PoolVotingThresholds
	DRepVotingThresholds       conway.DRepVotingThresholds
	MinCommitteeSize           uint
	CommitteeTermLimit         uint64
	GovActionValidityPeriod    uint64
	GovActionDeposit           uint64
	DRepDeposit                uint64
	DRepInactivityPeriod       uint64
	MinFeeRefScriptCostPerByte cbor.Rat
	// Leios specific parameters
	HeaderDiffusionPeriodLength      uint64
	VotingPeriodLength               uint64
	DiffusionPeriodLength            uint64
	RankingBlockMaxSize              uint64
	EndorserBlockReferenceableTxSize uint64
	EndorserBlockMaxSize             uint64
	MeanCommitteeSize                uint64
	QuorumSize                       cbor.Rat
	MaxPlutusStepsPerEndorserBlock   uint64
	MaxPlutusMemoryPerEndorserBlock  uint64
	MaxPlutusStepsPerTx              uint64
	PlutusMemoryLimitPerTx           uint64
}

type LeiosProtocolParameterUpdate struct {
	cbor.DecodeStoreCbor
	MinFeeA                          *uint                                     `cbor:"0,keyasint"`
	MinFeeB                          *uint                                     `cbor:"1,keyasint"`
	MaxBlockBodySize                 *uint                                     `cbor:"2,keyasint"`
	MaxTxSize                        *uint                                     `cbor:"3,keyasint"`
	MaxBlockHeaderSize               *uint                                     `cbor:"4,keyasint"`
	KeyDeposit                       *uint                                     `cbor:"5,keyasint"`
	PoolDeposit                      *uint                                     `cbor:"6,keyasint"`
	MaxEpoch                         *uint                                     `cbor:"7,keyasint"`
	NOpt                             *uint                                     `cbor:"8,keyasint"`
	A0                               *cbor.Rat                                 `cbor:"9,keyasint"`
	Rho                              *cbor.Rat                                 `cbor:"10,keyasint"`
	Tau                              *cbor.Rat                                 `cbor:"11,keyasint"`
	ProtocolVersion                  *common.ProtocolParametersProtocolVersion `cbor:"14,keyasint"`
	MinPoolCost                      *uint64                                   `cbor:"16,keyasint"`
	AdaPerUtxoByte                   *uint64                                   `cbor:"17,keyasint"`
	CostModels                       map[uint][]int64                          `cbor:"18,keyasint"`
	ExecutionCosts                   *common.ExUnitPrice                       `cbor:"19,keyasint"`
	MaxTxExUnits                     *common.ExUnits                           `cbor:"20,keyasint"`
	MaxBlockExUnits                  *common.ExUnits                           `cbor:"21,keyasint"`
	MaxValueSize                     *uint                                     `cbor:"22,keyasint"`
	CollateralPercentage             *uint                                     `cbor:"23,keyasint"`
	PoolVotingThresholds             *conway.PoolVotingThresholds              `cbor:"25,keyasint"`
	DRepVotingThresholds             *conway.DRepVotingThresholds              `cbor:"26,keyasint"`
	MinCommitteeSize                 *uint                                     `cbor:"27,keyasint"`
	CommitteeTermLimit               *uint64                                   `cbor:"28,keyasint"`
	GovActionValidityPeriod          *uint64                                   `cbor:"29,keyasint"`
	GovActionDeposit                 *uint64                                   `cbor:"30,keyasint"`
	DRepDeposit                      *uint64                                   `cbor:"31,keyasint"`
	DRepInactivityPeriod             *uint64                                   `cbor:"32,keyasint"`
	MinFeeRefScriptCostPerByte       *cbor.Rat                                 `cbor:"33,keyasint"`
	HeaderDiffusionPeriodLength      *uint64                                   `cbor:"34,keyasint"`
	VotingPeriodLength               *uint64                                   `cbor:"35,keyasint"`
	DiffusionPeriodLength            *uint64                                   `cbor:"36,keyasint"`
	RankingBlockMaxSize              *uint64                                   `cbor:"37,keyasint"`
	EndorserBlockReferenceableTxSize *uint64                                   `cbor:"38,keyasint"`
	EndorserBlockMaxSize             *uint64                                   `cbor:"39,keyasint"`
	MeanCommitteeSize                *uint64                                   `cbor:"40,keyasint"`
	QuorumSize                       *cbor.Rat                                 `cbor:"41,keyasint"`
	MaxPlutusStepsPerEndorserBlock   *uint64                                   `cbor:"42,keyasint"`
	MaxPlutusMemoryPerEndorserBlock  *uint64                                   `cbor:"43,keyasint"`
	MaxPlutusStepsPerTx              *uint64                                   `cbor:"44,keyasint"`
	PlutusMemoryLimitPerTx           *uint64                                   `cbor:"45,keyasint"`
}

func (p *LeiosProtocolParameters) Update(
	paramUpdate *LeiosProtocolParameterUpdate,
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
		p.A0 = *paramUpdate.A0
	}
	if paramUpdate.Rho != nil {
		p.Rho = *paramUpdate.Rho
	}
	if paramUpdate.Tau != nil {
		p.Tau = *paramUpdate.Tau
	}
	if paramUpdate.ProtocolVersion != nil {
		p.ProtocolVersion = *paramUpdate.ProtocolVersion
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
	if paramUpdate.PoolVotingThresholds != nil {
		p.PoolVotingThresholds = *paramUpdate.PoolVotingThresholds
	}
	if paramUpdate.DRepVotingThresholds != nil {
		p.DRepVotingThresholds = *paramUpdate.DRepVotingThresholds
	}
	if paramUpdate.MinCommitteeSize != nil {
		p.MinCommitteeSize = *paramUpdate.MinCommitteeSize
	}
	if paramUpdate.CommitteeTermLimit != nil {
		p.CommitteeTermLimit = *paramUpdate.CommitteeTermLimit
	}
	if paramUpdate.GovActionValidityPeriod != nil {
		p.GovActionValidityPeriod = *paramUpdate.GovActionValidityPeriod
	}
	if paramUpdate.GovActionDeposit != nil {
		p.GovActionDeposit = *paramUpdate.GovActionDeposit
	}
	if paramUpdate.DRepDeposit != nil {
		p.DRepDeposit = *paramUpdate.DRepDeposit
	}
	if paramUpdate.DRepInactivityPeriod != nil {
		p.DRepInactivityPeriod = *paramUpdate.DRepInactivityPeriod
	}
	if paramUpdate.MinFeeRefScriptCostPerByte != nil {
		p.MinFeeRefScriptCostPerByte = *paramUpdate.MinFeeRefScriptCostPerByte
	}
	if paramUpdate.HeaderDiffusionPeriodLength != nil {
		p.HeaderDiffusionPeriodLength = *paramUpdate.HeaderDiffusionPeriodLength
	}
	if paramUpdate.VotingPeriodLength != nil {
		p.VotingPeriodLength = *paramUpdate.VotingPeriodLength
	}
	if paramUpdate.DiffusionPeriodLength != nil {
		p.DiffusionPeriodLength = *paramUpdate.DiffusionPeriodLength
	}
	if paramUpdate.RankingBlockMaxSize != nil {
		p.RankingBlockMaxSize = *paramUpdate.RankingBlockMaxSize
	}
	if paramUpdate.EndorserBlockReferenceableTxSize != nil {
		p.EndorserBlockReferenceableTxSize = *paramUpdate.EndorserBlockReferenceableTxSize
	}
	if paramUpdate.EndorserBlockMaxSize != nil {
		p.EndorserBlockMaxSize = *paramUpdate.EndorserBlockMaxSize
	}
	if paramUpdate.MeanCommitteeSize != nil {
		p.MeanCommitteeSize = *paramUpdate.MeanCommitteeSize
	}
	if paramUpdate.QuorumSize != nil {
		p.QuorumSize = *paramUpdate.QuorumSize
	}
	if paramUpdate.MaxPlutusStepsPerEndorserBlock != nil {
		p.MaxPlutusStepsPerEndorserBlock = *paramUpdate.MaxPlutusStepsPerEndorserBlock
	}
	if paramUpdate.MaxPlutusMemoryPerEndorserBlock != nil {
		p.MaxPlutusMemoryPerEndorserBlock = *paramUpdate.MaxPlutusMemoryPerEndorserBlock
	}
	if paramUpdate.MaxPlutusStepsPerTx != nil {
		p.MaxPlutusStepsPerTx = *paramUpdate.MaxPlutusStepsPerTx
	}
	if paramUpdate.PlutusMemoryLimitPerTx != nil {
		p.PlutusMemoryLimitPerTx = *paramUpdate.PlutusMemoryLimitPerTx
	}
}

func (*LeiosProtocolParameterUpdate) IsProtocolParameterUpdate() {}

func (u *LeiosProtocolParameterUpdate) UnmarshalCBOR(cborData []byte) error {
	type tLeiosProtocolParameterUpdate LeiosProtocolParameterUpdate
	var tmp tLeiosProtocolParameterUpdate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*u = LeiosProtocolParameterUpdate(tmp)
	u.SetCbor(cborData)
	return nil
}

func (p *LeiosProtocolParameters) Utxorpc() (any, error) {
	return nil, errors.New("not implemented")
}
