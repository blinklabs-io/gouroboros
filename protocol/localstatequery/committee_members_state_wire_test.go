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
	"encoding/hex"
	"math/big"
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixtures follow cardano-ledger's Cardano.Ledger.Api.State.Query.Governance
// encoders and the ouroboros-consensus hard fork combinator, whose
// encodeEitherMismatch writes a one-element list around the era result:
//
//	CommitteeMembersState  Rec: [Map ColdCred CommitteeMemberState,
//	                       Maybe UnitInterval, EpochNo]
//	CommitteeMemberState   Rec: [HotCredAuthStatus, MemberStatus,
//	                       Maybe EpochNo, NextEpochChange]
//	HotCredAuthStatus      Sum: [0, cred] authorized, [1] not authorized,
//	                       [2, Maybe Anchor] resigned
//	MemberStatus           encodeEnum: Active 0, Expired 1, Unrecognized 2
//	NextEpochChange        Sum: [0] to be enacted, [1] to be removed,
//	                       [2] no change, [3] to be expired,
//	                       [4, epoch] term adjusted
//	Maybe                  [] or [x]
//	UnitInterval           tag 30, [numerator, denominator]
//	Map                    keys in Ord order, ScriptHashObj before KeyHashObj

// committeeMembersStateGoldenHex is cardano-ledger's
// libs/cardano-ledger-api/golden/conway/cbor/queryCommitteeMembersState.cbor,
// byte for byte: a list of two results. The first has no members and no
// threshold; the second has five members covering every hot credential
// status, member status and next-epoch change.
const committeeMembersStateGoldenHex = "" +
	"8283a0800083a58201581cb16b56f5ec064be6ac3cab6035efae86b366cc3dc4" +
	"a0d571603d70e584820281827368747470733a2f2f6578616d706c652e636f6d" +
	"582003170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c11" +
	"1314028081008200581c58e1b65718531b42494610c506cef10ff031fa817a8f" +
	"f75c0ab180e78482008201581c4bb4efce5b9566c44aafce0437573d28d6b226" +
	"cbb15cf8ff3601293c008118fa81038200581c65fc709a5e019b8aba76f6977c" +
	"1c8770e4b36fa76f434efc588747b784820280008119012c820419015e820058" +
	"1ca646474b8f5431261506b6c273d307c7569a4eb6c96b42dd4a29520a848101" +
	"0181186481018200581ce0a714319812c3f773ba04ec5d6b3ffcd5aad8500680" +
	"5b047b0825418482008200581c92bf8969a49623af689a49861adf7639b25fd6" +
	"63c95733f147f44e71008118c8810281d81e8202031896"

func committeeMembersStateGolden(t *testing.T) []byte {
	t.Helper()
	out, err := hex.DecodeString(committeeMembersStateGoldenHex)
	require.NoError(t, err)
	return out
}

// goldenResults returns the two results of the golden file as separate
// encoded items.
func goldenResults(t *testing.T) [][]byte {
	t.Helper()
	var items []cbor.RawMessage
	_, err := cbor.Decode(committeeMembersStateGolden(t), &items)
	require.NoError(t, err)
	require.Len(t, items, 2)
	return [][]byte{items[0], items[1]}
}

func hsTagged30(num, den int) []byte {
	return cat([]byte{0xd8, 0x1e}, hsArray(hsUint(num), hsUint(den)))
}

func hsHotAuthorized(cred []byte) []byte { return hsArray(hsUint(0), cred) }

var hsHotNotAuthorized = hsArray(hsUint(1))

func hsHotResigned(anchor []byte) []byte {
	if anchor == nil {
		return hsArray(hsUint(2), hsNone)
	}
	return hsArray(hsUint(2), hsSome(anchor))
}

func hsNextEpoch(tag int) []byte { return hsArray(hsUint(tag)) }

func hsMember(hot []byte, status int, expiry []byte, next []byte) []byte {
	return hsArray(hot, hsUint(status), expiry, next)
}

