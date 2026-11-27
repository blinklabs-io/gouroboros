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

// rewardFilterSnapshot has three pools that made blocks. P1 and P2 share the
// reward account S, which is also a member of P2. X is the reward account of
// P3, whose pledge is not met, so P3's leader reward is zero, and a member of
// P1.
func rewardFilterSnapshot(pv uint) (
	common.AdaPots,
	common.RewardSnapshot,
	common.RewardParameters,
) {
	p1 := common.PoolKeyHash{1}
	p2 := common.PoolKeyHash{2}
	p3 := common.PoolKeyHash{3}
	shared := common.AddrKeyHash{9}
	x := common.AddrKeyHash{8}
	m1 := common.AddrKeyHash{7}
	pool := func(account common.AddrKeyHash, pledge uint64) *common.PoolRegistrationCertificate {
		return &common.PoolRegistrationCertificate{
			Cost:          100,
			Margin:        common.NewGenesisRat(1, 10),
			Pledge:        pledge,
			RewardAccount: account,
		}
	}
	snapshot := common.RewardSnapshot{
		TotalStake:       1_000_000,
		TotalActiveStake: 600_000,
		PoolStake: map[common.PoolKeyHash]uint64{
			p1: 200_000, p2: 200_000, p3: 200_000,
		},
		DelegatorStake: map[common.PoolKeyHash]map[common.AddrKeyHash]uint64{
			p1: {m1: 100_000, x: 100_000},
			p2: {shared: 200_000},
			p3: {common.AddrKeyHash{6}: 200_000},
		},
		PoolParams: map[common.PoolKeyHash]*common.PoolRegistrationCertificate{
			p1: pool(shared, 0),
			p2: pool(shared, 0),
			p3: pool(x, 1),
		},
		StakeRegistrations: map[common.AddrKeyHash]bool{
			shared: true, x: true, m1: true, {6}: true,
		},
		PoolBlocks:         map[common.PoolKeyHash]uint32{p1: 10, p2: 10, p3: 10},
		TotalBlocksInEpoch: 30,
	}
	params := common.RewardParameters{
		DesiredPools:    5,
		PoolInfluence:   big.NewRat(3, 10),
		ProtocolVersion: common.ProtocolParametersProtocolVersion{Major: pv},
	}
	return common.AdaPots{Reserves: 10_000_000, Rewards: 1_000_000}, snapshot, params
}

// TestCalculateRewardsPaysOneRewardPerCredentialBeforeVersion3 follows
// filterRewards in cardano-ledger Shelley/Rewards.hs: until
// hardforkAllegraAggregatedRewards (protocol version 3) a credential is paid
// only the least of its rewards under Ord Reward, which puts every leader
// reward before every member reward and orders rewards of one kind by pool
// key hash. A leader reward is computed for every pool that made blocks, zero
// included (collectLRs), and what is not paid returns to the reserves.
func TestCalculateRewardsPaysOneRewardPerCredentialBeforeVersion3(t *testing.T) {
	cred := func(hash common.AddrKeyHash) common.RewardCredential {
		return common.RewardCredential{
			CredType:   common.CredentialTypeAddrKeyHash,
			Credential: common.CredentialHash(hash),
		}
	}
	p1 := common.PoolKeyHash{1}
	p2 := common.PoolKeyHash{2}
	p3 := common.PoolKeyHash{3}
	pots, snapshot, params := rewardFilterSnapshot(3)
	all, err := common.CalculateRewards(pots, snapshot, params)
	require.NoError(t, err)
	leader1 := all.PoolRewards[p1].OperatorRewards
	leader2 := all.PoolRewards[p2].OperatorRewards
	sharedMember := all.PoolRewards[p2].DelegatorRewards[cred(common.AddrKeyHash{9})]
	xMember := all.PoolRewards[p1].DelegatorRewards[cred(common.AddrKeyHash{8})]
	require.NotZero(t, leader1)
	require.NotZero(t, leader2)
	require.NotZero(t, sharedMember)
	require.NotZero(t, xMember)
	require.Zero(t, all.PoolRewards[p3].TotalRewards, "P3 pledge is not met")

	pots, snapshot, params = rewardFilterSnapshot(2)
	got, err := common.CalculateRewards(pots, snapshot, params)
	require.NoError(t, err)
	// S keeps P1's leader reward; its P2 leader and member rewards return.
	require.Equal(t, leader1, got.PoolRewards[p1].OperatorRewards)
	require.Zero(t, got.PoolRewards[p2].OperatorRewards)
	require.NotContains(t, got.PoolRewards[p2].DelegatorRewards, cred(common.AddrKeyHash{9}))
	// X keeps P3's zero leader reward; its P1 member reward returns.
	require.NotContains(t, got.PoolRewards[p1].DelegatorRewards, cred(common.AddrKeyHash{8}))
	// M1, with one reward, is paid as from version 3.
	require.Equal(
		t,
		all.PoolRewards[p1].DelegatorRewards[cred(common.AddrKeyHash{7})],
		got.PoolRewards[p1].DelegatorRewards[cred(common.AddrKeyHash{7})],
	)
	returned := leader2 + sharedMember + xMember
	require.Equal(t, all.TotalRewards-returned, got.TotalRewards)
	require.Equal(t, all.Undistributed+returned, got.Undistributed)
	require.Equal(t, pots.Rewards, got.TotalRewards+got.Undistributed)
	require.Equal(t, pots.Reserves+got.Undistributed, got.UpdatedPots.Reserves)
}

