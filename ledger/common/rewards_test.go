// Copyright 2025 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
package common_test

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateAdaPots(t *testing.T) {
	// Test basic ADA pots calculation
	currentPots := common.AdaPots{
		Reserves: 1000000000, // 1 billion ADA
		Treasury: 200000000,  // 200 million ADA
		Rewards:  0,          // No existing rewards
	}

	params := common.RewardParameters{
		MonetaryExpansion:     3000,              // 0.3% = 3000 millionths
		TreasuryGrowth:        2000,              // 20% = 2000 millionths
		Decentralization:      500000,            // 50% decentralization
		ActiveSlotsCoeff:      big.NewRat(1, 20), // f = 0.05
		ExpectedSlotsPerEpoch: 432000,            // Standard mainnet value
	}

	epochFees := uint64(1000000)        // 1 million ADA in fees
	totalBlocksInEpoch := uint32(21600) // Expected blocks per epoch

	newPots := common.CalculateAdaPots(
		currentPots,
		params,
		epochFees,
		totalBlocksInEpoch,
	)

	// Expected monetary expansion: 1000000000 * 0.003 * eta
	// With Amaru approach: expected_blocks = 432000 * 0.05 = 21600
	// eta = min(1, 21600/21600) = 1
	// expansion = 1000000000 * 0.003 * 1 = 3000000
	// total_rewards = 3000000 + 1000000 = 4000000
	// treasury contribution = 4000000 * 0.2 = 800000
	// reward pot = 4000000 - 800000 = 3200000

	expectedReserves := uint64(1000000000 - 3000000)
	expectedTreasury := uint64(200000000 + 800000)
	expectedRewards := uint64(3200000)

	assert.Equal(t, expectedReserves, newPots.Reserves)
	assert.Equal(t, expectedTreasury, newPots.Treasury)
	assert.Equal(t, expectedRewards, newPots.Rewards)
}

func TestCalculateRewards(t *testing.T) {
	// Setup test data
	pots := common.AdaPots{
		Reserves: 1000000000,
		Treasury: 200000000,
		Rewards:  1000000, // 1 million ADA to distribute
	}
	snapshot := common.RewardSnapshot{
		TotalActiveStake: 50000000, // 50 million ADA total active stake (delegated to pools)
		PoolStake: map[common.PoolKeyHash]uint64{
			{1}: 30000000, // Pool 1: 30 million ADA
			{2}: 20000000, // Pool 2: 20 million ADA
		},
		DelegatorStake: map[common.PoolKeyHash]map[common.AddrKeyHash]uint64{
			{1}: {
				common.AddrKeyHash{1}: 25000000, // Delegator 1: 25 million ADA
				common.AddrKeyHash{2}: 5000000,  // Delegator 2: 5 million ADA
			},
			{2}: {
				common.AddrKeyHash{3}: 20000000, // Delegator 3: 20 million ADA
			},
		},
		PoolParams: map[common.PoolKeyHash]*common.PoolRegistrationCertificate{
			{1}: {
				Cost:   50000,                       // 50k ADA fixed cost
				Margin: common.NewGenesisRat(1, 20), // 5% margin
				Pledge: 1000000,                     // 1M ADA pledge
				PoolOwners: []common.AddrKeyHash{
					{1}, // Owner 1
					{2}, // Owner 2
				},
			},
			{2}: {
				Cost:   30000,                       // 30k ADA fixed cost
				Margin: common.NewGenesisRat(1, 10), // 10% margin
				Pledge: 500000,                      // 500k ADA pledge
				PoolOwners: []common.AddrKeyHash{
					{3}, // Owner 3
				},
			},
		},
		StakeRegistrations: map[common.AddrKeyHash]bool{
			{1}: true,
			{2}: true,
			{3}: true,
		},
		PoolBlocks: map[common.PoolKeyHash]uint32{
			{1}: 1500, // Pool 1 produced 1500 blocks
			{2}: 1000, // Pool 2 produced 1000 blocks
		},
		TotalBlocksInEpoch: 2500, // Total blocks by stake pools
	}

	params := common.RewardParameters{
		PoolInfluence: big.NewRat(1, 1), // a0 = 1
	}

	result, err := common.CalculateRewards(pots, snapshot, params)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Check that rewards were calculated
	assert.Greater(t, len(result.PoolRewards), 0)

	// Check that total rewards equal the reward pot
	totalDistributed := uint64(0)
	for _, poolRewards := range result.PoolRewards {
		totalDistributed += poolRewards.TotalRewards
	}
	assert.Equal(t, pots.Rewards, totalDistributed)

	// Check that updated pots have zero rewards
	assert.Equal(t, uint64(0), result.UpdatedPots.Rewards)
}

func TestMarginFloat(t *testing.T) {
	// Test zero margin
	zeroMargin := cbor.Rat{}
	assert.Equal(t, 0.0, marginFloat(zeroMargin))

	// Test 5% margin (1/20)
	margin5Percent := common.NewGenesisRat(1, 20)
	assert.InDelta(t, 0.05, marginFloat(margin5Percent), 0.001)

	// Test 10% margin (1/10)
	margin10Percent := common.NewGenesisRat(1, 10)
	assert.InDelta(t, 0.1, marginFloat(margin10Percent), 0.001)
}

func TestValidatePoolPledge(t *testing.T) {
	poolParams := &common.PoolRegistrationCertificate{
		Pledge: 1000000, // 1M ADA pledge
		PoolOwners: []common.AddrKeyHash{
			{1},
			{2},
		},
	}

	ownerStake := map[common.AddrKeyHash]uint64{
		{1}: 600000, // 600k ADA
		{2}: 500000, // 500k ADA
	}

	// Total owner stake: 1.1M ADA > 1M pledge, should pass
	assert.True(
		t,
		common.ValidatePoolPledge(
			common.PoolKeyHash{1},
			poolParams,
			ownerStake,
		),
	)

	// Reduce stake to not meet pledge
	ownerStake[common.AddrKeyHash{1}] = 400000 // Total: 900k < 1M
	assert.False(
		t,
		common.ValidatePoolPledge(
			common.PoolKeyHash{1},
			poolParams,
			ownerStake,
		),
	)
}

func TestCalculatePoolSaturation(t *testing.T) {
	poolStake := uint64(50000000)    // 50M ADA pool stake
	totalStake := uint64(1000000000) // 1B ADA total stake
	saturationPoint := uint64(5)     // 5% saturation point

	saturation := common.CalculatePoolSaturation(
		poolStake,
		totalStake,
		saturationPoint,
	)

	// Pool stake is 50M, saturation point stake is 50M (5% of 1B)
	// So saturation should be 1.0 (fully saturated)
	assert.Equal(t, 1.0, saturation)

	// Test with smaller pool
	smallPoolStake := uint64(10000000) // 10M ADA
	saturation2 := common.CalculatePoolSaturation(
		smallPoolStake,
		totalStake,
		saturationPoint,
	)

	// 10M / 50M = 0.2
	assert.Equal(t, 0.2, saturation2)
}

// marginFloat converts a cbor.Rat (or common.GenesisRat) to a float64 fraction.
func marginFloat(r cbor.Rat) float64 {
	br := r.ToBigRat()
	if br == nil {
		return 0.0
	}
	f, _ := new(big.Float).SetRat(br).Float64()
	return f
}
