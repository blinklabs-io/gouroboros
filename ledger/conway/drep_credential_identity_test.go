// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");

package conway_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

var drepIdentityHash = common.Blake2b224{0x71}

func drepIdentityCredential(credType uint) common.Credential {
	return common.Credential{CredType: credType, Credential: drepIdentityHash}
}

func scriptOnlyDRepState(deposit *uint64) common.LedgerState {
	return mockledger.NewLedgerStateBuilder().
		WithStakeCredentialRegistered(drepIdentityHash, true).
		WithDRepRegistration(func(credential common.Credential) (*common.DRepRegistration, error) {
			if credential.CredType != common.CredentialTypeScriptHash || credential.Credential != drepIdentityHash {
				return nil, nil
			}
			return &common.DRepRegistration{Credential: credential, Deposit: deposit}, nil
		}).
		Build()
}

func TestUnknownVotersDistinguishesDRepCredentialType(t *testing.T) {
	deposit := uint64(500_000_000)
	ls := scriptOnlyDRepState(&deposit)
	pp := &conway.ConwayProtocolParameters{DRepDeposit: deposit}
	actionID := common.GovActionId{TransactionId: common.Blake2b256{0x01}}

	t.Run("script voter matches", func(t *testing.T) {
		tx := mkVoteTx(common.Voter{Type: common.VoterTypeDRepScriptHash, Hash: drepIdentityHash}, actionID, common.GovVoteYes)
		require.NoError(t, conway.UtxoValidateUnknownVoters(tx, 0, ls, pp))
	})
	t.Run("key voter does not match", func(t *testing.T) {
		tx := mkVoteTx(common.Voter{Type: common.VoterTypeDRepKeyHash, Hash: drepIdentityHash}, actionID, common.GovVoteYes)
		var target conway.UnknownVoterError
		require.ErrorAs(t, conway.UtxoValidateUnknownVoters(tx, 0, ls, pp), &target)
		require.Equal(t, common.Voter{Type: common.VoterTypeDRepKeyHash, Hash: drepIdentityHash}, target.Voter)
	})
}

func TestVoteDelegationDistinguishesDRepCredentialType(t *testing.T) {
	deposit := uint64(500_000_000)
	ls := scriptOnlyDRepState(&deposit)
	pp := &conway.ConwayProtocolParameters{DRepDeposit: deposit}
	makeTx := func(drepType int) *conway.ConwayTransaction {
		return &conway.ConwayTransaction{Body: conway.ConwayTransactionBody{TxCertificates: []common.CertificateWrapper{{Certificate: &common.VoteDelegationCertificate{
			StakeCredential: drepIdentityCredential(common.CredentialTypeAddrKeyHash),
			Drep:            common.Drep{Type: drepType, Credential: drepIdentityHash[:]},
		}}}}, TxIsValid: true}
	}
	require.NoError(t, conway.UtxoValidateDelegation(makeTx(common.DrepTypeScriptHash), 0, ls, pp))
	var target conway.DelegateVoteToUnregisteredDRepError
	require.ErrorAs(t, conway.UtxoValidateDelegation(makeTx(common.DrepTypeAddrKeyHash), 0, ls, pp), &target)
	require.Equal(t, drepIdentityCredential(common.CredentialTypeAddrKeyHash), target.DRepCredential)
}

