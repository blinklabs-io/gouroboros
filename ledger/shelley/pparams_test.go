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
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

func TestShelleyProtocolParamsUpdate(t *testing.T) {
	testDefs := []struct {
		startParams    shelley.ShelleyProtocolParameters
		updateCbor     string
		expectedParams shelley.ShelleyProtocolParameters
	}{
		{
			startParams: shelley.ShelleyProtocolParameters{
				Decentralization: &cbor.Rat{Rat: new(big.Rat).SetInt64(1)},
			},
			updateCbor: "a10cd81e82090a",
			expectedParams: shelley.ShelleyProtocolParameters{
				Decentralization: &cbor.Rat{Rat: big.NewRat(9, 10)},
			},
		},
		{
			startParams: shelley.ShelleyProtocolParameters{
				ProtocolMajor: 2,
			},
			updateCbor: "a10e820300",
			expectedParams: shelley.ShelleyProtocolParameters{
				ProtocolMajor: 3,
			},
		},
	}
	for _, testDef := range testDefs {
		cborBytes, err := hex.DecodeString(testDef.updateCbor)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		var tmpUpdate shelley.ShelleyProtocolParameterUpdate
		if _, err := cbor.Decode(cborBytes, &tmpUpdate); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		tmpParams := testDef.startParams
		tmpParams.Update(&tmpUpdate)
		if !reflect.DeepEqual(tmpParams, testDef.expectedParams) {
			t.Fatalf(
				"did not get expected params:\n     got: %#v\n  wanted: %#v",
				tmpParams,
				testDef.expectedParams,
			)
		}
	}
}

func TestShelleyProtocolParamsUpdateFromGenesis(t *testing.T) {
	testDefs := []struct {
		startParams    shelley.ShelleyProtocolParameters
		genesisJson    string
		expectedParams shelley.ShelleyProtocolParameters
	}{
		{
			startParams: shelley.ShelleyProtocolParameters{},
			genesisJson: `{"protocolParams":{"decentralisationParam":0.9}}`,
			expectedParams: shelley.ShelleyProtocolParameters{
				Decentralization: &cbor.Rat{Rat: big.NewRat(9, 10)},
			},
		},
	}
	for _, testDef := range testDefs {
		tmpGenesis, err := shelley.NewShelleyGenesisFromReader(
			strings.NewReader(testDef.genesisJson),
		)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		tmpParams := testDef.startParams
		if err := tmpParams.UpdateFromGenesis(&tmpGenesis); err != nil {
			t.Fatalf("unexpected error updating pparams from genesis: %s", err)
		}
		if !reflect.DeepEqual(tmpParams, testDef.expectedParams) {
			t.Fatalf(
				"did not get expected params:\n     got: %#v\n  wanted: %#v",
				tmpParams,
				testDef.expectedParams,
			)
		}
	}
}

func TestShelleyUtxorpc(t *testing.T) {
	inputParams := shelley.ShelleyProtocolParameters{
		MinFeeA:            500,
		MinFeeB:            2,
		MaxBlockBodySize:   65536,
		MaxTxSize:          16384,
		MaxBlockHeaderSize: 1024,
		KeyDeposit:         2000,
		PoolDeposit:        500000,
		MaxEpoch:           2160,
		NOpt:               100,
		A0:                 &cbor.Rat{Rat: big.NewRat(1, 2)},
		Rho:                &cbor.Rat{Rat: big.NewRat(3, 4)},
		Tau:                &cbor.Rat{Rat: big.NewRat(5, 6)},
		ProtocolMajor:      8,
		ProtocolMinor:      0,
	}

	expectedUtxorpc := &utxorpc.PParams{
		MaxTxSize:                16384,
		MinFeeCoefficient:        common.ToUtxorpcBigInt(500),
		MinFeeConstant:           common.ToUtxorpcBigInt(2),
		MaxBlockBodySize:         65536,
		MaxBlockHeaderSize:       1024,
		StakeKeyDeposit:          common.ToUtxorpcBigInt(2000),
		PoolDeposit:              common.ToUtxorpcBigInt(500000),
		PoolRetirementEpochBound: 2160,
		DesiredNumberOfPools:     100,
		PoolInfluence: &utxorpc.RationalNumber{
			Numerator:   int32(1),
			Denominator: uint32(2),
		},
		MonetaryExpansion: &utxorpc.RationalNumber{
			Numerator:   int32(3),
			Denominator: uint32(4),
		},
		TreasuryExpansion: &utxorpc.RationalNumber{
			Numerator:   int32(5),
			Denominator: uint32(6),
		},
		ProtocolVersion: &utxorpc.ProtocolVersion{
			Major: 8,
			Minor: 0,
		},
	}

	result, err := inputParams.Utxorpc()
	if err != nil {
		t.Fatalf("Utxorpc() conversion failed")
	}

	if !reflect.DeepEqual(result, expectedUtxorpc) {
		t.Fatalf(
			"Utxorpc() test failed for Shelley:\nExpected: %#v\nGot: %#v",
			expectedUtxorpc,
			result,
		)
	}
}

// Tests conversion of a ShelleyTransactionInput to its utxorpc-compatible representation.
func TestShelleyTransactionInput_Utxorpc(t *testing.T) {
	// Create a mock transaction input with dummy transaction hash and index
	input := shelley.NewShelleyTransactionInput(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		1,
	)

	// Convert it to utxorpc TxInput
	got, err := input.Utxorpc()
	if err != nil {
		t.Errorf("Could not convert to  utxorpc TxInput")
	}

	// Expected value with same hash and index
	want := &utxorpc.TxInput{
		TxHash:      input.Id().Bytes(),
		OutputIndex: input.Index(),
	}

	// Compare actual and expected results
	if !reflect.DeepEqual(got, want) {
		t.Errorf("TxInput.Utxorpc() mismatch\nGot: %+v\nWant: %+v", got, want)
	}
}

