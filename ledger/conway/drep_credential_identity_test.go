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

// drepIdentityHash is shared by a key-hash and a script-hash DRep credential
// in these tests. The reference ledger keys DRep state by the full credential,
// so the two are distinct DReps that happen to share their hash.
var drepIdentityHash = common.Blake2b224{0x71}

func drepIdentityCredential(credType uint) common.Credential {
	return common.Credential{
		CredType:   credType,
		Credential: drepIdentityHash,
	}
}

// scriptOnlyDRepState returns a ledger state in which the script-hash DRep
// with drepIdentityHash is registered and the key-hash DRep sharing that hash
// is not, with the given deposit recorded against the registration.
func scriptOnlyDRepState(deposit *uint64) common.LedgerState {
	return mockledger.NewLedgerStateBuilder().
		WithStakeCredentialRegistered(drepIdentityHash, true).
		WithDRepRegistration(func(
			credential common.Credential,
		) (*common.DRepRegistration, error) {
			if credential.CredType != common.CredentialTypeScriptHash ||
				credential.Credential != drepIdentityHash {
				return nil, nil
			}
			return &common.DRepRegistration{
				Credential: credential,
				Deposit:    deposit,
			}, nil
		}).
		Build()
}

func drepIdentityPparams(drepDeposit uint64) *conway.ConwayProtocolParameters {
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

// TestUnknownVotersDistinguishesDRepCredentialType pins that the voter type
// selects which of a same-hash key/script DRep pair a vote is resolved
// against. Resolving the bare hash accepts either voter type against
// whichever registration the state happens to hold.
func TestUnknownVotersDistinguishesDRepCredentialType(t *testing.T) {
	deposit := uint64(500_000_000)
	ls := scriptOnlyDRepState(&deposit)
	pp := drepIdentityPparams(deposit)
	actionId := common.GovActionId{TransactionId: common.Blake2b256{0x01}}

	t.Run("script voter matches the script registration", func(t *testing.T) {
		tx := mkVoteTx(
			common.Voter{
				Type: common.VoterTypeDRepScriptHash,
				Hash: drepIdentityHash,
			},
			actionId,
			common.GovVoteYes,
		)
		require.NoError(t, conway.UtxoValidateUnknownVoters(tx, 0, ls, pp))
	})

	t.Run("key voter does not match it", func(t *testing.T) {
		tx := mkVoteTx(
			common.Voter{
				Type: common.VoterTypeDRepKeyHash,
				Hash: drepIdentityHash,
			},
			actionId,
			common.GovVoteYes,
		)
		err := conway.UtxoValidateUnknownVoters(tx, 0, ls, pp)
		var target conway.UnknownVoterError
		require.ErrorAs(t, err, &target)
	})
}

// TestVoteDelegationDistinguishesDRepCredentialType pins the same identity
// for the delegation rule: the DRep type in the certificate selects the
// credential type the registration is looked up under.
func TestVoteDelegationDistinguishesDRepCredentialType(t *testing.T) {
	deposit := uint64(500_000_000)
	ls := scriptOnlyDRepState(&deposit)
	pp := drepIdentityPparams(deposit)
	mkTx := func(drepType int) *conway.ConwayTransaction {
		return &conway.ConwayTransaction{
			Body: conway.ConwayTransactionBody{
				TxCertificates: []common.CertificateWrapper{
					{Certificate: &common.VoteDelegationCertificate{
						StakeCredential: drepIdentityCredential(
							common.CredentialTypeAddrKeyHash,
						),
						Drep: common.Drep{
							Type:       drepType,
							Credential: drepIdentityHash[:],
						},
					}},
				},
			},
			TxIsValid: true,
		}
	}

	t.Run("script DRep target is registered", func(t *testing.T) {
		require.NoError(t, conway.UtxoValidateDelegation(
			mkTx(common.DrepTypeScriptHash), 0, ls, pp,
		))
	})

	t.Run("key DRep target is not", func(t *testing.T) {
		err := conway.UtxoValidateDelegation(
			mkTx(common.DrepTypeAddrKeyHash), 0, ls, pp,
		)
		var target conway.DelegateVoteToUnregisteredDRepError
		require.ErrorAs(t, err, &target)
		require.Equal(
			t,
			uint(common.CredentialTypeAddrKeyHash),
			target.DRepCredential.CredType,
		)
	})
}

// TestCertificateDepositsDistinguishesDRepCredentialType pins that a
// deregistration certificate is checked against the registration of its own
// credential type, not against whichever DRep shares its hash.
func TestCertificateDepositsDistinguishesDRepCredentialType(t *testing.T) {
	deposit := uint64(500_000_000)
	ls := scriptOnlyDRepState(&deposit)
	pp := drepIdentityPparams(deposit)

	t.Run("script deregistration refunds the record", func(t *testing.T) {
		tx := drepDeregistrationTx(
			drepIdentityCredential(common.CredentialTypeScriptHash),
			int64(deposit),
		)
		require.NoError(
			t,
			conway.UtxoValidateCertificateDeposits(tx, 0, ls, pp),
		)
	})

	t.Run("key deregistration finds no registration", func(t *testing.T) {
		tx := drepDeregistrationTx(
			drepIdentityCredential(common.CredentialTypeAddrKeyHash),
			int64(deposit),
		)
		err := conway.UtxoValidateCertificateDeposits(tx, 0, ls, pp)
		var target conway.DRepNotRegisteredError
		require.ErrorAs(t, err, &target)
	})
}

// TestCertificateDepositsRejectsDRepWithoutRecordedDeposit pins that a
// registration carrying no recorded deposit fails closed. The reference
// ledger's DRepState always carries a deposit, so there is no refund to
// compare against; reading the absence as zero rejects the refund the network
// accepts.
func TestCertificateDepositsRejectsDRepWithoutRecordedDeposit(t *testing.T) {
	const paid = uint64(500_000_000)
	ls := scriptOnlyDRepState(nil)
	pp := drepIdentityPparams(paid)
	tx := drepDeregistrationTx(
		drepIdentityCredential(common.CredentialTypeScriptHash),
		int64(paid),
	)

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
	ls := scriptOnlyDRepState(&recorded)
	pp := drepIdentityPparams(current)
	credential := drepIdentityCredential(common.CredentialTypeScriptHash)

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
