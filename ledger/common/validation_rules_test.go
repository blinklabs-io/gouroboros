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
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUtxoValidationRuleDescriptors(t *testing.T) {
	tests := []struct {
		name        string
		descriptors func() []common.UtxoValidationRuleDescriptor
		legacy      []common.UtxoValidationRuleFunc
		ids         []common.UtxoValidationRuleId
	}{
		{
			name:        "Shelley",
			descriptors: shelley.UtxoValidationRuleDescriptors,
			legacy:      shelley.UtxoValidationRules,
			ids: []common.UtxoValidationRuleId{
				common.UtxoValidationRuleMetadata,
				common.UtxoValidationRuleRequiredVKeyWitnesses,
				common.UtxoValidationRuleSignatures,
				common.UtxoValidationRuleTimeToLive,
				common.UtxoValidationRuleInputSetEmpty,
				common.UtxoValidationRuleNoDuplicateInputs,
				common.UtxoValidationRuleFeeTooSmall,
				common.UtxoValidationRuleBadInputs,
				common.UtxoValidationRuleNativeScripts,
				common.UtxoValidationRuleScriptWitnesses,
				common.UtxoValidationRuleWrongNetwork,
				common.UtxoValidationRuleWrongNetworkWithdrawal,
				common.UtxoValidationRuleValueNotConserved,
				common.UtxoValidationRuleOutputTooSmall,
				common.UtxoValidationRuleOutputBootAddrAttrsTooBig,
				common.UtxoValidationRuleMaxTxSize,
				common.UtxoValidationRuleDelegation,
				common.UtxoValidationRuleWithdrawals,
			},
		},
		{
			name:        "Allegra",
			descriptors: allegra.UtxoValidationRuleDescriptors,
			legacy:      allegra.UtxoValidationRules,
			ids: []common.UtxoValidationRuleId{
				common.UtxoValidationRuleMetadata,
				common.UtxoValidationRuleRequiredVKeyWitnesses,
				common.UtxoValidationRuleSignatures,
				common.UtxoValidationRuleOutsideValidityInterval,
				common.UtxoValidationRuleInputSetEmpty,
				common.UtxoValidationRuleNoDuplicateInputs,
				common.UtxoValidationRuleFeeTooSmall,
				common.UtxoValidationRuleBadInputs,
				common.UtxoValidationRuleScriptWitnesses,
				common.UtxoValidationRuleWrongNetwork,
				common.UtxoValidationRuleWrongNetworkWithdrawal,
				common.UtxoValidationRuleValueNotConserved,
				common.UtxoValidationRuleOutputTooSmall,
				common.UtxoValidationRuleOutputBootAddrAttrsTooBig,
				common.UtxoValidationRuleMaxTxSize,
				common.UtxoValidationRuleNativeScripts,
				common.UtxoValidationRuleDelegation,
				common.UtxoValidationRuleWithdrawals,
			},
		},
		{
			name:        "Mary",
			descriptors: mary.UtxoValidationRuleDescriptors,
			legacy:      mary.UtxoValidationRules,
			ids: []common.UtxoValidationRuleId{
				common.UtxoValidationRuleMetadata,
				common.UtxoValidationRuleRequiredVKeyWitnesses,
				common.UtxoValidationRuleSignatures,
				common.UtxoValidationRuleOutsideValidityInterval,
				common.UtxoValidationRuleInputSetEmpty,
				common.UtxoValidationRuleNoDuplicateInputs,
				common.UtxoValidationRuleFeeTooSmall,
				common.UtxoValidationRuleBadInputs,
				common.UtxoValidationRuleScriptWitnesses,
				common.UtxoValidationRuleWrongNetwork,
				common.UtxoValidationRuleWrongNetworkWithdrawal,
				common.UtxoValidationRuleValueNotConserved,
				common.UtxoValidationRuleOutputTooSmall,
				common.UtxoValidationRuleOutputTooBig,
				common.UtxoValidationRuleOutputBootAddrAttrsTooBig,
				common.UtxoValidationRuleMaxTxSize,
				common.UtxoValidationRuleNativeScripts,
				common.UtxoValidationRuleDelegation,
				common.UtxoValidationRuleWithdrawals,
			},
		},
		{
			name:        "Alonzo",
			descriptors: alonzo.UtxoValidationRuleDescriptors,
			legacy:      alonzo.UtxoValidationRules,
			ids: []common.UtxoValidationRuleId{
				common.UtxoValidationRuleMetadata,
				common.UtxoValidationRuleIsValidFlag,
				common.UtxoValidationRuleRequiredVKeyWitnesses,
				common.UtxoValidationRuleSignatures,
				common.UtxoValidationRuleCollateralVKeyWitnesses,
				common.UtxoValidationRuleRedeemerAndScriptWitnesses,
				common.UtxoValidationRuleCostModelsPresent,
				common.UtxoValidationRuleScriptDataHash,
				common.UtxoValidationRuleOutsideValidityInterval,
				common.UtxoValidationRuleInputSetEmpty,
				common.UtxoValidationRuleNoDuplicateInputs,
				common.UtxoValidationRuleFeeTooSmall,
				common.UtxoValidationRuleInsufficientCollateral,
				common.UtxoValidationRuleCollateralContainsNonAda,
				common.UtxoValidationRuleNoCollateralInputs,
				common.UtxoValidationRuleBadInputs,
				common.UtxoValidationRuleScriptWitnesses,
				common.UtxoValidationRuleValueNotConserved,
				common.UtxoValidationRuleOutputTooSmall,
				common.UtxoValidationRuleOutputTooBig,
				common.UtxoValidationRuleOutputBootAddrAttrsTooBig,
				common.UtxoValidationRuleWrongNetwork,
				common.UtxoValidationRuleWrongNetworkWithdrawal,
				common.UtxoValidationRuleMaxTxSize,
				common.UtxoValidationRuleExUnitsTooBig,
				common.UtxoValidationRuleNativeScripts,
				common.UtxoValidationRuleExtraneousRedeemers,
				common.UtxoValidationRulePlutusScripts,
				common.UtxoValidationRuleDelegation,
				common.UtxoValidationRuleWithdrawals,
			},
		},
		{
			name:        "Babbage",
			descriptors: babbage.UtxoValidationRuleDescriptors,
			legacy:      babbage.UtxoValidationRules,
			ids: []common.UtxoValidationRuleId{
				common.UtxoValidationRuleMetadata,
				common.UtxoValidationRuleIsValidFlag,
				common.UtxoValidationRuleRequiredVKeyWitnesses,
				common.UtxoValidationRuleSignatures,
				common.UtxoValidationRuleCollateralVKeyWitnesses,
				common.UtxoValidationRuleRedeemerAndScriptWitnesses,
				common.UtxoValidationRuleCostModelsPresent,
				common.UtxoValidationRuleScriptDataHash,
				common.UtxoValidationRuleInlineDatumsWithPlutusV1,
				common.UtxoValidationRuleDisjointRefInputs,
				common.UtxoValidationRuleOutsideValidityInterval,
				common.UtxoValidationRuleInputSetEmpty,
				common.UtxoValidationRuleNoDuplicateInputs,
				common.UtxoValidationRuleFeeTooSmall,
				common.UtxoValidationRuleInsufficientCollateral,
				common.UtxoValidationRuleCollateralContainsNonAda,
				common.UtxoValidationRuleCollateralEqBalance,
				common.UtxoValidationRuleNoCollateralInputs,
				common.UtxoValidationRuleBadInputs,
				common.UtxoValidationRuleScriptWitnesses,
				common.UtxoValidationRuleRequiredRedeemers,
				common.UtxoValidationRuleValueNotConserved,
				common.UtxoValidationRuleOutputTooSmall,
				common.UtxoValidationRuleOutputTooBig,
				common.UtxoValidationRuleOutputBootAddrAttrsTooBig,
				common.UtxoValidationRuleWrongNetwork,
				common.UtxoValidationRuleWrongNetworkWithdrawal,
				common.UtxoValidationRuleMaxTxSize,
				common.UtxoValidationRuleExUnitsTooBig,
				common.UtxoValidationRuleTooManyCollateralInputs,
				common.UtxoValidationRuleNativeScripts,
				common.UtxoValidationRuleExtraneousRedeemers,
				common.UtxoValidationRuleMalformedReferenceScripts,
				common.UtxoValidationRulePlutusScripts,
				common.UtxoValidationRuleDelegation,
				common.UtxoValidationRuleWithdrawals,
			},
		},
		{
			name:        "Conway",
			descriptors: conway.UtxoValidationRuleDescriptors,
			legacy:      conway.UtxoValidationRules,
			ids: []common.UtxoValidationRuleId{
				common.UtxoValidationRuleCurrentTreasuryValue,
				common.UtxoValidationRuleMetadata,
				common.UtxoValidationRuleProposalProcedures,
				common.UtxoValidationRuleGovActionWellFormedness,
				common.UtxoValidationRuleHardForkCanFollow,
				common.UtxoValidationRuleProposalAncestry,
				common.UtxoValidationRuleProposalDeposit,
				common.UtxoValidationRuleProposalNetworkIds,
				common.UtxoValidationRuleProposalReturnAccounts,
				common.UtxoValidationRuleEmptyTreasuryWithdrawals,
				common.UtxoValidationRuleBootstrapAllowedGovActions,
				common.UtxoValidationRuleBootstrapParameterGroups,
				common.UtxoValidationRuleIsValidFlag,
				common.UtxoValidationRuleRequiredVKeyWitnesses,
				common.UtxoValidationRuleCollateralVKeyWitnesses,
				common.UtxoValidationRuleRedeemerAndScriptWitnesses,
				common.UtxoValidationRuleSignatures,
				common.UtxoValidationRuleCostModelsPresent,
				common.UtxoValidationRuleScriptDataHash,
				common.UtxoValidationRuleInlineDatumsWithPlutusV1,
				common.UtxoValidationRuleConwayFeaturesWithPlutusV1V2,
				common.UtxoValidationRuleDisjointRefInputs,
				common.UtxoValidationRuleOutsideValidityInterval,
				common.UtxoValidationRuleInputSetEmpty,
				common.UtxoValidationRuleNoDuplicateInputs,
				common.UtxoValidationRuleFeeTooSmall,
				common.UtxoValidationRuleInsufficientCollateral,
				common.UtxoValidationRuleCollateralContainsNonAda,
				common.UtxoValidationRuleCollateralEqBalance,
				common.UtxoValidationRuleNoCollateralInputs,
				common.UtxoValidationRuleBadInputs,
				common.UtxoValidationRuleScriptWitnesses,
				common.UtxoValidationRuleRequiredRedeemers,
				common.UtxoValidationRuleValueNotConserved,
				common.UtxoValidationRuleOutputTooSmall,
				common.UtxoValidationRuleOutputTooBig,
				common.UtxoValidationRuleOutputBootAddrAttrsTooBig,
				common.UtxoValidationRuleWrongNetwork,
				common.UtxoValidationRuleWrongNetworkWithdrawal,
				common.UtxoValidationRuleTransactionNetworkId,
				common.UtxoValidationRuleMaxTxSize,
				common.UtxoValidationRuleExUnitsTooBig,
				common.UtxoValidationRuleTooManyCollateralInputs,
				common.UtxoValidationRuleSupplementalDatums,
				common.UtxoValidationRuleExtraneousRedeemers,
				common.UtxoValidationRuleMalformedReferenceScripts,
				common.UtxoValidationRulePlutusScripts,
				common.UtxoValidationRuleNativeScripts,
				common.UtxoValidationRuleDelegation,
				common.UtxoValidationRuleWithdrawals,
				common.UtxoValidationRuleCertificateDeposits,
				common.UtxoValidationRuleCommitteeCertificates,
				common.UtxoValidationRuleUnknownVoters,
				common.UtxoValidationRuleUnknownGovActionIds,
				common.UtxoValidationRuleVotingOnExpiredGovAction,
				common.UtxoValidationRuleBootstrapVotingRestrictions,
				common.UtxoValidationRuleStakePoolVotingRestrictions,
				common.UtxoValidationRuleCCVotingRestrictions,
				common.UtxoValidationRuleRefScriptSizePerTx,
			},
		},
		{
			name:        "Dijkstra",
			descriptors: dijkstra.UtxoValidationRuleDescriptors,
			legacy:      dijkstra.UtxoValidationRules,
			ids: []common.UtxoValidationRuleId{
				common.UtxoValidationRuleCurrentTreasuryValue,
				common.UtxoValidationRuleMetadata,
				common.UtxoValidationRuleProposalProcedures,
				common.UtxoValidationRuleGovActionWellFormedness,
				common.UtxoValidationRuleHardForkCanFollow,
				common.UtxoValidationRuleProposalAncestry,
				common.UtxoValidationRuleProposalDeposit,
				common.UtxoValidationRuleProposalNetworkIds,
				common.UtxoValidationRuleProposalReturnAccounts,
				common.UtxoValidationRuleEmptyTreasuryWithdrawals,
				common.UtxoValidationRuleBootstrapAllowedGovActions,
				common.UtxoValidationRuleBootstrapParameterGroups,
				common.UtxoValidationRuleIsValidFlag,
				common.UtxoValidationRuleRequiredVKeyWitnesses,
				common.UtxoValidationRuleCollateralVKeyWitnesses,
				common.UtxoValidationRuleRedeemerAndScriptWitnesses,
				common.UtxoValidationRuleSignatures,
				common.UtxoValidationRuleCostModelsPresent,
				common.UtxoValidationRuleScriptDataHash,
				common.UtxoValidationRuleInlineDatumsWithPlutusV1,
				common.UtxoValidationRuleConwayFeaturesWithPlutusV1V2,
				common.UtxoValidationRuleDisjointRefInputs,
				common.UtxoValidationRuleOutsideValidityInterval,
				common.UtxoValidationRuleInputSetEmpty,
				common.UtxoValidationRuleNoDuplicateInputs,
				common.UtxoValidationRuleFeeTooSmall,
				common.UtxoValidationRuleInsufficientCollateral,
				common.UtxoValidationRuleCollateralContainsNonAda,
				common.UtxoValidationRuleCollateralEqBalance,
				common.UtxoValidationRuleNoCollateralInputs,
				common.UtxoValidationRuleBadInputs,
				common.UtxoValidationRuleScriptWitnesses,
				common.UtxoValidationRuleRequiredRedeemers,
				common.UtxoValidationRuleValueNotConserved,
				common.UtxoValidationRuleOutputTooSmall,
				common.UtxoValidationRuleOutputTooBig,
				common.UtxoValidationRuleOutputBootAddrAttrsTooBig,
				common.UtxoValidationRuleWrongNetwork,
				common.UtxoValidationRuleWrongNetworkWithdrawal,
				common.UtxoValidationRuleTransactionNetworkId,
				common.UtxoValidationRuleMaxTxSize,
				common.UtxoValidationRuleExUnitsTooBig,
				common.UtxoValidationRuleTooManyCollateralInputs,
				common.UtxoValidationRuleSupplementalDatums,
				common.UtxoValidationRuleExtraneousRedeemers,
				common.UtxoValidationRuleMalformedReferenceScripts,
				common.UtxoValidationRulePlutusScripts,
				common.UtxoValidationRuleNativeScripts,
				common.UtxoValidationRuleDelegation,
				common.UtxoValidationRuleWithdrawals,
				common.UtxoValidationRuleCertificateDeposits,
				common.UtxoValidationRuleCommitteeCertificates,
				common.UtxoValidationRuleUnknownVoters,
				common.UtxoValidationRuleUnknownGovActionIds,
				common.UtxoValidationRuleVotingOnExpiredGovAction,
				common.UtxoValidationRuleBootstrapVotingRestrictions,
				common.UtxoValidationRuleStakePoolVotingRestrictions,
				common.UtxoValidationRuleCCVotingRestrictions,
				common.UtxoValidationRuleRefScriptSizePerTx,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptors := test.descriptors()
			validators, ok := expectedUtxoValidationRuleValidators[test.name]
			require.True(t, ok, "missing expected validators for %s", test.name)
			expected, err := expectedUtxoValidationRuleDescriptors(
				test.ids,
				validators,
			)
			require.NoError(t, err)
			require.NoError(
				t,
				compareUtxoValidationRuleDescriptorMappings(
					descriptors,
					expected,
				),
			)

			derived, err := common.UtxoValidationRulesFromDescriptors(
				descriptors,
			)
			require.NoError(t, err)
			require.Len(t, derived, len(test.legacy))
			for idx := range derived {
				require.Equal(
					t,
					validationRuleIdentity(expected[idx].Validator),
					validationRuleIdentity(derived[idx]),
					"derived validator mismatch at index %d for ID %q",
					idx,
					expected[idx].Id,
				)
				require.Equal(
					t,
					validationRuleIdentity(expected[idx].Validator),
					validationRuleIdentity(test.legacy[idx]),
					"legacy validator mismatch at index %d for ID %q",
					idx,
					expected[idx].Id,
				)
			}

			descriptors[0].Id = "mutated"
			descriptors[0].Validator = nil
			freshDescriptors := test.descriptors()
			require.NoError(
				t,
				compareUtxoValidationRuleDescriptorMappings(
					freshDescriptors,
					expected,
				),
			)
		})
	}
}

