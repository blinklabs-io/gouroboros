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

package common_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func blockTestCredential(b byte) common.Credential {
	return common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224(bytes.Repeat([]byte{b}, 28)),
	}
}

func blockTestTx(
	t *testing.T,
	id byte,
	valid bool,
) *mockledger.MockTransaction {
	t.Helper()
	input, err := mockledger.NewSimpleTransactionInput(
		bytes.Repeat([]byte{id}, 32),
		0,
	)
	require.NoError(t, err)
	output, err := mockledger.NewSimpleTransactionOutput(
		"addr_test1qqx80sj9nwxdnglmzdl95v2k40d9422au0klwav8jz2dj985v0wma0mza32f8z6pv2jmkn7cen50f9vn9jmp7dd0njcqqpce07",
		2_000_000,
	)
	require.NoError(t, err)
	tx, err := mockledger.NewTransactionBuilder().
		WithId(bytes.Repeat([]byte{id}, 32)).
		WithInputs(input).
		WithOutputs(output).
		WithValid(valid).
		Build()
	require.NoError(t, err)
	return tx.(*mockledger.MockTransaction)
}

func applyBlockTestTxs(
	t *testing.T,
	state *common.BlockLedgerState,
	txs ...common.Transaction,
) {
	t.Helper()
	for _, tx := range txs {
		require.NoError(
			t,
			state.ApplyTransaction(tx, &conway.ConwayProtocolParameters{}),
		)
	}
}

func TestBlockLedgerStateAppliesStakeRegistration(t *testing.T) {
	registered := blockTestCredential(0x01)
	deregistered := blockTestCredential(0x02)
	base := mockledger.NewLedgerStateBuilder().
		WithRewardAccountCredentialBalance(deregistered, 500).
		WithStakeCredentialDeposits(map[mockledger.RewardAccountKey]uint64{
			mockledger.NewRewardAccountKey(deregistered): 2_000_000,
		}).
		Build()
	state := common.NewBlockLedgerState(base)
	applyBlockTestTxs(t, state, blockTestTx(t, 0xa1, true).WithCertificates(
		&common.RegistrationCertificate{
			StakeCredential: registered,
			Amount:          3_000_000,
		},
		&common.DeregistrationCertificate{
			StakeCredential: deregistered,
			Amount:          2_000_000,
		},
	))

	require.True(t, state.IsStakeCredentialRegistered(registered))
	require.True(t, state.IsRewardAccountRegistered(registered))
	balance, err := state.RewardAccountBalance(registered)
	require.NoError(t, err)
	require.Equal(t, uint64(0), *balance)
	require.False(t, state.IsStakeCredentialRegistered(deregistered))
	balance, err = state.RewardAccountBalance(deregistered)
	require.NoError(t, err)
	require.Nil(t, balance)

	deposits, ok := common.StakeCredentialDepositStateFor(state)
	require.True(t, ok)
	deposit, err := deposits.StakeCredentialDeposit(registered)
	require.NoError(t, err)
	require.Equal(t, uint64(3_000_000), *deposit)
	deposit, err = deposits.StakeCredentialDeposit(deregistered)
	require.NoError(t, err)
	require.Nil(t, deposit)
}

// A pre-Conway registration certificate carries no amount; its deposit is
// the protocol parameters' key deposit.
func TestBlockLedgerStateRecordsKeyDepositForLegacyRegistration(
	t *testing.T,
) {
	cred := blockTestCredential(0x01)
	state := common.NewBlockLedgerState(
		mockledger.NewLedgerStateBuilder().Build(),
	)
	pp := &conway.ConwayProtocolParameters{KeyDeposit: 2_000_000}
	require.NoError(t, state.ApplyTransaction(
		blockTestTx(t, 0xa1, true).WithCertificates(
			&common.StakeRegistrationCertificate{StakeCredential: cred},
		),
		pp,
	))
	deposits, ok := common.StakeCredentialDepositStateFor(state)
	require.True(t, ok)
	deposit, err := deposits.StakeCredentialDeposit(cred)
	require.NoError(t, err)
	require.Equal(t, uint64(2_000_000), *deposit)
}

