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
	"bytes"
	"math"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func TestPointDecodeOriginClearsPreviousPoint(t *testing.T) {
	t.Parallel()
	point := NewPoint(42, bytes.Repeat([]byte{0xab}, 32))
	require.NoError(t, point.UnmarshalCBOR([]byte{0x80}))
	require.Equal(t, NewPointOrigin(), point)
	encoded, err := point.MarshalCBOR()
	require.NoError(t, err)
	require.Equal(t, []byte{0x80}, encoded)
}

func TestPointDecodeConcreteRoundTrip(t *testing.T) {
	t.Parallel()
	for _, slot := range []uint64{0, 1, math.MaxUint64} {
		want := NewPoint(slot, bytes.Repeat([]byte{0xcd}, 32))
		encoded, err := want.MarshalCBOR()
		require.NoError(t, err)
		point := NewPointOrigin()
		require.NoError(t, point.UnmarshalCBOR(encoded))
		require.Equal(t, want, point)
	}
}

func TestPointDecodeRejectsInvalidEnvelope(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"trailing value", []byte{0x80, 0x00}},
		{"indefinite array", []byte{0x9f, 0xff}},
		{"tagged array", []byte{0xc0, 0x80}},
		{"truncated array", []byte{0x82, 0x00}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			point := NewPoint(42, bytes.Repeat([]byte{0xee}, 32))
			want := NewPoint(point.Slot, bytes.Clone(point.Hash))
			require.Error(t, point.UnmarshalCBOR(tc.data))
			require.Equal(t, want, point)
		})
	}
}

func TestPointDecodeRejectsMalformedWithoutMutation(t *testing.T) {
	t.Parallel()
	hash := bytes.Repeat([]byte{0xab}, 32)
	cases := []struct {
		name  string
		value any
	}{
		{"null", nil},
		{"one element", []any{uint64(1)}},
		{"three elements", []any{uint64(1), hash, uint64(2)}},
		{"negative slot", []any{int64(-1), hash}},
		{"text slot", []any{"1", hash}},
		{"text hash", []any{uint64(1), "hash"}},
		{"null hash", []any{uint64(1), nil}},
		{"empty hash", []any{uint64(1), []byte{}}},
		{"short hash", []any{uint64(1), hash[:31]}},
		{"long hash", []any{uint64(1), append(bytes.Clone(hash), 0)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			encoded, err := cbor.Encode(tc.value)
			require.NoError(t, err)
			want := NewPoint(42, bytes.Repeat([]byte{0xee}, 32))
			point := NewPoint(want.Slot, bytes.Clone(want.Hash))
			require.Error(t, point.UnmarshalCBOR(encoded))
			require.Equal(t, want, point)
		})
	}
}
