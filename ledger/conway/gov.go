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
	"fmt"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/data"
)

type ConwayProposalProcedure struct {
	common.ProposalProcedureBase
	cbor.StructAsArray
	PPDeposit       uint64
	PPRewardAccount common.Address
	PPGovAction     ConwayGovAction
	PPAnchor        common.GovAnchor
}

func (p ConwayProposalProcedure) ToPlutusData() data.PlutusData {
	return data.NewConstr(0,
		data.NewInteger(new(big.Int).SetUint64(p.PPDeposit)),
		p.PPRewardAccount.ToPlutusData(),
		p.PPGovAction.ToPlutusData(),
	)
}

func (p ConwayProposalProcedure) Deposit() uint64 {
	return p.PPDeposit
}

func (p ConwayProposalProcedure) RewardAccount() common.Address {
	return p.PPRewardAccount
}

func (p ConwayProposalProcedure) GovAction() common.GovAction {
	return p.PPGovAction.Action
}

func (p ConwayProposalProcedure) Anchor() common.GovAnchor {
	return p.PPAnchor
}

type ConwayGovAction struct {
	Type   uint
	Action common.GovAction
}

func (g ConwayGovAction) ToPlutusData() data.PlutusData {
	return g.Action.ToPlutusData()
}

func (g *ConwayGovAction) UnmarshalCBOR(data []byte) error {
	// Determine action type
	actionType, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	var tmpAction common.GovAction
	switch actionType {
	case common.GovActionTypeParameterChange:
		tmpAction = &ConwayParameterChangeGovAction{}
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
	// Decode action
	if _, err := cbor.Decode(data, tmpAction); err != nil {
		return err
	}
	// action type is known within uint range
	g.Type = uint(actionType) // #nosec G115
	g.Action = tmpAction
	return nil
}

func (g *ConwayGovAction) MarshalCBOR() ([]byte, error) {
	return cbor.Encode(g.Action)
}

type ConwayParameterChangeGovAction struct {
	common.GovActionBase
	cbor.StructAsArray
	Type        uint
	ActionId    *common.GovActionId
	ParamUpdate ConwayProtocolParameterUpdate
	PolicyHash  []byte
}

func (a *ConwayParameterChangeGovAction) ToPlutusData() data.PlutusData {
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

// GetPolicyHash returns the policy script hash for this governance action
func (a *ConwayParameterChangeGovAction) GetPolicyHash() []byte {
	return a.PolicyHash
}
