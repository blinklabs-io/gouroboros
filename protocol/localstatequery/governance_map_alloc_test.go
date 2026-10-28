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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

// twoByteEntries encodes a well-formed map of governanceAllocEntries entries
// whose one-byte keys and values do not decode as a governance map entry.
func twoByteEntries() []byte {
	data := []byte{
		0xb9,
		governanceAllocEntries >> 8,
		governanceAllocEntries & 0xff,
	}
	return append(data, make([]byte, 2*governanceAllocEntries)...)
}

// govActionStateWithMap encodes a GovActionState whose map field at index
// mapField (1, 2 or 3) is replaced by data and whose other maps are empty.
func govActionStateWithMap(mapField int, data []byte) []byte {
	out := []byte{0x87, 0x82, 0x58, 0x20}
	out = append(out, make([]byte, 32)...)
	out = append(out, 0x00)
	for field := 1; field <= 3; field++ {
		if field == mapField {
			out = append(out, data...)
			continue
		}
		out = append(out, 0xa0)
	}
	return append(out, 0xf6, 0x00, 0x00)
}

// Each governance map must grow only with entries that decode. Sized from the
// declared count, a map of two-byte entries reserves a whole bucket slot per
// entry before the first entry fails.
func TestGovernanceMapsGrowAsEntriesDecode(t *testing.T) {
	decodeAction := func(data []byte) error {
		var s GovActionState
		_, err := cbor.Decode(data, &s)
		return err
	}
	tests := []struct {
		name   string
		data   []byte
		decode func([]byte) error
	}{
		{
			name:   "committee votes",
			data:   govActionStateWithMap(1, twoByteEntries()),
			decode: decodeAction,
		},
		{
			name:   "DRep votes",
			data:   govActionStateWithMap(2, twoByteEntries()),
			decode: decodeAction,
		},
		{
			name:   "SPO votes",
			data:   govActionStateWithMap(3, twoByteEntries()),
			decode: decodeAction,
		},
		{
			name: "enact state withdrawals",
			data: concat(
				[]byte{0x87, 0xf6, 0x82, 0xf6, 0xf6, 0xf6, 0xf6, 0x00},
				twoByteEntries(),
				[]byte{0xf6},
			),
			decode: func(data []byte) error {
				var s EnactState
				_, err := cbor.Decode(data, &s)
				return err
			},
		},
		{
			name: "committee members",
			data: concat(
				[]byte{0x82, 0x01, 0x82},
				twoByteEntries(),
				[]byte{0xd8, 0x1e, 0x82, 0x01, 0x02},
			),
			decode: func(data []byte) error {
				g := GovStateResult{Committee: data}
				_, err := g.DecodeCommittee()
				return err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.decode(tc.data))
			result := testing.Benchmark(func(b *testing.B) {
				for range b.N {
					if err := tc.decode(tc.data); err == nil {
						b.Fatal("expected decode error")
					}
				}
			})
			t.Logf(
				"%d two-byte entries: %d bytes/op",
				governanceAllocEntries,
				result.AllocedBytesPerOp(),
			)
			// The input itself is about 2 bytes per entry; anything near
			// the declared count times a map slot means the map was sized
			// from the header.
			require.Less(
				t,
				result.AllocedBytesPerOp(),
				int64(governanceAllocEntries*8),
			)
		})
	}
}

// Valid maps decode unchanged through the incremental decoder, and a repeated
// key is rejected as the default decoder rejects it.
func TestGrowingMapDecodesValidMaps(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    map[uint64]uint64
		wantErr bool
	}{
		{name: "empty", data: []byte{0xa0}, want: map[uint64]uint64{}},
		{name: "null", data: []byte{0xf6}, want: nil},
		{
			name: "definite",
			data: []byte{0xa2, 0x01, 0x02, 0x03, 0x04},
			want: map[uint64]uint64{1: 2, 3: 4},
		},
		{
			name: "indefinite",
			data: []byte{0xbf, 0x01, 0x02, 0xff},
			want: map[uint64]uint64{1: 2},
		},
		{
			name:    "duplicate key",
			data:    []byte{0xa2, 0x01, 0x02, 0x01, 0x03},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var plain map[uint64]uint64
			_, plainErr := cbor.Decode(tc.data, &plain)
			var got growingMap[uint64, uint64]
			_, err := cbor.Decode(tc.data, &got)
			if tc.wantErr {
				require.Error(t, plainErr)
				require.Error(t, err)
				return
			}
			require.NoError(t, plainErr)
			require.NoError(t, err)
			require.Equal(t, plain == nil, got == nil)
			if tc.want == nil {
				require.Nil(t, got)
				return
			}
			require.Equal(t, tc.want, map[uint64]uint64(got))
		})
	}
}
