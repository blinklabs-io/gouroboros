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

// This file covers gouroboros#2386: Conway GOV predicates must see the
// effect of certificates earlier in the same transaction's own certificate
// list (the reference applies CERTS before GOV and passes GOV the resulting
// CertState), not only already-committed ledger state.
//
// Every scenario below runs through govOverlayPipeline, a curated,
// order-preserving slice of the real, phase-2-gated conway.UtxoValidationRules
// (not the bare rule functions) covering every certificate-legality and
// governance predicate the overlay interacts with -- UtxoValidateDelegation,
// UtxoValidateCertificateDeposits, UtxoValidateCommitteeCertificates,
// UtxoValidatePoolCertificates (certificate legality, unmodified by this
// change) alongside UtxoValidateProposalReturnAccounts and
// UtxoValidateUnknownVoters (the two predicates this change fixes). This
// proves the fix through the actual certs-then-governance interaction rather
// than by calling either fixed predicate directly. It omits the UTXO/fee/
// witness rules that sit between the two groups in the full 60-rule
// pipeline: those rules are unrelated to CERTS/GOV ordering and satisfying
// them would need a fully realized, fee-correct transaction for every
// scenario without changing what this test proves.
package conway_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// govOverlayPipeline returns, in their real pipeline order, the
// phase-2-gated rule functions this file's scenarios exercise.
func govOverlayPipeline(t *testing.T) []common.UtxoValidationRuleFunc {
	t.Helper()
	wanted := []common.UtxoValidationRuleId{
		common.UtxoValidationRuleProposalReturnAccounts,
		common.UtxoValidationRuleEmptyTreasuryWithdrawals,
		common.UtxoValidationRuleDelegation,
		common.UtxoValidationRuleCertificateDeposits,
		common.UtxoValidationRuleCommitteeCertificates,
		common.UtxoValidationRuleUnknownVoters,
		common.UtxoValidationRuleUnknownGovActionIds,
		common.UtxoValidationRuleVotingOnExpiredGovAction,
		common.UtxoValidationRuleStakePoolVotingRestrictions,
		common.UtxoValidationRulePoolCertificates,
	}
	descriptors := conway.UtxoValidationRuleDescriptors()
	indexOf := make(map[common.UtxoValidationRuleId]int, len(descriptors))
	for idx, d := range descriptors {
		indexOf[d.Id] = idx
	}
	rules := make([]common.UtxoValidationRuleFunc, 0, len(wanted))
	for _, id := range wanted {
		idx, ok := indexOf[id]
		if !ok {
			t.Fatalf("validation rule %q is not registered", id)
		}
		rules = append(rules, conway.UtxoValidationRules[idx])
	}
	return rules
}

func runGovOverlayPipeline(
	t *testing.T,
	tx common.Transaction,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	t.Helper()
	return common.VerifyTransaction(tx, 0, ls, pp, govOverlayPipeline(t))
}

// govOverlayPparams returns Conway protocol parameters at PV10 (Plomin, past
// the bootstrap phase) with deposit and pool-cost values this file's
// certificates are built to satisfy.
func govOverlayPparams() *conway.ConwayProtocolParameters {
	return &conway.ConwayProtocolParameters{
		ProtocolVersion: common.ProtocolParametersProtocolVersion{
			Major: common.ProtocolVersionPlomin,
		},
		KeyDeposit:  2_000_000,
		DRepDeposit: 500_000_000,
		MinPoolCost: 0,
	}
}

func mkCertWrapper(
	certType uint,
	cert common.Certificate,
) common.CertificateWrapper {
	return common.CertificateWrapper{Type: certType, Certificate: cert}
}

// mkCertsAndVoteTx builds a transaction carrying certs followed by a single
// vote, matching a transaction's own certificate list preceding its own
// governance signals.
func mkCertsAndVoteTx(
	certs []common.CertificateWrapper,
	voter common.Voter,
	actionId common.GovActionId,
) *conway.ConwayTransaction {
	tx := mkVoteTx(voter, actionId, common.GovVoteYes)
	tx.Body.TxCertificates = certs
	return tx
}

// mkCertsAndProposalTx builds a transaction carrying certs followed by a
// single proposal procedure.
func mkCertsAndProposalTx(
	certs []common.CertificateWrapper,
	rewardAccount common.Address,
	action common.GovAction,
) *conway.ConwayTransaction {
	tx := mkProposalTx(0, rewardAccount, action)
	tx.Body.TxCertificates = certs
	return tx
}

// govOverlayGovAction registers a votable InfoAction in ls so
// UtxoValidateUnknownGovActionIds and UtxoValidateVotingOnExpiredGovAction
// (both in govOverlayPipeline) do not themselves reject the vote: this
// file's scenarios are about voter identity, not action existence.
func govOverlayGovAction(
	actionId common.GovActionId,
) map[string]*common.GovActionState {
	return map[string]*common.GovActionState{
		govActionKey(actionId): {
			ActionId:   actionId,
			ActionType: common.GovActionTypeInfo,
		},
	}
}

