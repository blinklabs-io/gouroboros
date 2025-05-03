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
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	cardano "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
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
			0: completeCostModel(166), // PlutusV1 with exactly 166 parameters
			1: completeCostModel(175), // PlutusV2 with exactly 175 parameters
		},
	}
}

// Helper function to create complete cost models
func completeCostModel(size int) []int64 {
	model := make([]int64, size)
	for i := range model {
		model[i] = int64(i + 1) // Fill with sequential values
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
		{
			startParams: alonzo.AlonzoProtocolParameters{
				MaxBlockBodySize: 1,
				MaxTxExUnits: common.ExUnits{
					Memory: 1,
					Steps:  1,
				},
			},
			updateCbor: "a2021a0001200014821a00aba9501b00000002540be400",
			expectedParams: alonzo.AlonzoProtocolParameters{
				MaxBlockBodySize: 73728,
				MaxTxExUnits: common.ExUnits{
					Memory: 11250000,
					Steps:  10000000000,
				},
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
	// Create cost models in the format the UpdateFromGenesis expects
	plutusV1CostModel := make(map[string]interface{})
	for i := 1; i <= 166; i++ {
		plutusV1CostModel[fmt.Sprintf("param%d", i)] = i
	}

	plutusV2CostModel := make(map[string]interface{})
	for i := 1; i <= 175; i++ {
		plutusV2CostModel[fmt.Sprintf("param%d", i)] = i
	}

	tests := []struct {
		name        string
		genesisJSON string
	}{
		{
			name: "Basic Parameters",
			genesisJSON: `{
                "lovelacePerUTxOWord": 34482,
                "maxValueSize": 5000,
                "collateralPercentage": 150,
                "maxCollateralInputs": 3,
                "maxTxExUnits": {"mem": 10000000, "steps": 10000000000},
                "maxBlockExUnits": {"mem": 50000000, "steps": 40000000000},
                "executionPrices": {
                    "prMem": {"numerator": 577, "denominator": 10000},
                    "prSteps": {"numerator": 721, "denominator": 10000000}
                },
                "costModels": {
                    "PlutusV1": ` + toJSON(plutusV1CostModel) + `,
                    "PlutusV2": ` + toJSON(plutusV2CostModel) + `
                }
            }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var genesis alonzo.AlonzoGenesis
			if err := json.Unmarshal([]byte(tt.genesisJSON), &genesis); err != nil {
				t.Fatalf("failed to parse genesis: %v", err)
			}

			params := newBaseProtocolParams()
			if err := params.UpdateFromGenesis(&genesis); err != nil {
				t.Fatalf("UpdateFromGenesis failed: %v", err)
			}

			if len(params.CostModels[0]) != 166 {
				t.Errorf("expected 166 PlutusV1 parameters, got %d", len(params.CostModels[0]))
			}
			if len(params.CostModels[1]) != 175 {
				t.Errorf("expected 175 PlutusV2 parameters, got %d", len(params.CostModels[1]))
			}
		})
	}
}

func TestCostModelArrayFormat(t *testing.T) {
	// Create a PlutusV1 cost model as an array
	plutusV1Array := make([]int, 166)
	for i := range plutusV1Array {
		plutusV1Array[i] = i + 1
	}

	genesisJSON := fmt.Sprintf(`{
		"lovelacePerUTxOWord": 34482,
		"maxValueSize": 5000,
		"collateralPercentage": 150,
		"maxCollateralInputs": 3,
		"executionPrices": {
			"prMem": {"numerator": 577, "denominator": 10000},
			"prSteps": {"numerator": 721, "denominator": 10000000}
		},
		"maxTxExUnits": {"mem": 10000000, "steps": 10000000000},
		"maxBlockExUnits": {"mem": 50000000, "steps": 40000000000},
		"costModels": {
			"PlutusV1": %s
		}
	}`, toJSON(plutusV1Array))

	var genesis alonzo.AlonzoGenesis
	if err := json.Unmarshal([]byte(genesisJSON), &genesis); err != nil {
		t.Fatalf("failed to unmarshal genesis JSON: %v", err)
	}

	params := alonzo.AlonzoProtocolParameters{}
	if err := params.UpdateFromGenesis(&genesis); err != nil {
		t.Fatalf("UpdateFromGenesis failed: %v", err)
	}

	if len(params.CostModels[alonzo.PlutusV1Key]) != 166 {
		t.Errorf("expected 166 parameters, got %d", len(params.CostModels[alonzo.PlutusV1Key]))
	}

	// Verify first and last values
	if params.CostModels[alonzo.PlutusV1Key][0] != 1 {
		t.Errorf("expected first parameter to be 1, got %d", params.CostModels[alonzo.PlutusV1Key][0])
	}
	if params.CostModels[alonzo.PlutusV1Key][165] != 166 {
		t.Errorf("expected last parameter to be 166, got %d", params.CostModels[alonzo.PlutusV1Key][165])
	}
}

func TestScientificNotationInCostModels(t *testing.T) {
	// Create a full cost model with 166 parameters, using scientific notation for some
	costModel := make(map[string]interface{})
	for i := 1; i <= 166; i++ {
		switch i {
		case 1:
			costModel[fmt.Sprintf("param%d", i)] = 2.477736e+06
		case 2:
			costModel[fmt.Sprintf("param%d", i)] = 1.5e6
		case 3:
			costModel[fmt.Sprintf("param%d", i)] = 1000000
		default:
			costModel[fmt.Sprintf("param%d", i)] = i * 1000
		}
	}

	genesisJSON := fmt.Sprintf(`{
        "lovelacePerUTxOWord": 34482,
        "maxValueSize": 5000,
        "collateralPercentage": 150,
        "maxCollateralInputs": 3,
        "executionPrices": {
            "prMem": {"numerator": 577, "denominator": 10000},
            "prSteps": {"numerator": 721, "denominator": 10000000}
        },
        "maxTxExUnits": {"mem": 10000000, "steps": 10000000000},
        "maxBlockExUnits": {"mem": 50000000, "steps": 40000000000},
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

	// Verify the scientific notation conversions
	expected := []int64{2477736, 1500000, 1000000}
	for i := 0; i < 3; i++ {
		if params.CostModels[alonzo.PlutusV1Key][i] != expected[i] {
			t.Errorf("parameter %d conversion failed: got %d, want %d",
				i+1, params.CostModels[alonzo.PlutusV1Key][i], expected[i])
		}
	}

	// Verify we have all 166 parameters
	if len(params.CostModels[alonzo.PlutusV1Key]) != 166 {
		t.Errorf("expected 166 parameters, got %d", len(params.CostModels[alonzo.PlutusV1Key]))
	}
}

func TestInvalidCostModelFormats(t *testing.T) {
	baseJSON := `{
        "lovelacePerUTxOWord": 34482,
        "maxValueSize": 5000,
        "collateralPercentage": 150,
        "maxCollateralInputs": 3,
        "executionPrices": {
            "prMem": {"numerator": 577, "denominator": 10000},
            "prSteps": {"numerator": 721, "denominator": 10000000}
        },
        "maxTxExUnits": {"mem": 10000000, "steps": 10000000000},
        "maxBlockExUnits": {"mem": 50000000, "steps": 40000000000},
        %s
    }`

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
			expectError: "invalid cost model format",
		},
		{
			name: "ShortArray",
			costModels: `"costModels": {
                "PlutusV1": [1, 2, 3]
            }`,
			expectError: "expected 166, got 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fullJSON := fmt.Sprintf(baseJSON, tt.costModels)

			var genesis alonzo.AlonzoGenesis
			if err := json.Unmarshal([]byte(fullJSON), &genesis); err != nil {
				t.Fatalf("failed to unmarshal genesis: %v", err)
			}

			params := alonzo.AlonzoProtocolParameters{}
			err := params.UpdateFromGenesis(&genesis)
			if err == nil {
				t.Fatal("expected error but got none")
			}
			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %v", tt.expectError, err)
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

func verifyCostModel(t *testing.T, models map[string]interface{}, name string, expectedCount int) {
	cm, ok := models[name].(map[string]interface{})
	if !ok {
		t.Fatalf("%s cost model not found or wrong type", name)
	}
	if len(cm) != expectedCount {
		t.Fatalf("%s parameter count mismatch: got %d, want %d", name, len(cm), expectedCount)
	}
}

func mustMarshalJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal JSON: %v", err))
	}
	return string(b)
}

func jsonStringFromMap(m map[string]int64) string {
	b, _ := json.Marshal(m)
	return string(b)
}
