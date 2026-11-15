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

package localstatequery_test

import (
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixtures follow the cardano-ledger encoders named in gov_state_wire_test.go
// and the ouroboros-consensus hard fork combinator, whose encodeEitherMismatch
// writes a one-element list around every era query result:
//
//	Constitution  Conway/Governance/Procedures.hs, EncCBOR Constitution:
//	              [anchor, encodeNullStrictMaybe script]; a missing script is
//	              null and a present one is the bare hash
//	RatifyState   Conway/Governance/Internal.hs, EncCBOR RatifyState:
//	              [EnactState, Seq GovActionState, Set GovActionId, Bool]
//	Set           Binary/Encoding/Encoder.hs, encodeSet: tag 258 from protocol
//	              version 9, indefinite length above 23 elements
//	Seq           Binary/Encoding/Encoder.hs, encodeSeq: indefinite length
//	              above 23 elements

// hsEraResult is the hard fork combinator's wrapper around an era result.
func hsEraResult(result []byte) []byte {
	return hsArray(result)
}

func hsSet(items [][]byte) []byte {
	return cat([]byte{0xd9, 0x01, 0x02}, hsList(items))
}

func hsConstitution(url string, fill byte, script []byte) []byte {
	guardrails := []byte{0xf6}
	if script != nil {
		guardrails = hsBytes(script)
	}
	return hsArray(hsAnchor(url, fill), guardrails)
}

func hsEnactStateWith(
	committee []byte,
	constitution []byte,
	withdrawals [][2][]byte,
) []byte {
	return hsArray(
		committee,
		constitution,
		hsPParams,
		hsPrevPPar,
		hsUint(1000),
		hsMap(withdrawals...),
		hsRoots(),
	)
}

func hsRatifyState(
	enactState []byte,
	enacted [][]byte,
	expired [][]byte,
	delayed bool,
) []byte {
	flag := []byte{0xf4}
	if delayed {
		flag = []byte{0xf5}
	}
	return hsArray(enactState, hsList(enacted), hsSet(expired), flag)
}

// conversationEra is conversationConwayEra for an arbitrary era index.
func conversationEra(era byte) []ouroboros_mock.ConversationEntry {
	return append(
		conversationHandshakeAcquire,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult([]byte{era}),
			},
		},
	)
}

func eraQueryConversation(era byte, reply []byte) []ouroboros_mock.ConversationEntry {
	return append(
		conversationEra(era),
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(reply),
			},
		},
	)
}

var wireEras = []struct {
	name string
	era  byte
}{
	{"conway", 6},
	{"dijkstra", 7},
}

func TestGetConstitutionDecodesNodeReply(t *testing.T) {
	// Not t.Parallel: runTest verifies goroutines with goleak.VerifyNone, a
	// process-wide check.
	script := hash(0x5c, 28)
	cases := []struct {
		name       string
		reply      []byte
		wantScript []byte
	}{
		{
			name: "no guardrails script",
			reply: hsEraResult(
				hsConstitution("https://example.invalid/constitution", 0xde, nil),
			),
		},
		{
			name: "guardrails script",
			reply: hsEraResult(
				hsConstitution("https://example.invalid/constitution", 0xde, script),
			),
			wantScript: script,
		},
	}
	for _, era := range wireEras {
		for _, tc := range cases {
			t.Run(era.name+"/"+tc.name, func(t *testing.T) {
				runTest(
					t,
					eraQueryConversation(era.era, tc.reply),
					func(t *testing.T, oConn *ouroboros.Connection) {
						got, err := oConn.LocalStateQuery().Client.GetConstitution()
						require.NoError(t, err)
						assert.Equal(
							t,
							"https://example.invalid/constitution",
							got.Anchor.Url,
						)
						assert.Equal(t, [32]byte(hash(0xde, 32)), got.Anchor.DataHash)
						assert.Equal(t, tc.wantScript, got.ScriptHash)
					},
				)
			})
		}
	}
}

func TestGetConstitutionRejectsUnwrappedReply(t *testing.T) {
	// Not t.Parallel: runTest verifies goroutines with goleak.VerifyNone, a
	// process-wide check.
	reply := hsConstitution("https://example.invalid/constitution", 0xde, nil)
	runTest(
		t,
		eraQueryConversation(6, reply),
		func(t *testing.T, oConn *ouroboros.Connection) {
			_, err := oConn.LocalStateQuery().Client.GetConstitution()
			require.Error(t, err)
		},
	)
}

