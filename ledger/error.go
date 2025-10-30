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

package ledger

import (
	"errors"
	"fmt"
	"strings"

	"github.com/blinklabs-io/gouroboros/cbor"
)

const (
	ApplyTxErrorUtxowFailure = 0

	UTXOWFailureUtxoFailure = 2

	UtxoFailureFromAlonzo = 1

	UtxoFailureBadInputsUtxo               = 0
	UtxoFailureOutsideValidityIntervalUtxo = 1
	UtxoFailureMaxTxSizeUtxo               = 2
	UtxoFailureInputSetEmpty               = 3
	UtxoFailureFeeTooSmallUtxo             = 4
	UtxoFailureValueNotConservedUtxo       = 5
	UtxoFailureOutputTooSmallUtxo          = 6
	UtxoFailureUtxosFailure                = 7
	UtxoFailureWrongNetwork                = 8
	UtxoFailureWrongNetworkWithdrawal      = 9
	UtxoFailureOutputBootAddrAttrsTooBig   = 10
	UtxoFailureTriesToForgeAda             = 11
	UtxoFailureOutputTooBigUtxo            = 12
	UtxoFailureInsufficientCollateral      = 13
	UtxoFailureScriptsNotPaidUtxo          = 14
	UtxoFailureExUnitsTooBigUtxo           = 15
	UtxoFailureCollateralContainsNonAda    = 16
	UtxoFailureWrongNetworkInTxBody        = 17
	UtxoFailureOutsideForecast             = 18
	UtxoFailureTooManyCollateralInputs     = 19
	UtxoFailureNoCollateralInputs          = 20
)

// Helper type to make the code a little cleaner
type NewErrorFromCborFunc func([]byte) (error, error)

func NewGenericErrorFromCbor(cborData []byte) (error, error) {
	newErr := &GenericError{}
	if _, err := cbor.Decode(cborData, newErr); err != nil {
		return nil, err
	}
	return newErr, nil
}

type GenericError struct {
	Value any
	Cbor  []byte
}

func (e *GenericError) UnmarshalCBOR(data []byte) error {
	var tmpValue cbor.Value
	if _, err := cbor.Decode(data, &tmpValue); err != nil {
		return err
	}
	e.Value = tmpValue.Value()
	e.Cbor = data
	return nil
}

func (e *GenericError) Error() string {
	return fmt.Sprintf("GenericError (%v)", e.Value)
}

func NewEraMismatchErrorFromCbor(cborData []byte) (error, error) {
	newErr := &EraMismatch{}
	if _, err := cbor.Decode(cborData, newErr); err != nil {
		return nil, err
	}
	return newErr, nil
}

type EraMismatch struct {
	cbor.StructAsArray
	LedgerEra uint8
	OtherEra  uint8
}

func (e *EraMismatch) Error() string {
	return fmt.Sprintf(
		"The era of the node and the tx do not match. The node is running in the %s era, but the transaction is for the %s era.",
		GetEraById(e.LedgerEra).Name,
		GetEraById(e.OtherEra).Name,
	)
}

// Helper function to try to parse CBOR as various error types
func NewTxSubmitErrorFromCbor(cborData []byte) (error, error) {
	for _, newErrFunc := range []NewErrorFromCborFunc{
		NewEraMismatchErrorFromCbor,
		NewShelleyTxValidationErrorFromCbor,
		// This should always be last in the list as a fallback
		NewGenericErrorFromCbor,
	} {
		newErr, err := newErrFunc(cborData)
		if err == nil {
			return newErr, nil
		}
	}
	return nil, errors.New("failed to parse error as any known types")
}

func NewShelleyTxValidationErrorFromCbor(cborData []byte) (error, error) {
	newErr := &ShelleyTxValidationError{}
	if _, err := cbor.Decode(cborData, newErr); err != nil {
		return nil, err
	}
	return newErr, nil
}

type ShelleyTxValidationError struct {
	Era uint8
	Err ApplyTxError
}

func (e *ShelleyTxValidationError) UnmarshalCBOR(data []byte) error {
	var tmpData struct {
		cbor.StructAsArray
		Inner struct {
			cbor.StructAsArray
			Era          uint8
			ApplyTxError ApplyTxError
		}
	}
	if _, err := cbor.Decode(data, &tmpData); err != nil {
		return err
	}
	e.Era = tmpData.Inner.Era
	e.Err = tmpData.Inner.ApplyTxError
	return nil
}

func (e *ShelleyTxValidationError) Error() string {
	return fmt.Sprintf(
		"ShelleyTxValidationError ShelleyBasedEra%s (%s)",
		GetEraById(e.Era).Name,
		e.Err.Error(),
	)
}

