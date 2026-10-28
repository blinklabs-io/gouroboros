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
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListLengthEncodings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data []byte
		want int
	}{
		{"one-byte header", []byte{0x82, 0x01, 0x02}, 2},
		{"uint8 header", []byte{0x98, 0x02, 0x01, 0x02}, 2},
		{"uint16 header", []byte{0x99, 0x00, 0x02, 0x01, 0x02}, 2},
		{"uint32 header", []byte{0x9a, 0, 0, 0, 2, 0x01, 0x02}, 2},
		{"uint64 header", []byte{0x9b, 0, 0, 0, 0, 0, 0, 0, 2, 1, 2}, 2},
		{"indefinite", []byte{0x9f, 0x01, 0x02, 0xff}, 2},
		{"indefinite empty", []byte{0x9f, 0xff}, 0},
		{"indefinite nested items", []byte{
			0x9f, 0xf6, 0x82, 0x01, 0x02, 0xa1, 0x01, 0x02, 0xff,
		}, 3},
		{"definite empty", []byte{0x98, 0x00}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := cbor.ListLength(tt.data)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestListLengthRejectsInvalid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"not a list", []byte{0xa0}},
		{"declares more items than bytes", []byte{0x99, 0x00, 0x03, 1, 2}},
		{"declares 64-bit count with no body", []byte{
			0x9b, 0x00, 0x00, 0x00, 0x00, 0x7f, 0xff, 0xff, 0xff,
		}},
		{"unterminated indefinite", []byte{0x9f, 0x01}},
		{"indefinite malformed item", []byte{0x9f, 0x82, 0x01, 0xff}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := cbor.ListLength(tt.data)
			require.Error(t, err)
		})
	}
}

// Not t.Parallel: testing.AllocsPerRun measures process-wide allocation.
func TestListLengthDoesNotAllocateForDeclaredItems(t *testing.T) {
	// A non-shortcut header followed by one byte per declared item: counting
	// the items must not build a slice of them.
	const items = 100_000
	data := append(
		[]byte{0x9a, 0x00, 0x01, 0x86, 0xa0},
		make([]byte, items)...,
	)
	var got int
	allocs := testing.AllocsPerRun(5, func() {
		got, _ = cbor.ListLength(data)
	})
	assert.Equal(t, items, got)
	assert.Zero(t, allocs, "ListLength allocated for the declared items")

	// An indefinite list has no count to read, but walking its items must
	// not build a slice of them either.
	indefinite := append(
		append([]byte{0x9f}, make([]byte, items)...),
		0xff,
	)
	allocs = testing.AllocsPerRun(5, func() {
		got, _ = cbor.ListLength(indefinite)
	})
	assert.Equal(t, items, got)
	assert.Less(t, allocs, float64(10),
		"ListLength allocated for the indefinite list items")

	// The declared count exceeds the bytes present, so it is refused by
	// the header alone.
	lying := []byte{0x9a, 0x00, 0x98, 0x96, 0x80}
	_, err := cbor.ListLength(lying)
	require.Error(t, err)
	assert.True(
		t,
		strings.Contains(err.Error(), "declares"),
		"want a declared-length error, got %v", err,
	)
}
