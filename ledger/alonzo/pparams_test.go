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

package alonzo_test

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	cardano "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

func newBaseProtocolParams() alonzo.AlonzoProtocolParameters {
	return alonzo.AlonzoProtocolParameters{
		MinFeeA:            44,
		MinFeeB:            155381,
		MaxBlockBodySize:   65536,
		MaxTxSize:          16384,
		MaxBlockHeaderSize: 1100,
		KeyDeposit:         2000000,
		PoolDeposit:        500000000,
		MaxEpoch:           18,
		NOpt:               500,
		A0:                 &cbor.Rat{Rat: big.NewRat(1, 2)},
		Rho:                &cbor.Rat{Rat: big.NewRat(3, 4)},
		Tau:                &cbor.Rat{Rat: big.NewRat(5, 6)},
		ProtocolMajor:      8,
		ProtocolMinor:      0,
		MinPoolCost:        0,
		AdaPerUtxoByte:     4310,
		ExecutionCosts: common.ExUnitPrice{
			MemPrice:  &cbor.Rat{Rat: big.NewRat(577, 10000)},
			StepPrice: &cbor.Rat{Rat: big.NewRat(721, 10000000)},
		},
		MaxTxExUnits: common.ExUnits{
			Memory: 10000000,
			Steps:  10000000000,
		},
		MaxBlockExUnits: common.ExUnits{
			Memory: 50000000,
			Steps:  40000000000,
		},
		MaxValueSize:         5000,
		CollateralPercentage: 150,
		MaxCollateralInputs:  3,
		CostModels: map[uint][]int64{
			alonzo.PlutusV1Key: completeCostModel(166),
			alonzo.PlutusV2Key: completeCostModel(175),
		},
	}
}

func completeCostModel(size int) []int64 {
	model := make([]int64, size)
	for i := range model {
		model[i] = int64(i + 1)
	}
	return model
}

