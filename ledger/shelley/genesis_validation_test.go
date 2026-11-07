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

package shelley_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// loadShelleyDoc reads a genesis fixture into a generic document. Numbers keep
// their literal text so a test can substitute exact JSON number forms.
func loadShelleyDoc(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	doc := map[string]any{}
	require.NoError(t, dec.Decode(&doc))
	return doc
}

// shelleyParent returns the object holding the last element of a dotted path.
func shelleyParent(
	t *testing.T,
	doc map[string]any,
	path string,
) (map[string]any, string) {
	t.Helper()
	parts := strings.Split(path, ".")
	cur := doc
	for _, part := range parts[:len(parts)-1] {
		next, ok := cur[part].(map[string]any)
		require.True(t, ok, "path element %q is not an object", part)
		cur = next
	}
	return cur, parts[len(parts)-1]
}

func decodeShelleyDoc(doc map[string]any) error {
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	_, err = shelley.NewShelleyGenesisFromReader(bytes.NewReader(data))
	return err
}

// shelleyWith decodes the mainnet fixture after applying set to the dotted
// path. A nil value deletes the key.
func shelleyWith(t *testing.T, path string, value any) error {
	t.Helper()
	doc := loadShelleyDoc(t, "testdata/mainnet-shelley-genesis.json")
	parent, key := shelleyParent(t, doc, path)
	if value == nil {
		delete(parent, key)
	} else {
		parent[key] = value
	}
	return decodeShelleyDoc(doc)
}

func TestShelleyGenesisRealNetworkFilesAccepted(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"testdata/mainnet-shelley-genesis.json",
		"testdata/preprod-shelley-genesis.json",
		"testdata/preview-shelley-genesis.json",
		"testdata/cardano-node-devnet-genesis.json",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			_, err := shelley.NewShelleyGenesisFromFile(path)
			require.NoError(t, err)
		})
	}
}

func TestShelleyGenesisRejectsEmptyObject(t *testing.T) {
	t.Parallel()
	_, err := shelley.NewShelleyGenesisFromReader(strings.NewReader(`{}`))
	require.ErrorContains(t, err, "missing required field")
}

func TestShelleyGenesisUnknownKeyOnlyIsMissingRequiredFields(t *testing.T) {
	t.Parallel()
	_, err := shelley.NewShelleyGenesisFromReader(
		strings.NewReader(`{"bogus": 1}`),
	)
	require.ErrorContains(t, err, "missing required field")
	require.NotContains(t, err.Error(), "unknown field")
}

func TestShelleyGenesisRequiredTopLevelFields(t *testing.T) {
	t.Parallel()
	for _, field := range []string{
		"systemStart",
		"networkMagic",
		"networkId",
		"activeSlotsCoeff",
		"securityParam",
		"epochLength",
		"slotsPerKESPeriod",
		"maxKESEvolutions",
		"slotLength",
		"updateQuorum",
		"maxLovelaceSupply",
		"protocolParams",
		"genDelegs",
		"initialFunds",
	} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			err := shelleyWith(t, field, nil)
			require.ErrorContains(t, err, "missing required field")
			require.ErrorContains(t, err, `"`+field+`"`)
		})
	}
}

func TestShelleyGenesisRequiredProtocolParams(t *testing.T) {
	t.Parallel()
	for _, field := range []string{
		"minFeeA",
		"minFeeB",
		"maxBlockBodySize",
		"maxTxSize",
		"maxBlockHeaderSize",
		"keyDeposit",
		"poolDeposit",
		"eMax",
		"nOpt",
		"a0",
		"rho",
		"tau",
		"decentralisationParam",
		"extraEntropy",
		"protocolVersion",
		"protocolVersion.major",
		"protocolVersion.minor",
	} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			err := shelleyWith(t, "protocolParams."+field, nil)
			require.ErrorContains(t, err, "missing required field")
			require.ErrorContains(t, err, "protocolParams")
		})
	}
}

func TestShelleyGenesisOptionalFieldsAbsent(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"staking",
		"extraConfig",
		"protocolParams.minUTxOValue",
		"protocolParams.minPoolCost",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, shelleyWith(t, path, nil))
		})
	}
}

