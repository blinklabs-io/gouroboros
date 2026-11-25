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
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
)

// The ouroboros-consensus golden files in testdata are copied unchanged from
// ouroboros-consensus-cardano/golden at revision ec239d57f618; see
// testdata/README.md. That revision speaks only node-to-client version 23
// (CardanoNodeToClientVersion19, ShelleyNodeToClientVersion15).
const (
	referenceGoldenRoot = "testdata/ouroboros-consensus/ouroboros-consensus-cardano/golden"
	referenceCardanoN2C = referenceGoldenRoot +
		"/cardano/QueryVersion3/CardanoNodeToClientVersion19"
	referenceShelleyN2C = referenceGoldenRoot +
		"/shelley/QueryVersion3/ShelleyNodeToClientVersion15"
	referenceN2CVersion = 23
)

// referenceEras are the Cardano eras in hard-fork index order, as the golden
// file names spell them.
var referenceEras = []string{
	"Byron", "Shelley", "Allegra", "Mary", "Alonzo", "Babbage", "Conway", "Dijkstra",
}

const referenceConwayIndex = 6

func readReferenceGolden(t *testing.T, dir, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	return raw
}

func referenceHex(t *testing.T, s string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(s)
	require.NoError(t, err)
	return raw
}

// The pool and credentials of the Shelley ledger examples
// (Test.Cardano.Ledger.Shelley.Examples: leRewardsCredentials, lePoolDistr),
// which the GetNonMyopicMemberRewards and QueryStakePoolDefaultVote goldens
// carry.
const (
	referenceExamplePool       = "e0a714319812c3f773ba04ec5d6b3ffcd5aad85006805b047b082541"
	referenceExampleKeyCred    = "a646474b8f5431261506b6c273d307c7569a4eb6c96b42dd4a29520a"
	referenceExampleScriptCred = referenceExamplePool
)

func requireEmptySet[T any](t *testing.T, set cbor.SetType[T]) {
	t.Helper()
	require.Empty(t, set.Items())
}

// referenceBlockQueries lists every Shelley-family block query the
// Test.Consensus.Shelley.Examples queries carry (eraIndependentQueries and
// conwayQueries), with the decoded value the golden must give. Arguments
// are the examples' own: mostly empty sets and Nothing.
var referenceBlockQueries = map[string]func(*testing.T, any){
	"GetLedgerTip": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyLedgerTipQuery{}, q)
	},
	"GetEpochNo": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyEpochNoQuery{}, q)
	},
	"GetNonMyopicMemberRewards": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyNonMyopicMemberRewardsQuery)
		require.True(t, ok, "%T", q)
		// Left (Coin 100), then the script and the key credential, in the
		// order of the set's encoded members.
		items := query.Stakes.Items()
		require.Len(t, items, 3)
		keys := make([]localstatequery.NonMyopicMemberRewardsKey, len(items))
		for i, item := range items {
			_, err := cbor.Decode(item, &keys[i])
			require.NoError(t, err)
		}
		require.False(t, keys[0].IsCredential)
		require.Equal(t, uint64(100), keys[0].Coin)
		require.True(t, keys[1].IsCredential)
		require.Equal(t, uint64(1), keys[1].Credential.Tag)
		require.Equal(t, referenceExampleScriptCred, hex.EncodeToString(keys[1].Credential.Bytes.Bytes()))
		require.True(t, keys[2].IsCredential)
		require.Equal(t, uint64(0), keys[2].Credential.Tag)
		require.Equal(t, referenceExampleKeyCred, hex.EncodeToString(keys[2].Credential.Bytes.Bytes()))
	},
	"GetCurrentPParams": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyCurrentProtocolParamsQuery{}, q)
	},
	"GetUTxOByAddress": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyUtxoByAddressQuery)
		require.True(t, ok, "%T", q)
		require.Empty(t, query.Addrs)
	},
	"GetUTxOWhole": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyUtxoWholeQuery{}, q)
	},
	"DebugEpochState": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyDebugEpochStateQuery{}, q)
	},
	"GetCBOR": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyCborQuery)
		require.True(t, ok, "%T", q)
		require.IsType(t, &localstatequery.ShelleyLedgerTipQuery{}, query.Query)
	},
	"GetFilteredDelegationsAndRewardAccounts": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyFilteredDelegationAndRewardAccountsQuery)
		require.True(t, ok, "%T", q)
		requireEmptySet(t, query.Creds)
	},
	"GetGenesisConfig": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyGenesisConfigQuery{}, q)
	},
	"DebugNewEpochState": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyDebugNewEpochStateQuery{}, q)
	},
	"DebugChainDepState": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyDebugChainDepStateQuery{}, q)
	},
	"GetRewardProvenance": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyRewardProvenanceQuery{}, q)
	},
	"GetUTxOByTxIn": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyUtxoByTxinQuery)
		require.True(t, ok, "%T", q)
		require.Empty(t, query.TxIns)
	},
	"GetStakePools": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyStakePoolsQuery{}, q)
	},
	"GetStakePoolParams": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyStakePoolParamsQuery)
		require.True(t, ok, "%T", q)
		requireEmptySet(t, query.PoolIds)
	},
	"GetRewardInfoPools": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyRewardInfoPoolsQuery{}, q)
	},
	"GetPoolState": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyPoolStateQuery)
		require.True(t, ok, "%T", q)
		require.Empty(t, query.Pools, "Nothing is the empty list")
	},
	"GetStakeSnapshots": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyStakeSnapshotsQuery)
		require.True(t, ok, "%T", q)
		pools, all := query.PoolFilter()
		require.True(t, all)
		require.Empty(t, pools)
	},
	"GetStakeDelegDeposits": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyStakeDelegDepositsQuery)
		require.True(t, ok, "%T", q)
		requireEmptySet(t, query.Creds)
	},
	"GetGovState": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyGovStateQuery{}, q)
	},
	"GetAccountState": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyAccountStateQuery{}, q)
	},
	"GetBigLedgerPeerSnapshot": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyGetLedgerPeerSnapshotQuery)
		require.True(t, ok, "%T", q)
		require.True(t, query.WithPeerKind)
		require.Equal(t, localstatequery.LedgerPeerKindBig, query.PeerKind)
	},
	"GetAllLedgerPeerSnapshot": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyGetLedgerPeerSnapshotQuery)
		require.True(t, ok, "%T", q)
		require.True(t, query.WithPeerKind)
		require.Equal(t, localstatequery.LedgerPeerKindAll, query.PeerKind)
	},
	"GetPoolDistr2": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyPoolDistr2Query)
		require.True(t, ok, "%T", q)
		pools, all := query.PoolFilter()
		require.True(t, all)
		require.Empty(t, pools)
	},
	"GetStakeDistribution2": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyStakeDistribution2Query{}, q)
	},
	"GetMaxMajorProtocolVersion": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyMaxMajorProtocolVersionQuery{}, q)
	},
}

