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
	"reflect"
	"testing"

	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

func TestConvertToUtxorpcCardanoCostModels_Mapping(t *testing.T) {
	models := map[uint][]int64{
		1:  {10, 20},
		2:  {30},
		3:  {40, 50, 60},
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
}

func TestConvertToUtxorpcCardanoCostModels_Empty(t *testing.T) {
	cm := ConvertToUtxorpcCardanoCostModels(map[uint][]int64{})
	if cm == nil {
		t.Fatal("expected non-nil CostModels")
	}
	if cm.PlutusV1 != nil || cm.PlutusV2 != nil || cm.PlutusV3 != nil {
		t.Fatalf("expected all nil cost model fields for empty input: %+v", cm)
	}
	// ensure it is a *cardano.CostModels
	if _, ok := any(cm).(*cardano.CostModels); !ok {
		t.Fatalf("expected *cardano.CostModels, got %T", cm)
	}
}
