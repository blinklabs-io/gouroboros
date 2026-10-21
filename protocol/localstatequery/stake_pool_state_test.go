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

package localstatequery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stakePoolStateConwayGolden is the ledger's golden file for the
// GetStakePoolState reply in Conway, byte for byte, from cardano-ledger at
// commit 9df9f98cce86c5f2195142b2c02dd9b39dd9dc7c:
// libs/cardano-ledger-api/golden/conway/cbor/queryStakePoolState.cbor.
//
// It is a three-element array of replies: no pools, two pools with their
// delegators, and the same two pools without delegators. The node's
// single-element era result wrapper is not part of the file; the tests add it.
const stakePoolStateConwayGolden = "" +
	"83a0a2581c0d6a577e9441ad8ed9663931906e4d43ece8f82c712b1d0235affb" +
	"068e5820c5e21ab1c9f6022d81c3b25e3436cb7f1df77f9652ae3e1310c28e62" +
	"1dd87b4cf61b000000174876e8001a0a21fe80d81e82010a8200581c0d6a577e" +
	"9441ad8ed9663931906e4d43ece8f82c712b1d0235affb06d9010281581c0d6a" +
	"577e9441ad8ed9663931906e4d43ece8f82c712b1d0235affb06838400190bb8" +
	"447f00000150b80d01200000000000000000010000008301190bb97172656c61" +
	"792e6578616d706c652e636f6d82027172656c61792e6578616d706c652e636f" +
	"6d826e636f6e73656e7375732e706f6f6c427b7d1a1dcd650002d90102828201" +
	"581c92bf8969a49623af689a49861adf7639b25fd663c95733f147f44e718200" +
	"581c6d8143487c22dc58eceb5b53f9876e008f052212542d25a0ae8b859b18fa" +
	"89581c0d6a577e9441ad8ed9663931906e4d43ece8f82c712b1d0235affb0658" +
	"20c5e21ab1c9f6022d81c3b25e3436cb7f1df77f9652ae3e1310c28e621dd87b" +
	"4c1b00000022ecb25c001a0a21fe80d81e82010a581de00d6a577e9441ad8ed9" +
	"663931906e4d43ece8f82c712b1d0235affb06d9010281581c0d6a577e9441ad" +
	"8ed9663931906e4d43ece8f82c712b1d0235affb06838400190bb8447f000001" +
	"50b80d01200000000000000000010000008301190bb97172656c61792e657861" +
	"6d706c652e636f6d82027172656c61792e6578616d706c652e636f6d826e636f" +
	"6e73656e7375732e706f6f6c427b7d581cb77908a84072eb9151782d64504f0f" +
	"6975b7143d49514eaeb0991d008e5820e07a156ccb70a41eef8339001bd6a0e0" +
	"f773e0ee33ce94fb9222df1874d6fdddf61b0000000ba43b74001a0a21fe80d8" +
	"1e820118648200581c3113f778bfe62098fb783f7defac9b1f5f5d054e267005" +
	"f630b85acbd9010281581c3113f778bfe62098fb783f7defac9b1f5f5d054e26" +
	"7005f630b85acb80f61a1dcd650003d90102838201581c0fb2c57096165367d4" +
	"9491db222d5fc66d38f59be2c1cb467f002ac28200581c1a7d5e232ad01edfcd" +
	"4d8d651b616364c11cdc041bb5b92a4a01575e8200581c1cd2b7f71d400aaabd" +
	"4f85803ae5ff6f77c1a978586fda00e174216cf6f6a2581c0d6a577e9441ad8e" +
	"d9663931906e4d43ece8f82c712b1d0235affb068e5820c5e21ab1c9f6022d81" +
	"c3b25e3436cb7f1df77f9652ae3e1310c28e621dd87b4cf61b000000174876e8" +
	"001a0a21fe80d81e82010a8200581c0d6a577e9441ad8ed9663931906e4d43ec" +
	"e8f82c712b1d0235affb06d9010281581c0d6a577e9441ad8ed9663931906e4d" +
	"43ece8f82c712b1d0235affb06838400190bb8447f00000150b80d0120000000" +
	"0000000000010000008301190bb97172656c61792e6578616d706c652e636f6d" +
	"82027172656c61792e6578616d706c652e636f6d826e636f6e73656e7375732e" +
	"706f6f6c427b7d1a1dcd650002f618fa89581c0d6a577e9441ad8ed966393190" +
	"6e4d43ece8f82c712b1d0235affb065820c5e21ab1c9f6022d81c3b25e3436cb" +
	"7f1df77f9652ae3e1310c28e621dd87b4c1b00000022ecb25c001a0a21fe80d8" +
	"1e82010a581de00d6a577e9441ad8ed9663931906e4d43ece8f82c712b1d0235" +
	"affb06d9010281581c0d6a577e9441ad8ed9663931906e4d43ece8f82c712b1d" +
	"0235affb06838400190bb8447f00000150b80d01200000000000000000010000" +
	"008301190bb97172656c61792e6578616d706c652e636f6d82027172656c6179" +
	"2e6578616d706c652e636f6d826e636f6e73656e7375732e706f6f6c427b7d58" +
	"1cb77908a84072eb9151782d64504f0f6975b7143d49514eaeb0991d008e5820" +
	"e07a156ccb70a41eef8339001bd6a0e0f773e0ee33ce94fb9222df1874d6fddd" +
	"f61b0000000ba43b74001a0a21fe80d81e820118648200581c3113f778bfe620" +
	"98fb783f7defac9b1f5f5d054e267005f630b85acbd9010281581c3113f778bf" +
	"e62098fb783f7defac9b1f5f5d054e267005f630b85acb80f61a1dcd650003f6" +
	"f6f6"

