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
	"testing"

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
				require.True(t, tc.match(rule(validTx, 0, ls, pp)))
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
		rules := conwayComposedRulesForError(t, tx, ls, pp, match)
		require.True(t, match(rules[0](tx, 0, ls, pp)))
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
		rules := conwayComposedRulesForError(t, tx, ls, pp, match)
		require.True(t, match(rules[0](tx, 0, ls, pp)))
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
		rules := conwayComposedRulesForError(t, tx, ls, pp, match)
		require.True(t, match(rules[0](tx, 0, ls, pp)))
	})
}
