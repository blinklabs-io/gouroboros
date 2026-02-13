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

package localstatequery_test

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

var conversationHandshakeAcquire = []ouroboros_mock.ConversationEntry{
	ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
	ouroboros_mock.ConversationEntryHandshakeNtCResponse,
	ouroboros_mock.ConversationEntryInput{
		ProtocolId:  localstatequery.ProtocolId,
		MessageType: localstatequery.MessageTypeAcquireVolatileTip,
	},
	ouroboros_mock.ConversationEntryOutput{
		ProtocolId: localstatequery.ProtocolId,
		IsResponse: true,
		Messages: []protocol.Message{
			localstatequery.NewMsgAcquired(),
		},
	},
}

var conversationCurrentEra = append(
	conversationHandshakeAcquire,
	ouroboros_mock.ConversationEntryInput{
		ProtocolId:  localstatequery.ProtocolId,
		MessageType: localstatequery.MessageTypeQuery,
	},
	ouroboros_mock.ConversationEntryOutput{
		ProtocolId: localstatequery.ProtocolId,
		IsResponse: true,
		Messages: []protocol.Message{
			localstatequery.NewMsgResult([]byte{0x5}),
		},
	},
)

type testInnerFunc func(*testing.T, *ouroboros.Connection)

func runTest(
	t *testing.T,
	conversation []ouroboros_mock.ConversationEntry,
	innerFunc testInnerFunc,
) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		conversation,
	)
	// Async mock connection error handler
	asyncErrChan := make(chan error, 1)
	go func() {
		err := <-mockConn.(*ouroboros_mock.Connection).ErrorChan()
		if err != nil {
			asyncErrChan <- fmt.Errorf("received unexpected error: %w", err)
		}
		close(asyncErrChan)
	}()
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Ouroboros object: %s", err)
	}
	// Async error handler
	go func() {
		err, ok := <-oConn.ErrorChan()
		if !ok {
			return
		}
		// We can't call t.Fatalf() from a different Goroutine, so we panic instead
		panic(fmt.Sprintf("unexpected Ouroboros error: %s", err))
	}()
	// Run test inner function
	innerFunc(t, oConn)
	// Wait for mock connection shutdown
	select {
	case err, ok := <-asyncErrChan:
		if ok {
			t.Fatal(err.Error())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("did not complete within timeout")
	}
	// Close Ouroboros connection
	if err := oConn.Close(); err != nil {
		t.Fatalf("unexpected error when closing Ouroboros object: %s", err)
	}
	// Wait for connection shutdown
	select {
	case <-oConn.ErrorChan():
	case <-time.After(10 * time.Second):
		t.Errorf("did not shutdown within timeout")
	}
}

func TestGetCurrentEra(t *testing.T) {
	runTest(
		t,
		conversationCurrentEra,
		func(t *testing.T, oConn *ouroboros.Connection) {
			// Run query
			currentEra, err := oConn.LocalStateQuery().Client.GetCurrentEra()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if currentEra != 5 {
				t.Fatalf(
					"did not receive expected result: got %d, expected %d",
					currentEra,
					5,
				)
			}
		},
	)
}

func TestGetChainPoint(t *testing.T) {
	expectedPoint := pcommon.NewPoint(123, []byte{0xa, 0xb, 0xc})
	cborData, err := cbor.Encode(expectedPoint)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	conversation := append(
		conversationHandshakeAcquire,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(cborData),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			chainPoint, err := oConn.LocalStateQuery().Client.GetChainPoint()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if !reflect.DeepEqual(chainPoint, &expectedPoint) {
				t.Fatalf(
					"did not receive expected result\n  got:    %#v\n  wanted: %#v",
					chainPoint,
					expectedPoint,
				)
			}
		},
	)
}

