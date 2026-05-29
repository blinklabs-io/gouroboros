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
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/data"
)

type DijkstraProposalProcedure struct {
	common.ProposalProcedureBase
	cbor.StructAsArray
	PPDeposit       uint64
	PPRewardAccount common.Address
	PPGovAction     DijkstraGovAction
	PPAnchor        common.GovAnchor
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
	var tmpAction common.GovAction
	switch common.GovActionType(actionType) {
	case common.GovActionTypeParameterChange:
		tmpAction = &DijkstraParameterChangeGovAction{}
	case common.GovActionTypeHardForkInitiation:
		tmpAction = &common.HardForkInitiationGovAction{}
	case common.GovActionTypeTreasuryWithdrawal:
		tmpAction = &common.TreasuryWithdrawalGovAction{}
	case common.GovActionTypeNoConfidence:
		tmpAction = &common.NoConfidenceGovAction{}
	case common.GovActionTypeUpdateCommittee:
		tmpAction = &common.UpdateCommitteeGovAction{}
	case common.GovActionTypeNewConstitution:
		tmpAction = &common.NewConstitutionGovAction{}
	case common.GovActionTypeInfo:
		tmpAction = &common.InfoGovAction{}
	default:
		return fmt.Errorf("unknown governance action type: %d", actionType)
	}
	if _, err := cbor.Decode(cborData, tmpAction); err != nil {
		return err
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

func (a *DijkstraParameterChangeGovAction) ToPlutusData() data.PlutusData {
	actionId := data.NewConstr(1)
	if a.ActionId != nil {
		actionId = data.NewConstr(0, a.ActionId.ToPlutusData())
	}
	policyHash := data.NewConstr(1)
	if len(a.PolicyHash) > 0 {
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
	return a.PolicyHash
}
