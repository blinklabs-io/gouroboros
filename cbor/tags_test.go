// Copyright 2024 Blink Labs Software
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
	"encoding/json"
	"math/big"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

var tagsTestDefs = []struct {
	cborHex string
	object  any
}{
	{
		cborHex: "d81843abcdef",
		object:  cbor.WrappedCbor([]byte{0xab, 0xcd, 0xef}),
	},
	{
		cborHex: "d81e82031903e8",
		object: cbor.Rat{
			Rat: big.NewRat(3, 1000),
		},
	},
	{
		cborHex: "d9010283010203",
		object: cbor.Set(
			[]any{
				uint64(1), uint64(2), uint64(3),
			},
		),
	},
	{
		cborHex: "d90103a201020304",
		object: cbor.Map(
			map[any]any{
				uint64(1): uint64(2),
				uint64(3): uint64(4),
			},
		),
	},
	// 30([9223372036854775809, 10000000000000000000])
	{
		cborHex: "d81e821b80000000000000011b8ac7230489e80000",
		object: cbor.Rat{
			Rat: new(big.Rat).SetFrac(
				new(big.Int).SetUint64(9223372036854775809),
				new(big.Int).SetUint64(10000000000000000000),
			),
		},
	},
	// 30([-1, 2])
	{
		cborHex: "d81e822002",
		object: cbor.Rat{
			Rat: big.NewRat(-1, 2),
		},
	},
}

func TestTagsDecode(t *testing.T) {
	for _, testDef := range tagsTestDefs {
		cborData, err := hex.DecodeString(testDef.cborHex)
		if err != nil {
			t.Fatalf("failed to decode CBOR hex: %s", err)
		}
		var dest any
		if _, err := cbor.Decode(cborData, &dest); err != nil {
			t.Fatalf("failed to decode CBOR: %s", err)
		}
		if !reflect.DeepEqual(dest, testDef.object) {
			t.Fatalf(
				"CBOR did not decode to expected object\n  got: %#v\n  wanted: %#v",
				dest,
				testDef.object,
			)
		}
	}
}

func TestTagsEncode(t *testing.T) {
	for _, testDef := range tagsTestDefs {
		cborData, err := cbor.Encode(testDef.object)
		if err != nil {
			t.Fatalf("failed to encode object to CBOR: %s", err)
		}
		cborHex := hex.EncodeToString(cborData)
		if cborHex != testDef.cborHex {
			t.Fatalf(
				"object did not encode to expected CBOR\n  got: %s\n  wanted: %s",
				cborHex,
				testDef.cborHex,
			)
		}
	}
}

func TestRatJsonUnmarshalNumDenom(t *testing.T) {
	jsonData := `{"testRat": { "numerator": 721, "denominator": 10000 }}`
	expectedRat := big.NewRat(721, 10000)
	var testData struct {
		TestRat common.GenesisRat `json:"testRat"`
	}
	if err := json.Unmarshal([]byte(jsonData), &testData); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if testData.TestRat.Cmp(expectedRat) != 0 {
		t.Errorf(
			"did not get expected value: got %s, wanted %s",
			testData.TestRat.String(),
			expectedRat.String(),
		)
	}
}

func TestRatJsonUnmarshalFloat(t *testing.T) {
	jsonData := `{"testRat": 0.0721}`
	expectedRat := big.NewRat(721, 10000)
	var testData struct {
		TestRat common.GenesisRat `json:"testRat"`
	}
	if err := json.Unmarshal([]byte(jsonData), &testData); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if testData.TestRat.Cmp(expectedRat) != 0 {
		t.Errorf(
			"did not get expected value: got %s, wanted %s",
			testData.TestRat.String(),
			expectedRat.String(),
		)
	}
}

func TestRatUnmarshalCBORErrors(t *testing.T) {
	tests := []struct {
		name        string
		cborHex     string
		expectError string
	}{
		{
			name:        "wrong number of elements",
			cborHex:     "d81e8301020304", // 30([1, 2, 3, 4])
			expectError: "expected exactly 2 elements",
		},
		{
			name:        "zero denominator",
			cborHex:     "d81e820100", // 30([1, 0])
			expectError: "denominator cannot be zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := hex.DecodeString(tt.cborHex)
			var rat cbor.Rat
			_, err := cbor.Decode(data, &rat)
			if err == nil {
				t.Fatal("expected error but got none")
			}
			if !reflect.DeepEqual(err.Error(), tt.expectError) && !contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestRatToBigRat(t *testing.T) {
	rat := cbor.Rat{Rat: big.NewRat(3, 4)}
	bigRat := rat.ToBigRat()
	if bigRat.Cmp(big.NewRat(3, 4)) != 0 {
		t.Errorf("expected 3/4, got %s", bigRat.String())
	}
}

func TestWrappedCborBytes(t *testing.T) {
	data := []byte{0xab, 0xcd, 0xef}
	wrapped := cbor.WrappedCbor(data)
	if !reflect.DeepEqual(wrapped.Bytes(), data) {
		t.Errorf("expected %v, got %v", data, wrapped.Bytes())
	}
}

func TestSetTypeItems(t *testing.T) {
	// Test SetType decoding with Set tag
	setWithTagHex := "d9010283010203" // 258([1, 2, 3])
	data, _ := hex.DecodeString(setWithTagHex)

	var setType cbor.SetType[uint64]
	_, err := cbor.Decode(data, &setType)
	if err != nil {
		t.Fatalf("failed to decode SetType: %s", err)
	}

	items := setType.Items()
	expected := []uint64{1, 2, 3}
	if !reflect.DeepEqual(items, expected) {
		t.Errorf("expected %v, got %v", expected, items)
	}

	// Verify stored CBOR
	if setType.Cbor() == nil {
		t.Error("expected CBOR to be stored")
	}
}

func TestSetTypeWithoutTag(t *testing.T) {
	// Test SetType decoding without Set tag (plain array)
	plainArrayHex := "83010203" // [1, 2, 3]
	data, _ := hex.DecodeString(plainArrayHex)

	var setType cbor.SetType[uint64]
	_, err := cbor.Decode(data, &setType)
	if err != nil {
		t.Fatalf("failed to decode SetType without tag: %s", err)
	}

	items := setType.Items()
	expected := []uint64{1, 2, 3}
	if !reflect.DeepEqual(items, expected) {
		t.Errorf("expected %v, got %v", expected, items)
	}
}

func TestSetTypeMarshalCBOR(t *testing.T) {
	// Create SetType with tag
	setType := cbor.NewSetType([]uint64{1, 2, 3}, true)
	encoded, err := cbor.Encode(&setType)
	if err != nil {
		t.Fatalf("failed to encode SetType: %s", err)
	}

	// Should have Set tag
	encodedHex := hex.EncodeToString(encoded)
	if encodedHex[:4] != "d901" { // Tag 258 prefix
		t.Errorf("expected Set tag prefix, got %s", encodedHex[:4])
	}
}

func TestSetTypeMarshalCBORWithoutTag(t *testing.T) {
	// Create SetType without tag
	setType := cbor.NewSetType([]uint64{1, 2, 3}, false)
	encoded, err := cbor.Encode(&setType)
	if err != nil {
		t.Fatalf("failed to encode SetType: %s", err)
	}

	// Should be plain array (no tag)
	if encoded[0] != 0x83 { // array of 3 elements
		t.Errorf("expected plain array, got 0x%02x", encoded[0])
	}
}