func TestUtxoValidationRulesFromDescriptorsPreservesOrder(t *testing.T) {
	var calls []int
	descriptors := make([]common.UtxoValidationRuleDescriptor, 3)
	for i := range descriptors {
		idx := i
		descriptors[i] = common.UtxoValidationRuleDescriptor{
			Id: common.UtxoValidationRuleId("rule-" + string(rune('a'+i))),
			Validator: func(
				common.Transaction,
				uint64,
				common.LedgerState,
				common.ProtocolParameters,
			) error {
				calls = append(calls, idx)
				return nil
			},
		}
	}

	rules, err := common.UtxoValidationRulesFromDescriptors(descriptors)
	require.NoError(t, err)
	require.Len(t, rules, len(descriptors))
	for _, rule := range rules {
		require.NoError(t, rule(nil, 0, nil, nil))
	}
	assert.Equal(t, []int{0, 1, 2}, calls)
}

func TestUtxoValidationRulesFromDescriptorsRejectsInvalidDescriptors(
	t *testing.T,
) {
	validator := func(
		common.Transaction,
		uint64,
		common.LedgerState,
		common.ProtocolParameters,
	) error {
		return nil
	}
	tests := []struct {
		name        string
		descriptors []common.UtxoValidationRuleDescriptor
		err         string
	}{
		{
			name: "empty ID",
			descriptors: []common.UtxoValidationRuleDescriptor{
				{Id: "valid", Validator: validator},
				{Validator: validator},
			},
			err: "UTxO validation rule descriptor at index 1 has an empty ID",
		},
		{
			name: "duplicate ID",
			descriptors: []common.UtxoValidationRuleDescriptor{
				{Id: "duplicate", Validator: validator},
				{Id: "valid", Validator: validator},
				{Id: "duplicate", Validator: validator},
			},
			err: "UTxO validation rule descriptor at index 2 has duplicate ID " +
				"\"duplicate\" (first used at index 0)",
		},
		{
			name: "nil validator",
			descriptors: []common.UtxoValidationRuleDescriptor{
				{Id: "valid", Validator: validator},
				{Id: "nil-validator"},
			},
			err: "UTxO validation rule descriptor at index 1 with ID " +
				"\"nil-validator\" has a nil validator",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rules, err := common.UtxoValidationRulesFromDescriptors(
				test.descriptors,
			)
			assert.Nil(t, rules)
			assert.EqualError(t, err, test.err)
		})
	}
}