// referenceConwayBlockQueries are the queries conwayQueries adds; their
// goldens exist for Conway and Dijkstra only.
var referenceConwayBlockQueries = map[string]func(*testing.T, any){
	"GetConstitution": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyConstitutionQuery{}, q)
	},
	"GetDRepState": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyDRepStateQuery)
		require.True(t, ok, "%T", q)
		requireEmptySet(t, query.Credentials)
	},
	"GetDRepStakeDistr": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyDRepStakeDistrQuery)
		require.True(t, ok, "%T", q)
		requireEmptySet(t, query.DReps)
	},
	"GetCommitteeMembersState": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyCommitteeMembersStateQuery)
		require.True(t, ok, "%T", q)
		requireEmptySet(t, query.ColdCreds)
		requireEmptySet(t, query.HotCreds)
		requireEmptySet(t, query.MemberStatus)
	},
	"GetFilteredVoteDelegatees": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyFilteredVoteDelegateesQuery)
		require.True(t, ok, "%T", q)
		requireEmptySet(t, query.Credentials)
	},
	"GetSPOStakeDistr": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleySPOStakeDistrQuery)
		require.True(t, ok, "%T", q)
		requireEmptySet(t, query.PoolIds)
	},
	"GetProposals": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyGetProposalsQuery)
		require.True(t, ok, "%T", q)
		requireEmptySet(t, query.ActionIds)
	},
	"GetRatifyState": func(t *testing.T, q any) {
		require.IsType(t, &localstatequery.ShelleyGetRatifyStateQuery{}, q)
	},
	"GetDRepDelegations": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyDRepDelegationsQuery)
		require.True(t, ok, "%T", q)
		requireEmptySet(t, query.DReps)
	},
	"QueryStakePoolDefaultVote": func(t *testing.T, q any) {
		query, ok := q.(*localstatequery.ShelleyStakePoolDefaultVoteQuery)
		require.True(t, ok, "%T", q)
		require.Equal(t, referenceExamplePool, hex.EncodeToString(query.PoolId[:]))
	},
}

// referenceQueryReencodeDiffers lists the goldens whose decoded query does
// not re-encode to the same bytes. The address and transaction-input filters
// are plain slices, so the Conway set tag the node writes (toEraCBOR from
// protocol version 9) is dropped on re-encoding. The node accepts both forms
// (allowTag in decodeSetLikeEnforceNoDuplicates).
var referenceQueryReencodeDiffers = map[string]bool{
	"Query_Conway_GetUTxOByAddress":   true,
	"Query_Conway_GetUTxOByTxIn":      true,
	"Query_Dijkstra_GetUTxOByAddress": true,
	"Query_Dijkstra_GetUTxOByTxIn":    true,
}

