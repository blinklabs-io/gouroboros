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

package byron

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func TestCborItemEndNestingLimit(t *testing.T) {
	testCases := map[string]func(int) []byte{
		"arrays": func(depth int) []byte {
			return append(bytes.Repeat([]byte{0x81}, depth), 0x00)
		},
		"maps": func(depth int) []byte {
			ret := bytes.Repeat([]byte{0xa1, 0x00}, depth)
			return append(ret, 0x00)
		},
		"tags": func(depth int) []byte {
			return append(bytes.Repeat([]byte{0xc0}, depth), 0x00)
		},
	}
	for name, nestedItem := range testCases {
		t.Run(name, func(t *testing.T) {
			atLimit := nestedItem(cbor.MaxNestedLevels)
			end, err := cborItemEnd(atLimit, 0)
			require.NoError(t, err)
			require.Equal(t, len(atLimit), end)

			tooDeep := nestedItem(cbor.MaxNestedLevels + 1)
			_, err = cborItemEnd(tooDeep, 0)
			require.ErrorContains(t, err, "CBOR nesting exceeds")
		})
	}
}

func TestCborItemEndRejectsInvalidStringChunksWithoutTraversal(t *testing.T) {
	for _, major := range []byte{0x40, 0x60} {
		t.Run("wrong major type", func(t *testing.T) {
			wrong := byte(0x40 ^ 0x60 ^ major)
			_, err := cborItemEnd([]byte{major | 31, wrong | 1, 0x00, 0xff}, 0)
			require.ErrorContains(t, err, "invalid indefinite string chunk")
		})
		t.Run("nested indefinite chunk", func(t *testing.T) {
			_, err := cborItemEnd(
				[]byte{major | 31, major | 31, major | 1, 0x00, 0xff, 0xff},
				0,
			)
			require.ErrorContains(t, err, "invalid indefinite string chunk")
		})
		t.Run("non-string chunk", func(t *testing.T) {
			_, err := cborItemEnd([]byte{major | 31, 0x81, 0x00, 0xff}, 0)
			require.ErrorContains(t, err, "invalid indefinite string chunk")
		})
		t.Run("deeply nested indefinite chunks are not traversed", func(t *testing.T) {
			// Indefinite string nesting consumes no depth budget, so recursing into
			// each chunk would cost one frame and one error per nesting level.
			raw := bytes.Repeat([]byte{major | 31}, 100_000)
			allocs := testing.AllocsPerRun(5, func() {
				_, _ = cborItemEnd(raw, 0)
			})
			require.Less(t, allocs, 100.0)
			_, err := cborItemEnd(raw, 0)
			require.ErrorContains(t, err, "invalid indefinite string chunk")
		})
	}
}
