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

package alonzo

import (
	"errors"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

var UtxoValidationRules = []common.UtxoValidationRuleFunc{
	UtxoValidateMetadata,
	UtxoValidateIsValidFlag,
	UtxoValidateRequiredVKeyWitnesses,
	UtxoValidateSignatures,
	UtxoValidateCollateralVKeyWitnesses,
	UtxoValidateRedeemerAndScriptWitnesses,
	UtxoValidateCostModelsPresent,
	UtxoValidateOutsideValidityIntervalUtxo,
	UtxoValidateInputSetEmptyUtxo,
	UtxoValidateFeeTooSmallUtxo,
	UtxoValidateInsufficientCollateral,
	UtxoValidateCollateralContainsNonAda,
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
	UtxoValidateNativeScripts,
	UtxoValidateDelegation,
	UtxoValidateWithdrawals,
}

// UtxoValidateOutputTooBigUtxo ensures that transaction output values are not too large
func UtxoValidateOutputTooBigUtxo(
	tx common.Transaction,
	slot uint64,
	_ common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*AlonzoProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	badOutputs := []common.TransactionOutput{}
	for _, txOutput := range tx.Outputs() {
		tmpOutput, ok := txOutput.(*AlonzoTransactionOutput)
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

// UtxoValidateExUnitsTooBigUtxo ensures that ExUnits for a transaction do not exceed the maximum specified via protocol parameters
func UtxoValidateExUnitsTooBigUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*AlonzoProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*AlonzoTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	var totalSteps, totalMemory int64
	for _, redeemer := range tmpTx.WitnessSet.WsRedeemers {
		totalSteps += redeemer.ExUnits.Steps
		totalMemory += redeemer.ExUnits.Memory
	}
	if totalSteps <= tmpPparams.MaxTxExUnits.Steps &&
		totalMemory <= tmpPparams.MaxTxExUnits.Memory {
		return nil
	}
	return ExUnitsTooBigUtxoError{
		TotalExUnits: common.ExUnits{
			Memory: totalMemory,
			Steps:  totalSteps,
		},
		MaxTxExUnits: tmpPparams.MaxTxExUnits,
	}
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
	tmpPparams, ok := pp.(*AlonzoProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*AlonzoTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}

	required := map[uint]struct{}{}
	wits := tmpTx.WitnessSet
	if len(wits.WsPlutusV1Scripts) > 0 {
		required[0] = struct{}{}
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

func UtxoValidateInputSetEmptyUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateInputSetEmptyUtxo(tx, slot, ls, pp)
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

// UtxoValidateInsufficientCollateral ensures that there is sufficient collateral provided
func UtxoValidateInsufficientCollateral(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*AlonzoProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*AlonzoTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	// There's nothing to check if there are no redeemers
	if len(tmpTx.WitnessSet.WsRedeemers) == 0 {
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
	minCollateral := new(big.Int).Mul(fee, new(big.Int).SetUint64(uint64(tmpPparams.CollateralPercentage)))
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
	return InsufficientCollateralError{
		Provided: providedU,
		Required: requiredU,
	}
}

// UtxoValidateCollateralContainsNonAda ensures that collateral inputs don't contain non-ADA
func UtxoValidateCollateralContainsNonAda(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpTx, ok := tx.(*AlonzoTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	// There's nothing to check if there are no redeemers
	if len(tmpTx.WitnessSet.WsRedeemers) == 0 {
		return nil
	}
	badOutputs := []common.TransactionOutput{}
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
		if utxo.Output.Assets() == nil {
			continue
		}
		badOutputs = append(badOutputs, utxo.Output)
	}
	if len(badOutputs) == 0 {
		return nil
	}
	var providedU uint64
	if totalCollateral.IsUint64() {
		providedU = totalCollateral.Uint64()
	}
	return CollateralContainsNonAdaError{
		Provided: providedU,
	}
}

// UtxoValidateNoCollateralInputs ensures that collateral inputs are provided when redeemers are present
func UtxoValidateNoCollateralInputs(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpTx, ok := tx.(*AlonzoTransaction)
	if !ok {
		return errors.New("transaction is not expected type")
	}
	// There's nothing to check if there are no redeemers
	if len(tmpTx.WitnessSet.WsRedeemers) == 0 {
		return nil
	}
	if len(tx.Collateral()) > 0 {
		return nil
	}
	return NoCollateralInputsError{}
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
	tmpPparams, ok := pp.(*AlonzoProtocolParameters)
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
	minCoin, err := MinCoinTxOut(tx, pp)
	if err != nil {
		return err
	}
	minCoinBig := new(big.Int).SetUint64(minCoin)
	var badOutputs []common.TransactionOutput
	for _, tmpOutput := range tx.Outputs() {
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
	tmpPparams, ok := pp.(*AlonzoProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	txBytes := tx.Cbor()
	if len(txBytes) == 0 {
		var err error
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

// MinFeeTx calculates the minimum required fee for a transaction based on protocol parameters
// Fee is calculated using the transaction body CBOR size as per Cardano protocol
func MinFeeTx(
	tx common.Transaction,
	pparams common.ProtocolParameters,
) (uint64, error) {
	tmpPparams, ok := pparams.(*AlonzoProtocolParameters)
	if !ok {
		return 0, errors.New("pparams are not expected type")
	}
	tmpTx, ok := tx.(*AlonzoTransaction)
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

// MinCoinTxOut calculates the minimum coin for a transaction output based on protocol parameters
func MinCoinTxOut(
	_ common.Transaction,
	pparams common.ProtocolParameters,
) (uint64, error) {
	tmpPparams, ok := pparams.(*AlonzoProtocolParameters)
	if !ok {
		return 0, errors.New("pparams are not expected type")
	}
	minCoinTxOut := uint64(tmpPparams.MinUtxoValue)
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
	return mary.UtxoValidateNativeScripts(tx, slot, ls, pp)
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
