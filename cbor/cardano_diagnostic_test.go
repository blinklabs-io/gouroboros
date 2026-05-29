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
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCardanoTxBodyLabelsConwayKeys(t *testing.T) {
	// The Conway CDDL keeps key 6 (update) absent and adds 19-22; these
	// reference checks pin the label map to the spec so renaming or
	// re-numbering across eras gets caught.
	cases := map[int]string{
		0:  "inputs",
		1:  "outputs",
		2:  "fee",
		11: "script_data_hash",
		13: "collateral_inputs",
		14: "required_signers",
		19: "voting_procedures",
		22: "donation",
	}
	for k, want := range cases {
		got, ok := cbor.CardanoTxBodyLabels[k]
		require.True(t, ok, "missing key %d", k)
		assert.Equal(t, want, got)
	}
	// Keys that are intentionally retired/reserved in Conway.
	for _, retired := range []int{10, 12} {
		_, ok := cbor.CardanoTxBodyLabels[retired]
		assert.False(t, ok, "key %d should not be labelled", retired)
	}
}

func TestCardanoWitnessLabels(t *testing.T) {
	assert.Equal(t, "vkey_witnesses", cbor.CardanoWitnessLabels[0])
	assert.Equal(t, "redeemers", cbor.CardanoWitnessLabels[5])
	assert.Equal(t, "plutus_v3_scripts", cbor.CardanoWitnessLabels[7])
}

func TestCardanoBlockLabels(t *testing.T) {
	require.Len(t, cbor.CardanoBlockLabels, 7)
	assert.Equal(t, "header", cbor.CardanoBlockLabels[0])
	assert.Equal(t, "invalid_transactions", cbor.CardanoBlockLabels[4])
	assert.Equal(t, "leios_cert", cbor.CardanoBlockLabels[5])
	assert.Equal(t, "peras_cert", cbor.CardanoBlockLabels[6])
}

func TestCardanoPlutusConstrFromTag(t *testing.T) {
	cases := []struct {
		tag     uint64
		wantIdx uint64
		wantOk  bool
	}{
		{121, 0, true},    // Constr_0
		{127, 6, true},    // Constr_6
		{1280, 7, true},   // Constr_7
		{1400, 127, true}, // Constr_127
		{102, 0, false},   // general fallback tag — not in compact range
		{24, 0, false},
	}
	for _, c := range cases {
		gotIdx, gotOk := cbor.PlutusConstrFromTag(c.tag)
		assert.Equal(t, c.wantOk, gotOk, "tag %d", c.tag)
		if c.wantOk {
			assert.Equal(t, c.wantIdx, gotIdx, "tag %d", c.tag)
		}
	}
}

