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

func TestParseDiagnosticArrayOffsets(t *testing.T) {
	data, err := hex.DecodeString("83010203")
	require.NoError(t, err)

	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	assert.Equal(t, cbor.DiagTypeArray, node.Type)
	assert.Equal(t, 0, node.Offset)
	assert.Equal(t, 4, node.Length)
	require.Len(t, node.Children, 3)

	assert.Equal(t, 1, node.Children[0].Offset)
	assert.Equal(t, 2, node.Children[1].Offset)
	assert.Equal(t, 3, node.Children[2].Offset)
}

func TestParseDiagnosticIndefiniteArray(t *testing.T) {
	data, err := hex.DecodeString("9f0102ff")
	require.NoError(t, err)

	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	assert.Equal(t, cbor.DiagTypeArray, node.Type)
	assert.True(t, node.Indefinite)
	require.Len(t, node.Children, 2)
}

func TestFormatDiagnosticCompactWithOffsets(t *testing.T) {
	data, err := hex.DecodeString("83010203")
	require.NoError(t, err)

	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	formatted := node.FormatDiagnostic(cbor.DiagnosticOptions{
		ShowOffsets: true,
	})
	assert.Equal(t, "[1, 2, 3]  / @0-4 /", formatted)
}

func TestFormatDiagnosticPrettyWithOffsets(t *testing.T) {
	data, err := hex.DecodeString("83010203")
	require.NoError(t, err)

	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	formatted := node.FormatDiagnosticPretty(cbor.DiagnosticOptions{
		ShowOffsets: true,
	})
	expected := "[  / @0 /\n" +
		"  1  / @1 /,\n" +
		"  2  / @2 /,\n" +
		"  3  / @3 /\n" +
		"]  / end @4 /"
	assert.Equal(t, expected, formatted)
}

func TestParseDiagnosticMapAndTag(t *testing.T) {
	// {1: 2}
	mapData, err := hex.DecodeString("a10102")
	require.NoError(t, err)
	mapNode, err := cbor.ParseDiagnostic(mapData)
	require.NoError(t, err)
	assert.Equal(t, cbor.DiagTypeMap, mapNode.Type)
	require.Len(t, mapNode.Children, 2)

	// 258([1]) - Cardano set tag wrapping [1]
	tagData, err := hex.DecodeString("d901028101")
	require.NoError(t, err)
	tagNode, err := cbor.ParseDiagnostic(tagData)
	require.NoError(t, err)
	assert.Equal(t, cbor.DiagTypeTag, tagNode.Type)
	require.NotNil(t, tagNode.Tag)
	assert.Equal(t, uint64(cbor.CborTagSet), *tagNode.Tag)
	assert.Equal(
		t,
		"set([1])",
		tagNode.FormatDiagnostic(cbor.DiagnosticOptions{CardanoAware: true}),
	)
	assert.Equal(
		t,
		"258([1])",
		tagNode.FormatDiagnostic(cbor.DiagnosticOptions{CardanoAware: false}),
	)
}

func TestParseDiagnosticCollectionOverflow(t *testing.T) {
	// array header with uint64 max length should be rejected
	data, err := hex.DecodeString("9bffffffffffffffff")
	require.NoError(t, err)
	_, err = cbor.ParseDiagnostic(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds int32 range")
}

func TestFormatDiagnosticLimits(t *testing.T) {
	data, err := hex.DecodeString("83010203")
	require.NoError(t, err)
	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	limitedItems := node.FormatDiagnostic(cbor.DiagnosticOptions{
		MaxArrayItems: 2,
	})
	assert.Equal(t, "[1, 2, ...]", limitedItems)

	limitedDepth := node.FormatDiagnostic(cbor.DiagnosticOptions{
		MaxDepth: 1,
	})
	assert.Equal(t, "[..., ..., ...]", limitedDepth)
}

func TestFormatDiagnosticPrettyNestedMapValue(t *testing.T) {
	// {"k": [1, 2]}
	data, err := hex.DecodeString("a1616b820102")
	require.NoError(t, err)
	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	formatted := node.FormatDiagnosticPretty(cbor.DiagnosticOptions{})
	expected := "{\n" +
		"  \"k\":\n" +
		"    [\n" +
		"      1,\n" +
		"      2\n" +
		"    ]\n" +
		"}"
	assert.Equal(t, expected, formatted)
}

func TestParseDiagnosticIndefiniteByteString(t *testing.T) {
	data, err := hex.DecodeString("5f4201024103ff")
	require.NoError(t, err)

	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	assert.Equal(t, cbor.DiagTypeBytes, node.Type)
	assert.True(t, node.Indefinite)
	require.Len(t, node.Children, 2)
	assert.Equal(
		t,
		"(_ h'0102', h'03')",
		node.FormatDiagnostic(cbor.DiagnosticOptions{}),
	)
}

func TestParseDiagnosticIndefiniteTextString(t *testing.T) {
	data, err := hex.DecodeString("7f61616162ff")
	require.NoError(t, err)

	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	assert.Equal(t, cbor.DiagTypeText, node.Type)
	assert.True(t, node.Indefinite)
	require.Len(t, node.Children, 2)
	assert.Equal(
		t,
		"(_ \"a\", \"b\")",
		node.FormatDiagnostic(cbor.DiagnosticOptions{}),
	)
}

func TestParseDiagnosticMaxNestedLevels(t *testing.T) {
	data := make([]byte, 0, 260)
	for i := 0; i < 257; i++ {
		data = append(data, 0x81)
	}
	data = append(data, 0x00)

	_, err := cbor.ParseDiagnostic(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max depth of 256")
}

func TestFormatDiagnosticPrettyShowHex(t *testing.T) {
	data, err := hex.DecodeString("83010203")
	require.NoError(t, err)
	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	formatted := node.FormatDiagnosticPretty(cbor.DiagnosticOptions{
		ShowHex: true,
	})
	assert.True(t, strings.Contains(formatted, "/ 83010203 /"))
}

