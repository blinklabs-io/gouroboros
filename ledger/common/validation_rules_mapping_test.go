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
	"fmt"
	"reflect"
	"runtime"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

var expectedUtxoValidationRuleValidators = map[string][]common.UtxoValidationRuleFunc{
	"Shelley": {
		shelley.UtxoValidateMetadata,                  // UtxoValidationRuleMetadata
		shelley.UtxoValidateRequiredVKeyWitnesses,     // UtxoValidationRuleRequiredVKeyWitnesses
		shelley.UtxoValidateSignatures,                // UtxoValidationRuleSignatures
		shelley.UtxoValidateTimeToLive,                // UtxoValidationRuleTimeToLive
		shelley.UtxoValidateInputSetEmptyUtxo,         // UtxoValidationRuleInputSetEmpty
		shelley.UtxoValidateNoDuplicateInputs,         // UtxoValidationRuleNoDuplicateInputs
		shelley.UtxoValidateFeeTooSmallUtxo,           // UtxoValidationRuleFeeTooSmall
		shelley.UtxoValidateBadInputsUtxo,             // UtxoValidationRuleBadInputs
		shelley.UtxoValidateNativeScripts,             // UtxoValidationRuleNativeScripts
		shelley.UtxoValidateScriptWitnesses,           // UtxoValidationRuleScriptWitnesses
		shelley.UtxoValidateWrongNetwork,              // UtxoValidationRuleWrongNetwork
		shelley.UtxoValidateWrongNetworkWithdrawal,    // UtxoValidationRuleWrongNetworkWithdrawal
		shelley.UtxoValidateValueNotConservedUtxo,     // UtxoValidationRuleValueNotConserved
		shelley.UtxoValidateOutputTooSmallUtxo,        // UtxoValidationRuleOutputTooSmall
		shelley.UtxoValidateOutputBootAddrAttrsTooBig, // UtxoValidationRuleOutputBootAddrAttrsTooBig
		shelley.UtxoValidateMaxTxSizeUtxo,             // UtxoValidationRuleMaxTxSize
		shelley.UtxoValidateDelegation,                // UtxoValidationRuleDelegation
		shelley.UtxoValidateWithdrawals,               // UtxoValidationRuleWithdrawals
		shelley.UtxoValidatePoolCertificates,          // UtxoValidationRulePoolCertificates
	},
	"Allegra": {
		allegra.UtxoValidateMetadata,                    // UtxoValidationRuleMetadata
		allegra.UtxoValidateRequiredVKeyWitnesses,       // UtxoValidationRuleRequiredVKeyWitnesses
		allegra.UtxoValidateSignatures,                  // UtxoValidationRuleSignatures
		allegra.UtxoValidateOutsideValidityIntervalUtxo, // UtxoValidationRuleOutsideValidityInterval
		allegra.UtxoValidateInputSetEmptyUtxo,           // UtxoValidationRuleInputSetEmpty
		allegra.UtxoValidateNoDuplicateInputs,           // UtxoValidationRuleNoDuplicateInputs
		allegra.UtxoValidateFeeTooSmallUtxo,             // UtxoValidationRuleFeeTooSmall
		allegra.UtxoValidateBadInputsUtxo,               // UtxoValidationRuleBadInputs
		allegra.UtxoValidateScriptWitnesses,             // UtxoValidationRuleScriptWitnesses
		allegra.UtxoValidateWrongNetwork,                // UtxoValidationRuleWrongNetwork
		allegra.UtxoValidateWrongNetworkWithdrawal,      // UtxoValidationRuleWrongNetworkWithdrawal
		allegra.UtxoValidateValueNotConservedUtxo,       // UtxoValidationRuleValueNotConserved
		allegra.UtxoValidateOutputTooSmallUtxo,          // UtxoValidationRuleOutputTooSmall
		allegra.UtxoValidateOutputBootAddrAttrsTooBig,   // UtxoValidationRuleOutputBootAddrAttrsTooBig
		allegra.UtxoValidateMaxTxSizeUtxo,               // UtxoValidationRuleMaxTxSize
		allegra.UtxoValidateNativeScripts,               // UtxoValidationRuleNativeScripts
		allegra.UtxoValidateDelegation,                  // UtxoValidationRuleDelegation
		allegra.UtxoValidateWithdrawals,                 // UtxoValidationRuleWithdrawals
		allegra.UtxoValidatePoolCertificates,            // UtxoValidationRulePoolCertificates
	},
	"Mary": {
		mary.UtxoValidateMetadata,                    // UtxoValidationRuleMetadata
		mary.UtxoValidateRequiredVKeyWitnesses,       // UtxoValidationRuleRequiredVKeyWitnesses
		mary.UtxoValidateSignatures,                  // UtxoValidationRuleSignatures
		mary.UtxoValidateOutsideValidityIntervalUtxo, // UtxoValidationRuleOutsideValidityInterval
		mary.UtxoValidateInputSetEmptyUtxo,           // UtxoValidationRuleInputSetEmpty
		mary.UtxoValidateNoDuplicateInputs,           // UtxoValidationRuleNoDuplicateInputs
		mary.UtxoValidateFeeTooSmallUtxo,             // UtxoValidationRuleFeeTooSmall
		mary.UtxoValidateBadInputsUtxo,               // UtxoValidationRuleBadInputs
		mary.UtxoValidateScriptWitnesses,             // UtxoValidationRuleScriptWitnesses
		mary.UtxoValidateWrongNetwork,                // UtxoValidationRuleWrongNetwork
		mary.UtxoValidateWrongNetworkWithdrawal,      // UtxoValidationRuleWrongNetworkWithdrawal
		mary.UtxoValidateValueNotConservedUtxo,       // UtxoValidationRuleValueNotConserved
		mary.UtxoValidateOutputTooSmallUtxo,          // UtxoValidationRuleOutputTooSmall
		mary.UtxoValidateOutputTooBigUtxo,            // UtxoValidationRuleOutputTooBig
		mary.UtxoValidateOutputBootAddrAttrsTooBig,   // UtxoValidationRuleOutputBootAddrAttrsTooBig
		mary.UtxoValidateMaxTxSizeUtxo,               // UtxoValidationRuleMaxTxSize
		mary.UtxoValidateNativeScripts,               // UtxoValidationRuleNativeScripts
		mary.UtxoValidateDelegation,                  // UtxoValidationRuleDelegation
		mary.UtxoValidateWithdrawals,                 // UtxoValidationRuleWithdrawals
		mary.UtxoValidatePoolCertificates,            // UtxoValidationRulePoolCertificates
	},
	"Alonzo": {
		alonzo.UtxoValidateMetadata,                    // UtxoValidationRuleMetadata
		alonzo.UtxoValidateIsValidFlag,                 // UtxoValidationRuleIsValidFlag
		alonzo.UtxoValidateRequiredVKeyWitnesses,       // UtxoValidationRuleRequiredVKeyWitnesses
		alonzo.UtxoValidateSignatures,                  // UtxoValidationRuleSignatures
		alonzo.UtxoValidateCollateralVKeyWitnesses,     // UtxoValidationRuleCollateralVKeyWitnesses
		alonzo.UtxoValidateRedeemerAndScriptWitnesses,  // UtxoValidationRuleRedeemerAndScriptWitnesses
		alonzo.UtxoValidateCostModelsPresent,           // UtxoValidationRuleCostModelsPresent
		alonzo.UtxoValidateScriptDataHash,              // UtxoValidationRuleScriptDataHash
		alonzo.UtxoValidateOutsideValidityIntervalUtxo, // UtxoValidationRuleOutsideValidityInterval
		alonzo.UtxoValidateInputSetEmptyUtxo,           // UtxoValidationRuleInputSetEmpty
		alonzo.UtxoValidateNoDuplicateInputs,           // UtxoValidationRuleNoDuplicateInputs
		alonzo.UtxoValidateFeeTooSmallUtxo,             // UtxoValidationRuleFeeTooSmall
		alonzo.UtxoValidateInsufficientCollateral,      // UtxoValidationRuleInsufficientCollateral
		alonzo.UtxoValidateCollateralContainsNonAda,    // UtxoValidationRuleCollateralContainsNonAda
		alonzo.UtxoValidateNoCollateralInputs,          // UtxoValidationRuleNoCollateralInputs
		alonzo.UtxoValidateBadInputsUtxo,               // UtxoValidationRuleBadInputs
		alonzo.UtxoValidateScriptWitnesses,             // UtxoValidationRuleScriptWitnesses
		alonzo.UtxoValidateValueNotConservedUtxo,       // UtxoValidationRuleValueNotConserved
		alonzo.UtxoValidateOutputTooSmallUtxo,          // UtxoValidationRuleOutputTooSmall
		alonzo.UtxoValidateOutputTooBigUtxo,            // UtxoValidationRuleOutputTooBig
		alonzo.UtxoValidateOutputBootAddrAttrsTooBig,   // UtxoValidationRuleOutputBootAddrAttrsTooBig
		alonzo.UtxoValidateWrongNetwork,                // UtxoValidationRuleWrongNetwork
		alonzo.UtxoValidateWrongNetworkWithdrawal,      // UtxoValidationRuleWrongNetworkWithdrawal
		alonzo.UtxoValidateMaxTxSizeUtxo,               // UtxoValidationRuleMaxTxSize
		alonzo.UtxoValidateExUnitsTooBigUtxo,           // UtxoValidationRuleExUnitsTooBig
		alonzo.UtxoValidateNativeScripts,               // UtxoValidationRuleNativeScripts
		alonzo.UtxoValidateExtraneousRedeemers,         // UtxoValidationRuleExtraneousRedeemers
		alonzo.UtxoValidatePlutusScripts,               // UtxoValidationRulePlutusScripts
		alonzo.UtxoValidateDelegation,                  // UtxoValidationRuleDelegation
		alonzo.UtxoValidateWithdrawals,                 // UtxoValidationRuleWithdrawals
		alonzo.UtxoValidatePoolCertificates,            // UtxoValidationRulePoolCertificates
	},
	"Babbage": {
		babbage.UtxoValidateMetadata,                    // UtxoValidationRuleMetadata
		babbage.UtxoValidateIsValidFlag,                 // UtxoValidationRuleIsValidFlag
		babbage.UtxoValidateRequiredVKeyWitnesses,       // UtxoValidationRuleRequiredVKeyWitnesses
		babbage.UtxoValidateSignatures,                  // UtxoValidationRuleSignatures
		babbage.UtxoValidateCollateralVKeyWitnesses,     // UtxoValidationRuleCollateralVKeyWitnesses
		babbage.UtxoValidateRedeemerAndScriptWitnesses,  // UtxoValidationRuleRedeemerAndScriptWitnesses
		babbage.UtxoValidateCostModelsPresent,           // UtxoValidationRuleCostModelsPresent
		babbage.UtxoValidateScriptDataHash,              // UtxoValidationRuleScriptDataHash
		babbage.UtxoValidateInlineDatumsWithPlutusV1,    // UtxoValidationRuleInlineDatumsWithPlutusV1
		babbage.UtxoValidateDisjointRefInputs,           // UtxoValidationRuleDisjointRefInputs
		babbage.UtxoValidateOutsideValidityIntervalUtxo, // UtxoValidationRuleOutsideValidityInterval
		babbage.UtxoValidateInputSetEmptyUtxo,           // UtxoValidationRuleInputSetEmpty
		babbage.UtxoValidateNoDuplicateInputs,           // UtxoValidationRuleNoDuplicateInputs
		babbage.UtxoValidateFeeTooSmallUtxo,             // UtxoValidationRuleFeeTooSmall
		babbage.UtxoValidateInsufficientCollateral,      // UtxoValidationRuleInsufficientCollateral
		babbage.UtxoValidateCollateralContainsNonAda,    // UtxoValidationRuleCollateralContainsNonAda
		babbage.UtxoValidateCollateralEqBalance,         // UtxoValidationRuleCollateralEqBalance
		babbage.UtxoValidateNoCollateralInputs,          // UtxoValidationRuleNoCollateralInputs
		babbage.UtxoValidateBadInputsUtxo,               // UtxoValidationRuleBadInputs
		babbage.UtxoValidateScriptWitnesses,             // UtxoValidationRuleScriptWitnesses
		babbage.UtxoValidateRequiredRedeemers,           // UtxoValidationRuleRequiredRedeemers
		babbage.UtxoValidateValueNotConservedUtxo,       // UtxoValidationRuleValueNotConserved
		babbage.UtxoValidateOutputTooSmallUtxo,          // UtxoValidationRuleOutputTooSmall
		babbage.UtxoValidateOutputTooBigUtxo,            // UtxoValidationRuleOutputTooBig
		babbage.UtxoValidateOutputBootAddrAttrsTooBig,   // UtxoValidationRuleOutputBootAddrAttrsTooBig
		babbage.UtxoValidateWrongNetwork,                // UtxoValidationRuleWrongNetwork
		babbage.UtxoValidateWrongNetworkWithdrawal,      // UtxoValidationRuleWrongNetworkWithdrawal
		babbage.UtxoValidateMaxTxSizeUtxo,               // UtxoValidationRuleMaxTxSize
		babbage.UtxoValidateExUnitsTooBigUtxo,           // UtxoValidationRuleExUnitsTooBig
		babbage.UtxoValidateTooManyCollateralInputs,     // UtxoValidationRuleTooManyCollateralInputs
		babbage.UtxoValidateNativeScripts,               // UtxoValidationRuleNativeScripts
		babbage.UtxoValidateExtraneousRedeemers,         // UtxoValidationRuleExtraneousRedeemers
		babbage.UtxoValidateMalformedReferenceScripts,   // UtxoValidationRuleMalformedReferenceScripts
		babbage.UtxoValidatePlutusScripts,               // UtxoValidationRulePlutusScripts
		babbage.UtxoValidateDelegation,                  // UtxoValidationRuleDelegation
		babbage.UtxoValidateWithdrawals,                 // UtxoValidationRuleWithdrawals
		babbage.UtxoValidatePoolCertificates,            // UtxoValidationRulePoolCertificates
	},
	"Conway": {
		common.UtxoValidateCurrentTreasuryValue,         // UtxoValidationRuleCurrentTreasuryValue
		conway.UtxoValidateMetadata,                     // UtxoValidationRuleMetadata
		conway.UtxoValidateProposalProcedures,           // UtxoValidationRuleProposalProcedures
		conway.UtxoValidateGovActionWellFormedness,      // UtxoValidationRuleGovActionWellFormedness
		conway.UtxoValidateHardForkCanFollow,            // UtxoValidationRuleHardForkCanFollow
		conway.UtxoValidateProposalAncestry,             // UtxoValidationRuleProposalAncestry
		conway.UtxoValidateProposalDeposit,              // UtxoValidationRuleProposalDeposit
		conway.UtxoValidateProposalNetworkIds,           // UtxoValidationRuleProposalNetworkIds
		conway.UtxoValidateProposalReturnAccounts,       // UtxoValidationRuleProposalReturnAccounts
		conway.UtxoValidateEmptyTreasuryWithdrawals,     // UtxoValidationRuleEmptyTreasuryWithdrawals
		conway.UtxoValidateBootstrapAllowedGovActions,   // UtxoValidationRuleBootstrapAllowedGovActions
		conway.UtxoValidateBootstrapParameterGroups,     // UtxoValidationRuleBootstrapParameterGroups
		conway.UtxoValidateIsValidFlag,                  // UtxoValidationRuleIsValidFlag
		conway.UtxoValidateRequiredVKeyWitnesses,        // UtxoValidationRuleRequiredVKeyWitnesses
		conway.UtxoValidateCollateralVKeyWitnesses,      // UtxoValidationRuleCollateralVKeyWitnesses
		conway.UtxoValidateRedeemerAndScriptWitnesses,   // UtxoValidationRuleRedeemerAndScriptWitnesses
		conway.UtxoValidateSignatures,                   // UtxoValidationRuleSignatures
		conway.UtxoValidateCostModelsPresent,            // UtxoValidationRuleCostModelsPresent
		conway.UtxoValidateScriptDataHash,               // UtxoValidationRuleScriptDataHash
		conway.UtxoValidateInlineDatumsWithPlutusV1,     // UtxoValidationRuleInlineDatumsWithPlutusV1
		conway.UtxoValidateConwayFeaturesWithPlutusV1V2, // UtxoValidationRuleConwayFeaturesWithPlutusV1V2
		conway.UtxoValidateDisjointRefInputs,            // UtxoValidationRuleDisjointRefInputs
		conway.UtxoValidateOutsideValidityIntervalUtxo,  // UtxoValidationRuleOutsideValidityInterval
		conway.UtxoValidateInputSetEmptyUtxo,            // UtxoValidationRuleInputSetEmpty
		conway.UtxoValidateNoDuplicateInputs,            // UtxoValidationRuleNoDuplicateInputs
		conway.UtxoValidateFeeTooSmallUtxo,              // UtxoValidationRuleFeeTooSmall
		conway.UtxoValidateInsufficientCollateral,       // UtxoValidationRuleInsufficientCollateral
		conway.UtxoValidateCollateralContainsNonAda,     // UtxoValidationRuleCollateralContainsNonAda
		conway.UtxoValidateCollateralEqBalance,          // UtxoValidationRuleCollateralEqBalance
		conway.UtxoValidateNoCollateralInputs,           // UtxoValidationRuleNoCollateralInputs
		conway.UtxoValidateBadInputsUtxo,                // UtxoValidationRuleBadInputs
		conway.UtxoValidateScriptWitnesses,              // UtxoValidationRuleScriptWitnesses
		conway.UtxoValidateRequiredRedeemers,            // UtxoValidationRuleRequiredRedeemers
		conway.UtxoValidateValueNotConservedUtxo,        // UtxoValidationRuleValueNotConserved
		conway.UtxoValidateOutputTooSmallUtxo,           // UtxoValidationRuleOutputTooSmall
		conway.UtxoValidateOutputTooBigUtxo,             // UtxoValidationRuleOutputTooBig
		conway.UtxoValidateOutputBootAddrAttrsTooBig,    // UtxoValidationRuleOutputBootAddrAttrsTooBig
		conway.UtxoValidateWrongNetwork,                 // UtxoValidationRuleWrongNetwork
		conway.UtxoValidateWrongNetworkWithdrawal,       // UtxoValidationRuleWrongNetworkWithdrawal
		conway.UtxoValidateTransactionNetworkId,         // UtxoValidationRuleTransactionNetworkId
		conway.UtxoValidateMaxTxSizeUtxo,                // UtxoValidationRuleMaxTxSize
		conway.UtxoValidateExUnitsTooBigUtxo,            // UtxoValidationRuleExUnitsTooBig
		conway.UtxoValidateTooManyCollateralInputs,      // UtxoValidationRuleTooManyCollateralInputs
		conway.UtxoValidateSupplementalDatums,           // UtxoValidationRuleSupplementalDatums
		conway.UtxoValidateExtraneousRedeemers,          // UtxoValidationRuleExtraneousRedeemers
		conway.UtxoValidateMalformedReferenceScripts,    // UtxoValidationRuleMalformedReferenceScripts
		conway.UtxoValidatePlutusScripts,                // UtxoValidationRulePlutusScripts
		conway.UtxoValidateNativeScripts,                // UtxoValidationRuleNativeScripts
		conway.UtxoValidateDelegation,                   // UtxoValidationRuleDelegation
		conway.UtxoValidateWithdrawals,                  // UtxoValidationRuleWithdrawals
		conway.UtxoValidateCertificateDeposits,          // UtxoValidationRuleCertificateDeposits
		conway.UtxoValidateCommitteeCertificates,        // UtxoValidationRuleCommitteeCertificates
		conway.UtxoValidateUnknownVoters,                // UtxoValidationRuleUnknownVoters
		conway.UtxoValidateUnknownGovActionIds,          // UtxoValidationRuleUnknownGovActionIds
		conway.UtxoValidateVotingOnExpiredGovAction,     // UtxoValidationRuleVotingOnExpiredGovAction
		conway.UtxoValidateBootstrapVotingRestrictions,  // UtxoValidationRuleBootstrapVotingRestrictions
		conway.UtxoValidateStakePoolVotingRestrictions,  // UtxoValidationRuleStakePoolVotingRestrictions
		conway.UtxoValidateCCVotingRestrictions,         // UtxoValidationRuleCCVotingRestrictions
		conway.UtxoValidateRefScriptSizePerTx,           // UtxoValidationRuleRefScriptSizePerTx
		conway.UtxoValidatePoolCertificates,             // UtxoValidationRulePoolCertificates
	},
	"Dijkstra": {
		common.UtxoValidateCurrentTreasuryValue,           // UtxoValidationRuleCurrentTreasuryValue
		conway.UtxoValidateMetadata,                       // UtxoValidationRuleMetadata
		dijkstra.UtxoValidateProposalProcedures,           // UtxoValidationRuleProposalProcedures
		conway.UtxoValidateGovActionWellFormedness,        // UtxoValidationRuleGovActionWellFormedness
		dijkstra.UtxoValidateHardForkCanFollow,            // UtxoValidationRuleHardForkCanFollow
		conway.UtxoValidateProposalAncestry,               // UtxoValidationRuleProposalAncestry
		dijkstra.UtxoValidateProposalDeposit,              // UtxoValidationRuleProposalDeposit
		conway.UtxoValidateProposalNetworkIds,             // UtxoValidationRuleProposalNetworkIds
		conway.UtxoValidateProposalReturnAccounts,         // UtxoValidationRuleProposalReturnAccounts
		conway.UtxoValidateEmptyTreasuryWithdrawals,       // UtxoValidationRuleEmptyTreasuryWithdrawals
		dijkstra.UtxoValidateBootstrapAllowedGovActions,   // UtxoValidationRuleBootstrapAllowedGovActions
		dijkstra.UtxoValidateBootstrapParameterGroups,     // UtxoValidationRuleBootstrapParameterGroups
		dijkstra.UtxoValidateIsValidFlag,                  // UtxoValidationRuleIsValidFlag
		dijkstra.UtxoValidateRequiredVKeyWitnesses,        // UtxoValidationRuleRequiredVKeyWitnesses
		conway.UtxoValidateCollateralVKeyWitnesses,        // UtxoValidationRuleCollateralVKeyWitnesses
		dijkstra.UtxoValidateRedeemerAndScriptWitnesses,   // UtxoValidationRuleRedeemerAndScriptWitnesses
		dijkstra.UtxoValidateSignatures,                   // UtxoValidationRuleSignatures
		dijkstra.UtxoValidateCostModelsPresent,            // UtxoValidationRuleCostModelsPresent
		dijkstra.UtxoValidateScriptDataHash,               // UtxoValidationRuleScriptDataHash
		conway.UtxoValidateInlineDatumsWithPlutusV1,       // UtxoValidationRuleInlineDatumsWithPlutusV1
		dijkstra.UtxoValidateConwayFeaturesWithPlutusV1V2, // UtxoValidationRuleConwayFeaturesWithPlutusV1V2
		dijkstra.UtxoValidateDisjointRefInputs,            // UtxoValidationRuleDisjointRefInputs
		conway.UtxoValidateOutsideValidityIntervalUtxo,    // UtxoValidationRuleOutsideValidityInterval
		conway.UtxoValidateInputSetEmptyUtxo,              // UtxoValidationRuleInputSetEmpty
		conway.UtxoValidateNoDuplicateInputs,              // UtxoValidationRuleNoDuplicateInputs
		dijkstra.UtxoValidateFeeTooSmallUtxo,              // UtxoValidationRuleFeeTooSmall
		dijkstra.UtxoValidateInsufficientCollateral,       // UtxoValidationRuleInsufficientCollateral
		dijkstra.UtxoValidateCollateralContainsNonAda,     // UtxoValidationRuleCollateralContainsNonAda
		conway.UtxoValidateCollateralEqBalance,            // UtxoValidationRuleCollateralEqBalance
		dijkstra.UtxoValidateNoCollateralInputs,           // UtxoValidationRuleNoCollateralInputs
		conway.UtxoValidateBadInputsUtxo,                  // UtxoValidationRuleBadInputs
		dijkstra.UtxoValidateScriptWitnesses,              // UtxoValidationRuleScriptWitnesses
		conway.UtxoValidateRequiredRedeemers,              // UtxoValidationRuleRequiredRedeemers
		dijkstra.UtxoValidateBatchWithdrawals,             // UtxoValidationRuleBatchWithdrawals
		dijkstra.UtxoValidateValueNotConservedUtxo,        // UtxoValidationRuleValueNotConserved
		dijkstra.UtxoValidateOutputTooSmallUtxo,           // UtxoValidationRuleOutputTooSmall
		dijkstra.UtxoValidateOutputTooBigUtxo,             // UtxoValidationRuleOutputTooBig
		conway.UtxoValidateOutputBootAddrAttrsTooBig,      // UtxoValidationRuleOutputBootAddrAttrsTooBig
		conway.UtxoValidateWrongNetwork,                   // UtxoValidationRuleWrongNetwork
		conway.UtxoValidateWrongNetworkWithdrawal,         // UtxoValidationRuleWrongNetworkWithdrawal
		dijkstra.UtxoValidateTransactionNetworkId,         // UtxoValidationRuleTransactionNetworkId
		dijkstra.UtxoValidateMaxTxSizeUtxo,                // UtxoValidationRuleMaxTxSize
		dijkstra.UtxoValidateExUnitsTooBigUtxo,            // UtxoValidationRuleExUnitsTooBig
		dijkstra.UtxoValidateTooManyCollateralInputs,      // UtxoValidationRuleTooManyCollateralInputs
		dijkstra.UtxoValidateSupplementalDatums,           // UtxoValidationRuleSupplementalDatums
		dijkstra.UtxoValidateExtraneousRedeemers,          // UtxoValidationRuleExtraneousRedeemers
		dijkstra.UtxoValidateMalformedReferenceScripts,    // UtxoValidationRuleMalformedReferenceScripts
		dijkstra.UtxoValidatePlutusScripts,                // UtxoValidationRulePlutusScripts
		dijkstra.UtxoValidateNativeScripts,                // UtxoValidationRuleNativeScripts
		conway.UtxoValidateDelegation,                     // UtxoValidationRuleDelegation
		conway.UtxoValidateWithdrawals,                    // UtxoValidationRuleWithdrawals
		conway.UtxoValidateCertificateDeposits,            // UtxoValidationRuleCertificateDeposits
		conway.UtxoValidateCommitteeCertificates,          // UtxoValidationRuleCommitteeCertificates
		conway.UtxoValidateUnknownVoters,                  // UtxoValidationRuleUnknownVoters
		conway.UtxoValidateUnknownGovActionIds,            // UtxoValidationRuleUnknownGovActionIds
		conway.UtxoValidateVotingOnExpiredGovAction,       // UtxoValidationRuleVotingOnExpiredGovAction
		dijkstra.UtxoValidateBootstrapVotingRestrictions,  // UtxoValidationRuleBootstrapVotingRestrictions
		conway.UtxoValidateStakePoolVotingRestrictions,    // UtxoValidationRuleStakePoolVotingRestrictions
		dijkstra.UtxoValidateCCVotingRestrictions,         // UtxoValidationRuleCCVotingRestrictions
		dijkstra.UtxoValidateRefScriptSizePerTx,           // UtxoValidationRuleRefScriptSizePerTx
		conway.UtxoValidatePoolCertificates,               // UtxoValidationRulePoolCertificates
	},
}

