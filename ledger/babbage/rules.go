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

package babbage

import (
	"errors"
	"math"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

var utxoValidationRuleDescriptors = []common.UtxoValidationRuleDescriptor{
	{Id: common.UtxoValidationRuleMetadata, Validator: UtxoValidateMetadata},
	{
		Id:        common.UtxoValidationRuleIsValidFlag,
		Validator: UtxoValidateIsValidFlag,
	},
	{
		Id:        common.UtxoValidationRuleRequiredVKeyWitnesses,
		Validator: UtxoValidateRequiredVKeyWitnesses,
	},
	{
		Id:        common.UtxoValidationRuleSignatures,
		Validator: UtxoValidateSignatures,
	},
	{
		Id:        common.UtxoValidationRuleProtocolParameterUpdates,
		Validator: UtxoValidateProtocolParameterUpdates,
	},
	{
		Id:        common.UtxoValidationRuleCollateralVKeyWitnesses,
		Validator: UtxoValidateCollateralVKeyWitnesses,
	},
	{
		Id:        common.UtxoValidationRuleCollateralKeyLocked,
		Validator: common.UtxoValidateCollateralKeyLocked,
	},
	{
		Id:        common.UtxoValidationRuleRedeemerAndScriptWitnesses,
		Validator: UtxoValidateRedeemerAndScriptWitnesses,
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
		Validator: UtxoValidateInlineDatumsWithPlutusV1,
	},
	{
		Id:        common.UtxoValidationRuleSupplementalDatums,
		Validator: UtxoValidateSupplementalDatums,
	},
	{
		Id:        common.UtxoValidationRuleDisjointRefInputs,
		Validator: UtxoValidateDisjointRefInputs,
	},
	{
		Id:        common.UtxoValidationRuleOutsideValidityInterval,
		Validator: UtxoValidateOutsideValidityIntervalUtxo,
	},
	{
		Id:        common.UtxoValidationRuleOutsideForecast,
		Validator: common.UtxoValidateOutsideForecast,
	},
	{
		Id:        common.UtxoValidationRuleInputSetEmpty,
		Validator: UtxoValidateInputSetEmptyUtxo,
	},
	{
		Id:        common.UtxoValidationRuleNoDuplicateInputs,
		Validator: UtxoValidateNoDuplicateInputs,
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
		Validator: UtxoValidateCollateralEqBalance,
	},
	{
		Id:        common.UtxoValidationRuleNoCollateralInputs,
		Validator: UtxoValidateNoCollateralInputs,
	},
	{
		Id:        common.UtxoValidationRuleBadInputs,
		Validator: UtxoValidateBadInputsUtxo,
	},
	{
		Id:        common.UtxoValidationRuleScriptWitnesses,
		Validator: UtxoValidateScriptWitnesses,
	},
	{
		Id:        common.UtxoValidationRuleRequiredRedeemers,
		Validator: UtxoValidateRequiredRedeemers,
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
		Validator: UtxoValidateOutputBootAddrAttrsTooBig,
	},
	{
		Id:        common.UtxoValidationRuleWrongNetwork,
		Validator: UtxoValidateWrongNetwork,
	},
	{
		Id:        common.UtxoValidationRuleWrongNetworkWithdrawal,
		Validator: UtxoValidateWrongNetworkWithdrawal,
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
		Id:        common.UtxoValidationRuleNativeScripts,
		Validator: UtxoValidateNativeScripts,
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
		Id:        common.UtxoValidationRuleDelegation,
		Validator: UtxoValidateDelegation,
	},
	{
		Id:        common.UtxoValidationRuleWithdrawals,
		Validator: UtxoValidateWithdrawals,
	},
	{
		Id:        common.UtxoValidationRulePoolCertificates,
		Validator: UtxoValidatePoolCertificates,
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
var UtxoValidationRules = common.ComposeUtxoValidationRules(
	common.AlwaysUtxoValidationRules(
		UtxoValidateMetadata, UtxoValidateIsValidFlag, UtxoValidateRequiredVKeyWitnesses,
		UtxoValidateSignatures, UtxoValidateProtocolParameterUpdates,
		UtxoValidateCollateralVKeyWitnesses,
		common.UtxoValidateCollateralKeyLocked,
	),
	common.AlwaysUtxoValidationRules(
		UtxoValidateRedeemerAndScriptWitnesses, UtxoValidateCostModelsPresent,
		UtxoValidateScriptDataHash, UtxoValidateInlineDatumsWithPlutusV1,
		UtxoValidateSupplementalDatums,
		UtxoValidateDisjointRefInputs, UtxoValidateOutsideValidityIntervalUtxo,
		common.UtxoValidateOutsideForecast,
		UtxoValidateInputSetEmptyUtxo, UtxoValidateNoDuplicateInputs,
		UtxoValidateFeeTooSmallUtxo, UtxoValidateInsufficientCollateral,
		UtxoValidateCollateralContainsNonAda, UtxoValidateCollateralEqBalance,
		UtxoValidateNoCollateralInputs, UtxoValidateBadInputsUtxo,
		UtxoValidateScriptWitnesses, UtxoValidateRequiredRedeemers,
		UtxoValidateValueNotConservedUtxo, UtxoValidateOutputTooSmallUtxo,
		UtxoValidateOutputTooBigUtxo, UtxoValidateOutputBootAddrAttrsTooBig,
		UtxoValidateWrongNetwork, UtxoValidateWrongNetworkWithdrawal,
		UtxoValidateTransactionNetworkId, UtxoValidateMaxTxSizeUtxo,
		UtxoValidateExUnitsTooBigUtxo,
		UtxoValidateTooManyCollateralInputs, UtxoValidateNativeScripts,
		UtxoValidateExtraneousRedeemers, UtxoValidateMalformedReferenceScripts,
		UtxoValidatePlutusScripts,
	),
	common.Phase2ValidUtxoValidationRules(
		UtxoValidateDelegation, UtxoValidateWithdrawals, UtxoValidatePoolCertificates,
	),
)

func UtxoValidateOutsideValidityIntervalUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return allegra.UtxoValidateOutsideValidityIntervalUtxo(tx, slot, ls, pp)
}

// UtxoValidateIsValidFlag ensures transactions marked invalid have Plutus scripts
func UtxoValidateIsValidFlag(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// If IsValid is true, no check needed
	if tx.IsValid() {
		return nil
	}

	// If IsValid is false, transaction must have redeemers (indicating phase-2 validation)
	w := tx.Witnesses()
	if w != nil && w.Redeemers() != nil {
		for range w.Redeemers().Iter() {
			// Has at least one redeemer
			return nil
		}
	}

	// IsValid=false but no redeemers present
	return common.InvalidIsValidFlagError{}
}

func UtxoValidatePlutusScripts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateUnsupportedPlutusExecution(tx, "Babbage")
}

// UtxoValidateSupplementalDatums enforces required and supplemental datum
// rules for Babbage transactions.
func UtxoValidateSupplementalDatums(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	if err := common.ValidateRequiredSpendingDatums(tx, ls); err != nil {
		return err
	}
	return common.ValidateSupplementalDatums(tx, ls)
}

// UtxoValidateExtraneousRedeemers checks that all redeemers have valid
// purposes: a spending redeemer's index must reference an existing input,
// a minting redeemer an existing mint policy, a certifying redeemer an
// existing certificate, and a reward redeemer an existing withdrawal.
// Babbage predates governance, so voting/proposing/guarding redeemer tags
// are always extraneous here.
func UtxoValidateExtraneousRedeemers(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateExactExtraneousRedeemers(tx, ls)
}

// UtxoValidateRequiredVKeyWitnesses ensures required signers are accompanied by vkey witnesses
func UtxoValidateRequiredVKeyWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateRequiredVKeyWitnesses(tx)
}

