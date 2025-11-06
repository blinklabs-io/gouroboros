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
	"github.com/stretchr/testify/assert"
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
	for i := range 166 {
		plutusV1CostModel[strconv.Itoa(i)] = i + 1 // "0":1, "1":2, etc.
	}

	plutusV2CostModel := make(map[string]int)
	for i := range 175 {
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
	for i := range 166 {
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
	costModel := map[string]any{
		"0": 2.477736e+06, // Changed from param1 to 0
		"1": 1.5e6,        // Changed from param2 to 1
		"2": 1000000,      // Changed from param3 to 2
	}
	// Fill remaining parameters
	for i := 3; i < 166; i++ {
		costModel[`test`+strconv.Itoa(i)] = i * 1000
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
	if params.CostModels == nil ||
		params.CostModels[alonzo.PlutusV1Key] == nil {
		t.Fatal("CostModels not properly initialized")
	}
	for i := range 3 {
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
			expectError: "incorrect param count for PlutusV1: 3",
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

	expectedUtxorpc := &utxorpc.PParams{
		CoinsPerUtxoByte: common.ToUtxorpcBigInt(44 / 8),
		MaxTxSize: 16384,
		MinFeeCoefficient: common.ToUtxorpcBigInt(500),
		MinFeeConstant: common.ToUtxorpcBigInt(2),
		MaxBlockBodySize:   65536,
		MaxBlockHeaderSize: 1024,
		StakeKeyDeposit: common.ToUtxorpcBigInt(2000),
		PoolDeposit: common.ToUtxorpcBigInt(500000),
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
		MinPoolCost: common.ToUtxorpcBigInt(340000000),
		ProtocolVersion: &utxorpc.ProtocolVersion{
			Major: 8,
			Minor: 0,
		},
		MaxValueSize:         1024,
		CollateralPercentage: 150,
		MaxCollateralInputs:  5,
		CostModels: &utxorpc.CostModels{
			PlutusV1: &utxorpc.CostModel{
				Values: []int64{100, 200, 300},
			},
			PlutusV2: &utxorpc.CostModel{
				Values: []int64{400, 500, 600},
			},
			PlutusV3: &utxorpc.CostModel{
				Values: []int64{700, 800, 900},
			},
		},
		Prices: &utxorpc.ExPrices{
			Memory: &utxorpc.RationalNumber{
				Numerator:   int32(1),
				Denominator: uint32(2),
			},
			Steps: &utxorpc.RationalNumber{
				Numerator:   int32(2),
				Denominator: uint32(3),
			},
		},
		MaxExecutionUnitsPerTransaction: &utxorpc.ExUnits{
			Memory: 1000000,
			Steps:  200000,
		},
		MaxExecutionUnitsPerBlock: &utxorpc.ExUnits{
			Memory: 5000000,
			Steps:  1000000,
		},
	}

	result, err := inputParams.Utxorpc()
	if err != nil {
		t.Fatalf("Utxorpc() conversion failed")
	}

	if !reflect.DeepEqual(result, expectedUtxorpc) {
		t.Fatalf(
			"Utxorpc() test failed for Alonzo:\nExpected: %#v\nGot: %#v",
			expectedUtxorpc,
			result,
		)
	}
}

func toJSON(v any) string {
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
		OutputAddress:   address,
		OutputAmount:    mary.MaryTransactionOutputValue{Amount: amount},
		OutputDatumHash: &datumHash,
	}

	got, err := output.Utxorpc()
	assert.NoError(t, err)
	addr, err := address.Bytes()
	assert.NoError(t, err)
	want := &utxorpc.TxOutput{
		Address: addr,
		Coin:    common.ToUtxorpcBigInt(amount),
		Assets:  nil,
		Datum: &utxorpc.Datum{
			Hash: datumHash.Bytes(),
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf(
			"AlonzoTransactionOutput.Utxorpc() mismatch\nGot: %+v\nWant: %+v",
			got,
			want,
		)
	}
}

// Unit test for AlonzoTransactionBody.Utxorpc()
func TestAlonzoTransactionBody_Utxorpc(t *testing.T) {
	// Mock input
	input := shelley.NewShelleyTransactionInput(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		0,
	)
	inputSet := shelley.NewShelleyTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)

	// Mock output
	address := common.Address{}
	datumHash := common.Blake2b256{1, 2, 3, 4}
	output := alonzo.AlonzoTransactionOutput{
		OutputAddress:   address,
		OutputAmount:    mary.MaryTransactionOutputValue{Amount: 1000},
		OutputDatumHash: &datumHash,
	}

	body := alonzo.AlonzoTransactionBody{
		TxInputs:  inputSet,
		TxOutputs: []alonzo.AlonzoTransactionOutput{output},
		TxFee:     50,
	}

	// Run Utxorpc conversion
	got, err := body.Utxorpc()
	if err != nil {
		t.Fatalf(
			"Could not convert transaction body to utxorpc format: %v",
			err,
		)
	}

	// Assertion checks
	if got.Fee.GetInt() != 50 {
		t.Errorf("Fee mismatch: got %d, want 50", got.Fee.GetInt())
	}
	if len(got.Inputs) != 1 {
		t.Errorf("Expected 1 input, got %d", len(got.Inputs))
	}
	if got.Inputs[0].OutputIndex != input.Index() {
		t.Errorf(
			"Input index mismatch: got %d, want %d",
			got.Inputs[0].OutputIndex,
			input.Index(),
		)
	}
	if len(got.Outputs) != 1 {
		t.Fatalf("Expected 1 output, got %d", len(got.Outputs))
	}
	var coinValue uint64
	coin := got.Outputs[0].Coin
	if bigUInt := coin.GetBigUInt(); bigUInt != nil {
		coinValue = new(big.Int).SetBytes(bigUInt).Uint64()
	} else {
		coinValue = uint64(coin.GetInt())
	}
	if coinValue != uint64(1000) {
		t.Errorf(
			"Output coin mismatch: got %d, want 1000",
			coinValue,
		)
	}
	if len(got.Hash) == 0 {
		t.Error("Expected non-empty transaction hash")
	}
}

// Unit test for AlonzoTransaction.Utxorpc()
func TestAlonzoTransaction_Utxorpc(t *testing.T) {
	// Mock input
	input := shelley.NewShelleyTransactionInput(
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		1,
	)
	inputSet := shelley.NewShelleyTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)

	// Mock output
	address := common.Address{}
	datumHash := &common.Blake2b256{0x11, 0x22, 0x33}
	output := alonzo.AlonzoTransactionOutput{
		OutputAddress:   address,
		OutputAmount:    mary.MaryTransactionOutputValue{Amount: 2000},
		OutputDatumHash: datumHash,
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
	got, err := tx.Utxorpc()
	if err != nil {
		t.Fatalf("Could not convert transaction to utxorpc format: %v", err)
	}

	// Assertion checks
	if got.Fee.GetInt() != 75 {
		t.Errorf("Transaction fee mismatch: got %d, want 75", got.Fee.GetInt())
	}
	if len(got.Inputs) != 1 {
		t.Errorf("Expected 1 input, got %d", len(got.Inputs))
	}
	if len(got.Outputs) != 1 {
		t.Fatalf("Expected 1 output, got %d", len(got.Outputs))
	}
	var coinValue uint64
	coin := got.Outputs[0].Coin
	if bigUInt := coin.GetBigUInt(); bigUInt != nil {
		coinValue = new(big.Int).SetBytes(bigUInt).Uint64()
	} else {
		coinValue = uint64(coin.GetInt())
	}
	if coinValue != uint64(2000) {
		t.Errorf(
			"Output coin mismatch: got %d, want 2000",
			coinValue,
		)
	}

	expectedDatum := datumHash.Bytes()
	if !reflect.DeepEqual(got.Outputs[0].Datum.Hash, expectedDatum) {
		t.Errorf(
			"Datum hash mismatch: got %x, want %x",
			got.Outputs[0].Datum.Hash,
			expectedDatum,
		)
	}
	if len(got.Hash) == 0 {
		t.Error("Expected non-empty transaction hash")
	}
}

// ===================================================================
// Tests for UpgradePParams with MinPoolCost
// ===================================================================

func TestAlonzoUpgradePParams(t *testing.T) {
	testCases := []struct {
		name       string
		prevParams mary.MaryProtocolParameters
		validate   func(*testing.T, alonzo.AlonzoProtocolParameters)
	}{
		{
			name: "Upgrade with MinPoolCost zero",
			prevParams: mary.MaryProtocolParameters{
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
				MinPoolCost:        0,
			},
			validate: func(t *testing.T, alonzoParams alonzo.AlonzoProtocolParameters) {
				if alonzoParams.MinPoolCost != 0 {
					t.Errorf(
						"MinPoolCost: got %d, want 0",
						alonzoParams.MinPoolCost,
					)
				}
				if alonzoParams.MinFeeA != 44 {
					t.Errorf("MinFeeA: got %d, want 44", alonzoParams.MinFeeA)
				}
				if alonzoParams.ProtocolMajor != 6 {
					t.Errorf(
						"ProtocolMajor: got %d, want 6",
						alonzoParams.ProtocolMajor,
					)
				}
			},
		},
		{
			name: "Upgrade with MinPoolCost standard value",
			prevParams: mary.MaryProtocolParameters{
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
				MinPoolCost:        340000000,
			},
			validate: func(t *testing.T, alonzoParams alonzo.AlonzoProtocolParameters) {
				if alonzoParams.MinPoolCost != 340000000 {
					t.Errorf(
						"MinPoolCost: got %d, want 340000000",
						alonzoParams.MinPoolCost,
					)
				}
				if alonzoParams.MinFeeB != 155381 {
					t.Errorf(
						"MinFeeB: got %d, want 155381",
						alonzoParams.MinFeeB,
					)
				}
			},
		},
		{
			name: "Upgrade with MinPoolCost maximum uint64",
			prevParams: mary.MaryProtocolParameters{
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
				ProtocolMinor:      2,
				MinUtxoValue:       500000,
				MinPoolCost:        18446744073709551615,
			},
			validate: func(t *testing.T, alonzoParams alonzo.AlonzoProtocolParameters) {
				if alonzoParams.MinPoolCost != 18446744073709551615 {
					t.Errorf(
						"MinPoolCost: got %d, want 18446744073709551615",
						alonzoParams.MinPoolCost,
					)
				}
				if alonzoParams.ProtocolMinor != 2 {
					t.Errorf(
						"ProtocolMinor: got %d, want 2",
						alonzoParams.ProtocolMinor,
					)
				}
			},
		},
		{
			name: "Upgrade with all Mary fields",
			prevParams: mary.MaryProtocolParameters{
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
				MinPoolCost:        340000000,
			},
			validate: func(t *testing.T, alonzoParams alonzo.AlonzoProtocolParameters) {
				if alonzoParams.MinFeeA != 44 {
					t.Errorf("MinFeeA: got %d, want 44", alonzoParams.MinFeeA)
				}
				if alonzoParams.MinFeeB != 155381 {
					t.Errorf(
						"MinFeeB: got %d, want 155381",
						alonzoParams.MinFeeB,
					)
				}
				if alonzoParams.MaxBlockBodySize != 65536 {
					t.Errorf(
						"MaxBlockBodySize: got %d, want 65536",
						alonzoParams.MaxBlockBodySize,
					)
				}
				if alonzoParams.MaxTxSize != 16384 {
					t.Errorf(
						"MaxTxSize: got %d, want 16384",
						alonzoParams.MaxTxSize,
					)
				}
				if alonzoParams.MaxBlockHeaderSize != 1100 {
					t.Errorf(
						"MaxBlockHeaderSize: got %d, want 1100",
						alonzoParams.MaxBlockHeaderSize,
					)
				}
				if alonzoParams.KeyDeposit != 2000000 {
					t.Errorf(
						"KeyDeposit: got %d, want 2000000",
						alonzoParams.KeyDeposit,
					)
				}
				if alonzoParams.PoolDeposit != 500000000 {
					t.Errorf(
						"PoolDeposit: got %d, want 500000000",
						alonzoParams.PoolDeposit,
					)
				}
				if alonzoParams.MaxEpoch != 18 {
					t.Errorf("MaxEpoch: got %d, want 18", alonzoParams.MaxEpoch)
				}
				if alonzoParams.NOpt != 500 {
					t.Errorf("NOpt: got %d, want 500", alonzoParams.NOpt)
				}
				if alonzoParams.A0 == nil ||
					alonzoParams.A0.Cmp(big.NewRat(3, 10)) != 0 {
					t.Error("A0 not correctly copied")
				}
				if alonzoParams.Rho == nil ||
					alonzoParams.Rho.Cmp(big.NewRat(3, 1000)) != 0 {
					t.Error("Rho not correctly copied")
				}
				if alonzoParams.Tau == nil ||
					alonzoParams.Tau.Cmp(big.NewRat(2, 10)) != 0 {
					t.Error("Tau not correctly copied")
				}
				if alonzoParams.Decentralization == nil ||
					alonzoParams.Decentralization.Cmp(big.NewRat(1, 2)) != 0 {
					t.Error("Decentralization not correctly copied")
				}
				if alonzoParams.ProtocolMajor != 6 {
					t.Errorf(
						"ProtocolMajor: got %d, want 6",
						alonzoParams.ProtocolMajor,
					)
				}
				if alonzoParams.ProtocolMinor != 0 {
					t.Errorf(
						"ProtocolMinor: got %d, want 0",
						alonzoParams.ProtocolMinor,
					)
				}
				if alonzoParams.MinUtxoValue != 1000000 {
					t.Errorf(
						"MinUtxoValue: got %d, want 1000000",
						alonzoParams.MinUtxoValue,
					)
				}
				if alonzoParams.MinPoolCost != 340000000 {
					t.Errorf(
						"MinPoolCost: got %d, want 340000000",
						alonzoParams.MinPoolCost,
					)
				}
			},
		},
		{
			name: "Upgrade with nil rational numbers",
			prevParams: mary.MaryProtocolParameters{
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
				MinPoolCost:        123456789,
			},
			validate: func(t *testing.T, alonzoParams alonzo.AlonzoProtocolParameters) {
				if alonzoParams.A0 != nil {
					t.Error("A0 should be nil")
				}
				if alonzoParams.Rho != nil {
					t.Error("Rho should be nil")
				}
				if alonzoParams.Tau != nil {
					t.Error("Tau should be nil")
				}
				if alonzoParams.Decentralization != nil {
					t.Error("Decentralization should be nil")
				}
				if alonzoParams.MinPoolCost != 123456789 {
					t.Errorf(
						"MinPoolCost: got %d, want 123456789",
						alonzoParams.MinPoolCost,
					)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			alonzoParams := alonzo.UpgradePParams(tc.prevParams)
			tc.validate(t, alonzoParams)
		})
	}
}

func TestAlonzoUpgradePParams_PointerIdentity(t *testing.T) {
	// Test that pointers to rational numbers are correctly copied (not shared)
	prevParams := mary.MaryProtocolParameters{
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
		MinPoolCost:        340000000,
	}

	alonzoParams := alonzo.UpgradePParams(prevParams)

	// Verify that the pointers are the same (direct copy)
	if alonzoParams.A0 != prevParams.A0 {
		t.Error("A0 pointer should be the same")
	}
	if alonzoParams.Rho != prevParams.Rho {
		t.Error("Rho pointer should be the same")
	}
	if alonzoParams.Tau != prevParams.Tau {
		t.Error("Tau pointer should be the same")
	}
	if alonzoParams.Decentralization != prevParams.Decentralization {
		t.Error("Decentralization pointer should be the same")
	}
}

func TestAlonzoUpgradePParams_AlonzoSpecificFieldsZero(t *testing.T) {
	// Verify that Alonzo-specific fields are initialized to zero values
	prevParams := mary.MaryProtocolParameters{
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
		MinPoolCost:        340000000,
	}

	alonzoParams := alonzo.UpgradePParams(prevParams)

	// Alonzo-specific fields should be zero/nil
	if alonzoParams.AdaPerUtxoByte != 0 {
		t.Errorf(
			"AdaPerUtxoByte should be 0, got %d",
			alonzoParams.AdaPerUtxoByte,
		)
	}
	if alonzoParams.CostModels != nil {
		t.Error("CostModels should be nil")
	}
	if alonzoParams.ExecutionCosts.MemPrice != nil {
		t.Error("ExecutionCosts.MemPrice should be nil")
	}
	if alonzoParams.ExecutionCosts.StepPrice != nil {
		t.Error("ExecutionCosts.StepPrice should be nil")
	}
	if alonzoParams.MaxTxExUnits.Memory != 0 {
		t.Errorf(
			"MaxTxExUnits.Memory should be 0, got %d",
			alonzoParams.MaxTxExUnits.Memory,
		)
	}
	if alonzoParams.MaxTxExUnits.Steps != 0 {
		t.Errorf(
			"MaxTxExUnits.Steps should be 0, got %d",
			alonzoParams.MaxTxExUnits.Steps,
		)
	}
	if alonzoParams.MaxBlockExUnits.Memory != 0 {
		t.Errorf(
			"MaxBlockExUnits.Memory should be 0, got %d",
			alonzoParams.MaxBlockExUnits.Memory,
		)
	}
	if alonzoParams.MaxBlockExUnits.Steps != 0 {
		t.Errorf(
			"MaxBlockExUnits.Steps should be 0, got %d",
			alonzoParams.MaxBlockExUnits.Steps,
		)
	}
	if alonzoParams.MaxValueSize != 0 {
		t.Errorf("MaxValueSize should be 0, got %d", alonzoParams.MaxValueSize)
	}
	if alonzoParams.CollateralPercentage != 0 {
		t.Errorf(
			"CollateralPercentage should be 0, got %d",
			alonzoParams.CollateralPercentage,
		)
	}
	if alonzoParams.MaxCollateralInputs != 0 {
		t.Errorf(
			"MaxCollateralInputs should be 0, got %d",
			alonzoParams.MaxCollateralInputs,
		)
	}
}

func TestAlonzoUpgradePParams_CompareWithExistingParams(t *testing.T) {
	// Create Mary params that match existing test patterns
	prevParams := mary.MaryProtocolParameters{
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
		MinUtxoValue:       1000000,
		MinPoolCost:        340000000,
	}

	alonzoParams := alonzo.UpgradePParams(prevParams)

	if alonzoParams.MinFeeA != prevParams.MinFeeA {
		t.Error("MinFeeA mismatch after upgrade")
	}
	if alonzoParams.MinFeeB != prevParams.MinFeeB {
		t.Error("MinFeeB mismatch after upgrade")
	}
	if alonzoParams.MinPoolCost != prevParams.MinPoolCost {
		t.Errorf(
			"MinPoolCost mismatch: got %d, want %d",
			alonzoParams.MinPoolCost,
			prevParams.MinPoolCost,
		)
	}

	// Verify Alonzo-specific fields are zero (not from Mary)
	if alonzoParams.AdaPerUtxoByte != 0 {
		t.Error("AdaPerUtxoByte should be zero after upgrade, not base value")
	}
}