// referenceQueryUnsupported lists goldens gOuroboros does not decode: the
// Byron era's own queries and the hard-fork combinator's QueryAnytime
// (GetEraStart).
func referenceQueryUnsupported(name string) bool {
	switch name {
	case "Query_Byron", "Query_AnytimeByron", "Query_AnytimeShelley":
		return true
	}
	return false
}

// referenceQueryLeftOut lists goldens this test does not judge:
// GetFuturePParams (Shelley block query 33), whose decoding is covered where
// the server learns the query.
func referenceQueryLeftOut(name string) bool {
	return strings.HasSuffix(name, "_GetFuturePParams")
}

// decodeReferenceQuery decodes a MsgQuery carrying the golden block query the
// way the server does, and returns the wrapper and the message bytes.
func decodeReferenceQuery(
	t *testing.T,
	query []byte,
) (localstatequery.QueryWrapper, []byte, error) {
	t.Helper()
	msgBytes, err := cbor.Encode([]any{
		localstatequery.MessageTypeQuery,
		cbor.RawMessage(query),
	})
	require.NoError(t, err)
	msg, err := localstatequery.NewMsgFromCbor(
		localstatequery.MessageTypeQuery,
		msgBytes,
	)
	if err != nil {
		return localstatequery.QueryWrapper{}, nil, err
	}
	msgQuery, ok := msg.(*localstatequery.MsgQuery)
	require.True(t, ok, "%T", msg)
	return msgQuery.Query, msgBytes, nil
}

// blockQueryGolden returns the full query for a block-query golden: the
// golden is the BlockQuery, and the top-level Query wraps it as [0, q].
func blockQueryGolden(raw []byte) []byte {
	return append([]byte{0x82, 0x00}, raw...)
}

// TestReferenceQueryGoldensDecode decodes every block query golden of the
// Cardano block as the server does, checks the decoded query against the
// examples it was written from, and checks it re-encodes to the same bytes.
func TestReferenceQueryGoldensDecode(t *testing.T) {
	entries, err := os.ReadDir(referenceCardanoN2C)
	require.NoError(t, err)
	seen := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "Query_") {
			continue
		}
		seen++
		t.Run(name, func(t *testing.T) {
			if referenceQueryLeftOut(name) {
				t.Skip("GetFuturePParams is not judged here")
			}
			raw := readReferenceGolden(t, referenceCardanoN2C, name)
			full := blockQueryGolden(raw)
			wrapper, _, err := decodeReferenceQuery(t, full)
			if referenceQueryUnsupported(name) {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, full, wrapper.Cbor())
			block, ok := wrapper.Query.(*localstatequery.BlockQuery)
			require.True(t, ok, "%T", wrapper.Query)
			if name == "Query_HardFork" {
				hf, ok := block.Query.(*localstatequery.HardForkQuery)
				require.True(t, ok, "%T", block.Query)
				require.IsType(t, &localstatequery.HardForkEraHistoryQuery{}, hf.Query)
			} else {
				parts := strings.SplitN(strings.TrimPrefix(name, "Query_"), "_", 2)
				require.Len(t, parts, 2)
				era := indexOfReferenceEra(t, parts[0])
				shelley, ok := block.Query.(*localstatequery.ShelleyQuery)
				require.True(t, ok, "%T", block.Query)
				require.Equal(t, uint(era), shelley.Era)
				check, ok := referenceBlockQueries[parts[1]]
				if !ok {
					check, ok = referenceConwayBlockQueries[parts[1]]
					require.True(t, ok, "no expectation for %s", parts[1])
					require.GreaterOrEqual(t, era, referenceConwayIndex)
				}
				check(t, shelley.Query)
			}
			if referenceQueryReencodeDiffers[name] {
				return
			}
			reencoded, err := cbor.Encode(&wrapper)
			require.NoError(t, err)
			require.Equal(t, hex.EncodeToString(full), hex.EncodeToString(reencoded))
		})
	}
	// Seven Shelley-family eras times the era-independent queries (one of
	// them GetFuturePParams), two eras times the Conway queries, and the
	// Byron, two QueryAnytime and one hard-fork query.
	require.Equal(
		t,
		7*(len(referenceBlockQueries)+1)+2*len(referenceConwayBlockQueries)+4,
		seen,
	)
}

func indexOfReferenceEra(t *testing.T, name string) int {
	t.Helper()
	for i, era := range referenceEras {
		if era == name {
			return i
		}
	}
	t.Fatalf("unknown era %q", name)
	return -1
}

// TestReferenceShelleyQueryGoldensMatchCardano checks that each query of the
// single-era Shelley block, once wrapped in the Shelley era's envelope, is the
// Cardano block's Shelley query golden.
func TestReferenceShelleyQueryGoldensMatchCardano(t *testing.T) {
	entries, err := os.ReadDir(referenceShelleyN2C)
	require.NoError(t, err)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "Query_") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			raw := readReferenceGolden(t, referenceShelleyN2C, name)
			cardano := readReferenceGolden(
				t,
				referenceCardanoN2C,
				"Query_Shelley_"+strings.TrimPrefix(name, "Query_"),
			)
			wrapped := append([]byte{0x82, 0x00, 0x82, 0x01}, raw...)
			require.Equal(t, cardano, wrapped)
		})
	}
}

