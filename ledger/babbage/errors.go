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

package babbage

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

type TooManyCollateralInputsError struct {
	Provided uint
	Max      uint
}

func (e TooManyCollateralInputsError) Error() string {
	return fmt.Sprintf(
		"too many collateral inputs: provided %d, maximum %d",
		e.Provided,
		e.Max,
	)
}

type IncorrectTotalCollateralFieldError struct {
	Provided        uint64
	TotalCollateral uint64
}

func (e IncorrectTotalCollateralFieldError) Error() string {
	return fmt.Sprintf(
		"incorrect total collateral field: provided %d, total collateral %d",
		e.Provided,
		e.TotalCollateral,
	)
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