func TestUnregisteredDRepDelegationDuringBootstrap(t *testing.T) {
	stake := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224{0x72},
	}
	pool := common.PoolKeyHash(common.Blake2b224{0x73})
	registrationStake := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224{0x75},
	}
	unregistered := common.Drep{
		Type:       common.DrepTypeAddrKeyHash,
		Credential: common.Blake2b224{0x74}.Bytes(),
	}
	state := mockledger.NewLedgerStateBuilder().
		WithStakeCredentialRegistered(stake.Credential, true).
		WithPools([]*common.PoolRegistrationCertificate{{Operator: pool}}).
		Build()
	certificates := []struct {
		name string
		cert common.Certificate
	}{
		{
			name: "vote delegation",
			cert: &common.VoteDelegationCertificate{
				StakeCredential: stake,
				Drep:            unregistered,
			},
		},
		{
			name: "stake and vote delegation",
			cert: &common.StakeVoteDelegationCertificate{
				StakeCredential: stake,
				PoolKeyHash:     pool,
				Drep:            unregistered,
			},
		},
		{
			name: "vote registration delegation",
			cert: &common.VoteRegistrationDelegationCertificate{
				StakeCredential: registrationStake,
				Drep:            unregistered,
			},
		},
		{
			name: "stake and vote registration delegation",
			cert: &common.StakeVoteRegistrationDelegationCertificate{
				StakeCredential: registrationStake,
				PoolKeyHash:     pool,
				Drep:            unregistered,
			},
		},
	}
	for _, tc := range certificates {
		t.Run(tc.name, func(t *testing.T) {
			tx := &conway.ConwayTransaction{
				Body: conway.ConwayTransactionBody{
					TxCertificates: []common.CertificateWrapper{{Certificate: tc.cert}},
				},
				TxIsValid: true,
			}
			pv9 := &conway.ConwayProtocolParameters{
				ProtocolVersion: common.ProtocolParametersProtocolVersion{Major: 9},
			}
			require.NoError(t, conway.UtxoValidateDelegation(tx, 0, state, pv9))

			pv10 := &conway.ConwayProtocolParameters{
				ProtocolVersion: common.ProtocolParametersProtocolVersion{Major: 10},
			}
			var target conway.DelegateVoteToUnregisteredDRepError
			require.ErrorAs(t, conway.UtxoValidateDelegation(tx, 0, state, pv10), &target)
			require.Equal(t, common.Credential{
				CredType:   common.CredentialTypeAddrKeyHash,
				Credential: common.NewBlake2b224(unregistered.Credential),
			}, target.DRepCredential)
		})
	}
}

func TestUnregisteredDRepDelegationBootstrapFullValidation(t *testing.T) {
	fixture := newCertificateDepositCredentialFixture(
		t,
		common.CredentialTypeAddrKeyHash,
	)
	pool := common.PoolKeyHash(common.Blake2b224Hash([]byte("bootstrap-pool")))
	certificate, err := cbor.Encode([]any{
		uint64(common.CertificateTypeVoteDelegation),
		[]any{fixture.credential.CredType, fixture.credential.Credential.Bytes()},
		[]any{
			uint64(common.DrepTypeAddrKeyHash),
			common.Blake2b224{0x74}.Bytes(),
		},
	})
	require.NoError(t, err)
	tx := certificateDepositTransaction(t, fixture, [][]byte{certificate}, 0, 0)
	state := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{{
			Id: shelley.NewShelleyTransactionInput(certificateDepositTxId, 0),
			Output: shelley.ShelleyTransactionOutput{
				OutputAmount: certificateDepositInputAmount,
			},
		}}).
		WithNetworkId(1).
		WithPoolRegistrations([]common.PoolRegistrationCertificate{{Operator: pool}}).
		WithStakeCredentialRegistered(fixture.credential.Credential, true).
		Build()
	params := certificateDepositPparams()
	params.ProtocolVersion.Major = common.ProtocolVersionConway
	require.NoError(t, runCertificateDepositProductionRules(t, tx, state, params))

	params.ProtocolVersion.Major = common.ProtocolVersionPlomin
	var target conway.DelegateVoteToUnregisteredDRepError
	require.ErrorAs(
		t,
		runCertificateDepositProductionRules(t, tx, state, params),
		&target,
	)
}

func TestCertificateDepositsDistinguishesDRepCredentialType(t *testing.T) {
	deposit := uint64(500_000_000)
	ls := scriptOnlyDRepState(&deposit)
	pp := &conway.ConwayProtocolParameters{DRepDeposit: deposit}
	require.NoError(t, conway.UtxoValidateCertificateDeposits(drepDeregistrationTx(drepIdentityCredential(common.CredentialTypeScriptHash), int64(deposit)), 0, ls, pp))
	var target conway.DRepNotRegisteredError
	require.ErrorAs(t, conway.UtxoValidateCertificateDeposits(drepDeregistrationTx(drepIdentityCredential(common.CredentialTypeAddrKeyHash), int64(deposit)), 0, ls, pp), &target)
	require.Equal(t, drepIdentityCredential(common.CredentialTypeAddrKeyHash), target.Credential)
}