type ApplyTxError struct {
	cbor.StructAsArray
	Failures []error
}

func (e *ApplyTxError) UnmarshalCBOR(data []byte) error {
	var tmpData []cbor.RawMessage
	if _, err := cbor.Decode(data, &tmpData); err != nil {
		return err
	}
	for _, failure := range tmpData {
		tmpFailure := []cbor.RawMessage{}
		if _, err := cbor.Decode(failure, &tmpFailure); err != nil {
			return err
		}
		failureType, err := cbor.DecodeIdFromList(failure)
		if err != nil {
			return err
		}
		var newErr error
		switch failureType {
		case ApplyTxErrorUtxowFailure:
			newErr = &UtxowFailure{}
		default:
			if tmpErr, err := NewGenericErrorFromCbor(data); err != nil {
				return err
			} else {
				newErr = tmpErr
			}
		}
		if _, err := cbor.Decode(tmpFailure[1], newErr); err != nil {
			return err
		}
		e.Failures = append(e.Failures, newErr)
	}
	return nil
}

func (e *ApplyTxError) Error() string {
	ret := "ApplyTxError (["
	var retSb200 strings.Builder
	for idx, failure := range e.Failures {
		ret = fmt.Sprintf("%s%s", ret, failure)
		if idx < (len(e.Failures) - 1) {
			retSb200.WriteString(", ")
		}
	}
	ret += retSb200.String()
	ret = ret + "])"
	return ret
}

type UtxowFailure struct {
	cbor.StructAsArray
	Err error
}

func (e *UtxowFailure) UnmarshalCBOR(data []byte) error {
	tmpFailure := []cbor.RawMessage{}
	if _, err := cbor.Decode(data, &tmpFailure); err != nil {
		return err
	}
	failureType, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	var newErr error
	switch failureType {
	case UTXOWFailureUtxoFailure:
		newErr = &UtxoFailure{}
	default:
		if tmpErr, err := NewGenericErrorFromCbor(data); err != nil {
			return err
		} else {
			newErr = tmpErr
		}
	}
	if _, err := cbor.Decode(tmpFailure[1], newErr); err != nil {
		return err
	}
	e.Err = newErr
	return nil
}

func (e *UtxowFailure) Error() string {
	return fmt.Sprintf("UtxowFailure (%s)", e.Err)
}

type UtxoFailure struct {
	cbor.StructAsArray
	Era uint8
	Err error
}

func (e *UtxoFailure) UnmarshalCBOR(data []byte) error {
	var tmpData struct {
		cbor.StructAsArray
		Era uint8
		Err cbor.RawMessage
	}
	if _, err := cbor.Decode(data, &tmpData); err != nil {
		return err
	}
	e.Era = tmpData.Era
	newErr, err := cbor.DecodeById(
		tmpData.Err,
		map[int]any{
			UtxoFailureBadInputsUtxo:               &BadInputsUtxo{},
			UtxoFailureOutsideValidityIntervalUtxo: &OutsideValidityIntervalUtxo{},
			UtxoFailureMaxTxSizeUtxo:               &MaxTxSizeUtxo{},
			UtxoFailureInputSetEmpty:               &InputSetEmptyUtxo{},
			UtxoFailureFeeTooSmallUtxo:             &FeeTooSmallUtxo{},
			UtxoFailureValueNotConservedUtxo:       &ValueNotConservedUtxo{},
			UtxoFailureOutputTooSmallUtxo:          &OutputTooSmallUtxo{},
			UtxoFailureUtxosFailure:                &UtxosFailure{},
			UtxoFailureWrongNetwork:                &WrongNetwork{},
			UtxoFailureWrongNetworkWithdrawal:      &WrongNetworkWithdrawal{},
			UtxoFailureOutputBootAddrAttrsTooBig:   &OutputBootAddrAttrsTooBig{},
			UtxoFailureTriesToForgeAda:             &TriesToForgeADA{},
			UtxoFailureOutputTooBigUtxo:            &OutputTooBigUtxo{},
			UtxoFailureInsufficientCollateral:      &InsufficientCollateral{},
			UtxoFailureScriptsNotPaidUtxo:          &ScriptsNotPaidUtxo{},
			UtxoFailureExUnitsTooBigUtxo:           &ExUnitsTooBigUtxo{},
			UtxoFailureCollateralContainsNonAda:    &CollateralContainsNonADA{},
			UtxoFailureWrongNetworkInTxBody:        &WrongNetworkInTxBody{},
			UtxoFailureOutsideForecast:             &OutsideForecast{},
			UtxoFailureTooManyCollateralInputs:     &TooManyCollateralInputs{},
			UtxoFailureNoCollateralInputs:          &NoCollateralInputs{},
		},
	)
	if err != nil {
		newErr, err = NewGenericErrorFromCbor(tmpData.Err)
		if err != nil {
			return fmt.Errorf("failed to parse UtxoFailure: %w", err)
		}
	}
	e.Err = newErr.(error)
	return nil
}

