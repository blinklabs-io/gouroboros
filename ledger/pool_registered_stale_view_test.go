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

package ledger_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// TestPoolRegisteredStaleView drives every rule that asks whether a pool is
// registered with a ledger state that still reports the registration of a
// pool whose retirement has taken effect. The reference asks one question,
// membership of psStakePools, and POOLREAP removes the pool at the start of
// its retirement epoch (Shelley Deleg.hs 330-331, Pool.hs 299, Conway
// Deleg.hs 219-222, Conway Gov.hs 473 and 596). The mock derives
// IsPoolRegistered from PoolCurrentState, so it answers true for such a pool.
func TestPoolRegisteredStaleView(t *testing.T) {
	t.Parallel()
	const currentEpoch = 5
	pool := common.PoolKeyHash(common.NewBlake2b224([]byte("stale-view-pool")))
	stake := stakeKeyCredential(0x51)

	stateWithRetirement := func(retirementEpoch uint64) common.LedgerState {
		return mockledger.NewLedgerStateBuilder().
			WithNetworkId(common.AddressNetworkMainnet).
			WithStakeRegistrations([]common.StakeRegistrationCertificate{
				{StakeCredential: stake},
			}).
			WithPoolCurrentState(func(
				operator common.PoolKeyHash,
			) (*common.PoolRegistrationCertificate, *uint64, error) {
				if operator != pool {
					return nil, nil, nil
				}
				return &common.PoolRegistrationCertificate{
					Operator: pool,
				}, &retirementEpoch, nil
			}).
			WithEpochForSlot(func(uint64) (uint64, error) {
				return currentEpoch, nil
			}).
			Build()
	}

	shelleyDelegation := func(ls common.LedgerState) error {
		pp := mockledger.NewMockShelleyProtocolParams()
		return shelley.UtxoValidateDelegation(
			poolCertTx(&common.StakeDelegationCertificate{
				CertType:        uint(common.CertificateTypeStakeDelegation),
				StakeCredential: &stake,
				PoolKeyHash:     pool,
			}),
			0, ls, &pp,
		)
	}
	shelleyRetirement := func(ls common.LedgerState) error {
		return shelley.UtxoValidatePoolCertificates(
			poolCertTx(&common.PoolRetirementCertificate{
				CertType:    uint(common.CertificateTypePoolRetirement),
				PoolKeyHash: pool,
				Epoch:       currentEpoch + 2,
			}),
			0, ls, maryPparams(0),
		)
	}
	conwayPparams := func() *conway.ConwayProtocolParameters {
		return &conway.ConwayProtocolParameters{
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: common.ProtocolVersionPlomin,
			},
		}
	}
	conwayDelegation := func(ls common.LedgerState) error {
		tx := &conway.ConwayTransaction{TxIsValid: true}
		tx.Body.TxCertificates = []common.CertificateWrapper{{
			Type: uint(common.CertificateTypeStakeDelegation),
			Certificate: &common.StakeDelegationCertificate{
				CertType:        uint(common.CertificateTypeStakeDelegation),
				StakeCredential: &stake,
				PoolKeyHash:     pool,
			},
		}}
		return conway.UtxoValidateDelegation(tx, 0, ls, conwayPparams())
	}
	conwayPoolVoter := func(ls common.LedgerState) error {
		voter := common.Voter{
			Type: common.VoterTypeStakingPoolKeyHash,
			Hash: common.Blake2b224(pool),
		}
		actionId := common.GovActionId{TransactionId: common.Blake2b256{0x51}}
		tx := &conway.ConwayTransaction{TxIsValid: true}
		tx.Body.TxVotingProcedures = common.VotingProcedures{
			&voter: {&actionId: common.VotingProcedure{Vote: common.GovVoteYes}},
		}
		return conway.UtxoValidateUnknownVoters(tx, 0, ls, conwayPparams())
	}

	rules := []struct {
		name     string
		validate func(common.LedgerState) error
		rejected func(*testing.T, error)
	}{
		{
			name:     "Shelley DELEG delegation",
			validate: shelleyDelegation,
			rejected: func(t *testing.T, err error) {
				var target shelley.DelegateToUnregisteredPoolError
				require.ErrorAs(t, err, &target)
			},
		},
		{
			name:     "Shelley POOL retirement",
			validate: shelleyRetirement,
			rejected: func(t *testing.T, err error) {
				var target shelley.StakePoolNotRegisteredOnKeyError
				require.ErrorAs(t, err, &target)
			},
		},
		{
			name:     "Conway DELEG delegation",
			validate: conwayDelegation,
			rejected: func(t *testing.T, err error) {
				var target conway.DelegateToUnregisteredPoolError
				require.ErrorAs(t, err, &target)
			},
		},
		{
			name:     "Conway GOV stake pool voter",
			validate: conwayPoolVoter,
			rejected: func(t *testing.T, err error) {
				var target conway.UnknownVoterError
				require.ErrorAs(t, err, &target)
			},
		},
	}
	for _, rule := range rules {
		t.Run(rule.name+": reaped pool is not registered", func(t *testing.T) {
			t.Parallel()
			rule.rejected(t, rule.validate(stateWithRetirement(currentEpoch)))
		})
		t.Run(rule.name+": retiring pool is registered", func(t *testing.T) {
			t.Parallel()
			require.NoError(
				t,
				rule.validate(stateWithRetirement(currentEpoch+1)),
			)
		})
	}
}
