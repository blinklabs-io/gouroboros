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
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

const alonzoGenesisConfig = `
{
    "lovelacePerUTxOWord": 34482,
    "executionPrices": {
        "prSteps":
	{
	    "numerator" :   721,
	    "denominator" : 10000000
		},
        "prMem":
	{
	    "numerator" :   577,
	    "denominator" : 10000
	}
    },
    "maxTxExUnits": {
        "exUnitsMem":   10000000,
        "exUnitsSteps": 10000000000
    },
    "maxBlockExUnits": {
        "exUnitsMem":   50000000,
        "exUnitsSteps": 40000000000
    },
    "maxValueSize": 5000,
    "collateralPercentage": 150,
    "maxCollateralInputs": 3,
    "costModels": {
        "PlutusV1": {
            "sha2_256-memory-arguments": 4,
            "equalsString-cpu-arguments-constant": 1000,
            "cekDelayCost-exBudgetMemory": 100,
            "lessThanEqualsByteString-cpu-arguments-intercept": 103599,
            "divideInteger-memory-arguments-minimum": 1,
            "appendByteString-cpu-arguments-slope": 621,
            "blake2b-cpu-arguments-slope": 29175,
            "iData-cpu-arguments": 150000,
            "encodeUtf8-cpu-arguments-slope": 1000,
            "unBData-cpu-arguments": 150000,
            "multiplyInteger-cpu-arguments-intercept": 61516,
            "cekConstCost-exBudgetMemory": 100,
            "nullList-cpu-arguments": 150000,
            "equalsString-cpu-arguments-intercept": 150000,
            "trace-cpu-arguments": 150000,
            "mkNilData-memory-arguments": 32,
            "lengthOfByteString-cpu-arguments": 150000,
            "cekBuiltinCost-exBudgetCPU": 29773,
            "bData-cpu-arguments": 150000,
            "subtractInteger-cpu-arguments-slope": 0,
            "unIData-cpu-arguments": 150000,
            "consByteString-memory-arguments-intercept": 0,
            "divideInteger-memory-arguments-slope": 1,
            "divideInteger-cpu-arguments-model-arguments-slope": 118,
            "listData-cpu-arguments": 150000,
            "headList-cpu-arguments": 150000,
            "chooseData-memory-arguments": 32,
            "equalsInteger-cpu-arguments-intercept": 136542,
            "sha3_256-cpu-arguments-slope": 82363,
            "sliceByteString-cpu-arguments-slope": 5000,
            "unMapData-cpu-arguments": 150000,
            "lessThanInteger-cpu-arguments-intercept": 179690,
            "mkCons-cpu-arguments": 150000,
            "appendString-memory-arguments-intercept": 0,
            "modInteger-cpu-arguments-model-arguments-slope": 118,
            "ifThenElse-cpu-arguments": 1,
            "mkNilPairData-cpu-arguments": 150000,
            "lessThanEqualsInteger-cpu-arguments-intercept": 145276,
            "addInteger-memory-arguments-slope": 1,
            "chooseList-memory-arguments": 32,
            "constrData-memory-arguments": 32,
            "decodeUtf8-cpu-arguments-intercept": 150000,
            "equalsData-memory-arguments": 1,
            "subtractInteger-memory-arguments-slope": 1,
            "appendByteString-memory-arguments-intercept": 0,
            "lengthOfByteString-memory-arguments": 4,
            "headList-memory-arguments": 32,
            "listData-memory-arguments": 32,
            "consByteString-cpu-arguments-intercept": 150000,
            "unIData-memory-arguments": 32,
            "remainderInteger-memory-arguments-minimum": 1,
            "bData-memory-arguments": 32,
            "lessThanByteString-cpu-arguments-slope": 248,
            "encodeUtf8-memory-arguments-intercept": 0,
            "cekStartupCost-exBudgetCPU": 100,
            "multiplyInteger-memory-arguments-intercept": 0,
            "unListData-memory-arguments": 32,
            "remainderInteger-cpu-arguments-model-arguments-slope": 118,
            "cekVarCost-exBudgetCPU": 29773,
            "remainderInteger-memory-arguments-slope": 1,
            "cekForceCost-exBudgetCPU": 29773,
            "sha2_256-cpu-arguments-slope": 29175,
            "equalsInteger-memory-arguments": 1,
            "indexByteString-memory-arguments": 1,
            "addInteger-memory-arguments-intercept": 1,
            "chooseUnit-cpu-arguments": 150000,
            "sndPair-cpu-arguments": 150000,
            "cekLamCost-exBudgetCPU": 29773,
            "fstPair-cpu-arguments": 150000,
            "quotientInteger-memory-arguments-minimum": 1,
            "decodeUtf8-cpu-arguments-slope": 1000,
            "lessThanInteger-memory-arguments": 1,
            "lessThanEqualsInteger-cpu-arguments-slope": 1366,
            "fstPair-memory-arguments": 32,
            "modInteger-memory-arguments-intercept": 0,
            "unConstrData-cpu-arguments": 150000,
            "lessThanEqualsInteger-memory-arguments": 1,
            "chooseUnit-memory-arguments": 32,
            "sndPair-memory-arguments": 32,
            "addInteger-cpu-arguments-intercept": 197209,
            "decodeUtf8-memory-arguments-slope": 8,
            "equalsData-cpu-arguments-intercept": 150000,
            "mapData-cpu-arguments": 150000,
            "mkPairData-cpu-arguments": 150000,
            "quotientInteger-cpu-arguments-constant": 148000,
            "consByteString-memory-arguments-slope": 1,
            "cekVarCost-exBudgetMemory": 100,
            "indexByteString-cpu-arguments": 150000,
            "unListData-cpu-arguments": 150000,
            "equalsInteger-cpu-arguments-slope": 1326,
            "cekStartupCost-exBudgetMemory": 100,
            "subtractInteger-cpu-arguments-intercept": 197209,
            "divideInteger-cpu-arguments-model-arguments-intercept": 425507,
            "divideInteger-memory-arguments-intercept": 0,
            "cekForceCost-exBudgetMemory": 100,
            "blake2b-cpu-arguments-intercept": 2477736,
            "remainderInteger-cpu-arguments-constant": 148000,
            "tailList-cpu-arguments": 150000,
            "encodeUtf8-cpu-arguments-intercept": 150000,
            "equalsString-cpu-arguments-slope": 1000,
            "lessThanByteString-memory-arguments": 1,
            "multiplyInteger-cpu-arguments-slope": 11218,
            "appendByteString-cpu-arguments-intercept": 396231,
            "lessThanEqualsByteString-cpu-arguments-slope": 248,
            "modInteger-memory-arguments-slope": 1,
            "addInteger-cpu-arguments-slope": 0,
            "equalsData-cpu-arguments-slope": 10000,
            "decodeUtf8-memory-arguments-intercept": 0,
            "chooseList-cpu-arguments": 150000,
            "constrData-cpu-arguments": 150000,
            "equalsByteString-memory-arguments": 1,
            "cekApplyCost-exBudgetCPU": 29773,
            "quotientInteger-memory-arguments-slope": 1,
            "verifySignature-cpu-arguments-intercept": 3345831,
            "unMapData-memory-arguments": 32,
            "mkCons-memory-arguments": 32,
            "sliceByteString-memory-arguments-slope": 1,
            "sha3_256-memory-arguments": 4,
            "ifThenElse-memory-arguments": 1,
            "mkNilPairData-memory-arguments": 32,
            "equalsByteString-cpu-arguments-slope": 247,
            "appendString-cpu-arguments-intercept": 150000,
            "quotientInteger-cpu-arguments-model-arguments-slope": 118,
            "cekApplyCost-exBudgetMemory": 100,
            "equalsString-memory-arguments": 1,
            "multiplyInteger-memory-arguments-slope": 1,
            "cekBuiltinCost-exBudgetMemory": 100,
            "remainderInteger-memory-arguments-intercept": 0,
            "sha2_256-cpu-arguments-intercept": 2477736,
            "remainderInteger-cpu-arguments-model-arguments-intercept": 425507,
            "lessThanEqualsByteString-memory-arguments": 1,
            "tailList-memory-arguments": 32,
            "mkNilData-cpu-arguments": 150000,
            "chooseData-cpu-arguments": 150000,
            "unBData-memory-arguments": 32,
            "blake2b-memory-arguments": 4,
            "iData-memory-arguments": 32,
            "nullList-memory-arguments": 32,
            "cekDelayCost-exBudgetCPU": 29773,
            "subtractInteger-memory-arguments-intercept": 1,
            "lessThanByteString-cpu-arguments-intercept": 103599,
            "consByteString-cpu-arguments-slope": 1000,
            "appendByteString-memory-arguments-slope": 1,
            "trace-memory-arguments": 32,
            "divideInteger-cpu-arguments-constant": 148000,
            "cekConstCost-exBudgetCPU": 29773,
            "encodeUtf8-memory-arguments-slope": 8,
            "quotientInteger-cpu-arguments-model-arguments-intercept": 425507,
            "mapData-memory-arguments": 32,
            "appendString-cpu-arguments-slope": 1000,
            "modInteger-cpu-arguments-constant": 148000,
            "verifySignature-cpu-arguments-slope": 1,
            "unConstrData-memory-arguments": 32,
            "quotientInteger-memory-arguments-intercept": 0,
            "equalsByteString-cpu-arguments-constant": 150000,
            "sliceByteString-memory-arguments-intercept": 0,
            "mkPairData-memory-arguments": 32,
            "equalsByteString-cpu-arguments-intercept": 112536,
            "appendString-memory-arguments-slope": 1,
            "lessThanInteger-cpu-arguments-slope": 497,
            "modInteger-cpu-arguments-model-arguments-intercept": 425507,
            "modInteger-memory-arguments-minimum": 1,
            "sha3_256-cpu-arguments-intercept": 0,
            "verifySignature-memory-arguments": 1,
            "cekLamCost-exBudgetMemory": 100,
            "sliceByteString-cpu-arguments-intercept": 150000
        }
    }
}
`

