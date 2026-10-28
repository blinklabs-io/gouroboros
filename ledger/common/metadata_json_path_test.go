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

package common

import (
	"encoding/hex"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allocatedBytes reports the heap bytes allocated while running fn. TotalAlloc
// is cumulative, so collection during fn does not affect the delta.
func allocatedBytes(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

func pathNoSchemaArrayJSON(levels int) []byte {
	return []byte(
		`{"1":` + strings.Repeat("[", levels) + "1" +
			strings.Repeat("]", levels) + `}`,
	)
}

func pathNoSchemaObjectJSON(levels int) []byte {
	return []byte(
		`{"1":` + strings.Repeat(`{"member":`, levels) + `1` +
			strings.Repeat("}", levels) + `}`,
	)
}

func pathDetailedListJSON(levels int) []byte {
	return []byte(
		`{"1":` + strings.Repeat(`{"list":[`, levels) + `{"int":1}` +
			strings.Repeat("]}", levels) + `}`,
	)
}

func pathDetailedMapJSON(levels int) []byte {
	return []byte(
		`{"1":` + strings.Repeat(`{"map":[{"k":{"int":1},"v":`, levels) +
			`{"int":1}` + strings.Repeat("}]}", levels) + `}`,
	)
}

func TestMetadataJSONSuccessPathWorkIsLinearInDepth(t *testing.T) {
	// Not t.Parallel: measures process-wide heap allocation (TotalAlloc),
	// which concurrent tests would inflate.
	cases := []struct {
		name  string
		parse func([]byte) (TransactionMetadatum, error)
		build func(int) []byte
		// small is the shallow depth; the deep one is 8x it. Bases are
		// chosen so the deep document opens 960 containers, under
		// MetadataJSONMaxNestingDepth: a detailed list level opens two and a
		// detailed map level three.
		small int
	}{
		{"no schema array", ParseCardanoCLIMetadataJSONNoSchema,
			pathNoSchemaArrayJSON, 120},
		{"no schema object", ParseCardanoCLIMetadataJSONNoSchema,
			pathNoSchemaObjectJSON, 120},
		{"detailed list", ParseCardanoCLIMetadataJSONDetailedSchema,
			pathDetailedListJSON, 60},
		{"detailed map", ParseCardanoCLIMetadataJSONDetailedSchema,
			pathDetailedMapJSON, 40},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			small, large := tc.small, tc.small*8
			smallInput, largeInput := tc.build(small), tc.build(large)
			measure := func(input []byte) uint64 {
				best := ^uint64(0)
				for range 5 {
					b := allocatedBytes(func() {
						_, err := tc.parse(input)
						require.NoError(t, err)
					})
					best = min(best, b)
				}
				return best
			}
			smallBytes, largeBytes := measure(smallInput), measure(largeInput)
			ratio := float64(largeBytes) / float64(smallBytes)
			// 8x the depth is ~8x the bytes when linear and approaches 64x
			// when each level copies the whole path. The bound sits between
			// so neither allocator drift nor pointer width decides it.
			assert.Less(
				t,
				ratio,
				12.0,
				"allocated %d bytes at depth %d and %d at depth %d",
				smallBytes, small, largeBytes, large,
			)
		})
	}
}

func TestMetadataJSONErrorPathsAreUnchanged(t *testing.T) {
	t.Parallel()
	noSchema := func(prefix, suffix, leaf string) []byte {
		return []byte(`{"7":` + prefix + leaf + suffix + `}`)
	}
	cases := []struct {
		name     string
		parse    func([]byte) (TransactionMetadatum, error)
		input    []byte
		expected string
		wraps    error
	}{
		{
			name:  "no schema nested arrays and objects",
			parse: ParseCardanoCLIMetadataJSONNoSchema,
			input: noSchema(`[1,{"a":[2,`, `]}]`, `null`),
			expected: "metadata label 7: $.7[1].a[1]: " +
				"null is not allowed in metadata JSON",
		},
		{
			name:  "no schema bool",
			parse: ParseCardanoCLIMetadataJSONNoSchema,
			input: noSchema(`{"x":[`, `]}`, `true`),
			expected: "metadata label 7: $.7.x[0]: " +
				"bool is not allowed in metadata JSON",
		},
		{
			name:  "no schema float",
			parse: ParseCardanoCLIMetadataJSONNoSchema,
			input: noSchema(`[[`, `]]`, `1.5`),
			expected: "metadata label 7: $.7[0][0]: " +
				`metadata JSON number "1.5" is not an integer`,
		},
		{
			name:  "detailed list element",
			parse: ParseCardanoCLIMetadataJSONDetailedSchema,
			input: []byte(`{"7":{"list":[{"int":1},{"list":[{"int":"x"}]}]}}`),
			expected: "metadata label 7: $.7.list[1].list[0].int: " +
				"expected JSON number",
		},
		{
			name:  "detailed map value",
			parse: ParseCardanoCLIMetadataJSONDetailedSchema,
			input: []byte(
				`{"7":{"list":[{"map":[{"k":{"int":1},"v":{"bytes":"abc"}}]}]}}`,
			),
			expected: "metadata label 7: $.7.list[0].map[0].v.bytes: " +
				"invalid hex: encoding/hex: odd length hex string",
			wraps: hex.ErrLength,
		},
		{
			name:  "detailed map key position",
			parse: ParseCardanoCLIMetadataJSONDetailedSchema,
			input: []byte(
				`{"7":{"map":[{"k":{"string":1},"v":{"int":1}}]}}`,
			),
			expected: "metadata label 7: $.7.map[0].k.string: " +
				"expected JSON string",
		},
		{
			name:  "detailed map entry shape",
			parse: ParseCardanoCLIMetadataJSONDetailedSchema,
			input: []byte(
				`{"7":{"list":[{"map":[1]}]}}`,
			),
			expected: "metadata label 7: $.7.list[0].map[0]: " +
				"map entry must be an object",
		},
		{
			name:  "no schema key containing a format verb",
			parse: ParseCardanoCLIMetadataJSONNoSchema,
			input: []byte(`{"7":{"%s%d":null}}`),
			expected: "metadata label 7: $.7.%s%d: " +
				"null is not allowed in metadata JSON",
		},
		{
			name:  "detailed unknown field",
			parse: ParseCardanoCLIMetadataJSONDetailedSchema,
			input: []byte(`{"7":{"list":[{"nope":1}]}}`),
			expected: "metadata label 7: $.7.list[0]: " +
				`unknown detailed metadatum field "nope"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tc.parse(tc.input)
			require.EqualError(t, err, tc.expected)
			if tc.wraps != nil {
				assert.ErrorIs(t, err, tc.wraps)
			}
		})
	}
}
