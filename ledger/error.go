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

	// LEDGER incomplete withdrawals failure tags.
	ShelleyLedgerIncompleteWithdrawals = 3
	ConwayLedgerIncompleteWithdrawals  = 9

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

	// Dijkstra-only UTXOW failure tags. Dijkstra's UTXOW predicate
	// failure (DijkstraUtxowPredFailure) shares Conway's tags 0-18
	// unchanged and adds two new constructors for guarded-subtransaction
	// validation (confirmed against cardano-ledger
	// eras/dijkstra/impl/src/Cardano/Ledger/Dijkstra/Rules/Utxow.hs,
	// EncCBOR/DecCBOR instances).
	DijkstraUtxowMissingRequiredGuards = 19
	DijkstraUtxowMalformedGuardDatums  = 20

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

	// Dijkstra UTXO failure tags. Dijkstra shares Conway's tags 0-7
	// (UtxosFailure through WrongNetwork) unchanged; tag 8
	// (WrongNetworkWithdrawal) has no Dijkstra equivalent, see below.
	// Its Utxo.hs does NOT carry forward a separate OutputTooSmallUTxO
	// constructor at tag 9 the way Conway does: everything from
	// OutputBootAddrAttrsTooBig onward is shifted down by one tag versus
	// Conway's numbering (cardano-ledger
	// eras/dijkstra/impl/src/Cardano/Ledger/Dijkstra/Rules/Utxo.hs).
	// Reusing Conway's map/constants here would misdecode a Dijkstra
	// OutputBootAddrAttrsTooBig (tag 9) as OutputTooSmallUTxO, a Dijkstra
	// OutputTooBigUTxO (tag 10) as OutputBootAddrAttrsTooBig, and so on.
	DijkstraUtxoOutputBootAddrAttrsTooBig = 9
	DijkstraUtxoOutputTooBigUTxO          = 10
	DijkstraUtxoInsufficientCollateral    = 11
	DijkstraUtxoScriptsNotPaidUTxO        = 12
	DijkstraUtxoExUnitsTooBigUTxO         = 13
	DijkstraUtxoCollateralContainsNonADA  = 14
	DijkstraUtxoWrongNetworkInTxBody      = 15
	DijkstraUtxoOutsideForecast           = 16
	DijkstraUtxoTooManyCollateralInputs   = 17
	DijkstraUtxoNoCollateralInputs        = 18

	// Dijkstra-only UTXO failure tags 19-24 (confirmed against
	// cardano-ledger eras/dijkstra/impl/src/Cardano/Ledger/Dijkstra/
	// Rules/Utxo.hs, EncCBOR/DecCBOR instances for
	// DijkstraUtxoPredFailure). Tag 23 is deliberately absent: Dijkstra's
	// decCBOR has no case for it (falls through to `Invalid n`), so there
	// is no real constructor at tag 23 to map here.
	DijkstraUtxoIncorrectTotalCollateral        = 19
	DijkstraUtxoBabbageOutputTooSmallUTxO       = 20
	DijkstraUtxoBabbageNonDisjointRefInputs     = 21
	DijkstraUtxoPtrPresentInCollateralReturn    = 22
	DijkstraUtxoWithdrawalsExceedAccountBalance = 24

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
	// Allegra/Mary era error constants (Shelley has no equivalent tag)
	UtxoFailureOutputTooBigUtxoAllegraMary = 12

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

