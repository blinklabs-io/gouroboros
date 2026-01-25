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

	// Shelley UTXOW failure tags (also used by Allegra and Mary)
	ShelleyUtxowInvalidWitnesses           = 0
	ShelleyUtxowMissingVKeyWitnesses       = 1
	ShelleyUtxowMissingScriptWitnesses     = 2
	ShelleyUtxowScriptWitnessNotValidating = 3
	ShelleyUtxowUtxoFailure                = 4
	ShelleyUtxowMissingTxBodyMetadataHash  = 5
	ShelleyUtxowMissingTxMetadata          = 6
	ShelleyUtxowConflictingMetadataHash    = 7
	ShelleyUtxowInvalidMetadata            = 8
	ShelleyUtxowExtraneousScriptWitnesses  = 9

	// Babbage UTXOW failure tags
	BabbageUtxowAlonzoInBabbage             = 1
	BabbageUtxowUtxoFailure                 = 2
	BabbageUtxowMalformedScriptWitnesses    = 3
	BabbageUtxowMalformedReferenceScripts   = 4
	BabbageUtxowScriptIntegrityHashMismatch = 5

	// Alonzo UTXOW failure tags (wrapped by Babbage tag 1)
	AlonzoUtxowShelleyInAlonzo              = 0
	AlonzoUtxowMissingRedeemers             = 1
	AlonzoUtxowMissingRequiredDatums        = 2
	AlonzoUtxowNotAllowedSupplementalDatums = 3
	AlonzoUtxowPPViewHashesDontMatch        = 4
	AlonzoUtxowUnspendableUTxONoDatumHash   = 6
	AlonzoUtxowExtraRedeemers               = 7

	// Legacy constant for backward compatibility
	UTXOWFailureUtxoFailure = 2

	// Babbage UTXO failure tags
	BabbageUtxoAlonzoInBabbage          = 1
	BabbageUtxoIncorrectTotalCollateral = 2
	BabbageUtxoOutputTooSmallUTxO       = 3
	BabbageUtxoNonDisjointRefInputs     = 4

	// Conway UTXOW failure tags (flat enumeration - no wrapping)
	ConwayUtxowUtxoFailure                  = 0
	ConwayUtxowInvalidWitnesses             = 1
	ConwayUtxowMissingVKeyWitnesses         = 2
	ConwayUtxowMissingScriptWitnesses       = 3
	ConwayUtxowScriptWitnessNotValidating   = 4
	ConwayUtxowMissingTxBodyMetadataHash    = 5
	ConwayUtxowMissingTxMetadata            = 6
	ConwayUtxowConflictingMetadataHash      = 7
	ConwayUtxowInvalidMetadata              = 8
	ConwayUtxowExtraneousScriptWitnesses    = 9
	ConwayUtxowMissingRedeemers             = 10
	ConwayUtxowMissingRequiredDatums        = 11
	ConwayUtxowNotAllowedSupplementalDatums = 12
	ConwayUtxowPPViewHashesDontMatch        = 13
	ConwayUtxowUnspendableUTxONoDatumHash   = 14
	ConwayUtxowExtraRedeemers               = 15
	ConwayUtxowMalformedScriptWitnesses     = 16
	ConwayUtxowMalformedReferenceScripts    = 17
	ConwayUtxowScriptIntegrityHashMismatch  = 18

	// Conway UTXO failure tags (renumbered from Babbage)
	ConwayUtxoUtxosFailure                = 0
	ConwayUtxoBadInputsUTxO               = 1
	ConwayUtxoOutsideValidityIntervalUTxO = 2
	ConwayUtxoMaxTxSizeUTxO               = 3
	ConwayUtxoInputSetEmptyUTxO           = 4
	ConwayUtxoFeeTooSmallUTxO             = 5
	ConwayUtxoValueNotConservedUTxO       = 6
	ConwayUtxoWrongNetwork                = 7
	ConwayUtxoWrongNetworkWithdrawal      = 8
	ConwayUtxoOutputTooSmallUTxO          = 9
	ConwayUtxoOutputBootAddrAttrsTooBig   = 10
	ConwayUtxoOutputTooBigUTxO            = 11
	ConwayUtxoInsufficientCollateral      = 12
	ConwayUtxoScriptsNotPaidUTxO          = 13
	ConwayUtxoExUnitsTooBigUTxO           = 14
	ConwayUtxoCollateralContainsNonADA    = 15
	ConwayUtxoWrongNetworkInTxBody        = 16
	ConwayUtxoOutsideForecast             = 17
	ConwayUtxoTooManyCollateralInputs     = 18
	ConwayUtxoNoCollateralInputs          = 19
	ConwayUtxoIncorrectTotalCollateral    = 20
	ConwayUtxoBabbageOutputTooSmallUTxO   = 21
	ConwayUtxoBabbageNonDisjointRefInputs = 22

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
		// Conway completely renumbered UTXO failure tags - use Conway-specific map
		conwayMap := map[int]any{
			ConwayUtxoUtxosFailure:                &UtxosFailure{},
			ConwayUtxoBadInputsUTxO:               &BadInputsUtxo{},
			ConwayUtxoOutsideValidityIntervalUTxO: &OutsideValidityIntervalUtxo{},
			ConwayUtxoMaxTxSizeUTxO:               &MaxTxSizeUtxo{},
			ConwayUtxoInputSetEmptyUTxO:           &InputSetEmptyUtxo{},
			ConwayUtxoFeeTooSmallUTxO:             &FeeTooSmallUtxo{},
			ConwayUtxoValueNotConservedUTxO:       &ValueNotConservedUtxo{},
			ConwayUtxoWrongNetwork:                &WrongNetwork{},
			ConwayUtxoWrongNetworkWithdrawal:      &WrongNetworkWithdrawal{},
			ConwayUtxoOutputTooSmallUTxO:          &OutputTooSmallUtxo{},
			ConwayUtxoOutputBootAddrAttrsTooBig:   &OutputBootAddrAttrsTooBig{},
			ConwayUtxoOutputTooBigUTxO:            &OutputTooBigUtxo{},
			ConwayUtxoInsufficientCollateral:      &InsufficientCollateral{},
			ConwayUtxoScriptsNotPaidUTxO:          &ScriptsNotPaidUtxo{},
			ConwayUtxoExUnitsTooBigUTxO:           &ExUnitsTooBigUtxo{},
			ConwayUtxoCollateralContainsNonADA:    &CollateralContainsNonADA{},
			ConwayUtxoWrongNetworkInTxBody:        &WrongNetworkInTxBody{},
			ConwayUtxoOutsideForecast:             &OutsideForecast{},
			ConwayUtxoTooManyCollateralInputs:     &TooManyCollateralInputs{},
			ConwayUtxoNoCollateralInputs:          &NoCollateralInputs{},
			ConwayUtxoIncorrectTotalCollateral:    &IncorrectTotalCollateralField{},
			ConwayUtxoBabbageOutputTooSmallUTxO:   &BabbageOutputTooSmallUTxO{},
			ConwayUtxoBabbageNonDisjointRefInputs: &BabbageNonDisjointRefInputs{},
		}
		return conwayMap, ConwayUtxoOutputTooBigUTxO, ConwayUtxoScriptsNotPaidUTxO, ConwayUtxoExUnitsTooBigUTxO, ConwayUtxoCollateralContainsNonADA
	default:
		// For other eras (Byron, Shelley, Allegra, Mary), use Babbage constants as fallback
		baseMap[UtxoFailureOutputTooBigUtxoBabbage] = &OutputTooBigUtxo{}
		baseMap[UtxoFailureScriptsNotPaidUtxoBabbage] = &ScriptsNotPaidUtxo{}
		baseMap[UtxoFailureExUnitsTooBigUtxoBabbage] = &ExUnitsTooBigUtxo{}
		baseMap[UtxoFailureCollateralContainsNonAdaBabbage] = &CollateralContainsNonADA{}
		return baseMap, UtxoFailureOutputTooBigUtxoBabbage, UtxoFailureScriptsNotPaidUtxoBabbage, UtxoFailureExUnitsTooBigUtxoBabbage, UtxoFailureCollateralContainsNonAdaBabbage
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
			ApplyTxError cbor.RawMessage
		}
	}
	if _, err := cbor.Decode(data, &tmpData); err != nil {
		return err
	}
	e.Era = tmpData.Inner.Era
	// Decode ApplyTxError with era context using wrapper
	applyErr := &ApplyTxError{}
	applyErr.era = tmpData.Inner.Era
	if _, err := cbor.Decode(tmpData.Inner.ApplyTxError, applyErr); err != nil {
		return err
	}
	e.Err = *applyErr
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
	era      uint8 // Era context for era-aware decoding (private, not CBOR-encoded)
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
			// Use era-aware UTXOW failure decoding
			utxowErr := &UtxowFailure{era: e.era}
			if _, err := cbor.Decode(tmpFailure[1], utxowErr); err != nil {
				return err
			}
			newErr = utxowErr
		default:
			if tmpErr, err := NewGenericErrorFromCbor(data); err != nil {
				return err
			} else {
				newErr = tmpErr
			}
			if _, err := cbor.Decode(tmpFailure[1], newErr); err != nil {
				return err
			}
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
	era uint8 // Era context for era-aware decoding (private, not CBOR-encoded)
}

