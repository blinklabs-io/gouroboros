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
	"fmt"
	"io"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/test"
)

var testDefs = []struct {
	cborHex             string
	expectedObject      interface{}
	expectedAstJson     string
	expectedDecodeError error
}{
	// []
	{
		cborHex:         "80",
		expectedObject:  []any{},
		expectedAstJson: `{"list":[]}`,
	},
	// Invalid CBOR
	{
		cborHex:             "81",
		expectedObject:      nil,
		expectedDecodeError: io.ErrUnexpectedEOF,
	},
	// Invalid map key type
	{
		cborHex: "A1810000",
		expectedDecodeError: fmt.Errorf(
			"decode failure, probably due to type unsupported by Go: runtime error: hash of unhashable type []interface {}",
		),
	},
	// [1, 2, 3]
	{
		cborHex:         "83010203",
		expectedObject:  []any{uint64(1), uint64(2), uint64(3)},
		expectedAstJson: `{"list":[{"int":1},{"int":2},{"int":3}]}`,
	},
	// {1: 2, 3: 4}
	{
		cborHex: "A201020304",
		expectedObject: map[any]any{
			uint64(1): uint64(2),
			uint64(3): uint64(4),
		},
		expectedAstJson: `{"map":[{"k":{"int":1},"v":{"int":2}},{"k":{"int":3},"v":{"int":4}}]}`,
	},
	// {1: [2], 3: [4]}
	{
		cborHex: "A2018102038104",
		expectedObject: map[any]any{
			uint64(1): []any{uint64(2)},
			uint64(3): []any{uint64(4)},
		},
		expectedAstJson: `{"map":[{"k":{"int":1},"v":{"list":[{"int":2}]}},{"k":{"int":3},"v":{"list":[{"int":4}]}}]}`,
	},
	// [22318265904693663008365, 8535038193994223137511702528]
	{
		cborHex: "82C24A04B9E028911409DC866DC24C1B9404A39BD8000000000000",
		expectedObject: []any{
			*(new(big.Int).SetBytes(test.DecodeHexString("04B9E028911409DC866D"))),
			*(new(big.Int).SetBytes(test.DecodeHexString("1B9404A39BD8000000000000"))),
		},
		expectedAstJson: `{"list":[{"int":22318265904693663008365},{"int":8535038193994223137511702528}]}`,
	},
	// 24('abcdef')
	{
		cborHex:         "d81843abcdef",
		expectedObject:  cbor.WrappedCbor([]byte{0xab, 0xcd, 0xef}),
		expectedAstJson: `{"bytes":"abcdef"}`,
	},
	// 30([3, 1000])
	{
		cborHex: "d81e82031903e8",
		expectedObject: cbor.Rat{
			Rat: big.NewRat(3, 1000),
		},
		expectedAstJson: `{"list":[{"int":3},{"int":1000}]}`,
	},
	// 258([1, 2, 3])
	{
		cborHex: "d9010283010203",
		expectedObject: cbor.Set(
			[]any{
				uint64(1), uint64(2), uint64(3),
			},
		),
		expectedAstJson: `{"list":[{"int":1},{"int":2},{"int":3}]}`,
	},
	// 259({1: 2, 3: 4})
	{
		cborHex: "d90103a201020304",
		expectedObject: cbor.Map(
			map[any]any{
				uint64(1): uint64(2),
				uint64(3): uint64(4),
			},
		),
		expectedAstJson: `{"map":[{"k":{"int":1},"v":{"int":2}},{"k":{"int":3},"v":{"int":4}}]}`,
	},
}

