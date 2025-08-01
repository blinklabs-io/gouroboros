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

package common_test

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

func TestGenesisRatNumDenom(t *testing.T) {
	jsonData := `{"testRat": { "numerator": 721, "denominator": 10000 }}`
	expectedRat := big.NewRat(721, 10000)
	var testData struct {
		TestRat common.GenesisRat `json:"testRat"`
	}
	if err := json.Unmarshal([]byte(jsonData), &testData); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if testData.TestRat.Cmp(expectedRat) != 0 {
		t.Errorf("did not get expected value: got %s, wanted %s", testData.TestRat.String(), expectedRat.String())
	}
}

func TestGenesisRatFloat(t *testing.T) {
	jsonData := `{"testRat": 0.0721}`
	expectedRat := big.NewRat(721, 10000)
	var testData struct {
		TestRat common.GenesisRat `json:"testRat"`
	}
	if err := json.Unmarshal([]byte(jsonData), &testData); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if testData.TestRat.Cmp(expectedRat) != 0 {
		t.Errorf("did not get expected value: got %s, wanted %s", testData.TestRat.String(), expectedRat.String())
	}
}
