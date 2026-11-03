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
	"runtime"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func decodeValueJSON(t *testing.T, cborHex string) string {
	t.Helper()
	data, err := hex.DecodeString(cborHex)
	require.NoError(t, err)
	var v cbor.Value
	require.NoError(t, v.UnmarshalCBOR(data))
	out, err := v.MarshalJSON()
	require.NoError(t, err)
	return string(out)
}

func TestValueMarshalJSONAstOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cborHex string
		wantAst string
	}{
		{"uint", "0a", `{"int":10}`},
		{"negative", "20", `{"int":-1}`},
		{"bignum", "c249010000000000000000",
			`{"int":18446744073709551616}`},
		{"bytes", "42dead", `{"bytes":"dead"}`},
		{"empty bytes", "40", `{"bytes":""}`},
		{"string", "63616263", `{"string":"abc"}`},
		{"escapable string", "693c613e26e280a8225c",
			`{"string":"` + "\\u003ca\\u003e\\u0026\\u2028" + `\"\\"}`},
		{"bool", "f5", `{"bool":true}`},
		{"list", "8301820203f4",
			`{"list":[{"int":1},{"list":[{"int":2},{"int":3}]},` +
				`{"bool":false}]}`},
		{"empty list", "80", `{"list":[]}`},
		{"set", "d9010282010a",
			`{"list":[{"int":1},{"int":10}]}`},
		{"empty map", "a0", `{"map":[]}`},
		{"map keys sorted by rendering", "a3" + "02" + "01" + "01" + "02" +
			"4161" + "03",
			`{"map":[{"k":{"bytes":"61"},"v":{"int":3}},` +
				`{"k":{"int":1},"v":{"int":2}},` +
				`{"k":{"int":2},"v":{"int":1}}]}`},
		{"keys that are digit prefixes", "a301010a02186403",
			`{"map":[{"k":{"int":100},"v":{"int":3}},` +
				`{"k":{"int":10},"v":{"int":2}},` +
				`{"k":{"int":1},"v":{"int":1}}]}`},
		{"nested maps", "a101a102a1034161",
			`{"map":[{"k":{"int":1},"v":{"map":[{"k":{"int":2},` +
				`"v":{"map":[{"k":{"int":3},"v":{"bytes":"61"}}]}}]}}]}`},
		{"rat", "d81e820103",
			`{"list":[{"int":1},{"int":3}]}`},
		{"constructor", "d8798201426162",
			`{"constructor":0,"fields":[{"int":1},{"bytes":"6162"}]}`},
		{"nested constructor", "d87a81d87981a1010a",
			`{"constructor":1,"fields":[{"constructor":0,"fields":` +
				`[{"map":[{"k":{"int":1},"v":{"int":10}}]}]}]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := decodeValueJSON(t, tc.cborHex)
			want := `{"cbor":"` + tc.cborHex + `","json":` + tc.wantAst + `}`
			require.Equal(t, want, got)
		})
	}
}

// Not t.Parallel: measures process-wide allocation via runtime.MemStats.
func TestValueMarshalJSONDeepNestingAllocation(t *testing.T) {
	const (
		depth    = 1000
		leafSize = 16 * 1024
	)
	// depth nested single-element arrays around one byte string leaf.
	var data []byte
	for range depth {
		data = append(data, 0x81)
	}
	data = append(data, 0x59, byte(leafSize>>8), byte(leafSize&0xff))
	data = append(data, make([]byte, leafSize)...)
	var v cbor.Value
	require.NoError(t, v.UnmarshalCBOR(data))

	marshal := func() int {
		out, err := v.MarshalJSON()
		require.NoError(t, err)
		return len(out)
	}
	outLen := marshal()
	require.Greater(t, outLen, 2*leafSize)

	const runs = 5
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for range runs {
		marshal()
	}
	runtime.ReadMemStats(&after)
	perRun := (after.TotalAlloc - before.TotalAlloc) / runs

	// Rendering into one shared buffer allocates a small multiple of the
	// output. Copying each child's output at every level allocates
	// roughly depth times the leaf.
	require.Less(
		t,
		perRun,
		uint64(8*outLen),
		"allocated %d bytes per run for %d bytes of output",
		perRun,
		outLen,
	)
}
