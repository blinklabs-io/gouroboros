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
	"unsafe"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

const governanceAllocEntries = 10_000

// oneByteEntries encodes a well-formed array of governanceAllocEntries
// one-byte entries, none of which decodes as a governance item.
func oneByteEntries(prefix ...byte) []byte {
	data := append(
		prefix,
		0x99,
		governanceAllocEntries>>8,
		governanceAllocEntries&0xff,
	)
	return append(data, make([]byte, governanceAllocEntries)...)
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

// Each governance slice must grow only with entries that decode. Sized from
// the declared count, a reply of one-byte entries reserves a whole element per
// byte before the first entry fails.
func TestGovernanceResultsGrowAsEntriesDecode(t *testing.T) {
	nulls := func(n int) []byte {
		out := make([]byte, n)
		for i := range out {
			out[i] = 0xf6
		}
		return out
	}
	tests := []struct {
		name     string
		data     []byte
		elemSize uintptr
		decode   func([]byte) error
	}{
		{
			name:     "gov state proposals",
			data:     concat([]byte{0x87}, oneByteEntries(), nulls(6)),
			elemSize: unsafe.Sizeof(GovActionState{}),
			decode: func(data []byte) error {
				var r GovStateResult
				_, err := cbor.Decode(data, &r)
				return err
			},
		},
		{
			name:     "proposals",
			data:     oneByteEntries(0x81),
			elemSize: unsafe.Sizeof(GovActionState{}),
			decode: func(data []byte) error {
				var r ProposalsResult
				_, err := cbor.Decode(data, &r)
				return err
			},
		},
		{
			name: "ratify state enacted",
			data: concat(
				[]byte{0x84, 0xf6},
				oneByteEntries(),
				[]byte{0x80, 0xf4},
			),
			elemSize: unsafe.Sizeof(GovActionState{}),
			decode: func(data []byte) error {
				var r RatifyStateResult
				_, err := cbor.Decode(data, &r)
				return err
			},
		},
		{
			name: "ratify state expired",
			data: concat(
				[]byte{0x84, 0xf6, 0x80},
				oneByteEntries(),
				[]byte{0xf4},
			),
			elemSize: unsafe.Sizeof(lcommon.GovActionId{}),
			decode: func(data []byte) error {
				var r RatifyStateResult
				_, err := cbor.Decode(data, &r)
				return err
			},
		},
		{
			name:     "DRep state delegators",
			data:     oneByteEntries(0x84, 0x00, 0x80, 0x00, 0xd9, 0x01, 0x02),
			elemSize: unsafe.Sizeof(StakeCredential{}),
			decode: func(data []byte) error {
				var e DRepStateEntry
				_, err := cbor.Decode(data, &e)
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
				"%d one-byte entries: %d bytes/op",
				governanceAllocEntries,
				result.AllocedBytesPerOp(),
			)
			require.Less(
				t,
				result.AllocedBytesPerOp(),
				int64(governanceAllocEntries*tc.elemSize/4),
			)
		})
	}
}

// Valid replies decode through the incremental decoder as they do into a
// plain slice, including an empty list, a null, a null under the
// self-described tag, and an indefinite-length array.
func TestGrowingArrayDecodesValidArrays(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want []uint64
	}{
		{name: "empty", data: []byte{0x80}, want: []uint64{}},
		{name: "null", data: []byte{0xf6}, want: nil},
		{
			name: "self-described null",
			data: []byte{0xd9, 0xd9, 0xf7, 0xf6},
			want: nil,
		},
		{
			name: "self-described undefined",
			data: []byte{0xd9, 0xd9, 0xf7, 0xf7},
			want: nil,
		},
		{
			name: "definite",
			data: []byte{0x82, 0x01, 0x02},
			want: []uint64{1, 2},
		},
		{
			name: "indefinite",
			data: []byte{0x9f, 0x01, 0x02, 0xff},
			want: []uint64{1, 2},
		},
		{
			name: "tagged set",
			data: []byte{0xd9, 0x01, 0x02, 0x81, 0x07},
			want: []uint64{7},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var plain []uint64
			_, err := cbor.Decode(tc.data, &plain)
			require.NoError(t, err)
			var got growingArray[uint64]
			_, err = cbor.Decode(tc.data, &got)
			require.NoError(t, err)
			require.Equal(t, plain == nil, got == nil)
			if tc.want == nil {
				require.Nil(t, got)
				return
			}
			require.Equal(t, tc.want, []uint64(got))
		})
	}
}
