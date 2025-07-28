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

	"github.com/blinklabs-io/plutigo/pkg/data"
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
