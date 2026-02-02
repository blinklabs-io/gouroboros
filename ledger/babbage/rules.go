// Copyright 2025 Blink Labs Software
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
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/syn"
)

var UtxoValidationRules = []common.UtxoValidationRuleFunc{
	UtxoValidateMetadata,
	UtxoValidateIsValidFlag,
	UtxoValidateRequiredVKeyWitnesses,
	UtxoValidateSignatures,
	UtxoValidateCollateralVKeyWitnesses,
	UtxoValidateRedeemerAndScriptWitnesses,
	UtxoValidateCostModelsPresent,
	UtxoValidateScriptDataHash,
	UtxoValidateInlineDatumsWithPlutusV1,
	UtxoValidateDisjointRefInputs,
	UtxoValidateOutsideValidityIntervalUtxo,
	UtxoValidateInputSetEmptyUtxo,
	UtxoValidateFeeTooSmallUtxo,
	UtxoValidateInsufficientCollateral,
	UtxoValidateCollateralContainsNonAda,
	UtxoValidateCollateralEqBalance,
	UtxoValidateNoCollateralInputs,
	UtxoValidateBadInputsUtxo,
	UtxoValidateScriptWitnesses,
	UtxoValidateValueNotConservedUtxo,
	UtxoValidateOutputTooSmallUtxo,
	UtxoValidateOutputTooBigUtxo,
	UtxoValidateOutputBootAddrAttrsTooBig,
	UtxoValidateWrongNetwork,
	UtxoValidateWrongNetworkWithdrawal,
	UtxoValidateMaxTxSizeUtxo,
	UtxoValidateExUnitsTooBigUtxo,
	UtxoValidateTooManyCollateralInputs,
	UtxoValidateNativeScripts,
	UtxoValidateDelegation,
	UtxoValidateWithdrawals,
	UtxoValidateMalformedReferenceScripts,
}

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

// UtxoValidateRequiredVKeyWitnesses ensures required signers are accompanied by vkey witnesses
func UtxoValidateRequiredVKeyWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return common.ValidateRequiredVKeyWitnesses(tx)
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
	tmpTx, ok := tx.(*BabbageTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}

	required := map[uint]struct{}{}
	wits := tmpTx.WitnessSet
	if len(wits.WsPlutusV1Scripts) > 0 {
		required[0] = struct{}{}
	}
	if len(wits.WsPlutusV2Scripts) > 0 {
		required[1] = struct{}{}
	}
	// Include reference scripts on reference inputs
	// Note: Reference input errors must be caught here since there's no separate
	// BadReferenceInputsUtxo rule, unlike regular inputs which are caught by BadInputsUtxo
	for _, refInput := range tmpTx.ReferenceInputs() {
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
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		switch script.(type) {
		case common.PlutusV1Script:
			required[0] = struct{}{}
		case common.PlutusV2Script:
			required[1] = struct{}{}
		}
	}

	// Per CIP-33, also include reference scripts on regular (spent) inputs
	for _, input := range tmpTx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			// Skip errors - BadInputsUtxo will catch this
			continue
		}
		if utxo.Output == nil {
			continue
		}
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		switch script.(type) {
		case common.PlutusV1Script:
			required[0] = struct{}{}
		case common.PlutusV2Script:
			required[1] = struct{}{}
		}
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