func TestValueDecode(t *testing.T) {
	for _, testDef := range testDefs {
		cborData, err := hex.DecodeString(testDef.cborHex)
		if err != nil {
			t.Fatalf("failed to decode CBOR hex: %s", err)
		}
		var tmpValue cbor.Value
		if _, err := cbor.Decode(cborData, &tmpValue); err != nil {
			if testDef.expectedDecodeError != nil {
				if err.Error() != testDef.expectedDecodeError.Error() {
					t.Fatalf(
						"did not receive expected decode error, got:    %s, wanted: %s",
						err,
						testDef.expectedDecodeError,
					)
				}
				continue
			} else {
				t.Fatalf("failed to decode CBOR data: %s", err)
			}
		} else {
			if testDef.expectedDecodeError != nil {
				t.Fatalf("did not receive expected decode error: %s", testDef.expectedDecodeError)
			}
		}
		newObj := tmpValue.Value()
		if !reflect.DeepEqual(newObj, testDef.expectedObject) {
			t.Fatalf(
				"CBOR did not decode to expected object\n  got:    %#v\n  wanted: %#v",
				newObj,
				testDef.expectedObject,
			)
		}
	}
}

func TestValueMarshalJSON(t *testing.T) {
	for _, testDef := range testDefs {
		// Skip test if the CBOR decode is expected to fail
		if testDef.expectedDecodeError != nil {
			continue
		}
		cborData, err := hex.DecodeString(testDef.cborHex)
		if err != nil {
			t.Fatalf("failed to decode CBOR hex: %s", err)
		}
		var tmpValue cbor.Value
		if _, err := cbor.Decode(cborData, &tmpValue); err != nil {
			t.Fatalf("failed to decode CBOR data: %s", err)
		}
		jsonData, err := json.Marshal(&tmpValue)
		if err != nil {
			t.Fatalf("failed to marshal Value as JSON: %s", err)
		}
		// We create the wrapper JSON here, since it would otherwise result in the CBOR hex being duplicated in each test definition
		fullExpectedJson := fmt.Sprintf(
			`{"cbor":"%s","json":%s}`,
			strings.ToLower(testDef.cborHex),
			testDef.expectedAstJson,
		)
		if testDef.expectedObject == nil {
			fullExpectedJson = fmt.Sprintf(
				`{"cbor":"%s"}`,
				strings.ToLower(testDef.cborHex),
			)
		}
		if !test.JsonStringsEqual(jsonData, []byte(fullExpectedJson)) {
			t.Fatalf(
				"CBOR did not marshal to expected JSON\n  got:    %s\n  wanted: %s",
				jsonData,
				fullExpectedJson,
			)
		}
	}
}

func TestLazyValueDecode(t *testing.T) {
	for _, testDef := range testDefs {
		cborData, err := hex.DecodeString(testDef.cborHex)
		if err != nil {
			t.Fatalf("failed to decode CBOR hex: %s", err)
		}
		var tmpValue cbor.LazyValue
		if _, err := cbor.Decode(cborData, &tmpValue); err != nil {
			if testDef.expectedDecodeError != nil {
				if err.Error() != testDef.expectedDecodeError.Error() {
					t.Fatalf(
						"did not receive expected decode error, got:    %s, wanted: %s",
						err,
						testDef.expectedDecodeError,
					)
				}
				continue
			} else {
				t.Fatalf("failed to decode CBOR data: %s", err)
			}
		}
		newObj, err := tmpValue.Decode()
		if err != nil {
			if testDef.expectedDecodeError != nil {
				if err.Error() != testDef.expectedDecodeError.Error() {
					t.Fatalf(
						"did not receive expected decode error, got:    %s, wanted: %s",
						err,
						testDef.expectedDecodeError,
					)
				}
				continue
			} else {
				t.Fatalf("failed to decode CBOR data: %s", err)
			}
		} else {
			if testDef.expectedDecodeError != nil {
				t.Fatalf("did not receive expected decode error: %s", testDef.expectedDecodeError)
			}
		}
		if !reflect.DeepEqual(newObj, testDef.expectedObject) {
			t.Fatalf(
				"CBOR did not decode to expected object\n  got:    %#v\n  wanted: %#v",
				newObj,
				testDef.expectedObject,
			)
		}
	}
}

