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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// TestRatAcceptsOnlyOneTag30 pins the bounded-ratio decoding that every
// ledger rational (UnitInterval, NonNegativeInterval, PositiveInterval)
// uses: exactly one tag 30 in front of [numerator, denominator], at every
// protocol version. An untagged list, any other tag, or a second tag is
// refused.
func TestRatAcceptsOnlyOneTag30(t *testing.T) {
	for _, h := range []string{"d81e820102", "d81e9f0102ff"} {
		data, _ := hex.DecodeString(h)
		var r cbor.Rat
		if _, err := cbor.Decode(data, &r); err != nil {
			t.Errorf("%s: %v", h, err)
		}
	}
	for _, h := range []string{
		"820102",
		"9f0102ff",
		"d81f820102",
		"c2820102",
		"d81ed81e820102",
		"d90102820102",
	} {
		data, _ := hex.DecodeString(h)
		var r cbor.Rat
		if _, err := cbor.Decode(data, &r); err == nil {
			t.Errorf("%s: accepted as %s", h, r.RatString())
		}
	}
}