func TestGetEpochNo(t *testing.T) {
	expectedEpochNo := 123456
	conversation := append(
		conversationCurrentEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(
					// [123456]
					test.DecodeHexString("811a0001e240"),
				),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			epochNo, err := oConn.LocalStateQuery().Client.GetEpochNo()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if epochNo != expectedEpochNo {
				t.Fatalf(
					"did not receive expected result, got %d, wanted %d",
					epochNo,
					expectedEpochNo,
				)
			}
		},
	)
}

func TestGetUTxOByAddress(t *testing.T) {
	testAddress, err := ledger.NewAddress(
		"addr_test1vrk294czhxhglflvxla7vxj2cjz7wyrdpxl3fj0vych5wws77xuc7",
	)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	expectedUtxoId := localstatequery.UtxoId{
		Hash: ledger.NewBlake2b256([]byte{0x1, 0x2}),
		Idx:  7,
	}
	expectedResult := localstatequery.UTxOsResult{
		Results: map[localstatequery.UtxoId]ledger.BabbageTransactionOutput{
			expectedUtxoId: {
				OutputAddress: testAddress,
				OutputAmount: ledger.MaryTransactionOutputValue{
					Amount: 234567,
				},
			},
		},
	}
	cborData, err := cbor.Encode(expectedResult)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	conversation := append(
		conversationCurrentEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(cborData),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			utxos, err := oConn.LocalStateQuery().Client.GetUTxOByAddress(
				[]ledger.Address{testAddress},
			)
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			// Set stored CBOR to nil to make comparison easier
			for k, v := range utxos.Results {
				v.SetCbor(nil)
				utxos.Results[k] = v
			}
			if !reflect.DeepEqual(utxos, &expectedResult) {
				t.Fatalf(
					"did not receive expected result\n got:    %#v\n  wanted: %#v",
					utxos,
					expectedResult,
				)
			}
		},
	)
}

// Conway governance query tests

// Create a conversation that starts with Conway era (6)
var conversationConwayEra = append(
	conversationHandshakeAcquire,
	ouroboros_mock.ConversationEntryInput{
		ProtocolId:  localstatequery.ProtocolId,
		MessageType: localstatequery.MessageTypeQuery,
	},
	ouroboros_mock.ConversationEntryOutput{
		ProtocolId: localstatequery.ProtocolId,
		IsResponse: true,
		Messages: []protocol.Message{
			localstatequery.NewMsgResult([]byte{0x6}), // Conway era = 6
		},
	},
)

func TestGetConstitution(t *testing.T) {
	// Constitution result: [anchor, script_hash]
	// anchor: [url, data_hash]
	expectedResult := localstatequery.ConstitutionResult{
		Anchor: lcommon.GovAnchor{
			Url:      "https://constitution.gov.cardano",
			DataHash: [32]byte{0x1, 0x2, 0x3, 0x4},
		},
		ScriptHash: []byte{0xa, 0xb, 0xc},
	}
	cborData, err := cbor.Encode(expectedResult)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	conversation := append(
		conversationConwayEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(cborData),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			constitution, err := oConn.LocalStateQuery().Client.GetConstitution()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if !reflect.DeepEqual(constitution, &expectedResult) {
				t.Fatalf(
					"did not receive expected result\n  got:    %#v\n  wanted: %#v",
					constitution,
					expectedResult,
				)
			}
		},
	)
}

func TestGetConstitutionPreConway(t *testing.T) {
	// Test that GetConstitution returns an error on pre-Conway eras
	runTest(
		t,
		conversationCurrentEra, // Era 5 (Babbage)
		func(t *testing.T, oConn *ouroboros.Connection) {
			_, err := oConn.LocalStateQuery().Client.GetConstitution()
			if err == nil {
				t.Fatal("expected error for pre-Conway era, got nil")
			}
			expectedErrMsg := "GetConstitution requires Conway era or later"
			if !strings.Contains(err.Error(), expectedErrMsg) {
				t.Fatalf(
					"expected error containing %q, got: %s",
					expectedErrMsg,
					err.Error(),
				)
			}
		},
	)
}

