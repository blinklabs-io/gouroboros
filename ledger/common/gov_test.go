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

package common

import (
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestVotingProceduresAddOrReplaceValueSemantics(t *testing.T) {
	voter := testVotingProceduresVoter(
		VoterTypeDRepKeyHash,
		0x11,
	)
	action := testVotingProceduresGovActionId(0x22, 0)

	votes := VotingProcedures(nil)
	votes = votes.AddOrReplace(
		voter,
		action,
		VotingProcedure{Vote: GovVoteNo},
	)
	votes = votes.AddOrReplace(
		testVotingProceduresVoter(VoterTypeDRepKeyHash, 0x11),
		testVotingProceduresGovActionId(0x22, 0),
		VotingProcedure{Vote: GovVoteYes},
	)

	require.Len(t, votes, 1)
	_, actions, ok := votes.LookupVoter(voter)
	require.True(t, ok)
	require.Len(t, actions, 1)

	procedure, ok := votes.Lookup(voter, action)
	require.True(t, ok)
	assert.Equal(t, GovVoteYes, procedure.Vote)
}

func TestVotingProceduresAddOrReplaceDistinctActionsForOneVoter(
	t *testing.T,
) {
	voter := testVotingProceduresVoter(
		VoterTypeDRepKeyHash,
		0x11,
	)
	action1 := testVotingProceduresGovActionId(0x22, 0)
	action2 := testVotingProceduresGovActionId(0x22, 1)

	votes := VotingProcedures(nil)
	votes = votes.AddOrReplace(
		voter,
		action1,
		VotingProcedure{Vote: GovVoteYes},
	)
	votes = votes.AddOrReplace(
		testVotingProceduresVoter(VoterTypeDRepKeyHash, 0x11),
		action2,
		VotingProcedure{Vote: GovVoteAbstain},
	)

	require.Len(t, votes, 1)
	_, actions, ok := votes.LookupVoter(voter)
	require.True(t, ok)
	require.Len(t, actions, 2)

	procedure1, ok := votes.Lookup(voter, action1)
	require.True(t, ok)
	assert.Equal(t, GovVoteYes, procedure1.Vote)

	procedure2, ok := votes.Lookup(voter, action2)
	require.True(t, ok)
	assert.Equal(t, GovVoteAbstain, procedure2.Vote)
}

func TestVotingProceduresAddOrReplaceDistinctVotersForOneAction(
	t *testing.T,
) {
	voter1 := testVotingProceduresVoter(
		VoterTypeDRepKeyHash,
		0x11,
	)
	voter2 := testVotingProceduresVoter(
		VoterTypeDRepKeyHash,
		0x12,
	)
	action := testVotingProceduresGovActionId(0x22, 0)

	votes := VotingProcedures(nil)
	votes = votes.AddOrReplace(
		voter1,
		action,
		VotingProcedure{Vote: GovVoteYes},
	)
	votes = votes.AddOrReplace(
		voter2,
		testVotingProceduresGovActionId(0x22, 0),
		VotingProcedure{Vote: GovVoteNo},
	)

	require.Len(t, votes, 2)

	procedure1, ok := votes.Lookup(voter1, action)
	require.True(t, ok)
	assert.Equal(t, GovVoteYes, procedure1.Vote)

	procedure2, ok := votes.Lookup(voter2, action)
	require.True(t, ok)
	assert.Equal(t, GovVoteNo, procedure2.Vote)
}

func TestVotingProceduresAddOrReplaceCopiesInputValues(t *testing.T) {
	voter := testVotingProceduresVoter(
		VoterTypeDRepKeyHash,
		0x11,
	)
	action := testVotingProceduresGovActionId(0x22, 0)
	anchor := testVotingProceduresAnchor("https://example.com/a", 0x33)
	originalVoter := voter
	originalAction := action

	votes := VotingProcedures(nil).AddOrReplace(
		voter,
		action,
		VotingProcedure{
			Vote:   GovVoteYes,
			Anchor: anchor,
		},
	)

	voter.Hash[0] = 0xff
	action.GovActionIdx = 99
	anchor.Url = "https://example.com/changed"

	procedure, ok := votes.Lookup(originalVoter, originalAction)
	require.True(t, ok)
	require.NotNil(t, procedure.Anchor)
	assert.Equal(t, GovVoteYes, procedure.Vote)
	assert.Equal(t, "https://example.com/a", procedure.Anchor.Url)
}

func TestVotingProceduresCloneIndependence(t *testing.T) {
	voter := testVotingProceduresVoter(
		VoterTypeDRepKeyHash,
		0x11,
	)
	action := testVotingProceduresGovActionId(0x22, 0)
	anchor := testVotingProceduresAnchor("https://example.com/a", 0x33)

	original := VotingProcedures(nil).AddOrReplace(
		voter,
		action,
		VotingProcedure{
			Vote:   GovVoteYes,
			Anchor: anchor,
		},
	)
	clone := original.Clone()

	cloneVoterKey, cloneActions, ok := clone.LookupVoter(voter)
	require.True(t, ok)
	require.NotNil(t, cloneActions)
	cloneActionKey, cloneProcedure, ok := clone.LookupGovActionId(
		voter,
		action,
	)
	require.True(t, ok)
	require.NotNil(t, cloneProcedure.Anchor)

	cloneVoterKey.Hash[0] = 0xff
	cloneActionKey.GovActionIdx = 99
	cloneProcedure.Anchor.Url = "https://example.com/changed"
	cloneActions[cloneActionKey] = VotingProcedure{Vote: GovVoteNo}

	originalProcedure, ok := original.Lookup(voter, action)
	require.True(t, ok)
	require.NotNil(t, originalProcedure.Anchor)
	assert.Equal(t, GovVoteYes, originalProcedure.Vote)
	assert.Equal(t, "https://example.com/a", originalProcedure.Anchor.Url)
}

func TestVotingProceduresClonePreservesNilActions(t *testing.T) {
	voter := testVotingProceduresVoter(
		VoterTypeDRepKeyHash,
		0x11,
	)
	voterKey := voter
	original := VotingProcedures{
		&voterKey: nil,
	}

	clone := original.Clone()
	_, actions, ok := clone.LookupVoter(voter)

	require.True(t, ok)
	assert.Nil(t, actions)
}

func TestVotingProceduresAddOrReplaceCborEncodingUnchanged(t *testing.T) {
	voter := testVotingProceduresVoter(
		VoterTypeDRepKeyHash,
		0x11,
	)
	action := testVotingProceduresGovActionId(0x22, 0)
	procedure := VotingProcedure{
		Vote: GovVoteYes,
		Anchor: testVotingProceduresAnchor(
			"https://example.com/a",
			0x33,
		),
	}
	directVoter := voter
	directAction := action
	direct := VotingProcedures{
		&directVoter: {
			&directAction: procedure,
		},
	}
	viaHelper := VotingProcedures(nil).AddOrReplace(
		voter,
		action,
		procedure,
	)

	directCbor, err := cbor.Encode(direct)
	require.NoError(t, err)
	helperCbor, err := cbor.Encode(viaHelper)
	require.NoError(t, err)
	assert.Equal(t, directCbor, helperCbor)
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

func TestVoterTextRoundTrip(t *testing.T) {
	var zeroHash [28]byte
	var sequentialHash [28]byte
	for i := range sequentialHash {
		sequentialHash[i] = byte(i)
	}
	testCases := []struct {
		name  string
		voter Voter
	}{
		{
			name:  "CcHotKeyHash",
			voter: Voter{Type: VoterTypeConstitutionalCommitteeHotKeyHash, Hash: zeroHash},
		},
		{
			name:  "CcHotScriptHash",
			voter: Voter{Type: VoterTypeConstitutionalCommitteeHotScriptHash, Hash: zeroHash},
		},
		{
			name:  "DRepKeyHash",
			voter: Voter{Type: VoterTypeDRepKeyHash, Hash: sequentialHash},
		},
		{
			name:  "DRepScriptHash",
			voter: Voter{Type: VoterTypeDRepScriptHash, Hash: sequentialHash},
		},
		{
			name:  "StakingPoolKeyHash",
			voter: Voter{Type: VoterTypeStakingPoolKeyHash, Hash: sequentialHash},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			text, err := tc.voter.MarshalText()
			require.NoError(t, err)
			var decoded Voter
			require.NoError(t, decoded.UnmarshalText(text))
			assert.Equal(t, tc.voter.Type, decoded.Type)
			assert.Equal(t, tc.voter.Hash, decoded.Hash)
		})
	}
}

func testVotingProceduresVoter(voterType uint8, seed byte) Voter {
	var hash [28]byte
	for i := range hash {
		hash[i] = seed + byte(i)
	}
	return Voter{
		Type: voterType,
		Hash: hash,
	}
}

func testVotingProceduresGovActionId(seed byte, idx uint32) GovActionId {
	var txId [32]byte
	for i := range txId {
		txId[i] = seed + byte(i)
	}
	return GovActionId{
		TransactionId: txId,
		GovActionIdx:  idx,
	}
}

func testVotingProceduresAnchor(url string, seed byte) *GovAnchor {
	var dataHash [32]byte
	for i := range dataHash {
		dataHash[i] = seed + byte(i)
	}
	return &GovAnchor{
		Url:      url,
		DataHash: dataHash,
	}
}

func TestVoterMarshalTextUnknownType(t *testing.T) {
	v := Voter{Type: 99}
	_, err := v.MarshalText()
	assert.Error(t, err)
}

func TestVoterUnmarshalTextErrors(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{"InvalidBech32", "not-valid-bech32!!!"},
		{"UnknownPrefix", "unknown1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq2uwmrs"},
		{"CcHotWithDRepType", "cc_hot1ygqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq7guj37"},
		{"DRepWithCcHotType", "drep1qgqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqvuwczn"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var v Voter
			assert.Error(t, v.UnmarshalText([]byte(tc.input)))
		})
	}
}

