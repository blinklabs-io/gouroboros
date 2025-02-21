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

package common

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// VotingProcedures is a convenience type to avoid needing to duplicate the full type definition everywhere
type VotingProcedures map[*Voter]map[*GovActionId]VotingProcedure

const (
	VoterTypeConstitutionalCommitteeHotKeyHash    uint8 = 0
	VoterTypeConstitutionalCommitteeHotScriptHash uint8 = 1
	VoterTypeDRepKeyHash                          uint8 = 2
	VoterTypeDRepScriptHash                       uint8 = 3
	VoterTypeStakingPoolKeyHash                   uint8 = 4
)

type Voter struct {
	cbor.StructAsArray
	Type uint8
	Hash [28]byte
}

const (
	GovVoteNo      uint8 = 0
	GovVoteYes     uint8 = 1
	GovVoteAbstain uint8 = 2
)

type VotingProcedure struct {
	cbor.StructAsArray
	Vote   uint8
	Anchor *GovAnchor
}

type GovAnchor struct {
	cbor.StructAsArray
	Url      string
	DataHash [32]byte
}

type GovActionId struct {
	cbor.StructAsArray
	TransactionId [32]byte
	GovActionIdx  uint32
}

type ProposalProcedure struct {
	cbor.StructAsArray
	Deposit       uint64
	RewardAccount Address
	GovAction     GovActionWrapper
	Anchor        GovAnchor
}

const (
	GovActionTypeParameterChange    = 0
	GovActionTypeHardForkInitiation = 1
	GovActionTypeTreasuryWithdrawal = 2
	GovActionTypeNoConfidence       = 3
	GovActionTypeUpdateCommittee    = 4
	GovActionTypeNewConstitution    = 5
	GovActionTypeInfo               = 6
)

type GovActionWrapper struct {
	Type   uint
	Action GovAction
}

func (g *GovActionWrapper) UnmarshalCBOR(data []byte) error {
	// Determine action type
	actionType, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	var tmpAction GovAction
	switch actionType {
	case GovActionTypeParameterChange:
		tmpAction = &ParameterChangeGovAction{}
	case GovActionTypeHardForkInitiation:
		tmpAction = &HardForkInitiationGovAction{}
	case GovActionTypeTreasuryWithdrawal:
		tmpAction = &TreasuryWithdrawalGovAction{}
	case GovActionTypeNoConfidence:
		tmpAction = &NoConfidenceGovAction{}
	case GovActionTypeUpdateCommittee:
		tmpAction = &UpdateCommitteeGovAction{}
	case GovActionTypeNewConstitution:
		tmpAction = &NewConstitutionGovAction{}
	case GovActionTypeInfo:
		tmpAction = &InfoGovAction{}
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

func (g *GovActionWrapper) MarshalCBOR() ([]byte, error) {
	return cbor.Encode(g.Action)
}

type GovAction interface {
	isGovAction()
}

type ParameterChangeGovAction struct {
	cbor.StructAsArray
	Type        uint
	ActionId    *GovActionId
	ParamUpdate cbor.RawMessage // NOTE: we use raw to defer processing to account for per-era types
	PolicyHash  []byte
}

func (a ParameterChangeGovAction) isGovAction() {}

type HardForkInitiationGovAction struct {
	cbor.StructAsArray
	Type            uint
	ActionId        *GovActionId
	ProtocolVersion struct {
		cbor.StructAsArray
		Major uint
		Minor uint
	}
}

func (a HardForkInitiationGovAction) isGovAction() {}

type TreasuryWithdrawalGovAction struct {
	cbor.StructAsArray
	Type        uint
	Withdrawals map[*Address]uint64
	PolicyHash  []byte
}

func (a TreasuryWithdrawalGovAction) isGovAction() {}

type NoConfidenceGovAction struct {
	cbor.StructAsArray
	Type     uint
	ActionId *GovActionId
}

func (a NoConfidenceGovAction) isGovAction() {}

type UpdateCommitteeGovAction struct {
	cbor.StructAsArray
	Type        uint
	ActionId    *GovActionId
	Credentials []StakeCredential
	CredEpochs  map[*StakeCredential]uint
	Unknown     cbor.Rat
}

func (a UpdateCommitteeGovAction) isGovAction() {}

type NewConstitutionGovAction struct {
	cbor.StructAsArray
	Type         uint
	ActionId     *GovActionId
	Constitution struct {
		cbor.StructAsArray
		Anchor     GovAnchor
		ScriptHash []byte
	}
}

func (a NewConstitutionGovAction) isGovAction() {}

type InfoGovAction struct {
	cbor.StructAsArray
	Type uint
}

func (a InfoGovAction) isGovAction() {}
