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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

// A reject reason is [type] or [type, message]. The array count must be
// checked before any element is materialized, so a reply declaring many
// elements is rejected without allocating for them.
func TestRejectReasonDataRejectsLongArrayBeforeDecoding(t *testing.T) {
	const count = 10_000
	data := []byte{0x99, count >> 8, count & 0xff}
	for range count {
		// An empty array materializes as a []any of its own.
		data = append(data, 0x80)
	}
	var r RejectReasonData
	require.Error(t, r.UnmarshalCBOR(data))
	result := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			var r RejectReasonData
			if err := r.UnmarshalCBOR(data); err == nil {
				b.Fatal("expected decode error")
			}
		}
	})
	t.Logf(
		"%d-element reject reason: %d bytes/op",
		count,
		result.AllocedBytesPerOp(),
	)
	require.Less(t, result.AllocedBytesPerOp(), int64(count))
}

func TestRejectReasonDataDecodeShapes(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    RejectReasonData
		wantErr bool
	}{
		{
			name: "type only",
			data: []byte{0x81, 0x01},
			want: RejectReasonData{Type: 1},
		},
		{
			name: "type and message",
			data: []byte{0x82, 0x00, 0x63, 'b', 'a', 'd'},
			want: RejectReasonData{Type: 0, Message: "bad"},
		},
		{
			name: "null message",
			data: []byte{0x82, 0x02, 0xf6},
			want: RejectReasonData{Type: 2},
		},
		{
			name: "undefined message",
			data: []byte{0x82, 0x02, 0xf7},
			want: RejectReasonData{Type: 2},
		},
		{
			name: "indefinite array and message",
			data: []byte{0x9f, 0x03, 0x7f, 0x61, 'o', 0x61, 'k', 0xff, 0xff},
			want: RejectReasonData{Type: 3, Message: "ok"},
		},
		{name: "empty array", data: []byte{0x80}, wantErr: true},
		{
			name:    "three elements",
			data:    []byte{0x83, 0x03, 0x60, 0x00},
			wantErr: true,
		},
		{name: "not an array", data: []byte{0x01}, wantErr: true},
		{name: "type out of range", data: []byte{0x81, 0x04}, wantErr: true},
		{name: "negative type", data: []byte{0x81, 0x20}, wantErr: true},
		{name: "text type", data: []byte{0x81, 0x60}, wantErr: true},
		{
			name:    "bignum type",
			data:    []byte{0x81, 0xc2, 0x41, 0x01},
			wantErr: true,
		},
		{
			name:    "integer message",
			data:    []byte{0x82, 0x00, 0x01},
			wantErr: true,
		},
		{
			name:    "array message",
			data:    []byte{0x82, 0x00, 0x80},
			wantErr: true,
		},
		{
			name:    "tagged message",
			data:    []byte{0x82, 0x00, 0xd8, 0x20, 0x60},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := RejectReasonData{Type: 3, Message: "stale"}
			_, err := cbor.Decode(tc.data, &r)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, r)
		})
	}
}
