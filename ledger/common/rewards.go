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
	"math"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// AdaPots represents the three main ADA pots in Cardano
type AdaPots struct {
	Reserves uint64 // The reserves pot
	Treasury uint64 // The treasury pot
	Rewards  uint64 // The rewards pot
}

// RewardParameters contains the protocol parameters needed for reward calculation
type RewardParameters struct {
	// Monetary expansion parameters
	MonetaryExpansion uint64 // rho - monetary expansion rate (0.003 means 0.3%)
	TreasuryGrowth    uint64 // tau - treasury growth rate (0.2 means 20%)

	// Decentralization parameter (0 = fully decentralized, 1 = fully centralized)
	Decentralization uint64

	// Protocol version for reward calculation rules
	ProtocolVersion ProtocolParametersProtocolVersion

	// Minimum pool cost
	MinPoolCost uint64

	// Pool influence parameter (a0)
	PoolInfluence *big.Rat

	// Expansion rate (rho)
	ExpansionRate *big.Rat

	// Treasury expansion rate (tau)
	TreasuryRate *big.Rat

	// Active slots coefficient (f) - from CIP-9
	ActiveSlotsCoeff *big.Rat

	// Expected slots per epoch - from network configuration
	ExpectedSlotsPerEpoch uint32
}

// RewardSnapshot represents the stake snapshot for reward calculation
type RewardSnapshot struct {
	// Total active stake in the system
	TotalActiveStake uint64

	// Stake distribution by pool
	PoolStake map[PoolKeyHash]uint64

	// Delegator stake by pool
	DelegatorStake map[PoolKeyHash]map[AddrKeyHash]uint64

	// Pool parameters
	PoolParams map[PoolKeyHash]*PoolRegistrationCertificate

	// Stake registrations for reward eligibility
	StakeRegistrations map[AddrKeyHash]bool

	// Block production data for pool performance calculation
	PoolBlocks         map[PoolKeyHash]uint32 // Blocks produced by each pool
	TotalBlocksInEpoch uint32                 // Total blocks produced in epoch by stake pools

	// Deregistration timing (slot when deregistration occurred)
	// Accounts deregistered before reward calculation start (slot 172800)
	// don't receive rewards (rewards go to reserves)
	EarlyDeregistrations map[AddrKeyHash]uint64

	// Accounts deregistered after reward calculation start but before epoch end
	// receive rewards but they go to treasury since they can't be paid
	LateDeregistrations map[AddrKeyHash]uint64

	// Pool retirement information
	RetiredPools map[PoolKeyHash]PoolRetirementInfo

	// Multiple pool associations (for pre-Allegra behavior)
	// Maps stake key to list of pools they have reward addresses for
	StakeKeyPoolAssociations map[AddrKeyHash][]PoolKeyHash
}

// PoolRetirementInfo contains information about retired pools
type PoolRetirementInfo struct {
	RewardAddress AddrKeyHash // Pool reward address
	Epoch         uint64      // Epoch when retirement was announced
}

// RewardCalculationResult contains the calculated rewards
type RewardCalculationResult struct {
	// Total rewards to be distributed
	TotalRewards uint64

	// Rewards per pool (operator + delegators)
	PoolRewards map[PoolKeyHash]PoolRewards

	// Updated ADA pots after reward distribution
	UpdatedPots AdaPots
}

// PoolRewards contains rewards for a specific pool
type PoolRewards struct {
	// Pool operator rewards
	OperatorRewards uint64

	// Delegator rewards by stake key hash
	DelegatorRewards map[AddrKeyHash]uint64

	// Total rewards for this pool
	TotalRewards uint64
}

