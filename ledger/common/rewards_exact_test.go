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

package common

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reference vectors below are chosen so the expected value follows from
// algebra rather than from a second implementation of the same formula.
// When the stakeholder pot is an exact integer multiple k of the total pool
// stake, every stakeholder is owed exactly k times their own stake.

func TestDistributePoolRewardsExactDelegatorShare(t *testing.T) {
	const (
		poolCost   = uint64(340_000_000)
		firstStake = uint64(437_261_796_338)
		totalStake = uint64(812_595_025_641)
	)
	secondStake := totalStake - firstStake

	first := AddrKeyHash{0x03}
	second := AddrKeyHash{0x04}

	// stakeholder pot == total pool stake, so k == 1 and each delegator is
	// owed exactly their own stake. float64 rounds the first share down.
	rewards := distributePoolRewards(
		PoolKeyHash{0x01},
		totalStake+poolCost,
		map[AddrKeyHash]uint64{first: firstStake, second: secondStake},
		&PoolRegistrationCertificate{
			Cost:   poolCost,
			Margin: NewGenesisRat(0, 1),
		},
		RewardSnapshot{StakeRegistrations: map[AddrKeyHash]bool{
			first:  true,
			second: true,
		}},
	)

	assert.Equal(t, firstStake, rewards.DelegatorRewards[first])
	assert.Equal(t, secondStake, rewards.DelegatorRewards[second])
	// No rounding remainder is left over for the operator to absorb.
	assert.Equal(t, poolCost, rewards.OperatorRewards)
}

func TestDistributePoolRewardsExactLargeDenominator(t *testing.T) {
	const (
		poolCost   = uint64(340_000_000)
		firstStake = uint64(251_226_243_903)
		totalStake = uint64(990_637_021_518)
	)
	secondStake := totalStake - firstStake
	// stakeholder pot == 2 * total pool stake, so k == 2.
	stakeholderPot := 2 * totalStake

	first := AddrKeyHash{0x03}
	second := AddrKeyHash{0x04}

	rewards := distributePoolRewards(
		PoolKeyHash{0x01},
		stakeholderPot+poolCost,
		map[AddrKeyHash]uint64{first: firstStake, second: secondStake},
		&PoolRegistrationCertificate{
			Cost:   poolCost,
			Margin: NewGenesisRat(0, 1),
		},
		RewardSnapshot{StakeRegistrations: map[AddrKeyHash]bool{
			first:  true,
			second: true,
		}},
	)

	assert.Equal(t, 2*firstStake, rewards.DelegatorRewards[first])
	assert.Equal(t, 2*secondStake, rewards.DelegatorRewards[second])
	assert.Equal(t, poolCost, rewards.OperatorRewards)
}

// Stake totals on mainnet exceed 2^53 lovelace, above which float64 can no
// longer represent consecutive integers. Two pools whose stake differs by one
// lovelace must not collapse to the same apparent performance.
func TestCalculatePoolPerformanceDistinguishesStakeAbovePow53(t *testing.T) {
	const pow53 = uint64(1) << 53

	low := PoolKeyHash{0x01}
	high := PoolKeyHash{0x02}
	snapshot := RewardSnapshot{
		TotalActiveStake: pow53 + 2,
		PoolStake: map[PoolKeyHash]uint64{
			low:  pow53,
			high: pow53 + 1,
		},
		PoolBlocks: map[PoolKeyHash]uint32{
			low:  1,
			high: 1,
		},
		TotalBlocksInEpoch: 2,
	}

	require.Equal(
		t,
		float64(pow53),
		float64(pow53+1),
		"vector is only meaningful while float64 cannot separate these stakes",
	)

	lowPerf := calculatePoolPerformance(low, snapshot, RewardParameters{})
	highPerf := calculatePoolPerformance(high, snapshot, RewardParameters{})

	// performance = (blocks/totalBlocks) * (totalActiveStake/poolStake)
	wantLow := new(big.Rat).SetFrac(
		new(big.Int).SetUint64(pow53+2),
		new(big.Int).SetUint64(2*pow53),
	)
	wantHigh := new(big.Rat).SetFrac(
		new(big.Int).SetUint64(pow53+2),
		new(big.Int).SetUint64(2*(pow53+1)),
	)

	assert.Zero(t, lowPerf.Cmp(wantLow), "low pool performance")
	assert.Zero(t, highPerf.Cmp(wantHigh), "high pool performance")
	assert.NotZero(
		t,
		lowPerf.Cmp(highPerf),
		"a one lovelace stake difference must change apparent performance",
	)
}

