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
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fixtures are written out byte by byte from the layout the node uses for
// NonMyopicMemberRewards, a Map (Either Coin (Credential Staking))
// (Map KeyHash Coin) written with the plain ToCBOR instances:
//
//	Either x = [0, x] for Left, [1, x] for Right
//	Credential = [0, hash] for a key hash, [1, hash] for a script hash
//	Map = entries in ascending Ord, definite-length for the plain encoder
//
// The ledger's derived Ord on Credential declares the script constructor
// first, so a script credential (wire tag 1) sorts before a key credential
// (wire tag 0), and a Left sorts before every Right.

// nmHash is a 28-byte hash of one repeated byte as a CBOR byte string.
func nmHash(b byte) string {
	return "581c" + strings.Repeat(fmt.Sprintf("%02x", b), 28)
}

func nmBlake(b byte) ledger.Blake2b224 {
	return ledger.NewBlake2b224(bytes.Repeat([]byte{b}, 28))
}

func nmFromHex(t *testing.T, parts ...string) []byte {
	t.Helper()
	data, err := hex.DecodeString(strings.Join(parts, ""))
	require.NoError(t, err)
	return data
}

var (
	nmKeyCoin5     = "820005"
	nmKeyCoin1M    = "82001a000f4240"
	nmKeyScriptAA  = "82018201" + nmHash(0xaa)
	nmKeyKeyHash11 = "82018200" + nmHash(0x11)
	nmKeyKeyHashFF = "82018200" + nmHash(0xff)
)

// nmMixedWire holds every key form, in the node's ascending order. Sorting
// the keys bytewise would put the key hashes before the script hash.
func nmMixedWire(t *testing.T) []byte {
	return nmFromHex(t,
		"a5",
		nmKeyCoin5, "a1", nmHash(1), "193039", // {pool01: 12345}
		nmKeyCoin1M, "a0", // {}
		nmKeyScriptAA, "a2", nmHash(1), "01", nmHash(2), "00",
		nmKeyKeyHash11, "a1", nmHash(2), "1b0000010000000000",
		nmKeyKeyHashFF, "a2", nmHash(1), "1a000f4240", nmHash(2), "07",
	)
}

func nmMixedResult() localstatequery.NonMyopicMemberRewardsResult {
	script := localstatequery.StakeCredential{Tag: 1, Bytes: nmBlake(0xaa)}
	key11 := localstatequery.StakeCredential{Tag: 0, Bytes: nmBlake(0x11)}
	keyFF := localstatequery.StakeCredential{Tag: 0, Bytes: nmBlake(0xff)}
	return localstatequery.NonMyopicMemberRewardsResult{
		localstatequery.NewNonMyopicCoinKey(5): {
			nmBlake(1): 12345,
		},
		localstatequery.NewNonMyopicCoinKey(1_000_000): {},
		localstatequery.NewNonMyopicCredentialKey(script): {
			nmBlake(1): 1,
			nmBlake(2): 0,
		},
		localstatequery.NewNonMyopicCredentialKey(key11): {
			nmBlake(2): 1 << 40,
		},
		localstatequery.NewNonMyopicCredentialKey(keyFF): {
			nmBlake(1): 1_000_000,
			nmBlake(2): 7,
		},
	}
}

// Golden bytes from the consensus serialisation tests, produced by the
// node's own encoders: the Shelley query and its result. Pool b16b... is a
// key hash and e0a7... a script hash, and the script credential precedes the
// key credential.
const (
	nmGoldenPoolA  = "a646474b8f5431261506b6c273d307c7569a4eb6c96b42dd4a29520a"
	nmGoldenPoolB  = "b16b56f5ec064be6ac3cab6035efae86b366cc3dc4a0d571603d70e5"
	nmGoldenScript = "e0a714319812c3f773ba04ec5d6b3ffcd5aad85006805b047b082541"

	nmGoldenQuery = "8202d9010283" +
		"8200" + "1864" +
		"82018201" + "581c" + nmGoldenScript +
		"82018200" + "581c" + nmGoldenPoolA
	nmGoldenResult = "a3" +
		"8200" + "1864" + "a1" + "581c" + nmGoldenPoolA + "03" +
		"82018201" + "581c" + nmGoldenScript + "a0" +
		"82018200" + "581c" + nmGoldenPoolA +
		"a1" + "581c" + nmGoldenPoolB + "09"
)