// TestCalculateRewardsDecentralizationFixesPerformance follows
// mkApparentPerformance in cardano-ledger Shelley/Rewards.hs: from d >= 0.8
// every pool's apparent performance is 1, below it the pool's block share
// over its active stake share, and 0 for a pool with no active stake. P1
// made 1/10 of the blocks with 2/10 of the active stake, so it earns half of
// maxPool' below 0.8 and all of it from 0.8.
func TestCalculateRewardsDecentralizationFixesPerformance(t *testing.T) {
	p1 := common.PoolKeyHash{1}
	p2 := common.PoolKeyHash{2}
	p3 := common.PoolKeyHash{3}
	run := func(d *big.Rat) common.RewardCalculationResult {
		snapshot := common.RewardSnapshot{
			TotalStake:       1_000_000,
			TotalActiveStake: 1_000_000,
			PoolStake:        map[common.PoolKeyHash]uint64{p1: 200_000, p2: 800_000},
			DelegatorStake: map[common.PoolKeyHash]map[common.AddrKeyHash]uint64{
				p1: {{1}: 200_000},
				p2: {{2}: 800_000},
			},
			PoolParams: map[common.PoolKeyHash]*common.PoolRegistrationCertificate{
				p1: {Margin: common.NewGenesisRat(0, 1), RewardAccount: common.AddrKeyHash{5}},
				p2: {Margin: common.NewGenesisRat(0, 1), RewardAccount: common.AddrKeyHash{5}},
				p3: {Margin: common.NewGenesisRat(0, 1), RewardAccount: common.AddrKeyHash{5}},
			},
			StakeRegistrations: map[common.AddrKeyHash]bool{{1}: true, {2}: true, {5}: true},
			PoolBlocks:         map[common.PoolKeyHash]uint32{p1: 1, p2: 9, p3: 1},
			TotalBlocksInEpoch: 10,
		}
		got, err := common.CalculateRewards(
			common.AdaPots{Rewards: 1_000_000},
			snapshot,
			common.RewardParameters{
				DesiredPools:         2,
				PoolInfluence:        big.NewRat(0, 1),
				DecentralizationRate: d,
				ProtocolVersion:      common.ProtocolParametersProtocolVersion{Major: 6},
			},
		)
		require.NoError(t, err)
		return *got
	}
	// With a0 = 0, maxPool' for 2/10 of the stake is floor(R * 2/10).
	const maxPool = 200_000
	for _, tc := range []struct {
		d    *big.Rat
		want uint64
	}{
		{big.NewRat(0, 1), maxPool / 2},
		{big.NewRat(79, 100), maxPool / 2},
		{big.NewRat(4, 5), maxPool},
		{big.NewRat(9, 10), maxPool},
	} {
		got := run(tc.d)
		require.Equal(t, tc.want, got.PoolRewards[p1].TotalRewards, "d = %s", tc.d)
		require.Zero(t, got.PoolRewards[p3].TotalRewards, "d = %s: no active stake", tc.d)
	}
}
