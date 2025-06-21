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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
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
			Memory: p.MaxTxExUnits.Memory,
			Steps:  p.MaxTxExUnits.Steps,
		},
		MaxExecutionUnitsPerBlock: &cardano.ExUnits{
			Memory: p.MaxBlockExUnits.Memory,
			Steps:  p.MaxBlockExUnits.Steps,
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
	babbage.BabbageProtocolParameterUpdate
	PoolVotingThresholds       *PoolVotingThresholds `cbor:"25,keyasint"`
	DRepVotingThresholds       *DRepVotingThresholds `cbor:"26,keyasint"`
	MinCommitteeSize           *uint                 `cbor:"27,keyasint"`
	CommitteeTermLimit         *uint64               `cbor:"28,keyasint"`
	GovActionValidityPeriod    *uint64               `cbor:"29,keyasint"`
	GovActionDeposit           *uint64               `cbor:"30,keyasint"`
	DRepDeposit                *uint64               `cbor:"31,keyasint"`
	DRepInactivityPeriod       *uint64               `cbor:"32,keyasint"`
	MinFeeRefScriptCostPerByte *cbor.Rat             `cbor:"33,keyasint"`
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

type PoolVotingThresholds struct {
	cbor.StructAsArray
	MotionNoConfidence    cbor.Rat
	CommitteeNormal       cbor.Rat
	CommitteeNoConfidence cbor.Rat
	HardForkInitiation    cbor.Rat
	PpSecurityGroup       cbor.Rat
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