func nmGoldenHash(t *testing.T, h string) ledger.Blake2b224 {
	t.Helper()
	raw, err := hex.DecodeString(h)
	require.NoError(t, err)
	return ledger.NewBlake2b224(raw)
}

func nmGoldenValue(t *testing.T) localstatequery.NonMyopicMemberRewardsResult {
	t.Helper()
	poolA := nmGoldenHash(t, nmGoldenPoolA)
	return localstatequery.NonMyopicMemberRewardsResult{
		localstatequery.NewNonMyopicCoinKey(100): {poolA: 3},
		localstatequery.NewNonMyopicCredentialKey(
			localstatequery.StakeCredential{
				Tag:   1,
				Bytes: nmGoldenHash(t, nmGoldenScript),
			},
		): {},
		localstatequery.NewNonMyopicCredentialKey(
			localstatequery.StakeCredential{Tag: 0, Bytes: poolA},
		): {nmGoldenHash(t, nmGoldenPoolB): 9},
	}
}

func TestNonMyopicMemberRewardsGoldenResult(t *testing.T) {
	t.Parallel()
	golden := nmFromHex(t, nmGoldenResult)
	var got localstatequery.NonMyopicMemberRewardsResult
	_, err := cbor.Decode(golden, &got)
	require.NoError(t, err)
	assert.Equal(t, nmGoldenValue(t), got)

	encoded, err := cbor.Encode(nmGoldenValue(t))
	require.NoError(t, err)
	assert.Equal(t, nmGoldenResult, hex.EncodeToString(encoded))
}

func TestNonMyopicMemberRewardsDecodesNodeEncoding(t *testing.T) {
	t.Parallel()
	var got localstatequery.NonMyopicMemberRewardsResult
	_, err := cbor.Decode(nmMixedWire(t), &got)
	require.NoError(t, err)
	assert.Equal(t, nmMixedResult(), got)
}

func TestNonMyopicMemberRewardsEncodesAsNode(t *testing.T) {
	t.Parallel()
	got, err := cbor.Encode(nmMixedResult())
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(nmMixedWire(t)), hex.EncodeToString(got))
}

func TestNonMyopicMemberRewardsEmpty(t *testing.T) {
	t.Parallel()
	got, err := cbor.Encode(localstatequery.NonMyopicMemberRewardsResult{})
	require.NoError(t, err)
	assert.Equal(t, []byte{0xa0}, got)
	var decoded localstatequery.NonMyopicMemberRewardsResult
	_, err = cbor.Decode([]byte{0xa0}, &decoded)
	require.NoError(t, err)
	assert.Empty(t, decoded)
}

// The plain map encoder writes a definite length whatever the size, so a map
// of 24 entries has the one-byte length header 0xb8 0x18, not 0xbf.
func TestNonMyopicMemberRewardsLargeMapsStayDefinite(t *testing.T) {
	t.Parallel()
	const count = 24
	result := localstatequery.NonMyopicMemberRewardsResult{}
	var wire strings.Builder
	wire.WriteString("b818")
	for i := range count {
		pools := map[ledger.Blake2b224]uint64{}
		var inner strings.Builder
		inner.WriteString("b818")
		for p := 1; p <= count; p++ {
			pools[nmBlake(byte(p))] = uint64(p - 1)
			inner.WriteString(nmHash(byte(p)) + fmt.Sprintf("%02x", p-1))
		}
		result[localstatequery.NewNonMyopicCoinKey(uint64(i))] = pools
		wire.WriteString(fmt.Sprintf("8200%02x", i) + inner.String())
	}
	want := nmFromHex(t, wire.String())

	got, err := cbor.Encode(result)
	require.NoError(t, err)
	// Compare once as bytes: a mismatch would otherwise print both blobs.
	require.True(
		t,
		bytes.Equal(want, got),
		"encoding differs: got %d bytes starting %x, want %d bytes starting %x",
		len(got), got[:min(len(got), 8)], len(want), want[:8],
	)

	var decoded localstatequery.NonMyopicMemberRewardsResult
	_, err = cbor.Decode(want, &decoded)
	require.NoError(t, err)
	assert.Equal(t, result, decoded)
}