// getEraSpecificUtxoFailureConstants returns the correct error constants
// for the given era. It returns an error for any era it does not
// explicitly recognize instead of silently guessing another era's
// constructor numbering, since a wrong guess would misdecode the failure
// payload (Conway, for example, completely renumbered these tags).
func getEraSpecificUtxoFailureConstants(
	eraId uint8,
) (map[int]any, int, int, int, int, error) {
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
		return baseMap, UtxoFailureOutputTooBigUtxoAlonzo, UtxoFailureScriptsNotPaidUtxoAlonzo, UtxoFailureExUnitsTooBigUtxoAlonzo, UtxoFailureCollateralContainsNonAdaAlonzo, nil
	case EraIdBabbage:
		baseMap[UtxoFailureOutputTooBigUtxoBabbage] = &OutputTooBigUtxo{}
		baseMap[UtxoFailureScriptsNotPaidUtxoBabbage] = &ScriptsNotPaidUtxo{}
		baseMap[UtxoFailureExUnitsTooBigUtxoBabbage] = &ExUnitsTooBigUtxo{}
		baseMap[UtxoFailureCollateralContainsNonAdaBabbage] = &CollateralContainsNonADA{}
		return baseMap, UtxoFailureOutputTooBigUtxoBabbage, UtxoFailureScriptsNotPaidUtxoBabbage, UtxoFailureExUnitsTooBigUtxoBabbage, UtxoFailureCollateralContainsNonAdaBabbage, nil
	case EraIdConway:
		// Conway completely renumbered UTXO failure tags - use
		// Conway-specific map.
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
		return conwayMap, ConwayUtxoOutputTooBigUTxO, ConwayUtxoScriptsNotPaidUTxO, ConwayUtxoExUnitsTooBigUTxO, ConwayUtxoCollateralContainsNonADA, nil
	case EraIdDijkstra:
		// Dijkstra reuses Conway's tags 0-7 (UtxosFailure through
		// WrongNetwork) verbatim, but its Utxo.hs does not
		// carry forward a distinct OutputTooSmallUTxO constructor at
		// tag 9 the way Conway does: everything from
		// OutputBootAddrAttrsTooBig onward is shifted down by one tag
		// versus Conway (cardano-ledger
		// eras/dijkstra/impl/src/Cardano/Ledger/Dijkstra/Rules/Utxo.hs).
		// Aliasing this to Conway's map/constants would misdecode a
		// Dijkstra OutputBootAddrAttrsTooBig (tag 9) as
		// OutputTooSmallUTxO, a Dijkstra OutputTooBigUTxO (tag 10) as
		// OutputBootAddrAttrsTooBig, and so on. Tags 19-24 (beyond
		// NoCollateralInputs at 18) are confirmed against
		// cardano-ledger's EncCBOR/DecCBOR instances for
		// DijkstraUtxoPredFailure: 19=IncorrectTotalCollateralField,
		// 20=BabbageOutputTooSmallUTxO, 21=BabbageNonDisjointRefInputs
		// (all three reuse the existing Babbage/Conway Go types below,
		// since their field layout is unchanged in Dijkstra),
		// 22=PtrPresentInCollateralReturn (new: the collateral return
		// output uses a pointer-address stake reference, which Dijkstra
		// disallows), and 24=WithdrawalsExceedAccountBalance (new:
		// per-account withdrawals across a tx batch exceed the account's
		// original balance). Tag 23 does not exist in Dijkstra's decCBOR
		// (it falls through to `Invalid n` there), so it is deliberately
		// left unmapped here too instead of being invented. Tag 8
		// (ConwayUtxoWrongNetworkWithdrawal) is likewise absent from
		// Dijkstra's encCBOR/decCBOR: the constructor list jumps
		// directly from WrongNetwork at tag 7 to
		// OutputBootAddrAttrsTooBig at tag 9, so tag 8 also falls
		// through to `Invalid n` there and must not be mapped to
		// WrongNetworkWithdrawal here.
		dijkstraMap := map[int]any{
			ConwayUtxoUtxosFailure:                      &UtxosFailure{},
			ConwayUtxoBadInputsUTxO:                     &BadInputsUtxo{},
			ConwayUtxoOutsideValidityIntervalUTxO:       &OutsideValidityIntervalUtxo{},
			ConwayUtxoMaxTxSizeUTxO:                     &MaxTxSizeUtxo{},
			ConwayUtxoInputSetEmptyUTxO:                 &InputSetEmptyUtxo{},
			ConwayUtxoFeeTooSmallUTxO:                   &FeeTooSmallUtxo{},
			ConwayUtxoValueNotConservedUTxO:             &ValueNotConservedUtxo{},
			ConwayUtxoWrongNetwork:                      &WrongNetwork{},
			DijkstraUtxoOutputBootAddrAttrsTooBig:       &OutputBootAddrAttrsTooBig{},
			DijkstraUtxoOutputTooBigUTxO:                &OutputTooBigUtxo{},
			DijkstraUtxoInsufficientCollateral:          &InsufficientCollateral{},
			DijkstraUtxoScriptsNotPaidUTxO:              &ScriptsNotPaidUtxo{},
			DijkstraUtxoExUnitsTooBigUTxO:               &ExUnitsTooBigUtxo{},
			DijkstraUtxoCollateralContainsNonADA:        &CollateralContainsNonADA{},
			DijkstraUtxoWrongNetworkInTxBody:            &WrongNetworkInTxBody{},
			DijkstraUtxoOutsideForecast:                 &OutsideForecast{},
			DijkstraUtxoTooManyCollateralInputs:         &TooManyCollateralInputs{},
			DijkstraUtxoNoCollateralInputs:              &NoCollateralInputs{},
			DijkstraUtxoIncorrectTotalCollateral:        &IncorrectTotalCollateralField{},
			DijkstraUtxoBabbageOutputTooSmallUTxO:       &BabbageOutputTooSmallUTxO{},
			DijkstraUtxoBabbageNonDisjointRefInputs:     &BabbageNonDisjointRefInputs{},
			DijkstraUtxoPtrPresentInCollateralReturn:    &PtrPresentInCollateralReturn{},
			DijkstraUtxoWithdrawalsExceedAccountBalance: &WithdrawalsExceedAccountBalance{},
		}
		return dijkstraMap,
			DijkstraUtxoOutputTooBigUTxO,
			DijkstraUtxoScriptsNotPaidUTxO,
			DijkstraUtxoExUnitsTooBigUTxO,
			DijkstraUtxoCollateralContainsNonADA,
			nil
	case EraIdShelley:
		// Shelley predates Plutus scripts and collateral entirely, so
		// baseMap (which includes the collateral-related tags added
		// for Alonzo's UTXO failure enumeration:
		// UtxoFailureInsufficientCollateral (12),
		// UtxoFailureWrongNetworkInTxBody (17),
		// UtxoFailureOutsideForecast (18),
		// UtxoFailureTooManyCollateralInputs (19), and
		// UtxoFailureNoCollateralInputs (20)) is NOT correct here: tag
		// 12 doesn't exist at all in Shelley, so decoding it as
		// InsufficientCollateral would misdecode a genuinely
		// unknown/malformed tag as a real (and wrong) failure kind.
		// The authoritative Shelley Utxo.hs constructor list is tags
		// 0-10 only (BadInputsUtxo through OutputBootAddrAttrsTooBig):
		// tag 11 is NOT TriesToForgeADA in Shelley (unlike the
		// Alonzo+/baseMap numbering, where TriesToForgeAda legitimately
		// occupies tag 11), so it's deliberately excluded here too.
		// Allegra/Mary additionally define OutputTooBigUTxO at tag 12
		// (also not TriesToForgeADA at tag 11), so they cannot share
		// this map either; see the EraIdAllegra, EraIdMary case below.
		// Use an explicit map literal (rather than filtering baseMap
		// procedurally) so the set of valid Shelley tags is easy to
		// audit here.
		shelleyMap := map[int]any{
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
		}
		return shelleyMap, 0, 0, 0, 0, nil
	case EraIdAllegra, EraIdMary:
		// Allegra and Mary share Shelley's tags 0-10 but additionally
		// define OutputTooBigUTxO at tag 12 (tag 11 is not
		// TriesToForgeADA in Allegra/Mary either, so it's excluded the
		// same as in Shelley). Sharing Shelley's map here would report
		// a genuine Allegra/Mary tag 12 (OutputTooBigUTxO) as unknown.
		allegraMaryMap := map[int]any{
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
			UtxoFailureOutputTooBigUtxoAllegraMary: &OutputTooBigUtxo{},
		}
		return allegraMaryMap, UtxoFailureOutputTooBigUtxoAllegraMary, 0, 0, 0, nil
	default:
		// Byron and any future/unrecognized era: don't guess at another
		// era's constructor numbering. The caller surfaces this as an
		// UnknownUtxoFailureError instead of silently misdecoding using
		// Babbage's tag numbers.
		return nil, 0, 0, 0, 0, fmt.Errorf(
			"getEraSpecificUtxoFailureConstants: unrecognized era id %d",
			eraId,
		)
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

// UnknownApplyTxFailureError preserves the era, the raw LEDGER-level
// failure constructor tag, and the raw CBOR bytes for an ApplyTxError
// failure that this era-aware decoder does not recognize. It is returned
// instead of silently decoding the failure as a GenericError, which would
// otherwise discard the tag/era context needed to diagnose a
// forward-incompatible or malformed constructor.
type UnknownApplyTxFailureError struct {
	Era         uint8
	FailureType int
	Cbor        []byte
}

func (e *UnknownApplyTxFailureError) Error() string {
	return fmt.Sprintf(
		"UnknownApplyTxFailureError (Era %d, FailureType %d)",
		e.Era,
		e.FailureType,
	)
}

// UnknownUtxowFailureError preserves the era, the raw UTXOW failure
// constructor tag, and the raw CBOR bytes for a UTXOW failure that this
// era-aware decoder does not recognize. This covers both an unrecognized
// era (where we don't know the constructor numbering at all) and a known
// era with an unrecognized constructor tag. It is returned instead of
// silently guessing another era's constructor numbering (e.g. Babbage) or
// decoding as a GenericError, both of which lose the tag/era context
// needed to diagnose a forward-incompatible or malformed constructor.
type UnknownUtxowFailureError struct {
	Era         uint8
	FailureType int
	Cbor        []byte
}

func (e *UnknownUtxowFailureError) Error() string {
	return fmt.Sprintf(
		"UnknownUtxowFailureError (Era %d, FailureType %d)",
		e.Era,
		e.FailureType,
	)
}

// UnknownUtxoFailureError preserves the era, the raw UTXO failure
// constructor tag, and the raw CBOR bytes for a UTXO failure that this
// era-aware decoder does not recognize. This covers both an unrecognized
// era (where we don't know the constructor numbering at all) and a known
// era with an unrecognized constructor tag. It is returned instead of
// silently guessing another era's constructor numbering (e.g. Babbage) or
// decoding as a GenericError, both of which lose the tag/era context
// needed to diagnose a forward-incompatible or malformed constructor.
type UnknownUtxoFailureError struct {
	Era         uint8
	FailureType int
	Cbor        []byte
}

func (e *UnknownUtxoFailureError) Error() string {
	return fmt.Sprintf(
		"UnknownUtxoFailureError (Era %d, FailureType %d)",
		e.Era,
		e.FailureType,
	)
}

func NewEraMismatchErrorFromCbor(cborData []byte) (error, error) {
	newErr := &EraMismatch{}
	if _, err := cbor.Decode(cborData, newErr); err != nil {
		return nil, err
	}
	return newErr, nil
}

// EraInfo identifies one era in a Cardano EraMismatch by its position in
// the CardanoEras list and its canonical name. Wire-encoded as
// [eraIndex_uint8, eraName_text] — the N-ary-sum encoding produced by
// Haskell's encodeNS in
// Ouroboros.Consensus.HardFork.Combinator.Serialisation.Common.
//
// Indices follow CardanoEras position: 0=Byron, 1=Shelley, 2=Allegra,
// 3=Mary, 4=Alonzo, 5=Babbage, 6=Conway. Names match the strings emitted
// by singleEraName in the Haskell node ("Byron", "Shelley", "Allegra",
// "Mary", "Alonzo", "Babbage", "Conway").
type EraInfo struct {
	cbor.StructAsArray
	Index uint8
	Name  string
}

// EraMismatch is the canonical wire encoding of the Cardano HardFork
// Combinator's era-mismatch error. Wire format:
//
//	[2,                                 -- listLen 2 (matches encodeListLen 2)
//	 [2, otherEraIndex, "OtherEra"],    -- N-ary sum of the offered era
//	 [2, ledgerEraIndex, "LedgerEra"]]  -- N-ary sum of the ledger era
//
// Produced by Haskell's encodeEitherMismatch (Left case) for
// HardForkApplyTxErrWrongEra (in tx-submission MsgRejectTx Reason) and
// for QueryResultEraMismatch (in local-state-query results). The Right
// case (era-specific apply-tx-error or in-era query result) uses listLen
// 1 with a different payload and is handled by the era-specific decoders
// (e.g. ShelleyTxValidationError).
type EraMismatch struct {
	cbor.StructAsArray
	OtherEra  EraInfo // era of the offered tx/query/header
	LedgerEra EraInfo // era the ledger is currently in
}

func (e *EraMismatch) Error() string {
	return fmt.Sprintf(
		"The era of the node and the tx do not match. The node is running in the %s era, but the transaction is for the %s era.",
		e.LedgerEra.Name,
		e.OtherEra.Name,
	)
}

// MarshalCBOR encodes the EraMismatch as canonical wire bytes (see the
// type doc for the format). Defining this method makes *EraMismatch
// satisfy the localtxsubmission.CborRejectReason interface, so the
// tx-submission server places the structured CBOR directly in
// MsgRejectTx.Reason instead of falling back to encoding Error() as a
// CBOR text string.
//
// EncodeGeneric is used to avoid infinite recursion: cbor.Encode would
// otherwise re-enter MarshalCBOR.
func (e *EraMismatch) MarshalCBOR() ([]byte, error) {
	return cbor.EncodeGeneric(e)
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
		switch {
		case failureType == ApplyTxErrorUtxowFailure:
			// Use era-aware UTXOW failure decoding
			utxowErr := &UtxowFailure{era: e.era}
			if _, err := cbor.Decode(tmpFailure[1], utxowErr); err != nil {
				return err
			}
			newErr = utxowErr
		case isLedgerIncompleteWithdrawalsFailure(e.era, failureType):
			incorrectWithdrawals := &IncorrectWithdrawals{}
			if _, err := cbor.Decode(failure, incorrectWithdrawals); err != nil {
				return err
			}
			newErr = incorrectWithdrawals
		default:
			// Unrecognized LEDGER-level failure constructor: preserve
			// the era, tag, and raw bytes instead of silently decoding
			// as an opaque GenericError.
			newErr = &UnknownApplyTxFailureError{
				Era:         e.era,
				FailureType: failureType,
				Cbor:        failure,
			}
		}
		e.Failures = append(e.Failures, newErr)
	}
	return nil
}

