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
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
)

// alonzoDecodeWith decodes the network fixture after setting the dotted path
// to value. A nil value deletes the key.
func alonzoDecodeWith(
	t *testing.T,
	path string,
	value any,
) (alonzo.AlonzoGenesis, error) {
	t.Helper()
	doc := loadAlonzoDoc(t)
	parts := splitPath(path)
	cur := doc
	for _, part := range parts[:len(parts)-1] {
		next, ok := cur[part].(map[string]any)
		require.True(t, ok, "path element %q is not an object", part)
		cur = next
	}
	last := parts[len(parts)-1]
	if value == nil {
		delete(cur, last)
	} else {
		cur[last] = value
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	return alonzo.NewAlonzoGenesisFromReader(bytes.NewReader(data))
}

func splitPath(path string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '.' {
			parts = append(parts, path[start:i])
			start = i + 1
		}
	}
	return parts
}

func TestAlonzoGenesisIntegralNumberFormsDecode(t *testing.T) {
	t.Parallel()
	g, err := alonzoDecodeWith(
		t,
		"lovelacePerUTxOWord",
		json.Number("3.4482e4"),
	)
	require.NoError(t, err)
	require.Equal(t, uint64(34482), g.LovelacePerUtxoWord)
	g, err = alonzoDecodeWith(t, "maxValueSize", json.Number("5e3"))
	require.NoError(t, err)
	require.Equal(t, uint(5000), g.MaxValueSize)
	g, err = alonzoDecodeWith(t, "collateralPercentage", json.Number("150.0"))
	require.NoError(t, err)
	require.Equal(t, uint(150), g.CollateralPercentage)
	g, err = alonzoDecodeWith(t, "maxTxExUnits", map[string]any{
		"exUnitsMem":   json.Number("1.0e7"),
		"exUnitsSteps": json.Number("1e10"),
	})
	require.NoError(t, err)
	require.Equal(t, uint64(10000000), g.MaxTxExUnits.Mem)
	require.Equal(t, uint64(10000000000), g.MaxTxExUnits.Steps)
	g, err = alonzoDecodeWith(t, "executionPrices", map[string]any{
		"prSteps": json.RawMessage(`{"numerator":7.21e2,"denominator":1e7}`),
		"prMem":   json.Number("5.77e-2"),
	})
	require.NoError(t, err)
	require.Equal(t, "721/10000000", g.ExecutionPrices.Steps.String())
	require.Equal(t, "577/10000", g.ExecutionPrices.Mem.String())
}

func TestAlonzoGenesisRatioObjectCaseVariantCannotOverride(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ path, value string }{
		{
			"executionPrices.prSteps",
			`{"numerator":1,"Numerator":-1,"denominator":2}`,
		},
		{
			"executionPrices.prMem",
			`{"numerator":1,"denominator":2,"DENOMINATOR":0}`,
		},
	} {
		_, err := alonzoDecodeWith(t, tc.path, json.RawMessage(tc.value))
		require.ErrorContains(t, err, "differs only in case", tc.path)
	}
}

func TestAlonzoGenesisEveryValidatedObjectRejectsCaseVariants(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ path, variant string }{
		{"", "LovelacePerUTxOWord"},
		{"costModels", "plutusv1"},
		{"executionPrices", "PRSTEPS"},
		{"maxTxExUnits", "ExUnitsMem"},
		{"maxBlockExUnits", "Memory"},
	} {
		t.Run(tc.path+"+"+tc.variant, func(t *testing.T) {
			t.Parallel()
			doc := loadAlonzoDoc(t)
			cur := doc
			if tc.path != "" {
				cur = doc[tc.path].(map[string]any)
			}
			cur[tc.variant] = json.Number("0")
			data, err := json.Marshal(doc)
			require.NoError(t, err)
			_, err = alonzo.NewAlonzoGenesisFromReader(bytes.NewReader(data))
			require.ErrorContains(t, err, "differs only in case")
		})
	}
}

