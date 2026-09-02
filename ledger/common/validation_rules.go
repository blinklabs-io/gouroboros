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

package common

import "fmt"

// UtxoValidationRuleId is the stable semantic identifier of a UTxO validation
// rule. Its string values are part of the public compatibility contract and
// must not be changed or reused for different semantics.
type UtxoValidationRuleId string

const (
	UtxoValidationRuleMetadata                     UtxoValidationRuleId = "metadata"
	UtxoValidationRuleRequiredVKeyWitnesses        UtxoValidationRuleId = "required-vkey-witnesses"
	UtxoValidationRuleSignatures                   UtxoValidationRuleId = "signatures"
	UtxoValidationRuleTimeToLive                   UtxoValidationRuleId = "time-to-live"
	UtxoValidationRuleInputSetEmpty                UtxoValidationRuleId = "input-set-empty"
	UtxoValidationRuleNoDuplicateInputs            UtxoValidationRuleId = "no-duplicate-inputs"
	UtxoValidationRuleFeeTooSmall                  UtxoValidationRuleId = "fee-too-small"
	UtxoValidationRuleBadInputs                    UtxoValidationRuleId = "bad-inputs"
	UtxoValidationRuleNativeScripts                UtxoValidationRuleId = "native-scripts"
	UtxoValidationRuleScriptWitnesses              UtxoValidationRuleId = "script-witnesses"
	UtxoValidationRuleWrongNetwork                 UtxoValidationRuleId = "wrong-network"
	UtxoValidationRuleWrongNetworkWithdrawal       UtxoValidationRuleId = "wrong-network-withdrawal"
	UtxoValidationRuleValueNotConserved            UtxoValidationRuleId = "value-not-conserved"
	UtxoValidationRuleOutputTooSmall               UtxoValidationRuleId = "output-too-small"
	UtxoValidationRuleOutputBootAddrAttrsTooBig    UtxoValidationRuleId = "output-bootstrap-address-attributes-too-big"
	UtxoValidationRuleMaxTxSize                    UtxoValidationRuleId = "max-transaction-size"
	UtxoValidationRuleDelegation                   UtxoValidationRuleId = "delegation"
	UtxoValidationRuleWithdrawals                  UtxoValidationRuleId = "withdrawals"
	UtxoValidationRuleCertificateDeposits          UtxoValidationRuleId = "certificate-deposits"
	UtxoValidationRuleOutsideValidityInterval      UtxoValidationRuleId = "outside-validity-interval"
	UtxoValidationRuleOutputTooBig                 UtxoValidationRuleId = "output-too-big"
	UtxoValidationRuleIsValidFlag                  UtxoValidationRuleId = "is-valid-flag"
	UtxoValidationRuleCollateralVKeyWitnesses      UtxoValidationRuleId = "collateral-vkey-witnesses"
	UtxoValidationRuleRedeemerAndScriptWitnesses   UtxoValidationRuleId = "redeemer-and-script-witnesses"
	UtxoValidationRuleCostModelsPresent            UtxoValidationRuleId = "cost-models-present"
	UtxoValidationRuleScriptDataHash               UtxoValidationRuleId = "script-data-hash"
	UtxoValidationRuleInsufficientCollateral       UtxoValidationRuleId = "insufficient-collateral"
	UtxoValidationRuleCollateralContainsNonAda     UtxoValidationRuleId = "collateral-contains-non-ada"
	UtxoValidationRuleNoCollateralInputs           UtxoValidationRuleId = "no-collateral-inputs"
	UtxoValidationRuleExUnitsTooBig                UtxoValidationRuleId = "execution-units-too-big"
	UtxoValidationRuleExtraneousRedeemers          UtxoValidationRuleId = "extraneous-redeemers"
	UtxoValidationRulePlutusScripts                UtxoValidationRuleId = "plutus-scripts"
	UtxoValidationRuleInlineDatumsWithPlutusV1     UtxoValidationRuleId = "inline-datums-with-plutus-v1"
	UtxoValidationRuleDisjointRefInputs            UtxoValidationRuleId = "disjoint-reference-inputs"
	UtxoValidationRuleCollateralEqBalance          UtxoValidationRuleId = "collateral-equals-balance"
	UtxoValidationRuleTooManyCollateralInputs      UtxoValidationRuleId = "too-many-collateral-inputs"
	UtxoValidationRuleMalformedReferenceScripts    UtxoValidationRuleId = "malformed-reference-scripts"
	UtxoValidationRuleCurrentTreasuryValue         UtxoValidationRuleId = "current-treasury-value"
	UtxoValidationRuleProposalProcedures           UtxoValidationRuleId = "proposal-procedures"
	UtxoValidationRuleGovActionWellFormedness      UtxoValidationRuleId = "governance-action-well-formedness"
	UtxoValidationRuleHardForkCanFollow            UtxoValidationRuleId = "hard-fork-can-follow"
	UtxoValidationRuleProposalAncestry             UtxoValidationRuleId = "proposal-ancestry"
	UtxoValidationRuleProposalDeposit              UtxoValidationRuleId = "proposal-deposit"
	UtxoValidationRuleProposalNetworkIds           UtxoValidationRuleId = "proposal-network-ids"
	UtxoValidationRuleProposalReturnAccounts       UtxoValidationRuleId = "proposal-return-accounts"
	UtxoValidationRuleEmptyTreasuryWithdrawals     UtxoValidationRuleId = "empty-treasury-withdrawals"
	UtxoValidationRuleBootstrapAllowedGovActions   UtxoValidationRuleId = "bootstrap-allowed-governance-actions"
	UtxoValidationRuleBootstrapParameterGroups     UtxoValidationRuleId = "bootstrap-parameter-groups"
	UtxoValidationRuleConwayFeaturesWithPlutusV1V2 UtxoValidationRuleId = "conway-features-with-plutus-v1-v2"
	UtxoValidationRuleTransactionNetworkId         UtxoValidationRuleId = "transaction-network-id"
	UtxoValidationRuleSupplementalDatums           UtxoValidationRuleId = "supplemental-datums"
	UtxoValidationRuleCommitteeCertificates        UtxoValidationRuleId = "committee-certificates"
	UtxoValidationRuleUnknownVoters                UtxoValidationRuleId = "unknown-voters"
	UtxoValidationRuleUnknownGovActionIds          UtxoValidationRuleId = "unknown-governance-action-ids"
	UtxoValidationRuleVotingOnExpiredGovAction     UtxoValidationRuleId = "voting-on-expired-governance-action"
	UtxoValidationRuleBootstrapVotingRestrictions  UtxoValidationRuleId = "bootstrap-voting-restrictions"
	UtxoValidationRuleStakePoolVotingRestrictions  UtxoValidationRuleId = "stake-pool-voting-restrictions"
	UtxoValidationRuleCCVotingRestrictions         UtxoValidationRuleId = "constitutional-committee-voting-restrictions"
	UtxoValidationRuleRefScriptSizePerTx           UtxoValidationRuleId = "reference-script-size-per-transaction"
	UtxoValidationRuleRequiredRedeemers            UtxoValidationRuleId = "required-redeemers"
)

