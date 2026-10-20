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
	"fmt"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/stretchr/testify/require"
)

// seqCostModel returns n parameters starting at start, so each injected
// language can be told apart from the base one by value and by length.
func seqCostModel(start int64, n int) []int64 {
	ret := make([]int64, n)
	for i := range ret {
		ret[i] = start + int64(i)
	}
	return ret
}

// alonzoGenesisWithExtraConfig builds an Alonzo genesis in the shape of the
// Leios prototype respin files: the base costModels carry only PlutusV1 at
// its initial length, and extraConfig.costModels carries longer models.
func alonzoGenesisWithExtraConfig(t *testing.T, extraConfig string) string {
	t.Helper()
	base, err := json.Marshal(seqCostModel(1, 166))
	require.NoError(t, err)
	return fmt.Sprintf(`{
  "lovelacePerUTxOWord": 34482,
  "executionPrices": {
    "prSteps": {"numerator": 721, "denominator": 10000000},
    "prMem": {"numerator": 577, "denominator": 10000}
  },
  "maxTxExUnits": {"exUnitsMem": 10000000, "exUnitsSteps": 10000000000},
  "maxBlockExUnits": {"exUnitsMem": 50000000, "exUnitsSteps": 40000000000},
  "maxValueSize": 5000,
  "collateralPercentage": 150,
  "maxCollateralInputs": 3,
  "costModels": {"PlutusV1": %s},
  "extraConfig": %s
}`, base, extraConfig)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	ret, err := json.Marshal(v)
	require.NoError(t, err)
	return string(ret)
}

func TestNewAlonzoGenesisAcceptsExtraConfigCostModels(t *testing.T) {
	v1 := seqCostModel(1000, 332)
	v2 := seqCostModel(2000, 332)
	v3 := seqCostModel(3000, 350)
	extra := fmt.Sprintf(
		`{"costModels": {"PlutusV1": %s, "PlutusV2": %s, "PlutusV3": %s}}`,
		mustJSON(t, v1),
		mustJSON(t, v2),
		mustJSON(t, v3),
	)
	genesis, err := alonzo.NewAlonzoGenesisFromReader(
		strings.NewReader(alonzoGenesisWithExtraConfig(t, extra)),
	)
	require.NoError(t, err)

	// The base costModels are untouched by extraConfig.
	require.Len(t, genesis.CostModels, 1)
	require.Equal(t, seqCostModel(1, 166), genesis.CostModels["PlutusV1"])

	require.NotNil(t, genesis.ExtraConfig)
	require.Equal(
		t,
		map[uint][]int64{
			alonzo.PlutusV1Key: v1,
			alonzo.PlutusV2Key: v2,
			alonzo.PlutusV3Key: v3,
		},
		genesis.ExtraConfig.CostModels,
	)
}

func TestNewAlonzoGenesisWithoutExtraConfig(t *testing.T) {
	genesis, err := alonzo.NewAlonzoGenesisFromReader(
		strings.NewReader(alonzoGenesisConfig),
	)
	require.NoError(t, err)
	require.Nil(t, genesis.ExtraConfig)
}

// The reference AlonzoExtraConfig parser reads only "costModels", parses it
// leniently for every Plutus language plus "Unknown", and ignores other keys.
func TestAlonzoGenesisExtraConfigLenientCostModels(t *testing.T) {
	extra := `{
  "futureInjection": {"data": {}},
  "costModels": {
    "PlutusV4": [7, 8, 9],
    "Unknown": {"9": [4, 5, 6]}
  }
}`
	genesis, err := alonzo.NewAlonzoGenesisFromReader(
		strings.NewReader(alonzoGenesisWithExtraConfig(t, extra)),
	)
	require.NoError(t, err)
	require.NotNil(t, genesis.ExtraConfig)
	require.Equal(
		t,
		map[uint][]int64{
			alonzo.PlutusV4Key: {7, 8, 9},
			9:                  {4, 5, 6},
		},
		genesis.ExtraConfig.CostModels,
	)
}

func TestAlonzoGenesisExtraConfigWithoutCostModels(t *testing.T) {
	genesis, err := alonzo.NewAlonzoGenesisFromReader(
		strings.NewReader(alonzoGenesisWithExtraConfig(t, `{}`)),
	)
	require.NoError(t, err)
	require.NotNil(t, genesis.ExtraConfig)
	require.Nil(t, genesis.ExtraConfig.CostModels)
}