func TestAlonzoProtocolParamsUpdate(t *testing.T) {
	testDefs := []struct {
		startParams    alonzo.AlonzoProtocolParameters
		updateCbor     string
		expectedParams alonzo.AlonzoProtocolParameters
	}{
		{
			startParams: alonzo.AlonzoProtocolParameters{
				Decentralization: &cbor.Rat{
					Rat: new(big.Rat).SetInt64(1),
				},
			},
			updateCbor: "a10cd81e82090a",
			expectedParams: alonzo.AlonzoProtocolParameters{
				Decentralization: &cbor.Rat{Rat: big.NewRat(9, 10)},
			},
		},
		{
			startParams: alonzo.AlonzoProtocolParameters{
				ProtocolMajor: 5,
			},
			updateCbor: "a10e820600",
			expectedParams: alonzo.AlonzoProtocolParameters{
				ProtocolMajor: 6,
			},
		},
	}
	for _, testDef := range testDefs {
		cborBytes, err := hex.DecodeString(testDef.updateCbor)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		var tmpUpdate alonzo.AlonzoProtocolParameterUpdate
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

func TestAlonzoProtocolParametersUpdateFromGenesis(t *testing.T) {
	// Create cost models with numeric string keys
	plutusV1CostModel := make(map[string]int)
	for i := 0; i < 166; i++ {
		plutusV1CostModel[strconv.Itoa(i)] = i + 1 // "0":1, "1":2, etc.
	}

	plutusV2CostModel := make(map[string]int)
	for i := 0; i < 175; i++ {
		plutusV2CostModel[strconv.Itoa(i)] = i + 1 // "0":1, "1":2, etc.
	}

	genesisJSON := fmt.Sprintf(`{
        "lovelacePerUTxOWord": 34482,
        "maxValueSize": 5000,
        "collateralPercentage": 150,
        "maxCollateralInputs": 3,
        "executionPrices": {
            "prSteps": { "numerator": 721, "denominator": 10000000 },
            "prMem": { "numerator": 577, "denominator": 10000 }
        },
        "maxTxExUnits": { "exUnitsMem": 10000000, "exUnitsSteps": 10000000000 },
        "maxBlockExUnits": { "exUnitsMem": 50000000, "exUnitsSteps": 40000000000 },
        "costModels": {
            "PlutusV1": %s,
            "PlutusV2": %s
        }
    }`, toJSON(plutusV1CostModel), toJSON(plutusV2CostModel))

	var genesis alonzo.AlonzoGenesis
	if err := json.Unmarshal([]byte(genesisJSON), &genesis); err != nil {
		t.Fatalf("failed to parse genesis: %v", err)
	}

	params := newBaseProtocolParams()
	if err := params.UpdateFromGenesis(&genesis); err != nil {
		t.Fatalf("UpdateFromGenesis failed: %v", err)
	}

	if len(params.CostModels[alonzo.PlutusV1Key]) != 166 {
		t.Errorf(
			"expected 166 PlutusV1 parameters, got %d",
			len(params.CostModels[alonzo.PlutusV1Key]),
		)
	}
	if len(params.CostModels[alonzo.PlutusV2Key]) != 175 {
		t.Errorf(
			"expected 175 PlutusV2 parameters, got %d",
			len(params.CostModels[alonzo.PlutusV2Key]),
		)
	}
}

func TestCostModelArrayFormat(t *testing.T) {
	// Create cost model with numeric string keys
	plutusV1CostModel := make(map[string]int)
	for i := 0; i < 166; i++ {
		plutusV1CostModel[strconv.Itoa(i)] = i + 1 // "0":1, "1":2, etc.
	}

	genesisJSON := fmt.Sprintf(`{
        "lovelacePerUTxOWord": 34482,
        "maxValueSize": 5000,
        "collateralPercentage": 150,
        "maxCollateralInputs": 3,
        "executionPrices": {
            "prSteps": { "numerator": 721, "denominator": 10000000 },
            "prMem": { "numerator": 577, "denominator": 10000 }
        },
        "maxTxExUnits": { "exUnitsMem": 10000000, "exUnitsSteps": 10000000000 },
        "maxBlockExUnits": { "exUnitsMem": 50000000, "exUnitsSteps": 40000000000 },
        "costModels": {
            "PlutusV1": %s
        }
    }`, toJSON(plutusV1CostModel))

	var genesis alonzo.AlonzoGenesis
	if err := json.Unmarshal([]byte(genesisJSON), &genesis); err != nil {
		t.Fatalf("failed to unmarshal genesis JSON: %v", err)
	}

	params := alonzo.AlonzoProtocolParameters{}
	if err := params.UpdateFromGenesis(&genesis); err != nil {
		t.Fatalf("UpdateFromGenesis failed: %v", err)
	}

	if len(params.CostModels[alonzo.PlutusV1Key]) != 166 {
		t.Errorf(
			"expected 166 parameters, got %d",
			len(params.CostModels[alonzo.PlutusV1Key]),
		)
	}
}

func TestScientificNotationInCostModels(t *testing.T) {
	costModel := map[string]interface{}{
		"0": 2.477736e+06, // Changed from param1 to 0
		"1": 1.5e6,        // Changed from param2 to 1
		"2": 1000000,      // Changed from param3 to 2
	}
	// Fill remaining parameters
	for i := 3; i < 166; i++ {
		costModel[strconv.Itoa(i)] = i * 1000
	}

	genesisJSON := fmt.Sprintf(`{
        "lovelacePerUTxOWord": 34482,
        "maxValueSize": 5000,
        "collateralPercentage": 150,
        "maxCollateralInputs": 3,
        "executionPrices": {
            "prSteps": { "numerator": 721, "denominator": 10000000 },
            "prMem": { "numerator": 577, "denominator": 10000 }
        },
        "maxTxExUnits": { "exUnitsMem": 10000000, "exUnitsSteps": 10000000000 },
        "maxBlockExUnits": { "exUnitsMem": 50000000, "exUnitsSteps": 40000000000 },
        "costModels": {
            "PlutusV1": %s
        }
    }`, toJSON(costModel))

	var genesis alonzo.AlonzoGenesis
	if err := json.Unmarshal([]byte(genesisJSON), &genesis); err != nil {
		t.Fatalf("failed to unmarshal genesis: %v", err)
	}

	params := alonzo.AlonzoProtocolParameters{}
	if err := params.UpdateFromGenesis(&genesis); err != nil {
		t.Fatalf("UpdateFromGenesis failed: %v", err)
	}

	expected := []int64{2477736, 1500000, 1000000}
	for i := 0; i < 3; i++ {
		if params.CostModels[alonzo.PlutusV1Key][i] != expected[i] {
			t.Errorf("parameter %d conversion failed: got %d, want %d",
				i, params.CostModels[alonzo.PlutusV1Key][i], expected[i])
		}
	}
}

func TestInvalidCostModelFormats(t *testing.T) {
	tests := []struct {
		name        string
		costModels  string
		expectError string
	}{
		{
			name: "InvalidType",
			costModels: `"costModels": {
                "PlutusV1": "invalid"
            }`,
			expectError: "cannot unmarshal string into Go struct field AlonzoGenesis.costModels",
		},
		{
			name: "MissingParameters",
			costModels: `"costModels": {
                "PlutusV1": {"0":1, "1":2, "2":3}
            }`,
			expectError: "missing parameter at index 3 for PlutusV1",
		},
	}

	baseJSON := `{
        "lovelacePerUTxOWord": 34482,
        "maxValueSize": 5000,
        "collateralPercentage": 150,
        "maxCollateralInputs": 3,
        "executionPrices": {
            "prSteps": { "numerator": 721, "denominator": 10000000 },
            "prMem": { "numerator": 577, "denominator": 10000 }
        },
        "maxTxExUnits": { "exUnitsMem": 10000000, "exUnitsSteps": 10000000000 },
        "maxBlockExUnits": { "exUnitsMem": 50000000, "exUnitsSteps": 40000000000 },
        %s
    }`

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fullJSON := fmt.Sprintf(baseJSON, tt.costModels)

			var genesis alonzo.AlonzoGenesis
			err := json.Unmarshal([]byte(fullJSON), &genesis)
			if err == nil {
				params := alonzo.AlonzoProtocolParameters{}
				err = params.UpdateFromGenesis(&genesis)
				if err == nil {
					t.Fatal("expected error but got none")
				}
			}
			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf(
					"expected error containing %q, got %v",
					tt.expectError,
					err,
				)
			}
		})
	}
}