// TestReferenceTopLevelQueryGoldens decodes the three top-level query goldens,
// re-encodes them, and checks the client sends the same bytes.
func TestReferenceTopLevelQueryGoldens(t *testing.T) {
	cases := []struct {
		name string
		want any
		call func(*localstatequery.Client) error
	}{
		{
			name: "GetSystemStart",
			want: &localstatequery.SystemStartQuery{},
			call: func(c *localstatequery.Client) error {
				_, err := c.GetSystemStart()
				return err
			},
		},
		{
			name: "GetChainBlockNo",
			want: &localstatequery.ChainBlockNoQuery{},
			call: func(c *localstatequery.Client) error {
				_, err := c.GetChainBlockNo()
				return err
			},
		},
		{
			name: "GetChainPoint",
			want: &localstatequery.ChainPointQuery{},
			call: func(c *localstatequery.Client) error {
				_, err := c.GetChainPoint()
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, dir := range []string{referenceCardanoN2C, referenceShelleyN2C} {
				raw := readReferenceGolden(t, dir, "TopLevelQuery_"+tc.name)
				wrapper, _, err := decodeReferenceQuery(t, raw)
				require.NoError(t, err)
				require.IsType(t, tc.want, wrapper.Query)
				reencoded, err := cbor.Encode(&wrapper)
				require.NoError(t, err)
				require.Equal(t, raw, reencoded)
			}
			raw := readReferenceGolden(t, referenceCardanoN2C, "TopLevelQuery_"+tc.name)
			sent := captureTopLevelQuery(t, tc.call)
			require.Equal(t, hex.EncodeToString(raw), hex.EncodeToString(sent))
		})
	}
}

// captureTopLevelQuery runs a top-level query, which needs no era, against a
// version 23 node and returns the query the client sent.
func captureTopLevelQuery(
	t *testing.T,
	call func(*localstatequery.Client) error,
) []byte {
	t.Helper()
	conversation := conversationEraQueryAt(referenceN2CVersion, 0, nil)[:6]
	return captureQueryAt(t, conversation, 4, call)
}

// captureBlockQuery runs a block query against a version 23 node whose
// current era is era, and returns the query the client sent.
func captureBlockQuery(
	t *testing.T,
	era int,
	call func(*localstatequery.Client) error,
) []byte {
	t.Helper()
	conversation := conversationEraQueryAt(
		referenceN2CVersion,
		byte(era),
		[]byte{0x81, 0x00},
	)
	return captureQueryAt(t, conversation, 6, call)
}

func captureQueryAt(
	t *testing.T,
	conversation []ouroboros_mock.ConversationEntry,
	index int,
	call func(*localstatequery.Client) error,
) []byte {
	t.Helper()
	var mu sync.Mutex
	var sent []byte
	// The capture hands the decoded message back as the expected one, so that
	// the mock accepts it and the bytes recorded are the client's own.
	expected := localstatequery.NewMsgQuery(nil)
	conversation[index] = ouroboros_mock.ConversationEntryInput{
		ProtocolId: localstatequery.ProtocolId,
		Message:    expected,
		MsgFromCborFunc: func(
			msgType uint,
			data []byte,
		) (protocol.Message, error) {
			mu.Lock()
			sent = append([]byte{}, data...)
			mu.Unlock()
			return expected, nil
		},
	}
	if index+1 < len(conversation) {
		conversation = conversation[:index+1]
	}
	conversation = append(conversation, ouroboros_mock.ConversationEntryOutput{
		ProtocolId: localstatequery.ProtocolId,
		IsResponse: true,
		Messages: []protocol.Message{
			localstatequery.NewMsgResult([]byte{0x81, 0x00}),
		},
	})
	runTest(t, conversation, func(t *testing.T, oConn *ouroboros.Connection) {
		// The reply is a placeholder; only the request is under test.
		_ = call(oConn.LocalStateQuery().Client)
	})
	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, sent, "the client sent no query")
	var msg struct {
		cbor.StructAsArray
		Type  uint
		Query cbor.RawMessage
	}
	_, err := cbor.Decode(sent, &msg)
	require.NoError(t, err)
	require.Equal(t, uint(localstatequery.MessageTypeQuery), msg.Type)
	return msg.Query
}

type referenceClientQuery struct {
	name string
	// fromEra is the first era whose golden the client must reproduce, and
	// toEra the last (inclusive).
	fromEra, toEra int
	call           func(*localstatequery.Client) error
}

