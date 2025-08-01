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
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/assert"
)

// Ttests the ToPlutusData method for Voter types
func TestVoterToPlutusData(t *testing.T) {
	// Use the same hash value for all tests to avoid confusion
	testHash := [28]byte{0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11}
	testHashBytes := []byte{0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11}

	testCases := []struct {
		name         string
		voter        Voter
		expectedData data.PlutusData
	}{
		{
			name: "ConstitutionalCommitteeHotScriptHash",
			voter: Voter{
				Type: VoterTypeConstitutionalCommitteeHotScriptHash,
				Hash: testHash,
			},
			expectedData: data.NewConstr(0, data.NewConstr(1, data.NewByteString(testHashBytes))),
		},
		{
			name: "ConstitutionalCommitteeHotKeyHash",
			voter: Voter{
				Type: VoterTypeConstitutionalCommitteeHotKeyHash,
				Hash: testHash,
			},
			expectedData: data.NewConstr(0, data.NewConstr(0, data.NewByteString(testHashBytes))),
		},
		{
			name: "DRepScriptHash",
			voter: Voter{
				Type: VoterTypeDRepScriptHash,
				Hash: testHash,
			},
			expectedData: data.NewConstr(1, data.NewConstr(1, data.NewByteString(testHashBytes))),
		},
		{
			name: "DRepKeyHash",
			voter: Voter{
				Type: VoterTypeDRepKeyHash,
				Hash: testHash,
			},
			expectedData: data.NewConstr(1, data.NewConstr(0, data.NewByteString(testHashBytes))),
		},
		{
			name: "StakingPoolKeyHash",
			voter: Voter{
				Type: VoterTypeStakingPoolKeyHash,
				Hash: testHash,
			},
			expectedData: data.NewConstr(2, data.NewByteString(testHashBytes)),
		},
		{
			name: "UnknownType",
			voter: Voter{
				Type: 255, // Unknown type
				Hash: [28]byte{},
			},
			expectedData: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.voter.ToPlutusData()
			if !reflect.DeepEqual(result, tc.expectedData) {
				t.Errorf("ToPlutusData() = %#v, want %#v", result, tc.expectedData)
			}
		})
	}
}

// Tests the ToPlutusData method for Vote types
func TestVoteToPlutusData(t *testing.T) {
	testCases := []struct {
		name         string
		vote         Vote
		expectedData data.PlutusData
	}{
		{
			name:         "No",
			vote:         Vote(GovVoteNo),
			expectedData: data.NewConstr(0),
		},
		{
			name:         "Yes",
			vote:         Vote(GovVoteYes),
			expectedData: data.NewConstr(1),
		},
		{
			name:         "Abstain",
			vote:         Vote(GovVoteAbstain),
			expectedData: data.NewConstr(2),
		},
		{
			name:         "Unknown",
			vote:         Vote(255), // Unknown vote
			expectedData: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.vote.ToPlutusData()
			if !reflect.DeepEqual(result, tc.expectedData) {
				t.Errorf("ToPlutusData() = %#v, want %#v", result, tc.expectedData)
			}
		})
	}
}

// Tests the ToPlutusData method for VotingProcedure types
func TestVotingProcedureToPlutusData(t *testing.T) {
	testCases := []struct {
		name         string
		procedure    VotingProcedure
		expectedData data.PlutusData
	}{
		{
			name: "NoVote",
			procedure: VotingProcedure{
				Vote: GovVoteNo,
			},
			expectedData: data.NewConstr(0),
		},
		{
			name: "YesVote",
			procedure: VotingProcedure{
				Vote: GovVoteYes,
			},
			expectedData: data.NewConstr(1),
		},
		{
			name: "AbstainVote",
			procedure: VotingProcedure{
				Vote: GovVoteAbstain,
			},
			expectedData: data.NewConstr(2),
		},
		{
			name: "UnknownVote",
			procedure: VotingProcedure{
				Vote: 255, // Unknown vote
			},
			expectedData: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.procedure.ToPlutusData()
			if !reflect.DeepEqual(result, tc.expectedData) {
				t.Errorf("ToPlutusData() = %#v, want %#v", result, tc.expectedData)
			}
		})
	}
}

func TestProposalProcedureToPlutusData(t *testing.T) {
	addr := Address{}
	action := &InfoGovAction{}

	pp := &ProposalProcedure{
		Deposit:       1000000,
		RewardAccount: addr,
		GovAction:     GovActionWrapper{Action: action},
	}

	pd := pp.ToPlutusData()
	constr, ok := pd.(*data.Constr)
	assert.True(t, ok)
	assert.Equal(t, uint(0), constr.Tag)
	assert.Len(t, constr.Fields, 3)
}

