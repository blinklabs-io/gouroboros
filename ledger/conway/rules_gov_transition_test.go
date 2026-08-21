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

// This file covers the Conway GOV proposal/voting checks added to close the
// gap described in https://github.com/blinklabs-io/gouroboros/issues/1938:
// proposal deposit, return-account registration, action well-formedness,
// hard-fork protocol version compatibility, ancestry, unknown voters,
// unknown governance action ids, expired-action voting, and bootstrap/SPO
// voter authorization.

package conway_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func govActionKey(id common.GovActionId) string {
	return fmt.Sprintf("%x#%d", id.TransactionId[:], id.GovActionIdx)
}

func mkConwayPp(
	major uint,
	govActionDeposit uint64,
) *conway.ConwayProtocolParameters {
	return &conway.ConwayProtocolParameters{
		ProtocolVersion: common.ProtocolParametersProtocolVersion{
			Major: major,
		},
		GovActionDeposit: govActionDeposit,
	}
}

func mkProposalTx(
	deposit uint64,
	rewardAccount common.Address,
	action common.GovAction,
) *conway.ConwayTransaction {
	tx := &conway.ConwayTransaction{}
	tx.Body.TxProposalProcedures = []conway.ConwayProposalProcedure{
		{
			PPDeposit:       deposit,
			PPRewardAccount: rewardAccount,
			PPGovAction:     conway.ConwayGovAction{Action: action},
		},
	}
	return tx
}

func TestUtxoValidateProposalDeposit(t *testing.T) {
	pp := mkConwayPp(common.ProtocolVersionConway, 500_000_000)
	rewardAddr := makeConwayRewardAddress(
		t,
		common.Blake2b224Hash([]byte("deposit-return")),
	)

	t.Run("correct deposit", func(t *testing.T) {
		tx := mkProposalTx(500_000_000, rewardAddr, &common.InfoGovAction{})
		err := conway.UtxoValidateProposalDeposit(tx, 0, nil, pp)
		require.NoError(t, err)
	})

	t.Run("incorrect deposit", func(t *testing.T) {
		tx := mkProposalTx(1, rewardAddr, &common.InfoGovAction{})
		err := conway.UtxoValidateProposalDeposit(tx, 0, nil, pp)
		var depErr conway.ProposalDepositIncorrectError
		require.ErrorAs(t, err, &depErr)
		assert.Equal(t, uint64(1), depErr.Supplied)
		assert.Equal(t, uint64(500_000_000), depErr.Expected)
	})
}

func TestUtxoValidateProposalReturnAccounts(t *testing.T) {
	registeredCred := common.Blake2b224Hash([]byte("registered-return"))
	unregisteredCred := common.Blake2b224Hash([]byte("unregistered-return"))
	registeredAddr := makeConwayRewardAddress(t, registeredCred)
	unregisteredAddr := makeConwayRewardAddress(t, unregisteredCred)

	ls := mockledger.NewLedgerStateBuilder().
		WithStakeCredentialRegistered(registeredCred, true).
		Build()

	// Per cardano-ledger's conwayGovTransition (Cardano.Ledger.Conway.Rules.Gov),
	// the return-account existence checks are wrapped in
	// `unless (hardforkConwayBootstrapPhase ...)`, so they are genuinely
	// skipped at PV9. See the NOTE on UtxoValidateProposalReturnAccounts.
	t.Run("PV9 bootstrap skips the check", func(t *testing.T) {
		pp := mkConwayPp(common.ProtocolVersionConway, 0)
		tx := mkProposalTx(0, unregisteredAddr, &common.InfoGovAction{})
		err := conway.UtxoValidateProposalReturnAccounts(tx, 0, ls, pp)
		require.NoError(t, err)
	})

	t.Run("PV10 registered return account", func(t *testing.T) {
		pp := mkConwayPp(common.ProtocolVersionPlomin, 0)
		tx := mkProposalTx(0, registeredAddr, &common.InfoGovAction{})
		err := conway.UtxoValidateProposalReturnAccounts(tx, 0, ls, pp)
		require.NoError(t, err)
	})

	t.Run("PV10 unregistered return account", func(t *testing.T) {
		pp := mkConwayPp(common.ProtocolVersionPlomin, 0)
		tx := mkProposalTx(0, unregisteredAddr, &common.InfoGovAction{})
		err := conway.UtxoValidateProposalReturnAccounts(tx, 0, ls, pp)
		var acctErr conway.ProposalReturnAccountDoesNotExistError
		require.ErrorAs(t, err, &acctErr)
		assert.Equal(t, unregisteredAddr, acctErr.Address)
	})

	t.Run(
		"PV10 treasury withdrawal to unregistered account",
		func(t *testing.T) {
			pp := mkConwayPp(common.ProtocolVersionPlomin, 0)
			action := &common.TreasuryWithdrawalGovAction{
				Withdrawals: map[*common.Address]uint64{
					&unregisteredAddr: 1_000_000,
				},
			}
			tx := mkProposalTx(0, registeredAddr, action)
			err := conway.UtxoValidateProposalReturnAccounts(tx, 0, ls, pp)
			var twErr conway.TreasuryWithdrawalReturnAccountsDoNotExistError
			require.ErrorAs(t, err, &twErr)
			require.Len(t, twErr.Addresses, 1)
			assert.Equal(t, unregisteredAddr, twErr.Addresses[0])
		},
	)

	t.Run(
		"PV10 rejects a base address as a return account",
		func(t *testing.T) {
			// A base address carries a staking payload, so
			// addr.StakeCredential() succeeds for it even though it is not
			// a reward_account per the CDDL (AddressTypeNoneKey /
			// AddressTypeNoneScript only). It must be rejected as
			// malformed here, not accepted just because the underlying
			// credential happens to be registered.
			baseAddr := makeConwayBaseAddress(t, registeredCred)
			pp := mkConwayPp(common.ProtocolVersionPlomin, 0)
			tx := mkProposalTx(0, baseAddr, &common.InfoGovAction{})
			err := conway.UtxoValidateProposalReturnAccounts(tx, 0, ls, pp)
			var acctErr conway.ProposalReturnAccountDoesNotExistError
			require.ErrorAs(t, err, &acctErr)
			assert.Equal(t, baseAddr, acctErr.Address)
		},
	)

	t.Run(
		"PV10 rejects a base address as a treasury withdrawal account",
		func(t *testing.T) {
			baseAddr := makeConwayBaseAddress(t, registeredCred)
			pp := mkConwayPp(common.ProtocolVersionPlomin, 0)
			action := &common.TreasuryWithdrawalGovAction{
				Withdrawals: map[*common.Address]uint64{
					&baseAddr: 1_000_000,
				},
			}
			tx := mkProposalTx(0, registeredAddr, action)
			err := conway.UtxoValidateProposalReturnAccounts(tx, 0, ls, pp)
			var twErr conway.TreasuryWithdrawalReturnAccountsDoNotExistError
			require.ErrorAs(t, err, &twErr)
			require.Len(t, twErr.Addresses, 1)
			assert.Equal(t, baseAddr, twErr.Addresses[0])
		},
	)

	t.Run(
		"PV10 multiple bad treasury withdrawal accounts sort deterministically",
		func(t *testing.T) {
			// Withdrawals is a map, so iteration order is non-deterministic
			// across runs; the reported Addresses must still come back in
			// the same (sorted-by-bytes) order every time.
			unregisteredCredA := common.Blake2b224Hash(
				[]byte("unregistered-return-a"),
			)
			unregisteredCredB := common.Blake2b224Hash(
				[]byte("unregistered-return-b"),
			)
			addrA := makeConwayRewardAddress(t, unregisteredCredA)
			addrB := makeConwayRewardAddress(t, unregisteredCredB)
			pp := mkConwayPp(common.ProtocolVersionPlomin, 0)
			action := &common.TreasuryWithdrawalGovAction{
				Withdrawals: map[*common.Address]uint64{
					&addrA: 1_000_000,
					&addrB: 2_000_000,
				},
			}
			tx := mkProposalTx(0, registeredAddr, action)
			var wantAddrs []common.Address
			addrABytes, _ := addrA.Bytes()
			addrBBytes, _ := addrB.Bytes()
			if bytes.Compare(addrABytes, addrBBytes) <= 0 {
				wantAddrs = []common.Address{addrA, addrB}
			} else {
				wantAddrs = []common.Address{addrB, addrA}
			}
			for range 10 {
				err := conway.UtxoValidateProposalReturnAccounts(
					tx,
					0,
					ls,
					pp,
				)
				var twErr conway.TreasuryWithdrawalReturnAccountsDoNotExistError
				require.ErrorAs(t, err, &twErr)
				require.Len(t, twErr.Addresses, 2)
				assert.Equal(t, wantAddrs, twErr.Addresses)
			}
		},
	)
}

