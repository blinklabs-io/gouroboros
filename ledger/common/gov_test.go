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
	testHash := [28]byte{
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
	}
	testHashBytes := []byte{
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
		0x11,
	}

	testCases := []struct {
		name          string
		voter         Voter
		expectedData  data.PlutusData
		expectedPanic string
	}{
		{
			name: "ConstitutionalCommitteeHotScriptHash",
			voter: Voter{
				Type: VoterTypeConstitutionalCommitteeHotScriptHash,
				Hash: testHash,
			},
			expectedData: data.NewConstr(
				0,
				data.NewConstr(1, data.NewByteString(testHashBytes)),
			),
		},
		{
			name: "ConstitutionalCommitteeHotKeyHash",
			voter: Voter{
				Type: VoterTypeConstitutionalCommitteeHotKeyHash,
				Hash: testHash,
			},
			expectedData: data.NewConstr(
				0,
				data.NewConstr(0, data.NewByteString(testHashBytes)),
			),
		},
		{
			name: "DRepScriptHash",
			voter: Voter{
				Type: VoterTypeDRepScriptHash,
				Hash: testHash,
			},
			expectedData: data.NewConstr(
				1,
				data.NewConstr(1, data.NewByteString(testHashBytes)),
			),
		},
		{
			name: "DRepKeyHash",
			voter: Voter{
				Type: VoterTypeDRepKeyHash,
				Hash: testHash,
			},
			expectedData: data.NewConstr(
				1,
				data.NewConstr(0, data.NewByteString(testHashBytes)),
			),
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
			expectedData:  nil,
			expectedPanic: "unsupported voter type: 255",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			func() {
				defer func() {
					if r := recover(); r != nil {
						if tc.expectedPanic == "" {
							t.Fatalf("unexpected panic: %v", r)
						} else {
							if r != tc.expectedPanic {
								t.Errorf("did not get expected panic: got %v, wanted %s", r, tc.expectedPanic)
							}
						}
					}
				}()
				result := tc.voter.ToPlutusData()
				if tc.expectedPanic != "" {
					t.Errorf("did not panic as expected")
					return
				}
				if !reflect.DeepEqual(result, tc.expectedData) {
					t.Errorf(
						"ToPlutusData() = %#v, want %#v",
						result,
						tc.expectedData,
					)
				}
			}()
		})
	}
}

func TestVoterString(t *testing.T) {
	var zeroHash [28]byte
	var sequentialHash [28]byte
	for i := range sequentialHash {
		sequentialHash[i] = byte(i)
	}
	testCases := []struct {
		name  string
		voter Voter
		want  string
	}{
		{
			name: "CIP129CcHotKeyHash",
			voter: Voter{
				Type: VoterTypeConstitutionalCommitteeHotKeyHash,
				Hash: zeroHash,
			},
			want: "cc_hot1qgqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqvcdjk7",
		},
		{
			name: "CIP129CcHotScriptHash",
			voter: Voter{
				Type: VoterTypeConstitutionalCommitteeHotScriptHash,
				Hash: zeroHash,
			},
			want: "cc_hot1zvqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq9zprpe",
		},
		{
			name: "CIP129DRepKeyHash",
			voter: Voter{
				Type: VoterTypeDRepKeyHash,
				Hash: zeroHash,
			},
			want: "drep1ygqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq7vlc9n",
		},
		{
			name: "CIP129DRepScriptHash",
			voter: Voter{
				Type: VoterTypeDRepScriptHash,
				Hash: zeroHash,
			},
			want: "drep1xvqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqhknfj5",
		},
		{
			name: "StakingPoolKeyHash",
			voter: Voter{
				Type: VoterTypeStakingPoolKeyHash,
				Hash: sequentialHash,
			},
			want: "pool1qqqsyqcyq5rqwzqfpg9scrgwpugpzysnzs23v9ccrydpk35lkuk",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.voter.String())
		})
	}
	// Test unknown type returns descriptive string (doesn't panic)
	t.Run("UnknownType", func(t *testing.T) {
		result := Voter{Type: 99}.String()
		assert.Equal(t, "voter_unknown_99", result)
	})
}

