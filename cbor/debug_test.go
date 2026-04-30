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

package cbor

import (
	"strings"
	"testing"

	_cbor "github.com/fxamacker/cbor/v2"
)

func TestDumpCborStructureMaxDepthNoPanic(t *testing.T) {
	depth := cborMaxNestedLevels + 100
	cborData := deeplyNestedArrayCbor(depth)

	decMode, err := (_cbor.DecOptions{
		MaxNestedLevels: depth + 10,
	}).DecMode()
	if err != nil {
		t.Fatalf("failed creating decoder mode: %v", err)
	}

	var decoded any
	if err := decMode.Unmarshal(cborData, &decoded); err != nil {
		t.Fatalf("failed decoding deeply nested CBOR: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("DumpCborStructure panicked: %v", r)
		}
	}()

	out := DumpCborStructure(decoded, "", cborMaxNestedLevels)
	if !strings.Contains(out, "...\n") {
		t.Fatalf("expected depth-limit placeholder in output, got: %q", out)
	}
}

func deeplyNestedArrayCbor(depth int) []byte {
	// Repeating [ (0x81) around a final empty array [] (0x80)
	ret := make([]byte, depth+1)
	for i := 0; i < depth; i++ {
		ret[i] = 0x81
	}
	ret[len(ret)-1] = 0x80
	return ret
}