func TestUtxoValidateGovActionWellFormedness(t *testing.T) {
	pp := mkConwayPp(common.ProtocolVersionConway, 0)

	t.Run("conflicting committee update", func(t *testing.T) {
		cred := common.Credential{
			CredType:   common.CredentialTypeAddrKeyHash,
			Credential: common.Blake2b224Hash([]byte("cc-member")),
		}
		action := &common.UpdateCommitteeGovAction{
			Credentials: []common.Credential{cred},
			CredEpochs: map[*common.Credential]uint{
				&cred: 500,
			},
		}
		tx := mkProposalTx(0, common.Address{}, action)
		err := conway.UtxoValidateGovActionWellFormedness(tx, 0, nil, pp)
		var confErr conway.ConflictingCommitteeUpdateError
		require.ErrorAs(t, err, &confErr)
		require.Len(t, confErr.Credentials, 1)
	})

	t.Run("non-conflicting committee update", func(t *testing.T) {
		removeCred := common.Credential{
			CredType:   common.CredentialTypeAddrKeyHash,
			Credential: common.Blake2b224Hash([]byte("removed")),
		}
		addCred := common.Credential{
			CredType:   common.CredentialTypeAddrKeyHash,
			Credential: common.Blake2b224Hash([]byte("added")),
		}
		action := &common.UpdateCommitteeGovAction{
			Credentials: []common.Credential{removeCred},
			CredEpochs: map[*common.Credential]uint{
				&addCred: 500,
			},
		}
		tx := mkProposalTx(0, common.Address{}, action)
		err := conway.UtxoValidateGovActionWellFormedness(tx, 0, nil, pp)
		require.NoError(t, err)
	})

	t.Run("malformed policy hash length", func(t *testing.T) {
		action := &common.TreasuryWithdrawalGovAction{
			Withdrawals: map[*common.Address]uint64{},
			PolicyHash:  []byte{0x01, 0x02},
		}
		tx := mkProposalTx(0, common.Address{}, action)
		err := conway.UtxoValidateGovActionWellFormedness(tx, 0, nil, pp)
		var malErr conway.MalformedGovActionError
		require.ErrorAs(t, err, &malErr)
	})

	t.Run("malformed constitution script hash length", func(t *testing.T) {
		action := &common.NewConstitutionGovAction{}
		action.Constitution.ScriptHash = []byte{0x01}
		tx := mkProposalTx(0, common.Address{}, action)
		err := conway.UtxoValidateGovActionWellFormedness(tx, 0, nil, pp)
		var malErr conway.MalformedGovActionError
		require.ErrorAs(t, err, &malErr)
	})

	t.Run("well formed action", func(t *testing.T) {
		tx := mkProposalTx(0, common.Address{}, &common.InfoGovAction{})
		err := conway.UtxoValidateGovActionWellFormedness(tx, 0, nil, pp)
		require.NoError(t, err)
	})

	t.Run(
		"multiple conflicting credentials sort deterministically",
		func(t *testing.T) {
			// CredEpochs is a map, so iteration order is non-deterministic
			// across runs; the reported Credentials must still come back
			// in the same (sorted-by-type-then-hash) order every time.
			credA := common.Credential{
				CredType:   common.CredentialTypeAddrKeyHash,
				Credential: common.Blake2b224Hash([]byte("cc-member-a")),
			}
			credB := common.Credential{
				CredType:   common.CredentialTypeAddrKeyHash,
				Credential: common.Blake2b224Hash([]byte("cc-member-b")),
			}
			action := &common.UpdateCommitteeGovAction{
				Credentials: []common.Credential{credA, credB},
				CredEpochs: map[*common.Credential]uint{
					&credA: 500,
					&credB: 600,
				},
			}
			tx := mkProposalTx(0, common.Address{}, action)
			var wantCreds []common.Credential
			if bytes.Compare(
				credA.Credential.Bytes(),
				credB.Credential.Bytes(),
			) <= 0 {
				wantCreds = []common.Credential{credA, credB}
			} else {
				wantCreds = []common.Credential{credB, credA}
			}
			for range 10 {
				err := conway.UtxoValidateGovActionWellFormedness(
					tx,
					0,
					nil,
					pp,
				)
				var confErr conway.ConflictingCommitteeUpdateError
				require.ErrorAs(t, err, &confErr)
				require.Len(t, confErr.Credentials, 2)
				assert.Equal(t, wantCreds, confErr.Credentials)
			}
		},
	)
}