func validationRuleIdentity(rule common.UtxoValidationRuleFunc) uintptr {
	if rule == nil {
		return 0
	}
	return reflect.ValueOf(rule).Pointer()
}

func validationRuleName(rule common.UtxoValidationRuleFunc) string {
	identity := validationRuleIdentity(rule)
	if identity == 0 {
		return "<nil>"
	}
	function := runtime.FuncForPC(identity)
	if function == nil {
		return fmt.Sprintf("<unknown:%d>", identity)
	}
	return function.Name()
}

func expectedUtxoValidationRuleDescriptors(
	ids []common.UtxoValidationRuleId,
	validators []common.UtxoValidationRuleFunc,
) ([]common.UtxoValidationRuleDescriptor, error) {
	if len(ids) != len(validators) {
		return nil, fmt.Errorf(
			"expected ID/validator length mismatch: %d IDs, %d validators",
			len(ids),
			len(validators),
		)
	}
	descriptors := make([]common.UtxoValidationRuleDescriptor, len(ids))
	for idx := range ids {
		descriptors[idx] = common.UtxoValidationRuleDescriptor{
			Id:        ids[idx],
			Validator: validators[idx],
		}
	}
	return descriptors, nil
}

func compareUtxoValidationRuleDescriptorMappings(
	actual []common.UtxoValidationRuleDescriptor,
	expected []common.UtxoValidationRuleDescriptor,
) error {
	if len(actual) != len(expected) {
		return fmt.Errorf(
			"descriptor length mismatch: got %d, want %d",
			len(actual),
			len(expected),
		)
	}
	for idx := range expected {
		if actual[idx].Id != expected[idx].Id {
			return fmt.Errorf(
				"descriptor ID mismatch at index %d: got %q, want %q",
				idx,
				actual[idx].Id,
				expected[idx].Id,
			)
		}
		if validationRuleIdentity(actual[idx].Validator) !=
			validationRuleIdentity(expected[idx].Validator) {
			return fmt.Errorf(
				"descriptor validator mismatch at index %d for ID %q: got %s, want %s",
				idx,
				expected[idx].Id,
				validationRuleName(actual[idx].Validator),
				validationRuleName(expected[idx].Validator),
			)
		}
	}
	return nil
}

