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
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// multiAssetVerdict is the cardano-ledger 1210d37f verdict on a multiasset
// map at protocol versions 8 (Babbage), 9 (Conway) and 12 (Dijkstra), for an
// output value and for a mint field. It is written by hand from
// decodeMultiAsset (eras/mary/impl/src/Cardano/Ledger/Mary/Value.hs:331-352):
// before 9 duplicates keep the last (decodeMapByKey's Map.fromList,
// cardano-ledger-binary Decoding/Decoder.hs:801-815) and zero quantities are
// pruned; from 9 duplicates, zero quantities and empty asset maps fail; from
// 12 an empty outer map fails too. Output amounts decode with decodeWord64
// (Value.hs:299-308) and mint amounts with decodeIntegerBounded64
// (Value.hs:369-393); both accept only integer major types 0 and 1, so a tag-2
// bignum fails at every version. Asset names hold at most 32 bytes.
type multiAssetVerdict struct {
	name   string
	hex    string
	output [3]bool // pv8, pv9, pv12
	mint   [3]bool
}

var multiAssetVersions = [3]uint{8, 9, 12}

// refHash is n copies of byte b as hex.
func refHash(n int, b byte) string {
	return strings.Repeat(fmt.Sprintf("%02x", b), n)
}

func refAssets(entries ...string) string {
	return fmt.Sprintf("%02x", 0xa0+len(entries)) + strings.Join(entries, "")
}

func refPolicy(b byte, assets string) string {
	return "581c" + refHash(28, b) + assets
}

const (
	refAssetA = "4161" // h'61'
	refAssetB = "4162"
)

var (
	allOk   = [3]bool{true, true, true}
	allFail = [3]bool{false, false, false}
	pre9Ok  = [3]bool{true, false, false}
	pre12Ok = [3]bool{true, true, false}
)

var multiAssetVerdicts = []multiAssetVerdict{
	{
		"one asset", refAssets(refPolicy(1, refAssets(refAssetA+"05"))),
		allOk, allOk,
	},
	{
		"zero quantity",
		refAssets(refPolicy(1, refAssets(refAssetA+"00"))),
		pre9Ok, pre9Ok,
	},
	{
		"zero beside a nonzero quantity",
		refAssets(refPolicy(1, refAssets(refAssetA+"00", refAssetB+"01"))),
		pre9Ok, pre9Ok,
	},
	{
		"empty asset map", refAssets(refPolicy(1, "a0")),
		pre9Ok, pre9Ok,
	},
	{"empty multiasset", "a0", pre12Ok, pre12Ok},
	{
		"duplicate asset name",
		refAssets(refPolicy(1, refAssets(refAssetA+"05", refAssetA+"06"))),
		pre9Ok, pre9Ok,
	},
	{
		"duplicate policy",
		refAssets(
			refPolicy(1, refAssets(refAssetA+"05")),
			refPolicy(1, refAssets(refAssetB+"06")),
		),
		pre9Ok, pre9Ok,
	},
	{
		"negative quantity",
		refAssets(refPolicy(1, refAssets(refAssetA+"20"))),
		allFail, allOk,
	},
	{
		"maxBound Word64",
		refAssets(refPolicy(1, refAssets(refAssetA+"1bffffffffffffffff"))),
		allOk, allFail,
	},
	{
		"maxBound Int64 + 1",
		refAssets(refPolicy(1, refAssets(refAssetA+"1b8000000000000000"))),
		allOk, allFail,
	},
	{
		"minBound Int64",
		refAssets(refPolicy(1, refAssets(refAssetA+"3b7fffffffffffffff"))),
		allFail, allOk,
	},
	{
		"minBound Int64 - 1",
		refAssets(refPolicy(1, refAssets(refAssetA+"3b8000000000000000"))),
		allFail, allFail,
	},
	{
		"32-byte asset name",
		refAssets(refPolicy(1, refAssets("5820"+refHash(32, 0x61)+"05"))),
		allOk, allOk,
	},
	{
		"33-byte asset name",
		refAssets(refPolicy(1, refAssets("5821"+refHash(33, 0x61)+"05"))),
		allFail, allFail,
	},
}

// referenceBignumVerdicts are the tag-2 and tag-3 bignum quantities, in
// every tag-head width and behind a duplicate asset name. The reference
// refuses each at every version, since it never reads a quantity as a tag.
var referenceBignumVerdicts = []multiAssetVerdict{
	{
		"bignum quantity five",
		refAssets(refPolicy(1, refAssets(refAssetA+"c24105"))),
		allFail, allFail,
	},
	{
		// A tag head need not be minimal (d8 02 is tag 2).
		"bignum quantity five with a two-byte tag head",
		refAssets(refPolicy(1, refAssets(refAssetA+"d8024105"))),
		allFail, allFail,
	},
	{
		"negative bignum with a two-byte tag head",
		refAssets(refPolicy(1, refAssets(refAssetA+"d8034104"))),
		allFail, allFail,
	},
	{
		// Before 9 the later duplicate wins, but the reference still
		// decodes the earlier entry's quantity and refuses the bignum.
		"bignum behind a duplicate asset name",
		refAssets(refPolicy(1, refAssets(refAssetA+"c24105", refAssetA+"05"))),
		allFail, allFail,
	},
}

// decodeMultiAssetAsEra applies the checks each era's decoder applies to a
// decoded multiasset: quantity domains everywhere, duplicate keys, zero
// quantities and empty asset maps from Conway, and an empty map from
// Dijkstra.
func decodeMultiAssetAsEra(raw []byte, pv uint, mint bool) error {
	var ma MultiAsset[MultiAssetTypeOutput]
	if _, err := cbor.Decode(raw, &ma); err != nil {
		return err
	}
	if pv >= 9 {
		if err := ma.CheckForDuplicateKeys(); err != nil {
			return err
		}
		if err := ma.CheckForZeroAssets(); err != nil {
			return err
		}
	}
	if pv >= 12 {
		if err := ma.CheckForEmptyMultiAsset(); err != nil {
			return err
		}
	}
	if mint {
		return ma.ValidateMintQuantities()
	}
	return ma.ValidateOutputQuantities()
}

func runMultiAssetVerdicts(t *testing.T, cases []multiAssetVerdict) {
	for _, tc := range cases {
		raw, err := hex.DecodeString(tc.hex)
		if err != nil {
			t.Fatalf("%s: bad hex: %v", tc.name, err)
		}
		for i, pv := range multiAssetVersions {
			for _, mint := range []bool{false, true} {
				want := tc.output[i]
				field := "output"
				if mint {
					want = tc.mint[i]
					field = "mint"
				}
				err := decodeMultiAssetAsEra(raw, pv, mint)
				if got := err == nil; got != want {
					t.Errorf("%s/%s/pv%d: accepted = %v, reference = %v"+
						" (err %v)", tc.name, field, pv, got, want, err)
				}
			}
		}
	}
}

func TestMultiAssetWellFormednessAsReference(t *testing.T) {
	runMultiAssetVerdicts(t, multiAssetVerdicts)
}

func TestMultiAssetBignumQuantityAsReference(t *testing.T) {
	runMultiAssetVerdicts(t, referenceBignumVerdicts)
}