func TestGetGovState(t *testing.T) {
	// GovState is complex, test basic decoding with minimal result
	expectedResult := localstatequery.GovStateResult{
		Proposals: cbor.RawMessage([]byte{0x80}), // Empty list
		Committee: cbor.RawMessage([]byte{0xf6}), // null
		Constitution: localstatequery.ConstitutionResult{
			Anchor: lcommon.GovAnchor{
				Url:      "https://constitution.cardano.org",
				DataHash: [32]byte{0xde, 0xad, 0xbe, 0xef},
			},
			ScriptHash: nil,
		},
		CurrentPParams:   cbor.RawMessage([]byte{0xa0}), // Empty map
		PrevPParams:      cbor.RawMessage([]byte{0xa0}),
		FuturePParams:    cbor.RawMessage([]byte{0xa0}),
		DRepPulsingState: cbor.RawMessage([]byte{0x80}),
	}
	cborData, err := cbor.Encode(expectedResult)
	if err != nil {
		t.Fatalf("unexpected error encoding: %s", err)
	}
	conversation := append(
		conversationConwayEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(cborData),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			govState, err := oConn.LocalStateQuery().Client.GetGovState()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			// Check constitution anchor was decoded correctly
			if govState.Constitution.Anchor.Url != expectedResult.Constitution.Anchor.Url {
				t.Fatalf(
					"constitution URL mismatch: got %s, wanted %s",
					govState.Constitution.Anchor.Url,
					expectedResult.Constitution.Anchor.Url,
				)
			}
		},
	)
}

func TestGetGovStatePreConway(t *testing.T) {
	// Test that GetGovState returns an error on pre-Conway eras
	runTest(
		t,
		conversationCurrentEra, // Era 5 (Babbage)
		func(t *testing.T, oConn *ouroboros.Connection) {
			_, err := oConn.LocalStateQuery().Client.GetGovState()
			if err == nil {
				t.Fatal("expected error for pre-Conway era, got nil")
			}
			expectedErrMsg := "GetGovState requires Conway era or later"
			if !strings.Contains(err.Error(), expectedErrMsg) {
				t.Fatalf(
					"expected error containing %q, got: %s",
					expectedErrMsg,
					err.Error(),
				)
			}
		},
	)
}

func TestGetCommitteeMembersStateEmpty(t *testing.T) {
	// Empty members map with threshold and epoch
	threshold := &cbor.Rat{Rat: big.NewRat(2, 3)}
	expectedResult := localstatequery.CommitteeMembersStateResult{
		Members:   map[localstatequery.StakeCredential]localstatequery.CommitteeMemberState{},
		Threshold: threshold,
		Epoch:     520,
	}
	cborData, err := cbor.Encode(expectedResult)
	require.NoError(t, err, "unexpected error encoding CommitteeMembersStateResult")

	conversation := append(
		conversationConwayEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(cborData),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			committeeState, err := oConn.LocalStateQuery().Client.GetCommitteeMembersState(
				nil,
				nil,
				nil,
			)
			require.NoError(t, err, "received unexpected error")
			require.NotNil(t, committeeState)
			assert.Empty(t, committeeState.Members)
			require.NotNil(t, committeeState.Threshold)
			assert.Equal(t, big.NewRat(2, 3), committeeState.Threshold.Rat)
			assert.Equal(t, uint64(520), committeeState.Epoch)
		},
	)
}

