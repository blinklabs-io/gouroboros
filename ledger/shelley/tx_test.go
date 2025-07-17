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

package shelley_test

import (
	"encoding/hex"
	"math/big"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/pkg/data"
)

func TestShelleyTransactionInputToPlutusData(t *testing.T) {
	testTxIdHex := "1639f61ed08f5e489dd64db20f86451a0db06e83d21ea39c73ea0a93b478a370"
	testTxOutputIdx := 2
	testInput := shelley.NewShelleyTransactionInput(
		testTxIdHex,
		testTxOutputIdx,
	)
	testTxId, err := hex.DecodeString(testTxIdHex)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	expectedData := data.NewConstr(
		0,
		data.NewByteString(testTxId),
		data.NewInteger(big.NewInt(int64(testTxOutputIdx))),
	)
	tmpData := testInput.ToPlutusData()
	if !reflect.DeepEqual(tmpData, expectedData) {
		t.Fatalf("did not get expected PlutusData\n     got: %#v\n  wanted: %#v", tmpData, expectedData)
	}
}