func TestUtxoValidationRuleDescriptorMappingDetectsMutation(t *testing.T) {
	expected := []common.UtxoValidationRuleDescriptor{
		{
			Id:        common.UtxoValidationRuleMetadata,
			Validator: shelley.UtxoValidateMetadata,
		},
		{
			Id:        common.UtxoValidationRuleRequiredVKeyWitnesses,
			Validator: shelley.UtxoValidateRequiredVKeyWitnesses,
		},
	}
	tests := []struct {
		name   string
		mutate func([]common.UtxoValidationRuleDescriptor)
		want   string
	}{
		{
			name: "substituted validator",
			mutate: func(descriptors []common.UtxoValidationRuleDescriptor) {
				descriptors[0].Validator = shelley.UtxoValidateSignatures
			},
			want: "descriptor validator mismatch at index 0",
		},
		{
			name: "reordered descriptors",
			mutate: func(descriptors []common.UtxoValidationRuleDescriptor) {
				descriptors[0], descriptors[1] = descriptors[1], descriptors[0]
			},
			want: "descriptor ID mismatch at index 0",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := append(
				[]common.UtxoValidationRuleDescriptor(nil),
				expected...,
			)
			test.mutate(actual)
			err := compareUtxoValidationRuleDescriptorMappings(actual, expected)
			require.ErrorContains(t, err, test.want)
			t.Log(err)
		})
	}
}