func TestGetCommitteeMembersStateAuthorized(t *testing.T) {
	// Active member with authorized hot credential
	coldCred := localstatequery.StakeCredential{
		Tag:   0,
		Bytes: ledger.Blake2b224{0x01, 0x02, 0x03},
	}
	hotCred := lcommon.Credential{
		CredType:   0,
		Credential: lcommon.Blake2b224{0xaa, 0xbb, 0xcc},
	}
	expiry := uint64(600)
	memberState := localstatequery.CommitteeMemberState{
		HotCredStatus: localstatequery.HotCredAuthStatusValue{
			Status:     localstatequery.HotCredAuthorized,
			Credential: &hotCred,
		},
		Status:          localstatequery.MemberStatusActive,
		Expiry:          &expiry,
		NextEpochChange: localstatequery.NextEpochChangeValue{Change: localstatequery.NextEpochNoChange},
	}
	threshold := &cbor.Rat{Rat: big.NewRat(2, 3)}
	expectedResult := localstatequery.CommitteeMembersStateResult{
		Members: map[localstatequery.StakeCredential]localstatequery.CommitteeMemberState{
			coldCred: memberState,
		},
		Threshold: threshold,
		Epoch:     520,
	}
	cborData, err := cbor.Encode(expectedResult)
	require.NoError(t, err, "unexpected error encoding CommitteeMembersStateResult")

	conversation := append(
		conversationConwayEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(cborData),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			committeeState, err := oConn.LocalStateQuery().Client.GetCommitteeMembersState(
				nil,
				nil,
				nil,
			)
			require.NoError(t, err, "received unexpected error")
			require.NotNil(t, committeeState)
			require.Len(t, committeeState.Members, 1)

			member, ok := committeeState.Members[coldCred]
			require.True(t, ok, "expected cold credential key in result")
			assert.Equal(t, localstatequery.HotCredAuthorized, member.HotCredStatus.Status)
			require.NotNil(t, member.HotCredStatus.Credential)
			assert.Equal(t, localstatequery.MemberStatusActive, member.Status)
			require.NotNil(t, member.Expiry)
			assert.Equal(t, uint64(600), *member.Expiry)
			assert.Equal(t, localstatequery.NextEpochNoChange, member.NextEpochChange.Change)
			assert.Equal(t, uint64(520), committeeState.Epoch)
		},
	)
}

func TestGetCommitteeMembersStateNotAuthorized(t *testing.T) {
	// Expired member with NotAuthorized status, nil threshold,
	// nil expiry, TermAdjusted change
	coldCred := localstatequery.StakeCredential{
		Tag:   1, // ScriptHash
		Bytes: ledger.Blake2b224{0xde, 0xad},
	}
	adjustedEpoch := uint64(700)
	memberState := localstatequery.CommitteeMemberState{
		HotCredStatus: localstatequery.HotCredAuthStatusValue{
			Status: localstatequery.HotCredNotAuthorized,
		},
		Status: localstatequery.MemberStatusExpired,
		Expiry: nil,
		NextEpochChange: localstatequery.NextEpochChangeValue{
			Change:        localstatequery.NextEpochTermAdjusted,
			AdjustedEpoch: &adjustedEpoch,
		},
	}
	expectedResult := localstatequery.CommitteeMembersStateResult{
		Members: map[localstatequery.StakeCredential]localstatequery.CommitteeMemberState{
			coldCred: memberState,
		},
		Threshold: nil,
		Epoch:     521,
	}
	cborData, err := cbor.Encode(expectedResult)
	require.NoError(t, err, "unexpected error encoding CommitteeMembersStateResult")

	conversation := append(
		conversationConwayEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(cborData),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			committeeState, err := oConn.LocalStateQuery().Client.GetCommitteeMembersState(
				nil,
				nil,
				nil,
			)
			require.NoError(t, err, "received unexpected error")
			require.NotNil(t, committeeState)
			require.Len(t, committeeState.Members, 1)

			member, ok := committeeState.Members[coldCred]
			require.True(t, ok, "expected cold credential key in result")
			assert.Equal(t, localstatequery.HotCredNotAuthorized, member.HotCredStatus.Status)
			assert.Nil(t, member.HotCredStatus.Credential)
			assert.Equal(t, localstatequery.MemberStatusExpired, member.Status)
			assert.Nil(t, member.Expiry)
			assert.Equal(t, localstatequery.NextEpochTermAdjusted, member.NextEpochChange.Change)
			require.NotNil(t, member.NextEpochChange.AdjustedEpoch)
			assert.Equal(t, uint64(700), *member.NextEpochChange.AdjustedEpoch)
			assert.Nil(t, committeeState.Threshold)
			assert.Equal(t, uint64(521), committeeState.Epoch)
		},
	)
}

