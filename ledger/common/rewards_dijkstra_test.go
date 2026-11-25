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

package common_test

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

type dijkstraRewardPool struct {
	stake  uint64
	pledge uint64
	margin common.GenesisRat
}

func dijkstraRewardSnapshot(
	pools map[common.PoolKeyHash]dijkstraRewardPool,
) common.RewardSnapshot {
	snapshot := common.RewardSnapshot{
		TotalActiveStake: 1_000,
		PoolStake:        make(map[common.PoolKeyHash]uint64, len(pools)),
		DelegatorStake: make(
			map[common.PoolKeyHash]map[common.AddrKeyHash]uint64,
			len(pools),
		),
		PoolParams: make(
			map[common.PoolKeyHash]*common.PoolRegistrationCertificate,
			len(pools),
		),
		StakeRegistrations: map[common.AddrKeyHash]bool{},
		PoolBlocks:         map[common.PoolKeyHash]uint32{},
		TotalBlocksInEpoch: 1_000,
	}
	// Each pool makes blocks in proportion to its stake, so its apparent
	// performance is 1. Its owner, whose key is the pool id's first byte
	// plus 0x10, delegates exactly the pledge and receives the leader
	// reward; the member, keyed by the pool id's first byte, delegates the
	// rest.
	for poolID, pool := range pools {
		member := common.AddrKeyHash{poolID[0]}
		owner := common.AddrKeyHash{poolID[0] + 0x10}
		snapshot.PoolBlocks[poolID] = uint32(pool.stake) // #nosec G115 -- test stakes are small
		snapshot.PoolStake[poolID] = pool.stake
		snapshot.DelegatorStake[poolID] = map[common.AddrKeyHash]uint64{
			member: pool.stake - pool.pledge,
		}
		if pool.pledge > 0 {
			snapshot.DelegatorStake[poolID][owner] = pool.pledge
		}
		snapshot.StakeRegistrations[member] = true
		snapshot.StakeRegistrations[owner] = true
		snapshot.PoolParams[poolID] = &common.PoolRegistrationCertificate{
			Margin:        pool.margin,
			Pledge:        pool.pledge,
			PoolOwners:    []common.AddrKeyHash{owner},
			RewardAccount: owner,
		}
	}
	return snapshot
}

func TestCalculateRewardsAppliesDijkstraPledgeLeverage(t *testing.T) {
	t.Parallel()

	underPledged := common.PoolKeyHash{1}
	wellPledged := common.PoolKeyHash{2}
	snapshot := dijkstraRewardSnapshot(
		map[common.PoolKeyHash]dijkstraRewardPool{
			underPledged: {
				stake:  400,
				pledge: 1,
				margin: common.NewGenesisRat(0, 1),
			},
			wellPledged: {
				stake:  400,
				pledge: 400,
				margin: common.NewGenesisRat(0, 1),
			},
		},
	)
	pots := common.AdaPots{Rewards: 100_000}
	baseParams := common.RewardParameters{
		DesiredPools: 10,
		ProtocolVersion: common.ProtocolParametersProtocolVersion{
			Major: common.ProtocolVersionDijkstra,
		},
		PoolInfluence: big.NewRat(3, 10),
	}

	baseline, err := common.CalculateRewards(pots, snapshot, baseParams)
	require.NoError(t, err)
	withLeverage := baseParams
	withLeverage.MaxPledgeLeverage = big.NewRat(10, 1)
	limited, err := common.CalculateRewards(pots, snapshot, withLeverage)
	require.NoError(t, err)

	require.Less(
		t,
		limited.PoolRewards[underPledged].TotalRewards,
		baseline.PoolRewards[underPledged].TotalRewards,
		"pledge leverage must cap the under-pledged pool's reward share",
	)
	// maxPool' caps only the under-pledged pool; the amount it loses
	// returns to the reserves rather than to other pools.
	require.Equal(
		t,
		baseline.PoolRewards[wellPledged].TotalRewards,
		limited.PoolRewards[wellPledged].TotalRewards,
	)
	require.Greater(t, limited.Undistributed, baseline.Undistributed)
}