// stakePoolStateDijkstraGolden is the ledger's golden file for the
// GetStakePoolState reply in Dijkstra, byte for byte, from cardano-ledger at
// commit 9df9f98cce86c5f2195142b2c02dd9b39dd9dc7c:
// libs/cardano-ledger-api/golden/dijkstra/cbor/queryStakePoolState.cbor.
//
// It has the shape of the Conway file, with a BLS key on the first pool and in
// its pending registration.
const stakePoolStateDijkstraGolden = "" +
	"83a0a2581c0d6a577e9441ad8ed9663931906e4d43ece8f82c712b1d0235affb" +
	"068e5820c5e21ab1c9f6022d81c3b25e3436cb7f1df77f9652ae3e1310c28e62" +
	"1dd87b4c82825860a57565542eaa01ef2b910bf0eaba4d98a1e5b8b79cc425db" +
	"08f8780732d0ea9bc85fc6175f272b2344bb27bc572ebf14022e52689dcedfcc" +
	"f44a00e5bd1aa59db44517217d6b0f21b372169ee761938c28914ddcb9663de5" +
	"4db288e760a8e14f5830b408288be1abfd4c34ce9d275b4cc1520aa600158faa" +
	"4791a74ff474a79223b359193aec1bb3fb2a1aadb3b054842aac18c81b000000" +
	"174876e8001a0a21fe80d81e82010a8200581c0d6a577e9441ad8ed966393190" +
	"6e4d43ece8f82c712b1d0235affb06d9010281581c0d6a577e9441ad8ed96639" +
	"31906e4d43ece8f82c712b1d0235affb06838400190bb8447f00000150b80d01" +
	"200000000000000000010000008301190bb97172656c61792e6578616d706c65" +
	"2e636f6d82027172656c61792e6578616d706c652e636f6d826e636f6e73656e" +
	"7375732e706f6f6c427b7d1a1dcd650002d90102828201581c92bf8969a49623" +
	"af689a49861adf7639b25fd663c95733f147f44e718200581c6d8143487c22dc" +
	"58eceb5b53f9876e008f052212542d25a0ae8b859b18fa8a581c0d6a577e9441" +
	"ad8ed9663931906e4d43ece8f82c712b1d0235affb065820c5e21ab1c9f6022d" +
	"81c3b25e3436cb7f1df77f9652ae3e1310c28e621dd87b4c825860a57565542e" +
	"aa01ef2b910bf0eaba4d98a1e5b8b79cc425db08f8780732d0ea9bc85fc6175f" +
	"272b2344bb27bc572ebf14022e52689dcedfccf44a00e5bd1aa59db44517217d" +
	"6b0f21b372169ee761938c28914ddcb9663de54db288e760a8e14f5830b40828" +
	"8be1abfd4c34ce9d275b4cc1520aa600158faa4791a74ff474a79223b359193a" +
	"ec1bb3fb2a1aadb3b054842aac1b00000022ecb25c001a0a21fe80d81e82010a" +
	"581de00d6a577e9441ad8ed9663931906e4d43ece8f82c712b1d0235affb06d9" +
	"010281581c0d6a577e9441ad8ed9663931906e4d43ece8f82c712b1d0235affb" +
	"06838400190bb8447f00000150b80d0120000000000000000001000000830119" +
	"0bb97172656c61792e6578616d706c652e636f6d82027172656c61792e657861" +
	"6d706c652e636f6d826e636f6e73656e7375732e706f6f6c427b7d581cb77908" +
	"a84072eb9151782d64504f0f6975b7143d49514eaeb0991d008e5820e07a156c" +
	"cb70a41eef8339001bd6a0e0f773e0ee33ce94fb9222df1874d6fdddf61b0000" +
	"000ba43b74001a0a21fe80d81e820118648200581c3113f778bfe62098fb783f" +
	"7defac9b1f5f5d054e267005f630b85acbd9010281581c3113f778bfe62098fb" +
	"783f7defac9b1f5f5d054e267005f630b85acb80f61a1dcd650003d901028382" +
	"01581c0fb2c57096165367d49491db222d5fc66d38f59be2c1cb467f002ac282" +
	"00581c1a7d5e232ad01edfcd4d8d651b616364c11cdc041bb5b92a4a01575e82" +
	"00581c1cd2b7f71d400aaabd4f85803ae5ff6f77c1a978586fda00e174216cf6" +
	"f6a2581c0d6a577e9441ad8ed9663931906e4d43ece8f82c712b1d0235affb06" +
	"8e5820c5e21ab1c9f6022d81c3b25e3436cb7f1df77f9652ae3e1310c28e621d" +
	"d87b4c82825860a57565542eaa01ef2b910bf0eaba4d98a1e5b8b79cc425db08" +
	"f8780732d0ea9bc85fc6175f272b2344bb27bc572ebf14022e52689dcedfccf4" +
	"4a00e5bd1aa59db44517217d6b0f21b372169ee761938c28914ddcb9663de54d" +
	"b288e760a8e14f5830b408288be1abfd4c34ce9d275b4cc1520aa600158faa47" +
	"91a74ff474a79223b359193aec1bb3fb2a1aadb3b054842aac18c81b00000017" +
	"4876e8001a0a21fe80d81e82010a8200581c0d6a577e9441ad8ed9663931906e" +
	"4d43ece8f82c712b1d0235affb06d9010281581c0d6a577e9441ad8ed9663931" +
	"906e4d43ece8f82c712b1d0235affb06838400190bb8447f00000150b80d0120" +
	"0000000000000000010000008301190bb97172656c61792e6578616d706c652e" +
	"636f6d82027172656c61792e6578616d706c652e636f6d826e636f6e73656e73" +
	"75732e706f6f6c427b7d1a1dcd650002f618fa8a581c0d6a577e9441ad8ed966" +
	"3931906e4d43ece8f82c712b1d0235affb065820c5e21ab1c9f6022d81c3b25e" +
	"3436cb7f1df77f9652ae3e1310c28e621dd87b4c825860a57565542eaa01ef2b" +
	"910bf0eaba4d98a1e5b8b79cc425db08f8780732d0ea9bc85fc6175f272b2344" +
	"bb27bc572ebf14022e52689dcedfccf44a00e5bd1aa59db44517217d6b0f21b3" +
	"72169ee761938c28914ddcb9663de54db288e760a8e14f5830b408288be1abfd" +
	"4c34ce9d275b4cc1520aa600158faa4791a74ff474a79223b359193aec1bb3fb" +
	"2a1aadb3b054842aac1b00000022ecb25c001a0a21fe80d81e82010a581de00d" +
	"6a577e9441ad8ed9663931906e4d43ece8f82c712b1d0235affb06d901028158" +
	"1c0d6a577e9441ad8ed9663931906e4d43ece8f82c712b1d0235affb06838400" +
	"190bb8447f00000150b80d01200000000000000000010000008301190bb97172" +
	"656c61792e6578616d706c652e636f6d82027172656c61792e6578616d706c65" +
	"2e636f6d826e636f6e73656e7375732e706f6f6c427b7d581cb77908a84072eb" +
	"9151782d64504f0f6975b7143d49514eaeb0991d008e5820e07a156ccb70a41e" +
	"ef8339001bd6a0e0f773e0ee33ce94fb9222df1874d6fdddf61b0000000ba43b" +
	"74001a0a21fe80d81e820118648200581c3113f778bfe62098fb783f7defac9b" +
	"1f5f5d054e267005f630b85acbd9010281581c3113f778bfe62098fb783f7def" +
	"ac9b1f5f5d054e267005f630b85acb80f61a1dcd650003f6f6f6"

