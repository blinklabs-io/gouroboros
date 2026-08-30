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

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

const (
	conwayValidationRuleCount          = 55
	conwayIsValidFlagRuleIndex         = 11
	conwayNoCollateralInputsRuleIndex  = 28
	conwayExtraneousRedeemersRuleIndex = 42
	conwayDelegationRuleIndex          = 45
	conwayUnknownVotersRuleIndex       = 48
	conwayUnknownGovActionIdsRuleIndex = 49
)

func TestPhase2InvalidSkipsCertificateAndGovernanceRules(t *testing.T) {
	require.Len(t, conway.UtxoValidationRules, conwayValidationRuleCount)

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
		name            string
		ruleIndex       int
		checkValidError func(*testing.T, error)
	}{
		{
			name:      "certificate data",
			ruleIndex: conwayDelegationRuleIndex,
			checkValidError: func(t *testing.T, err error) {
				var target conway.StakeCredentialAlreadyRegisteredError
				require.ErrorAs(t, err, &target)
			},
		},
		{
			name:      "unknown voter",
			ruleIndex: conwayUnknownVotersRuleIndex,
			checkValidError: func(t *testing.T, err error) {
				var target conway.UnknownVoterError
				require.ErrorAs(t, err, &target)
			},
		},
		{
			name:      "unknown governance action id",
			ruleIndex: conwayUnknownGovActionIdsRuleIndex,
			checkValidError: func(t *testing.T, err error) {
				var target conway.UnknownGovActionIdError
				require.ErrorAs(t, err, &target)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rule := conway.UtxoValidationRules[tc.ruleIndex]
			require.NoError(t, rule(invalidTx, 0, ls, pp))

			err := rule(validTx, 0, ls, pp)
			tc.checkValidError(t, err)
		})
	}
}

func TestPhase2InvalidStillRunsUtxowRules(t *testing.T) {
	require.Len(t, conway.UtxoValidationRules, conwayValidationRuleCount)
	pp := &conway.ConwayProtocolParameters{}
	ls := mockledger.NewLedgerStateBuilder().Build()

	t.Run("is-valid flag requires a redeemer", func(t *testing.T) {
		tx := &conway.ConwayTransaction{TxIsValid: false}
		err := conway.UtxoValidationRules[conwayIsValidFlagRuleIndex](
			tx,
			0,
			ls,
			pp,
		)
		var target common.InvalidIsValidFlagError
		require.ErrorAs(t, err, &target)
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
		err := conway.UtxoValidationRules[conwayExtraneousRedeemersRuleIndex](
			tx,
			0,
			ls,
			pp,
		)
		var target conway.ExtraRedeemerError
		require.ErrorAs(t, err, &target)
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
		err := conway.UtxoValidationRules[conwayNoCollateralInputsRuleIndex](
			tx,
			0,
			ls,
			pp,
		)
		var target alonzo.NoCollateralInputsError
		require.ErrorAs(t, err, &target)
	})
}