// CalculateAdaPots calculates the ADA pots for the next epoch
func CalculateAdaPots(
	currentPots AdaPots,
	params RewardParameters,
	epochFees uint64,
	totalBlocksInEpoch uint32,
) AdaPots {
	// Calculate eta (pool performance factor)
	eta := calculateEta(totalBlocksInEpoch, params)

	// Calculate monetary expansion (rho * eta * reserves)
	// rho is in millionths (3000 = 0.3%)
	monetaryExpansion := new(big.Int).SetUint64(params.MonetaryExpansion)
	reserves := new(big.Int).SetUint64(currentPots.Reserves)
	expansion := new(big.Int).Mul(monetaryExpansion, reserves)
	expansion.Div(
		expansion,
		new(big.Int).SetUint64(1000000),
	) // Convert from millionths

	// Apply eta factor
	etaBig := eta
	expansion = new(big.Int).Mul(expansion, etaBig.Num())
	expansion.Div(expansion, etaBig.Denom())

	// Add epoch fees to reward pot
	rewardPot := new(big.Int).SetUint64(epochFees)
	rewardPot.Add(rewardPot, expansion)

	// Calculate treasury contribution (tau * (expansion + fees))
	// tau is in ten-thousandths (2000 = 20%)
	treasuryGrowth := new(big.Int).SetUint64(params.TreasuryGrowth)
	treasuryContribution := new(big.Int).Mul(treasuryGrowth, rewardPot)
	treasuryContribution.Div(
		treasuryContribution,
		new(big.Int).SetUint64(10000),
	) // Convert from ten-thousandths

	// Subtract treasury contribution from reward pot
	rewardPot.Sub(rewardPot, treasuryContribution)

	// Update pots with safe arithmetic to prevent underflow
	reservesBig := new(big.Int).SetUint64(currentPots.Reserves)
	reservesMinusExpansion := new(big.Int).Sub(reservesBig, expansion)
	var newReserves uint64
	if reservesMinusExpansion.Sign() < 0 {
		newReserves = 0 // Prevent underflow, set to 0
	} else {
		newReserves = reservesMinusExpansion.Uint64()
	}

	newPots := AdaPots{
		Reserves: newReserves,
		Treasury: currentPots.Treasury + treasuryContribution.Uint64(),
		Rewards:  rewardPot.Uint64(),
	}

	return newPots
}

// calculateEta calculates the pool performance factor (η)
// Following Amaru's approach: eta = min(1, blocks_produced / (epoch_length * active_slot_coeff))
func calculateEta(totalBlocksInEpoch uint32, params RewardParameters) *big.Rat {
	// Defensive guards
	activeSlotsCoeff := params.ActiveSlotsCoeff
	if activeSlotsCoeff == nil {
		activeSlotsCoeff = new(big.Rat).SetInt64(0) // Treat nil as zero
	}
	if params.ExpectedSlotsPerEpoch == 0 {
		return new(big.Rat).SetInt64(0) // Avoid division by zero
	}

	// Calculate expected blocks: epoch_length * active_slot_coeff
	expectedBlocks := new(big.Rat).Mul(
		new(big.Rat).SetUint64(uint64(params.ExpectedSlotsPerEpoch)),
		activeSlotsCoeff,
	)

	// eta = min(1, totalBlocksInEpoch / expectedBlocks)
	actualBlocks := new(big.Rat).SetUint64(uint64(totalBlocksInEpoch))
	eta := new(big.Rat).Quo(actualBlocks, expectedBlocks)

	// Cap at 1.0
	oneRat := new(big.Rat).SetInt64(1)
	if eta.Cmp(oneRat) > 0 {
		eta = oneRat
	}

	return eta
}

// calculatePoolPerformance calculates the apparent performance of a pool
// Following Amaru's approach: performance = (pool_blocks / total_blocks) * (total_stake / pool_stake)
// params is unused as performance calculation only requires block production data from snapshot
func calculatePoolPerformance(
	poolID PoolKeyHash,
	snapshot RewardSnapshot,
	_ RewardParameters,
) float64 {
	// Get blocks produced by this pool
	poolBlocks := snapshot.PoolBlocks[poolID]

	// If no total blocks, assume optimal performance
	if snapshot.TotalBlocksInEpoch == 0 {
		return 1.0
	}

	// If no blocks produced, performance = 0
	if poolBlocks == 0 {
		return 0.0
	}

	// Calculate blocks ratio: pool_blocks / total_blocks
	blocksRatio := float64(poolBlocks) / float64(snapshot.TotalBlocksInEpoch)

	// Calculate stake ratio: total_stake / pool_stake
	poolStake := snapshot.PoolStake[poolID]
	if poolStake == 0 {
		return 0.0 // Zero-stake pools have zero performance
	}
	stakeRatio := float64(snapshot.TotalActiveStake) / float64(poolStake)

	// Performance = blocks_ratio * stake_ratio
	performance := blocksRatio * stakeRatio

	return performance
}

