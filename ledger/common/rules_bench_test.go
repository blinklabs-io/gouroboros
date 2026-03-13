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

package common_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

var benchmarkSink any

func BenchmarkEncodeLangViews(b *testing.B) {
	usedVersions := map[uint]struct{}{0: {}, 1: {}, 2: {}}
	costModels := map[uint][]int64{
		0: make([]int64, 166),
		1: make([]int64, 175),
		2: make([]int64, 251),
	}
	for version, model := range costModels {
		for i := range model {
			model[i] = int64(version*1000 + uint(i))
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoded, err := common.EncodeLangViews(usedVersions, costModels)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkSink = encoded
	}
}

// benchMarshalNativeScript encodes a native script struct to CBOR and
// unmarshals it back into a NativeScript with stored CBOR bytes so that
// Hash() works correctly.
func benchMarshalNativeScript(b *testing.B, script any) common.NativeScript {
	b.Helper()
	data, err := cbor.Encode(script)
	require.NoError(b, err, "encode native script")
	var ns common.NativeScript
	require.NoError(b, ns.UnmarshalCBOR(data), "unmarshal native script")
	return ns
}

func BenchmarkValidateScriptWitnesses(b *testing.B) {
	b.Run("no_scripts", benchValidateScriptWitnessesNoScripts)
	b.Run("with_scripts", benchValidateScriptWitnessesWithScripts)
}

// benchValidateScriptWitnessesNoScripts benchmarks the fast path where all
// inputs use key-based addresses, so no script hashes are required.
func benchValidateScriptWitnessesNoScripts(b *testing.B) {
	const addr = "addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd"

	inputs := make([]common.TransactionInput, 0, 64)
	utxos := make([]common.Utxo, 0, 64)
	for i := range 64 {
		txID := []byte{byte(i + 1)}
		input, err := mockledger.NewTransactionInputBuilder().WithTxId(txID).WithIndex(0).Build()
		require.NoError(b, err)
		utxo, err := mockledger.NewUtxoBuilder().WithTxId(txID).WithIndex(0).WithAddress(addr).WithLovelace(2_000_000).Build()
		require.NoError(b, err)
		inputs = append(inputs, input)
		utxos = append(utxos, utxo)
	}
	output, err := mockledger.NewTransactionOutputBuilder().WithAddress(addr).WithLovelace(1_000_000).Build()
	require.NoError(b, err)
	tx, err := mockledger.NewTransactionBuilder().WithInputs(inputs...).WithOutputs(output).WithFee(10).Build()
	require.NoError(b, err)
	ls := mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build()

	require.NoError(b, common.ValidateScriptWitnesses(tx, ls), "precheck failed")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := common.ValidateScriptWitnesses(tx, ls); err != nil {
			b.Fatal(err)
		}
	}
}

// benchValidateScriptWitnessesWithScripts benchmarks the path where inputs
// spend from script addresses, requiring script-witness matching logic.
func benchValidateScriptWitnessesWithScripts(b *testing.B) {
	// Build a NativeScript (pubkey type) and compute its hash.
	keyHash := make([]byte, common.AddressHashSize)
	for i := range keyHash {
		keyHash[i] = byte(i + 0xA0)
	}
	nativeScript := benchMarshalNativeScript(b, &common.NativeScriptPubkey{
		Type: 0,
		Hash: keyHash,
	})
	scriptHash := nativeScript.Hash()

	// Build a script address (type ScriptKey) using the script hash as the
	// payment credential and a dummy staking key hash.
	stakingHash := make([]byte, common.AddressHashSize)
	for i := range stakingHash {
		stakingHash[i] = byte(i + 0xB0)
	}
	scriptAddr, err := common.NewAddressFromParts(
		common.AddressTypeScriptKey,
		common.AddressNetworkMainnet,
		scriptHash.Bytes(),
		stakingHash,
	)
	require.NoError(b, err)
	scriptAddrStr := scriptAddr.String()

	// Create 64 inputs all spending from the script address.
	inputs := make([]common.TransactionInput, 0, 64)
	utxos := make([]common.Utxo, 0, 64)
	for i := range 64 {
		txID := []byte{byte(i + 1)}
		input, err := mockledger.NewTransactionInputBuilder().WithTxId(txID).WithIndex(0).Build()
		require.NoError(b, err)
		utxo, err := mockledger.NewUtxoBuilder().
			WithTxId(txID).
			WithIndex(0).
			WithAddress(scriptAddrStr).
			WithLovelace(2_000_000).
			Build()
		require.NoError(b, err)
		inputs = append(inputs, input)
		utxos = append(utxos, utxo)
	}

	// Use the same script address for the output (valid for benchmarking).
	output, err := mockledger.NewTransactionOutputBuilder().
		WithAddress(scriptAddrStr).
		WithLovelace(1_000_000).
		Build()
	require.NoError(b, err)

	// Build a witness set containing the matching native script.
	witnesses := mockledger.NewMockTransactionWitnessSet().
		WithNativeScripts(nativeScript)

	// Build the transaction with the witness set.
	txBuilder := mockledger.NewTransactionBuilder()
	txBuilder.WithWitnesses(witnesses)
	tx, err := txBuilder.
		WithInputs(inputs...).
		WithOutputs(output).
		WithFee(10).
		Build()
	require.NoError(b, err)
	ls := mockledger.NewLedgerStateBuilder().WithUtxos(utxos).Build()

	// Verify the precheck passes before benchmarking.
	require.NoError(b, common.ValidateScriptWitnesses(tx, ls), "precheck failed")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := common.ValidateScriptWitnesses(tx, ls); err != nil {
			b.Fatal(err)
		}
	}
}
