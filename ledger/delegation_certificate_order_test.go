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

	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func TestDelegationCertificatesUseEffectiveStakeState(t *testing.T) {
	type validationRule func(
		common.Transaction,
		uint64,
		common.LedgerState,
		common.ProtocolParameters,
	) error
	rules := map[string]validationRule{
		"Shelley": shelley.UtxoValidateDelegation,
		"Allegra": allegra.UtxoValidateDelegation,
		"Mary":    mary.UtxoValidateDelegation,
		"Alonzo":  alonzo.UtxoValidateDelegation,
		"Babbage": babbage.UtxoValidateDelegation,
	}
	credential := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224{0x11},
	}
	scriptCredential := common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: credential.Credential,
	}
	pool := &common.PoolRegistrationCertificate{
		Operator: common.PoolKeyHash{0x22},
	}
	registeredState := func() common.LedgerState {
		return mockledger.NewLedgerStateBuilder().
			WithStakeRegistrations([]common.StakeRegistrationCertificate{{
				StakeCredential: credential,
			}}).
			WithPools([]*common.PoolRegistrationCertificate{pool}).
			Build()
	}
	unregisteredState := func() common.LedgerState {
		return mockledger.NewLedgerStateBuilder().
			WithPools([]*common.PoolRegistrationCertificate{pool}).
			Build()
	}
	deregister := func(cred common.Credential) common.Certificate {
		return &common.StakeDeregistrationCertificate{StakeCredential: cred}
	}
	register := func(cred common.Credential) common.Certificate {
		return &common.StakeRegistrationCertificate{StakeCredential: cred}
	}
	delegate := func(cred common.Credential) common.Certificate {
		return &common.StakeDelegationCertificate{
			StakeCredential: &cred,
			PoolKeyHash:     pool.Operator,
		}
	}

	for era, rule := range rules {
		t.Run(era, func(t *testing.T) {
			t.Run("deregistration overrides starting state", func(t *testing.T) {
				tx := mockledger.NewTransactionBuilder().WithCertificates(
					deregister(credential),
					delegate(credential),
				)
				var target shelley.DelegateUnregisteredStakeCredentialError
				require.ErrorAs(t, rule(tx, 0, registeredState(), nil), &target)
			})

			t.Run("re-registration restores delegation", func(t *testing.T) {
				tx := mockledger.NewTransactionBuilder().WithCertificates(
					deregister(credential),
					register(credential),
					delegate(credential),
				)
				require.NoError(t, rule(tx, 0, registeredState(), nil))
			})

			t.Run("later deregistration blocks delegation", func(t *testing.T) {
				tx := mockledger.NewTransactionBuilder().WithCertificates(
					register(credential),
					deregister(credential),
					delegate(credential),
				)
				var target shelley.DelegateUnregisteredStakeCredentialError
				require.ErrorAs(t, rule(tx, 0, unregisteredState(), nil), &target)
			})

			t.Run("registration enables delegation", func(t *testing.T) {
				tx := mockledger.NewTransactionBuilder().WithCertificates(
					register(credential),
					delegate(credential),
				)
				require.NoError(t, rule(tx, 0, unregisteredState(), nil))
			})

			t.Run("credential type separates same-hash state", func(t *testing.T) {
				tx := mockledger.NewTransactionBuilder().WithCertificates(
					deregister(credential),
					register(scriptCredential),
					delegate(credential),
				)
				var target shelley.DelegateUnregisteredStakeCredentialError
				require.ErrorAs(t, rule(tx, 0, registeredState(), nil), &target)
			})
		})
	}
}