// referenceClientQueries are the client calls whose arguments can be those of
// the examples. Where a call's eras are narrower than the golden's, the
// reason is given next to the call.
func referenceClientQueries(t *testing.T) []referenceClientQuery {
	t.Helper()
	all := func(name string, call func(*localstatequery.Client) error) referenceClientQuery {
		return referenceClientQuery{name: name, fromEra: 1, toEra: 7, call: call}
	}
	conway := func(name string, call func(*localstatequery.Client) error) referenceClientQuery {
		return referenceClientQuery{name: name, fromEra: referenceConwayIndex, toEra: 7, call: call}
	}
	pool := ledger.PoolId(referenceHex28(t, referenceExamplePool))
	nonMyopicKeys := []localstatequery.NonMyopicMemberRewardsKey{
		{Coin: 100},
		{
			IsCredential: true,
			Credential: localstatequery.StakeCredential{
				Tag:   1,
				Bytes: ledger.NewBlake2b224(referenceHex(t, referenceExampleScriptCred)),
			},
		},
		{
			IsCredential: true,
			Credential: localstatequery.StakeCredential{
				Tag:   0,
				Bytes: ledger.NewBlake2b224(referenceHex(t, referenceExampleKeyCred)),
			},
		},
	}
	return []referenceClientQuery{
		all("GetEpochNo", func(c *localstatequery.Client) error {
			_, err := c.GetEpochNo()
			return err
		}),
		all("GetNonMyopicMemberRewards", func(c *localstatequery.Client) error {
			_, err := c.GetNonMyopicMemberRewards(nonMyopicKeys)
			return err
		}),
		all("GetCurrentPParams", func(c *localstatequery.Client) error {
			_, err := c.GetCurrentProtocolParams()
			return err
		}),
		// The address and input filters are sent without the set tag; from
		// Conway the node writes the tag, which it also accepts without.
		{name: "GetUTxOByAddress", fromEra: 1, toEra: 5, call: func(c *localstatequery.Client) error {
			_, err := c.GetUTxOByAddress([]ledger.Address{})
			return err
		}},
		all("GetUTxOWhole", func(c *localstatequery.Client) error {
			_, err := c.GetUTxOWhole()
			return err
		}),
		all("DebugEpochState", func(c *localstatequery.Client) error {
			_, err := c.DebugEpochState()
			return err
		}),
		// Sent with the set tag in every era; before Conway the node writes
		// a plain array.
		conway("GetFilteredDelegationsAndRewardAccounts", func(c *localstatequery.Client) error {
			_, err := c.GetFilteredDelegationsAndRewardAccounts(nil)
			return err
		}),
		all("GetGenesisConfig", func(c *localstatequery.Client) error {
			_, err := c.GetGenesisConfig()
			return err
		}),
		all("DebugNewEpochState", func(c *localstatequery.Client) error {
			_, err := c.DebugNewEpochState()
			return err
		}),
		all("DebugChainDepState", func(c *localstatequery.Client) error {
			_, err := c.DebugChainDepState()
			return err
		}),
		all("GetRewardProvenance", func(c *localstatequery.Client) error {
			_, err := c.GetRewardProvenance()
			return err
		}),
		{name: "GetUTxOByTxIn", fromEra: 1, toEra: 5, call: func(c *localstatequery.Client) error {
			_, err := c.GetUTxOByTxIn([]ledger.TransactionInput{})
			return err
		}},
		all("GetStakePools", func(c *localstatequery.Client) error {
			_, err := c.GetStakePools()
			return err
		}),
		all("GetStakePoolParams", func(c *localstatequery.Client) error {
			_, err := c.GetStakePoolParams(nil)
			return err
		}),
		all("GetRewardInfoPools", func(c *localstatequery.Client) error {
			_, err := c.GetRewardInfoPools()
			return err
		}),
		all("GetPoolState", func(c *localstatequery.Client) error {
			_, err := c.GetPoolState(nil)
			return err
		}),
		all("GetStakeSnapshots", func(c *localstatequery.Client) error {
			_, err := c.GetStakeSnapshots(nil)
			return err
		}),
		all("GetStakeDelegDeposits", func(c *localstatequery.Client) error {
			_, err := c.GetStakeDelegDeposits(nil)
			return err
		}),
		// GovStateResult decodes the Conway governance state only, so the
		// client refuses earlier eras.
		conway("GetGovState", func(c *localstatequery.Client) error {
			_, err := c.GetGovState()
			return err
		}),
		// Refused before Conway; the reference answers it in every era.
		conway("GetAccountState", func(c *localstatequery.Client) error {
			_, err := c.GetAccountState()
			return err
		}),
		all("GetBigLedgerPeerSnapshot", func(c *localstatequery.Client) error {
			_, err := c.GetLedgerPeerSnapshot(localstatequery.LedgerPeerKindBig)
			return err
		}),
		all("GetAllLedgerPeerSnapshot", func(c *localstatequery.Client) error {
			_, err := c.GetLedgerPeerSnapshot(localstatequery.LedgerPeerKindAll)
			return err
		}),
		all("GetPoolDistr2", func(c *localstatequery.Client) error {
			_, err := c.GetPoolDistr2(nil)
			return err
		}),
		all("GetStakeDistribution2", func(c *localstatequery.Client) error {
			_, err := c.GetStakeDistribution2()
			return err
		}),
		all("GetMaxMajorProtocolVersion", func(c *localstatequery.Client) error {
			_, err := c.GetMaxMajorProtocolVersion()
			return err
		}),
		conway("GetConstitution", func(c *localstatequery.Client) error {
			_, err := c.GetConstitution()
			return err
		}),
		conway("GetDRepState", func(c *localstatequery.Client) error {
			_, err := c.GetDRepState(nil)
			return err
		}),
		conway("GetDRepStakeDistr", func(c *localstatequery.Client) error {
			_, err := c.GetDRepStakeDistr(nil)
			return err
		}),
		conway("GetCommitteeMembersState", func(c *localstatequery.Client) error {
			_, err := c.GetCommitteeMembersState(nil, nil, nil)
			return err
		}),
		conway("GetFilteredVoteDelegatees", func(c *localstatequery.Client) error {
			_, err := c.GetFilteredVoteDelegatees(nil)
			return err
		}),
		conway("GetSPOStakeDistr", func(c *localstatequery.Client) error {
			_, err := c.GetSPOStakeDistr(nil)
			return err
		}),
		conway("GetProposals", func(c *localstatequery.Client) error {
			_, err := c.GetProposals()
			return err
		}),
		conway("GetRatifyState", func(c *localstatequery.Client) error {
			_, err := c.GetRatifyState()
			return err
		}),
		conway("GetDRepDelegations", func(c *localstatequery.Client) error {
			_, err := c.GetDRepDelegations(nil)
			return err
		}),
		conway("QueryStakePoolDefaultVote", func(c *localstatequery.Client) error {
			_, err := c.GetStakePoolDefaultVote(pool)
			return err
		}),
	}
}

