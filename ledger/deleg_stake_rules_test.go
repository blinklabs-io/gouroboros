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

// Tests for the stake registration predicates of the Shelley DELEG rule
// (delegationTransition in
// eras/shelley/impl/src/Cardano/Ledger/Shelley/Rules/Deleg.hs), driven through
// each era's registered delegation rule.

package ledger_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// delegEra is a Shelley-family era whose registered UtxoValidationRules carry
// the DELEG predicates.
type delegEra struct {
	name        string
	descriptors []common.UtxoValidationRuleDescriptor
	pparams     func() common.ProtocolParameters
}

func delegEras() []delegEra {
	shelleyPparams := func(major uint) func() common.ProtocolParameters {
		return func() common.ProtocolParameters {
			pparams := mockledger.NewMockShelleyProtocolParams()
			pparams.ProtocolMajor = major
			return &pparams
		}
	}
	return []delegEra{
		{
			"Shelley",
			shelley.UtxoValidationRuleDescriptors(),
			shelleyPparams(2),
		},
		{
			"Allegra",
			allegra.UtxoValidationRuleDescriptors(),
			shelleyPparams(3),
		},
		{
			"Mary",
			mary.UtxoValidationRuleDescriptors(),
			func() common.ProtocolParameters { return maryPparams(0) },
		},
		{
			"Alonzo",
			alonzo.UtxoValidationRuleDescriptors(),
			func() common.ProtocolParameters { return alonzoPparams(0) },
		},
		{
			"Babbage",
			babbage.UtxoValidationRuleDescriptors(),
			func() common.ProtocolParameters {
				pparams := mockledger.NewMockBabbageProtocolParams()
				pparams.ProtocolMajor = 7
				return &pparams
			},
		},
	}
}

// validateRegisteredRule runs the rule registered under id for the era.
func (e delegEra) validateRegisteredRule(
	t *testing.T,
	id common.UtxoValidationRuleId,
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
) error {
	t.Helper()
	for _, desc := range e.descriptors {
		if desc.Id == id {
			return desc.Validator(tx, slot, ls, e.pparams())
		}
	}
	t.Fatalf("%s registers no %q rule", e.name, id)
	return nil
}

func (e delegEra) validateDelegation(
	t *testing.T,
	tx common.Transaction,
	ls common.LedgerState,
) error {
	t.Helper()
	return e.validateRegisteredRule(
		t,
		common.UtxoValidationRuleDelegation,
		tx,
		0,
		ls,
	)
}

func stakeKeyCredential(b byte) common.Credential {
	return common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224{b},
	}
}

func rewardAddress(t *testing.T, cred common.Credential) common.Address {
	t.Helper()
	addr, err := common.NewAddressFromParts(
		common.AddressTypeNoneKey,
		common.AddressNetworkTestnet,
		nil,
		cred.Credential.Bytes(),
	)
	require.NoError(t, err)
	return addr
}

// TestDelegStakeRegistrationPredicates covers AccountAlreadyRegistered,
// StakeKeyNotRegisteredDELEG and StakeKeyNonZeroAccountBalanceDELEG.
func TestDelegStakeRegistrationPredicates(t *testing.T) {
	t.Parallel()

	cred := stakeKeyCredential(0x11)
	register := &common.StakeRegistrationCertificate{StakeCredential: cred}
	deregister := &common.StakeDeregistrationCertificate{
		StakeCredential: cred,
	}
	registered := func(balance uint64) common.LedgerState {
		return mockledger.NewLedgerStateBuilder().
			WithRewardAccountCredentialBalance(cred, balance).
			Build()
	}
	unregistered := func() common.LedgerState {
		return mockledger.NewLedgerStateBuilder().Build()
	}

	for _, era := range delegEras() {
		t.Run(era.name, func(t *testing.T) {
			t.Parallel()

			t.Run(
				"register an already registered credential",
				func(t *testing.T) {
					t.Parallel()
					err := era.validateDelegation(
						t, poolCertTx(register), registered(0),
					)
					var target shelley.StakeKeyAlreadyRegisteredError
					require.ErrorAs(t, err, &target)
					assert.Equal(t, cred, target.Credential)
				},
			)

			t.Run("register an unregistered credential", func(t *testing.T) {
				t.Parallel()
				require.NoError(t, era.validateDelegation(
					t, poolCertTx(register), unregistered(),
				))
			})

			t.Run("deregister an unregistered credential", func(t *testing.T) {
				t.Parallel()
				err := era.validateDelegation(
					t, poolCertTx(deregister), unregistered(),
				)
				var target shelley.StakeKeyNotRegisteredError
				require.ErrorAs(t, err, &target)
				assert.Equal(t, cred, target.Credential)
			})

			t.Run("deregister with a zero balance", func(t *testing.T) {
				t.Parallel()
				require.NoError(t, era.validateDelegation(
					t, poolCertTx(deregister), registered(0),
				))
			})

			t.Run("deregister with a non-zero balance", func(t *testing.T) {
				t.Parallel()
				err := era.validateDelegation(
					t, poolCertTx(deregister), registered(7),
				)
				var target shelley.StakeKeyNonZeroAccountBalanceError
				require.ErrorAs(t, err, &target)
				assert.Equal(t, cred, target.Credential)
				assert.Equal(t, uint64(7), target.Balance)
			})

			t.Run(
				"withdrawal drains the balance before deregistration",
				func(t *testing.T) {
					t.Parallel()
					addr := rewardAddress(t, cred)
					tx := poolCertTx(deregister)
					tx.Body.TxWithdrawals = map[*common.Address]uint64{&addr: 7}
					require.NoError(t, era.validateDelegation(
						t, tx, registered(7),
					))
				},
			)

			t.Run(
				"register then deregister in one transaction",
				func(t *testing.T) {
					t.Parallel()
					require.NoError(t, era.validateDelegation(
						t, poolCertTx(register, deregister), unregistered(),
					))
				},
			)

			t.Run(
				"deregister then register in one transaction",
				func(t *testing.T) {
					t.Parallel()
					require.NoError(t, era.validateDelegation(
						t, poolCertTx(deregister, register), registered(0),
					))
				},
			)

			t.Run(
				"deregister then register then deregister",
				func(t *testing.T) {
					t.Parallel()
					// The new account starts empty, so the old balance does not
					// carry over to the second deregistration.
					require.NoError(t, era.validateDelegation(
						t,
						poolCertTx(deregister, register, deregister),
						registered(0),
					))
				},
			)

			t.Run("register twice in one transaction", func(t *testing.T) {
				t.Parallel()
				err := era.validateDelegation(
					t, poolCertTx(register, register), unregistered(),
				)
				var target shelley.StakeKeyAlreadyRegisteredError
				require.ErrorAs(t, err, &target)
			})

			t.Run("deregister twice in one transaction", func(t *testing.T) {
				t.Parallel()
				err := era.validateDelegation(
					t, poolCertTx(deregister, deregister), registered(0),
				)
				var target shelley.StakeKeyNotRegisteredError
				require.ErrorAs(t, err, &target)
			})
		})
	}
}
