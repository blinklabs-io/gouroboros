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

	result, err := inputParams.Utxorpc()
	if err != nil {
		t.Fatalf("Utxorpc() conversion failed")
	}

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

	got, err := input.Utxorpc()
	if err != nil {
		t.Errorf("Could not get correct UTxorpc input")
	}
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

	got, err := body.Utxorpc()
	if err != nil {
		t.Fatalf(
			"Could not convert the transaction body to utxorpc format %v",
			err,
		)
	}

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

	got, err := tx.Utxorpc()
	if err != nil {
		t.Fatalf("Could not convert transaction to utxorpc format: %v", err)
	}

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

// ===================================================================
// Tests for new MaryProtocolParameters implementation
// ===================================================================

func TestMaryProtocolParametersUpdate_MinPoolCost(t *testing.T) {
	testCases := []struct {
		name           string
		startParams    mary.MaryProtocolParameters
		updateCbor     string
		expectedParams mary.MaryProtocolParameters
	}{
		{
			name: "Update MinPoolCost from zero to value",
			startParams: mary.MaryProtocolParameters{
				MinPoolCost: 0,
			},
			updateCbor: "a1101a1443fd00", // {16: 340000000}
			expectedParams: mary.MaryProtocolParameters{
				MinPoolCost: 340000000,
			},
		},
		{
			name: "Update MinPoolCost from existing value to new value",
			startParams: mary.MaryProtocolParameters{
				MinPoolCost: 340000000,
			},
			updateCbor: "a1101a0007a120", // {16: 500000}
			expectedParams: mary.MaryProtocolParameters{
				MinPoolCost: 500000,
			},
		},
		{
			name: "Update MinPoolCost to maximum uint64 value",
			startParams: mary.MaryProtocolParameters{
				MinPoolCost: 100,
			},
			updateCbor: "a1101bffffffffffffffff", // {16: 18446744073709551615}
			expectedParams: mary.MaryProtocolParameters{
				MinPoolCost: 18446744073709551615,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cborBytes, err := hex.DecodeString(tc.updateCbor)
			if err != nil {
				t.Fatalf("failed to decode CBOR hex: %s", err)
			}

			var update mary.MaryProtocolParameterUpdate
			if _, err := cbor.Decode(cborBytes, &update); err != nil {
				t.Fatalf("failed to decode CBOR: %s", err)
			}

			params := tc.startParams
			params.Update(&update)

			if params.MinPoolCost != tc.expectedParams.MinPoolCost {
				t.Errorf(
					"MinPoolCost mismatch: got %d, want %d",
					params.MinPoolCost,
					tc.expectedParams.MinPoolCost,
				)
			}
		})
	}
}

func TestMaryProtocolParametersUpdate_MultipleFields(t *testing.T) {
	startParams := mary.MaryProtocolParameters{
		MinFeeA:       100,
		MinFeeB:       200,
		MinPoolCost:   0,
		ProtocolMajor: 7,
	}

	// Update with multiple fields including MinPoolCost
	updateCbor := "a3101a00989680001818011819" // {0:24, 1:25, 16:10000000}
	cborBytes, err := hex.DecodeString(updateCbor)
	if err != nil {
		t.Fatalf("failed to decode CBOR hex: %s", err)
	}

	var update mary.MaryProtocolParameterUpdate
	if _, err := cbor.Decode(cborBytes, &update); err != nil {
		t.Fatalf("failed to decode CBOR: %s", err)
	}

	startParams.Update(&update)

	if startParams.MinFeeA != 24 {
		t.Errorf("MinFeeA not updated: got %d, want 24", startParams.MinFeeA)
	}
	if startParams.MinFeeB != 25 {
		t.Errorf("MinFeeB not updated: got %d, want 25", startParams.MinFeeB)
	}
	if startParams.MinPoolCost != 10000000 {
		t.Errorf(
			"MinPoolCost not updated: got %d, want 10000000",
			startParams.MinPoolCost,
		)
	}
	if startParams.ProtocolMajor != 7 {
		t.Errorf(
			"ProtocolMajor should not change: got %d, want 7",
			startParams.ProtocolMajor,
		)
	}
}