// TestReferenceClientQueryBytes checks that the client, talking to a version
// 23 node in each era, sends byte for byte the query the reference encodes
// for the same arguments.
func TestReferenceClientQueryBytes(t *testing.T) {
	for _, tc := range referenceClientQueries(t) {
		for era := tc.fromEra; era <= tc.toEra; era++ {
			t.Run(tc.name+"/"+referenceEras[era], func(t *testing.T) {
				raw := readReferenceGolden(
					t,
					referenceCardanoN2C,
					"Query_"+referenceEras[era]+"_"+tc.name,
				)
				sent := captureBlockQuery(t, era, tc.call)
				require.Equal(
					t,
					hex.EncodeToString(blockQueryGolden(raw)),
					hex.EncodeToString(sent),
				)
			})
		}
	}
}

// TestReferenceClientEraHistoryQueryBytes checks GetEraHistory sends the
// hard-fork GetInterpreter golden.
func TestReferenceClientEraHistoryQueryBytes(t *testing.T) {
	raw := readReferenceGolden(t, referenceCardanoN2C, "Query_HardFork")
	sent := captureTopLevelQuery(t, func(c *localstatequery.Client) error {
		_, err := c.GetEraHistory()
		return err
	})
	require.Equal(t, hex.EncodeToString(blockQueryGolden(raw)), hex.EncodeToString(sent))
}

// runReferenceResult answers a block query in era with the golden result and
// returns what the client call returned.
func runReferenceResult[T any](
	t *testing.T,
	era int,
	result []byte,
	call func(*localstatequery.Client) (T, error),
) (T, error) {
	t.Helper()
	var got T
	var gotErr error
	runTest(
		t,
		conversationEraQueryAt(referenceN2CVersion, byte(era), result),
		func(t *testing.T, oConn *ouroboros.Connection) {
			got, gotErr = call(oConn.LocalStateQuery().Client)
		},
	)
	return got, gotErr
}

// unwrapEraResult strips the one-element array the hard-fork combinator wraps
// a QueryIfCurrent answer in (Right in EitherMismatch).
func unwrapEraResult(t *testing.T, raw []byte) []byte {
	t.Helper()
	var wrapped []cbor.RawMessage
	_, err := cbor.Decode(raw, &wrapped)
	require.NoError(t, err)
	require.Len(t, wrapped, 1)
	return wrapped[0]
}

// The block hash the peer snapshot examples carry: the ASCII text
// "<BLOCK HASH, padded to 32 bytes>" (Test.Consensus.Shelley.Examples).
const referenceSnapshotBlockHash = "<BLOCK HASH, padded to 32 bytes>"