func TestUtxoValidateHardForkCanFollow(t *testing.T) {
	mkHfTx := func(actionId *common.GovActionId, major, minor uint) *conway.ConwayTransaction {
		action := &common.HardForkInitiationGovAction{
			ActionId: actionId,
		}
		action.ProtocolVersion.Major = major
		action.ProtocolVersion.Minor = minor
		return mkProposalTx(0, common.Address{}, action)
	}

	t.Run("major version bump is allowed", func(t *testing.T) {
		pp := mkConwayPp(9, 0)
		tx := mkHfTx(nil, 10, 0)
		require.NoError(t, conway.UtxoValidateHardForkCanFollow(tx, 0, nil, pp))
	})

	t.Run("minor version bump is allowed", func(t *testing.T) {
		pp := &conway.ConwayProtocolParameters{
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: 9,
				Minor: 0,
			},
		}
		tx := mkHfTx(nil, 9, 1)
		require.NoError(t, conway.UtxoValidateHardForkCanFollow(tx, 0, nil, pp))
	})

	t.Run("skipping a major version is rejected", func(t *testing.T) {
		pp := mkConwayPp(9, 0)
		tx := mkHfTx(nil, 11, 0)
		err := conway.UtxoValidateHardForkCanFollow(tx, 0, nil, pp)
		var hfErr conway.BadHardForkProtocolVersionError
		require.ErrorAs(t, err, &hfErr)
		assert.Equal(t, uint(11), hfErr.Supplied.Major)
		assert.Equal(t, uint(9), hfErr.Expected.Major)
	})

	t.Run("decreasing version is rejected", func(t *testing.T) {
		pp := mkConwayPp(10, 0)
		tx := mkHfTx(nil, 9, 0)
		err := conway.UtxoValidateHardForkCanFollow(tx, 0, nil, pp)
		var hfErr conway.BadHardForkProtocolVersionError
		require.ErrorAs(t, err, &hfErr)
	})

	t.Run(
		"a version jump is rejected even with an ancestor",
		func(t *testing.T) {
			pp := mkConwayPp(9, 0)
			ancestor := common.GovActionId{
				TransactionId: common.Blake2b256{0x01},
			}
			// preceedingHardFork compares against the enacted protocol
			// version once the proposed major version is more than one
			// above it, whatever the proposal's predecessor is.
			tx := mkHfTx(&ancestor, 99, 0)
			err := conway.UtxoValidateHardForkCanFollow(tx, 0, nil, pp)
			var hfErr conway.BadHardForkProtocolVersionError
			require.ErrorAs(t, err, &hfErr)
			assert.Equal(t, uint(99), hfErr.Supplied.Major)
			assert.Equal(t, uint(9), hfErr.Expected.Major)
		},
	)
}

func TestUtxoValidateProposalAncestry(t *testing.T) {
	pp := mkConwayPp(common.ProtocolVersionConway, 0)
	hfAncestorId := common.GovActionId{TransactionId: common.Blake2b256{0x01}}
	constitutionAncestorId := common.GovActionId{
		TransactionId: common.Blake2b256{0x02},
	}

	govActions := map[string]*common.GovActionState{
		govActionKey(hfAncestorId): {
			ActionId:   hfAncestorId,
			ActionType: common.GovActionTypeHardForkInitiation,
		},
		govActionKey(constitutionAncestorId): {
			ActionId:   constitutionAncestorId,
			ActionType: common.GovActionTypeNewConstitution,
		},
	}
	ls := mockledger.NewLedgerStateBuilder().WithGovActions(govActions).Build()

	t.Run("no ancestor is fine", func(t *testing.T) {
		action := &common.HardForkInitiationGovAction{}
		tx := mkProposalTx(0, common.Address{}, action)
		require.NoError(t, conway.UtxoValidateProposalAncestry(tx, 0, ls, pp))
	})

	t.Run("existing ancestor with matching purpose", func(t *testing.T) {
		action := &common.HardForkInitiationGovAction{ActionId: &hfAncestorId}
		tx := mkProposalTx(0, common.Address{}, action)
		require.NoError(t, conway.UtxoValidateProposalAncestry(tx, 0, ls, pp))
	})

	t.Run("nonexistent ancestor", func(t *testing.T) {
		missing := common.GovActionId{TransactionId: common.Blake2b256{0xFF}}
		action := &common.HardForkInitiationGovAction{ActionId: &missing}
		tx := mkProposalTx(0, common.Address{}, action)
		err := conway.UtxoValidateProposalAncestry(tx, 0, ls, pp)
		var ancErr conway.InvalidGovActionAncestorError
		require.ErrorAs(t, err, &ancErr)
	})

	t.Run("ancestor with mismatched purpose", func(t *testing.T) {
		// HardForkInitiation referencing a NewConstitution ancestor is
		// invalid: the ancestor belongs to a different purpose chain.
		action := &common.HardForkInitiationGovAction{
			ActionId: &constitutionAncestorId,
		}
		tx := mkProposalTx(0, common.Address{}, action)
		err := conway.UtxoValidateProposalAncestry(tx, 0, ls, pp)
		var ancErr conway.InvalidGovActionAncestorError
		require.ErrorAs(t, err, &ancErr)
	})
}

func mkVoteTx(
	voter common.Voter,
	actionId common.GovActionId,
	vote uint8,
) *conway.ConwayTransaction {
	tx := &conway.ConwayTransaction{}
	v := voter
	aid := actionId
	tx.Body.TxVotingProcedures = common.VotingProcedures{
		&v: {
			&aid: common.VotingProcedure{Vote: vote},
		},
	}
	return tx
}

// mkNilActionIdVoteTx builds a transaction with a voting procedure whose
// governance action id key is nil, simulating a malformed vote entry.
func mkNilActionIdVoteTx(
	voter common.Voter,
	vote uint8,
) *conway.ConwayTransaction {
	tx := &conway.ConwayTransaction{}
	v := voter
	tx.Body.TxVotingProcedures = common.VotingProcedures{
		&v: {
			nil: common.VotingProcedure{Vote: vote},
		},
	}
	return tx
}

