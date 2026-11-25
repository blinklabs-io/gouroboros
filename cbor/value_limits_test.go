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
	"bytes"
	"crypto/sha256"
	"testing"

	_cbor "github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func TestValueHonorsSelectedModeLimits(t *testing.T) {
	t.Parallel()
	mode, err := (_cbor.DecOptions{
		MaxNestedLevels: 4, MaxArrayElements: 16, MaxMapPairs: 16,
	}).DecMode()
	require.NoError(t, err)
	array := append([]byte{0x91}, make([]byte, 17)...)
	indefArray := append([]byte{0x9f}, make([]byte, 17)...)
	indefArray = append(indefArray, 0xff)
	mapData := []byte{0xb1}
	for i := range 17 {
		mapData = append(mapData, byte(i), 0)
	}
	indefMap := append([]byte{0xbf}, mapData[1:]...)
	indefMap = append(indefMap, 0xff)
	for name, data := range map[string][]byte{
		"array": array, "indefinite array": indefArray,
		"map": mapData, "indefinite map": indefMap,
		"nesting": append(bytes.Repeat([]byte{0x81}, 5), 0),
	} {
		t.Run(name, func(t *testing.T) {
			var value Value
			_, err := decodeWithMode(data, &value, mode, rejectDuplicateMapKeys)
			require.Error(
				t,
				err,
				"custom Value must enforce selected mode limits",
			)
		})
	}
	for _, data := range [][]byte{
		append([]byte{0x90}, make([]byte, 16)...),
		append(bytes.Repeat([]byte{0x81}, 4), 0),
	} {
		var value Value
		_, err := decodeWithMode(data, &value, mode, rejectDuplicateMapKeys)
		require.NoError(t, err)
	}
}

func TestValueStrictLimitPreservesLargeLedgerArray(t *testing.T) {
	t.Parallel()
	const count = 131073
	data := append([]byte{0x9a, 0, 2, 0, 1}, make([]byte, count)...)
	var strict Value
	_, err := DecodeStrict(data, &strict)
	require.Error(t, err, "strict mode must reject its first excessive element")
	var ledger Value
	consumed, err := Decode(data, &ledger)
	require.NoError(t, err)
	require.Equal(t, len(data), consumed)
	require.Len(t, ledger.Value(), count)
}

func TestValuePrefixRetainsOnlyConsumedBytes(t *testing.T) {
	first := []byte{0x81, 0}
	tail := append([]byte{0x5a, 0, 0x20, 0, 0}, make([]byte, 2*1024*1024)...)
	data := append(append([]byte{}, first...), tail...)
	var value Value
	consumed, err := Decode(data, &value)
	require.NoError(t, err)
	require.Equal(t, len(first), consumed)
	encoded, err := value.MarshalCBOR()
	require.NoError(t, err)
	require.Equal(t, len(first), len(encoded))
	require.Equal(t, first, encoded)
	var direct Value
	require.NoError(t, direct.UnmarshalCBOR(data))
	require.Equal(t, first, direct.Cbor())
	var second []byte
	consumedSecond, err := Decode(data[consumed:], &second)
	require.NoError(t, err)
	require.Equal(t, len(tail), consumedSecond)
	require.Len(t, second, 2*1024*1024)
}

func TestValuePrefixAllocationIgnoresUnconsumedTail(t *testing.T) {
	data := append([]byte{0x81, 0}, make([]byte, 2*1024*1024)...)
	result := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			var value Value
			_, err := Decode(data, &value)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	t.Logf(
		"two consumed bytes, %d unconsumed bytes: %d allocated bytes/op",
		len(data)-2,
		result.AllocedBytesPerOp(),
	)
	require.Less(t, result.AllocedBytesPerOp(), int64(32*1024))
}