// Test the conversion of a ShelleyTransactionOutput to its utxorpc-compatible representation.
func TestShelleyTransactionOutput_Utxorpc(t *testing.T) {
	// Use a zero-value common.Address as a placeholder
	address := common.Address{}

	// Create a transaction output
	output := shelley.ShelleyTransactionOutput{
		OutputAddress: address,
		OutputAmount: new(big.Int).SetUint64(1000),
	}

	// Convert it to utxorpc format
	actual, err := output.Utxorpc()
	if err != nil {
		t.Fatalf("Utxorpc() failed: %v", err) // Fail immediately on error
	}

	// Get expected address bytes (with error handling)
	expectedAddressBytes, err := address.Bytes()
	if err != nil {
		t.Fatalf("address.Bytes() failed: %v", err)
	}

	// expected output in utxorpc format
	expected := &utxorpc.TxOutput{
		Address: expectedAddressBytes,
		Coin:    common.ToUtxorpcBigInt(1000),
	}

	// Debug prints
	t.Logf(
		"DEBUG got.Address=%#v want.Address=%#v\n",
		actual.Address,
		expected.Address,
	)

	// Compare actual and expected results
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf(
			"TxOutput.Utxorpc() mismatch\nGot: %+v\nWant: %+v",
			actual,
			expected,
		)
	}
}

// Test the conversion of a full ShelleyTransactionBody to utxorpc format, verifying fee, input count, and output count.
func TestShelleyTransactionBody_Utxorpc(t *testing.T) {
	// Create input set with one mock input
	input := shelley.NewShelleyTransactionInput(
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		0,
	)
	inputSet := shelley.NewShelleyTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)

	// Use mock address
	address := common.Address{}

	// Define a transaction output
	output := shelley.ShelleyTransactionOutput{
		OutputAddress: address,
		OutputAmount: new(big.Int).SetUint64(2000),
	}

	// Create the transaction body
	txBody := &shelley.ShelleyTransactionBody{
		TxInputs:  inputSet,
		TxOutputs: []shelley.ShelleyTransactionOutput{output},
		TxFee:     150,
		Ttl:       1000,
	}

	// Convert the transaction body to utxorpc format
	actual, err := txBody.Utxorpc()
	if err != nil {
		t.Fatalf(
			"Could not convert the transaction body to utxorpc format %v",
			err,
		)
	}

	// Check that the fee matches
	if actual.Fee.GetInt() != int64(txBody.Fee()) {
		t.Errorf(
			"TxBody.Utxorpc() fee mismatch\nGot: %d\nWant: %d",
			actual.Fee.GetInt(),
			txBody.Fee(),
		)
	}

	// Check number of inputs
	if len(actual.Inputs) != len(txBody.Inputs()) {
		t.Errorf(
			"TxBody.Utxorpc() input length mismatch\nGot: %d\nWant: %d",
			len(actual.Inputs),
			len(txBody.Inputs()),
		)
	}

	// Check number of outputs
	if len(actual.Outputs) != len(txBody.Outputs()) {
		t.Errorf(
			"TxBody.Utxorpc() output length mismatch\nGot: %d\nWant: %d",
			len(actual.Outputs),
			len(txBody.Outputs()),
		)
	}
}

// Test the conversion of a full ShelleyTransaction to utxorpc format, verifying fee, input count, and output count.
func TestShelleyTransaction_Utxorpc(t *testing.T) {
	// Create input set with one mock input
	input := shelley.NewShelleyTransactionInput(
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		0,
	)
	inputSet := shelley.NewShelleyTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)

	// Use mock address
	address := common.Address{}

	// Define a  transaction output
	output := shelley.ShelleyTransactionOutput{
		OutputAddress: address,
		OutputAmount: new(big.Int).SetUint64(5000),
	}

	// Build the transaction body
	txBody := shelley.ShelleyTransactionBody{
		TxInputs:  inputSet,
		TxOutputs: []shelley.ShelleyTransactionOutput{output},
		TxFee:     250,
		Ttl:       800,
	}

	// Create full transaction with body and empty witness set
	tx := shelley.ShelleyTransaction{
		Body:       txBody,
		WitnessSet: shelley.ShelleyTransactionWitnessSet{},
	}

	// Invoke Utxorpc conversion
	got, err := tx.Utxorpc()
	if err != nil {
		t.Fatalf("Could not convert transaction to utxorpc format: %v", err)
	}

	// Verify the fee
	if got.Fee.GetInt() != int64(tx.Body.Fee()) {
		t.Errorf(
			"ShelleyTransaction.Utxorpc() fee mismatch\nGot: %d\nWant: %d",
			got.Fee.GetInt(),
			tx.Body.Fee(),
		)
	}

	// Verify input count
	if len(got.Inputs) != len(tx.Body.Inputs()) {
		t.Errorf(
			"ShelleyTransaction.Utxorpc() input count mismatch\nGot: %d\nWant: %d",
			len(got.Inputs),
			len(tx.Body.Inputs()),
		)
	}

	// Verify output count
	if len(got.Outputs) != len(tx.Body.Outputs()) {
		t.Errorf(
			"ShelleyTransaction.Utxorpc() output count mismatch\nGot: %d\nWant: %d",
			len(got.Outputs),
			len(tx.Body.Outputs()),
		)
	}
}
