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

package common_test

import (
	"testing"

	test_ledger "github.com/blinklabs-io/gouroboros/internal/test/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRewardServiceIntegration(t *testing.T) {
	// Setup test data
	pots := common.AdaPots{
		Reserves: 1000000000,
		Treasury: 200000000,
		Rewards:  1000000, // 1 million ADA to distribute
	}

	snapshot := common.RewardSnapshot{
		TotalActiveStake: 50000000, // 50 million ADA total active stake
		PoolStake: map[common.PoolKeyHash]uint64{
			{1}: 30000000, // Pool 1: 30 million ADA
			{2}: 20000000, // Pool 2: 20 million ADA
		},
		DelegatorStake: map[common.PoolKeyHash]map[common.AddrKeyHash]uint64{
			{1}: {
				common.AddrKeyHash{1}: 25000000,
				common.AddrKeyHash{2}: 5000000,
			},
			{2}: {
				common.AddrKeyHash{3}: 20000000,
			},
		},
		PoolParams: map[common.PoolKeyHash]*common.PoolRegistrationCertificate{
			{1}: {
				Cost:       50000,
				Margin:     common.GenesisRat{},
				Pledge:     1000000,
				PoolOwners: []common.AddrKeyHash{{1}},
			},
			{2}: {
				Cost:       30000,
				Margin:     common.GenesisRat{},
				Pledge:     500000,
				PoolOwners: []common.AddrKeyHash{{3}},
			},
		},
		StakeRegistrations: map[common.AddrKeyHash]bool{
			{1}: true,
			{2}: true,
			{3}: true,
		},
		PoolBlocks: map[common.PoolKeyHash]uint32{
			{1}: 1500,
			{2}: 1000,
		},
		TotalBlocksInEpoch: 2500,
	}

	mockState := &test_ledger.MockLedgerState{
		AdaPotsVal:        pots,
		RewardSnapshotVal: snapshot,
	}

	rewardService := common.NewRewardService(mockState)

	result, err := rewardService.CalculateEpochRewards(100)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Greater(t, len(result.PoolRewards), 0)
	assert.Equal(t, pots.Rewards, result.TotalRewards)

	updatedPots := mockState.GetAdaPots()
	assert.Equal(t, uint64(0), updatedPots.Rewards)
}
