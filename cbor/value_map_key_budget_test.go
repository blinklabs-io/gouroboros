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
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

// mapKeyBudgetArray encodes a definite array of count zero entries.
func mapKeyBudgetArray(count int) []byte {
	data := []byte{0x9a, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(data[1:], uint32(count))
	return append(data, make([]byte, count)...)
}

// mapKeyBudgetArrays encodes an array holding one array per count.
func mapKeyBudgetArrays(counts ...int) []byte {
	data := []byte{0x9a, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(data[1:], uint32(len(counts)))
	for _, count := range counts {
		data = append(data, mapKeyBudgetArray(count)...)
	}
	return data
}

// mapKeyBudgetMap encodes {key: 0}.
func mapKeyBudgetMap(key []byte) []byte {
	return append(append([]byte{0xa1}, key...), 0)
}

func TestValueMapKeyItemBudget(t *testing.T) {
	t.Parallel()
	half := MaxMapKeyItems / 2
	cases := map[string]struct {
		data   []byte
		reject bool
	}{
		"array key at budget": {
			data: mapKeyBudgetMap(mapKeyBudgetArray(MaxMapKeyItems)),
		},
		"array key over budget": {
			data:   mapKeyBudgetMap(mapKeyBudgetArray(MaxMapKeyItems + 1)),
			reject: true,
		},
		// Each inner array is under the budget; together with their own
		// headers they exceed it.
		"nested arrays over budget": {
			data:   mapKeyBudgetMap(mapKeyBudgetArrays(half, half)),
			reject: true,
		},
		// A map nested in the key spends the enclosing key's budget,
		// including the items of its own composite keys.
		"key within key over budget": {
			data: mapKeyBudgetMap(
				mapKeyBudgetMap(mapKeyBudgetArray(MaxMapKeyItems)),
			),
			reject: true,
		},
		"oversized array as value": {
			data: append(
				[]byte{0xa1, 0x00},
				mapKeyBudgetArray(4*MaxMapKeyItems)...,
			),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var value Value
			_, err := Decode(tc.data, &value)
			if tc.reject {
				require.ErrorIs(t, err, ErrMapKeyItemBudget)
				return
			}
			require.NoError(t, err)
		})
	}
}

// Every map in a decode opens its own budget for its keys, so the budget
// bounds one key rather than the whole document.
func TestValueMapKeyItemBudgetIsPerKey(t *testing.T) {
	t.Parallel()
	data := []byte{0xa2}
	data = append(data, mapKeyBudgetArray(MaxMapKeyItems)...)
	data = append(data, 0)
	data = append(data, mapKeyBudgetArrays(MaxMapKeyItems-1)...)
	data = append(data, 0)
	var value Value
	_, err := Decode(data, &value)
	require.NoError(t, err)
}

// Tag content is decoded by the library, so a tagged collection inside a key
// must be counted before it decodes.
func TestValueMapKeyItemBudgetCountsTagContent(t *testing.T) {
	t.Parallel()
	set := func(count int) []byte {
		return append([]byte{0xd9, 0x01, 0x02}, mapKeyBudgetArray(count)...)
	}
	var value Value
	_, err := Decode(mapKeyBudgetMap(set(MaxMapKeyItems)), &value)
	require.ErrorIs(t, err, ErrMapKeyItemBudget)
	_, err = Decode(mapKeyBudgetMap(set(MaxMapKeyItems-1)), &value)
	require.NoError(t, err)
}
