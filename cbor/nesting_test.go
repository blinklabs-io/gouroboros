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
	"os"
	"os/exec"
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

type unmarshalProbe struct {
	called bool
}

func (p *unmarshalProbe) UnmarshalCBOR([]byte) error {
	p.called = true
	return nil
}

type recursiveUnmarshalProbe struct {
	calls int
}

func (p *recursiveUnmarshalProbe) UnmarshalCBOR(data []byte) error {
	p.calls++
	if len(data) == 0 || data[0] != 0x81 {
		return nil
	}
	_, err := cbor.Decode(data[1:], p)
	return err
}

func TestDecodeExactChecksDepthBeforeCustomUnmarshal(t *testing.T) {
	const childEnv = "GOUROBOROS_TEST_DECODE_EXACT_DEPTH_CHILD"
	if os.Getenv(childEnv) != "1" {
		// The cached decoder mode keeps the depth limit from the first decode.
		cmd := exec.Command(os.Args[0], "-test.run=^TestDecodeExactChecksDepthBeforeCustomUnmarshal$")
		cmd.Env = append(os.Environ(), childEnv+"=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("depth-check subprocess failed: %v\n%s", err, output)
		}
		return
	}

	cbor.MaxUntrustedNestedLevels = 8
	data := nestedArrays(cbor.MaxUntrustedNestedLevels + 1)
	var dest recursiveUnmarshalProbe
	if _, err := cbor.DecodeExact(data, &dest); err == nil {
		t.Fatalf("accepted nesting depth %d", cbor.MaxUntrustedNestedLevels+1)
	}
	if dest.calls != 0 {
		t.Fatalf("custom UnmarshalCBOR ran %d times before the nesting limit was checked", dest.calls)
	}
}

func TestDecodeExactChecksCollectionBoundsBeforeCustomUnmarshal(t *testing.T) {
	data, err := cbor.Encode(make([]any, 131073))
	if err != nil {
		t.Fatal(err)
	}
	dest := &unmarshalProbe{}
	if _, err := cbor.DecodeExact(data, dest); err == nil {
		t.Fatal("accepted an array exceeding the untrusted collection limit")
	}
	if dest.called {
		t.Fatal("custom decoder ran before the collection bound was checked")
	}
}

// TestDecodeRejectsNestingPastConfiguredLimit keeps all public decode modes
// behind the stack-safety bound. These paths process peer-controlled payloads
// and custom UnmarshalCBOR implementations can recurse on the Go stack.
func TestDecodeRejectsNestingPastConfiguredLimit(t *testing.T) {
	data := nestedArrays(cbor.MaxNestedLevels + 1)
	for _, tc := range []struct {
		name   string
		decode func([]byte, any) (int, error)
	}{
		{"Decode", cbor.Decode},
		{"DecodeStrict", cbor.DecodeStrict},
		{"DecodeLenient", cbor.DecodeLenient},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var dest cbor.Value
			if _, err := tc.decode(data, &dest); err == nil {
				t.Fatalf("accepted nesting depth %d", cbor.MaxNestedLevels+1)
			}
		})
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

func TestDecodeStrictAcceptsDeepTransactionNesting(t *testing.T) {
	var value cbor.Value
	if _, err := cbor.DecodeStrict(nestedArrays(257), &value); err != nil {
		t.Fatalf("DecodeStrict rejected valid deep transaction nesting: %v", err)
	}
}