// The ledger's era encoder for Map switches to an indefinite-length map above
// 23 entries, and a node that writes the result with it sends such maps for
// any query covering more than 23 pools.
func TestNonMyopicMemberRewardsDecodesIndefiniteMaps(t *testing.T) {
	t.Parallel()
	const count = 24
	want := localstatequery.NonMyopicMemberRewardsResult{}
	var wire strings.Builder
	wire.WriteString("bf")
	for i := range count {
		pools := map[ledger.Blake2b224]uint64{}
		wire.WriteString(fmt.Sprintf("8200%02x", i) + "bf")
		for p := 1; p <= count; p++ {
			pools[nmBlake(byte(p))] = uint64(p - 1)
			wire.WriteString(nmHash(byte(p)) + fmt.Sprintf("%02x", p-1))
		}
		wire.WriteString("ff")
		want[localstatequery.NewNonMyopicCoinKey(uint64(i))] = pools
	}
	wire.WriteString("ff")

	var got localstatequery.NonMyopicMemberRewardsResult
	_, err := cbor.Decode(nmFromHex(t, wire.String()), &got)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestNonMyopicMemberRewardsRejectsNonCanonicalReplies(t *testing.T) {
	t.Parallel()
	pools := "a1" + nmHash(1) + "01"
	tests := []struct {
		name string
		wire string
	}{
		{
			name: "unknown Either tag",
			wire: "a1" + "820201" + pools,
		},
		{
			name: "unknown credential tag",
			wire: "a1" + "82018202" + nmHash(1) + pools,
		},
		{
			name: "credential before coin",
			wire: "a2" + nmKeyKeyHash11 + pools + nmKeyCoin5 + pools,
		},
		{
			name: "key credential before script credential",
			wire: "a2" + nmKeyKeyHash11 + pools + nmKeyScriptAA + pools,
		},
		{
			name: "descending coins",
			wire: "a2" + nmKeyCoin1M + pools + nmKeyCoin5 + pools,
		},
		{
			name: "repeated key",
			wire: "a2" + nmKeyCoin5 + pools + nmKeyCoin5 + pools,
		},
		{
			name: "descending pools",
			wire: "a1" + nmKeyCoin5 + "a2" + nmHash(
				2,
			) + "01" + nmHash(
				1,
			) + "01",
		},
		{
			name: "short hash",
			wire: "a1" + "82018200" + "581b" + strings.Repeat("11", 27) + pools,
		},
		{
			name: "negative coin",
			wire: "a1" + "820020" + pools,
		},
		{
			name: "negative reward",
			wire: "a1" + nmKeyCoin5 + "a1" + nmHash(1) + "20",
		},
		{
			name: "null coin",
			wire: "a1" + "8200f6" + pools,
		},
		{
			name: "undefined coin",
			wire: "a1" + "8200f7" + pools,
		},
		{
			name: "null Either tag",
			wire: "a1" + "82f605" + pools,
		},
		{
			name: "null credential tag",
			wire: "a1" + "8201" + "82f6" + nmHash(1) + pools,
		},
		{
			name: "null credential hash",
			wire: "a1" + "82018200f6" + pools,
		},
		{
			name: "null pool id",
			wire: "a1" + nmKeyCoin5 + "a1f601",
		},
		{
			name: "null reward",
			wire: "a1" + nmKeyCoin5 + "a1" + nmHash(1) + "f6",
		},
		{
			name: "undefined reward",
			wire: "a1" + nmKeyCoin5 + "a1" + nmHash(1) + "f7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got localstatequery.NonMyopicMemberRewardsResult
			_, err := cbor.Decode(nmFromHex(t, tt.wire), &got)
			require.Error(t, err)
		})
	}
}

func TestNonMyopicMemberRewardsKeyRejectsUnknownCredentialTag(t *testing.T) {
	t.Parallel()
	key := localstatequery.NewNonMyopicCredentialKey(
		localstatequery.StakeCredential{Tag: 2, Bytes: nmBlake(1)},
	)
	_, err := cbor.Encode(key)
	require.Error(t, err)
}

func TestNonMyopicMemberRewardsRejectsAliasedKeys(t *testing.T) {
	t.Parallel()
	coin := localstatequery.NewNonMyopicCoinKey(5)
	coinAlias := coin
	coinAlias.Credential.Tag = 1
	credential := localstatequery.NewNonMyopicCredentialKey(
		localstatequery.StakeCredential{Tag: 0, Bytes: nmBlake(1)},
	)
	credentialAlias := credential
	credentialAlias.Coin = 9
	pools := map[ledger.Blake2b224]uint64{nmBlake(1): 1}
	for name, keys := range map[string][2]localstatequery.NonMyopicMemberRewardsKey{
		"coin":       {coin, coinAlias},
		"credential": {credential, credentialAlias},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result := localstatequery.NonMyopicMemberRewardsResult{
				keys[0]: pools,
				keys[1]: pools,
			}
			_, err := cbor.Encode(result)
			require.Error(t, err)
		})
	}
}

