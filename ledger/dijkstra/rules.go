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

package dijkstra

import (
	"bytes"
	"errors"
	"fmt"
	"iter"
	"math"
	"math/big"
	"slices"
	"strings"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/cek"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
)

const minUtxoOverheadBytes = 160

var utxoValidationRuleDescriptors = []common.UtxoValidationRuleDescriptor{
	{
		Id:        common.UtxoValidationRuleCurrentTreasuryValue,
		Validator: common.UtxoValidateCurrentTreasuryValue,
	},
	{
		Id:        common.UtxoValidationRuleMetadata,
		Validator: conway.UtxoValidateMetadata,
	},
	{
		Id:        common.UtxoValidationRuleProposalProcedures,
		Validator: UtxoValidateProposalProcedures,
	},
	{
		Id:        common.UtxoValidationRuleGovActionWellFormedness,
		Validator: conway.UtxoValidateGovActionWellFormedness,
	},
	{
		Id:        common.UtxoValidationRuleHardForkCanFollow,
		Validator: UtxoValidateHardForkCanFollow,
	},
	{
		Id:        common.UtxoValidationRuleProposalAncestry,
		Validator: conway.UtxoValidateProposalAncestry,
	},
	{
		Id:        common.UtxoValidationRuleProposalDeposit,
		Validator: UtxoValidateProposalDeposit,
	},
	{
		Id:        common.UtxoValidationRuleProposalNetworkIds,
		Validator: conway.UtxoValidateProposalNetworkIds,
	},
	{
		Id:        common.UtxoValidationRuleProposalReturnAccounts,
		Validator: conway.UtxoValidateProposalReturnAccounts,
	},
	{
		Id:        common.UtxoValidationRuleEmptyTreasuryWithdrawals,
		Validator: conway.UtxoValidateEmptyTreasuryWithdrawals,
	},
	{
		Id:        common.UtxoValidationRuleBootstrapAllowedGovActions,
		Validator: UtxoValidateBootstrapAllowedGovActions,
	},
	{
		Id:        common.UtxoValidationRuleBootstrapParameterGroups,
		Validator: UtxoValidateBootstrapParameterGroups,
	},
	{
		Id:        common.UtxoValidationRuleIsValidFlag,
		Validator: UtxoValidateIsValidFlag,
	},
	{
		Id:        common.UtxoValidationRuleRequiredVKeyWitnesses,
		Validator: UtxoValidateRequiredVKeyWitnesses,
	},
	{
		Id:        common.UtxoValidationRuleCollateralVKeyWitnesses,
		Validator: conway.UtxoValidateCollateralVKeyWitnesses,
	},
	{
		Id:        common.UtxoValidationRuleRedeemerAndScriptWitnesses,
		Validator: UtxoValidateRedeemerAndScriptWitnesses,
	},
	{
		Id:        common.UtxoValidationRuleSignatures,
		Validator: UtxoValidateSignatures,
	},
	{
		Id:        common.UtxoValidationRuleCostModelsPresent,
		Validator: UtxoValidateCostModelsPresent,
	},
	{
		Id:        common.UtxoValidationRuleScriptDataHash,
		Validator: UtxoValidateScriptDataHash,
	},
	{
		Id:        common.UtxoValidationRuleInlineDatumsWithPlutusV1,
		Validator: conway.UtxoValidateInlineDatumsWithPlutusV1,
	},
	{
		Id:        common.UtxoValidationRuleConwayFeaturesWithPlutusV1V2,
		Validator: UtxoValidateConwayFeaturesWithPlutusV1V2,
	},
	{
		Id:        common.UtxoValidationRuleDisjointRefInputs,
		Validator: UtxoValidateDisjointRefInputs,
	},
	{
		Id:        common.UtxoValidationRuleOutsideValidityInterval,
		Validator: conway.UtxoValidateOutsideValidityIntervalUtxo,
	},
	{
		Id:        common.UtxoValidationRuleInputSetEmpty,
		Validator: conway.UtxoValidateInputSetEmptyUtxo,
	},
	{
		Id:        common.UtxoValidationRuleNoDuplicateInputs,
		Validator: conway.UtxoValidateNoDuplicateInputs,
	},
	{
		Id:        common.UtxoValidationRuleFeeTooSmall,
		Validator: UtxoValidateFeeTooSmallUtxo,
	},
	{
		Id:        common.UtxoValidationRuleInsufficientCollateral,
		Validator: UtxoValidateInsufficientCollateral,
	},
	{
		Id:        common.UtxoValidationRuleCollateralContainsNonAda,
		Validator: UtxoValidateCollateralContainsNonAda,
	},
	{
		Id:        common.UtxoValidationRuleCollateralEqBalance,
		Validator: conway.UtxoValidateCollateralEqBalance,
	},
	{
		Id:        common.UtxoValidationRuleNoCollateralInputs,
		Validator: UtxoValidateNoCollateralInputs,
	},
	{
		Id:        common.UtxoValidationRuleBadInputs,
		Validator: conway.UtxoValidateBadInputsUtxo,
	},
	{
		Id:        common.UtxoValidationRuleScriptWitnesses,
		Validator: UtxoValidateScriptWitnesses,
	},
	{
		Id:        common.UtxoValidationRuleRequiredRedeemers,
		Validator: conway.UtxoValidateRequiredRedeemers,
	},
	{
		Id:        common.UtxoValidationRuleValueNotConserved,
		Validator: UtxoValidateValueNotConservedUtxo,
	},
	{
		Id:        common.UtxoValidationRuleOutputTooSmall,
		Validator: UtxoValidateOutputTooSmallUtxo,
	},
	{
		Id:        common.UtxoValidationRuleOutputTooBig,
		Validator: UtxoValidateOutputTooBigUtxo,
	},
	{
		Id:        common.UtxoValidationRuleOutputBootAddrAttrsTooBig,
		Validator: conway.UtxoValidateOutputBootAddrAttrsTooBig,
	},
	{
		Id:        common.UtxoValidationRuleWrongNetwork,
		Validator: conway.UtxoValidateWrongNetwork,
	},
	{
		Id:        common.UtxoValidationRuleWrongNetworkWithdrawal,
		Validator: conway.UtxoValidateWrongNetworkWithdrawal,
	},
	{
		Id:        common.UtxoValidationRuleTransactionNetworkId,
		Validator: UtxoValidateTransactionNetworkId,
	},
	{
		Id:        common.UtxoValidationRuleMaxTxSize,
		Validator: UtxoValidateMaxTxSizeUtxo,
	},
	{
		Id:        common.UtxoValidationRuleExUnitsTooBig,
		Validator: UtxoValidateExUnitsTooBigUtxo,
	},
	{
		Id:        common.UtxoValidationRuleTooManyCollateralInputs,
		Validator: UtxoValidateTooManyCollateralInputs,
	},
	{
		Id:        common.UtxoValidationRuleSupplementalDatums,
		Validator: UtxoValidateSupplementalDatums,
	},
	{
		Id:        common.UtxoValidationRuleExtraneousRedeemers,
		Validator: UtxoValidateExtraneousRedeemers,
	},
	{
		Id:        common.UtxoValidationRuleMalformedReferenceScripts,
		Validator: UtxoValidateMalformedReferenceScripts,
	},
	{
		Id:        common.UtxoValidationRulePlutusScripts,
		Validator: UtxoValidatePlutusScripts,
	},
	{
		Id:        common.UtxoValidationRuleNativeScripts,
		Validator: UtxoValidateNativeScripts,
	},
	{
		Id:        common.UtxoValidationRuleDelegation,
		Validator: conway.UtxoValidateDelegation,
	},
	{
		Id:        common.UtxoValidationRuleWithdrawals,
		Validator: conway.UtxoValidateWithdrawals,
	},
	{
		Id:        common.UtxoValidationRuleCertificateDeposits,
		Validator: conway.UtxoValidateCertificateDeposits,
	},
	{
		Id:        common.UtxoValidationRuleCommitteeCertificates,
		Validator: conway.UtxoValidateCommitteeCertificates,
	},
	{
		Id:        common.UtxoValidationRuleUnknownVoters,
		Validator: conway.UtxoValidateUnknownVoters,
	},
	{
		Id:        common.UtxoValidationRuleUnknownGovActionIds,
		Validator: conway.UtxoValidateUnknownGovActionIds,
	},
	{
		Id:        common.UtxoValidationRuleVotingOnExpiredGovAction,
		Validator: conway.UtxoValidateVotingOnExpiredGovAction,
	},
	{
		Id:        common.UtxoValidationRuleBootstrapVotingRestrictions,
		Validator: UtxoValidateBootstrapVotingRestrictions,
	},
	{
		Id:        common.UtxoValidationRuleStakePoolVotingRestrictions,
		Validator: conway.UtxoValidateStakePoolVotingRestrictions,
	},
	{
		Id:        common.UtxoValidationRuleCCVotingRestrictions,
		Validator: UtxoValidateCCVotingRestrictions,
	},
	{
		Id:        common.UtxoValidationRuleRefScriptSizePerTx,
		Validator: UtxoValidateRefScriptSizePerTx,
	},
	{
		Id:        common.UtxoValidationRulePoolCertificates,
		Validator: conway.UtxoValidatePoolCertificates,
	},
}

// UtxoValidationRuleDescriptors returns the authoritative ordered rule
// descriptors. The returned slice is a defensive copy and may be modified by
// callers without changing package state.
func UtxoValidationRuleDescriptors() []common.UtxoValidationRuleDescriptor {
	return append(
		[]common.UtxoValidationRuleDescriptor(nil),
		utxoValidationRuleDescriptors...,
	)
}

// UtxoValidationRules is initialized from the authoritative descriptors. It
// remains mutable for compatibility; mutations are not reflected by
// UtxoValidationRuleDescriptors.
var UtxoValidationRules = common.MustUtxoValidationRulesFromDescriptors(
	utxoValidationRuleDescriptors,
)

func dijkstraPparams(
	pp common.ProtocolParameters,
) (*DijkstraProtocolParameters, error) {
	var ret DijkstraProtocolParameters
	switch p := pp.(type) {
	case *DijkstraProtocolParameters:
		if p == nil {
			return nil, errors.New("pparams are not expected type")
		}
		ret = *p
	case *conway.ConwayProtocolParameters:
		if p == nil {
			return nil, errors.New("pparams are not expected type")
		}
		ret.ConwayProtocolParameters = *p
	default:
		return nil, errors.New("pparams are not expected type")
	}
	applyConwayRefScriptFeeDefaults(&ret)
	return &ret, nil
}

func conwayPparams(
	pp common.ProtocolParameters,
) (*conway.ConwayProtocolParameters, error) {
	switch p := pp.(type) {
	case *DijkstraProtocolParameters:
		return &p.ConwayProtocolParameters, nil
	case *conway.ConwayProtocolParameters:
		return p, nil
	default:
		return nil, errors.New("pparams are not expected type")
	}
}

func isInDijkstraBootstrapPhase(pp common.ProtocolParameters) (bool, error) {
	conwayPp, err := conwayPparams(pp)
	if err != nil {
		return false, err
	}
	major := conwayPp.ProtocolVersion.Major
	return major >= common.ProtocolVersionConway &&
		major < common.ProtocolVersionPlomin, nil
}

func UtxoValidateProposalProcedures(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	for _, proposal := range tx.ProposalProcedures() {
		govAction := proposal.GovAction()
		if isNilGovAction(govAction) {
			continue
		}
		paramChangeAction, ok := govAction.(*DijkstraParameterChangeGovAction)
		if !ok {
			continue
		}
		if err := validateDijkstraProtocolParameterUpdate(
			&paramChangeAction.ParamUpdate,
		); err != nil {
			return err
		}
	}
	return nil
}

