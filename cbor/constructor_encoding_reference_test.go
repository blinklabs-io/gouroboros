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
	"math/big"
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

// TestConstructorEncoderItemsAsPlutus covers the field values encodeData
// writes differently from a plain CBOR encoding: every Data list, whether a
// Go slice, a Go array or an IndefLengthList, at any depth, is an indefinite
// list when non-empty and 0x80 when empty; a byte string longer than 64
// bytes is an indefinite byte string of 64-byte chunks (encodeBs, Data.hs
// 185-198), and so is the magnitude of an integer outside the 64-bit range.
func TestConstructorEncoderItemsAsPlutus(t *testing.T) {
	bytes65 := make([]byte, 65)
	for i := range bytes65 {
		bytes65[i] = byte(i)
	}
	hex65 := hex.EncodeToString(bytes65)
	chunked65 := "5f5840" + hex65[:128] + "41" + hex65[128:] + "ff"
	bytes129 := make([]byte, 129)
	hex129 := hex.EncodeToString(bytes129)
	chunked129 := "5f5840" + hex129[:128] + "5840" + hex129[128:256] +
		"41" + hex129[256:] + "ff"
	big65 := new(big.Int).SetBytes(append([]byte{1}, make([]byte, 64)...))
	bigMag := hex.EncodeToString(big65.Bytes())
	negBig65 := new(big.Int).Sub(new(big.Int).Neg(big65), big.NewInt(1))
	cases := []struct {
		name   string
		fields any
		want   string
	}{
		{"go array", [2]uint64{1, 2}, "d8799f0102ff"},
		{"empty go array", [0]uint64{}, "d87980"},
		{"empty IndefLengthList", cbor.IndefLengthList{}, "d87980"},
		{"nested slice", []any{[]any{uint64(1)}}, "d8799f9f01ffff"},
		{"nested empty slice", []any{[]any{}}, "d8799f80ff"},
		{"nested go array", []any{[1]uint64{1}}, "d8799f9f01ffff"},
		{"nested empty IndefLengthList", []any{cbor.IndefLengthList{}}, "d8799f80ff"},
		{"64-byte field", []any{bytes65[:64]}, "d8799f5840" + hex65[:128] + "ff"},
		{"65-byte field", []any{bytes65}, "d8799f" + chunked65 + "ff"},
		{"129-byte field", []any{bytes129}, "d8799f" + chunked129 + "ff"},
		{"65-byte ByteString", []any{cbor.NewByteString(bytes65)}, "d8799f" + chunked65 + "ff"},
		{"65-byte field in a nested list", []any{[]any{bytes65}}, "d8799f9f" + chunked65 + "ffff"},
		{"bignum over 64 bytes", []any{big65}, "d8799fc25f5840" + bigMag[:128] + "4100" + "ff" + "ff"},
		{"negative bignum over 64 bytes", []any{negBig65}, "d8799fc35f5840" + bigMag[:128] + "4100" + "ff" + "ff"},
	}
	for _, tc := range cases {
		got, err := cbor.NewConstructorEncoder(0, tc.fields).MarshalCBOR()
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if hex.EncodeToString(got) != tc.want {
			t.Errorf("%s:\n got %x\nwant %s", tc.name, got, tc.want)
		}
	}
}