// Not t.Parallel: runTest verifies goroutine leaks for the whole process.
//
// The request carries the keys as the node's set decoder requires: tag 258,
// a definite-length array and strictly ascending keys by the ledger's Ord,
// whatever order the caller supplied. The reply is decoded from the node's
// layout, inside the one-element list the hard fork combinator adds.
func TestGetNonMyopicMemberRewardsWire(t *testing.T) {
	script := localstatequery.StakeCredential{Tag: 1, Bytes: nmBlake(0xaa)}
	key11 := localstatequery.StakeCredential{Tag: 0, Bytes: nmBlake(0x11)}
	keyFF := localstatequery.StakeCredential{Tag: 0, Bytes: nmBlake(0xff)}
	// Descending, with a repeat, so the client has to reorder and collapse.
	keys := []localstatequery.NonMyopicMemberRewardsKey{
		localstatequery.NewNonMyopicCredentialKey(keyFF),
		localstatequery.NewNonMyopicCredentialKey(key11),
		localstatequery.NewNonMyopicCredentialKey(script),
		localstatequery.NewNonMyopicCoinKey(1_000_000),
		localstatequery.NewNonMyopicCoinKey(5),
		localstatequery.NewNonMyopicCoinKey(5),
	}
	set := nmFromHex(t,
		"d90102", "85",
		nmKeyCoin5, nmKeyCoin1M, nmKeyScriptAA, nmKeyKeyHash11, nmKeyKeyHashFF,
	)
	nmRunClient(t, keys, set, nmMixedWire(t), nmMixedResult())
}

// Not t.Parallel: runTest verifies goroutine leaks for the whole process.
//
// The keys are supplied in descending order; the request must still match
// the golden query set byte for byte.
func TestGetNonMyopicMemberRewardsGolden(t *testing.T) {
	want := nmGoldenValue(t)
	keys := make([]localstatequery.NonMyopicMemberRewardsKey, 0, len(want))
	for key := range want {
		keys = append(keys, key)
	}
	slices.SortFunc(
		keys,
		func(a, b localstatequery.NonMyopicMemberRewardsKey) int {
			return b.Compare(a)
		},
	)
	// The golden query is [2, set]; the set follows the two-byte prefix.
	set := nmFromHex(t, nmGoldenQuery)[2:]
	nmRunClient(t, keys, set, nmFromHex(t, nmGoldenResult), want)
}

// nmRunClient runs GetNonMyopicMemberRewards against a mock node that
// expects exactly the given query set and answers with the given result.
func nmRunClient(
	t *testing.T,
	keys []localstatequery.NonMyopicMemberRewardsKey,
	set []byte,
	result []byte,
	want localstatequery.NonMyopicMemberRewardsResult,
) {
	t.Helper()
	const (
		conwayEra = 6
		version   = 23
	)
	reply := append([]byte{0x81}, result...)
	wire, err := cbor.Encode([]any{
		localstatequery.MessageTypeQuery,
		[]any{
			localstatequery.QueryTypeBlock,
			[]any{
				localstatequery.QueryTypeShelley,
				[]any{
					conwayEra,
					[]any{
						localstatequery.QueryTypeShelleyNonMyopicMemberRewards,
						cbor.RawMessage(set),
					},
				},
			},
		},
	})
	require.NoError(t, err)
	expected, err := localstatequery.NewMsgFromCbor(
		localstatequery.MessageTypeQuery,
		wire,
	)
	require.NoError(t, err)
	expected.SetCbor(nil)

	setup := conversationEraQueryAt(version, conwayEra, nil)
	conversation := append(
		append([]ouroboros_mock.ConversationEntry{}, setup[:len(setup)-2]...),
		ouroboros_mock.ConversationEntryInput{
			ProtocolId: localstatequery.ProtocolId,
			Message:    expected,
			MsgFromCborFunc: func(
				msgType uint,
				data []byte,
			) (protocol.Message, error) {
				return localstatequery.NewMsgFromCbor(msgType, data)
			},
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
			got, err := oConn.LocalStateQuery().Client.GetNonMyopicMemberRewards(
				keys,
			)
			require.NoError(t, err)
			assert.Equal(t, want, *got)
		},
	)
}
