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

package ledger_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// Before protocol version 9, cardano-ledger-binary decodes every
// set-shaped field with decodeSet or decodeList, and neither accepts a tag:
// decodeSet for versions 2-8 is decodeCollection decodeListLenOrIndef
// ("set tag 258 is not permitted", Decoder.hs 935-953 at cardano-ledger
// 1210d37f), and the Shelley and Alonzo witness decoders use decodeList
// below version 9 (Shelley/TxWits.hs 261-273, Alonzo/TxWits.hs 347-357,
// 569-580, 660-700, 721-730). Shelley to Babbage transactions decode at
// versions 2-8, so a tag 258 on any of these fields is refused there. Each
// case also decodes the same field without the tag, so a refusal is due to
// the tag alone.

func hdr(major byte, n int) []byte {
	switch {
	case n < 24:
		return []byte{major | byte(n)}
	case n < 256:
		return []byte{major | 24, byte(n)}
	default:
		return []byte{major | 25, byte(n >> 8), byte(n)}
	}
}

func bstr(n int, fill byte) []byte {
	return append(hdr(0x40, n), bytes.Repeat([]byte{fill}, n)...)
}

func list(tagged bool, items ...[]byte) []byte {
	var out []byte
	if tagged {
		out = []byte{0xd9, 0x01, 0x02}
	}
	out = append(out, hdr(0x80, len(items))...)
	for _, it := range items {
		out = append(out, it...)
	}
	return out
}

func arr(items ...[]byte) []byte { return list(false, items...) }

type field struct {
	key   byte
	value []byte
}

func cmap(fields ...field) []byte {
	out := hdr(0xa0, len(fields))
	for _, f := range fields {
		out = append(out, f.key)
		out = append(out, f.value...)
	}
	return out
}

func txIn(i byte) []byte { return arr(bstr(32, i), []byte{0x00}) }

func poolCert(tagged bool) []byte {
	return arr(
		[]byte{0x03},
		bstr(28, 1),
		bstr(32, 2),
		[]byte{0x00},
		[]byte{0x00},
		[]byte{0xd8, 0x1e, 0x82, 0x00, 0x01},
		append(hdr(0x40, 29), append([]byte{0xe0}, bytes.Repeat(
			[]byte{3}, 28)...)...),
		list(tagged, bstr(28, 4)),
		arr(),
		[]byte{0xf6},
	)
}

func shelleyBodyFields(extra ...field) []field {
	return append([]field{
		{0x00, arr(txIn(1))},
		{0x01, arr()},
		{0x02, []byte{0x00}},
		{0x03, []byte{0x19, 0x03, 0xe8}},
	}, extra...)
}

func alonzoBodyFields(extra ...field) []field {
	return append([]field{
		{0x00, arr(txIn(1))},
		{0x01, arr()},
		{0x02, []byte{0x00}},
	}, extra...)
}

func TestPreConwaySetFieldsRefuseTag258(t *testing.T) {
	type tc struct {
		name string
		make func(tagged bool) []byte
		dest func() any
	}
	certs := func(f func(...field) []field) func(bool) []byte {
		return func(tagged bool) []byte {
			return cmap(f(field{0x04, arr(poolCert(tagged))})...)
		}
	}
	bodyField := func(
		f func(...field) []field, key byte, item []byte,
	) func(bool) []byte {
		return func(tagged bool) []byte {
			return cmap(f(field{key, list(tagged, item)})...)
		}
	}
	vkey := arr(bstr(32, 5), bstr(64, 6))
	native := arr([]byte{0x00}, bstr(28, 7))
	boot := arr(bstr(32, 5), bstr(64, 6), bstr(32, 8), bstr(1, 0xa0))
	redeemer := arr([]byte{0x00}, []byte{0x00}, []byte{0x01},
		arr([]byte{0x00}, []byte{0x00}))
	script := bstr(4, 9)
	witness := func(key byte, item []byte) func(bool) []byte {
		return func(tagged bool) []byte {
			return cmap(field{key, list(tagged, item)})
		}
	}
	cases := []tc{
		{"shelley pool owners", certs(shelleyBodyFields),
			func() any { return &shelley.ShelleyTransactionBody{} }},
		{"allegra pool owners", certs(alonzoBodyFields),
			func() any { return &allegra.AllegraTransactionBody{} }},
		{"mary pool owners", certs(alonzoBodyFields),
			func() any { return &mary.MaryTransactionBody{} }},
		{"alonzo pool owners", certs(alonzoBodyFields),
			func() any { return &alonzo.AlonzoTransactionBody{} }},
		{"babbage pool owners", certs(alonzoBodyFields),
			func() any { return &babbage.BabbageTransactionBody{} }},
		{"alonzo collateral", bodyField(alonzoBodyFields, 0x0d, txIn(2)),
			func() any { return &alonzo.AlonzoTransactionBody{} }},
		{"alonzo required signers",
			bodyField(alonzoBodyFields, 0x0e, bstr(28, 2)),
			func() any { return &alonzo.AlonzoTransactionBody{} }},
		{"babbage collateral", bodyField(alonzoBodyFields, 0x0d, txIn(2)),
			func() any { return &babbage.BabbageTransactionBody{} }},
		{"babbage required signers",
			bodyField(alonzoBodyFields, 0x0e, bstr(28, 2)),
			func() any { return &babbage.BabbageTransactionBody{} }},
		{"babbage reference inputs",
			bodyField(alonzoBodyFields, 0x12, txIn(2)),
			func() any { return &babbage.BabbageTransactionBody{} }},
	}
	witnessFields := []struct {
		key  byte
		name string
		item []byte
	}{
		{0, "vkey witnesses", vkey},
		{1, "native scripts", native},
		{2, "bootstrap witnesses", boot},
		{3, "plutus v1 scripts", script},
		{4, "plutus data", []byte{0x01}},
		{5, "redeemers", redeemer},
		{6, "plutus v2 scripts", script},
	}
	for _, w := range witnessFields {
		if w.key <= 2 {
			cases = append(cases, tc{"shelley " + w.name,
				witness(w.key, w.item),
				func() any { return &shelley.ShelleyTransactionWitnessSet{} }})
		}
		if w.key <= 5 {
			cases = append(cases, tc{"alonzo " + w.name,
				witness(w.key, w.item),
				func() any { return &alonzo.AlonzoTransactionWitnessSet{} }})
		}
		cases = append(cases, tc{"babbage " + w.name,
			witness(w.key, w.item),
			func() any { return &babbage.BabbageTransactionWitnessSet{} }})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := cbor.Decode(c.make(false), c.dest()); err != nil {
				t.Fatalf("untagged control: %v", err)
			}
			data := c.make(true)
			if _, err := cbor.Decode(data, c.dest()); err == nil {
				t.Errorf("tag 258 accepted: %s", fmt.Sprintf("%x", data))
			}
		})
	}
}
