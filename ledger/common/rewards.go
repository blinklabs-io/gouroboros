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
	// MonetaryExpansion is rho in millionths (3000 means 0.003). It is used
	// only when ExpansionRate is nil.
	MonetaryExpansion uint64
	// TreasuryGrowth is tau in ten-thousandths (2000 means 0.2). It is used
	// only when TreasuryRate is nil.
	TreasuryGrowth uint64

	// Decentralization is d as a whole number: 0 is fully decentralized and
	// 1 or more is fully centralized. It is used only when
	// DecentralizationRate is nil.
	Decentralization uint64

	// DecentralizationRate is the decentralization parameter d in [0, 1].
	// It overrides Decentralization when set. From d >= 0.8 the reference
	// fixes eta and every pool's apparent performance at 1.
	DecentralizationRate *big.Rat

	// DesiredPools is nOpt (the k parameter). The pool reward cap z0 is
	// 1/DesiredPools; CalculateRewards fails when it is zero.
	DesiredPools uint64

	// Protocol version for reward calculation rules
	ProtocolVersion ProtocolParametersProtocolVersion

	// MaxPledgeLeverage is the Dijkstra CIP-50 pledge-to-stake cap. The
	// protocol parameter is any non-negative rational, but CalculateRewards
	// only supports [1, 10000]: below 1 cardano-ledger's maxPool' can go
	// negative, which this calculator does not model. It is inactive before
	// Dijkstra and when unset.
	MaxPledgeLeverage *big.Rat

	// MinPoolMargin is the Dijkstra CIP-23 minimum pool margin in [0, 1]. It is
	// inactive before Dijkstra and when unset or zero.
	MinPoolMargin *big.Rat

	// Minimum pool cost
	MinPoolCost uint64

	// Pool influence parameter (a0)
	PoolInfluence *big.Rat

	// ExpansionRate is rho exactly; it overrides MonetaryExpansion.
	ExpansionRate *big.Rat

	// TreasuryRate is tau exactly; it overrides TreasuryGrowth.
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

	// TotalStake is the circulation the reward update measures relative
	// stake against: the maximum lovelace supply less the reserves when the
	// update starts (circulation in cardano-ledger PulsingReward.hs). Zero
	// falls back to TotalActiveStake.
	TotalStake uint64

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
	// TotalRewards is the sum of every computed leader and member reward,
	// including rewards to unregistered credentials that UpdatedPots moves
	// to the treasury.
	TotalRewards uint64

	// Undistributed is the part of the reward pot no reward was computed
	// for (deltaR2 in cardano-ledger); UpdatedPots returns it to the
	// reserves.
	Undistributed uint64

	// Unregistered is the total of the computed rewards whose credential is
	// not registered; UpdatedPots moves it to the treasury and the
	// credentials are left out of PoolRewards.
	Unregistered uint64

	// Rewards per pool (operator + delegators)
	PoolRewards map[PoolKeyHash]PoolRewards

	// Updated ADA pots after reward distribution
	UpdatedPots AdaPots
}

// RewardCredential identifies a reward recipient by credential kind and hash.
type RewardCredential struct {
	CredType   uint
	Credential CredentialHash
}

// PoolRewards contains rewards for a specific pool
type PoolRewards struct {
	// OperatorRewardAccount receives the pool operator rewards.
	OperatorRewardAccount RewardCredential

	// Pool operator rewards
	OperatorRewards uint64

	// Delegator rewards by stake credential
	DelegatorRewards map[RewardCredential]uint64

	// Total rewards for this pool
	TotalRewards uint64
}

