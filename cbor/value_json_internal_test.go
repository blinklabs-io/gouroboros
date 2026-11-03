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
	"math/big"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValueMarshalJSONMapTiesAndBigInts(t *testing.T) {
	t.Parallel()
	// int 1 and uint64 1 are distinct Go map keys that render identically,
	// so both entries must survive the sort.
	v := Value{value: Map{
		int(1):    big.NewInt(7),
		uint64(1): *big.NewInt(-8),
		"k":       true,
		"<&>":     (*big.Int)(nil),
	}}
	out, err := v.MarshalJSON()
	require.NoError(t, err)
	require.Equal(
		t,
		`{"cbor":"","json":{"map":[`+
			`{"k":{"int":1},"v":{"int":-8}},`+
			`{"k":{"int":1},"v":{"int":7}},`+
			`{"k":{"string":"`+"\\u003c\\u0026\\u003e"+`"},"v":{"int":0}},`+
			`{"k":{"string":"k"},"v":{"bool":true}}]}}`,
		string(out),
	)
}

// Not t.Parallel: measures process-wide allocation via runtime.MemStats.
func TestAppendAstJsonMapNestingDoesNotMoveRenderedValues(t *testing.T) {
	const (
		depth    = 1000
		leafSize = 128 * 1024
	)
	// Each level is a two-entry map: a small sibling under key 1 and the
	// large child under key 2. Levels alternate which entry is written
	// first, so the iteration order of the decoded maps is out of rendered
	// order at about half the levels whichever way the runtime iterates.
	var prefix, suffix []byte
	for i := range depth {
		if i%2 == 0 {
			prefix = append(prefix, 0xa2, 0x02)
			suffix = append([]byte{0x01, 0x00}, suffix...)
		} else {
			prefix = append(prefix, 0xa2, 0x01, 0x00, 0x02)
		}
	}
	data := append([]byte{}, prefix...)
	data = append(data, 0x5a, 0x00, byte(leafSize>>16&0xff),
		byte(leafSize>>8&0xff), byte(leafSize&0xff))
	data = append(data, make([]byte, leafSize)...)
	data = append(data, suffix...)
	var v Value
	require.NoError(t, v.UnmarshalCBOR(data))

	first, err := appendAstJson(nil, v.value)
	require.NoError(t, err)
	outLen := len(first)
	require.Greater(t, outLen, 2*leafSize)

	// The destination is sized to the output, so rendering that only
	// appends in place never reallocates it. Moving a rendered subtree to
	// reorder entries appends past the capacity and reallocates.
	const runs = 5
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for range runs {
		out, err := appendAstJson(make([]byte, 0, outLen), v.value)
		require.NoError(t, err)
		require.Len(t, out, outLen)
	}
	runtime.ReadMemStats(&after)
	perRun := (after.TotalAlloc - before.TotalAlloc) / runs

	require.Less(
		t,
		perRun,
		uint64(4*outLen),
		"allocated %d bytes per run for %d bytes of output",
		perRun,
		outLen,
	)
}
