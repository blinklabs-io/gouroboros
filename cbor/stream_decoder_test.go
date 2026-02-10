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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamDecoderCreation(t *testing.T) {
	// Test creating a stream decoder with valid data
	data, _ := hex.DecodeString("83010203") // [1, 2, 3]
	dec, err := cbor.NewStreamDecoder(data)
	require.NoError(t, err)
	require.NotNil(t, dec)
}

func TestStreamDecoderPosition(t *testing.T) {
	// [1, 2, 3]
	data, _ := hex.DecodeString("83010203")
	dec, err := cbor.NewStreamDecoder(data)
	require.NoError(t, err)

	// Initial position should be 0
	assert.Equal(t, 0, dec.Position())

	// Decode the array
	var result []uint64
	_, _, err = dec.Decode(&result)
	require.NoError(t, err)

	// Position should be at end
	assert.Equal(t, len(data), dec.Position())
}

func TestStreamDecoderDecodeWithOffsets(t *testing.T) {
	// [1, 2, 3]
	data, _ := hex.DecodeString("83010203")
	dec, err := cbor.NewStreamDecoder(data)
	require.NoError(t, err)

	var result []uint64
	start, length, err := dec.Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, 0, start)
	assert.Equal(t, 4, length)
	assert.Equal(t, []uint64{1, 2, 3}, result)
}

func TestStreamDecoderSkip(t *testing.T) {
	// Two CBOR items: [1] followed by [2, 3]
	data, _ := hex.DecodeString("8101820203")
	dec, err := cbor.NewStreamDecoder(data)
	require.NoError(t, err)

	// Skip first item
	start, length, err := dec.Skip()
	require.NoError(t, err)
	assert.Equal(t, 0, start)
	assert.Equal(t, 2, length)

	// Decode second item
	var result []uint64
	start, length, err = dec.Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, 2, start)
	assert.Equal(t, 3, length)
	assert.Equal(t, []uint64{2, 3}, result)
}

func TestStreamDecoderDecodeRaw(t *testing.T) {
	// [1, 2]
	data, _ := hex.DecodeString("820102")
	dec, err := cbor.NewStreamDecoder(data)
	require.NoError(t, err)

	var result []uint64
	start, rawBytes, err := dec.DecodeRaw(&result)
	require.NoError(t, err)

	assert.Equal(t, 0, start)
	assert.Equal(t, data, rawBytes)
	assert.Equal(t, []uint64{1, 2}, result)
}

func TestStreamDecoderRawBytes(t *testing.T) {
	data, _ := hex.DecodeString("83010203")
	dec, err := cbor.NewStreamDecoder(data)
	require.NoError(t, err)

	// Get raw bytes at specific offset and length
	raw := dec.RawBytes(0, 4)
	assert.Equal(t, data, raw)

	// Edge cases
	assert.Nil(t, dec.RawBytes(-1, 2))            // negative offset
	assert.Nil(t, dec.RawBytes(0, -1))            // negative length
	assert.Nil(t, dec.RawBytes(0, 100))           // beyond bounds
	assert.Nil(t, dec.RawBytes(100, 1))           // offset beyond bounds
	assert.Equal(t, []byte{}, dec.RawBytes(0, 0)) // zero length (valid)
}

func TestStreamDecoderData(t *testing.T) {
	data, _ := hex.DecodeString("83010203")
	dec, err := cbor.NewStreamDecoder(data)
	require.NoError(t, err)

	assert.Equal(t, data, dec.Data())
}

func TestStreamDecoderEOF(t *testing.T) {
	data, _ := hex.DecodeString("83010203")
	dec, err := cbor.NewStreamDecoder(data)
	require.NoError(t, err)

	// Not at EOF initially
	assert.False(t, dec.EOF())

	// Decode the data
	var result any
	_, _, err = dec.Decode(&result)
	require.NoError(t, err)

	// Now at EOF
	assert.True(t, dec.EOF())
}

func TestStreamDecoderAdvance(t *testing.T) {
	data, _ := hex.DecodeString("8301020383040506")
	dec, err := cbor.NewStreamDecoder(data)
	require.NoError(t, err)

	// Advance past first array header (1 byte) and first element (1 byte)
	err = dec.Advance(2)
	require.NoError(t, err)
	assert.Equal(t, 2, dec.Position())

	// Test error cases
	err = dec.Advance(-1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "negative")

	err = dec.Advance(1000)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceed")
}

func TestStreamDecoderDecodeArrayHeader(t *testing.T) {
	tests := []struct {
		name        string
		cborHex     string
		expectLen   int
		expectError bool
	}{
		{
			name:      "small array",
			cborHex:   "83010203",
			expectLen: 3,
		},
		{
			name:      "array with 1-byte length",
			cborHex:   "9818000102030405060708090a0b0c0d0e0f101112131415161718",
			expectLen: 24,
		},
		{
			name:        "not an array (map)",
			cborHex:     "a201020304",
			expectError: true,
		},
		{
			name:        "empty data",
			cborHex:     "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := hex.DecodeString(tt.cborHex)
			dec, err := cbor.NewStreamDecoder(data)
			require.NoError(t, err)

			length, _, headerLen, err := dec.DecodeArrayHeader()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectLen, length)
				assert.Greater(t, headerLen, 0)
			}
		})
	}
}