// CalculateAdaPots starts an epoch's reward update from the pots at the
// epoch boundary, following startStep in cardano-ledger
// eras/shelley/impl/src/Cardano/Ledger/Shelley/LedgerState/PulsingReward.hs:
//
//	deltaR1 = floor(min(1, eta) * rho * reserves)
//	rewardPot = epochFees + deltaR1
//	deltaT1 = floor(tau * rewardPot)
//	R = rewardPot - deltaT1
//
// The returned pots have deltaR1 taken from the reserves, deltaT1 added to
// the treasury and R added to the rewards pot, which CalculateRewards then
// distributes. epochFees are the fees in the stake snapshot and are moved out
// of the fee pot by the caller. totalBlocksInEpoch is the number of blocks
// stake pools made in the epoch the update covers.
func CalculateAdaPots(
	currentPots AdaPots,
	params RewardParameters,
	epochFees uint64,
	totalBlocksInEpoch uint32,
) (AdaPots, error) {
	rho, err := unitIntervalParam(
		"monetary expansion",
		params.ExpansionRate,
		params.MonetaryExpansion,
		1_000_000,
	)
	if err != nil {
		return AdaPots{}, err
	}
	tau, err := unitIntervalParam(
		"treasury growth",
		params.TreasuryRate,
		params.TreasuryGrowth,
		10_000,
	)
	if err != nil {
		return AdaPots{}, err
	}
	eta, err := calculateEta(totalBlocksInEpoch, params)
	if err != nil {
		return AdaPots{}, err
	}

	deltaR1 := ratFloorInt(
		new(big.Rat).Mul(
			new(big.Rat).Mul(eta, rho),
			ratFromUint64(currentPots.Reserves),
		),
	)
	rewardPot := new(big.Int).Add(
		new(big.Int).SetUint64(epochFees),
		deltaR1,
	)
	deltaT1 := ratFloorInt(
		new(big.Rat).Mul(tau, new(big.Rat).SetInt(rewardPot)),
	)
	r := new(big.Int).Sub(rewardPot, deltaT1)

	reserves := new(big.Int).Sub(
		new(big.Int).SetUint64(currentPots.Reserves),
		deltaR1,
	)
	treasury := new(big.Int).Add(
		new(big.Int).SetUint64(currentPots.Treasury),
		deltaT1,
	)
	rewards := new(big.Int).Add(
		new(big.Int).SetUint64(currentPots.Rewards),
		r,
	)
	if !reserves.IsUint64() || !treasury.IsUint64() || !rewards.IsUint64() {
		return AdaPots{}, errors.New("ada pots overflow uint64")
	}
	return AdaPots{
		Reserves: reserves.Uint64(),
		Treasury: treasury.Uint64(),
		Rewards:  rewards.Uint64(),
	}, nil
}

// unitIntervalParam returns the exact rate when set, and otherwise the
// legacy fixed-point value divided by scale. The reference rates are unit
// intervals.
func unitIntervalParam(
	name string,
	exact *big.Rat,
	fixed uint64,
	scale int64,
) (*big.Rat, error) {
	rate := exact
	if rate == nil {
		rate = new(big.Rat).SetFrac(
			new(big.Int).SetUint64(fixed),
			big.NewInt(scale),
		)
	}
	if rate.Sign() < 0 || rate.Cmp(big.NewRat(1, 1)) > 0 {
		return nil, fmt.Errorf(
			"%s rate %s is outside [0, 1]",
			name,
			rate.RatString(),
		)
	}
	return rate, nil
}

// decentralization returns d in [0, 1].
func decentralization(params RewardParameters) (*big.Rat, error) {
	if params.DecentralizationRate != nil {
		d := params.DecentralizationRate
		if d.Sign() < 0 || d.Cmp(big.NewRat(1, 1)) > 0 {
			return nil, fmt.Errorf(
				"decentralization %s is outside [0, 1]",
				d.RatString(),
			)
		}
		return d, nil
	}
	if params.Decentralization >= 1 {
		return big.NewRat(1, 1), nil
	}
	return new(big.Rat), nil
}

// decentralizationFixesPerformance reports d >= 0.8, from which the
// reference sets eta and every pool's apparent performance to 1.
func decentralizationFixesPerformance(d *big.Rat) bool {
	return d.Cmp(big.NewRat(4, 5)) >= 0
}

// calculateEta returns min(1, eta) for the reserve expansion. cardano-ledger
// startStep sets eta to 1 when d >= 0.8, and otherwise to
// blocksMade / floor((1 - d) * activeSlotCoeff * slotsPerEpoch).
func calculateEta(
	totalBlocksInEpoch uint32,
	params RewardParameters,
) (*big.Rat, error) {
	activeSlotsCoeff := params.ActiveSlotsCoeff
	if activeSlotsCoeff == nil || activeSlotsCoeff.Sign() <= 0 {
		return nil, errors.New("active slots coefficient must be positive")
	}
	if params.ExpectedSlotsPerEpoch == 0 {
		return nil, errors.New("expected slots per epoch must be positive")
	}
	d, err := decentralization(params)
	if err != nil {
		return nil, err
	}
	one := big.NewRat(1, 1)
	if decentralizationFixesPerformance(d) {
		return one, nil
	}
	expectedBlocks := ratFloorInt(
		new(big.Rat).Mul(
			new(big.Rat).Mul(new(big.Rat).Sub(one, d), activeSlotsCoeff),
			new(big.Rat).SetUint64(uint64(params.ExpectedSlotsPerEpoch)),
		),
	)
	if expectedBlocks.Sign() == 0 {
		return nil, errors.New("expected blocks per epoch is zero")
	}
	eta := new(big.Rat).SetFrac(
		big.NewInt(int64(totalBlocksInEpoch)),
		expectedBlocks,
	)
	if eta.Cmp(one) > 0 {
		return one, nil
	}
	return eta, nil
}