func (e *UtxowFailure) UnmarshalCBOR(data []byte) error {
	tmpFailure := []cbor.RawMessage{}
	if _, err := cbor.Decode(data, &tmpFailure); err != nil {
		return err
	}
	if len(tmpFailure) < 1 {
		return errors.New("UtxowFailure: expected at least 1 element")
	}
	failureType, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}

	// Use era-aware decoding (oldest to newest)
	switch e.era {
	case EraIdShelley, EraIdAllegra, EraIdMary:
		// Shelley, Allegra, and Mary share the same UTXOW failure structure
		return e.unmarshalShelley(data, tmpFailure, failureType)
	case EraIdAlonzo:
		// Alonzo wraps Shelley failures in tag 0, adds Plutus-related tags
		return e.unmarshalAlonzo(data, tmpFailure, failureType)
	case EraIdBabbage:
		// Babbage wraps Alonzo failures in tag 1, adds Babbage-specific tags
		return e.unmarshalBabbage(data, tmpFailure, failureType)
	case EraIdConway:
		// Conway uses flat enumeration (no wrapping)
		return e.unmarshalConway(data, tmpFailure, failureType)
	default:
		// For unknown eras (Byron or future eras), try Babbage as fallback
		return e.unmarshalBabbage(data, tmpFailure, failureType)
	}
}