func TestShelleyGenesisStakingRequiresPoolsAndStake(t *testing.T) {
	t.Parallel()
	doc := loadShelleyDoc(t, "testdata/cardano-node-devnet-genesis.json")
	staking, ok := doc["staking"].(map[string]any)
	require.True(t, ok)
	for _, field := range []string{"pools", "stake"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			doc := loadShelleyDoc(
				t,
				"testdata/cardano-node-devnet-genesis.json",
			)
			delete(doc["staking"].(map[string]any), field)
			err := decodeShelleyDoc(doc)
			require.ErrorContains(t, err, "missing required field")
			require.ErrorContains(t, err, "staking")
		})
	}
	require.Contains(t, staking, "pools")
}

func TestShelleyGenesisIgnoresUnknownKeys(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"bogusTopLevel",
		"protocolParams.bogusNested",
		"protocolParams.protocolVersion.bogusDeep",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, shelleyWith(t, path, json.Number("1")))
		})
	}
}

func TestShelleyGenesisExtraConfigFileInjectionRejected(t *testing.T) {
	t.Parallel()
	// A file-backed injection is part of the reference format but is not
	// modelled here; ignoring it would silently drop injected state.
	extra := map[string]any{
		"initialFunds": map[string]any{
			"file": []any{"funds.json"},
			"hash": "00",
		},
	}
	err := shelleyWith(t, "extraConfig", extra)
	require.ErrorContains(t, err, "extraConfig.initialFunds")
}

type shelleyBoundCase struct {
	name  string
	value any
	valid bool
}

func runShelleyBoundCases(
	t *testing.T,
	path string,
	cases []shelleyBoundCase,
) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := shelleyWith(t, path, tc.value)
			if tc.valid {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.ErrorContains(t, err, path)
		})
	}
}

func TestShelleyGenesisNonNegativeIntervalA0(t *testing.T) {
	t.Parallel()
	runShelleyBoundCases(t, "protocolParams.a0", []shelleyBoundCase{
		{"negative one", json.Number("-1"), false},
		{"negative tiny", json.Number("-0.1"), false},
		{"negative zero", json.Number("-0.0"), true},
		{"zero", json.Number("0"), true},
		{"above one", json.Number("2.5"), true},
		{"word64 max", json.Number("18446744073709551615"), true},
		{"above word64 max", json.Number("18446744073709551616"), false},
		{
			"too precise",
			json.Number("0.00000000000000000001"),
			false,
		},
		{
			"ratio object",
			map[string]any{
				"numerator":   json.Number("3"),
				"denominator": json.Number("10"),
			},
			true,
		},
		{
			"ratio object negative",
			map[string]any{
				"numerator":   json.Number("-3"),
				"denominator": json.Number("10"),
			},
			false,
		},
		{
			"ratio object zero denominator",
			map[string]any{
				"numerator":   json.Number("3"),
				"denominator": json.Number("0"),
			},
			false,
		},
		{"string", "0.3", false},
	})
}

func TestShelleyGenesisUnitIntervals(t *testing.T) {
	t.Parallel()
	for _, field := range []string{
		"rho",
		"tau",
		"decentralisationParam",
	} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			runShelleyBoundCases(
				t,
				"protocolParams."+field,
				[]shelleyBoundCase{
					{"zero", json.Number("0"), true},
					{"one", json.Number("1"), true},
					{"one point zero", json.Number("1.0"), true},
					{"scientific half", json.Number("5e-1"), true},
					{"half object", map[string]any{
						"numerator":   json.Number("1"),
						"denominator": json.Number("2"),
					}, true},
					{"two", json.Number("2"), false},
					{"above one", json.Number("1.5"), false},
					{
						"just above one",
						json.Number("1.0000000000000000001"),
						false,
					},
					{"negative", json.Number("-0.5"), false},
					{"scientific above one", json.Number("1e1"), false},
					{"scientific too big", json.Number("1e20"), false},
				},
			)
		})
	}
}

