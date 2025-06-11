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

package mary_test

import (
	"encoding/hex"
	"math/big"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/assert"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

func TestMaryProtocolParamsUpdate(t *testing.T) {
	testDefs := []struct {
		startParams    mary.MaryProtocolParameters
		updateCbor     string
		expectedParams mary.MaryProtocolParameters
	}{
		{
			startParams: mary.MaryProtocolParameters{
				Decentralization: &cbor.Rat{
					Rat: new(big.Rat).SetInt64(1),
				},
			},
			updateCbor: "a10cd81e82090a",
			expectedParams: mary.MaryProtocolParameters{
				Decentralization: &cbor.Rat{Rat: big.NewRat(9, 10)},
			},
		},
		{
			startParams: mary.MaryProtocolParameters{
				ProtocolMajor: 4,
			},
			updateCbor: "a10e820500",
			expectedParams: mary.MaryProtocolParameters{
				ProtocolMajor: 5,
			},
		},
	}
	for _, testDef := range testDefs {
		cborBytes, err := hex.DecodeString(testDef.updateCbor)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		var tmpUpdate mary.MaryProtocolParameterUpdate
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

func TestMaryUtxorpc(t *testing.T) {
	inputParams := mary.MaryProtocolParameters{
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
		MinUtxoValue:       1000000,
	}

	expectedUtxorpc := &cardano.PParams{
		MinFeeCoefficient:        500,
		MinFeeConstant:           2,
		MaxBlockBodySize:         65536,
		MaxTxSize:                16384,
		MaxBlockHeaderSize:       1024,
		StakeKeyDeposit:          2000,
		PoolDeposit:              500000,
		PoolRetirementEpochBound: 2160,
		DesiredNumberOfPools:     100,
		PoolInfluence: &cardano.RationalNumber{
			Numerator:   int32(1),
			Denominator: uint32(2),
		},
		MonetaryExpansion: &cardano.RationalNumber{
			Numerator:   int32(3),
			Denominator: uint32(4),
		},
		TreasuryExpansion: &cardano.RationalNumber{
			Numerator:   int32(5),
			Denominator: uint32(6),
		},
		ProtocolVersion: &cardano.ProtocolVersion{
			Major: 8,
			Minor: 0,
		},
	}

	result := inputParams.Utxorpc()

	if !reflect.DeepEqual(result, expectedUtxorpc) {
		t.Fatalf(
			"Utxorpc() test failed for Mary:\nExpected: %#v\nGot: %#v",
			expectedUtxorpc,
			result,
		)
	}
}

// Unit test for MaryTransactionInput.Utxorpc()
func TestMaryTransactionInput_Utxorpc(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		0,
	)

	got := input.Utxorpc()
	want := &cardano.TxInput{
		TxHash:      input.Id().Bytes(),
		OutputIndex: input.Index(),
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf(
			"MaryTransactionInput.Utxorpc() mismatch\\nGot: %+v\\nWant: %+v",
			got,
			want,
		)
	}
}

// Unit test for MaryTransactionOutput.Utxorpc()
func TestMaryTransactionOutput_Utxorpc(t *testing.T) {
	address := common.Address{}
	amount := uint64(4200)

	output := mary.MaryTransactionOutput{
		OutputAddress: address,
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: amount},
	}

	got, err := output.Utxorpc()
	assert.NoError(t, err)
	addr, err := address.Bytes()
	assert.NoError(t, err)
	want := &cardano.TxOutput{
		Address: addr,
		Coin:    amount,
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf(
			"MaryTransactionOutput.Utxorpc() mismatch\\nGot: %+v\\nWant: %+v",
			got,
			want,
		)
	}
}

// Unit test for MaryTransactionBody.Utxorpc()
func TestMaryTransactionBody_Utxorpc(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		1,
	)
	inputSet := shelley.NewShelleyTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)

	address := common.Address{}
	output := mary.MaryTransactionOutput{
		OutputAddress: address,
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: 5000},
	}

	body := mary.MaryTransactionBody{
		TxInputs:  inputSet,
		TxOutputs: []mary.MaryTransactionOutput{output},
		TxFee:     100,
	}

	got := body.Utxorpc()

	if got.Fee != 100 {
		t.Errorf("Fee mismatch: got %d, want 100", got.Fee)
	}
	if len(got.Inputs) != 1 {
		t.Errorf("Expected 1 input, got %d", len(got.Inputs))
	}
	if len(got.Outputs) != 1 {
		t.Errorf("Expected 1 output, got %d", len(got.Outputs))
	}
	if got.Outputs[0].Coin != 5000 {
		t.Errorf("Output coin mismatch: got %d, want 5000", got.Outputs[0].Coin)
	}
	if len(got.Hash) == 0 {
		t.Error("Expected non-empty transaction hash")
	}
}

// Unit test for MaryTransaction.Utxorpc()
func TestMaryTransaction_Utxorpc(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		2,
	)
	inputSet := shelley.NewShelleyTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)

	address := common.Address{}
	output := mary.MaryTransactionOutput{
		OutputAddress: address,
		OutputAmount:  mary.MaryTransactionOutputValue{Amount: 8000},
	}

	body := mary.MaryTransactionBody{
		TxInputs:  inputSet,
		TxOutputs: []mary.MaryTransactionOutput{output},
		TxFee:     25,
	}

	tx := mary.MaryTransaction{
		Body: body,
	}

	got := tx.Utxorpc()

	if got.Fee != 25 {
		t.Errorf("Transaction fee mismatch: got %d, want 25", got.Fee)
	}
	if len(got.Inputs) != 1 {
		t.Errorf("Expected 1 input, got %d", len(got.Inputs))
	}
	if len(got.Outputs) != 1 {
		t.Errorf("Expected 1 output, got %d", len(got.Outputs))
	}
	if got.Outputs[0].Coin != 8000 {
		t.Errorf("Output coin mismatch: got %d, want 8000", got.Outputs[0].Coin)
	}
	if len(got.Hash) == 0 {
		t.Error("Expected non-empty transaction hash")
	}
}
