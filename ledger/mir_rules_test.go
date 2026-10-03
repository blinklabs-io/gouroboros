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

// Tests for the move instantaneous rewards predicates of the Shelley DELEG
// rule (delegTransition in
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Deleg.hs). They live in
// package ledger_test rather than shelley_test because the predicate is gated
// on protocol version and has to be exercised with the real protocol
// parameters of the eras that carry those versions.

package ledger_test

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mirStakeCredential(b byte) *common.Credential {
	credential := common.Credential{
		CredType: common.CredentialTypeAddrKeyHash,
	}
	for i := range credential.Credential {
		credential.Credential[i] = b
	}
	return &credential
}

func mirCert(
	source uint,
	rewards map[*common.Credential]*big.Int,
) *common.MoveInstantaneousRewardsCertificate {
	return &common.MoveInstantaneousRewardsCertificate{
		CertType: uint(common.CertificateTypeMoveInstantaneousRewards),
		Reward: common.MoveInstantaneousRewardsCertificateReward{
			Source:  source,
			Rewards: rewards,
		},
	}
}

func mirOppositePotCert(
	source uint,
	amount uint64,
) *common.MoveInstantaneousRewardsCertificate {
	return &common.MoveInstantaneousRewardsCertificate{
		CertType: uint(common.CertificateTypeMoveInstantaneousRewards),
		Reward: common.MoveInstantaneousRewardsCertificateReward{
			Source:   source,
			OtherPot: amount,
		},
	}
}

// TestUtxoValidateDelegationMirNegativeDelta covers
// MIRNegativesNotCurrentlyAllowed. delta_coin is int on the wire, so the
// decoder accepts a negative delta and the DELEG rule decides whether it is
// permitted: hardforkAlonzoAllowMIRTransfer
// (eras/shelley/impl/src/Cardano/Ledger/Shelley/Era.hs) gates it on major
// version > 4.
func TestUtxoValidateDelegationMirNegativeDelta(t *testing.T) {
	ls := mockledger.NewLedgerStateBuilder().Build()
	credential := mirStakeCredential(0xab)

	t.Run("negative delta is rejected before Alonzo", func(t *testing.T) {
		err := shelley.UtxoValidateDelegation(
			poolCertTx(mirCert(0, map[*common.Credential]*big.Int{
				credential: big.NewInt(-1),
			})),
			0,
			ls,
			maryPparams(0),
		)
		var target shelley.MIRNegativesNotCurrentlyAllowedError
		require.ErrorAs(t, err, &target)
		assert.Equal(t, credential.Credential, target.Credential.Credential)
		assert.Zero(t, big.NewInt(-1).Cmp(target.Delta))
	})

	t.Run("negative delta is not an embargo from Alonzo", func(t *testing.T) {
		// The sign is no longer rejected on its own; the delta is bounded
		// by the pending rewards (TestUtxoValidateDelegationMirProducesNegativeUpdate).
		err := shelley.UtxoValidateDelegation(
			poolCertTx(mirCert(0, map[*common.Credential]*big.Int{
				credential: big.NewInt(-1),
			})),
			0,
			ls,
			alonzoPparams(0),
		)
		var embargo shelley.MIRNegativesNotCurrentlyAllowedError
		require.NotErrorAs(t, err, &embargo)
	})

	t.Run("non-negative deltas are accepted before Alonzo", func(t *testing.T) {
		require.NoError(t, shelley.UtxoValidateDelegation(
			poolCertTx(mirCert(0, map[*common.Credential]*big.Int{
				credential:               big.NewInt(77),
				mirStakeCredential(0xcd): big.NewInt(0),
			})),
			0,
			ls,
			maryPparams(0),
		))
	})

	t.Run("pot-to-pot transfer is rejected before Alonzo", func(t *testing.T) {
		err := shelley.UtxoValidateDelegation(
			poolCertTx(mirOppositePotCert(1, 1_000_000)),
			0,
			ls,
			maryPparams(0),
		)
		var target shelley.MIRTransferNotCurrentlyAllowedError
		require.ErrorAs(t, err, &target)
		assert.Equal(t, uint(1), target.Source)
		assert.Equal(t, uint64(1_000_000), target.Amount)
	})

	t.Run("pot-to-pot transfer is accepted from Alonzo", func(t *testing.T) {
		require.NoError(t, shelley.UtxoValidateDelegation(
			poolCertTx(mirOppositePotCert(1, 1_000_000)),
			0,
			ls,
			alonzoPparams(0),
		))
	})
}