func TestUtxoValidateUnknownGovActionIds(t *testing.T) {
	knownId := common.GovActionId{TransactionId: common.Blake2b256{0x01}}
	unknownId := common.GovActionId{TransactionId: common.Blake2b256{0xFF}}
	govActions := map[string]*common.GovActionState{
		govActionKey(knownId): {
			ActionId:   knownId,
			ActionType: common.GovActionTypeInfo,
		},
	}
	ls := mockledger.NewLedgerStateBuilder().WithGovActions(govActions).Build()
	pp := mkConwayPp(common.ProtocolVersionConway, 0)
	voter := common.Voter{
		Type: common.VoterTypeDRepKeyHash,
		Hash: common.Blake2b224{0x01},
	}

	t.Run("known action id", func(t *testing.T) {
		tx := mkVoteTx(voter, knownId, common.GovVoteYes)
		require.NoError(
			t,
			conway.UtxoValidateUnknownGovActionIds(tx, 0, ls, pp),
		)
	})

	t.Run("unknown action id", func(t *testing.T) {
		tx := mkVoteTx(voter, unknownId, common.GovVoteYes)
		err := conway.UtxoValidateUnknownGovActionIds(tx, 0, ls, pp)
		var unkErr conway.UnknownGovActionIdError
		require.ErrorAs(t, err, &unkErr)
		require.Len(t, unkErr.ActionIds, 1)
		assert.Equal(t, unknownId, unkErr.ActionIds[0])
	})

	t.Run("nil action id errors instead of being skipped", func(t *testing.T) {
		tx := mkNilActionIdVoteTx(voter, common.GovVoteYes)
		err := conway.UtxoValidateUnknownGovActionIds(tx, 0, ls, pp)
		var unkErr conway.UnknownGovActionIdError
		require.ErrorAs(t, err, &unkErr)
		require.Len(t, unkErr.ActionIds, 1)
		assert.Equal(t, common.GovActionId{}, unkErr.ActionIds[0])
	})

	t.Run(
		"multiple unknown action ids sort deterministically",
		func(t *testing.T) {
			// VotingProcedures is keyed by voter/action-id pointers with
			// map iteration underneath; the reported ActionIds must still
			// come back in the same (sorted-by-bytes) order every time.
			unknownIdA := common.GovActionId{
				TransactionId: common.Blake2b256{0xAA},
			}
			unknownIdB := common.GovActionId{
				TransactionId: common.Blake2b256{0xBB},
			}
			v := voter
			tx := &conway.ConwayTransaction{}
			tx.Body.TxVotingProcedures = common.VotingProcedures{
				&v: {
					&unknownIdA: common.VotingProcedure{
						Vote: common.GovVoteYes,
					},
					&unknownIdB: common.VotingProcedure{
						Vote: common.GovVoteYes,
					},
				},
			}
			wantIds := []common.GovActionId{unknownIdA, unknownIdB}
			for range 10 {
				err := conway.UtxoValidateUnknownGovActionIds(tx, 0, ls, pp)
				var unkErr conway.UnknownGovActionIdError
				require.ErrorAs(t, err, &unkErr)
				require.Len(t, unkErr.ActionIds, 2)
				assert.Equal(t, wantIds, unkErr.ActionIds)
			}
		},
	)
}

func TestUtxoValidateUnknownVoters(t *testing.T) {
	pp := mkConwayPp(common.ProtocolVersionConway, 0)
	actionId := common.GovActionId{TransactionId: common.Blake2b256{0x01}}

	drepHash := common.Blake2b224{0x10}
	poolHash := common.Blake2b224{0x20}
	ccHotHash := common.Blake2b224{0x30}

	ls := mockledger.NewLedgerStateBuilder().
		WithDRepRegistrations([]common.DRepRegistration{
			{Credential: drepHash},
		}).
		WithPools([]*common.PoolRegistrationCertificate{
			{Operator: common.PoolKeyHash(poolHash)},
		}).
		WithCommitteeMembers([]common.CommitteeMember{
			{ColdKey: common.Blake2b224{0x31}, HotKey: &ccHotHash},
		}).
		Build()

	t.Run("registered DRep", func(t *testing.T) {
		voter := common.Voter{Type: common.VoterTypeDRepKeyHash, Hash: drepHash}
		tx := mkVoteTx(voter, actionId, common.GovVoteYes)
		require.NoError(t, conway.UtxoValidateUnknownVoters(tx, 0, ls, pp))
	})

	t.Run("unregistered DRep", func(t *testing.T) {
		voter := common.Voter{
			Type: common.VoterTypeDRepKeyHash,
			Hash: common.Blake2b224{0x99},
		}
		tx := mkVoteTx(voter, actionId, common.GovVoteYes)
		err := conway.UtxoValidateUnknownVoters(tx, 0, ls, pp)
		var unkErr conway.UnknownVoterError
		require.ErrorAs(t, err, &unkErr)
	})

	t.Run("registered pool", func(t *testing.T) {
		voter := common.Voter{
			Type: common.VoterTypeStakingPoolKeyHash,
			Hash: poolHash,
		}
		tx := mkVoteTx(voter, actionId, common.GovVoteYes)
		require.NoError(t, conway.UtxoValidateUnknownVoters(tx, 0, ls, pp))
	})

	t.Run("unregistered pool", func(t *testing.T) {
		voter := common.Voter{
			Type: common.VoterTypeStakingPoolKeyHash,
			Hash: common.Blake2b224{0x99},
		}
		tx := mkVoteTx(voter, actionId, common.GovVoteYes)
		err := conway.UtxoValidateUnknownVoters(tx, 0, ls, pp)
		var unkErr conway.UnknownVoterError
		require.ErrorAs(t, err, &unkErr)
	})

	t.Run("authorized committee hot key", func(t *testing.T) {
		voter := common.Voter{
			Type: common.VoterTypeConstitutionalCommitteeHotKeyHash,
			Hash: ccHotHash,
		}
		tx := mkVoteTx(voter, actionId, common.GovVoteYes)
		require.NoError(t, conway.UtxoValidateUnknownVoters(tx, 0, ls, pp))
	})

	t.Run("unauthorized committee hot key", func(t *testing.T) {
		voter := common.Voter{
			Type: common.VoterTypeConstitutionalCommitteeHotKeyHash,
			Hash: common.Blake2b224{0x99},
		}
		tx := mkVoteTx(voter, actionId, common.GovVoteYes)
		err := conway.UtxoValidateUnknownVoters(tx, 0, ls, pp)
		var unkErr conway.UnknownVoterError
		require.ErrorAs(t, err, &unkErr)
	})

	t.Run("empty committee state is treated as unmodeled", func(t *testing.T) {
		emptyLs := mockledger.NewLedgerStateBuilder().Build()
		voter := common.Voter{
			Type: common.VoterTypeConstitutionalCommitteeHotKeyHash,
			Hash: common.Blake2b224{0x99},
		}
		tx := mkVoteTx(voter, actionId, common.GovVoteYes)
		require.NoError(t, conway.UtxoValidateUnknownVoters(tx, 0, emptyLs, pp))
	})

	t.Run("out-of-range voter type is rejected", func(t *testing.T) {
		// Voter.Type is decoded from CBOR with no range check, so a value
		// outside the five defined VoterType* constants (0-4) is possible
		// on the wire. It must be rejected here, since no other rule in
		// UtxoValidationRules validates voter type.
		voter := common.Voter{
			Type: 200,
			Hash: common.Blake2b224{0x01},
		}
		tx := mkVoteTx(voter, actionId, common.GovVoteYes)
		err := conway.UtxoValidateUnknownVoters(tx, 0, ls, pp)
		var unkErr conway.UnknownVoterError
		require.ErrorAs(t, err, &unkErr)
	})
}

