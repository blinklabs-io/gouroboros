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

// Metadata validation errors are in the common package; use directly

// Witness validation errors
type MissingVKeyWitnessesError struct{}

func (MissingVKeyWitnessesError) Error() string {
	return "missing required vkey witnesses"
}

type MissingRequiredVKeyWitnessForSignerError struct {
	Signer common.Blake2b224
}

func (e MissingRequiredVKeyWitnessForSignerError) Error() string {
	return "missing required vkey witness for required signer"
}

// Plutus script/redeemer relationship errors
type MissingRedeemersForScriptDataHashError struct{}

func (MissingRedeemersForScriptDataHashError) Error() string {
	return "missing redeemers for script data hash"
}

type MissingPlutusScriptWitnessesError struct{}

func (MissingPlutusScriptWitnessesError) Error() string {
	return "missing Plutus script witnesses for redeemers"
}

type ExtraneousPlutusScriptWitnessesError struct{}

func (ExtraneousPlutusScriptWitnessesError) Error() string {
	return "extraneous Plutus script witnesses"
}

// Cost model and IsValid flag errors moved to ledger/common/era_errors.go

// Reference input resolution errors moved to ledger/common/reference_errors.go