func TestCalculateRewardsZeroPledgeHasNoEligibleShare(t *testing.T) {
	t.Parallel()

	poolID := common.PoolKeyHash{1}
	snapshot := dijkstraRewardSnapshot(
		map[common.PoolKeyHash]dijkstraRewardPool{
			poolID: {
				stake:  1_000,
				pledge: 0,
				margin: common.NewGenesisRat(0, 1),
			},
		},
	)
	pots := common.AdaPots{Reserves: 1_000_000, Rewards: 100_000}
	result, err := common.CalculateRewards(
		pots,
		snapshot,
		common.RewardParameters{
			DesiredPools: 10,
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: common.ProtocolVersionDijkstra,
			},
			PoolInfluence:     big.NewRat(3, 10),
			MaxPledgeLeverage: big.NewRat(10, 1),
		},
	)
	// A zero pledge caps the pool's stake at 0 under leverage, so maxPool'
	// is 0 and the whole pot returns to the reserves.
	require.NoError(t, err)
	require.Zero(t, result.PoolRewards[poolID].TotalRewards)
	require.Equal(t, common.AdaPots{Reserves: 1_100_000}, result.UpdatedPots)

	zeroPledge := common.PoolKeyHash{1}
	pledged := common.PoolKeyHash{2}
	snapshot = dijkstraRewardSnapshot(map[common.PoolKeyHash]dijkstraRewardPool{
		zeroPledge: {stake: 500, pledge: 0, margin: common.NewGenesisRat(0, 1)},
		pledged: {
			stake:  500,
			pledge: 500,
			margin: common.NewGenesisRat(0, 1),
		},
	})
	result, err = common.CalculateRewards(
		pots,
		snapshot,
		common.RewardParameters{
			DesiredPools: 10,
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: common.ProtocolVersionDijkstra,
			},
			PoolInfluence:     big.NewRat(3, 10),
			MaxPledgeLeverage: big.NewRat(10, 1),
		},
	)
	require.NoError(t, err)
	require.Zero(t, result.PoolRewards[zeroPledge].TotalRewards)
	// sigma and the relative pledge are both 0.5, capped at z0 = 1/10:
	// floor(100000 / 1.3 * (0.1 + 0.1 * 0.3 * 1)) = 10000.
	require.Equal(t, uint64(10_000), result.PoolRewards[pledged].TotalRewards)
}

func TestCalculateRewardsPledgeLeverageIsInactiveBeforeDijkstra(t *testing.T) {
	t.Parallel()

	pool1 := common.PoolKeyHash{1}
	pool2 := common.PoolKeyHash{2}
	snapshot := dijkstraRewardSnapshot(
		map[common.PoolKeyHash]dijkstraRewardPool{
			pool1: {stake: 400, pledge: 1, margin: common.NewGenesisRat(0, 1)},
			pool2: {
				stake:  400,
				pledge: 400,
				margin: common.NewGenesisRat(0, 1),
			},
		},
	)
	pots := common.AdaPots{Rewards: 100_000}
	params := common.RewardParameters{
		DesiredPools:    10,
		ProtocolVersion: common.ProtocolParametersProtocolVersion{Major: 11},
		PoolInfluence:   big.NewRat(3, 10),
	}
	withoutLeverage, err := common.CalculateRewards(pots, snapshot, params)
	require.NoError(t, err)
	params.MaxPledgeLeverage = big.NewRat(10, 1)
	withLeverage, err := common.CalculateRewards(pots, snapshot, params)
	require.NoError(t, err)
	require.Equal(t, withoutLeverage, withLeverage)
}

