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
	"fmt"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// The expected values in this file were computed with exact rationals from
// a transcription of cardano-ledger (revision 1210d37f):
//
//   - startStep, eras/shelley/impl/src/Cardano/Ledger/Shelley/LedgerState/PulsingReward.hs:
//     deltaR1 = floor(min 1 eta * rho * reserves), eta = 1 when d >= 0.8 and
//     otherwise blocksMade / floor((1-d) * f * slotsPerEpoch);
//     deltaT1 = floor(tau * (fees + deltaR1)); R = fees + deltaR1 - deltaT1.
//   - maxPool', libs/cardano-ledger-core/src/Cardano/Ledger/State/SnapShots.hs.
//   - mkApparentPerformance, calcStakePoolOperatorReward,
//     calcStakePoolMemberReward, mkPoolRewardInfo and rewardOnePoolMember,
//     eras/shelley/impl/src/Cardano/Ledger/Shelley/Rewards.hs.
//   - completeRupd (deltaR2 = R - sum of rewards back to the reserves) and
//     applyRUpdFiltered (rewards to unregistered credentials to the
//     treasury), Shelley/LedgerState/PulsingReward.hs and IncrementalStake.hs.
//
// None of them comes from running gOuroboros.

func TestCalculateAdaPotsReferenceTable(t *testing.T) {
	cases := []struct {
		reserves, fees uint64
		rho, tau, d    *big.Rat
		blocks         uint32
		deltaR1        uint64
		deltaT1        uint64
		r              uint64
	}{
		// eta = 21000 / 21600
		{10_000_000_000, 12_345, big.NewRat(3, 1000), big.NewRat(1, 5), big.NewRat(0, 1), 21000, 29_166_666, 5_835_802, 23_343_209},
		// d = 1/2: expected blocks 10800, eta capped at 1
		{10_000_000_000, 12_345, big.NewRat(3, 1000), big.NewRat(1, 5), big.NewRat(1, 2), 21000, 30_000_000, 6_002_469, 24_009_876},
		// d = 1/2: eta = 5000 / 10800
		{10_000_000_000, 12_345, big.NewRat(3, 1000), big.NewRat(1, 5), big.NewRat(1, 2), 5000, 13_888_888, 2_780_246, 11_120_987},
		// d >= 0.8: eta is 1 whatever the blocks
		{10_000_000_000, 12_345, big.NewRat(3, 1000), big.NewRat(1, 5), big.NewRat(9, 10), 7, 30_000_000, 6_002_469, 24_009_876},
		// rates that are not decimal fractions
		{999_999, 7, big.NewRat(1, 3), big.NewRat(1, 7), big.NewRat(0, 1), 21599, 333_317, 47_617, 285_707},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("case%d", i), func(t *testing.T) {
			const oldRewards, treasury = 5, 1_000
			params := common.RewardParameters{
				ExpansionRate:         tc.rho,
				TreasuryRate:          tc.tau,
				DecentralizationRate:  tc.d,
				ActiveSlotsCoeff:      big.NewRat(1, 20),
				ExpectedSlotsPerEpoch: 432_000,
			}
			got, err := common.CalculateAdaPots(
				common.AdaPots{
					Reserves: tc.reserves,
					Treasury: treasury,
					Rewards:  oldRewards,
				},
				params,
				tc.fees,
				tc.blocks,
			)
			require.NoError(t, err)
			require.Equal(t, common.AdaPots{
				Reserves: tc.reserves - tc.deltaR1,
				Treasury: treasury + tc.deltaT1,
				Rewards:  oldRewards + tc.r,
			}, got)
		})
	}
}

func TestCalculateAdaPotsFixedPointRatesMatchExactRates(t *testing.T) {
	pots := common.AdaPots{Reserves: 10_000_000_000}
	exact, err := common.CalculateAdaPots(pots, common.RewardParameters{
		ExpansionRate:         big.NewRat(3, 1000),
		TreasuryRate:          big.NewRat(1, 5),
		ActiveSlotsCoeff:      big.NewRat(1, 20),
		ExpectedSlotsPerEpoch: 432_000,
	}, 12_345, 21000)
	require.NoError(t, err)
	fixed, err := common.CalculateAdaPots(pots, common.RewardParameters{
		MonetaryExpansion:     3000,
		TreasuryGrowth:        2000,
		ActiveSlotsCoeff:      big.NewRat(1, 20),
		ExpectedSlotsPerEpoch: 432_000,
	}, 12_345, 21000)
	require.NoError(t, err)
	require.Equal(t, exact, fixed)
}

