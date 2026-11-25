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

// The expectations here are derived from plutus bdc12f105a98,
// plutus-core/plutus-core/src/PlutusCore/Data.hs: encodeData (lines
// 147-160) writes alternative i as tag 121+i for 0-6, tag 1280+(i-7) for
// 7-127 and tag 102 around [i, fields] otherwise; decodeConstr (286-307)
// reads the same three forms, and tag 102 accepts any Word64 alternative.

// TestConstructorEncoderAsReference checks the tag chosen for each
// alternative and the tag 102 wrapper. The fields are given as an
// indefinite list because Data.hs encodes a non-empty [Data] with the
// Serialise list instance, which writes it indefinite; an empty list is
// written 0x80.
func TestConstructorEncoderAsReference(t *testing.T) {
	one := cbor.IndefLengthList{uint64(1)}
	cases := []struct {
		alt    uint
		fields any
		want   string
	}{
		{0, []any{}, "d87980"},
		{0, one, "d8799f01ff"},
		{6, one, "d87f9f01ff"},
		{7, one, "d905009f01ff"},
		{127, one, "d905789f01ff"},
		{128, one, "d8668218809f01ff"},
		{1000, []any{}, "d866821903e880"},
	}
	for _, c := range cases {
		got, err := cbor.NewConstructorEncoder(c.alt, c.fields).MarshalCBOR()
		if err != nil {
			t.Errorf("alt %d: %v", c.alt, err)
			continue
		}
		if hex.EncodeToString(got) != c.want {
			t.Errorf("alt %d: got %x, want %s", c.alt, got, c.want)
		}
	}
}

// TestConstructorDecoderAsReference decodes each constructor form, checks
// the alternative and fields, and that the original bytes are re-emitted.
func TestConstructorDecoderAsReference(t *testing.T) {
	cases := []struct {
		hex    string
		alt    uint
		fields int
	}{
		{"d87980", 0, 0},
		{"d8799f01ff", 0, 1},
		{"d87a820102", 1, 2},
		{"d87f9f01ff", 6, 1},
		{"d905009f01ff", 7, 1},
		{"d905789f01ff", 127, 1},
		{"d8668218809f01ff", 128, 1},
		{"d866820380", 3, 0},
		{"d8669f1903e880ff", 1000, 0},
	}
	for _, c := range cases {
		data := mustHex(t, c.hex)
		var cd cbor.ConstructorDecoder
		if _, err := cbor.Decode(data, &cd); err != nil {
			t.Errorf("%s: %v", c.hex, err)
			continue
		}
		fields, err := cd.ParsedFields()
		if cd.Tag() != c.alt || err != nil || len(fields) != c.fields {
			t.Errorf(
				"%s: alt %d fields %v (%v), want alt %d with %d",
				c.hex, cd.Tag(), fields, err, c.alt, c.fields,
			)
		}
		out, err := cd.MarshalCBOR()
		if err != nil || hex.EncodeToString(out) != c.hex {
			t.Errorf("%s: re-encoded as %x (%v)", c.hex, out, err)
		}
	}
	for _, h := range []string{
		"d87880",         // tag 120 is no constructor
		"d9057980",       // tag 1401 is no constructor
		"d8668180",       // tag 102 with one element
		"d86683018080",   // tag 102 with three elements
		"d866822080",     // negative alternative
		"d86682f93c0080", // float alternative
		"d866829f80",     // truncated
	} {
		var cd cbor.ConstructorDecoder
		if _, err := cbor.Decode(mustHex(t, h), &cd); err == nil {
			t.Errorf("%s: accepted as alternative %d", h, cd.Tag())
		}
	}
}
