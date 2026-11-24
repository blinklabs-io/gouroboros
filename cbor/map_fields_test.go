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
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"runtime"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func TestValidateMapFields(t *testing.T) {
	t.Run(
		"required key distinguishes missing from zero value",
		func(t *testing.T) {
			data, err := cbor.Encode(map[uint64]any{0: uint64(0)})
			require.NoError(t, err)
			require.NoError(t, cbor.ValidateMapFields(data, []uint64{0}, nil))
			require.ErrorContains(
				t,
				cbor.ValidateMapFields(data, []uint64{1}, nil),
				"field 1 is missing",
			)
		},
	)

	t.Run("empty outputs are permitted when not guarded", func(t *testing.T) {
		data, err := cbor.Encode(map[uint64]any{0: []any{}})
		require.NoError(t, err)
		require.NoError(t, cbor.ValidateMapFields(data, nil, nil))
	})

	t.Run("rejects empty arrays, maps, and tagged sets", func(t *testing.T) {
		for _, value := range []any{
			[]any{},
			map[uint64]any{},
			cbor.Set{},
		} {
			data, err := cbor.Encode(map[uint64]any{4: value})
			require.NoError(t, err)
			require.ErrorContains(
				t,
				cbor.ValidateMapFields(data, nil, []uint64{4}),
				"field 4 must not be empty",
			)
		}
	})

	t.Run("accepts non-empty tagged sets", func(t *testing.T) {
		data, err := cbor.Encode(map[uint64]any{4: cbor.Set{uint64(1)}})
		require.NoError(t, err)
		require.NoError(t, cbor.ValidateMapFields(data, nil, []uint64{4}))
	})
}

// Not t.Parallel: measures process-wide allocation via runtime.MemStats.
func TestValidateMapFieldsRejectsBadKeysWithoutSizingMap(t *testing.T) {
	const pairs = 50_000
	// Map header declaring pairs entries, each an empty byte string key and
	// a zero value: well-formed, but no key decodes as an integer.
	mapData := []byte{0xba, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(mapData[1:], pairs)
	mapData = append(mapData, bytes.Repeat([]byte{0x40, 0x00}, pairs)...)

	for name, data := range map[string][]byte{
		"bare map": mapData,
		// Tag 55799 (self-described CBOR) is accepted around any item.
		"tagged map": append([]byte{0xd9, 0xd9, 0xf7}, mapData...),
	} {
		t.Run(name, func(t *testing.T) {
			validate := func() {
				require.Error(t, cbor.ValidateMapFields(data, nil, nil))
			}
			validate()

			const runs = 5
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			for range runs {
				validate()
			}
			runtime.ReadMemStats(&after)
			perRun := (after.TotalAlloc - before.TotalAlloc) / runs
			require.Less(
				t,
				perRun,
				uint64(len(data)),
				"ValidateMapFields allocated %d bytes per call for %d input bytes",
				perRun,
				len(data),
			)
		})
	}
}

func TestDecodeRawMapMatchesDecodeLedgerMap(t *testing.T) {
	for _, input := range []string{
		"a0",                       // empty map
		"f6",                       // null
		"80",                       // not a map
		"a2010002820102",           // definite map
		"bf010002820102ff",         // indefinite map
		"a1010000",                 // trailing byte after the map
		"bb0000000000000001010000", // 64-bit pair count
		"a1c1010000",               // tagged key
		"a201000100",               // duplicate key
		"bf01000100ff",             // duplicate key, indefinite map
		"a20100180100",             // duplicate key, non-minimal head
		"a2200001000100",           // negative key
		"bf0100",                   // indefinite map without break
		"b9000101",                 // fewer pairs than declared
		"a2014161024162",           // equal-length byte string values
		"d9d9f7a10100",             // self-described CBOR tag
		"d9d9f7d9d9f7a10100",       // repeated self-described CBOR tag
		"d81ea10100",               // tag 30 around a map
		"d90102a10100",             // tag 258 around a map
		"d90103a10100",             // tag 259 around a map
		"c1a10100",                 // tag 1 around a map
		"c0a10100",                 // tag 0 around a map
		"d9d9f7f6",                 // tagged null
		"d9d9f7a2400001000100",     // tagged map with a bad key
		"a101a0",                   // nested map value
	} {
		t.Run(input, func(t *testing.T) {
			data, err := hex.DecodeString(input)
			require.NoError(t, err)
			var want map[uint]cbor.RawMessage
			wantN, wantErr := cbor.DecodeLedgerMap(data, &want)
			got, gotN, gotErr := cbor.DecodeRawMap[uint](data)
			if wantErr != nil {
				require.EqualError(t, gotErr, wantErr.Error())
				require.Equal(
					t,
					cbor.IsDuplicateMapKeyError(wantErr),
					cbor.IsDuplicateMapKeyError(gotErr),
				)
				return
			}
			require.NoError(t, gotErr)
			require.Equal(t, wantN, gotN)
			require.Equal(t, want == nil, got == nil)
			require.Equal(t, want, got)
		})
	}
}

// Not t.Parallel: testing.AllocsPerRun measures process-wide allocation.
func TestDecodeRawMapAllocatesNoMoreThanDecodeLedgerMap(t *testing.T) {
	fields := map[uint]any{}
	for key := range uint(16) {
		fields[key] = []any{make([]byte, 32), key}
	}
	data, err := cbor.Encode(fields)
	require.NoError(t, err)
	full := testing.AllocsPerRun(20, func() {
		var decoded map[uint]cbor.RawMessage
		_, _ = cbor.DecodeLedgerMap(data, &decoded)
	})
	raw := testing.AllocsPerRun(20, func() {
		_, _, _ = cbor.DecodeRawMap[uint](data)
	})
	require.LessOrEqual(t, raw, full)
}
