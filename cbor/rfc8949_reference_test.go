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

// rfc8949WellFormed is every encoded example of RFC 8949 Appendix A. The
// hex strings are copied from the RFC's table, not produced by this package.
var rfc8949WellFormed = []string{
	"00", "01", "0a", "17", "1818", "1819", "1864", "1903e8",
	"1a000f4240", "1b000000e8d4a51000", "1bffffffffffffffff",
	"c249010000000000000000", "3bffffffffffffffff",
	"c349010000000000000000", "20", "29", "3863", "3903e7",
	"f90000", "f98000", "f93c00", "fb3ff199999999999a", "f93e00",
	"f97bff", "fa47c35000", "fa7f7fffff", "fb7e37e43c8800759c",
	"f90001", "f90400", "f9c400", "fbc010666666666666", "f97c00",
	"f97e00", "f9fc00", "fa7f800000", "fa7fc00000", "faff800000",
	"fb7ff0000000000000", "fb7ff8000000000000", "fbfff0000000000000",
	"f4", "f5", "f6", "f7", "f0", "f8ff",
	"c074323031332d30332d32315432303a30343a30305a",
	"c11a514b67b0", "c1fb41d452d9ec200000", "d74401020304",
	"d818456449455446",
	"d82076687474703a2f2f7777772e6578616d706c652e636f6d",
	"40", "4401020304", "60", "6161", "6449455446", "62225c", "62c3bc",
	"63e6b0b4", "64f0908591", "80", "83010203", "8301820203820405",
	"98190102030405060708090a0b0c0d0e0f101112131415161718181819",
	"a0", "a201020304", "a26161016162820203", "826161a161626163",
	"a56161614161626142616361436164614461656145",
	"5f42010243030405ff", "7f657374726561646d696e67ff", "9fff",
	"9f018202039f0405ffff", "9f01820203820405ff",
	"83018202039f0405ff", "83019f0203ff820405",
	"9f0102030405060708090a0b0c0d0e0f101112131415161718181819ff",
	"bf61610161629f0203ffff", "826161bf61626163ff",
	"bf6346756ef563416d7421ff",
}

