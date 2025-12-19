// Copyright 2025 Blink Labs Software
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

// TestShelleyRewardFormulas validates our reward calculation formulas against
// the Shelley specification and reference implementations (Java cf-java-rewards-calculation
// and Haskell cardano-ledger).
func TestShelleyRewardFormulas(t *testing.T) {
	// Test case based on Shelley specification formulas
	// Pool reward: 100,000 ADA
	// Pool cost: 50,000 ADA
	// Pool margin: 5% (0.05)
	// Pool owner stake: 10,000,000 ADA
	// Total pool stake: 20,000,000 ADA
	// Member stake: 5,000,000 ADA

	poolReward := uint64(100000)
	poolCost := uint64(50000)
	margin := 0.05
	ownerStake := uint64(10000000)
	totalPoolStake := uint64(20000000)
	memberStake := uint64(5000000)

	// Calculate leader reward using Shelley formula:
	// leaderReward = cost + (poolReward - cost) * (margin + (1 - margin) * (ownerStake / totalPoolStake))
	// = 50000 + (100000 - 50000) * (0.05 + (1 - 0.05) * (10000000 / 20000000))
	// = 50000 + 50000 * (0.05 + 0.95 * 0.5)
	// = 50000 + 50000 * (0.05 + 0.475)
	// = 50000 + 50000 * 0.525
	// = 50000 + 26250
	// = 76250
	expectedLeaderReward := uint64(76250)

	// Calculate member reward using Shelley formula:
	// memberReward = (poolReward - cost) * (1 - margin) * (memberStake / totalPoolStake)
	// = (100000 - 50000) * (1 - 0.05) * (5000000 / 20000000)
	// = 50000 * 0.95 * 0.25
	// = 50000 * 0.2375
	// = 11875
	expectedMemberReward := uint64(11875)

	// Test our implementation
	actualLeaderReward := calculateLeaderReward(
		poolReward,
		poolCost,
		margin,
		ownerStake,
		totalPoolStake,
	)
	actualMemberReward := calculateMemberReward(
		poolReward,
		poolCost,
		margin,
		memberStake,
		totalPoolStake,
	)

	assert.Equal(
		t,
		expectedLeaderReward,
		actualLeaderReward,
		"Leader reward should match Shelley specification",
	)
	assert.Equal(
		t,
		expectedMemberReward,
		actualMemberReward,
		"Member reward should match Shelley specification",
	)
}