func TestMarginRatIsBoundedAndExact(t *testing.T) {
	for _, test := range []struct {
		name   string
		margin GenesisRat
		want   *big.Rat
	}{
		{name: "missing", margin: GenesisRat{}, want: big.NewRat(0, 1)},
		{name: "negative", margin: NewGenesisRat(-1, 1), want: big.NewRat(0, 1)},
		{name: "zero", margin: NewGenesisRat(0, 1), want: big.NewRat(0, 1)},
		{name: "half", margin: NewGenesisRat(1, 2), want: big.NewRat(1, 2)},
		{name: "one", margin: NewGenesisRat(1, 1), want: big.NewRat(1, 1)},
		{name: "above one", margin: NewGenesisRat(2, 1), want: big.NewRat(1, 1)},
		{
			// 1/3 has no exact float64 representation.
			name:   "one third",
			margin: NewGenesisRat(1, 3),
			want:   big.NewRat(1, 3),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Zero(t, test.want.Cmp(marginRat(test.margin)))
		})
	}
}

// buildDeterminismSnapshot returns a multi-pool snapshot whose reward split
// leaves a rounding remainder, which is where map iteration order used to
// leak into the result.
func buildDeterminismSnapshot() (AdaPots, RewardSnapshot, RewardParameters) {
	poolStakes := []uint64{
		1_234_567_890_123, 9_876_543_210_987, 5_555_555_555_557,
		3_333_333_333_331, 7_777_777_777_771, 2_468_013_579_113,
		8_642_097_531_119, 1_357_913_579_137,
	}
	snapshot := RewardSnapshot{
		PoolStake:          map[PoolKeyHash]uint64{},
		PoolParams:         map[PoolKeyHash]*PoolRegistrationCertificate{},
		DelegatorStake:     map[PoolKeyHash]map[AddrKeyHash]uint64{},
		StakeRegistrations: map[AddrKeyHash]bool{},
		PoolBlocks:         map[PoolKeyHash]uint32{},
		TotalBlocksInEpoch: 21600,
	}
	totalActiveStake := uint64(0)
	for i, stake := range poolStakes {
		poolID := PoolKeyHash{byte(i + 1)}
		owner := AddrKeyHash{byte(100 + i)}
		delegator := AddrKeyHash{byte(200 + i)}
		snapshot.PoolStake[poolID] = stake
		snapshot.PoolBlocks[poolID] = uint32(100 * (i + 1)) // #nosec G115
		snapshot.PoolParams[poolID] = &PoolRegistrationCertificate{
			Cost:       340_000_000,
			Margin:     NewGenesisRat(int64(i+1), 100),
			PoolOwners: []AddrKeyHash{owner},
		}
		snapshot.DelegatorStake[poolID] = map[AddrKeyHash]uint64{
			owner:     stake / 10,
			delegator: stake - stake/10,
		}
		snapshot.StakeRegistrations[owner] = true
		snapshot.StakeRegistrations[delegator] = true
		totalActiveStake += stake
	}
	snapshot.TotalActiveStake = totalActiveStake

	pots := AdaPots{
		Reserves: 13_000_000_000_000_000,
		Treasury: 1_000_000_000_000_000,
		Rewards:  33_000_000_000_000,
	}
	params := RewardParameters{
		PoolInfluence:         big.NewRat(3, 10),
		ActiveSlotsCoeff:      big.NewRat(1, 20),
		ExpectedSlotsPerEpoch: 432000,
		MinPoolCost:           340_000_000,
	}
	return pots, snapshot, params
}

func TestCalculateRewardsIsDeterministic(t *testing.T) {
	pots, snapshot, params := buildDeterminismSnapshot()

	first, err := CalculateRewards(pots, snapshot, params)
	require.NoError(t, err)

	// Map iteration order is randomized per range statement, so repeating the
	// call is enough to surface an order-dependent result.
	for i := range 200 {
		got, err := CalculateRewards(pots, snapshot, params)
		require.NoError(t, err)
		require.Equal(t, first.PoolRewards, got.PoolRewards, "run %d", i)
		require.Equal(t, first.UpdatedPots, got.UpdatedPots, "run %d", i)
	}
}

func TestCalculateRewardsConservesRewardPot(t *testing.T) {
	pots, snapshot, params := buildDeterminismSnapshot()

	result, err := CalculateRewards(pots, snapshot, params)
	require.NoError(t, err)

	distributed := uint64(0)
	for _, poolRewards := range result.PoolRewards {
		perPool := poolRewards.OperatorRewards
		for _, reward := range poolRewards.DelegatorRewards {
			perPool += reward
		}
		assert.Equal(t, poolRewards.TotalRewards, perPool)
		distributed += poolRewards.TotalRewards
	}
	assert.Equal(t, pots.Rewards, distributed)
}
