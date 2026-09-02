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
	"errors"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// conwayComposedRulesForError returns every composed rule that reports the
// matched error. More than one rule can report the same failure: certificate
// state is checked by both the delegation rule and the deposit rule.
func conwayComposedRulesForError(
	t *testing.T,
	tx common.Transaction,
	ls common.LedgerState,
	pp common.ProtocolParameters,
	match func(error) bool,
) []common.UtxoValidationRuleFunc {
	t.Helper()
	var matches []common.UtxoValidationRuleFunc
	for _, rule := range conway.UtxoValidationRules {
		if match(rule(tx, 0, ls, pp)) {
			matches = append(matches, rule)
		}
	}
	require.NotEmpty(
		t,
		matches,
		"expected a composed validation rule to match",
	)
	return matches
}

func TestPhase2InvalidSkipsCertificateAndGovernanceRules(t *testing.T) {
	credential := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: common.Blake2b224{0x01},
	}
	voter := &common.Voter{
		Type: common.VoterTypeDRepKeyHash,
		Hash: common.Blake2b224{0x02},
	}
	actionId := &common.GovActionId{
		TransactionId: common.Blake2b256{0x03},
		GovActionIdx:  1,
	}
	body := conway.ConwayTransactionBody{
		TxCertificates: []common.CertificateWrapper{{
			Type: uint(common.CertificateTypeStakeRegistration),
			Certificate: &common.StakeRegistrationCertificate{
				CertType:        uint(common.CertificateTypeStakeRegistration),
				StakeCredential: credential,
			},
		}},
		TxVotingProcedures: common.VotingProcedures{
			voter: {
				actionId: {Vote: common.GovVoteYes},
			},
		},
	}
	invalidTx := &conway.ConwayTransaction{Body: body, TxIsValid: false}
	validTx := &conway.ConwayTransaction{Body: body, TxIsValid: true}
	ls := mockledger.NewLedgerStateBuilder().
		WithStakeCredentialRegistered(credential.Credential, true).
		Build()
	pp := &conway.ConwayProtocolParameters{}

	tests := []struct {
		name  string
		match func(error) bool
	}{
		{
			name: "certificate data",
			match: func(err error) bool {
				var target conway.StakeCredentialAlreadyRegisteredError
				return errors.As(err, &target)
			},
		},
		{
			name: "unknown voter",
			match: func(err error) bool {
				var target conway.UnknownVoterError
				return errors.As(err, &target)
			},
		},
		{
			name: "unknown governance action id",
			match: func(err error) bool {
				var target conway.UnknownGovActionIdError
				return errors.As(err, &target)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rules := conwayComposedRulesForError(t, validTx, ls, pp, tc.match)
			for _, rule := range rules {
				require.NoError(t, rule(invalidTx, 0, ls, pp))
			}
		})
	}
}

func TestPhase2InvalidStillRunsUtxowRules(t *testing.T) {
	pp := &conway.ConwayProtocolParameters{}
	ls := mockledger.NewLedgerStateBuilder().Build()

	t.Run("is-valid flag requires a redeemer", func(t *testing.T) {
		tx := &conway.ConwayTransaction{TxIsValid: false}
		match := func(err error) bool {
			var target common.InvalidIsValidFlagError
			return errors.As(err, &target)
		}
		// The helper fails when no composed rule reports the error, which
		// is the assertion: the rule still runs for an invalid transaction.
		conwayComposedRulesForError(t, tx, ls, pp, match)
	})

	t.Run("redeemer purpose remains well formed", func(t *testing.T) {
		tx := &conway.ConwayTransaction{
			TxIsValid: false,
			WitnessSet: conway.ConwayTransactionWitnessSet{
				WsRedeemers: conway.ConwayRedeemers{
					Redeemers: map[common.RedeemerKey]common.RedeemerValue{
						{Tag: common.RedeemerTagSpend}: {},
					},
				},
			},
		}
		match := func(err error) bool {
			var target conway.ExtraRedeemerError
			return errors.As(err, &target)
		}
		// The helper fails when no composed rule reports the error, which
		// is the assertion: the rule still runs for an invalid transaction.
		conwayComposedRulesForError(t, tx, ls, pp, match)
	})

	t.Run("collateral is still required", func(t *testing.T) {
		tx := &conway.ConwayTransaction{
			TxIsValid: false,
			WitnessSet: conway.ConwayTransactionWitnessSet{
				WsRedeemers: conway.ConwayRedeemers{
					Redeemers: map[common.RedeemerKey]common.RedeemerValue{
						{Tag: common.RedeemerTagSpend}: {},
					},
				},
			},
		}
		match := func(err error) bool {
			var target alonzo.NoCollateralInputsError
			return errors.As(err, &target)
		}
		// The helper fails when no composed rule reports the error, which
		// is the assertion: the rule still runs for an invalid transaction.
		conwayComposedRulesForError(t, tx, ls, pp, match)
	})
}

