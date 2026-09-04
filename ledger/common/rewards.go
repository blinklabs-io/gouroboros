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
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"

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
) (AdaPots, error) {
	// Calculate eta (pool performance factor)
	eta, err := calculateEta(totalBlocksInEpoch, params)
	if err != nil {
		return AdaPots{}, err
	}

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

	return newPots, nil
}

// calculateEta calculates the pool performance factor (η)
// Following Amaru's approach: eta = min(1, blocks_produced / (epoch_length * active_slot_coeff))
func calculateEta(totalBlocksInEpoch uint32, params RewardParameters) (*big.Rat, error) {
	activeSlotsCoeff := params.ActiveSlotsCoeff
	if activeSlotsCoeff == nil || activeSlotsCoeff.Sign() <= 0 {
		return nil, errors.New("active slots coefficient must be positive")
	}
	if params.ExpectedSlotsPerEpoch == 0 {
		return nil, errors.New("expected slots per epoch must be positive")
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

	return eta, nil
}

// poolSaturationThreshold (z0) is the relative stake at which a pool counts as
// fully saturated. Held as an exact rational so the reward split does not
// depend on the binary representation of 0.05. Read-only: never pass it as a
// big.Rat receiver.
// TODO(enhancement): Extract to a parameter for consistency with
// CalculatePoolSaturation
var poolSaturationThreshold = big.NewRat(1, 20)

// ratFromUint64 converts a lovelace amount to an exact rational
func ratFromUint64(value uint64) *big.Rat {
	return new(big.Rat).SetInt(new(big.Int).SetUint64(value))
}

// ratFloorUint64 returns floor(value) saturated to the uint64 range. Reward
// amounts are always floored; a stakeholder is never credited a fraction of a
// lovelace.
func ratFloorUint64(value *big.Rat) uint64 {
	if value.Sign() <= 0 {
		return 0
	}
	floor := new(big.Int).Quo(value.Num(), value.Denom())
	if !floor.IsUint64() {
		return math.MaxUint64
	}
	return floor.Uint64()
}

// sortedPoolIDs returns a map's pool IDs in ascending byte order. Reward
// arithmetic iterates this instead of the map so a result never depends on
// Go's randomized map iteration order.
func sortedPoolIDs[V any](pools map[PoolKeyHash]V) []PoolKeyHash {
	poolIDs := make([]PoolKeyHash, 0, len(pools))
	for poolID := range pools {
		poolIDs = append(poolIDs, poolID)
	}
	slices.SortFunc(poolIDs, func(a, b PoolKeyHash) int {
		return bytes.Compare(a[:], b[:])
	})
	return poolIDs
}

// calculatePoolPerformance calculates the apparent performance of a pool
// Following Amaru's approach: performance = (pool_blocks / total_blocks) * (total_stake / pool_stake)
// params is unused as performance calculation only requires block production data from snapshot
func calculatePoolPerformance(
	poolID PoolKeyHash,
	snapshot RewardSnapshot,
	_ RewardParameters,
) *big.Rat {
	// Get blocks produced by this pool
	poolBlocks := snapshot.PoolBlocks[poolID]

	// If no total blocks, assume optimal performance
	if snapshot.TotalBlocksInEpoch == 0 {
		return big.NewRat(1, 1)
	}

	// If no blocks produced, performance = 0
	if poolBlocks == 0 {
		return new(big.Rat)
	}

	// Zero-stake pools have zero performance
	poolStake := snapshot.PoolStake[poolID]
	if poolStake == 0 {
		return new(big.Rat)
	}

	// Calculate blocks ratio: pool_blocks / total_blocks
	blocksRatio := new(big.Rat).SetFrac64(
		int64(poolBlocks),
		int64(snapshot.TotalBlocksInEpoch),
	)

	// Calculate stake ratio: total_stake / pool_stake
	stakeRatio := new(big.Rat).SetFrac(
		new(big.Int).SetUint64(snapshot.TotalActiveStake),
		new(big.Int).SetUint64(poolStake),
	)

	// Performance = blocks_ratio * stake_ratio
	return new(big.Rat).Mul(blocksRatio, stakeRatio)
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
	poolShares := make(map[PoolKeyHash]*big.Rat)
	totalShare := new(big.Rat)

	// First pass: calculate raw shares for all pools
	for _, poolID := range sortedPoolIDs(snapshot.PoolStake) {
		poolStake := snapshot.PoolStake[poolID]
		poolParams, exists := snapshot.PoolParams[poolID]
		if !exists {
			continue // Skip pools without parameters
		}
		if poolParams == nil {
			return nil, fmt.Errorf(
				"pool parameters not found for pool %s",
				poolID,
			)
		}
		if err := ValidatePoolMargin(poolParams.Margin); err != nil {
			return nil, fmt.Errorf(
				"invalid margin for pool %s: %w",
				poolID,
				err,
			)
		}

		share := calculatePoolShare(
			poolStake,
			poolParams,
			snapshot,
			params,
			poolID,
		)
		poolShares[poolID] = share
		totalShare.Add(totalShare, share)
	}

	// Guard against malformed snapshot with no valid pools
	if len(poolShares) == 0 {
		return nil, errors.New("no valid pools found in reward snapshot")
	}

	// Guard against zero total share (all pools have zero share)
	if totalShare.Sign() == 0 {
		// Assign equal shares to all pools to avoid division by zero
		equalShare := big.NewRat(1, int64(len(poolShares)))
		for poolID := range poolShares {
			poolShares[poolID] = new(big.Rat).Set(equalShare)
		}
		totalShare = big.NewRat(1, 1)
	}

	// Second pass: split the reward pot across pools in proportion to their
	// shares. The amounts sum to the pot exactly and do not depend on map
	// iteration order.
	poolRewardAmounts := apportionRewardPot(pots.Rewards, poolShares, totalShare)

	// Now distribute rewards for each pool
	for _, poolID := range sortedPoolIDs(poolRewardAmounts) {
		totalPoolRewards := poolRewardAmounts[poolID]
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

// apportionRewardPot splits pot across pools in proportion to their shares.
// Each pool takes the floor of its exact share; the lovelace left unassigned
// by flooring are handed out one each by descending fractional remainder,
// breaking ties on pool ID. The result therefore sums to pot exactly and is
// identical for every node computing it from the same snapshot.
func apportionRewardPot(
	pot uint64,
	poolShares map[PoolKeyHash]*big.Rat,
	totalShare *big.Rat,
) map[PoolKeyHash]uint64 {
	poolIDs := sortedPoolIDs(poolShares)
	amounts := make(map[PoolKeyHash]uint64, len(poolIDs))
	remainders := make(map[PoolKeyHash]*big.Rat, len(poolIDs))

	potRat := ratFromUint64(pot)
	assigned := new(big.Int)
	for _, poolID := range poolIDs {
		// exact = pot * share / totalShare
		exact := new(big.Rat).Quo(poolShares[poolID], totalShare)
		exact.Mul(exact, potRat)
		if exact.Sign() < 0 {
			exact = new(big.Rat)
		}
		floor := new(big.Int).Quo(exact.Num(), exact.Denom())
		if !floor.IsUint64() {
			floor = new(big.Int).SetUint64(pot)
		}
		amounts[poolID] = floor.Uint64()
		assigned.Add(assigned, floor)
		remainders[poolID] = exact.Sub(exact, new(big.Rat).SetInt(floor))
	}

	// Each fractional remainder is below 1, so at most len(poolIDs)-1 lovelace
	// are left to hand out.
	leftover := new(big.Int).Sub(new(big.Int).SetUint64(pot), assigned)
	if leftover.Sign() <= 0 {
		return amounts
	}
	// poolIDs is a fresh slice and its ID order is no longer needed, so
	// reorder it in place by descending remainder.
	slices.SortFunc(poolIDs, func(a, b PoolKeyHash) int {
		if cmp := remainders[b].Cmp(remainders[a]); cmp != 0 {
			return cmp
		}
		return bytes.Compare(a[:], b[:])
	})
	for i := range min(leftover.Int64(), int64(len(poolIDs))) {
		amounts[poolIDs[i]]++
	}
	return amounts
}

// calculatePoolShare calculates the reward share for a single pool
func calculatePoolShare(
	poolStake uint64,
	poolParams *PoolRegistrationCertificate,
	snapshot RewardSnapshot,
	params RewardParameters,
	poolID PoolKeyHash,
) *big.Rat {
	if snapshot.TotalActiveStake == 0 {
		return new(big.Rat)
	}

	// Calculate pool performance using actual block production data
	performance := calculatePoolPerformance(poolID, snapshot, params)

	// Calculate stake ratio (pool stake / total active stake)
	stakeRatio := new(big.Rat).SetFrac(
		new(big.Int).SetUint64(poolStake),
		new(big.Int).SetUint64(snapshot.TotalActiveStake),
	)

	// Calculate saturation (capped at 1)
	one := big.NewRat(1, 1)
	saturation := new(big.Rat).Quo(stakeRatio, poolSaturationThreshold)
	if saturation.Cmp(one) > 0 {
		saturation.Set(one)
	}

	// Calculate pool reward share using leader stake influence formula
	// R_pool = (stake_ratio * performance * (1 - margin)) / (1 + a0 * saturation)
	a0 := params.PoolInfluence
	switch {
	case a0 == nil:
		a0 = big.NewRat(1, 1) // Default a0 = 1
	case a0.Sign() < 0:
		a0 = new(big.Rat) // Defensive clamp against pathological values
	}

	// Calculate numerator: stake_ratio * performance * (1 - margin)
	numerator := new(big.Rat).Mul(stakeRatio, performance)
	numerator.Mul(numerator, new(big.Rat).Sub(one, marginRat(poolParams.Margin)))

	// Calculate denominator: 1 + a0 * saturation
	denominator := new(big.Rat).Mul(a0, saturation)
	denominator.Add(denominator, one)
	if denominator.Sign() <= 0 {
		return new(big.Rat) // Prevent division by zero or negative denominator
	}

	return numerator.Quo(numerator, denominator)
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
	margin := marginRat(poolParams.Margin)

	// Calculate total pool stake (delegators + owners)
	totalPoolStake := uint64(0)
	for _, stake := range delegatorStake {
		totalPoolStake += stake
	}

	// Check if poolCost already uses up all the available rewards
	if totalPoolRewards <= poolCost {
		return &PoolRewards{
			OperatorRewards:  totalPoolRewards,
			DelegatorRewards: make(map[AddrKeyHash]uint64),
			TotalRewards:     totalPoolRewards,
		}
	}

	// Calculate operator (leader) reward
	operatorRewards := poolCost

	// Find owner stake (owners are also delegators in the snapshot)
	ownerStake := uint64(0)
	for _, owner := range poolParams.PoolOwners {
		if stake, exists := delegatorStake[owner]; exists {
			ownerStake += stake
		}
	}

	if totalPoolStake > 0 {
		variableRewardsAvailable := totalPoolRewards - poolCost
		available := ratFromUint64(variableRewardsAvailable)

		// operator_share = margin + (1 - margin) * owner_stake / total_pool_stake
		ownerStakeRatio := new(big.Rat).SetFrac(
			new(big.Int).SetUint64(ownerStake),
			new(big.Int).SetUint64(totalPoolStake),
		)
		operatorShare := new(big.Rat).Sub(big.NewRat(1, 1), margin)
		operatorShare.Mul(operatorShare, ownerStakeRatio)
		operatorShare.Add(operatorShare, margin)

		variableRewards := new(big.Rat).Mul(available, operatorShare)
		if variableRewards.Cmp(available) >= 0 {
			operatorRewards = totalPoolRewards
		} else if variableRewards.Sign() > 0 {
			operatorRewards += ratFloorUint64(variableRewards)
		}
	} else {
		// If no stake, operator gets all rewards above cost
		operatorRewards = totalPoolRewards
	}

	// Remaining rewards go to all stakeholders (owners and delegators)
	stakeholderRewardsTotal := uint64(0)
	if operatorRewards < totalPoolRewards {
		stakeholderRewardsTotal = totalPoolRewards - operatorRewards
	} else {
		// Keep the split conservative even if a malformed programmatic value
		// reaches this internal helper without passing CalculateRewards.
		operatorRewards = totalPoolRewards
	}

	// Distribute stakeholder rewards proportionally to stake
	delegatorRewards := make(map[AddrKeyHash]uint64)

	assigned := uint64(0)
	if totalPoolStake > 0 && stakeholderRewardsTotal > 0 {
		pot := new(big.Int).SetUint64(stakeholderRewardsTotal)
		total := new(big.Int).SetUint64(totalPoolStake)
		for stakeKey, stake := range delegatorStake {
			// Only reward registered stake keys
			if snapshot.StakeRegistrations[stakeKey] {
				// reward = floor(stake * stakeholder_pot / total_pool_stake)
				reward := new(big.Int).Mul(
					new(big.Int).SetUint64(stake),
					pot,
				)
				reward.Div(reward, total)
				delegatorRewards[stakeKey] = reward.Uint64()
				assigned += reward.Uint64()
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

// marginRat converts a GenesisRat margin to an exact rational clamped to
// [0, 1]. ValidatePoolMargin is the gate that rejects an out-of-range margin;
// this helper only keeps the arithmetic well defined when one reaches it.
func marginRat(margin GenesisRat) *big.Rat {
	if margin == (cbor.Rat{}) || margin.Rat == nil {
		return new(big.Rat)
	}
	num := margin.Num()
	den := margin.Denom()
	if num == nil || den == nil {
		return new(big.Rat)
	}
	if num.Sign() <= 0 {
		return new(big.Rat)
	}
	if num.Cmp(den) >= 0 {
		return big.NewRat(1, 1)
	}
	return new(big.Rat).SetFrac(num, den)
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
// The ratio is derived exactly and converted once on return, so a large
// saturationPoint cannot overflow the intermediate product.
func CalculatePoolSaturation(
	poolStake uint64,
	totalActiveStake uint64,
	saturationPoint uint64, // Usually 5% of total stake
) float64 {
	if totalActiveStake == 0 {
		return 0.0
	}

	saturationPointStake := new(big.Int).Mul(
		new(big.Int).SetUint64(totalActiveStake),
		new(big.Int).SetUint64(saturationPoint),
	)
	saturationPointStake.Div(saturationPointStake, big.NewInt(100))
	if saturationPointStake.Sign() == 0 {
		return 1.0 // Fully saturated if saturation point is 0 (edge case handling)
	}

	saturation := new(big.Rat).SetFrac(
		new(big.Int).SetUint64(poolStake),
		saturationPointStake,
	)
	if saturation.Cmp(big.NewRat(1, 1)) > 0 {
		return 1.0 // Cap at 1.0
	}
	value, _ := saturation.Float64()
	return value
}