func TestBlockLedgerStateAppliesPoolCertificates(t *testing.T) {
	pool := common.PoolKeyHash(bytes.Repeat([]byte{0x31}, 28))
	vrf := common.Blake2b256(bytes.Repeat([]byte{0x41}, 32))
	state := common.NewBlockLedgerState(
		mockledger.NewLedgerStateBuilder().Build(),
	)
	registration := &common.PoolRegistrationCertificate{
		Operator:   pool,
		VrfKeyHash: vrf,
	}
	applyBlockTestTxs(t, state,
		blockTestTx(t, 0xa1, true).WithCertificates(registration),
		blockTestTx(t, 0xa2, true).WithCertificates(
			&common.PoolRetirementCertificate{PoolKeyHash: pool, Epoch: 9},
		),
	)
	require.True(t, state.IsPoolRegistered(pool))
	reg, retirement, err := state.PoolCurrentState(pool)
	require.NoError(t, err)
	require.Same(t, registration, reg)
	require.Equal(t, uint64(9), *retirement)
	inUse, owner, err := state.IsVrfKeyInUse(vrf)
	require.NoError(t, err)
	require.True(t, inUse)
	require.Equal(t, pool, owner)

	// Re-registration cancels the pending retirement.
	applyBlockTestTxs(t, state,
		blockTestTx(t, 0xa3, true).WithCertificates(registration),
	)
	_, retirement, err = state.PoolCurrentState(pool)
	require.NoError(t, err)
	require.Nil(t, retirement)
}

func TestBlockLedgerStateAppliesDRepCertificates(t *testing.T) {
	drep := blockTestCredential(0x51)
	state := common.NewBlockLedgerState(
		mockledger.NewLedgerStateBuilder().Build(),
	)
	anchor := &common.GovAnchor{Url: "https://example.com/drep"}
	applyBlockTestTxs(t, state,
		blockTestTx(t, 0xa1, true).WithCertificates(
			&common.RegistrationDrepCertificate{
				DrepCredential: drep,
				Amount:         500_000_000,
			},
		),
		blockTestTx(t, 0xa2, true).WithCertificates(
			&common.UpdateDrepCertificate{
				DrepCredential: drep,
				Anchor:         anchor,
			},
		),
	)
	reg, err := state.DRepRegistration(drep)
	require.NoError(t, err)
	require.NotNil(t, reg)
	require.Equal(t, uint64(500_000_000), *reg.Deposit)
	require.Equal(t, anchor, reg.Anchor)
	regs, err := state.DRepRegistrations()
	require.NoError(t, err)
	require.Len(t, regs, 1)

	applyBlockTestTxs(t, state,
		blockTestTx(t, 0xa3, true).WithCertificates(
			&common.DeregistrationDrepCertificate{DrepCredential: drep},
		),
	)
	reg, err = state.DRepRegistration(drep)
	require.NoError(t, err)
	require.Nil(t, reg)
}

// A delegation made after a DRep deregistered, to the same DRep after it
// re-registered, survives; only delegations from before are cleared.
func TestBlockLedgerStateClearsDelegationsMadeBeforeDRepDeregistration(
	t *testing.T,
) {
	stake := blockTestCredential(0x01)
	drepCred := blockTestCredential(0x51)
	drep := common.Drep{
		Type:       common.DrepTypeAddrKeyHash,
		Credential: drepCred.Credential.Bytes(),
	}
	state := common.NewBlockLedgerState(
		mockledger.NewLedgerStateBuilder().
			WithDRepDelegation(func(common.Credential) (*common.Drep, error) {
				return &drep, nil
			}).
			Build(),
	)
	delegations, ok := common.DRepDelegationStateFor(state)
	require.True(t, ok)

	applyBlockTestTxs(t, state, blockTestTx(t, 0xa1, true).WithCertificates(
		&common.DeregistrationDrepCertificate{DrepCredential: drepCred},
	))
	got, err := delegations.DRepDelegation(stake)
	require.NoError(t, err)
	require.Nil(t, got)

	applyBlockTestTxs(t, state, blockTestTx(t, 0xa2, true).WithCertificates(
		&common.RegistrationDrepCertificate{DrepCredential: drepCred},
		&common.VoteDelegationCertificate{
			StakeCredential: stake,
			Drep:            drep,
		},
	))
	got, err = delegations.DRepDelegation(stake)
	require.NoError(t, err)
	require.Equal(t, &drep, got)
}