func TestShelleyGenesisActiveSlotsCoeffIsPositiveUnitInterval(t *testing.T) {
	t.Parallel()
	runShelleyBoundCases(t, "activeSlotsCoeff", []shelleyBoundCase{
		{"zero", json.Number("0"), false},
		{"smallest decimal", json.Number("0.0000000000000000001"), true},
		{"typical", json.Number("0.05"), true},
		{"one", json.Number("1"), true},
		{"above one", json.Number("1.1"), false},
		{"negative", json.Number("-0.05"), false},
	})
}

func TestShelleyGenesisIntegerBounds(t *testing.T) {
	t.Parallel()
	word32 := []shelleyBoundCase{
		{"zero", json.Number("0"), true},
		{"word32 max", json.Number("4294967295"), true},
		{"above word32 max", json.Number("4294967296"), false},
		{"two to the 40", json.Number("1099511627776"), false},
		{"negative", json.Number("-1"), false},
	}
	word16 := []shelleyBoundCase{
		{"zero", json.Number("0"), true},
		{"word16 max", json.Number("65535"), true},
		{"above word16 max", json.Number("65536"), false},
		{"negative", json.Number("-1"), false},
	}
	word64 := []shelleyBoundCase{
		{"zero", json.Number("0"), true},
		{"negative", json.Number("-1"), false},
		{"fractional", json.Number("1.5"), false},
		{"string", "1", false},
	}
	for path, cases := range map[string][]shelleyBoundCase{
		"protocolParams.maxTxSize":             word32,
		"protocolParams.maxBlockBodySize":      word32,
		"protocolParams.eMax":                  word32,
		"protocolParams.protocolVersion.minor": word32,
		"networkMagic":                         word32,
		"protocolParams.maxBlockHeaderSize":    word16,
		"protocolParams.nOpt":                  word16,
		"epochLength":                          word64,
		"slotsPerKESPeriod":                    word64,
		"maxKESEvolutions":                     word64,
		"updateQuorum":                         word64,
		"maxLovelaceSupply":                    word64,
		"protocolParams.minFeeA":               word64,
		"protocolParams.minFeeB":               word64,
		"protocolParams.keyDeposit":            word64,
		"protocolParams.poolDeposit":           word64,
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			runShelleyBoundCases(t, path, cases)
		})
	}
}

func TestShelleyGenesisSecurityParamIsNonZero(t *testing.T) {
	t.Parallel()
	runShelleyBoundCases(t, "securityParam", []shelleyBoundCase{
		{"zero", json.Number("0"), false},
		{"negative", json.Number("-1"), false},
		{"one", json.Number("1"), true},
		{"typical", json.Number("2160"), true},
	})
}

func TestShelleyGenesisNetworkId(t *testing.T) {
	t.Parallel()
	runShelleyBoundCases(t, "networkId", []shelleyBoundCase{
		{"mainnet", "Mainnet", true},
		{"testnet", "Testnet", true},
		{"unknown", "Devnet", false},
		{"lowercase", "mainnet", false},
		{"number", json.Number("1"), false},
	})
}

// A key differing from a validated key only in case is ignored, as in the
// reference: the validated value is the decoded one.
func TestShelleyGenesisCaseVariantKeyIsIgnored(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/mainnet-shelley-genesis.json")
	require.NoError(t, err)
	want, err := shelley.NewShelleyGenesisFromReader(bytes.NewReader(data))
	require.NoError(t, err)
	for _, tc := range []struct{ exact, variant string }{
		{`"rho": 0.003`, `"RHO": 5`},
		{`"securityParam": 2160`, `"SecurityParam": 0`},
		{`"networkId": "Mainnet"`, `"NetworkID": "Bogus"`},
	} {
		require.Contains(t, string(data), tc.exact)
		doc := strings.Replace(
			string(data),
			tc.exact,
			tc.exact+", "+tc.variant,
			1,
		)
		got, err := shelley.NewShelleyGenesisFromReader(strings.NewReader(doc))
		require.NoError(t, err, tc.variant)
		require.Equal(t, want, got, tc.variant)
	}
}