// TestReferenceResultGoldens answers each client call with the result golden
// of every era, checks the decoded value against the examples
// (Test.Consensus.Shelley.Examples results), and checks it re-encodes to the
// golden.
func TestReferenceResultGoldens(t *testing.T) {
	for era := 1; era < len(referenceEras); era++ {
		eraName := referenceEras[era]
		golden := func(t *testing.T, name string) []byte {
			t.Helper()
			return readReferenceGolden(
				t,
				referenceCardanoN2C,
				"Result_"+eraName+"_"+name,
			)
		}
		t.Run(eraName+"/MaxMajorProtocolVersion", func(t *testing.T) {
			raw := golden(t, "MaxMajorProtocolVersion")
			got, err := runReferenceResult(t, era, raw,
				func(c *localstatequery.Client) (uint64, error) {
					return c.GetMaxMajorProtocolVersion()
				})
			require.NoError(t, err)
			// MaxMajorProtVer (maxBound @Version); cardano-ledger-binary
			// Version.hs: type MaxVersion = 13.
			require.Equal(t, uint64(13), got)
		})
		t.Run(eraName+"/LedgerTip", func(t *testing.T) {
			raw := golden(t, "LedgerTip")
			var point pcommon.Point
			_, err := cbor.Decode(unwrapEraResult(t, raw), &point)
			require.NoError(t, err)
			// blockPoint of the example block, whose header is at slot 9
			// (Test.Cardano.Protocol.TPraos.Examples exampleBlockHeader).
			require.Equal(t, uint64(9), point.Slot)
			require.Len(t, point.Hash, 32)
			reencoded, err := cbor.Encode([]any{point})
			require.NoError(t, err)
			require.Equal(t, raw, reencoded)
		})
		t.Run(eraName+"/GenesisConfig", func(t *testing.T) {
			raw := golden(t, "GenesisConfig")
			got, err := runReferenceResult(t, era, raw,
				func(c *localstatequery.Client) (*localstatequery.GenesisConfigResult, error) {
					return c.GetGenesisConfig()
				})
			require.NoError(t, err)
			// compactGenesis of testShelleyGenesis
			// (Test.Cardano.Ledger.Shelley.Examples).
			require.Equal(t, 10, got.SecurityParam)
			require.Equal(t, 100, got.EpochLength)
			require.Equal(t, 20, got.SlotsPerKESPeriod)
			require.Equal(t, 10, got.MaxKESEvolutions)
			require.Equal(t, 5, got.UpdateQuorum)
			require.Equal(t, int64(45_000_000_000_000_000), got.MaxLovelaceSupply)
			reencoded, err := cbor.Encode([]any{got})
			require.NoError(t, err)
			require.Equal(t, hex.EncodeToString(raw), hex.EncodeToString(reencoded))
		})
		for _, tc := range []struct {
			name    string
			kind    localstatequery.LedgerPeerKind
			version uint64
			origin  bool
		}{
			{"GetBigLedgerPeerSnapshot", localstatequery.LedgerPeerKindBig, 2, false},
			{"GetAllLedgerPeerSnapshot", localstatequery.LedgerPeerKindAll, 3, false},
			{"GetBigLedgerPeerSnapshotAtOrigin", localstatequery.LedgerPeerKindBig, 2, true},
		} {
			t.Run(eraName+"/"+tc.name, func(t *testing.T) {
				raw := golden(t, tc.name)
				got, err := runReferenceResult(t, era, raw,
					func(c *localstatequery.Client) (*localstatequery.LedgerPeerSnapshotResult, error) {
						return c.GetLedgerPeerSnapshot(tc.kind)
					})
				require.NoError(t, err)
				require.Equal(t, tc.version, got.Version)
				require.Equal(t, uint32(42), got.NetworkMagic)
				if tc.origin {
					require.False(t, got.Slot.HasSlot)
					require.Empty(t, got.Pools)
				} else {
					require.True(t, got.Slot.HasSlot)
					require.Equal(t, uint64(42), got.Slot.Slot)
					require.Equal(t, referenceSnapshotBlockHash, string(got.BlockHash))
					require.Len(t, got.Pools, 1)
					pool := got.Pools[0]
					require.Zero(t, big.NewRat(9, 10).Cmp(pool.Detail.PoolStake.Rat))
					if tc.kind == localstatequery.LedgerPeerKindBig {
						require.Zero(t, big.NewRat(9, 10).Cmp(pool.AccumulatedStake.Rat))
					} else {
						require.Nil(t, pool.AccumulatedStake)
					}
					require.Len(t, pool.Detail.Relays, 1)
					relay := pool.Detail.Relays[0]
					require.Equal(t, "1.1.1.1", relay.IPv4.String())
					require.NotNil(t, relay.Port)
					require.Equal(t, uint16(1234), *relay.Port)
				}
				reencoded, err := cbor.Encode([]any{got})
				require.NoError(t, err)
				require.Equal(t, hex.EncodeToString(raw), hex.EncodeToString(reencoded))
			})
		}
	}
}

