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

package genesisjson_test

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	gj "github.com/blinklabs-io/gouroboros/internal/genesisjson"
)

func TestNormalizeRewritesCheckedNumbersToPlainForm(t *testing.T) {
	t.Parallel()
	out, err := gj.Normalize(
		"",
		[]byte(
			`{"a":4.32e5,"b":1.0,"c":5E-1,"d":{"numerator":1e19,"denominator":2e19},"e":2.0e-1,"f":{"k":1e3}}`,
		),
		gj.Required("a", gj.Uint(64)),
		gj.Required("b", gj.NonZeroUint64()),
		gj.Required("c", gj.Interval(gj.UnitInterval)),
		gj.Required("d", gj.Interval(gj.UnitInterval)),
		gj.Required("e", gj.Number()),
		gj.Required("f", gj.MapOf(gj.Uint(16))),
	)
	require.NoError(t, err)
	require.JSONEq(
		t,
		`{"a":432000,"b":1,"c":0.5,"d":{"numerator":1,"denominator":2},"e":0.2,"f":{"k":1000}}`,
		string(out),
	)
}

// Keys and values the validator does not examine must reach the decode
// byte-for-byte, so their meaning cannot change.
func TestNormalizeLeavesUncheckedValuesUntouched(t *testing.T) {
	t.Parallel()
	out, err := gj.Normalize(
		"",
		[]byte(
			`{"a":1,"costModels":{"PlutusV1":[1.0,2e3,-0],"x":1E2},"unknown":[1.50]}`,
		),
		gj.Required("a", gj.Uint(8)),
		gj.Required("costModels", gj.AnyObject()),
	)
	require.NoError(t, err)
	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out, &got))
	require.Equal(
		t,
		`{"PlutusV1":[1.0,2e3,-0],"x":1E2}`,
		string(got["costModels"]),
	)
	require.Equal(t, `[1.50]`, string(got["unknown"]))
}

func TestIntervalRatioObjectRejectsCaseVariants(t *testing.T) {
	t.Parallel()
	for _, doc := range []string{
		`{"numerator":1,"Numerator":-1,"denominator":2}`,
		`{"numerator":1,"denominator":2,"DENOMINATOR":0}`,
	} {
		_, err := gj.Interval(gj.UnitInterval)("v", json.RawMessage(doc))
		require.ErrorContains(t, err, "differs only in case", doc)
		require.ErrorContains(t, err, "v:", doc)
	}
}

func TestIntervalRatioObjectComponentLimit(t *testing.T) {
	t.Parallel()
	_, err := gj.Interval(gj.UnitInterval)("v", json.RawMessage(
		`{"numerator":9223372036854775808,"denominator":18446744073709551615}`))
	require.ErrorContains(t, err, "int64")
	out, err := gj.Interval(gj.UnitInterval)("v", json.RawMessage(
		`{"numerator":-1,"denominator":-2}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"numerator":1,"denominator":2}`, string(out))
}

func TestGoIntBoundsFollowPlatformInt(t *testing.T) {
	t.Parallel()
	limit := "9223372036854775807"
	if strconv.IntSize == 32 {
		limit = "2147483647"
	}
	_, err := gj.GoInt(64)("v", json.RawMessage(limit))
	require.NoError(t, err)
	_, err = gj.GoInt(64)("v", json.RawMessage(strconv.Quote("1")))
	require.Error(t, err)
	over := "9223372036854775808"
	if strconv.IntSize == 32 {
		over = "2147483648"
	}
	_, err = gj.GoInt(64)("v", json.RawMessage(over))
	require.ErrorContains(t, err, "decoded Go int")
	_, err = gj.NonZeroGoInt()("v", json.RawMessage("0"))
	require.ErrorContains(t, err, "zero")
}

// aeson's (<|>) falls through to the next spelling on null or a value of the
// wrong type, but a range failure is final.
func TestValidateAlternateFallthrough(t *testing.T) {
	t.Parallel()
	fields := []gj.Field{gj.RequiredAnyOf(gj.Uint(8), "first", "second")}
	for _, doc := range []string{
		`{"first":null,"second":1}`,
		`{"first":"x","second":1}`,
		`{"first":1.5,"second":1}`,
	} {
		require.NoError(t, gj.Validate("", []byte(doc), fields...), doc)
	}
	for _, doc := range []string{
		`{"first":256,"second":1}`,
		`{"first":"x","second":256}`,
		`{"first":"x"}`,
		`{"first":null}`,
	} {
		require.Error(t, gj.Validate("", []byte(doc), fields...), doc)
	}
}

func TestArrayOrObjectRejectsNull(t *testing.T) {
	t.Parallel()
	for doc, valid := range map[string]bool{
		`[]`: true, `{}`: true, `null`: false, `1`: false, `"x"`: false,
	} {
		_, err := gj.ArrayOrObject()("v", json.RawMessage(doc))
		require.Equal(t, valid, err == nil, doc)
	}
}

func TestNumberRejectsExponentBeyondDecodeLimit(t *testing.T) {
	t.Parallel()
	_, err := gj.Number()("v", json.RawMessage("1e-1000000000"))
	require.ErrorContains(t, err, "exponent out of range")
	out, err := gj.Number()("v", json.RawMessage("1.5e2"))
	require.NoError(t, err)
	require.Equal(t, "150", string(out))
}
