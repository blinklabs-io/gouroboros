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
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

const benchmarkKeyAddress = "addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp" +
	"777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd"

// Store the latest result outside the benchmark loop so the compiler cannot
// remove the NewTxScriptView call as unused work.
var benchmarkTxScriptView script.TxScriptView

func BenchmarkNewTxScriptView(b *testing.B) {
	// Compare the common no-script path with one and several script spends.
	tests := []struct {
		name         string
		scriptSpends int
	}{
		{name: "script_free", scriptSpends: 0},
		{name: "single_script_spend", scriptSpends: 1},
		{name: "several_purposes", scriptSpends: 8},
	}
	for _, tc := range tests {
		b.Run(tc.name, func(b *testing.B) {
			// Build the transaction and ledger state before timing starts.
			tx, ls := newTxScriptViewBenchmarkFixture(b, tc.scriptSpends)

			// Run once outside the benchmark to verify that the fixture has the
			// expected number of scripts in Needed.
			view, err := script.NewTxScriptView(tx, ls)
			require.NoError(b, err)
			require.Len(b, view.Needed, tc.scriptSpends)

			// Include memory allocations in the benchmark output. ResetTimer
			// excludes fixture creation and the correctness check above.
			b.ReportAllocs()
			b.ResetTimer()

			// b.Loop chooses an iteration count that produces a stable timing.
			for b.Loop() {
				view, err = script.NewTxScriptView(tx, ls)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkTxScriptView = view
			}
		})
	}
}

func newTxScriptViewBenchmarkFixture(
	b *testing.B,
	scriptSpends int,
) (common.Transaction, common.LedgerState) {
	b.Helper()

	// The script-free case still needs one ordinary input. Script cases use
	// one input for every script-spending purpose.
	inputCount := max(scriptSpends, 1)
	inputs := make([]common.TransactionInput, 0, inputCount)
	utxos := make([]common.Utxo, 0, inputCount)
	scripts := make([]common.PlutusV1Script, 0, scriptSpends)
	// Every script address also needs a staking credential. Its value does not
	// affect this benchmark, so all fixtures share this deterministic hash.
	stakingHash := bytes.Repeat([]byte{0x55}, common.AddressHashSize)

	for i := range inputCount {
		// Give each input a distinct transaction ID so input resolution and
		// sorting see separate UTxOs.
		txId := bytes.Repeat([]byte{byte(i + 1)}, len(common.Blake2b256{}))
		address := benchmarkKeyAddress
		if i < scriptSpends {
			// Create a distinct Plutus script and lock this input at an address
			// derived from that script's hash. This makes the script needed when
			// NewTxScriptView examines the spending purpose.
			plutusScript := common.PlutusV1Script{byte(i + 1), 0x02}
			scripts = append(scripts, plutusScript)
			scriptAddress, err := common.NewAddressFromParts(
				common.AddressTypeScriptKey,
				common.AddressNetworkMainnet,
				plutusScript.Hash().Bytes(),
				stakingHash,
			)
			require.NoError(b, err)
			address = scriptAddress.String()
		}

		// Add the output to the ledger state and its ID to the transaction.
		// NewTxScriptView will resolve this ID back to the complete UTxO.
		utxo, err := mockledger.NewUtxoBuilder().
			WithTxId(txId).
			WithIndex(0).
			WithAddress(address).
			WithLovelace(2_000_000).
			Build()
		require.NoError(b, err)
		inputs = append(inputs, utxo.Id)
		utxos = append(utxos, utxo)
	}

	// The mock transaction builder requires an output. It is unrelated to
	// script discovery, so one ordinary key-address output is sufficient.
	output, err := mockledger.NewTransactionOutputBuilder().
		WithAddress(benchmarkKeyAddress).
		WithLovelace(1_000_000).
		Build()
	require.NoError(b, err)
	txBuilder := mockledger.NewTransactionBuilder()
	txBuilder.WithInputs(inputs...)
	txBuilder.WithOutputs(output)
	// Put every Plutus script in the witness set, making it available to the
	// transaction. The script-free case supplies an empty slice.
	txBuilder.WithWitnesses(
		mockledger.NewMockTransactionWitnessSet().
			WithPlutusV1Scripts(scripts...),
	)
	tx, err := txBuilder.Build()
	require.NoError(b, err)

	// The ledger state supplies the UTxOs that NewTxScriptView must resolve.
	ls := mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build()
	return tx, ls
}
