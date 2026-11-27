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

package common

import (
	"encoding/hex"
	"fmt"
	"testing"
)

// boundedRatioDecodes reports whether cardano-ledger 1210d37f decodes a
// NonNegativeInterval from the given numerator and denominator, as read from
// libs/cardano-ledger-core/src/Cardano/Ledger/BaseTypes.hs:
//
//   - Before decoder version 12 (DecCBOR at lines 419-428 falls back to the
//     FromCBOR instance at lines 389-401): Plain.decodeRationalWithTag
//     (cardano-ledger-binary Plain.hs:159-167) requires tag 30, accepts a
//     definite or indefinite list of exactly two arbitrary integers and a
//     nonzero denominator, and builds n % d, which normalises sign and common
//     factors. boundRational (fromRationalBoundedRatio, lines 350-361) then
//     requires the normalised numerator and denominator to lie in
//     [0, maxBound Word64].
//   - From version 12: decodeIntegralRational @Word64 (DecCBOR.hs:271-280)
//     requires tag 30 and two Word64 components, then the same boundRational.
//
// The cases below state both verdicts by hand from those rules; none comes
// from gOuroboros.
type boundedRatioCase struct {
	name string
	hex  string
	// plain is the verdict before version 12, word64 from version 12.
	plain, word64 bool
}

// twoTo64 is 2^64 as a tag-2 bignum: nine content bytes, 0x01 then eight
// zero bytes.
const twoTo64 = "c249" + "01" + "0000000000000000"

var boundedRatioCases = []boundedRatioCase{
	{"one half", "d81e820102", true, true},
	{"zero over five", "d81e820005", true, true},
	{"three halves (no upper bound below Word64)", "d81e820302", true, true},
	{"unreduced two quarters", "d81e820204", true, true},
	{"indefinite list", "d81e9f0102ff", true, true},
	{"non-shortest numerator head", "d81e82180102", true, true},
	{
		"maxBound Word64 numerator",
		"d81e821bffffffffffffffff01",
		true, true,
	},
	{"no tag 30", "820102", false, false},
	{"wrong tag", "d81f820102", false, false},
	{"three components", "d81e83010203", false, false},
	{"one component", "d81e8101", false, false},
	{"zero denominator", "d81e820100", false, false},
	{"negative numerator", "d81e822001", false, false},
	// (-1) % (-2) normalises to 1 % 2, so the plain decoder accepts it;
	// Word64 components reject the negative heads.
	{"both negative", "d81e822021", true, false},
	// 2^64 / 2 normalises to 2^63 / 1, inside Word64.
	{
		"bignum numerator reducing into Word64",
		"d81e82" + twoTo64 + "02",
		true, false,
	},
	// 1 / 2^64 is already reduced and its denominator exceeds Word64.
	{
		"bignum denominator outside Word64",
		"d81e8201" + twoTo64,
		false, false,
	},
	{"text numerator", "d81e82613101", false, false},
}

// boundedRatioVersions crosses the only gate in the decoder (12) and the
// version boundaries of the surrounding decoders (2 and 9), which must not
// change the verdict.
var boundedRatioVersions = []uint{1, 2, 8, 9, 11, 12, 13}

func TestNonNegativeBoundedRatCBORForVersionAsReference(t *testing.T) {
	for _, tc := range boundedRatioCases {
		raw, err := hex.DecodeString(tc.hex)
		if err != nil {
			t.Fatalf("%s: bad hex: %v", tc.name, err)
		}
		for _, version := range boundedRatioVersions {
			want := tc.plain
			if version >= 12 {
				want = tc.word64
			}
			t.Run(fmt.Sprintf("%s/pv%d", tc.name, version), func(t *testing.T) {
				err := ValidateNonNegativeBoundedRatCBORForVersion(raw, version)
				if got := err == nil; got != want {
					t.Fatalf("accepted = %v, reference = %v (err %v)",
						got, want, err)
				}
			})
		}
	}
}

// TestNonNegativeBoundedRatArrayCBORForVersionAsReference checks that a
// record of rationals, such as the DRep voting thresholds, is accepted only
// when every element is (each field is decoded by the BoundedRatio decoder
// above), at each side of the version-12 gate.
func TestNonNegativeBoundedRatArrayCBORForVersionAsReference(t *testing.T) {
	half := "d81e820102"
	bigNum := "d81e82" + twoTo64 + "02"
	cases := []struct {
		name          string
		hex           string
		plain, word64 bool
	}{
		{"two halves", "82" + half + half, true, true},
		{"indefinite record", "9f" + half + half + "ff", true, true},
		{"bignum second element", "82" + half + bigNum, true, false},
		{"untagged second element", "82" + half + "820102", false, false},
		{"not an array", half, false, false},
	}
	for _, tc := range cases {
		raw, err := hex.DecodeString(tc.hex)
		if err != nil {
			t.Fatalf("%s: bad hex: %v", tc.name, err)
		}
		for _, version := range []uint{8, 9, 11, 12} {
			want := tc.plain
			if version >= 12 {
				want = tc.word64
			}
			err := ValidateNonNegativeBoundedRatArrayCBORForVersion(
				raw,
				version,
			)
			if got := err == nil; got != want {
				t.Errorf("%s/pv%d: accepted = %v, reference = %v (err %v)",
					tc.name, version, got, want, err)
			}
		}
	}
}

// TestCostModelLanguageIDsAsReference checks the Word8 domain of cost-model
// keys. From decoder version 9 cardano-ledger decodes CostModels as a
// Map Word8 [Int64] and keeps unknown languages (decodeCostModelsLenient,
// libs/cardano-ledger-core/src/Cardano/Ledger/Plutus/CostModels.hs:303-318,
// costModelsUnknown at 381), so any key that fits Word8 is retained and 256
// does not decode.
func TestCostModelLanguageIDsAsReference(t *testing.T) {
	for _, id := range []uint{0, 1, 2, 3, 4, 200, 255} {
		if err := ValidateCostModelLanguageIDs(
			map[uint][]int64{id: {1}},
		); err != nil {
			t.Errorf("language %d: rejected: %v", id, err)
		}
	}
	for _, id := range []uint{256, 1 << 16} {
		if err := ValidateCostModelLanguageIDs(
			map[uint][]int64{0: {1}, id: {1}},
		); err == nil {
			t.Errorf("language %d: accepted outside Word8", id)
		}
	}
}
