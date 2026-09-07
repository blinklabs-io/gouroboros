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

// drepIdentityHashKeyedState returns a ledger state that answers DRep queries
// only through the hash-keyed common.GovState.DRepRegistration method and does
// not implement common.DRepCredentialState. Its answer is the same for a
// key-hash and a script-hash DRep sharing drepIdentityHash, because a bare
// hash cannot distinguish them.
func drepIdentityHashKeyedState(deposit uint64) common.LedgerState {
	return mockledger.NewLedgerStateBuilder().
		WithStakeCredentialRegistered(drepIdentityHash, true).
		WithDRepRegistration(func(
			hash common.Blake2b224,
		) (*common.DRepRegistration, error) {
			if hash != drepIdentityHash {
				return nil, nil
			}
			return &common.DRepRegistration{
				Credential: hash,
				Deposit:    deposit,
			}, nil
		}).
		Build()
}

// drepIdentityEmptyState returns a hash-keyed ledger state holding no DRep
// registration at all, so only in-transaction certificates can register one.
func drepIdentityEmptyState() common.LedgerState {
	return mockledger.NewLedgerStateBuilder().
		WithStakeCredentialRegistered(drepIdentityHash, true).
		WithDRepRegistration(func(
			common.Blake2b224,
		) (*common.DRepRegistration, error) {
			return nil, nil
		}).
		Build()
}

// drepCredentialLedgerState adds the optional common.DRepCredentialState
// capability to a ledger state that otherwise only answers hash-keyed DRep
// queries.
type drepCredentialLedgerState struct {
	common.LedgerState
	registration func(
		common.Credential,
	) (*common.DRepCredentialRegistration, error)
}

func (s drepCredentialLedgerState) DRepCredentialRegistration(
	credential common.Credential,
) (*common.DRepCredentialRegistration, error) {
	return s.registration(credential)
}

// scriptOnlyDRepState returns a ledger state implementing
// common.DRepCredentialState in which the script-hash DRep with
// drepIdentityHash is registered and the key-hash DRep sharing that hash is
// not, with the given deposit recorded against the registration.
//
// The state it wraps reports a registration for the bare hash with a deposit
// of dRepDepositFallbackSentinel, so any answer that distinguishes the two
// credential types, or that reports the recorded deposit, came from the
// capability rather than from the hash-keyed fallback.
func scriptOnlyDRepState(deposit *uint64) common.LedgerState {
	return drepCredentialLedgerState{
		LedgerState: drepIdentityHashKeyedState(
			dRepDepositFallbackSentinel,
		),
		registration: func(
			credential common.Credential,
		) (*common.DRepCredentialRegistration, error) {
			if credential.CredType != common.CredentialTypeScriptHash ||
				credential.Credential != drepIdentityHash {
				return nil, nil
			}
			return &common.DRepCredentialRegistration{
				Credential: credential,
				Deposit:    deposit,
			}, nil
		},
	}
}

// dRepDepositFallbackSentinel is the deposit the hash-keyed state underneath
// scriptOnlyDRepState reports. It is never a value a capability-path
// assertion expects, so a refund checked against it identifies a fallback the
// capability path should not have taken.
const dRepDepositFallbackSentinel = uint64(123_456_789)

