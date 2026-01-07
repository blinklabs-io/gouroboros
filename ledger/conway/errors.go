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

package conway

import (
	"fmt"
	"strings"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

type NonDisjointRefInputsError struct {
	Inputs []common.TransactionInput
}

func (e NonDisjointRefInputsError) Error() string {
	tmpInputs := make([]string, len(e.Inputs))
	for idx, tmpInput := range e.Inputs {
		tmpInputs[idx] = tmpInput.String()
	}
	return "non-disjoint reference inputs: " + strings.Join(tmpInputs, ", ")
}

// Witness validation errors (alias to common types)
type MissingVKeyWitnessesError = common.MissingVKeyWitnessesError

type MissingRequiredVKeyWitnessForSignerError = common.MissingRequiredVKeyWitnessForSignerError

type MissingRedeemersForScriptDataHashError = common.MissingRedeemersForScriptDataHashError

type MissingPlutusScriptWitnessesError = common.MissingPlutusScriptWitnessesError

type ExtraneousPlutusScriptWitnessesError = common.ExtraneousPlutusScriptWitnessesError

// Metadata / cost model / IsValid aliases
type MissingTransactionMetadataError = common.MissingTransactionMetadataError

type (
	MissingTransactionAuxiliaryDataHashError = common.MissingTransactionAuxiliaryDataHashError
	ConflictingMetadataHashError             = common.ConflictingMetadataHashError
	MissingCostModelError                    = common.MissingCostModelError
	InvalidIsValidFlagError                  = common.InvalidIsValidFlagError
)

type WrongTransactionNetworkIdError struct {
	TxNetworkId     uint8
	LedgerNetworkId uint
}

func (e WrongTransactionNetworkIdError) Error() string {
	return fmt.Sprintf("wrong transaction network ID: transaction has %d, ledger expects %d", e.TxNetworkId, e.LedgerNetworkId)
}

type TreasuryDonationWithPlutusV1V2Error struct {
	Donation      uint64
	PlutusVersion string
}

func (e TreasuryDonationWithPlutusV1V2Error) Error() string {
	return fmt.Sprintf("treasury donation (%d lovelace) cannot be used with %s scripts - treasury donation is a Conway feature only available for PlutusV3", e.Donation, e.PlutusVersion)
}

// PlutusScriptFailedError indicates that a Plutus script execution failed
type PlutusScriptFailedError struct {
	ScriptHash common.ScriptHash
	Tag        common.RedeemerTag
	Index      uint32
	Err        error
}

func (e PlutusScriptFailedError) Error() string {
	return fmt.Sprintf("plutus script failed (hash=%x, tag=%d, index=%d): %v", e.ScriptHash[:], e.Tag, e.Index, e.Err)
}

func (e PlutusScriptFailedError) Unwrap() error {
	return e.Err
}

// ScriptContextConstructionError indicates that the script context could not be built
type ScriptContextConstructionError struct {
	Err error
}

func (e ScriptContextConstructionError) Error() string {
	return fmt.Sprintf("failed to construct script context: %v", e.Err)
}

func (e ScriptContextConstructionError) Unwrap() error {
	return e.Err
}

// MissingDatumForSpendingScriptError indicates that a spending script requires a datum but none was provided
type MissingDatumForSpendingScriptError struct {
	ScriptHash common.ScriptHash
	Input      common.TransactionInput
}

func (e MissingDatumForSpendingScriptError) Error() string {
	return fmt.Sprintf("missing datum for spending script (hash=%x, input=%s)", e.ScriptHash[:], e.Input.String())
}
