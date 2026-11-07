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
	"maps"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// shelleyDecodeWith decodes the mainnet fixture after setting the dotted path
// to value. A nil value deletes the key.
func shelleyDecodeWith(
	t *testing.T,
	path string,
	value any,
) (shelley.ShelleyGenesis, error) {
	t.Helper()
	doc := loadShelleyDoc(t, "testdata/mainnet-shelley-genesis.json")
	parent, key := shelleyParent(t, doc, path)
	if value == nil {
		delete(parent, key)
	} else {
		parent[key] = value
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	return shelley.NewShelleyGenesisFromReader(bytes.NewReader(data))
}

// A value the validator accepts must decode to the same value: the reference
// accepts an integral number written with an exponent or a trailing ".0".
func TestShelleyGenesisIntegralNumberFormsDecode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path  string
		input any
		check func(*testing.T, shelley.ShelleyGenesis)
	}{
		{
			"epochLength",
			"432000.0",
			func(t *testing.T, g shelley.ShelleyGenesis) {
				require.Equal(t, 432000, g.EpochLength)
			},
		},
		{"epochLength", "4.32e5", func(t *testing.T, g shelley.ShelleyGenesis) {
			require.Equal(t, 432000, g.EpochLength)
		}},
		{
			"maxLovelaceSupply",
			"4.5e16",
			func(t *testing.T, g shelley.ShelleyGenesis) {
				require.Equal(t, uint64(45000000000000000), g.MaxLovelaceSupply)
			},
		},
		{
			"securityParam",
			"2.16E3",
			func(t *testing.T, g shelley.ShelleyGenesis) {
				require.Equal(t, 2160, g.SecurityParam)
			},
		},
		{
			"networkMagic",
			"7.64824073e8",
			func(t *testing.T, g shelley.ShelleyGenesis) {
				require.Equal(t, uint32(764824073), g.NetworkMagic)
			},
		},
		{
			"protocolParams.minFeeA",
			"4.4e1",
			func(t *testing.T, g shelley.ShelleyGenesis) {
				require.Equal(t, uint(44), g.ProtocolParameters.MinFeeA)
			},
		},
		{
			"protocolParams.protocolVersion.major",
			"2.0",
			func(t *testing.T, g shelley.ShelleyGenesis) {
				require.Equal(
					t,
					uint(2),
					g.ProtocolParameters.ProtocolVersion.Major,
				)
			},
		},
		{
			"initialFunds",
			map[string]any{"addr": json.Number("1.0e3")},
			func(t *testing.T, g shelley.ShelleyGenesis) {
				require.Equal(
					t,
					map[string]uint64{"addr": 1000},
					g.InitialFunds,
				)
			},
		},
		{
			"activeSlotsCoeff",
			json.RawMessage(`{"numerator":1.0,"denominator":2e1}`),
			func(t *testing.T, g shelley.ShelleyGenesis) {
				require.Equal(t, "1/20", g.ActiveSlotsCoeff.String())
			},
		},
		{
			"activeSlotsCoeff",
			"5e-2",
			func(t *testing.T, g shelley.ShelleyGenesis) {
				require.Equal(t, "1/20", g.ActiveSlotsCoeff.String())
			},
		},
		{
			"protocolParams.a0",
			"3E-1",
			func(t *testing.T, g shelley.ShelleyGenesis) {
				require.Equal(t, "3/10", g.ProtocolParameters.A0.String())
			},
		},
		{"slotLength", "2.0e-1", func(t *testing.T, g shelley.ShelleyGenesis) {
			require.Equal(t, "1/5", g.SlotLength.String())
		}},
	}
	for _, tc := range tests {
		t.Run(tc.path+"="+toText(tc.input), func(t *testing.T) {
			t.Parallel()
			v := tc.input
			if s, ok := tc.input.(string); ok {
				v = json.Number(s)
			}
			g, err := shelleyDecodeWith(t, tc.path, v)
			require.NoError(t, err)
			tc.check(t, g)
		})
	}
}