// CalculateRewards calculates stake pool and delegator rewards
func CalculateRewards(
	pots AdaPots,
	snapshot RewardSnapshot,
	params RewardParameters,
) (*RewardCalculationResult, error) {
	if snapshot.TotalActiveStake == 0 || pots.Rewards == 0 {
		return &RewardCalculationResult{
			TotalRewards: 0,
			PoolRewards:  make(map[PoolKeyHash]PoolRewards),
			UpdatedPots:  pots,
		}, nil
	}

	result := &RewardCalculationResult{
		TotalRewards: pots.Rewards,
		PoolRewards:  make(map[PoolKeyHash]PoolRewards),
		UpdatedPots:  pots, // Preserve original pots until successful distribution
	}

	// Calculate rewards for each pool
	poolShares := make(map[PoolKeyHash]float64)
	totalShare := 0.0

	// First pass: calculate raw shares for all pools
	for poolID, poolStake := range snapshot.PoolStake {
		poolParams, exists := snapshot.PoolParams[poolID]
		if !exists {
			continue // Skip pools without parameters
		}

		share := calculatePoolShare(
			poolStake,
			poolParams,
			snapshot,
			params,
			poolID,
		)
		poolShares[poolID] = share
		totalShare += share
	}

	// Guard against malformed snapshot with no valid pools
	if len(poolShares) == 0 {
		return nil, errors.New("no valid pools found in reward snapshot")
	}

	// Guard against zero total share (all pools have zero share)
	if totalShare == 0 {
		// Assign equal shares to all pools to avoid division by zero
		equalShare := 1.0 / float64(len(poolShares))
		for poolID := range poolShares {
			poolShares[poolID] = equalShare
		}
		totalShare = 1.0
	}

	// Second pass: calculate actual rewards using normalized shares
	totalDistributed := uint64(0)
	var lastPoolID PoolKeyHash
	poolRewardAmounts := make(map[PoolKeyHash]uint64)

	for poolID, rawShare := range poolShares {
		lastPoolID = poolID
		// Normalize share
		normalizedShare := rawShare / totalShare
		totalPoolRewards := uint64(float64(pots.Rewards) * normalizedShare)
		poolRewardAmounts[poolID] = totalPoolRewards
		totalDistributed += totalPoolRewards
	}

	// Adjust the last pool's total rewards to ensure sum equals reward pot exactly
	if totalDistributed != pots.Rewards && len(poolRewardAmounts) > 0 {
		// Calculate adjustment - this may overflow in extreme cases, but Cardano values are reasonable
		adjustment := int64(
			pots.Rewards,
		) - int64(
			totalDistributed,
		) // #nosec G115
		poolRewardAmounts[lastPoolID] = uint64(
			int64(poolRewardAmounts[lastPoolID]) + adjustment,
		) // #nosec G115
	}

	// Now distribute rewards for each pool
	for poolID, totalPoolRewards := range poolRewardAmounts {
		poolParams := snapshot.PoolParams[poolID]
		if poolParams == nil {
			return nil, fmt.Errorf(
				"pool parameters not found for pool %s",
				poolID,
			)
		}
		delegatorStake := snapshot.DelegatorStake[poolID]
		if delegatorStake == nil {
			delegatorStake = make(map[AddrKeyHash]uint64)
		}

		poolRewards := distributePoolRewards(
			poolID,
			totalPoolRewards,
			delegatorStake,
			poolParams,
			snapshot,
		)

		result.PoolRewards[poolID] = *poolRewards
	}

	// Set rewards pot to 0 after successful distribution
	result.UpdatedPots.Rewards = 0

	return result, nil
}

// calculatePoolShare calculates the reward share for a single pool
func calculatePoolShare(
	poolStake uint64,
	poolParams *PoolRegistrationCertificate,
	snapshot RewardSnapshot,
	params RewardParameters,
	poolID PoolKeyHash,
) float64 {
	// Calculate pool performance using actual block production data
	performance := calculatePoolPerformance(poolID, snapshot, params)

	// Calculate stake ratio (pool stake / total active stake)
	stakeRatio := float64(poolStake) / float64(snapshot.TotalActiveStake)

	// Calculate saturation (capped at 1.0)
	saturation := math.Min(
		stakeRatio/0.05,
		1.0,
	) // TODO(enhancement): Extract 0.05 saturation threshold to param for consistency with CalculatePoolSaturation

	// Calculate pool reward share using leader stake influence formula
	// R_pool = (stake_ratio * performance * (1 - margin)) / (1 + a0 * saturation)
	a0 := params.PoolInfluence
	if a0 == nil {
		a0 = big.NewRat(1, 1) // Default a0 = 1
	}

	margin := poolParams.Margin

	// Calculate numerator: stake_ratio * performance * (1 - margin)
	numerator := stakeRatio * performance * (1 - marginFloat(margin))

	// Calculate denominator: 1 + a0 * saturation
	a0Float, _ := a0.Float64()
	if a0Float < 0 {
		a0Float = 0 // Defensive clamp against pathological values
	}
	denominator := 1.0 + a0Float*saturation
	if denominator <= 0 {
		return 0 // Prevent division by zero or negative denominator
	}

	poolRewardShare := numerator / denominator

	return poolRewardShare
}

