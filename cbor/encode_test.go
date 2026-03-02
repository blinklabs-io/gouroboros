// Copyright 2023 Blink Labs Software
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

type encodeTestDefinition struct {
	CborHex string
	Object  any
}

var encodeTests = []encodeTestDefinition{
	// Simple list of numbers
	{
		CborHex: "83010203",
		Object:  []any{1, 2, 3},
	},
}

func TestEncode(t *testing.T) {
	for _, test := range encodeTests {
		cborData, err := cbor.Encode(test.Object)
		if err != nil {
			t.Fatalf("failed to encode object to CBOR: %s", err)
		}
		cborHex := hex.EncodeToString(cborData)
		if cborHex != test.CborHex {
			t.Fatalf(
				"object did not encode to expected CBOR\n  got: %s\n  wanted: %s",
				cborHex,
				test.CborHex,
			)
		}
	}
}

func TestEncodeIndefLengthList(t *testing.T) {
	expectedCborHex := "9f1904d219162eff"
	tmpData := cbor.IndefLengthList{
		1234,
		5678,
	}
	cborData, err := cbor.Encode(tmpData)
	if err != nil {
		t.Fatalf("failed to encode object to CBOR: %s", err)
	}
	cborHex := hex.EncodeToString(cborData)
	if cborHex != expectedCborHex {
		t.Fatalf(
			"object did not encode to expected CBOR\n  got %s\n  wanted: %s",
			cborHex,
			expectedCborHex,
		)
	}
}

func TestEncodeIndefLengthByteString(t *testing.T) {
	expectedCborHex := "5f440102030443abcdefff"
	tmpData := cbor.IndefLengthByteString{
		[]byte{1, 2, 3, 4},
		[]byte{0xab, 0xcd, 0xef},
	}
	cborData, err := cbor.Encode(tmpData)
	if err != nil {
		t.Fatalf("failed to encode object to CBOR: %s", err)
	}
	cborHex := hex.EncodeToString(cborData)
	if cborHex != expectedCborHex {
		t.Fatalf(
			"object did not encode to expected CBOR\n  got %s\n  wanted: %s",
			cborHex,
			expectedCborHex,
		)
	}
}

func TestEncodeIndefLengthMap(t *testing.T) {
	expectedCborHex := "bf616101616202ff"
	tmpData := cbor.IndefLengthMap{
		"a": 1,
		"b": 2,
	}
	cborData, err := cbor.Encode(tmpData)
	if err != nil {
		t.Fatalf("failed to encode object to CBOR: %s", err)
	}
	cborHex := hex.EncodeToString(cborData)
	if cborHex != expectedCborHex {
		t.Fatalf(
			"object did not encode to expected CBOR\n  got %s\n  wanted: %s",
			cborHex,
			expectedCborHex,
		)
	}
}

// Test struct for EncodeGeneric
type encodeGenericTestStruct struct {
	Foo uint
	Bar string
}

func TestEncodeGeneric(t *testing.T) {
	t.Run("valid struct pointer", func(t *testing.T) {
		src := &encodeGenericTestStruct{Foo: 5, Bar: "ba"}
		result, err := cbor.EncodeGeneric(src)
		require.NoError(t, err)
		assert.NotEmpty(t, result, "expected non-empty CBOR data")
	})

	t.Run("pointer to non-struct (int)", func(t *testing.T) {
		src := new(int)
		_, err := cbor.EncodeGeneric(src)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "source must be a pointer to a struct")
	})

	t.Run("pointer to non-struct (string)", func(t *testing.T) {
		src := new(string)
		_, err := cbor.EncodeGeneric(src)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "source must be a pointer to a struct")
	})
}

func TestEncodeGenericRoundTrip(t *testing.T) {
	original := &encodeGenericTestStruct{Foo: 42, Bar: "hello"}

	cborData, err := cbor.EncodeGeneric(original)
	require.NoError(t, err, "EncodeGeneric failed")

	decoded := &encodeGenericTestStruct{}
	err = cbor.DecodeGeneric(cborData, decoded)
	require.NoError(t, err, "DecodeGeneric failed")

	assert.Equal(t, original.Foo, decoded.Foo)
	assert.Equal(t, original.Bar, decoded.Bar)
}

func TestEncodeGenericTypeCache(t *testing.T) {
	src := &encodeGenericTestStruct{Foo: 5, Bar: "ba"}

	// Encode twice to exercise type cache
	result1, err := cbor.EncodeGeneric(src)
	require.NoError(t, err, "first encode failed")

	result2, err := cbor.EncodeGeneric(src)
	require.NoError(t, err, "second encode failed")

	assert.Equal(t, hex.EncodeToString(result1), hex.EncodeToString(result2),
		"encoded results should be identical after cache use")
}

func BenchmarkEncode(b *testing.B) {
	obj := []any{1, 2, 3}
	b.ResetTimer()
	for b.Loop() {
		_, err := cbor.Encode(obj)
		if err != nil {
			b.Fatal(err)
		}
	}
}
