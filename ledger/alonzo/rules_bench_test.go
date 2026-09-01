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
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
)

func BenchmarkUtxoValidateValueNotConservedUtxo_MultiAsset(b *testing.B) {
	const addr = "addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd"

	var inputAssets []mockledger.Asset
	var outputAssets []mockledger.Asset
	for policyIdx := range 8 {
		policyID := make([]byte, 28)
		policyID[27] = byte(policyIdx + 1)
		for assetIdx := range 16 {
			asset := mockledger.Asset{
				PolicyId:  policyID,
				AssetName: []byte{byte(policyIdx), byte(assetIdx)},
				Amount:    uint64((policyIdx + 1) * (assetIdx + 2)),
			}
			inputAssets = append(inputAssets, asset)
			outputAssets = append(outputAssets, asset)
		}
	}

	input, err := mockledger.NewTransactionInputBuilder().WithTxId([]byte{0xaa}).WithIndex(0).Build()
	if err != nil {
		b.Fatal(err)
	}
	utxo, err := mockledger.NewUtxoBuilder().WithTxId([]byte{0xaa}).WithIndex(0).WithAddress(addr).WithLovelace(2_000_000).WithAssets(inputAssets...).Build()
	if err != nil {
		b.Fatal(err)
	}
	output, err := mockledger.NewTransactionOutputBuilder().WithAddress(addr).WithLovelace(1_900_000).WithAssets(outputAssets...).Build()
	if err != nil {
		b.Fatal(err)
	}
	tx, err := mockledger.NewTransactionBuilder().WithInputs(input).WithOutputs(output).WithFee(100_000).Build()
	if err != nil {
		b.Fatal(err)
	}
	ls := mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{utxo}).Build()
	pp := &alonzo.AlonzoProtocolParameters{}

	if err := alonzo.UtxoValidateValueNotConservedUtxo(tx, 0, ls, pp); err != nil {
		b.Fatalf("precheck failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := alonzo.UtxoValidateValueNotConservedUtxo(tx, 0, ls, pp); err != nil {
			b.Fatal(err)
		}
	}
}