// distributePoolRewards distributes rewards within a pool between operator and delegators
// poolID is unused as reward distribution logic only requires stake data and pool parameters
func distributePoolRewards(
	_ PoolKeyHash,
	totalPoolRewards uint64,
	delegatorStake map[AddrKeyHash]uint64,
	poolParams *PoolRegistrationCertificate,
	snapshot RewardSnapshot,
) *PoolRewards {
	poolCost := poolParams.Cost
	margin := marginFloat(poolParams.Margin)

	// Calculate total pool stake (delegators + owners)
	totalPoolStake := uint64(0)
	for _, stake := range delegatorStake {
		totalPoolStake += stake
	}

	// Calculate operator (leader) reward
	operatorRewards := poolCost
	if totalPoolRewards > poolCost {
		// Find owner stake (owners are also delegators in the snapshot)
		ownerStake := uint64(0)
		for _, owner := range poolParams.PoolOwners {
			if stake, exists := delegatorStake[owner]; exists {
				ownerStake += stake
			}
		}

		if totalPoolStake > 0 {
			ownerStakeRatio := float64(ownerStake) / float64(totalPoolStake)
			operatorRewards += uint64(
				float64(
					totalPoolRewards-poolCost,
				) * (margin + (1.0-margin)*ownerStakeRatio),
			)
		} else {
			// If no stake, operator gets all rewards above cost
			operatorRewards = totalPoolRewards
		}
	}

	// Remaining rewards go to all stakeholders (owners and delegators)
	stakeholderRewardsTotal := totalPoolRewards - operatorRewards

	// Distribute stakeholder rewards proportionally to stake
	delegatorRewards := make(map[AddrKeyHash]uint64)

	assigned := uint64(0)
	if totalPoolStake > 0 && stakeholderRewardsTotal > 0 {
		for stakeKey, stake := range delegatorStake {
			// Only reward registered stake keys
			if snapshot.StakeRegistrations[stakeKey] {
				reward := uint64(
					float64(
						stake,
					) / float64(
						totalPoolStake,
					) * float64(
						stakeholderRewardsTotal,
					),
				)
				delegatorRewards[stakeKey] = reward
				assigned += reward
			}
		}
	}

	// Handle rounding remainder by assigning to operator
	if stakeholderRewardsTotal > assigned {
		operatorRewards += stakeholderRewardsTotal - assigned
	}

	return &PoolRewards{
		OperatorRewards:  operatorRewards,
		DelegatorRewards: delegatorRewards,
		TotalRewards:     totalPoolRewards,
	}
}

// marginFloat converts a GenesisRat margin to float64
func marginFloat(margin GenesisRat) float64 {
	// Check if margin is zero (empty cbor.Rat)
	if margin == (cbor.Rat{}) {
		return 0.0
	}
	num := margin.Num()
	den := margin.Denom()
	if num == nil || den == nil {
		return 0.0
	}
	marginRat := big.NewRat(0, 1)
	marginRat.SetFrac(num, den)
	marginFloat, _ := marginRat.Float64()
	return marginFloat
}

// CalculateOptimalPoolCount calculates the optimal number of stake pools
// totalActiveStake is unused as current implementation is simplified and only uses k parameter.
// TODO(enhancement): Implement real optimal pool count formula when Cardano spec requirements are finalized
func CalculateOptimalPoolCount(_ uint64, k uint64) uint64 {
	// Optimal pool count is approximately sqrt(total_stake / stake_per_pool)
	// where stake_per_pool is determined by the k parameter
	if k == 0 {
		return 1
	}

	// For simplicity, return k (desired number of pools)
	// In a real implementation, this would be more sophisticated
	return k
}

// ValidatePoolPledge validates that a pool meets its pledge requirement
// poolID is unused as validation only depends on pool parameters and owner stake amounts
func ValidatePoolPledge(
	_ PoolKeyHash,
	poolParams *PoolRegistrationCertificate,
	ownerStake map[AddrKeyHash]uint64,
) bool {
	if poolParams == nil {
		return false
	}

	// Calculate total owner stake
	totalOwnerStake := uint64(0)
	for _, owner := range poolParams.PoolOwners {
		if stake, exists := ownerStake[owner]; exists {
			totalOwnerStake += stake
		}
	}

	// Check if owners meet the pledge requirement
	return totalOwnerStake >= poolParams.Pledge
}

// CalculatePoolSaturation calculates the saturation level of a pool
// Returns 1.0 (fully saturated) when saturationPointStake == 0 to handle edge cases
// like very small totalActiveStake or misconfigured saturationPoint parameters.
func CalculatePoolSaturation(
	poolStake uint64,
	totalActiveStake uint64,
	saturationPoint uint64, // Usually 5% of total stake
) float64 {
	if totalActiveStake == 0 {
		return 0.0
	}

	saturationPointStake := (totalActiveStake * saturationPoint) / 100
	if saturationPointStake == 0 {
		return 1.0 // Fully saturated if saturation point is 0 (edge case handling)
	}

	saturation := float64(poolStake) / float64(saturationPointStake)
	return math.Min(saturation, 1.0) // Cap at 1.0
}
