// Copyright 2023 Blink Labs, LLC.
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
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

type decodeTestDefinition struct {
	CborHex   string
	Object    interface{}
	BytesRead int
}

var decodeTests = []decodeTestDefinition{
	// Simple list of numbers
	{
		CborHex: "83010203",
		Object:  []interface{}{uint64(1), uint64(2), uint64(3)},
	},
	// Multiple CBOR objects
	{
		CborHex:   "81018102",
		Object:    []interface{}{uint64(1)},
		BytesRead: 2,
	},
}

func TestDecode(t *testing.T) {
	for _, test := range decodeTests {
		cborData, err := hex.DecodeString(test.CborHex)
		if err != nil {
			t.Fatalf("failed to decode CBOR hex: %s", err)
		}
		var dest interface{}
		bytesRead, err := cbor.Decode(cborData, &dest)
		if err != nil {
			t.Fatalf("failed to decode CBOR: %s", err)
		}
		if test.BytesRead > 0 {
			if bytesRead != test.BytesRead {
				t.Fatalf("expected to read %d bytes, read %d instead", test.BytesRead, bytesRead)
			}
		}
		if !reflect.DeepEqual(dest, test.Object) {
			t.Fatalf("CBOR did not decode to expected object\n  got: %#v\n  wanted: %#v", dest, test.Object)
		}
	}
}

type listLenTestDefinition struct {
	CborHex string
	Length  int
	Error   error
}

var listLenTests = []listLenTestDefinition{
	// [1]
	{
		CborHex: "8101",
		Length:  1,
	},
	// [1, 3]
	{
		CborHex: "820103",
		Length:  2,
	},
	// [4, 5, 6]
	{
		CborHex: "83040506",
		Length:  3,
	},
	// [0, 1, ... 24, 25]
	{
		CborHex: "981A000102030405060708090A0B0C0D0E0F101112131415161718181819",
		Length:  26,
	},
}

func TestListLen(t *testing.T) {
	for _, test := range listLenTests {
		cborData, err := hex.DecodeString(test.CborHex)
		if err != nil {
			t.Fatalf("failed to decode CBOR hex: %s", err)
		}
		listLen, err := cbor.ListLength(cborData)
		if !reflect.DeepEqual(err, test.Error) {
			t.Fatalf("did not find expected error\n  got: %#v\n  wanted:  %#v", err, test.Error)
		}
		if listLen != test.Length {
			t.Fatalf("did not get expected length, got: %d, wanted: %d", listLen, test.Length)
		}
	}
}

type decodeIdFromListTestDefinition struct {
	CborHex string
	Id      int
	Error   error
}

var decodeIdFromListTests = []decodeIdFromListTestDefinition{
	// [1]
	{
		CborHex: "8101",
		Id:      1,
	},
	// [1, 3]
	{
		CborHex: "820103",
		Id:      1,
	},
	// [4, 1]
	{
		CborHex: "820401",
		Id:      4,
	},
	// [25]
	{
		CborHex: "811819",
		Id:      25,
	},
	// [true]
	{
		CborHex: "81F5",
		Error:   fmt.Errorf("first list item was not numeric, found: %v", true),
	},
}

func TestDecodeIdFromList(t *testing.T) {
	for _, test := range decodeIdFromListTests {
		cborData, err := hex.DecodeString(test.CborHex)
		if err != nil {
			t.Fatalf("failed to decode CBOR hex: %s", err)
		}
		id, err := cbor.DecodeIdFromList(cborData)
		if !reflect.DeepEqual(err, test.Error) {
			t.Fatalf("did not find expected error\n  got: %#v\n  wanted:  %#v", err, test.Error)
		}
		if id != test.Id {
			t.Fatalf("did not get expected ID, got: %d, wanted: %d", id, test.Id)
		}
	}
}

type decodeByIdObjectA struct {
	cbor.StructAsArray
	Type uint
}

type decodeByIdObjectB struct {
	cbor.StructAsArray
	Type uint
	Foo  bool
}

type decodeByIdObjectC struct {
	cbor.StructAsArray
	Type uint
	Foo  uint
	Bar  uint
	Baz  uint
}

type decodeByIdTestDefinition struct {
	CborHex string
	Object  interface{}
	Error   error
}

var decodeByIdTests = []decodeByIdTestDefinition{
	// [1]
	{
		CborHex: "8101",
		Object: &decodeByIdObjectA{
			Type: 1,
		},
	},
	// [2, true]
	{
		CborHex: "8202F5",
		Object: &decodeByIdObjectB{
			Type: 2,
			Foo:  true,
		},
	},
	// [3, 1, 2, 3]
	{
		CborHex: "8403010203",
		Object: &decodeByIdObjectC{
			Type: 3,
			Foo:  1,
			Bar:  2,
			Baz:  3,
		},
	},
	// [5]
	{
		CborHex: "8105",
		Error:   fmt.Errorf("found unknown ID: %x", 5),
	},
}

func TestDecodeById(t *testing.T) {
	for _, test := range decodeByIdTests {
		// We define this inside the loop to make sure to get fresh objects each time
		var decodeByIdObjectMap = map[int]interface{}{
			1: &decodeByIdObjectA{},
			2: &decodeByIdObjectB{},
			3: &decodeByIdObjectC{},
		}
		cborData, err := hex.DecodeString(test.CborHex)
		if err != nil {
			t.Fatalf("failed to decode CBOR hex: %s", err)
		}
		obj, err := cbor.DecodeById(cborData, decodeByIdObjectMap)
		if !reflect.DeepEqual(err, test.Error) {
			t.Fatalf("did not find expected error\n  got: %#v\n  wanted:  %#v", err, test.Error)
		}
		if !reflect.DeepEqual(obj, test.Object) {
			t.Fatalf("CBOR did not decode to expected object\n  got: %#v\n  wanted: %#v", obj, test.Object)
		}
	}
}
