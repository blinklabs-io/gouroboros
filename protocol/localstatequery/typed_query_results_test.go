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
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/stretchr/testify/require"
)

// Wire fixtures for the three typed query results in #1691. Every one of them
// is the era codec's single-element result array wrapped around the era's own
// value, the same wrapper GetDRepState carries (#2169).

const (
	poolIdLowHex  = "11111111111111111111111111111111111111111111111111111111"
	poolIdHighHex = "99999999999999999999999999999999999999999999999999999999"
	vrfHashHex    = "3333333333333333333333333333333333333333333333333333333333333333"
	stakeCredHex  = "44444444444444444444444444444444444444444444444444444444"
	delegPoolHex  = "55555555555555555555555555555555555555555555555555555555"
)

// --- GetStakeDistribution (query 5) -----------------------------------------

// stakeDistributionReplyHex is one pool's entry in a GetStakeDistribution
// reply:
//
//	81                      ; result wrapper, array(1)
//	  a1                    ; map(1)
//	    581c 1111..         ; pool key hash
//	    82                  ; array(2), the individual pool stake
//	      d81e 82 01 02     ; tag(30) [1, 2], the stake fraction
//	      5820 3333..       ; VRF key hash
//
// The fraction's denominator is the total circulating supply, not the sum of
// delegated stake: see the raw wire bytes captured from a live cardano-node in
// blinklabs-io/dingo's totalCirculatingSupply. The entry has two elements, not
// the three GetPoolDistr2 (query 36) carries.
const stakeDistributionReplyHex = "81a1581c" + poolIdLowHex +
	"82d81e820102" + "5820" + vrfHashHex

func TestStakeDistributionResultDecodesWrappedMap(t *testing.T) {
	var result StakeDistributionResult
	_, err := cbor.Decode(mustDecodeHex(t, stakeDistributionReplyHex), &result)
	require.NoError(t, err)
	require.Len(t, result.Results, 1)

	poolId := ledger.PoolId(
		ledger.NewBlake2b224(mustDecodeHex(t, poolIdLowHex)),
	)
	entry, ok := result.Results[poolId]
	require.True(t, ok, "pool key hash must survive as the map key")
	require.NotNil(t, entry.StakeFraction)
	require.Equal(t, 0, entry.StakeFraction.Cmp(big.NewRat(1, 2)))
	require.Equal(t, vrfHashHex, hex.EncodeToString(entry.VrfHash[:]))
}

// TestStakeDistributionResultRejectsBareMap pins the wrapper: an unwrapped map
// is the shape GetDRepState was decoded against in #2169.
func TestStakeDistributionResultRejectsBareMap(t *testing.T) {
	bare := "a1581c" + poolIdLowHex + "82d81e820102" + "5820" + vrfHashHex
	var result StakeDistributionResult
	_, err := cbor.Decode(mustDecodeHex(t, bare), &result)
	require.Error(t, err)
}

// TestStakeDistributionResultRejectsPoolDistr2Entry pins the entry width.
// Query 5 is the consensus distribution and its entry is the two-element
// [tag(30) fraction, vrf_hash]. The ledger's own pool distribution (query 36,
// PoolDistr2Result) carries the pool's total stake as a further element, so
// accepting a wider entry here would silently read a PoolDistr2 reply as a
// GetStakeDistribution one.
func TestStakeDistributionResultRejectsPoolDistr2Entry(t *testing.T) {
	threeElement := "81a1581c" + poolIdLowHex +
		"83d81e820102" + "1a000f4240" + "5820" + vrfHashHex
	var result StakeDistributionResult
	_, err := cbor.Decode(mustDecodeHex(t, threeElement), &result)
	require.Error(t, err)
}

// --- GetStakePools (query 16) -----------------------------------------------

// stakePoolsReplyHex is a GetStakePools reply holding two pools:
//
//	81                ; result wrapper, array(1)
//	  d9 0102         ; tag(258), a set
//	    82            ; array(2)
//	      581c 1111.. ; pool key hash
//	      581c 9999.. ; pool key hash
//
// cardano-cli rejects an untagged or unsorted set here ("expected tag" /
// "Canonicity violation while decoding Set"), so the pool ids are in ascending
// byte order.
const stakePoolsReplyHex = "81d9010282581c" + poolIdLowHex +
	"581c" + poolIdHighHex

