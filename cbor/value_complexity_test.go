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
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
)

func nestedSingletonMapKey(depth int) []byte {
	data := make([]byte, 0, depth*2+1)
	for range depth {
		data = append(data, 0xa1)
	}
	data = append(data, 0)
	for range depth {
		data = append(data, 0)
	}
	return data
}

func TestValueCompositeMapKeyDecodeHasBoundedAllocations(t *testing.T) {
	allocations := func(depth int) float64 {
		data := nestedSingletonMapKey(depth)
		return testing.AllocsPerRun(10, func() {
			var value Value
			decoded, err := Decode(data, &value)
			require.NoError(t, err)
			require.Equal(t, len(data), decoded)
		})
	}

	shallow := allocations(24)
	deep := allocations(48)
	require.Less(
		t,
		deep,
		shallow*3,
		"doubling nesting must not approach quadratic allocation growth",
	)
}

func TestValueCompositeMapKeyAtConfiguredNestingLimit(t *testing.T) {
	data := nestedSingletonMapKey(MaxNestedLevels)
	var value Value
	decoded, err := Decode(data, &value)
	require.NoError(t, err)
	require.Equal(t, len(data), decoded)
}

func TestValueIncrementalMapKeyDigestPreservesIdentity(t *testing.T) {
	decMode, err := getLenientDecMode()
	require.NoError(t, err)
	require.NotNil(t, decMode)

	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "nested maps",
			data: nestedSingletonMapKey(64),
		},
		{
			name: "array",
			data: []byte{0x83, 0x01, 0x82, 0x02, 0x03, 0x04},
		},
		{
			name: "duplicate map key",
			data: []byte{0xa2, 0x01, 0x00, 0x01, 0x02},
		},
		{
			name: "unhashable map key",
			data: []byte{0xa1, 0x81, 0x01, 0x02},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				value  Value
				digest [sha256.Size]byte
			)
			decoded, err := value.unmarshalCBOR(
				tt.data,
				false,
				&digest,
				0,
				keepLastDuplicateMapKey,
				decMode,
			)
			require.NoError(t, err)
			require.Equal(t, len(tt.data), decoded)
			require.Equal(t, mapKeyDigest(value.Value()), digest)
		})
	}
}