func TestFormatCardanoNativeScriptPubkey(t *testing.T) {
	// [0, h'aa']  -> script_pubkey(key_hash: h'aa')
	data, err := hex.DecodeString("82" + "00" + "41aa")
	require.NoError(t, err)
	out, err := cbor.FormatNativeScript(data, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	assert.Contains(t, out, "script_pubkey")
	assert.Contains(t, out, "key_hash: h'aa'")
}

func TestFormatCardanoNativeScriptAllNested(t *testing.T) {
	// [1, [[0, h'aa'], [4, 42]]]  ->  script_all(scripts: [script_pubkey(...), invalid_before(slot: 42)])
	// inner1 = [0, h'aa']                = 82 00 41 aa
	// inner2 = [4, 42 as uint(18)]       = 82 04 18 2a
	// scripts = [inner1, inner2]         = 82 <inner1> <inner2>
	// outer = [1, scripts]               = 82 01 <scripts>
	data, err := hex.DecodeString("8201" + "82" + "820041aa" + "8204182a")
	require.NoError(t, err)
	out, err := cbor.FormatNativeScript(data, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	assert.Contains(t, out, "script_all(")
	assert.Contains(t, out, "script_pubkey")
	assert.Contains(t, out, "invalid_before(slot: 42)")
}

func TestFormatCardanoNativeScriptNofK(t *testing.T) {
	// [3, 1, [[0, h'bb']]] -> script_n_of_k(n: 1, scripts: [script_pubkey(...)])
	// inner = [0, h'bb'] -> 82 00 41 bb
	// scripts = [inner]   -> 81 <inner>
	// outer = [3, 1, scripts] -> 83 03 01 <scripts>
	data, err := hex.DecodeString("83" + "03" + "01" + "81" + "820041bb")
	require.NoError(t, err)
	out, err := cbor.FormatNativeScript(data, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	assert.Contains(t, out, "script_n_of_k(n: 1")
	assert.Contains(t, out, "script_pubkey")
}

func TestFormatCardanoPlutusDataCompactConstructor(t *testing.T) {
	// Tag 121 (Constr_0) wrapping the empty array [].
	data, err := hex.DecodeString("d87980")
	require.NoError(t, err)
	out, err := cbor.FormatPlutusData(data, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	assert.Contains(t, out, "Constr_0([")
}

func TestFormatCardanoPlutusDataGeneralConstructor(t *testing.T) {
	// Tag 102 wrapping [7, []] -> Constr_7([])
	// 0xd8 0x66 = tag(102); 0x82 = array(2); 0x07 = 7; 0x80 = array(0)
	data, err := hex.DecodeString("d866" + "82" + "07" + "80")
	require.NoError(t, err)
	out, err := cbor.FormatPlutusData(data, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	assert.Contains(t, out, "Constr_7([")
}

func TestFormatCardanoPlutusDataNestedConstructorInArray(t *testing.T) {
	// Outer: tag 121 (Constr_0) wrapping array containing
	//   - tag 122 (Constr_1) wrapping []
	//   - tag 102 [3, []] (general Constr_3)
	// Expect both inner constructors to render as Constr_N(...) form.
	// d879 = tag(121)
	// 82   = array(2) — outer constructor payload
	// d87a 80 = tag(122) [] = Constr_1([])
	// d866 82 03 80 = tag(102) [3, []]
	data, err := hex.DecodeString(
		"d879" + "82" + "d87a80" + "d8668203" + "80",
	)
	require.NoError(t, err)
	out, err := cbor.FormatPlutusData(data, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	assert.Contains(t, out, "Constr_0([")
	assert.Contains(t, out, "Constr_1([")
	assert.Contains(t, out, "Constr_3([")
}

func TestFormatCardanoPlutusDataArrayShowsOffsets(t *testing.T) {
	// Tag 121 wrapping array [1, 2] — non-empty array path should still
	// emit the / @start-end / range comment when ShowOffsets is set.
	data, err := hex.DecodeString("d879" + "82" + "01" + "02")
	require.NoError(t, err)
	out, err := cbor.FormatPlutusData(
		data, cbor.DiagnosticOptions{ShowOffsets: true},
	)
	require.NoError(t, err)
	assert.Contains(t, out, "Constr_0([")
	assert.Contains(t, out, "/ @2-")
}

func TestFormatCardanoPlutusDataPrimitive(t *testing.T) {
	// Plain integer 42 — should round-trip through the formatter.
	data, err := hex.DecodeString("182a")
	require.NoError(t, err)
	out, err := cbor.FormatPlutusData(data, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	assert.Equal(t, "42", out)
}

func TestFormatCardanoTransactionBodyLabels(t *testing.T) {
	// Transaction = [body, witness_set, true, null]
	// body = {0: [], 1: [], 2: 100}
	//   a3 00 80 01 80 02 18 64
	// witness_set = {} -> a0
	// true -> f5, null -> f6
	// tx = 84 <body> a0 f5 f6
	body := "a3" + "00" + "80" + "01" + "80" + "02" + "1864"
	tx := "84" + body + "a0" + "f5" + "f6"
	data, err := hex.DecodeString(tx)
	require.NoError(t, err)

	out, err := cbor.FormatTransactionDiagnostic(data, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	assert.Contains(t, out, "transaction [")
	assert.Contains(t, out, "body: {")
	assert.Contains(t, out, "witness_set: {")
	assert.Contains(t, out, "is_valid: true")
	assert.Contains(t, out, "auxiliary_data: null")
	// Body key labels appear as / comment / markers.
	assert.Contains(t, out, "/ inputs /")
	assert.Contains(t, out, "/ outputs /")
	assert.Contains(t, out, "/ fee /")
}

func TestFormatCardanoTransactionPreAlonzoShape(t *testing.T) {
	// Pre-Alonzo (Shelley/Allegra/Mary) transactions are 3-element
	// [body, witness_set, auxiliary_data] — no is_valid bool.
	// The third position must render as auxiliary_data, not is_valid.
	body := "a1" + "02" + "1864" // {2: 100}
	auxiliary := "a0"            // empty map stands in for aux data
	tx := "83" + body + "a0" + auxiliary
	data, err := hex.DecodeString(tx)
	require.NoError(t, err)

	out, err := cbor.FormatTransactionDiagnostic(data, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	assert.Contains(t, out, "auxiliary_data: {}")
	assert.NotContains(t, out, "is_valid:")
}

func TestFormatCardanoTransactionAlonzoShapeRequiresBool(t *testing.T) {
	// 4-element tx where element 2 is *not* a bool must not be labelled
	// is_valid — defensive fallback to a numeric-suffix label.
	body := "a1" + "02" + "1864"
	tx := "84" + body + "a0" + "1864" + "a0" // [body, ws, 100, {}]
	data, err := hex.DecodeString(tx)
	require.NoError(t, err)

	out, err := cbor.FormatTransactionDiagnostic(data, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	assert.NotContains(t, out, "is_valid: 100")
	assert.Contains(t, out, "field_2: 100")
}

func TestFormatCardanoTransactionWithOffsets(t *testing.T) {
	body := "a1" + "02" + "1864" // {2: 100}
	tx := "82" + body + "a0"     // [body, {}]
	data, err := hex.DecodeString(tx)
	require.NoError(t, err)

	out, err := cbor.FormatTransactionDiagnostic(
		data, cbor.DiagnosticOptions{ShowOffsets: true},
	)
	require.NoError(t, err)
	assert.Contains(t, out, "/ @0-")
}

func TestFormatCardanoTransactionRejectsNonArray(t *testing.T) {
	data, err := hex.DecodeString("a0") // {}
	require.NoError(t, err)
	_, err = cbor.FormatTransactionDiagnostic(data, cbor.DiagnosticOptions{})
	require.Error(t, err)
}

func TestFormatCardanoBlockLabels(t *testing.T) {
	// inner block = [header, [], [], {}, []]
	// header = 81 00 (single-element array, just a placeholder)
	// 85 <header> 80 80 a0 80
	inner := "85" + "8100" + "80" + "80" + "a0" + "80"
	// Wrap as [era, inner] under no tag (era = 7 for Conway, encoded as 0x07)
	full := "82" + "07" + inner
	data, err := hex.DecodeString(full)
	require.NoError(t, err)

	out, err := cbor.FormatBlockDiagnostic(data, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	assert.Contains(t, out, "block [")
	assert.Contains(t, out, "/ era 7 /")
	assert.Contains(t, out, "header: [")
	assert.Contains(t, out, "transaction_bodies: []")
	assert.Contains(t, out, "transaction_witnesses: []")
	assert.Contains(t, out, "auxiliary_data_set: {}")
	assert.Contains(t, out, "invalid_transactions: []")
}

func TestFormatCardanoBlockTag24Wrapped(t *testing.T) {
	// inner block = [header, [], [], {}, []]
	// header = 81 00
	inner := "85" + "8100" + "80" + "80" + "a0" + "80"
	innerBytes, err := hex.DecodeString(inner)
	require.NoError(t, err)

	// Wrap inner as a byte string under tag 24 (CborTagCbor).
	// tag(24) = 0xd8 0x18; byte string of len(innerBytes) — use 0x58 lN form.
	var wrapped []byte
	wrapped = append(wrapped, 0xd8, 0x18)
	if len(innerBytes) < 24 {
		wrapped = append(wrapped, 0x40|byte(len(innerBytes)))
	} else if len(innerBytes) <= 0xff {
		wrapped = append(wrapped, 0x58, byte(len(innerBytes)))
	} else {
		t.Fatalf("inner too large for test helper")
	}
	wrapped = append(wrapped, innerBytes...)

	out, err := cbor.FormatBlockDiagnostic(wrapped, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	assert.Contains(t, out, "block [")
	assert.Contains(t, out, "header:")
	assert.Contains(t, out, "transaction_bodies: []")
	assert.Contains(t, out, "invalid_transactions: []")
}

func TestFormatCardanoBlockInnerOnly(t *testing.T) {
	// Bare 5-element block without era wrapper should still format.
	inner := "85" + "8100" + "80" + "80" + "a0" + "80"
	data, err := hex.DecodeString(inner)
	require.NoError(t, err)

	out, err := cbor.FormatBlockDiagnostic(data, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	assert.Contains(t, out, "header:")
	assert.NotContains(t, out, "/ era ")
}

func TestCardanoAnnotateAddressesUsesRegisteredFormatter(t *testing.T) {
	// Register a stub formatter that recognises 29-byte inputs.
	cbor.RegisterAddressFormatter(func(b []byte) (string, bool) {
		if len(b) == 29 {
			return "addr_test1stub", true
		}
		return "", false
	})
	t.Cleanup(func() { cbor.RegisterAddressFormatter(nil) })

	addr := strings.Repeat("aa", 29) // 29 raw bytes
	// {0: h'aa...aa'}
	data, err := hex.DecodeString("a100581d" + addr)
	require.NoError(t, err)
	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	annotated := cbor.AnnotateAddresses(node)
	require.NotNil(t, annotated)
	// Locate the bytes node and check it carries the comment.
	require.Equal(t, cbor.DiagTypeMap, annotated.Type)
	require.Len(t, annotated.Children, 2)
	valNode := annotated.Children[1]
	require.Equal(t, cbor.DiagTypeBytes, valNode.Type)
	assert.Equal(t, "addr_test1stub", valNode.Comment)
}

func TestCardanoAnnotateAddressesNoFormatter(t *testing.T) {
	// Without a registered formatter, AnnotateAddresses must return the
	// tree unchanged (no panics, no comments).
	cbor.RegisterAddressFormatter(nil)
	addr := strings.Repeat("bb", 29)
	data, err := hex.DecodeString("581d" + addr)
	require.NoError(t, err)
	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	annotated := cbor.AnnotateAddresses(node)
	require.NotNil(t, annotated)
	assert.Equal(t, "", annotated.Comment)
}

func TestFormatCardanoTransactionBytesGetAddressComment(t *testing.T) {
	cbor.RegisterAddressFormatter(func(b []byte) (string, bool) {
		if len(b) == 29 {
			return "addr1qstubbed", true
		}
		return "", false
	})
	t.Cleanup(func() { cbor.RegisterAddressFormatter(nil) })

	// Build a transaction with an output [addr, value].
	// output = [h'<29 bytes>', 1000000]
	addr := strings.Repeat("cc", 29)
	output := "82" + "581d" + addr + "1a000f4240" // 1_000_000 as uint32
	body := "a1" + "01" + "81" + output           // {1: [output]}
	tx := "82" + body + "a0"
	data, err := hex.DecodeString(tx)
	require.NoError(t, err)

	out, err := cbor.FormatTransactionDiagnostic(data, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	assert.Contains(t, out, "/ addr1qstubbed /")
}