func TestStreamDecoderDecodeMapHeader(t *testing.T) {
	tests := []struct {
		name        string
		cborHex     string
		expectLen   int
		expectError bool
	}{
		{
			name:      "small map",
			cborHex:   "a201020304",
			expectLen: 2,
		},
		{
			name:        "not a map (array)",
			cborHex:     "83010203",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := hex.DecodeString(tt.cborHex)
			dec, err := cbor.NewStreamDecoder(data)
			require.NoError(t, err)

			length, _, headerLen, err := dec.DecodeMapHeader()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectLen, length)
				assert.Greater(t, headerLen, 0)
			}
		})
	}
}

func TestStreamDecoderSkipN(t *testing.T) {
	// Three items: 1, 2, 3
	data, _ := hex.DecodeString("010203")
	dec, err := cbor.NewStreamDecoder(data)
	require.NoError(t, err)

	// Skip 2 items
	start, length, err := dec.SkipN(2)
	require.NoError(t, err)
	assert.Equal(t, 0, start)
	assert.Equal(t, 2, length)

	// Decode remaining item
	var result uint64
	_, _, err = dec.Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), result)
}

func TestStreamDecoderSkipNZero(t *testing.T) {
	data, _ := hex.DecodeString("01")
	dec, err := cbor.NewStreamDecoder(data)
	require.NoError(t, err)

	// Skip 0 items should be valid
	start, length, err := dec.SkipN(0)
	require.NoError(t, err)
	assert.Equal(t, 0, start)
	assert.Equal(t, 0, length)
}

func TestStreamDecoderDecodeArrayItems(t *testing.T) {
	// [1, 2, 3]
	data, _ := hex.DecodeString("83010203")
	dec, err := cbor.NewStreamDecoder(data)
	require.NoError(t, err)

	var items []struct {
		index  int
		offset int
		length int
		data   []byte
	}

	callback := func(index int, offset int, length int, data []byte) error {
		items = append(items, struct {
			index  int
			offset int
			length int
			data   []byte
		}{index, offset, length, data})
		return nil
	}

	start, length, err := dec.DecodeArrayItems(callback)
	require.NoError(t, err)

	assert.Equal(t, 0, start)
	assert.Equal(t, 4, length)
	assert.Len(t, items, 3)

	// Verify item offsets - first item at offset 1 (after header)
	assert.Equal(t, 0, items[0].index)
	assert.Equal(t, 1, items[0].offset)
	assert.Equal(t, 1, items[0].length)
}

func TestArrayInfo(t *testing.T) {
	tests := []struct {
		name          string
		cborHex       string
		expectCount   int
		expectHeader  uint32
		expectIndef   bool
		expectInvalid bool
	}{
		{
			name:         "small array (0)",
			cborHex:      "80",
			expectCount:  0,
			expectHeader: 1,
		},
		{
			name:         "small array (3)",
			cborHex:      "83010203",
			expectCount:  3,
			expectHeader: 1,
		},
		{
			name:         "array with 23 elements",
			cborHex:      "970102030405060708090a0b0c0d0e0f10111213141516",
			expectCount:  23,
			expectHeader: 1,
		},
		{
			name:         "array with 1-byte length (24)",
			cborHex:      "9818000102030405060708090a0b0c0d0e0f101112131415161718",
			expectCount:  24,
			expectHeader: 2,
		},
		{
			name:         "indefinite array",
			cborHex:      "9f010203ff",
			expectCount:  0,
			expectHeader: 1,
			expectIndef:  true,
		},
		{
			name:          "not an array (map)",
			cborHex:       "a0",
			expectInvalid: true,
		},
		{
			name:          "empty input",
			cborHex:       "",
			expectInvalid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := hex.DecodeString(tt.cborHex)
			count, headerSize, isIndefinite := cbor.ArrayInfo(data)

			if tt.expectInvalid {
				assert.Equal(t, -1, count)
				return
			}

			assert.Equal(t, tt.expectCount, count)
			assert.Equal(t, tt.expectHeader, headerSize)
			assert.Equal(t, tt.expectIndef, isIndefinite)
		})
	}
}