func UtxoValidateHardForkCanFollow(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, err := conwayPparams(pp)
	if err != nil {
		return err
	}
	return conway.UtxoValidateHardForkCanFollow(tx, slot, ls, tmpPparams)
}

func UtxoValidateProposalDeposit(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, err := conwayPparams(pp)
	if err != nil {
		return err
	}
	return conway.UtxoValidateProposalDeposit(tx, slot, ls, tmpPparams)
}

func UtxoValidateBootstrapVotingRestrictions(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, err := conwayPparams(pp)
	if err != nil {
		return err
	}
	return conway.UtxoValidateBootstrapVotingRestrictions(
		tx,
		slot,
		ls,
		tmpPparams,
	)
}

func UtxoValidateBootstrapAllowedGovActions(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	inBootstrap, err := isInDijkstraBootstrapPhase(pp)
	if err != nil {
		return err
	}
	if !inBootstrap {
		return nil
	}
	for _, proposal := range tx.ProposalProcedures() {
		govAction := proposal.GovAction()
		if isNilGovAction(govAction) {
			continue
		}
		switch govAction.(type) {
		case *common.InfoGovAction:
		case *common.HardForkInitiationGovAction:
		case *DijkstraParameterChangeGovAction:
		case *common.TreasuryWithdrawalGovAction:
			return conway.BootstrapDisallowedGovActionError{
				ActionType: common.GovActionTypeTreasuryWithdrawal,
			}
		case *common.NoConfidenceGovAction:
			return conway.BootstrapDisallowedGovActionError{
				ActionType: common.GovActionTypeNoConfidence,
			}
		case *common.UpdateCommitteeGovAction:
			return conway.BootstrapDisallowedGovActionError{
				ActionType: common.GovActionTypeUpdateCommittee,
			}
		case *common.NewConstitutionGovAction:
			return conway.BootstrapDisallowedGovActionError{
				ActionType: common.GovActionTypeNewConstitution,
			}
		default:
			return fmt.Errorf("unknown governance action type %T", govAction)
		}
	}
	return nil
}

func UtxoValidateBootstrapParameterGroups(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	inBootstrap, err := isInDijkstraBootstrapPhase(pp)
	if err != nil {
		return err
	}
	if !inBootstrap {
		return nil
	}
	for _, proposal := range tx.ProposalProcedures() {
		govAction := proposal.GovAction()
		if isNilGovAction(govAction) {
			continue
		}
		switch paramChange := govAction.(type) {
		case *DijkstraParameterChangeGovAction:
			fields := paramChange.ParamUpdate.BootstrapRestrictedFields()
			if len(fields) > 0 {
				return conway.BootstrapDisallowedParameterChangeError{
					Fields: fields,
				}
			}
		}
	}
	return nil
}

func validateDijkstraProtocolParameterUpdate(
	ppu *DijkstraProtocolParameterUpdate,
) error {
	if ppu == nil || !ppu.hasUpdate() {
		return conway.ProtocolParameterUpdateEmptyError{}
	}
	if ppu.MaxBlockHeaderSize != nil && *ppu.MaxBlockHeaderSize == 0 {
		return conway.ProtocolParameterUpdateFieldZeroError{
			FieldName: "maxBHSize",
			Value:     *ppu.MaxBlockHeaderSize,
		}
	}
	if ppu.MaxTxSize != nil && *ppu.MaxTxSize == 0 {
		return conway.ProtocolParameterUpdateFieldZeroError{
			FieldName: "maxTxSize",
			Value:     *ppu.MaxTxSize,
		}
	}
	if ppu.MaxValueSize != nil && *ppu.MaxValueSize == 0 {
		return conway.ProtocolParameterUpdateFieldZeroError{
			FieldName: "maxValSize",
			Value:     *ppu.MaxValueSize,
		}
	}
	if ppu.MaxBlockBodySize != nil && *ppu.MaxBlockBodySize == 0 {
		return conway.ProtocolParameterUpdateFieldZeroError{
			FieldName: "maxBlockBodySize",
			Value:     *ppu.MaxBlockBodySize,
		}
	}
	if ppu.RefScriptCostStride != nil && *ppu.RefScriptCostStride == 0 {
		return conway.ProtocolParameterUpdateFieldZeroError{
			FieldName: "refScriptCostStride",
			Value:     uint(*ppu.RefScriptCostStride),
		}
	}
	return validateLeiosCommitteeStakeParameters(
		ppu.CommitteeStakeCoverage,
		ppu.QuorumStakeThreshold,
	)
}

func UtxoValidateDisjointRefInputs(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, err := conwayPparams(pp)
	if err != nil {
		return err
	}
	return conway.UtxoValidateDisjointRefInputs(tx, slot, ls, tmpPparams)
}

// dijkstraConwayFeatureTransaction presents one sub-transaction's body and
// witnesses while retaining the enclosing transaction's unrelated methods.
// Script purposes remain scoped to this body; callers can aggregate script
// availability across transaction levels separately.
type dijkstraConwayFeatureTransaction struct {
	common.Transaction
	body      common.TransactionBody
	witnesses common.TransactionWitnessSet
}

func (t dijkstraConwayFeatureTransaction) Inputs() []common.TransactionInput {
	return t.body.Inputs()
}

func (t dijkstraConwayFeatureTransaction) Outputs() []common.TransactionOutput {
	return t.body.Outputs()
}

func (t dijkstraConwayFeatureTransaction) Fee() *big.Int {
	return t.body.Fee()
}

func (t dijkstraConwayFeatureTransaction) Id() common.Blake2b256 {
	return t.body.Id()
}

func (t dijkstraConwayFeatureTransaction) Hash() common.Blake2b256 {
	return t.body.Id()
}

func (t dijkstraConwayFeatureTransaction) TTL() uint64 {
	return t.body.TTL()
}

func (t dijkstraConwayFeatureTransaction) ValidityIntervalUpperBound() (uint64, bool) {
	return common.TransactionValidityIntervalUpperBound(t.body)
}

func (t dijkstraConwayFeatureTransaction) ValidityIntervalStart() uint64 {
	return t.body.ValidityIntervalStart()
}

func (t dijkstraConwayFeatureTransaction) ReferenceInputs() []common.TransactionInput {
	return t.body.ReferenceInputs()
}

func (t dijkstraConwayFeatureTransaction) Collateral() []common.TransactionInput {
	return t.body.Collateral()
}

func (t dijkstraConwayFeatureTransaction) CollateralReturn() common.TransactionOutput {
	return t.body.CollateralReturn()
}

func (t dijkstraConwayFeatureTransaction) TotalCollateral() *big.Int {
	return t.body.TotalCollateral()
}

func (t dijkstraConwayFeatureTransaction) Witnesses() common.TransactionWitnessSet {
	return t.witnesses
}

func (t dijkstraConwayFeatureTransaction) AuxDataHash() *common.Blake2b256 {
	return t.body.AuxDataHash()
}

func (t dijkstraConwayFeatureTransaction) RequiredSigners() []common.Blake2b224 {
	return t.body.RequiredSigners()
}

func (t dijkstraConwayFeatureTransaction) ScriptDataHash() *common.Blake2b256 {
	return t.body.ScriptDataHash()
}

func (t dijkstraConwayFeatureTransaction) CurrentTreasuryValue() *big.Int {
	return t.body.CurrentTreasuryValue()
}

func (t dijkstraConwayFeatureTransaction) CurrentTreasuryValuePresent() bool {
	return common.TransactionCurrentTreasuryValuePresent(t.body)
}

func (t dijkstraConwayFeatureTransaction) ProposalProcedures() []common.ProposalProcedure {
	return t.body.ProposalProcedures()
}

func (t dijkstraConwayFeatureTransaction) VotingProcedures() common.VotingProcedures {
	return t.body.VotingProcedures()
}

func (t dijkstraConwayFeatureTransaction) Certificates() []common.Certificate {
	return t.body.Certificates()
}

func (t dijkstraConwayFeatureTransaction) Withdrawals() map[*common.Address]*big.Int {
	return t.body.Withdrawals()
}

func (t dijkstraConwayFeatureTransaction) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return t.body.AssetMint()
}

func (t dijkstraConwayFeatureTransaction) Donation() *big.Int {
	return t.body.Donation()
}

func (t dijkstraConwayFeatureTransaction) Consumed() []common.TransactionInput {
	if t.IsValid() {
		return t.Inputs()
	}
	return t.Collateral()
}

func (t dijkstraConwayFeatureTransaction) Produced() []common.Utxo {
	outputs := t.Outputs()
	ret := make([]common.Utxo, 0, len(outputs))
	for idx, output := range outputs {
		ret = append(ret, common.Utxo{
			Id:     shelley.NewShelleyTransactionInput(t.Id().String(), idx),
			Output: output,
		})
	}
	return ret
}

// GuardingCredentials exposes only this transaction level's guards to the
// shared script-purpose resolver. Script availability is aggregated across
// levels separately by UtxoValidateConwayFeaturesWithPlutusV1V2.
func (t dijkstraConwayFeatureTransaction) GuardingCredentials() []common.Credential {
	switch body := t.body.(type) {
	case *DijkstraTransactionBody:
		if body.TxGuards != nil {
			return body.TxGuards.Credentials
		}
	case *DijkstraSubTransactionBody:
		if body.TxGuards != nil {
			return body.TxGuards.Credentials
		}
	}
	return nil
}

func dijkstraTransactionLevels(
	tx *DijkstraTransaction,
) []dijkstraConwayFeatureTransaction {
	subTxs := tx.Body.TxSubTransactions.Items()
	levels := make([]dijkstraConwayFeatureTransaction, 0, len(subTxs)+1)
	for idx := range subTxs {
		levels = append(levels, dijkstraConwayFeatureTransaction{
			Transaction: tx,
			body:        &subTxs[idx].Body,
			witnesses:   subTxs[idx].WitnessSet,
		})
	}
	return append(levels, dijkstraConwayFeatureTransaction{
		Transaction: tx,
		body:        &tx.Body,
		witnesses:   tx.WitnessSet,
	})
}

// UtxoValidateIsValidFlag accepts a phase-2-invalid Dijkstra transaction when
// any transaction level supplies a redeemer.
func UtxoValidateIsValidFlag(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok {
		return conway.UtxoValidateIsValidFlag(tx, slot, ls, pp)
	}
	if tx.IsValid() {
		return nil
	}
	for _, level := range dijkstraTransactionLevels(dijkstraTx) {
		wits := level.Witnesses()
		if wits == nil || wits.Redeemers() == nil {
			continue
		}
		for range wits.Redeemers().Iter() {
			return nil
		}
	}
	return common.InvalidIsValidFlagError{}
}

// UtxoValidateRequiredVKeyWitnesses validates required signers independently
// at the transaction level where the requirement occurs.
func UtxoValidateRequiredVKeyWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok {
		return conway.UtxoValidateRequiredVKeyWitnesses(tx, slot, ls, pp)
	}
	for _, level := range dijkstraTransactionLevels(dijkstraTx) {
		if err := common.ValidateRequiredVKeyWitnesses(level); err != nil {
			return err
		}
	}
	return nil
}

// UtxoValidateSignatures verifies each transaction level's witnesses against
// that level's body hash.
func UtxoValidateSignatures(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok {
		return conway.UtxoValidateSignatures(tx, slot, ls, pp)
	}
	for _, level := range dijkstraTransactionLevels(dijkstraTx) {
		if err := common.UtxoValidateSignatures(
			level,
			slot,
			ls,
			pp,
		); err != nil {
			return err
		}
	}
	return nil
}

