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
package genesisjson_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	gj "github.com/blinklabs-io/gouroboros/internal/genesisjson"
)

func TestIntervalMatchesBoundedRatioFromJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		kind  gj.IntervalKind
		input string
		valid bool
	}{
		{"unit zero", gj.UnitInterval, "0", true},
		{"unit negative zero", gj.UnitInterval, "-0.0", true},
		{"unit one", gj.UnitInterval, "1", true},
		{"unit exponent", gj.UnitInterval, "1e0", true},
		{"unit small exponent", gj.UnitInterval, "5E-1", true},
		{"unit above", gj.UnitInterval, "1.0000000000000000001", false},
		{
			"unit nineteen decimals",
			gj.UnitInterval,
			"0.1234567890123456789",
			true,
		},
		{
			"unit twenty decimals",
			gj.UnitInterval,
			"0.12345678901234567891",
			false,
		},
		{
			"unit trailing zeros ignored",
			gj.UnitInterval,
			"0.50000000000000000000000",
			true,
		},
		{"unit huge exponent", gj.UnitInterval, "1e999999999999", false},
		{
			"unit huge negative exponent",
			gj.UnitInterval,
			"1e-999999999999",
			false,
		},
		{"unit string", gj.UnitInterval, `"0.5"`, false},
		{"unit null", gj.UnitInterval, "null", false},
		{"unit bool", gj.UnitInterval, "true", false},
		{
			"unit object",
			gj.UnitInterval,
			`{"numerator":1,"denominator":2}`,
			true,
		},
		{
			"unit object reduces",
			gj.UnitInterval,
			`{"numerator":2,"denominator":4}`,
			true,
		},
		{
			"unit object both negative normalises",
			gj.UnitInterval,
			`{"numerator":-1,"denominator":-2}`,
			true,
		},
		{
			"unit object zero denominator",
			gj.UnitInterval,
			`{"numerator":1,"denominator":0}`,
			false,
		},
		{
			"unit object missing denominator",
			gj.UnitInterval,
			`{"numerator":1}`,
			false,
		},
		{
			"unit object fractional numerator",
			gj.UnitInterval,
			`{"numerator":0.5,"denominator":1}`,
			false,
		},
		{
			"unit object above one",
			gj.UnitInterval,
			`{"numerator":3,"denominator":2}`,
			false,
		},
		{"positive zero", gj.PositiveUnitInterval, "0", false},
		{
			"positive epsilon",
			gj.PositiveUnitInterval,
			"0.0000000000000000001",
			true,
		},
		{"positive one", gj.PositiveUnitInterval, "1", true},
		{"non-negative zero", gj.NonNegativeInterval, "0", true},
		{"non-negative negative", gj.NonNegativeInterval, "-1", false},
		{
			"non-negative word64 max",
			gj.NonNegativeInterval,
			"18446744073709551615",
			true,
		},
		{
			"non-negative above word64",
			gj.NonNegativeInterval,
			"18446744073709551616",
			false,
		},
		{"non-negative exponent form", gj.NonNegativeInterval, "1.5e1", true},
		{
			"non-negative denominator above word64",
			gj.NonNegativeInterval,
			`{"numerator":1,"denominator":18446744073709551616}`,
			false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := gj.Interval(tc.kind)("v", json.RawMessage(tc.input))
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestUintAcceptsIntegralNumberForms(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		bits  uint
		valid bool
	}{
		{"0", 16, true},
		{"65535", 16, true},
		{"65536", 16, false},
		{"-1", 16, false},
		{"1e3", 16, true},
		{"1.0", 16, true},
		{"1.5", 16, false},
		{"1e99999", 64, false},
		{`"1"`, 64, false},
		{"null", 64, false},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			_, err := gj.Uint(tc.bits)("v", json.RawMessage(tc.input))
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestValidateOptionalNullAndAlternates(t *testing.T) {
	t.Parallel()
	fields := []gj.Field{
		gj.Optional("opt", gj.Uint(8)),
		gj.RequiredAnyOf(gj.Uint(8), "first", "second"),
	}
	require.NoError(
		t,
		gj.Validate("", []byte(`{"opt":null,"second":1}`), fields...),
	)
	require.NoError(
		t,
		gj.Validate("", []byte(`{"first":1,"extra":[]}`), fields...),
	)
	err := gj.Validate("", []byte(`{"opt":256,"first":1}`), fields...)
	require.ErrorContains(t, err, "opt")
	err = gj.Validate("ctx", []byte(`{}`), fields...)
	require.ErrorContains(
		t,
		err,
		`ctx: missing required field "first" or "second"`,
	)
	// A null alternate is skipped, so only its absence is reported.
	err = gj.Validate("", []byte(`{"first":null}`), fields...)
	require.ErrorContains(t, err, "first")
	require.Error(t, gj.Validate("", []byte(`[]`), fields...))
}

// encoding/json matches struct fields case-insensitively and lets a later key
// overwrite an earlier one, so a key differing from a validated field only in
// case would replace the validated value after validation.
func TestValidateRejectsCaseVariantOfDeclaredKey(t *testing.T) {
	t.Parallel()
	fields := []gj.Field{
		gj.Required("rho", gj.Interval(gj.UnitInterval)),
		gj.Optional("opt", gj.Uint(8)),
		gj.RequiredAnyOf(gj.Uint(8), "memory", "exUnitsMem"),
	}
	require.NoError(t, gj.Validate(
		"",
		[]byte(`{"rho":0.5,"memory":1,"unrelated":1}`),
		fields...,
	))
	for _, doc := range []string{
		`{"rho":0.5,"RHO":5,"memory":1}`,
		`{"rho":0.5,"memory":1,"Opt":300}`,
		`{"rho":0.5,"memory":1,"EXUNITSMEM":1}`,
	} {
		err := gj.Validate("", []byte(doc), fields...)
		require.ErrorContains(t, err, "differs only in case", doc)
	}
}

// A coefficient this long is outside every bounded type, and converting it
// costs superlinear time, so it is refused before conversion.
func TestNumberRejectsExcessiveSignificantDigits(t *testing.T) {
	t.Parallel()
	long := "1" + strings.Repeat("7", 2000)
	for _, check := range []gj.Check{
		gj.Uint(64),
		gj.Interval(gj.NonNegativeInterval),
		gj.Number(),
	} {
		_, err := check("v", json.RawMessage(long))
		require.ErrorContains(t, err, "too many significant digits")
	}
	// Zeros around the coefficient do not count.
	_, err := gj.Uint(64)(
		"v",
		json.RawMessage("0.5"+strings.Repeat("0", 2000)+"e1"),
	)
	require.NoError(t, err)
}
