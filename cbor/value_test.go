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
	"encoding/json"
	"fmt"
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
		expectedDecodeError: fmt.Errorf("EOF"),
	},
	// Invalid map key type
	{
		cborHex:             "A1810000",
		expectedDecodeError: fmt.Errorf("decode failure, probably due to type unsupported by Go: runtime error: hash of unhashable type []interface {}"),
	},
	// [1, 2, 3]
	{
		cborHex:         "83010203",
		expectedObject:  []any{uint64(1), uint64(2), uint64(3)},
		expectedAstJson: `{"list":[{"int":1},{"int":2},{"int":3}]}`,
	},
	// {1: 2, 3: 4}
	{
		cborHex:         "A201020304",
		expectedObject:  map[any]any{uint64(1): uint64(2), uint64(3): uint64(4)},
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
					t.Fatalf("did not receive expected decode error, got: %s, wanted: %s", err, testDef.expectedDecodeError)
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
			t.Fatalf("CBOR did not decode to expected object\n  got: %#v\n  wanted: %#v", newObj, testDef.expectedObject)
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
			t.Fatalf("CBOR did not marshal to expected JSON\n  got: %s\n  wanted: %s", jsonData, fullExpectedJson)
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
					t.Fatalf("did not receive expected decode error, got: %s, wanted: %s", err, testDef.expectedDecodeError)
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
					t.Fatalf("did not receive expected decode error, got: %s, wanted: %s", err, testDef.expectedDecodeError)
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
			t.Fatalf("CBOR did not decode to expected object\n  got: %#v\n  wanted: %#v", newObj, testDef.expectedObject)
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
		if !test.JsonStringsEqual(jsonData, []byte(fullExpectedJson)) {
			t.Fatalf("CBOR did not marshal to expected JSON\n  got: %s\n  wanted: %s", jsonData, fullExpectedJson)
		}
	}
}
