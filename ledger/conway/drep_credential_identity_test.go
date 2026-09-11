// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");

package conway_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
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
}

func TestCertificateDepositsDistinguishesDRepCredentialType(t *testing.T) {
	deposit := uint64(500_000_000)
	ls := scriptOnlyDRepState(&deposit)
	pp := &conway.ConwayProtocolParameters{DRepDeposit: deposit}
	require.NoError(t, conway.UtxoValidateCertificateDeposits(drepDeregistrationTx(drepIdentityCredential(common.CredentialTypeScriptHash), int64(deposit)), 0, ls, pp))
	var target conway.DRepNotRegisteredError
	require.ErrorAs(t, conway.UtxoValidateCertificateDeposits(drepDeregistrationTx(drepIdentityCredential(common.CredentialTypeAddrKeyHash), int64(deposit)), 0, ls, pp), &target)
}