// ratFromUint64 converts a lovelace amount to an exact rational
func ratFromUint64(value uint64) *big.Rat {
	return new(big.Rat).SetInt(new(big.Int).SetUint64(value))
}

// ratFloorInt returns floor(value) for a non-negative value and 0 otherwise.
func ratFloorInt(value *big.Rat) *big.Int {
	if value.Sign() <= 0 {
		return new(big.Int)
	}
	return new(big.Int).Quo(value.Num(), value.Denom())
}

// ratFloorUint64 returns floor(value) saturated to the uint64 range. Reward
// amounts are always floored; a stakeholder is never credited a fraction of a
// lovelace.
func ratFloorUint64(value *big.Rat) uint64 {
	floor := ratFloorInt(value)
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

func pledgeLeverageActive(params RewardParameters) bool {
	return params.ProtocolVersion.Major >= ProtocolVersionDijkstra &&
		params.MaxPledgeLeverage != nil
}

func validateDijkstraRewardParameters(params RewardParameters) error {
	if params.ProtocolVersion.Major < ProtocolVersionDijkstra {
		return nil
	}
	if leverage := params.MaxPledgeLeverage; leverage != nil {
		if leverage.Cmp(big.NewRat(1, 1)) < 0 ||
			leverage.Cmp(big.NewRat(10_000, 1)) > 0 {
			return fmt.Errorf(
				"max pledge leverage %s is outside [1, 10000]",
				leverage.RatString(),
			)
		}
	}
	if minMargin := params.MinPoolMargin; minMargin != nil {
		if minMargin.Sign() < 0 || minMargin.Cmp(big.NewRat(1, 1)) > 0 {
			return fmt.Errorf(
				"minimum pool margin %s is outside [0, 1]",
				minMargin.RatString(),
			)
		}
	}
	return nil
}

func effectivePoolMargin(margin GenesisRat, params RewardParameters) *big.Rat {
	result := marginRat(margin)
	if params.ProtocolVersion.Major < ProtocolVersionDijkstraMinPoolMargin ||
		params.MinPoolMargin == nil ||
		params.MinPoolMargin.Sign() == 0 {
		return result
	}
	if result.Cmp(params.MinPoolMargin) < 0 {
		return new(big.Rat).Set(params.MinPoolMargin)
	}
	return result
}

// effectivePoolCost raises the pool's declared cost to the minimum pool cost
// from protocol version 12, as mkPoolRewardInfo and rewardOnePoolMember in
// cardano-ledger Shelley/Rewards.hs do.
func effectivePoolCost(cost uint64, params RewardParameters) uint64 {
	if params.ProtocolVersion.Major >= ProtocolVersionDijkstra &&
		cost < params.MinPoolCost {
		return params.MinPoolCost
	}
	return cost
}

func minRat(a, b *big.Rat) *big.Rat {
	if a.Cmp(b) <= 0 {
		return new(big.Rat).Set(a)
	}
	return new(big.Rat).Set(b)
}

// maxPoolReward is maxPool' in cardano-ledger
// libs/cardano-ledger-core/src/Cardano/Ledger/State/SnapShots.hs: the most a
// pool with relative stake sigma and relative pledge pledgeRatio can earn
// from the reward pot r, before apparent performance is applied.
func maxPoolReward(
	a0 *big.Rat,
	nOpt uint64,
	r uint64,
	sigma, pledgeRatio *big.Rat,
	params RewardParameters,
) *big.Rat {
	z0 := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).SetUint64(nOpt))
	sigmaCapped := minRat(sigma, z0)
	if pledgeLeverageActive(params) {
		sigmaCapped = minRat(
			sigmaCapped,
			new(big.Rat).Mul(params.MaxPledgeLeverage, pledgeRatio),
		)
	}
	pledgeCapped := minRat(pledgeRatio, z0)
	one := big.NewRat(1, 1)
	factor1 := new(big.Rat).Quo(ratFromUint64(r), new(big.Rat).Add(one, a0))
	factor4 := new(big.Rat).Quo(new(big.Rat).Sub(z0, sigmaCapped), z0)
	factor3 := new(big.Rat).Quo(
		new(big.Rat).Sub(
			sigmaCapped,
			new(big.Rat).Mul(pledgeCapped, factor4),
		),
		z0,
	)
	factor2 := new(big.Rat).Add(
		sigmaCapped,
		new(big.Rat).Mul(new(big.Rat).Mul(pledgeCapped, a0), factor3),
	)
	return new(big.Rat).Mul(factor1, factor2)
}