func TestMapInfo(t *testing.T) {
	tests := []struct {
		name          string
		cborHex       string
		expectCount   int
		expectHeader  uint32
		expectIndef   bool
		expectInvalid bool
	}{
		{
			name:         "empty map",
			cborHex:      "a0",
			expectCount:  0,
			expectHeader: 1,
		},
		{
			name:         "small map (2 pairs)",
			cborHex:      "a201020304",
			expectCount:  2,
			expectHeader: 1,
		},
		{
			name:         "indefinite map",
			cborHex:      "bf01020304ff",
			expectCount:  0,
			expectHeader: 1,
			expectIndef:  true,
		},
		{
			name:          "not a map (array)",
			cborHex:       "80",
			expectInvalid: true,
		},
		{
			name:          "empty input",
			cborHex:       "",
			expectInvalid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := hex.DecodeString(tt.cborHex)
			count, headerSize, isIndefinite := cbor.MapInfo(data)

			if tt.expectInvalid {
				assert.Equal(t, -1, count)
				return
			}

			assert.Equal(t, tt.expectCount, count)
			assert.Equal(t, tt.expectHeader, headerSize)
			assert.Equal(t, tt.expectIndef, isIndefinite)
		})
	}
}

func TestArrayHeaderSize(t *testing.T) {
	tests := []struct {
		length     int
		expectSize uint32
	}{
		{0, 1},
		{1, 1},
		{23, 1},
		{24, 2},
		{255, 2},
		{256, 3},
		{65535, 3},
		{65536, 5},
	}

	for _, tt := range tests {
		size := cbor.ArrayHeaderSize(tt.length)
		assert.Equal(t, tt.expectSize, size, "length %d", tt.length)
	}
}

func TestDecodeEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		cborHex     string
		expectError bool
	}{
		{
			name:        "empty input",
			cborHex:     "",
			expectError: true,
		},
		{
			name:        "truncated array",
			cborHex:     "82",
			expectError: true,
		},
		{
			name:        "truncated map",
			cborHex:     "a2",
			expectError: true,
		},
		{
			name:        "valid empty array",
			cborHex:     "80",
			expectError: false,
		},
		{
			name:        "valid empty map",
			cborHex:     "a0",
			expectError: false,
		},
		{
			name:        "valid integer",
			cborHex:     "1903e8",
			expectError: false,
		},
		{
			name:        "valid negative integer",
			cborHex:     "3903e7",
			expectError: false,
		},
		{
			name:        "valid bytestring",
			cborHex:     "44deadbeef",
			expectError: false,
		},
		{
			name:        "valid text string",
			cborHex:     "6568656c6c6f",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := hex.DecodeString(tt.cborHex)
			var dest any
			_, err := cbor.Decode(data, &dest)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestListLengthEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		cborHex     string
		expectLen   int
		expectError bool
	}{
		{
			name:      "empty array",
			cborHex:   "80",
			expectLen: 0,
		},
		{
			name:      "array with 23 elements",
			cborHex:   "970102030405060708090a0b0c0d0e0f10111213141516",
			expectLen: 23,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := hex.DecodeString(tt.cborHex)
			length, err := cbor.ListLength(data)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectLen, length)
			}
		})
	}
}

func TestDecodeIdFromListEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		cborHex     string
		expectId    int
		expectError bool
	}{
		{
			name:        "empty list",
			cborHex:     "80",
			expectError: true,
		},
		{
			name:     "list with zero",
			cborHex:  "8100",
			expectId: 0,
		},
		{
			name:     "list with max simple uint",
			cborHex:  "8117",
			expectId: 23,
		},
		{
			name:     "list with 24 (1-byte)",
			cborHex:  "811818",
			expectId: 24,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := hex.DecodeString(tt.cborHex)
			id, err := cbor.DecodeIdFromList(data)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectId, id)
			}
		})
	}
}

func TestDecodeStoreCbor(t *testing.T) {
	var store cbor.DecodeStoreCbor

	// Test setting CBOR data
	data := []byte{0x83, 0x01, 0x02, 0x03}
	store.SetCbor(data)

	// Verify data is stored
	assert.Equal(t, data, store.Cbor())

	// Verify data is a copy (modifying original doesn't affect stored)
	data[0] = 0xff
	assert.NotEqual(t, data, store.Cbor())

	// Test setting nil
	store.SetCbor(nil)
	assert.Nil(t, store.Cbor())
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		object any
	}{
		{"integer", uint64(42)},
		{"string", "hello"},
		{"array", []any{uint64(1), uint64(2), uint64(3)}},
		{"map", map[any]any{"a": uint64(1), "b": uint64(2)}},
		{"nested", []any{map[any]any{"x": uint64(1)}, []any{uint64(2)}}},
		{"empty array", []any{}},
		{"empty map", map[any]any{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded, err := cbor.Encode(tt.object)
			require.NoError(t, err)

			// Decode
			var decoded any
			_, err = cbor.Decode(encoded, &decoded)
			require.NoError(t, err)

			// Re-encode to verify
			reEncoded, err := cbor.Encode(decoded)
			require.NoError(t, err)

			assert.Equal(t, encoded, reEncoded)
		})
	}
}
