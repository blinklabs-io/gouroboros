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
	"bytes"
	"encoding/hex"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// The expectations in this file are derived from cardano-ledger-binary at
// cardano-ledger 1210d37f, file
// libs/cardano-ledger-binary/src/Cardano/Ledger/Binary/Decoding/Decoder.hs
// (decoders) and .../Encoding/Encoder.hs (encoders). No golden file covers
// these primitives, so each expected byte string is built by hand from the
// reference rule named in the test, never from this package's output.

// TestRatAsReferenceDecodeRational follows decodeRational (Decoder.hs
// 465-484) for protocol version 9 and later: tag 30 is consumed when present
// but not required, the list may be definite or indefinite, it must hold
// exactly two integers (decodeInteger, so bignums too), and only a zero
// denominator is refused (decodeRationalWithoutTag, 519-527); the value is
// n % d, which normalises a negative denominator.
func TestRatAsReferenceDecodeRational(t *testing.T) {
	accept := []struct{ hex, want string }{
		{"d81e820102", "1/2"},
		{"820102", "1/2"},
		{"d81e9f0102ff", "1/2"},
		{"9f0306ff", "1/2"},
		{"d81e82190fa01a000f4240", "1/250"},
		{"d81e820120", "-1/1"},
		{"d81e823863190190", "-1/4"},
		{"d81e82c2490100000000000000000a", "9223372036854775808/5"},
		{"d81e82181801", "24/1"},
	}
	for _, c := range accept {
		var r cbor.Rat
		if _, err := cbor.Decode(mustHex(t, c.hex), &r); err != nil {
			t.Errorf("%s: %v", c.hex, err)
			continue
		}
		want, _ := new(big.Rat).SetString(c.want)
		if r.Cmp(want) != 0 {
			t.Errorf("%s: got %s, want %s", c.hex, r.RatString(), want)
		}
	}
	reject := map[string]string{
		"d81e820100":     "zero denominator",
		"d81e9f0100ff":   "zero denominator, indefinite",
		"d81e8101":       "one element",
		"d81e83010203":   "three elements",
		"d81e80":         "empty list",
		"d81e9f01ff":     "indefinite, one element",
		"d81e9f010203ff": "indefinite, three elements",
		"d81e82f93c0001": "float numerator",
		"d81e820141":     "byte string denominator",
		"d81e8201f6":     "null denominator",
		"d81ea10102":     "map instead of list",
		"d81e8201d81801": "tag 24 around the denominator",
	}
	for h, why := range reject {
		var r cbor.Rat
		if _, err := cbor.Decode(mustHex(t, h), &r); err == nil {
			t.Errorf("%s (%s): accepted as %s", h, why, r.RatString())
		}
	}
}

// TestRatMarshalAsReferenceEncodeRatio follows encodeRatio (Encoder.hs
// 287-306): from version 9 a ratio is tag 30 then a two-element definite
// list of its numerator and denominator, each a minimal integer.
func TestRatMarshalAsReferenceEncodeRatio(t *testing.T) {
	cases := []struct {
		num, den int64
		want     string
	}{
		{1, 2, "d81e820102"},
		{3, 1000, "d81e82031903e8"},
		{-1, 4, "d81e822004"},
		{0, 1, "d81e820001"},
		{24, 25, "d81e8218181819"},
	}
	for _, c := range cases {
		r := cbor.Rat{Rat: big.NewRat(c.num, c.den)}
		got, err := r.MarshalCBOR()
		if err != nil {
			t.Errorf("%d/%d: %v", c.num, c.den, err)
			continue
		}
		if hex.EncodeToString(got) != c.want {
			t.Errorf("%d/%d: got %x, want %s", c.num, c.den, got, c.want)
		}
	}
}

