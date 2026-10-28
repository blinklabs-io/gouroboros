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

package localstatequery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Not t.Parallel: testing.AllocsPerRun measures process-wide allocation.
func TestTypedResultsDoNotAllocateForDeclaredListItems(t *testing.T) {
	// One-byte items behind a uint32 list header, and the same items in an
	// indefinite list. The shape check on each type must count them without
	// building a slice of them.
	const items = 100_000
	inputs := map[string][]byte{
		"definite": append(
			[]byte{0x9a, 0x00, 0x01, 0x86, 0xa0},
			make([]byte, items)...,
		),
		"indefinite": append(
			append([]byte{0x9f}, make([]byte, items)...),
			0xff,
		),
	}
	tests := []struct {
		name   string
		decode func([]byte) error
	}{
		{"WithOriginSlot", func(d []byte) error {
			var v WithOriginSlot
			return v.UnmarshalCBOR(d)
		}},
		{"NextEpochChangeValue", func(d []byte) error {
			var v NextEpochChangeValue
			return v.UnmarshalCBOR(d)
		}},
		{"HotCredAuthStatusValue", func(d []byte) error {
			var v HotCredAuthStatusValue
			return v.UnmarshalCBOR(d)
		}},
		{"UtxoId", func(d []byte) error {
			var v UtxoId
			return v.UnmarshalCBOR(d)
		}},
	}
	for _, tt := range tests {
		for inputName, input := range inputs {
			t.Run(tt.name+"/"+inputName, func(t *testing.T) {
				assert.Error(t, tt.decode(input))
				allocs := testing.AllocsPerRun(3, func() {
					_ = tt.decode(input)
				})
				// The error path allocates a few small values;
				// allocating per item is orders of magnitude more.
				assert.Less(t, allocs, float64(50),
					"allocations scale with list items")
			})
		}
	}
}