// rewardReferenceSnapshot is four pools measured against a circulation of
// 1,000,000 and an active stake of 800,000, with nOpt 10 (z0 = 1/10) and
// a0 = 3/10:
//
//   - P1: sigma 0.08, pledge met, two members, M2 unregistered;
//   - P2: sigma 0.15, above z0, apparent performance above 1;
//   - P3: pledge 30,000 above its owner's 10,000, so maxPool is 0;
//   - P4: made no blocks.
func rewardReferenceSnapshot(pv uint) (common.AdaPots, common.RewardSnapshot, common.RewardParameters, map[string]common.AddrKeyHash, map[string]common.PoolKeyHash) {
	key := map[string]common.AddrKeyHash{}
	for i, name := range []string{"O1", "M1", "M2", "O2", "M3", "O3", "M4", "M5", "OP1", "OP2", "OP3", "OP4"} {
		key[name] = common.AddrKeyHash{0xA0, byte(i)}
	}
	pool := map[string]common.PoolKeyHash{
		"P1": {0x01}, "P2": {0x02}, "P3": {0x03}, "P4": {0x04},
	}
	mkParams := func(op string, owner string, pledge, cost uint64, margin *big.Rat) *common.PoolRegistrationCertificate {
		cert := &common.PoolRegistrationCertificate{
			Pledge: pledge,
			Cost:   cost,
			Margin: common.NewGenesisRat(margin.Num().Int64(), margin.Denom().Int64()),
		}
		if owner != "" {
			cert.PoolOwners = []common.AddrKeyHash{key[owner]}
		}
		cert.RewardAccount = key[op]
		return cert
	}
	snapshot := common.RewardSnapshot{
		TotalStake:       1_000_000,
		TotalActiveStake: 800_000,
		PoolStake: map[common.PoolKeyHash]uint64{
			pool["P1"]: 80_000, pool["P2"]: 150_000,
			pool["P3"]: 60_000, pool["P4"]: 100_000,
		},
		DelegatorStake: map[common.PoolKeyHash]map[common.AddrKeyHash]uint64{
			pool["P1"]: {key["O1"]: 20_000, key["M1"]: 50_000, key["M2"]: 10_000},
			pool["P2"]: {key["O2"]: 5_000, key["M3"]: 145_000},
			pool["P3"]: {key["O3"]: 10_000, key["M4"]: 50_000},
			pool["P4"]: {key["M5"]: 100_000},
		},
		PoolParams: map[common.PoolKeyHash]*common.PoolRegistrationCertificate{
			pool["P1"]: mkParams("OP1", "O1", 10_000, 340, big.NewRat(1, 20)),
			pool["P2"]: mkParams("OP2", "O2", 5_000, 500, big.NewRat(0, 1)),
			pool["P3"]: mkParams("OP3", "O3", 30_000, 340, big.NewRat(1, 10)),
			pool["P4"]: mkParams("OP4", "", 0, 340, big.NewRat(0, 1)),
		},
		StakeRegistrations: map[common.AddrKeyHash]bool{},
		PoolBlocks: map[common.PoolKeyHash]uint32{
			pool["P1"]: 10, pool["P2"]: 20, pool["P3"]: 5,
		},
		TotalBlocksInEpoch: 100,
	}
	for _, name := range []string{"O1", "M1", "O2", "M3", "O3", "M4", "OP1", "OP2", "OP3"} {
		snapshot.StakeRegistrations[key[name]] = true
	}
	params := common.RewardParameters{
		DesiredPools:    10,
		PoolInfluence:   big.NewRat(3, 10),
		ProtocolVersion: common.ProtocolParametersProtocolVersion{Major: pv},
	}
	pots := common.AdaPots{Reserves: 7_000, Treasury: 3_000, Rewards: 1_000_000_000}
	return pots, snapshot, params, key, pool
}