// TestRatUnmarshalJSONAsReference covers the two JSON forms the reference
// accepts for a rational protocol parameter: a {numerator, denominator}
// object and a decimal number (BaseTypes.hs boundedRationalFromJSON reads a
// Scientific; the object form is the canonical-state rendering).
func TestRatUnmarshalJSONAsReference(t *testing.T) {
	cases := map[string]string{
		`{"numerator":1,"denominator":2}`: "1/2",
		`0.5`:                             "1/2",
		`0.0577`:                          "577/10000",
		`3`:                               "3/1",
	}
	for in, want := range cases {
		var r cbor.Rat
		if err := r.UnmarshalJSON([]byte(in)); err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		w, _ := new(big.Rat).SetString(want)
		if r.Cmp(w) != 0 {
			t.Errorf("%s: got %s, want %s", in, r.RatString(), want)
		}
	}
	var r cbor.Rat
	if err := r.UnmarshalJSON(
		[]byte(`{"numerator":1,"denominator":0}`),
	); err == nil {
		t.Error("zero denominator accepted")
	}
}

// TestSetTypeAsReferenceDecodeSet follows decodeSetLikeEnforceNoDuplicates
// (Decoder.hs 1060-1079), the version 9 and later set decoder: tag 258 is
// allowed but not required, any other tag is refused, the list may be
// definite or indefinite, and two members that decode to the same value are
// refused whether or not the tag is present. Members are compared as
// decoded values (Set.size against the count), so a member written with a
// non-minimal head still duplicates its minimal twin.
func TestSetTypeAsReferenceDecodeSet(t *testing.T) {
	accept := map[string][]uint64{
		"d9010283010203":   {1, 2, 3},
		"83010203":         {1, 2, 3},
		"d901029f010203ff": {1, 2, 3},
		"9f010203ff":       {1, 2, 3},
		"d9010280":         {},
		"80":               {},
	}
	for h, want := range accept {
		var s cbor.SetType[uint64]
		if _, err := cbor.Decode(mustHex(t, h), &s); err != nil {
			t.Errorf("%s: %v", h, err)
			continue
		}
		got := s.Items()
		if len(got) != len(want) {
			t.Errorf("%s: got %v, want %v", h, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s: got %v, want %v", h, got, want)
			}
		}
		if err := s.CheckForDuplicatesAlways(); err != nil {
			t.Errorf("%s: %v", h, err)
		}
		// The stored bytes are re-emitted unchanged.
		out, err := s.MarshalCBOR()
		if err != nil || hex.EncodeToString(out) != h {
			t.Errorf("%s: re-encoded as %x (%v)", h, out, err)
		}
	}
	for _, h := range []string{"d9010383010203", "d81e83010203"} {
		var s cbor.SetType[uint64]
		if _, err := cbor.Decode(mustHex(t, h), &s); err == nil {
			t.Errorf("%s: a tag other than 258 was accepted", h)
		}
	}
	duplicates := []string{
		"d90102820101",
		"820101",
		"d901029f0101ff",
		"9f0101ff",
		"d9010282011801",
		"8201190001",
	}
	for _, h := range duplicates {
		var s cbor.SetType[uint64]
		if _, err := cbor.Decode(mustHex(t, h), &s); err != nil {
			t.Errorf("%s: %v", h, err)
			continue
		}
		if err := s.CheckForDuplicatesAlways(); err == nil {
			t.Errorf("%s: duplicate members accepted", h)
		}
	}
	if err := cbor.CheckForDuplicateCBORMembers(
		[]uint64{1, 2, 1},
	); err == nil {
		t.Error("CheckForDuplicateCBORMembers accepted 1, 2, 1")
	}
	if err := cbor.CheckForDuplicateCBORMembers(
		[]uint64{1, 2, 3},
	); err != nil {
		t.Errorf("CheckForDuplicateCBORMembers: %v", err)
	}
}

// TestSetTypeCheckForDuplicatesTaggedOnly pins the documented narrower
// check: CheckForDuplicates looks only at sets that carried tag 258. Before
// version 9 decodeSet (Decoder.hs 943-953) drops duplicates silently, so an
// untagged list is never refused there.
func TestSetTypeCheckForDuplicatesTaggedOnly(t *testing.T) {
	var untagged, tagged cbor.SetType[uint64]
	if _, err := cbor.Decode(mustHex(t, "820101"), &untagged); err != nil {
		t.Fatal(err)
	}
	if err := untagged.CheckForDuplicates(); err != nil {
		t.Errorf("untagged: %v", err)
	}
	if _, err := cbor.Decode(mustHex(t, "d90102820101"), &tagged); err != nil {
		t.Fatal(err)
	}
	if err := tagged.CheckForDuplicates(); err == nil {
		t.Error("tagged duplicates accepted")
	}
}

