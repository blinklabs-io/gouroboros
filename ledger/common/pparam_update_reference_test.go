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
	"math/big"
	"slices"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// TestClassicCostModelUpdateAsReference checks the pre-version-9 cost-model
// rule. decodeCostModelsFailing
// (libs/cardano-ledger-core/src/Cardano/Ledger/Plutus/CostModels.hs:307-318)
// decodes every key as a known Language and each model with
// decodeCostModelLegacy (lines 320-338), which requires exactly
// costModelInitParamCount values for the language; from version 9 the
// lenient decoder applies and this rule no longer does. The parameter counts
// are the caller's, so small ones stand in for the real ones here.
func TestClassicCostModelUpdateAsReference(t *testing.T) {
	counts := map[uint]int{0: 3, 1: 4}
	ones := func(n int) []int64 { return slices.Repeat([]int64{1}, n) }
	cases := []struct {
		name   string
		models map[uint][]int64
		legacy bool
	}{
		{"none", map[uint][]int64{}, true},
		{"V1 exact", map[uint][]int64{0: ones(3)}, true},
		{"V1 and V2 exact", map[uint][]int64{0: ones(3), 1: ones(4)}, true},
		{"V1 short", map[uint][]int64{0: ones(2)}, false},
		{"V1 long", map[uint][]int64{0: ones(4)}, false},
		{"V2 short", map[uint][]int64{0: ones(3), 1: ones(3)}, false},
		{"unknown language", map[uint][]int64{2: ones(3)}, false},
	}
	for _, tc := range cases {
		for _, pv := range []uint{2, 5, 7, 8, 9, 10} {
			want := tc.legacy || pv >= 9
			err := ValidateClassicCostModelUpdate(tc.models, pv, counts)
			if got := err == nil; got != want {
				t.Errorf("%s/pv%d: accepted = %v, reference = %v (err %v)",
					tc.name, pv, got, want, err)
			}
		}
	}
}

// TestNonNegativeIntervalAsReference checks the decoded-value domain of
// NonNegativeInterval and UnitInterval: boundRational
// (libs/cardano-ledger-core/src/Cardano/Ledger/BaseTypes.hs:350-361) needs
// the reduced components in Word64, and UnitInterval's maxBound is 1.
func TestNonNegativeIntervalAsReference(t *testing.T) {
	two64 := new(big.Int).Lsh(big.NewInt(1), 64)
	cases := []struct {
		name          string
		rat           *big.Rat
		nonNeg, unitI bool
	}{
		{"zero", big.NewRat(0, 1), true, true},
		{"one", big.NewRat(1, 1), true, true},
		{"three halves", big.NewRat(3, 2), true, false},
		{"negative", big.NewRat(-1, 3), false, false},
		{
			"maxBound Word64",
			new(big.Rat).SetInt(new(big.Int).Sub(two64, big.NewInt(1))),
			true, false,
		},
		{"2^64", new(big.Rat).SetInt(two64), false, false},
		{"1 / 2^64", new(big.Rat).SetFrac(big.NewInt(1), two64), false, false},
	}
	for _, tc := range cases {
		for _, unit := range []bool{false, true} {
			want := tc.nonNeg
			if unit {
				want = tc.unitI
			}
			err := ValidateNonNegativeInterval(&cbor.Rat{Rat: tc.rat}, unit)
			if got := err == nil; got != want {
				t.Errorf("%s/unit=%v: accepted = %v, reference = %v",
					tc.name, unit, got, want)
			}
		}
	}
	if ValidateNonNegativeInterval(nil, false) == nil {
		t.Error("nil rational accepted")
	}
}

// TestProtocolParameterUpdateDomainsAsReference checks single-field classic
// parameter updates against the field types of cardano-ledger's
// ShelleyPParams (eras/shelley/impl/src/Cardano/Ledger/Shelley/PParams.hs:
// 106-126: maxBBSize and maxTxSize Word32, maxBHSize Word16, eMax a Word32
// EpochInterval, nOpt Word16, a0 NonNegativeInterval, rho, tau and d
// UnitInterval) and AlonzoPParams (eras/alonzo/impl/src/Cardano/Ledger/
// Alonzo/PParams.hs:413-418: Prices of two NonNegativeIntervals, ExUnits of
// two Naturals, maxValSize Word32, collateralPercentage and
// maxCollateralInputs Word16). The encodings are written by hand.
func TestProtocolParameterUpdateDomainsAsReference(t *testing.T) {
	type field struct {
		key   uint64
		value string
		ok    bool
	}
	widths := []struct {
		key  uint64
		bits int
	}{
		{2, 32}, {3, 32}, {4, 16}, {7, 32}, {8, 16},
		{22, 32}, {23, 16}, {24, 16},
	}
	var cases []field
	for _, w := range widths {
		maxHex := map[int]string{16: "19ffff", 32: "1affffffff"}[w.bits]
		overHex := map[int]string{
			16: "1a00010000", 32: "1b0000000100000000",
		}[w.bits]
		cases = append(cases,
			field{w.key, maxHex, true},
			field{w.key, overHex, false},
			field{w.key, "20", false}, // -1
		)
	}
	cases = append(cases,
		field{9, "d81e820302", true},  // a0 = 3/2
		field{9, "d81e822001", false}, // a0 = -1
		field{9, "820302", false},     // a0 untagged
		field{10, "d81e820101", true}, // rho = 1
		field{10, "d81e820302", false},
		field{11, "d81e820204", true}, // tau = 2/4
		field{11, "d81e820100", false},
		field{12, "d81e820102", true}, // d = 1/2
		field{12, "d81e820302", false},
		field{14, "820900", true}, // version 9.0
		field{14, "8209" + "1bffffffffffffffff", false},
		field{19, "82d81e820102d81e820302", true},
		field{19, "82d81e820102820302", false},
		field{20, "820102", true},
		field{20, "822002", false},
		field{21, "820120", false},
	)
	for _, tc := range cases {
		raw, _ := hex.DecodeString(
			fmt.Sprintf("a1%s%s", uintHex(tc.key), tc.value),
		)
		_, err := ValidateProtocolParameterUpdateDomains(
			raw,
			ProtocolParameterUpdateEraAlonzo,
		)
		if got := err == nil; got != tc.ok {
			t.Errorf("key %d value %s: accepted = %v, reference = %v"+
				" (err %v)", tc.key, tc.value, got, tc.ok, err)
		}
	}
	// Babbage removed d (key 12); its decoder, not this check, refuses it.
	raw, _ := hex.DecodeString("a10cd81e820302")
	if _, err := ValidateProtocolParameterUpdateDomains(
		raw,
		ProtocolParameterUpdateEraBabbage,
	); err != nil {
		t.Errorf("Babbage d checked as a unit interval: %v", err)
	}
	// No classic era defines a key above 24.
	raw, _ = hex.DecodeString("a1181900")
	if _, err := ValidateProtocolParameterUpdateDomains(
		raw,
		ProtocolParameterUpdateEraBabbage,
	); err == nil {
		t.Error("key 25 accepted")
	}
}

func uintHex(v uint64) string {
	if v < 24 {
		return fmt.Sprintf("%02x", v)
	}
	return fmt.Sprintf("18%02x", v)
}
