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

package allegra

import (
	"errors"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

var UtxoValidationRules = []common.UtxoValidationRuleFunc{
	UtxoValidateOutsideValidityIntervalUtxo,
	UtxoValidateInputSetEmptyUtxo,
	UtxoValidateFeeTooSmallUtxo,
	UtxoValidateBadInputsUtxo,
	UtxoValidateWrongNetwork,
	UtxoValidateWrongNetworkWithdrawal,
	UtxoValidateValueNotConservedUtxo,
	UtxoValidateOutputTooSmallUtxo,
	UtxoValidateOutputBootAddrAttrsTooBig,
	UtxoValidateMaxTxSizeUtxo,
}

// UtxoValidateOutsideValidityIntervalUtxo ensures that the current tip slot has reached the specified validity interval
func UtxoValidateOutsideValidityIntervalUtxo(
	tx common.Transaction,
	slot uint64,
	_ common.LedgerState,
	_ common.ProtocolParameters,
) error {
	validityIntervalStart := tx.ValidityIntervalStart()
	if validityIntervalStart == 0 || slot >= validityIntervalStart {
		return nil
	}
	return OutsideValidityIntervalUtxoError{
		ValidityIntervalStart: validityIntervalStart,
		Slot:                  slot,
	}
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
	tmpPparams, ok := pp.(*AllegraProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	return shelley.UtxoValidateFeeTooSmallUtxo(
		tx,
		slot,
		ls,
		&tmpPparams.ShelleyProtocolParameters,
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
		&tmpPparams.ShelleyProtocolParameters,
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
		&tmpPparams.ShelleyProtocolParameters,
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
		&tmpPparams.ShelleyProtocolParameters,
	)
}