func toText(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// Reduced, not raw, ratio components must fit the decoded int64; the reference
// reduces before bounding, so an unreduced Word64-overflowing ratio is valid.
func TestShelleyGenesisRatioObjectDecodesReducedComponents(t *testing.T) {
	t.Parallel()
	g, err := shelleyDecodeWith(t, "activeSlotsCoeff",
		json.RawMessage(`{"numerator":1e19,"denominator":2e19}`))
	require.NoError(t, err)
	require.Equal(t, "1/2", g.ActiveSlotsCoeff.String())
}

// A ratio whose reduced component exceeds int64 is refused by validation with
// the field named, rather than failing later in the struct decode.
func TestShelleyGenesisRatioComponentAboveInt64IsValidationError(t *testing.T) {
	t.Parallel()
	_, err := shelleyDecodeWith(t, "activeSlotsCoeff", json.RawMessage(
		`{"numerator":9223372036854775808,"denominator":18446744073709551615}`))
	require.ErrorContains(t, err, "invalid Shelley genesis")
	require.ErrorContains(t, err, "activeSlotsCoeff")
}

// The reference reads a ratio's exact "numerator" and "denominator" and ignores
// any other key, so a case variant must not replace the validated component.
func TestShelleyGenesisRatioObjectCaseVariantIsIgnored(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ path, value, want string }{
		{"protocolParams.a0", `{"numerator":1,"Numerator":-1,"denominator":2}`, "1/2"},
		{"protocolParams.rho", `{"numerator":0,"denominator":1,"NUMERATOR":2}`, "0/1"},
		{"activeSlotsCoeff", `{"numerator":1,"denominator":2,"Denominator":0}`, "1/2"},
	} {
		g, err := shelleyDecodeWith(t, tc.path, json.RawMessage(tc.value))
		require.NoError(t, err, tc.path)
		got := map[string]string{
			"protocolParams.a0":  g.ProtocolParameters.A0.String(),
			"protocolParams.rho": g.ProtocolParameters.Rho.String(),
			"activeSlotsCoeff":   g.ActiveSlotsCoeff.String(),
		}[tc.path]
		require.Equal(t, tc.want, got, tc.path)
	}
	// A required key present only as a case variant is missing.
	_, err := shelleyDecodeWith(t, "protocolParams.a0",
		json.RawMessage(`{"Numerator":1,"denominator":2}`))
	require.ErrorContains(t, err, "numerator")
}