func (e *UtxoFailure) Error() string {
	// TODO: lookup era name programmatically (#846)
	return fmt.Sprintf("UtxoFailure (FromAlonzoUtxoFail (%s))", e.Err)
}

type UtxoFailureErrorBase struct {
	cbor.StructAsArray
	Type uint8
}

type BadInputsUtxo struct {
	UtxoFailureErrorBase
	Inputs []TxIn
}

func (e *BadInputsUtxo) Error() string {
	ret := "BadInputsUtxo (["
	var retSb315 strings.Builder
	for idx, input := range e.Inputs {
		ret = fmt.Sprintf("%s%s", ret, input.String())
		if idx < (len(e.Inputs) - 1) {
			retSb315.WriteString(", ")
		}
	}
	ret += retSb315.String()
	ret = ret + "])"
	return ret
}

type TxIn struct {
	cbor.StructAsArray
	Utxo cbor.ByteString
	TxIx uint8
}

func (e *TxIn) String() string {
	return fmt.Sprintf("TxIn (Utxo %s, TxIx %d)", e.Utxo, e.TxIx)
}

type OutsideValidityIntervalUtxo struct {
	UtxoFailureErrorBase
	ValidityInterval cbor.Value
	Slot             uint32
}

func (e *OutsideValidityIntervalUtxo) Error() string {
	validityInterval := e.ValidityInterval.Value().([]any)
	return fmt.Sprintf(
		"OutsideValidityIntervalUtxo (ValidityInterval { invalidBefore = %v, invalidHereafter = %v }, Slot %d)",
		validityInterval[0],
		validityInterval[1],
		e.Slot,
	)
}

type MaxTxSizeUtxo struct {
	UtxoFailureErrorBase
	ActualSize int
	MaxSize    int
}

func (e *MaxTxSizeUtxo) Error() string {
	return fmt.Sprintf(
		"MaxTxSizeUtxo (ActualSize %d, MaxSize %d)",
		e.ActualSize,
		e.MaxSize,
	)
}

type InputSetEmptyUtxo struct {
	UtxoFailureErrorBase
}

func (e *InputSetEmptyUtxo) Error() string {
	return "InputSetEmptyUtxo"
}

type FeeTooSmallUtxo struct {
	UtxoFailureErrorBase
	MinimumFee  uint64
	SuppliedFee uint64
}

func (e *FeeTooSmallUtxo) Error() string {
	return fmt.Sprintf(
		"FeeTooSmallUtxo (MinimumFee %d, SuppliedFee %d)",
		e.MinimumFee,
		e.SuppliedFee,
	)
}

type ValueNotConservedUtxo struct {
	UtxoFailureErrorBase
	Consumed uint64
	Produced uint64
}

func (e *ValueNotConservedUtxo) Error() string {
	return fmt.Sprintf(
		"ValueNotConservedUtxo (Consumed %d, Produced %d)",
		e.Consumed,
		e.Produced,
	)
}

type OutputTooSmallUtxo struct {
	UtxoFailureErrorBase
	Outputs []TxOut
}

func (e *OutputTooSmallUtxo) Error() string {
	ret := "OutputTooSmallUtxo (["
	var retSb408 strings.Builder
	for idx, output := range e.Outputs {
		ret = fmt.Sprintf("%s%s", ret, output.String())
		if idx < (len(e.Outputs) - 1) {
			retSb408.WriteString(", ")
		}
	}
	ret += retSb408.String()
	ret = ret + "])"
	return ret
}

type TxOut struct {
	cbor.Value
}

func (t *TxOut) String() string {
	return fmt.Sprintf("TxOut (%v)", t.Value.Value())
}

type UtxosFailure struct {
	UtxoFailureErrorBase
	Err GenericError
}

func (e *UtxosFailure) Error() string {
	return fmt.Sprintf("UtxosFailure (%s)", e.Err)
}

type WrongNetwork struct {
	UtxoFailureErrorBase
	ExpectedNetworkId int
	Addresses         cbor.Value
}

func (e *WrongNetwork) Error() string {
	return fmt.Sprintf(
		"WrongNetwork (ExpectedNetworkId %d, Addresses (%v))",
		e.ExpectedNetworkId,
		e.Addresses.Value(),
	)
}

