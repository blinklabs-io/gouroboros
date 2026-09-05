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

	t.Run("negative delta is accepted from Alonzo", func(t *testing.T) {
		require.NoError(t, shelley.UtxoValidateDelegation(
			poolCertTx(mirCert(0, map[*common.Credential]*big.Int{
				credential: big.NewInt(-1),
			})),
			0,
			ls,
			alonzoPparams(0),
		))
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

	t.Run("pot-to-pot transfer is untouched before Alonzo", func(t *testing.T) {
		// The opposite-pot amount is coin, which is uint, so this branch
		// carries no sign for the predicate to reject. Whether the
		// transfer itself is embargoed before Alonzo
		// (MIRTransferNotCurrentlyAllowed) is a separate predicate that
		// this rule does not implement.
		require.NoError(t, shelley.UtxoValidateDelegation(
			poolCertTx(mirOppositePotCert(0, 1_000_000)),
			0,
			ls,
			maryPparams(0),
		))
	})
}