var expectedGenesisObj = alonzo.AlonzoGenesis{
	LovelacePerUtxoWord:  34482,
	MaxValueSize:         5000,
	CollateralPercentage: 150,
	MaxCollateralInputs:  3,
	ExecutionPrices: alonzo.AlonzoGenesisExecutionPrices{
		Mem: &common.GenesisRat{
			Rat: big.NewRat(577, 10000),
		},
		Steps: &common.GenesisRat{
			Rat: big.NewRat(721, 10000000),
		},
	},
	MaxTxExUnits: alonzo.AlonzoGenesisExUnits{
		Mem:   10000000,
		Steps: 10000000000,
	},
	MaxBlockExUnits: alonzo.AlonzoGenesisExUnits{
		Mem:   50000000,
		Steps: 40000000000,
	},
	CostModels: map[string]alonzo.CostModel{
		"PlutusV1": map[string]int{
			"addInteger-cpu-arguments-intercept":                       197209,
			"addInteger-cpu-arguments-slope":                           0,
			"addInteger-memory-arguments-intercept":                    1,
			"addInteger-memory-arguments-slope":                        1,
			"appendByteString-cpu-arguments-intercept":                 396231,
			"appendByteString-cpu-arguments-slope":                     621,
			"appendByteString-memory-arguments-intercept":              0,
			"appendByteString-memory-arguments-slope":                  1,
			"appendString-cpu-arguments-intercept":                     150000,
			"appendString-cpu-arguments-slope":                         1000,
			"appendString-memory-arguments-intercept":                  0,
			"appendString-memory-arguments-slope":                      1,
			"bData-cpu-arguments":                                      150000,
			"bData-memory-arguments":                                   32,
			"blake2b-cpu-arguments-intercept":                          2477736,
			"blake2b-cpu-arguments-slope":                              29175,
			"blake2b-memory-arguments":                                 4,
			"cekApplyCost-exBudgetCPU":                                 29773,
			"cekApplyCost-exBudgetMemory":                              100,
			"cekBuiltinCost-exBudgetCPU":                               29773,
			"cekBuiltinCost-exBudgetMemory":                            100,
			"cekConstCost-exBudgetCPU":                                 29773,
			"cekConstCost-exBudgetMemory":                              100,
			"cekDelayCost-exBudgetCPU":                                 29773,
			"cekDelayCost-exBudgetMemory":                              100,
			"cekForceCost-exBudgetCPU":                                 29773,
			"cekForceCost-exBudgetMemory":                              100,
			"cekLamCost-exBudgetCPU":                                   29773,
			"cekLamCost-exBudgetMemory":                                100,
			"cekStartupCost-exBudgetCPU":                               100,
			"cekStartupCost-exBudgetMemory":                            100,
			"cekVarCost-exBudgetCPU":                                   29773,
			"cekVarCost-exBudgetMemory":                                100,
			"chooseData-cpu-arguments":                                 150000,
			"chooseData-memory-arguments":                              32,
			"chooseList-cpu-arguments":                                 150000,
			"chooseList-memory-arguments":                              32,
			"chooseUnit-cpu-arguments":                                 150000,
			"chooseUnit-memory-arguments":                              32,
			"consByteString-cpu-arguments-intercept":                   150000,
			"consByteString-cpu-arguments-slope":                       1000,
			"consByteString-memory-arguments-intercept":                0,
			"consByteString-memory-arguments-slope":                    1,
			"constrData-cpu-arguments":                                 150000,
			"constrData-memory-arguments":                              32,
			"decodeUtf8-cpu-arguments-intercept":                       150000,
			"decodeUtf8-cpu-arguments-slope":                           1000,
			"decodeUtf8-memory-arguments-intercept":                    0,
			"decodeUtf8-memory-arguments-slope":                        8,
			"divideInteger-cpu-arguments-constant":                     148000,
			"divideInteger-cpu-arguments-model-arguments-intercept":    425507,
			"divideInteger-cpu-arguments-model-arguments-slope":        118,
			"divideInteger-memory-arguments-intercept":                 0,
			"divideInteger-memory-arguments-minimum":                   1,
			"divideInteger-memory-arguments-slope":                     1,
			"encodeUtf8-cpu-arguments-intercept":                       150000,
			"encodeUtf8-cpu-arguments-slope":                           1000,
			"encodeUtf8-memory-arguments-intercept":                    0,
			"encodeUtf8-memory-arguments-slope":                        8,
			"equalsByteString-cpu-arguments-constant":                  150000,
			"equalsByteString-cpu-arguments-intercept":                 112536,
			"equalsByteString-cpu-arguments-slope":                     247,
			"equalsByteString-memory-arguments":                        1,
			"equalsData-cpu-arguments-intercept":                       150000,
			"equalsData-cpu-arguments-slope":                           10000,
			"equalsData-memory-arguments":                              1,
			"equalsInteger-cpu-arguments-intercept":                    136542,
			"equalsInteger-cpu-arguments-slope":                        1326,
			"equalsInteger-memory-arguments":                           1,
			"equalsString-cpu-arguments-constant":                      1000,
			"equalsString-cpu-arguments-intercept":                     150000,
			"equalsString-cpu-arguments-slope":                         1000,
			"equalsString-memory-arguments":                            1,
			"fstPair-cpu-arguments":                                    150000,
			"fstPair-memory-arguments":                                 32,
			"headList-cpu-arguments":                                   150000,
			"headList-memory-arguments":                                32,
			"iData-cpu-arguments":                                      150000,
			"iData-memory-arguments":                                   32,
			"ifThenElse-cpu-arguments":                                 1,
			"ifThenElse-memory-arguments":                              1,
			"indexByteString-cpu-arguments":                            150000,
			"indexByteString-memory-arguments":                         1,
			"lengthOfByteString-cpu-arguments":                         150000,
			"lengthOfByteString-memory-arguments":                      4,
			"lessThanByteString-cpu-arguments-intercept":               103599,
			"lessThanByteString-cpu-arguments-slope":                   248,
			"lessThanByteString-memory-arguments":                      1,
			"lessThanEqualsByteString-cpu-arguments-intercept":         103599,
			"lessThanEqualsByteString-cpu-arguments-slope":             248,
			"lessThanEqualsByteString-memory-arguments":                1,
			"lessThanEqualsInteger-cpu-arguments-intercept":            145276,
			"lessThanEqualsInteger-cpu-arguments-slope":                1366,
			"lessThanEqualsInteger-memory-arguments":                   1,
			"lessThanInteger-cpu-arguments-intercept":                  179690,
			"lessThanInteger-cpu-arguments-slope":                      497,
			"lessThanInteger-memory-arguments":                         1,
			"listData-cpu-arguments":                                   150000,
			"listData-memory-arguments":                                32,
			"mapData-cpu-arguments":                                    150000,
			"mapData-memory-arguments":                                 32,
			"mkCons-cpu-arguments":                                     150000,
			"mkCons-memory-arguments":                                  32,
			"mkNilData-cpu-arguments":                                  150000,
			"mkNilData-memory-arguments":                               32,
			"mkNilPairData-cpu-arguments":                              150000,
			"mkNilPairData-memory-arguments":                           32,
			"mkPairData-cpu-arguments":                                 150000,
			"mkPairData-memory-arguments":                              32,
			"modInteger-cpu-arguments-constant":                        148000,
			"modInteger-cpu-arguments-model-arguments-intercept":       425507,
			"modInteger-cpu-arguments-model-arguments-slope":           118,
			"modInteger-memory-arguments-intercept":                    0,
			"modInteger-memory-arguments-minimum":                      1,
			"modInteger-memory-arguments-slope":                        1,
			"multiplyInteger-cpu-arguments-intercept":                  61516,
			"multiplyInteger-cpu-arguments-slope":                      11218,
			"multiplyInteger-memory-arguments-intercept":               0,
			"multiplyInteger-memory-arguments-slope":                   1,
			"nullList-cpu-arguments":                                   150000,
			"nullList-memory-arguments":                                32,
			"quotientInteger-cpu-arguments-constant":                   148000,
			"quotientInteger-cpu-arguments-model-arguments-intercept":  425507,
			"quotientInteger-cpu-arguments-model-arguments-slope":      118,
			"quotientInteger-memory-arguments-intercept":               0,
			"quotientInteger-memory-arguments-minimum":                 1,
			"quotientInteger-memory-arguments-slope":                   1,
			"remainderInteger-cpu-arguments-constant":                  148000,
			"remainderInteger-cpu-arguments-model-arguments-intercept": 425507,
			"remainderInteger-cpu-arguments-model-arguments-slope":     118,
			"remainderInteger-memory-arguments-intercept":              0,
			"remainderInteger-memory-arguments-minimum":                1,
			"remainderInteger-memory-arguments-slope":                  1,
			"sha2_256-cpu-arguments-intercept":                         2477736,
			"sha2_256-cpu-arguments-slope":                             29175,
			"sha2_256-memory-arguments":                                4,
			"sha3_256-cpu-arguments-intercept":                         0,
			"sha3_256-cpu-arguments-slope":                             82363,
			"sha3_256-memory-arguments":                                4,
			"sliceByteString-cpu-arguments-intercept":                  150000,
			"sliceByteString-cpu-arguments-slope":                      5000,
			"sliceByteString-memory-arguments-intercept":               0,
			"sliceByteString-memory-arguments-slope":                   1,
			"sndPair-cpu-arguments":                                    150000,
			"sndPair-memory-arguments":                                 32,
			"subtractInteger-cpu-arguments-intercept":                  197209,
			"subtractInteger-cpu-arguments-slope":                      0,
			"subtractInteger-memory-arguments-intercept":               1,
			"subtractInteger-memory-arguments-slope":                   1,
			"tailList-cpu-arguments":                                   150000,
			"tailList-memory-arguments":                                32,
			"trace-cpu-arguments":                                      150000,
			"trace-memory-arguments":                                   32,
			"unBData-cpu-arguments":                                    150000,
			"unBData-memory-arguments":                                 32,
			"unConstrData-cpu-arguments":                               150000,
			"unConstrData-memory-arguments":                            32,
			"unIData-cpu-arguments":                                    150000,
			"unIData-memory-arguments":                                 32,
			"unListData-cpu-arguments":                                 150000,
			"unListData-memory-arguments":                              32,
			"unMapData-cpu-arguments":                                  150000,
			"unMapData-memory-arguments":                               32,
			"verifySignature-cpu-arguments-intercept":                  3345831,
			"verifySignature-cpu-arguments-slope":                      1,
			"verifySignature-memory-arguments":                         1,
		},
	},
}