// TestReferenceShelleyResultGoldensMatchCardano checks the single-era Shelley
// block's results are the Cardano block's Shelley results without the
// one-element wrapper, so the per-era tests above cover them.
func TestReferenceShelleyResultGoldensMatchCardano(t *testing.T) {
	entries, err := os.ReadDir(referenceShelleyN2C)
	require.NoError(t, err)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "Result_") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			raw := readReferenceGolden(t, referenceShelleyN2C, name)
			cardano := readReferenceGolden(
				t,
				referenceCardanoN2C,
				"Result_Shelley_"+strings.TrimPrefix(name, "Result_"),
			)
			require.Equal(t, cardano, append([]byte{0x81}, raw...))
		})
	}
}

// TestReferenceEraHistoryResultGolden answers GetEraHistory with the
// hard-fork GetInterpreter golden: the summary the examples reconstruct from a
// Byron ledger state with the Shelley transition known at epoch 10
// (Test.Consensus.Cardano.Examples: summary, shelleyTransitionEpoch).
func TestReferenceEraHistoryResultGolden(t *testing.T) {
	raw := readReferenceGolden(t, referenceCardanoN2C, "Result_HardFork")
	var got []localstatequery.EraHistoryResult
	var gotErr error
	runTest(
		t,
		append(
			conversationEraQueryAt(referenceN2CVersion, 0, nil)[:4],
			ouroboros_mock.ConversationEntryInput{
				ProtocolId:  localstatequery.ProtocolId,
				MessageType: localstatequery.MessageTypeQuery,
			},
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: localstatequery.ProtocolId,
				IsResponse: true,
				Messages:   []protocol.Message{localstatequery.NewMsgResult(raw)},
			},
		),
		func(t *testing.T, oConn *ouroboros.Connection) {
			got, gotErr = oConn.LocalStateQuery().Client.GetEraHistory()
		},
	)
	require.NoError(t, gotErr)
	// The Byron era, which ends at the transition, and the Shelley era,
	// which ends where the safe zone stops.
	require.Len(t, got, 2)
	require.Equal(t, 0, got[0].Begin.SlotNo)
	require.Equal(t, 0, got[0].Begin.EpochNo)
	require.False(t, got[0].End.Unbounded)
	require.Equal(t, 10, got[0].End.EpochNo)
	require.Equal(t, got[0].End.SlotNo, got[1].Begin.SlotNo)
	require.Equal(t, got[0].End.EpochNo, got[1].Begin.EpochNo)
	// Shelley's epoch length and slot length (milliseconds) come from
	// testShelleyGenesis: 100 slots of 2 s.
	require.Equal(t, 100, got[1].Params.EpochLength)
	require.Equal(t, 2000, got[1].Params.SlotLength)

	// The reference writes the summary as an indefinite-length list; each
	// era summary re-encodes to its own bytes.
	var items []cbor.RawMessage
	_, err := cbor.Decode(raw, &items)
	require.NoError(t, err)
	if len(items) != len(got) {
		t.Fatalf("summary has %d items, want %d", len(items), len(got))
	}
	for i := range got {
		reencoded, err := cbor.Encode(got[i])
		require.NoError(t, err)
		require.Equal(t, hex.EncodeToString(items[i]), hex.EncodeToString(reencoded))
	}
}

// TestReferenceEraMismatchResultGoldens answers an era query with the
// reference's era-mismatch reply (Left in EitherMismatch), which a client
// must report as an error rather than decode as an answer.
func TestReferenceEraMismatchResultGoldens(t *testing.T) {
	for _, name := range []string{"EraMismatchByron", "EraMismatchShelley"} {
		t.Run(name, func(t *testing.T) {
			raw := readReferenceGolden(t, referenceCardanoN2C, "Result_"+name)
			_, err := runReferenceResult(t, 1, raw,
				func(c *localstatequery.Client) (uint64, error) {
					return c.GetMaxMajorProtocolVersion()
				})
			require.Error(t, err)
		})
	}
}

// TestReferenceGoldenFilesAreVerbatim guards the vendored files: each one
// matches the SHA-256 recorded from the reference checkout, and no file is
// missing from or added to that list.
func TestReferenceGoldenFilesAreVerbatim(t *testing.T) {
	readme, err := os.ReadFile("testdata/README.md")
	require.NoError(t, err)
	require.Contains(t, string(readme), "ec239d57f618d698b6350b48bf017b02c0c96a5f")
	sums, err := os.ReadFile("testdata/ouroboros-consensus.sha256")
	require.NoError(t, err)
	want := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(sums)), "\n") {
		sum, path, ok := strings.Cut(line, "  ")
		require.True(t, ok, line)
		want[path] = sum
	}
	got := map[string]string{}
	root := "testdata/ouroboros-consensus"
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		got[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func referenceHex28(t *testing.T, s string) [28]byte {
	t.Helper()
	var out [28]byte
	raw := referenceHex(t, s)
	require.Len(t, raw, 28)
	copy(out[:], raw)
	return out
}