// UtxoValidationRuleDescriptor pairs a stable semantic rule identifier with
// its validator implementation.
type UtxoValidationRuleDescriptor struct {
	Id        UtxoValidationRuleId
	Validator UtxoValidationRuleFunc
}

// UtxoValidationRulesFromDescriptors validates descriptors and returns their
// validators in the same order. It returns no rules when any descriptor has an
// empty ID, a duplicate ID, or a nil validator.
func UtxoValidationRulesFromDescriptors(
	descriptors []UtxoValidationRuleDescriptor,
) ([]UtxoValidationRuleFunc, error) {
	rules := make([]UtxoValidationRuleFunc, len(descriptors))
	seen := make(map[UtxoValidationRuleId]int, len(descriptors))
	for i, descriptor := range descriptors {
		if descriptor.Id == "" {
			return nil, fmt.Errorf(
				"UTxO validation rule descriptor at index %d has an empty ID",
				i,
			)
		}
		if firstIndex, ok := seen[descriptor.Id]; ok {
			return nil, fmt.Errorf(
				"UTxO validation rule descriptor at index %d has duplicate ID %q (first used at index %d)",
				i,
				descriptor.Id,
				firstIndex,
			)
		}
		if descriptor.Validator == nil {
			return nil, fmt.Errorf(
				"UTxO validation rule descriptor at index %d with ID %q has a nil validator",
				i,
				descriptor.Id,
			)
		}
		seen[descriptor.Id] = i
		rules[i] = descriptor.Validator
	}
	return rules, nil
}

// MustUtxoValidationRulesFromDescriptors is intended for authoritative static
// era rule lists. It panics rather than allowing an invalid list to initialize.
func MustUtxoValidationRulesFromDescriptors(
	descriptors []UtxoValidationRuleDescriptor,
) []UtxoValidationRuleFunc {
	rules, err := UtxoValidationRulesFromDescriptors(descriptors)
	if err != nil {
		panic(err)
	}
	return rules
}
