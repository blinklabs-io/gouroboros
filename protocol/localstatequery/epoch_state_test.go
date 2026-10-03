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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/stretchr/testify/require"
)

// epochStateHex is an EpochState as the ledger encodes it:
//
//	84                 ; array(4)
//	  82 0a 14         ; account state [treasury 10, reserves 20]
//	  82 80 80         ; ledger state [certState, utxoState], left opaque
//	  84 a0 a0 a0 00   ; snapshots [mark, set, go, fee]
//	  82 a0 00         ; non-myopic [likelihoods, reward pot]
const (
	epochStateHex = "84" + "820a14" + "828080" + "84a0a0a000" + "82a000"

	// ledgerStateHex, snapshotsHex and nonMyopicHex are the opaque
	// components of epochStateHex.
	ledgerStateHex = "828080"
	snapshotsHex   = "84a0a0a000"
	nonMyopicHex   = "82a000"
)

func TestDebugEpochStateResultDecodesTypedAndPreservesRaw(t *testing.T) {
	t.Parallel()
	var result DebugEpochStateResult
	_, err := cbor.Decode(mustDecodeHex(t, epochStateHex), &result)
	require.NoError(t, err)
	require.Equal(t, AccountState{Treasury: 10, Reserves: 20}, result.AccountState)
	require.Equal(t, mustDecodeHex(t, ledgerStateHex), []byte(result.LedgerState))
	require.Equal(t, mustDecodeHex(t, snapshotsHex), []byte(result.Snapshots))
	require.Equal(t, mustDecodeHex(t, nonMyopicHex), []byte(result.NonMyopic))
}

func TestDebugEpochStateResultRejectsShortArray(t *testing.T) {
	t.Parallel()
	var result DebugEpochStateResult
	_, err := cbor.Decode(mustDecodeHex(t, "83820a14828080"+"84a0a0a000"), &result)
	require.Error(t, err)
}

// newEpochStateHex is a NewEpochState: [epoch, blocksMadePrev, blocksMadeCur,
// epochState, rewardUpdate, poolDistr, stashedAVVMAddresses].
const newEpochStateHex = "87" +
	"05" +
	"a1581c" + poolIdLowHex + "02" +
	"a1581c" + poolIdHighHex + "03" +
	epochStateHex +
	"80" + // StrictMaybe PulsingRewUpdate, SNothing
	"82a1581c" + poolIdLowHex + "83d81e820102" + "0b" + "5820" + vrfHashHex +
	"0a" + // PoolDistr [individual, total active stake]
	"f6" // stashed AVVM addresses

func TestDebugNewEpochStateResultDecodes(t *testing.T) {
	t.Parallel()
	var result DebugNewEpochStateResult
	_, err := cbor.Decode(mustDecodeHex(t, newEpochStateHex), &result)
	require.NoError(t, err)
	require.Equal(t, uint64(5), result.Epoch)
	low := ledger.NewBlake2b224(mustDecodeHex(t, poolIdLowHex))
	high := ledger.NewBlake2b224(mustDecodeHex(t, poolIdHighHex))
	require.Equal(t, map[ledger.Blake2b224]uint64{low: 2}, result.BlocksMadePrev)
	require.Equal(t, map[ledger.Blake2b224]uint64{high: 3}, result.BlocksMadeCur)
	require.Equal(t, AccountState{Treasury: 10, Reserves: 20}, result.EpochState.AccountState)
	require.Equal(t, mustDecodeHex(t, nonMyopicHex), []byte(result.EpochState.NonMyopic))
	require.Equal(t, mustDecodeHex(t, "80"), []byte(result.RewardUpdate))
	require.Equal(t, uint64(10), result.PoolDistr.TotalActiveStake)
	entry, ok := result.PoolDistr.Pools[ledger.PoolId(low)]
	require.True(t, ok)
	require.Equal(t, uint64(11), entry.TotalPoolStake)
	require.Equal(t, mustDecodeHex(t, "f6"), []byte(result.StashedAVVMAddresses))
}