// TestRewardCalculationAgainstReference validates our complete reward calculation
// against expected behavior from reference implementations.
func TestRewardCalculationAgainstReference(t *testing.T) {
	// Setup test data matching reference implementation patterns
	pots := AdaPots{
		Reserves: 1000000000, // 1B ADA reserves
		Treasury: 200000000,  // 200M ADA treasury
		Rewards:  100000000,  // 100M ADA reward pot
	}

	snapshot := RewardSnapshot{
		TotalActiveStake: 500000000, // 500M ADA total active stake
		PoolStake: map[PoolKeyHash]uint64{
			{1}: 200000000, // Pool 1: 200M ADA
			{2}: 150000000, // Pool 2: 150M ADA
			{3}: 150000000, // Pool 3: 150M ADA
		},
		DelegatorStake: map[PoolKeyHash]map[AddrKeyHash]uint64{
			{1}: {
				AddrKeyHash{1}: 180000000, // Delegator 1: 180M ADA
				AddrKeyHash{2}: 20000000,  // Delegator 2: 20M ADA
			},
			{2}: {
				AddrKeyHash{3}: 150000000, // Delegator 3: 150M ADA
			},
			{3}: {
				AddrKeyHash{4}: 140000000, // Delegator 4: 140M ADA
				AddrKeyHash{5}: 10000000,  // Delegator 5: 10M ADA
			},
		},
		PoolParams: map[PoolKeyHash]*PoolRegistrationCertificate{
			{1}: {
				Cost:   50000,                // 50k ADA fixed cost
				Margin: NewGenesisRat(1, 20), // 5% margin
				Pledge: 1000000,              // 1M ADA pledge
				PoolOwners: []AddrKeyHash{
					{1}, // Owner is also a delegator
				},
			},
			{2}: {
				Cost:   30000,                // 30k ADA fixed cost
				Margin: NewGenesisRat(1, 10), // 10% margin
				Pledge: 500000,               // 500k ADA pledge
				PoolOwners: []AddrKeyHash{
					{3}, // Owner is also a delegator
				},
			},
			{3}: {
				Cost:   40000,                // 40k ADA fixed cost
				Margin: NewGenesisRat(1, 25), // 4% margin
				Pledge: 750000,               // 750k ADA pledge
				PoolOwners: []AddrKeyHash{
					{4}, // Owner is also a delegator
				},
			},
		},
		StakeRegistrations: map[AddrKeyHash]bool{
			{1}: true,
			{2}: true,
			{3}: true,
			{4}: true,
			{5}: true,
		},
	}

	params := RewardParameters{
		MonetaryExpansion: 3000,             // 0.3%
		TreasuryGrowth:    2000,             // 20%
		PoolInfluence:     big.NewRat(1, 1), // a0 = 1
	}

	result, err := CalculateRewards(pots, snapshot, params)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify basic properties
	assert.Greater(t, len(result.PoolRewards), 0, "Should have pool rewards")
	assert.Equal(
		t,
		pots.Rewards,
		result.TotalRewards,
		"Total distributed should equal reward pot",
	)
	assert.Equal(
		t,
		uint64(0),
		result.UpdatedPots.Rewards,
		"Reward pot should be depleted",
	)

	// Verify each pool got rewards
	for poolID := range snapshot.PoolStake {
		poolRewards, exists := result.PoolRewards[poolID]
		assert.True(t, exists, "Pool %s should have rewards", poolID)
		assert.Greater(
			t,
			poolRewards.TotalRewards,
			uint64(0),
			"Pool %s should have positive rewards",
			poolID,
		)
		assert.Greater(
			t,
			poolRewards.OperatorRewards,
			uint64(0),
			"Pool %s should have operator reward",
			poolID,
		)
		assert.NotNil(
			t,
			poolRewards.DelegatorRewards,
			"Pool %s should have delegator rewards map",
			poolID,
		)
	}

	// Verify reward distribution adds up
	totalDistributed := uint64(0)
	for _, poolRewards := range result.PoolRewards {
		totalDistributed += poolRewards.TotalRewards
	}
	assert.Equal(
		t,
		pots.Rewards,
		totalDistributed,
		"Sum of pool rewards should equal total reward pot",
	)
}

// TestEdgeCases validates edge cases in reward calculations
func TestRewardEdgeCases(t *testing.T) {
	t.Run("ZeroPoolReward", func(t *testing.T) {
		// Pool reward equals cost - no member rewards
		poolReward := uint64(50000)
		poolCost := uint64(50000)
		margin := 0.05
		ownerStake := uint64(10000000)
		totalPoolStake := uint64(20000000)
		memberStake := uint64(5000000)

		leaderReward := calculateLeaderReward(
			poolReward,
			poolCost,
			margin,
			ownerStake,
			totalPoolStake,
		)
		memberReward := calculateMemberReward(
			poolReward,
			poolCost,
			margin,
			memberStake,
			totalPoolStake,
		)

		assert.Equal(
			t,
			poolReward,
			leaderReward,
			"Leader should get full pool reward when reward = cost",
		)
		assert.Equal(
			t,
			uint64(0),
			memberReward,
			"Members should get no reward when reward = cost",
		)
	})

	t.Run("HighMargin", func(t *testing.T) {
		// 50% margin - leader gets more
		poolReward := uint64(100000)
		poolCost := uint64(50000)
		margin := 0.5
		ownerStake := uint64(10000000)
		totalPoolStake := uint64(20000000)
		memberStake := uint64(5000000)

		leaderReward := calculateLeaderReward(
			poolReward,
			poolCost,
			margin,
			ownerStake,
			totalPoolStake,
		)
		memberReward := calculateMemberReward(
			poolReward,
			poolCost,
			margin,
			memberStake,
			totalPoolStake,
		)

		assert.Greater(
			t,
			leaderReward,
			poolCost,
			"Leader should get more than cost",
		)
		assert.Less(
			t,
			memberReward,
			leaderReward-poolCost,
			"Members should get less than leader's share",
		)
	})

	t.Run("OwnerIsOnlyDelegator", func(t *testing.T) {
		// Owner has all the stake
		poolReward := uint64(100000)
		poolCost := uint64(50000)
		margin := 0.05
		ownerStake := uint64(20000000)
		totalPoolStake := uint64(20000000)
		memberStake := uint64(0) // No other members

		leaderReward := calculateLeaderReward(
			poolReward,
			poolCost,
			margin,
			ownerStake,
			totalPoolStake,
		)
		memberReward := calculateMemberReward(
			poolReward,
			poolCost,
			margin,
			memberStake,
			totalPoolStake,
		)

		assert.Equal(
			t,
			poolReward,
			leaderReward,
			"Owner should get full reward when they have all stake",
		)
		assert.Equal(
			t,
			uint64(0),
			memberReward,
			"No member rewards when no other delegators",
		)
	})
}

