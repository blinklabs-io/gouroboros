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

package alonzo_test

import (
	"slices"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// TestAlonzoCostModelUpdateLanguagesAsReference checks which cost models an
// Alonzo-era (protocol versions 5 and 6) parameter update may carry.
// cardano-ledger 1210d37f decodes the update's cost models with the same
// decodeCostModelsFailing as every pre-version-9 era
// (libs/cardano-ledger-core/src/Cardano/Ledger/Plutus/CostModels.hs:307-338):
// each key decodes as any Language (decodeEnumBounded, Plutus/Language.hs:
// 332-333) and decodeCostModelLegacy accepts PlutusV1 and PlutusV2 with
// exactly their initial parameter counts and refuses later languages. A
// PlutusV2 model is therefore accepted in Alonzo, as in Babbage.
func TestAlonzoCostModelUpdateLanguagesAsReference(t *testing.T) {
	model := func(language uint) []int64 {
		count, ok := alonzo.PlutusCostModelParameterCount(language)
		if !ok {
			count = 10
		}
		return slices.Repeat([]int64{1}, count)
	}
	cases := []struct {
		name   string
		models map[uint][]int64
		ok     bool
	}{
		{"PlutusV1", map[uint][]int64{0: model(0)}, true},
		{"PlutusV2", map[uint][]int64{1: model(1)}, true},
		{"PlutusV1 and PlutusV2", map[uint][]int64{
			0: model(0), 1: model(1),
		}, true},
		{"PlutusV2 one short", map[uint][]int64{1: model(1)[1:]}, false},
		{"PlutusV3", map[uint][]int64{2: model(2)}, false},
	}
	for _, tc := range cases {
		for _, major := range []uint{5, 6} {
			update := alonzo.AlonzoProtocolParameterUpdate{
				CostModels: tc.models,
			}
			err := update.ValidateProtocolParameterUpdateVersion(
				common.ProtocolParametersProtocolVersion{Major: major},
			)
			if got := err == nil; got != tc.ok {
				t.Errorf("%s/pv%d: accepted = %v, reference = %v (err %v)",
					tc.name, major, got, tc.ok, err)
			}
		}
	}
}