func hsMembersState(members []byte, threshold []byte, epoch int) []byte {
	return hsArray(members, threshold, hsUint(epoch))
}

func mustHex(t *testing.T, s string) ledger.Blake2b224 {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	var out ledger.Blake2b224
	copy(out[:], b)
	return out
}

func TestCommitteeMembersStateDecodesGolden(t *testing.T) {
	t.Parallel()
	items := goldenResults(t)

	var empty localstatequery.CommitteeMembersStateResult
	_, err := cbor.Decode(items[0], &empty)
	require.NoError(t, err)
	assert.Empty(t, empty.Members)
	assert.Nil(t, empty.Threshold)
	assert.Equal(t, uint64(0), empty.Epoch)

	var full localstatequery.CommitteeMembersStateResult
	_, err = cbor.Decode(items[1], &full)
	require.NoError(t, err)
	require.NotNil(t, full.Threshold)
	assert.Equal(t, big.NewRat(2, 3), full.Threshold.Rat)
	assert.Equal(t, uint64(150), full.Epoch)
	require.Len(t, full.Members, 5)

	scriptKey := localstatequery.StakeCredential{
		Tag:   1,
		Bytes: mustHex(t, "b16b56f5ec064be6ac3cab6035efae86b366cc3dc4a0d571603d70e5"),
	}
	resignedWithAnchor, ok := full.Members[scriptKey]
	require.True(t, ok)
	assert.Equal(
		t,
		localstatequery.HotCredResigned,
		resignedWithAnchor.HotCredStatus.Status,
	)
	require.NotNil(t, resignedWithAnchor.HotCredStatus.Anchor)
	assert.Equal(
		t,
		"https://example.com",
		resignedWithAnchor.HotCredStatus.Anchor.Url,
	)
	assert.Equal(
		t,
		localstatequery.MemberStatusUnrecognized,
		resignedWithAnchor.Status,
	)
	assert.Nil(t, resignedWithAnchor.Expiry)
	assert.Equal(
		t,
		localstatequery.NextEpochToBeEnacted,
		resignedWithAnchor.NextEpochChange.Change,
	)

	authorizedScript := full.Members[localstatequery.StakeCredential{
		Tag:   0,
		Bytes: mustHex(t, "58e1b65718531b42494610c506cef10ff031fa817a8ff75c0ab180e7"),
	}]
	assert.Equal(
		t,
		localstatequery.HotCredAuthorized,
		authorizedScript.HotCredStatus.Status,
	)
	require.NotNil(t, authorizedScript.HotCredStatus.Credential)
	assert.Equal(
		t,
		uint(lcommon.CredentialTypeScriptHash),
		authorizedScript.HotCredStatus.Credential.CredType,
	)
	assert.Equal(t, localstatequery.MemberStatusActive, authorizedScript.Status)
	require.NotNil(t, authorizedScript.Expiry)
	assert.Equal(t, uint64(250), *authorizedScript.Expiry)
	assert.Equal(
		t,
		localstatequery.NextEpochToBeExpired,
		authorizedScript.NextEpochChange.Change,
	)

	resignedNoAnchor := full.Members[localstatequery.StakeCredential{
		Tag:   0,
		Bytes: mustHex(t, "65fc709a5e019b8aba76f6977c1c8770e4b36fa76f434efc588747b7"),
	}]
	assert.Equal(
		t,
		localstatequery.HotCredResigned,
		resignedNoAnchor.HotCredStatus.Status,
	)
	assert.Nil(t, resignedNoAnchor.HotCredStatus.Anchor)
	require.NotNil(t, resignedNoAnchor.Expiry)
	assert.Equal(t, uint64(300), *resignedNoAnchor.Expiry)
	assert.Equal(
		t,
		localstatequery.NextEpochTermAdjusted,
		resignedNoAnchor.NextEpochChange.Change,
	)
	require.NotNil(t, resignedNoAnchor.NextEpochChange.AdjustedEpoch)
	assert.Equal(t, uint64(350), *resignedNoAnchor.NextEpochChange.AdjustedEpoch)

	notAuthorized := full.Members[localstatequery.StakeCredential{
		Tag:   0,
		Bytes: mustHex(t, "a646474b8f5431261506b6c273d307c7569a4eb6c96b42dd4a29520a"),
	}]
	assert.Equal(
		t,
		localstatequery.HotCredNotAuthorized,
		notAuthorized.HotCredStatus.Status,
	)
	assert.Equal(t, localstatequery.MemberStatusExpired, notAuthorized.Status)
	assert.Equal(
		t,
		localstatequery.NextEpochToBeRemoved,
		notAuthorized.NextEpochChange.Change,
	)

	authorizedKey := full.Members[localstatequery.StakeCredential{
		Tag:   0,
		Bytes: mustHex(t, "e0a714319812c3f773ba04ec5d6b3ffcd5aad85006805b047b082541"),
	}]
	assert.Equal(
		t,
		localstatequery.HotCredAuthorized,
		authorizedKey.HotCredStatus.Status,
	)
	require.NotNil(t, authorizedKey.HotCredStatus.Credential)
	assert.Equal(
		t,
		uint(lcommon.CredentialTypeAddrKeyHash),
		authorizedKey.HotCredStatus.Credential.CredType,
	)
	assert.Equal(
		t,
		localstatequery.NextEpochNoChange,
		authorizedKey.NextEpochChange.Change,
	)
}