type dijkstraScriptLevel struct {
	tx         dijkstraConwayFeatureTransaction
	resolved   []common.Utxo
	view       script.TxScriptView
	slotState  common.SlotState
	subTxIndex *uint32
}

func dijkstraScriptLevels(
	tx *DijkstraTransaction,
	ls common.LedgerState,
) ([]dijkstraScriptLevel, map[common.ScriptHash]common.Script, error) {
	txLevels := dijkstraTransactionLevels(tx)
	levels := make([]dijkstraScriptLevel, 0, len(txLevels))
	available := make(map[common.ScriptHash]common.Script)
	for levelIndex, txLevel := range txLevels {
		addDijkstraWitnessScripts(available, txLevel.Witnesses())
		var inputs, refInputs, resolved []common.Utxo
		if len(txLevel.Inputs())+len(txLevel.ReferenceInputs()) > 0 {
			if ls == nil {
				return nil, nil, errors.New(
					"ledger state is required for Dijkstra script validation",
				)
			}
			var err error
			inputs, refInputs, err = script.ResolveTxInputs(txLevel, ls)
			if err != nil {
				return nil, nil, err
			}
			resolved = script.ConcatResolvedInputs(inputs, refInputs)
			for _, utxo := range resolved {
				if utxo.Output == nil || utxo.Output.ScriptRef() == nil {
					continue
				}
				candidate := utxo.Output.ScriptRef()
				available[candidate.Hash()] = candidate
			}
		}
		level := dijkstraScriptLevel{
			tx:        txLevel,
			resolved:  resolved,
			slotState: ls,
			view: script.TxScriptView{
				ResolvedInputs:          inputs,
				ResolvedReferenceInputs: refInputs,
			},
		}
		if levelIndex < len(txLevels)-1 {
			idx := uint32(
				levelIndex,
			) // #nosec G115 -- bounded by transaction size
			level.subTxIndex = &idx
		}
		levels = append(levels, level)
	}
	for idx := range levels {
		levels[idx].view = levels[idx].view.WithAvailableScripts(
			levels[idx].tx,
			available,
		)
	}
	return levels, available, nil
}

// dijkstraWitnessRuleLevels builds the global script view used by phase-1
// witness predicates. Missing consumed inputs are left to BadInputs, while a
// missing reference input remains a reference-input resolution failure.
func dijkstraWitnessRuleLevels(
	tx *DijkstraTransaction,
	ls common.LedgerState,
) ([]dijkstraScriptLevel, map[common.ScriptHash]common.Script, error) {
	txLevels := dijkstraTransactionLevels(tx)
	levels := make([]dijkstraScriptLevel, 0, len(txLevels))
	available := make(map[common.ScriptHash]common.Script)
	for levelIndex, txLevel := range txLevels {
		addDijkstraWitnessScripts(available, txLevel.Witnesses())
		var inputs, refInputs []common.Utxo
		if ls != nil {
			for _, input := range txLevel.Inputs() {
				utxo, err := ls.UtxoById(input)
				if err != nil {
					continue
				}
				inputs = append(inputs, utxo)
			}
			for _, input := range txLevel.ReferenceInputs() {
				utxo, err := ls.UtxoById(input)
				if err != nil {
					return nil, nil, common.ReferenceInputResolutionError{
						Input: input,
						Err:   err,
					}
				}
				refInputs = append(refInputs, utxo)
			}
		}
		resolved := script.ConcatResolvedInputs(inputs, refInputs)
		for _, utxo := range resolved {
			if utxo.Output == nil || utxo.Output.ScriptRef() == nil {
				continue
			}
			candidate := utxo.Output.ScriptRef()
			available[candidate.Hash()] = candidate
		}
		level := dijkstraScriptLevel{
			tx:       txLevel,
			resolved: resolved,
			view: script.TxScriptView{
				ResolvedInputs:          inputs,
				ResolvedReferenceInputs: refInputs,
			},
			slotState: ls,
		}
		if levelIndex < len(txLevels)-1 {
			idx := uint32(levelIndex) // #nosec G115 -- bounded by tx size
			level.subTxIndex = &idx
		}
		levels = append(levels, level)
	}
	for idx := range levels {
		levels[idx].view = levels[idx].view.WithAvailableScripts(
			levels[idx].tx,
			available,
		)
	}
	return levels, available, nil
}

func addDijkstraWitnessScripts(
	available map[common.ScriptHash]common.Script,
	wits common.TransactionWitnessSet,
) {
	if wits == nil {
		return
	}
	for _, candidate := range wits.NativeScripts() {
		available[candidate.Hash()] = candidate
	}
	for _, candidate := range wits.PlutusV1Scripts() {
		available[candidate.Hash()] = candidate
	}
	for _, candidate := range wits.PlutusV2Scripts() {
		available[candidate.Hash()] = candidate
	}
	for _, candidate := range wits.PlutusV3Scripts() {
		available[candidate.Hash()] = candidate
	}
	for _, candidate := range common.PlutusV4ScriptsFromWitnessSet(wits) {
		available[candidate.Hash()] = candidate
	}
}

// UtxoValidateConwayFeaturesWithPlutusV1V2 applies the complete Conway
// compatibility predicate to each Dijkstra sub-transaction before the
// top-level transaction. Script availability is shared across every level,
// matching Dijkstra script resolution, while each level computes its own
// needed scripts and checks only its own Conway features.
func UtxoValidateConwayFeaturesWithPlutusV1V2(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok {
		return conway.UtxoValidateConwayFeaturesWithPlutusV1V2(
			tx,
			slot,
			ls,
			pp,
		)
	}

	type levelView struct {
		tx   dijkstraConwayFeatureTransaction
		view script.TxScriptView
	}
	txLevels := dijkstraTransactionLevels(dijkstraTx)
	levels := make([]levelView, 0, len(txLevels))
	for _, txLevel := range txLevels {
		levels = append(levels, levelView{tx: txLevel})
	}

	available := make(map[common.ScriptHash]common.Script)
	for idx := range levels {
		view, err := script.NewTxScriptView(levels[idx].tx, ls)
		if err != nil {
			if !errors.Is(err, common.ErrInputResolution) {
				return err
			}
		}
		levels[idx].view = view
		for hash, candidate := range view.Available {
			available[hash] = candidate
		}
	}

	for idx := range levels {
		view := levels[idx].view.WithAvailableScripts(
			levels[idx].tx,
			available,
		)
		if err := conway.ValidateConwayFeaturesWithPlutusV1V2(
			levels[idx].tx,
			view,
		); err != nil {
			return err
		}
	}
	return nil
}

func UtxoValidateValueNotConservedUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, err := conwayPparams(pp)
	if err != nil {
		return err
	}
	return conway.UtxoValidateValueNotConservedUtxo(tx, slot, ls, tmpPparams)
}

func UtxoValidateCCVotingRestrictions(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, err := conwayPparams(pp)
	if err != nil {
		return err
	}
	return conway.UtxoValidateCCVotingRestrictions(tx, slot, ls, tmpPparams)
}

func UtxoValidatePlutusScripts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, err := conwayPparams(pp)
	if err != nil {
		return err
	}
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok {
		return conway.UtxoValidatePlutusScripts(tx, slot, ls, tmpPparams)
	}
	levels, available, err := dijkstraScriptLevels(dijkstraTx, ls)
	if err != nil {
		return err
	}
	for _, level := range levels {
		if level.subTxIndex == nil {
			continue
		}
		for _, candidate := range level.view.Needed {
			version, ok := common.PlutusScriptVersion(candidate)
			if ok && version < 3 {
				return UnsupportedScriptInSubtransactionError{
					Version:             version,
					SubtransactionIndex: *level.subTxIndex,
					TransactionId:       level.tx.Id(),
				}
			}
		}
	}
	for _, level := range levels {
		for _, required := range dijkstraRequiredScriptPurposes(level) {
			hash := required.purpose.ScriptHash()
			if _, ok := available[hash]; !ok {
				return common.MissingScriptWitnessesError{ScriptHash: hash}
			}
		}
	}
	for _, level := range levels {
		if err := validateDijkstraPlutusRedeemers(
			level,
			available,
		); err != nil {
			return err
		}
	}
	if !tx.IsValid() {
		return nil
	}
	for _, level := range levels {
		v4Keys, err := dijkstraPlutusV4RedeemerKeys(level, available)
		if err != nil {
			return err
		}
		levelTx := transactionWithAvailablePlutusScripts{
			Transaction: level.tx,
			available:   available,
		}
		if err := conway.UtxoValidatePlutusScripts(
			transactionWithoutRedeemers{
				Transaction: levelTx,
				excluded:    v4Keys,
				guarding:    true,
			},
			slot,
			ls,
			tmpPparams,
		); err != nil {
			return err
		}
		if level.subTxIndex == nil {
			if err := validateGuardingPlutusScripts(
				level.tx,
				ls,
				tmpPparams,
				available,
				level.resolved,
			); err != nil {
				return err
			}
		}
		if err := validateDijkstraPlutusV4Scripts(
			level,
			tmpPparams,
			available,
			v4Keys,
		); err != nil {
			return err
		}
	}
	return nil
}

type dijkstraRequiredScriptPurpose struct {
	key     common.RedeemerKey
	purpose script.ScriptPurpose
}

func dijkstraRequiredPlutusPurposes(
	level dijkstraScriptLevel,
	available map[common.ScriptHash]common.Script,
) []dijkstraRequiredScriptPurpose {
	allPurposes := dijkstraRequiredScriptPurposes(level)
	ret := make([]dijkstraRequiredScriptPurpose, 0, len(allPurposes))
	for _, required := range allPurposes {
		candidate, ok := available[required.purpose.ScriptHash()]
		if !ok {
			continue
		}
		if _, ok := common.PlutusScriptVersion(candidate); !ok {
			continue
		}
		ret = append(ret, required)
	}
	return ret
}

