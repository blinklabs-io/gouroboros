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

// Test vectors matching the existing constructor tests in value_test.go
// to ensure compatibility between the old Constructor and new ConstructorEncoder/Decoder.
var constructorDecoderTestDefs = []struct {
	name    string
	cborHex string
	tag     uint
	fields  []any
}{
	{
		name:    "alternative 1 (tag 122)",
		cborHex: "D87A83010203",
		tag:     1,
		fields:  []any{uint64(1), uint64(2), uint64(3)},
	},
	{
		name:    "alternative 15 (tag 1288)",
		cborHex: "D9050883030405",
		tag:     15,
		fields:  []any{uint64(3), uint64(4), uint64(5)},
	},
	{
		name:    "alternative 999 (tag 101)",
		cborHex: "D865821903E7820607",
		tag:     999,
		fields:  []any{uint64(6), uint64(7)},
	},
}

func TestConstructorDecoderDecode(t *testing.T) {
	for _, tt := range constructorDecoderTestDefs {
		t.Run(tt.name, func(t *testing.T) {
			cborData, err := hex.DecodeString(tt.cborHex)
			require.NoError(t, err)

			var cd cbor.ConstructorDecoder
			_, err = cbor.Decode(cborData, &cd)
			require.NoError(t, err)

			assert.Equal(t, tt.tag, cd.Tag())

			// Decode fields and verify
			var fields []any
			err = cd.DecodeFields(&fields)
			require.NoError(t, err)
			assert.Equal(t, tt.fields, fields)
		})
	}
}

func TestConstructorDecoderRoundTrip(t *testing.T) {
	for _, tt := range constructorDecoderTestDefs {
		t.Run(tt.name, func(t *testing.T) {
			cborData, err := hex.DecodeString(tt.cborHex)
			require.NoError(t, err)

			// Decode
			var cd cbor.ConstructorDecoder
			_, err = cbor.Decode(cborData, &cd)
			require.NoError(t, err)

			// Re-encode (should use stored bytes)
			reEncoded, err := cbor.Encode(&cd)
			require.NoError(t, err)

			assert.Equal(t,
				strings.ToLower(tt.cborHex),
				hex.EncodeToString(reEncoded),
			)
		})
	}
}

func TestConstructorEncoderEncode(t *testing.T) {
	for _, tt := range constructorDecoderTestDefs {
		t.Run(tt.name, func(t *testing.T) {
			ce := cbor.NewConstructorEncoder(tt.tag, tt.fields)
			assert.Equal(t, tt.tag, ce.Tag())

			encoded, err := cbor.Encode(&ce)
			require.NoError(t, err)

			assert.Equal(t,
				strings.ToLower(tt.cborHex),
				hex.EncodeToString(encoded),
			)
		})
	}
}

func TestConstructorEncoderDecoderCompatibility(t *testing.T) {
	// Encode with ConstructorEncoder, decode with ConstructorDecoder
	for _, tt := range constructorDecoderTestDefs {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			ce := cbor.NewConstructorEncoder(tt.tag, tt.fields)
			encoded, err := cbor.Encode(&ce)
			require.NoError(t, err)

			// Decode
			var cd cbor.ConstructorDecoder
			_, err = cbor.Decode(encoded, &cd)
			require.NoError(t, err)

			assert.Equal(t, tt.tag, cd.Tag())

			var fields []any
			err = cd.DecodeFields(&fields)
			require.NoError(t, err)
			assert.Equal(t, tt.fields, fields)
		})
	}
}

func TestConstructorDecoderRebuildWithoutStoredBytes(t *testing.T) {
	// Test MarshalCBOR when no stored bytes are available (manually constructed)
	for _, tt := range constructorDecoderTestDefs {
		t.Run(tt.name, func(t *testing.T) {
			cborData, err := hex.DecodeString(tt.cborHex)
			require.NoError(t, err)

			// Decode normally
			var cd cbor.ConstructorDecoder
			_, err = cbor.Decode(cborData, &cd)
			require.NoError(t, err)

			// Clear stored CBOR to force rebuild
			cd.SetCbor(nil)

			// Re-encode (should rebuild from tag + fields)
			reEncoded, err := cbor.Encode(&cd)
			require.NoError(t, err)

			assert.Equal(t,
				strings.ToLower(tt.cborHex),
				hex.EncodeToString(reEncoded),
			)
		})
	}
}