// TestStakePoolStateQueryEncoding pins the request bytes of GetStakePoolState,
// as encodeShelleyQuery writes them: the pool filter is a StrictMaybe, so an
// empty list asks about every pool.
func TestStakePoolStateQueryEncoding(t *testing.T) {
	poolId := ledger.PoolId(ledger.NewBlake2b224(spacedHex(t, testPoolIdHex)))
	cases := []struct {
		name  string
		query any
		want  string
	}{
		{
			name: "all_pools",
			query: &ShelleyStakePoolStateQuery{
				Type:  QueryTypeShelleyStakePoolState,
				Pools: []cbor.SetType[ledger.PoolId]{},
			},
			want: "83 18 28 80 f4",
		},
		{
			name: "one_pool_with_delegators",
			query: &ShelleyStakePoolStateQuery{
				Type: QueryTypeShelleyStakePoolState,
				Pools: []cbor.SetType[ledger.PoolId]{
					cbor.NewSetType([]ledger.PoolId{poolId}, true),
				},
				IncludeDelegators: true,
			},
			want: "83 18 28 81 d9 0102 81 58 1c " + testPoolIdHex + " f5",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := cbor.Encode(tc.query)
			require.NoError(t, err)
			assert.Equal(
				t,
				strings.ReplaceAll(tc.want, " ", ""),
				hex.EncodeToString(data),
			)
			decoded, err := decodeShelleyQuery(data)
			require.NoError(t, err)
			q, ok := decoded.(*ShelleyStakePoolStateQuery)
			require.True(t, ok)
			assert.Equal(t, QueryTypeShelleyStakePoolState, q.Type)
			assert.Equal(t, tc.name == "one_pool_with_delegators", q.IncludeDelegators)
		})
	}
}

