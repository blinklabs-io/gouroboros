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
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// nestedArrays builds the CBOR encoding of depth nested single-element arrays
// wrapping a zero, which is the shape the metadatum CDDL rule
// "metadatum = ... / [* metadatum] / int / ..." permits without bound
// (cardano-ledger eras/conway/impl/cddl/data/conway.cddl).
func nestedArrays(depth int) []byte {
	out := make([]byte, 0, depth+1)
	for range depth {
		out = append(out, 0x81)
	}
	return append(out, 0x00)
}

// TestDecodeAcceptsNestingPastPreviousCap covers nesting depths that the
// previous fixed cap of 256 rejected. The reference decoders carry no depth
// counter, so every depth a transaction can encode has to decode.
func TestDecodeAcceptsNestingPastPreviousCap(t *testing.T) {
	// 16384 is the current mainnet max_tx_size, and therefore the deepest
	// structure a transaction can carry, since a nesting level costs at
	// least one byte.
	for _, depth := range []int{257, 300, 1000, 16384, 65535} {
		data := nestedArrays(depth)
		for _, tc := range []struct {
			name   string
			decode func([]byte, any) (int, error)
		}{
			{"Decode", cbor.Decode},
			{"DecodeStrict", cbor.DecodeStrict},
			{"DecodeLenient", cbor.DecodeLenient},
		} {
			var dest cbor.Value
			if _, err := tc.decode(data, &dest); err != nil {
				t.Errorf(
					"%s rejected nesting depth %d: %v",
					tc.name,
					depth,
					err,
				)
			}
		}
	}
}

// TestDecodeRejectsNestingPastLibraryMaximum is the negative control: the cap
// is raised to the maximum the CBOR library supports, not removed, so a value
// past it is still rejected rather than being decoded or overflowing the
// stack.
func TestDecodeRejectsNestingPastLibraryMaximum(t *testing.T) {
	data := nestedArrays(65536)
	var dest cbor.Value
	_, err := cbor.Decode(data, &dest)
	if err == nil {
		t.Fatal("expected nesting depth 65536 to be rejected")
	}
	if !strings.Contains(err.Error(), "max nested level") {
		t.Fatalf("unexpected error for nesting depth 65536: %v", err)
	}
}
