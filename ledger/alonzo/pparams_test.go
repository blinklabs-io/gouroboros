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
	"math/big"
	"reflect"

	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// Helper to create properly initialized base protocol parameters
func newBaseProtocolParams() mary.MaryProtocolParameters {
	return mary.MaryProtocolParameters{
		AllegraProtocolParameters: allegra.AllegraProtocolParameters{
			ShelleyProtocolParameters: shelley.ShelleyProtocolParameters{
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
			},
		},
	}
}

func TestAlonzoProtocolParametersUpdate(t *testing.T) {
	tests := []struct {
		name        string
		updateCbor  string
		expected    alonzo.AlonzoProtocolParameters
		expectError bool
	}{
		{
			name:       "Update MinPoolCost",
			updateCbor: "a1101903e8", // {16: 1000}
			expected: alonzo.AlonzoProtocolParameters{
				MaryProtocolParameters: newBaseProtocolParams(),
				MinPoolCost:            1000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := hex.DecodeString(tt.updateCbor)
			if err != nil {
				t.Fatalf("failed to decode CBOR: %v", err)
			}

			var update alonzo.AlonzoProtocolParameterUpdate
			if _, err := cbor.Decode(data, &update); err != nil {
				if !tt.expectError {
					t.Fatalf("failed to decode update: %v", err)
				}
				return
			}

			params := alonzo.AlonzoProtocolParameters{
				MaryProtocolParameters: newBaseProtocolParams(),
			}
			params.Update(&update)

			if !reflect.DeepEqual(params, tt.expected) {
				t.Errorf("unexpected result:\ngot: %+v\nwant: %+v", params, tt.expected)
			}
		})
	}
}
func TestAlonzoProtocolParametersUpdateFromGenesis(t *testing.T) {
	tests := []struct {
		name        string
		genesisJSON string
		validate    func(*alonzo.AlonzoProtocolParameters) bool
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
                    "mem": {"numerator": 577, "denominator": 10000},
                    "steps": {"numerator": 721, "denominator": 10000000}
                },
                "costModels": {
                    "PlutusV1": {
                        "param0": 100,
                        "param1": 200,
                        "param2": 300
                    }
                }
            }`,
			validate: func(p *alonzo.AlonzoProtocolParameters) bool {
				return p.AdaPerUtxoByte == 4310 && // 34482 / 8
					p.MaxValueSize == 5000 &&
					p.CollateralPercentage == 150 &&
					p.MaxCollateralInputs == 3 &&
					p.MaxTxExUnits.Memory == 10000000 &&
					p.MaxTxExUnits.Steps == 10000000000 &&
					p.ExecutionCosts.MemPrice.Rat.Cmp(big.NewRat(577, 10000)) == 0 &&
					p.CostModels[alonzo.PlutusV1] != nil &&
					len(p.CostModels[alonzo.PlutusV1].Order) == 3
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary struct with the exact JSON structure
			type TempExecutionPrices struct {
				Mem *struct {
					Numerator   int64 `json:"numerator"`
					Denominator int64 `json:"denominator"`
				} `json:"mem"`
				Steps *struct {
					Numerator   int64 `json:"numerator"`
					Denominator int64 `json:"denominator"`
				} `json:"steps"`
			}

			type TempCostModels map[string]map[string]int

			type TempGenesis struct {
				LovelacePerUtxoWord  uint64 `json:"lovelacePerUTxOWord"`
				MaxValueSize         uint   `json:"maxValueSize"`
				CollateralPercentage uint   `json:"collateralPercentage"`
				MaxCollateralInputs  uint   `json:"maxCollateralInputs"`
				MaxTxExUnits         struct {
					Mem   uint64 `json:"mem"`
					Steps uint64 `json:"steps"`
				} `json:"maxTxExUnits"`
				MaxBlockExUnits struct {
					Mem   uint64 `json:"mem"`
					Steps uint64 `json:"steps"`
				} `json:"maxBlockExUnits"`
				ExecutionPrices TempExecutionPrices `json:"executionPrices"`
				CostModels      TempCostModels      `json:"costModels"`
			}

			var tempGenesis TempGenesis
			err := json.Unmarshal([]byte(tt.genesisJSON), &tempGenesis)
			if err != nil {
				t.Fatalf("failed to parse genesis: %v", err)
			}

			// Convert to the actual AlonzoGenesis type with proper type conversions
			genesis := alonzo.AlonzoGenesis{
				LovelacePerUtxoWord:  tempGenesis.LovelacePerUtxoWord,
				MaxValueSize:         tempGenesis.MaxValueSize,
				CollateralPercentage: tempGenesis.CollateralPercentage,
				MaxCollateralInputs:  tempGenesis.MaxCollateralInputs,
				MaxTxExUnits: alonzo.AlonzoGenesisExUnits{
					Mem:   uint(tempGenesis.MaxTxExUnits.Mem),
					Steps: uint(tempGenesis.MaxTxExUnits.Steps),
				},
				MaxBlockExUnits: alonzo.AlonzoGenesisExUnits{
					Mem:   uint(tempGenesis.MaxBlockExUnits.Mem),
					Steps: uint(tempGenesis.MaxBlockExUnits.Steps),
				},
				ExecutionPrices: alonzo.AlonzoGenesisExecutionPrices{
					Mem:   convertToExecutionPricesRat(tempGenesis.ExecutionPrices.Mem),
					Steps: convertToExecutionPricesRat(tempGenesis.ExecutionPrices.Steps),
				},
				CostModels: convertCostModels(tempGenesis.CostModels),
			}

			params := alonzo.AlonzoProtocolParameters{
				MaryProtocolParameters: newBaseProtocolParams(),
			}
			err = params.UpdateFromGenesis(&genesis)
			if err != nil {
				t.Fatalf("UpdateFromGenesis failed: %v", err)
			}

			if !tt.validate(&params) {
				t.Errorf("validation failed for params: %+v", params)
			}
		})
	}
}


func convertToExecutionPricesRat(r *struct {
	Numerator   int64 `json:"numerator"`
	Denominator int64 `json:"denominator"`
}) *alonzo.AlonzoGenesisExecutionPricesRat {
	if r == nil {
		return nil
	}
	return &alonzo.AlonzoGenesisExecutionPricesRat{
		Rat: big.NewRat(r.Numerator, r.Denominator),
	}
}

func convertCostModels(tempModels map[string]map[string]int) map[string]map[string]int {
	// Create a new map with the correct type
	models := make(map[string]map[string]int)
	for k, v := range tempModels {
		// Create a new inner map
		innerMap := make(map[string]int)
		for k2, v2 := range v {
			innerMap[k2] = v2
		}
		models[k] = innerMap
	}
	return models
}


func TestAlonzoUtxorpc(t *testing.T) {
	params := alonzo.AlonzoProtocolParameters{
		MaryProtocolParameters: newBaseProtocolParams(),
		MinPoolCost:            340000000,
		AdaPerUtxoByte:         4310,
		MaxValueSize:           5000,
		CollateralPercentage:   150,
		MaxCollateralInputs:    3,
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
		CostModels: map[alonzo.PlutusVersion]*alonzo.CostModel{
			alonzo.PlutusV1: {
				Parameters: map[string]int64{"param0": 100, "param1": 200, "param2": 300},
				Order:      []string{"param0", "param1", "param2"},
			},
		},
	}

	result := params.Utxorpc()
	if result == nil {
		t.Fatal("Utxorpc() returned nil")
	}

	// Verify cost models
	if result.CostModels == nil {
		t.Fatal("CostModels should not be nil")
	}
	if result.CostModels.PlutusV1 == nil {
		t.Fatal("PlutusV1 cost model should not be nil")
	}
	if len(result.CostModels.PlutusV1.Values) != 3 {
		t.Errorf("expected 3 cost model values, got %d", len(result.CostModels.PlutusV1.Values))
	}

	// Verify other parameters
	if result.MaxTxSize != uint64(params.MaxTxSize) {
		t.Errorf("incorrect MaxTxSize conversion")
	}
	if result.MinPoolCost != params.MinPoolCost {
		t.Errorf("incorrect MinPoolCost conversion")
	}
	if result.CoinsPerUtxoByte != params.AdaPerUtxoByte {
		t.Errorf("incorrect AdaPerUtxoByte conversion")
	}
}

func TestCostModelConversions(t *testing.T) {
	tests := []struct {
		name     string
		input    map[alonzo.PlutusVersion]*alonzo.CostModel
		expected map[uint][]int64
	}{
		{
			name: "Single Version",
			input: map[alonzo.PlutusVersion]*alonzo.CostModel{
				alonzo.PlutusV1: {
					Parameters: map[string]int64{"param0": 1, "param1": 2},
					Order:      []string{"param0", "param1"},
				},
			},
			expected: map[uint][]int64{
				0: {1, 2},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := alonzo.AlonzoProtocolParameters{
				MaryProtocolParameters: newBaseProtocolParams(),
				CostModels:             tt.input,
			}
			result := params.ToLegacyCostModels()
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("unexpected result:\ngot: %v\nwant: %v", result, tt.expected)
			}
		})
	}
}