// apparentPerformance is mkApparentPerformance in cardano-ledger
// Shelley/Rewards.hs: the pool's share of the blocks made over its share of
// the active stake, or 1 when d >= 0.8.
func apparentPerformance(
	d *big.Rat,
	sigmaActive *big.Rat,
	poolBlocks, totalBlocks uint32,
) *big.Rat {
	if sigmaActive.Sign() == 0 {
		return new(big.Rat)
	}
	if decentralizationFixesPerformance(d) {
		return big.NewRat(1, 1)
	}
	beta := new(big.Rat).SetFrac64(
		int64(poolBlocks),
		int64(max(1, totalBlocks)),
	)
	return beta.Quo(beta, sigmaActive)
}

// leaderReward is calcStakePoolOperatorReward in cardano-ledger
// Shelley/Rewards.hs.
func leaderReward(
	poolReward, cost uint64,
	margin, ownerShare, sigma *big.Rat,
) uint64 {
	if poolReward <= cost {
		return poolReward
	}
	one := big.NewRat(1, 1)
	share := new(big.Rat).Quo(
		new(big.Rat).Mul(new(big.Rat).Sub(one, margin), ownerShare),
		sigma,
	)
	share.Add(share, margin)
	return cost + ratFloorUint64(
		new(big.Rat).Mul(ratFromUint64(poolReward-cost), share),
	)
}

// memberReward is calcStakePoolMemberReward in cardano-ledger
// Shelley/Rewards.hs.
func memberReward(
	poolReward, cost uint64,
	margin, memberShare, sigma *big.Rat,
) uint64 {
	if poolReward <= cost {
		return 0
	}
	one := big.NewRat(1, 1)
	reward := new(big.Rat).Mul(
		ratFromUint64(poolReward-cost),
		new(big.Rat).Sub(one, margin),
	)
	reward.Mul(reward, memberShare)
	return ratFloorUint64(reward.Quo(reward, sigma))
}

// poolOwnerStake sums the stake each distinct owner delegates to the pool:
// the self-delegated owners' stake that the pledge is checked against.
func poolOwnerStake(
	owners []AddrKeyHash,
	delegatorStake map[AddrKeyHash]uint64,
) (uint64, map[AddrKeyHash]bool) {
	seen := make(map[AddrKeyHash]bool, len(owners))
	total := uint64(0)
	for _, owner := range owners {
		if seen[owner] {
			continue
		}
		seen[owner] = true
		total += delegatorStake[owner]
	}
	return total, seen
}

