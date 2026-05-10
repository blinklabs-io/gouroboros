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

func TestDiagnosticGetNodeAtOffset(t *testing.T) {
	// {"a": [1, 2]}
	data, err := hex.DecodeString("a16161820102")
	require.NoError(t, err)
	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	// Out-of-range
	assert.Nil(t, node.GetNodeAtOffset(-1))
	assert.Nil(t, node.GetNodeAtOffset(len(data)))

	// Root map at offset 0
	root := node.GetNodeAtOffset(0)
	require.NotNil(t, root)
	assert.Equal(t, cbor.DiagTypeMap, root.Type)

	// Inside the key "a" (offsets 1-2)
	keyNode := node.GetNodeAtOffset(1)
	require.NotNil(t, keyNode)
	assert.Equal(t, cbor.DiagTypeText, keyNode.Type)
	assert.Equal(t, "a", keyNode.Value)

	// Array header
	arrNode := node.GetNodeAtOffset(3)
	require.NotNil(t, arrNode)
	assert.Equal(t, cbor.DiagTypeArray, arrNode.Type)

	// Array element 0 at offset 4
	elem0 := node.GetNodeAtOffset(4)
	require.NotNil(t, elem0)
	assert.Equal(t, cbor.DiagTypeUint, elem0.Type)
	assert.Equal(t, uint64(1), elem0.Value)

	// Array element 1 at offset 5
	elem1 := node.GetNodeAtOffset(5)
	require.NotNil(t, elem1)
	assert.Equal(t, cbor.DiagTypeUint, elem1.Type)
	assert.Equal(t, uint64(2), elem1.Value)
}

func TestDiagnosticGetPathToOffset(t *testing.T) {
	// {"a": [1, 2]}
	data, err := hex.DecodeString("a16161820102")
	require.NoError(t, err)
	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	assert.Nil(t, node.GetPathToOffset(-1))
	assert.Nil(t, node.GetPathToOffset(len(data)))

	// Root map: empty path
	assert.Equal(t, []string{}, node.GetPathToOffset(0))

	// Inside key "a"
	assert.Equal(t, []string{"a"}, node.GetPathToOffset(1))
	assert.Equal(t, []string{"a"}, node.GetPathToOffset(2))

	// Inside the array value
	assert.Equal(t, []string{"a"}, node.GetPathToOffset(3))

	// Array elements
	assert.Equal(t, []string{"a", "[0]"}, node.GetPathToOffset(4))
	assert.Equal(t, []string{"a", "[1]"}, node.GetPathToOffset(5))
}

func TestDiagnosticGetPathToOffsetNested(t *testing.T) {
	// {"body": {"outputs": [{"address": h'01'}]}}
	// Hex fixture (avoids non-deterministic map encoding).
	data, err := hex.DecodeString(
		"a164626f6479a1676f75747075747381a167616464726573734101",
	)
	require.NoError(t, err)

	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	// Walk to the address bytes node to confirm offsets are right.
	require.Equal(t, cbor.DiagTypeMap, node.Type)
	require.Len(t, node.Children, 2)
	bodyVal := node.Children[1]
	require.Equal(t, cbor.DiagTypeMap, bodyVal.Type)
	outputsVal := bodyVal.Children[1]
	require.Equal(t, cbor.DiagTypeArray, outputsVal.Type)
	firstOutput := outputsVal.Children[0]
	require.Equal(t, cbor.DiagTypeMap, firstOutput.Type)
	addrNode := firstOutput.Children[1]
	require.Equal(t, cbor.DiagTypeBytes, addrNode.Type)

	path := node.GetPathToOffset(addrNode.Offset)
	assert.Equal(t, []string{"body", "outputs", "[0]", "address"}, path)
}

func TestDiagnosticFormatHexDumpArray(t *testing.T) {
	// [1, 2, 3]
	data, err := hex.DecodeString("83010203")
	require.NoError(t, err)
	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	out := node.FormatHexDump(cbor.DiagnosticOptions{})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	require.Len(t, lines, 6) // header, separator, root, 3 elements
	assert.Contains(t, lines[0], "Offset")
	assert.Contains(t, lines[0], "Hex")
	assert.Contains(t, lines[0], "Structure")
	assert.Contains(t, lines[2], "0000")
	assert.Contains(t, lines[2], "83")
	assert.Contains(t, lines[2], "array(3)")
	assert.Contains(t, lines[3], "0001")
	assert.Contains(t, lines[3], "01")
	assert.Contains(t, lines[3], "| 1")
	assert.Contains(t, lines[4], "0002")
	assert.Contains(t, lines[4], "| 2")
	assert.Contains(t, lines[5], "0003")
	assert.Contains(t, lines[5], "| 3")
}