// Tests the ToPlutusData method for Vote types
func TestVoteToPlutusData(t *testing.T) {
	testCases := []struct {
		name          string
		vote          Vote
		expectedData  data.PlutusData
		expectedPanic string
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
			name:          "Unknown",
			vote:          Vote(255), // Unknown vote
			expectedData:  nil,
			expectedPanic: "unsupported vote type: 255",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			func() {
				defer func() {
					if r := recover(); r != nil {
						if tc.expectedPanic == "" {
							t.Fatalf("unexpected panic: %v", r)
						} else {
							if r != tc.expectedPanic {
								t.Errorf("did not get expected panic: got %v, wanted %s", r, tc.expectedPanic)
							}
						}
					}
				}()
				result := tc.vote.ToPlutusData()
				if tc.expectedPanic != "" {
					t.Errorf("did not panic as expected")
					return
				}
				if !reflect.DeepEqual(result, tc.expectedData) {
					t.Errorf(
						"ToPlutusData() = %#v, want %#v",
						result,
						tc.expectedData,
					)
				}
			}()
		})
	}
}

// Tests the ToPlutusData method for VotingProcedure types
func TestVotingProcedureToPlutusData(t *testing.T) {
	testCases := []struct {
		name          string
		procedure     VotingProcedure
		expectedData  data.PlutusData
		expectedPanic string
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
			expectedData:  nil,
			expectedPanic: "unsupported vote type: 255",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			func() {
				defer func() {
					if r := recover(); r != nil {
						if tc.expectedPanic == "" {
							t.Fatalf("unexpected panic: %v", r)
						} else {
							if r != tc.expectedPanic {
								t.Errorf("did not get expected panic: got %v, wanted %s", r, tc.expectedPanic)
							}
						}
					}
				}()
				result := tc.procedure.ToPlutusData()
				if tc.expectedPanic != "" {
					t.Errorf("did not panic as expected")
					return
				}
				if !reflect.DeepEqual(result, tc.expectedData) {
					t.Errorf(
						"ToPlutusData() = %#v, want %#v",
						result,
						tc.expectedData,
					)
				}
			}()
		})
	}
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

		innerConstr, ok := constr.Fields[3].(*data.Constr)
		assert.True(t, ok)
		assert.Equal(t, uint(0), innerConstr.Tag)

		// Verify default values were used
		num, ok := innerConstr.Fields[0].(*data.Integer)
		assert.True(t, ok)
		assert.Equal(t, int64(0), num.Inner.Int64())

		den, ok := innerConstr.Fields[1].(*data.Integer)
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

// TestGovActionIdString tests CIP-0129 bech32 encoding for governance action IDs.
func TestGovActionIdString(t *testing.T) {
	testCases := []struct {
		name        string
		govActionId GovActionId
		wantPrefix  string
	}{
		{
			name: "ZeroTxIdZeroIdx",
			govActionId: GovActionId{
				TransactionId: [32]byte{},
				GovActionIdx:  0,
			},
			wantPrefix: "gov_action",
		},
		{
			name: "NonZeroTxIdWithIdx",
			govActionId: GovActionId{
				TransactionId: [32]byte{
					0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
					0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
					0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
					0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
				},
				GovActionIdx: 42,
			},
			wantPrefix: "gov_action",
		},
		{
			name: "MaxValidActionIdx",
			govActionId: GovActionId{
				TransactionId: [32]byte{
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
					0xff,
				},
				GovActionIdx: 255,
			},
			wantPrefix: "gov_action",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.govActionId.String()
			assert.True(
				t,
				len(result) > len(tc.wantPrefix),
				"result should be longer than prefix",
			)
			assert.Equal(
				t,
				tc.wantPrefix+"1",
				result[:len(tc.wantPrefix)+1],
				"should have correct prefix",
			)
		})
	}

	// Test panic on index exceeding CIP-0129 limit (must fit in single byte)
	t.Run("PanicOnIndexExceedsLimit", func(t *testing.T) {
		assert.Panics(t, func() {
			govActionId := GovActionId{
				TransactionId: [32]byte{},
				GovActionIdx:  256,
			}
			_ = govActionId.String()
		})
	})

	// Test panic on large index value
	t.Run("PanicOnLargeIndex", func(t *testing.T) {
		assert.Panics(t, func() {
			govActionId := GovActionId{
				TransactionId: [32]byte{},
				GovActionIdx:  65535,
			}
			_ = govActionId.String()
		})
	})
}