func TestAlonzoUtxorpc(t *testing.T) {
	inputParams := alonzo.AlonzoProtocolParameters{
		MaxTxSize:            16384,
		MinFeeA:              500,
		MinFeeB:              2,
		MaxBlockBodySize:     65536,
		MaxBlockHeaderSize:   1024,
		KeyDeposit:           2000,
		PoolDeposit:          500000,
		MaxEpoch:             2160,
		NOpt:                 100,
		A0:                   &cbor.Rat{Rat: big.NewRat(1, 2)},
		Rho:                  &cbor.Rat{Rat: big.NewRat(3, 4)},
		Tau:                  &cbor.Rat{Rat: big.NewRat(5, 6)},
		ProtocolMajor:        8,
		ProtocolMinor:        0,
		AdaPerUtxoByte:       44 / 8,
		MinPoolCost:          340000000,
		MaxValueSize:         1024,
		CollateralPercentage: 150,
		MaxCollateralInputs:  5,
		ExecutionCosts: common.ExUnitPrice{
			MemPrice:  &cbor.Rat{Rat: big.NewRat(1, 2)},
			StepPrice: &cbor.Rat{Rat: big.NewRat(2, 3)},
		},
		MaxTxExUnits: common.ExUnits{
			Memory: 1000000,
			Steps:  200000,
		},
		MaxBlockExUnits: common.ExUnits{
			Memory: 5000000,
			Steps:  1000000,
		},
		CostModels: map[uint][]int64{
			1: {100, 200, 300},
			2: {400, 500, 600},
			3: {700, 800, 900},
		},
	}

	expectedUtxorpc := &cardano.PParams{
		CoinsPerUtxoByte:         44 / 8,
		MaxTxSize:                16384,
		MinFeeCoefficient:        500,
		MinFeeConstant:           2,
		MaxBlockBodySize:         65536,
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
		MinPoolCost: 340000000,
		ProtocolVersion: &cardano.ProtocolVersion{
			Major: 8,
			Minor: 0,
		},
		MaxValueSize:         1024,
		CollateralPercentage: 150,
		MaxCollateralInputs:  5,
		CostModels: &cardano.CostModels{
			PlutusV1: &cardano.CostModel{
				Values: []int64{100, 200, 300},
			},
			PlutusV2: &cardano.CostModel{
				Values: []int64{400, 500, 600},
			},
			PlutusV3: &cardano.CostModel{
				Values: []int64{700, 800, 900},
			},
		},
		Prices: &cardano.ExPrices{
			Memory: &cardano.RationalNumber{
				Numerator:   int32(1),
				Denominator: uint32(2),
			},
			Steps: &cardano.RationalNumber{
				Numerator:   int32(2),
				Denominator: uint32(3),
			},
		},
		MaxExecutionUnitsPerTransaction: &cardano.ExUnits{
			Memory: 1000000,
			Steps:  200000,
		},
		MaxExecutionUnitsPerBlock: &cardano.ExUnits{
			Memory: 5000000,
			Steps:  1000000,
		},
	}

	result := inputParams.Utxorpc()

	if !reflect.DeepEqual(result, expectedUtxorpc) {
		t.Fatalf(
			"Utxorpc() test failed for Alonzo:\nExpected: %#v\nGot: %#v",
			expectedUtxorpc,
			result,
		)
	}
}

func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal JSON: %v", err))
	}
	return string(b)
}