func TestDiagnosticFormatHexDumpMapAndBytes(t *testing.T) {
	// {0: h'deadbeef'}
	data, err := hex.DecodeString("a10044deadbeef")
	require.NoError(t, err)
	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	out := node.FormatHexDump(cbor.DiagnosticOptions{})
	assert.Contains(t, out, "map(1)")
	assert.Contains(t, out, "key: 0")
	assert.Contains(t, out, "value: bytes(4)")
	assert.Contains(t, out, "de ad be ef")
}

func TestDiagnosticFormatHexDumpBytesTruncated(t *testing.T) {
	// 32-byte byte string, encoded with 0x58 0x20 + 32 bytes
	prefix, err := hex.DecodeString("5820")
	require.NoError(t, err)
	contents := make([]byte, 32)
	for i := range contents {
		contents[i] = byte(i)
	}
	data := append(prefix, contents...)
	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	out := node.FormatHexDump(cbor.DiagnosticOptions{MaxByteLength: 4})
	assert.Contains(t, out, "bytes(32)")
	assert.Contains(t, out, "58 20")
	assert.Contains(t, out, "00 01 02 03 ... (32 bytes)")
}

func TestDiagnosticFormatHexDumpTag(t *testing.T) {
	// 258([1])
	data, err := hex.DecodeString("d901028101")
	require.NoError(t, err)
	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	out := node.FormatHexDump(cbor.DiagnosticOptions{})
	assert.Contains(t, out, "tag(258)")
	assert.Contains(t, out, "d9 01 02")
	assert.Contains(t, out, "| array(1)")
	assert.Contains(t, out, "| | 1")
}

func TestStreamDecoderDecodeDiagnostic(t *testing.T) {
	// [1, 2, 3]
	data, err := hex.DecodeString("83010203")
	require.NoError(t, err)
	dec, err := cbor.NewStreamDecoder(data)
	require.NoError(t, err)

	node, err := dec.DecodeDiagnostic()
	require.NoError(t, err)
	require.NotNil(t, node)
	assert.Equal(t, cbor.DiagTypeArray, node.Type)
	assert.Equal(t, 0, node.Offset)
	assert.Equal(t, 4, node.Length)
	require.Len(t, node.Children, 3)
	assert.True(t, dec.EOF())
}

func TestStreamDecoderDecodeDiagnosticEOF(t *testing.T) {
	dec, err := cbor.NewStreamDecoder([]byte{})
	require.NoError(t, err)
	_, err = dec.DecodeDiagnostic()
	require.Error(t, err)
}

func TestStreamDecoderDecodeAllDiagnostic(t *testing.T) {
	// Three consecutive items: uint 1, uint 2, array [3, 4]
	data, err := hex.DecodeString("0102820304")
	require.NoError(t, err)
	dec, err := cbor.NewStreamDecoder(data)
	require.NoError(t, err)

	nodes, err := dec.DecodeAllDiagnostic()
	require.NoError(t, err)
	require.Len(t, nodes, 3)

	// First: uint 1 at offset 0
	assert.Equal(t, cbor.DiagTypeUint, nodes[0].Type)
	assert.Equal(t, 0, nodes[0].Offset)
	assert.Equal(t, uint64(1), nodes[0].Value)

	// Second: uint 2 at offset 1
	assert.Equal(t, cbor.DiagTypeUint, nodes[1].Type)
	assert.Equal(t, 1, nodes[1].Offset)
	assert.Equal(t, uint64(2), nodes[1].Value)

	// Third: array [3, 4] at offset 2
	assert.Equal(t, cbor.DiagTypeArray, nodes[2].Type)
	assert.Equal(t, 2, nodes[2].Offset)
	assert.Equal(t, 3, nodes[2].Length)

	assert.True(t, dec.EOF())
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
