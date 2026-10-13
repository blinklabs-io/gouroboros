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

package cbor_test

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// limitTestNesting is well past every MaxDepth used below.
	limitTestNesting = 200
	// limitTestLeaf is h'cafe', which must not render once elided.
	limitTestLeaf  = "42cafe"
	limitTestDepth = 4
	limitTestItems = 3
)

// limitTestFormatter is the signature shared by the Cardano formatters.
type limitTestFormatter func([]byte, cbor.DiagnosticOptions) (string, error)

// limitTestHex decodes a hex fixture, failing the test on invalid input.
func limitTestHex(t *testing.T, s string) []byte {
	t.Helper()
	data, err := hex.DecodeString(s)
	require.NoError(t, err)
	return data
}

// limitTestBytes returns n one-byte byte strings h'00', h'01', ...
func limitTestBytes(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "41%02x", i)
	}
	return b.String()
}

// limitTestByteString encodes data as a CBOR byte string.
func limitTestByteString(data []byte) string {
	return fmt.Sprintf("59%04x", len(data)) + hex.EncodeToString(data)
}

// requireDepthElided checks that output stops at MaxDepth: the deep leaf is
// gone, the elision marker is present and the output stays small.
func requireDepthElided(t *testing.T, out string) {
	t.Helper()
	assert.NotContains(t, out, "cafe")
	assert.Contains(t, out, "...")
	assert.Less(t, len(out), 1024, "output:\n%s", out)
}

// requireItemsElided checks that only the first limitTestItems entries of a
// h'00', h'01', ... collection render.
func requireItemsElided(t *testing.T, out string) {
	t.Helper()
	assert.Contains(t, out, fmt.Sprintf("h'%02x'", limitTestItems-1))
	assert.NotContains(t, out, fmt.Sprintf("h'%02x'", limitTestItems))
	assert.Contains(t, out, "...")
}

// TestFormatCardanoTransactionHonorsMaxArrayItems checks that
// FormatTransactionDiagnostic stops at MaxArrayItems both in the top-level
// transaction array and in a collection nested inside the body (#2712).
func TestFormatCardanoTransactionHonorsMaxArrayItems(t *testing.T) {
	// Top level: [{}, {}, true, null, h'04', h'05'].
	data := limitTestHex(t, "86"+"a0"+"a0"+"f5"+"f6"+"4104"+"4105")
	out, err := cbor.FormatTransactionDiagnostic(
		data,
		cbor.DiagnosticOptions{MaxArrayItems: limitTestItems},
	)
	require.NoError(t, err)
	assert.Contains(t, out, "is_valid: true")
	assert.NotContains(t, out, "auxiliary_data")
	assert.NotContains(t, out, "h'04'")
	assert.Contains(t, out, "...")

	// Nested: [{0: [20 inputs]}, {}].
	data = limitTestHex(t, "82"+"a1"+"00"+"94"+limitTestBytes(20)+"a0")
	out, err = cbor.FormatTransactionDiagnostic(
		data,
		cbor.DiagnosticOptions{MaxArrayItems: limitTestItems},
	)
	require.NoError(t, err)
	requireItemsElided(t, out)
}

// TestFormatCardanoTransactionHonorsMaxDepth checks that
// FormatTransactionDiagnostic elides values nested beyond MaxDepth, whether
// the nesting is arrays, Plutus constructors or plain tags, in both the body
// map and the trailing transaction fields (#2712).
func TestFormatCardanoTransactionHonorsMaxDepth(t *testing.T) {
	tests := map[string]string{
		"array": strings.Repeat("81", limitTestNesting) + limitTestLeaf,
		"constructor": strings.Repeat("d87981", limitTestNesting) +
			limitTestLeaf,
		"tag": strings.Repeat("c1", limitTestNesting) + limitTestLeaf,
	}
	for name, deep := range tests {
		t.Run(name, func(t *testing.T) {
			// [{0: deep}, {}, true, deep]
			data := limitTestHex(t, "84"+"a1"+"00"+deep+"a0"+"f5"+deep)
			out, err := cbor.FormatTransactionDiagnostic(
				data,
				cbor.DiagnosticOptions{MaxDepth: limitTestDepth},
			)
			require.NoError(t, err)
			requireDepthElided(t, out)
		})
	}
}