// rfc8949Malformed is every example of RFC 8949 Appendix F.1, grouped as
// the RFC groups them. The reference node reads ledger data with cborg's
// Codec.CBOR.Read, which refuses each of these as not well-formed.
var rfc8949Malformed = map[string][]string{
	"end of input in a head": {
		"18", "19", "1a", "1b", "1901", "1a0102", "1b01020304050607",
		"38", "58", "78", "98", "9a01ff00", "b8", "d8", "f8", "f900",
		"fa0000", "fb000000",
	},
	"definite-length strings with short data": {
		"41", "61", "5affffffff00", "5bffffffffffffffff010203",
		"7affffffff00", "7b7fffffffffffffff010203",
	},
	"definite-length maps and arrays not closed": {
		"81", "818181818181818181", "8200", "a1", "a20102", "a100",
		"a2000000",
	},
	"tag number not followed by tag content": {"c0"},
	"indefinite-length strings not closed":   {"5f4100", "7f6100"},
	"indefinite-length maps and arrays not closed": {
		"9f", "9f0102", "bf", "bf01020102", "819f", "9f8000",
		"9f9f9f9f9fffffffff", "9f819f819f9fffffff",
	},
	"reserved additional information values": {
		"1c", "1d", "1e", "3c", "3d", "3e", "5c", "5d", "5e", "7c",
		"7d", "7e", "9c", "9d", "9e", "bc", "bd", "be", "dc", "dd",
		"de", "fc", "fd", "fe",
	},
	"reserved two-byte encodings of simple values": {
		"f800", "f801", "f818", "f81f",
	},
	"indefinite-length string chunks of the wrong type": {
		"5f00ff", "5f21ff", "5f6100ff", "5f80ff", "5fa0ff", "5fc000ff",
		"5fe0ff", "7f4100ff",
	},
	"indefinite-length string chunks not definite length": {
		"5f5f4100ffff", "7f7f6100ffff",
	},
	"break outside an indefinite-length item": {"ff"},
	"break in a definite-length array, map or tag": {
		"81ff", "8200ff", "a1ff", "a1ff00", "a100ff", "a20000ff",
		"9f81ff", "9f829f819f9fffffffff",
	},
	"break in a map value position": {"bf00ff", "bf000000ff"},
	"major type 0, 1 or 6 with additional information 31": {
		"1f", "3f", "df",
	},
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

// TestRFC8949WellFormedVectors decodes every RFC 8949 Appendix A example
// through each exact and streaming entry point. Each is one complete item,
// so each must be accepted and consumed to its last byte.
func TestRFC8949WellFormedVectors(t *testing.T) {
	for _, h := range rfc8949WellFormed {
		data := mustHex(t, h)
		t.Run(h, func(t *testing.T) {
			if err := cbor.ValidateExact(data); err != nil {
				t.Errorf("ValidateExact: %v", err)
			}
			if err := cbor.ValidateUntrustedExact(data); err != nil {
				t.Errorf("ValidateUntrustedExact: %v", err)
			}
			var v cbor.Value
			n, err := cbor.DecodeUntrustedExact(data, &v)
			if err != nil || n != len(data) {
				t.Errorf("DecodeUntrustedExact: n=%d err=%v", n, err)
			}
			var lv cbor.LazyValue
			if _, err := cbor.Decode(data, &lv); err != nil {
				t.Errorf("LazyValue decode: %v", err)
			} else if hex.EncodeToString(lv.Cbor()) != h {
				t.Errorf("LazyValue kept %x", lv.Cbor())
			}
			sd, err := cbor.NewUntrustedStreamDecoder(data)
			if err != nil {
				t.Fatal(err)
			}
			off, l, err := sd.Skip()
			if err != nil || off != 0 || l != len(data) || !sd.EOF() {
				t.Errorf(
					"Skip: off=%d len=%d eof=%v err=%v",
					off, l, sd.EOF(), err,
				)
			}
			sd, _ = cbor.NewStreamDecoder(data)
			var raw cbor.RawMessage
			_, rb, err := sd.DecodeRaw(&raw)
			if err != nil || hex.EncodeToString(rb) != h {
				t.Errorf("DecodeRaw: %x %v", rb, err)
			}
		})
	}
}

// TestRFC8949TrailingBytesRejected follows cardano-ledger-binary's
// decodeFullDecoder, which fails with DecoderErrorLeftover when bytes remain
// after the item (libs/cardano-ledger-binary/src/Cardano/Ledger/Binary/
// Decoding/Decoder.hs, toPlainDecoder/decodeFullDecoder at 1210d37f). The
// non-exact Decode reports only the first item's length.
func TestRFC8949TrailingBytesRejected(t *testing.T) {
	for _, h := range rfc8949WellFormed {
		item := mustHex(t, h)
		data := append(append([]byte{}, item...), 0x00)
		var v cbor.Value
		if _, err := cbor.DecodeUntrustedExact(data, &v); err == nil {
			t.Errorf("DecodeUntrustedExact accepted %s 00", h)
		}
		if err := cbor.ValidateUntrustedExact(data); err == nil {
			t.Errorf("ValidateUntrustedExact accepted %s 00", h)
		}
		if err := cbor.ValidateExact(data); err == nil {
			t.Errorf("ValidateExact accepted %s 00", h)
		}
		var raw cbor.RawMessage
		n, err := cbor.Decode(data, &raw)
		if err != nil || n != len(item) {
			t.Errorf("Decode %s 00: n=%d err=%v", h, n, err)
		}
	}
}

// TestRFC8949MalformedVectors runs every RFC 8949 Appendix F.1 example
// through each entry point; none may be accepted.
func TestRFC8949MalformedVectors(t *testing.T) {
	for group, vectors := range rfc8949Malformed {
		for _, h := range vectors {
			data := mustHex(t, h)
			t.Run(group+"/"+h, func(t *testing.T) {
				if err := cbor.ValidateExact(data); err == nil {
					t.Error("ValidateExact accepted it")
				}
				if err := cbor.ValidateUntrustedExact(data); err == nil {
					t.Error("ValidateUntrustedExact accepted it")
				}
				var v cbor.Value
				if _, err := cbor.DecodeUntrustedExact(data, &v); err == nil {
					t.Error("DecodeUntrustedExact accepted it")
				}
				var v2 cbor.Value
				if _, err := cbor.Decode(data, &v2); err == nil {
					t.Error("Decode into Value accepted it")
				}
				var lv cbor.LazyValue
				if _, err := cbor.Decode(data, &lv); err == nil {
					t.Error("Decode into LazyValue accepted it")
				}
				sd, err := cbor.NewUntrustedStreamDecoder(data)
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err := sd.Skip(); err == nil {
					t.Error("StreamDecoder.Skip accepted it")
				}
				sd, _ = cbor.NewStreamDecoder(data)
				var raw cbor.RawMessage
				if _, _, err := sd.Decode(&raw); err == nil {
					t.Error("StreamDecoder.Decode accepted it")
				}
			})
		}
	}
}

// TestRFC8949IntegerValues checks the integer examples of RFC 8949
// Appendix A decode to the values the RFC gives for them, including the
// tag 2 and tag 3 bignums that cardano-ledger-binary's decodeInteger reads.
func TestRFC8949IntegerValues(t *testing.T) {
	cases := []struct{ hex, value string }{
		{"00", "0"},
		{"01", "1"},
		{"0a", "10"},
		{"17", "23"},
		{"1818", "24"},
		{"1819", "25"},
		{"1864", "100"},
		{"1903e8", "1000"},
		{"1a000f4240", "1000000"},
		{"1b000000e8d4a51000", "1000000000000"},
		{"1bffffffffffffffff", "18446744073709551615"},
		{"c249010000000000000000", "18446744073709551616"},
		{"3bffffffffffffffff", "-18446744073709551616"},
		{"c349010000000000000000", "-18446744073709551617"},
		{"20", "-1"},
		{"29", "-10"},
		{"3863", "-100"},
		{"3903e7", "-1000"},
	}
	for _, c := range cases {
		want, _ := new(big.Int).SetString(c.value, 10)
		var got big.Int
		if _, err := cbor.DecodeUntrustedExact(
			mustHex(t, c.hex), &got,
		); err != nil {
			t.Errorf("%s: %v", c.hex, err)
			continue
		}
		if got.Cmp(want) != 0 {
			t.Errorf("%s: got %s, want %s", c.hex, &got, want)
		}
	}
}
