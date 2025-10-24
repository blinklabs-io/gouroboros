// Copyright 2024 Blink Labs Software
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

package allegra_test

import (
	"encoding/hex"
	"math/big"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

func TestAllegraProtocolParamsUpdate(t *testing.T) {
	testDefs := []struct {
		startParams    allegra.AllegraProtocolParameters
		updateCbor     string
		expectedParams allegra.AllegraProtocolParameters
	}{
		{
			startParams: allegra.AllegraProtocolParameters{
				Decentralization: &cbor.Rat{Rat: new(big.Rat).SetInt64(1)},
			},
			updateCbor: "a10cd81e82090a",
			expectedParams: allegra.AllegraProtocolParameters{
				Decentralization: &cbor.Rat{Rat: big.NewRat(9, 10)},
			},
		},
		{
			startParams: allegra.AllegraProtocolParameters{
				ProtocolMajor: 3,
			},
			updateCbor: "a10e820400",
			expectedParams: allegra.AllegraProtocolParameters{
				ProtocolMajor: 4,
			},
		},
	}
	for _, testDef := range testDefs {
		cborBytes, err := hex.DecodeString(testDef.updateCbor)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		var tmpUpdate allegra.AllegraProtocolParameterUpdate
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

func TestAllegraUtxorpc(t *testing.T) {
	inputParams := allegra.AllegraProtocolParameters{
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

	result, err := inputParams.Utxorpc()
	if err != nil {
		t.Fatalf("Utxorpc() conversion failed: %v", err)
	}
	if !reflect.DeepEqual(result, expectedUtxorpc) {
		t.Fatalf(
			"Utxorpc() test failed for Allegra:\nExpected: %#v\nGot: %#v",
			expectedUtxorpc,
			result,
		)
	}
}

// Unit test for AllegraTransactionBody.Utxorpc()
func TestAllegraTransactionBody_Utxorpc(t *testing.T) {
	// mock input
	input := shelley.NewShelleyTransactionInput(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		1,
	)
	inputSet := shelley.NewShelleyTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)

	address := common.Address{}

	// Define a transaction output
	output := shelley.ShelleyTransactionOutput{
		OutputAddress: address,
		OutputAmount:  1000,
	}

	// Create transaction body
	txBody := &allegra.AllegraTransactionBody{
		TxInputs:                inputSet,
		TxOutputs:               []shelley.ShelleyTransactionOutput{output},
		TxFee:                   200,
		Ttl:                     5000,
		TxValidityIntervalStart: 4500,
	}

	// Run Utxorpc conversion
	actual, err := txBody.Utxorpc()
	if err != nil {
		t.Fatalf(
			"Could not convert transaction body to utxorpc format: %v",
			err,
		)
	}

	// Check that the fee matches
	if actual.Fee != txBody.Fee() {
		t.Errorf(
			"AllegraTransactionBody.Utxorpc() fee mismatch\nGot: %d\nWant: %d",
			actual.Fee,
			txBody.Fee(),
		)
	}
	// Check number of inputs
	if len(actual.Inputs) != len(txBody.Inputs()) {
		t.Errorf(
			"AllegraTransactionBody.Utxorpc() input length mismatch\nGot: %d\nWant: %d",
			len(actual.Inputs),
			len(txBody.Inputs()),
		)
	}
	// Check number of outputs
	if len(actual.Outputs) != len(txBody.Outputs()) {
		t.Errorf(
			"AllegraTransactionBody.Utxorpc() output length mismatch\nGot: %d\nWant: %d",
			len(actual.Outputs),
			len(txBody.Outputs()),
		)
	}
}

// Unit test for AllegraTransaction.Utxorpc()
func TestAllegraTransaction_Utxorpc(t *testing.T) {
	// Prepare mock input
	input := shelley.NewShelleyTransactionInput(
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		0,
	)
	inputSet := shelley.NewShelleyTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)

	// Prepare mock output
	address := common.Address{}
	output := shelley.ShelleyTransactionOutput{
		OutputAddress: address,
		OutputAmount:  2000,
	}

	// Create transaction body
	tx := &allegra.AllegraTransaction{
		Body: allegra.AllegraTransactionBody{
			TxInputs:                inputSet,
			TxOutputs:               []shelley.ShelleyTransactionOutput{output},
			TxFee:                   150,
			Ttl:                     1000,
			TxValidityIntervalStart: 950,
		},
		WitnessSet: shelley.ShelleyTransactionWitnessSet{},
		TxMetadata: nil,
	}

	// Run Utxorpc conversion
	actual, err := tx.Utxorpc()
	if err != nil {
		t.Fatalf("Could not convert transaction to utxorpc format: %v", err)
	}
	// Assertion checks
	if actual.Fee != tx.Fee() {
		t.Errorf(
			"AllegraTransaction.Utxorpc() fee mismatch\nGot: %d\nWant: %d",
			actual.Fee,
			tx.Fee(),
		)
	}

	if len(actual.Inputs) != len(tx.Inputs()) {
		t.Errorf(
			"AllegraTransaction.Utxorpc() input length mismatch\nGot: %d\nWant: %d",
			len(actual.Inputs),
			len(tx.Inputs()),
		)
	}

	if len(actual.Outputs) != len(tx.Outputs()) {
		t.Errorf(
			"AllegraTransaction.Utxorpc() output length mismatch\nGot: %d\nWant: %d",
			len(actual.Outputs),
			len(tx.Outputs()),
		)
	}
}
