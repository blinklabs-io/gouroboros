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
	"bytes"
	"encoding/hex"
	"runtime"
	"strings"
	"testing"

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

func TestDecodeMetadatumRawOwnsInputBytes(t *testing.T) {
	data := []byte{0x81, 0x01}
	md, err := common.DecodeMetadatumRaw(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	data[0] = 0x00
	if got := md.Cbor(); !bytes.Equal(got, []byte{0x81, 0x01}) {
		t.Fatalf("metadata CBOR changed with input mutation: %x", got)
	}
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
	for _, depth := range []int{128, 200} {
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
// both the work and retained bytes stay bounded by the input size.
func TestDeeplyNestedMetadatumDecodesInLinearSpace(t *testing.T) {
	data := nestedListMetadatum(common.MaxMetadataNestedLevels)
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
//
// A nested metadatum map's duplicate keys are NOT rejected (see
// blinklabs-io/gouroboros#2323): upstream cardano-ledger's decodeMapN
// (libs/cardano-ledger-core/src/Cardano/Ledger/Metadata.hs) conses every pair
// unconditionally, at every era, so a duplicate key here must decode and keep
// every pair rather than error or silently drop one. wantPairs checks the
// count directly so a future regression that silently deduplicates instead of
// rejecting is also caught.
func TestMetadatumDecodeShapes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		cbor      string
		wantErr   bool
		wantPairs int // 0 means "don't check"
	}{
		{"indefinite list", "9f0102ff", false, 0},
		{"indefinite map", "bf0102ff", false, 0},
		{"indefinite bytes", "5f4201024103ff", false, 0},
		{"indefinite text", "7f61616162ff", false, 0},
		{"empty map", "a0", false, 0},
		{"empty list", "80", false, 0},
		{"max uint", "1bffffffffffffffff", false, 0},
		{"min negative", "3bffffffffffffffff", false, 0},
		{"container key", "a1810102", false, 0},
		{"mixed key types", "a2010061610a", false, 0},
		// decodeMetadatum in cardano-ledger accepts neither bignums nor any
		// other tag, nor simple values
		{"bignum", "c249010000000000000000", true, 0},
		{"null", "f6", true, 0},
		{"tag", "d81841ff", true, 0},
		{"duplicate key", "a20100016161", false, 2},
		{"duplicate key same value", "a201000100", false, 2},
		{"duplicate list key encodings", "a28101009f01ff00", false, 2},
		{"duplicate map key encodings", "a2a1010200bf0102ff00", false, 2},
		{"invalid UTF-8 text chunk", "7f42c3286161ff", true, 0},
		{"trailing data", "010101", true, 0},
		{"truncated", "81", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data, err := hex.DecodeString(tc.cbor)
			if err != nil {
				t.Fatalf("bad test hex: %v", err)
			}
			md, err := common.DecodeMetadatumRaw(data)
			if tc.wantErr && err == nil {
				t.Fatalf("expected an error for %s", tc.cbor)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %s: %v", tc.cbor, err)
			}
			if tc.wantPairs != 0 {
				mm, ok := md.(common.MetaMap)
				if !ok {
					t.Fatalf("expected MetaMap for %s, got %T", tc.cbor, md)
				}
				if len(mm.Pairs) != tc.wantPairs {
					t.Fatalf(
						"expected %d pair(s) for %s, got %d",
						tc.wantPairs,
						tc.cbor,
						len(mm.Pairs),
					)
				}
			}
		})
	}
}

// TestMetadatumNestingBound is the negative control for a direct decode of an
// isolated metadatum: the decoder recurses on the Go stack, so it holds the
// same bound the CBOR decode modes apply rather than none at all.
func TestMetadatumNestingBound(t *testing.T) {
	if _, err := common.DecodeMetadatumRaw(
		nestedListMetadatum(common.MaxMetadataNestedLevels),
	); err != nil {
		t.Fatalf("depth %d rejected: %v", common.MaxMetadataNestedLevels, err)
	}
	_, err := common.DecodeMetadatumRaw(
		nestedListMetadatum(common.MaxMetadataNestedLevels + 1),
	)
	if err == nil {
		t.Fatalf("expected depth %d to be rejected", common.MaxMetadataNestedLevels+1)
	}
	if !strings.Contains(err.Error(), "nesting exceeds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestMetadatumAcceptsIssue4351Vector is the exact minimal vector from
// blinklabs-io/dingo#4351: a Shelley-era auxiliary metadata value consisting
// of 1025 nested one-element lists around integer 0. Cardano's reference
// decoder places no nesting bound on transaction metadata, so it accepts
// this value; only gouroboros's own resource bound (MaxMetadataNestedLevels,
// which must recurse on the Go stack) can reject it. Before this fix that
// bound was 1024, a value with no derivation from anything the ledger
// actually enforces, so this exact input was rejected. It must now decode.
func TestMetadatumAcceptsIssue4351Vector(t *testing.T) {
	const issue4351Depth = 1025
	md, err := common.DecodeMetadatumRaw(nestedListMetadatum(issue4351Depth))
	if err != nil {
		t.Fatalf("depth %d rejected: %v", issue4351Depth, err)
	}
	depth := 0
	for {
		list, ok := md.(common.MetaList)
		if !ok || len(list.Items) != 1 {
			break
		}
		depth++
		md = list.Items[0]
	}
	if depth != issue4351Depth {
		t.Fatalf("decoded depth %d, want %d", depth, issue4351Depth)
	}
	if _, ok := md.(common.MetaInt); !ok {
		t.Fatalf("innermost value: unexpected %T", md)
	}
}

// TestMetadatumUsesConfiguredMaxNestedLevels proves MaxMetadataNestedLevels
// is a live var an application can raise or lower, matching the pattern
// cbor.MaxNestedLevels already established (blinklabs-io/gouroboros#2335):
// unlike that var, this one is read fresh on every decode rather than cached
// behind a sync.Once, so the change takes effect immediately, not only
// before the first Decode call ever made.
func TestMetadatumUsesConfiguredMaxNestedLevels(t *testing.T) {
	previous := common.MaxMetadataNestedLevels
	common.MaxMetadataNestedLevels = 2
	defer func() {
		common.MaxMetadataNestedLevels = previous
	}()

	if _, err := common.DecodeMetadatumRaw(nestedListMetadatum(2)); err != nil {
		t.Fatalf("depth 2 rejected at configured limit 2: %v", err)
	}
	_, err := common.DecodeMetadatumRaw(nestedListMetadatum(3))
	if err == nil {
		t.Fatalf("expected depth 3 to be rejected at configured limit 2")
	}
	if !strings.Contains(err.Error(), "nesting exceeds 2 levels") {
		t.Fatalf("unexpected error: %v", err)
	}
}
