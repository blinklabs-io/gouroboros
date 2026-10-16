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
	"errors"
	"fmt"
	"maps"
	"math/big"
)

// RewardService provides reward calculation services for the ledger
type RewardService struct {
	ledgerState LedgerState
}

// NewRewardService creates a new reward service
func NewRewardService(ledgerState LedgerState) *RewardService {
	return &RewardService{
		ledgerState: ledgerState,
	}
}

// CalculateEpochRewards calculates rewards for the given epoch
func (rs *RewardService) CalculateEpochRewards(
	epoch uint64,
) (*RewardCalculationResult, error) {
	// Get current ADA pots
	currentPots := rs.ledgerState.GetAdaPots()

	// Get reward snapshot for the epoch
	snapshot, err := rs.ledgerState.GetRewardSnapshot(epoch)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get reward snapshot for epoch %d: %w",
			epoch,
			err,
		)
	}

	// Get protocol parameters (this would need to be implemented in the ledger state)
	// For now, use default parameters
	params := RewardParameters{
		MonetaryExpansion: 3000,             // 0.3%
		TreasuryGrowth:    2000,             // 20%
		PoolInfluence:     big.NewRat(1, 1), // a0 = 1
	}

	// Calculate rewards
	result, err := rs.ledgerState.CalculateRewards(
		currentPots,
		snapshot,
		params,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to calculate rewards for epoch %d: %w",
			epoch,
			err,
		)
	}

	// Update ADA pots with the result
	err = rs.ledgerState.UpdateAdaPots(result.UpdatedPots)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to update ADA pots after reward calculation: %w",
			err,
		)
	}

	return result, nil
}

// ValidateRewardDistribution validates that a reward distribution transaction is correct
func (rs *RewardService) ValidateRewardDistribution(
	expectedRewards *RewardCalculationResult,
	actualRewards map[RewardCredential]uint64,
) error {
	expectedAllocations := make(map[RewardCredential]uint64)
	addAllocation := func(recipient RewardCredential, reward uint64) {
		if reward > 0 {
			expectedAllocations[recipient] += reward
		}
	}
	for _, poolRewards := range expectedRewards.PoolRewards {
		addAllocation(
			poolRewards.OperatorRewardAccount,
			poolRewards.OperatorRewards,
		)
		for recipient, reward := range poolRewards.DelegatorRewards {
			addAllocation(recipient, reward)
		}
	}

	if !maps.Equal(expectedAllocations, actualRewards) {
		return errors.New("reward distribution does not match calculated allocations")
	}

	return nil
}