// Unit test for AlonzoTransactionOutput.Utxorpc()
func TestAlonzoTransactionOutput_Utxorpc(t *testing.T) {
	address := common.Address{}
	amount := uint64(5000)
	datumHash := common.Blake2b256{1, 2, 3, 4}

	// Mock output
	output := alonzo.AlonzoTransactionOutput{
		OutputAddress:     address,
		OutputAmount:      mary.MaryTransactionOutputValue{Amount: amount},
		TxOutputDatumHash: &datumHash,
	}

	got := output.Utxorpc()
	want := &utxorpc.TxOutput{
		Address: address.Bytes(),
		Coin:    amount,
		Assets:  nil,
		Datum: &utxorpc.Datum{
			Hash: datumHash.Bytes(),
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("AlonzoTransactionOutput.Utxorpc() mismatch\nGot: %+v\nWant: %+v", got, want)
	}
}

// Unit test for AlonzoTransactionBody.Utxorpc()
func TestAlonzoTransactionBody_Utxorpc(t *testing.T) {
	// Mock input
	input := shelley.NewShelleyTransactionInput("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 0)
	inputSet := shelley.NewShelleyTransactionInputSet([]shelley.ShelleyTransactionInput{input})

	// Mock output
	address := common.Address{}
	datumHash := common.Blake2b256{1, 2, 3, 4}
	output := alonzo.AlonzoTransactionOutput{
		OutputAddress:     address,
		OutputAmount:      mary.MaryTransactionOutputValue{Amount: 1000},
		TxOutputDatumHash: &datumHash,
	}

	body := alonzo.AlonzoTransactionBody{
		TxInputs:  inputSet,
		TxOutputs: []alonzo.AlonzoTransactionOutput{output},
		TxFee:     50,
	}

	// Run Utxorpc conversion
	got := body.Utxorpc()

	// Assertion checks
	if got.Fee != 50 {
		t.Errorf("Fee mismatch: got %d, want 50", got.Fee)
	}
	if len(got.Inputs) != 1 {
		t.Errorf("Expected 1 input, got %d", len(got.Inputs))
	}
	if got.Inputs[0].OutputIndex != input.Index() {
		t.Errorf("Input index mismatch: got %d, want %d", got.Inputs[0].OutputIndex, input.Index())
	}
	if len(got.Outputs) != 1 {
		t.Errorf("Expected 1 output, got %d", len(got.Outputs))
	}
	if got.Outputs[0].Coin != 1000 {
		t.Errorf("Output coin mismatch: got %d, want 1000", got.Outputs[0].Coin)
	}
	if len(got.Hash) == 0 {
		t.Error("Expected non-empty transaction hash")
	}
}

// Unit test for AlonzoTransaction.Utxorpc()
func TestAlonzoTransaction_Utxorpc(t *testing.T) {
	// Mock input
	input := shelley.NewShelleyTransactionInput("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1)
	inputSet := shelley.NewShelleyTransactionInputSet([]shelley.ShelleyTransactionInput{input})

	// Mock output
	address := common.Address{}
	datumHash := &common.Blake2b256{0x11, 0x22, 0x33}
	output := alonzo.AlonzoTransactionOutput{
		OutputAddress:     address,
		OutputAmount:      mary.MaryTransactionOutputValue{Amount: 2000},
		TxOutputDatumHash: datumHash,
	}

	body := alonzo.AlonzoTransactionBody{
		TxInputs:  inputSet,
		TxOutputs: []alonzo.AlonzoTransactionOutput{output},
		TxFee:     75,
	}

	// Wrap in transaction
	tx := alonzo.AlonzoTransaction{
		Body:      body,
		TxIsValid: true,
	}

	// Run Utxorpc conversion
	got := tx.Utxorpc()

	// Assertion checks
	if got.Fee != 75 {
		t.Errorf("Transaction fee mismatch: got %d, want 75", got.Fee)
	}
	if len(got.Inputs) != 1 {
		t.Errorf("Expected 1 input, got %d", len(got.Inputs))
	}
	if len(got.Outputs) != 1 {
		t.Errorf("Expected 1 output, got %d", len(got.Outputs))
	}
	if got.Outputs[0].Coin != 2000 {
		t.Errorf("Output coin mismatch: got %d, want 2000", got.Outputs[0].Coin)
	}
	expectedDatum := datumHash.Bytes()
	if !reflect.DeepEqual(got.Outputs[0].Datum.Hash, expectedDatum) {
		t.Errorf("Datum hash mismatch: got %x, want %x", got.Outputs[0].Datum.Hash, expectedDatum)
	}
	if len(got.Hash) == 0 {
		t.Error("Expected non-empty transaction hash")
	}
}
