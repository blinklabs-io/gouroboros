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
	"fmt"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

type decodeTestDefinition struct {
	CborHex   string
	Object    any
	BytesRead int
}

var decodeTests = []decodeTestDefinition{
	// Simple list of numbers
	{
		CborHex: "83010203",
		Object:  []any{uint64(1), uint64(2), uint64(3)},
	},
	// Multiple CBOR objects
	{
		CborHex:   "81018102",
		Object:    []any{uint64(1)},
		BytesRead: 2,
	},
}

func TestDecode(t *testing.T) {
	for _, test := range decodeTests {
		cborData, err := hex.DecodeString(test.CborHex)
		if err != nil {
			t.Fatalf("failed to decode CBOR hex: %s", err)
		}
		var dest any
		bytesRead, err := cbor.Decode(cborData, &dest)
		if err != nil {
			t.Fatalf("failed to decode CBOR: %s", err)
		}
		if test.BytesRead > 0 {
			if bytesRead != test.BytesRead {
				t.Fatalf(
					"expected to read %d bytes, read %d instead",
					test.BytesRead,
					bytesRead,
				)
			}
		}
		if !reflect.DeepEqual(dest, test.Object) {
			t.Fatalf(
				"CBOR did not decode to expected object\n  got: %#v\n  wanted: %#v",
				dest,
				test.Object,
			)
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
			t.Fatalf(
				"did not find expected error\n  got: %#v\n  wanted:  %#v",
				err,
				test.Error,
			)
		}
		if listLen != test.Length {
			t.Fatalf(
				"did not get expected length, got: %d, wanted: %d",
				listLen,
				test.Length,
			)
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
			t.Fatalf(
				"did not find expected error\n  got: %#v\n  wanted:  %#v",
				err,
				test.Error,
			)
		}
		if id != test.Id {
			t.Fatalf(
				"did not get expected ID, got: %d, wanted: %d",
				id,
				test.Id,
			)
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
	Object  any
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
		var decodeByIdObjectMap = map[int]any{
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
			t.Fatalf(
				"did not find expected error\n  got: %#v\n  wanted:  %#v",
				err,
				test.Error,
			)
		}
		if !reflect.DeepEqual(obj, test.Object) {
			t.Fatalf(
				"CBOR did not decode to expected object\n  got: %#v\n  wanted: %#v",
				obj,
				test.Object,
			)
		}
	}
}

func TestDecodeMaryTransactionCbor(t *testing.T) {
	// Mary transaction CBOR hex from real chain data
	maryTxHex := "83a50081825820377732953cbd7eb824e58291dd08599cfcfe6eedb49f590633610674fc3c33c50001818258390187acac5a3d0b41cd1c5e8c03af5be782f261f21beed70970ddee0873ae34f9d442c7f1a01de9f2dd520791122d6fbf3968c5f8328e9091331a597dbcf1021a0002fd99031a025c094a04828a03581c27b1b4470c84db78ce1ffbfff77bb068abb4e47d43cb6009caaa352358204a7537ce9eeaba1c650261f167827b4e12dd403c4bf13c56b2cba06288f7e9ab1a59682f001a1443fd00d81e82011864581de1ae34f9d442c7f1a01de9f2dd520791122d6fbf3968c5f8328e90913381581cae34f9d442c7f1a01de9f2dd520791122d6fbf3968c5f8328e9091338183011917706e33342e3139382e3234312e323336827468747470733a2f2f6769742e696f2f4a7543786e5820fa77d30bb41e2998233245d269ff5763ecf4371388214943ecef277cae45492783028200581cae34f9d442c7f1a01de9f2dd520791122d6fbf3968c5f8328e909133581c27b1b4470c84db78ce1ffbfff77bb068abb4e47d43cb6009caaa3523a10083825820922a22d07c0ca148105760cb767ece603574ea465d6697c87da8207c8936ebea58405594a100197379c0de715de0b5304e0546e661dae2f36b12173cc150a42215356a5600bf0c02954f02ce3620cfb7f12c23a19328fd00dd1194b4f363675ef407825820727c1891d01cf29ccd1146528221827dcf00a093498509404af77a8b15d77c925840f52e0e1403167212b11fe5d87b7cfdb2f39e5384979ac3625917127ad46763d864a7fcb7147c7b85322ada7ba8fe91c0b5152c74ef4ff0c8132b125e681af50382582073c16f2b67ff85307c4c5935bad1389b9ead473419dbad20f5d5e6436982992b58400572eed773b9a199fd486ebe61b480f05803d107ea97ff649f28b8874d3117f890f80657cbb6eea0d833c21e4e8bc7f1a27cddb9e24fc1ed79b04ddbdcd11d0ff6"

	cborData, err := hex.DecodeString(maryTxHex)
	if err != nil {
		t.Fatalf("failed to decode Mary transaction hex: %s", err)
	}

	var decoded any
	bytesRead, err := cbor.Decode(cborData, &decoded)
	if err != nil {
		t.Fatalf("failed to decode Mary transaction CBOR: %s", err)
	}

	if bytesRead != len(cborData) {
		t.Fatalf(
			"did not read all bytes: read %d, expected %d",
			bytesRead,
			len(cborData),
		)
	}

	// Verify it's an array (transactions are arrays)
	decodedSlice, ok := decoded.([]any)
	if !ok {
		t.Fatal("decoded Mary transaction is not an array")
	}

	if len(decodedSlice) != 3 {
		t.Fatalf(
			"Mary transaction array length mismatch: got %d, expected 3",
			len(decodedSlice),
		)
	}
}

func BenchmarkDecode(b *testing.B) {
	cborData, _ := hex.DecodeString("83010203")
	var dest any
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cbor.Decode(cborData, &dest)
		if err != nil {
			b.Fatal(err)
		}
	}
}
