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

package alonzo

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

type ExUnitsTooBigUtxoError struct {
	TotalExUnits common.ExUnits
	MaxTxExUnits common.ExUnits
}

func (e ExUnitsTooBigUtxoError) Error() string {
	return fmt.Sprintf(
		"ExUnits too big: total %d/%d steps/memory, maximum %d/%d steps/memory",
		e.TotalExUnits.Steps,
		e.TotalExUnits.Memory,
		e.MaxTxExUnits.Steps,
		e.MaxTxExUnits.Memory,
	)
}

type InsufficientCollateralError struct {
	Provided uint64
	Required uint64
}

func (e InsufficientCollateralError) Error() string {
	return fmt.Sprintf(
		"insufficient collateral: provided %d, required %d",
		e.Provided,
		e.Required,
	)
}

type CollateralContainsNonAdaError struct {
	Provided uint64
}

func (e CollateralContainsNonAdaError) Error() string {
	return fmt.Sprintf(
		"collateral contains non-ADA: provided %d",
		e.Provided,
	)
}

type NoCollateralInputsError struct{}

func (NoCollateralInputsError) Error() string {
	return "no collateral inputs"
}
