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
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fixtures below are the exact LocalStateQuery MsgQuery payloads
// (mini-protocol 7) captured off the wire from cardano-cli 11.0.0.0
// `query stake-snapshot` talking to cardano-node 11.0.1. They decode to:
//
//	[3, [0, [0, [6, [9, [20, poolFilter]]]]]]
//	MsgQuery( Block( Shelley( era=6, GetCBOR( GetStakeSnapshots(poolFilter) ) )))
//
// Modelling ShelleyCborQuery (GetCBOR, tag 9) as a bare [9] made both fail
// to decode with "cannot decode CBOR array to struct with different number
// of elements", closing the NtC connection. See dingo issue #2917.
const (
	// stake-snapshot --stake-pool-id <pool>; poolFilter = [ 258{pool} ]
	stakeSnapshotQueryOnePoolHex = "82038200820082068209821481" +
		"d9010281581c728ea45cc4888f97d1c3233956fd6abf9362854d880efd6e577807f2"
	// stake-snapshot --all-stake-pools; poolFilter = [] (SNothing)
	stakeSnapshotQueryAllPoolsHex = "82038200820082068209821480"
	// The single pool ID carried by the one-pool query.
	stakeSnapshotPoolIDHex = "728ea45cc4888f97d1c3233956fd6abf9362854d880efd6e577807f2"
	// Malformed: poolFilter = [ 258{poolA}, 258{poolB} ]. A StrictMaybe may
	// carry at most one set, so this must be rejected at decode.
	stakeSnapshotQueryTwoSetsHex = "820382008200820682098214" + "82" +
		"d9010281581c728ea45cc4888f97d1c3233956fd6abf9362854d880efd6e577807f2" +
		"d9010281581cccfa09b0c1f3fe9650a11b4d23d5c461df76f6ff10eb95018940984f"
)

// decodeStakeSnapshotsQuery decodes a captured MsgQuery payload and walks the
// wrapper chain down to the GetStakeSnapshots leaf, asserting each layer's
// type, era, and query tag.
func decodeStakeSnapshotsQuery(
	t *testing.T,
	payloadHex string,
) *ShelleyStakeSnapshotsQuery {
	t.Helper()
	data, err := hex.DecodeString(payloadHex)
	require.NoError(t, err)
	msg, err := NewMsgFromCbor(MessageTypeQuery, data)
	require.NoError(t, err, "captured stake-snapshot query must decode (issue #2917)")

	msgQuery, ok := msg.(*MsgQuery)
	require.Truef(t, ok, "expected *MsgQuery, got %T", msg)
	blockQuery, ok := msgQuery.Query.Query.(*BlockQuery)
	require.Truef(t, ok, "expected *BlockQuery, got %T", msgQuery.Query.Query)
	shelleyQuery, ok := blockQuery.Query.(*ShelleyQuery)
	require.Truef(t, ok, "expected *ShelleyQuery, got %T", blockQuery.Query)
	require.Equal(t, uint(6), shelleyQuery.Era, "expected era 6 (Conway)")
	cborQuery, ok := shelleyQuery.Query.(*ShelleyCborQuery)
	require.Truef(t, ok, "expected *ShelleyCborQuery (GetCBOR), got %T", shelleyQuery.Query)
	require.Equal(t, QueryTypeShelleyCbor, cborQuery.Type, "expected GetCBOR tag")
	snapQuery, ok := cborQuery.Query.(*ShelleyStakeSnapshotsQuery)
	require.Truef(t, ok, "expected *ShelleyStakeSnapshotsQuery, got %T", cborQuery.Query)
	require.Equal(
		t,
		QueryTypeShelleyStakeSnapshots,
		snapQuery.Type,
		"expected GetStakeSnapshots tag",
	)
	return snapQuery
}

func TestDecodeStakeSnapshotsQuerySpecificPool(t *testing.T) {
	snapQuery := decodeStakeSnapshotsQuery(t, stakeSnapshotQueryOnePoolHex)
	pools, all := snapQuery.PoolFilter()
	require.False(t, all, "expected a specific-pool filter, not all-pools (SNothing)")
	require.Len(t, pools, 1)

	wantPool, err := hex.DecodeString(stakeSnapshotPoolIDHex)
	require.NoError(t, err)
	assert.Equal(t, stakeSnapshotPoolIDHex, hex.EncodeToString(pools[0][:]), "pool ID mismatch")
	// Sanity: the requested pool must round-trip through ledger.PoolId.
	var wantID ledger.PoolId
	copy(wantID[:], wantPool)
	assert.Equal(t, wantID, pools[0], "pool ID did not round-trip through ledger.PoolId")
}

func TestDecodeStakeSnapshotsQueryAllPools(t *testing.T) {
	snapQuery := decodeStakeSnapshotsQuery(t, stakeSnapshotQueryAllPoolsHex)
	pools, all := snapQuery.PoolFilter()
	require.True(t, all, "expected all-pools (SNothing)")
	assert.Nil(t, pools, "expected nil pool slice for all-pools")
}

// TestDecodeStakeSnapshotsQueryRejectsMultipleSets ensures a malformed
// StrictMaybe carrying more than one pool set is rejected at decode rather
// than silently dropping the extra sets.
func TestDecodeStakeSnapshotsQueryRejectsMultipleSets(t *testing.T) {
	data, err := hex.DecodeString(stakeSnapshotQueryTwoSetsHex)
	require.NoError(t, err)
	_, err = NewMsgFromCbor(MessageTypeQuery, data)
	require.Error(t, err, "a pool filter with more than one set must be rejected")
	assert.Contains(t, err.Error(), "at most one pool set")
}
