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
