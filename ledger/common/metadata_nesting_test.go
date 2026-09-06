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

package common_test

import (
	"encoding/hex"
	"runtime"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// nestedListMetadatum builds depth nested single-element lists around a zero.
func nestedListMetadatum(depth int) []byte {
	out := make([]byte, 0, depth+1)
	for range depth {
		out = append(out, 0x81)
	}
	return append(out, 0x00)
}

// nestedMapMetadatum builds depth nested single-entry maps keyed by zero,
// around a zero.
func nestedMapMetadatum(depth int) []byte {
	out := make([]byte, 0, 2*depth+1)
	for range depth {
		out = append(out, 0xa1, 0x00)
	}
	return append(out, 0x00)
}

func TestDeeplyNestedMetadatumDecodes(t *testing.T) {
	for _, depth := range []int{257, 1000, 16000} {
		list, err := common.DecodeMetadatumRaw(nestedListMetadatum(depth))
		if err != nil {
			t.Fatalf("nested lists at depth %d: %v", depth, err)
		}
		for i := range depth {
			inner, ok := list.(common.MetaList)
			if !ok || len(inner.Items) != 1 {
				t.Fatalf("list level %d: unexpected %T", i, list)
			}
			list = inner.Items[0]
		}
		if _, ok := list.(common.MetaInt); !ok {
			t.Fatalf("innermost list value: unexpected %T", list)
		}

		mapMd, err := common.DecodeMetadatumRaw(nestedMapMetadatum(depth))
		if err != nil {
			t.Fatalf("nested maps at depth %d: %v", depth, err)
		}
		for i := range depth {
			inner, ok := mapMd.(common.MetaMap)
			if !ok || len(inner.Pairs) != 1 {
				t.Fatalf("map level %d: unexpected %T", i, mapMd)
			}
			mapMd = inner.Pairs[0].Value
		}
		if _, ok := mapMd.(common.MetaInt); !ok {
			t.Fatalf("innermost map value: unexpected %T", mapMd)
		}
	}
}

// TestDeeplyNestedMetadatumDecodesInLinearSpace pins the decode to one pass
// over the input. Re-entering the CBOR library once per nesting level made
// both the work and the retained bytes grow with the product of size and
// depth: a 16001 byte value allocated 581 MiB.
func TestDeeplyNestedMetadatumDecodesInLinearSpace(t *testing.T) {
	data := nestedListMetadatum(16000)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if _, err := common.DecodeMetadatumRaw(data); err != nil {
		t.Fatalf("decode: %v", err)
	}
	runtime.ReadMemStats(&after)
	const limit = 64 << 20
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > limit {
		t.Fatalf(
			"decoding a %d byte metadatum allocated %d bytes, over the %d byte limit",
			len(data),
			allocated,
			limit,
		)
	}
}

// TestMetadatumDecodeShapes covers the shapes the single-pass decoder has to
// handle directly, including the ones the reference rejects.
func TestMetadatumDecodeShapes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cbor    string
		wantErr bool
	}{
		{"indefinite list", "9f0102ff", false},
		{"indefinite map", "bf0102ff", false},
		{"indefinite bytes", "5f4201024103ff", false},
		{"indefinite text", "7f61616162ff", false},
		{"empty map", "a0", false},
		{"empty list", "80", false},
		{"max uint", "1bffffffffffffffff", false},
		{"min negative", "3bffffffffffffffff", false},
		{"container key", "a1810102", false},
		{"mixed key types", "a2010061610a", false},
		// decodeMetadatum in cardano-ledger accepts neither bignums nor any
		// other tag, nor simple values
		{"bignum", "c249010000000000000000", true},
		{"null", "f6", true},
		{"tag", "d81841ff", true},
		{"duplicate key", "a20100016161", true},
		{"duplicate key same value", "a201000100", true},
		{"trailing data", "010101", true},
		{"truncated", "8101", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := hex.DecodeString(tc.cbor)
			if err != nil {
				t.Fatalf("bad test hex: %v", err)
			}
			_, err = common.DecodeMetadatumRaw(data)
			if tc.wantErr && err == nil {
				t.Fatalf("expected an error for %s", tc.cbor)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %s: %v", tc.cbor, err)
			}
		})
	}
}

// TestMetadatumNestingBound is the negative control for a direct decode of an
// isolated metadatum: the decoder recurses on the Go stack, so it holds the
// same bound the CBOR decode modes apply rather than none at all.
func TestMetadatumNestingBound(t *testing.T) {
	if _, err := common.DecodeMetadatumRaw(
		nestedListMetadatum(cbor.MaxNestedLevels),
	); err != nil {
		t.Fatalf("depth %d rejected: %v", cbor.MaxNestedLevels, err)
	}
	_, err := common.DecodeMetadatumRaw(
		nestedListMetadatum(cbor.MaxNestedLevels + 1),
	)
	if err == nil {
		t.Fatalf("expected depth %d to be rejected", cbor.MaxNestedLevels+1)
	}
	if !strings.Contains(err.Error(), "nesting exceeds") {
		t.Fatalf("unexpected error: %v", err)
	}
}
