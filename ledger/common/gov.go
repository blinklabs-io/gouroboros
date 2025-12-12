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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/btcsuite/btcd/btcutil/bech32"
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

func encodeCip129Voter(
	prefix string,
	keyType uint8,
	credentialType uint8,
	hash []byte,
) string {
	// Header packs the 4-bit key type (per CIP-129) in the high nibble and the credential semantics in the low nibble.
	// Since CIP-129 reserves values 0 and 1, we offset the existing credential constants (0 = key hash, 1 = script hash) by 2
	// so the output nibble matches the spec's 0x2/0x3 tags.
	header := byte((keyType << 4) | ((credentialType + 2) & 0x0f))
	data := make([]byte, 1+len(hash))
	data[0] = header
	copy(data[1:], hash)
	convData, err := bech32.ConvertBits(data, 8, 5, true)
	if err != nil {
		panic(fmt.Sprintf("unexpected error converting voter data to base32: %s", err))
	}
	encoded, err := bech32.Encode(prefix, convData)
	if err != nil {
		panic(fmt.Sprintf("unexpected error encoding voter data as bech32: %s", err))
	}
	return encoded
}

// Generates bech32-encoded identifier for the voter credential.
func (v Voter) String() string {
	switch v.Type {
	case VoterTypeConstitutionalCommitteeHotKeyHash:
		return encodeCip129Voter(
			"cc_hot",
			VoterTypeConstitutionalCommitteeHotKeyHash,
			CredentialTypeAddrKeyHash,
			v.Hash[:],
		)
	case VoterTypeConstitutionalCommitteeHotScriptHash:
		return encodeCip129Voter(
			"cc_hot",
			VoterTypeConstitutionalCommitteeHotKeyHash,
			CredentialTypeScriptHash,
			v.Hash[:],
		)
	case VoterTypeDRepKeyHash:
		return encodeCip129Voter(
			"drep",
			VoterTypeDRepKeyHash,
			CredentialTypeAddrKeyHash,
			v.Hash[:],
		)
	case VoterTypeDRepScriptHash:
		return encodeCip129Voter(
			"drep",
			VoterTypeDRepKeyHash,
			CredentialTypeScriptHash,
			v.Hash[:],
		)
	case VoterTypeStakingPoolKeyHash:
		poolId := PoolId(v.Hash)
		return poolId.String()
	default:
		panic(fmt.Sprintf("unknown voter type: %d", v.Type))
	}
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

type ProposalProcedure interface {
	isProposalProcedure()
	ToPlutusData() data.PlutusData
	Deposit() uint64
	RewardAccount() Address
	GovAction() GovAction
	Anchor() GovAnchor
}

type ProposalProcedureBase struct{}

//nolint:unused
func (ProposalProcedureBase) isProposalProcedure() {}

const (
	GovActionTypeParameterChange    = 0
	GovActionTypeHardForkInitiation = 1
	GovActionTypeTreasuryWithdrawal = 2
	GovActionTypeNoConfidence       = 3
	GovActionTypeUpdateCommittee    = 4
	GovActionTypeNewConstitution    = 5
	GovActionTypeInfo               = 6
)

type GovAction interface {
	isGovAction()
	ToPlutusData() data.PlutusData
}

type GovActionBase struct{}

//nolint:unused
func (GovActionBase) isGovAction() {}

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
	actionId := data.NewConstr(1)
	if a.ActionId != nil {
		actionId = data.NewConstr(0, a.ActionId.ToPlutusData())
	}
	return data.NewConstr(1,
		actionId,
		data.NewConstr(
			0,
			data.NewInteger(
				new(big.Int).SetUint64(uint64(a.ProtocolVersion.Major)),
			),
			data.NewInteger(
				new(big.Int).SetUint64(uint64(a.ProtocolVersion.Minor)),
			),
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
			addr.ToPlutusData(),
			data.NewInteger(new(big.Int).SetUint64(amount)),
		})
	}
	policyHash := data.NewConstr(1)
	if len(a.PolicyHash) > 0 {
		policyHash = data.NewConstr(
			0,
			data.NewByteString(a.PolicyHash),
		)
	}
	return data.NewConstr(2,
		data.NewMap(pairs),
		policyHash,
	)
}

func (a TreasuryWithdrawalGovAction) isGovAction() {}

type NoConfidenceGovAction struct {
	cbor.StructAsArray
	Type     uint
	ActionId *GovActionId
}

func (a *NoConfidenceGovAction) ToPlutusData() data.PlutusData {
	actionId := data.NewConstr(1)
	if a.ActionId != nil {
		actionId = data.NewConstr(0, a.ActionId.ToPlutusData())
	}
	return data.NewConstr(3,
		actionId,
	)
}

func (a NoConfidenceGovAction) isGovAction() {}

type UpdateCommitteeGovAction struct {
	cbor.StructAsArray
	Type        uint
	ActionId    *GovActionId
	Credentials []Credential
	CredEpochs  map[*Credential]uint
	Quorum      cbor.Rat
}

func (a *UpdateCommitteeGovAction) ToPlutusData() data.PlutusData {
	actionId := data.NewConstr(1)
	if a.ActionId != nil {
		actionId = data.NewConstr(0, a.ActionId.ToPlutusData())
	}
	removedItems := make([]data.PlutusData, 0, len(a.Credentials))
	for _, cred := range a.Credentials {
		removedItems = append(removedItems, cred.ToPlutusData())
	}

	addedPairs := make([][2]data.PlutusData, 0, len(a.CredEpochs))
	for cred, epoch := range a.CredEpochs {
		addedPairs = append(addedPairs, [2]data.PlutusData{
			cred.ToPlutusData(),
			data.NewInteger(new(big.Int).SetUint64(uint64(epoch))),
		})
	}

	// Get numerator and denominator using Rat methods
	var num, den *big.Int
	if a.Quorum != (cbor.Rat{}) {
		num = a.Quorum.Num()
		den = a.Quorum.Denom()
	} else {
		num = big.NewInt(0)
		den = big.NewInt(1)
	}

	return data.NewConstr(4,
		actionId,
		data.NewList(removedItems...),
		data.NewMap(addedPairs),
		data.NewConstr(
			0,
			data.NewInteger(num),
			data.NewInteger(den),
		),
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
	actionId := data.NewConstr(1)
	if a.ActionId != nil {
		actionId = data.NewConstr(0, a.ActionId.ToPlutusData())
	}
	scriptHash := data.NewConstr(1)
	if len(a.Constitution.ScriptHash) > 0 {
		scriptHash = data.NewConstr(
			0,
			data.NewByteString(a.Constitution.ScriptHash),
		)
	}
	return data.NewConstr(5,
		actionId,
		data.NewConstr(0,
			scriptHash,
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