// CalculateRewards distributes the reward pot pots.Rewards (R, from
// CalculateAdaPots) to the pools that made blocks and their members, as
// startStep, mkPoolRewardInfo and rewardOnePoolMember in cardano-ledger
// (Shelley/LedgerState/PulsingReward.hs, Shelley/Rewards.hs) do:
//
//   - A pool that made no blocks earns nothing.
//   - A pool earns floor(apparentPerformance * maxPool'), where maxPool' is
//     0 when its pledge exceeds its owners' delegated stake.
//   - The leader takes the cost plus margin and owner share of the rest; each
//     member that is not an owner takes its stake share of the rest after
//     margin. Every amount is floored.
//   - Before protocol version 7 a reward is computed only for a registered
//     credential. From version 7 rewards are computed for every credential,
//     and those to unregistered credentials go to the treasury when applied.
//
// Relative stake is measured against snapshot.TotalStake. The part of R no
// reward was computed for (deltaR2) returns to the reserves: UpdatedPots has
// it added to the reserves, unregistered rewards added to the treasury, and
// the rewards pot emptied. Registration is read from
// snapshot.StakeRegistrations by credential hash.
//
// Before protocol version 3 a credential is paid only one reward, as
// filterRewards in cardano-ledger Shelley/Rewards.hs does: the least under
// Ord Reward, which puts leader rewards before member rewards and orders
// rewards of one kind by pool key hash. A leader reward counts even when it
// is zero. The credential's other rewards are left out of PoolRewards and
// return to the reserves with deltaR2. From version 3 every reward is paid,
// and the caller applying the result sums a credential's rewards across
// pools.
func CalculateRewards(
	pots AdaPots,
	snapshot RewardSnapshot,
	params RewardParameters,
) (*RewardCalculationResult, error) {
	if err := validateDijkstraRewardParameters(params); err != nil {
		return nil, err
	}
	if params.DesiredPools == 0 {
		return nil, errors.New("desired number of pools (nOpt) is zero")
	}
	d, err := decentralization(params)
	if err != nil {
		return nil, err
	}
	a0 := params.PoolInfluence
	switch {
	case a0 == nil:
		a0 = big.NewRat(1, 1)
	case a0.Sign() < 0:
		return nil, fmt.Errorf(
			"pool influence %s is negative",
			a0.RatString(),
		)
	}
	totalStake := snapshot.TotalStake
	if totalStake == 0 {
		totalStake = snapshot.TotalActiveStake
	}
	totalStakeRat := ratFromUint64(totalStake)
	forgoPrefilter := params.ProtocolVersion.Major > 6
	aggregated := params.ProtocolVersion.Major > 2
	owed := make(map[RewardCredential][]owedReward)
	registered := func(hash AddrKeyHash) bool {
		return snapshot.StakeRegistrations[hash]
	}

	result := &RewardCalculationResult{
		PoolRewards: make(map[PoolKeyHash]PoolRewards),
		UpdatedPots: pots,
	}
	distributed := new(big.Int)
	unregistered := new(big.Int)
	if totalStake > 0 && snapshot.TotalActiveStake > 0 && pots.Rewards > 0 {
		for _, poolID := range sortedPoolIDs(snapshot.PoolParams) {
			poolParams := snapshot.PoolParams[poolID]
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
			blocks := snapshot.PoolBlocks[poolID]
			if blocks == 0 {
				continue
			}
			delegators := snapshot.DelegatorStake[poolID]
			ownerStake, owners := poolOwnerStake(
				poolParams.PoolOwners,
				delegators,
			)
			poolStake := ratFromUint64(snapshot.PoolStake[poolID])
			sigma := new(big.Rat).Quo(poolStake, totalStakeRat)
			sigmaActive := new(big.Rat).Quo(
				poolStake,
				ratFromUint64(snapshot.TotalActiveStake),
			)
			maxP := new(big.Rat)
			if poolParams.Pledge <= ownerStake {
				maxP = ratFromUint64(ratFloorUint64(maxPoolReward(
					a0,
					params.DesiredPools,
					pots.Rewards,
					sigma,
					new(big.Rat).Quo(
						ratFromUint64(poolParams.Pledge),
						totalStakeRat,
					),
					params,
				)))
			}
			poolReward := ratFloorUint64(new(big.Rat).Mul(
				apparentPerformance(
					d,
					sigmaActive,
					blocks,
					snapshot.TotalBlocksInEpoch,
				),
				maxP,
			))
			cost := effectivePoolCost(poolParams.Cost, params)
			margin := effectivePoolMargin(poolParams.Margin, params)

			account := poolParams.RewardAccountCredential()
			pr := PoolRewards{
				OperatorRewardAccount: rewardCredential(account),
				DelegatorRewards:      make(map[RewardCredential]uint64),
				TotalRewards:          poolReward,
			}
			credit := func(hash AddrKeyHash, amount uint64) bool {
				if amount == 0 {
					return false
				}
				if !registered(hash) {
					if !forgoPrefilter {
						return false
					}
					unregistered.Add(
						unregistered,
						new(big.Int).SetUint64(amount),
					)
					distributed.Add(
						distributed,
						new(big.Int).SetUint64(amount),
					)
					return false
				}
				distributed.Add(distributed, new(big.Int).SetUint64(amount))
				return true
			}

			leader := leaderReward(
				poolReward,
				cost,
				margin,
				new(big.Rat).Quo(ratFromUint64(ownerStake), totalStakeRat),
				sigma,
			)
			if credit(AddrKeyHash(account.Credential), leader) {
				pr.OperatorRewards = leader
			}
			if !aggregated && registered(AddrKeyHash(account.Credential)) {
				owed[pr.OperatorRewardAccount] = append(
					owed[pr.OperatorRewardAccount],
					owedReward{leader: true, pool: poolID, amount: leader},
				)
			}
			members := make([]AddrKeyHash, 0, len(delegators))
			for member := range delegators {
				if !owners[member] {
					members = append(members, member)
				}
			}
			slices.SortFunc(members, func(a, b AddrKeyHash) int {
				return bytes.Compare(a[:], b[:])
			})
			for _, member := range members {
				reward := memberReward(
					poolReward,
					cost,
					margin,
					new(big.Rat).Quo(
						ratFromUint64(delegators[member]),
						totalStakeRat,
					),
					sigma,
				)
				if credit(member, reward) {
					pr.DelegatorRewards[keyRewardCredential(member)] = reward
					if !aggregated {
						owed[keyRewardCredential(member)] = append(
							owed[keyRewardCredential(member)],
							owedReward{pool: poolID, amount: reward},
						)
					}
				}
			}
			result.PoolRewards[poolID] = pr
		}
	}

	for credential, rewards := range owed {
		if len(rewards) < 2 {
			continue
		}
		paid := slices.MinFunc(rewards, compareOwedRewards)
		for _, reward := range rewards {
			if reward == paid {
				continue
			}
			pr := result.PoolRewards[reward.pool]
			if reward.leader {
				pr.OperatorRewards = 0
			} else {
				delete(pr.DelegatorRewards, credential)
			}
			result.PoolRewards[reward.pool] = pr
			distributed.Sub(
				distributed,
				new(big.Int).SetUint64(reward.amount),
			)
		}
	}

	pot := new(big.Int).SetUint64(pots.Rewards)
	if distributed.Cmp(pot) > 0 {
		return nil, fmt.Errorf(
			"rewards %s exceed the reward pot %d: delegator stake exceeds pool stake",
			distributed,
			pots.Rewards,
		)
	}
	undistributed := new(big.Int).Sub(pot, distributed)
	reserves := new(big.Int).Add(
		new(big.Int).SetUint64(pots.Reserves),
		undistributed,
	)
	treasury := new(big.Int).Add(
		new(big.Int).SetUint64(pots.Treasury),
		unregistered,
	)
	if !reserves.IsUint64() || !treasury.IsUint64() {
		return nil, errors.New("ada pots overflow uint64")
	}
	result.TotalRewards = distributed.Uint64()
	result.Undistributed = undistributed.Uint64()
	result.Unregistered = unregistered.Uint64()
	result.UpdatedPots = AdaPots{
		Reserves: reserves.Uint64(),
		Treasury: treasury.Uint64(),
		Rewards:  0,
	}
	return result, nil
}