// TestFormatCardanoBlockHonorsMaxArrayItems checks that FormatBlockDiagnostic
// stops at MaxArrayItems in the outer block array and in the nested
// transaction-bodies collection (#2713).
func TestFormatCardanoBlockHonorsMaxArrayItems(t *testing.T) {
	// [header, [20 bodies], [], {}, [], null, null]
	block := "87" + "8100" + "94" + limitTestBytes(20) + "80" + "a0" + "80" +
		"f6" + "f6"
	out, err := cbor.FormatBlockDiagnostic(
		limitTestHex(t, block),
		cbor.DiagnosticOptions{MaxArrayItems: limitTestItems},
	)
	require.NoError(t, err)
	assert.Contains(t, out, "transaction_witnesses")
	assert.NotContains(t, out, "auxiliary_data_set")
	assert.NotContains(t, out, "peras_cert")
	requireItemsElided(t, out)
}

// TestFormatCardanoBlockHonorsMaxDepth checks that FormatBlockDiagnostic
// elides a deeply nested header for a bare block, an [era, block] pair and a
// tag-24 wrapped block, and for constructor nesting that shares a line (#2713).
func TestFormatCardanoBlockHonorsMaxDepth(t *testing.T) {
	deep := strings.Repeat("81", limitTestNesting) + limitTestLeaf
	block := "85" + deep + "80" + "80" + "a0" + "80"
	tests := map[string]string{
		"bare": block,
		"era":  "82" + "07" + block,
		"constructor": "85" + strings.Repeat("d87981", limitTestNesting) +
			limitTestLeaf + "80" + "80" + "a0" + "80",
		"tag-24": "d818" + limitTestByteString(
			limitTestHex(t, "82"+"07"+block),
		),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			out, err := cbor.FormatBlockDiagnostic(
				limitTestHex(t, data),
				cbor.DiagnosticOptions{MaxDepth: limitTestDepth},
			)
			require.NoError(t, err)
			assert.Contains(t, out, "header:")
			requireDepthElided(t, out)
		})
	}
}

// TestFormatCardanoPlutusDataHonorsMaxDepth checks that FormatPlutusData
// elides deep chains of compact constructors, tag-102 general constructors
// and maps, and renders no more constructors than MaxDepth allows (#2714).
func TestFormatCardanoPlutusDataHonorsMaxDepth(t *testing.T) {
	tests := map[string]string{
		"constructor": strings.Repeat("d87981", limitTestNesting) +
			limitTestLeaf,
		// Tag 102 [0, [inner]].
		"general constructor": strings.Repeat(
			"d866"+"82"+"00"+"81",
			limitTestNesting,
		) + limitTestLeaf,
		"map": strings.Repeat("a100", limitTestNesting) + limitTestLeaf,
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			out, err := cbor.FormatPlutusData(
				limitTestHex(t, data),
				cbor.DiagnosticOptions{MaxDepth: limitTestDepth},
			)
			require.NoError(t, err)
			requireDepthElided(t, out)
			assert.LessOrEqual(
				t,
				strings.Count(out, "Constr_"),
				limitTestDepth,
			)
		})
	}
}

// TestFormatCardanoPlutusDataHonorsMaxArrayItems checks that FormatPlutusData
// stops at MaxArrayItems for map pairs and for constructor fields (#2714).
func TestFormatCardanoPlutusDataHonorsMaxArrayItems(t *testing.T) {
	var pairs strings.Builder
	for i := range 20 {
		fmt.Fprintf(&pairs, "41%02x%02x", i, i)
	}
	tests := map[string]string{
		"map":         "b4" + pairs.String(),
		"constructor": "d879" + "94" + limitTestBytes(20),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			out, err := cbor.FormatPlutusData(
				limitTestHex(t, data),
				cbor.DiagnosticOptions{MaxArrayItems: limitTestItems},
			)
			require.NoError(t, err)
			requireItemsElided(t, out)
		})
	}
}