func dijkstraRequiredScriptPurposes(
	level dijkstraScriptLevel,
) []dijkstraRequiredScriptPurpose {
	ret := make([]dijkstraRequiredScriptPurpose, 0)
	appendPurpose := func(
		key common.RedeemerKey,
		purpose script.ScriptPurpose,
	) {
		if purpose == nil || purpose.ScriptHash() == (common.ScriptHash{}) {
			return
		}
		ret = append(ret, dijkstraRequiredScriptPurpose{
			key:     key,
			purpose: purpose,
		})
	}

	resolved := make(map[string]common.Utxo, len(level.view.ResolvedInputs))
	for _, utxo := range level.view.ResolvedInputs {
		if utxo.Id != nil {
			resolved[utxo.Id.String()] = utxo
		}
	}
	for idx, input := range script.SortInputs(level.tx.Inputs()) {
		utxo, ok := resolved[input.String()]
		if !ok || utxo.Output == nil ||
			utxo.Output.Address().Type()&common.AddressTypeScriptBit == 0 {
			continue
		}
		appendPurpose(
			common.RedeemerKey{
				Tag: common.RedeemerTagSpend,
				Index: uint32(
					idx,
				), // #nosec G115 -- bounded by transaction size
			},
			script.ScriptPurposeSpending{Input: utxo},
		)
	}

	if mint := level.tx.AssetMint(); mint != nil {
		policies := mint.Policies()
		slices.SortFunc(policies, func(a, b common.Blake2b224) int {
			return bytes.Compare(a.Bytes(), b.Bytes())
		})
		for idx, policy := range policies {
			appendPurpose(
				common.RedeemerKey{
					Tag: common.RedeemerTagMint,
					Index: uint32(
						idx,
					), // #nosec G115 -- bounded by transaction size
				},
				script.ScriptPurposeMinting{PolicyId: policy},
			)
		}
	}

	for idx, certificate := range level.tx.Certificates() {
		appendPurpose(
			common.RedeemerKey{
				Tag: common.RedeemerTagCert,
				Index: uint32(
					idx,
				), // #nosec G115 -- bounded by transaction size
			},
			script.ScriptPurposeCertifying{
				Index: uint32(
					idx,
				), // #nosec G115 -- bounded by transaction size
				Certificate: certificate,
			},
		)
	}

	withdrawals := make([]*common.Address, 0, len(level.tx.Withdrawals()))
	for address := range level.tx.Withdrawals() {
		withdrawals = append(withdrawals, address)
	}
	slices.SortFunc(withdrawals, func(a, b *common.Address) int {
		if a == nil {
			return -1
		}
		if b == nil {
			return 1
		}
		aBytes, aErr := a.Bytes()
		bBytes, bErr := b.Bytes()
		if aErr != nil || bErr != nil {
			return strings.Compare(a.String(), b.String())
		}
		return bytes.Compare(aBytes, bBytes)
	})
	for idx, address := range withdrawals {
		if address == nil ||
			address.Type()&common.AddressTypeScriptBit == 0 {
			continue
		}
		appendPurpose(
			common.RedeemerKey{
				Tag: common.RedeemerTagReward,
				Index: uint32(
					idx,
				), // #nosec G115 -- bounded by transaction size
			},
			script.ScriptPurposeRewarding{
				StakeCredential: common.Credential{
					CredType:   common.CredentialTypeScriptHash,
					Credential: address.StakeKeyHash(),
				},
			},
		)
	}

	voters := make([]*common.Voter, 0, len(level.tx.VotingProcedures()))
	for voter := range level.tx.VotingProcedures() {
		voters = append(voters, voter)
	}
	slices.SortFunc(voters, func(a, b *common.Voter) int {
		if a == nil {
			return -1
		}
		if b == nil {
			return 1
		}
		aTag := dijkstraVoterTag(a)
		bTag := dijkstraVoterTag(b)
		if aTag != bTag {
			return aTag - bTag
		}
		return bytes.Compare(a.Hash[:], b.Hash[:])
	})
	for idx, voter := range voters {
		if voter == nil || !dijkstraVoterUsesScriptCredential(*voter) {
			continue
		}
		appendPurpose(
			common.RedeemerKey{
				Tag: common.RedeemerTagVoting,
				Index: uint32(
					idx,
				), // #nosec G115 -- bounded by transaction size
			},
			script.ScriptPurposeVoting{Voter: *voter},
		)
	}

	for idx, proposal := range level.tx.ProposalProcedures() {
		appendPurpose(
			common.RedeemerKey{
				Tag: common.RedeemerTagProposing,
				Index: uint32(
					idx,
				), // #nosec G115 -- bounded by transaction size
			},
			script.ScriptPurposeProposing{
				Index: uint32(
					idx,
				), // #nosec G115 -- bounded by transaction size
				ProposalProcedure: proposal,
			},
		)
	}

	for idx, guard := range level.tx.GuardingCredentials() {
		appendPurpose(
			common.RedeemerKey{
				Tag: common.RedeemerTagGuarding,
				Index: uint32(
					idx,
				), // #nosec G115 -- bounded by transaction size
			},
			script.ScriptPurposeGuarding{Guard: guard},
		)
	}
	return ret
}

func validateDijkstraPlutusRedeemers(
	level dijkstraScriptLevel,
	available map[common.ScriptHash]common.Script,
) error {
	requiredPurposes := dijkstraRequiredPlutusPurposes(level, available)
	required := make(map[common.RedeemerKey]struct{}, len(requiredPurposes))
	for _, purpose := range requiredPurposes {
		required[purpose.key] = struct{}{}
	}
	provided := make(map[common.RedeemerKey]struct{})
	if wits := level.tx.Witnesses(); wits != nil {
		if redeemers := wits.Redeemers(); redeemers != nil {
			for key := range redeemers.Iter() {
				provided[key] = struct{}{}
				if _, ok := required[key]; !ok {
					return conway.ExtraRedeemerError{RedeemerKey: key}
				}
			}
		}
	}
	for _, purpose := range requiredPurposes {
		if _, ok := provided[purpose.key]; ok {
			continue
		}
		return conway.MissingRedeemerForScriptError{
			ScriptHash: purpose.purpose.ScriptHash(),
			Tag:        purpose.key.Tag,
			Index:      purpose.key.Index,
		}
	}
	return nil
}

func dijkstraVoterUsesScriptCredential(voter common.Voter) bool {
	switch voter.Type {
	case common.VoterTypeConstitutionalCommitteeHotScriptHash,
		common.VoterTypeDRepScriptHash:
		return true
	default:
		return false
	}
}

func dijkstraVoterTag(voter *common.Voter) int {
	switch voter.Type {
	case common.VoterTypeConstitutionalCommitteeHotScriptHash:
		return 0
	case common.VoterTypeConstitutionalCommitteeHotKeyHash:
		return 1
	case common.VoterTypeDRepScriptHash:
		return 2
	case common.VoterTypeDRepKeyHash:
		return 3
	case common.VoterTypeStakingPoolKeyHash:
		return 4
	default:
		return -1
	}
}

func dijkstraPlutusV4RedeemerKeys(
	level dijkstraScriptLevel,
	available map[common.ScriptHash]common.Script,
) (map[common.RedeemerKey]struct{}, error) {
	ret := make(map[common.RedeemerKey]struct{})
	wits := level.tx.Witnesses()
	if wits == nil || wits.Redeemers() == nil {
		return ret, nil
	}
	for key := range wits.Redeemers().Iter() {
		purpose, err := dijkstraPurposeForKey(level, key)
		if err != nil {
			return nil, conway.ExtraRedeemerError{RedeemerKey: key}
		}
		if _, ok := available[purpose.ScriptHash()].(common.PlutusV4Script); ok {
			ret[key] = struct{}{}
		}
	}
	return ret, nil
}

func validateDijkstraPlutusV4Scripts(
	level dijkstraScriptLevel,
	pp *conway.ConwayProtocolParameters,
	available map[common.ScriptHash]common.Script,
	keys map[common.RedeemerKey]struct{},
) error {
	if len(keys) == 0 {
		return nil
	}
	wits := level.tx.Witnesses()
	if wits == nil {
		return nil
	}
	redeemers := wits.Redeemers()
	if redeemers == nil {
		return nil
	}
	evalContext, err := cek.NewEvalContext(
		lang.LanguageVersionV4,
		cek.ProtoVersion{
			Major: pp.ProtocolVersion.Major,
			Minor: pp.ProtocolVersion.Minor,
		},
		pp.CostModels[3],
	)
	if err != nil {
		return fmt.Errorf("build Plutus V4 evaluation context: %w", err)
	}
	for key, value := range redeemers.Iter() {
		if _, ok := keys[key]; !ok {
			continue
		}
		purpose, err := dijkstraPurposeForKey(level, key)
		if err != nil {
			return conway.ExtraRedeemerError{RedeemerKey: key}
		}
		candidate, ok := available[purpose.ScriptHash()].(common.PlutusV4Script)
		if !ok {
			return common.MissingScriptWitnessesError{
				ScriptHash: purpose.ScriptHash(),
			}
		}
		context, err := dijkstraPlutusV4Context(level, purpose, key, value)
		if err != nil {
			return conway.ScriptContextConstructionError{Err: err}
		}
		if _, err := candidate.Evaluate(context, value.ExUnits, evalContext); err != nil {
			return conway.PlutusScriptFailedError{
				ScriptHash: purpose.ScriptHash(),
				Tag:        key.Tag,
				Index:      key.Index,
				Err:        err,
			}
		}
	}
	return nil
}

type transactionWithAvailablePlutusScripts struct {
	common.Transaction
	available map[common.ScriptHash]common.Script
}

func (t transactionWithAvailablePlutusScripts) Witnesses() common.TransactionWitnessSet {
	return witnessSetWithAvailablePlutusScripts{
		TransactionWitnessSet: t.Transaction.Witnesses(),
		available:             t.available,
	}
}

type witnessSetWithAvailablePlutusScripts struct {
	common.TransactionWitnessSet
	available map[common.ScriptHash]common.Script
}

func (w witnessSetWithAvailablePlutusScripts) Vkey() []common.VkeyWitness {
	if w.TransactionWitnessSet == nil {
		return nil
	}
	return w.TransactionWitnessSet.Vkey()
}

func (w witnessSetWithAvailablePlutusScripts) NativeScripts() []common.NativeScript {
	if w.TransactionWitnessSet == nil {
		return nil
	}
	return w.TransactionWitnessSet.NativeScripts()
}

func (w witnessSetWithAvailablePlutusScripts) Bootstrap() []common.BootstrapWitness {
	if w.TransactionWitnessSet == nil {
		return nil
	}
	return w.TransactionWitnessSet.Bootstrap()
}

func (w witnessSetWithAvailablePlutusScripts) PlutusData() []common.Datum {
	if w.TransactionWitnessSet == nil {
		return nil
	}
	return w.TransactionWitnessSet.PlutusData()
}

func (w witnessSetWithAvailablePlutusScripts) Redeemers() common.TransactionWitnessRedeemers {
	if w.TransactionWitnessSet == nil {
		return nil
	}
	return w.TransactionWitnessSet.Redeemers()
}

func (w witnessSetWithAvailablePlutusScripts) PlutusV1Scripts() []common.PlutusV1Script {
	ret := make([]common.PlutusV1Script, 0)
	for _, candidate := range w.available {
		if plutusScript, ok := candidate.(common.PlutusV1Script); ok {
			ret = append(ret, plutusScript)
		}
	}
	return ret
}

func (w witnessSetWithAvailablePlutusScripts) PlutusV2Scripts() []common.PlutusV2Script {
	ret := make([]common.PlutusV2Script, 0)
	for _, candidate := range w.available {
		if plutusScript, ok := candidate.(common.PlutusV2Script); ok {
			ret = append(ret, plutusScript)
		}
	}
	return ret
}

func (w witnessSetWithAvailablePlutusScripts) PlutusV3Scripts() []common.PlutusV3Script {
	ret := make([]common.PlutusV3Script, 0)
	for _, candidate := range w.available {
		if plutusScript, ok := candidate.(common.PlutusV3Script); ok {
			ret = append(ret, plutusScript)
		}
	}
	return ret
}

func (w witnessSetWithAvailablePlutusScripts) PlutusV4Scripts() []common.PlutusV4Script {
	ret := make([]common.PlutusV4Script, 0)
	for _, candidate := range w.available {
		if plutusScript, ok := candidate.(common.PlutusV4Script); ok {
			ret = append(ret, plutusScript)
		}
	}
	return ret
}

type transactionWithoutGuardingRedeemers struct {
	common.Transaction
}

type transactionWithoutRedeemers struct {
	common.Transaction
	excluded map[common.RedeemerKey]struct{}
	guarding bool
}

func (t transactionWithoutRedeemers) Witnesses() common.TransactionWitnessSet {
	wits := t.Transaction.Witnesses()
	if wits == nil {
		return nil
	}
	return witnessSetWithoutRedeemers{
		TransactionWitnessSet: wits,
		excluded:              t.excluded,
		guarding:              t.guarding,
	}
}

type witnessSetWithoutRedeemers struct {
	common.TransactionWitnessSet
	excluded map[common.RedeemerKey]struct{}
	guarding bool
}

func (w witnessSetWithoutRedeemers) PlutusV4Scripts() []common.PlutusV4Script {
	return common.PlutusV4ScriptsFromWitnessSet(w.TransactionWitnessSet)
}