// Every validated object drops a case variant of a declared key, so the decode
// sees the same document whether or not the variant is present.
func TestShelleyGenesisEveryValidatedObjectIgnoresCaseVariants(t *testing.T) {
	t.Parallel()
	doc := loadShelleyDoc(t, "testdata/mainnet-shelley-genesis.json")
	doc["staking"] = map[string]any{
		"pools": map[string]any{},
		"stake": map[string]any{},
	}
	doc["extraConfig"] = map[string]any{
		"initialFunds": map[string]any{"data": map[string]any{}},
	}
	var delegKey string
	for k := range doc["genDelegs"].(map[string]any) {
		delegKey = k
		break
	}
	encode := func(d map[string]any) []byte {
		data, err := json.Marshal(d)
		require.NoError(t, err)
		return data
	}
	want, err := shelley.NewShelleyGenesisFromReader(
		bytes.NewReader(encode(doc)),
	)
	require.NoError(t, err)
	for _, tc := range []struct {
		path    string
		variant string
	}{
		{"", "NetworkMagic"},
		{"protocolParams", "MinFeeA"},
		{"protocolParams.protocolVersion", "MAJOR"},
		{"genDelegs." + delegKey, "VRF"},
		{"staking", "Pools"},
		{"extraConfig", "InitialFunds"},
	} {
		t.Run(tc.path+"+"+tc.variant, func(t *testing.T) {
			t.Parallel()
			copyDoc := map[string]any{}
			require.NoError(t, json.Unmarshal(encode(doc), &copyDoc))
			cur := copyDoc
			if tc.path != "" {
				for _, part := range strings.Split(tc.path, ".") {
					cur = cur[part].(map[string]any)
				}
			}
			cur[tc.variant] = json.Number("0")
			got, err := shelley.NewShelleyGenesisFromReader(
				bytes.NewReader(encode(copyDoc)),
			)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}

func TestShelleyGenesisInitialFundsEntriesAreWord64(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		value any
		valid bool
	}{
		"null":      {json.RawMessage("null"), false},
		"string":    {"1", false},
		"negative":  {json.Number("-1"), false},
		"fraction":  {json.Number("1.5"), false},
		"too large": {json.Number("18446744073709551616"), false},
		"max":       {json.Number("18446744073709551615"), true},
		"zero":      {json.Number("0"), true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := shelleyDecodeWith(t, "initialFunds",
				map[string]any{"addr": tc.value})
			if tc.valid {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, "invalid Shelley genesis")
			require.ErrorContains(t, err, "initialFunds.addr")
		})
	}
}

// Fields held in a Go int or uint hold less than the Word64 the reference
// accepts. A value beyond the platform's type is refused by validation, with
// the field named; one at the limit still decodes.
func TestShelleyGenesisValueBeyondDecodedGoTypeIsValidationError(t *testing.T) {
	t.Parallel()
	maxInt := new(big.Int).Sub(
		new(big.Int).Lsh(big.NewInt(1), uint(strconv.IntSize-1)),
		big.NewInt(1),
	)
	maxUint := new(big.Int).Sub(
		new(big.Int).Lsh(big.NewInt(1), uint(strconv.IntSize)),
		big.NewInt(1),
	)
	plusOne := func(v *big.Int) json.Number {
		return json.Number(new(big.Int).Add(v, big.NewInt(1)).String())
	}
	for _, path := range []string{
		"securityParam",
		"epochLength",
		"slotsPerKESPeriod",
		"maxKESEvolutions",
		"updateQuorum",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			_, err := shelleyDecodeWith(t, path, json.Number(maxInt.String()))
			require.NoError(t, err)
			_, err = shelleyDecodeWith(t, path, plusOne(maxInt))
			require.ErrorContains(t, err, "invalid Shelley genesis")
			require.ErrorContains(t, err, path)
		})
	}
	for _, path := range []string{
		"protocolParams.minFeeA",
		"protocolParams.minFeeB",
		"protocolParams.keyDeposit",
		"protocolParams.poolDeposit",
		"protocolParams.minUTxOValue",
		"protocolParams.minPoolCost",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			_, err := shelleyDecodeWith(t, path, json.Number(maxUint.String()))
			require.NoError(t, err)
			// Above the platform uint: 2^64 on 64-bit is also above Word64.
			_, err = shelleyDecodeWith(t, path, plusOne(maxUint))
			require.ErrorContains(t, err, "invalid Shelley genesis")
			require.ErrorContains(t, err, path)
		})
	}
}

// aeson resolves a repeated key to its first occurrence as a whole value;
// encoding/json would merge the two objects, letting the last one win.
func TestShelleyGenesisRepeatedObjectKeyTakesFirstOccurrence(t *testing.T) {
	t.Parallel()
	doc := loadShelleyDoc(t, "testdata/mainnet-shelley-genesis.json")
	pp, err := json.Marshal(doc["protocolParams"])
	require.NoError(t, err)
	first := map[string]any{}
	dec := json.NewDecoder(bytes.NewReader(pp))
	dec.UseNumber()
	require.NoError(t, dec.Decode(&first))
	last := maps.Clone(first)
	delete(last, "minPoolCost")
	lastJSON, err := json.Marshal(last)
	require.NoError(t, err)
	first["minPoolCost"] = json.Number("7")
	firstJSON, err := json.Marshal(first)
	require.NoError(t, err)
	delete(doc, "protocolParams")
	rest, err := json.Marshal(doc)
	require.NoError(t, err)
	// rest is {...}; splice both protocolParams members before its end.
	spliced := strings.TrimSuffix(string(rest), "}") +
		`,"protocolParams":` + string(firstJSON) +
		`,"protocolParams":` + string(lastJSON) + `}`
	g, err := shelley.NewShelleyGenesisFromReader(strings.NewReader(spliced))
	require.NoError(t, err)
	require.Equal(t, uint(7), g.ProtocolParameters.MinPoolCost)
	require.Equal(t, g.ProtocolParameters.MinFeeA, uint(44))
}