func TestUtxoValidateVotingOnExpiredGovAction(t *testing.T) {
	pp := mkConwayPp(common.ProtocolVersionConway, 0)
	actionId := common.GovActionId{TransactionId: common.Blake2b256{0x01}}
	govActions := map[string]*common.GovActionState{
		govActionKey(actionId): {
			ActionId:   actionId,
			ActionType: common.GovActionTypeInfo,
			ExpirySlot: 1000,
		},
	}
	ls := mockledger.NewLedgerStateBuilder().WithGovActions(govActions).Build()
	voter := common.Voter{
		Type: common.VoterTypeDRepKeyHash,
		Hash: common.Blake2b224{0x01},
	}

	t.Run("vote before expiry", func(t *testing.T) {
		tx := mkVoteTx(voter, actionId, common.GovVoteYes)
		require.NoError(
			t,
			conway.UtxoValidateVotingOnExpiredGovAction(tx, 999, ls, pp),
		)
	})

	t.Run("vote at expiry slot", func(t *testing.T) {
		tx := mkVoteTx(voter, actionId, common.GovVoteYes)
		require.NoError(
			t,
			conway.UtxoValidateVotingOnExpiredGovAction(tx, 1000, ls, pp),
		)
	})

	t.Run("vote after expiry", func(t *testing.T) {
		tx := mkVoteTx(voter, actionId, common.GovVoteYes)
		err := conway.UtxoValidateVotingOnExpiredGovAction(tx, 1001, ls, pp)
		var expErr conway.VotingOnExpiredGovActionError
		require.ErrorAs(t, err, &expErr)
		assert.Equal(t, uint64(1000), expErr.ExpirySlot)
		assert.Equal(t, uint64(1001), expErr.Slot)
	})

	t.Run("nil action id errors instead of being skipped", func(t *testing.T) {
		tx := mkNilActionIdVoteTx(voter, common.GovVoteYes)
		err := conway.UtxoValidateVotingOnExpiredGovAction(tx, 0, ls, pp)
		var unkErr conway.UnknownGovActionIdError
		require.ErrorAs(t, err, &unkErr)
	})

	// A LedgerState implementation that does not model gov-action expiry
	// leaves ExpirySlot at its zero value. That must be treated as
	// "expiry not modeled" rather than "expired at slot 0", which would
	// otherwise reject every vote at any slot > 0 (the production bug this
	// case pins).
	t.Run("unset ExpirySlot is treated as not modeled", func(t *testing.T) {
		unexpiringActionId := common.GovActionId{
			TransactionId: common.Blake2b256{0x02},
		}
		unexpiringGovActions := map[string]*common.GovActionState{
			govActionKey(unexpiringActionId): {
				ActionId:   unexpiringActionId,
				ActionType: common.GovActionTypeInfo,
			},
		}
		unexpiringLs := mockledger.NewLedgerStateBuilder().
			WithGovActions(unexpiringGovActions).
			Build()
		tx := mkVoteTx(voter, unexpiringActionId, common.GovVoteYes)
		require.NoError(
			t,
			conway.UtxoValidateVotingOnExpiredGovAction(
				tx,
				1_000_000,
				unexpiringLs,
				pp,
			),
		)
	})
}

// NOTE: this deliberately does not add a PV11 (ProtocolVersionVanRossem)
// case alongside the PV10 ("PV10 lifts the bootstrap voting restriction")
// case below: isInConwayBootstrapPhase only distinguishes PV9 from
// everything else, so PV10 and PV11 behave identically here and a PV11
// case would add no real coverage.
func TestUtxoValidateBootstrapVotingRestrictions(t *testing.T) {
	infoId := common.GovActionId{TransactionId: common.Blake2b256{0x01}}
	pparamId := common.GovActionId{TransactionId: common.Blake2b256{0x02}}
	treasuryId := common.GovActionId{TransactionId: common.Blake2b256{0x03}}
	govActions := map[string]*common.GovActionState{
		govActionKey(infoId): {
			ActionId:   infoId,
			ActionType: common.GovActionTypeInfo,
		},
		govActionKey(pparamId): {
			ActionId:   pparamId,
			ActionType: common.GovActionTypeParameterChange,
		},
		govActionKey(treasuryId): {
			ActionId:   treasuryId,
			ActionType: common.GovActionTypeTreasuryWithdrawal,
		},
	}
	ls := mockledger.NewLedgerStateBuilder().WithGovActions(govActions).Build()

	pv9 := mkConwayPp(common.ProtocolVersionConway, 0)
	pv10 := mkConwayPp(common.ProtocolVersionPlomin, 0)

	drepVoter := common.Voter{
		Type: common.VoterTypeDRepKeyHash,
		Hash: common.Blake2b224{0x01},
	}
	ccVoter := common.Voter{
		Type: common.VoterTypeConstitutionalCommitteeHotKeyHash,
		Hash: common.Blake2b224{0x02},
	}

	t.Run("PV9 DRep can vote on InfoAction", func(t *testing.T) {
		tx := mkVoteTx(drepVoter, infoId, common.GovVoteYes)
		require.NoError(
			t,
			conway.UtxoValidateBootstrapVotingRestrictions(tx, 0, ls, pv9),
		)
	})

	t.Run("PV9 DRep cannot vote on ParameterChange", func(t *testing.T) {
		tx := mkVoteTx(drepVoter, pparamId, common.GovVoteYes)
		err := conway.UtxoValidateBootstrapVotingRestrictions(tx, 0, ls, pv9)
		var bootErr conway.BootstrapVotingRestrictionError
		require.ErrorAs(t, err, &bootErr)
	})

	t.Run("PV9 CC can vote on ParameterChange", func(t *testing.T) {
		tx := mkVoteTx(ccVoter, pparamId, common.GovVoteYes)
		require.NoError(
			t,
			conway.UtxoValidateBootstrapVotingRestrictions(tx, 0, ls, pv9),
		)
	})

	t.Run("PV9 CC cannot vote on TreasuryWithdrawal", func(t *testing.T) {
		tx := mkVoteTx(ccVoter, treasuryId, common.GovVoteYes)
		err := conway.UtxoValidateBootstrapVotingRestrictions(tx, 0, ls, pv9)
		var bootErr conway.BootstrapVotingRestrictionError
		require.ErrorAs(t, err, &bootErr)
	})

	t.Run("PV10 lifts the bootstrap voting restriction", func(t *testing.T) {
		tx := mkVoteTx(drepVoter, treasuryId, common.GovVoteYes)
		require.NoError(
			t,
			conway.UtxoValidateBootstrapVotingRestrictions(tx, 0, ls, pv10),
		)
	})

	t.Run("nil action id errors instead of being skipped", func(t *testing.T) {
		tx := mkNilActionIdVoteTx(drepVoter, common.GovVoteYes)
		err := conway.UtxoValidateBootstrapVotingRestrictions(tx, 0, ls, pv9)
		var bootErr conway.BootstrapVotingRestrictionError
		require.ErrorAs(t, err, &bootErr)
		assert.Equal(t, common.GovActionId{}, bootErr.ActionId)
	})
}

