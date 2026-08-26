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

package alonzo_test

import (
	"encoding/json"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/lang"
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
	CostModels: alonzo.AlonzoGenesisCostModels{
		"PlutusV1": []int64{
			197209,
			0,
			1,
			1,
			396231,
			621,
			0,
			1,
			150000,
			1000,
			0,
			1,
			150000,
			32,
			2477736,
			29175,
			4,
			29773,
			100,
			29773,
			100,
			29773,
			100,
			29773,
			100,
			29773,
			100,
			29773,
			100,
			100,
			100,
			29773,
			100,
			150000,
			32,
			150000,
			32,
			150000,
			32,
			150000,
			1000,
			0,
			1,
			150000,
			32,
			150000,
			1000,
			0,
			8,
			148000,
			425507,
			118,
			0,
			1,
			1,
			150000,
			1000,
			0,
			8,
			150000,
			112536,
			247,
			1,
			150000,
			10000,
			1,
			136542,
			1326,
			1,
			1000,
			150000,
			1000,
			1,
			150000,
			32,
			150000,
			32,
			150000,
			32,
			1,
			1,
			150000,
			1,
			150000,
			4,
			103599,
			248,
			1,
			103599,
			248,
			1,
			145276,
			1366,
			1,
			179690,
			497,
			1,
			150000,
			32,
			150000,
			32,
			150000,
			32,
			150000,
			32,
			150000,
			32,
			150000,
			32,
			148000,
			425507,
			118,
			0,
			1,
			1,
			61516,
			11218,
			0,
			1,
			150000,
			32,
			148000,
			425507,
			118,
			0,
			1,
			1,
			148000,
			425507,
			118,
			0,
			1,
			1,
			2477736,
			29175,
			4,
			0,
			82363,
			4,
			150000,
			5000,
			0,
			1,
			150000,
			32,
			197209,
			0,
			1,
			1,
			150000,
			32,
			150000,
			32,
			150000,
			32,
			150000,
			32,
			150000,
			32,
			150000,
			32,
			150000,
			32,
			3345831,
			1,
			1,
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

func TestGenesisMapCostModelPreservesLegacyMainnetNames(t *testing.T) {
	genesis, err := alonzo.NewAlonzoGenesisFromReader(
		strings.NewReader(alonzoGenesisConfig),
	)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	model := genesis.CostModels["PlutusV1"]
	if len(model) != 166 {
		t.Fatalf(
			"legacy PlutusV1 cost model length = %d, want 166",
			len(model),
		)
	}
	for _, tc := range []struct {
		name  string
		index int
		want  int64
	}{
		{name: "blake2b intercept", index: 14, want: 2477736},
		{name: "blake2b slope", index: 15, want: 29175},
		{name: "blake2b memory", index: 16, want: 4},
		{name: "verifySignature intercept", index: 163, want: 3345831},
		{name: "verifySignature slope", index: 164, want: 1},
		{name: "verifySignature memory", index: 165, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := model[tc.index]; got != tc.want {
				t.Fatalf("cost model[%d] = %d, want %d", tc.index, got, tc.want)
			}
		})
	}
}

func TestGenesisMapCostModelsPreserveLegacyParameterTables(t *testing.T) {
	for _, tc := range []struct {
		name        string
		version     lang.LanguageVersion
		wantLength  int
		legacyStart int
	}{
		{
			name:        "PlutusV1",
			version:     lang.LanguageVersionV1,
			wantLength:  328,
			legacyStart: 319,
		},
		{
			name:        "PlutusV2",
			version:     lang.LanguageVersionV2,
			wantLength:  328,
			legacyStart: 319,
		},
		{
			name:        "PlutusV3",
			version:     lang.LanguageVersionV3,
			wantLength:  346,
			legacyStart: 337,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paramNames := legacyCostModelParamNamesForTest(
				lang.GetParamNamesForVersion(tc.version),
			)
			params := make(map[string]int64, len(paramNames))
			want := make([]int64, len(paramNames))
			for i, name := range paramNames {
				want[i] = int64(i + 1)
				params[name] = want[i]
			}
			// These zero-based boundary indices come from the exact upstream
			// legacy tables at aee28e0467be0561de5c2f097f639914d8d7a294.
			// Fixed sentinels keep this oracle independent of the helper's
			// production-shaped 9-to-5 rewrite.
			boundary := []struct {
				name string
				want int64
			}{
				{name: "valueData-cpu-arguments", want: 10_001},
				{name: "valueData-memory-arguments", want: 10_002},
				{
					name: "unValueData-cpu-arguments-intercept",
					want: 10_003,
				},
				{name: "unValueData-cpu-arguments-slope", want: 10_004},
				{name: "unValueData-memory-arguments", want: 10_005},
				{
					name: "scaleValue-cpu-arguments-intercept",
					want: 10_006,
				},
			}
			for i, item := range boundary {
				params[item.name] = item.want
				want[tc.legacyStart+i] = item.want
			}
			data, err := json.Marshal(map[string]any{tc.name: params})
			if err != nil {
				t.Fatalf("marshal legacy cost model: %v", err)
			}
			var models alonzo.AlonzoGenesisCostModels
			if err := json.Unmarshal(data, &models); err != nil {
				t.Fatalf("unmarshal legacy cost model: %v", err)
			}
			got := models[tc.name]
			if len(got) != tc.wantLength {
				t.Fatalf(
					"legacy cost model length = %d, want %d",
					len(got),
					tc.wantLength,
				)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("legacy cost model values = %v, want %v", got, want)
			}
			for i, item := range boundary {
				index := tc.legacyStart + i
				if got[index] != item.want {
					t.Fatalf(
						"legacy cost model[%d] = %d, want %d for %s",
						index,
						got[index],
						item.want,
						item.name,
					)
				}
			}
		})
	}
}

func TestGenesisMapCostModelsPreferCanonicalNames(t *testing.T) {
	paramNames := lang.GetParamNamesForVersion(lang.LanguageVersionV1)
	if len(paramNames) != 332 ||
		paramNames[14] != "blake2b_256-cpu-arguments-intercept" {
		t.Skip("Plutigo release exposes the legacy parameter table")
	}
	params := make(map[string]int64, len(paramNames)+11)
	for i, name := range paramNames {
		params[name] = int64(i + 1)
	}
	aliases := []struct {
		canonical string
		legacy    string
		index     int
		want      int64
	}{
		{
			canonical: "blake2b_256-cpu-arguments-intercept",
			legacy:    "blake2b-cpu-arguments-intercept",
			index:     14,
			want:      20_001,
		},
		{
			canonical: "blake2b_256-cpu-arguments-slope",
			legacy:    "blake2b-cpu-arguments-slope",
			index:     15,
			want:      20_002,
		},
		{
			canonical: "blake2b_256-memory-arguments",
			legacy:    "blake2b-memory-arguments",
			index:     16,
			want:      20_003,
		},
		{
			canonical: "verifyEd25519Signature-cpu-arguments-intercept",
			legacy:    "verifySignature-cpu-arguments-intercept",
			index:     163,
			want:      20_004,
		},
		{
			canonical: "verifyEd25519Signature-cpu-arguments-slope",
			legacy:    "verifySignature-cpu-arguments-slope",
			index:     164,
			want:      20_005,
		},
		{
			canonical: "verifyEd25519Signature-memory-arguments",
			legacy:    "verifySignature-memory-arguments",
			index:     165,
			want:      20_006,
		},
	}
	for _, item := range aliases {
		params[item.canonical] = item.want
		params[item.legacy] = -item.want
	}
	// The exact canonical table at
	// 3109dc1e6501ecbbed0b7ae27361c255d7a16173 uses nine Value entries.
	// Supplying all five legacy names as well must not select their ordering.
	for i, name := range []string{
		"valueData-cpu-arguments",
		"valueData-memory-arguments",
		"unValueData-cpu-arguments-intercept",
		"unValueData-cpu-arguments-slope",
		"unValueData-memory-arguments",
	} {
		params[name] = -int64(30_001 + i)
	}
	data, err := json.Marshal(map[string]any{"PlutusV1": params})
	if err != nil {
		t.Fatalf("marshal mixed cost model: %v", err)
	}
	var models alonzo.AlonzoGenesisCostModels
	if err := json.Unmarshal(data, &models); err != nil {
		t.Fatalf("unmarshal mixed cost model: %v", err)
	}
	model := models["PlutusV1"]
	if len(model) != 332 {
		t.Fatalf("canonical cost model length = %d, want 332", len(model))
	}
	for _, item := range aliases {
		if model[item.index] != item.want {
			t.Fatalf(
				"canonical cost model[%d] = %d, want %d for %s",
				item.index,
				model[item.index],
				item.want,
				item.canonical,
			)
		}
	}
	for i := 0; i < 9; i++ {
		index := 319 + i
		want := int64(index + 1)
		if model[index] != want {
			t.Fatalf(
				"canonical Value cost model[%d] = %d, want %d",
				index,
				model[index],
				want,
			)
		}
	}
	if got, want := model[328], int64(329); got != want {
		t.Fatalf(
			"first parameter after canonical Value span = %d, want %d",
			got,
			want,
		)
	}
}

func TestGenesisMapCostModelsAllowShorterLegacyModels(t *testing.T) {
	const data = `{"PlutusV1":{"addInteger-cpu-arguments-intercept":123}}`
	var models alonzo.AlonzoGenesisCostModels
	if err := json.Unmarshal([]byte(data), &models); err != nil {
		t.Fatalf("unmarshal short legacy cost model: %v", err)
	}
	want := []int64{123}
	if got := models["PlutusV1"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("short legacy cost model = %v, want %v", got, want)
	}
}

func legacyCostModelParamNamesForTest(paramNames []string) []string {
	ret := make([]string, 0, len(paramNames))
	for i := 0; i < len(paramNames); i++ {
		switch paramNames[i] {
		case "blake2b_256-cpu-arguments-intercept":
			ret = append(ret, "blake2b-cpu-arguments-intercept")
		case "blake2b_256-cpu-arguments-slope":
			ret = append(ret, "blake2b-cpu-arguments-slope")
		case "blake2b_256-memory-arguments":
			ret = append(ret, "blake2b-memory-arguments")
		case "verifyEd25519Signature-cpu-arguments-intercept":
			ret = append(ret, "verifySignature-cpu-arguments-intercept")
		case "verifyEd25519Signature-cpu-arguments-slope":
			ret = append(ret, "verifySignature-cpu-arguments-slope")
		case "verifyEd25519Signature-memory-arguments":
			ret = append(ret, "verifySignature-memory-arguments")
		case "valueData-cpu-arguments-intercept":
			ret = append(ret,
				"valueData-cpu-arguments",
				"valueData-memory-arguments",
				"unValueData-cpu-arguments-intercept",
				"unValueData-cpu-arguments-slope",
				"unValueData-memory-arguments",
			)
			// The canonical table replaces the five legacy entries above with
			// nine entries ending at unValueData-memory-arguments-slope.
			i += 8
		default:
			ret = append(ret, paramNames[i])
		}
	}
	return ret
}
