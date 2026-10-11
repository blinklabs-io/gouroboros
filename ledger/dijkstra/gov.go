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
	"fmt"
	"math"
	"math/big"
	"reflect"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/data"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

type DijkstraProposalProcedure struct {
	common.ProposalProcedureBase
	cbor.StructAsArray
	PPDeposit       uint64
	PPRewardAccount common.Address
	PPGovAction     DijkstraGovAction
	PPAnchor        common.GovAnchor
}

func (p *DijkstraProposalProcedure) UnmarshalCBOR(cborData []byte) error {
	if err := common.ValidateCBORArrayLength(
		cborData,
		4,
		"Dijkstra proposal procedure",
	); err != nil {
		return err
	}
	type tDijkstraProposalProcedure DijkstraProposalProcedure
	var tmp tDijkstraProposalProcedure
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	if err := common.CheckAccountAddress(tmp.PPRewardAccount); err != nil {
		return err
	}
	*p = DijkstraProposalProcedure(tmp)
	return nil
}

func (p DijkstraProposalProcedure) ToPlutusData() data.PlutusData {
	return data.NewConstr(0,
		data.NewInteger(new(big.Int).SetUint64(p.PPDeposit)),
		p.PPRewardAccount.ToPlutusData(),
		p.PPGovAction.ToPlutusData(),
	)
}

func (p DijkstraProposalProcedure) Deposit() uint64 {
	return p.PPDeposit
}

func (p DijkstraProposalProcedure) RewardAccount() common.Address {
	return p.PPRewardAccount
}

func (p DijkstraProposalProcedure) GovAction() common.GovAction {
	return p.PPGovAction.Action
}

func (p DijkstraProposalProcedure) Anchor() common.GovAnchor {
	return p.PPAnchor
}

type DijkstraGovAction struct {
	Type   uint
	Action common.GovAction
}

// isNilGovAction reports whether action is nil, including an interface that
// contains a typed-nil pointer.
func isNilGovAction(action common.GovAction) bool {
	if action == nil {
		return true
	}
	rv := reflect.ValueOf(action)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}

func (g DijkstraGovAction) ToPlutusData() data.PlutusData {
	return g.Action.ToPlutusData()
}

func (g *DijkstraGovAction) UnmarshalCBOR(cborData []byte) error {
	actionType, err := cbor.DecodeIdFromList(cborData)
	if err != nil {
		return err
	}
	if actionType < 0 {
		return fmt.Errorf("invalid governance action type: %d", actionType)
	}
	arrayLength := 0
	var tmpAction common.GovAction
	switch common.GovActionType(actionType) {
	case common.GovActionTypeParameterChange:
		arrayLength = 4
		tmpAction = &DijkstraParameterChangeGovAction{}
	case common.GovActionTypeHardForkInitiation:
		arrayLength = 3
		tmpAction = &common.HardForkInitiationGovAction{}
	case common.GovActionTypeTreasuryWithdrawal:
		arrayLength = 3
		tmpAction = &common.TreasuryWithdrawalGovAction{}
	case common.GovActionTypeNoConfidence:
		arrayLength = 2
		tmpAction = &common.NoConfidenceGovAction{}
	case common.GovActionTypeUpdateCommittee:
		arrayLength = 5
		tmpAction = &common.UpdateCommitteeGovAction{}
	case common.GovActionTypeNewConstitution:
		arrayLength = 3
		tmpAction = &common.NewConstitutionGovAction{}
	case common.GovActionTypeInfo:
		arrayLength = 1
		tmpAction = &common.InfoGovAction{}
	default:
		return fmt.Errorf("unknown governance action type: %d", actionType)
	}
	if err := common.ValidateCBORArrayLength(
		cborData,
		arrayLength,
		"Dijkstra governance action",
	); err != nil {
		return err
	}
	if _, err := cbor.Decode(cborData, tmpAction); err != nil {
		return err
	}
	if action, ok := tmpAction.(*common.HardForkInitiationGovAction); ok {
		if action.ProtocolVersion.Major > common.ProtocolVersionDijkstra+1 {
			return fmt.Errorf(
				"hard-fork protocol major version %d exceeds Dijkstra decoder limit %d",
				action.ProtocolVersion.Major,
				common.ProtocolVersionDijkstra+1,
			)
		}
		if action.ProtocolVersion.Minor > math.MaxUint32 {
			return fmt.Errorf(
				"hard-fork protocol minor version %d exceeds Word32",
				action.ProtocolVersion.Minor,
			)
		}
	}
	g.Type = uint(actionType) // #nosec G115
	g.Action = tmpAction
	return nil
}

func (g *DijkstraGovAction) MarshalCBOR() ([]byte, error) {
	return cbor.Encode(g.Action)
}

type DijkstraParameterChangeGovAction struct {
	common.GovActionBase
	cbor.StructAsArray
	Type        uint
	ActionId    *common.GovActionId
	ParamUpdate DijkstraProtocolParameterUpdate
	PolicyHash  []byte
}