// TestPhase2InvalidSkipsGovernanceProposalRules covers the Conway governance
// group that runs before UTXOW. Nothing else in this package reaches it through
// the composition: the rules matched above sit in the post-UTXOW group, and
// UtxoValidateUnknownVoters, UtxoValidateCommitteeCertificates and
// UtxoValidateCertificateDeposits each carry their own phase-2 guard, so they
// still skip an invalid transaction with the composition gate removed.
func TestPhase2InvalidSkipsGovernanceProposalRules(t *testing.T) {
	newTx := func(isValid bool) *conway.ConwayTransaction {
		return &conway.ConwayTransaction{
			Body: conway.ConwayTransactionBody{
				TxProposalProcedures: []conway.ConwayProposalProcedure{{
					PPGovAction: conway.ConwayGovAction{
						Action: &conway.ConwayParameterChangeGovAction{},
					},
				}},
			},
			TxIsValid: isValid,
		}
	}
	ls := mockledger.NewLedgerStateBuilder().Build()
	pp := &conway.ConwayProtocolParameters{}
	match := func(err error) bool {
		var target conway.ProtocolParameterUpdateEmptyError
		return errors.As(err, &target)
	}

	rules := conwayComposedRulesForError(t, newTx(true), ls, pp, match)
	for _, rule := range rules {
		require.NoError(t, rule(newTx(false), 0, ls, pp))
	}
}

// TestGovActionRepresentabilityIsNotPhase2Gated pins the split inside
// UtxoValidateGovActionWellFormedness. Upstream rejects a proposal whose
// policy hash is not a 28-byte ScriptHash at CBOR decode, before the LEDGER
// rule and so before phase-2 validity is consulted, while its
// ConflictingCommitteeUpdate check is GOV and runs only for a phase-2-valid
// transaction.
func TestGovActionRepresentabilityIsNotPhase2Gated(t *testing.T) {
	ls := mockledger.NewLedgerStateBuilder().Build()
	pp := &conway.ConwayProtocolParameters{}

	proposalTx := func(
		action common.GovAction,
		isValid bool,
	) *conway.ConwayTransaction {
		return &conway.ConwayTransaction{
			Body: conway.ConwayTransactionBody{
				TxProposalProcedures: []conway.ConwayProposalProcedure{{
					PPGovAction: conway.ConwayGovAction{Action: action},
				}},
			},
			TxIsValid: isValid,
		}
	}

	t.Run("short policy hash is rejected in both phases", func(t *testing.T) {
		shortPolicy := make([]byte, common.Blake2b224Size-8)
		action := func() common.GovAction {
			return &conway.ConwayParameterChangeGovAction{
				PolicyHash: shortPolicy,
			}
		}
		match := func(err error) bool {
			var target conway.MalformedGovActionError
			return errors.As(err, &target)
		}

		// The rule must report the malformed hash for a phase-2-invalid
		// transaction, so it has to sit in the always-run group.
		rules := conwayComposedRulesForError(
			t,
			proposalTx(action(), false),
			ls,
			pp,
			match,
		)
		for _, rule := range rules {
			require.Error(t, rule(proposalTx(action(), true), 0, ls, pp))
		}
	})

	t.Run("conflicting committee update is gated", func(t *testing.T) {
		credential := common.Credential{
			CredType:   common.CredentialTypeAddrKeyHash,
			Credential: common.Blake2b224{0x04},
		}
		conflicting := credential
		action := func() common.GovAction {
			return &common.UpdateCommitteeGovAction{
				Credentials: []common.Credential{credential},
				CredEpochs:  map[*common.Credential]uint{&conflicting: 100},
				Quorum:      cbor.Rat{Rat: big.NewRat(1, 2)},
			}
		}
		match := func(err error) bool {
			var target conway.ConflictingCommitteeUpdateError
			return errors.As(err, &target)
		}

		rules := conwayComposedRulesForError(
			t,
			proposalTx(action(), true),
			ls,
			pp,
			match,
		)
		for _, rule := range rules {
			require.NoError(t, rule(proposalTx(action(), false), 0, ls, pp))
		}
	})
}
