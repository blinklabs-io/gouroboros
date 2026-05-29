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
	"github.com/blinklabs-io/plutigo/data"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

type DijkstraProtocolParameters struct {
	conway.ConwayProtocolParameters
	// TODO: Wire reference-script size limits into Dijkstra validation rules.
	MaxRefScriptSizePerBlock uint32
	MaxRefScriptSizePerTx    uint32
	RefScriptCostStride      uint32
	RefScriptCostMultiplier  *cbor.Rat
	CommitteeStakeCoverage   *cbor.Rat
	QuorumStakeThreshold     *cbor.Rat
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
	_ = p.ApplyUpdate(paramUpdate)
}

func (p *DijkstraProtocolParameters) updateUnchecked(
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
	if paramUpdate.CommitteeStakeCoverage != nil {
		p.CommitteeStakeCoverage = paramUpdate.CommitteeStakeCoverage
	}
	if paramUpdate.QuorumStakeThreshold != nil {
		p.QuorumStakeThreshold = paramUpdate.QuorumStakeThreshold
	}
}

func (p *DijkstraProtocolParameters) ApplyUpdate(
	paramUpdate *DijkstraProtocolParameterUpdate,
) error {
	if paramUpdate == nil {
		return nil
	}
	committeeStakeCoverage := p.CommitteeStakeCoverage
	if paramUpdate.CommitteeStakeCoverage != nil {
		committeeStakeCoverage = paramUpdate.CommitteeStakeCoverage
	}
	quorumStakeThreshold := p.QuorumStakeThreshold
	if paramUpdate.QuorumStakeThreshold != nil {
		quorumStakeThreshold = paramUpdate.QuorumStakeThreshold
	}
	if err := validateLeiosCommitteeStakeParameters(
		committeeStakeCoverage,
		quorumStakeThreshold,
	); err != nil {
		return err
	}
	p.updateUnchecked(paramUpdate)
	return nil
}