func TestGetSPOStakeDistr(t *testing.T) {
	// SPOStakeDistr maps pool IDs to stake amounts
	poolId := ledger.PoolId{0x1, 0x2, 0x3}
	expectedResult := localstatequery.SPOStakeDistrResult{
		Results: map[ledger.PoolId]uint64{
			poolId: 1000000000000, // 1M ADA
		},
	}
	cborData, err := cbor.Encode(expectedResult)
	if err != nil {
		t.Fatalf("unexpected error encoding: %s", err)
	}
	conversation := append(
		conversationConwayEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(cborData),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			spoDistr, err := oConn.LocalStateQuery().Client.GetSPOStakeDistr(
				nil,
			)
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if len(spoDistr.Results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(spoDistr.Results))
			}
			if stake, ok := spoDistr.Results[poolId]; !ok ||
				stake != 1000000000000 {
				t.Fatalf(
					"unexpected stake distribution result: %v",
					spoDistr.Results,
				)
			}
		},
	)
}

func TestGetDRepState(t *testing.T) {
	// Create a sample DRep state result with one entry
	cred := localstatequery.StakeCredential{
		Tag:   0, // KeyHash
		Bytes: ledger.Blake2b224{0x1, 0x2, 0x3},
	}
	anchor := lcommon.GovAnchor{
		Url:      "https://drep.example.com",
		DataHash: [32]byte{0xaa, 0xbb, 0xcc},
	}
	expectedResult := localstatequery.DRepStateResult{
		cred: localstatequery.DRepStateEntry{
			Expiry:  100,
			Anchor:  &anchor,
			Deposit: 500000000,
		},
	}
	cborData, err := cbor.Encode(expectedResult)
	require.NoError(t, err, "unexpected error encoding DRepStateResult")

	conversation := append(
		conversationConwayEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(cborData),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			drepState, err := oConn.LocalStateQuery().Client.GetDRepState(nil)
			require.NoError(t, err, "received unexpected error")
			require.NotNil(t, drepState)

			result := *drepState
			assert.Len(t, result, 1, "expected 1 DRep state entry")

			entry, ok := result[cred]
			require.True(t, ok, "expected credential key in result")
			assert.Equal(t, uint64(100), entry.Expiry)
			assert.Equal(t, uint64(500000000), entry.Deposit)
			require.NotNil(t, entry.Anchor)
			assert.Equal(t, "https://drep.example.com", entry.Anchor.Url)
			assert.Equal(t, anchor.DataHash, entry.Anchor.DataHash)
		},
	)
}

func TestGetDRepStateEmpty(t *testing.T) {
	// Test with empty result
	expectedResult := localstatequery.DRepStateResult{}
	cborData, err := cbor.Encode(expectedResult)
	require.NoError(t, err, "unexpected error encoding empty DRepStateResult")

	conversation := append(
		conversationConwayEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(cborData),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			drepState, err := oConn.LocalStateQuery().Client.GetDRepState(nil)
			require.NoError(t, err, "received unexpected error")
			require.NotNil(t, drepState)
			assert.Empty(t, *drepState, "expected empty DRep state result")
		},
	)
}

func TestGetDRepStatePreConway(t *testing.T) {
	// Test that GetDRepState returns an error on pre-Conway eras
	runTest(
		t,
		conversationCurrentEra, // Era 5 (Babbage)
		func(t *testing.T, oConn *ouroboros.Connection) {
			_, err := oConn.LocalStateQuery().Client.GetDRepState(nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "GetDRepState requires Conway era or later")
		},
	)
}

