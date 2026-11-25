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