func TestUtxoValidateUnknownVotersSeesInTxDRepRegistration(t *testing.T) {
	t.Parallel()
	pp := govOverlayPparams()
	actionId := common.GovActionId{TransactionId: common.Blake2b256{0xA1}}
	drep := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte("in-tx-drep")),
	}
	ls := mockledger.NewLedgerStateBuilder().
		WithGovActions(govOverlayGovAction(actionId)).
		Build()
	certs := []common.CertificateWrapper{
		mkCertWrapper(
			uint(common.CertificateTypeRegistrationDrep),
			&common.RegistrationDrepCertificate{
				CertType:       uint(common.CertificateTypeRegistrationDrep),
				DrepCredential: drep,
				Amount:         int64(pp.DRepDeposit),
			},
		),
	}
	tx := mkCertsAndVoteTx(
		certs,
		common.Voter{Type: common.VoterTypeDRepKeyHash, Hash: drep.Credential},
		actionId,
	)

	// gouroboros#2386: reference accepts (RegisterDRep(D) then vote as D).
	require.NoError(t, runGovOverlayPipeline(t, tx, ls, pp))
}

func TestUtxoValidateUnknownVotersRejectsInTxDRepDeregistration(t *testing.T) {
	t.Parallel()
	pp := govOverlayPparams()
	actionId := common.GovActionId{TransactionId: common.Blake2b256{0xA2}}
	drep := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte("dereg-drep")),
	}
	drepDeposit := pp.DRepDeposit
	ls := mockledger.NewLedgerStateBuilder().
		WithGovActions(govOverlayGovAction(actionId)).
		WithDRepRegistrations([]common.DRepRegistration{
			{Credential: drep, Deposit: &drepDeposit},
		}).
		Build()
	certs := []common.CertificateWrapper{
		mkCertWrapper(
			uint(common.CertificateTypeDeregistrationDrep),
			&common.DeregistrationDrepCertificate{
				CertType:       uint(common.CertificateTypeDeregistrationDrep),
				DrepCredential: drep,
				Amount:         int64(drepDeposit),
			},
		),
	}
	tx := mkCertsAndVoteTx(
		certs,
		common.Voter{Type: common.VoterTypeDRepKeyHash, Hash: drep.Credential},
		actionId,
	)

	// gouroboros#2386: reference rejects (DeregisterDRep(D) then vote as D).
	err := runGovOverlayPipeline(t, tx, ls, pp)
	var unkErr conway.UnknownVoterError
	require.ErrorAs(t, err, &unkErr)
}

func TestUtxoValidateUnknownVotersSeesInTxPoolRegistration(t *testing.T) {
	t.Parallel()
	pp := govOverlayPparams()
	actionId := common.GovActionId{TransactionId: common.Blake2b256{0xA3}}
	pool := common.PoolKeyHash(common.Blake2b224Hash([]byte("in-tx-pool")))
	ls := mockledger.NewLedgerStateBuilder().
		WithGovActions(govOverlayGovAction(actionId)).
		Build()
	certs := []common.CertificateWrapper{
		mkCertWrapper(
			uint(common.CertificateTypePoolRegistration),
			&common.PoolRegistrationCertificate{
				CertType: uint(common.CertificateTypePoolRegistration),
				Operator: pool,
				Cost:     0,
			},
		),
	}
	tx := mkCertsAndVoteTx(
		certs,
		common.Voter{
			Type: common.VoterTypeStakingPoolKeyHash,
			Hash: common.Blake2b224(pool),
		},
		actionId,
	)

	// gouroboros#2386: reference accepts (RegisterPool(P) then vote as P).
	require.NoError(t, runGovOverlayPipeline(t, tx, ls, pp))
}

func TestUtxoValidateUnknownVotersSeesInTxCommitteeHotAuthorization(
	t *testing.T,
) {
	t.Parallel()
	pp := govOverlayPparams()
	actionId := common.GovActionId{TransactionId: common.Blake2b256{0xA4}}
	cold := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte("cc-cold-auth")),
	}
	hot := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224Hash([]byte("cc-hot-auth")),
	}
	ls := mockledger.NewLedgerStateBuilder().
		WithGovActions(govOverlayGovAction(actionId)).
		WithCommitteeMembers([]common.CommitteeMember{
			{ColdKey: cold.Credential, ExpiryEpoch: 500},
		}).
		Build()
	certs := []common.CertificateWrapper{
		mkCertWrapper(
			uint(common.CertificateTypeAuthCommitteeHot),
			&common.AuthCommitteeHotCertificate{
				CertType:       uint(common.CertificateTypeAuthCommitteeHot),
				ColdCredential: cold,
				HotCredential:  hot,
			},
		),
	}
	tx := mkCertsAndVoteTx(
		certs,
		common.Voter{
			Type: common.VoterTypeConstitutionalCommitteeHotKeyHash,
			Hash: hot.Credential,
		},
		actionId,
	)

	// gouroboros#2386: reference accepts (authorize a CC hot key then vote
	// with it in the same transaction).
	require.NoError(t, runGovOverlayPipeline(t, tx, ls, pp))
}