func TestGetProposalsEmpty(t *testing.T) {
	// Empty proposals list
	expectedResult := localstatequery.ProposalsResult{}
	cborData, err := cbor.Encode(expectedResult)
	if err != nil {
		t.Fatalf("unexpected error encoding: %s", err)
	}
	conversation := append(
		conversationConwayEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(cborData),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			proposals, err := oConn.LocalStateQuery().Client.GetProposals()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if len(*proposals) != 0 {
				t.Fatalf("expected 0 proposals, got %d", len(*proposals))
			}
		},
	)
}

func TestGetProposalsSingleProposal(t *testing.T) {
	// Build a single proposal with votes
	govActionId := lcommon.GovActionId{
		TransactionId: [32]byte{0x01, 0x02, 0x03},
		GovActionIdx:  0,
	}
	committeeCred := localstatequery.StakeCredential{
		Tag:   0,
		Bytes: ledger.Blake2b224{0xaa, 0xbb},
	}
	drepCred := localstatequery.StakeCredential{
		Tag:   0,
		Bytes: ledger.Blake2b224{0xcc, 0xdd},
	}
	spoKey := ledger.Blake2b224{0xee, 0xff}

	// Encode a minimal proposal procedure as RawMessage
	// (empty array is simplest valid CBOR for testing)
	proposalCbor, err := cbor.Encode([]any{
		uint64(500000000),   // deposit
		[]byte{0xe0, 0x01}, // reward account (minimal)
		[]any{uint(6)},     // info gov action
		[]any{"https://example.com", [32]byte{0xab}}, // anchor
	})
	if err != nil {
		t.Fatalf("unexpected error encoding proposal procedure: %s", err)
	}

	proposal := localstatequery.GovActionState{
		Id: govActionId,
		CommitteeVotes: map[localstatequery.StakeCredential]lcommon.Vote{
			committeeCred: lcommon.Vote(lcommon.GovVoteYes),
		},
		DRepVotes: map[localstatequery.StakeCredential]lcommon.Vote{
			drepCred: lcommon.Vote(lcommon.GovVoteNo),
		},
		SPOVotes: map[ledger.Blake2b224]lcommon.Vote{
			spoKey: lcommon.Vote(lcommon.GovVoteAbstain),
		},
		ProposalProcedure: cbor.RawMessage(proposalCbor),
		ProposedIn:        100,
		ExpiresAfter:      110,
	}
	expectedResult := localstatequery.ProposalsResult{proposal}
	cborData, err := cbor.Encode(expectedResult)
	if err != nil {
		t.Fatalf("unexpected error encoding: %s", err)
	}
	conversation := append(
		conversationConwayEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(cborData),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			proposals, err := oConn.LocalStateQuery().Client.GetProposals()
			if err != nil {
				t.Fatalf("received unexpected error: %s", err)
			}
			if len(*proposals) != 1 {
				t.Fatalf("expected 1 proposal, got %d", len(*proposals))
			}
			p := (*proposals)[0]
			if p.Id.TransactionId != govActionId.TransactionId {
				t.Fatalf("gov action ID mismatch: got %x, wanted %x",
					p.Id.TransactionId, govActionId.TransactionId)
			}
			if p.Id.GovActionIdx != govActionId.GovActionIdx {
				t.Fatalf("gov action index mismatch: got %d, wanted %d",
					p.Id.GovActionIdx, govActionId.GovActionIdx)
			}
			if len(p.CommitteeVotes) != 1 {
				t.Fatalf("expected 1 committee vote, got %d", len(p.CommitteeVotes))
			}
			if v, ok := p.CommitteeVotes[committeeCred]; !ok || v != lcommon.Vote(lcommon.GovVoteYes) {
				t.Fatalf("unexpected committee vote: %v", p.CommitteeVotes)
			}
			if len(p.DRepVotes) != 1 {
				t.Fatalf("expected 1 DRep vote, got %d", len(p.DRepVotes))
			}
			if v, ok := p.DRepVotes[drepCred]; !ok || v != lcommon.Vote(lcommon.GovVoteNo) {
				t.Fatalf("unexpected DRep vote: %v", p.DRepVotes)
			}
			if len(p.SPOVotes) != 1 {
				t.Fatalf("expected 1 SPO vote, got %d", len(p.SPOVotes))
			}
			if v, ok := p.SPOVotes[spoKey]; !ok || v != lcommon.Vote(lcommon.GovVoteAbstain) {
				t.Fatalf("unexpected SPO vote: %v", p.SPOVotes)
			}
			if p.ProposedIn != 100 {
				t.Fatalf("expected ProposedIn 100, got %d", p.ProposedIn)
			}
			if p.ExpiresAfter != 110 {
				t.Fatalf("expected ExpiresAfter 110, got %d", p.ExpiresAfter)
			}
		},
	)
}