func TestConstructorDecoderAllAlternativeRanges(t *testing.T) {
	tests := []struct {
		name string
		tag  uint
	}{
		{"alternative 0 (min range 1)", 0},
		{"alternative 6 (max range 1)", 6},
		{"alternative 7 (min range 2)", 7},
		{"alternative 127 (max range 2)", 127},
		{"alternative 128 (min range 3)", 128},
		{"alternative 256 (range 3)", 256},
		{"alternative 65535 (large range 3)", 65535},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := []any{uint64(42)}

			// Encode with ConstructorEncoder
			ce := cbor.NewConstructorEncoder(tt.tag, fields)
			encoded, err := cbor.Encode(&ce)
			require.NoError(t, err)

			// Decode with ConstructorDecoder
			var cd cbor.ConstructorDecoder
			_, err = cbor.Decode(encoded, &cd)
			require.NoError(t, err)

			assert.Equal(t, tt.tag, cd.Tag())

			var decodedFields []any
			err = cd.DecodeFields(&decodedFields)
			require.NoError(t, err)
			assert.Equal(t, fields, decodedFields)

			// Verify round-trip
			reEncoded, err := cbor.Encode(&cd)
			require.NoError(t, err)
			assert.Equal(t, encoded, reEncoded)
		})
	}
}

func TestConstructorDecoderEmptyFields(t *testing.T) {
	// Constructor with empty fields array
	ce := cbor.NewConstructorEncoder(0, []any{})
	encoded, err := cbor.Encode(&ce)
	require.NoError(t, err)

	var cd cbor.ConstructorDecoder
	_, err = cbor.Decode(encoded, &cd)
	require.NoError(t, err)

	assert.Equal(t, uint(0), cd.Tag())

	var fields []any
	err = cd.DecodeFields(&fields)
	require.NoError(t, err)
	assert.Empty(t, fields)
}

func TestConstructorDecoderTypedFields(t *testing.T) {
	// Demonstrate decoding fields into a typed struct
	type MyFields struct {
		_ struct{} `cbor:",toarray"`
		A uint64
		B string
	}

	fields := []any{uint64(42), "hello"}
	ce := cbor.NewConstructorEncoder(1, fields)
	encoded, err := cbor.Encode(&ce)
	require.NoError(t, err)

	var cd cbor.ConstructorDecoder
	_, err = cbor.Decode(encoded, &cd)
	require.NoError(t, err)

	var typed MyFields
	err = cd.DecodeFields(&typed)
	require.NoError(t, err)
	assert.Equal(t, uint64(42), typed.A)
	assert.Equal(t, "hello", typed.B)
}

func TestIsAlternativeTag(t *testing.T) {
	tests := []struct {
		tagNum   uint64
		expected bool
	}{
		{120, false},
		{121, true},  // min range 1
		{124, true},  // mid range 1
		{127, true},  // max range 1
		{128, false}, // gap
		{101, true},  // alternative 3
		{100, false},
		{1279, false},
		{1280, true},  // min range 2
		{1340, true},  // mid range 2
		{1400, true},  // max range 2
		{1401, false}, // after range 2
		{24, false},   // CBOR tag
		{258, false},  // Set tag
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, tt.expected, cbor.IsAlternativeTag(tt.tagNum))
		})
	}
}

func TestConstructorDecoderInvalidTag(t *testing.T) {
	// Encode a regular tag (not an alternative) and try to decode as constructor
	tmpTag := cbor.Tag{Number: 24, Content: []byte{0x01}}
	encoded, err := cbor.Encode(&tmpTag)
	require.NoError(t, err)

	var cd cbor.ConstructorDecoder
	_, err = cbor.Decode(encoded, &cd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported constructor tag")
}