// TestSetTypeMarshalAsReferenceEncodeSet follows encodeSet (Encoder.hs
// 479-491) with variableListLenEncoding (458-468): from version 9 tag 258,
// then a definite list up to 23 members and an indefinite list above that;
// versions 2-8 write the same list with no tag.
func TestSetTypeMarshalAsReferenceEncodeSet(t *testing.T) {
	for _, n := range []int{0, 1, 23, 24, 30} {
		items := make([]uint64, n)
		var body bytes.Buffer
		for i := range items {
			items[i] = uint64(i)
			if i < 24 {
				body.WriteByte(byte(i))
			} else {
				body.Write([]byte{0x18, byte(i)})
			}
		}
		var list []byte
		if n <= 23 {
			list = append([]byte{0x80 | byte(n)}, body.Bytes()...)
		} else {
			list = append([]byte{0x9f}, body.Bytes()...)
			list = append(list, 0xff)
		}
		for _, useTag := range []bool{true, false} {
			want := list
			if useTag {
				want = append([]byte{0xd9, 0x01, 0x02}, list...)
			}
			s := cbor.NewSetType(items, useTag)
			got, err := s.MarshalCBOR()
			if err != nil {
				t.Fatalf("n=%d tag=%v: %v", n, useTag, err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("n=%d tag=%v: got %x, want %x", n, useTag, got, want)
			}
		}
	}
	js, err := cbor.NewSetType([]uint64{1, 2}, true).MarshalJSON()
	if err != nil || string(js) != "[1,2]" {
		t.Errorf("MarshalJSON: %s %v", js, err)
	}
}

// TestDuplicateMapKeysAsReferenceDecodeMap follows decodeMapByKey
// (Decoder.hs 801-822): from version 9 a repeated key is refused
// (decodeMapLikeEnforceNoDuplicates); from version 2 to 8 the map is built
// with Map.fromList, so the last value for a key wins. Keys are compared as
// decoded values, so a key with a non-minimal head repeats its minimal
// twin.
func TestDuplicateMapKeysAsReferenceDecodeMap(t *testing.T) {
	cases := map[string]uint64{
		"a201020103":   3,
		"a20102181803": 0,
		"a2010218010a": 10,
		"bf01020103ff": 3,
	}
	for h, last := range cases {
		data := mustHex(t, h)
		var strict map[uint64]uint64
		_, err := cbor.Decode(data, &strict)
		dup := last != 0
		if dup && err == nil {
			t.Errorf("%s: Decode accepted a repeated key", h)
		}
		if !dup && err != nil {
			t.Errorf("%s: Decode: %v", h, err)
		}
		var lenient map[uint64]uint64
		if _, err := cbor.DecodeLenient(data, &lenient); err != nil {
			t.Errorf("%s: DecodeLenient: %v", h, err)
			continue
		}
		if dup && (len(lenient) != 1 || lenient[1] != last) {
			t.Errorf("%s: DecodeLenient kept %v, want {1:%d}", h, lenient, last)
		}
		var v cbor.Value
		if _, err := cbor.DecodeLenient(data, &v); err != nil {
			t.Errorf("%s: DecodeLenient Value: %v", h, err)
		}
		var sv cbor.Value
		if _, err := cbor.Decode(data, &sv); dup && err == nil {
			t.Errorf("%s: Decode Value accepted a repeated key", h)
		}
	}
}

// TestDecodeIdFromListAsReference follows the sum-type decoders
// (decodeRecordSum, Decoder.hs, and the Coders.hs Summands): the list header
// may be definite in any width or indefinite, and the tag is read with
// decodeWord, which accepts a non-minimal head. An empty list or a tag that
// is not an unsigned integer is refused.
func TestDecodeIdFromListAsReference(t *testing.T) {
	accept := map[string]int{
		"8200f6":                 0,
		"820301":                 3,
		"82180001":               0,
		"8219000701":             7,
		"821a0000000701":         7,
		"821b000000000000000701": 7,
		"9f05ff":                 5,
		"98020401":               4,
		"8118ff":                 255,
	}
	for h, want := range accept {
		got, err := cbor.DecodeIdFromList(mustHex(t, h))
		if err != nil || got != want {
			t.Errorf("%s: got %d, %v; want %d", h, got, err, want)
		}
	}
	for _, h := range []string{
		"80", "9fff", "824100", "822000", "82f500", "a10000", "00",
		"9f",
	} {
		if got, err := cbor.DecodeIdFromList(mustHex(t, h)); err == nil {
			t.Errorf("%s: accepted with id %d", h, got)
		}
	}
	var a, b uint64
	ret, err := cbor.DecodeById(
		mustHex(t, "8201"),
		map[int]any{0: &a, 1: &b},
	)
	if err == nil {
		t.Errorf("DecodeById decoded [1] into %T", ret)
	}
	var arr []uint64
	ret, err = cbor.DecodeById(
		mustHex(t, "820107"),
		map[int]any{1: &arr},
	)
	if err != nil || ret != &arr || len(arr) != 2 || arr[1] != 7 {
		t.Errorf("DecodeById: %v %v %v", ret, arr, err)
	}
	if _, err := cbor.DecodeById(
		mustHex(t, "820207"),
		map[int]any{1: &arr},
	); err == nil {
		t.Error("DecodeById accepted an unknown id")
	}
}

// TestDecodeBoolAsReference follows cborg's decodeBool, which reads only
// the simple values false (0xf4) and true (0xf5).
func TestDecodeBoolAsReference(t *testing.T) {
	if v, err := cbor.DecodeBool([]byte{0xf4}); err != nil || v {
		t.Errorf("f4: %v %v", v, err)
	}
	if v, err := cbor.DecodeBool([]byte{0xf5}); err != nil || !v {
		t.Errorf("f5: %v %v", v, err)
	}
	for _, h := range []string{"f6", "f7", "00", "01", "f814", "f815", "", "f5f5"} {
		if _, err := cbor.DecodeBool(mustHex(t, h)); err == nil {
			t.Errorf("%q accepted", h)
		}
	}
}

// TestListAndHeaderInfoAsRFC8949 checks the head readers against RFC 8949
// section 3: the argument follows in 1, 2, 4 or 8 bytes for additional
// information 24-27, 31 means indefinite, and 28-30 are reserved.
// cborg's decodeListLen accepts a non-minimal head, so ListLength must too.
func TestListAndHeaderInfoAsRFC8949(t *testing.T) {
	lists := map[string]int{
		"80":                       0,
		"83010203":                 3,
		"9803010203":               3,
		"990003010203":             3,
		"9a00000003010203":         3,
		"9b0000000000000003010203": 3,
		"9f010203ff":               3,
		"9fff":                     0,
	}
	for h, want := range lists {
		got, err := cbor.ListLength(mustHex(t, h))
		if err != nil || got != want {
			t.Errorf("ListLength %s: %d %v, want %d", h, got, err, want)
		}
	}
	for _, h := range []string{"", "9a0000000401", "9f0102", "a0", "9c"} {
		if _, err := cbor.ListLength(mustHex(t, h)); err == nil {
			t.Errorf("ListLength %q accepted", h)
		}
	}
	heads := []struct {
		hex        string
		count      int
		size       uint32
		indefinite bool
	}{
		{"80", 0, 1, false},
		{"97", 23, 1, false},
		{"9818", 24, 2, false},
		{"990100", 256, 3, false},
		{"9a00010000", 65536, 5, false},
		{"9b0000000000000001", 1, 9, false},
		{"9f", 0, 1, true},
		{"9c", -1, 0, false},
		{"98", -1, 0, false},
		{"a0", -1, 0, false},
	}
	for _, c := range heads {
		data := mustHex(t, c.hex)
		n, sz, ind := cbor.ArrayInfo(data)
		if n != c.count || sz != c.size || ind != c.indefinite {
			t.Errorf("ArrayInfo %s: %d %d %v", c.hex, n, sz, ind)
		}
		if c.hex == "a0" {
			continue
		}
		m := append([]byte{data[0] + 0x20}, data[1:]...)
		n, sz, ind = cbor.MapInfo(m)
		if n != c.count || sz != c.size || ind != c.indefinite {
			t.Errorf("MapInfo %x: %d %d %v", m, n, sz, ind)
		}
	}
	sizes := map[int]uint32{
		0: 1, 23: 1, 24: 2, 255: 2, 256: 3, 65535: 3, 65536: 5,
		math.MaxInt32: 5,
	}
	for n, want := range sizes {
		if got := cbor.ArrayHeaderSize(n); got != want {
			t.Errorf("ArrayHeaderSize(%d) = %d, want %d", n, got, want)
		}
	}
}

// TestIsEmptyCollectionAsReference follows the non-empty set and map
// decoders (decodeNonEmptySetLikeEnforceNoDuplicates, Decoder.hs 1082-1098):
// emptiness is the item count, whatever the list encoding, behind an
// optional tag 258.
func TestIsEmptyCollectionAsReference(t *testing.T) {
	cases := map[string]bool{
		"80": true, "a0": true, "9fff": true, "bfff": true,
		"d9010280": true, "d901029fff": true, "8100": false,
		"a10000": false, "d901028100": false, "9f00ff": false,
		"9800": true,
	}
	for h, want := range cases {
		got, err := cbor.IsEmptyCollection(mustHex(t, h))
		if err != nil || got != want {
			t.Errorf("%s: %v %v, want %v", h, got, err, want)
		}
	}
	for _, h := range []string{"", "00", "d9010380", "d90102", "c080"} {
		if _, err := cbor.IsEmptyCollection(mustHex(t, h)); err == nil {
			t.Errorf("%q accepted", h)
		}
	}
}

// TestValidateMapFieldsAsReference follows the transaction body decoders
// (Babbage/Conway TxBody.hs, decodeRecordSparse with required fields 0, 1
// and 2 and the non-empty set fields): a required key must be present and
// a non-empty field must hold at least one item.
func TestValidateMapFieldsAsReference(t *testing.T) {
	ok := "a3008100018100020a"
	if err := cbor.ValidateMapFields(
		mustHex(t, ok), []uint64{0, 1, 2}, []uint64{0},
	); err != nil {
		t.Errorf("%s: %v", ok, err)
	}
	bad := map[string]string{
		"a2008100018100":         "missing key 2",
		"a3008001810002 0a":      "empty field 0",
		"a300d9010280018100020a": "empty tagged field 0",
		"a300000181000200":       "field 0 not a collection",
		"a3008100008100020a":     "repeated key 0",
	}
	for h, why := range bad {
		err := cbor.ValidateMapFields(
			mustHex(t, strings.ReplaceAll(h, " ", "")),
			[]uint64{0, 1, 2},
			[]uint64{0},
		)
		if err == nil {
			t.Errorf("%s (%s) accepted", h, why)
		}
	}
}

// TestUntrustedNestingLimit pins the peer nesting bound: an item nested
// MaxUntrustedNestedLevels deep is accepted and one level more is refused
// by every untrusted entry point. The reference decoder has no such bound;
// it is this package's guard for peer-controlled input.
func TestUntrustedNestingLimit(t *testing.T) {
	nest := func(depth int) []byte {
		b := bytes.Repeat([]byte{0x81}, depth-1)
		return append(b, 0x80)
	}
	atLimit := nest(cbor.MaxUntrustedNestedLevels)
	over := nest(cbor.MaxUntrustedNestedLevels + 1)
	if err := cbor.ValidateUntrustedExact(atLimit); err != nil {
		t.Errorf("at limit: %v", err)
	}
	var v cbor.Value
	if _, err := cbor.DecodeUntrustedExact(atLimit, &v); err != nil {
		t.Errorf("at limit: %v", err)
	}
	if err := cbor.ValidateUntrustedExact(over); err == nil {
		t.Error("ValidateUntrustedExact accepted one level too deep")
	}
	var v2 cbor.Value
	if _, err := cbor.DecodeUntrustedExact(over, &v2); err == nil {
		t.Error("DecodeUntrustedExact accepted one level too deep")
	}
	sd, _ := cbor.NewUntrustedStreamDecoder(over)
	if _, _, err := sd.Skip(); err == nil {
		t.Error("untrusted Skip accepted one level too deep")
	}
	if err := cbor.ValidateExact(over); err != nil {
		t.Errorf("ValidateExact uses the ledger limit: %v", err)
	}
}
