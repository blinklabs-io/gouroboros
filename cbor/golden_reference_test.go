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
	"os"
	"path/filepath"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// goldenTxItems is the number of elements of each era's transaction array:
// [body, witnesses, auxiliary data] up to Mary and
// [body, witnesses, is_valid, auxiliary data] from Alonzo to Conway, per
// eras/*/impl/cddl/data/*.cddl at cardano-ledger 1210d37f. The Dijkstra
// golden is a three-element transaction.
var goldenTxItems = map[string]int{
	"shelley": 3, "allegra": 3, "mary": 3, "alonzo": 4, "babbage": 4,
	"conway": 4, "dijkstra": 3,
}

func readGoldenTx(t *testing.T, era string) []byte {
	t.Helper()
	data, err := os.ReadFile(
		filepath.Join("testdata", "cardano-ledger", era+"-tx.cbor"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestGoldenTransactionsStreamAsReference walks each era's cardano-ledger
// golden transaction with the stream decoder. The array splits into the
// CDDL's element count, the items tile the input exactly, the body is a
// map whose 2n keys and values end where the witness set starts, and the
// generic and lazy decoders keep the bytes unchanged.
func TestGoldenTransactionsStreamAsReference(t *testing.T) {
	for era, items := range goldenTxItems {
		t.Run(era, func(t *testing.T) {
			data := readGoldenTx(t, era)
			if err := cbor.ValidateExact(data); err != nil {
				t.Fatalf("ValidateExact: %v", err)
			}
			if err := cbor.ValidateUntrustedExact(data); err != nil {
				t.Fatalf("ValidateUntrustedExact: %v", err)
			}
			sd, err := cbor.NewUntrustedStreamDecoder(data)
			if err != nil {
				t.Fatal(err)
			}
			var tiled []byte
			var offsets []int
			_, total, err := sd.DecodeArrayItems(
				func(_ int, off, _ int, item []byte) error {
					offsets = append(offsets, off)
					tiled = append(tiled, item...)
					return nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(offsets) != items || total != len(data) || !sd.EOF() {
				t.Fatalf(
					"%d items over %d bytes, want %d over %d",
					len(offsets), total, items, len(data),
				)
			}
			if !bytes.Equal(tiled, data[1:]) {
				t.Error("items do not tile the transaction")
			}

			sd, _ = cbor.NewStreamDecoder(data)
			if n, _, _, err := sd.DecodeArrayHeader(); err != nil ||
				n != items {
				t.Fatalf("DecodeArrayHeader: %d %v", n, err)
			}
			n, off, hl, err := sd.DecodeMapHeader()
			if err != nil || off != 1 || hl < 1 || n < 3 {
				t.Fatalf("DecodeMapHeader: %d %d %d %v", n, off, hl, err)
			}
			if _, _, err := sd.SkipN(2 * n); err != nil {
				t.Fatal(err)
			}
			if sd.Position() != offsets[1] {
				t.Errorf(
					"body ends at %d, witnesses start at %d",
					sd.Position(), offsets[1],
				)
			}
			start, l, err := sd.Skip()
			if err != nil || start != offsets[1] {
				t.Fatalf("Skip witnesses: %d %v", start, err)
			}
			if raw := sd.RawBytes(start, l); !bytes.Equal(
				raw, data[offsets[1]:offsets[1]+l],
			) {
				t.Error("RawBytes differs from the input")
			}
			if sd.RawBytes(0, len(data)+1) != nil ||
				sd.RawBytes(-1, 1) != nil {
				t.Error("RawBytes out of range returned data")
			}

			var lv cbor.LazyValue
			if _, err := cbor.Decode(data, &lv); err != nil {
				t.Fatal(err)
			}
			if out, err := lv.MarshalCBOR(); err != nil ||
				!bytes.Equal(out, data) {
				t.Error("LazyValue did not keep the transaction bytes")
			}
			var v cbor.Value
			if _, err := cbor.DecodeUntrustedExact(data, &v); err != nil {
				t.Fatal(err)
			}
			if out, err := v.MarshalCBOR(); err != nil ||
				!bytes.Equal(out, data) {
				t.Error("Value did not keep the transaction bytes")
			}
			if _, err := lv.Decode(); err != nil || lv.Value() == nil {
				t.Errorf("LazyValue.Decode: %v", err)
			}
		})
	}
}

// TestGoldenConwayInputSetAsReference reads field 0 of the Conway golden
// body, a tag 258 set of inputs (conway.cddl transaction_body), through
// SetType: the tag is kept and the reference's no-duplicates rule holds.
func TestGoldenConwayInputSetAsReference(t *testing.T) {
	data := readGoldenTx(t, "conway")
	var tx []cbor.RawMessage
	if _, err := cbor.Decode(data, &tx); err != nil {
		t.Fatal(err)
	}
	if len(tx) == 0 {
		t.Fatal("transaction has no components")
	}
	fields, _, err := cbor.DecodeRawMap[uint64](tx[0])
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := fields[0]
	if !ok || len(raw) < 3 {
		t.Fatalf("field 0 missing or short: %x", raw)
	}
	if hex.EncodeToString(raw[:3]) != "d90102" {
		t.Fatalf("field 0 starts %x, not tag 258", raw[:3])
	}
	var inputs cbor.SetType[cbor.RawMessage]
	if _, err := cbor.Decode(raw, &inputs); err != nil {
		t.Fatal(err)
	}
	if len(inputs.Items()) == 0 {
		t.Fatal("no inputs")
	}
	if err := inputs.CheckForDuplicatesAlways(); err != nil {
		t.Error(err)
	}
	if out, err := inputs.MarshalCBOR(); err != nil ||
		!bytes.Equal(out, raw) {
		t.Error("input set re-encoded differently")
	}
}

// TestStreamDecoderOverRFC8949Sequence reads a CBOR sequence of RFC 8949
// Appendix A items, whose lengths are known from the RFC, and checks every
// offset the stream decoder reports.
func TestStreamDecoderOverRFC8949Sequence(t *testing.T) {
	items := []string{
		"1903e8", "83010203", "a26161016162820203",
		"9f018202039f0405ffff", "f5", "c249010000000000000000",
	}
	var seq []byte
	starts := make([]int, 0, len(items))
	for _, h := range items {
		starts = append(starts, len(seq))
		seq = append(seq, mustHex(t, h)...)
	}
	sd, err := cbor.NewStreamDecoder(seq)
	if err != nil {
		t.Fatal(err)
	}
	var n uint64
	if off, l, err := sd.Decode(&n); err != nil || off != 0 || l != 3 ||
		n != 1000 {
		t.Fatalf("Decode: %d %d %d %v", off, l, n, err)
	}
	if off, l, err := sd.SkipN(2); err != nil || off != starts[1] ||
		l != starts[3]-starts[1] {
		t.Fatalf("SkipN: %d %d %v", off, l, err)
	}
	var raw cbor.RawMessage
	if off, b, err := sd.DecodeRaw(&raw); err != nil || off != starts[3] ||
		hex.EncodeToString(b) != items[3] {
		t.Fatalf("DecodeRaw: %d %x %v", off, b, err)
	}
	if err := sd.Advance(1); err != nil || sd.Position() != starts[5] {
		t.Fatalf("Advance: %d %v", sd.Position(), err)
	}
	if err := sd.Advance(len(seq)); err == nil {
		t.Error("Advance past the end accepted")
	}
	if err := sd.Advance(-1); err == nil {
		t.Error("negative Advance accepted")
	}
	if off, l, err := sd.Skip(); err != nil || off != starts[5] ||
		l != 11 || !sd.EOF() {
		t.Fatalf("Skip: %d %d %v", off, l, err)
	}
	if _, _, err := sd.Skip(); err == nil {
		t.Error("Skip at end of input accepted")
	}
	if off, l, err := sd.SkipN(0); err != nil || off != len(seq) || l != 0 {
		t.Errorf("SkipN(0): %d %d %v", off, l, err)
	}
}

// TestIndefLengthEncodersAsRFC8949 encodes RFC 8949 Appendix A's
// indefinite-length examples and checks the exact bytes the RFC gives.
func TestIndefLengthEncodersAsRFC8949(t *testing.T) {
	cases := []struct {
		value any
		want  string
	}{
		{cbor.IndefLengthList{}, "9fff"},
		{
			cbor.IndefLengthList{
				uint64(1),
				[]any{uint64(2), uint64(3)},
				cbor.IndefLengthList{uint64(4), uint64(5)},
			},
			"9f018202039f0405ffff",
		},
		{
			cbor.IndefLengthByteString{
				[]byte{0x01, 0x02}, []byte{0x03, 0x04, 0x05},
			},
			"5f42010243030405ff",
		},
		{
			cbor.IndefLengthMap{
				"a": uint64(1),
				"b": cbor.IndefLengthList{uint64(2), uint64(3)},
			},
			"bf61610161629f0203ffff",
		},
		{
			[]any{"a", cbor.IndefLengthMap{"b": "c"}},
			"826161bf61626163ff",
		},
		{
			[]any{
				uint64(1),
				[]any{uint64(2), uint64(3)},
				[]any{uint64(4), uint64(5)},
			},
			"8301820203820405",
		},
		{
			map[uint64]uint64{3: 4, 1: 2},
			"a201020304",
		},
		{uint64(18446744073709551615), "1bffffffffffffffff"},
		{int64(-1000), "3903e7"},
		{"水", "63e6b0b4"},
	}
	for _, c := range cases {
		got, err := cbor.Encode(c.value)
		if err != nil {
			t.Errorf("%s: %v", c.want, err)
			continue
		}
		if hex.EncodeToString(got) != c.want {
			t.Errorf("got %x, want %s", got, c.want)
		}
	}
}