func TestCalculateRewardsPledgeLeverageBelowAndAtCap(t *testing.T) {
	t.Parallel()

	pool1 := common.PoolKeyHash{1}
	pool2 := common.PoolKeyHash{2}
	for _, test := range []struct {
		name   string
		stake  uint64
		pledge uint64
	}{
		{name: "below cap", stake: 30, pledge: 4},
		{name: "at cap", stake: 40, pledge: 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := dijkstraRewardSnapshot(
				map[common.PoolKeyHash]dijkstraRewardPool{
					pool1: {
						stake: test.stake, pledge: test.pledge,
						margin: common.NewGenesisRat(0, 1),
					},
					pool2: {
						stake: 30, pledge: 30,
						margin: common.NewGenesisRat(0, 1),
					},
				},
			)
			params := common.RewardParameters{
				DesiredPools: 10,
				ProtocolVersion: common.ProtocolParametersProtocolVersion{
					Major: common.ProtocolVersionDijkstra,
				},
				PoolInfluence: big.NewRat(3, 10),
			}
			baseline, err := common.CalculateRewards(
				common.AdaPots{Rewards: 100_000}, snapshot, params,
			)
			require.NoError(t, err)
			params.MaxPledgeLeverage = big.NewRat(10, 1)
			capped, err := common.CalculateRewards(
				common.AdaPots{Rewards: 100_000}, snapshot, params,
			)
			require.NoError(t, err)
			require.Equal(t, baseline.PoolRewards, capped.PoolRewards)
		})
	}
}

func TestCalculateRewardsPledgeLeverageKeepsGlobalSaturation(t *testing.T) {
	t.Parallel()

	poolIDs := []common.PoolKeyHash{{1}, {2}}
	snapshot := dijkstraRewardSnapshot(
		map[common.PoolKeyHash]dijkstraRewardPool{
			poolIDs[0]: {
				stake:  400,
				pledge: 400,
				margin: common.NewGenesisRat(0, 1),
			},
			poolIDs[1]: {
				stake:  100,
				pledge: 100,
				margin: common.NewGenesisRat(0, 1),
			},
		},
	)
	result, err := common.CalculateRewards(
		common.AdaPots{Rewards: 100_000},
		snapshot,
		common.RewardParameters{
			DesiredPools: 10,
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: common.ProtocolVersionDijkstra,
			},
			PoolInfluence:     new(big.Rat),
			MaxPledgeLeverage: big.NewRat(10_000, 1),
		},
	)
	// Both pools are at or above z0 = 1/10 of the stake, so with a0 = 0
	// each earns R * z0 whatever its pledge leverage allows.
	require.NoError(t, err)
	require.Equal(
		t,
		uint64(10_000),
		result.PoolRewards[poolIDs[0]].TotalRewards,
	)
	require.Equal(
		t,
		uint64(10_000),
		result.PoolRewards[poolIDs[1]].TotalRewards,
	)
}

func TestCalculateRewardsDijkstraMinPoolMargin(t *testing.T) {
	t.Parallel()

	poolID := common.PoolKeyHash{1}
	calculate := func(
		version uint,
		margin common.GenesisRat,
		floor *big.Rat,
	) common.PoolRewards {
		t.Helper()
		snapshot := dijkstraRewardSnapshot(
			map[common.PoolKeyHash]dijkstraRewardPool{
				poolID: {stake: 1_000, pledge: 100, margin: margin},
			},
		)
		result, err := common.CalculateRewards(
			common.AdaPots{Rewards: 100_000},
			snapshot,
			common.RewardParameters{
				DesiredPools: 10,
				ProtocolVersion: common.ProtocolParametersProtocolVersion{
					Major: version,
				},
				PoolInfluence: big.NewRat(3, 10),
				MinPoolMargin: floor,
			},
		)
		require.NoError(t, err)
		return result.PoolRewards[poolID]
	}

	belowFloor := calculate(
		common.ProtocolVersionDijkstraMinPoolMargin,
		common.NewGenesisRat(1, 20),
		big.NewRat(1, 10),
	)
	equalFloor := calculate(
		common.ProtocolVersionDijkstraMinPoolMargin,
		common.NewGenesisRat(1, 10),
		big.NewRat(1, 10),
	)
	aboveFloor := calculate(
		common.ProtocolVersionDijkstraMinPoolMargin,
		common.NewGenesisRat(1, 5),
		big.NewRat(1, 10),
	)
	beforeActivation := calculate(
		common.ProtocolVersionDijkstra,
		common.NewGenesisRat(1, 20),
		big.NewRat(1, 10),
	)
	zeroFloor := calculate(
		common.ProtocolVersionDijkstraMinPoolMargin,
		common.NewGenesisRat(1, 20),
		new(big.Rat),
	)
	unsetFloor := calculate(
		common.ProtocolVersionDijkstraMinPoolMargin,
		common.NewGenesisRat(1, 20),
		nil,
	)

	// The pool earns 10000 (sigma 1 and relative pledge 0.1, both capped at
	// z0 = 1/10, a0 = 3/10). With margin m and owner share 0.1 the leader
	// takes floor(10000 * (m + (1 - m) * 0.1)) and the member
	// floor(10000 * (1 - m) * 0.9).
	require.Equal(t, uint64(1_900), belowFloor.OperatorRewards)
	require.Equal(t, belowFloor, equalFloor)
	require.Equal(t, uint64(2_800), aboveFloor.OperatorRewards)
	require.Equal(t, uint64(1_450), beforeActivation.OperatorRewards)
	require.Equal(t, beforeActivation, zeroFloor)
	require.Equal(t, zeroFloor, unsetFloor)
	member := common.RewardCredential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.CredentialHash{1},
	}
	require.Equal(t, uint64(8_100), belowFloor.DelegatorRewards[member])
	require.Equal(t, uint64(7_200), aboveFloor.DelegatorRewards[member])
	require.Equal(t, uint64(8_550), beforeActivation.DelegatorRewards[member])
}