func UtxoValidateProtocolParameterUpdates(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateProtocolParameterUpdates(tx, slot, ls, pp)
}

// UtxoValidateCollateralVKeyWitnesses ensures collateral inputs are backed by vkey witnesses
func UtxoValidateCollateralVKeyWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateCollateralVKeyWitnesses(tx, ls)
}

// UtxoValidateRedeemerAndScriptWitnesses performs lightweight UTXOW checks for presence/absence of scripts vs redeemers
func UtxoValidateRedeemerAndScriptWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateRedeemerAndScriptWitnesses(tx, ls)
}

// UtxoValidateCostModelsPresent ensures Plutus scripts have cost models in protocol parameters
func UtxoValidateCostModelsPresent(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*BabbageProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	required, err := common.UsedPlutusVersions(tx, ls)
	if err != nil {
		return err
	}

	if len(required) == 0 {
		return nil
	}

	for version := range required {
		model, ok := tmpPparams.CostModels[version]
		if !ok || len(model) == 0 {
			return common.MissingCostModelError{Version: version}
		}
	}

	return nil
}

// UtxoValidateInlineDatumsWithPlutusV1 rejects a transaction that requires a
// PlutusV1 script while carrying an inline datum. Inline datums are a Babbage
// feature the PlutusV1 script context cannot represent, so translating an output
// that carries one into the V1 context fails.
//
// Three properties matter, and getting any of them wrong diverges from
// cardano-node:
//
//   - Only a *needed* script constrains the transaction. A V1 script that is
//     merely reachable -- sitting in the witness set, or as a reference script
//     on some UTxO being spent -- but required by no script purpose does not
//     make the transaction invalid. Rejecting on availability turns an ordinary
//     transaction that happens to spend a UTxO carrying an unrelated V1
//     reference script into a permanent validation failure.
//   - Inline datums are disqualifying wherever the V1 context translates an
//     output: consumed inputs, reference inputs, and produced outputs.
//   - Babbage rejects reference inputs and reference scripts on translated
//     inputs and outputs. Conway keeps its more permissive translation.
//
// Datum presence is read through the TransactionOutput interface rather than a
// concrete Babbage type assertion, so outputs wrapped by a later era are still
// inspected. Datum-*hash* outputs are correctly not treated as inline, because
// Datum() reports nil for them.
func UtxoValidateInlineDatumsWithPlutusV1(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// An unresolvable input yields an empty view rather than an error:
	// UtxoValidateBadInputsUtxo reports it with the right error, and reporting
	// it here as well would make this rule a second, competing source of
	// input-resolution failures.
	view, err := script.NewTxScriptViewSkippingUnresolved(tx, ls)
	if err != nil {
		return err
	}
	needsV1 := view.NeedsAny(func(s common.Script) bool {
		_, ok := s.(common.PlutusV1Script)
		return ok
	})
	if !needsV1 {
		return nil
	}
	babbageEra := tx.Type() == TxTypeBabbage
	if babbageEra && len(tx.ReferenceInputs()) > 0 {
		return PlutusV1ReferenceInputsNotSupportedError{}
	}
	for _, utxo := range view.AllResolvedInputs() {
		if utxo.Output == nil {
			continue
		}
		if utxo.Output.Datum() != nil {
			return common.InlineDatumsNotSupportedError{
				PlutusVersion: "PlutusV1",
			}
		}
		if babbageEra && utxo.Output.ScriptRef() != nil {
			return PlutusV1ReferenceScriptsNotSupportedError{}
		}
	}
	for _, output := range tx.Outputs() {
		if output == nil {
			continue
		}
		if output.Datum() != nil {
			return common.InlineDatumsNotSupportedError{
				PlutusVersion: "PlutusV1",
			}
		}
		if babbageEra && output.ScriptRef() != nil {
			return PlutusV1ReferenceScriptsNotSupportedError{}
		}
	}
	return nil
}

func UtxoValidateInputSetEmptyUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateInputSetEmptyUtxo(tx, slot, ls, pp)
}

func UtxoValidateNoDuplicateInputs(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateNoDuplicateInputs(tx, slot, ls, pp)
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
	minFeeBig := new(big.Int).SetUint64(minFee)
	fee := tx.Fee()
	if fee == nil {
		fee = new(big.Int)
	}
	if fee.Cmp(minFeeBig) >= 0 {
		return nil
	}
	return shelley.FeeTooSmallUtxoError{
		Provided: fee,
		Min:      minFeeBig,
	}
}

func UtxoValidateInsufficientCollateral(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*BabbageProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*BabbageTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	// There's nothing to check if there are no redeemers
	if len(tmpTx.WitnessSet.WsRedeemers.Redeemers) == 0 {
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
	if collateralReturn := tx.CollateralReturn(); collateralReturn != nil {
		if amount := collateralReturn.Amount(); amount != nil {
			totalCollateral.Sub(totalCollateral, amount)
		}
	}
	fee := tmpTx.Fee()
	if fee == nil {
		fee = new(big.Int)
	}
	return alonzo.ValidateInsufficientCollateral(
		totalCollateral,
		fee,
		tmpPparams.CollateralPercentage,
	)
}

func UtxoValidateCollateralContainsNonAda(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpTx, ok := tx.(*BabbageTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	// There's nothing to check if there are no redeemers
	if len(tmpTx.WitnessSet.WsRedeemers.Redeemers) == 0 {
		return nil
	}
	totalCollateral := new(big.Int)
	totalAssets := common.NewMultiAsset[common.MultiAssetTypeOutput](nil)
	for _, collateralInput := range tx.Collateral() {
		utxo, err := common.ResolveInputUtxo(ls, collateralInput)
		if err != nil {
			return err
		}
		if amount := utxo.Output.Amount(); amount != nil {
			totalCollateral.Add(totalCollateral, amount)
		}
		totalAssets.Add(utxo.Output.Assets())
	}
	// Check if all collateral assets are accounted for in the collateral return
	collReturn := tx.CollateralReturn()
	var collReturnAssets *common.MultiAsset[common.MultiAssetTypeOutput]
	if collReturn != nil {
		collReturnAssets = collReturn.Assets()
	}
	if (&totalAssets).Compare(collReturnAssets) {
		return nil
	}
	var providedU uint64
	if totalCollateral.IsUint64() {
		providedU = totalCollateral.Uint64()
	}
	return alonzo.CollateralContainsNonAdaError{
		Provided: providedU,
	}
}

// UtxoValidateCollateralEqBalance ensures that the collateral return amount is equal to the collateral input amount minus the total collateral
//
// This is Part 6 of the reference's validateTotalCollateral, which feesOK runs
// only when the redeemer map is non-empty, exactly as it does for the rest of
// the collateral group. A transaction with no redeemers runs no phase-2
// scripts, so its total_collateral field is not checked and a mismatch is not
// a rejection. Conway and Dijkstra delegate here, so the guard uses the
// interface-level helper rather than the Babbage-typed witness set: a Dijkstra
// transaction can carry its redeemers in a sub-transaction, which the helper
// counts.
func UtxoValidateCollateralEqBalance(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	if !common.TransactionRunsPhase2Scripts(tx) {
		return nil
	}
	totalCollateral := tx.TotalCollateral()
	if !common.TransactionTotalCollateralPresent(tx) {
		return nil
	}
	// Collect collateral input amounts
	collBalance := new(big.Int)
	for _, collInput := range tx.Collateral() {
		utxo, err := common.ResolveInputUtxo(ls, collInput)
		if err != nil {
			return err
		}
		if amount := utxo.Output.Amount(); amount != nil {
			collBalance.Add(collBalance, amount)
		}
	}

	// Subtract collateral return amount with underflow protection
	collReturn := tx.CollateralReturn()
	if collReturn != nil {
		if returnAmount := collReturn.Amount(); returnAmount != nil {
			if collBalance.Cmp(returnAmount) < 0 {
				var totalCollU uint64
				if totalCollateral.IsUint64() {
					totalCollU = totalCollateral.Uint64()
				}
				return IncorrectTotalCollateralFieldError{
					Provided:        0,
					TotalCollateral: totalCollU,
				}
			}
			collBalance.Sub(collBalance, returnAmount)
		}
	}

	if totalCollateral.Cmp(collBalance) == 0 {
		return nil
	}
	var providedU, totalCollU uint64
	if collBalance.IsUint64() {
		providedU = collBalance.Uint64()
	}
	if totalCollateral.IsUint64() {
		totalCollU = totalCollateral.Uint64()
	}
	return IncorrectTotalCollateralFieldError{
		Provided:        providedU,
		TotalCollateral: totalCollU,
	}
}

// UtxoValidateSignatures verifies vkey and bootstrap signatures present in the transaction.
func UtxoValidateSignatures(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.UtxoValidateSignatures(tx, slot, ls, pp)
}

func UtxoValidateNoCollateralInputs(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpTx, ok := tx.(*BabbageTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	// There's nothing to check if there are no redeemers
	if len(tmpTx.WitnessSet.WsRedeemers.Redeemers) == 0 {
		return nil
	}
	if len(tx.Collateral()) > 0 {
		return nil
	}
	return alonzo.NoCollateralInputsError{}
}

func UtxoValidateBadInputsUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateBadInputsUtxo(tx, slot, ls, pp)
}

func UtxoValidateValueNotConservedUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*BabbageProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	// Calculate consumed value
	// consumed = value from input(s) + withdrawals + refunds
	consumedValue := new(big.Int)
	for _, tmpInput := range tx.Inputs() {
		tmpUtxo, err := ls.UtxoById(tmpInput)
		// Ignore errors fetching the UTxO and exclude it from calculations
		if err != nil {
			continue
		}
		if amount := tmpUtxo.Output.Amount(); amount != nil {
			consumedValue.Add(consumedValue, amount)
		}
	}
	for _, tmpWithdrawalAmount := range tx.Withdrawals() {
		if tmpWithdrawalAmount != nil {
			consumedValue.Add(consumedValue, tmpWithdrawalAmount)
		}
	}
	seenPoolRegistrations := make(map[common.PoolKeyHash]struct{})
	stakeCertificateEffectsValid := tx.IsValid() ||
		!common.TransactionRunsPhase2Scripts(tx)
	type stakeCredentialKey struct {
		credType uint
		hash     string
	}
	keyForCredential := func(cred common.Credential) stakeCredentialKey {
		return stakeCredentialKey{credType: cred.CredType, hash: string(cred.Credential[:])}
	}
	stakeRegistered := make(map[stakeCredentialKey]bool)
	stakeDeposits := make(map[stakeCredentialKey]uint64)
	if stakeCertificateEffectsValid {
		for _, cert := range tx.Certificates() {
			switch tmpCert := cert.(type) {
			case *common.StakeDeregistrationCertificate:
				cred := tmpCert.StakeCredential
				key := keyForCredential(cred)
				registered, ok := stakeRegistered[key]
				if !ok {
					registered = ls.IsStakeCredentialRegistered(cred)
					if registered {
						deposit, err := common.StakeCredentialDepositOrDefault(ls, cred, uint64(tmpPparams.KeyDeposit))
						if err != nil {
							return err
						}
						stakeDeposits[key] = deposit
					}
				}
				if registered {
					consumedValue.Add(consumedValue, new(big.Int).SetUint64(stakeDeposits[key]))
					stakeRegistered[key] = false
				}
			case *common.StakeRegistrationCertificate:
				key := keyForCredential(tmpCert.StakeCredential)
				stakeRegistered[key] = true
				stakeDeposits[key] = uint64(tmpPparams.KeyDeposit)
				// Note: PoolRetirementCertificate does NOT refund the deposit as part of the transaction.
				// Pool deposits are refunded to the reward account at the end of the retiring epoch.
			}
		}
	}
	// Calculate produced value
	// produced = value from output(s) + fee + deposits
	producedValue := new(big.Int)
	for _, tmpOutput := range tx.Outputs() {
		if amount := tmpOutput.Amount(); amount != nil {
			producedValue.Add(producedValue, amount)
		}
	}
	if fee := tx.Fee(); fee != nil {
		producedValue.Add(producedValue, fee)
	}
	for _, cert := range tx.Certificates() {
		switch tmpCert := cert.(type) {
		case *common.PoolRegistrationCertificate:
			operator := common.Blake2b224(tmpCert.Operator)
			if _, seen := seenPoolRegistrations[operator]; seen {
				continue
			}
			seenPoolRegistrations[operator] = struct{}{}
			depositDue, err := common.PoolRegistrationDepositDue(
				ls, slot, operator,
			)
			if err != nil {
				return err
			}
			if depositDue {
				producedValue.Add(producedValue, new(big.Int).SetUint64(uint64(tmpPparams.PoolDeposit)))
			}
		case *common.StakeRegistrationCertificate:
			if stakeCertificateEffectsValid {
				producedValue.Add(producedValue, new(big.Int).SetUint64(uint64(tmpPparams.KeyDeposit)))
			}
		}
	}
	if consumedValue.Cmp(producedValue) != 0 {
		return shelley.ValueNotConservedUtxoError{
			Consumed: consumedValue,
			Produced: producedValue,
		}
	}

	// Multi-asset value conservation check
	// For each policy and asset: consumed + minted == produced
	type assetKey struct {
		policy common.Blake2b224
		asset  string
	}

	consumedAssets := make(map[assetKey]*big.Int)
	producedAssets := make(map[assetKey]*big.Int)

	// Collect consumed multi-assets from inputs
	for _, tmpInput := range tx.Inputs() {
		tmpUtxo, err := ls.UtxoById(tmpInput)
		if err != nil {
			continue
		}
		if assets := tmpUtxo.Output.Assets(); assets != nil {
			for _, policy := range assets.Policies() {
				for _, assetName := range assets.Assets(policy) {
					amount := assets.Asset(policy, assetName)
					if amount == nil {
						continue
					}
					key := assetKey{policy: policy, asset: string(assetName)}
					if consumedAssets[key] == nil {
						consumedAssets[key] = new(big.Int)
					}
					consumedAssets[key].Add(consumedAssets[key], amount)
				}
			}
		}
	}

	// Add minted/burned assets to consumed (positive for mint, negative for burn)
	if mint := tx.AssetMint(); mint != nil {
		for _, policy := range mint.Policies() {
			for _, assetName := range mint.Assets(policy) {
				if policy == (common.Blake2b224{}) && len(assetName) == 0 {
					continue
				}
				amount := mint.Asset(policy, assetName)
				if amount == nil {
					continue
				}
				key := assetKey{policy: policy, asset: string(assetName)}
				if consumedAssets[key] == nil {
					consumedAssets[key] = new(big.Int)
				}
				consumedAssets[key].Add(consumedAssets[key], amount)
			}
		}
	}

	// Collect produced multi-assets from outputs
	for _, tmpOutput := range tx.Outputs() {
		if assets := tmpOutput.Assets(); assets != nil {
			for _, policy := range assets.Policies() {
				for _, assetName := range assets.Assets(policy) {
					amount := assets.Asset(policy, assetName)
					if amount == nil {
						continue
					}
					key := assetKey{policy: policy, asset: string(assetName)}
					if producedAssets[key] == nil {
						producedAssets[key] = new(big.Int)
					}
					producedAssets[key].Add(producedAssets[key], amount)
				}
			}
		}
	}

	// Check that all consumed assets match produced assets without building
	// an intermediate union set of keys.
	zero := new(big.Int)
	for key, consumed := range consumedAssets {
		produced := producedAssets[key]
		if produced == nil {
			produced = zero
		}
		if consumed.Cmp(produced) != 0 {
			return shelley.ValueNotConservedUtxoError{
				Consumed: consumed,
				Produced: produced,
			}
		}
		delete(producedAssets, key)
	}
	for _, produced := range producedAssets {
		if produced.Cmp(zero) != 0 {
			return shelley.ValueNotConservedUtxoError{
				Consumed: zero,
				Produced: produced,
			}
		}
	}

	return nil
}

func UtxoValidateOutputTooSmallUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	var badOutputs []common.TransactionOutput
	for _, tmpOutput := range common.TransactionOutputsAndCollateralReturn(tx) {
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
	return shelley.OutputTooSmallUtxoError{
		Outputs: badOutputs,
	}
}

func UtxoValidateOutputTooBigUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*BabbageProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	badOutputs := []common.TransactionOutput{}
	for _, txOutput := range common.TransactionOutputsAndCollateralReturn(tx) {
		tmpOutput, ok := txOutput.(*BabbageTransactionOutput)
		if !ok {
			return errors.New("transaction output is not expected type")
		}
		outputValBytes, err := cbor.Encode(tmpOutput.OutputAmount)
		if err != nil {
			return err
		}
		if uint(len(outputValBytes)) <= tmpPparams.MaxValueSize {
			continue
		}
		badOutputs = append(badOutputs, tmpOutput)
	}
	if len(badOutputs) == 0 {
		return nil
	}
	return mary.OutputTooBigUtxoError{
		Outputs: badOutputs,
	}
}

