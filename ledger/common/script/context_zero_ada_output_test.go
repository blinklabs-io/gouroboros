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

package script_test

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/data"
)

// TestTxOutPlutusDataKeepsZeroAda checks the value of a PlutusV2/V3 TxOut
// whose coin is zero. cardano-ledger translates an output value with
// transValue (MaryValue c m) = transCoinToValue c <> transMultiAsset m, where
// transCoinToValue (Coin c) = PV1.singleton adaSymbol adaToken c
// (libs/cardano-ledger-core/src/Cardano/Ledger/Plutus/TxInfo.hs and
// eras/alonzo/impl/src/Cardano/Ledger/Alonzo/Plutus/TxInfo.hs). The singleton
// is built for any amount, so the ada entry is present even at zero, and the
// transaction outputs and resolved inputs of a Babbage or Conway TxInfo carry
// it. The cardano-ledger Conway translation golden records such outputs.
func TestTxOutPlutusDataKeepsZeroAda(t *testing.T) {
	addr, err := lcommon.NewAddress(
		"addr_test1vqg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zygxrcya6",
	)
	if err != nil {
		t.Fatal(err)
	}
	outputs := map[string]lcommon.TransactionOutput{
		"shelley": &shelley.ShelleyTransactionOutput{OutputAddress: addr},
		"mary":    &mary.MaryTransactionOutput{OutputAddress: addr},
		"alonzo":  &alonzo.AlonzoTransactionOutput{OutputAddress: addr},
		"babbage": &babbage.BabbageTransactionOutput{OutputAddress: addr},
	}
	wantValue := data.NewMap([][2]data.PlutusData{{
		data.NewByteString(nil),
		data.NewMap([][2]data.PlutusData{{
			data.NewByteString(nil),
			data.NewInteger(big.NewInt(0)),
		}}),
	}})
	for name, output := range outputs {
		t.Run(name, func(t *testing.T) {
			txOut, ok := output.ToPlutusData().(*data.Constr)
			if !ok || len(txOut.Fields()) < 2 {
				t.Fatalf("TxOut is not a constructor with a value: %s", output.ToPlutusData())
			}
			if got := txOut.Fields()[1]; !got.Equal(wantValue) {
				t.Fatalf("zero-coin value:\n got %s\nwant %s", got, wantValue)
			}
		})
	}
}