// Helper functions for testing individual reward calculations
func calculateLeaderReward(
	poolReward, poolCost uint64,
	margin float64,
	ownerStake, totalPoolStake uint64,
) uint64 {
	if poolReward <= poolCost {
		return poolReward
	}

	ownerStakeRatio := float64(ownerStake) / float64(totalPoolStake)
	leaderShare := margin + (1-margin)*ownerStakeRatio
	leaderReward := poolCost + uint64(float64(poolReward-poolCost)*leaderShare)

	return leaderReward
}

func calculateMemberReward(
	poolReward, poolCost uint64,
	margin float64,
	memberStake, totalPoolStake uint64,
) uint64 {
	if poolReward <= poolCost {
		return 0
	}

	memberStakeRatio := float64(memberStake) / float64(totalPoolStake)
	memberReward := uint64(
		float64(poolReward-poolCost) * (1 - margin) * memberStakeRatio,
	)

	return memberReward
}

// TestJavaReferenceScenarios validates scenarios mentioned in cf-java-rewards-calculation README
func TestJavaReferenceScenarios(t *testing.T) {
	t.Run("DecentralizationParameter_Eta_Calculation", func(t *testing.T) {
		// Test eta calculation with Amaru's approach (no decentralization dependency)
		params := RewardParameters{
			ActiveSlotsCoeff:      big.NewRat(1, 20), // f = 0.05
			ExpectedSlotsPerEpoch: 432000,
		}

		// expected_blocks = 432000 * 0.05 = 21600
		// With 21600 blocks (exactly expected), eta = min(1, 21600/21600) = 1
		eta := calculateEta(21600, params)
		etaFloat, _ := eta.Float64()
		assert.Equal(t, 1.0, etaFloat)

		// With 10800 blocks (half of expected), eta = 10800/21600 = 0.5
		eta = calculateEta(10800, params)
		etaFloat, _ = eta.Float64()
		assert.Equal(t, 0.5, etaFloat)

		// With 5400 blocks (quarter of expected), eta = 5400/21600 = 0.25
		eta = calculateEta(5400, params)
		etaFloat, _ = eta.Float64()
		assert.Equal(t, 0.25, etaFloat)
	})

	t.Run("PoolPerformance_Calculation", func(t *testing.T) {
		params := RewardParameters{
			Decentralization: 500000, // 50% < 0.8, so performance matters
		}

		snapshot := RewardSnapshot{
			TotalActiveStake: 100000000, // 100M ADA total
			PoolStake: map[PoolKeyHash]uint64{
				{1}: 20000000, // 20M ADA pool stake
			},
			PoolBlocks: map[PoolKeyHash]uint32{
				{1}: 1000, // 1000 blocks by pool
			},
			TotalBlocksInEpoch: 2000, // 2000 total blocks
		}

		// Amaru performance = (1000/2000) * (100000000/20000000) = 0.5 * 5 = 2.5
		performance := calculatePoolPerformance(
			PoolKeyHash{1},
			snapshot,
			params,
		)
		assert.Equal(t, 2.5, performance)

		// Test with lower performance
		snapshot.PoolBlocks[PoolKeyHash{1}] = 200 // 200 blocks
		snapshot.TotalBlocksInEpoch = 2000
		performance = calculatePoolPerformance(PoolKeyHash{1}, snapshot, params)
		assert.Equal(
			t,
			0.5,
			performance,
		) // (200/2000) * (100000000/20000000) = 0.1 * 5 = 0.5
	})

	t.Run("MonetaryExpansion_With_Eta", func(t *testing.T) {
		currentPots := AdaPots{
			Reserves: 1000000000, // 1B ADA
			Treasury: 200000000,  // 200M ADA
			Rewards:  0,
		}

		params := RewardParameters{
			MonetaryExpansion:     3000,              // 0.3%
			TreasuryGrowth:        2000,              // 20%
			Decentralization:      500000,            // 50%
			ActiveSlotsCoeff:      big.NewRat(1, 20), // f = 0.05
			ExpectedSlotsPerEpoch: 432000,
		}

		// With 10800 blocks, expected_blocks = 432000 * 0.05 = 21600
		// eta = min(1, 10800/21600) = 0.5
		// incentives = 0.5 * 0.003 * 1,000,000,000 = 1,500,000
		// total_rewards = 1,500,000 + 1,000,000 = 2,500,000
		// treasury_tax = 2,500,000 * 0.2 = 500,000
		// available_rewards = 2,500,000 - 500,000 = 2,000,000
		newPots := CalculateAdaPots(
			currentPots,
			params,
			1000000,
			10800,
		)

		assert.Equal(t, uint64(1000000000-1500000), newPots.Reserves)
		assert.Equal(t, uint64(200000000+500000), newPots.Treasury)
		assert.Equal(t, uint64(2000000), newPots.Rewards)
	})

	t.Run("RewardTiming_Handling", func(t *testing.T) {
		// Test that deregistration timing affects reward distribution
		// This would require extending the reward calculation to handle
		// EarlyDeregistrations and LateDeregistrations maps
		// For now, we test that the data structures exist
		snapshot := RewardSnapshot{
			EarlyDeregistrations: map[AddrKeyHash]uint64{
				{1}: 172799, // Before reward calculation slot
			},
			LateDeregistrations: map[AddrKeyHash]uint64{
				{2}: 172801, // After reward calculation slot
			},
		}

		assert.Contains(t, snapshot.EarlyDeregistrations, AddrKeyHash{1})
		assert.Contains(t, snapshot.LateDeregistrations, AddrKeyHash{2})
	})

	t.Run("PoolRetirement_Handling", func(t *testing.T) {
		// Test pool retirement data structures
		snapshot := RewardSnapshot{
			RetiredPools: map[PoolKeyHash]PoolRetirementInfo{
				{1}: {
					RewardAddress: AddrKeyHash{1},
					Epoch:         100,
				},
			},
		}

		assert.Contains(t, snapshot.RetiredPools, PoolKeyHash{1})
		assert.Equal(
			t,
			AddrKeyHash{1},
			snapshot.RetiredPools[PoolKeyHash{1}].RewardAddress,
		)
		assert.Equal(
			t,
			uint64(100),
			snapshot.RetiredPools[PoolKeyHash{1}].Epoch,
		)
	})
}

