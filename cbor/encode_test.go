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
)

type encodeTestDefinition struct {
	CborHex string
	Object  interface{}
}

var encodeTests = []encodeTestDefinition{
	// Simple list of numbers
	{
		CborHex: "83010203",
		Object:  []interface{}{1, 2, 3},
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