// A map-key array is digested element by element, so its digest buffer must
// grow with the elements that decode rather than with its declared length.
func TestValueMapKeyArrayDigestsGrowAsEntriesDecode(t *testing.T) {
	const count = 100_000
	// {[{0: 0, 0: 0}, 0, ...]: 0} is well formed, so it passes the selected
	// mode's checks, but the duplicate key fails the first entry.
	data := []byte{0xa1, 0x9a, 0, 1, 0x86, 0xa0, 0xa2, 0, 0, 0, 0}
	data = append(data, make([]byte, count-1)...)
	data = append(data, 0)
	var value Value
	_, err := Decode(data, &value)
	require.True(t, IsDuplicateMapKeyError(err), "got %v", err)
	result := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			var value Value
			if _, err := Decode(data, &value); err == nil {
				b.Fatal("expected duplicate map key error")
			}
		}
	})
	t.Logf(
		"%d-entry map-key array failing at its first entry: %d bytes/op",
		count,
		result.AllocedBytesPerOp(),
	)
	require.Less(t, result.AllocedBytesPerOp(), int64(count*sha256.Size/4))
}

func TestValueModeLimitsApplyToWholeTree(t *testing.T) {
	t.Parallel()
	mode, err := (_cbor.DecOptions{
		MaxNestedLevels: 8, MaxArrayElements: 16, MaxMapPairs: 16,
	}).DecMode()
	require.NoError(t, err)
	// Wrap count zero items in a definite array.
	arrayOf := func(count int) []byte {
		return append([]byte{0x80 + byte(count)}, make([]byte, count)...)
	}
	mapOf := func(count int) []byte {
		data := []byte{0xa0 + byte(count)}
		for i := range count {
			data = append(data, byte(i), 0)
		}
		return data
	}
	join := func(header byte, parts ...[]byte) []byte {
		return append([]byte{header}, bytes.Join(parts, nil)...)
	}
	// Tag 1000 has no registered type, so the library decodes its content.
	tagged := func(content []byte) []byte {
		return append([]byte{0xd9, 0x03, 0xe8}, content...)
	}
	cases := map[string]struct {
		data   []byte
		reject bool
	}{
		// 2 outer + 14 inner elements: exactly the array limit.
		"arrays at limit": {
			data: join(0x82, arrayOf(7), arrayOf(7)),
		},
		// Each array is within the limit; the tree holds 2 + 20.
		"arrays over limit": {
			data:   join(0x82, arrayOf(10), arrayOf(10)),
			reject: true,
		},
		"indefinite arrays over limit": {
			data: append(
				join(0x9f, arrayOf(10), arrayOf(10)),
				0xff,
			),
			reject: true,
		},
		"arrays inside map values over limit": {
			data: join(
				0xa2,
				[]byte{0x00}, arrayOf(10),
				[]byte{0x01}, arrayOf(10),
			),
			reject: true,
		},
		"maps at limit": {
			data: join(0x82, mapOf(7), mapOf(7)),
		},
		"maps over limit": {
			data:   join(0x82, mapOf(10), mapOf(10)),
			reject: true,
		},
		"arrays inside tags at limit": {
			data: join(0x82, tagged(arrayOf(7)), tagged(arrayOf(7))),
		},
		"arrays inside tags over limit": {
			data:   join(0x82, tagged(arrayOf(10)), tagged(arrayOf(10))),
			reject: true,
		},
		"indefinite arrays inside tags over limit": {
			data: join(
				0x82,
				tagged(append(join(0x9f, make([]byte, 10)), 0xff)),
				tagged(append(join(0x9f, make([]byte, 10)), 0xff)),
			),
			reject: true,
		},
		"maps inside tags over limit": {
			data:   join(0x82, tagged(mapOf(10)), tagged(mapOf(10))),
			reject: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var value Value
			_, err := decodeWithMode(
				tc.data,
				&value,
				mode,
				rejectDuplicateMapKeys,
			)
			if tc.reject {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValueStrictLimitAppliesAcrossNestedArrays(t *testing.T) {
	t.Parallel()
	const inner = 100000
	element := func() []byte {
		return append([]byte{0x9a, 0, 1, 0x86, 0xa0}, make([]byte, inner)...)
	}
	data := append([]byte{0x82}, element()...)
	data = append(data, element()...)
	var strict Value
	_, err := DecodeStrict(data, &strict)
	require.Error(t, err, "strict mode must bound the whole tree")
	var ledger Value
	_, err = Decode(data, &ledger)
	require.NoError(t, err)
}
