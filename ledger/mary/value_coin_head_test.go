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

package mary

import (
	"encoding/hex"
	"strings"
	"testing"
)

// TestMaryValueCoinMustBeUnsignedHead covers the coin of a [coin,
// multiasset] output value. Like a bare coin, it is a plain unsigned
// integer: a tag 2 or 3 bignum is refused even when its value fits a
// uint64.
func TestMaryValueCoinMustBeUnsignedHead(t *testing.T) {
	multiAsset := "a1581c" + strings.Repeat("01", 28) + "a1416105"
	for _, tc := range []struct {
		name string
		hex  string
		ok   bool
	}{
		{"unsigned coin", "8205" + multiAsset, true},
		{"unsigned 8-byte coin", "821bffffffffffffffff" + multiAsset, true},
		{"bignum coin", "82c24105" + multiAsset, false},
		{"empty bignum coin", "82c240" + multiAsset, false},
		{"negative bignum coin", "82c34105" + multiAsset, false},
		{"negative coin", "8220" + multiAsset, false},
		{"bare bignum coin", "c24105", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := hex.DecodeString(tc.hex)
			if err != nil {
				t.Fatal(err)
			}
			var v MaryTransactionOutputValue
			err = v.UnmarshalCBOR(data)
			if tc.ok && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("accepted with coin %d", v.Amount)
			}
		})
	}
}
