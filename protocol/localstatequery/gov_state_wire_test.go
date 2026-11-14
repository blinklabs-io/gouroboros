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
	"bytes"
	"math/big"
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fixtures below are written byte by byte from the cardano-ledger
// encoders, never through the Go types under test:
//
//	ConwayGovState        Conway/Governance.hs, EncCBOR ConwayGovState
//	Proposals             Conway/Governance/Proposals.hs, EncCBOR Proposals
//	GovRelation           Conway/Governance/Procedures.hs, EncCBOR GovRelation
//	GovActionState        Conway/Governance/Procedures.hs, EncCBOR GovActionState
//	Committee             Conway/Governance/Procedures.hs, EncCBOR Committee
//	Constitution          Conway/Governance/Procedures.hs, EncCBOR Constitution
//	FuturePParams         State/Governance.hs, EncCBOR FuturePParams
//	DRepPulsingState      Conway/Governance/DRepPulser.hs, EncCBOR DRepPulsingState
//	PulsingSnapshot       Conway/Governance/DRepPulser.hs, EncCBOR PulsingSnapshot
//	RatifyState           Conway/Governance/Internal.hs, EncCBOR RatifyState
//	EnactState            Conway/Governance/Internal.hs, EncCBOR EnactState
//	StrictMaybe / Maybe   Binary/Encoding/Encoder.hs, encodeStrictMaybe: [] or [x]
//	Map                   Binary/Encoding/Encoder.hs, encodeMap: keys in Ord
//	                      order, indefinite length above lengthThreshold (23)
//	Credential            Credential.hs: ScriptHashObj orders before KeyHashObj

func cat(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}

func head(major byte, n int) []byte {
	switch {
	case n < 24:
		return []byte{major<<5 | byte(n)}
	case n < 256:
		return []byte{major<<5 | 24, byte(n)}
	case n < 65536:
		return []byte{major<<5 | 25, byte(n >> 8), byte(n)}
	default:
		return []byte{
			major<<5 | 26,
			byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n),
		}
	}
}

func hsArray(items ...[]byte) []byte {
	return cat(append([][]byte{head(4, len(items))}, items...)...)
}

// hsList is the ledger's variableListLenEncoding.
func hsList(items [][]byte) []byte {
	if len(items) <= 23 {
		return hsArray(items...)
	}
	return cat([]byte{0x9f}, cat(items...), []byte{0xff})
}

// hsMap is the ledger's variableMapLenEncoding over entries already in the
// ledger's key order.
func hsMap(pairs ...[2][]byte) []byte {
	var body []byte
	for _, p := range pairs {
		body = cat(body, p[0], p[1])
	}
	if len(pairs) <= 23 {
		return cat(head(5, len(pairs)), body)
	}
	return cat([]byte{0xbf}, body, []byte{0xff})
}

func hsUint(n int) []byte { return head(0, n) }

func hsBytes(b []byte) []byte { return cat(head(2, len(b)), b) }

func hsText(s string) []byte { return cat(head(3, len(s)), []byte(s)) }

func hsSome(x []byte) []byte { return hsArray(x) }

var hsNone = []byte{0x80}

func hash(fill byte, n int) []byte {
	return bytes.Repeat([]byte{fill}, n)
}

func hsKeyCred(fill byte) []byte {
	return hsArray(hsUint(0), hsBytes(hash(fill, 28)))
}

func hsScriptCred(fill byte) []byte {
	return hsArray(hsUint(1), hsBytes(hash(fill, 28)))
}

func hsGovActionId(fill byte, ix int) []byte {
	return hsArray(hsBytes(hash(fill, 32)), hsUint(ix))
}

func hsAnchor(url string, fill byte) []byte {
	return hsArray(hsText(url), hsBytes(hash(fill, 32)))
}

// hsProposalProcedure is an InfoAction proposal:
// [deposit, reward account, [6], anchor].
func hsProposalProcedure(fill byte) []byte {
	return hsArray(
		[]byte{0x1a, 0x05, 0xf5, 0xe1, 0x00},
		hsBytes(append([]byte{0xe0}, hash(fill, 28)...)),
		hsArray(hsUint(6)),
		hsAnchor("https://example.invalid/p", fill),
	)
}

// hsGovActionState encodes a GovActionState whose committee and DRep vote
// maps are given already ordered.
func hsGovActionState(
	id, committeeVotes, drepVotes, spoVotes, procedure []byte,
	proposedIn, expiresAfter int,
) []byte {
	return hsArray(
		id,
		committeeVotes,
		drepVotes,
		spoVotes,
		procedure,
		hsUint(proposedIn),
		hsUint(expiresAfter),
	)
}

