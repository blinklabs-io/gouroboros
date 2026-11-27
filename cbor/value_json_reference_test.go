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
	"encoding/json"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// TestValueJSONAsReferencePlutusDataToJson renders Plutus Data through
// Value and LazyValue and expects the "json" member to be what
// plutusDataToJson writes for the same Data (cardano-ledger 1210d37f
// libs/cardano-ledger-core/src/Cardano/Ledger/Plutus/Data.hs 140-156):
// {"constructor": n, "fields": [...]}, {"map": [{"k": ..., "v": ...}]},
// {"list": [...]}, {"int": n} and {"bytes": hex}. The reference keeps a
// map's entries in wire order and Value does not keep that order, so each
// map here is already in the order Value writes.
func TestValueJSONAsReferencePlutusDataToJson(t *testing.T) {
	cases := map[string]string{
		"01":                     `{"int":1}`,
		"20":                     `{"int":-1}`,
		"1bffffffffffffffff":     `{"int":18446744073709551615}`,
		"c249010000000000000000": `{"int":18446744073709551616}`,
		"3bffffffffffffffff":     `{"int":-18446744073709551616}`,
		"40":                     `{"bytes":""}`,
		"4401020304":             `{"bytes":"01020304"}`,
		"5f42010243030405ff":     `{"bytes":"0102030405"}`,
		"80":                     `{"list":[]}`,
		"83010203":               `{"list":[{"int":1},{"int":2},{"int":3}]}`,
		"9f01820203ff":           `{"list":[{"int":1},{"list":[{"int":2},{"int":3}]}]}`,
		"a0":                     `{"map":[]}`,
		"a201020304": `{"map":[{"k":{"int":1},"v":{"int":2}},` +
			`{"k":{"int":3},"v":{"int":4}}]}`,
		"a14101d87980": `{"map":[{"k":{"bytes":"01"},` +
			`"v":{"constructor":0,"fields":[]}}]}`,
		"d87980":           `{"constructor":0,"fields":[]}`,
		"d8799f01ff":       `{"constructor":0,"fields":[{"int":1}]}`,
		"d905009f4100ff":   `{"constructor":7,"fields":[{"bytes":"00"}]}`,
		"d866821903e880":   `{"constructor":1000,"fields":[]}`,
		"d87a9f9f01ffa0ff": `{"constructor":1,"fields":[{"list":[{"int":1}]},{"map":[]}]}`,
		// plutus bdc12f105a98 test/CBOR/DataStability.hs "auction_1-1",
		// both encodings: Constr 0 [B pkh, Constr 0 []].
		"d87982581cbd99a373075d42fe4ac9109515e46303d0940cb9620bf058b87986a9d87980":   `{"constructor":0,"fields":[{"bytes":"bd99a373075d42fe4ac9109515e46303d0940cb9620bf058b87986a9"},{"constructor":0,"fields":[]}]}`,
		"d8799f581cbd99a373075d42fe4ac9109515e46303d0940cb9620bf058b87986a9d87980ff": `{"constructor":0,"fields":[{"bytes":"bd99a373075d42fe4ac9109515e46303d0940cb9620bf058b87986a9"},{"constructor":0,"fields":[]}]}`,
	}
	for h, want := range cases {
		data := mustHex(t, h)
		var v cbor.Value
		if _, err := cbor.Decode(data, &v); err != nil {
			t.Errorf("%s: %v", h, err)
			continue
		}
		out, err := v.MarshalJSON()
		if err != nil {
			t.Errorf("%s: %v", h, err)
			continue
		}
		checkJSONMember(t, h, out, want)
		var lv cbor.LazyValue
		if _, err := cbor.Decode(data, &lv); err != nil {
			t.Fatal(err)
		}
		out, err = lv.MarshalJSON()
		if err != nil {
			t.Errorf("%s: LazyValue: %v", h, err)
			continue
		}
		checkJSONMember(t, h, out, want)
	}
}

func checkJSONMember(t *testing.T, h string, out []byte, want string) {
	t.Helper()
	var got struct {
		Cbor string          `json:"cbor"`
		JSON json.RawMessage `json:"json"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("%s: %s: %v", h, out, err)
	}
	if got.Cbor != h {
		t.Errorf("%s: cbor member %q", h, got.Cbor)
	}
	// UseNumber keeps integers beyond float64 precision exact.
	var a, b any
	for _, p := range []struct {
		src []byte
		dst *any
	}{{got.JSON, &a}, {[]byte(want), &b}} {
		dec := json.NewDecoder(bytes.NewReader(p.src))
		dec.UseNumber()
		if err := dec.Decode(p.dst); err != nil {
			t.Fatal(err)
		}
	}
	if ja, _ := json.Marshal(a); string(ja) != mustCanon(t, b) {
		t.Errorf("%s:\n got %s\nwant %s", h, got.JSON, want)
	}
}

func mustCanon(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
