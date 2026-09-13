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
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

var drepDepositHash = common.Blake2b224{0x71}

// drepStateWithDeposit returns a ledger state holding a registered DRep with
// the given recorded deposit. A nil deposit is a registration the state
// reports without a recorded amount.
func drepStateWithDeposit(deposit *uint64) common.LedgerState {
	return mockledger.NewLedgerStateBuilder().
		WithStakeCredentialRegistered(drepDepositHash, true).
		WithDRepRegistration(func(
			credential common.Blake2b224,
		) (*common.DRepRegistration, error) {
			if credential != drepDepositHash {
				return nil, nil
			}
			return &common.DRepRegistration{
				Credential: credential,
				Deposit:    deposit,
			}, nil
		}).
		Build()
}

func drepDepositPparams(drepDeposit uint64) *conway.ConwayProtocolParameters {
	return &conway.ConwayProtocolParameters{DRepDeposit: drepDeposit}
}

func drepDeregistrationTx(
	credential common.Credential,
	refund int64,
) *conway.ConwayTransaction {
	return &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxCertificates: []common.CertificateWrapper{
				{Certificate: &common.DeregistrationDrepCertificate{
					CertType: uint(
						common.CertificateTypeDeregistrationDrep,
					),
					DrepCredential: credential,
					Amount:         refund,
				}},
			},
		},
		TxIsValid: true,
	}
}

func drepDepositCredential() common.Credential {
	return common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: drepDepositHash,
	}
}

// TestCertificateDepositsRejectsDRepWithoutRecordedDeposit pins that a
// registration reported without a recorded deposit fails closed. Reading the
// absence as zero rejects every deregistration that supplies its real refund.
func TestCertificateDepositsRejectsDRepWithoutRecordedDeposit(t *testing.T) {
	const paid = uint64(500_000_000)
	ls := drepStateWithDeposit(nil)
	pp := drepDepositPparams(paid)
	tx := drepDeregistrationTx(drepDepositCredential(), int64(paid))

	err := conway.UtxoValidateCertificateDeposits(tx, 0, ls, pp)
	var inconsistent conway.DRepDepositStateInconsistentError
	require.ErrorAs(t, err, &inconsistent)
	require.Equal(
		t,
		uint(common.CredentialTypeScriptHash),
		inconsistent.Credential.CredType,
	)
	var refund conway.CertificateRefundIncorrectError
	require.NotErrorAs(
		t,
		err,
		&refund,
		"an absent recorded deposit must not be reported as an expected refund",
	)
}

// TestCertificateDepositsUsesRecordedDRepDeposit pins that the refund is
// checked against the deposit recorded at registration, not the current
// dRepDeposit parameter, which is governable and can have moved since.
func TestCertificateDepositsUsesRecordedDRepDeposit(t *testing.T) {
	recorded := uint64(400_000_000)
	current := uint64(500_000_000)
	ls := drepStateWithDeposit(&recorded)
	pp := drepDepositPparams(current)
	credential := drepDepositCredential()

	t.Run("recorded refund is accepted", func(t *testing.T) {
		tx := drepDeregistrationTx(credential, int64(recorded))
		require.NoError(
			t,
			conway.UtxoValidateCertificateDeposits(tx, 0, ls, pp),
		)
	})

	t.Run("current parameter is rejected", func(t *testing.T) {
		tx := drepDeregistrationTx(credential, int64(current))
		err := conway.UtxoValidateCertificateDeposits(tx, 0, ls, pp)
		var target conway.CertificateRefundIncorrectError
		require.ErrorAs(t, err, &target)
		require.Equal(t, int64(recorded), int64(target.Expected))
	})
}