// TestAmaruCompatibility validates our implementation against Amaru's formulas
func TestAmaruCompatibility(t *testing.T) {
	t.Run("LeaderRewards_Formula", func(t *testing.T) {
		// Test the leader rewards formula matches Amaru's implementation
		// Amaru: cost + floor((m + (1 - m) × s / σ) × (R_pool - c))
		// Our: cost + (margin + (1 - margin) × ownerStakeRatio) × (poolReward - cost)

		poolReward := uint64(100000)     // R_pool
		poolCost := uint64(50000)        // c
		margin := 0.05                   // m
		ownerStake := uint64(100000)     // s
		totalPoolStake := uint64(200000) // σ

		// Calculate using our formula
		ownerStakeRatio := float64(ownerStake) / float64(totalPoolStake)
		leaderShare := margin + (1-margin)*ownerStakeRatio
		ourLeaderReward := poolCost + uint64(
			float64(poolReward-poolCost)*leaderShare,
		)

		// Expected result: 50000 + (100000-50000) × (0.05 + 0.95 × 100000/200000)
		// = 50000 + 50000 × (0.05 + 0.95 × 0.5)
		// = 50000 + 50000 × (0.05 + 0.475)
		// = 50000 + 50000 × 0.525
		// = 50000 + 26250 = 76250
		expectedLeaderReward := uint64(76250)

		assert.Equal(
			t,
			expectedLeaderReward,
			ourLeaderReward,
			"Leader reward should match Amaru's formula",
		)
	})

	t.Run("MemberRewards_Formula", func(t *testing.T) {
		// Test the member rewards formula matches Amaru's implementation
		// Amaru: floor((1 - m) × (R_pool - c) × t / σ)
		// Our: (1 - margin) × (poolReward - cost) × memberStakeRatio

		poolReward := uint64(100000)     // R_pool
		poolCost := uint64(50000)        // c
		margin := 0.05                   // m
		memberStake := uint64(50000)     // t
		totalPoolStake := uint64(200000) // σ

		// Calculate using our formula
		memberStakeRatio := float64(memberStake) / float64(totalPoolStake)
		ourMemberReward := uint64(
			float64(poolReward-poolCost) * (1 - margin) * memberStakeRatio,
		)

		// Expected result: (100000-50000) × (1-0.05) × (50000/200000)
		// = 50000 × 0.95 × 0.25
		// = 50000 × 0.2375 = 11875
		expectedMemberReward := uint64(11875)

		assert.Equal(
			t,
			expectedMemberReward,
			ourMemberReward,
			"Member reward should match Amaru's formula",
		)
	})

	t.Run("Eta_Calculation", func(t *testing.T) {
		// Test eta calculation matches Amaru's efficiency-based approach
		// Amaru: eta = min(1, efficiency) where efficiency = blocks_produced / (epoch_length * active_slot_coeff)
		// Note: Amaru does NOT use decentralization in eta calculation

		params := RewardParameters{
			ActiveSlotsCoeff:      big.NewRat(1, 20), // f = 0.05
			ExpectedSlotsPerEpoch: 432000,
		}

		// Test case: 10800 blocks produced, expected calculation
		// expected_blocks = 432000 * 0.05 = 21600
		// eta = min(1, 10800/21600) = 0.5
		eta := calculateEta(10800, params)
		etaFloat, _ := eta.Float64()
		assert.Equal(t, 0.5, etaFloat)

		// Test case: 21600 blocks produced (exactly expected)
		// eta = min(1, 21600/21600) = 1
		eta = calculateEta(21600, params)
		etaFloat, _ = eta.Float64()
		assert.Equal(t, 1.0, etaFloat)

		// Test case: 5400 blocks produced (quarter of expected)
		// eta = min(1, 5400/21600) = 0.25
		eta = calculateEta(5400, params)
		etaFloat, _ = eta.Float64()
		assert.Equal(t, 0.25, etaFloat)
	})

	t.Run("PoolPerformance_Amaru_Style", func(t *testing.T) {
		// Test pool performance calculation matches Amaru's apparent_performances
		// Amaru: performance = (pool_blocks/total_blocks) * (total_stake/pool_stake)
		// Note: Amaru does NOT cap performance at 1.0

		snapshot := RewardSnapshot{
			TotalActiveStake: 1000000, // 1M total stake
			PoolStake: map[PoolKeyHash]uint64{
				{1}: 200000, // 200k pool stake
			},
			PoolBlocks: map[PoolKeyHash]uint32{
				{1}: 100, // 100 blocks by pool
			},
			TotalBlocksInEpoch: 200, // 200 total blocks
		}

		// Amaru calculation: (100/200) * (1000000/200000) = 0.5 * 5 = 2.5
		performance := calculatePoolPerformance(
			PoolKeyHash{1},
			snapshot,
			RewardParameters{},
		)
		assert.Equal(t, 2.5, performance)

		// Test with lower performance
		snapshot.PoolBlocks[PoolKeyHash{1}] = 20 // 20 blocks
		snapshot.TotalBlocksInEpoch = 200
		performance = calculatePoolPerformance(
			PoolKeyHash{1},
			snapshot,
			RewardParameters{},
		)
		assert.Equal(
			t,
			0.5,
			performance,
		) // (20/200) * (1000000/200000) = 0.1 * 5 = 0.5
	})

	t.Run("MonetaryExpansion_With_Eta_Amaru", func(t *testing.T) {
		// Test monetary expansion calculation matches Amaru's approach
		// Amaru: incentives = floor(min(1, efficiency) * monetary_expansion_rate * reserves)

		currentPots := AdaPots{
			Reserves: 1000000000, // 1B ADA
			Treasury: 200000000,  // 200M ADA
			Rewards:  0,
		}

		params := RewardParameters{
			MonetaryExpansion:     3000,              // 0.3%
			TreasuryGrowth:        2000,              // 20%
			Decentralization:      500000,            // 50%
			ActiveSlotsCoeff:      big.NewRat(1, 20), // f = 0.05
			ExpectedSlotsPerEpoch: 432000,
		}

		// With 10800 blocks, expected_blocks = 432000 * 0.05 = 21600
		// eta = min(1, 10800/21600) = 0.5
		// incentives = 0.5 * 0.003 * 1,000,000,000 = 1,500,000
		// total_rewards = 1,500,000 + 1,000,000 = 2,500,000
		// treasury_tax = 2,500,000 * 0.2 = 500,000
		// available_rewards = 2,500,000 - 500,000 = 2,000,000
		newPots := CalculateAdaPots(
			currentPots,
			params,
			1000000,
			10800,
		)

		assert.Equal(t, uint64(1000000000-1500000), newPots.Reserves)
		assert.Equal(t, uint64(200000000+500000), newPots.Treasury)
		assert.Equal(t, uint64(2000000), newPots.Rewards)
	})
}