func TestStakePoolsResultDecodesTaggedSet(t *testing.T) {
	var result StakePoolsResult
	_, err := cbor.Decode(mustDecodeHex(t, stakePoolsReplyHex), &result)
	require.NoError(t, err)
	require.Len(t, result.Results, 2)
	require.Equal(
		t,
		poolIdLowHex,
		hex.EncodeToString(result.Results[0][:]),
	)
	require.Equal(
		t,
		poolIdHighHex,
		hex.EncodeToString(result.Results[1][:]),
	)
}

// TestStakePoolsResultDecodesUntaggedArray covers the untagged form. A
// cardano-node always tags it: GetStakePools encodes through cardano-binary's
// encodeSetSkel, which prepends tag 258 with no version gate. Accepting the
// untagged array is deliberate leniency on the read side, matching the
// ledger's own decoder, which admits the tag rather than requiring it.
func TestStakePoolsResultDecodesUntaggedArray(t *testing.T) {
	untagged := "8182581c" + poolIdLowHex + "581c" + poolIdHighHex
	var result StakePoolsResult
	_, err := cbor.Decode(mustDecodeHex(t, untagged), &result)
	require.NoError(t, err)
	require.Len(t, result.Results, 2)
}

func TestStakePoolsResultRejectsBareSet(t *testing.T) {
	bare := "d9010282581c" + poolIdLowHex + "581c" + poolIdHighHex
	var result StakePoolsResult
	_, err := cbor.Decode(mustDecodeHex(t, bare), &result)
	require.Error(t, err)
}

// --- GetFilteredDelegationsAndRewardAccounts (query 10) ----------------------

// delegationsAndRewardsReplyHex is a reply for one registered, delegated stake
// credential:
//
//	81                       ; result wrapper, array(1)
//	  82                     ; array(2)
//	    a1                   ; delegations, map(1)
//	      82 00 581c 4444..  ; [0, key hash] stake credential
//	      581c 5555..        ; pool key hash
//	    a1                   ; rewards, map(1)
//	      82 00 581c 4444..  ; the same stake credential
//	      1a 000f4240        ; reward balance in lovelace
//
// A registered but undelegated account appears in the rewards map only, so the
// two maps are not required to hold the same keys.
const delegationsAndRewardsReplyHex = "8182" +
	"a1" + "8200581c" + stakeCredHex + "581c" + delegPoolHex +
	"a1" + "8200581c" + stakeCredHex + "1a000f4240"

func TestFilteredDelegationsAndRewardAccountsResultDecodesPopulatedMaps(
	t *testing.T,
) {
	var result FilteredDelegationsAndRewardAccountsResult
	_, err := cbor.Decode(
		mustDecodeHex(t, delegationsAndRewardsReplyHex),
		&result,
	)
	require.NoError(t, err)

	cred := StakeCredential{
		Tag:   0,
		Bytes: ledger.NewBlake2b224(mustDecodeHex(t, stakeCredHex)),
	}
	require.Len(t, result.Delegations, 1)
	pool, ok := result.Delegations[cred]
	require.True(t, ok, "stake credential must survive as the map key")
	require.Equal(t, delegPoolHex, hex.EncodeToString(pool[:]))
	require.Len(t, result.Rewards, 1)
	require.Equal(t, uint64(1000000), result.Rewards[cred])
}

// TestFilteredDelegationsAndRewardAccountsResultRejectsUnwrappedPair pins the
// wrapper: the delegations and rewards pair is nested inside it, not the
// result itself.
func TestFilteredDelegationsAndRewardAccountsResultRejectsUnwrappedPair(
	t *testing.T,
) {
	var result FilteredDelegationsAndRewardAccountsResult
	_, err := cbor.Decode([]byte{0x82, 0xa0, 0xa0}, &result)
	require.Error(t, err)
}

// TestFilteredDelegationsAndRewardAccountsResultRejectsSingleMap pins the
// inner pair: a reply carrying only the delegations map is not a valid answer
// to this query.
func TestFilteredDelegationsAndRewardAccountsResultRejectsSingleMap(
	t *testing.T,
) {
	var result FilteredDelegationsAndRewardAccountsResult
	_, err := cbor.Decode([]byte{0x81, 0xa0}, &result)
	require.Error(t, err)
}