func TestCommitteeMembersStateReencodesGolden(t *testing.T) {
	t.Parallel()
	golden := committeeMembersStateGolden(t)
	var results []localstatequery.CommitteeMembersStateResult
	_, err := cbor.Decode(golden, &results)
	require.NoError(t, err)
	require.Len(t, results, 2)

	encoded := make([][]byte, len(results))
	for i, r := range results {
		encoded[i], err = cbor.Encode(r)
		require.NoError(t, err)
	}
	assert.Equal(
		t,
		hex.EncodeToString(golden),
		hex.EncodeToString(hsArray(encoded...)),
	)
}

func TestCommitteeMembersStateEncodesLargeMapIndefinite(t *testing.T) {
	t.Parallel()
	// The ledger writes a map above 23 entries with an indefinite-length
	// header, and orders keys script before key, then by hash.
	const count = 24
	members := map[localstatequery.StakeCredential]localstatequery.CommitteeMemberState{}
	var pairs [][2][]byte
	for i := count - 1; i >= 0; i-- {
		cred := localstatequery.StakeCredential{Tag: 0}
		cred.Bytes[0] = byte(i)
		members[cred] = localstatequery.CommitteeMemberState{
			HotCredStatus: localstatequery.HotCredAuthStatusValue{
				Status: localstatequery.HotCredNotAuthorized,
			},
			Status: localstatequery.MemberStatusActive,
			NextEpochChange: localstatequery.NextEpochChangeValue{
				Change: localstatequery.NextEpochNoChange,
			},
		}
	}
	for i := range count {
		key := hsArray(hsUint(0), hsBytes(cat([]byte{byte(i)}, hash(0, 27))))
		pairs = append(pairs, [2][]byte{
			key,
			hsMember(hsHotNotAuthorized, 0, hsNone, hsNextEpoch(2)),
		})
	}
	want := hsMembersState(hsMap(pairs...), hsNone, 9)
	got, err := cbor.Encode(localstatequery.CommitteeMembersStateResult{
		Members: members,
		Epoch:   9,
	})
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(want), hex.EncodeToString(got))
	require.Equal(t, byte(0xbf), got[1])
}

