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

package conway_test

import (
	"math"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

func committeeTermAction(
	credential *common.Credential,
	expiry uint64,
) *common.UpdateCommitteeGovAction {
	return &common.UpdateCommitteeGovAction{
		CredEpochs: map[*common.Credential]uint64{credential: expiry},
	}
}

func committeeTermCredential(seed string) common.Credential {
	return common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte(seed)),
	}
}

// TestValidateCommitteeTermBound covers the ratification bound itself. The
// reference is validCommitteeTerm in Cardano.Ledger.Conway.Rules.Ratify:
// all (<= addEpochInterval currentEpoch committeeMaxTermLength), so the
// boundary epoch is within the term and one past it is not.
func TestValidateCommitteeTermBound(t *testing.T) {
	t.Parallel()
	credential := committeeTermCredential("committee-cold-key")
	tests := []struct {
		name         string
		currentEpoch uint64
		expiry       uint64
		maxTerm      uint64
		wantError    bool
	}{
		{name: "below boundary", currentEpoch: 100, expiry: 109, maxTerm: 10},
		{name: "at boundary", currentEpoch: 100, expiry: 110, maxTerm: 10},
		{
			name:         "over boundary",
			currentEpoch: 100,
			expiry:       111,
			maxTerm:      10,
			wantError:    true,
		},
		{name: "expiry already past", currentEpoch: 100, expiry: 50, maxTerm: 10},
		{name: "zero limit at current epoch", currentEpoch: 100, expiry: 100},
		{
			name:         "zero limit with positive remaining term",
			currentEpoch: 100,
			expiry:       101,
			wantError:    true,
		},
		// addEpochInterval is Word64 addition in the reference and wraps, so
		// the bound here is 2 and a MaxUint64 expiry is outside it. Computing
		// the bound with saturating arithmetic instead would accept this.
		{
			name:         "bound wraps past uint64",
			currentEpoch: math.MaxUint64 - 2,
			expiry:       math.MaxUint64,
			maxTerm:      5,
			wantError:    true,
		},
		{
			name:         "wrapped bound still admits a low expiry",
			currentEpoch: math.MaxUint64 - 2,
			expiry:       2,
			maxTerm:      5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := &conway.ConwayProtocolParameters{
				CommitteeTermLimit: tt.maxTerm,
			}
			err := conway.ValidateCommitteeTerm(
				committeeTermAction(&credential, tt.expiry),
				pp,
				tt.currentEpoch,
			)
			if !tt.wantError {
				require.NoError(t, err)
				return
			}
			var termErr conway.CommitteeTermTooLongError
			require.ErrorAs(t, err, &termErr)
			assert.Equal(t, credential.Credential, termErr.Credential)
			assert.Equal(t, tt.currentEpoch, termErr.CurrentEpoch)
			assert.Equal(t, tt.expiry, termErr.ExpiryEpoch)
			assert.Equal(t, tt.maxTerm, termErr.MaxTermLength)
		})
	}
}

// TestValidateCommitteeTermLoosensWithEpoch pins that the bound is relative to
// the current epoch. The same action is outside the term now and inside it
// later, which is why this predicate cannot be a transaction-validity check:
// a permanent rejection would contradict a reference rule that re-evaluates
// every epoch.
func TestValidateCommitteeTermLoosensWithEpoch(t *testing.T) {
	t.Parallel()
	credential := committeeTermCredential("committee-cold-key")
	action := committeeTermAction(&credential, 120)
	pp := &conway.ConwayProtocolParameters{CommitteeTermLimit: 10}

	require.Error(t, conway.ValidateCommitteeTerm(action, pp, 100))
	require.NoError(t, conway.ValidateCommitteeTerm(action, pp, 110))
}

