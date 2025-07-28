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
	"math/big"
	"reflect"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/pkg/data"
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

func (v Voter) ToPlutusData() data.PlutusData {
	switch v.Type {
	case VoterTypeConstitutionalCommitteeHotScriptHash:
		cred := &Credential{
			CredType:   CredentialTypeScriptHash,
			Credential: NewBlake2b224(v.Hash[:]),
		}
		return data.NewConstr(0, cred.ToPlutusData())
	case VoterTypeConstitutionalCommitteeHotKeyHash:
		cred := &Credential{
			CredType:   CredentialTypeAddrKeyHash,
			Credential: NewBlake2b224(v.Hash[:]),
		}
		return data.NewConstr(0, cred.ToPlutusData())
	case VoterTypeDRepScriptHash:
		cred := &Credential{
			CredType:   CredentialTypeScriptHash,
			Credential: NewBlake2b224(v.Hash[:]),
		}
		return data.NewConstr(1, cred.ToPlutusData())
	case VoterTypeDRepKeyHash:
		cred := &Credential{
			CredType:   CredentialTypeAddrKeyHash,
			Credential: NewBlake2b224(v.Hash[:]),
		}
		return data.NewConstr(1, cred.ToPlutusData())
	case VoterTypeStakingPoolKeyHash:
		return data.NewConstr(2, data.NewByteString(v.Hash[:]))
	default:
		return nil
	}
}

const (
	GovVoteNo      uint8 = 0
	GovVoteYes     uint8 = 1
	GovVoteAbstain uint8 = 2
)

type Vote uint8

func (v Vote) ToPlutusData() data.PlutusData {
	switch v {
	case Vote(GovVoteNo):
		return data.NewConstr(0)
	case Vote(GovVoteYes):
		return data.NewConstr(1)
	case Vote(GovVoteAbstain):
		return data.NewConstr(2)
	default:
		return nil
	}
}

type VotingProcedure struct {
	cbor.StructAsArray
	Vote   uint8
	Anchor *GovAnchor
}

func (vp VotingProcedure) ToPlutusData() data.PlutusData {
	return Vote(vp.Vote).ToPlutusData()
}

type GovAnchor struct {
	cbor.StructAsArray
	Url      string
	DataHash [32]byte
}

func (a *GovAnchor) ToPlutusData() data.PlutusData {
	return data.NewConstr(0,
		data.NewByteString([]byte(a.Url)),
		data.NewByteString(a.DataHash[:]),
	)
}

type GovActionId struct {
	cbor.StructAsArray
	TransactionId [32]byte
	GovActionIdx  uint32
}

func (id *GovActionId) ToPlutusData() data.PlutusData {
	return data.NewConstr(0,
		data.NewByteString(id.TransactionId[:]),
		data.NewInteger(big.NewInt(int64(id.GovActionIdx))),
	)
}

type ProposalProcedure struct {
	cbor.StructAsArray
	Deposit       uint64
	RewardAccount Address
	GovAction     GovActionWrapper
	Anchor        GovAnchor
}