// Every value the validator accepts must decode: a failure in the struct
// decode after a successful validation means the two disagree on that value.
func TestShelleyGenesisValidatedValuesAlwaysDecode(t *testing.T) {
	t.Parallel()
	paths := []string{
		"networkMagic", "securityParam", "epochLength", "slotsPerKESPeriod",
		"maxKESEvolutions", "slotLength", "updateQuorum", "maxLovelaceSupply",
		"activeSlotsCoeff", "initialFunds",
		"protocolParams.minFeeA", "protocolParams.minFeeB",
		"protocolParams.maxBlockBodySize", "protocolParams.maxTxSize",
		"protocolParams.maxBlockHeaderSize", "protocolParams.keyDeposit",
		"protocolParams.poolDeposit", "protocolParams.eMax",
		"protocolParams.nOpt", "protocolParams.a0", "protocolParams.rho",
		"protocolParams.tau", "protocolParams.decentralisationParam",
		"protocolParams.protocolVersion.major",
		"protocolParams.protocolVersion.minor",
		"protocolParams.minUTxOValue", "protocolParams.minPoolCost",
	}
	literals := []string{
		"0", "-0", "1", "1.0", "1e3", "4.5e16", "0.5", "5e-1", "1e0",
		"9223372036854775807", "9223372036854775808",
		"18446744073709551615", "18446744073709551616",
		"4294967295", "4294967296", "2147483648", "65536",
		"0e999999999", "1e-1000000000", "1e5000", "1e-5000",
		`{"numerator":1.0,"denominator":2}`,
		`{"numerator":9223372036854775808,"denominator":18446744073709551615}`,
		`{"numerator":-1,"denominator":-2}`,
		`null`, `"1"`, `1.5`, `[]`, `{}`,
	}
	for _, path := range paths {
		for _, lit := range literals {
			t.Run(path+"="+lit, func(t *testing.T) {
				t.Parallel()
				value := any(json.RawMessage(lit))
				if path == "initialFunds" {
					value = map[string]any{"a": json.RawMessage(lit)}
				}
				_, err := shelleyDecodeWith(t, path, value)
				if err != nil {
					require.ErrorContains(t, err, "invalid Shelley genesis",
						"decode failed after validation passed")
				}
			})
		}
	}
}

// The reference ignores unknown keys in every object it reads, so an extra key
// next to a validated one must not reach the struct decode.
func TestShelleyGenesisUnknownKeysInValidatedObjectsAreIgnored(t *testing.T) {
	t.Parallel()
	doc := loadShelleyDoc(t, "testdata/mainnet-shelley-genesis.json")
	var delegKey string
	for k := range doc["genDelegs"].(map[string]any) {
		delegKey = k
		break
	}
	want, err := shelleyDecodeWith(t, "genDelegs."+delegKey+"x", nil)
	require.NoError(t, err)
	for _, tc := range []struct {
		path  string
		value any
	}{
		{"genDelegs." + delegKey + ".note", json.Number("1")},
		{"genDelegs." + delegKey + ".nested", map[string]any{"a": []any{}}},
		{"protocolParams.protocolVersion.note", "x"},
		{"protocolParams.note", json.Number("1")},
	} {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			got, err := shelleyDecodeWith(t, tc.path, tc.value)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}