// owedReward is one reward computed for a credential before protocol
// version 3, when the credential is paid only the least of them.
type owedReward struct {
	leader bool
	pool   PoolKeyHash
	amount uint64
}

// compareOwedRewards is Ord Reward in cardano-ledger Rewards.hs: leader
// rewards first, then by pool key hash; the amount is not compared.
func compareOwedRewards(a, b owedReward) int {
	if a.leader != b.leader {
		if a.leader {
			return -1
		}
		return 1
	}
	return bytes.Compare(a.pool[:], b.pool[:])
}

func rewardCredential(credential Credential) RewardCredential {
	return RewardCredential{
		CredType:   credential.CredType,
		Credential: credential.Credential,
	}
}

func keyRewardCredential(hash AddrKeyHash) RewardCredential {
	return RewardCredential{
		CredType:   CredentialTypeAddrKeyHash,
		Credential: CredentialHash(hash),
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

// ValidatePoolPledge reports whether the stake the pool's owners delegate
// meets its pledge. Owners are a set, as in cardano-ledger: an owner listed
// more than once counts once. poolID is unused.
func ValidatePoolPledge(
	_ PoolKeyHash,
	poolParams *PoolRegistrationCertificate,
	ownerStake map[AddrKeyHash]uint64,
) bool {
	if poolParams == nil {
		return false
	}
	total, _ := poolOwnerStake(poolParams.PoolOwners, ownerStake)
	return total >= poolParams.Pledge
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