func isLedgerIncompleteWithdrawalsFailure(era uint8, failureType int) bool {
	if era == EraIdConway {
		return failureType == ConwayLedgerIncompleteWithdrawals
	}
	return failureType == ShelleyLedgerIncompleteWithdrawals
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
		// Conway uses flat enumeration (no wrapping).
		return e.unmarshalConway(data, tmpFailure, failureType)
	case EraIdDijkstra:
		// Dijkstra's UTXOW predicate failure (DijkstraUtxowPredFailure,
		// cardano-ledger
		// eras/dijkstra/impl/src/Cardano/Ledger/Dijkstra/Rules/Utxow.hs)
		// shares Conway's flat enumeration for tags 0-18 unchanged, but
		// adds two Dijkstra-only constructors at tags 19 and 20 for
		// guarded-subtransaction validation that Conway doesn't have.
		// Falling back to unmarshalConway here would misreport those two
		// as UnknownUtxowFailureError, so Dijkstra gets its own decoder.
		return e.unmarshalDijkstra(data, tmpFailure, failureType)
	default:
		// Unknown era (Byron or a future era this decoder doesn't yet
		// know about): we don't know this era's UTXOW constructor
		// numbering, so don't guess by decoding as Babbage.
		e.Err = &UnknownUtxowFailureError{
			Era:         e.era,
			FailureType: failureType,
			Cbor:        data,
		}
		return nil
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
		e.Err = &UnknownUtxowFailureError{
			Era:         e.era,
			FailureType: failureType,
			Cbor:        data,
		}
		return nil
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
		e.Err = &UnknownUtxowFailureError{
			Era:         e.era,
			FailureType: failureType,
			Cbor:        data,
		}
		return nil
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
		e.Err = &UnknownUtxowFailureError{
			Era:         e.era,
			FailureType: failureType,
			Cbor:        data,
		}
		return nil
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
		e.Err = &UnknownUtxowFailureError{
			Era:         e.era,
			FailureType: failureType,
			Cbor:        data,
		}
		return nil
	}
	if len(tmpFailure) >= 2 {
		if _, err := cbor.Decode(tmpFailure[1], newErr); err != nil {
			return err
		}
	}
	e.Err = newErr
	return nil
}