func TestAlonzoGenesisExtraConfigRejectsMalformedCostModels(t *testing.T) {
	for name, extra := range map[string]string{
		"not an object":     `[]`,
		"model not a list":  `{"costModels": {"PlutusV1": "x"}}`,
		"non-integer value": `{"costModels": {"PlutusV2": [1, "a"]}}`,
		"unknown not keyed": `{"costModels": {"Unknown": {"x": [1]}}}`,
		"unknown key range": `{"costModels": {"Unknown": {"256": [1]}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := alonzo.NewAlonzoGenesisFromReader(
				strings.NewReader(alonzoGenesisWithExtraConfig(t, extra)),
			)
			require.Error(t, err)
		})
	}
}

// The reference node overrides the era-translated cost models with the
// injected ones, language by language, and keeps every other language.
func TestAlonzoGenesisInjectExtraCostModels(t *testing.T) {
	v1 := seqCostModel(1000, 332)
	v3 := seqCostModel(3000, 350)
	extra := fmt.Sprintf(
		`{"costModels": {"PlutusV1": %s, "PlutusV3": %s}}`,
		mustJSON(t, v1),
		mustJSON(t, v3),
	)
	genesis, err := alonzo.NewAlonzoGenesisFromReader(
		strings.NewReader(alonzoGenesisWithExtraConfig(t, extra)),
	)
	require.NoError(t, err)

	current := map[uint][]int64{
		alonzo.PlutusV1Key: seqCostModel(1, 166),
		alonzo.PlutusV2Key: seqCostModel(10, 175),
		alonzo.PlutusV3Key: seqCostModel(20, 251),
		alonzo.PlutusV4Key: seqCostModel(30, 251),
	}
	got := genesis.InjectExtraCostModels(current)
	require.Equal(
		t,
		map[uint][]int64{
			alonzo.PlutusV1Key: v1,
			alonzo.PlutusV2Key: seqCostModel(10, 175),
			alonzo.PlutusV3Key: v3,
			alonzo.PlutusV4Key: seqCostModel(30, 251),
		},
		got,
	)
	// The input is not modified and the result shares no slices with the
	// genesis, so a later parameter update cannot write through.
	require.Equal(t, seqCostModel(1, 166), current[alonzo.PlutusV1Key])
	gotV1 := got[alonzo.PlutusV1Key]
	require.Len(t, gotV1, 332)
	gotV1[0] = -1
	require.NotNil(t, genesis.ExtraConfig)
	require.Equal(t, v1, genesis.ExtraConfig.CostModels[alonzo.PlutusV1Key])
	gotV2 := got[alonzo.PlutusV2Key]
	require.Len(t, gotV2, 175)
	gotV2[0] = -1
	require.Equal(t, seqCostModel(10, 175), current[alonzo.PlutusV2Key])
}

func TestAlonzoGenesisInjectExtraCostModelsWithoutExtraConfig(t *testing.T) {
	genesis, err := alonzo.NewAlonzoGenesisFromReader(
		strings.NewReader(alonzoGenesisConfig),
	)
	require.NoError(t, err)
	current := map[uint][]int64{alonzo.PlutusV1Key: seqCostModel(1, 166)}
	require.Equal(t, current, genesis.InjectExtraCostModels(current))

	var nilGenesis *alonzo.AlonzoGenesis
	require.Equal(t, current, nilGenesis.InjectExtraCostModels(current))
}

func TestAlonzoGenesisExtraConfigJSONRoundTrip(t *testing.T) {
	extra := `{"costModels": {"PlutusV2": [1, 2, 3], "Unknown": {"7": [4]}}}`
	genesis, err := alonzo.NewAlonzoGenesisFromReader(
		strings.NewReader(alonzoGenesisWithExtraConfig(t, extra)),
	)
	require.NoError(t, err)
	encoded, err := json.Marshal(genesis.ExtraConfig)
	require.NoError(t, err)
	require.JSONEq(
		t,
		`{"costModels": {"PlutusV2": [1, 2, 3], "Unknown": {"7": [4]}}}`,
		string(encoded),
	)
	var decoded alonzo.AlonzoGenesisExtraConfig
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, *genesis.ExtraConfig, decoded)
}

// paramMap names the first count parameters of version in the genesis
// spelling, with values start, start+1, and so on.
func paramMap(
	t *testing.T,
	version lang.LanguageVersion,
	count int,
	start int64,
) map[string]int64 {
	t.Helper()
	names := lang.GetParamNamesForVersion(version)
	require.GreaterOrEqual(t, len(names), count)
	legacy := map[string]string{
		"blake2b_256-cpu-arguments-intercept":            "blake2b-cpu-arguments-intercept",
		"blake2b_256-cpu-arguments-slope":                "blake2b-cpu-arguments-slope",
		"blake2b_256-memory-arguments":                   "blake2b-memory-arguments",
		"verifyEd25519Signature-cpu-arguments-intercept": "verifySignature-cpu-arguments-intercept",
		"verifyEd25519Signature-cpu-arguments-slope":     "verifySignature-cpu-arguments-slope",
		"verifyEd25519Signature-memory-arguments":        "verifySignature-memory-arguments",
	}
	ret := make(map[string]int64, count)
	for i, name := range names[:count] {
		if version == lang.LanguageVersionV1 {
			if genesisName, ok := legacy[name]; ok {
				name = genesisName
			}
		}
		ret[name] = start + int64(i)
	}
	return ret
}

// A map-form model is read with the parameter names the language was
// introduced with (166, 175, 251, and the PlutusV3 names for PlutusV4), in
// parameter order, and names past them are ignored.
func TestAlonzoGenesisExtraConfigMapCostModels(t *testing.T) {
	v1 := paramMap(t, lang.LanguageVersionV1, 166, 1000)
	v2 := paramMap(t, lang.LanguageVersionV2, 176, 2000)
	v3 := paramMap(t, lang.LanguageVersionV3, 251, 3000)
	v4 := paramMap(t, lang.LanguageVersionV3, 251, 4000)
	v4["notAParameter-cpu-arguments"] = 9
	extra := fmt.Sprintf(
		`{"costModels": {"PlutusV1": %s, "PlutusV2": %s, "PlutusV3": %s, "PlutusV4": %s}}`,
		mustJSON(t, v1),
		mustJSON(t, v2),
		mustJSON(t, v3),
		mustJSON(t, v4),
	)
	genesis, err := alonzo.NewAlonzoGenesisFromReader(
		strings.NewReader(alonzoGenesisWithExtraConfig(t, extra)),
	)
	require.NoError(t, err)
	require.NotNil(t, genesis.ExtraConfig)
	require.Equal(
		t,
		map[uint][]int64{
			alonzo.PlutusV1Key: seqCostModel(1000, 166),
			alonzo.PlutusV2Key: seqCostModel(2000, 175),
			alonzo.PlutusV3Key: seqCostModel(3000, 251),
			alonzo.PlutusV4Key: seqCostModel(4000, 251),
		},
		genesis.ExtraConfig.CostModels,
	)
}

func TestAlonzoGenesisExtraConfigMapCostModelMissingParameter(t *testing.T) {
	v2 := paramMap(t, lang.LanguageVersionV2, 175, 2000)
	delete(v2, lang.GetParamNamesForVersion(lang.LanguageVersionV2)[174])
	extra := fmt.Sprintf(`{"costModels": {"PlutusV2": %s}}`, mustJSON(t, v2))
	_, err := alonzo.NewAlonzoGenesisFromReader(
		strings.NewReader(alonzoGenesisWithExtraConfig(t, extra)),
	)
	require.ErrorContains(t, err, "parameter names missing from cost model")
}

// PlutusV1 maps are read with the genesis spelling of the six renamed
// parameters only.
func TestAlonzoGenesisExtraConfigMapCostModelPlutusV1Names(t *testing.T) {
	v1 := paramMap(t, lang.LanguageVersionV1, 166, 1000)
	value := v1["blake2b-cpu-arguments-intercept"]
	delete(v1, "blake2b-cpu-arguments-intercept")
	v1["blake2b_256-cpu-arguments-intercept"] = value
	extra := fmt.Sprintf(`{"costModels": {"PlutusV1": %s}}`, mustJSON(t, v1))
	_, err := alonzo.NewAlonzoGenesisFromReader(
		strings.NewReader(alonzoGenesisWithExtraConfig(t, extra)),
	)
	require.ErrorContains(t, err, "blake2b-cpu-arguments-intercept")
}