func TestUtxoValidateUnknownVotersRequiresElectedCommitteeAtPV11(t *testing.T) {
	t.Parallel()
	hotHash := common.Blake2b224{0x42}
	coldHash := common.Blake2b224{0x43}
	hotKey := hotHash
	base := mockledger.NewLedgerStateBuilder().Build()
	pendingState := committeeCredentialLedgerState{
		LedgerState: base,
		available:   true,
		hotLookup: func(common.Credential) (*common.CommitteeMember, error) {
			return &common.CommitteeMember{ColdKey: coldHash, HotKey: &hotKey}, nil
		},
	}
	voter := common.Voter{
		Type: common.VoterTypeConstitutionalCommitteeHotKeyHash,
		Hash: hotHash,
	}
	tx := mkVoteTx(voter, common.GovActionId{}, common.GovVoteYes)
	pv11 := govOverlayPparams()
	pv11.ProtocolVersion.Major = common.ProtocolVersionVanRossem
	var unknown conway.UnknownVoterError
	err := conway.UtxoValidateUnknownVoters(tx, 0, pendingState, pv11)
	require.ErrorAs(t, err, &unknown)
	require.NoError(
		t,
		conway.UtxoValidateUnknownVoters(tx, 0, pendingState, govOverlayPparams()),
	)

	electedState := mockledger.NewLedgerStateBuilder().WithCommitteeMembers(
		[]common.CommitteeMember{{ColdKey: coldHash, HotKey: &hotKey}},
	).Build()
	require.NoError(t, conway.UtxoValidateUnknownVoters(tx, 0, electedState, pv11))
}

func TestUtxoValidateUnknownVotersRejectsResignedCommitteeOldHotKey(
	t *testing.T,
) {
	t.Parallel()
	pp := govOverlayPparams()
	actionId := common.GovActionId{TransactionId: common.Blake2b256{0xA5}}
	coldHash := common.Blake2b224Hash([]byte("cc-cold-resign"))
	hotHash := common.Blake2b224Hash([]byte("cc-hot-resign"))
	cold := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: coldHash,
	}
	ls := mockledger.NewLedgerStateBuilder().
		WithGovActions(govOverlayGovAction(actionId)).
		WithCommitteeMembers([]common.CommitteeMember{
			{ColdKey: coldHash, HotKey: &hotHash, ExpiryEpoch: 500},
		}).
		Build()
	certs := []common.CertificateWrapper{
		mkCertWrapper(
			uint(common.CertificateTypeResignCommitteeCold),
			&common.ResignCommitteeColdCertificate{
				CertType:       uint(common.CertificateTypeResignCommitteeCold),
				ColdCredential: cold,
			},
		),
	}
	tx := mkCertsAndVoteTx(
		certs,
		common.Voter{
			Type: common.VoterTypeConstitutionalCommitteeHotKeyHash,
			Hash: hotHash,
		},
		actionId,
	)

	// gouroboros#2386: reference rejects (resign a CC member, then vote
	// with the now-superseded hot key, in the same transaction).
	err := runGovOverlayPipeline(t, tx, ls, pp)
	var unkErr conway.UnknownVoterError
	require.ErrorAs(t, err, &unkErr)

	// The committee member's base-state record must not have been mutated
	// by the rejected validation call.
	member, err := ls.CommitteeCredentialMember(cold)
	require.NoError(t, err)
	require.NotNil(t, member)
	require.False(t, member.Resigned)
	require.NotNil(t, member.HotKey)
	require.Equal(t, hotHash, *member.HotKey)
}

func TestUtxoValidateProposalReturnAccountsSeesInTxStakeRegistration(
	t *testing.T,
) {
	t.Parallel()
	pp := govOverlayPparams()
	cred := common.Blake2b224Hash([]byte("in-tx-return-account"))
	returnAddr := makeConwayRewardAddress(t, cred)
	ls := mockledger.NewLedgerStateBuilder().Build()
	certs := []common.CertificateWrapper{
		mkCertWrapper(
			uint(common.CertificateTypeRegistration),
			&common.RegistrationCertificate{
				CertType: uint(common.CertificateTypeRegistration),
				StakeCredential: common.Credential{
					CredType:   common.CredentialTypeAddrKeyHash,
					Credential: cred,
				},
				Amount: int64(pp.KeyDeposit),
			},
		),
	}
	tx := mkCertsAndProposalTx(certs, returnAddr, &common.InfoGovAction{})

	// gouroboros#2386, and the confirmed live Preview regression in the
	// linked issue
	// (tx 89e26414e949af4e2b60d422727a5e2bbf2242276345ccf14802576a20faf135):
	// reference accepts a stake account this same transaction registers as
	// the proposal's own return account.
	require.NoError(t, runGovOverlayPipeline(t, tx, ls, pp))
}

