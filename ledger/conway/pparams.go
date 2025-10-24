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

package conway

import (
	"errors"
	"math"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/data"
	cardano "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

type ConwayProtocolParameters struct {
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
	PoolVotingThresholds       PoolVotingThresholds
	DRepVotingThresholds       DRepVotingThresholds
	MinCommitteeSize           uint
	CommitteeTermLimit         uint64
	GovActionValidityPeriod    uint64
	GovActionDeposit           uint64
	DRepDeposit                uint64
	DRepInactivityPeriod       uint64
	MinFeeRefScriptCostPerByte *cbor.Rat
}

func (p *ConwayProtocolParameters) Utxorpc() (*cardano.PParams, error) {
	// sanity check
	if p.A0.Num().Int64() > math.MaxInt32 ||
		p.A0.Denom().Int64() < 0 ||
		p.A0.Denom().Int64() > math.MaxUint32 {
		return nil, errors.New("invalid A0 rational number values")
	}
	if p.Rho.Num().Int64() > math.MaxInt32 ||
		p.Rho.Denom().Int64() < 0 ||
		p.Rho.Denom().Int64() > math.MaxUint32 {
		return nil, errors.New("invalid Rho rational number values")
	}
	if p.Tau.Num().Int64() > math.MaxInt32 ||
		p.Tau.Denom().Int64() < 0 ||
		p.Tau.Denom().Int64() > math.MaxUint32 {
		return nil, errors.New("invalid Tau rational number values")
	}
	if p.ExecutionCosts.MemPrice.Num().Int64() > math.MaxInt32 ||
		p.ExecutionCosts.MemPrice.Denom().Int64() < 0 ||
		p.ExecutionCosts.MemPrice.Denom().Int64() > math.MaxUint32 {
		return nil, errors.New("invalid memory price rational number values")
	}
	if p.ExecutionCosts.StepPrice.Num().Int64() > math.MaxInt32 ||
		p.ExecutionCosts.StepPrice.Denom().Int64() < 0 ||
		p.ExecutionCosts.StepPrice.Denom().Int64() > math.MaxUint32 {
		return nil, errors.New("invalid step price rational number values")
	}
	// #nosec G115
	return &cardano.PParams{
		CoinsPerUtxoByte:         p.AdaPerUtxoByte,
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
		MinPoolCost: p.MinPoolCost,
		ProtocolVersion: &cardano.ProtocolVersion{
			Major: uint32(p.ProtocolVersion.Major),
			Minor: uint32(p.ProtocolVersion.Minor),
		},
		MaxValueSize:         uint64(p.MaxValueSize),
		CollateralPercentage: uint64(p.CollateralPercentage),
		MaxCollateralInputs:  uint64(p.MaxCollateralInputs),
		CostModels: common.ConvertToUtxorpcCardanoCostModels(
			p.CostModels,
		),
		Prices: &cardano.ExPrices{
			Memory: &cardano.RationalNumber{
				Numerator:   int32(p.ExecutionCosts.MemPrice.Num().Int64()),
				Denominator: uint32(p.ExecutionCosts.MemPrice.Denom().Int64()),
			},
			Steps: &cardano.RationalNumber{
				Numerator:   int32(p.ExecutionCosts.StepPrice.Num().Int64()),
				Denominator: uint32(p.ExecutionCosts.StepPrice.Denom().Int64()),
			},
		},
		MaxExecutionUnitsPerTransaction: &cardano.ExUnits{
			Memory: uint64(p.MaxTxExUnits.Memory),
			Steps:  uint64(p.MaxTxExUnits.Steps),
		},
		MaxExecutionUnitsPerBlock: &cardano.ExUnits{
			Memory: uint64(p.MaxBlockExUnits.Memory),
			Steps:  uint64(p.MaxBlockExUnits.Steps),
		},
	}, nil
}

func (p *ConwayProtocolParameters) Update(
	paramUpdate *ConwayProtocolParameterUpdate,
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
	if paramUpdate.MaxCollateralInputs != nil {
		p.MaxCollateralInputs = *paramUpdate.MaxCollateralInputs
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
		p.MinFeeRefScriptCostPerByte = paramUpdate.MinFeeRefScriptCostPerByte
	}
}

func (p *ConwayProtocolParameters) UpdateFromGenesis(
	genesis *ConwayGenesis,
) error {
	if genesis == nil {
		return nil
	}
	p.MinCommitteeSize = genesis.MinCommitteeSize
	p.CommitteeTermLimit = genesis.CommitteeTermLimit
	p.GovActionValidityPeriod = genesis.GovActionValidityPeriod
	p.GovActionDeposit = genesis.GovActionDeposit
	p.DRepDeposit = genesis.DRepDeposit
	p.DRepInactivityPeriod = genesis.DRepInactivityPeriod
	if genesis.MinFeeRefScriptCostPerByte != nil {
		p.MinFeeRefScriptCostPerByte = &cbor.Rat{
			Rat: genesis.MinFeeRefScriptCostPerByte.Rat,
		}
	}
	if len(genesis.PlutusV3CostModel) > 0 {
		if p.CostModels == nil {
			p.CostModels = make(map[uint][]int64)
		}
		p.CostModels[2] = genesis.PlutusV3CostModel
	}
	if genesis.PoolVotingThresholds.MotionNoConfidence != nil {
		p.PoolVotingThresholds.MotionNoConfidence = cbor.Rat{
			Rat: genesis.PoolVotingThresholds.MotionNoConfidence.Rat,
		}
	}
	if genesis.PoolVotingThresholds.CommitteeNormal != nil {
		p.PoolVotingThresholds.CommitteeNormal = cbor.Rat{
			Rat: genesis.PoolVotingThresholds.CommitteeNormal.Rat,
		}
	}
	if genesis.PoolVotingThresholds.CommitteeNoConfidence != nil {
		p.PoolVotingThresholds.CommitteeNoConfidence = cbor.Rat{
			Rat: genesis.PoolVotingThresholds.CommitteeNoConfidence.Rat,
		}
	}
	if genesis.PoolVotingThresholds.HardForkInitiation != nil {
		p.PoolVotingThresholds.HardForkInitiation = cbor.Rat{
			Rat: genesis.PoolVotingThresholds.HardForkInitiation.Rat,
		}
	}
	if genesis.PoolVotingThresholds.PpSecurityGroup != nil {
		p.PoolVotingThresholds.PpSecurityGroup = cbor.Rat{
			Rat: genesis.PoolVotingThresholds.PpSecurityGroup.Rat,
		}
	}
	if genesis.DRepVotingThresholds.MotionNoConfidence != nil {
		p.DRepVotingThresholds.MotionNoConfidence = cbor.Rat{
			Rat: genesis.DRepVotingThresholds.MotionNoConfidence.Rat,
		}
	}
	if genesis.DRepVotingThresholds.CommitteeNormal != nil {
		p.DRepVotingThresholds.CommitteeNormal = cbor.Rat{
			Rat: genesis.DRepVotingThresholds.CommitteeNormal.Rat,
		}
	}
	if genesis.DRepVotingThresholds.CommitteeNoConfidence != nil {
		p.DRepVotingThresholds.CommitteeNoConfidence = cbor.Rat{
			Rat: genesis.DRepVotingThresholds.CommitteeNoConfidence.Rat,
		}
	}
	if genesis.DRepVotingThresholds.UpdateToConstitution != nil {
		p.DRepVotingThresholds.UpdateToConstitution = cbor.Rat{
			Rat: genesis.DRepVotingThresholds.UpdateToConstitution.Rat,
		}
	}
	if genesis.DRepVotingThresholds.HardForkInitiation != nil {
		p.DRepVotingThresholds.HardForkInitiation = cbor.Rat{
			Rat: genesis.DRepVotingThresholds.HardForkInitiation.Rat,
		}
	}
	if genesis.DRepVotingThresholds.PpNetworkGroup != nil {
		p.DRepVotingThresholds.PpNetworkGroup = cbor.Rat{
			Rat: genesis.DRepVotingThresholds.PpNetworkGroup.Rat,
		}
	}
	if genesis.DRepVotingThresholds.PpEconomicGroup != nil {
		p.DRepVotingThresholds.PpEconomicGroup = cbor.Rat{
			Rat: genesis.DRepVotingThresholds.PpEconomicGroup.Rat,
		}
	}
	if genesis.DRepVotingThresholds.PpTechnicalGroup != nil {
		p.DRepVotingThresholds.PpTechnicalGroup = cbor.Rat{
			Rat: genesis.DRepVotingThresholds.PpTechnicalGroup.Rat,
		}
	}
	if genesis.DRepVotingThresholds.PpGovGroup != nil {
		p.DRepVotingThresholds.PpGovGroup = cbor.Rat{
			Rat: genesis.DRepVotingThresholds.PpGovGroup.Rat,
		}
	}
	if genesis.DRepVotingThresholds.TreasuryWithdrawal != nil {
		p.DRepVotingThresholds.TreasuryWithdrawal = cbor.Rat{
			Rat: genesis.DRepVotingThresholds.TreasuryWithdrawal.Rat,
		}
	}
	return nil
}

type ConwayProtocolParameterUpdate struct {
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
	PoolVotingThresholds       *PoolVotingThresholds                     `cbor:"25,keyasint"`
	DRepVotingThresholds       *DRepVotingThresholds                     `cbor:"26,keyasint"`
	MinCommitteeSize           *uint                                     `cbor:"27,keyasint"`
	CommitteeTermLimit         *uint64                                   `cbor:"28,keyasint"`
	GovActionValidityPeriod    *uint64                                   `cbor:"29,keyasint"`
	GovActionDeposit           *uint64                                   `cbor:"30,keyasint"`
	DRepDeposit                *uint64                                   `cbor:"31,keyasint"`
	DRepInactivityPeriod       *uint64                                   `cbor:"32,keyasint"`
	MinFeeRefScriptCostPerByte *cbor.Rat                                 `cbor:"33,keyasint"`
}

func (u *ConwayProtocolParameterUpdate) UnmarshalCBOR(cborData []byte) error {
	type tConwayProtocolParameterUpdate ConwayProtocolParameterUpdate
	var tmp tConwayProtocolParameterUpdate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*u = ConwayProtocolParameterUpdate(tmp)
	u.SetCbor(cborData)
	return nil
}

func (u ConwayProtocolParameterUpdate) ToPlutusData() data.PlutusData {
	tmpPairs := make([][2]data.PlutusData, 0, 33)
	push := func(idx int, pd data.PlutusData) {
		tmpPairs = append(
			tmpPairs,
			[2]data.PlutusData{
				data.NewInteger(new(big.Int).SetInt64(int64(idx))),
				pd,
			},
		)
	}
	if u.MinFeeA != nil {
		push(0, data.NewInteger(new(big.Int).SetUint64(uint64(*u.MinFeeA))))
	}
	if u.MinFeeB != nil {
		push(1, data.NewInteger(new(big.Int).SetUint64(uint64(*u.MinFeeB))))
	}
	if u.MaxBlockBodySize != nil {
		push(
			2,
			data.NewInteger(
				new(big.Int).SetUint64(uint64(*u.MaxBlockBodySize)),
			),
		)
	}
	if u.MaxTxSize != nil {
		push(3, data.NewInteger(new(big.Int).SetUint64(uint64(*u.MaxTxSize))))
	}
	if u.MaxBlockHeaderSize != nil {
		push(
			4,
			data.NewInteger(
				new(big.Int).SetUint64(uint64(*u.MaxBlockHeaderSize)),
			),
		)
	}
	if u.KeyDeposit != nil {
		push(5, data.NewInteger(new(big.Int).SetUint64(uint64(*u.KeyDeposit))))
	}
	if u.PoolDeposit != nil {
		push(6, data.NewInteger(new(big.Int).SetUint64(uint64(*u.PoolDeposit))))
	}
	if u.MaxEpoch != nil {
		push(7, data.NewInteger(new(big.Int).SetUint64(uint64(*u.MaxEpoch))))
	}
	if u.NOpt != nil {
		push(8, data.NewInteger(new(big.Int).SetUint64(uint64(*u.NOpt))))
	}
	if u.A0 != nil {
		push(9,
			data.NewList(
				data.NewInteger(u.A0.Num()),
				data.NewInteger(u.A0.Denom()),
			),
		)
	}
	if u.Rho != nil {
		push(10,
			data.NewList(
				data.NewInteger(u.Rho.Num()),
				data.NewInteger(u.Rho.Denom()),
			),
		)
	}
	if u.Tau != nil {
		push(11,
			data.NewList(
				data.NewInteger(u.Tau.Num()),
				data.NewInteger(u.Tau.Denom()),
			),
		)
	}
	if u.MinPoolCost != nil {
		push(
			16,
			data.NewInteger(new(big.Int).SetUint64(uint64(*u.MinPoolCost))),
		)
	}
	if u.AdaPerUtxoByte != nil {
		push(
			17,
			data.NewInteger(new(big.Int).SetUint64(uint64(*u.AdaPerUtxoByte))),
		)
	}
	// TODO: CostModels
	if u.ExecutionCosts != nil {
		push(19,
			data.NewList(
				data.NewList(
					data.NewInteger(u.ExecutionCosts.MemPrice.Num()),
					data.NewInteger(u.ExecutionCosts.MemPrice.Denom()),
				),
				data.NewList(
					data.NewInteger(u.ExecutionCosts.StepPrice.Num()),
					data.NewInteger(u.ExecutionCosts.StepPrice.Denom()),
				),
			),
		)
	}
	if u.MaxTxExUnits != nil {
		push(20,
			data.NewList(
				data.NewInteger(big.NewInt(u.MaxTxExUnits.Memory)),
				data.NewInteger(big.NewInt(u.MaxTxExUnits.Steps)),
			),
		)
	}
	if u.MaxBlockExUnits != nil {
		push(21,
			data.NewList(
				data.NewInteger(big.NewInt(u.MaxBlockExUnits.Memory)),
				data.NewInteger(big.NewInt(u.MaxBlockExUnits.Steps)),
			),
		)
	}
	if u.MaxValueSize != nil {
		push(
			22,
			data.NewInteger(new(big.Int).SetUint64(uint64(*u.MaxValueSize))),
		)
	}
	if u.CollateralPercentage != nil {
		push(
			23,
			data.NewInteger(
				new(big.Int).SetUint64(uint64(*u.CollateralPercentage)),
			),
		)
	}
	if u.MaxCollateralInputs != nil {
		push(
			24,
			data.NewInteger(
				new(big.Int).SetUint64(uint64(*u.MaxCollateralInputs)),
			),
		)
	}
	if u.PoolVotingThresholds != nil {
		push(25, u.PoolVotingThresholds.ToPlutusData())
	}
	if u.DRepVotingThresholds != nil {
		push(26, u.DRepVotingThresholds.ToPlutusData())
	}
	if u.MinCommitteeSize != nil {
		push(
			27,
			data.NewInteger(
				new(big.Int).SetUint64(uint64(*u.MinCommitteeSize)),
			),
		)
	}
	if u.CommitteeTermLimit != nil {
		push(28, data.NewInteger(new(big.Int).SetUint64(*u.CommitteeTermLimit)))
	}
	if u.GovActionValidityPeriod != nil {
		push(
			29,
			data.NewInteger(new(big.Int).SetUint64(*u.GovActionValidityPeriod)),
		)
	}
	if u.GovActionDeposit != nil {
		push(30, data.NewInteger(new(big.Int).SetUint64(*u.GovActionDeposit)))
	}
	if u.DRepDeposit != nil {
		push(31, data.NewInteger(new(big.Int).SetUint64(*u.DRepDeposit)))
	}
	if u.DRepInactivityPeriod != nil {
		push(
			32,
			data.NewInteger(new(big.Int).SetUint64(*u.DRepInactivityPeriod)),
		)
	}
	if u.MinFeeRefScriptCostPerByte != nil {
		push(33,
			data.NewList(
				data.NewInteger(u.MinFeeRefScriptCostPerByte.Num()),
				data.NewInteger(u.MinFeeRefScriptCostPerByte.Denom()),
			),
		)
	}
	return data.NewMap(tmpPairs)
}

type PoolVotingThresholds struct {
	cbor.StructAsArray
	MotionNoConfidence    cbor.Rat
	CommitteeNormal       cbor.Rat
	CommitteeNoConfidence cbor.Rat
	HardForkInitiation    cbor.Rat
	PpSecurityGroup       cbor.Rat
}

func (t PoolVotingThresholds) ToPlutusData() data.PlutusData {
	return data.NewList(
		data.NewList(
			data.NewInteger(t.MotionNoConfidence.Num()),
			data.NewInteger(t.MotionNoConfidence.Denom()),
		),
		data.NewList(
			data.NewInteger(t.CommitteeNormal.Num()),
			data.NewInteger(t.CommitteeNormal.Denom()),
		),
		data.NewList(
			data.NewInteger(t.CommitteeNoConfidence.Num()),
			data.NewInteger(t.CommitteeNoConfidence.Denom()),
		),
		data.NewList(
			data.NewInteger(t.HardForkInitiation.Num()),
			data.NewInteger(t.HardForkInitiation.Denom()),
		),
		data.NewList(
			data.NewInteger(t.PpSecurityGroup.Num()),
			data.NewInteger(t.PpSecurityGroup.Denom()),
		),
	)
}

type DRepVotingThresholds struct {
	cbor.StructAsArray
	MotionNoConfidence    cbor.Rat
	CommitteeNormal       cbor.Rat
	CommitteeNoConfidence cbor.Rat
	UpdateToConstitution  cbor.Rat
	HardForkInitiation    cbor.Rat
	PpNetworkGroup        cbor.Rat
	PpEconomicGroup       cbor.Rat
	PpTechnicalGroup      cbor.Rat
	PpGovGroup            cbor.Rat
	TreasuryWithdrawal    cbor.Rat
}

func (t DRepVotingThresholds) ToPlutusData() data.PlutusData {
	return data.NewList(
		data.NewList(
			data.NewInteger(t.MotionNoConfidence.Num()),
			data.NewInteger(t.MotionNoConfidence.Denom()),
		),
		data.NewList(
			data.NewInteger(t.CommitteeNormal.Num()),
			data.NewInteger(t.CommitteeNormal.Denom()),
		),
		data.NewList(
			data.NewInteger(t.CommitteeNoConfidence.Num()),
			data.NewInteger(t.CommitteeNoConfidence.Denom()),
		),
		data.NewList(
			data.NewInteger(t.UpdateToConstitution.Num()),
			data.NewInteger(t.UpdateToConstitution.Denom()),
		),
		data.NewList(
			data.NewInteger(t.HardForkInitiation.Num()),
			data.NewInteger(t.HardForkInitiation.Denom()),
		),
		data.NewList(
			data.NewInteger(t.PpNetworkGroup.Num()),
			data.NewInteger(t.PpNetworkGroup.Denom()),
		),
		data.NewList(
			data.NewInteger(t.PpEconomicGroup.Num()),
			data.NewInteger(t.PpEconomicGroup.Denom()),
		),
		data.NewList(
			data.NewInteger(t.PpTechnicalGroup.Num()),
			data.NewInteger(t.PpTechnicalGroup.Denom()),
		),
		data.NewList(
			data.NewInteger(t.PpGovGroup.Num()),
			data.NewInteger(t.PpGovGroup.Denom()),
		),
		data.NewList(
			data.NewInteger(t.TreasuryWithdrawal.Num()),
			data.NewInteger(t.TreasuryWithdrawal.Denom()),
		),
	)
}

func UpgradePParams(
	prevPParams babbage.BabbageProtocolParameters,
) ConwayProtocolParameters {
	ret := ConwayProtocolParameters{
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
		ProtocolVersion: common.ProtocolParametersProtocolVersion{
			Major: prevPParams.ProtocolMajor,
			Minor: prevPParams.ProtocolMinor,
		},
		MinPoolCost:          prevPParams.MinPoolCost,
		AdaPerUtxoByte:       prevPParams.AdaPerUtxoByte,
		CostModels:           prevPParams.CostModels,
		ExecutionCosts:       prevPParams.ExecutionCosts,
		MaxTxExUnits:         prevPParams.MaxTxExUnits,
		MaxBlockExUnits:      prevPParams.MaxBlockExUnits,
		MaxValueSize:         prevPParams.MaxValueSize,
		CollateralPercentage: prevPParams.CollateralPercentage,
		MaxCollateralInputs:  prevPParams.MaxCollateralInputs,
	}
	return ret
}