func TestMaryUtxorpc_WithMinPoolCost(t *testing.T) {
	testCases := []struct {
		name                string
		inputParams         mary.MaryProtocolParameters
		expectedMinPoolCost uint64
	}{
		{
			name: "MinPoolCost zero",
			inputParams: mary.MaryProtocolParameters{
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
				MinPoolCost:        0,
			},
			expectedMinPoolCost: 0,
		},
		{
			name: "MinPoolCost standard value",
			inputParams: mary.MaryProtocolParameters{
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
				MinPoolCost:        340000000,
			},
			expectedMinPoolCost: 340000000,
		},
		{
			name: "MinPoolCost maximum value",
			inputParams: mary.MaryProtocolParameters{
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
				MinPoolCost:        18446744073709551615,
			},
			expectedMinPoolCost: 18446744073709551615,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.inputParams.Utxorpc()
			if err != nil {
				t.Fatalf("Utxorpc() conversion failed: %v", err)
			}

			if result.MinPoolCost != tc.expectedMinPoolCost {
				t.Errorf(
					"MinPoolCost mismatch: got %d, want %d",
					result.MinPoolCost,
					tc.expectedMinPoolCost,
				)
			}
		})
	}
}

func TestMaryProtocolParameterUpdate_CBORDecoding(t *testing.T) {
	testCases := []struct {
		name        string
		cborHex     string
		expectError bool
		validate    func(*testing.T, *mary.MaryProtocolParameterUpdate)
	}{
		{
			name:        "Decode MinPoolCost only",
			cborHex:     "a1101a1443fd00",
			expectError: false,
			validate: func(t *testing.T, u *mary.MaryProtocolParameterUpdate) {
				if u.MinPoolCost == nil {
					t.Error("MinPoolCost should not be nil")
				} else if *u.MinPoolCost != 340000000 {
					t.Errorf("MinPoolCost: got %d, want 340000000", *u.MinPoolCost)
				}
			},
		},
		{
			name:        "Decode empty update",
			cborHex:     "a0",
			expectError: false,
			validate: func(t *testing.T, u *mary.MaryProtocolParameterUpdate) {
				if u.MinPoolCost != nil {
					t.Error("MinPoolCost should be nil for empty update")
				}
			},
		},
		{
			name:        "Decode MinPoolCost with other fields",
			cborHex:     "a3011901f4101a000186a0001864",
			expectError: false,
			validate: func(t *testing.T, u *mary.MaryProtocolParameterUpdate) {
				if u.MinFeeA == nil || *u.MinFeeA != 100 {
					t.Error("MinFeeA should be 100")
				}
				if u.MinFeeB == nil || *u.MinFeeB != 500 {
					t.Error("MinFeeB should be 500")
				}
				if u.MinPoolCost == nil || *u.MinPoolCost != 100000 {
					t.Error("MinPoolCost should be 100000")
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cborBytes, err := hex.DecodeString(tc.cborHex)
			if err != nil {
				t.Fatalf("failed to decode hex: %s", err)
			}

			var update mary.MaryProtocolParameterUpdate
			_, err = cbor.Decode(cborBytes, &update)

			if tc.expectError && err == nil {
				t.Fatal("expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if !tc.expectError && tc.validate != nil {
				tc.validate(t, &update)
			}
		})
	}
}

func TestMaryUpgradePParams(t *testing.T) {
	testCases := []struct {
		name       string
		prevParams shelley.ShelleyProtocolParameters
		validate   func(*testing.T, mary.MaryProtocolParameters)
	}{
		{
			name: "Upgrade with all fields",
			prevParams: shelley.ShelleyProtocolParameters{
				MinFeeA:            44,
				MinFeeB:            155381,
				MaxBlockBodySize:   65536,
				MaxTxSize:          16384,
				MaxBlockHeaderSize: 1100,
				KeyDeposit:         2000000,
				PoolDeposit:        500000000,
				MaxEpoch:           18,
				NOpt:               500,
				A0:                 &cbor.Rat{Rat: big.NewRat(3, 10)},
				Rho:                &cbor.Rat{Rat: big.NewRat(3, 1000)},
				Tau:                &cbor.Rat{Rat: big.NewRat(2, 10)},
				Decentralization:   &cbor.Rat{Rat: big.NewRat(1, 2)},
				ExtraEntropy:       common.Nonce{},
				ProtocolMajor:      6,
				ProtocolMinor:      0,
				MinUtxoValue:       1000000,
			},
			validate: func(t *testing.T, maryParams mary.MaryProtocolParameters) {
				if maryParams.MinFeeA != 44 {
					t.Errorf("MinFeeA: got %d, want 44", maryParams.MinFeeA)
				}
				if maryParams.MinFeeB != 155381 {
					t.Errorf("MinFeeB: got %d, want 155381", maryParams.MinFeeB)
				}
				if maryParams.MaxBlockBodySize != 65536 {
					t.Errorf(
						"MaxBlockBodySize: got %d, want 65536",
						maryParams.MaxBlockBodySize,
					)
				}
				if maryParams.MaxTxSize != 16384 {
					t.Errorf(
						"MaxTxSize: got %d, want 16384",
						maryParams.MaxTxSize,
					)
				}
				if maryParams.MaxBlockHeaderSize != 1100 {
					t.Errorf(
						"MaxBlockHeaderSize: got %d, want 1100",
						maryParams.MaxBlockHeaderSize,
					)
				}
				if maryParams.KeyDeposit != 2000000 {
					t.Errorf(
						"KeyDeposit: got %d, want 2000000",
						maryParams.KeyDeposit,
					)
				}
				if maryParams.PoolDeposit != 500000000 {
					t.Errorf(
						"PoolDeposit: got %d, want 500000000",
						maryParams.PoolDeposit,
					)
				}
				if maryParams.MaxEpoch != 18 {
					t.Errorf("MaxEpoch: got %d, want 18", maryParams.MaxEpoch)
				}
				if maryParams.NOpt != 500 {
					t.Errorf("NOpt: got %d, want 500", maryParams.NOpt)
				}
				if maryParams.A0 == nil ||
					maryParams.A0.Cmp(big.NewRat(3, 10)) != 0 {
					t.Error("A0 not correctly copied")
				}
				if maryParams.Rho == nil ||
					maryParams.Rho.Cmp(big.NewRat(3, 1000)) != 0 {
					t.Error("Rho not correctly copied")
				}
				if maryParams.Tau == nil ||
					maryParams.Tau.Cmp(big.NewRat(2, 10)) != 0 {
					t.Error("Tau not correctly copied")
				}
				if maryParams.Decentralization == nil ||
					maryParams.Decentralization.Cmp(big.NewRat(1, 2)) != 0 {
					t.Error("Decentralization not correctly copied")
				}
				if maryParams.ProtocolMajor != 6 {
					t.Errorf(
						"ProtocolMajor: got %d, want 6",
						maryParams.ProtocolMajor,
					)
				}
				if maryParams.ProtocolMinor != 0 {
					t.Errorf(
						"ProtocolMinor: got %d, want 0",
						maryParams.ProtocolMinor,
					)
				}
				if maryParams.MinUtxoValue != 1000000 {
					t.Errorf(
						"MinUtxoValue: got %d, want 1000000",
						maryParams.MinUtxoValue,
					)
				}
				// MinPoolCost should default to 340 ADA for Shelley upgrade
				if maryParams.MinPoolCost != 340000000 {
					t.Errorf(
						"MinPoolCost: got %d, want 340000000",
						maryParams.MinPoolCost,
					)
				}
			},
		},
		{
			name: "Upgrade with zero values",
			prevParams: shelley.ShelleyProtocolParameters{
				MinFeeA:            0,
				MinFeeB:            0,
				MaxBlockBodySize:   0,
				MaxTxSize:          0,
				MaxBlockHeaderSize: 0,
				KeyDeposit:         0,
				PoolDeposit:        0,
				MaxEpoch:           0,
				NOpt:               0,
				A0:                 &cbor.Rat{Rat: big.NewRat(0, 1)},
				Rho:                &cbor.Rat{Rat: big.NewRat(0, 1)},
				Tau:                &cbor.Rat{Rat: big.NewRat(0, 1)},
				Decentralization:   &cbor.Rat{Rat: big.NewRat(0, 1)},
				ExtraEntropy:       common.Nonce{},
				ProtocolMajor:      0,
				ProtocolMinor:      0,
				MinUtxoValue:       0,
			},
			validate: func(t *testing.T, maryParams mary.MaryProtocolParameters) {
				if maryParams.MinFeeA != 0 {
					t.Errorf("MinFeeA should be 0, got %d", maryParams.MinFeeA)
				}
				if maryParams.MinPoolCost != 340000000 {
					t.Errorf(
						"MinPoolCost should be 340000000, got %d",
						maryParams.MinPoolCost,
					)
				}
			},
		},
		{
			name: "Upgrade with nil rational numbers",
			prevParams: shelley.ShelleyProtocolParameters{
				MinFeeA:            100,
				MinFeeB:            200,
				MaxBlockBodySize:   1000,
				MaxTxSize:          500,
				MaxBlockHeaderSize: 100,
				KeyDeposit:         1000,
				PoolDeposit:        5000,
				MaxEpoch:           100,
				NOpt:               50,
				A0:                 nil,
				Rho:                nil,
				Tau:                nil,
				Decentralization:   nil,
				ExtraEntropy:       common.Nonce{},
				ProtocolMajor:      5,
				ProtocolMinor:      0,
				MinUtxoValue:       1000,
			},
			validate: func(t *testing.T, maryParams mary.MaryProtocolParameters) {
				if maryParams.A0 != nil {
					t.Error("A0 should be nil")
				}
				if maryParams.Rho != nil {
					t.Error("Rho should be nil")
				}
				if maryParams.Tau != nil {
					t.Error("Tau should be nil")
				}
				if maryParams.Decentralization != nil {
					t.Error("Decentralization should be nil")
				}
				if maryParams.MinPoolCost != 340000000 {
					t.Errorf(
						"MinPoolCost should be 340000000, got %d",
						maryParams.MinPoolCost,
					)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			maryParams := mary.UpgradePParams(tc.prevParams)
			tc.validate(t, maryParams)
		})
	}
}

func TestMaryUpgradePParams_PointerIdentity(t *testing.T) {
	// Test that pointers to rational numbers are correctly copied (not shared)
	prevParams := shelley.ShelleyProtocolParameters{
		MinFeeA:            100,
		MinFeeB:            200,
		MaxBlockBodySize:   1000,
		MaxTxSize:          500,
		MaxBlockHeaderSize: 100,
		KeyDeposit:         1000,
		PoolDeposit:        5000,
		MaxEpoch:           100,
		NOpt:               50,
		A0:                 &cbor.Rat{Rat: big.NewRat(1, 2)},
		Rho:                &cbor.Rat{Rat: big.NewRat(1, 3)},
		Tau:                &cbor.Rat{Rat: big.NewRat(1, 4)},
		Decentralization:   &cbor.Rat{Rat: big.NewRat(1, 5)},
		ExtraEntropy:       common.Nonce{},
		ProtocolMajor:      5,
		ProtocolMinor:      0,
		MinUtxoValue:       1000,
	}

	maryParams := mary.UpgradePParams(prevParams)

	// Verify that the pointers are the same (direct copy)
	if maryParams.A0 != prevParams.A0 {
		t.Error("A0 pointer should be the same")
	}
	if maryParams.Rho != prevParams.Rho {
		t.Error("Rho pointer should be the same")
	}
	if maryParams.Tau != prevParams.Tau {
		t.Error("Tau pointer should be the same")
	}
	if maryParams.Decentralization != prevParams.Decentralization {
		t.Error("Decentralization pointer should be the same")
	}
}

func TestMaryUtxorpc_InvalidRationalNumbers(t *testing.T) {
	testCases := []struct {
		name        string
		params      mary.MaryProtocolParameters
		expectError string
	}{
		{
			name: "A0 numerator exceeds int32 max",
			params: mary.MaryProtocolParameters{
				MinFeeA:            500,
				MinFeeB:            2,
				MaxBlockBodySize:   65536,
				MaxTxSize:          16384,
				MaxBlockHeaderSize: 1024,
				KeyDeposit:         2000,
				PoolDeposit:        500000,
				MaxEpoch:           2160,
				NOpt:               100,
				A0: &cbor.Rat{
					Rat: big.NewRat(2147483648, 1),
				}, // MaxInt32 + 1
				Rho:           &cbor.Rat{Rat: big.NewRat(3, 4)},
				Tau:           &cbor.Rat{Rat: big.NewRat(5, 6)},
				ProtocolMajor: 8,
				ProtocolMinor: 0,
				MinUtxoValue:  1000000,
			},
			expectError: "invalid A0 rational number values",
		},
		{
			name: "Rho numerator exceeds int32 max",
			params: mary.MaryProtocolParameters{
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
				Rho: &cbor.Rat{
					Rat: big.NewRat(2147483648, 1),
				}, // MaxInt32 + 1
				Tau:           &cbor.Rat{Rat: big.NewRat(5, 6)},
				ProtocolMajor: 8,
				ProtocolMinor: 0,
				MinUtxoValue:  1000000,
			},
			expectError: "invalid Rho rational number values",
		},
		{
			name: "Tau denominator exceeds uint32 max",
			params: mary.MaryProtocolParameters{
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
				Tau: &cbor.Rat{
					Rat: big.NewRat(1, 4294967296),
				}, // MaxUint32 + 1
				ProtocolMajor: 8,
				ProtocolMinor: 0,
				MinUtxoValue:  1000000,
			},
			expectError: "invalid Tau rational number values",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.params.Utxorpc()
			if err == nil {
				t.Fatal("expected error but got none")
			}
			if !reflect.DeepEqual(err.Error(), tc.expectError) {
				t.Errorf(
					"expected error %q, got %q",
					tc.expectError,
					err.Error(),
				)
			}
		})
	}
}