func TestUtxoValidateProposalReturnAccountsSeesInTxTreasuryReturnAccount(
	t *testing.T,
) {
	t.Parallel()
	pp := govOverlayPparams()
	proposerCred := common.Blake2b224Hash([]byte("proposer-return-account"))
	proposerAddr := makeConwayRewardAddress(t, proposerCred)
	treasuryCred := common.Blake2b224Hash([]byte("in-tx-treasury-return"))
	treasuryAddr := makeConwayRewardAddress(t, treasuryCred)
	ls := mockledger.NewLedgerStateBuilder().
		WithStakeCredentialRegistered(proposerCred, true).
		Build()
	certs := []common.CertificateWrapper{
		mkCertWrapper(
			uint(common.CertificateTypeRegistration),
			&common.RegistrationCertificate{
				CertType: uint(common.CertificateTypeRegistration),
				StakeCredential: common.Credential{
					CredType:   common.CredentialTypeAddrKeyHash,
					Credential: treasuryCred,
				},
				Amount: int64(pp.KeyDeposit),
			},
		),
	}
	action := &common.TreasuryWithdrawalGovAction{
		Withdrawals: map[*common.Address]uint64{&treasuryAddr: 1_000_000},
	}
	tx := mkCertsAndProposalTx(certs, proposerAddr, action)

	// gouroboros#2386: the same in-transaction visibility applies to
	// treasury withdrawal return accounts.
	require.NoError(t, runGovOverlayPipeline(t, tx, ls, pp))
}

func TestUtxoValidateProposalReturnAccountsRejectsInTxStakeDeregistration(
	t *testing.T,
) {
	t.Parallel()
	pp := govOverlayPparams()
	cred := common.Blake2b224Hash([]byte("dereg-return-account"))
	returnAddr := makeConwayRewardAddress(t, cred)
	credential := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: cred,
	}
	ls := mockledger.NewLedgerStateBuilder().
		WithStakeCredentialRegistered(cred, true).
		WithStakeCredentialDeposits(map[mockledger.RewardAccountKey]uint64{
			mockledger.NewRewardAccountKey(credential): uint64(pp.KeyDeposit),
		}).
		Build()
	certs := []common.CertificateWrapper{
		mkCertWrapper(
			uint(common.CertificateTypeStakeDeregistration),
			&common.StakeDeregistrationCertificate{
				CertType:        uint(common.CertificateTypeStakeDeregistration),
				StakeCredential: credential,
			},
		),
	}
	tx := mkCertsAndProposalTx(certs, returnAddr, &common.InfoGovAction{})

	// gouroboros#2386: reference rejects (deregister a stake account, then
	// use it as a return account, in the same transaction).
	err := runGovOverlayPipeline(t, tx, ls, pp)
	var acctErr conway.ProposalReturnAccountDoesNotExistError
	require.ErrorAs(t, err, &acctErr)
	require.Equal(t, returnAddr, acctErr.Address)

	// The rejected validation call must not have mutated base ledger state.
	require.True(t, ls.IsStakeCredentialRegistered(credential))
}

// TestUtxoValidateUnknownVotersUnaffectedWithoutCerts pins the existing,
// no-certificate behavior of a vote from an already-registered DRep
// (TestUtxoValidateUnknownVoters in rules_gov_transition_test.go) so a
// future change to the overlay cannot silently start requiring certificates
// to be present for an ordinary proposal-and-vote transaction to validate.
func TestUtxoValidateUnknownVotersUnaffectedWithoutCerts(t *testing.T) {
	t.Parallel()
	pp := govOverlayPparams()
	actionId := common.GovActionId{TransactionId: common.Blake2b256{0xA6}}
	drepHash := common.Blake2b224Hash([]byte("already-registered-drep"))
	ls := mockledger.NewLedgerStateBuilder().
		WithGovActions(govOverlayGovAction(actionId)).
		WithDRepRegistrations([]common.DRepRegistration{{
			Credential: common.Credential{
				CredType:   common.CredentialTypeAddrKeyHash,
				Credential: drepHash,
			},
		}}).
		Build()
	tx := mkVoteTx(
		common.Voter{Type: common.VoterTypeDRepKeyHash, Hash: drepHash},
		actionId,
		common.GovVoteYes,
	)

	require.NoError(t, runGovOverlayPipeline(t, tx, ls, pp))
}