// unmarshalDijkstra handles Dijkstra era UTXOW failures. Dijkstra's
// DijkstraUtxowPredFailure shares Conway's flat enumeration for tags 0-18
// unchanged (cardano-ledger
// eras/dijkstra/impl/src/Cardano/Ledger/Dijkstra/Rules/Utxow.hs), so those
// are delegated to unmarshalConway; tags 19 and 20 are Dijkstra-only
// additions for guarded-subtransaction validation that Conway doesn't have.
func (e *UtxowFailure) unmarshalDijkstra(
	data []byte,
	tmpFailure []cbor.RawMessage,
	failureType int,
) error {
	var newErr error
	switch failureType {
	case DijkstraUtxowMissingRequiredGuards:
		newErr = &MissingRequiredGuards{}
	case DijkstraUtxowMalformedGuardDatums:
		newErr = &MalformedGuardDatums{}
	default:
		// Tags 0-18 are shared with Conway's flat enumeration; any
		// failureType not handled above (including truly unknown tags)
		// falls through to unmarshalConway, whose default case already
		// produces UnknownUtxowFailureError with the correct era.
		return e.unmarshalConway(data, tmpFailure, failureType)
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

// MissingRequiredGuards represents guard credentials that subtransactions
// require but that are absent from the top-level guard set. Dijkstra-only
// (UTXOW constructor tag 19); introduced for guarded-subtransaction
// validation.
// Upstream: DijkstraUtxowPredFailure.MissingRequiredGuards
//
//	(NonEmptySet (Credential Guard))
//
// CBOR (constructor payload only, tag already stripped by the caller):
//
//	[credential, ...]
type MissingRequiredGuards struct {
	Guards []common.Credential
}

func (e *MissingRequiredGuards) UnmarshalCBOR(cborData []byte) error {
	if _, err := cbor.Decode(cborData, &e.Guards); err != nil {
		return err
	}
	return nil
}

func (e *MissingRequiredGuards) Error() string {
	var sb strings.Builder
	sb.WriteString("MissingRequiredGuards ([")
	for idx, cred := range e.Guards {
		sb.WriteString(cred.Credential.String())
		if idx < len(e.Guards)-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
}

// MalformedGuardDatums represents guard credentials whose datum presence in
// requiredTopLevelGuards is inconsistent. Dijkstra-only (UTXOW constructor
// tag 20); introduced for guarded-subtransaction validation.
// Upstream: DijkstraUtxowPredFailure.MalformedGuardDatums
//
//	(NonEmptySet (Credential Guard))
//
// CBOR (constructor payload only, tag already stripped by the caller):
//
//	[credential, ...]
type MalformedGuardDatums struct {
	Guards []common.Credential
}

func (e *MalformedGuardDatums) UnmarshalCBOR(cborData []byte) error {
	if _, err := cbor.Decode(cborData, &e.Guards); err != nil {
		return err
	}
	return nil
}

func (e *MalformedGuardDatums) Error() string {
	var sb strings.Builder
	sb.WriteString("MalformedGuardDatums ([")
	for idx, cred := range e.Guards {
		sb.WriteString(cred.Credential.String())
		if idx < len(e.Guards)-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("])")
	return sb.String()
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

	// Extract the raw constructor tag. A failure here means tmpData.Err
	// is structurally malformed (e.g. a CBOR string/map where a
	// constructor-tagged list was expected), which is a genuine decode
	// error, not an unknown-failure case: UnknownUtxoFailureError is
	// reserved for when tag extraction succeeds but the resulting tag
	// isn't present in the era's map. Propagate idErr instead of masking
	// it behind a placeholder tag.
	failureType, idErr := cbor.DecodeIdFromList(tmpData.Err)
	if idErr != nil {
		return fmt.Errorf(
			"UtxoFailure: failed to extract constructor tag: %w",
			idErr,
		)
	}

	errorMap, _, _, _, _, mapErr := getEraSpecificUtxoFailureConstants(
		tmpData.Era,
	)
	if mapErr != nil {
		// Unrecognized era: we don't know this era's constructor
		// numbering, so don't guess at another era's (e.g. Babbage's).
		// mapErr is deliberately not propagated: it's turned into a
		// typed UnknownUtxoFailureError value instead of a hard decode
		// failure.
		e.Err = &UnknownUtxoFailureError{
			Era:         tmpData.Era,
			FailureType: failureType,
			Cbor:        tmpData.Err,
		}
		return nil //nolint:nilerr
	}

	newErr, err := cbor.DecodeById(tmpData.Err, errorMap)
	if err != nil {
		if _, known := errorMap[failureType]; known {
			// Recognized constructor tag whose payload we couldn't
			// decode: a real decode failure, not an unknown-failure
			// case, so propagate it instead of masking it.
			return err
		}
		// Known era, unrecognized constructor tag: preserve the tag
		// instead of silently decoding as an opaque GenericError. err
		// is deliberately not propagated for the same reason as above.
		e.Err = &UnknownUtxoFailureError{
			Era:         tmpData.Era,
			FailureType: failureType,
			Cbor:        tmpData.Err,
		}
		return nil //nolint:nilerr
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

// IncorrectWithdrawals represents withdrawal amounts that do not exactly match
// their reward account balances.
// CBOR: [tag, {account_address: [supplied_withdrawal, expected_balance]}]
type IncorrectWithdrawals struct {
	cbor.StructAsArray
	Type        uint8
	Withdrawals cbor.Value
}

func (e *IncorrectWithdrawals) Error() string {
	return fmt.Sprintf(
		"IncorrectWithdrawals (Withdrawals %v)",
		e.Withdrawals.Value(),
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
// CBOR: [14, utxo_map] (Alonzo/Babbage), [13, utxo_map] (Conway),
// [12, utxo_map] (Dijkstra)
type ScriptsNotPaidUtxo struct {
	UtxoFailureErrorBase
	Utxos []common.Utxo // Each Utxo contains Id (input) and Output
}

func (e *ScriptsNotPaidUtxo) MarshalCBOR() ([]byte, error) {
	// The era-specific constructor index must be set explicitly by the
	// caller before marshaling. We used to default silently to Conway's
	// numbering when Type was unset, which would emit the wrong bytes
	// for Alonzo/Babbage callers that forgot to set it.
	if e.Type == 0 {
		return nil, errors.New(
			"ScriptsNotPaidUtxo: Type (era-specific constructor index) " +
				"must be set explicitly before marshaling; use one of " +
				"UtxoFailureScriptsNotPaidUtxoAlonzo/Babbage/Conway",
		)
	}
	validConstructors := []int{
		UtxoFailureScriptsNotPaidUtxoAlonzo,
		UtxoFailureScriptsNotPaidUtxoBabbage,
		UtxoFailureScriptsNotPaidUtxoConway,
		DijkstraUtxoScriptsNotPaidUTxO,
	}
	isValid := false
	for _, valid := range validConstructors {
		if int(e.Type) == valid {
			isValid = true
			break
		}
	}
	if !isValid {
		return nil, fmt.Errorf(
			"ScriptsNotPaidUtxo: invalid constructor index %d, expected one of %v",
			e.Type,
			validConstructors,
		)
	}
	constantToUse := int(e.Type)

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
		DijkstraUtxoScriptsNotPaidUTxO,
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
// CBOR: [16, provided] (Alonzo/Babbage), [15, provided] (Conway),
// [14, provided] (Dijkstra)
type CollateralContainsNonADA struct {
	UtxoFailureErrorBase
	Provided cbor.Value
}

func (e *CollateralContainsNonADA) MarshalCBOR() ([]byte, error) {
	// The era-specific constructor index must be set explicitly by the
	// caller before marshaling. We used to default silently to Conway's
	// numbering when Type was unset, which would emit the wrong bytes
	// for Alonzo/Babbage callers that forgot to set it.
	if e.Type == 0 {
		return nil, errors.New(
			"CollateralContainsNonADA: Type (era-specific constructor " +
				"index) must be set explicitly before marshaling; use " +
				"one of UtxoFailureCollateralContainsNonAdaAlonzo/" +
				"Babbage/Conway",
		)
	}
	validConstructors := []int{
		UtxoFailureCollateralContainsNonAdaAlonzo,
		UtxoFailureCollateralContainsNonAdaBabbage,
		UtxoFailureCollateralContainsNonAdaConway,
		DijkstraUtxoCollateralContainsNonADA,
	}
	isValid := false
	for _, valid := range validConstructors {
		if int(e.Type) == valid {
			isValid = true
			break
		}
	}
	if !isValid {
		return nil, fmt.Errorf(
			"CollateralContainsNonADA: invalid constructor index %d, expected one of %v",
			e.Type,
			validConstructors,
		)
	}
	constantToUse := int(e.Type)
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
		DijkstraUtxoCollateralContainsNonADA,
	}
	isValid := false
	for _, valid := range validConstructors {
		//nolint:gosec // G115: integer overflow conversion int -> uint64
		// Safe conversion since constants are small positive values (14, 15, 16)
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
		fmt.Fprintf(&sb, "(Output %s, MinRequired %d)",
			entry.Output.String(), entry.MinRequired)
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
// Dijkstra-only UTXO Predicate Failures
// =============================================================================

// PtrPresentInCollateralReturn represents the Dijkstra-only
// PtrPresentInCollateralReturn error from cardano-ledger: the collateral
// return output uses a pointer-address stake reference, which Dijkstra
// disallows.
// CBOR: [22, txout]
type PtrPresentInCollateralReturn struct {
	UtxoFailureErrorBase
	Output TxOut
}

func (e *PtrPresentInCollateralReturn) Error() string {
	return fmt.Sprintf(
		"PtrPresentInCollateralReturn (Output %s)",
		e.Output.String(),
	)
}

// WithdrawalsExceedAccountBalance represents the Dijkstra-only
// WithdrawalsExceedAccountBalance error from cardano-ledger: total
// withdrawals for one or more accounts across a transaction batch exceed
// the account's original balance. The Haskell field is
// NonEmptyMap AccountAddress (Mismatch RelLTEQ Coin); rather than assume
// the wire-level encoding of AccountAddress (not otherwise represented in
// this codebase), the map is preserved as a generic cbor.Value, mirroring
// the same approach already used for IncorrectWithdrawals and
// WrongNetworkWithdrawal above.
// CBOR: [24, {account_address: [supplied, expected], ...}]
type WithdrawalsExceedAccountBalance struct {
	UtxoFailureErrorBase
	Withdrawals cbor.Value
}

func (e *WithdrawalsExceedAccountBalance) Error() string {
	return fmt.Sprintf(
		"WithdrawalsExceedAccountBalance (Withdrawals %v)",
		e.Withdrawals.Value(),
	)
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
		fmt.Fprintf(&sb, "(Purpose %v, ScriptHash %s)",
			entry.Purpose.Value(), entry.ScriptHash.String())
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
		fmt.Fprintf(&sb, "(%s, Index %d)", tagName, entry.Index)
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
		e.Err = &UnknownUtxowFailureError{
			Era:         EraIdShelley,
			FailureType: failureType,
			Cbor:        data,
		}
		return nil
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
		e.Err = &UnknownUtxowFailureError{
			Era:         EraIdAlonzo,
			FailureType: failureType,
			Cbor:        data,
		}
		return nil
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
		e.Err = &UnknownUtxoFailureError{
			Era:         EraIdBabbage,
			FailureType: failureType,
			Cbor:        data,
		}
		return nil
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
		e.Err = &UnknownUtxowFailureError{
			Era:         EraIdConway,
			FailureType: failureType,
			Cbor:        data,
		}
		return nil
	}
	if len(tmpFailure) >= 2 {
		if _, err := cbor.Decode(tmpFailure[1], newErr); err != nil {
			return err
		}
	}
	e.Err = newErr
	return nil
}