// TestStakePoolStateQueryDecodesPoolFilter covers the pool filter of the
// decoded query, which decides whether the reply covers every pool.
func TestStakePoolStateQueryDecodesPoolFilter(t *testing.T) {
	decoded, err := decodeShelleyQuery(spacedHex(t, "83 18 28 80 f5"))
	require.NoError(t, err)
	q, ok := decoded.(*ShelleyStakePoolStateQuery)
	require.True(t, ok)
	pools, all := q.PoolFilter()
	assert.True(t, all)
	assert.Empty(t, pools)
	assert.True(t, q.IncludeDelegators)

	decoded, err = decodeShelleyQuery(
		spacedHex(t, "83 18 28 81 d9 0102 81 58 1c "+testPoolIdHex+" f4"),
	)
	require.NoError(t, err)
	q, ok = decoded.(*ShelleyStakePoolStateQuery)
	require.True(t, ok)
	pools, all = q.PoolFilter()
	assert.False(t, all)
	require.Len(t, pools, 1)
	assert.Equal(
		t,
		ledger.PoolId(ledger.NewBlake2b224(spacedHex(t, testPoolIdHex))),
		pools[0],
	)
	assert.False(t, q.IncludeDelegators)
}

// TestStakePoolStateQueryPoolSetCanonicity covers the pool set of a decoded
// query. The node decodes it with decodeSetSkel, which refuses anything but a
// tag-258, definite-length array in strictly ascending order of the encoded
// members, so the decoder refuses the same.
func TestStakePoolStateQueryPoolSetCanonicity(t *testing.T) {
	t.Parallel()
	member := func(b byte) string {
		return "581c" + strings.Repeat(fmt.Sprintf("%02x", b), 28)
	}
	query := func(set string) string { return "83182881" + set + "f4" }
	cases := []struct {
		name    string
		set     string
		wantErr string
	}{
		{name: "ascending", set: "d9010282" + member(1) + member(2)},
		{name: "empty_set", set: "d9010280"},
		{
			name:    "untagged",
			set:     "82" + member(1) + member(2),
			wantErr: "tag 258",
		},
		{
			name:    "indefinite_length",
			set:     "d901029f" + member(1) + member(2) + "ff",
			wantErr: "definite",
		},
		{
			name:    "wrong_tag",
			set:     "d9010382" + member(1) + member(2),
			wantErr: "tag 258",
		},
		{
			name:    "duplicates",
			set:     "d9010282" + member(1) + member(1),
			wantErr: "duplicate",
		},
		{
			name:    "descending",
			set:     "d9010282" + member(2) + member(1),
			wantErr: "ascending",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decoded, err := decodeShelleyQuery(spacedHex(t, query(tc.set)))
			if tc.wantErr == "" {
				require.NoError(t, err)
				q, ok := decoded.(*ShelleyStakePoolStateQuery)
				require.True(t, ok)
				_, all := q.PoolFilter()
				assert.False(t, all)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// TestStakePoolStateQueryEncodesCanonicalPoolSet checks that a query built in
// Go writes its pool set the way the node and this package's decoder require,
// whatever the order and number of the pools it was given.
func TestStakePoolStateQueryEncodesCanonicalPoolSet(t *testing.T) {
	t.Parallel()
	// More than 23 pools, in descending order and with a duplicate.
	pools := make([]ledger.PoolId, 0, 31)
	for b := byte(30); b >= 1; b-- {
		pools = append(pools, ledger.PoolId(ledger.NewBlake2b224(
			[]byte(strings.Repeat(string([]byte{b}), 28)),
		)))
	}
	pools = append(pools, pools[0])
	query := &ShelleyStakePoolStateQuery{
		Type: QueryTypeShelleyStakePoolState,
		Pools: []cbor.SetType[ledger.PoolId]{
			cbor.NewSetType(pools, false),
		},
	}
	data, err := cbor.Encode(query)
	require.NoError(t, err)
	poolSet, err := encodeStakePoolIdSet(pools)
	require.NoError(t, err)
	assert.Equal(
		t,
		"83182881"+hex.EncodeToString(poolSet)+"f4",
		hex.EncodeToString(data),
	)
	decoded, err := decodeShelleyQuery(data)
	require.NoError(t, err)
	q, ok := decoded.(*ShelleyStakePoolStateQuery)
	require.True(t, ok)
	got, all := q.PoolFilter()
	assert.False(t, all)
	assert.Len(t, got, 30)

	// An explicit filter of the empty set is not the absent filter.
	data, err = cbor.Encode(&ShelleyStakePoolStateQuery{
		Type: QueryTypeShelleyStakePoolState,
		Pools: []cbor.SetType[ledger.PoolId]{
			cbor.NewSetType([]ledger.PoolId{}, false),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "83182881d9010280f4", hex.EncodeToString(data))

	// The filter is a StrictMaybe, so it holds one set at most.
	_, err = cbor.Encode(&ShelleyStakePoolStateQuery{
		Type: QueryTypeShelleyStakePoolState,
		Pools: []cbor.SetType[ledger.PoolId]{
			cbor.NewSetType(pools[:1], true),
			cbor.NewSetType(pools[1:2], true),
		},
	})
	require.Error(t, err)
}

const (
	goldenPoolA = "0d6a577e9441ad8ed9663931906e4d43ece8f82c712b1d0235affb06"
	goldenPoolB = "b77908a84072eb9151782d64504f0f6975b7143d49514eaeb0991d00"
)

func goldenPoolId(t *testing.T, id string) ledger.PoolId {
	t.Helper()
	return ledger.PoolId(ledger.NewBlake2b224(spacedHex(t, id)))
}

// ledgerGoldenReplies splits a ledger golden file into its replies, each in
// the single-element era result wrapper the node puts around the reply.
func ledgerGoldenReplies(t *testing.T, golden string) [][]byte {
	t.Helper()
	var maps []cbor.RawMessage
	_, err := cbor.Decode(spacedHex(t, golden), &maps)
	require.NoError(t, err)
	replies := make([][]byte, 0, len(maps))
	for _, m := range maps {
		replies = append(replies, append([]byte{0x81}, m...))
	}
	return replies
}

func decodeGoldenReply(t *testing.T, reply []byte) StakePoolStateResult {
	t.Helper()
	var result StakePoolStateResult
	_, err := cbor.Decode(reply, &result)
	require.NoError(t, err)
	return result
}

// TestStakePoolStateGoldenFilesAreVerbatim guards the fixtures: they are the
// ledger's files, not this package's idea of them.
func TestStakePoolStateGoldenFilesAreVerbatim(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ golden, digest string }{
		"conway": {
			stakePoolStateConwayGolden,
			"2a218f2927f25a7e2f939d7f959542f346b432fba5d636ec4471775d84d932cd",
		},
		"dijkstra": {
			stakePoolStateDijkstraGolden,
			"09794fa5860071b0a4d73fccdcaca79b7784c3b46f4ea08a55f701f087f576cd",
		},
	} {
		sum := sha256.Sum256(spacedHex(t, tc.golden))
		assert.Equal(t, tc.digest, hex.EncodeToString(sum[:]), name)
	}
}

// TestStakePoolStateResultDecodesConwayGolden decodes all three replies of the
// ledger's Conway golden file, so the shape of every field is the ledger's
// rather than this package's idea of it.
func TestStakePoolStateResultDecodesConwayGolden(t *testing.T) {
	t.Parallel()
	replies := ledgerGoldenReplies(t, stakePoolStateConwayGolden)
	require.Len(t, replies, 3)
	poolA := goldenPoolId(t, goldenPoolA)
	poolB := goldenPoolId(t, goldenPoolB)

	// No pools.
	assert.Empty(t, decodeGoldenReply(t, replies[0]).Results)

	for idx, withDelegators := range []bool{true, false} {
		result := decodeGoldenReply(t, replies[idx+1])
		require.Len(t, result.Results, 2)

		entry, ok := result.Results[poolA]
		require.True(t, ok, "pool %x missing from the reply", poolA)
		assert.Equal(
			t,
			"c5e21ab1c9f6022d81c3b25e3436cb7f1df77f9652ae3e1310c28e621dd87b4c",
			hex.EncodeToString(entry.Vrf[:]),
		)
		assert.Nil(t, entry.BlsKey)
		assert.Equal(t, uint64(100000000000), entry.Pledge)
		assert.Equal(t, uint64(170000000), entry.Cost)
		require.NotNil(t, entry.Margin)
		assert.Equal(t, "1/10", entry.Margin.Rat.String())
		assert.Equal(
			t,
			uint(lcommon.CredentialTypeAddrKeyHash),
			entry.AccountId.CredType,
		)
		assert.Equal(
			t,
			goldenPoolA,
			hex.EncodeToString(entry.AccountId.Credential[:]),
		)
		require.Len(t, entry.Owners.Items(), 1)
		require.Len(t, entry.Relays, 3)
		assert.Equal(t, 0, entry.Relays[0].Type)
		require.NotNil(t, entry.Metadata)
		assert.Equal(t, "consensus.pool", entry.Metadata.Url)
		assert.Equal(t, uint64(500000000), entry.Deposit)
		assert.Equal(t, 2, entry.NumDelegators)
		require.NotNil(t, entry.Retiring)
		assert.Equal(t, uint64(250), *entry.Retiring)
		if withDelegators {
			// The ledger orders a script credential before a key credential
			// (the derived Ord of Credential puts ScriptHashObj first).
			require.NotNil(t, entry.Delegators)
			items := entry.Delegators.Items()
			require.Len(t, items, 2)
			assert.Equal(
				t,
				uint(lcommon.CredentialTypeScriptHash),
				items[0].CredType,
			)
			assert.Equal(
				t,
				uint(lcommon.CredentialTypeAddrKeyHash),
				items[1].CredType,
			)
		} else {
			assert.Nil(t, entry.Delegators)
		}

		// The pending registration is a flat record of its own.
		require.NotNil(t, entry.FutureStakePoolParams)
		params := entry.FutureStakePoolParams
		assert.Equal(t, poolA, params.Id)
		assert.Equal(t, uint64(150000000000), params.Pledge)
		assert.Equal(t, uint64(170000000), params.Cost)
		require.NotNil(t, params.Margin)
		assert.Equal(t, "1/10", params.Margin.Rat.String())
		assert.Equal(t, 29, len(params.AccountAddress))
		require.Len(t, params.Owners.Items(), 1)
		require.Len(t, params.Relays, 3)
		require.NotNil(t, params.Metadata)
		assert.Nil(t, params.BlsKey)

		// The second pool has no metadata, retirement or pending
		// registration, and three delegators.
		other, ok := result.Results[poolB]
		require.True(t, ok, "pool %x missing from the reply", poolB)
		assert.Nil(t, other.BlsKey)
		assert.Equal(t, uint64(50000000000), other.Pledge)
		require.NotNil(t, other.Margin)
		assert.Equal(t, "1/100", other.Margin.Rat.String())
		assert.Empty(t, other.Relays)
		assert.Nil(t, other.Metadata)
		assert.Nil(t, other.Retiring)
		assert.Nil(t, other.FutureStakePoolParams)
		assert.Equal(t, 3, other.NumDelegators)
		if withDelegators {
			require.NotNil(t, other.Delegators)
			items := other.Delegators.Items()
			require.Len(t, items, 3)
			assert.Equal(
				t,
				uint(lcommon.CredentialTypeScriptHash),
				items[0].CredType,
			)
			assert.Equal(
				t,
				uint(lcommon.CredentialTypeAddrKeyHash),
				items[1].CredType,
			)
			assert.Equal(
				t,
				uint(lcommon.CredentialTypeAddrKeyHash),
				items[2].CredType,
			)
		} else {
			// A null is not an empty set.
			assert.Nil(t, other.Delegators)
		}
	}
}

// TestStakePoolStateResultDecodesDijkstraGolden decodes the ledger's Dijkstra
// golden file, whose first pool carries a BLS key, both in its state and in
// its pending registration (a ten-field record).
func TestStakePoolStateResultDecodesDijkstraGolden(t *testing.T) {
	t.Parallel()
	replies := ledgerGoldenReplies(t, stakePoolStateDijkstraGolden)
	require.Len(t, replies, 3)
	poolA := goldenPoolId(t, goldenPoolA)
	poolB := goldenPoolId(t, goldenPoolB)

	assert.Empty(t, decodeGoldenReply(t, replies[0]).Results)

	for idx, withDelegators := range []bool{true, false} {
		result := decodeGoldenReply(t, replies[idx+1])
		require.Len(t, result.Results, 2)

		entry, ok := result.Results[poolA]
		require.True(t, ok, "pool %x missing from the reply", poolA)
		require.NotNil(t, entry.BlsKey)
		assert.Len(t, entry.BlsKey.Key.PublicKey, 96)
		assert.Len(t, entry.BlsKey.Key.PossessionProof, 48)
		assert.Equal(t, uint64(200), entry.BlsKey.RegisteredIn)
		assert.Equal(t, uint64(100000000000), entry.Pledge)
		assert.Equal(t, 2, entry.NumDelegators)
		assert.Equal(t, withDelegators, entry.Delegators != nil)
		require.NotNil(t, entry.Retiring)
		assert.Equal(t, uint64(250), *entry.Retiring)

		require.NotNil(t, entry.FutureStakePoolParams)
		params := entry.FutureStakePoolParams
		assert.Equal(t, poolA, params.Id)
		require.NotNil(t, params.BlsKey)
		assert.Equal(t, entry.BlsKey.Key, *params.BlsKey)
		assert.Equal(t, uint64(150000000000), params.Pledge)
		assert.Equal(t, uint64(170000000), params.Cost)
		require.Len(t, params.Relays, 3)
		require.NotNil(t, params.Metadata)

		other, ok := result.Results[poolB]
		require.True(t, ok, "pool %x missing from the reply", poolB)
		assert.Nil(t, other.BlsKey)
		assert.Nil(t, other.FutureStakePoolParams)
		assert.Equal(t, 3, other.NumDelegators)
	}

	// The pending registration really is the ten-field record.
	var wrapper []cbor.RawMessage
	_, err := cbor.Decode(replies[1], &wrapper)
	require.NoError(t, err)
	if len(wrapper) != 1 {
		t.Fatalf("reply wrapper: got %d elements, want 1", len(wrapper))
	}
	var entries map[ledger.PoolId]cbor.RawMessage
	_, err = cbor.Decode(wrapper[0], &entries)
	require.NoError(t, err)
	var fields []cbor.RawMessage
	_, err = cbor.Decode(entries[poolA], &fields)
	require.NoError(t, err)
	require.Len(t, fields, stakePoolStateFieldCount)
	paramsLen, err := cbor.ListLength(fields[stakePoolStateFieldCount-1])
	require.NoError(t, err)
	assert.Equal(t, stakePoolParamsFieldCount+1, paramsLen)
}

// TestStakePoolStateResultRoundTripsGolden checks that every reply decoded
// from a ledger golden file is written back byte for byte as the ledger wrote
// it, which is the only assertion that catches a field moving. The expected
// bytes are the ledger's, not a re-encoding of the decoded value.
func TestStakePoolStateResultRoundTripsGolden(t *testing.T) {
	t.Parallel()
	for name, golden := range map[string]string{
		"conway":   stakePoolStateConwayGolden,
		"dijkstra": stakePoolStateDijkstraGolden,
	} {
		for idx, original := range ledgerGoldenReplies(t, golden) {
			result := decodeGoldenReply(t, original)
			reencoded, err := cbor.Encode(result)
			require.NoError(t, err)
			assert.Equal(
				t,
				hex.EncodeToString(original),
				hex.EncodeToString(reencoded),
				"%s reply %d",
				name,
				idx,
			)
		}
	}
}

// goldenPoolEntryFields returns the fourteen raw fields of the first pool of
// the second Conway reply, the one with every field set.
func goldenPoolEntryFields(t *testing.T) []cbor.RawMessage {
	t.Helper()
	replies := ledgerGoldenReplies(t, stakePoolStateConwayGolden)
	var wrapper []cbor.RawMessage
	_, err := cbor.Decode(replies[1], &wrapper)
	require.NoError(t, err)
	if len(wrapper) != 1 {
		t.Fatalf("reply wrapper: got %d elements, want 1", len(wrapper))
	}
	var entries map[ledger.PoolId]cbor.RawMessage
	_, err = cbor.Decode(wrapper[0], &entries)
	require.NoError(t, err)
	var fields []cbor.RawMessage
	_, err = cbor.Decode(entries[goldenPoolId(t, goldenPoolA)], &fields)
	require.NoError(t, err)
	require.Len(t, fields, stakePoolStateFieldCount)
	return fields
}

// TestStakePoolStateEntryRejectsWrongArity feeds a valid reply wrapper an
// entry that is the ledger's own record with one field removed or added, so
// that only the arity can be what the decoder refuses.
func TestStakePoolStateEntryRejectsWrongArity(t *testing.T) {
	t.Parallel()
	poolA := goldenPoolId(t, goldenPoolA)
	fields := goldenPoolEntryFields(t)
	for name, entryFields := range map[string][]cbor.RawMessage{
		"thirteen_fields": fields[:stakePoolStateFieldCount-1],
		"fifteen_fields": append(
			append([]cbor.RawMessage{}, fields...),
			cbor.RawMessage{0xf6},
		),
	} {
		entry, err := cbor.Encode(entryFields)
		require.NoError(t, err)
		reply, err := cbor.Encode(
			[]any{map[ledger.PoolId]cbor.RawMessage{poolA: entry}},
		)
		require.NoError(t, err)
		var result StakePoolStateResult
		_, err = cbor.Decode(reply, &result)
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "expected 14 fields", name)
	}
}

// goldenParamsFields returns the nine raw fields of the pending registration
// of the first pool of the second Conway reply.
func goldenParamsFields(t *testing.T) []cbor.RawMessage {
	t.Helper()
	var entry StakePoolStateEntry
	_, err := cbor.Decode(goldenPoolEntryFieldsRaw(t), &entry)
	require.NoError(t, err)
	params, err := cbor.Encode(entry.FutureStakePoolParams)
	require.NoError(t, err)
	var fields []cbor.RawMessage
	_, err = cbor.Decode(params, &fields)
	require.NoError(t, err)
	require.Len(t, fields, stakePoolParamsFieldCount)
	return fields
}

func goldenPoolEntryFieldsRaw(t *testing.T) []byte {
	t.Helper()
	raw, err := cbor.Encode(goldenPoolEntryFields(t))
	require.NoError(t, err)
	return raw
}

// TestStakePoolParamsEntryRejectsWrongArity covers the record's two valid
// arities by refusing the ones around them: eight and eleven fields.
func TestStakePoolParamsEntryRejectsWrongArity(t *testing.T) {
	t.Parallel()
	fields := goldenParamsFields(t)
	null := cbor.RawMessage{0xf6}
	for name, params := range map[string][]cbor.RawMessage{
		"eight_fields": fields[:stakePoolParamsFieldCount-1],
		"eleven_fields": append(
			append([]cbor.RawMessage{}, fields...),
			null,
			null,
		),
	} {
		encoded, err := cbor.Encode(params)
		require.NoError(t, err)
		var entry StakePoolParamsEntry
		_, err = cbor.Decode(encoded, &entry)
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "expected 9 or 10 fields", name)
	}
}

// TestStakePoolParamsEntryNullBlsKey covers a ten-field record whose BLS slot
// holds a null: the ledger reads that as no key (decodeBlsKeyBackwardsCompatible),
// not as a zero key.
func TestStakePoolParamsEntryNullBlsKey(t *testing.T) {
	t.Parallel()
	fields := goldenParamsFields(t)
	withNull := append(
		[]cbor.RawMessage{},
		fields[:2]...,
	)
	withNull = append(withNull, cbor.RawMessage{0xf6})
	withNull = append(withNull, fields[2:]...)
	require.Len(t, withNull, stakePoolParamsFieldCount+1)
	encoded, err := cbor.Encode(withNull)
	require.NoError(t, err)

	var entry StakePoolParamsEntry
	_, err = cbor.Decode(encoded, &entry)
	require.NoError(t, err)
	assert.Nil(t, entry.BlsKey)
	assert.Equal(t, goldenPoolId(t, goldenPoolA), entry.Id)
	assert.Equal(t, uint64(150000000000), entry.Pledge)
}

// TestStakePoolStateResultRejectsUnknownArity keeps the decoder from quietly
// accepting a record it cannot map.
func TestStakePoolStateResultRejectsUnknownArity(t *testing.T) {
	encoded, err := cbor.Encode([]any{1, 2, 3})
	require.NoError(t, err)
	var result StakePoolStateResult
	_, err = cbor.Decode(encoded, &result)
	require.Error(t, err)
}
