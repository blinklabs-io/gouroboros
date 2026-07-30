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
	"errors"
	"fmt"
	"iter"
	"math/big"

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
	"github.com/blinklabs-io/plutigo/syn"
)

const minUtxoOverheadBytes = 160

var UtxoValidationRules = []common.UtxoValidationRuleFunc{
	conway.UtxoValidateMetadata,
	UtxoValidateProposalProcedures,
	conway.UtxoValidateProposalNetworkIds,
	conway.UtxoValidateEmptyTreasuryWithdrawals,
	UtxoValidateBootstrapAllowedGovActions,
	UtxoValidateBootstrapParameterGroups,
	conway.UtxoValidateIsValidFlag,
	conway.UtxoValidateRequiredVKeyWitnesses,
	conway.UtxoValidateCollateralVKeyWitnesses,
	UtxoValidateRedeemerAndScriptWitnesses,
	conway.UtxoValidateSignatures,
	UtxoValidateCostModelsPresent,
	UtxoValidateScriptDataHash,
	conway.UtxoValidateInlineDatumsWithPlutusV1,
	conway.UtxoValidateConwayFeaturesWithPlutusV1V2,
	UtxoValidateDisjointRefInputs,
	conway.UtxoValidateOutsideValidityIntervalUtxo,
	conway.UtxoValidateInputSetEmptyUtxo,
	conway.UtxoValidateNoDuplicateInputs,
	UtxoValidateFeeTooSmallUtxo,
	UtxoValidateInsufficientCollateral,
	UtxoValidateCollateralContainsNonAda,
	conway.UtxoValidateCollateralEqBalance,
	UtxoValidateNoCollateralInputs,
	conway.UtxoValidateBadInputsUtxo,
	conway.UtxoValidateScriptWitnesses,
	UtxoValidateValueNotConservedUtxo,
	UtxoValidateOutputTooSmallUtxo,
	UtxoValidateOutputTooBigUtxo,
	conway.UtxoValidateOutputBootAddrAttrsTooBig,
	conway.UtxoValidateWrongNetwork,
	conway.UtxoValidateWrongNetworkWithdrawal,
	UtxoValidateTransactionNetworkId,
	UtxoValidateMaxTxSizeUtxo,
	UtxoValidateExUnitsTooBigUtxo,
	UtxoValidateTooManyCollateralInputs,
	conway.UtxoValidateSupplementalDatums,
	UtxoValidateExtraneousRedeemers,
	UtxoValidatePlutusScripts,
	UtxoValidateNativeScripts,
	conway.UtxoValidateDelegation,
	conway.UtxoValidateWithdrawals,
	conway.UtxoValidateCommitteeCertificates,
	UtxoValidateCCVotingRestrictions,
	UtxoValidateMalformedReferenceScripts,
	UtxoValidateRefScriptSizePerTx,
}