func UtxoValidateOutputBootAddrAttrsTooBig(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateOutputBootAddrAttrsTooBig(tx, slot, ls, pp)
}

func UtxoValidateWrongNetwork(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateWrongNetwork(tx, slot, ls, pp)
}

func UtxoValidateWrongNetworkWithdrawal(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateWrongNetworkWithdrawal(tx, slot, ls, pp)
}

// UtxoValidateTransactionNetworkId validates a present transaction network
// identifier against the active ledger network. An absent identifier retains
// the Babbage-era behavior and is accepted.
func UtxoValidateTransactionNetworkId(
	tx common.Transaction,
	_ uint64,
	ls common.LedgerState,
	_ common.ProtocolParameters,
) error {
	txWithNetworkId, ok := tx.(interface{ TransactionNetworkId() *uint8 })
	if !ok {
		return errors.New("transaction does not expose a network identifier")
	}
	txNetworkId := txWithNetworkId.TransactionNetworkId()
	if txNetworkId == nil || uint(*txNetworkId) == ls.NetworkId() {
		return nil
	}
	return common.WrongTransactionNetworkIdError{
		TxNetworkId:     *txNetworkId,
		LedgerNetworkId: ls.NetworkId(),
	}
}

func UtxoValidateMaxTxSizeUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*BabbageProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	txSize, sizeErr := common.TxSize(tx)
	if sizeErr != nil {
		return sizeErr
	}
	if uint(txSize) <= tmpPparams.MaxTxSize {
		return nil
	}
	return shelley.MaxTxSizeUtxoError{
		TxSize:    uint(txSize),
		MaxTxSize: tmpPparams.MaxTxSize,
	}
}

func UtxoValidateExUnitsTooBigUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*BabbageProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*BabbageTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	// Iterate the collapsed view rather than the raw list: the wire format is a
	// list but the ledger holds a map, so a key appearing more than once
	// contributes its budget once. See the Alonzo rule of the same name; this
	// is where it bites in practice, since Preview transaction 3ace3bc7f4c5 at
	// slot 12925989 is a Babbage-era block (blinklabs-io/dingo#3875).
	var totalSteps, totalMemory int64
	for _, redeemer := range tmpTx.WitnessSet.WsRedeemers.Iter() {
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
	tmpPparams, ok := pp.(*BabbageProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	collateralCount := uint(len(tx.Collateral()))
	if collateralCount <= tmpPparams.MaxCollateralInputs {
		return nil
	}
	return TooManyCollateralInputsError{
		Provided: collateralCount,
		Max:      tmpPparams.MaxCollateralInputs,
	}
}

// MinFeeTx calculates the minimum required fee for a transaction based on
// protocol parameters. The fee-relevant transaction size is determined by
// common.TxSizeForFee, which uses the original on-wire CBOR length and
// subtracts 1 byte for Alonzo+ eras (the IsValid boolean is excluded from
// the fee computation per the Cardano ledger spec toCBORForSizeComputation).
func MinFeeTx(
	tx common.Transaction,
	pparams common.ProtocolParameters,
) (uint64, error) {
	tmpPparams, ok := pparams.(*BabbageProtocolParameters)
	if !ok {
		return 0, errors.New("pparams are not expected type")
	}
	txSize, err := common.TxSizeForFee(tx)
	if err != nil {
		return 0, err
	}
	minFee, err := common.CalculateMinFee(
		txSize,
		tmpPparams.MinFeeA,
		tmpPparams.MinFeeB,
	)
	if err != nil {
		return 0, err
	}
	executionFee, err := common.CalculateExecutionUnitsFee(
		tx,
		tmpPparams.ExecutionCosts,
	)
	if err != nil {
		return 0, err
	}
	if minFee > math.MaxUint64-executionFee {
		return 0, errors.New("minimum transaction fee overflow")
	}
	return minFee + executionFee, nil
}

// MinCoinTxOut calculates the minimum coin for a transaction output based on protocol parameters.
// Per CIP-55, the formula includes a 160-byte constant overhead to account for the transaction
// input and UTxO map entry overhead that is not captured in the CBOR serialization.
// Formula: minCoin = coinsPerUTxOByte * (160 + serializedOutputSize)
// Reference: https://cips.cardano.org/cip/CIP-55
const minUtxoOverheadBytes = 160

func MinCoinTxOut(
	txOut common.TransactionOutput,
	pparams common.ProtocolParameters,
) (uint64, error) {
	tmpPparams, ok := pparams.(*BabbageProtocolParameters)
	if !ok {
		return 0, errors.New("pparams are not expected type")
	}
	txOutSize, err := common.TransactionOutputCborSize(txOut)
	if err != nil {
		return 0, err
	}
	// The reference computes this in unbounded Integer arithmetic, so a
	// coinsPerUTxOByte large enough to overflow uint64 yields a requirement
	// no output can meet. Wrapping would instead produce a small
	// requirement and admit those outputs.
	entrySize := minUtxoOverheadBytes + txOutSize
	if tmpPparams.AdaPerUtxoByte != 0 &&
		entrySize > math.MaxUint64/tmpPparams.AdaPerUtxoByte {
		return 0, errors.New("minimum UTxO value overflow")
	}
	return tmpPparams.AdaPerUtxoByte * entrySize, nil
}

func UtxoValidateMetadata(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	if err := shelley.UtxoValidateMetadata(tx, slot, ls, pp); err != nil {
		return err
	}
	params, ok := pp.(*BabbageProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	return common.ValidateAuxiliaryDataScriptsWellFormed(
		tx,
		params.ProtocolMajor,
	)
}

func UtxoValidateDelegation(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateDelegation(tx, slot, ls, pp)
}

// UtxoValidateDisjointRefInputs ensures reference inputs don't overlap with regular inputs.
// This rule only applies after Babbage era (protocol version > 8).
func UtxoValidateDisjointRefInputs(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// This rule only applies after Babbage era (protocol version > 8)
	// If the parameters are BabbageProtocolParameters and version <= 8, skip validation
	if tmpPparams, ok := pp.(*BabbageProtocolParameters); ok {
		if tmpPparams.ProtocolMajor <= 8 {
			return nil
		}
	}
	// For ConwayProtocolParameters or newer eras, always enforce the rule

	// Build a set of regular input strings for O(1) lookup
	inputSet := make(map[string]common.TransactionInput)
	for _, input := range tx.Inputs() {
		inputSet[input.String()] = input
	}

	// Check for overlaps with reference inputs
	var commonInputs []common.TransactionInput
	seen := make(map[string]bool)
	for _, refInput := range tx.ReferenceInputs() {
		key := refInput.String()
		if input, exists := inputSet[key]; exists && !seen[key] {
			commonInputs = append(commonInputs, input)
			seen[key] = true // Avoid duplicates
		}
	}
	if len(commonInputs) == 0 {
		return nil
	}
	return NonDisjointRefInputsError{
		Inputs: commonInputs,
	}
}

// UtxoValidateScriptWitnesses checks that script witnesses are provided for all script address inputs.
func UtxoValidateScriptWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateScriptWitnesses(tx, ls)
}

// UtxoValidateRequiredRedeemers checks that every Plutus script-address
// input -- whether its script is provided as an explicit witness or as a
// CIP-33 reference script -- has a matching spend redeemer. See
// script.ValidateRequiredRedeemers for details on the gap this closes.
// Babbage never executes Plutus itself (see UtxoValidatePlutusScripts
// below), but a reference-script-backed input with no redeemer must still
// be rejected rather than silently spent unexecuted.
func UtxoValidateRequiredRedeemers(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return script.ValidateRequiredRedeemers(tx, ls)
}

// UtxoValidateNativeScripts evaluates the native scripts this transaction has
// to satisfy.
//
// Babbage is the first era in which a script can reach a transaction as a
// reference script rather than a witness, so unlike the Alonzo rule this
// replaces, the scripts to evaluate come from the resolved transaction view
// rather than the witness set alone: a native script some script purpose
// requires counts whether the witness set, a reference input, or the spent
// input's own reference-script field supplies it -- the same three sources
// UtxoValidateScriptWitnesses accepts a required script from, so a script that
// rule counts as provided is a script this rule evaluates rather than ignores.
func UtxoValidateNativeScripts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	view, err := script.NewTxScriptViewSkippingUnresolved(tx, ls)
	if err != nil {
		return err
	}
	env := script.NewNativeScriptEnv(tx, slot)
	for _, nativeScript := range script.NativeScriptsToEvaluate(tx, view) {
		if !nativeScript.Evaluate(
			env.Slot,
			env.ValidityStart,
			env.ValidityEnd,
			env.KeyHashes,
		) {
			return allegra.NativeScriptFailedError{
				ScriptHash: nativeScript.Hash(),
			}
		}
	}
	return nil
}

