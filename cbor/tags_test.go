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
	"math/big"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
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
	/*
		// 121([1, 2, 3])
		{
			cborHex: "d87983010203",
			object: cbor.NewConstructor(
				0,
				[]any{0x1, 0x2, 0x3},
			),
		},
	*/
	// 122(1, 2, 3])
	{
		cborHex: "d87a83010203",
		object: cbor.NewConstructor(
			1,
			[]any{uint64(1), uint64(2), uint64(3)},
		),
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
		// Set stored CBOR for supported types to make comparison easier
		switch v := dest.(type) {
		case cbor.Constructor:
			v.SetFieldsCbor(nil)
			dest = v
		}
		switch v := testDef.object.(type) {
		case cbor.DecodeStoreCborInterface:
			v.SetCbor(cborData)
			testDef.object = v
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