func TestGovActionIdToPlutusData(t *testing.T) {
	txId := [32]byte{1, 2, 3, 4}
	govActionId := &GovActionId{
		TransactionId: txId,
		GovActionIdx:  42,
	}

	pd := govActionId.ToPlutusData()
	constr, ok := pd.(*data.Constr)
	assert.True(t, ok)
	assert.Equal(t, uint(0), constr.Tag)
	assert.Len(t, constr.Fields, 2)
}

func TestParameterChangeGovActionToPlutusData(t *testing.T) {
	action := &ParameterChangeGovAction{
		ActionId:    &GovActionId{},
		ParamUpdate: []byte{1, 2, 3},
		PolicyHash:  []byte{4, 5, 6},
	}

	pd := action.ToPlutusData()
	constr, ok := pd.(*data.Constr)
	assert.True(t, ok)
	assert.Equal(t, uint(0), constr.Tag)
	assert.Len(t, constr.Fields, 3)
}

func TestHardForkInitiationGovActionToPlutusData(t *testing.T) {
	action := &HardForkInitiationGovAction{
		ActionId: &GovActionId{},
		ProtocolVersion: struct {
			cbor.StructAsArray
			Major uint
			Minor uint
		}{
			Major: 8,
			Minor: 0,
		},
	}

	pd := action.ToPlutusData()
	constr, ok := pd.(*data.Constr)
	assert.True(t, ok)
	assert.Equal(t, uint(1), constr.Tag)
	assert.Len(t, constr.Fields, 2)
}

func TestTreasuryWithdrawalGovActionToPlutusData(t *testing.T) {
	addr := Address{}
	withdrawals := map[*Address]uint64{
		&addr: 5000000,
	}

	action := &TreasuryWithdrawalGovAction{
		Withdrawals: withdrawals,
		PolicyHash:  []byte{1, 2, 3},
	}

	pd := action.ToPlutusData()
	constr, ok := pd.(*data.Constr)
	assert.True(t, ok)
	assert.Equal(t, uint(2), constr.Tag)
	assert.Len(t, constr.Fields, 2)
}

func TestNoConfidenceGovActionToPlutusData(t *testing.T) {
	action := &NoConfidenceGovAction{
		ActionId: &GovActionId{},
	}

	pd := action.ToPlutusData()
	constr, ok := pd.(*data.Constr)
	assert.True(t, ok)
	assert.Equal(t, uint(3), constr.Tag)
	assert.Len(t, constr.Fields, 1)
}

func TestUpdateCommitteeGovActionToPlutusData(t *testing.T) {
	cred := Credential{
		CredType:   CredentialTypeAddrKeyHash,
		Credential: NewBlake2b224([]byte("test")),
	}
	creds := []Credential{cred}
	credEpochs := map[*Credential]uint{
		&cred: 42,
	}

	// Test with zero value Rat
	t.Run("ZeroValueRat", func(t *testing.T) {
		action := &UpdateCommitteeGovAction{
			ActionId:    &GovActionId{},
			Credentials: creds,
			CredEpochs:  credEpochs,
			Quorum:      cbor.Rat{}, // Zero value
		}

		pd := action.ToPlutusData()
		assert.NotNil(t, pd)

		constr, ok := pd.(*data.Constr)
		assert.True(t, ok)
		assert.Equal(t, uint(4), constr.Tag)

		// Verify default values were used
		num, ok := constr.Fields[3].(*data.Integer)
		assert.True(t, ok)
		assert.Equal(t, int64(0), num.Inner.Int64())

		den, ok := constr.Fields[4].(*data.Integer)
		assert.True(t, ok)
		assert.Equal(t, int64(1), den.Inner.Int64())
	})

	t.Run("NilCase", func(t *testing.T) {
	})
}

func TestNewConstitutionGovActionToPlutusData(t *testing.T) {
	action := &NewConstitutionGovAction{
		ActionId: &GovActionId{},
		Constitution: struct {
			cbor.StructAsArray
			Anchor     GovAnchor
			ScriptHash []byte
		}{
			ScriptHash: []byte{1, 2, 3},
		},
	}

	pd := action.ToPlutusData()
	constr, ok := pd.(*data.Constr)
	assert.True(t, ok)
	assert.Equal(t, uint(5), constr.Tag)
	assert.Len(t, constr.Fields, 2)
}

func TestInfoGovActionToPlutusData(t *testing.T) {
	action := &InfoGovAction{}

	pd := action.ToPlutusData()
	constr, ok := pd.(*data.Constr)
	assert.True(t, ok)
	assert.Equal(t, uint(6), constr.Tag)
	assert.Len(t, constr.Fields, 0)
}