func TestCommitteeMembersStateRejectsMalformedReplies(t *testing.T) {
	t.Parallel()
	key := hsKeyCred(0x11)
	member := func(parts ...[]byte) []byte { return hsArray(parts...) }
	cases := []struct {
		name  string
		reply []byte
	}{
		{
			name: "threshold with two elements",
			reply: hsMembersState(
				hsMap(),
				hsArray(hsTagged30(1, 2), hsTagged30(1, 2)),
				1,
			),
		},
		{
			name: "unknown hot credential tag",
			reply: hsMembersState(
				hsMap([2][]byte{key, member(
					hsArray(hsUint(3)), hsUint(0), hsNone, hsNextEpoch(2),
				)}),
				hsNone,
				1,
			),
		},
		{
			name: "authorized without credential",
			reply: hsMembersState(
				hsMap([2][]byte{key, member(
					hsArray(hsUint(0)), hsUint(0), hsNone, hsNextEpoch(2),
				)}),
				hsNone,
				1,
			),
		},
		{
			name: "resigned anchor as a bare anchor",
			reply: hsMembersState(
				hsMap([2][]byte{key, member(
					hsArray(hsUint(2), hsAnchor("https://example.invalid", 1)),
					hsUint(0), hsNone, hsNextEpoch(2),
				)}),
				hsNone,
				1,
			),
		},
		{
			name: "unknown next epoch change",
			reply: hsMembersState(
				hsMap([2][]byte{key, member(
					hsHotNotAuthorized, hsUint(0), hsNone, hsNextEpoch(5),
				)}),
				hsNone,
				1,
			),
		},
		{
			name: "term adjusted without epoch",
			reply: hsMembersState(
				hsMap([2][]byte{key, member(
					hsHotNotAuthorized, hsUint(0), hsNone, hsNextEpoch(4),
				)}),
				hsNone,
				1,
			),
		},
		{
			name: "unknown member status",
			reply: hsMembersState(
				hsMap([2][]byte{key, member(
					hsHotNotAuthorized, hsUint(3), hsNone, hsNextEpoch(2),
				)}),
				hsNone,
				1,
			),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got localstatequery.CommitteeMembersStateResult
			_, err := cbor.Decode(tc.reply, &got)
			require.Error(t, err)
		})
	}
}

func TestGetCommitteeMembersStateDecodesNodeReply(t *testing.T) {
	// Not t.Parallel: runTest verifies goroutines with goleak.VerifyNone, a
	// process-wide check.
	items := goldenResults(t)
	for _, era := range wireEras {
		t.Run(era.name, func(t *testing.T) {
			runTest(
				t,
				eraQueryConversation(era.era, hsEraResult(items[1])),
				func(t *testing.T, oConn *ouroboros.Connection) {
					got, err := oConn.LocalStateQuery().
						Client.GetCommitteeMembersState(nil, nil, nil)
					require.NoError(t, err)
					require.Len(t, got.Members, 5)
					require.NotNil(t, got.Threshold)
					assert.Equal(t, big.NewRat(2, 3), got.Threshold.Rat)
					assert.Equal(t, uint64(150), got.Epoch)
				},
			)
		})
	}
}

func TestGetCommitteeMembersStateNoThreshold(t *testing.T) {
	// Not t.Parallel: runTest verifies goroutines with goleak.VerifyNone, a
	// process-wide check.
	items := goldenResults(t)
	runTest(
		t,
		eraQueryConversation(6, hsEraResult(items[0])),
		func(t *testing.T, oConn *ouroboros.Connection) {
			got, err := oConn.LocalStateQuery().
				Client.GetCommitteeMembersState(nil, nil, nil)
			require.NoError(t, err)
			assert.Empty(t, got.Members)
			assert.Nil(t, got.Threshold)
		},
	)
}

func TestGetCommitteeMembersStateRejectsUnwrappedReply(t *testing.T) {
	// Not t.Parallel: runTest verifies goroutines with goleak.VerifyNone, a
	// process-wide check.
	items := goldenResults(t)
	runTest(
		t,
		eraQueryConversation(6, items[1]),
		func(t *testing.T, oConn *ouroboros.Connection) {
			_, err := oConn.LocalStateQuery().
				Client.GetCommitteeMembersState(nil, nil, nil)
			require.Error(t, err)
		},
	)
}