func TestUtxoValidateStakePoolVotingRestrictions(t *testing.T) {
	constitutionId := common.GovActionId{TransactionId: common.Blake2b256{0x01}}
	treasuryId := common.GovActionId{TransactionId: common.Blake2b256{0x02}}
	noConfidenceId := common.GovActionId{TransactionId: common.Blake2b256{0x03}}
	paramChangeId := common.GovActionId{TransactionId: common.Blake2b256{0x04}}
	govActions := map[string]*common.GovActionState{
		govActionKey(constitutionId): {
			ActionId:   constitutionId,
			ActionType: common.GovActionTypeNewConstitution,
		},
		govActionKey(treasuryId): {
			ActionId:   treasuryId,
			ActionType: common.GovActionTypeTreasuryWithdrawal,
		},
		govActionKey(noConfidenceId): {
			ActionId:   noConfidenceId,
			ActionType: common.GovActionTypeNoConfidence,
		},
		govActionKey(paramChangeId): {
			ActionId:   paramChangeId,
			ActionType: common.GovActionTypeParameterChange,
		},
	}
	ls := mockledger.NewLedgerStateBuilder().WithGovActions(govActions).Build()
	pp := mkConwayPp(common.ProtocolVersionConway, 0)
	spoVoter := common.Voter{
		Type: common.VoterTypeStakingPoolKeyHash,
		Hash: common.Blake2b224{0x01},
	}

	t.Run("SPO cannot vote on NewConstitution", func(t *testing.T) {
		tx := mkVoteTx(spoVoter, constitutionId, common.GovVoteYes)
		err := conway.UtxoValidateStakePoolVotingRestrictions(tx, 0, ls, pp)
		var spoErr conway.StakePoolVotingRestrictionError
		require.ErrorAs(t, err, &spoErr)
	})

	t.Run("SPO cannot vote on TreasuryWithdrawal", func(t *testing.T) {
		tx := mkVoteTx(spoVoter, treasuryId, common.GovVoteYes)
		err := conway.UtxoValidateStakePoolVotingRestrictions(tx, 0, ls, pp)
		var spoErr conway.StakePoolVotingRestrictionError
		require.ErrorAs(t, err, &spoErr)
	})

	t.Run("SPO can vote on NoConfidence", func(t *testing.T) {
		tx := mkVoteTx(spoVoter, noConfidenceId, common.GovVoteYes)
		require.NoError(
			t,
			conway.UtxoValidateStakePoolVotingRestrictions(tx, 0, ls, pp),
		)
	})

	// The security-group restriction on SPO votes over ParameterChange
	// needs the proposed parameter update. This ledger state records only
	// the action type, so the restriction stays unenforced for it; see
	// TestUtxoValidateStakePoolVotingRestrictionsParameterChange for the
	// enforced cases.
	t.Run(
		"SPO vote on an opaque ParameterChange is not classified",
		func(t *testing.T) {
			tx := mkVoteTx(spoVoter, paramChangeId, common.GovVoteYes)
			require.NoError(
				t,
				conway.UtxoValidateStakePoolVotingRestrictions(tx, 0, ls, pp),
			)
		},
	)

	t.Run("nil action id errors instead of being skipped", func(t *testing.T) {
		tx := mkNilActionIdVoteTx(spoVoter, common.GovVoteYes)
		err := conway.UtxoValidateStakePoolVotingRestrictions(tx, 0, ls, pp)
		var spoErr conway.StakePoolVotingRestrictionError
		require.ErrorAs(t, err, &spoErr)
		assert.Equal(t, common.GovActionId{}, spoErr.ActionId)
	})
}

// The tests below close https://github.com/blinklabs-io/gouroboros/issues/1986:
// hard-fork protocol version succession against a referenced ancestor,
// purpose-root ancestry, and the Conway security-group restriction on stake
// pool votes over parameter changes.

func mkHfAction(
	actionId *common.GovActionId,
	major, minor uint,
) *common.HardForkInitiationGovAction {
	action := &common.HardForkInitiationGovAction{ActionId: actionId}
	action.ProtocolVersion.Major = major
	action.ProtocolVersion.Minor = minor
	return action
}

// mkProposalsTx builds a transaction carrying several proposal procedures in
// order, so a later proposal can reference an earlier one in the same
// transaction by its (txid, index) governance action id.
func mkProposalsTx(actions ...common.GovAction) *conway.ConwayTransaction {
	tx := &conway.ConwayTransaction{}
	for _, action := range actions {
		tx.Body.TxProposalProcedures = append(
			tx.Body.TxProposalProcedures,
			conway.ConwayProposalProcedure{
				PPGovAction: conway.ConwayGovAction{Action: action},
			},
		)
	}
	return tx
}

// rootedLedgerState decorates a ledger state with the optional
// GovPurposeRootsState capability.
type rootedLedgerState struct {
	common.LedgerState
	roots *common.GovPurposeRoots
	err   error
}

func (l rootedLedgerState) GovPurposeRoots() (*common.GovPurposeRoots, error) {
	return l.roots, l.err
}