var _ common.ParameterChangeGovAction = (*DijkstraParameterChangeGovAction)(nil)

func (a *DijkstraParameterChangeGovAction) ToPlutusData() data.PlutusData {
	actionId := data.NewConstr(1)
	if a.ActionId != nil {
		actionId = data.NewConstr(0, a.ActionId.ToPlutusData())
	}
	policyHash := data.NewConstr(1)
	if a.PolicyHash != nil {
		policyHash = data.NewConstr(
			0,
			data.NewByteString(a.PolicyHash),
		)
	}
	return data.NewConstr(0,
		actionId,
		a.ParamUpdate.ToPlutusData(),
		policyHash,
	)
}

func (a *DijkstraParameterChangeGovAction) GetPolicyHash() []byte {
	if a == nil {
		return nil
	}
	return a.PolicyHash
}

// PreviousGovActionId returns the parameter-change action this action follows.
func (a *DijkstraParameterChangeGovAction) PreviousGovActionId() *common.GovActionId {
	if a == nil {
		return nil
	}
	return a.ActionId
}

// ProtocolParamUpdateUtxorpc converts the proposed parameter update. The
// parameters Dijkstra adds have no UTxO-RPC field and are not carried.
func (a *DijkstraParameterChangeGovAction) ProtocolParamUpdateUtxorpc() (*utxorpc.PParams, error) {
	if a == nil {
		return nil, nil
	}
	return a.ParamUpdate.conwayUpdate().Utxorpc()
}

// SecurityGroupFields returns the security-group parameters changed by this
// action.
func (a *DijkstraParameterChangeGovAction) SecurityGroupFields() []string {
	if a == nil {
		return nil
	}
	fields := a.ParamUpdate.conwayUpdate().SecurityGroupFields()
	if a.ParamUpdate.MaxRefScriptSizePerBlock != nil {
		fields = append(fields, "MaxRefScriptSizePerBlock")
	}
	if a.ParamUpdate.MaxRefScriptSizePerTx != nil {
		fields = append(fields, "MaxRefScriptSizePerTx")
	}
	if a.ParamUpdate.RefScriptCostStride != nil {
		fields = append(fields, "RefScriptCostStride")
	}
	if a.ParamUpdate.RefScriptCostMultiplier != nil {
		fields = append(fields, "RefScriptCostMultiplier")
	}
	if a.ParamUpdate.LeiosAnnouncementPeriodLength != nil {
		fields = append(fields, "LeiosAnnouncementPeriodLength")
	}
	if a.ParamUpdate.LeiosVotePeriodLength != nil {
		fields = append(fields, "LeiosVotePeriodLength")
	}
	if a.ParamUpdate.LeiosDiffusionPeriodLength != nil {
		fields = append(fields, "LeiosDiffusionPeriodLength")
	}
	if a.ParamUpdate.LeiosCommitteeSize != nil {
		fields = append(fields, "LeiosCommitteeSize")
	}
	if a.ParamUpdate.LeiosQuorumStakeThreshold != nil {
		fields = append(fields, "LeiosQuorumStakeThreshold")
	}
	if a.ParamUpdate.MaxEndorserBlockReferencesSize != nil {
		fields = append(fields, "MaxEndorserBlockReferencesSize")
	}
	if a.ParamUpdate.MaxEndorserBlockTxsSize != nil {
		fields = append(fields, "MaxEndorserBlockTxsSize")
	}
	if a.ParamUpdate.MaxEndorserBlockExUnits != nil {
		fields = append(fields, "MaxEndorserBlockExUnits")
	}
	if a.ParamUpdate.MaxRefScriptSizePerEndorserBlock != nil {
		fields = append(fields, "MaxRefScriptSizePerEndorserBlock")
	}
	if a.ParamUpdate.PerasMinCandidateBlockAge != nil {
		fields = append(fields, "PerasMinCandidateBlockAge")
	}
	if a.ParamUpdate.PerasHealingFactor != nil {
		fields = append(fields, "PerasHealingFactor")
	}
	if a.ParamUpdate.PerasCertBoost != nil {
		fields = append(fields, "PerasCertBoost")
	}
	if a.ParamUpdate.PerasTargetCommitteeSize != nil {
		fields = append(fields, "PerasTargetCommitteeSize")
	}
	if a.ParamUpdate.PerasBootstrapRoundSet || a.ParamUpdate.PerasBootstrapRound != nil {
		fields = append(fields, "PerasBootstrapRound")
	}
	if a.ParamUpdate.PerasQuorumThresholdSafetyMargin != nil {
		fields = append(fields, "PerasQuorumThresholdSafetyMargin")
	}
	if a.ParamUpdate.RefInputsCostPerMultiAssetPolicy != nil {
		fields = append(fields, "RefInputsCostPerMultiAssetPolicy")
	}
	if a.ParamUpdate.RefInputsCostPerDatumByte != nil {
		fields = append(fields, "RefInputsCostPerDatumByte")
	}
	return fields
}