// hsMixedVotes is a Map (Credential) Vote holding both credential kinds,
// where the ledger's Ord (script before key) and the CBOR key byte order
// disagree: the key entries encode as [0, ..] and sort first as bytes.
func hsMixedVotes() []byte {
	return hsMap(
		[2][]byte{hsScriptCred(0x11), hsUint(1)},
		[2][]byte{hsScriptCred(0x22), hsUint(2)},
		[2][]byte{hsKeyCred(0x33), hsUint(0)},
	)
}

func hsSimpleProposal(fill byte, ix int) []byte {
	return hsGovActionState(
		hsGovActionId(fill, ix),
		hsMap(),
		hsMap(),
		hsMap(),
		hsProposalProcedure(fill),
		10,
		20,
	)
}

func hsCommittee() []byte {
	return hsArray(
		hsMap(
			[2][]byte{hsScriptCred(0x61), hsUint(500)},
			[2][]byte{hsKeyCred(0x62), hsUint(600)},
		),
		[]byte{0xd8, 0x1e, 0x82, 0x02, 0x03},
	)
}

var (
	hsPParams  = hsArray(hsUint(1), hsUint(2), hsUint(3))
	hsPrevPPar = hsArray(hsUint(4), hsUint(5), hsUint(6))
)

func hsRoots() []byte {
	return hsArray(
		hsNone,
		hsSome(hsGovActionId(0xa1, 0)),
		hsNone,
		hsSome(hsGovActionId(0xa2, 3)),
	)
}

func hsEnactState() []byte {
	return hsArray(
		hsSome(hsCommittee()),
		hsArray(hsAnchor("https://example.invalid/c", 0x71), []byte{0xf6}),
		hsPParams,
		hsPrevPPar,
		hsUint(1000),
		hsMap([2][]byte{hsKeyCred(0x81), hsUint(7)}),
		hsRoots(),
	)
}

func hsPulsingState(proposal []byte) []byte {
	snapshot := hsArray(
		hsList([][]byte{proposal}),
		hsMap(
			[2][]byte{hsArray(hsUint(0), hsBytes(hash(0x91, 28))), hsUint(5)},
			[2][]byte{hsArray(hsUint(1), hsBytes(hash(0x92, 28))), hsUint(6)},
			[2][]byte{hsArray(hsUint(2)), hsUint(7)},
		),
		hsMap([2][]byte{
			hsKeyCred(0x93),
			hsArray(
				hsUint(400),
				hsSome(hsAnchor("https://example.invalid/d", 0x94)),
				hsUint(2000000),
				cat([]byte{0xd9, 0x01, 0x02}, hsArray(hsKeyCred(0x95))),
			),
		}),
		hsMap([2][]byte{hsBytes(hash(0x96, 28)), hsUint(8)}),
	)
	ratify := hsArray(
		hsEnactState(),
		hsList([][]byte{proposal}),
		cat([]byte{0xd9, 0x01, 0x02}, hsArray(hsGovActionId(0x97, 1))),
		[]byte{0xf4},
	)
	return hsArray(snapshot, ratify)
}

// hsGovState is EncCBOR ConwayGovState: a seven-element array.
func hsGovState(
	proposals [][]byte,
	committee []byte,
	constitution []byte,
	future []byte,
) []byte {
	return hsArray(
		hsArray(hsRoots(), hsList(proposals)),
		committee,
		constitution,
		hsPParams,
		hsPrevPPar,
		future,
		hsPulsingState(hsSimpleProposal(0xb9, 2)),
	)
}

func hsFullProposal() []byte {
	return hsGovActionState(
		hsGovActionId(0xc1, 1),
		hsMixedVotes(),
		hsMap([2][]byte{hsKeyCred(0x44), hsUint(2)}),
		hsMap(
			[2][]byte{hsBytes(hash(0x55, 28)), hsUint(1)},
			[2][]byte{hsBytes(hash(0x56, 28)), hsUint(0)},
			[2][]byte{hsBytes(hash(0x57, 28)), hsUint(2)},
		),
		hsProposalProcedure(0xc1),
		100,
		200,
	)
}

func hsConstitutionNoScript() []byte {
	return hsArray(hsAnchor("https://example.invalid/constitution", 0xde), []byte{0xf6})
}