func TestUtxoValidateHardForkCanFollowWithAncestor(t *testing.T) {
	// Current protocol version is 9.0 throughout.
	pp := mkConwayPp(9, 0)
	pendingId := common.GovActionId{TransactionId: common.Blake2b256{0x11}}
	opaqueId := common.GovActionId{TransactionId: common.Blake2b256{0x12}}
	govActions := map[string]*common.GovActionState{
		// A pending hard-fork proposal for 10.0 whose contents the ledger
		// state exposes.
		govActionKey(pendingId): {
			ActionId:   pendingId,
			ActionType: common.GovActionTypeHardForkInitiation,
			Action:     mkHfAction(nil, 10, 0),
		},
		// The same proposal from a state provider that only records the
		// action type.
		govActionKey(opaqueId): {
			ActionId:   opaqueId,
			ActionType: common.GovActionTypeHardForkInitiation,
		},
	}
	ls := mockledger.NewLedgerStateBuilder().WithGovActions(govActions).Build()

	t.Run(
		"minor bump on the ancestor's version is allowed",
		func(t *testing.T) {
			tx := mkProposalsTx(mkHfAction(&pendingId, 10, 1))
			require.NoError(
				t,
				conway.UtxoValidateHardForkCanFollow(tx, 0, ls, pp),
			)
		},
	)

	t.Run("gap from the ancestor's version is rejected", func(t *testing.T) {
		tx := mkProposalsTx(mkHfAction(&pendingId, 10, 5))
		err := conway.UtxoValidateHardForkCanFollow(tx, 0, ls, pp)
		var hfErr conway.BadHardForkProtocolVersionError
		require.ErrorAs(t, err, &hfErr)
		assert.Equal(t, uint(10), hfErr.Supplied.Major)
		assert.Equal(t, uint(5), hfErr.Supplied.Minor)
		assert.Equal(t, uint(10), hfErr.Expected.Major)
		assert.Equal(t, uint(0), hfErr.Expected.Minor)
	})

	// preceedingHardFork compares against the *current* protocol version,
	// not the ancestor's, once the proposed major version is more than one
	// above the current one. A chain of pending proposals therefore cannot
	// be used to jump ahead.
	t.Run(
		"major version too high is rejected via an ancestor",
		func(t *testing.T) {
			tx := mkProposalsTx(mkHfAction(&pendingId, 11, 0))
			err := conway.UtxoValidateHardForkCanFollow(tx, 0, ls, pp)
			var hfErr conway.BadHardForkProtocolVersionError
			require.ErrorAs(t, err, &hfErr)
			assert.Equal(t, uint(11), hfErr.Supplied.Major)
			assert.Equal(t, uint(9), hfErr.Expected.Major)
		},
	)

	t.Run("ancestor in the same transaction", func(t *testing.T) {
		first := mkHfAction(nil, 10, 0)
		tx := mkProposalsTx(first, nil)
		firstId := common.GovActionId{
			TransactionId: tx.Hash(),
			GovActionIdx:  0,
		}
		// Rebuild with the second proposal pointing at the first.
		tx = mkProposalsTx(first, mkHfAction(&firstId, 10, 1))
		require.NoError(
			t,
			conway.UtxoValidateHardForkCanFollow(tx, 0, ls, pp),
		)

		bad := mkProposalsTx(first, mkHfAction(&firstId, 10, 4))
		err := conway.UtxoValidateHardForkCanFollow(bad, 0, ls, pp)
		var hfErr conway.BadHardForkProtocolVersionError
		require.ErrorAs(t, err, &hfErr)
		assert.Equal(t, uint(10), hfErr.Expected.Major)
		assert.Equal(t, uint(0), hfErr.Expected.Minor)
	})

	// A state provider that does not expose the ancestor's proposed
	// protocol version cannot support the comparison; the check is skipped
	// rather than run against the wrong reference version.
	t.Run("ancestor without contents defers the check", func(t *testing.T) {
		tx := mkProposalsTx(mkHfAction(&opaqueId, 10, 7))
		require.NoError(
			t,
			conway.UtxoValidateHardForkCanFollow(tx, 0, ls, pp),
		)
	})
}

func TestUtxoValidateProposalAncestryPurposeRoot(t *testing.T) {
	pp := mkConwayPp(common.ProtocolVersionConway, 0)
	rootId := common.GovActionId{TransactionId: common.Blake2b256{0x21}}
	pendingId := common.GovActionId{TransactionId: common.Blake2b256{0x22}}
	expiredId := common.GovActionId{TransactionId: common.Blake2b256{0x23}}
	govActions := map[string]*common.GovActionState{
		govActionKey(rootId): {
			ActionId:   rootId,
			ActionType: common.GovActionTypeHardForkInitiation,
		},
		govActionKey(pendingId): {
			ActionId:   pendingId,
			ActionType: common.GovActionTypeHardForkInitiation,
			ExpirySlot: 100,
		},
		govActionKey(expiredId): {
			ActionId:   expiredId,
			ActionType: common.GovActionTypeHardForkInitiation,
			ExpirySlot: 10,
		},
	}
	base := mockledger.NewLedgerStateBuilder().
		WithGovActions(govActions).
		Build()
	withRoot := rootedLedgerState{
		LedgerState: base,
		roots:       &common.GovPurposeRoots{HardFork: &rootId},
	}
	emptyRoots := rootedLedgerState{
		LedgerState: base,
		roots:       &common.GovPurposeRoots{},
	}

	t.Run("predecessor is the purpose root", func(t *testing.T) {
		tx := mkProposalsTx(mkHfAction(&rootId, 10, 0))
		require.NoError(
			t,
			conway.UtxoValidateProposalAncestry(tx, 50, withRoot, pp),
		)
	})

	t.Run("predecessor is a pending proposal", func(t *testing.T) {
		tx := mkProposalsTx(mkHfAction(&pendingId, 10, 0))
		require.NoError(
			t,
			conway.UtxoValidateProposalAncestry(tx, 50, withRoot, pp),
		)
	})

	t.Run("no predecessor while a root exists is rejected", func(t *testing.T) {
		tx := mkProposalsTx(mkHfAction(nil, 10, 0))
		err := conway.UtxoValidateProposalAncestry(tx, 50, withRoot, pp)
		var ancErr conway.InvalidGovActionAncestorError
		require.ErrorAs(t, err, &ancErr)
		assert.Equal(t, rootId, ancErr.ActionId)
	})

	t.Run("no predecessor with an empty root is allowed", func(t *testing.T) {
		tx := mkProposalsTx(mkHfAction(nil, 10, 0))
		require.NoError(
			t,
			conway.UtxoValidateProposalAncestry(tx, 50, emptyRoots, pp),
		)
	})

	t.Run("expired predecessor is rejected", func(t *testing.T) {
		tx := mkProposalsTx(mkHfAction(&expiredId, 10, 0))
		err := conway.UtxoValidateProposalAncestry(tx, 50, withRoot, pp)
		var ancErr conway.InvalidGovActionAncestorError
		require.ErrorAs(t, err, &ancErr)
		assert.Equal(t, expiredId, ancErr.ActionId)
	})

	t.Run("predecessor in the same transaction", func(t *testing.T) {
		first := mkHfAction(&rootId, 10, 0)
		tx := mkProposalsTx(first, nil)
		firstId := common.GovActionId{
			TransactionId: tx.Hash(),
			GovActionIdx:  0,
		}
		tx = mkProposalsTx(first, mkHfAction(&firstId, 10, 1))
		require.NoError(
			t,
			conway.UtxoValidateProposalAncestry(tx, 50, withRoot, pp),
		)
	})

	// Only an earlier proposal of the same transaction is a candidate
	// predecessor: cardano-ledger folds the proposals in order, so a
	// proposal cannot name itself or a later proposal.
	t.Run(
		"self reference in the same transaction is rejected",
		func(t *testing.T) {
			tx := mkProposalsTx(mkHfAction(&rootId, 10, 0))
			selfId := common.GovActionId{
				TransactionId: tx.Hash(),
				GovActionIdx:  0,
			}
			tx = mkProposalsTx(mkHfAction(&selfId, 10, 0))
			err := conway.UtxoValidateProposalAncestry(tx, 50, withRoot, pp)
			var ancErr conway.InvalidGovActionAncestorError
			require.ErrorAs(t, err, &ancErr)
			assert.Equal(t, selfId, ancErr.ActionId)
		},
	)

	// A ledger state that cannot report purpose roots keeps the looser
	// existence-and-purpose behavior, so a syncing node backed by such a
	// state is not wedged by the stricter rule.
	t.Run("no roots capability keeps existence checks", func(t *testing.T) {
		tx := mkProposalsTx(mkHfAction(nil, 10, 0))
		require.NoError(
			t,
			conway.UtxoValidateProposalAncestry(tx, 50, base, pp),
		)
		missing := common.GovActionId{TransactionId: common.Blake2b256{0xFE}}
		bad := mkProposalsTx(mkHfAction(&missing, 10, 0))
		err := conway.UtxoValidateProposalAncestry(bad, 50, base, pp)
		var ancErr conway.InvalidGovActionAncestorError
		require.ErrorAs(t, err, &ancErr)
	})
}