func TestGenesisFromJson(t *testing.T) {
	tmpGenesis, err := alonzo.NewAlonzoGenesisFromReader(
		strings.NewReader(alonzoGenesisConfig),
	)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !reflect.DeepEqual(tmpGenesis, expectedGenesisObj) {
		t.Fatalf(
			"did not get expected object:\n     got: %#v\n  wanted: %#v",
			tmpGenesis,
			expectedGenesisObj,
		)
	}
}

func TestNewAlonzoGenesisFromReader(t *testing.T) {
	jsonData := `{
        "lovelacePerUTxOWord": 34482,
        "maxValueSize": 5000,
        "collateralPercentage": 150,
        "maxCollateralInputs": 3,
        "executionPrices": {
            "prSteps": { "numerator": 721, "denominator": 10000 },
            "prMem": { "numerator": 577, "denominator": 10000 }
        },
        "maxTxExUnits": { "exUnitsMem": 1000000, "exUnitsSteps": 10000000 },
        "maxBlockExUnits": { "exUnitsMem": 50000000, "exUnitsSteps": 40000000000 },
        "costModels": {
            "PlutusV1": {
                "addInteger-cpu-arguments-intercept": 205665,
                "addInteger-cpu-arguments-slope": 812
            }
        }
    }`

	reader := strings.NewReader(jsonData)
	result, err := alonzo.NewAlonzoGenesisFromReader(reader)
	if err != nil {
		t.Errorf("Failed to decode JSON: %v", err)
	}

	if result.LovelacePerUtxoWord != 34482 {
		t.Errorf(
			"Expected LovelacePerUtxoWord 34482, got %d",
			result.LovelacePerUtxoWord,
		)
	}

	if result.ExecutionPrices.Steps.Rat.Cmp(big.NewRat(721, 10000)) != 0 {
		t.Errorf(
			"Unexpected prSteps: got %v, expected 721/10000",
			result.ExecutionPrices.Steps.Rat,
		)
	}

	if result.ExecutionPrices.Mem.Rat.Cmp(big.NewRat(577, 10000)) != 0 {
		t.Errorf(
			"Unexpected prMem: got %v, expected 577/10000",
			result.ExecutionPrices.Mem.Rat,
		)
	}

	expectedCostModels := map[string]alonzo.CostModel{
		"PlutusV1": map[string]int{
			"addInteger-cpu-arguments-intercept": 205665,
			"addInteger-cpu-arguments-slope":     812,
		},
	}
	if !reflect.DeepEqual(result.CostModels, expectedCostModels) {
		t.Errorf(
			"Unexpected CostModels:\nGot:  %v\nExpected: %v",
			result.CostModels,
			expectedCostModels,
		)
	}
}