func TestLazyValueMarshalJSON(t *testing.T) {
	for _, testDef := range testDefs {
		// Skip test if the CBOR decode is expected to fail
		if testDef.expectedDecodeError != nil {
			continue
		}
		cborData, err := hex.DecodeString(testDef.cborHex)
		if err != nil {
			t.Fatalf("failed to decode CBOR hex: %s", err)
		}
		var tmpValue cbor.LazyValue
		if _, err := cbor.Decode(cborData, &tmpValue); err != nil {
			t.Fatalf("failed to decode CBOR data: %s", err)
		}
		jsonData, err := json.Marshal(&tmpValue)
		if err != nil {
			t.Fatalf("failed to marshal Value as JSON: %s", err)
		}
		// We create the wrapper JSON here, since it would otherwise result in the CBOR hex being duplicated in each test definition
		fullExpectedJson := fmt.Sprintf(
			`{"cbor":"%s","json":%s}`,
			strings.ToLower(testDef.cborHex),
			testDef.expectedAstJson,
		)
		if testDef.expectedObject == nil {
			fullExpectedJson = fmt.Sprintf(
				`{"cbor":"%s"}`,
				strings.ToLower(testDef.cborHex),
			)
		}
		if !test.JsonStringsEqual(jsonData, []byte(fullExpectedJson)) {
			t.Fatalf(
				"CBOR did not marshal to expected JSON\n  got:    %s\n  wanted: %s",
				jsonData,
				fullExpectedJson,
			)
		}
	}
}

var constructorTestDefs = []struct {
	cborHex     string
	expectedObj cbor.Constructor
}{
	{
		// 122([1, 2, 3])
		cborHex: "D87A83010203",
		expectedObj: cbor.NewConstructor(
			1,
			[]any{uint64(1), uint64(2), uint64(3)},
		),
	},
	{
		// 1288([3, 4, 5])
		cborHex: "D9050883030405",
		expectedObj: cbor.NewConstructor(
			15,
			[]any{uint64(3), uint64(4), uint64(5)},
		),
	},
	{
		// 101([999, [6, 7]])
		cborHex: "D865821903E7820607",
		expectedObj: cbor.NewConstructor(
			999,
			[]any{uint64(6), uint64(7)},
		),
	},
}

func TestConstructorDecode(t *testing.T) {
	for _, testDef := range constructorTestDefs {
		cborData, err := hex.DecodeString(testDef.cborHex)
		if err != nil {
			t.Fatalf("failed to decode CBOR hex: %s", err)
		}
		var tmpConstr cbor.Constructor
		if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
			t.Fatalf("failed to decode CBOR data: %s", err)
		}
		if tmpConstr.Constructor() != testDef.expectedObj.Constructor() {
			t.Fatalf(
				"did not get expected constructor number, got %d, wanted %d",
				tmpConstr.Constructor(),
				testDef.expectedObj.Constructor(),
			)
		}
		if !reflect.DeepEqual(tmpConstr.Fields(), testDef.expectedObj.Fields()) {
			t.Fatalf(
				"did not decode to expected fields\n  got:    %#v\n  wanted: %#v",
				tmpConstr.Fields(),
				testDef.expectedObj.Fields(),
			)
		}
	}
}

func TestConstructorEncode(t *testing.T) {
	for _, testDef := range constructorTestDefs {
		cborData, err := cbor.Encode(&testDef.expectedObj)
		if err != nil {
			t.Fatalf("failed to encode object to CBOR: %s", err)
		}
		cborDataHex := hex.EncodeToString(cborData)
		if cborDataHex != strings.ToLower(testDef.cborHex) {
			t.Fatalf(
				"did not encode to expected CBOR\n  got:    %s\n  wanted: %s",
				cborDataHex,
				strings.ToLower(testDef.cborHex),
			)
		}
	}
}