func (p *ProposalProcedure) ToPlutusData() data.PlutusData {
	return data.NewConstr(0,
		data.NewInteger(big.NewInt(int64(p.Deposit))),
		p.RewardAccount.ToPlutusData(),
		p.GovAction.ToPlutusData(),
	)
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

func (g *GovActionWrapper) ToPlutusData() data.PlutusData {
	return g.Action.ToPlutusData()
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
	ToPlutusData() data.PlutusData
}

type ParameterChangeGovAction struct {
	cbor.StructAsArray
	Type        uint
	ActionId    *GovActionId
	ParamUpdate cbor.RawMessage // NOTE: we use raw to defer processing to account for per-era types
	PolicyHash  []byte
}

func (a *ParameterChangeGovAction) ToPlutusData() data.PlutusData {
	return data.NewConstr(0,
		a.ActionId.ToPlutusData(),
		data.NewByteString(a.ParamUpdate),
		data.NewByteString(a.PolicyHash),
	)
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

func (a *HardForkInitiationGovAction) ToPlutusData() data.PlutusData {
	return data.NewConstr(1,
		a.ActionId.ToPlutusData(),
		data.NewConstr(0,
			data.NewInteger(big.NewInt(int64(a.ProtocolVersion.Major))),
			data.NewInteger(big.NewInt(int64(a.ProtocolVersion.Minor))),
		),
	)
}

func (a HardForkInitiationGovAction) isGovAction() {}

type TreasuryWithdrawalGovAction struct {
	cbor.StructAsArray
	Type        uint
	Withdrawals map[*Address]uint64
	PolicyHash  []byte
}

func (a *TreasuryWithdrawalGovAction) ToPlutusData() data.PlutusData {
	pairs := make([][2]data.PlutusData, 0, len(a.Withdrawals))
	for addr, amount := range a.Withdrawals {
		pairs = append(pairs, [2]data.PlutusData{
			data.NewConstr(0, addr.ToPlutusData()),
			data.NewInteger(big.NewInt(int64(amount))),
		})
	}
	return data.NewConstr(2,
		data.NewMap(pairs),
		data.NewByteString(a.PolicyHash),
	)
}

func (a TreasuryWithdrawalGovAction) isGovAction() {}

type NoConfidenceGovAction struct {
	cbor.StructAsArray
	Type     uint
	ActionId *GovActionId
}

func (a *NoConfidenceGovAction) ToPlutusData() data.PlutusData {
	return data.NewConstr(3,
		a.ActionId.ToPlutusData(),
	)
}

func (a NoConfidenceGovAction) isGovAction() {}

type UpdateCommitteeGovAction struct {
	cbor.StructAsArray
	Type        uint
	ActionId    *GovActionId
	Credentials []Credential
	CredEpochs  map[*Credential]uint
	Unknown     cbor.Rat
}

func (a *UpdateCommitteeGovAction) ToPlutusData() data.PlutusData {
	removedItems := make([]data.PlutusData, 0, len(a.Credentials))
	for _, cred := range a.Credentials {
		removedItems = append(removedItems, cred.ToPlutusData())
	}

	addedPairs := make([][2]data.PlutusData, 0, len(a.CredEpochs))
	for cred, epoch := range a.CredEpochs {
		addedPairs = append(addedPairs, [2]data.PlutusData{
			cred.ToPlutusData(),
			data.NewInteger(big.NewInt(int64(epoch))),
		})
	}

	// Safe handling of Unknown Rat
	var num, den *big.Int
	if rat := a.Unknown; rat != (cbor.Rat{}) {
		val := reflect.ValueOf(rat)
		numField := val.FieldByName("num")
		denField := val.FieldByName("den")

		if numField.IsValid() && !numField.IsNil() {
			num = numField.Interface().(*big.Int)
		}
		if denField.IsValid() && !denField.IsNil() {
			den = denField.Interface().(*big.Int)
		}
	}

	// Default values if still nil
	if num == nil {
		num = big.NewInt(0)
	}
	if den == nil {
		den = big.NewInt(1)
	}

	return data.NewConstr(4,
		a.ActionId.ToPlutusData(),
		data.NewList(removedItems...),
		data.NewMap(addedPairs),
		data.NewInteger(num),
		data.NewInteger(den),
	)
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

func (a *NewConstitutionGovAction) ToPlutusData() data.PlutusData {
	return data.NewConstr(5,
		a.ActionId.ToPlutusData(),
		data.NewConstr(0,
			a.Constitution.Anchor.ToPlutusData(),
			data.NewByteString(a.Constitution.ScriptHash),
		),
	)
}

func (a NewConstitutionGovAction) isGovAction() {}

type InfoGovAction struct {
	cbor.StructAsArray
	Type uint
}

func (a *InfoGovAction) ToPlutusData() data.PlutusData {
	return data.NewConstr(6)
}

func (a InfoGovAction) isGovAction() {}
