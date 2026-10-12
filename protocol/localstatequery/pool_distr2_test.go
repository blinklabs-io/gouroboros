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
	"math/big"
	"strings"
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GetPoolDistr2 (Shelley sub-query 36) replaces GetPoolDistr from node-to-client
// protocol version 21. cardano-cli sends it while computing a leadership
// schedule, and a node that cannot even decode the query tag fails the whole
// connection rather than the one query, so the caller sees a closed bearer.
//
// The result is the ledger's own pool distribution rather than the consensus
// one, which carries two things the old result did not: the total stake
// delegated to each pool, and the total active stake across all pools.

func poolDistr2Fixture(t *testing.T) (poolId ledger.PoolId, vrfHash []byte) {
	t.Helper()
	rawPoolId, err := hex.DecodeString(
		"1cb2c9bb54fb097fba58542c5adcf3a7d094db95e8b9c4e3d0bcbf1d",
	)
	if err != nil {
		t.Fatalf("decoding pool id: %v", err)
	}
	rawVrf, err := hex.DecodeString(
		"c0d1f9b040d2f6fd7fc8775d24753d6db4b0fb6846b76f0d8f4b8e8b96ed7d9a",
	)
	if err != nil {
		t.Fatalf("decoding vrf hash: %v", err)
	}
	return ledger.PoolId(ledger.NewBlake2b224(rawPoolId)), rawVrf
}

// TestPoolDistr2QueryDecodesAllPools covers the SNothing pool filter, which the
// ledger reads as "every pool".
func TestPoolDistr2QueryDecodesAllPools(t *testing.T) {
	encoded, err := cbor.Encode(
		[]any{localstatequery.QueryTypeShelleyPoolDistr2, []any{}},
	)
	if err != nil {
		t.Fatalf("encoding query: %v", err)
	}
	var query localstatequery.ShelleyPoolDistr2Query
	if _, err := cbor.Decode(encoded, &query); err != nil {
		t.Fatalf("decoding query: %v", err)
	}
	pools, all := query.PoolFilter()
	if !all {
		t.Errorf("an empty filter covers every pool, got %d pools", len(pools))
	}
}

// TestPoolDistr2QueryDecodesPoolFilter covers the SJust case, where the caller
// asks about specific pools.
func TestPoolDistr2QueryDecodesPoolFilter(t *testing.T) {
	poolId, _ := poolDistr2Fixture(t)
	encoded, err := cbor.Encode(
		[]any{
			localstatequery.QueryTypeShelleyPoolDistr2,
			[]any{cbor.NewSetType([]ledger.PoolId{poolId}, false)},
		},
	)
	if err != nil {
		t.Fatalf("encoding query: %v", err)
	}
	var query localstatequery.ShelleyPoolDistr2Query
	if _, err := cbor.Decode(encoded, &query); err != nil {
		t.Fatalf("decoding query: %v", err)
	}
	pools, all := query.PoolFilter()
	if all {
		t.Fatal("an explicit pool set does not cover every pool")
	}
	if len(pools) != 1 || pools[0] != poolId {
		t.Errorf("pool filter: got %v, want [%v]", pools, poolId)
	}
}

// TestPoolDistr2ResultDecodes pins the reply shape. The ledger's pool
// distribution is a two-field record, and each pool's entry is a three-field
// record; the old query's entry had no total-stake field, so a decoder written
// for it reads the VRF hash out of the wrong slot.
func TestPoolDistr2ResultDecodes(t *testing.T) {
	poolId, vrfHash := poolDistr2Fixture(t)
	encoded, err := cbor.Encode(
		[]any{
			map[any]any{
				poolId: []any{
					&cbor.Rat{Rat: big.NewRat(1, 3)},
					uint64(4200000000),
					vrfHash,
				},
			},
			uint64(12600000000),
		},
	)
	if err != nil {
		t.Fatalf("encoding result: %v", err)
	}
	var result localstatequery.PoolDistr2Result
	if _, err := cbor.Decode(encoded, &result); err != nil {
		t.Fatalf("decoding result: %v", err)
	}
	if result.TotalActiveStake != 12600000000 {
		t.Errorf(
			"total active stake: got %d, want 12600000000",
			result.TotalActiveStake,
		)
	}
	stake, ok := result.Pools[poolId]
	if !ok {
		t.Fatalf("pool %v missing from the distribution", poolId)
	}
	if stake.StakeFraction == nil ||
		stake.StakeFraction.Num().Int64() != 1 ||
		stake.StakeFraction.Denom().Int64() != 3 {
		t.Errorf("stake fraction: got %v, want 1/3", stake.StakeFraction)
	}
	if stake.TotalPoolStake != 4200000000 {
		t.Errorf(
			"total pool stake: got %d, want 4200000000",
			stake.TotalPoolStake,
		)
	}
	if !bytes.Equal(stake.VrfHash[:], vrfHash) {
		t.Errorf("vrf hash: got %x, want %x", stake.VrfHash, vrfHash)
	}
}

