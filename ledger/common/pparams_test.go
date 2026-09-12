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

package common

import (
	"reflect"
	"testing"

	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

// TestConvertToUtxorpcCardanoCostModels_Mapping pins the real cardano-ledger
// wire convention for the cost-models map: 0-indexed language keys
// (0=PlutusV1, 1=PlutusV2, 2=PlutusV3, 3=PlutusV4). Key 0 is exercised
// explicitly since it is the most common real-world value (PlutusV1) and was
// previously dropped by an off-by-one mapping. See the same fixture values
// used against a real on-chain transaction in
// ledger/babbage/script_data_hash_used_languages_test.go (cost models keyed
// 0 and 1 for PlutusV1/PlutusV2).
func TestConvertToUtxorpcCardanoCostModels_Mapping(t *testing.T) {
	models := map[uint][]int64{
		0:  {10, 20},
		1:  {30},
		2:  {40, 50, 60},
		3:  {70, 80},
		99: {999}, // unsupported, should be ignored
	}

	cm := ConvertToUtxorpcCardanoCostModels(models)
	if cm == nil {
		t.Fatal("expected non-nil CostModels")
	}
	if cm.PlutusV1 == nil ||
		!reflect.DeepEqual(cm.PlutusV1.Values, []int64{10, 20}) {
		t.Fatalf("PlutusV1 not mapped correctly: %+v", cm.PlutusV1)
	}
	if cm.PlutusV2 == nil ||
		!reflect.DeepEqual(cm.PlutusV2.Values, []int64{30}) {
		t.Fatalf("PlutusV2 not mapped correctly: %+v", cm.PlutusV2)
	}
	if cm.PlutusV3 == nil ||
		!reflect.DeepEqual(cm.PlutusV3.Values, []int64{40, 50, 60}) {
		t.Fatalf("PlutusV3 not mapped correctly: %+v", cm.PlutusV3)
	}
	if cm.PlutusV4 == nil ||
		!reflect.DeepEqual(cm.PlutusV4.Values, []int64{70, 80}) {
		t.Fatalf("PlutusV4 not mapped correctly: %+v", cm.PlutusV4)
	}
}

// TestConvertToUtxorpcCardanoCostModels_KeyZeroIsPlutusV1 is a focused
// regression test for the specific bug found by the node-parity audit: cost
// model key 0 (real-world PlutusV1) must not be silently dropped. Before the
// fix, the switch only handled keys 1/2/3 and key 0 fell into the "default"
// branch, logging "unsupported cost model version" and omitting PlutusV1
// entirely from every comparison.
func TestConvertToUtxorpcCardanoCostModels_KeyZeroIsPlutusV1(t *testing.T) {
	models := map[uint][]int64{
		0: {197209, 0},
	}
	cm := ConvertToUtxorpcCardanoCostModels(models)
	if cm.PlutusV1 == nil ||
		!reflect.DeepEqual(cm.PlutusV1.Values, []int64{197209, 0}) {
		t.Fatalf("key 0 not mapped to PlutusV1: %+v", cm.PlutusV1)
	}
	if cm.PlutusV2 != nil || cm.PlutusV3 != nil || cm.PlutusV4 != nil {
		t.Fatalf(
			"expected only PlutusV1 to be populated: %+v",
			cm,
		)
	}
}

func TestConvertToUtxorpcCardanoCostModels_Empty(t *testing.T) {
	cm := ConvertToUtxorpcCardanoCostModels(map[uint][]int64{})
	if cm == nil {
		t.Fatal("expected non-nil CostModels")
	}
	if cm.PlutusV1 != nil || cm.PlutusV2 != nil || cm.PlutusV3 != nil ||
		cm.PlutusV4 != nil {
		t.Fatalf("expected all nil cost model fields for empty input: %+v", cm)
	}
	// ensure it is a *cardano.CostModels
	if _, ok := any(cm).(*cardano.CostModels); !ok {
		t.Fatalf("expected *cardano.CostModels, got %T", cm)
	}
}