// noDRepCredentialState returns a ledger state implementing
// common.DRepCredentialState that holds no DRep registration, so only
// in-transaction certificates can register one.
func noDRepCredentialState() common.LedgerState {
	return drepCredentialLedgerState{
		LedgerState: drepIdentityEmptyState(),
		registration: func(
			common.Credential,
		) (*common.DRepCredentialRegistration, error) {
			return nil, nil
		},
	}
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

func drepVoteDelegationTx(drepType int) *conway.ConwayTransaction {
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

// drepRegisterThenDelegateTx registers a script-hash DRep and then delegates
// to a DRep of the given type, both sharing drepIdentityHash.
func drepRegisterThenDelegateTx(drepType int) *conway.ConwayTransaction {
	return &conway.ConwayTransaction{
		Body: conway.ConwayTransactionBody{
			TxCertificates: []common.CertificateWrapper{
				{Certificate: &common.RegistrationDrepCertificate{
					CertType: uint(
						common.CertificateTypeRegistrationDrep,
					),
					DrepCredential: drepIdentityCredential(
						common.CredentialTypeScriptHash,
					),
				}},
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

// TestUnknownVotersDistinguishesDRepCredentialType pins that the voter type
// selects which of a same-hash key/script DRep pair a vote is resolved
// against when the ledger state implements common.DRepCredentialState.
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

// TestUnknownVotersHashKeyedFallback pins the behaviour a ledger state
// without common.DRepCredentialState keeps: the voter is resolved through the
// bare hash, so either voter type matches the single hash-keyed registration.
func TestUnknownVotersHashKeyedFallback(t *testing.T) {
	deposit := uint64(500_000_000)
	ls := drepIdentityHashKeyedState(deposit)
	pp := drepIdentityPparams(deposit)
	actionId := common.GovActionId{TransactionId: common.Blake2b256{0x01}}

	for name, voterType := range map[string]uint8{
		"key voter":    common.VoterTypeDRepKeyHash,
		"script voter": common.VoterTypeDRepScriptHash,
	} {
		t.Run(name, func(t *testing.T) {
			tx := mkVoteTx(
				common.Voter{Type: voterType, Hash: drepIdentityHash},
				actionId,
				common.GovVoteYes,
			)
			require.NoError(
				t,
				conway.UtxoValidateUnknownVoters(tx, 0, ls, pp),
			)
		})
	}

	t.Run("unknown hash is still rejected", func(t *testing.T) {
		tx := mkVoteTx(
			common.Voter{
				Type: common.VoterTypeDRepKeyHash,
				Hash: common.Blake2b224{0x99},
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

	t.Run("script DRep target is registered", func(t *testing.T) {
		require.NoError(t, conway.UtxoValidateDelegation(
			drepVoteDelegationTx(common.DrepTypeScriptHash), 0, ls, pp,
		))
	})

	t.Run("key DRep target is not", func(t *testing.T) {
		err := conway.UtxoValidateDelegation(
			drepVoteDelegationTx(common.DrepTypeAddrKeyHash), 0, ls, pp,
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

// TestVoteDelegationDistinguishesInTxDRepCredentialType pins that an
// in-transaction DRep registration is recorded under its full credential when
// the ledger state implements common.DRepCredentialState, so a delegation to
// the same-hash DRep of the other credential type does not resolve against it.
func TestVoteDelegationDistinguishesInTxDRepCredentialType(t *testing.T) {
	ls := noDRepCredentialState()
	pp := drepIdentityPparams(500_000_000)

	t.Run("script DRep registered in this tx", func(t *testing.T) {
		require.NoError(t, conway.UtxoValidateDelegation(
			drepRegisterThenDelegateTx(common.DrepTypeScriptHash), 0, ls, pp,
		))
	})

	t.Run("key DRep sharing its hash is not", func(t *testing.T) {
		err := conway.UtxoValidateDelegation(
			drepRegisterThenDelegateTx(common.DrepTypeAddrKeyHash), 0, ls, pp,
		)
		var target conway.DelegateVoteToUnregisteredDRepError
		require.ErrorAs(t, err, &target)
	})
}

// TestVoteDelegationHashKeyedFallback pins the behaviour a ledger state
// without common.DRepCredentialState keeps: both the state lookup and the
// in-transaction registration map are keyed by the bare hash, so a delegation
// of either DRep type resolves against a same-hash registration.
func TestVoteDelegationHashKeyedFallback(t *testing.T) {
	deposit := uint64(500_000_000)
	pp := drepIdentityPparams(deposit)

	t.Run("state registration matches either type", func(t *testing.T) {
		ls := drepIdentityHashKeyedState(deposit)
		for _, drepType := range []int{
			common.DrepTypeAddrKeyHash,
			common.DrepTypeScriptHash,
		} {
			require.NoError(t, conway.UtxoValidateDelegation(
				drepVoteDelegationTx(drepType), 0, ls, pp,
			))
		}
	})

	t.Run("in-tx registration matches either type", func(t *testing.T) {
		ls := drepIdentityEmptyState()
		for _, drepType := range []int{
			common.DrepTypeAddrKeyHash,
			common.DrepTypeScriptHash,
		} {
			require.NoError(t, conway.UtxoValidateDelegation(
				drepRegisterThenDelegateTx(drepType), 0, ls, pp,
			))
		}
	})

	t.Run("unregistered hash is still rejected", func(t *testing.T) {
		ls := drepIdentityEmptyState()
		err := conway.UtxoValidateDelegation(
			drepVoteDelegationTx(common.DrepTypeAddrKeyHash), 0, ls, pp,
		)
		var target conway.DelegateVoteToUnregisteredDRepError
		require.ErrorAs(t, err, &target)
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

// TestCertificateDepositsHashKeyedFallback pins the behaviour a ledger state
// without common.DRepCredentialState keeps: the registration is resolved
// through the bare hash and the refund is compared against the non-optional
// common.DRepRegistration.Deposit.
func TestCertificateDepositsHashKeyedFallback(t *testing.T) {
	deposit := uint64(500_000_000)
	ls := drepIdentityHashKeyedState(deposit)
	pp := drepIdentityPparams(deposit)

	for name, credType := range map[string]uint{
		"key credential":    common.CredentialTypeAddrKeyHash,
		"script credential": common.CredentialTypeScriptHash,
	} {
		t.Run(name+" refunds the recorded deposit", func(t *testing.T) {
			tx := drepDeregistrationTx(
				drepIdentityCredential(credType),
				int64(deposit),
			)
			require.NoError(
				t,
				conway.UtxoValidateCertificateDeposits(tx, 0, ls, pp),
			)
		})
	}

	t.Run("a wrong refund reports the recorded deposit", func(t *testing.T) {
		tx := drepDeregistrationTx(
			drepIdentityCredential(common.CredentialTypeAddrKeyHash),
			int64(deposit)+1,
		)
		err := conway.UtxoValidateCertificateDeposits(tx, 0, ls, pp)
		var target conway.CertificateRefundIncorrectError
		require.ErrorAs(t, err, &target)
		require.Equal(t, int64(deposit), int64(target.Expected))
	})
}

// TestCertificateDepositsRejectsDRepWithoutRecordedDeposit pins that a
// registration reported through common.DRepCredentialState carrying no
// recorded deposit fails closed. The reference ledger's DRepState always
// carries a deposit, so there is no refund to compare against; reading the
// absence as zero rejects the refund the network accepts.
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