func TestBlockLedgerStateAppliesCommitteeCertificates(t *testing.T) {
	cold := blockTestCredential(0x61)
	oldHot := blockTestCredential(0x62)
	newHot := blockTestCredential(0x63)
	oldHotHash := oldHot.Credential
	member := &common.CommitteeMember{
		ColdKey: cold.Credential,
		HotKey:  &oldHotHash,
	}
	state := common.NewBlockLedgerState(
		mockledger.NewLedgerStateBuilder().
			WithCommitteeCredentialMember(func(c common.Credential) (*common.CommitteeMember, error) {
				if c.Credential == cold.Credential {
					return member, nil
				}
				return nil, nil
			}).
			WithCommitteeHotCredentialMember(func(c common.Credential) (*common.CommitteeMember, error) {
				if c.Credential == oldHot.Credential {
					return member, nil
				}
				return nil, nil
			}).
			Build(),
	)
	committee, ok := common.CommitteeCredentialStateFor(state)
	require.True(t, ok)

	applyBlockTestTxs(t, state, blockTestTx(t, 0xa1, true).WithCertificates(
		&common.AuthCommitteeHotCertificate{
			ColdCredential: cold,
			HotCredential:  newHot,
		},
	))
	got, err := committee.CommitteeHotCredentialMember(newHot)
	require.NoError(t, err)
	require.Equal(t, newHot.Credential, *got.HotKey)
	got, err = committee.CommitteeHotCredentialMember(oldHot)
	require.NoError(t, err)
	require.Nil(t, got)

	applyBlockTestTxs(t, state, blockTestTx(t, 0xa2, true).WithCertificates(
		&common.ResignCommitteeColdCertificate{ColdCredential: cold},
	))
	got, err = committee.CommitteeCredentialMember(cold)
	require.NoError(t, err)
	require.True(t, got.Resigned)
	require.Nil(t, got.HotKey)
	got, err = committee.CommitteeHotCredentialMember(newHot)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestBlockLedgerStateRecordsProposals(t *testing.T) {
	state := common.NewBlockLedgerState(
		mockledger.NewLedgerStateBuilder().Build(),
	)
	tx := blockTestTx(t, 0xa1, true).WithProposalProcedures(
		conway.ConwayProposalProcedure{
			PPGovAction: conway.ConwayGovAction{
				Action: &common.InfoGovAction{},
			},
		},
	)
	applyBlockTestTxs(t, state, tx)
	id := common.GovActionId{TransactionId: tx.Hash(), GovActionIdx: 0}
	require.True(t, state.GovActionExists(id))
	action, err := state.GovActionById(id)
	require.NoError(t, err)
	require.Equal(t, common.GovActionTypeInfo, action.ActionType)
}

// A phase-2-invalid transaction applies no certificate, withdrawal or
// proposal.
func TestBlockLedgerStateIgnoresInvalidTransactionEffects(t *testing.T) {
	cred := blockTestCredential(0x01)
	state := common.NewBlockLedgerState(
		mockledger.NewLedgerStateBuilder().Build(),
	)
	tx := blockTestTx(t, 0xa1, false).
		WithCertificates(&common.RegistrationCertificate{StakeCredential: cred}).
		WithProposalProcedures(conway.ConwayProposalProcedure{
			PPGovAction: conway.ConwayGovAction{
				Action: &common.InfoGovAction{},
			},
		})
	applyBlockTestTxs(t, state, tx)
	require.False(t, state.IsStakeCredentialRegistered(cred))
	require.False(t, state.GovActionExists(
		common.GovActionId{TransactionId: tx.Hash()},
	))
}

func TestBlockLedgerStateHelpersReportAbsentCapabilities(t *testing.T) {
	state := common.NewBlockLedgerState(
		mockledger.NewLedgerStateBuilder().Build(),
	)
	_, ok := common.CommitteeHotCredentialMembersFor(state)
	require.False(t, ok)
	_, ok = common.CommitteeVotingStateFor(state)
	require.False(t, ok)
}