func TestCalculateRewardsReferenceTable(t *testing.T) {
	for _, tc := range []struct {
		pv           uint
		unregistered uint64
	}{
		// Before protocol version 7 the unregistered member M2 is never
		// credited, so its share stays in deltaR2 and returns to the reserves.
		{6, 0},
		// From version 7 M2's reward is computed and goes to the treasury.
		{8, 7_521_401},
	} {
		t.Run(fmt.Sprintf("pv%d", tc.pv), func(t *testing.T) {
			pots, snapshot, params, key, pool := rewardReferenceSnapshot(tc.pv)
			got, err := common.CalculateRewards(pots, snapshot, params)
			require.NoError(t, err)

			member := func(name string) common.RewardCredential {
				return common.RewardCredential{
					CredType:   common.CredentialTypeAddrKeyHash,
					Credential: common.CredentialHash(key[name]),
				}
			}
			p1 := got.PoolRewards[pool["P1"]]
			require.Equal(t, uint64(63_338_461), p1.TotalRewards)
			require.Equal(t, uint64(18_210_049), p1.OperatorRewards)
			require.Equal(t, map[common.RewardCredential]uint64{
				member("M1"): 37_607_009,
			}, p1.DelegatorRewards)

			p2 := got.PoolRewards[pool["P2"]]
			require.Equal(t, uint64(83_282_051), p2.TotalRewards)
			require.Equal(t, uint64(2_776_551), p2.OperatorRewards)
			require.Equal(t, map[common.RewardCredential]uint64{
				member("M3"): 80_505_499,
			}, p2.DelegatorRewards)

			p3 := got.PoolRewards[pool["P3"]]
			require.Zero(t, p3.TotalRewards)
			require.Zero(t, p3.OperatorRewards)
			require.Empty(t, p3.DelegatorRewards)

			_, p4 := got.PoolRewards[pool["P4"]]
			require.False(t, p4, "a pool that made no blocks has no reward entry")

			paid := uint64(18_210_049 + 37_607_009 + 2_776_551 + 80_505_499)
			require.Equal(t, paid+tc.unregistered, got.TotalRewards)
			require.Equal(t, tc.unregistered, got.Unregistered)
			require.Equal(t, pots.Rewards-paid-tc.unregistered, got.Undistributed)
			require.Equal(t, common.AdaPots{
				Reserves: pots.Reserves + got.Undistributed,
				Treasury: pots.Treasury + tc.unregistered,
			}, got.UpdatedPots)
		})
	}
}

// TestRewardUpdateConservesAda walks an epoch boundary through
// CalculateAdaPots and CalculateRewards and checks the reference's value
// preservation: reserves + treasury + rewards pot + credited rewards + fees
// is unchanged, the treasury gains exactly deltaT1 plus unregistered rewards,
// and the reserves lose exactly deltaR1 less deltaR2.
func TestRewardUpdateConservesAda(t *testing.T) {
	for _, pv := range []uint{2, 6, 7, 9, 10} {
		for _, blocks := range []uint32{0, 1, 35, 100, 21_600} {
			t.Run(fmt.Sprintf("pv%d/blocks%d", pv, blocks), func(t *testing.T) {
				startPots, snapshot, params, _, _ := rewardReferenceSnapshot(pv)
				startPots.Rewards = 0
				startPots.Reserves = 10_000_000_000
				params.ExpansionRate = big.NewRat(3, 1000)
				params.TreasuryRate = big.NewRat(1, 5)
				params.ActiveSlotsCoeff = big.NewRat(1, 20)
				params.ExpectedSlotsPerEpoch = 432_000
				const fees = 12_345
				before := startPots.Reserves + startPots.Treasury + fees

				pots, err := common.CalculateAdaPots(startPots, params, fees, blocks)
				require.NoError(t, err)
				require.Equal(t, before, pots.Reserves+pots.Treasury+pots.Rewards)

				result, err := common.CalculateRewards(pots, snapshot, params)
				require.NoError(t, err)
				credited := uint64(0)
				for _, pr := range result.PoolRewards {
					credited += pr.OperatorRewards
					for _, r := range pr.DelegatorRewards {
						credited += r
					}
				}
				require.Equal(t, result.TotalRewards, credited+result.Unregistered)
				after := result.UpdatedPots
				require.Zero(t, after.Rewards)
				require.Equal(t, before, after.Reserves+after.Treasury+credited)
				require.Equal(t, pots.Treasury+result.Unregistered, after.Treasury)
				require.Equal(t, pots.Reserves+result.Undistributed, after.Reserves)
			})
		}
	}
}