func TestGovStateResultDecodesLedgerLayout(t *testing.T) {
	t.Parallel()
	data := hsGovState(
		[][]byte{hsFullProposal(), hsSimpleProposal(0xc2, 0)},
		hsSome(hsCommittee()),
		hsConstitutionNoScript(),
		hsArray(hsUint(2), hsSome(hsPParams)),
	)
	var got localstatequery.GovStateResult
	_, err := cbor.Decode(data, &got)
	require.NoError(t, err)

	require.NotNil(t, got.ProposalRoots.HardFork)
	assert.Nil(t, got.ProposalRoots.ParameterChange)
	assert.Nil(t, got.ProposalRoots.Committee)
	require.NotNil(t, got.ProposalRoots.Constitution)
	assert.Equal(t, uint32(3), got.ProposalRoots.Constitution.GovActionIdx)
	assert.Equal(
		t,
		[32]byte(hash(0xa1, 32)),
		got.ProposalRoots.HardFork.TransactionId,
	)

	require.Len(t, got.Proposals, 2)
	p := got.Proposals[0]
	assert.Equal(t, [32]byte(hash(0xc1, 32)), p.Id.TransactionId)
	assert.Equal(t, uint32(1), p.Id.GovActionIdx)
	assert.Equal(t, uint64(100), p.ProposedIn)
	assert.Equal(t, uint64(200), p.ExpiresAfter)
	assert.Len(t, p.CommitteeVotes, 3)
	assert.Equal(
		t,
		lcommon.Vote(lcommon.GovVoteAbstain),
		p.CommitteeVotes[localstatequery.StakeCredential{
			Tag: 1, Bytes: ledger.Blake2b224(hash(0x22, 28)),
		}],
	)
	assert.Equal(t, hsProposalProcedure(0xc1), []byte(p.ProposalProcedure))
	assert.Equal(t, uint32(0), got.Proposals[1].Id.GovActionIdx)

	committee, err := got.DecodeCommittee()
	require.NoError(t, err)
	require.NotNil(t, committee)
	assert.Equal(
		t,
		uint64(500),
		committee.Members[localstatequery.StakeCredential{
			Tag: 1, Bytes: ledger.Blake2b224(hash(0x61, 28)),
		}],
	)
	assert.Equal(t, big.NewRat(2, 3), committee.Threshold.Rat)

	assert.Equal(
		t,
		"https://example.invalid/constitution",
		got.Constitution.Anchor.Url,
	)
	assert.Nil(t, got.Constitution.ScriptHash)
	assert.Equal(t, []byte(hsPParams), []byte(got.CurrentPParams))
	assert.Equal(t, []byte(hsPrevPPar), []byte(got.PrevPParams))

	future, err := got.DecodeFuturePParams()
	require.NoError(t, err)
	assert.Equal(t, localstatequery.FuturePParamsPotential, future.Kind)
	assert.Equal(t, []byte(hsPParams), []byte(future.PParams))

	pulsing, err := got.DecodeDRepPulsingState()
	require.NoError(t, err)
	require.Len(t, pulsing.Snapshot.Proposals, 1)
	assert.Equal(
		t,
		[32]byte(hash(0xb9, 32)),
		pulsing.Snapshot.Proposals[0].Id.TransactionId,
	)
	require.Len(t, pulsing.Snapshot.DRepDistr, 3)
	assert.Equal(t, uint64(5), pulsing.Snapshot.DRepDistr[0].Stake)
	assert.Equal(t, uint64(7), pulsing.Snapshot.DRepDistr[2].Stake)
	drep := pulsing.Snapshot.DRepState[localstatequery.StakeCredential{
		Tag: 0, Bytes: ledger.Blake2b224(hash(0x93, 28)),
	}]
	assert.Equal(t, uint64(400), drep.Expiry)
	assert.Equal(t, uint64(2000000), drep.Deposit)
	require.NotNil(t, drep.Anchor)
	require.Len(t, drep.Delegators, 1)
	assert.Equal(
		t,
		uint64(8),
		pulsing.Snapshot.PoolDistr[ledger.Blake2b224(hash(0x96, 28))],
	)
	assert.Equal(t, uint64(1000), pulsing.RatifyState.EnactState.Treasury)
	require.Len(t, pulsing.RatifyState.Enacted, 1)
	require.Len(t, pulsing.RatifyState.Expired, 1)
	assert.Equal(
		t,
		[32]byte(hash(0x97, 32)),
		pulsing.RatifyState.Expired[0].TransactionId,
	)
	assert.False(t, pulsing.RatifyState.Delayed)
}

