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

// TestConstructorEncoderAsPlutusDataStability encodes two Data values from
// plutus bdc12f105a98 plutus-core/plutus-core/test/CBOR/DataStability.hs
// (SHA-256 4b12f6f7a83d3ec3de141e62b82c110f4d2bacbd92e9a23701ea10f6a48eb7c7)
// and expects that file's second encoding of each, which is what plutus's
// current encoder writes: encodeData (PlutusCore/Data.hs 147-160) writes a
// constructor's fields with the Serialise list instance, an indefinite list
// when non-empty and 0x80 when empty. Plutus List values are written the
// same way, so the test builds them as IndefLengthList.
func TestConstructorEncoderAsPlutusDataStability(t *testing.T) {
	c := cbor.NewConstructorEncoder
	pkh, _ := hex.DecodeString(
		"bd99a373075d42fe4ac9109515e46303d0940cb9620bf058b87986a9",
	)
	inner := func(n uint64) cbor.IndefLengthList {
		return cbor.IndefLengthList{
			c(0, []any{[]byte{}, cbor.IndefLengthList{
				c(0, []any{[]byte{}, n}),
			}}),
		}
	}
	cases := []struct {
		name  string
		value cbor.ConstructorEncoder
		want  string
	}{
		{
			"auction_1-1",
			c(0, []any{pkh, c(0, []any{})}),
			"d8799f581cbd99a373075d42fe4ac9109515e46303d0940cb9620bf058b8798" +
				"6a9d87980ff",
		},
		{
			"future-increase-margin-5 (a)",
			c(0, []any{c(0, []any{inner(2123), inner(2223)})}),
			"d8799fd8799f9fd8799f409fd8799f4019084bffffffff9fd8799f409fd8799f" +
				"401908afffffffffffff",
		},
	}
	for _, tc := range cases {
		got, err := tc.value.MarshalCBOR()
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if hex.EncodeToString(got) != tc.want {
			t.Errorf("%s:\n got %x\nwant %s", tc.name, got, tc.want)
		}
	}
}

// TestConstructorEncoderFieldListsAsPlutus covers each tag range of
// encodeData with empty and non-empty fields: tags 121-127, 1280-1400, and
// tag 102 around a definite two-element list [alternative, fields].
func TestConstructorEncoderFieldListsAsPlutus(t *testing.T) {
	cases := []struct {
		alt    uint
		fields any
		want   string
	}{
		{0, []any{}, "d87980"},
		{0, nil, "d87980"},
		{0, []any{uint64(1)}, "d8799f01ff"},
		{6, []uint64{1, 2}, "d87f9f0102ff"},
		{7, []any{uint64(1)}, "d905009f01ff"},
		{127, []any{}, "d9057880"},
		{128, []any{uint64(1)}, "d8668218809f01ff"},
		{1000, []any{}, "d866821903e880"},
		{0, cbor.IndefLengthList{uint64(1)}, "d8799f01ff"},
	}
	for _, tc := range cases {
		got, err := cbor.NewConstructorEncoder(tc.alt, tc.fields).MarshalCBOR()
		if err != nil {
			t.Errorf("alt %d: %v", tc.alt, err)
			continue
		}
		if hex.EncodeToString(got) != tc.want {
			t.Errorf("alt %d %v: got %x, want %s", tc.alt, tc.fields, got, tc.want)
		}
	}
}