// unmarshalShelley handles Shelley, Allegra, and Mary era UTXOW failures.
// These eras share the same UTXOW failure structure (direct tags, no wrapping).
func (e *UtxowFailure) unmarshalShelley(data []byte, tmpFailure []cbor.RawMessage, failureType int) error {
	var newErr error
	switch failureType {
	case ShelleyUtxowInvalidWitnesses:
		newErr = &InvalidWitnessesUTXOW{}
	case ShelleyUtxowMissingVKeyWitnesses:
		newErr = &MissingVKeyWitnessesUTXOW{}
	case ShelleyUtxowMissingScriptWitnesses:
		newErr = &MissingScriptWitnessesUTXOW{}
	case ShelleyUtxowScriptWitnessNotValidating:
		newErr = &ScriptWitnessNotValidatingUTXOW{}
	case ShelleyUtxowUtxoFailure:
		// UTXO failures - use era-aware UtxoFailure
		newErr = &UtxoFailure{}
	case ShelleyUtxowMissingTxBodyMetadataHash:
		newErr = &MissingTxBodyMetadataHash{}
	case ShelleyUtxowMissingTxMetadata:
		newErr = &MissingTxMetadata{}
	case ShelleyUtxowConflictingMetadataHash:
		newErr = &ConflictingMetadataHash{}
	case ShelleyUtxowInvalidMetadata:
		newErr = &InvalidMetadata{}
		// InvalidMetadata has no payload
		e.Err = newErr
		return nil
	case ShelleyUtxowExtraneousScriptWitnesses:
		newErr = &ExtraneousScriptWitnessesUTXOW{}
	default:
		if tmpErr, err := NewGenericErrorFromCbor(data); err != nil {
			return err
		} else {
			newErr = tmpErr
		}
	}
	if len(tmpFailure) >= 2 {
		if _, err := cbor.Decode(tmpFailure[1], newErr); err != nil {
			return err
		}
	}
	e.Err = newErr
	return nil
}