// refFloor, refMaxPool, refLeader and refMember transcribe maxPool',
// calcStakePoolOperatorReward and calcStakePoolMemberReward from
// cardano-ledger (SnapShots.hs, Shelley/Rewards.hs) for the cell walk below.
func refFloor(x *big.Rat) uint64 {
	if x.Sign() <= 0 {
		return 0
	}
	return new(big.Int).Quo(x.Num(), x.Denom()).Uint64()
}

func refMaxPool(a0 *big.Rat, nOpt int64, r uint64, sigma, pledge *big.Rat) uint64 {
	z0 := big.NewRat(1, nOpt)
	minR := func(a, b *big.Rat) *big.Rat {
		if a.Cmp(b) <= 0 {
			return a
		}
		return b
	}
	s, p := minR(sigma, z0), minR(pledge, z0)
	one := big.NewRat(1, 1)
	f1 := new(big.Rat).Quo(new(big.Rat).SetUint64(r), new(big.Rat).Add(one, a0))
	f4 := new(big.Rat).Quo(new(big.Rat).Sub(z0, s), z0)
	f3 := new(big.Rat).Quo(new(big.Rat).Sub(s, new(big.Rat).Mul(p, f4)), z0)
	f2 := new(big.Rat).Add(s, new(big.Rat).Mul(new(big.Rat).Mul(p, a0), f3))
	return refFloor(new(big.Rat).Mul(f1, f2))
}

func refLeader(f, c uint64, m, s, sigma *big.Rat) uint64 {
	if f <= c {
		return f
	}
	one := big.NewRat(1, 1)
	share := new(big.Rat).Add(m, new(big.Rat).Quo(new(big.Rat).Mul(new(big.Rat).Sub(one, m), s), sigma))
	return c + refFloor(new(big.Rat).Mul(new(big.Rat).SetUint64(f-c), share))
}

func refMember(f, c uint64, m, t, sigma *big.Rat) uint64 {
	if f <= c {
		return 0
	}
	one := big.NewRat(1, 1)
	x := new(big.Rat).Mul(new(big.Rat).SetUint64(f-c), new(big.Rat).Sub(one, m))
	x.Mul(x, t)
	return refFloor(x.Quo(x, sigma))
}

