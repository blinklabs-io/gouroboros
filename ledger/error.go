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
	"strconv"
	"strings"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
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
	UtxoFailureInsufficientCollateral      = 12
	UtxoFailureWrongNetworkInTxBody        = 17
	UtxoFailureOutsideForecast             = 18
	UtxoFailureTooManyCollateralInputs     = 19
	UtxoFailureNoCollateralInputs          = 20
)

// Era-specific constants for errors that differ between Cardano eras
const (
	// Alonzo era error constants
	UtxoFailureOutputTooBigUtxoAlonzo         = 12
	UtxoFailureScriptsNotPaidUtxoAlonzo       = 14
	UtxoFailureExUnitsTooBigUtxoAlonzo        = 15
	UtxoFailureCollateralContainsNonAdaAlonzo = 16

	// Babbage era error constants (same as Alonzo for these errors)
	UtxoFailureOutputTooBigUtxoBabbage         = 12
	UtxoFailureScriptsNotPaidUtxoBabbage       = 14
	UtxoFailureExUnitsTooBigUtxoBabbage        = 15
	UtxoFailureCollateralContainsNonAdaBabbage = 16

	// Conway era error constants
	UtxoFailureOutputTooBigUtxoConway         = 11
	UtxoFailureScriptsNotPaidUtxoConway       = 13
	UtxoFailureExUnitsTooBigUtxoConway        = 14
	UtxoFailureCollateralContainsNonAdaConway = 15
)

// Helper type to make the code a little cleaner
type NewErrorFromCborFunc func([]byte) (error, error)

