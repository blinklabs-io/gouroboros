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

package ledger

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/test"
)

func createMaryTransactionOutputValueAssets(policyId []byte, assetName []byte, amount uint64) map[Blake2b224]map[cbor.ByteString]uint64 {
	ret := map[Blake2b224]map[cbor.ByteString]uint64{}
	policyIdKey := Blake2b224{}
	copy(policyIdKey[:], policyId)
	assetKey := cbor.ByteString(assetName)
	ret[policyIdKey] = map[cbor.ByteString]uint64{
		assetKey: amount,
	}
	return ret
}

func TestMaryTransactionOutputValueEncodeDecode(t *testing.T) {
	var tests = []struct {
		CborHex string
		Object  interface{}
	}{
		{
			CborHex: "1a02d71996",
			Object:  MaryTransactionOutputValue{Amount: 47651222},
		},
		{
			CborHex: "1b0000000129d2de56",
			Object:  MaryTransactionOutputValue{Amount: 4996652630},
		},
		{
			CborHex: "821a003d0900a1581c00000002df633853f6a47465c9496721d2d5b1291b8398016c0e87aea1476e7574636f696e01",
			// [4000000, {h'00000002DF633853F6A47465C9496721D2D5B1291B8398016C0E87AE': {h'6E7574636F696E': 1}}]
			Object: MaryTransactionOutputValue{
				Amount: 4000000,
				Assets: createMaryTransactionOutputValueAssets(
					test.DecodeHexString("00000002DF633853F6A47465C9496721D2D5B1291B8398016C0E87AE"),
					test.DecodeHexString("6E7574636F696E"),
					1,
				),
			},
		},
		{
			CborHex: "821a004986e3a1581c3a9241cd79895e3a8d65261b40077d4437ce71e9d7c8c6c00e3f658ea1494669727374636f696e01",
			// [4818659, {h'3A9241CD79895E3A8D65261B40077D4437CE71E9D7C8C6C00E3F658E': {h'4669727374636F696E': 1}}]
			Object: MaryTransactionOutputValue{
				Amount: 4818659,
				Assets: createMaryTransactionOutputValueAssets(
					test.DecodeHexString("3A9241CD79895E3A8D65261B40077D4437CE71E9D7C8C6C00E3F658E"),
					test.DecodeHexString("4669727374636F696E"),
					1,
				),
			},
		},
	}
	for _, test := range tests {
		// Test decode
		cborData, err := hex.DecodeString(test.CborHex)
		if err != nil {
			t.Fatalf("failed to decode CBOR hex: %s", err)
		}
		tmpObj := MaryTransactionOutputValue{}
		_, err = cbor.Decode(cborData, &tmpObj)
		if err != nil {
			t.Fatalf("failed to decode CBOR: %s", err)
		}
		if !reflect.DeepEqual(tmpObj, test.Object) {
			t.Fatalf("CBOR did not decode to expected object\n  got:    %#v\n  wanted: %#v", tmpObj, test.Object)
		}
		// Test encode
		cborData, err = cbor.Encode(test.Object)
		if err != nil {
			t.Fatalf("failed to encode object to CBOR: %s", err)
		}
		cborHex := hex.EncodeToString(cborData)
		if cborHex != test.CborHex {
			t.Fatalf("object did not encode to expected CBOR\n  got:    %s\n  wanted: %s", cborHex, test.CborHex)
		}
	}
}