func (w witnessSetWithoutRedeemers) Redeemers() common.TransactionWitnessRedeemers {
	redeemers := w.TransactionWitnessSet.Redeemers()
	if redeemers == nil {
		return nil
	}
	return filteredDijkstraRedeemers{
		TransactionWitnessRedeemers: redeemers,
		excluded:                    w.excluded,
		guarding:                    w.guarding,
	}
}

type filteredDijkstraRedeemers struct {
	common.TransactionWitnessRedeemers
	excluded map[common.RedeemerKey]struct{}
	guarding bool
}

func (r filteredDijkstraRedeemers) excludedKey(key common.RedeemerKey) bool {
	if r.guarding && key.Tag == common.RedeemerTagGuarding {
		return true
	}
	_, ok := r.excluded[key]
	return ok
}

func (r filteredDijkstraRedeemers) Indexes(tag common.RedeemerTag) []uint {
	ret := make([]uint, 0)
	for key := range r.Iter() {
		if key.Tag == tag {
			ret = append(ret, uint(key.Index))
		}
	}
	return ret
}

func (r filteredDijkstraRedeemers) Value(
	index uint,
	tag common.RedeemerTag,
) common.RedeemerValue {
	key := common.RedeemerKey{Tag: tag, Index: uint32(index)} // #nosec G115
	if r.excludedKey(key) {
		return common.RedeemerValue{}
	}
	return r.TransactionWitnessRedeemers.Value(index, tag)
}

func (r filteredDijkstraRedeemers) Iter() iter.Seq2[common.RedeemerKey, common.RedeemerValue] {
	return func(yield func(common.RedeemerKey, common.RedeemerValue) bool) {
		for key, value := range r.TransactionWitnessRedeemers.Iter() {
			if r.excludedKey(key) {
				continue
			}
			if !yield(key, value) {
				return
			}
		}
	}
}

func (t transactionWithoutGuardingRedeemers) Witnesses() common.TransactionWitnessSet {
	wits := t.Transaction.Witnesses()
	if wits == nil {
		return nil
	}
	return witnessSetWithoutGuardingRedeemers{TransactionWitnessSet: wits}
}

type witnessSetWithoutGuardingRedeemers struct {
	common.TransactionWitnessSet
}

func (w witnessSetWithoutGuardingRedeemers) PlutusV4Scripts() []common.PlutusV4Script {
	return common.PlutusV4ScriptsFromWitnessSet(w.TransactionWitnessSet)
}

func (w witnessSetWithoutGuardingRedeemers) Redeemers() common.TransactionWitnessRedeemers {
	redeemers := w.TransactionWitnessSet.Redeemers()
	if redeemers == nil {
		return nil
	}
	return redeemersWithoutGuarding{TransactionWitnessRedeemers: redeemers}
}

type redeemersWithoutGuarding struct {
	common.TransactionWitnessRedeemers
}

func (r redeemersWithoutGuarding) Indexes(tag common.RedeemerTag) []uint {
	if tag == common.RedeemerTagGuarding {
		return nil
	}
	return r.TransactionWitnessRedeemers.Indexes(tag)
}

func (r redeemersWithoutGuarding) Value(
	index uint,
	tag common.RedeemerTag,
) common.RedeemerValue {
	if tag == common.RedeemerTagGuarding {
		return common.RedeemerValue{}
	}
	return r.TransactionWitnessRedeemers.Value(index, tag)
}

func (r redeemersWithoutGuarding) Iter() iter.Seq2[common.RedeemerKey, common.RedeemerValue] {
	return func(yield func(common.RedeemerKey, common.RedeemerValue) bool) {
		for key, value := range r.TransactionWitnessRedeemers.Iter() {
			if key.Tag == common.RedeemerTagGuarding {
				continue
			}
			if !yield(key, value) {
				return
			}
		}
	}
}

func validateGuardingPlutusScripts(
	tx common.Transaction,
	ls common.LedgerState,
	pp *conway.ConwayProtocolParameters,
	availableScripts map[common.ScriptHash]common.Script,
	resolvedInputs []common.Utxo,
) error {
	wits := tx.Witnesses()
	if wits == nil || wits.Redeemers() == nil {
		return nil
	}

	var txInfoV1 script.TxInfoV1
	var txInfoV2 script.TxInfoV2
	var txInfoV3 script.TxInfoV3
	var txInfoV1Built, txInfoV2Built, txInfoV3Built bool

	for redeemerKey, redeemerValue := range wits.Redeemers().Iter() {
		if redeemerKey.Tag != common.RedeemerTagGuarding {
			continue
		}
		purpose, ok := dijkstraGuardingPurpose(tx, redeemerKey)
		if !ok {
			return conway.ExtraRedeemerError{RedeemerKey: redeemerKey}
		}
		scriptHash := purpose.ScriptHash()
		plutusScript, ok := availableScripts[scriptHash]
		if !ok {
			return common.MissingScriptWitnessesError{ScriptHash: scriptHash}
		}
		if _, ok := plutusScript.(common.NativeScript); ok {
			return conway.ExtraRedeemerError{RedeemerKey: redeemerKey}
		}
		if ls == nil {
			return errors.New(
				"ledger state is required for Dijkstra guarding Plutus validation",
			)
		}

		var execErr error
		switch s := plutusScript.(type) {
		case common.PlutusV4Script:
			// V4 guarding scripts are evaluated with the Dijkstra V4 context by
			// validateDijkstraPlutusV4Scripts.
			continue
		case common.PlutusV3Script:
			if !txInfoV3Built {
				var err error
				txInfoV3, err = script.NewTxInfoV3FromTransaction(
					ls,
					transactionWithoutGuardingRedeemers{Transaction: tx},
					resolvedInputs,
				)
				if err != nil {
					return conway.ScriptContextConstructionError{Err: err}
				}
				txInfoV3Built = true
			}
			ctx := script.NewScriptContextV3(
				txInfoV3,
				guardingRedeemer(redeemerKey, redeemerValue),
				purpose,
			)
			evalContext, err := cek.NewEvalContext(
				lang.LanguageVersionV3,
				cek.ProtoVersion{
					Major: pp.ProtocolVersion.Major,
					Minor: pp.ProtocolVersion.Minor,
				},
				pp.CostModels[2],
			)
			if err != nil {
				return fmt.Errorf("build evaluation context: %w", err)
			}
			_, execErr = s.Evaluate(ctx.ToPlutusData(), redeemerValue.ExUnits, evalContext)
		case common.PlutusV2Script:
			if !txInfoV2Built {
				var err error
				txInfoV2, err = script.NewTxInfoV2FromTransaction(
					ls,
					transactionWithoutGuardingRedeemers{Transaction: tx},
					resolvedInputs,
					script.StrictValidityUpperBoundForTransaction(tx),
				)
				if err != nil {
					return conway.ScriptContextConstructionError{Err: err}
				}
				txInfoV2Built = true
			}
			ctx := script.NewScriptContextV1V2(txInfoV2, purpose)
			evalContext, err := cek.NewEvalContext(
				lang.LanguageVersionV2,
				cek.ProtoVersion{
					Major: pp.ProtocolVersion.Major,
					Minor: pp.ProtocolVersion.Minor,
				},
				pp.CostModels[1],
			)
			if err != nil {
				return fmt.Errorf("build evaluation context: %w", err)
			}
			var datum data.PlutusData
			_, execErr = s.Evaluate(
				datum,
				redeemerValue.Data.Data,
				ctx.ToPlutusData(),
				redeemerValue.ExUnits,
				evalContext,
			)
		case common.PlutusV1Script:
			if !txInfoV1Built {
				var err error
				txInfoV1, err = script.NewTxInfoV1FromTransaction(
					ls,
					transactionWithoutGuardingRedeemers{Transaction: tx},
					resolvedInputs,
					script.StrictValidityUpperBoundForTransaction(tx),
				)
				if err != nil {
					return conway.ScriptContextConstructionError{Err: err}
				}
				txInfoV1Built = true
			}
			ctx := script.NewScriptContextV1V2(txInfoV1, purpose)
			evalContext, err := cek.NewEvalContext(
				lang.LanguageVersionV1,
				cek.ProtoVersion{
					Major: pp.ProtocolVersion.Major,
					Minor: pp.ProtocolVersion.Minor,
				},
				pp.CostModels[0],
			)
			if err != nil {
				return fmt.Errorf("build evaluation context: %w", err)
			}
			var datum data.PlutusData
			_, execErr = s.Evaluate(
				datum,
				redeemerValue.Data.Data,
				ctx.ToPlutusData(),
				redeemerValue.ExUnits,
				evalContext,
			)
		default:
			continue
		}
		if execErr != nil {
			return conway.PlutusScriptFailedError{
				ScriptHash: scriptHash,
				Tag:        redeemerKey.Tag,
				Index:      redeemerKey.Index,
				Err:        execErr,
			}
		}
	}
	return nil
}

func dijkstraGuardingPurpose(
	tx common.Transaction,
	redeemerKey common.RedeemerKey,
) (script.ScriptPurposeGuarding, bool) {
	guardingTx, ok := tx.(interface {
		GuardingCredentials() []common.Credential
	})
	if !ok {
		return script.ScriptPurposeGuarding{}, false
	}
	guards := guardingTx.GuardingCredentials()
	if uint64(redeemerKey.Index) >= uint64(len(guards)) {
		return script.ScriptPurposeGuarding{}, false
	}
	guard := guards[redeemerKey.Index]
	if guard.CredType != common.CredentialTypeScriptHash {
		return script.ScriptPurposeGuarding{}, false
	}
	return script.ScriptPurposeGuarding{Guard: guard}, true
}

func guardingRedeemer(
	redeemerKey common.RedeemerKey,
	redeemerValue common.RedeemerValue,
) script.Redeemer {
	return script.Redeemer{
		Tag:     redeemerKey.Tag,
		Index:   redeemerKey.Index,
		Data:    redeemerValue.Data.Data,
		ExUnits: redeemerValue.ExUnits,
	}
}

func dijkstraRequiredTopLevelGuards(
	body *DijkstraSubTransactionBody,
) (map[dijkstraCredentialKey]cbor.RawMessage, error) {
	if body == nil || body.TxRequiredTopLevelGuards == nil {
		return nil, nil
	}
	raw := body.TxRequiredTopLevelGuards.Cbor()
	var required map[dijkstraCredentialKey]cbor.RawMessage
	consumed, err := cbor.Decode(raw, &required)
	if err != nil {
		return nil, fmt.Errorf("decode required top-level guards: %w", err)
	}
	if consumed != len(raw) {
		return nil, fmt.Errorf(
			"decode required top-level guards: %d trailing bytes",
			len(raw)-consumed,
		)
	}
	credentials := make([]dijkstraCredentialKey, 0, len(required))
	for credential := range required {
		credentials = append(credentials, credential)
	}
	dijkstraSortCredentialKeys(credentials)
	for _, credential := range credentials {
		rawDatum := required[credential]
		if bytes.Equal(rawDatum, []byte{0xf6}) {
			continue
		}
		var datum common.Datum
		consumed, err := cbor.Decode(rawDatum, &datum)
		if err != nil {
			return nil, fmt.Errorf(
				"decode required guard datum for credential %d:%x: %w",
				credential.Type,
				credential.Hash[:],
				err,
			)
		}
		if consumed != len(rawDatum) {
			return nil, fmt.Errorf(
				"decode required guard datum for credential %d:%x: %d trailing bytes",
				credential.Type,
				credential.Hash[:],
				len(rawDatum)-consumed,
			)
		}
		if datum.Data == nil {
			return nil, fmt.Errorf(
				"decode required guard datum for credential %d:%x: nil Plutus data",
				credential.Type,
				credential.Hash[:],
			)
		}
	}
	return required, nil
}