// TestValidateCommitteeTermIsNotATransactionPredicate pins that the bound is
// not enforced on the UTxO path. cardano-ledger checks committee terms in
// ratifyTransition, and its GOV analogue actionWellFormed does not mention
// them, so a transaction proposing an over-limit expiry is valid on mainnet.
// Rejecting it here would make gouroboros reject a block the Haskell node
// accepts.
func TestValidateCommitteeTermIsNotATransactionPredicate(t *testing.T) {
	t.Parallel()
	credential := committeeTermCredential("committee-cold-key")
	// Far beyond any plausible maximum term measured from epoch 100.
	action := committeeTermAction(&credential, 1_000_000)
	pp := &conway.ConwayProtocolParameters{CommitteeTermLimit: 10}

	// The ratification predicate rejects it.
	var termErr conway.CommitteeTermTooLongError
	require.ErrorAs(
		t,
		conway.ValidateCommitteeTerm(action, pp, 100),
		&termErr,
	)

	// Transaction validation accepts it.
	tx := mkProposalTx(0, common.Address{}, action)
	require.NoError(
		t,
		conway.UtxoValidateGovActionWellFormedness(tx, 0, nil, pp),
	)
}

func TestValidateCommitteeTermWithoutTermLimit(t *testing.T) {
	t.Parallel()
	credential := committeeTermCredential("committee-cold-key")

	t.Run("unsupported parameters", func(t *testing.T) {
		t.Parallel()
		var unavailable conway.CommitteeTermLimitUnavailableError
		require.ErrorAs(
			t,
			conway.ValidateCommitteeTerm(
				committeeTermAction(&credential, 101),
				mockProtocolParameters{},
				100,
			),
			&unavailable,
		)
	})

	// A removal-only update proposes no expiry epochs, so there is nothing to
	// bound and the missing limit must not make it unratifiable.
	t.Run("removal only without parameters", func(t *testing.T) {
		t.Parallel()
		action := &common.UpdateCommitteeGovAction{
			Credentials: []common.Credential{credential},
		}
		require.NoError(
			t,
			conway.ValidateCommitteeTerm(action, mockProtocolParameters{}, 100),
		)
	})

	t.Run("nil action", func(t *testing.T) {
		t.Parallel()
		require.NoError(
			t,
			conway.ValidateCommitteeTerm(nil, mockProtocolParameters{}, 100),
		)
	})
}

// TestValidateCommitteeTermReportsDeterministically pins that the reported
// credential does not vary with map iteration order.
func TestValidateCommitteeTermReportsDeterministically(t *testing.T) {
	t.Parallel()
	credA := committeeTermCredential("committee-cold-key-a")
	credB := committeeTermCredential("committee-cold-key-b")
	first, second := credA, credB
	if string(credB.Credential.Bytes()) < string(credA.Credential.Bytes()) {
		first, second = credB, credA
	}
	action := &common.UpdateCommitteeGovAction{
		CredEpochs: map[*common.Credential]uint64{
			&credA: 500,
			&credB: 600,
		},
	}
	pp := &conway.ConwayProtocolParameters{CommitteeTermLimit: 10}

	for range 32 {
		var termErr conway.CommitteeTermTooLongError
		require.ErrorAs(
			t,
			conway.ValidateCommitteeTerm(action, pp, 100),
			&termErr,
		)
		assert.Equal(t, first.Credential, termErr.Credential)
		assert.NotEqual(t, second.Credential, termErr.Credential)
	}
}

// TestCommitteeMaxTermLengthNilReceiver pins that a typed-nil parameter
// pointer reaching the capability check reports the limit as unavailable
// rather than panicking.
func TestCommitteeMaxTermLengthNilReceiver(t *testing.T) {
	t.Parallel()
	var conwayPP *conway.ConwayProtocolParameters
	limit, ok := conwayPP.CommitteeMaxTermLength()
	assert.False(t, ok)
	assert.Zero(t, limit)
}

type mockProtocolParameters struct{}

func (mockProtocolParameters) Utxorpc() (*utxorpc.PParams, error) {
	return nil, nil
}