func (p *DijkstraProtocolParameters) ValidateLeiosCommitteeParameters() error {
	return validateLeiosCommitteeStakeParameters(
		p.CommitteeStakeCoverage,
		p.QuorumStakeThreshold,
	)
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
	// The current Dijkstra CDDL assigns protocol_param_update keys through 37.
	// These Leios stake parameters have no confirmed on-chain update keys yet,
	// so they are intentionally excluded from CBOR and can only be applied from
	// local genesis/configuration data for now.
	CommitteeStakeCoverage *cbor.Rat `cbor:"-"`
	QuorumStakeThreshold   *cbor.Rat `cbor:"-"`
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

func (u DijkstraProtocolParameterUpdate) MarshalCBOR() ([]byte, error) {
	if raw := u.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	fields := map[uint]any{}
	if u.MinFeeA != nil {
		fields[0] = *u.MinFeeA
	}
	if u.MinFeeB != nil {
		fields[1] = *u.MinFeeB
	}
	if u.MaxBlockBodySize != nil {
		fields[2] = *u.MaxBlockBodySize
	}
	if u.MaxTxSize != nil {
		fields[3] = *u.MaxTxSize
	}
	if u.MaxBlockHeaderSize != nil {
		fields[4] = *u.MaxBlockHeaderSize
	}
	if u.KeyDeposit != nil {
		fields[5] = *u.KeyDeposit
	}
	if u.PoolDeposit != nil {
		fields[6] = *u.PoolDeposit
	}
	if u.MaxEpoch != nil {
		fields[7] = *u.MaxEpoch
	}
	if u.NOpt != nil {
		fields[8] = *u.NOpt
	}
	if u.A0 != nil {
		fields[9] = u.A0
	}
	if u.Rho != nil {
		fields[10] = u.Rho
	}
	if u.Tau != nil {
		fields[11] = u.Tau
	}
	if u.ProtocolVersion != nil {
		fields[14] = *u.ProtocolVersion
	}
	if u.MinPoolCost != nil {
		fields[16] = *u.MinPoolCost
	}
	if u.AdaPerUtxoByte != nil {
		fields[17] = *u.AdaPerUtxoByte
	}
	if u.CostModels != nil {
		fields[18] = u.CostModels
	}
	if u.ExecutionCosts != nil {
		fields[19] = *u.ExecutionCosts
	}
	if u.MaxTxExUnits != nil {
		fields[20] = *u.MaxTxExUnits
	}
	if u.MaxBlockExUnits != nil {
		fields[21] = *u.MaxBlockExUnits
	}
	if u.MaxValueSize != nil {
		fields[22] = *u.MaxValueSize
	}
	if u.CollateralPercentage != nil {
		fields[23] = *u.CollateralPercentage
	}
	if u.MaxCollateralInputs != nil {
		fields[24] = *u.MaxCollateralInputs
	}
	if u.PoolVotingThresholds != nil {
		fields[25] = *u.PoolVotingThresholds
	}
	if u.DRepVotingThresholds != nil {
		fields[26] = *u.DRepVotingThresholds
	}
	if u.MinCommitteeSize != nil {
		fields[27] = *u.MinCommitteeSize
	}
	if u.CommitteeTermLimit != nil {
		fields[28] = *u.CommitteeTermLimit
	}
	if u.GovActionValidityPeriod != nil {
		fields[29] = *u.GovActionValidityPeriod
	}
	if u.GovActionDeposit != nil {
		fields[30] = *u.GovActionDeposit
	}
	if u.DRepDeposit != nil {
		fields[31] = *u.DRepDeposit
	}
	if u.DRepInactivityPeriod != nil {
		fields[32] = *u.DRepInactivityPeriod
	}
	if u.MinFeeRefScriptCostPerByte != nil {
		fields[33] = u.MinFeeRefScriptCostPerByte
	}
	if u.MaxRefScriptSizePerBlock != nil {
		fields[34] = *u.MaxRefScriptSizePerBlock
	}
	if u.MaxRefScriptSizePerTx != nil {
		fields[35] = *u.MaxRefScriptSizePerTx
	}
	if u.RefScriptCostStride != nil {
		fields[36] = *u.RefScriptCostStride
	}
	if u.RefScriptCostMultiplier != nil {
		fields[37] = u.RefScriptCostMultiplier
	}
	return cbor.Encode(fields)
}

func (u *DijkstraProtocolParameterUpdate) hasUpdate() bool {
	return u.MinFeeA != nil ||
		u.MinFeeB != nil ||
		u.MaxBlockBodySize != nil ||
		u.MaxTxSize != nil ||
		u.MaxBlockHeaderSize != nil ||
		u.KeyDeposit != nil ||
		u.PoolDeposit != nil ||
		u.MaxEpoch != nil ||
		u.NOpt != nil ||
		u.A0 != nil ||
		u.Rho != nil ||
		u.Tau != nil ||
		u.ProtocolVersion != nil ||
		u.MinPoolCost != nil ||
		u.AdaPerUtxoByte != nil ||
		len(u.CostModels) > 0 ||
		u.ExecutionCosts != nil ||
		u.MaxTxExUnits != nil ||
		u.MaxBlockExUnits != nil ||
		u.MaxValueSize != nil ||
		u.CollateralPercentage != nil ||
		u.MaxCollateralInputs != nil ||
		u.PoolVotingThresholds != nil ||
		u.DRepVotingThresholds != nil ||
		u.MinCommitteeSize != nil ||
		u.CommitteeTermLimit != nil ||
		u.GovActionValidityPeriod != nil ||
		u.GovActionDeposit != nil ||
		u.DRepDeposit != nil ||
		u.DRepInactivityPeriod != nil ||
		u.MinFeeRefScriptCostPerByte != nil ||
		u.MaxRefScriptSizePerBlock != nil ||
		u.MaxRefScriptSizePerTx != nil ||
		u.RefScriptCostStride != nil ||
		u.RefScriptCostMultiplier != nil ||
		u.CommitteeStakeCoverage != nil ||
		u.QuorumStakeThreshold != nil
}

func (u *DijkstraProtocolParameterUpdate) BootstrapRestrictedFields() []string {
	fields := u.conwayUpdate().BootstrapRestrictedFields()
	if u.MaxRefScriptSizePerBlock != nil {
		fields = append(fields, "MaxRefScriptSizePerBlock")
	}
	if u.MaxRefScriptSizePerTx != nil {
		fields = append(fields, "MaxRefScriptSizePerTx")
	}
	if u.RefScriptCostStride != nil {
		fields = append(fields, "RefScriptCostStride")
	}
	if u.RefScriptCostMultiplier != nil {
		fields = append(fields, "RefScriptCostMultiplier")
	}
	if u.CommitteeStakeCoverage != nil {
		fields = append(fields, "CommitteeStakeCoverage")
	}
	if u.QuorumStakeThreshold != nil {
		fields = append(fields, "QuorumStakeThreshold")
	}
	return fields
}

func (u DijkstraProtocolParameterUpdate) ToPlutusData() data.PlutusData {
	tmpPairs := make([][2]data.PlutusData, 0, 37)
	push := func(idx int, pd data.PlutusData) {
		tmpPairs = append(
			tmpPairs,
			[2]data.PlutusData{
				data.NewInteger(new(big.Int).SetInt64(int64(idx))),
				pd,
			},
		)
	}
	pushRat := func(idx int, r *cbor.Rat) {
		push(idx,
			data.NewList(
				data.NewInteger(r.Num()),
				data.NewInteger(r.Denom()),
			),
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
		pushRat(9, u.A0)
	}
	if u.Rho != nil {
		pushRat(10, u.Rho)
	}
	if u.Tau != nil {
		pushRat(11, u.Tau)
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
	// TODO(enhancement): Add CostModels serialization for Plutus data conversion.
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
		pushRat(33, u.MinFeeRefScriptCostPerByte)
	}
	if u.MaxRefScriptSizePerBlock != nil {
		push(
			34,
			data.NewInteger(new(big.Int).SetUint64(
				uint64(*u.MaxRefScriptSizePerBlock),
			)),
		)
	}
	if u.MaxRefScriptSizePerTx != nil {
		push(
			35,
			data.NewInteger(new(big.Int).SetUint64(
				uint64(*u.MaxRefScriptSizePerTx),
			)),
		)
	}
	if u.RefScriptCostStride != nil {
		push(
			36,
			data.NewInteger(new(big.Int).SetUint64(
				uint64(*u.RefScriptCostStride),
			)),
		)
	}
	if u.RefScriptCostMultiplier != nil {
		pushRat(37, u.RefScriptCostMultiplier)
	}
	return data.NewMap(tmpPairs)
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