func TestGetProposalsPreConway(t *testing.T) {
	// Test that GetProposals returns an error on pre-Conway eras
	runTest(
		t,
		conversationCurrentEra, // Era 5 (Babbage)
		func(t *testing.T, oConn *ouroboros.Connection) {
			_, err := oConn.LocalStateQuery().Client.GetProposals()
			if err == nil {
				t.Fatal("expected error for pre-Conway era, got nil")
			}
			expectedErrMsg := "GetProposals requires Conway era or later"
			if !strings.Contains(err.Error(), expectedErrMsg) {
				t.Fatalf(
					"expected error containing %q, got: %s",
					expectedErrMsg,
					err.Error(),
				)
			}
		},
	)
}

func TestGenesisConfigJSON(t *testing.T) {
	genesisConfig := localstatequery.GenesisConfigResult{
		Start: localstatequery.SystemStartResult{
			Year:        *big.NewInt(2024),
			Day:         35,
			Picoseconds: *big.NewInt(1234567890123456),
		},
		NetworkMagic:      764824073,
		NetworkId:         1,
		ActiveSlotsCoeff:  []any{0.1, 0.2},
		SecurityParam:     2160,
		EpochLength:       432000,
		SlotsPerKESPeriod: 129600,
		MaxKESEvolutions:  62,
		SlotLength:        1,
		UpdateQuorum:      5,
		MaxLovelaceSupply: 45000000000000000,
		ProtocolParams: localstatequery.GenesisConfigResultProtocolParameters{
			MinFeeA:               44,
			MinFeeB:               155381,
			MaxBlockBodySize:      65536,
			MaxTxSize:             16384,
			MaxBlockHeaderSize:    1100,
			KeyDeposit:            2000000,
			PoolDeposit:           500000000,
			EMax:                  18,
			NOpt:                  500,
			A0:                    []int{0},
			Rho:                   []int{1},
			Tau:                   []int{2},
			DecentralizationParam: []int{3},
			ExtraEntropy:          nil,
			ProtocolVersionMajor:  2,
			ProtocolVersionMinor:  0,
			MinUTxOValue:          1000000,
			MinPoolCost:           340000000,
		},
		GenDelegs: []byte("mocked_cbor_data"),
		Unknown1:  nil,
		Unknown2:  nil,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(genesisConfig)
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}

	// Unmarshal back to struct
	var result localstatequery.GenesisConfigResult
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	//Compare everything after unmarshalling

	if !reflect.DeepEqual(genesisConfig, result) {
		t.Errorf(
			"Mismatch after JSON marshalling/unmarshalling. Expected:\n%+v\nGot:\n%+v",
			genesisConfig,
			result,
		)
	} else {
		t.Logf("Successfully validated the GenesisConfigResult after JSON marshalling and unmarshalling.")
	}
}
