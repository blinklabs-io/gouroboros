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

// TestDecodeAcceptsNestingPastPreviousCap covers safe recursive parsing depths
// beyond the previous fixed cap of 256.
func TestDecodeAcceptsNestingPastPreviousCap(t *testing.T) {
	// 16384 is the current mainnet max_tx_size, and therefore the deepest
	// structure a transaction can carry, since a nesting level costs at
	// least one byte.
	for _, depth := range []int{257, 300, 1000} {
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

func TestDecodeAcceptsNestingAtConfiguredLimit(t *testing.T) {
	var dest any
	if _, err := cbor.Decode(nestedArrays(cbor.MaxNestedLevels), &dest); err != nil {
		t.Fatalf("Decode rejected configured nesting limit: %v", err)
	}
	var value cbor.Value
	if _, err := cbor.Decode(nestedArrays(cbor.MaxNestedLevels), &value); err != nil {
		t.Fatalf("Decode rejected Value at configured nesting limit: %v", err)
	}
}

func TestDecodeRejectsNestingPastConfiguredLimit(t *testing.T) {
	data := nestedArrays(cbor.MaxNestedLevels + 1)
	var dest any
	_, err := cbor.Decode(data, &dest)
	if err == nil {
		t.Fatalf("expected nesting depth %d to be rejected", cbor.MaxNestedLevels+1)
	}
	if !strings.Contains(err.Error(), "max nested level") {
		t.Fatalf("unexpected error: %v", err)
	}
	var value cbor.Value
	if _, err := cbor.Decode(data, &value); err == nil {
		t.Fatal("expected Value to reject nesting past configured limit")
	}
}