type WrongNetworkWithdrawal struct {
	UtxoFailureErrorBase
	ExpectedNetworkId int
	RewardAccounts    cbor.Value
}

func (e *WrongNetworkWithdrawal) Error() string {
	return fmt.Sprintf(
		"WrongNetworkWithdrawal (ExpectedNetworkId %d, RewardAccounts (%v))",
		e.ExpectedNetworkId,
		e.RewardAccounts.Value(),
	)
}

type OutputBootAddrAttrsTooBig struct {
	UtxoFailureErrorBase
	Outputs []TxOut
}

func (e *OutputBootAddrAttrsTooBig) Error() string {
	ret := "OutputBootAddrAttrsTooBig (["
	var retSb470 strings.Builder
	for idx, output := range e.Outputs {
		ret = fmt.Sprintf("%s%s", ret, output.String())
		if idx < (len(e.Outputs) - 1) {
			retSb470.WriteString(", ")
		}
	}
	ret += retSb470.String()
	ret = ret + "])"
	return ret
}

type TriesToForgeADA struct {
	UtxoFailureErrorBase
}

func (e *TriesToForgeADA) Error() string {
	return "TriesToForgeADA"
}

type OutputTooBigUtxo struct {
	UtxoFailureErrorBase
	Outputs []struct {
		ActualSize int
		MaxSize    int
		Output     TxOut
	}
}

func (e *OutputTooBigUtxo) Error() string {
	ret := "OutputTooBigUtxo (["
	var retSb499 strings.Builder
	for idx, output := range e.Outputs {
		ret = fmt.Sprintf(
			"%s(ActualSize %d, MaxSize %d, Output (%s))",
			ret,
			output.ActualSize,
			output.MaxSize,
			output.Output.String(),
		)
		if idx < (len(e.Outputs) - 1) {
			retSb499.WriteString(", ")
		}
	}
	ret += retSb499.String()
	ret = ret + "])"
	return ret
}

type InsufficientCollateral struct {
	UtxoFailureErrorBase
	BalanceComputed    uint64
	RequiredCollateral uint64
}

func (e *InsufficientCollateral) Error() string {
	return fmt.Sprintf(
		"InsufficientCollateral (BalanceComputed %d, RequiredCollateral %d)",
		e.BalanceComputed,
		e.RequiredCollateral,
	)
}

type ScriptsNotPaidUtxo struct {
	UtxoFailureErrorBase
	// TODO: determine content/structure of this value (#847)
	Value cbor.Value
}

func (e *ScriptsNotPaidUtxo) Error() string {
	return fmt.Sprintf("ScriptsNotPaidUtxo (%v)", e.Value.Value())
}

type ExUnitsTooBigUtxo struct {
	UtxoFailureErrorBase
	MaxAllowed int
	Supplied   int
}

func (e *ExUnitsTooBigUtxo) Error() string {
	return fmt.Sprintf(
		"ExUnitsTooBigUtxo (MaxAllowed %d, Supplied %d)",
		e.MaxAllowed,
		e.Supplied,
	)
}

type CollateralContainsNonADA struct {
	UtxoFailureErrorBase
	// TODO: determine content/structure of this value (#848)
	Value cbor.Value
}

func (e *CollateralContainsNonADA) Error() string {
	return fmt.Sprintf("CollateralContainsNonADA (%v)", e.Value.Value())
}

type WrongNetworkInTxBody struct {
	UtxoFailureErrorBase
	ActualNetworkId      int
	TransactionNetworkId int
}

func (e *WrongNetworkInTxBody) Error() string {
	return fmt.Sprintf(
		"WrongNetworkInTxBody (ActualNetworkId %d, TransactionNetworkId %d)",
		e.ActualNetworkId,
		e.TransactionNetworkId,
	)
}

type OutsideForecast struct {
	UtxoFailureErrorBase
	Slot uint32
}

func (e *OutsideForecast) Error() string {
	return fmt.Sprintf("OutsideForecast (Slot %d)", e.Slot)
}

type TooManyCollateralInputs struct {
	UtxoFailureErrorBase
	MaxAllowed int
	Supplied   int
}

func (e *TooManyCollateralInputs) Error() string {
	return fmt.Sprintf(
		"TooManyCollateralInputs (MaxAllowed %d, Supplied %d)",
		e.MaxAllowed,
		e.Supplied,
	)
}

type NoCollateralInputs struct {
	UtxoFailureErrorBase
}

func (e *NoCollateralInputs) Error() string {
	return "NoMollateralInputs"
}
