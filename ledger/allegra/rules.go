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

package allegra

import (
	"errors"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

var UtxoValidationRules = []common.UtxoValidationRuleFunc{
	UtxoValidateMetadata,
	UtxoValidateRequiredVKeyWitnesses,
	UtxoValidateSignatures,
	UtxoValidateOutsideValidityIntervalUtxo,
	UtxoValidateInputSetEmptyUtxo,
	UtxoValidateNoDuplicateInputs,
	UtxoValidateFeeTooSmallUtxo,
	UtxoValidateBadInputsUtxo,
	UtxoValidateScriptWitnesses,
	UtxoValidateWrongNetwork,
	UtxoValidateWrongNetworkWithdrawal,
	UtxoValidateValueNotConservedUtxo,
	UtxoValidateOutputTooSmallUtxo,
	UtxoValidateOutputBootAddrAttrsTooBig,
	UtxoValidateMaxTxSizeUtxo,
	UtxoValidateNativeScripts,
	UtxoValidateDelegation,
	UtxoValidateWithdrawals,
	UtxoValidateMIRGenesisQuorum,
}

func UtxoValidateRequiredVKeyWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateRequiredVKeyWitnesses(tx, slot, ls, pp)
}

// UtxoValidateOutsideValidityIntervalUtxo ensures that the current tip slot is
// within the transaction's half-open validity interval.
func UtxoValidateOutsideValidityIntervalUtxo(
	tx common.Transaction,
	slot uint64,
	_ common.LedgerState,
	_ common.ProtocolParameters,
) error {
	validityIntervalStart := tx.ValidityIntervalStart()
	if validityIntervalStart != 0 && slot < validityIntervalStart {
		return OutsideValidityIntervalUtxoError{
			ValidityIntervalStart: validityIntervalStart,
			Slot:                  slot,
		}
	}
	validityIntervalEnd, validityIntervalEndPresent := common.TransactionValidityIntervalUpperBound(
		tx,
	)
	if validityIntervalEndPresent && slot >= validityIntervalEnd {
		return OutsideValidityIntervalUpperBoundUtxoError{
			End:  validityIntervalEnd,
			Slot: slot,
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

// UtxoValidateScriptWitnesses checks that every required script is provided.
func UtxoValidateScriptWitnesses(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateScriptWitnesses(tx, slot, ls, pp)
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
	tmpPparams, ok := pp.(*AllegraProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	return shelley.UtxoValidateFeeTooSmallUtxo(
		tx,
		slot,
		ls,
		tmpPparams,
	)
}

func UtxoValidateBadInputsUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateBadInputsUtxo(tx, slot, ls, pp)
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

func UtxoValidateValueNotConservedUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*AllegraProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	return shelley.UtxoValidateValueNotConservedUtxo(
		tx,
		slot,
		ls,
		tmpPparams,
	)
}

func UtxoValidateOutputTooSmallUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*AllegraProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	return shelley.UtxoValidateOutputTooSmallUtxo(
		tx,
		slot,
		ls,
		tmpPparams,
	)
}

func UtxoValidateOutputBootAddrAttrsTooBig(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateOutputBootAddrAttrsTooBig(tx, slot, ls, pp)
}

func UtxoValidateMaxTxSizeUtxo(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	tmpPparams, ok := pp.(*AllegraProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	return shelley.UtxoValidateMaxTxSizeUtxo(
		tx,
		slot,
		ls,
		tmpPparams,
	)
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

// UtxoValidateSignatures verifies vkey and bootstrap signatures present in the transaction.
func UtxoValidateSignatures(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	return shelley.UtxoValidateSignatures(tx, slot, ls, pp)
}

// UtxoValidateNativeScripts evaluates native scripts in the transaction.
func UtxoValidateNativeScripts(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	nativeScripts, err := common.NativeScriptsForValidation(tx, ls)
	if err != nil {
		return err
	}
	if scriptHash, failed := common.FirstInvalidNativeScriptIn(
		tx,
		slot,
		nativeScripts,
	); failed {
		return NativeScriptFailedError{ScriptHash: scriptHash}
	}
	return nil
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
