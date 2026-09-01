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

package babbage_test

import (
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// UtxoValidateDelegation has no internal phase-2 guard, so it is the rule whose
// behavior the composed partition actually changes in Alonzo and Babbage. The
// era-level regression test for this otherwise only covers Conway.
func TestAlonzoBabbagePhase2InvalidSkipsDelegation(t *testing.T) {
	credential := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224{0x01},
	}
	certificates := []common.CertificateWrapper{{
		Type: uint(common.CertificateTypeStakeDelegation),
		Certificate: &common.StakeDelegationCertificate{
			CertType:        uint(common.CertificateTypeStakeDelegation),
			StakeCredential: &credential,
			PoolKeyHash: common.PoolKeyHash(
				common.Blake2b224{0x02},
			),
		},
	}}
	ls := mockledger.NewLedgerStateBuilder().Build()

	isDelegationError := func(err error) bool {
		var unregisteredPool shelley.DelegateToUnregisteredPoolError
		var unregisteredCred shelley.DelegateUnregisteredStakeCredentialError
		return errors.As(err, &unregisteredPool) ||
			errors.As(err, &unregisteredCred)
	}

	// composedDelegationRule finds the single composed rule that reports the
	// delegation failure, so the assertion does not pin an absolute index.
	composedDelegationRule := func(
		t *testing.T,
		rules []common.UtxoValidationRuleFunc,
		tx common.Transaction,
		pp common.ProtocolParameters,
	) common.UtxoValidationRuleFunc {
		t.Helper()
		var matches []common.UtxoValidationRuleFunc
		for _, rule := range rules {
			if isDelegationError(rule(tx, 0, ls, pp)) {
				matches = append(matches, rule)
			}
		}
		require.Len(t, matches, 1, "expected one composed rule to match")
		return matches[0]
	}

	t.Run("alonzo", func(t *testing.T) {
		pp := &alonzo.AlonzoProtocolParameters{}
		body := alonzo.AlonzoTransactionBody{TxCertificates: certificates}
		validTx := &alonzo.AlonzoTransaction{Body: body, TxIsValid: true}
		invalidTx := &alonzo.AlonzoTransaction{Body: body, TxIsValid: false}

		rule := composedDelegationRule(
			t,
			alonzo.UtxoValidationRules,
			validTx,
			pp,
		)
		require.True(t, isDelegationError(rule(validTx, 0, ls, pp)))
		require.NoError(t, rule(invalidTx, 0, ls, pp))
	})

	t.Run("babbage", func(t *testing.T) {
		pp := &babbage.BabbageProtocolParameters{}
		body := babbage.BabbageTransactionBody{TxCertificates: certificates}
		validTx := &babbage.BabbageTransaction{Body: body, TxIsValid: true}
		invalidTx := &babbage.BabbageTransaction{Body: body, TxIsValid: false}

		rule := composedDelegationRule(
			t,
			babbage.UtxoValidationRules,
			validTx,
			pp,
		)
		require.True(t, isDelegationError(rule(validTx, 0, ls, pp)))
		require.NoError(t, rule(invalidTx, 0, ls, pp))
	})
}