func dijkstraPparams(pp common.ProtocolParameters) (*DijkstraProtocolParameters, error) {
	switch p := pp.(type) {
	case *DijkstraProtocolParameters:
		return p, nil
	case *conway.ConwayProtocolParameters:
		return &DijkstraProtocolParameters{
			ConwayProtocolParameters: *p,
		}, nil
	default:
		return nil, errors.New("pparams are not expected type")
	}
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
		if govAction == nil {
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
		if govAction == nil {
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
		if govAction == nil {
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
	if !tx.IsValid() {
		return nil
	}
	if !hasGuardingRedeemers(tx.Witnesses()) {
		return conway.UtxoValidatePlutusScripts(tx, slot, ls, tmpPparams)
	}
	if err := conway.UtxoValidatePlutusScripts(
		transactionWithoutGuardingRedeemers{Transaction: tx},
		slot,
		ls,
		tmpPparams,
	); err != nil {
		return err
	}
	return validateGuardingPlutusScripts(tx, ls, tmpPparams)
}

type transactionWithoutGuardingRedeemers struct {
	common.Transaction
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

func hasGuardingRedeemers(wits common.TransactionWitnessSet) bool {
	if wits == nil || wits.Redeemers() == nil {
		return false
	}
	for key := range wits.Redeemers().Iter() {
		if key.Tag == common.RedeemerTagGuarding {
			return true
		}
	}
	return false
}

func validateGuardingPlutusScripts(
	tx common.Transaction,
	ls common.LedgerState,
	pp *conway.ConwayProtocolParameters,
) error {
	wits := tx.Witnesses()
	if wits == nil || wits.Redeemers() == nil {
		return nil
	}

	availableScripts := make(map[common.ScriptHash]common.Script)
	addPlutusScriptsFromWitnessSet(availableScripts, wits)
	if dijkstraTx, ok := tx.(*DijkstraTransaction); ok {
		for _, subTx := range dijkstraTx.Body.TxSubTransactions.Items() {
			addPlutusScriptsFromWitnessSet(availableScripts, subTx.WitnessSet)
		}
	}

	resolvedInputs, err := resolvedInputsForGuardingPlutus(tx, ls)
	if err != nil {
		return err
	}
	for _, utxo := range resolvedInputs {
		if utxo.Output == nil {
			continue
		}
		scriptRef := utxo.Output.ScriptRef()
		if scriptRef == nil {
			continue
		}
		if _, ok := common.PlutusScriptVersion(scriptRef); ok {
			availableScripts[scriptRef.Hash()] = scriptRef
		}
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
			nativeGuard, err := dijkstraGuardResolvesToNativeScript(
				tx,
				ls,
				redeemerKey.Index,
			)
			if err != nil {
				return err
			}
			if nativeGuard {
				return conway.ExtraRedeemerError{RedeemerKey: redeemerKey}
			}
			return common.MissingScriptWitnessesError{ScriptHash: scriptHash}
		}
		if ls == nil {
			return errors.New(
				"ledger state is required for Dijkstra guarding Plutus validation",
			)
		}

		var execErr error
		switch s := plutusScript.(type) {
		case common.PlutusV4Script:
			if !txInfoV3Built {
				var err error
				txInfoV3, err = script.NewTxInfoV3FromTransaction(ls, tx, resolvedInputs)
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
				lang.LanguageVersionV4,
				cek.ProtoVersion{
					Major: pp.ProtocolVersion.Major,
					Minor: pp.ProtocolVersion.Minor,
				},
				pp.CostModels[3],
			)
			if err != nil {
				return fmt.Errorf("build evaluation context: %w", err)
			}
			_, execErr = s.Evaluate(ctx.ToPlutusData(), redeemerValue.ExUnits, evalContext)
		case common.PlutusV3Script:
			if !txInfoV3Built {
				var err error
				txInfoV3, err = script.NewTxInfoV3FromTransaction(ls, tx, resolvedInputs)
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
				txInfoV2, err = script.NewTxInfoV2FromTransaction(ls, tx, resolvedInputs)
				if err != nil {
					return conway.ScriptContextConstructionError{Err: err}
				}
				txInfoV2.ProtocolMajor = pp.ProtocolVersion.Major
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
				txInfoV1, err = script.NewTxInfoV1FromTransaction(ls, tx, resolvedInputs)
				if err != nil {
					return conway.ScriptContextConstructionError{Err: err}
				}
				txInfoV1.ProtocolMajor = pp.ProtocolVersion.Major
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

func addPlutusScriptsFromWitnessSet(
	availableScripts map[common.ScriptHash]common.Script,
	wits common.TransactionWitnessSet,
) {
	if wits == nil {
		return
	}
	for _, s := range wits.PlutusV1Scripts() {
		availableScripts[s.Hash()] = s
	}
	for _, s := range wits.PlutusV2Scripts() {
		availableScripts[s.Hash()] = s
	}
	for _, s := range wits.PlutusV3Scripts() {
		availableScripts[s.Hash()] = s
	}
	for _, s := range common.PlutusV4ScriptsFromWitnessSet(wits) {
		availableScripts[s.Hash()] = s
	}
}

func resolvedInputsForGuardingPlutus(
	tx common.Transaction,
	ls common.LedgerState,
) ([]common.Utxo, error) {
	inputCount := len(tx.Inputs()) + len(tx.ReferenceInputs())
	if inputCount == 0 {
		return nil, nil
	}
	if ls == nil {
		return nil, errors.New(
			"ledger state is required for Dijkstra guarding Plutus validation",
		)
	}
	resolvedInputs := make([]common.Utxo, 0, inputCount)
	for _, input := range tx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			return nil, common.InputResolutionError{
				Input: input,
				Err:   err,
			}
		}
		resolvedInputs = append(resolvedInputs, utxo)
	}
	for _, refInput := range tx.ReferenceInputs() {
		utxo, err := ls.UtxoById(refInput)
		if err != nil {
			return nil, common.ReferenceInputResolutionError{
				Input: refInput,
				Err:   err,
			}
		}
		resolvedInputs = append(resolvedInputs, utxo)
	}
	return resolvedInputs, nil
}

func dijkstraGuardingPurpose(
	tx common.Transaction,
	redeemerKey common.RedeemerKey,
) (script.ScriptPurposeGuarding, bool) {
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok || dijkstraTx.Body.TxGuards == nil {
		return script.ScriptPurposeGuarding{}, false
	}
	guards := dijkstraTx.Body.TxGuards
	if int(redeemerKey.Index) >= len(guards.Credentials) {
		return script.ScriptPurposeGuarding{}, false
	}
	guard := guards.Credentials[redeemerKey.Index]
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

func UtxoValidateRedeemerAndScriptWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	wits := tx.Witnesses()
	totalRedeemers, plutusRedeemers, err := dijkstraRedeemerCounts(tx, wits, ls)
	if err != nil {
		return err
	}
	hasPlutusWitness := witnessSetHasPlutus(wits)
	hasSharedPlutusWitness := hasPlutusWitness || dijkstraSubTxHasPlutusWitness(tx)

	hasPlutusReference := false
	if ls != nil {
		for _, refInput := range tx.ReferenceInputs() {
			utxo, err := ls.UtxoById(refInput)
			if err != nil {
				return common.ReferenceInputResolutionError{
					Input: refInput,
					Err:   err,
				}
			}
			if utxo.Output == nil {
				continue
			}
			if _, ok := common.PlutusScriptVersion(utxo.Output.ScriptRef()); ok {
				hasPlutusReference = true
				break
			}
		}
		if !hasPlutusReference {
			for _, input := range tx.Inputs() {
				utxo, err := ls.UtxoById(input)
				if err != nil || utxo.Output == nil {
					continue
				}
				if _, ok := common.PlutusScriptVersion(utxo.Output.ScriptRef()); ok {
					hasPlutusReference = true
					break
				}
			}
		}
	}

	hasWitnessPlutusData := wits != nil && len(wits.PlutusData()) > 0
	if tx.ScriptDataHash() != nil && totalRedeemers == 0 &&
		!hasWitnessPlutusData {
		return common.MissingRedeemersForScriptDataHashError{}
	}
	if plutusRedeemers > 0 && !hasSharedPlutusWitness && !hasPlutusReference {
		return common.MissingPlutusScriptWitnessesError{}
	}
	if plutusRedeemers == 0 && hasPlutusWitness {
		return common.ExtraneousPlutusScriptWitnessesError{}
	}
	return nil
}

func dijkstraRedeemerCounts(
	tx common.Transaction,
	wits common.TransactionWitnessSet,
	ls common.LedgerState,
) (total int, plutus int, err error) {
	if wits == nil || wits.Redeemers() == nil {
		return 0, 0, nil
	}
	for key := range wits.Redeemers().Iter() {
		total++
		needsPlutus, err := dijkstraRedeemerNeedsPlutus(tx, ls, key)
		if err != nil {
			return 0, 0, err
		}
		if needsPlutus {
			plutus++
		}
	}
	return total, plutus, nil
}

func dijkstraRedeemerNeedsPlutus(
	tx common.Transaction,
	ls common.LedgerState,
	key common.RedeemerKey,
) (bool, error) {
	if key.Tag != common.RedeemerTagGuarding {
		return true, nil
	}
	return dijkstraGuardNeedsPlutusRedeemer(tx, ls, key.Index)
}

func dijkstraSubTxHasPlutusWitness(tx common.Transaction) bool {
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok {
		return false
	}
	for _, subTx := range dijkstraTx.Body.TxSubTransactions.Items() {
		if witnessSetHasPlutus(subTx.WitnessSet) {
			return true
		}
	}
	return false
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

func UtxoValidateFeeTooSmallUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	minFee, err := MinFeeTx(tx, pp)
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
	addUsedPlutusVersionsFromWitnessSet(used, tx.Witnesses())
	if dijkstraTx, ok := tx.(*DijkstraTransaction); ok {
		for _, subTx := range dijkstraTx.Body.TxSubTransactions.Items() {
			addUsedPlutusVersionsFromWitnessSet(used, subTx.WitnessSet)
		}
	}
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
	tmpTx, ok := tx.(*DijkstraTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	wits := tmpTx.WitnessSet
	hasRedeemers := wits.WsRedeemers.Len() > 0
	hasDatums := len(wits.WsPlutusData.Items()) > 0

	usedVersions, err := usedPlutusVersions(tx, ls)
	if err != nil {
		return err
	}

	declaredHash := tx.ScriptDataHash()
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
		if _, ok := tmpPparams.CostModels[version]; !ok {
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
		tmpPparams.CostModels,
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

func redeemerCount(tx common.Transaction) int {
	wits := tx.Witnesses()
	if wits == nil || wits.Redeemers() == nil {
		return 0
	}
	count := 0
	for range wits.Redeemers().Iter() {
		count++
	}
	return count
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
		utxo, err := ls.UtxoById(collateralInput)
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
		utxo, err := ls.UtxoById(collateralInput)
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
	wits := tx.Witnesses()
	if wits == nil || wits.Redeemers() == nil {
		return nil
	}
	var totalSteps, totalMemory int64
	for _, redeemer := range wits.Redeemers().Iter() {
		newSteps, ok := common.AddInt64Checked(totalSteps, redeemer.ExUnits.Steps)
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
		newMemory, ok := common.AddInt64Checked(totalMemory, redeemer.ExUnits.Memory)
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
	wits := tx.Witnesses()
	if wits == nil {
		return nil
	}
	redeemers := wits.Redeemers()
	if redeemers == nil {
		return nil
	}

	inputCount := len(tx.Inputs())
	certCount := len(tx.Certificates())
	withdrawalCount := len(tx.Withdrawals())
	proposalCount := len(tx.ProposalProcedures())

	mintPolicyCount := 0
	if mint := tx.AssetMint(); mint != nil {
		mintPolicyCount = len(mint.Policies())
	}

	voterCount := 0
	if votingProcs := tx.VotingProcedures(); votingProcs != nil {
		voterCount = len(votingProcs)
	}

	for redeemerKey := range redeemers.Iter() {
		var maxIndex int
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
			needsPlutus, err := dijkstraGuardNeedsPlutusRedeemer(
				tx,
				ls,
				redeemerKey.Index,
			)
			if err != nil {
				return err
			}
			if needsPlutus {
				continue
			}
			return conway.ExtraRedeemerError{RedeemerKey: redeemerKey}
		default:
			return conway.ExtraRedeemerError{RedeemerKey: redeemerKey}
		}

		if int(redeemerKey.Index) >= maxIndex {
			return conway.ExtraRedeemerError{RedeemerKey: redeemerKey}
		}
	}

	return nil
}

func dijkstraGuardNeedsPlutusRedeemer(
	tx common.Transaction,
	ls common.LedgerState,
	index uint32,
) (bool, error) {
	guard, ok := dijkstraGuardCredentialAt(tx, index)
	if !ok || guard.CredType != common.CredentialTypeScriptHash {
		return false, nil
	}
	isNative, err := dijkstraGuardResolvesToNativeScript(tx, ls, index)
	if err != nil {
		return false, err
	}
	return !isNative, nil
}

func dijkstraGuardCredentialAt(
	tx common.Transaction,
	index uint32,
) (common.Credential, bool) {
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok || dijkstraTx.Body.TxGuards == nil {
		return common.Credential{}, false
	}
	guards := dijkstraTx.Body.TxGuards
	if int(index) >= len(guards.Credentials) {
		return common.Credential{}, false
	}
	return guards.Credentials[index], true
}

func dijkstraGuardResolvesToNativeScript(
	tx common.Transaction,
	ls common.LedgerState,
	index uint32,
) (bool, error) {
	guard, ok := dijkstraGuardCredentialAt(tx, index)
	if !ok || guard.CredType != common.CredentialTypeScriptHash {
		return false, nil
	}
	scriptHash := common.ScriptHash(guard.Credential)
	if witnessSetHasNativeScript(tx.Witnesses(), scriptHash) {
		return true, nil
	}
	if dijkstraTx, ok := tx.(*DijkstraTransaction); ok {
		for _, subTx := range dijkstraTx.Body.TxSubTransactions.Items() {
			if witnessSetHasNativeScript(subTx.WitnessSet, scriptHash) {
				return true, nil
			}
		}
	}
	if ls == nil {
		return false, nil
	}
	for _, refInput := range tx.ReferenceInputs() {
		utxo, err := ls.UtxoById(refInput)
		if err != nil {
			return false, common.ReferenceInputResolutionError{
				Input: refInput,
				Err:   err,
			}
		}
		if utxo.Output != nil &&
			scriptRefIsNativeHash(utxo.Output.ScriptRef(), scriptHash) {
			return true, nil
		}
	}
	for _, input := range tx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil || utxo.Output == nil {
			continue
		}
		if scriptRefIsNativeHash(utxo.Output.ScriptRef(), scriptHash) {
			return true, nil
		}
	}
	return false, nil
}

func witnessSetHasNativeScript(
	wits common.TransactionWitnessSet,
	scriptHash common.ScriptHash,
) bool {
	if wits == nil {
		return false
	}
	for _, script := range wits.NativeScripts() {
		if script.Hash() == scriptHash {
			return true
		}
	}
	return false
}

func scriptRefIsNativeHash(
	scriptRef common.Script,
	scriptHash common.ScriptHash,
) bool {
	if scriptRef == nil {
		return false
	}
	switch script := scriptRef.(type) {
	case common.NativeScript:
		return script.Hash() == scriptHash
	case *common.NativeScript:
		if script == nil {
			return false
		}
		return script.Hash() == scriptHash
	default:
		return false
	}
}

func UtxoValidateNativeScripts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	witnesses := tx.Witnesses()
	if witnesses == nil {
		return nil
	}
	nativeScripts := witnesses.NativeScripts()
	if len(nativeScripts) == 0 {
		return nil
	}
	keyHashes := make(map[common.Blake2b224]bool)
	for _, vkw := range witnesses.Vkey() {
		keyHashes[common.Blake2b224Hash(vkw.Vkey)] = true
	}
	for _, bw := range witnesses.Bootstrap() {
		keyHashes[common.Blake2b224Hash(bw.PublicKey)] = true
	}
	validityStart := tx.ValidityIntervalStart()
	validityEnd := tx.TTL()
	if validityEnd == 0 {
		validityEnd = ^uint64(0)
	}
	guardCredentials := nativeScriptGuardCredentials(tx)
	for _, nscript := range nativeScripts {
		scriptHash := nscript.Hash()
		if !nscript.EvaluateWithGuards(
			slot,
			validityStart,
			validityEnd,
			keyHashes,
			guardCredentials,
		) {
			return conway.NativeScriptFailedError{ScriptHash: scriptHash}
		}
	}
	return nil
}

func nativeScriptGuardCredentials(tx common.Transaction) []common.Credential {
	// Dijkstra native-script guards are Dijkstra body fields. This rule is
	// registered only for Dijkstra validation, whose transaction type has
	// pointer-only CBOR methods and therefore reaches this helper as
	// *DijkstraTransaction.
	dijkstraTx, ok := tx.(*DijkstraTransaction)
	if !ok || dijkstraTx.Body.TxGuards == nil {
		return nil
	}
	guards := dijkstraTx.Body.TxGuards
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
	var malformedHashes []common.ScriptHash
	for _, output := range tx.Outputs() {
		scriptRef := output.ScriptRef()
		if scriptRef == nil {
			continue
		}
		if _, ok := common.PlutusScriptVersion(scriptRef); !ok {
			continue
		}
		var innerScript []byte
		if _, err := cbor.Decode(scriptRef.RawScriptBytes(), &innerScript); err != nil {
			malformedHashes = append(malformedHashes, scriptRef.Hash())
			continue
		}
		if _, err := syn.Decode[syn.DeBruijn](innerScript); err != nil {
			malformedHashes = append(malformedHashes, scriptRef.Hash())
		}
	}
	if len(malformedHashes) > 0 {
		return common.MalformedReferenceScriptsError{
			ScriptHashes: malformedHashes,
		}
	}
	return nil
}

func UtxoValidateRefScriptSizePerTx(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, err := dijkstraPparams(pp)
	if err != nil {
		return err
	}
	maxSize := tmpPparams.MaxRefScriptSizePerTx
	if maxSize == 0 {
		return nil
	}
	totalSize := refScriptSize(tx)
	if totalSize > uint64(maxSize) {
		return common.RefScriptSizePerTxTooLargeError{
			TxSize:  totalSize,
			MaxSize: uint64(maxSize),
		}
	}
	return nil
}

func ValidateRefScriptSizePerBlock(
	block *DijkstraBlock,
	pp common.ProtocolParameters,
) error {
	tmpPparams, err := dijkstraPparams(pp)
	if err != nil {
		return err
	}
	maxSize := tmpPparams.MaxRefScriptSizePerBlock
	if maxSize == 0 {
		return nil
	}
	var totalSize uint64
	for _, tx := range block.Transactions() {
		totalSize += refScriptSize(tx)
	}
	if totalSize > uint64(maxSize) {
		return common.RefScriptSizePerBlockTooLargeError{
			BlockSize: totalSize,
			MaxSize:   uint64(maxSize),
		}
	}
	return nil
}

func refScriptSize(tx common.Transaction) uint64 {
	var totalSize uint64
	for _, output := range tx.Outputs() {
		scriptRef := output.ScriptRef()
		if scriptRef == nil {
			continue
		}
		totalSize += uint64(len(scriptRef.RawScriptBytes()))
	}
	return totalSize
}