func TestGetRatifyStateDecodesNodeReply(t *testing.T) {
	// Not t.Parallel: runTest verifies goroutines with goleak.VerifyNone, a
	// process-wide check.
	constitution := hsConstitution("https://example.invalid/c", 0x71, hash(0x5c, 28))
	many := func(n int, mk func(i int) []byte) [][]byte {
		out := make([][]byte, n)
		for i := range out {
			out[i] = mk(i)
		}
		return out
	}
	// More than 23 elements makes the ledger encoder write indefinite-length
	// lists, sets and maps.
	var withdrawals [][2][]byte
	for i := range 24 {
		withdrawals = append(
			withdrawals,
			[2][]byte{hsKeyCred(byte(0x10 + i)), hsUint(i)},
		)
	}
	cases := []struct {
		name        string
		reply       []byte
		wantEnacted int
		wantExpired int
		wantWithdr  int
		wantDelayed bool
		firstExpiry byte
		firstIdx    uint32
	}{
		{
			name: "small",
			reply: hsEraResult(hsRatifyState(
				hsEnactStateWith(
					hsSome(hsCommittee()),
					constitution,
					[][2][]byte{{hsKeyCred(0x81), hsUint(7)}},
				),
				[][]byte{hsFullProposal()},
				[][]byte{hsGovActionId(0x97, 1), hsGovActionId(0x98, 0)},
				true,
			)),
			wantEnacted: 1,
			wantExpired: 2,
			wantWithdr:  1,
			wantDelayed: true,
			firstExpiry: 0x97,
			firstIdx:    1,
		},
		{
			name: "empty",
			reply: hsEraResult(hsRatifyState(
				hsEnactStateWith(hsNone, constitution, nil),
				nil,
				nil,
				false,
			)),
		},
		{
			name: "indefinite length above 23 entries",
			reply: hsEraResult(hsRatifyState(
				hsEnactStateWith(hsNone, constitution, withdrawals),
				many(24, func(i int) []byte { return hsSimpleProposal(byte(0xc0+i), i) }),
				many(24, func(i int) []byte { return hsGovActionId(byte(0xe0+i), i) }),
				false,
			)),
			wantEnacted: 24,
			wantExpired: 24,
			wantWithdr:  24,
			firstExpiry: 0xe0,
			firstIdx:    0,
		},
	}
	for _, era := range wireEras {
		for _, tc := range cases {
			t.Run(era.name+"/"+tc.name, func(t *testing.T) {
				runTest(
					t,
					eraQueryConversation(era.era, tc.reply),
					func(t *testing.T, oConn *ouroboros.Connection) {
						got, err := oConn.LocalStateQuery().Client.GetRatifyState()
						require.NoError(t, err)
						assert.Equal(
							t,
							"https://example.invalid/c",
							got.EnactState.Constitution.Anchor.Url,
						)
						assert.Equal(
							t,
							hash(0x5c, 28),
							got.EnactState.Constitution.ScriptHash,
						)
						assert.Equal(t, uint64(1000), got.EnactState.Treasury)
						assert.Len(t, got.EnactState.Withdrawals, tc.wantWithdr)
						assert.Len(t, got.Enacted, tc.wantEnacted)
						require.Len(t, got.Expired, tc.wantExpired)
						if tc.wantExpired > 0 {
							assert.Equal(
								t,
								tc.firstIdx,
								got.Expired[0].GovActionIdx,
							)
							assert.Equal(
								t,
								[32]byte(hash(tc.firstExpiry, 32)),
								got.Expired[0].TransactionId,
							)
						}
						assert.Equal(t, tc.wantDelayed, got.Delayed)
					},
				)
			})
		}
	}
}

func TestGetRatifyStateRejectsUnwrappedReply(t *testing.T) {
	// Not t.Parallel: runTest verifies goroutines with goleak.VerifyNone, a
	// process-wide check.
	reply := hsRatifyState(
		hsEnactStateWith(
			hsNone,
			hsConstitution("https://example.invalid/c", 0x71, nil),
			nil,
		),
		nil,
		nil,
		false,
	)
	runTest(
		t,
		eraQueryConversation(6, reply),
		func(t *testing.T, oConn *ouroboros.Connection) {
			_, err := oConn.LocalStateQuery().Client.GetRatifyState()
			require.Error(t, err)
		},
	)
}