// getEraSpecificUtxoFailureConstants returns the correct error constants for the given era
func getEraSpecificUtxoFailureConstants(
	eraId uint8,
) (map[int]any, int, int, int, int) {
	baseMap := map[int]any{
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
		UtxoFailureInsufficientCollateral:      &InsufficientCollateral{},
		UtxoFailureWrongNetworkInTxBody:        &WrongNetworkInTxBody{},
		UtxoFailureOutsideForecast:             &OutsideForecast{},
		UtxoFailureTooManyCollateralInputs:     &TooManyCollateralInputs{},
		UtxoFailureNoCollateralInputs:          &NoCollateralInputs{},
	}

	switch eraId {
	case EraIdAlonzo:
		baseMap[UtxoFailureOutputTooBigUtxoAlonzo] = &OutputTooBigUtxo{}
		baseMap[UtxoFailureScriptsNotPaidUtxoAlonzo] = &ScriptsNotPaidUtxo{}
		baseMap[UtxoFailureExUnitsTooBigUtxoAlonzo] = &ExUnitsTooBigUtxo{}
		baseMap[UtxoFailureCollateralContainsNonAdaAlonzo] = &CollateralContainsNonADA{}
		return baseMap, UtxoFailureOutputTooBigUtxoAlonzo, UtxoFailureScriptsNotPaidUtxoAlonzo, UtxoFailureExUnitsTooBigUtxoAlonzo, UtxoFailureCollateralContainsNonAdaAlonzo
	case EraIdBabbage:
		baseMap[UtxoFailureOutputTooBigUtxoBabbage] = &OutputTooBigUtxo{}
		baseMap[UtxoFailureScriptsNotPaidUtxoBabbage] = &ScriptsNotPaidUtxo{}
		baseMap[UtxoFailureExUnitsTooBigUtxoBabbage] = &ExUnitsTooBigUtxo{}
		baseMap[UtxoFailureCollateralContainsNonAdaBabbage] = &CollateralContainsNonADA{}
		return baseMap, UtxoFailureOutputTooBigUtxoBabbage, UtxoFailureScriptsNotPaidUtxoBabbage, UtxoFailureExUnitsTooBigUtxoBabbage, UtxoFailureCollateralContainsNonAdaBabbage
	case EraIdConway:
		baseMap[UtxoFailureOutputTooBigUtxoConway] = &OutputTooBigUtxo{}
		baseMap[UtxoFailureScriptsNotPaidUtxoConway] = &ScriptsNotPaidUtxo{}
		baseMap[UtxoFailureExUnitsTooBigUtxoConway] = &ExUnitsTooBigUtxo{}
		baseMap[UtxoFailureCollateralContainsNonAdaConway] = &CollateralContainsNonADA{}
		return baseMap, UtxoFailureOutputTooBigUtxoConway, UtxoFailureScriptsNotPaidUtxoConway, UtxoFailureExUnitsTooBigUtxoConway, UtxoFailureCollateralContainsNonAdaConway
	default:
		// For other eras (Byron, Shelley, Allegra, Mary), use Conway constants as fallback
		baseMap[UtxoFailureOutputTooBigUtxoConway] = &OutputTooBigUtxo{}
		baseMap[UtxoFailureScriptsNotPaidUtxoConway] = &ScriptsNotPaidUtxo{}
		baseMap[UtxoFailureExUnitsTooBigUtxoConway] = &ExUnitsTooBigUtxo{}
		baseMap[UtxoFailureCollateralContainsNonAdaConway] = &CollateralContainsNonADA{}
		return baseMap, UtxoFailureOutputTooBigUtxoConway, UtxoFailureScriptsNotPaidUtxoConway, UtxoFailureExUnitsTooBigUtxoConway, UtxoFailureCollateralContainsNonAdaConway
	}
}

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
	var sb strings.Builder
	sb.WriteString("ApplyTxError ([")
	for idx, failure := range e.Failures {
		sb.WriteString(failure.Error())
		if idx < (len(e.Failures) - 1) {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
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
	errorMap, _, _, _, _ := getEraSpecificUtxoFailureConstants(tmpData.Era)
	newErr, err := cbor.DecodeById(tmpData.Err, errorMap)
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
	// Dynamically determine era name using the era ID from the struct
	eraName := GetEraById(e.Era).Name
	return fmt.Sprintf("UtxoFailure (From%sUtxoFail (%s))", eraName, e.Err)
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
	var sb strings.Builder
	sb.WriteString("BadInputsUtxo ([")
	for idx, input := range e.Inputs {
		sb.WriteString(input.String())
		if idx < (len(e.Inputs) - 1) {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
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
	var sb strings.Builder
	sb.WriteString("OutputTooSmallUtxo ([")
	for idx, output := range e.Outputs {
		sb.WriteString(output.String())
		if idx < (len(e.Outputs) - 1) {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
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
	var sb strings.Builder
	sb.WriteString("OutputBootAddrAttrsTooBig ([")
	for idx, output := range e.Outputs {
		sb.WriteString(output.String())
		if idx < (len(e.Outputs) - 1) {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
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
	var sb strings.Builder
	sb.WriteString("OutputTooBigUtxo ([")
	for idx, output := range e.Outputs {
		sb.WriteString("(ActualSize ")
		sb.WriteString(strconv.Itoa(output.ActualSize))
		sb.WriteString(", MaxSize ")
		sb.WriteString(strconv.Itoa(output.MaxSize))
		sb.WriteString(", Output (")
		sb.WriteString(output.Output.String())
		sb.WriteString("))")
		if idx < (len(e.Outputs) - 1) {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
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

// ScriptsNotPaidUtxo represents the ScriptsNotPaidUTxO error from cardano-ledger.
// Haskell: ScriptsNotPaidUTxO !(UTxO era) where UTxO era = Map TxIn TxOut
// CBOR: [14, utxo_map]
type ScriptsNotPaidUtxo struct {
	UtxoFailureErrorBase
	Utxos []common.Utxo // Each Utxo contains Id (input) and Output
}

func (e *ScriptsNotPaidUtxo) MarshalCBOR() ([]byte, error) {
	// Use era-specific constant - we'll use Conway as default since it has the most recent structure
	// In practice, this should be set when the error is created, but we provide a sensible fallback
	constantToUse := UtxoFailureScriptsNotPaidUtxoConway
	if e.Type != 0 {
		constantToUse = int(e.Type)
	}
	// Bounds check to prevent integer overflow
	if constantToUse < 0 || constantToUse > 255 {
		return nil, fmt.Errorf(
			"ScriptsNotPaidUtxo: invalid constructor index %d (must be 0-255)",
			constantToUse,
		)
	}
	e.Type = uint8(constantToUse)

	utxoMap := make(
		map[common.TransactionInput]common.TransactionOutput,
		len(e.Utxos),
	)
	for _, u := range e.Utxos {
		// Return error for nil entries instead of silently skipping
		if u.Id == nil || u.Output == nil {
			return nil, errors.New(
				"ScriptsNotPaidUtxo: cannot marshal UTxO with nil Id or Output",
			)
		}
		utxoMap[u.Id] = u.Output
	}
	arr := []any{constantToUse, utxoMap}
	return cbor.Encode(arr)
}

func (e *ScriptsNotPaidUtxo) UnmarshalCBOR(data []byte) error {
	type tScriptsNotPaidUtxo struct {
		cbor.StructAsArray
		ConstructorIdx uint64
		UtxoMapCbor    cbor.RawMessage
	}
	var tmp tScriptsNotPaidUtxo
	if _, err := cbor.Decode(data, &tmp); err != nil {
		return fmt.Errorf("failed to decode ScriptsNotPaidUtxo: %w", err)
	}

	// Check if the constructor index matches any valid era-specific constant
	validConstructors := []int{
		UtxoFailureScriptsNotPaidUtxoAlonzo,
		UtxoFailureScriptsNotPaidUtxoBabbage,
		UtxoFailureScriptsNotPaidUtxoConway,
	}

	isValid := false
	for _, valid := range validConstructors {
		//nolint:gosec // Constants are within valid range for uint64
		if tmp.ConstructorIdx == uint64(valid) {
			isValid = true
			break
		}
	}

	if !isValid {
		return fmt.Errorf(
			"ScriptsNotPaidUtxo: expected one of constructor indices %v, got %d",
			validConstructors,
			tmp.ConstructorIdx,
		)
	}

	// Set the struct tag to match the decoded constructor
	// Bounds check to prevent integer overflow
	if tmp.ConstructorIdx > 255 {
		return fmt.Errorf(
			"ScriptsNotPaidUtxo: constructor index %d exceeds uint8 range (0-255)",
			tmp.ConstructorIdx,
		)
	}
	e.Type = uint8(tmp.ConstructorIdx)

	// For era-agnostic decoding, we need to handle the map structure carefully
	// Since we can't use cbor.RawMessage as map keys, we'll decode to a concrete type first
	// and then convert to era-agnostic types. Try different era input types until one works.

	// Try Shelley-family transaction inputs first (most common from Shelley onwards)
	var shelleyUtxoMap map[shelley.ShelleyTransactionInput]cbor.RawMessage
	if _, err := cbor.Decode(tmp.UtxoMapCbor, &shelleyUtxoMap); err == nil {
		// Successfully decoded as Shelley-family inputs
		e.Utxos = make([]common.Utxo, 0, len(shelleyUtxoMap))
		for input, outputCbor := range shelleyUtxoMap {
			// Decode output using era-agnostic function (handles all eras)
			output, err := NewTransactionOutputFromCbor(outputCbor)
			if err != nil {
				return fmt.Errorf(
					"failed to decode transaction output: %w",
					err,
				)
			}

			e.Utxos = append(e.Utxos, common.Utxo{
				Id:     &input,
				Output: output,
			})
		}
		return nil
	}

	// Try Byron transaction inputs (for Byron era)
	var byronUtxoMap map[byron.ByronTransactionInput]cbor.RawMessage
	if _, err := cbor.Decode(tmp.UtxoMapCbor, &byronUtxoMap); err == nil {
		// Successfully decoded as Byron inputs
		e.Utxos = make([]common.Utxo, 0, len(byronUtxoMap))
		for input, outputCbor := range byronUtxoMap {
			// Decode output using era-agnostic function (handles all eras)
			output, err := NewTransactionOutputFromCbor(outputCbor)
			if err != nil {
				return fmt.Errorf(
					"failed to decode transaction output: %w",
					err,
				)
			}

			e.Utxos = append(e.Utxos, common.Utxo{
				Id:     &input,
				Output: output,
			})
		}
		return nil
	}

	// If both failed, return an error
	return errors.New(
		"failed to decode UTxO map as either Shelley-family or Byron transaction inputs",
	)
}

func (e *ScriptsNotPaidUtxo) Error() string {
	return fmt.Sprintf("ScriptsNotPaidUtxo (%d UTxOs)", len(e.Utxos))
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

// CollateralContainsNonADA represents the CollateralContainsNonADA error from cardano-ledger.
// CBOR: [16, provided] (Alonzo/Babbage), [15, provided] (Conway)
type CollateralContainsNonADA struct {
	UtxoFailureErrorBase
	Provided cbor.Value
}

func (e *CollateralContainsNonADA) MarshalCBOR() ([]byte, error) {
	// Use era-specific constant - fallback to Conway if not set
	constantToUse := UtxoFailureCollateralContainsNonAdaConway
	if e.Type != 0 {
		constantToUse = int(e.Type)
	}
	// Bounds check
	if constantToUse < 0 || constantToUse > 255 {
		return nil, fmt.Errorf(
			"CollateralContainsNonADA: invalid constructor index %d (must be 0-255)",
			constantToUse,
		)
	}
	e.Type = uint8(constantToUse)
	arr := []any{constantToUse, e.Provided.Value()}
	return cbor.Encode(arr)
}

func (e *CollateralContainsNonADA) UnmarshalCBOR(data []byte) error {
	type tCollateralContainsNonADA struct {
		cbor.StructAsArray
		ConstructorIdx uint64
		Provided       cbor.Value
	}
	var tmp tCollateralContainsNonADA
	if _, err := cbor.Decode(data, &tmp); err != nil {
		return fmt.Errorf("failed to decode CollateralContainsNonADA: %w", err)
	}

	// Check if the constructor index matches any valid era-specific constant
	validConstructors := []int{
		UtxoFailureCollateralContainsNonAdaAlonzo,
		UtxoFailureCollateralContainsNonAdaBabbage,
		UtxoFailureCollateralContainsNonAdaConway,
	}
	isValid := false
	for _, valid := range validConstructors {
		//nolint:gosec // G115: integer overflow conversion int -> uint64
		// Safe conversion since constants are small positive values (15, 16)
		if tmp.ConstructorIdx == uint64(valid) {
			isValid = true
			break
		}
	}
	if !isValid {
		return fmt.Errorf(
			"CollateralContainsNonADA: expected one of constructor indices %v, got %d",
			validConstructors,
			tmp.ConstructorIdx,
		)
	}
	if tmp.ConstructorIdx > uint64(255) {
		return fmt.Errorf(
			"CollateralContainsNonADA: constructor index %d exceeds uint8 range (0-255)",
			tmp.ConstructorIdx,
		)
	}
	e.Type = uint8(tmp.ConstructorIdx)
	e.Provided = tmp.Provided
	return nil
}

func (e *CollateralContainsNonADA) Error() string {
	return fmt.Sprintf(
		"CollateralContainsNonADA (Provided %v)",
		e.Provided.Value(),
	)
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
	return "NoCollateralInputs"
}
