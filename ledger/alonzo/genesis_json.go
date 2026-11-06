// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package alonzo

import (
	"encoding/json"

	gj "github.com/blinklabs-io/gouroboros/internal/genesisjson"
)

// validateAlonzoGenesisJSON checks an Alonzo genesis document against the
// FromJSON AlonzoGenesis instance in cardano-ledger
// (eras/alonzo/impl/src/Cardano/Ledger/Alonzo/Genesis.hs): every key except
// extraConfig is required and unknown keys are ignored. Value ranges follow
// the types of UpgradeAlonzoPParams (maxValueSize Word32, collateralPercentage
// and maxCollateralInputs Word16), Prices (NonNegativeInterval) and ExUnits
// (Word64, Plutus/ExUnits.hs).
func validateAlonzoGenesisJSON(data []byte) (json.RawMessage, error) {
	exUnits := gj.Object(
		gj.RequiredAnyOf(gj.Uint(64), "memory", "exUnitsMem"),
		gj.RequiredAnyOf(gj.Uint(64), "steps", "exUnitsSteps"),
	)
	return gj.Normalize(
		"",
		data,
		gj.Required("lovelacePerUTxOWord", gj.Uint(64)),
		// parseCostModels fails when PlutusV1 is not supplied.
		gj.Required("costModels", gj.Object(
			gj.Required("PlutusV1", gj.ArrayOrObject()),
		)),
		gj.Required("executionPrices", gj.Object(
			gj.RequiredAnyOf(
				gj.Interval(gj.NonNegativeInterval),
				"priceSteps",
				"prSteps",
			),
			gj.RequiredAnyOf(
				gj.Interval(gj.NonNegativeInterval),
				"priceMemory",
				"prMem",
			),
		)),
		gj.Required("maxTxExUnits", exUnits),
		gj.Required("maxBlockExUnits", exUnits),
		gj.Required("maxValueSize", gj.Uint(32)),
		gj.Required("collateralPercentage", gj.Uint(16)),
		gj.Required("maxCollateralInputs", gj.Uint(16)),
		gj.Optional("extraConfig", gj.Object(
			gj.Optional("costModels", gj.AnyObject()),
		)),
	)
}
