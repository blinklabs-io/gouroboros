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

package common

import (
	"errors"
	"fmt"
)

// InvalidIsValidFlagError indicates a tx marked invalid but lacking Plutus scripts
type InvalidIsValidFlagError struct{}

func (InvalidIsValidFlagError) Error() string {
	return "transaction marked as invalid but has no Plutus scripts requiring phase-2 validation"
}

// MissingCostModelError indicates a missing cost model for a Plutus version
type MissingCostModelError struct {
	Version uint
}

func (e MissingCostModelError) Error() string {
	return fmt.Sprintf("missing cost model for Plutus v%d", e.Version+1)
}

// ReferenceInputResolutionError indicates a failure to resolve a reference input UTxO
type ReferenceInputResolutionError struct {
	Input TransactionInput
	Err   error
}

func (e ReferenceInputResolutionError) Error() string {
	return fmt.Sprintf(
		"failed to resolve reference input %s: %v",
		e.Input.String(),
		e.Err,
	)
}

func (e ReferenceInputResolutionError) Unwrap() error { return e.Err }

// Sentinel error for reference input resolution failures so callers can use errors.Is
var ErrReferenceInputResolution = errors.New(
	"reference input resolution failed",
)

func (ReferenceInputResolutionError) Is(target error) bool {
	return target == ErrReferenceInputResolution
}