func TestGovStateResultEncodesLedgerLayout(t *testing.T) {
	t.Parallel()
	// More than 23 proposals and more than 23 votes force the indefinite
	// length forms the ledger emits above lengthThreshold.
	manyProposals := make([][]byte, 0, 24)
	for i := range 24 {
		manyProposals = append(manyProposals, hsSimpleProposal(0xd0, i))
	}
	var manyVotes [][2][]byte
	for i := range 12 {
		manyVotes = append(
			manyVotes,
			[2][]byte{hsScriptCred(byte(0x10 + i)), hsUint(1)},
		)
	}
	for i := range 12 {
		manyVotes = append(
			manyVotes,
			[2][]byte{hsKeyCred(byte(0x10 + i)), hsUint(0)},
		)
	}
	var manyPoolVotes [][2][]byte
	for i := range 24 {
		manyPoolVotes = append(
			manyPoolVotes,
			[2][]byte{hsBytes(hash(byte(0x10+i), 28)), hsUint(i % 3)},
		)
	}
	bigVotes := hsGovActionState(
		hsGovActionId(0xe1, 0),
		hsMap(manyVotes...),
		hsMap(),
		hsMap(manyPoolVotes...),
		hsProposalProcedure(0xe1),
		1,
		2,
	)
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "mixed credentials, committee present",
			data: hsGovState(
				[][]byte{hsFullProposal()},
				hsSome(hsCommittee()),
				hsConstitutionNoScript(),
				hsArray(hsUint(0)),
			),
		},
		{
			name: "no committee, guardrails script",
			data: hsGovState(
				nil,
				hsNone,
				hsArray(
					hsAnchor("https://example.invalid/g", 0xaa),
					hsBytes(hash(0xbb, 28)),
				),
				hsArray(hsUint(1), hsPParams),
			),
		},
		{
			name: "indefinite proposals and votes",
			data: hsGovState(
				append(manyProposals, bigVotes),
				hsSome(hsCommittee()),
				hsConstitutionNoScript(),
				hsArray(hsUint(2), hsNone),
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got localstatequery.GovStateResult
			_, err := cbor.Decode(tt.data, &got)
			require.NoError(t, err)
			out, err := cbor.Encode(&got)
			require.NoError(t, err)
			assert.True(
				t,
				bytes.Equal(tt.data, out),
				"encoding differs from the ledger layout\nwant %x\ngot  %x",
				tt.data,
				out,
			)
		})
	}
}

func TestDecodeCommitteeStrictMaybe(t *testing.T) {
	t.Parallel()
	someCommittee := hsSome(hsCommittee())
	tests := []struct {
		name    string
		data    []byte
		wantNil bool
		wantErr bool
	}{
		{name: "SNothing is an empty list", data: hsNone, wantNil: true},
		{name: "SJust is a one-element list", data: someCommittee},
		{
			name:    "legacy tagged SNothing is not a ledger encoding",
			data:    hsArray(hsUint(0)),
			wantErr: true,
		},
		{
			name:    "legacy tagged SJust is not a ledger encoding",
			data:    hsArray(hsUint(1), hsCommittee()),
			wantErr: true,
		},
		{
			name:    "null is not a ledger encoding",
			data:    []byte{0xf6},
			wantErr: true,
		},
		{
			name:    "two committees",
			data:    hsArray(hsCommittee(), hsCommittee()),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := localstatequery.GovStateResult{Committee: tt.data}
			committee, err := g.DecodeCommittee()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, committee)
				return
			}
			require.NotNil(t, committee)
			assert.Len(t, committee.Members, 2)
		})
	}
}

func TestEncodeCommitteeMatchesLedgerLayout(t *testing.T) {
	t.Parallel()
	committee := &localstatequery.Committee{
		Members: map[localstatequery.StakeCredential]uint64{
			{Tag: 0, Bytes: ledger.Blake2b224(hash(0x62, 28))}: 600,
			{Tag: 1, Bytes: ledger.Blake2b224(hash(0x61, 28))}: 500,
		},
		Threshold: cbor.Rat{Rat: big.NewRat(2, 3)},
	}
	got, err := localstatequery.EncodeCommittee(committee)
	require.NoError(t, err)
	assert.Equal(t, hsSome(hsCommittee()), []byte(got))
	none, err := localstatequery.EncodeCommittee(nil)
	require.NoError(t, err)
	assert.Equal(t, hsNone, []byte(none))
}

func TestGetGovStateDecodesNodeReply(t *testing.T) {
	// Not t.Parallel: runTest verifies goroutines with goleak.VerifyNone, a
	// process-wide check.
	state := hsGovState(
		[][]byte{hsFullProposal()},
		hsSome(hsCommittee()),
		hsConstitutionNoScript(),
		hsArray(hsUint(0)),
	)
	// The hard fork combinator wraps every era query result in a
	// single-element array.
	reply := hsArray(state)
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
				localstatequery.NewMsgResult(reply),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			got, err := oConn.LocalStateQuery().Client.GetGovState()
			require.NoError(t, err)
			require.Len(t, got.Proposals, 1)
			assert.Equal(t, uint64(200), got.Proposals[0].ExpiresAfter)
			require.NotNil(t, got.ProposalRoots.HardFork)
			committee, err := got.DecodeCommittee()
			require.NoError(t, err)
			require.NotNil(t, committee)
		},
	)
}
