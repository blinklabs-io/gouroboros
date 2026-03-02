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
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
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

// NonDisjointRefInputsError indicates reference inputs overlap with regular inputs
type NonDisjointRefInputsError struct {
	Inputs []common.TransactionInput
}

func (e NonDisjointRefInputsError) Error() string {
	return fmt.Sprintf(
		"non-disjoint reference inputs: %d common inputs",
		len(e.Inputs),
	)
}

// Delegation errors (alias to shelley types)
type (
	DelegateToUnregisteredPoolError              = shelley.DelegateToUnregisteredPoolError
	DelegateUnregisteredStakeCredentialError     = shelley.DelegateUnregisteredStakeCredentialError
	WithdrawalFromUnregisteredRewardAccountError = shelley.WithdrawalFromUnregisteredRewardAccountError
)