func dijkstraSortedCredentials(
	credentials map[dijkstraCredentialKey]struct{},
) []common.Credential {
	keys := make([]dijkstraCredentialKey, 0, len(credentials))
	for credential := range credentials {
		keys = append(keys, credential)
	}
	dijkstraSortCredentialKeys(keys)
	ret := make([]common.Credential, 0, len(keys))
	for _, credential := range keys {
		ret = append(ret, *credential.credential())
	}
	return ret
}

func validateDijkstraRequiredTopLevelGuards(
	tx *DijkstraTransaction,
) error {
	topLevel := make(map[dijkstraCredentialKey]struct{})
	topTx := dijkstraConwayFeatureTransaction{
		Transaction: tx,
		body:        &tx.Body,
		witnesses:   tx.WitnessSet,
	}
	for _, guard := range nativeScriptGuardCredentials(topTx) {
		topLevel[dijkstraCredentialKey{
			Type: guard.CredType,
			Hash: guard.Credential,
		}] = struct{}{}
	}
	missing := make(map[dijkstraCredentialKey]struct{})
	subTxs := tx.Body.TxSubTransactions.Items()
	for idx := range subTxs {
		required, err := dijkstraRequiredTopLevelGuards(&subTxs[idx].Body)
		if err != nil {
			return err
		}
		for credential := range required {
			if _, ok := topLevel[credential]; !ok {
				missing[credential] = struct{}{}
			}
		}
	}
	if len(missing) > 0 {
		return &MissingRequiredGuards{
			Guards: dijkstraSortedCredentials(missing),
		}
	}
	return nil
}

func validateDijkstraGuardDatums(
	tx *DijkstraTransaction,
	available map[common.ScriptHash]common.Script,
) error {
	malformed := make(map[dijkstraCredentialKey]struct{})
	subTxs := tx.Body.TxSubTransactions.Items()
	for idx := range subTxs {
		required, err := dijkstraRequiredTopLevelGuards(&subTxs[idx].Body)
		if err != nil {
			return err
		}
		for credential, rawDatum := range required {
			hasDatum := !bytes.Equal(rawDatum, []byte{0xf6})
			switch credential.Type {
			case common.CredentialTypeAddrKeyHash:
				if hasDatum {
					malformed[credential] = struct{}{}
				}
			case common.CredentialTypeScriptHash:
				candidate, ok := available[common.ScriptHash(credential.Hash)]
				if !ok {
					continue
				}
				_, plutus := common.PlutusScriptVersion(candidate)
				if plutus == hasDatum {
					continue
				}
				malformed[credential] = struct{}{}
			}
		}
	}
	if len(malformed) > 0 {
		return &MalformedGuardDatums{
			Guards: dijkstraSortedCredentials(malformed),
		}
	}
	return nil
}

func UtxoValidateRedeemerAndScriptWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok {
		return conway.UtxoValidateRedeemerAndScriptWitnesses(
			tx,
			slot,
			ls,
			pp,
		)
	}
	levels, available, err := dijkstraWitnessRuleLevels(dijkstraTx, ls)
	if err != nil {
		return err
	}
	hasPlutusWitness := false
	for _, level := range levels {
		hasPlutusWitness = hasPlutusWitness || witnessSetHasPlutus(
			level.tx.Witnesses(),
		)
	}
	for _, level := range levels {
		for _, required := range dijkstraRequiredScriptPurposes(level) {
			hash := required.purpose.ScriptHash()
			if _, ok := available[hash]; !ok {
				return common.MissingScriptWitnessesError{ScriptHash: hash}
			}
		}
	}
	if err := validateDijkstraRequiredTopLevelGuards(dijkstraTx); err != nil {
		return err
	}
	if err := validateDijkstraGuardDatums(dijkstraTx, available); err != nil {
		return err
	}
	for _, level := range levels {
		if err := validateDijkstraPlutusRedeemers(
			level,
			available,
		); err != nil {
			return err
		}
	}

	totalRedeemers := 0
	plutusRedeemers := 0
	for _, level := range levels {
		wits := level.tx.Witnesses()
		levelRedeemers := 0
		if wits != nil && wits.Redeemers() != nil {
			for key := range wits.Redeemers().Iter() {
				levelRedeemers++
				totalRedeemers++
				if key.Tag != common.RedeemerTagGuarding {
					plutusRedeemers++
					continue
				}
				purpose, ok := dijkstraGuardingPurpose(level.tx, key)
				if !ok {
					continue
				}
				switch available[purpose.ScriptHash()].(type) {
				case common.NativeScript, *common.NativeScript:
				default:
					plutusRedeemers++
				}
			}
		}
		if level.tx.ScriptDataHash() != nil && levelRedeemers == 0 &&
			(wits == nil || len(wits.PlutusData()) == 0) {
			return common.MissingRedeemersForScriptDataHashError{}
		}
	}

	hasAvailablePlutus := false
	for _, candidate := range available {
		if _, ok := common.PlutusScriptVersion(candidate); ok {
			hasAvailablePlutus = true
			break
		}
	}
	if plutusRedeemers > 0 && !hasAvailablePlutus {
		return common.MissingPlutusScriptWitnessesError{}
	}
	if totalRedeemers == 0 && hasPlutusWitness {
		return common.ExtraneousPlutusScriptWitnessesError{}
	}
	return nil
}

func witnessSetHasPlutus(wits common.TransactionWitnessSet) bool {
	if wits == nil {
		return false
	}
	return len(wits.PlutusV1Scripts()) > 0 ||
		len(wits.PlutusV2Scripts()) > 0 ||
		len(wits.PlutusV3Scripts()) > 0 ||
		len(common.PlutusV4ScriptsFromWitnessSet(wits)) > 0
}

// UtxoValidateScriptWitnesses compares globally supplied Dijkstra scripts to
// the scripts required across all transaction levels. Reference scripts from
// any level satisfy requirements without requiring a duplicate witness.
func UtxoValidateScriptWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok {
		return conway.UtxoValidateScriptWitnesses(tx, slot, ls, pp)
	}
	levels, available, err := dijkstraScriptLevels(dijkstraTx, ls)
	if err != nil {
		return err
	}
	required := make(map[common.ScriptHash]struct{})
	referenceScripts := make(map[common.ScriptHash]struct{})
	witnessScripts := make(map[common.ScriptHash]struct{})
	for _, level := range levels {
		for _, purpose := range dijkstraRequiredScriptPurposes(level) {
			required[purpose.purpose.ScriptHash()] = struct{}{}
		}
		for _, utxo := range level.resolved {
			if utxo.Output == nil || utxo.Output.ScriptRef() == nil {
				continue
			}
			referenceScripts[utxo.Output.ScriptRef().Hash()] = struct{}{}
		}
		levelScripts := make(map[common.ScriptHash]common.Script)
		addDijkstraWitnessScripts(levelScripts, level.tx.Witnesses())
		for hash := range levelScripts {
			witnessScripts[hash] = struct{}{}
		}
	}
	for hash := range required {
		if _, ok := available[hash]; !ok {
			return common.MissingScriptWitnessesError{ScriptHash: hash}
		}
		if _, referenced := referenceScripts[hash]; referenced {
			continue
		}
		if _, witnessed := witnessScripts[hash]; !witnessed {
			return common.MissingScriptWitnessesError{ScriptHash: hash}
		}
	}
	for hash := range witnessScripts {
		if _, needed := required[hash]; !needed {
			return common.ExtraneousScriptWitnessesError{ScriptHash: hash}
		}
		if _, referenced := referenceScripts[hash]; referenced {
			return common.ExtraneousScriptWitnessesError{ScriptHash: hash}
		}
	}
	return nil
}

func UtxoValidateFeeTooSmallUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	minFee, err := MinFeeTxWithUtxo(tx, pp, ls)
	if err != nil {
		return err
	}
	minFeeBigInt := new(big.Int).SetUint64(minFee)
	fee := tx.Fee()
	if fee == nil {
		fee = new(big.Int)
	}
	if fee.Cmp(minFeeBigInt) >= 0 {
		return nil
	}
	return shelley.FeeTooSmallUtxoError{
		Provided: fee,
		Min:      minFeeBigInt,
	}
}

func MinFeeTx(
	tx common.Transaction,
	pp common.ProtocolParameters,
) (uint64, error) {
	tmpPparams, err := conwayPparams(pp)
	if err != nil {
		return 0, err
	}
	txSize, err := common.TxSizeForFee(tx)
	if err != nil {
		return 0, err
	}
	return common.CalculateMinFee(
		txSize,
		tmpPparams.MinFeeA,
		tmpPparams.MinFeeB,
	)
}

// MinFeeTxWithRefScriptSize adds the Dijkstra tiered reference-script fee to
// the size-based transaction fee. Conway parameters retain their fixed Conway
// stride and multiplier for compatibility with cross-era callers.
func MinFeeTxWithRefScriptSize(
	tx common.Transaction,
	pp common.ProtocolParameters,
	scriptSize uint64,
) (uint64, error) {
	dijkstraPp, err := dijkstraPparams(pp)
	if err != nil {
		return 0, err
	}
	baseFee, err := MinFeeTx(tx, pp)
	if err != nil {
		return 0, err
	}
	refScriptFee, err := conway.CalculateRefScriptFee(
		scriptSize,
		dijkstraPp.MinFeeRefScriptCostPerByte,
		uint64(dijkstraPp.RefScriptCostStride),
		dijkstraPp.RefScriptCostMultiplier,
	)
	if err != nil {
		return 0, err
	}
	if baseFee > math.MaxUint64-refScriptFee {
		return 0, errors.New("minimum transaction fee overflow")
	}
	return baseFee + refScriptFee, nil
}

// MinFeeTxWithUtxo calculates the Dijkstra minimum fee from reference scripts
// consumed by the top-level transaction. Subtransaction reference scripts only
// contribute to the Dijkstra batch size limits.
func MinFeeTxWithUtxo(
	tx common.Transaction,
	pp common.ProtocolParameters,
	utxoState common.UtxoState,
) (uint64, error) {
	scriptSize, err := common.ConsumedReferenceScriptSize(tx, utxoState)
	if err != nil {
		return 0, err
	}
	return MinFeeTxWithRefScriptSize(tx, pp, scriptSize)
}

func UtxoValidateCostModelsPresent(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, err := conwayPparams(pp)
	if err != nil {
		return err
	}
	required, err := usedPlutusVersions(tx, ls)
	if err != nil {
		return err
	}
	for version := range required {
		model, ok := tmpPparams.CostModels[version]
		if !ok || len(model) == 0 {
			return common.MissingCostModelError{Version: version}
		}
	}
	return nil
}

func usedPlutusVersions(
	tx common.Transaction,
	ls common.LedgerState,
) (map[uint]struct{}, error) {
	used := make(map[uint]struct{})
	if dijkstraTx, ok := tx.(*DijkstraTransaction); ok {
		levels, _, err := dijkstraScriptLevels(dijkstraTx, ls)
		if err != nil {
			return nil, err
		}
		for _, level := range levels {
			addUsedPlutusVersionsFromNeeded(used, level.view)
		}
		return used, nil
	}
	addUsedPlutusVersionsFromWitnessSet(used, tx.Witnesses())
	if ls == nil {
		return used, nil
	}
	for _, refInput := range tx.ReferenceInputs() {
		utxo, err := ls.UtxoById(refInput)
		if err != nil {
			return nil, common.ReferenceInputResolutionError{
				Input: refInput,
				Err:   err,
			}
		}
		if utxo.Output == nil {
			continue
		}
		if version, ok := common.PlutusScriptVersion(utxo.Output.ScriptRef()); ok {
			used[version] = struct{}{}
		}
	}
	// Regular bad inputs are reported by BadInputsUtxo later in rule order.
	// Only resolved regular inputs can contribute script refs here.
	for _, input := range tx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil || utxo.Output == nil {
			continue
		}
		if version, ok := common.PlutusScriptVersion(utxo.Output.ScriptRef()); ok {
			used[version] = struct{}{}
		}
	}
	return used, nil
}