func TestUtxoValidateStakePoolVotingRestrictionsParameterChange(t *testing.T) {
	pp := mkConwayPp(common.ProtocolVersionPlomin, 0)
	securityId := common.GovActionId{TransactionId: common.Blake2b256{0x31}}
	nonSecurityId := common.GovActionId{TransactionId: common.Blake2b256{0x32}}
	emptyId := common.GovActionId{TransactionId: common.Blake2b256{0x33}}
	maxBlockBodySize := uint(90112)
	keyDeposit := uint(2_000_000)

	securityUpdate := conway.ConwayProtocolParameterUpdate{
		MaxBlockBodySize: &maxBlockBodySize,
	}
	nonSecurityUpdate := conway.ConwayProtocolParameterUpdate{
		KeyDeposit: &keyDeposit,
	}
	govActions := map[string]*common.GovActionState{
		govActionKey(securityId): {
			ActionId:   securityId,
			ActionType: common.GovActionTypeParameterChange,
			Action: &conway.ConwayParameterChangeGovAction{
				ParamUpdate: securityUpdate,
			},
		},
		govActionKey(nonSecurityId): {
			ActionId:   nonSecurityId,
			ActionType: common.GovActionTypeParameterChange,
			Action: &conway.ConwayParameterChangeGovAction{
				ParamUpdate: nonSecurityUpdate,
			},
		},
		govActionKey(emptyId): {
			ActionId:   emptyId,
			ActionType: common.GovActionTypeParameterChange,
			Action:     &conway.ConwayParameterChangeGovAction{},
		},
	}
	ls := mockledger.NewLedgerStateBuilder().WithGovActions(govActions).Build()
	spoVoter := common.Voter{
		Type: common.VoterTypeStakingPoolKeyHash,
		Hash: common.Blake2b224{0x01},
	}
	drepVoter := common.Voter{
		Type: common.VoterTypeDRepKeyHash,
		Hash: common.Blake2b224{0x02},
	}

	t.Run("SPO may vote on a security group change", func(t *testing.T) {
		tx := mkVoteTx(spoVoter, securityId, common.GovVoteYes)
		require.NoError(
			t,
			conway.UtxoValidateStakePoolVotingRestrictions(tx, 0, ls, pp),
		)
	})

	t.Run("SPO may not vote on a non-security change", func(t *testing.T) {
		tx := mkVoteTx(spoVoter, nonSecurityId, common.GovVoteYes)
		err := conway.UtxoValidateStakePoolVotingRestrictions(tx, 0, ls, pp)
		var spoErr conway.StakePoolVotingRestrictionError
		require.ErrorAs(t, err, &spoErr)
		assert.Equal(t, nonSecurityId, spoErr.ActionId)
	})

	t.Run("SPO may not vote on an empty parameter change", func(t *testing.T) {
		tx := mkVoteTx(spoVoter, emptyId, common.GovVoteYes)
		err := conway.UtxoValidateStakePoolVotingRestrictions(tx, 0, ls, pp)
		var spoErr conway.StakePoolVotingRestrictionError
		require.ErrorAs(t, err, &spoErr)
	})

	t.Run(
		"DRep vote on a non-security change is unaffected",
		func(t *testing.T) {
			tx := mkVoteTx(drepVoter, nonSecurityId, common.GovVoteYes)
			require.NoError(
				t,
				conway.UtxoValidateStakePoolVotingRestrictions(tx, 0, ls, pp),
			)
		},
	)

	// A vote may refer to an action proposed by its own transaction, whose
	// contents are then available whatever the ledger state records.
	t.Run("action proposed by the voting transaction", func(t *testing.T) {
		mkTx := func(
			update conway.ConwayProtocolParameterUpdate,
		) *conway.ConwayTransaction {
			tx := mkProposalsTx(&conway.ConwayParameterChangeGovAction{
				ParamUpdate: update,
			})
			actionId := common.GovActionId{
				TransactionId: tx.Hash(),
				GovActionIdx:  0,
			}
			v := spoVoter
			tx.Body.TxVotingProcedures = common.VotingProcedures{
				&v: {
					&actionId: common.VotingProcedure{
						Vote: common.GovVoteYes,
					},
				},
			}
			return tx
		}
		require.NoError(
			t,
			conway.UtxoValidateStakePoolVotingRestrictions(
				mkTx(securityUpdate), 0, ls, pp,
			),
		)
		err := conway.UtxoValidateStakePoolVotingRestrictions(
			mkTx(nonSecurityUpdate), 0, ls, pp,
		)
		var spoErr conway.StakePoolVotingRestrictionError
		require.ErrorAs(t, err, &spoErr)
	})
}