// UtxoValidateWithdrawals validates withdrawals against ledger state.
// For phase-2 invalid transactions (IsValid=false), withdrawal validation is
// skipped since their effects are reverted and only collateral rules apply.
func UtxoValidateWithdrawals(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	if !tx.IsValid() {
		return nil
	}
	return shelley.UtxoValidateWithdrawals(tx, slot, ls, pp)
}

// UtxoValidateScriptDataHash validates the transaction's ScriptDataHash against the expected hash
// computed from redeemers, datums, and cost models (language views).
// Babbage supports PlutusV1 and PlutusV2 scripts.
func UtxoValidateScriptDataHash(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*BabbageProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*BabbageTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}

	wits := tmpTx.WitnessSet
	hasRedeemers := len(wits.WsRedeemers.Redeemers) > 0
	hasDatums := len(wits.WsPlutusData.Items) > 0

	declaredHash := tx.ScriptDataHash()

	// ScriptDataHash is required only when the transaction has redeemers or
	// witness datums, indicating actual script execution. The mere presence
	// of ScriptRefs in consumed/referenced UTxOs does NOT require a hash —
	// they are inert data unless matched by a redeemer.
	if !hasRedeemers && !hasDatums {
		if declaredHash != nil {
			return common.ExtraneousScriptDataHashError{Provided: *declaredHash}
		}
		return nil
	}

	if declaredHash == nil {
		return common.MissingScriptDataHashError{}
	}

	// The language views cover the Plutus scripts some script purpose of this
	// transaction requires, not every script it can reach. A reference script
	// on a spent or referenced input that no purpose needs is inert data (see
	// the comment above), and counting it adds a view the producer did not,
	// which rejects a canonical transaction on a hash it never declared
	// (gouroboros #2188).
	view, err := script.NewTxScriptView(tx, ls)
	if err != nil {
		if errors.Is(err, common.ErrInputResolution) {
			// A spent input that does not resolve is reported by
			// UtxoValidateBadInputsUtxo, which runs on every transaction in
			// this same rule list. Reporting it from here would change which
			// error an invalid transaction produces, and this rule is
			// registered ahead of that one. A reference input that does not
			// resolve has no such dedicated rule, so it still surfaces here.
			return nil
		}
		return err
	}
	usedVersions := view.UsedPlutusVersions()

	// Verify cost models are present for all used Plutus versions
	for version := range usedVersions {
		if _, ok := tmpPparams.CostModels[version]; !ok {
			return common.MissingCostModelError{Version: version}
		}
	}

	// Compute the expected ScriptDataHash
	// ScriptDataHash = blake2b256(redeemers_cbor || datums_cbor || langviews_cbor)
	//
	// Use preserved CBOR bytes from the original transaction for exact byte-for-byte match.
	// The hash was computed by the original submitter using their CBOR encoding.

	redeemersCbor := wits.WsRedeemers.Cbor()
	if len(redeemersCbor) == 0 {
		// Fall back to re-encoding if no preserved CBOR
		// Note: Must encode empty slice explicitly, as nil encodes as 0xf6 (CBOR null)
		// but the spec expects 0x80 (empty array) for empty redeemers
		var err error
		if wits.WsRedeemers.Redeemers == nil {
			redeemersCbor, err = cbor.Encode([]alonzo.AlonzoRedeemer{})
		} else {
			redeemersCbor, err = cbor.Encode(wits.WsRedeemers.Redeemers)
		}
		if err != nil {
			return err
		}
	}

	// Get datums CBOR using preserved bytes (only if non-empty)
	var datumsCbor []byte
	if hasDatums {
		datumsCbor = wits.WsPlutusData.Cbor()
		if len(datumsCbor) == 0 {
			// Fall back to re-encoding if no preserved CBOR
			var err error
			datumsCbor, err = cbor.Encode(wits.WsPlutusData.Items)
			if err != nil {
				return err
			}
		}
	}

	// Encode language views per the Cardano spec
	langViewsCbor, err := common.EncodeLangViews(
		usedVersions,
		tmpPparams.CostModels,
	)
	if err != nil {
		return err
	}

	// Concatenate and hash
	hashInput := make(
		[]byte,
		0,
		len(redeemersCbor)+len(datumsCbor)+len(langViewsCbor),
	)
	hashInput = append(hashInput, redeemersCbor...)
	hashInput = append(hashInput, datumsCbor...)
	hashInput = append(hashInput, langViewsCbor...)

	computedHash := common.Blake2b256Hash(hashInput)

	// Compare with declared hash
	// Note: declaredHash is guaranteed non-nil here due to earlier checks,
	// but we add an explicit check to satisfy static analysis
	if declaredHash == nil {
		return common.MissingScriptDataHashError{}
	}
	if *declaredHash != computedHash {
		return common.ScriptDataHashMismatchError{
			Declared: *declaredHash,
			Computed: computedHash,
		}
	}

	return nil
}

// UtxoValidateMalformedReferenceScripts checks that Plutus witnesses and
// reference scripts are well-formed for the active protocol version.
func UtxoValidateMalformedReferenceScripts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	params, ok := pp.(*BabbageProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	return common.ValidatePlutusScriptsWellFormed(tx, params.ProtocolMajor)
}

// UtxoValidatePoolCertificates applies the Shelley POOL rule, which this era
// inherits unchanged.
//
// Reference: eras/babbage/impl/src/Cardano/Ledger/Babbage/Rules/Pool.hs
// declares only the EraRuleFailure and EraRuleEvent instances and reuses
// Shelley.poolTransition.
func UtxoValidatePoolCertificates(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidatePoolCertificates(tx, slot, ls, pp)
}

// UtxoValidateMIRGenesisQuorum ensures a move instantaneous rewards
// certificate is authorized by a quorum of the current genesis delegates
func UtxoValidateMIRGenesisQuorum(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateMIRGenesisQuorum(tx, slot, ls, pp)
}