// addUsedPlutusVersionsFromNeeded folds a script view's needed Plutus
// languages into used. It defers to script.TxScriptView.UsedPlutusVersions so
// the older eras, which had to be corrected to derive the set this way rather
// than from every reachable script (gouroboros #2188), and this one cannot
// drift apart.
func addUsedPlutusVersionsFromNeeded(
	used map[uint]struct{},
	view script.TxScriptView,
) {
	for version := range view.UsedPlutusVersions() {
		used[version] = struct{}{}
	}
}

func addUsedPlutusVersionsFromWitnessSet(
	used map[uint]struct{},
	wits common.TransactionWitnessSet,
) {
	if wits == nil {
		return
	}
	if len(wits.PlutusV1Scripts()) > 0 {
		used[0] = struct{}{}
	}
	if len(wits.PlutusV2Scripts()) > 0 {
		used[1] = struct{}{}
	}
	if len(wits.PlutusV3Scripts()) > 0 {
		used[2] = struct{}{}
	}
	if len(common.PlutusV4ScriptsFromWitnessSet(wits)) > 0 {
		used[3] = struct{}{}
	}
}

func UtxoValidateScriptDataHash(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, err := conwayPparams(pp)
	if err != nil {
		return err
	}
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	levels, _, err := dijkstraScriptLevels(dijkstraTx, ls)
	if err != nil {
		return err
	}
	for _, level := range levels {
		usedVersions := make(map[uint]struct{})
		addUsedPlutusVersionsFromNeeded(usedVersions, level.view)
		if err := validateDijkstraScriptDataHash(
			level,
			tmpPparams,
			usedVersions,
		); err != nil {
			return err
		}
	}
	return nil
}

func validateDijkstraScriptDataHash(
	level dijkstraScriptLevel,
	pp *conway.ConwayProtocolParameters,
	usedVersions map[uint]struct{},
) error {
	wits, ok := level.tx.Witnesses().(DijkstraTransactionWitnessSet)
	if !ok {
		return errors.New("witness set is not expected type")
	}
	hasRedeemers := wits.WsRedeemers.Len() > 0
	hasDatums := len(wits.WsPlutusData.Items()) > 0
	declaredHash := level.tx.ScriptDataHash()
	if !hasRedeemers && !hasDatums {
		if declaredHash != nil {
			return common.ExtraneousScriptDataHashError{Provided: *declaredHash}
		}
		return nil
	}
	if declaredHash == nil {
		return common.MissingScriptDataHashError{}
	}
	for version := range usedVersions {
		if _, ok := pp.CostModels[version]; !ok {
			return common.MissingCostModelError{Version: version}
		}
	}

	redeemersCbor := wits.WsRedeemers.Cbor()
	if len(redeemersCbor) == 0 {
		if wits.WsRedeemers.Len() == 0 {
			redeemersCbor = []byte{0xa0}
		} else {
			return errors.New(
				"missing preserved CBOR for redeemers: decode path must call SetCbor",
			)
		}
	}

	var datumsCbor []byte
	if hasDatums {
		datumsCbor = wits.WsPlutusData.Cbor()
		if len(datumsCbor) == 0 {
			return errors.New(
				"missing preserved CBOR for Plutus data: decode path must call SetCbor",
			)
		}
	}

	langViewsCbor, err := common.EncodeLangViews(
		usedVersions,
		pp.CostModels,
	)
	if err != nil {
		return err
	}

	hashInput := make(
		[]byte,
		0,
		len(redeemersCbor)+len(datumsCbor)+len(langViewsCbor),
	)
	hashInput = append(hashInput, redeemersCbor...)
	hashInput = append(hashInput, datumsCbor...)
	hashInput = append(hashInput, langViewsCbor...)

	computedHash := common.Blake2b256Hash(hashInput)
	if *declaredHash != computedHash {
		return common.ScriptDataHashMismatchError{
			Declared: *declaredHash,
			Computed: computedHash,
		}
	}
	return nil
}

func UtxoValidateSupplementalDatums(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok {
		return conway.UtxoValidateSupplementalDatums(tx, slot, ls, pp)
	}
	for _, level := range dijkstraTransactionLevels(dijkstraTx) {
		if err := conway.UtxoValidateSupplementalDatums(
			level,
			slot,
			ls,
			pp,
		); err != nil {
			return err
		}
	}
	return nil
}

func redeemerCount(tx common.Transaction) int {
	count := 0
	for wits := range dijkstraTransactionWitnessSets(tx) {
		if wits == nil || wits.Redeemers() == nil {
			continue
		}
		for range wits.Redeemers().Iter() {
			count++
		}
	}
	return count
}

// dijkstraTransactionWitnessSets visits the top-level witness set followed by
// each subtransaction witness set. The top level is retained first so existing
// Dijkstra behavior is unchanged when a transaction has no subtransactions.
func dijkstraTransactionWitnessSets(
	tx common.Transaction,
) iter.Seq[common.TransactionWitnessSet] {
	return func(yield func(common.TransactionWitnessSet) bool) {
		if !yield(tx.Witnesses()) {
			return
		}
		for _, wits := range common.SubTransactionWitnessSetsFromTransaction(tx) {
			if !yield(wits) {
				return
			}
		}
	}
}

func UtxoValidateInsufficientCollateral(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, err := conwayPparams(pp)
	if err != nil {
		return err
	}
	if redeemerCount(tx) == 0 {
		return nil
	}
	totalCollateral := new(big.Int)
	for _, collateralInput := range tx.Collateral() {
		utxo, err := common.ResolveInputUtxo(ls, collateralInput)
		if err != nil {
			return err
		}
		if amount := utxo.Output.Amount(); amount != nil {
			totalCollateral.Add(totalCollateral, amount)
		}
	}
	fee := tx.Fee()
	if fee == nil {
		fee = new(big.Int)
	}
	minCollateral := new(big.Int).Mul(
		fee,
		new(big.Int).SetUint64(uint64(tmpPparams.CollateralPercentage)),
	)
	minCollateral.Div(minCollateral, big.NewInt(100))
	if totalCollateral.Cmp(minCollateral) >= 0 {
		return nil
	}
	var providedU, requiredU uint64
	if totalCollateral.IsUint64() {
		providedU = totalCollateral.Uint64()
	}
	if minCollateral.IsUint64() {
		requiredU = minCollateral.Uint64()
	}
	return alonzo.InsufficientCollateralError{
		Provided: providedU,
		Required: requiredU,
	}
}

func UtxoValidateCollateralContainsNonAda(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	if redeemerCount(tx) == 0 {
		return nil
	}
	badOutputs := []common.TransactionOutput{}
	totalCollateral := new(big.Int)
	totalAssets := common.NewMultiAsset[common.MultiAssetTypeOutput](nil)
	for _, collateralInput := range tx.Collateral() {
		utxo, err := common.ResolveInputUtxo(ls, collateralInput)
		if err != nil {
			return err
		}
		amount := utxo.Output.Amount()
		if amount != nil {
			totalCollateral.Add(totalCollateral, amount)
		}
		assets := utxo.Output.Assets()
		totalAssets.Add(assets)
		if assets == nil || len(assets.Policies()) == 0 {
			continue
		}
		badOutputs = append(badOutputs, utxo.Output)
	}
	if len(badOutputs) == 0 {
		return nil
	}
	if collReturn := tx.CollateralReturn(); collReturn != nil {
		if (&totalAssets).Compare(collReturn.Assets()) {
			return nil
		}
	}
	var providedU uint64
	if totalCollateral.IsUint64() {
		providedU = totalCollateral.Uint64()
	}
	return alonzo.CollateralContainsNonAdaError{Provided: providedU}
}

func UtxoValidateNoCollateralInputs(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	if redeemerCount(tx) == 0 {
		return nil
	}
	if len(tx.Collateral()) > 0 {
		return nil
	}
	return alonzo.NoCollateralInputsError{}
}

func UtxoValidateOutputTooSmallUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	var badOutputs []common.TransactionOutput
	for _, tmpOutput := range tx.Outputs() {
		minCoin, err := MinCoinTxOut(tmpOutput, pp)
		if err != nil {
			return err
		}
		minCoinBig := new(big.Int).SetUint64(minCoin)
		amount := tmpOutput.Amount()
		if amount == nil {
			amount = new(big.Int)
		}
		if amount.Cmp(minCoinBig) < 0 {
			badOutputs = append(badOutputs, tmpOutput)
		}
	}
	if len(badOutputs) == 0 {
		return nil
	}
	return shelley.OutputTooSmallUtxoError{Outputs: badOutputs}
}

func MinCoinTxOut(
	txOut common.TransactionOutput,
	pp common.ProtocolParameters,
) (uint64, error) {
	tmpPparams, err := conwayPparams(pp)
	if err != nil {
		return 0, err
	}
	txOutBytes, err := cbor.Encode(txOut)
	if err != nil {
		return 0, err
	}
	return tmpPparams.AdaPerUtxoByte *
		(minUtxoOverheadBytes + uint64(len(txOutBytes))), nil
}

func UtxoValidateOutputTooBigUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, err := conwayPparams(pp)
	if err != nil {
		return err
	}
	var badOutputs []common.TransactionOutput
	for _, txOutput := range tx.Outputs() {
		outputVal, err := outputValue(txOutput)
		if err != nil {
			return err
		}
		outputValBytes, err := cbor.Encode(outputVal)
		if err != nil {
			return err
		}
		if uint(len(outputValBytes)) <= tmpPparams.MaxValueSize {
			continue
		}
		badOutputs = append(badOutputs, txOutput)
	}
	if len(badOutputs) == 0 {
		return nil
	}
	return mary.OutputTooBigUtxoError{Outputs: badOutputs}
}

func outputValue(
	output common.TransactionOutput,
) (mary.MaryTransactionOutputValue, error) {
	amount := output.Amount()
	if amount == nil {
		amount = new(big.Int)
	}
	if !amount.IsUint64() {
		return mary.MaryTransactionOutputValue{}, fmt.Errorf(
			"transaction output amount exceeds uint64: %s",
			amount,
		)
	}
	return mary.MaryTransactionOutputValue{
		Amount: amount.Uint64(),
		Assets: output.Assets(),
	}, nil
}

func UtxoValidateTransactionNetworkId(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	txNetworkId := dijkstraTx.NetworkId()
	if txNetworkId == nil {
		return nil
	}
	if ls == nil {
		return nil
	}
	ledgerNetworkId := ls.NetworkId()
	if uint(*txNetworkId) != ledgerNetworkId {
		return conway.WrongTransactionNetworkIdError{
			TxNetworkId:     *txNetworkId,
			LedgerNetworkId: ledgerNetworkId,
		}
	}
	return nil
}

func UtxoValidateMaxTxSizeUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, err := conwayPparams(pp)
	if err != nil {
		return err
	}
	txBytes := tx.Cbor()
	if len(txBytes) == 0 {
		txBytes, err = cbor.Encode(tx)
		if err != nil {
			return err
		}
	}
	if uint(len(txBytes)) <= tmpPparams.MaxTxSize {
		return nil
	}
	return shelley.MaxTxSizeUtxoError{
		TxSize:    uint(len(txBytes)),
		MaxTxSize: tmpPparams.MaxTxSize,
	}
}

func UtxoValidateExUnitsTooBigUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, err := conwayPparams(pp)
	if err != nil {
		return err
	}
	var totalSteps, totalMemory int64
	for wits := range dijkstraTransactionWitnessSets(tx) {
		if wits == nil || wits.Redeemers() == nil {
			continue
		}
		for _, redeemer := range wits.Redeemers().Iter() {
			newSteps, ok := common.AddInt64Checked(
				totalSteps,
				redeemer.ExUnits.Steps,
			)
			if !ok {
				return alonzo.ExUnitsTooBigUtxoError{
					TotalExUnits: common.ExUnits{
						Memory: totalMemory,
						Steps:  totalSteps,
					},
					MaxTxExUnits: tmpPparams.MaxTxExUnits,
				}
			}
			totalSteps = newSteps
			newMemory, ok := common.AddInt64Checked(
				totalMemory,
				redeemer.ExUnits.Memory,
			)
			if !ok {
				return alonzo.ExUnitsTooBigUtxoError{
					TotalExUnits: common.ExUnits{
						Memory: totalMemory,
						Steps:  totalSteps,
					},
					MaxTxExUnits: tmpPparams.MaxTxExUnits,
				}
			}
			totalMemory = newMemory
		}
	}
	if totalSteps <= tmpPparams.MaxTxExUnits.Steps &&
		totalMemory <= tmpPparams.MaxTxExUnits.Memory {
		return nil
	}
	return alonzo.ExUnitsTooBigUtxoError{
		TotalExUnits: common.ExUnits{
			Memory: totalMemory,
			Steps:  totalSteps,
		},
		MaxTxExUnits: tmpPparams.MaxTxExUnits,
	}
}

func UtxoValidateTooManyCollateralInputs(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, err := conwayPparams(pp)
	if err != nil {
		return err
	}
	collateralCount := uint(len(tx.Collateral()))
	if collateralCount <= tmpPparams.MaxCollateralInputs {
		return nil
	}
	return babbage.TooManyCollateralInputsError{
		Provided: collateralCount,
		Max:      tmpPparams.MaxCollateralInputs,
	}
}

func UtxoValidateExtraneousRedeemers(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok {
		return conway.UtxoValidateExtraneousRedeemers(tx, slot, ls, pp)
	}
	levels, available, err := dijkstraWitnessRuleLevels(dijkstraTx, ls)
	if err != nil {
		return err
	}
	for _, level := range levels {
		if err := validateDijkstraExtraneousRedeemers(
			level.tx,
			available,
		); err != nil {
			return err
		}
	}
	return nil
}

func validateDijkstraExtraneousRedeemers(
	tx common.Transaction,
	available map[common.ScriptHash]common.Script,
) error {
	wits := tx.Witnesses()
	if wits == nil {
		return nil
	}
	redeemers := wits.Redeemers()
	if redeemers == nil {
		return nil
	}

	// Collection lengths are kept at wire width so that a redeemer index near
	// the top of its uint32 range is compared, not narrowed to a platform int.
	inputCount := uint64(len(tx.Inputs()))
	certCount := uint64(len(tx.Certificates()))
	withdrawalCount := uint64(len(tx.Withdrawals()))
	proposalCount := uint64(len(tx.ProposalProcedures()))

	mintPolicyCount := uint64(0)
	if mint := tx.AssetMint(); mint != nil {
		mintPolicyCount = uint64(len(mint.Policies()))
	}

	voterCount := uint64(0)
	if votingProcs := tx.VotingProcedures(); votingProcs != nil {
		voterCount = uint64(len(votingProcs))
	}

	for redeemerKey := range redeemers.Iter() {
		var maxIndex uint64
		switch redeemerKey.Tag {
		case common.RedeemerTagSpend:
			maxIndex = inputCount
		case common.RedeemerTagMint:
			maxIndex = mintPolicyCount
		case common.RedeemerTagCert:
			maxIndex = certCount
		case common.RedeemerTagReward:
			maxIndex = withdrawalCount
		case common.RedeemerTagVoting:
			maxIndex = voterCount
		case common.RedeemerTagProposing:
			maxIndex = proposalCount
		case common.RedeemerTagGuarding:
			needsPlutus := dijkstraGuardNeedsPlutusRedeemer(
				tx,
				available,
				redeemerKey.Index,
			)
			if needsPlutus {
				continue
			}
			return conway.ExtraRedeemerError{RedeemerKey: redeemerKey}
		default:
			return conway.ExtraRedeemerError{RedeemerKey: redeemerKey}
		}

		if uint64(redeemerKey.Index) >= maxIndex {
			return conway.ExtraRedeemerError{RedeemerKey: redeemerKey}
		}
	}

	return nil
}

func dijkstraGuardNeedsPlutusRedeemer(
	tx common.Transaction,
	available map[common.ScriptHash]common.Script,
	index uint32,
) bool {
	guard, ok := dijkstraGuardCredentialAt(tx, index)
	if !ok || guard.CredType != common.CredentialTypeScriptHash {
		return false
	}
	candidate, ok := available[common.ScriptHash(guard.Credential)]
	if !ok {
		// Missing scripts are reported by the script-witness predicate. Until
		// then, do not misclassify an otherwise well-shaped guarding key as an
		// extraneous redeemer.
		return true
	}
	switch candidate.(type) {
	case common.NativeScript, *common.NativeScript:
		return false
	default:
		return true
	}
}

func dijkstraGuardCredentialAt(
	tx common.Transaction,
	index uint32,
) (common.Credential, bool) {
	var guards *DijkstraGuards
	switch tx := tx.(type) {
	case *DijkstraTransaction:
		guards = tx.Body.TxGuards
	case dijkstraConwayFeatureTransaction:
		switch body := tx.body.(type) {
		case *DijkstraTransactionBody:
			guards = body.TxGuards
		case *DijkstraSubTransactionBody:
			guards = body.TxGuards
		}
	}
	if guards == nil {
		return common.Credential{}, false
	}
	if uint64(index) >= uint64(len(guards.Credentials)) {
		return common.Credential{}, false
	}
	return guards.Credentials[index], true
}

// UtxoValidateNativeScripts evaluates the native scripts this transaction has
// to satisfy, with Dijkstra's guard credentials in scope for RequireGuard.
//
// The scripts to evaluate come from the resolved transaction view, as in
// Babbage: a native script some script purpose requires counts whether the
// witness set, a reference input, or the spent input's own reference-script
// field supplies it. Dijkstra keeps its own body rather than delegating to
// Babbage because only it evaluates guards, and it runs the check at every
// transaction level so a sub-transaction's native scripts are covered too.
func UtxoValidateNativeScripts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok {
		return conway.UtxoValidateNativeScripts(tx, slot, ls, pp)
	}
	levels, _, err := dijkstraScriptLevels(dijkstraTx, ls)
	if err != nil {
		return err
	}
	for _, level := range levels {
		env := script.NewNativeScriptEnv(level.tx, slot)
		guardCredentials := nativeScriptGuardCredentials(level.tx)
		for _, nativeScript := range script.NativeScriptsToEvaluate(
			level.tx,
			level.view,
		) {
			if !nativeScript.EvaluateWithGuards(
				env.Slot,
				env.ValidityStart,
				env.ValidityEnd,
				env.KeyHashes,
				guardCredentials,
			) {
				return conway.NativeScriptFailedError{
					ScriptHash: nativeScript.Hash(),
				}
			}
		}
	}
	return nil
}

func nativeScriptGuardCredentials(tx common.Transaction) []common.Credential {
	var guards *DijkstraGuards
	switch tx := tx.(type) {
	case *DijkstraTransaction:
		guards = tx.Body.TxGuards
	case dijkstraConwayFeatureTransaction:
		switch body := tx.body.(type) {
		case *DijkstraTransactionBody:
			guards = body.TxGuards
		case *DijkstraSubTransactionBody:
			guards = body.TxGuards
		}
	}
	if guards == nil {
		return nil
	}
	ret := make(
		[]common.Credential,
		0,
		len(guards.Credentials)+len(guards.KeyHashes),
	)
	ret = append(ret, guards.Credentials...)
	for _, hash := range guards.KeyHashes {
		ret = append(ret, common.Credential{
			CredType:   common.CredentialTypeAddrKeyHash,
			Credential: hash,
		})
	}
	return ret
}

func UtxoValidateMalformedReferenceScripts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	params, err := dijkstraPparams(pp)
	if err != nil {
		return err
	}
	return common.ValidatePlutusScriptsWellFormed(
		tx,
		params.ProtocolVersion.Major,
	)
}

func UtxoValidateRefScriptSizePerTx(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	maxSize, err := maxRefScriptSizePerTx(pp)
	if err != nil {
		return err
	}
	if !tx.IsValid() {
		return nil
	}
	totalSize, err := consumedRefScriptSize(tx, ls)
	if err != nil {
		return err
	}
	if totalSize > maxSize {
		return common.RefScriptSizePerTxTooLargeError{
			TxSize:  totalSize,
			MaxSize: maxSize,
		}
	}
	return nil
}

func ValidateRefScriptSizePerBlock(
	block *DijkstraBlock,
	pp common.ProtocolParameters,
	utxoStates ...common.UtxoState,
) error {
	maxSize, err := maxRefScriptSizePerBlock(pp)
	if err != nil {
		return err
	}
	if len(utxoStates) > 1 {
		return errors.New("expected at most one ledger state")
	}
	var utxoState common.UtxoState
	if len(utxoStates) == 1 {
		utxoState = utxoStates[0]
	}
	conwayPp, err := conwayPparams(pp)
	if err != nil {
		return err
	}
	totalSize, err := common.ConsumedReferenceScriptSizePerBlock(
		block,
		utxoState,
		conwayPp.ProtocolVersion.Major >= common.ProtocolVersionVanRossem,
		consumedRefScriptSize,
	)
	if err != nil {
		return err
	}
	if totalSize > maxSize {
		return common.RefScriptSizePerBlockTooLargeError{
			BlockSize: totalSize,
			MaxSize:   maxSize,
		}
	}
	return nil
}

func maxRefScriptSizePerTx(pp common.ProtocolParameters) (uint64, error) {
	switch p := pp.(type) {
	case *DijkstraProtocolParameters:
		return uint64(p.MaxRefScriptSizePerTx), nil
	case *conway.ConwayProtocolParameters:
		return conway.MaxRefScriptSizePerTx, nil
	default:
		return 0, errors.New("pparams are not expected type")
	}
}

func maxRefScriptSizePerBlock(pp common.ProtocolParameters) (uint64, error) {
	switch p := pp.(type) {
	case *DijkstraProtocolParameters:
		return uint64(p.MaxRefScriptSizePerBlock), nil
	case *conway.ConwayProtocolParameters:
		return conway.MaxRefScriptSizePerBlock, nil
	default:
		return 0, errors.New("pparams are not expected type")
	}
}

func consumedRefScriptSize(
	tx common.Transaction,
	utxoState common.UtxoState,
) (uint64, error) {
	totalSize, err := common.ConsumedReferenceScriptSize(tx, utxoState)
	if err != nil {
		return 0, err
	}
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok {
		return totalSize, nil
	}
	subTxs := dijkstraTx.Body.TxSubTransactions.Items()
	for idx := range subTxs {
		subTx := &subTxs[idx]
		subTxSize, err := common.ConsumedReferenceScriptSize(
			&subTx.Body,
			utxoState,
		)
		if err != nil {
			return 0, err
		}
		if totalSize > math.MaxUint64-subTxSize {
			return 0, errors.New("consumed reference-script size overflow")
		}
		totalSize += subTxSize
	}
	return totalSize, nil
}
