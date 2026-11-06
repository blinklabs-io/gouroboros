// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
)

const alonzoNetworkFixture = "testdata/network-alonzo-genesis.json"

func loadAlonzoDoc(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(alonzoNetworkFixture)
	require.NoError(t, err)
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var doc map[string]any
	require.NoError(t, dec.Decode(&doc))
	return doc
}

// alonzoWith decodes the network fixture after setting the dotted path. A nil
// value deletes the key.
func alonzoWith(t *testing.T, path string, value any) error {
	t.Helper()
	doc := loadAlonzoDoc(t)
	parts := strings.Split(path, ".")
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
	_, err = alonzo.NewAlonzoGenesisFromReader(bytes.NewReader(data))
	return err
}

func TestAlonzoGenesisRealNetworkFileAccepted(t *testing.T) {
	t.Parallel()
	// Mainnet, preprod and preview ship byte-identical Alonzo genesis files.
	for _, path := range []string{
		alonzoNetworkFixture,
		"testdata/alonzo-genesis-extra-config.json",
	} {
		_, err := alonzo.NewAlonzoGenesisFromFile(path)
		require.NoError(t, err, path)
	}
}

func TestAlonzoGenesisRejectsEmptyObject(t *testing.T) {
	t.Parallel()
	_, err := alonzo.NewAlonzoGenesisFromReader(strings.NewReader(`{}`))
	require.ErrorContains(t, err, "missing required field")
}

func TestAlonzoGenesisUnknownKeyOnlyIsMissingRequiredFields(t *testing.T) {
	t.Parallel()
	_, err := alonzo.NewAlonzoGenesisFromReader(
		strings.NewReader(`{"bogus": 1}`),
	)
	require.ErrorContains(t, err, "missing required field")
	require.NotContains(t, err.Error(), "unknown field")
}

func TestAlonzoGenesisRequiredFields(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"lovelacePerUTxOWord",
		"costModels",
		"costModels.PlutusV1",
		"executionPrices",
		"executionPrices.prSteps",
		"executionPrices.prMem",
		"maxTxExUnits",
		"maxTxExUnits.exUnitsMem",
		"maxTxExUnits.exUnitsSteps",
		"maxBlockExUnits",
		"maxBlockExUnits.exUnitsMem",
		"maxBlockExUnits.exUnitsSteps",
		"maxValueSize",
		"collateralPercentage",
		"maxCollateralInputs",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			err := alonzoWith(t, path, nil)
			require.ErrorContains(t, err, "missing required field")
			parts := strings.Split(path, ".")
			require.ErrorContains(t, err, parts[len(parts)-1])
		})
	}
}

func TestAlonzoGenesisOptionalExtraConfigAbsent(t *testing.T) {
	t.Parallel()
	require.NoError(t, alonzoWith(t, "extraConfig", nil))
}

func TestAlonzoGenesisIgnoresUnknownKeys(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"bogusTopLevel",
		"maxTxExUnits.bogusNested",
		"executionPrices.bogusNested",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, alonzoWith(t, path, json.Number("1")))
		})
	}
	t.Run("extraConfig", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, alonzoWith(
			t,
			"extraConfig",
			map[string]any{"bogus": json.Number("1")},
		))
	})
}

func TestAlonzoGenesisAlternateKeyNames(t *testing.T) {
	t.Parallel()
	// ExUnits and Prices accept two spellings each (Plutus/ExUnits.hs).
	require.NoError(t, alonzoWith(t, "maxTxExUnits", map[string]any{
		"memory": json.Number("10000000"),
		"steps":  json.Number("10000000000"),
	}))
	require.NoError(t, alonzoWith(t, "executionPrices", map[string]any{
		"priceSteps": map[string]any{
			"numerator":   721,
			"denominator": 10000000,
		},
		"priceMemory": map[string]any{"numerator": 577, "denominator": 10000},
	}))
	err := alonzoWith(t, "maxTxExUnits", map[string]any{
		"memory": json.Number("10000000"),
	})
	require.ErrorContains(t, err, "missing required field")
}

type alonzoBoundCase struct {
	name  string
	value any
	valid bool
}

func runAlonzoBoundCases(
	t *testing.T,
	path string,
	cases []alonzoBoundCase,
) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := alonzoWith(t, path, tc.value)
			if tc.valid {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.ErrorContains(t, err, path)
		})
	}
}

func TestAlonzoGenesisPriceBounds(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"executionPrices.prSteps",
		"executionPrices.prMem",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			runAlonzoBoundCases(t, path, []alonzoBoundCase{
				{"negative", json.Number("-1"), false},
				{"negative tiny", json.Number("-0.0001"), false},
				{"zero", json.Number("0"), true},
				{"decimal", json.Number("0.0577"), true},
				{"above one", json.Number("2"), true},
				{
					"ratio object",
					map[string]any{
						"numerator":   json.Number("721"),
						"denominator": json.Number("10000000"),
					},
					true,
				},
				{
					"ratio object negative",
					map[string]any{
						"numerator":   json.Number("-721"),
						"denominator": json.Number("10000000"),
					},
					false,
				},
				{
					"above word64",
					json.Number("18446744073709551616"),
					false,
				},
			})
		})
	}
}

func TestAlonzoGenesisIntegerBounds(t *testing.T) {
	t.Parallel()
	word32 := []alonzoBoundCase{
		{"zero", json.Number("0"), true},
		{"word32 max", json.Number("4294967295"), true},
		{"above word32 max", json.Number("4294967296"), false},
		{"two to the 40", json.Number("1099511627776"), false},
		{"negative", json.Number("-1"), false},
	}
	word16 := []alonzoBoundCase{
		{"zero", json.Number("0"), true},
		{"word16 max", json.Number("65535"), true},
		{"above word16 max", json.Number("65536"), false},
		{"negative", json.Number("-1"), false},
	}
	word64 := []alonzoBoundCase{
		{"zero", json.Number("0"), true},
		{"word64 max", json.Number("18446744073709551615"), true},
		{"above word64 max", json.Number("18446744073709551616"), false},
		{"negative", json.Number("-1"), false},
	}
	for path, cases := range map[string][]alonzoBoundCase{
		"maxValueSize":                 word32,
		"collateralPercentage":         word16,
		"maxCollateralInputs":          word16,
		"lovelacePerUTxOWord":          word64,
		"maxTxExUnits.exUnitsMem":      word64,
		"maxTxExUnits.exUnitsSteps":    word64,
		"maxBlockExUnits.exUnitsMem":   word64,
		"maxBlockExUnits.exUnitsSteps": word64,
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			runAlonzoBoundCases(t, path, cases)
		})
	}
}

// The reference reads "memory" <|> "exUnitsMem": a present "memory" wins even
// when it is zero, so the decoded value is the one that was validated.
func TestAlonzoGenesisExUnitsPrimaryKeyWinsWhenZero(t *testing.T) {
	t.Parallel()
	doc := loadAlonzoDoc(t)
	doc["maxTxExUnits"] = map[string]any{
		"memory":       json.Number("0"),
		"exUnitsMem":   json.Number("7"),
		"steps":        json.Number("0"),
		"exUnitsSteps": json.Number("9"),
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	genesis, err := alonzo.NewAlonzoGenesisFromReader(bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, uint64(0), genesis.MaxTxExUnits.Mem)
	require.Equal(t, uint64(0), genesis.MaxTxExUnits.Steps)
}