// pendingRewardsState is a ledger state that reports fixed pending
// instantaneous rewards, keyed by source pot and credential.
type pendingRewardsState struct {
	common.LedgerState
	pending map[[2]uint]*big.Int
}

func (s pendingRewardsState) PendingInstantaneousRewards(
	source uint,
	cred common.Credential,
) (*big.Int, error) {
	return s.pending[[2]uint{source, uint(cred.Credential[0])}], nil
}

// TestUtxoValidateDelegationMirProducesNegativeUpdate covers
// MIRProducesNegativeUpdate: from Alonzo a negative delta is legal, but the
// credential's pending rewards in that pot, including those of earlier
// certificates in the transaction and block, must stay non-negative.
func TestUtxoValidateDelegationMirProducesNegativeUpdate(t *testing.T) {
	t.Parallel()

	credential := mirStakeCredential(0xab)
	reserves := func(delta int64) common.Certificate {
		return mirCert(0, map[*common.Credential]*big.Int{
			credential: big.NewInt(delta),
		})
	}
	withPending := pendingRewardsState{
		LedgerState: mockledger.NewLedgerStateBuilder().Build(),
		pending:     map[[2]uint]*big.Int{{0, 0xab}: big.NewInt(5)},
	}
	noPending := pendingRewardsState{
		LedgerState: mockledger.NewLedgerStateBuilder().Build(),
	}
	validate := func(
		ls common.LedgerState,
		certs ...common.Certificate,
	) error {
		return shelley.UtxoValidateDelegation(
			poolCertTx(certs...), 0, ls, alonzoPparams(0),
		)
	}
	var target shelley.MIRProducesNegativeUpdateError

	t.Run("delta below the pending rewards is rejected", func(t *testing.T) {
		err := validate(withPending, reserves(-6))
		require.ErrorAs(t, err, &target)
		assert.Equal(t, uint(0), target.Source)
		assert.Zero(t, big.NewInt(5).Cmp(target.Pending))
	})

	t.Run("delta equal to the pending rewards is accepted", func(t *testing.T) {
		require.NoError(t, validate(withPending, reserves(-5)))
	})

	t.Run("negative delta with no pending rewards is rejected", func(t *testing.T) {
		require.ErrorAs(t, validate(noPending, reserves(-1)), &target)
	})

	t.Run("negative delta without pending rewards state is unavailable", func(t *testing.T) {
		var unavailable shelley.PendingInstantaneousRewardsUnavailableError
		require.ErrorAs(
			t,
			validate(mockledger.NewLedgerStateBuilder().Build(), reserves(-1)),
			&unavailable,
		)
		assert.Equal(t, uint(0), unavailable.Source)
		require.NoError(t, validate(
			mockledger.NewLedgerStateBuilder().Build(),
			reserves(4),
			reserves(-4),
		))
	})

	t.Run("pending rewards in the other pot do not count", func(t *testing.T) {
		treasury := mirCert(1, map[*common.Credential]*big.Int{
			credential: big.NewInt(-1),
		})
		require.ErrorAs(t, validate(withPending, treasury), &target)
	})

	t.Run("earlier certificates in the transaction accumulate", func(t *testing.T) {
		require.NoError(t, validate(withPending, reserves(-3), reserves(-2)))
		require.ErrorAs(
			t,
			validate(withPending, reserves(-3), reserves(-3)),
			&target,
		)
		require.NoError(t, validate(
			mockledger.NewLedgerStateBuilder().Build(),
			reserves(4),
			reserves(-4),
		))
	})

	t.Run("earlier transactions in the block accumulate", func(t *testing.T) {
		block := common.NewBlockLedgerState(noPending)
		require.NoError(
			t,
			block.ApplyTransaction(poolCertTx(reserves(5)), alonzoPparams(0)),
		)
		require.NoError(t, validate(block, reserves(-5)))
		require.ErrorAs(t, validate(block, reserves(-6)), &target)
	})
}