func TestGovActionIdTextRoundTrip(t *testing.T) {
	testCases := []struct {
		name        string
		govActionId GovActionId
	}{
		{
			name: "ZeroTxIdZeroIdx",
			govActionId: GovActionId{
				TransactionId: [32]byte{},
				GovActionIdx:  0,
			},
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
		},
		{
			name: "MaxValidIdx",
			govActionId: GovActionId{
				TransactionId: [32]byte{0xff},
				GovActionIdx:  255,
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			text, err := tc.govActionId.MarshalText()
			require.NoError(t, err)
			var decoded GovActionId
			require.NoError(t, decoded.UnmarshalText(text))
			assert.Equal(t, tc.govActionId.TransactionId, decoded.TransactionId)
			assert.Equal(t, tc.govActionId.GovActionIdx, decoded.GovActionIdx)
		})
	}
}

func TestGovActionIdMarshalTextNil(t *testing.T) {
	var id *GovActionId
	_, err := id.MarshalText()
	assert.Error(t, err)
}

func TestGovActionIdMarshalTextOverflow(t *testing.T) {
	id := &GovActionId{GovActionIdx: 256}
	_, err := id.MarshalText()
	assert.Error(t, err)
}

func TestGovActionIdUnmarshalTextErrors(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{"InvalidBech32", "not-valid!!!"},
		{"WrongPrefix", "pool1qqqsyqcyq5rqwzqfpg9scrgwpugpzysnzs23v9ccrydpk35lkuk"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var id GovActionId
			assert.Error(t, id.UnmarshalText([]byte(tc.input)))
		})
	}
}