// UtxoValidateInlineDatumsWithPlutusV1 rejects transactions that use inline datums with PlutusV1 scripts
// Inline datums are a Babbage-era feature and are only supported with PlutusV2+
func UtxoValidateInlineDatumsWithPlutusV1(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	// Check if transaction spends any UTxOs with inline datums
	hasInlineDatums := false
	for _, input := range tx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			// Input not found in ledger state, skip
			continue
		}
		babbageOutput, ok := utxo.Output.(*BabbageTransactionOutput)
		if !ok {
			continue
		}
		if babbageOutput.DatumOption != nil &&
			babbageOutput.DatumOption.data != nil {
			hasInlineDatums = true
			break
		}
	}

	if !hasInlineDatums {
		return nil
	}

	// Check if transaction uses PlutusV1 scripts in witnesses
	witnesses := tx.Witnesses()
	if witnesses != nil {
		v1Scripts := witnesses.PlutusV1Scripts()
		if len(v1Scripts) > 0 {
			return common.InlineDatumsNotSupportedError{
				PlutusVersion: "PlutusV1",
			}
		}
	}

	// Check reference scripts on reference inputs
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
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		switch script.(type) {
		case common.PlutusV1Script:
			return common.InlineDatumsNotSupportedError{
				PlutusVersion: "PlutusV1",
			}
		}
	}

	// Per CIP-33, also check reference scripts on regular (spent) inputs
	for _, input := range tx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			// Skip errors - BadInputsUtxo will catch this
			continue
		}
		if utxo.Output == nil {
			continue
		}
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		switch script.(type) {
		case common.PlutusV1Script:
			return common.InlineDatumsNotSupportedError{
				PlutusVersion: "PlutusV1",
			}
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
		utxo, err := ls.UtxoById(collateralInput)
		if err != nil {
			return err
		}
		if utxo.Output == nil {
			continue
		}
		if amount := utxo.Output.Amount(); amount != nil {
			totalCollateral.Add(totalCollateral, amount)
		}
	}
	// minCollateral = fee * collateralPercentage / 100
	fee := tmpTx.Fee()
	if fee == nil {
		fee = new(big.Int)
	}
	minCollateral := new(
		big.Int,
	).Mul(fee, new(big.Int).SetUint64(uint64(tmpPparams.CollateralPercentage)))
	minCollateral.Div(minCollateral, big.NewInt(100))
	if totalCollateral.Cmp(minCollateral) >= 0 {
		return nil
	}
	// Convert to uint64 for error struct (best effort)
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
	tmpTx, ok := tx.(*BabbageTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	// There's nothing to check if there are no redeemers
	if len(tmpTx.WitnessSet.WsRedeemers.Redeemers) == 0 {
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
		if utxo.Output == nil {
			continue
		}
		if amount := utxo.Output.Amount(); amount != nil {
			totalCollateral.Add(totalCollateral, amount)
		}
		totalAssets.Add(utxo.Output.Assets())
		if utxo.Output.Assets() == nil ||
			len(utxo.Output.Assets().Policies()) == 0 {
			continue
		}
		badOutputs = append(badOutputs, utxo.Output)
	}
	if len(badOutputs) == 0 {
		return nil
	}
	// Check if all collateral assets are accounted for in the collateral return
	collReturn := tx.CollateralReturn()
	if collReturn != nil {
		collReturnAssets := collReturn.Assets()
		if (&totalAssets).Compare(collReturnAssets) {
			return nil
		}
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
func UtxoValidateCollateralEqBalance(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	totalCollateral := tx.TotalCollateral()
	if totalCollateral == nil || totalCollateral.Sign() == 0 {
		return nil
	}
	// Collect collateral input amounts
	collBalance := new(big.Int)
	for _, collInput := range tx.Collateral() {
		utxo, err := ls.UtxoById(collInput)
		if err != nil {
			continue
		}
		if utxo.Output == nil {
			continue
		}
		if amount := utxo.Output.Amount(); amount != nil {
			collBalance.Add(collBalance, amount)
		}
	}

	// Skip validation if no valid collateral UTxOs were found
	// This avoids subtracting from zero and prevents uint underflow
	if collBalance.Sign() == 0 {
		return nil
	}

	// Subtract collateral return amount with underflow protection
	collReturn := tx.CollateralReturn()
	if collReturn != nil {
		if returnAmount := collReturn.Amount(); returnAmount != nil &&
			collBalance.Cmp(returnAmount) >= 0 {
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
	for _, cert := range tx.Certificates() {
		switch cert.(type) {
		case *common.StakeDeregistrationCertificate:
			consumedValue.Add(consumedValue, new(big.Int).SetUint64(uint64(tmpPparams.KeyDeposit)))
			// Note: PoolRetirementCertificate does NOT refund the deposit as part of the transaction.
			// Pool deposits are refunded to the reward account at the end of the retiring epoch.
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
			reg, _, err := ls.PoolCurrentState(common.Blake2b224(tmpCert.Operator))
			if err != nil {
				return err
			}
			if reg == nil {
				producedValue.Add(producedValue, new(big.Int).SetUint64(uint64(tmpPparams.PoolDeposit)))
			}
		case *common.StakeRegistrationCertificate:
			producedValue.Add(producedValue, new(big.Int).SetUint64(uint64(tmpPparams.KeyDeposit)))
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
			// Skip ADA (empty policy ID) as it's tracked separately in consumed/produced value
			if policy == (common.Blake2b224{}) {
				continue
			}
			for _, assetName := range mint.Assets(policy) {
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

	// Check that all consumed assets match produced assets
	allKeys := make(map[assetKey]bool)
	for k := range consumedAssets {
		allKeys[k] = true
	}
	for k := range producedAssets {
		allKeys[k] = true
	}

	for key := range allKeys {
		consumed := consumedAssets[key]
		produced := producedAssets[key]
		if consumed == nil {
			consumed = new(big.Int)
		}
		if produced == nil {
			produced = new(big.Int)
		}
		if consumed.Cmp(produced) != 0 {
			return shelley.ValueNotConservedUtxoError{
				Consumed: consumed,
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
	for _, txOutput := range tx.Outputs() {
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
	txBytes, err := cbor.Encode(tx)
	if err != nil {
		return err
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
	tmpPparams, ok := pp.(*BabbageProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*BabbageTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	var totalSteps, totalMemory int64
	for _, redeemer := range tmpTx.WitnessSet.WsRedeemers.Redeemers {
		totalSteps += redeemer.ExUnits.Steps
		totalMemory += redeemer.ExUnits.Memory
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

// MinFeeTx calculates the minimum required fee for a transaction based on protocol parameters
// Fee is calculated using the transaction body CBOR size as per Cardano protocol
func MinFeeTx(
	tx common.Transaction,
	pparams common.ProtocolParameters,
) (uint64, error) {
	tmpPparams, ok := pparams.(*BabbageProtocolParameters)
	if !ok {
		return 0, errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*BabbageTransaction)
	if !ok {
		return 0, errors.New("tx is not expected type")
	}
	// Encode a local copy of the body with TxFee set to 0 to calculate size without fee
	body := tmpTx.Body
	body.TxFee = 0
	txBytes, err := cbor.Encode(body)
	if err != nil {
		return 0, err
	}
	minFee := common.CalculateMinFee(
		len(txBytes),
		tmpPparams.MinFeeA,
		tmpPparams.MinFeeB,
	)
	return minFee, nil
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
	txOutBytes, err := cbor.Encode(txOut)
	if err != nil {
		return 0, err
	}
	minCoinTxOut := tmpPparams.AdaPerUtxoByte * (minUtxoOverheadBytes + uint64(len(txOutBytes)))
	return minCoinTxOut, nil
}

func UtxoValidateMetadata(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateMetadata(tx, slot, ls, pp)
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

// UtxoValidateNativeScripts evaluates native scripts in the transaction.
func UtxoValidateNativeScripts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return alonzo.UtxoValidateNativeScripts(tx, slot, ls, pp)
}

// UtxoValidateWithdrawals validates withdrawals against ledger state.
func UtxoValidateWithdrawals(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
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

	// Determine which Plutus versions are used (Babbage has PlutusV1 and V2)
	usedVersions := make(map[uint]struct{})
	if len(wits.WsPlutusV1Scripts) > 0 {
		usedVersions[0] = struct{}{}
	}
	if len(wits.WsPlutusV2Scripts) > 0 {
		usedVersions[1] = struct{}{}
	}

	// Also check reference scripts on reference inputs
	for _, refInput := range tmpTx.ReferenceInputs() {
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
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		switch script.(type) {
		case common.PlutusV1Script:
			usedVersions[0] = struct{}{}
		case common.PlutusV2Script:
			usedVersions[1] = struct{}{}
		}
	}

	// Check scripts in regular inputs
	for _, input := range tmpTx.Inputs() {
		utxo, err := ls.UtxoById(input)
		if err != nil {
			continue
		}
		if utxo.Output == nil {
			continue
		}
		script := utxo.Output.ScriptRef()
		if script == nil {
			continue
		}
		switch script.(type) {
		case common.PlutusV1Script:
			usedVersions[0] = struct{}{}
		case common.PlutusV2Script:
			usedVersions[1] = struct{}{}
		}
	}

	hasPlutusScripts := len(usedVersions) > 0
	declaredHash := tx.ScriptDataHash()

	// If no Plutus scripts and no redeemers/datums, ScriptDataHash should be absent
	if !hasPlutusScripts && !hasRedeemers && !hasDatums {
		if declaredHash != nil {
			return common.ExtraneousScriptDataHashError{Provided: *declaredHash}
		}
		return nil
	}

	// If there are Plutus scripts/redeemers/datums, ScriptDataHash is required
	if hasPlutusScripts || hasRedeemers || hasDatums {
		if declaredHash == nil {
			return common.MissingScriptDataHashError{}
		}
	}

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

// UtxoValidateMalformedReferenceScripts checks that any reference scripts in
// transaction outputs are well-formed and can be deserialized.
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

		// Check if the script can be decoded properly
		var innerScript []byte
		var isPlutus bool
		var scriptBytes []byte
		var scriptHash common.ScriptHash

		switch s := scriptRef.(type) {
		case common.PlutusV1Script:
			isPlutus = true
			scriptBytes = []byte(s)
			scriptHash = s.Hash()
		case common.PlutusV2Script:
			isPlutus = true
			scriptBytes = []byte(s)
			scriptHash = s.Hash()
		default:
			// Native scripts don't need UPLC validation
			continue
		}

		if isPlutus {
			// Decode the outer CBOR wrapper to get the actual script bytes
			if _, err := cbor.Decode(scriptBytes, &innerScript); err != nil {
				malformedHashes = append(malformedHashes, scriptHash)
				continue
			}
			// Try to decode as UPLC program
			if _, err := syn.Decode[syn.DeBruijn](innerScript); err != nil {
				malformedHashes = append(malformedHashes, scriptHash)
			}
		}
	}

	if len(malformedHashes) > 0 {
		return common.MalformedReferenceScriptsError{
			ScriptHashes: malformedHashes,
		}
	}
	return nil
}