func TestDijkstraMinPoolMarginDoesNotChangePoolRewardShares(t *testing.T) {
	t.Parallel()

	belowFloor := common.PoolKeyHash{1}
	aboveFloor := common.PoolKeyHash{2}
	snapshot := dijkstraRewardSnapshot(
		map[common.PoolKeyHash]dijkstraRewardPool{
			belowFloor: {
				stake: 400, pledge: 40, margin: common.NewGenesisRat(1, 20),
			},
			aboveFloor: {
				stake: 400, pledge: 40, margin: common.NewGenesisRat(1, 5),
			},
		},
	)
	pots := common.AdaPots{Rewards: 100_000}
	params := common.RewardParameters{
		DesiredPools: 10,
		ProtocolVersion: common.ProtocolParametersProtocolVersion{
			Major: common.ProtocolVersionDijkstraMinPoolMargin,
		},
		PoolInfluence: big.NewRat(3, 10),
	}
	baseline, err := common.CalculateRewards(pots, snapshot, params)
	require.NoError(t, err)

	params.MinPoolMargin = big.NewRat(1, 10)
	withMinimum, err := common.CalculateRewards(pots, snapshot, params)
	require.NoError(t, err)
	require.Equal(
		t,
		baseline.PoolRewards[belowFloor].TotalRewards,
		withMinimum.PoolRewards[belowFloor].TotalRewards,
	)
	require.Equal(
		t,
		baseline.PoolRewards[aboveFloor].TotalRewards,
		withMinimum.PoolRewards[aboveFloor].TotalRewards,
	)
	require.Greater(
		t,
		withMinimum.PoolRewards[belowFloor].OperatorRewards,
		baseline.PoolRewards[belowFloor].OperatorRewards,
	)
	require.Equal(
		t,
		baseline.PoolRewards[aboveFloor].OperatorRewards,
		withMinimum.PoolRewards[aboveFloor].OperatorRewards,
	)
}

func TestCalculateRewardsValidatesDijkstraRewardParameters(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		params common.RewardParameters
	}{
		{
			name: "leverage below one",
			params: common.RewardParameters{
				DesiredPools:      10,
				MaxPledgeLeverage: big.NewRat(1, 2),
			},
		},
		{
			name: "leverage above maximum",
			params: common.RewardParameters{
				DesiredPools:      10,
				MaxPledgeLeverage: big.NewRat(10_001, 1),
			},
		},
		{
			name: "negative minimum margin",
			params: common.RewardParameters{
				DesiredPools:  10,
				MinPoolMargin: big.NewRat(-1, 100),
			},
		},
		{
			name: "minimum margin above one",
			params: common.RewardParameters{
				DesiredPools:  10,
				MinPoolMargin: big.NewRat(101, 100),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.params.ProtocolVersion.Major = common.ProtocolVersionDijkstra
			_, err := common.CalculateRewards(
				common.AdaPots{},
				common.RewardSnapshot{},
				test.params,
			)
			require.Error(t, err)
		})
	}
}