// TestPoolDistr2QueryIsDispatchable checks the query reaches a server through
// the same table a real connection uses. Registering the result type without
// registering the tag leaves the node failing the connection on decode.
func TestPoolDistr2QueryIsDispatchable(t *testing.T) {
	blockQuery, err := cbor.Encode(
		[]any{localstatequery.QueryTypeShelleyPoolDistr2, []any{}},
	)
	if err != nil {
		t.Fatalf("encoding query: %v", err)
	}
	// A Shelley leaf query travels wrapped in its era and block-query envelopes.
	wrapped, err := cbor.Encode(
		[]any{
			localstatequery.QueryTypeBlock,
			[]any{
				localstatequery.QueryTypeShelley,
				[]any{ledger.EraIdConway, cbor.RawMessage(blockQuery)},
			},
		},
	)
	if err != nil {
		t.Fatalf("encoding wrapper: %v", err)
	}
	var query localstatequery.QueryWrapper
	if _, err := cbor.Decode(wrapped, &query); err != nil {
		t.Fatalf("decoding wrapped query: %v", err)
	}
	blockQ, ok := query.Query.(*localstatequery.BlockQuery)
	if !ok {
		t.Fatalf("expected a block query, got %T", query.Query)
	}
	shelleyQ, ok := blockQ.Query.(*localstatequery.ShelleyQuery)
	if !ok {
		t.Fatalf("expected a shelley query, got %T", blockQ.Query)
	}
	if _, ok := shelleyQ.Query.(*localstatequery.ShelleyPoolDistr2Query); !ok {
		t.Errorf(
			"expected a GetPoolDistr2 query, got %T",
			shelleyQ.Query,
		)
	}
}

// GetPoolDistr2 replies inside the single-element era result wrapper. Before
// the Dijkstra era each entry has three fields; from it, four, the last being
// the pool's BLS key as a list of at most one [public key, possession proof].
const (
	poolDistr2PoolHex = "1cb2c9bb54fb097fba58542c5adcf3a7d094db95e8b9c4e3d0bcbf1d"
	poolDistr2VrfHex  = "c0d1f9b040d2f6fd7fc8775d24753d6d" +
		"b4b0fb6846b76f0d8f4b8e8b96ed7d9a"
	poolDistr2Prefix = "81 82 a1 58 1c " + poolDistr2PoolHex
	poolDistr2Stake  = " d8 1e 82 01 03 1a 05 f5 e1 00 58 20 " +
		poolDistr2VrfHex
	poolDistr2Total = " 1a 11 e1 a3 00"
)

func poolDistr2BlsKeyHex() string {
	return "82 58 60 " + strings.Repeat("a1", 96) +
		" 58 60 " + strings.Repeat("b2", 96)
}

func TestGetPoolDistr2EraWrappedReply(t *testing.T) {
	cases := []struct {
		name   string
		wire   string
		blsKey string
		// A Dijkstra entry with no key re-encodes in the three-field form.
		reencodes bool
	}{
		{
			name:      "conway",
			wire:      poolDistr2Prefix + " 83" + poolDistr2Stake + poolDistr2Total,
			reencodes: true,
		},
		{
			name: "dijkstra_with_bls_key",
			wire: poolDistr2Prefix + " 84" + poolDistr2Stake +
				" 81 " + poolDistr2BlsKeyHex() + poolDistr2Total,
			blsKey:    poolDistr2BlsKeyHex(),
			reencodes: true,
		},
		{
			name: "dijkstra_without_bls_key",
			wire: poolDistr2Prefix + " 84" + poolDistr2Stake +
				" 80" + poolDistr2Total,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wire := poolDistr2Hex(t, tc.wire)
			runTest(
				t,
				conversationEraQueryAt(23, 6, wire),
				func(t *testing.T, oConn *ouroboros.Connection) {
					result, err := oConn.LocalStateQuery().Client.
						GetPoolDistr2(nil)
					require.NoError(t, err)
					require.NotNil(t, result)
					assert.Equal(
						t,
						uint64(300000000),
						result.TotalActiveStake,
					)
					poolId, vrfHash := poolDistr2Fixture(t)
					stake, ok := result.Pools[poolId]
					require.True(t, ok, "pool missing from the reply")
					require.NotNil(t, stake.StakeFraction)
					assert.Zero(
						t,
						stake.StakeFraction.Cmp(big.NewRat(1, 3)),
					)
					assert.Equal(t, uint64(100000000), stake.TotalPoolStake)
					assert.Equal(t, vrfHash, stake.VrfHash.Bytes())
					assert.Equal(
						t,
						strings.ReplaceAll(tc.blsKey, " ", ""),
						hex.EncodeToString(stake.BlsKey),
					)
					if !tc.reencodes {
						return
					}
					data, err := cbor.Encode(
						localstatequery.StakeDistribution2Result{
							Distribution: *result,
						},
					)
					require.NoError(t, err)
					assert.Equal(t, hex.EncodeToString(wire),
						hex.EncodeToString(data))
				},
			)
		})
	}
}

func poolDistr2Hex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	require.NoError(t, err)
	return b
}