// TestRewardUpdateCells walks one pool (an owner who is the leader and one
// member) through every combination of protocol version on each side of the
// version-7 prefilter change, blocks made (none, fewer than its stake share,
// its share, more), pledge met or not, and the member registered or not, and
// checks on each cell:
//
//   - a pool without blocks, or with its pledge unmet, earns nothing;
//   - the pool's reward is floor(apparentPerformance * maxPool');
//   - the leader and member rewards are the reference's, the member's is not
//     computed before version 7 when unregistered, and goes to the treasury
//     from version 7;
//   - every lovelace of the pot is a reward or returns to the reserves.
func TestRewardUpdateCells(t *testing.T) {
	const (
		owner, member   = 6_000_000, 4_000_000
		circulation     = 40_000_000
		pot             = 25_000_000
		cost            = 340
		totalBlocks     = 10
		nOpt            = 2
		reservesAtStart = 60_000_000
		treasuryAtStart = 1_000
	)
	ownerKey, memberKey := common.AddrKeyHash{0x01}, common.AddrKeyHash{0x02}
	poolID := common.PoolKeyHash{0x0A}
	a0, margin := big.NewRat(1, 2), big.NewRat(1, 10)
	for _, pv := range []uint{6, 8} {
		for _, blocks := range []uint32{0, 3, 10} {
			for _, pledge := range []uint64{owner, owner + 1} {
				for _, memberRegistered := range []bool{true, false} {
					name := fmt.Sprintf("pv%d/blocks%d/pledge%d/memberReg=%v", pv, blocks, pledge, memberRegistered)
					t.Run(name, func(t *testing.T) {
						snapshot := common.RewardSnapshot{
							TotalStake:       circulation,
							TotalActiveStake: 2 * (owner + member),
							PoolStake:        map[common.PoolKeyHash]uint64{poolID: owner + member},
							DelegatorStake: map[common.PoolKeyHash]map[common.AddrKeyHash]uint64{
								poolID: {ownerKey: owner, memberKey: member},
							},
							PoolParams: map[common.PoolKeyHash]*common.PoolRegistrationCertificate{
								poolID: {
									Pledge:        pledge,
									Cost:          cost,
									Margin:        common.NewGenesisRat(1, 10),
									PoolOwners:    []common.AddrKeyHash{ownerKey},
									RewardAccount: ownerKey,
								},
							},
							StakeRegistrations: map[common.AddrKeyHash]bool{
								ownerKey:  true,
								memberKey: memberRegistered,
							},
							PoolBlocks:         map[common.PoolKeyHash]uint32{poolID: blocks},
							TotalBlocksInEpoch: totalBlocks,
						}
						params := common.RewardParameters{
							DesiredPools:    nOpt,
							PoolInfluence:   a0,
							ProtocolVersion: common.ProtocolParametersProtocolVersion{Major: pv},
						}
						pots := common.AdaPots{Reserves: reservesAtStart, Treasury: treasuryAtStart, Rewards: pot}
						got, err := common.CalculateRewards(pots, snapshot, params)
						require.NoError(t, err)

						sigma := big.NewRat(owner+member, circulation)
						var poolReward, leader, memberReward uint64
						if blocks > 0 && pledge <= owner {
							maxP := refMaxPool(a0, nOpt, pot, sigma, big.NewRat(int64(pledge), circulation))
							// beta / sigmaA: blocks/10 over a half share of the active stake
							perf := new(big.Rat).Quo(big.NewRat(int64(blocks), totalBlocks), big.NewRat(1, 2))
							poolReward = refFloor(new(big.Rat).Mul(perf, new(big.Rat).SetUint64(maxP)))
							leader = refLeader(poolReward, cost, margin, big.NewRat(owner, circulation), sigma)
							memberReward = refMember(poolReward, cost, margin, big.NewRat(member, circulation), sigma)
						}
						var paidMember, unregistered uint64
						switch {
						case memberRegistered:
							paidMember = memberReward
						case pv >= 7:
							unregistered = memberReward
						}

						pr, made := got.PoolRewards[poolID]
						require.Equal(t, blocks > 0, made)
						require.Equal(t, poolReward, pr.TotalRewards)
						require.Equal(t, leader, pr.OperatorRewards)
						if paidMember > 0 {
							require.Equal(t, map[common.RewardCredential]uint64{
								{CredType: common.CredentialTypeAddrKeyHash, Credential: common.CredentialHash(memberKey)}: paidMember,
							}, pr.DelegatorRewards)
						} else {
							require.Empty(t, pr.DelegatorRewards)
						}
						require.Equal(t, unregistered, got.Unregistered)
						require.Equal(t, leader+paidMember+unregistered, got.TotalRewards)
						require.Equal(t, uint64(pot), got.TotalRewards+got.Undistributed)
						require.Equal(t, common.AdaPots{
							Reserves: reservesAtStart + got.Undistributed,
							Treasury: treasuryAtStart + unregistered,
						}, got.UpdatedPots)
					})
				}
			}
		}
	}
}

// TestValidatePoolPledgeCountsEachOwnerOnce checks that a pool owner listed
// twice counts once. cardano-ledger (1210d37f) keeps a pool's owners as a
// set (sppOwners in libs/cardano-ledger-core/src/Cardano/Ledger/State/
// StakePool.hs) and sums the self-delegated owners' stake over that set
// (SnapShots.hs 230-235) before comparing it with the pledge
// (mkPoolRewardInfo in Shelley/Rewards.hs).
func TestValidatePoolPledgeCountsEachOwnerOnce(t *testing.T) {
	owner, other := common.AddrKeyHash{0x01}, common.AddrKeyHash{0x02}
	stake := map[common.AddrKeyHash]uint64{owner: 6, other: 3}
	for _, tc := range []struct {
		name   string
		owners []common.AddrKeyHash
		pledge uint64
		met    bool
	}{
		{"once, met", []common.AddrKeyHash{owner}, 6, true},
		{"once, unmet", []common.AddrKeyHash{owner}, 7, false},
		{"repeated, unmet", []common.AddrKeyHash{owner, owner}, 7, false},
		{"repeated, met at set sum", []common.AddrKeyHash{owner, owner}, 6, true},
		{"two owners, met", []common.AddrKeyHash{owner, other}, 9, true},
		{"two owners, one repeated, unmet", []common.AddrKeyHash{owner, other, other}, 10, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.met, common.ValidatePoolPledge(
				common.PoolKeyHash{},
				&common.PoolRegistrationCertificate{Pledge: tc.pledge, PoolOwners: tc.owners},
				stake,
			))
		})
	}
}