// aeson reads (o .: "memory" <|> o .: "exUnitsMem"): null and a value of the
// wrong type fall through to the second spelling, and the spelling that is not
// selected is never decoded.
func TestAlonzoGenesisExUnitsAlternateSpellingSelection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		units string
		mem   uint64
		valid bool
	}{
		{"null primary", `{"memory":null,"exUnitsMem":1,"steps":2}`, 1, true},
		{
			"mistyped primary",
			`{"memory":"x","exUnitsMem":1,"steps":2}`,
			1,
			true,
		},
		{
			"fractional primary",
			`{"memory":1.5,"exUnitsMem":1,"steps":2}`,
			1,
			true,
		},
		{
			"unselected fallback ignored",
			`{"memory":5,"exUnitsMem":"bad","steps":2}`,
			5,
			true,
		},
		{
			"unselected null fallback",
			`{"memory":5,"exUnitsMem":null,"steps":2}`,
			5,
			true,
		},
		{"zero primary wins", `{"memory":0,"exUnitsMem":9,"steps":2}`, 0, true},
		{
			"out of range primary does not fall through",
			`{"memory":18446744073709551616,"exUnitsMem":1,"steps":2}`,
			0,
			false,
		},
		{"both null", `{"memory":null,"exUnitsMem":null,"steps":2}`, 0, false},
		{
			"both mistyped",
			`{"memory":"x","exUnitsMem":"y","steps":2}`,
			0,
			false,
		},
		{
			"steps fallback",
			`{"memory":1,"steps":null,"exUnitsSteps":7}`,
			1,
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, err := alonzoDecodeWith(
				t,
				"maxTxExUnits",
				json.RawMessage(tc.units),
			)
			if !tc.valid {
				require.ErrorContains(t, err, "invalid Alonzo genesis")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.mem, g.MaxTxExUnits.Mem)
		})
	}
}

func TestAlonzoGenesisExUnitsUnmarshalDecodesOnlySelectedSpelling(
	t *testing.T,
) {
	t.Parallel()
	var u alonzo.AlonzoGenesisExUnits
	require.NoError(t, json.Unmarshal(
		[]byte(`{"memory":5,"exUnitsMem":"bad","steps":6,"exUnitsSteps":{}}`),
		&u,
	))
	require.Equal(t, uint64(5), u.Mem)
	require.Equal(t, uint64(6), u.Steps)
}

func TestAlonzoGenesisPricesAlternateSpellingSelection(t *testing.T) {
	t.Parallel()
	g, err := alonzoDecodeWith(t, "executionPrices", json.RawMessage(
		`{"priceSteps":null,"prSteps":0.5,"priceMemory":0.25,"prMem":"bad"}`))
	require.NoError(t, err)
	require.Equal(t, "1/2", g.ExecutionPrices.Steps.String())
	require.Equal(t, "1/4", g.ExecutionPrices.Mem.String())
}

func TestAlonzoGenesisRequiredCostModelRejectsNull(t *testing.T) {
	t.Parallel()
	for name, value := range map[string]any{
		"null":   json.RawMessage("null"),
		"string": "x",
		"number": json.Number("1"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := alonzoDecodeWith(t, "costModels.PlutusV1", value)
			require.ErrorContains(t, err, "invalid Alonzo genesis")
			require.ErrorContains(t, err, "costModels.PlutusV1")
		})
	}
	for name, value := range map[string]any{
		"list": json.RawMessage(`[1,2,3]`),
		"map":  json.RawMessage(`{"a":1}`),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := alonzoDecodeWith(t, "costModels.PlutusV1", value)
			require.NoError(t, err)
		})
	}
}

// Numbers the validator does not examine reach the decode exactly as written.
func TestAlonzoGenesisUncheckedNumbersAreNotRewritten(t *testing.T) {
	t.Parallel()
	_, err := alonzoDecodeWith(
		t,
		"costModels.PlutusV1",
		json.RawMessage(`[1.0,2]`),
	)
	require.Error(t, err, "cost model entries are decoded as written")
	require.NotContains(t, err.Error(), "invalid Alonzo genesis")
}

func TestAlonzoGenesisValidatedValuesAlwaysDecode(t *testing.T) {
	t.Parallel()
	paths := []string{
		"lovelacePerUTxOWord", "maxValueSize", "collateralPercentage",
		"maxCollateralInputs", "executionPrices.prSteps",
		"executionPrices.prMem", "maxTxExUnits.exUnitsMem",
		"maxTxExUnits.exUnitsSteps", "maxBlockExUnits.exUnitsMem",
		"maxBlockExUnits.exUnitsSteps",
	}
	literals := []string{
		"0", "-0", "1.0", "1e3", "4.5e16", "0.5", "5e-1",
		"9223372036854775807", "9223372036854775808",
		"18446744073709551615", "18446744073709551616",
		"4294967295", "4294967296", "65536",
		"0e999999999", "1e-1000000000", "1e5000",
		`{"numerator":1.0,"denominator":2}`,
		`{"numerator":9223372036854775808,"denominator":18446744073709551615}`,
		`null`, `"1"`, `1.5`, `[]`, `{}`,
	}
	for _, path := range paths {
		for _, lit := range literals {
			t.Run(path+"="+lit, func(t *testing.T) {
				t.Parallel()
				_, err := alonzoDecodeWith(t, path, json.RawMessage(lit))
				if err != nil {
					require.ErrorContains(t, err, "invalid Alonzo genesis",
						"decode failed after validation passed")
				}
			})
		}
	}
}