// TestFormatCardanoNativeScriptHonorsMaxDepth checks that FormatNativeScript
// elides deep script_all and script_n_of_k chains before reaching the inner
// script_pubkey (#2715).
func TestFormatCardanoNativeScriptHonorsMaxDepth(t *testing.T) {
	leaf := "8200" + limitTestLeaf // script_pubkey(h'cafe')
	tests := map[string]string{
		"all":    strings.Repeat("8201"+"81", limitTestNesting) + leaf,
		"n_of_k": strings.Repeat("8303"+"01"+"81", limitTestNesting) + leaf,
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			out, err := cbor.FormatNativeScript(
				limitTestHex(t, data),
				cbor.DiagnosticOptions{MaxDepth: limitTestDepth},
			)
			require.NoError(t, err)
			requireDepthElided(t, out)
			assert.NotContains(t, out, "script_pubkey")
		})
	}
}

// TestFormatCardanoNativeScriptFieldsHonorMaxDepth checks that the scalar
// n and slot fields elide at MaxDepth like the other script fields, and that
// a signed n renders its actual value (#2715).
func TestFormatCardanoNativeScriptFieldsHonorMaxDepth(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		maxDepth int
		want     string
	}{
		{
			name:     "n_of_k elided",
			data:     "8303" + "01" + "80", // [3, 1, []]
			maxDepth: 1,
			want:     "script_n_of_k(n: ..., scripts: ...)",
		},
		{
			name:     "invalid_before elided",
			data:     "8204" + "182a", // [4, 42]
			maxDepth: 1,
			want:     "invalid_before(slot: ...)",
		},
		{
			name:     "invalid_hereafter within limit",
			data:     "8205" + "182a", // [5, 42]
			maxDepth: 2,
			want:     "invalid_hereafter(slot: 42)",
		},
		{
			name:     "negative n",
			data:     "8303" + "20" + "80", // [3, -1, []]
			maxDepth: 0,
			want:     "script_n_of_k(n: -1, scripts: [])",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := cbor.FormatNativeScript(
				limitTestHex(t, tc.data),
				cbor.DiagnosticOptions{MaxDepth: tc.maxDepth},
			)
			require.NoError(t, err)
			assert.Equal(t, tc.want, out)
		})
	}
}

// TestFormatCardanoNativeScriptHonorsMaxArrayItems checks that
// FormatNativeScript stops at MaxArrayItems in a script_any child list
// (#2715).
func TestFormatCardanoNativeScriptHonorsMaxArrayItems(t *testing.T) {
	var scripts strings.Builder
	for i := range 20 {
		fmt.Fprintf(&scripts, "8200"+"41%02x", i)
	}
	out, err := cbor.FormatNativeScript(
		limitTestHex(t, "8202"+"94"+scripts.String()),
		cbor.DiagnosticOptions{MaxArrayItems: limitTestItems},
	)
	require.NoError(t, err)
	assert.Contains(t, out, "script_any(")
	requireItemsElided(t, out)
}

// TestFormatCardanoUnchangedWithinLimits checks that each Cardano formatter
// renders identically with and without limits when the value fits within
// them, so labels and layout are preserved (#2712-#2715).
func TestFormatCardanoUnchangedWithinLimits(t *testing.T) {
	nested := strings.Repeat("d87981", 3) + "d866" + "82" + "07" + "82" +
		"a1" + "4100" + "81" + "01" + limitTestLeaf
	tests := []struct {
		name   string
		format limitTestFormatter
		data   string
	}{
		{
			name:   "transaction",
			format: cbor.FormatTransactionDiagnostic,
			data: "84" + "a2" + "00" + "82" + "4100" + "4101" + "02" +
				"1864" + "a1" + "00" + nested + "f5" + "c1" + "81" + "01",
		},
		{
			name:   "block",
			format: cbor.FormatBlockDiagnostic,
			data: "82" + "07" + "85" + "82" + nested + "4100" + "80" +
				"80" + "a0" + "80",
		},
		{
			name:   "plutus data",
			format: cbor.FormatPlutusData,
			data:   nested,
		},
		{
			name:   "native script",
			format: cbor.FormatNativeScript,
			data: "8303" + "01" + "82" + "8201" + "81" + "8200" + "4100" +
				"8204" + "182a",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := limitTestHex(t, tc.data)
			want, err := tc.format(data, cbor.DiagnosticOptions{})
			require.NoError(t, err)
			got, err := tc.format(data, cbor.DiagnosticOptions{
				MaxDepth:      64,
				MaxArrayItems: 64,
			})
			require.NoError(t, err)
			assert.Equal(t, want, got)
			assert.NotContains(t, want, "...")
		})
	}
}
