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
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

var UtxoValidationRules = []common.UtxoValidationRuleFunc{
	UtxoValidateOutsideValidityIntervalUtxo,
	// validateOutsideForecast
	UtxoValidateInputSetEmptyUtxo,
	UtxoValidateFeeTooSmallUtxo,
	UtxoValidateInsufficientCollateral,
	/*
	   [ -- Part 3: (∀(a,_,_) ∈ range (collateral txb ◁ utxo), a ∈ Addrvkey)

	   	validateScriptsNotPaidUTxO utxoCollateral

	   , -- Part 4: balance ∗ 100 ≥ txfee txb ∗ (collateralPercent pp)

	   	validateInsufficientCollateral pp txb bal

	   , -- Part 5: isAdaOnly balance

	   	validateCollateralContainsNonADA utxoCollateral

	   , -- Part 6: (∀(a,_,_) ∈ range (collateral txb ◁ utxo), a ∈ Addrvkey)

	   	failureIf (null utxoCollateral) NoCollateralInputs

	   ]
	*/
	UtxoValidateBadInputsUtxo,
	UtxoValidateValueNotConservedUtxo,
	UtxoValidateOutputTooSmallUtxo,
	UtxoValidateOutputTooBigUtxo,
	UtxoValidateOutputBootAddrAttrsTooBig,
	UtxoValidateWrongNetwork,
	UtxoValidateWrongNetworkWithdrawal,
	UtxoValidateMaxTxSizeUtxo,
	UtxoValidateExUnitsTooBigUtxo,
}

// UtxoValidateOutputTooBigUtxo ensures that transaction output values are not too large
func UtxoValidateOutputTooBigUtxo(tx common.Transaction, slot uint64, _ common.LedgerState, pp common.ProtocolParameters) error {
	tmpPparams, ok := pp.(*AlonzoProtocolParameters)
	if !ok {
		return fmt.Errorf("pparams are not expected type")
	}
	var badOutputs []common.TransactionOutput
	for _, txOutput := range tx.Outputs() {
		tmpOutput, ok := txOutput.(*AlonzoTransactionOutput)
		if !ok {
			return fmt.Errorf("transaction output is not expected type")
		}
		outputValBytes, err := cbor.Encode(tmpOutput.OutputAmount)
		if err != nil {
			return err
		}
		if len(outputValBytes) <= int(tmpPparams.MaxValueSize) {
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
func UtxoValidateExUnitsTooBigUtxo(tx common.Transaction, slot uint64, ls common.LedgerState, pp common.ProtocolParameters) error {
	tmpPparams, ok := pp.(*AlonzoProtocolParameters)
	if !ok {
		return fmt.Errorf("pparams are not expected type")
	}
	tmpTx, ok := tx.(*AlonzoTransaction)
	if !ok {
		return fmt.Errorf("transaction is not expected type")
	}
	var totalSteps, totalMemory uint64
	for _, redeemer := range tmpTx.WitnessSet.WsRedeemers {
		totalSteps += redeemer.ExUnits.Steps
		totalMemory += redeemer.ExUnits.Memory
	}
	if totalSteps <= tmpPparams.MaxTxExUnits.Steps && totalMemory <= tmpPparams.MaxTxExUnits.Memory {
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

func UtxoValidateOutsideValidityIntervalUtxo(tx common.Transaction, slot uint64, ls common.LedgerState, pp common.ProtocolParameters) error {
	return allegra.UtxoValidateOutsideValidityIntervalUtxo(tx, slot, ls, pp)
}

func UtxoValidateInputSetEmptyUtxo(tx common.Transaction, slot uint64, ls common.LedgerState, pp common.ProtocolParameters) error {
	return shelley.UtxoValidateInputSetEmptyUtxo(tx, slot, ls, pp)
}

func UtxoValidateFeeTooSmallUtxo(tx common.Transaction, slot uint64, ls common.LedgerState, pp common.ProtocolParameters) error {
	tmpPparams, ok := pp.(*AlonzoProtocolParameters)
	if !ok {
		return fmt.Errorf("pparams are not expected type")
	}
	return shelley.UtxoValidateFeeTooSmallUtxo(tx, slot, ls, &tmpPparams.ShelleyProtocolParameters)
}

func UtxoValidateInsufficientCollateral(tx common.Transaction, slot uint64, ls common.LedgerState, pp common.ProtocolParameters) error {
	tmpPparams, ok := pp.(*AlonzoProtocolParameters)
	if !ok {
		return fmt.Errorf("pparams are not expected type")
	}
	var totalCollateral uint64
	for _, collateralInput := range tx.Collateral() {
		utxo, err := ls.UtxoById(collateralInput)
		if err != nil {
			return err
		}
		totalCollateral += utxo.Output.Amount()
	}
	minFee, err := shelley.MinFeeTx(tx, &tmpPparams.ShelleyProtocolParameters)
	if err != nil {
		return err
	}
	minCollateral := minFee * uint64(tmpPparams.CollateralPercentage) / 100
	if totalCollateral >= minCollateral {
		return nil
	}
	return InsufficientCollateralError{
		Provided: totalCollateral,
		Required: minCollateral,
	}
}

func UtxoValidateBadInputsUtxo(tx common.Transaction, slot uint64, ls common.LedgerState, pp common.ProtocolParameters) error {
	return shelley.UtxoValidateBadInputsUtxo(tx, slot, ls, pp)
}

func UtxoValidateValueNotConservedUtxo(tx common.Transaction, slot uint64, ls common.LedgerState, pp common.ProtocolParameters) error {
	tmpPparams, ok := pp.(*AlonzoProtocolParameters)
	if !ok {
		return fmt.Errorf("pparams are not expected type")
	}
	return shelley.UtxoValidateValueNotConservedUtxo(tx, slot, ls, &tmpPparams.ShelleyProtocolParameters)
}

func UtxoValidateOutputTooSmallUtxo(tx common.Transaction, slot uint64, ls common.LedgerState, pp common.ProtocolParameters) error {
	tmpPparams, ok := pp.(*AlonzoProtocolParameters)
	if !ok {
		return fmt.Errorf("pparams are not expected type")
	}
	return shelley.UtxoValidateOutputTooSmallUtxo(tx, slot, ls, &tmpPparams.ShelleyProtocolParameters)
}

func UtxoValidateOutputBootAddrAttrsTooBig(tx common.Transaction, slot uint64, ls common.LedgerState, pp common.ProtocolParameters) error {
	return shelley.UtxoValidateOutputBootAddrAttrsTooBig(tx, slot, ls, pp)
}

func UtxoValidateWrongNetwork(tx common.Transaction, slot uint64, ls common.LedgerState, pp common.ProtocolParameters) error {
	return shelley.UtxoValidateWrongNetwork(tx, slot, ls, pp)
}

func UtxoValidateWrongNetworkWithdrawal(tx common.Transaction, slot uint64, ls common.LedgerState, pp common.ProtocolParameters) error {
	return shelley.UtxoValidateWrongNetworkWithdrawal(tx, slot, ls, pp)
}

func UtxoValidateMaxTxSizeUtxo(tx common.Transaction, slot uint64, ls common.LedgerState, pp common.ProtocolParameters) error {
	tmpPparams, ok := pp.(*AlonzoProtocolParameters)
	if !ok {
		return fmt.Errorf("pparams are not expected type")
	}
	return shelley.UtxoValidateMaxTxSizeUtxo(tx, slot, ls, &tmpPparams.ShelleyProtocolParameters)
}
