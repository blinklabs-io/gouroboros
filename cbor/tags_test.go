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
	"encoding/json"
	"math/big"
	"reflect"
	"runtime"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{
			name:        "null numerator",
			cborHex:     "d81e82f601", // 30([null, 1])
			expectError: "unsupported numerator",
		},
		{
			name:        "undefined numerator",
			cborHex:     "d81e82f701", // 30([undefined, 1])
			expectError: "unsupported numerator",
		},
		{
			name:        "tagged numerator",
			cborHex:     "d81e82d8640501", // 30([100(5), 1])
			expectError: "unsupported numerator",
		},
		{
			name:        "rational numerator",
			cborHex:     "d81e82d81e82010202", // 30([30([1, 2]), 2])
			expectError: "unsupported numerator",
		},
		{
			name:        "null denominator",
			cborHex:     "d81e8201f6", // 30([1, null])
			expectError: "unsupported denominator",
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
			if !reflect.DeepEqual(err.Error(), tt.expectError) &&
				!contains(err.Error(), tt.expectError) {
				t.Errorf(
					"expected error containing %q, got %q",
					tt.expectError,
					err.Error(),
				)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > 0 && containsHelper(s, substr))
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

func TestSetTypeWithoutTagRetainsItems(t *testing.T) {
	// [2, 1, 2, 3, 1], with the second 2 encoded non-canonically. Generic
	// SetType consumers decide whether their era and field coalesce duplicates.
	origHex := "85020118020301"
	orig, err := hex.DecodeString(origHex)
	require.NoError(t, err)

	var setType cbor.SetType[uint64]
	_, err = cbor.Decode(orig, &setType)
	require.NoError(t, err)
	assert.Equal(t, []uint64{2, 1, 2, 3, 1}, setType.Items())

	wire, err := cbor.Encode(&setType)
	require.NoError(t, err)
	assert.Equal(t, orig, wire, "decoded SetType must preserve original CBOR")
}

type countingSetItem struct {
	marshalCalls *int
}

func (i countingSetItem) MarshalCBOR() ([]byte, error) {
	(*i.marshalCalls)++
	return []byte{0}, nil
}

func TestSetTypeItemsDoesNotEncodeMembers(t *testing.T) {
	marshalCalls := 0
	setType := cbor.NewSetType(
		[]countingSetItem{
			{marshalCalls: &marshalCalls},
			{marshalCalls: &marshalCalls},
		},
		false,
	)

	assert.Len(t, setType.Items(), 2)
	assert.Zero(t, marshalCalls)
}

func TestSetTypeTaggedItemsRetainDuplicates(t *testing.T) {
	orig, err := hex.DecodeString("d9010283010201")
	require.NoError(t, err)

	var setType cbor.SetType[uint64]
	_, err = cbor.Decode(orig, &setType)
	require.NoError(t, err)
	assert.Equal(t, []uint64{1, 2, 1}, setType.Items())
	assert.ErrorContains(t, setType.CheckForDuplicates(), "duplicate member in set")
}

func TestSetTypeAlwaysRejectsUntaggedDuplicates(t *testing.T) {
	setType := cbor.NewSetType([]uint64{1, 2, 1}, false)
	assert.NoError(t, setType.CheckForDuplicates())
	assert.ErrorContains(t, setType.CheckForDuplicatesAlways(), "duplicate member in set")
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

func TestSetTypeMarshalCBORUsesIndefiniteArrayAbove23(t *testing.T) {
	items := make([]uint64, 24)
	for i := range items {
		items[i] = uint64(i)
	}

	t.Run("tagged set", func(t *testing.T) {
		setType := cbor.NewSetType(items, true)
		encoded, err := cbor.Encode(&setType)
		require.NoError(t, err)
		expected := []byte{0xd9, 0x01, 0x02, 0x9f}
		for _, item := range items {
			expected = append(expected, byte(item))
		}
		expected = append(expected, 0xff)
		assert.Equal(t, expected, encoded)
	})

	t.Run("untagged set", func(t *testing.T) {
		setType := cbor.NewSetType(items, false)
		encoded, err := cbor.Encode(&setType)
		require.NoError(t, err)
		expected := []byte{0x9f}
		for _, item := range items {
			expected = append(expected, byte(item))
		}
		expected = append(expected, 0xff)
		assert.Equal(t, expected, encoded)
	})
}

// TestSetTypeMarshalCBORPreservesOriginalBytes verifies that a SetType
// decoded from CBOR that isn't in canonical form (here, a non-minimally
// encoded uint) round-trips through MarshalCBOR() with its exact original
// bytes rather than silently re-encoding to a different (canonical) form.
// See GitHub issue #1929.
func TestSetTypeMarshalCBORPreservesOriginalBytes(t *testing.T) {
	// [1, 1] where the second element is non-minimally encoded as a
	// 1-byte-follow uint (0x1801) instead of the canonical 0x01.
	origHex := "82011801"
	orig, err := hex.DecodeString(origHex)
	require.NoError(t, err)

	var setType cbor.SetType[uint64]
	_, err = cbor.Decode(orig, &setType)
	require.NoError(t, err)

	wire, err := cbor.Encode(&setType)
	require.NoError(t, err)

	assert.Equal(
		t,
		orig,
		wire,
		"MarshalCBOR() should return the original decoded bytes",
	)
}

// TestRatRejectsOversizedArrayBeforeDecoding checks the shape of the
// rational's array and of its two items before they are decoded, so an array
// of a million items is rejected without building it, whether it is the
// rational itself or one of its items.
func TestRatRejectsOversizedArrayBeforeDecoding(t *testing.T) {
	// Each header declares array(1000000), followed by one-byte items (0).
	// The nested case wraps the array in a second tag (55799) to check the
	// count is read through every leading tag. The indefinite cases declare
	// no count. The numerator cases have the right count of two but put the
	// million items in the numerator, which must be an integer.
	tests := map[string]struct {
		header  []byte
		trailer []byte
		wantErr string
	}{
		"tagged": {
			header:  []byte{0xd8, 0x1e, 0x9a, 0x00, 0x0f, 0x42, 0x40},
			wantErr: "got 1000000 elements",
		},
		"nested tags": {
			header: []byte{
				0xd8, 0x1e, 0xd9, 0xd9, 0xf7, 0x9a, 0x00, 0x0f, 0x42, 0x40,
			},
			wantErr: "got 1000000 elements",
		},
		"indefinite": {
			header:  []byte{0xd8, 0x1e, 0x9f},
			trailer: []byte{0xff},
			wantErr: "got 1000000 elements",
		},
		"array numerator": {
			header:  []byte{0xd8, 0x1e, 0x82, 0x9a, 0x00, 0x0f, 0x42, 0x40},
			trailer: []byte{0x01},
			wantErr: "numerator",
		},
		"indefinite array numerator": {
			header:  []byte{0xd8, 0x1e, 0x82, 0x9f},
			trailer: []byte{0xff, 0x01},
			wantErr: "numerator",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			data := append(tc.header, make([]byte, 1_000_000)...)
			data = append(data, tc.trailer...)
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			var rat cbor.Rat
			_, err := cbor.Decode(data, &rat)
			runtime.ReadMemStats(&after)

			require.ErrorContains(t, err, tc.wantErr)
			// Building the items as []any costs about 16 MB; the payload
			// is 1 MB.
			require.Less(
				t,
				after.TotalAlloc-before.TotalAlloc,
				uint64(4<<20),
			)
		})
	}
}

// TestRatDecodesIndefiniteArray checks that the item count taken from an
// indefinite-length array still accepts a well-formed rational.
func TestRatDecodesIndefiniteArray(t *testing.T) {
	data, err := hex.DecodeString("d81e9f0102ff") // 30([_ 1, 2])
	require.NoError(t, err)
	var rat cbor.Rat
	_, err = cbor.Decode(data, &rat)
	require.NoError(t, err)
	assert.Equal(t, "1/2", rat.String())
}

// TestRatDecodesIntegerItems checks the integer forms each item may take:
// a plain integer, a bignum, and either under a self-describe tag, whatever
// the width of the tag header.
func TestRatDecodesIntegerItems(t *testing.T) {
	tests := map[string]struct {
		cborHex string
		want    string
	}{
		// 30([-1, 256])
		"negative": {"d81e8220190100", "-1/256"},
		// 30([2(h'01'), 2(h'0100')])
		"bignums": {"d81e82c24101c2420100", "1/256"},
		// 30([3(h'00'), 1])
		"negative bignum": {"d81e82c3410001", "-1/1"},
		// 30([55799(1), 2])
		"self-described": {"d81e82d9d9f70102", "1/2"},
		// 30([2(h'05'), 2]) with a two-byte tag header
		"long bignum tag": {"d81e82d802410502", "5/2"},
		// 30([55799(1), 2]) with a five-byte tag header
		"long self-describe tag": {"d81e82da0000d9f70102", "1/2"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			data, err := hex.DecodeString(tc.cborHex)
			require.NoError(t, err)
			var rat cbor.Rat
			_, err = cbor.Decode(data, &rat)
			require.NoError(t, err)
			assert.Equal(t, tc.want, rat.String())
		})
	}
}

func TestSkipTags(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected string
	}{
		"no tag":          {"820102", "820102"},
		"one byte tag":    {"d81e820102", "820102"},
		"nested tags":     {"d81ed9d9f7820102", "820102"},
		"two byte tag":    {"d90102820102", "820102"},
		"four byte tag":   {"da00000102820102", "820102"},
		"eight byte tag":  {"db0000000000000102820102", "820102"},
		"truncated tag":   {"d8", "d8"},
		"reserved header": {"dc820102", "dc820102"},
		"empty":           {"", ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			input, err := hex.DecodeString(tc.input)
			require.NoError(t, err)
			assert.Equal(
				t,
				tc.expected,
				hex.EncodeToString(cbor.SkipTags(input)),
			)
		})
	}
}