// unmarshalAlonzo handles Alonzo era UTXOW failures.
// Alonzo wraps Shelley failures in tag 0 and adds Plutus-related tags.
func (e *UtxowFailure) unmarshalAlonzo(data []byte, tmpFailure []cbor.RawMessage, failureType int) error {
	if len(tmpFailure) < 2 {
		return errors.New("UtxowFailure (Alonzo): expected at least 2 elements")
	}
	var newErr error
	switch failureType {
	case AlonzoUtxowShelleyInAlonzo:
		// Shelley UTXOW failures wrapped in Alonzo
		newErr = &ShelleyUtxowFailure{}
	case AlonzoUtxowMissingRedeemers:
		newErr = &MissingRedeemers{}
	case AlonzoUtxowMissingRequiredDatums:
		newErr = &MissingRequiredDatums{}
	case AlonzoUtxowNotAllowedSupplementalDatums:
		newErr = &NotAllowedSupplementalDatums{}
	case AlonzoUtxowPPViewHashesDontMatch:
		newErr = &PPViewHashesDontMatch{}
	case AlonzoUtxowUnspendableUTxONoDatumHash:
		newErr = &UnspendableUTxONoDatumHash{}
	case AlonzoUtxowExtraRedeemers:
		newErr = &ExtraRedeemers{}
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

// unmarshalBabbage handles Babbage era UTXOW failures.
// Babbage wraps Alonzo failures in tag 1 and adds Babbage-specific tags.
func (e *UtxowFailure) unmarshalBabbage(data []byte, tmpFailure []cbor.RawMessage, failureType int) error {
	if len(tmpFailure) < 2 {
		return errors.New("UtxowFailure (Babbage): expected at least 2 elements")
	}
	var newErr error
	switch failureType {
	case BabbageUtxowAlonzoInBabbage:
		// Alonzo UTXOW failures wrapped in Babbage
		newErr = &AlonzoUtxowFailure{}
	case BabbageUtxowUtxoFailure:
		// UTXO failures (may be Babbage-specific or wrapped Alonzo)
		newErr = &BabbageUtxoFailure{}
	case BabbageUtxowMalformedScriptWitnesses:
		newErr = &MalformedScriptWitnesses{}
	case BabbageUtxowMalformedReferenceScripts:
		newErr = &MalformedReferenceScripts{}
	case BabbageUtxowScriptIntegrityHashMismatch:
		// Babbage script integrity hash mismatch - use generic for complex structure
		if tmpErr, err := NewGenericErrorFromCbor(data); err != nil {
			return err
		} else {
			newErr = tmpErr
		}
		e.Err = newErr
		return nil
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

func (e *UtxowFailure) unmarshalConway(data []byte, tmpFailure []cbor.RawMessage, failureType int) error {
	var newErr error
	switch failureType {
	case ConwayUtxowUtxoFailure:
		// UTXO failures use Conway's renumbered tags
		newErr = &UtxoFailure{}
	case ConwayUtxowInvalidWitnesses:
		newErr = &InvalidWitnessesUTXOW{}
	case ConwayUtxowMissingVKeyWitnesses:
		newErr = &MissingVKeyWitnessesUTXOW{}
	case ConwayUtxowMissingScriptWitnesses:
		newErr = &MissingScriptWitnessesUTXOW{}
	case ConwayUtxowScriptWitnessNotValidating:
		newErr = &ScriptWitnessNotValidatingUTXOW{}
	case ConwayUtxowMissingTxBodyMetadataHash:
		newErr = &MissingTxBodyMetadataHash{}
	case ConwayUtxowMissingTxMetadata:
		newErr = &MissingTxMetadata{}
	case ConwayUtxowConflictingMetadataHash:
		newErr = &ConflictingMetadataHash{}
	case ConwayUtxowInvalidMetadata:
		newErr = &InvalidMetadata{}
		// InvalidMetadata has no payload
		e.Err = newErr
		return nil
	case ConwayUtxowExtraneousScriptWitnesses:
		newErr = &ExtraneousScriptWitnessesUTXOW{}
	case ConwayUtxowMissingRedeemers:
		newErr = &MissingRedeemers{}
	case ConwayUtxowMissingRequiredDatums:
		newErr = &MissingRequiredDatums{}
	case ConwayUtxowNotAllowedSupplementalDatums:
		newErr = &NotAllowedSupplementalDatums{}
	case ConwayUtxowPPViewHashesDontMatch:
		newErr = &PPViewHashesDontMatch{}
	case ConwayUtxowUnspendableUTxONoDatumHash:
		newErr = &UnspendableUTxONoDatumHash{}
	case ConwayUtxowExtraRedeemers:
		newErr = &ExtraRedeemers{}
	case ConwayUtxowMalformedScriptWitnesses:
		newErr = &MalformedScriptWitnesses{}
	case ConwayUtxowMalformedReferenceScripts:
		newErr = &MalformedReferenceScripts{}
	case ConwayUtxowScriptIntegrityHashMismatch:
		// Complex structure - use generic
		if tmpErr, err := NewGenericErrorFromCbor(data); err != nil {
			return err
		} else {
			newErr = tmpErr
		}
		e.Err = newErr
		return nil
	default:
		if tmpErr, err := NewGenericErrorFromCbor(data); err != nil {
			return err
		} else {
			newErr = tmpErr
		}
	}
	if len(tmpFailure) >= 2 {
		if _, err := cbor.Decode(tmpFailure[1], newErr); err != nil {
			return err
		}
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
				Id:     input,
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
				Id:     input,
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

// =============================================================================
// Babbage UTXOW Predicate Failures
// =============================================================================

// MalformedScriptWitnesses represents scripts in witnesses that failed well-formedness validation
// CBOR: [3, [script_hash, ...]]
type MalformedScriptWitnesses struct {
	cbor.StructAsArray
	Type         uint8
	ScriptHashes []common.Blake2b224
}

func (e *MalformedScriptWitnesses) Error() string {
	var sb strings.Builder
	sb.WriteString("MalformedScriptWitnesses ([")
	for idx, hash := range e.ScriptHashes {
		sb.WriteString(hash.String())
		if idx < len(e.ScriptHashes)-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
}

// MalformedReferenceScripts represents reference scripts that failed well-formedness validation
// CBOR: [4, [script_hash, ...]]
type MalformedReferenceScripts struct {
	cbor.StructAsArray
	Type         uint8
	ScriptHashes []common.Blake2b224
}

func (e *MalformedReferenceScripts) Error() string {
	var sb strings.Builder
	sb.WriteString("MalformedReferenceScripts ([")
	for idx, hash := range e.ScriptHashes {
		sb.WriteString(hash.String())
		if idx < len(e.ScriptHashes)-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
}

// =============================================================================
// Babbage UTXO Predicate Failures
// =============================================================================

// IncorrectTotalCollateralField represents when the declared total collateral
// doesn't match the actual collateral balance
// CBOR: [2, delta_coin, coin]
type IncorrectTotalCollateralField struct {
	cbor.StructAsArray
	Type            uint8
	BalanceComputed int64  // DeltaCoin (can be negative)
	TotalCollateral uint64 // Coin (declared in tx body)
}

func (e *IncorrectTotalCollateralField) Error() string {
	return fmt.Sprintf(
		"IncorrectTotalCollateralField (BalanceComputed %d, TotalCollateral %d)",
		e.BalanceComputed,
		e.TotalCollateral,
	)
}

// BabbageOutputTooSmallUTxO represents outputs that don't meet minimum ADA requirement
// Different from Alonzo's OutputTooSmallUtxo - includes the minimum required amount
// CBOR: [3, [[txout, min_required], ...]]
type BabbageOutputTooSmallUTxO struct {
	cbor.StructAsArray
	Type    uint8
	Outputs []BabbageOutputTooSmallEntry
}

// BabbageOutputTooSmallEntry contains the output and its minimum required ADA
type BabbageOutputTooSmallEntry struct {
	cbor.StructAsArray
	Output      TxOut
	MinRequired uint64
}

func (e *BabbageOutputTooSmallUTxO) Error() string {
	var sb strings.Builder
	sb.WriteString("BabbageOutputTooSmallUTxO ([")
	for idx, entry := range e.Outputs {
		sb.WriteString(fmt.Sprintf("(Output %s, MinRequired %d)",
			entry.Output.String(), entry.MinRequired))
		if idx < len(e.Outputs)-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
}

// BabbageNonDisjointRefInputs represents when reference inputs overlap with regular inputs
// CBOR: [4, [txin, ...]]
type BabbageNonDisjointRefInputs struct {
	cbor.StructAsArray
	Type   uint8
	Inputs []TxIn
}

func (e *BabbageNonDisjointRefInputs) Error() string {
	var sb strings.Builder
	sb.WriteString("BabbageNonDisjointRefInputs ([")
	for idx, input := range e.Inputs {
		sb.WriteString(input.String())
		if idx < len(e.Inputs)-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
}

// =============================================================================
// Alonzo UTXOW Predicate Failures (wrapped by Babbage)
// =============================================================================

// MissingRedeemers represents missing redeemers for script execution
// CBOR: [1, [[purpose, script_hash], ...]]
type MissingRedeemers struct {
	cbor.StructAsArray
	Type    uint8
	Missing []MissingRedeemerEntry
}

// MissingRedeemerEntry contains purpose and script hash for a missing redeemer
type MissingRedeemerEntry struct {
	cbor.StructAsArray
	Purpose    cbor.Value // PlutusPurpose as generic value
	ScriptHash common.Blake2b224
}

func (e *MissingRedeemers) Error() string {
	var sb strings.Builder
	sb.WriteString("MissingRedeemers ([")
	for idx, entry := range e.Missing {
		sb.WriteString(fmt.Sprintf("(Purpose %v, ScriptHash %s)",
			entry.Purpose.Value(), entry.ScriptHash.String()))
		if idx < len(e.Missing)-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
}

// MissingRequiredDatums represents required datums not provided in witness set
// CBOR: [2, [missing_hashes], [received_hashes]]
type MissingRequiredDatums struct {
	cbor.StructAsArray
	Type     uint8
	Missing  []common.Blake2b256
	Received []common.Blake2b256
}

func (e *MissingRequiredDatums) Error() string {
	return fmt.Sprintf(
		"MissingRequiredDatums (Missing %d datums, Received %d datums)",
		len(e.Missing),
		len(e.Received),
	)
}

// NotAllowedSupplementalDatums represents supplemental datums that aren't allowed
// CBOR: [3, [unallowed_hashes], [acceptable_hashes]]
type NotAllowedSupplementalDatums struct {
	cbor.StructAsArray
	Type       uint8
	Unallowed  []common.Blake2b256
	Acceptable []common.Blake2b256
}

func (e *NotAllowedSupplementalDatums) Error() string {
	return fmt.Sprintf(
		"NotAllowedSupplementalDatums (Unallowed %d datums, Acceptable %d datums)",
		len(e.Unallowed),
		len(e.Acceptable),
	)
}

// PPViewHashesDontMatch represents protocol parameter view hash mismatch
// CBOR: [4, [expected, computed]]
type PPViewHashesDontMatch struct {
	cbor.StructAsArray
	Type     uint8
	Expected cbor.Value // StrictMaybe ScriptIntegrityHash
	Computed cbor.Value // StrictMaybe ScriptIntegrityHash
}

func (e *PPViewHashesDontMatch) Error() string {
	return fmt.Sprintf(
		"PPViewHashesDontMatch (Expected %v, Computed %v)",
		e.Expected.Value(),
		e.Computed.Value(),
	)
}

// UnspendableUTxONoDatumHash represents script-locked UTxOs missing datum hash
// CBOR: [6, [txin, ...]]
type UnspendableUTxONoDatumHash struct {
	cbor.StructAsArray
	Type   uint8
	Inputs []TxIn
}

func (e *UnspendableUTxONoDatumHash) Error() string {
	var sb strings.Builder
	sb.WriteString("UnspendableUTxONoDatumHash ([")
	for idx, input := range e.Inputs {
		sb.WriteString(input.String())
		if idx < len(e.Inputs)-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
}

// ExtraRedeemers represents redeemers provided for non-existent scripts
// CBOR: [7, [[tag, index], ...]]
type ExtraRedeemers struct {
	cbor.StructAsArray
	Type      uint8
	Redeemers []ExtraRedeemerEntry
}

// ExtraRedeemerEntry contains the tag and index of an extra redeemer
type ExtraRedeemerEntry struct {
	cbor.StructAsArray
	Tag   uint8 // 0=spend, 1=mint, 2=cert, 3=reward
	Index uint64
}

func (e *ExtraRedeemers) Error() string {
	var sb strings.Builder
	sb.WriteString("ExtraRedeemers ([")
	tagNames := []string{"Spend", "Mint", "Cert", "Reward"}
	for idx, entry := range e.Redeemers {
		tagName := "Unknown"
		if int(entry.Tag) < len(tagNames) {
			tagName = tagNames[entry.Tag]
		}
		sb.WriteString(fmt.Sprintf("(%s, Index %d)", tagName, entry.Index))
		if idx < len(e.Redeemers)-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
}

// ShelleyUtxowFailure wraps Shelley-era UTXOW failures when encountered in Alonzo
// (and transitively in Babbage via AlonzoUtxowFailure)
type ShelleyUtxowFailure struct {
	cbor.StructAsArray
	Err error
}

func (e *ShelleyUtxowFailure) Error() string {
	return fmt.Sprintf("ShelleyInAlonzoUtxowPredFailure (%s)", e.Err)
}

func (e *ShelleyUtxowFailure) UnmarshalCBOR(data []byte) error {
	tmpFailure := []cbor.RawMessage{}
	if _, err := cbor.Decode(data, &tmpFailure); err != nil {
		return err
	}
	if len(tmpFailure) < 1 {
		return errors.New("ShelleyUtxowFailure: expected at least 1 element")
	}
	failureType, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	var newErr error
	switch failureType {
	case ShelleyUtxowInvalidWitnesses:
		newErr = &InvalidWitnessesUTXOW{}
	case ShelleyUtxowMissingVKeyWitnesses:
		newErr = &MissingVKeyWitnessesUTXOW{}
	case ShelleyUtxowMissingScriptWitnesses:
		newErr = &MissingScriptWitnessesUTXOW{}
	case ShelleyUtxowScriptWitnessNotValidating:
		newErr = &ScriptWitnessNotValidatingUTXOW{}
	case ShelleyUtxowUtxoFailure:
		newErr = &UtxoFailure{}
	case ShelleyUtxowMissingTxBodyMetadataHash:
		newErr = &MissingTxBodyMetadataHash{}
	case ShelleyUtxowMissingTxMetadata:
		newErr = &MissingTxMetadata{}
	case ShelleyUtxowConflictingMetadataHash:
		newErr = &ConflictingMetadataHash{}
	case ShelleyUtxowInvalidMetadata:
		newErr = &InvalidMetadata{}
		// InvalidMetadata has no payload
		e.Err = newErr
		return nil
	case ShelleyUtxowExtraneousScriptWitnesses:
		newErr = &ExtraneousScriptWitnessesUTXOW{}
	default:
		if tmpErr, err := NewGenericErrorFromCbor(data); err != nil {
			return err
		} else {
			newErr = tmpErr
		}
	}
	if len(tmpFailure) >= 2 {
		if _, err := cbor.Decode(tmpFailure[1], newErr); err != nil {
			return err
		}
	}
	e.Err = newErr
	return nil
}

// AlonzoUtxowFailure wraps Alonzo-era UTXOW failures when encountered in Babbage
type AlonzoUtxowFailure struct {
	cbor.StructAsArray
	Err error
}

func (e *AlonzoUtxowFailure) Error() string {
	return fmt.Sprintf("AlonzoInBabbageUtxowPredFailure (%s)", e.Err)
}

func (e *AlonzoUtxowFailure) UnmarshalCBOR(data []byte) error {
	tmpFailure := []cbor.RawMessage{}
	if _, err := cbor.Decode(data, &tmpFailure); err != nil {
		return err
	}
	if len(tmpFailure) < 2 {
		return errors.New("AlonzoUtxowFailure: expected at least 2 elements")
	}
	failureType, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	var newErr error
	switch failureType {
	case AlonzoUtxowShelleyInAlonzo:
		// Shelley failures wrapped in Alonzo - use ShelleyUtxowFailure
		newErr = &ShelleyUtxowFailure{}
	case AlonzoUtxowMissingRedeemers:
		newErr = &MissingRedeemers{}
	case AlonzoUtxowMissingRequiredDatums:
		newErr = &MissingRequiredDatums{}
	case AlonzoUtxowNotAllowedSupplementalDatums:
		newErr = &NotAllowedSupplementalDatums{}
	case AlonzoUtxowPPViewHashesDontMatch:
		newErr = &PPViewHashesDontMatch{}
	case AlonzoUtxowUnspendableUTxONoDatumHash:
		newErr = &UnspendableUTxONoDatumHash{}
	case AlonzoUtxowExtraRedeemers:
		newErr = &ExtraRedeemers{}
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

// BabbageUtxoFailure wraps Babbage-era UTXO failures
type BabbageUtxoFailure struct {
	cbor.StructAsArray
	Err error
}

func (e *BabbageUtxoFailure) Error() string {
	return fmt.Sprintf("BabbageUtxoFailure (%s)", e.Err)
}

func (e *BabbageUtxoFailure) UnmarshalCBOR(data []byte) error {
	tmpFailure := []cbor.RawMessage{}
	if _, err := cbor.Decode(data, &tmpFailure); err != nil {
		return err
	}
	if len(tmpFailure) < 2 {
		return errors.New("BabbageUtxoFailure: expected at least 2 elements")
	}
	failureType, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	var newErr error
	switch failureType {
	case BabbageUtxoAlonzoInBabbage:
		// Alonzo UTXO failures wrapped - delegate to existing UtxoFailure logic
		newErr = &UtxoFailure{}
	case BabbageUtxoIncorrectTotalCollateral:
		newErr = &IncorrectTotalCollateralField{}
	case BabbageUtxoOutputTooSmallUTxO:
		newErr = &BabbageOutputTooSmallUTxO{}
	case BabbageUtxoNonDisjointRefInputs:
		newErr = &BabbageNonDisjointRefInputs{}
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

// =============================================================================
// Conway UTXOW Predicate Failures (Shelley-derived)
// =============================================================================

// InvalidWitnessesUTXOW represents invalid VKey witnesses
// CBOR: [1, [vkey, ...]]
type InvalidWitnessesUTXOW struct {
	cbor.StructAsArray
	Type  uint8
	VKeys []cbor.ByteString
}

func (e *InvalidWitnessesUTXOW) Error() string {
	return fmt.Sprintf("InvalidWitnessesUTXOW (%d invalid witnesses)", len(e.VKeys))
}

// MissingVKeyWitnessesUTXOW represents missing VKey witnesses
// CBOR: [2, [keyhash, ...]]
type MissingVKeyWitnessesUTXOW struct {
	cbor.StructAsArray
	Type      uint8
	KeyHashes []common.Blake2b224
}

func (e *MissingVKeyWitnessesUTXOW) Error() string {
	var sb strings.Builder
	sb.WriteString("MissingVKeyWitnessesUTXOW ([")
	for idx, hash := range e.KeyHashes {
		sb.WriteString(hash.String())
		if idx < len(e.KeyHashes)-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
}

// MissingScriptWitnessesUTXOW represents missing script witnesses
// CBOR: [3, [scripthash, ...]]
type MissingScriptWitnessesUTXOW struct {
	cbor.StructAsArray
	Type         uint8
	ScriptHashes []common.Blake2b224
}

func (e *MissingScriptWitnessesUTXOW) Error() string {
	var sb strings.Builder
	sb.WriteString("MissingScriptWitnessesUTXOW ([")
	for idx, hash := range e.ScriptHashes {
		sb.WriteString(hash.String())
		if idx < len(e.ScriptHashes)-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
}

// ScriptWitnessNotValidatingUTXOW represents scripts that failed validation
// CBOR: [4, [scripthash, ...]]
type ScriptWitnessNotValidatingUTXOW struct {
	cbor.StructAsArray
	Type         uint8
	ScriptHashes []common.Blake2b224
}

func (e *ScriptWitnessNotValidatingUTXOW) Error() string {
	var sb strings.Builder
	sb.WriteString("ScriptWitnessNotValidatingUTXOW ([")
	for idx, hash := range e.ScriptHashes {
		sb.WriteString(hash.String())
		if idx < len(e.ScriptHashes)-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
}

// MissingTxBodyMetadataHash represents missing metadata hash in tx body
// CBOR: [5, auxdatahash]
type MissingTxBodyMetadataHash struct {
	cbor.StructAsArray
	Type uint8
	Hash common.Blake2b256
}

func (e *MissingTxBodyMetadataHash) Error() string {
	return fmt.Sprintf("MissingTxBodyMetadataHash (%s)", e.Hash.String())
}

// MissingTxMetadata represents missing metadata when hash is present
// CBOR: [6, auxdatahash]
type MissingTxMetadata struct {
	cbor.StructAsArray
	Type uint8
	Hash common.Blake2b256
}

func (e *MissingTxMetadata) Error() string {
	return fmt.Sprintf("MissingTxMetadata (%s)", e.Hash.String())
}

// ConflictingMetadataHash represents metadata hash mismatch
// CBOR: [7, [expected, found]]
type ConflictingMetadataHash struct {
	cbor.StructAsArray
	Type     uint8
	Expected common.Blake2b256
	Found    common.Blake2b256
}

func (e *ConflictingMetadataHash) Error() string {
	return fmt.Sprintf(
		"ConflictingMetadataHash (Expected %s, Found %s)",
		e.Expected.String(),
		e.Found.String(),
	)
}

// InvalidMetadata represents invalid metadata format
// CBOR: [8]
type InvalidMetadata struct {
	cbor.StructAsArray
	Type uint8
}

func (e *InvalidMetadata) Error() string {
	return "InvalidMetadata"
}

// ExtraneousScriptWitnessesUTXOW represents unnecessary script witnesses
// CBOR: [9, [scripthash, ...]]
type ExtraneousScriptWitnessesUTXOW struct {
	cbor.StructAsArray
	Type         uint8
	ScriptHashes []common.Blake2b224
}

func (e *ExtraneousScriptWitnessesUTXOW) Error() string {
	var sb strings.Builder
	sb.WriteString("ExtraneousScriptWitnessesUTXOW ([")
	for idx, hash := range e.ScriptHashes {
		sb.WriteString(hash.String())
		if idx < len(e.ScriptHashes)-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
}

// =============================================================================
// Conway UTXOW Failure Decoder
// =============================================================================

// ConwayUtxowFailure handles Conway-era UTXOW failures (flat enumeration)
type ConwayUtxowFailure struct {
	cbor.StructAsArray
	Err error
}

func (e *ConwayUtxowFailure) Error() string {
	return fmt.Sprintf("ConwayUtxowFailure (%s)", e.Err)
}

func (e *ConwayUtxowFailure) UnmarshalCBOR(data []byte) error {
	tmpFailure := []cbor.RawMessage{}
	if _, err := cbor.Decode(data, &tmpFailure); err != nil {
		return err
	}
	if len(tmpFailure) < 1 {
		return errors.New("ConwayUtxowFailure: expected at least 1 element")
	}
	failureType, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	var newErr error
	switch failureType {
	case ConwayUtxowUtxoFailure:
		// UTXO failures use Conway's renumbered tags
		newErr = &UtxoFailure{}
	case ConwayUtxowInvalidWitnesses:
		newErr = &InvalidWitnessesUTXOW{}
	case ConwayUtxowMissingVKeyWitnesses:
		newErr = &MissingVKeyWitnessesUTXOW{}
	case ConwayUtxowMissingScriptWitnesses:
		newErr = &MissingScriptWitnessesUTXOW{}
	case ConwayUtxowScriptWitnessNotValidating:
		newErr = &ScriptWitnessNotValidatingUTXOW{}
	case ConwayUtxowMissingTxBodyMetadataHash:
		newErr = &MissingTxBodyMetadataHash{}
	case ConwayUtxowMissingTxMetadata:
		newErr = &MissingTxMetadata{}
	case ConwayUtxowConflictingMetadataHash:
		newErr = &ConflictingMetadataHash{}
	case ConwayUtxowInvalidMetadata:
		newErr = &InvalidMetadata{}
		// InvalidMetadata has no payload, just return
		e.Err = newErr
		return nil
	case ConwayUtxowExtraneousScriptWitnesses:
		newErr = &ExtraneousScriptWitnessesUTXOW{}
	case ConwayUtxowMissingRedeemers:
		newErr = &MissingRedeemers{}
	case ConwayUtxowMissingRequiredDatums:
		newErr = &MissingRequiredDatums{}
	case ConwayUtxowNotAllowedSupplementalDatums:
		newErr = &NotAllowedSupplementalDatums{}
	case ConwayUtxowPPViewHashesDontMatch:
		newErr = &PPViewHashesDontMatch{}
	case ConwayUtxowUnspendableUTxONoDatumHash:
		newErr = &UnspendableUTxONoDatumHash{}
	case ConwayUtxowExtraRedeemers:
		newErr = &ExtraRedeemers{}
	case ConwayUtxowMalformedScriptWitnesses:
		newErr = &MalformedScriptWitnesses{}
	case ConwayUtxowMalformedReferenceScripts:
		newErr = &MalformedReferenceScripts{}
	case ConwayUtxowScriptIntegrityHashMismatch:
		// Complex structure - use generic
		if tmpErr, err := NewGenericErrorFromCbor(data); err != nil {
			return err
		} else {
			newErr = tmpErr
		}
		e.Err = newErr
		return nil
	default:
		if tmpErr, err := NewGenericErrorFromCbor(data); err != nil {
			return err
		} else {
			newErr = tmpErr
		}
	}
	if len(tmpFailure) >= 2 {
		if _, err := cbor.Decode(tmpFailure[1], newErr); err != nil {
			return err
		}
	}
	e.Err = newErr
	return nil
}
