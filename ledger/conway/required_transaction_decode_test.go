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

package conway

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func TestConwayTransactionDecodeRejectsInvalidBodyAndWitnessFields(
	t *testing.T,
) {
	for _, test := range []struct {
		name     string
		body     map[uint]any
		witness  map[uint]any
		wantText string
	}{
		{
			name:     "missing required fee",
			body:     map[uint]any{0: []any{}, 1: []any{}},
			wantText: "required CBOR map field 2 is missing",
		},
		{
			name:     "null required fee",
			body:     map[uint]any{0: []any{}, 1: []any{}, 2: nil},
			wantText: "required CBOR map field 2 must not be null",
		},
		{
			name: "undefined required outputs",
			body: map[uint]any{
				0: []any{}, 1: cbor.RawMessage{0xf7}, 2: uint64(0),
			},
			wantText: "required CBOR map field 1 must not be undefined",
		},
		{
			name: "undefined required fee",
			body: map[uint]any{
				0: []any{}, 1: []any{}, 2: cbor.RawMessage{0xf7},
			},
			wantText: "required CBOR map field 2 must not be undefined",
		},
		{
			name:     "empty certificates",
			body:     map[uint]any{0: []any{}, 1: []any{}, 2: uint64(0), 4: []any{}},
			wantText: "must not be empty",
		},
		{
			name:     "empty witness vkeys",
			body:     map[uint]any{0: []any{}, 1: []any{}, 2: uint64(0)},
			witness:  map[uint]any{0: []any{}},
			wantText: "must not be empty",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, err := cbor.Encode(test.body)
			require.NoError(t, err)
			tx, err := cbor.Encode([]any{
				cbor.RawMessage(body), test.witness, true, nil,
			})
			require.NoError(t, err)
			_, err = NewConwayTransactionFromCbor(tx)
			require.ErrorContains(t, err, test.wantText)
		})
	}
}

func TestConwayTransactionOptionalFieldTypes(t *testing.T) {
	address := append([]byte{0x60}, bytes.Repeat([]byte{0x11}, 28)...)
	for _, field := range []struct {
		key   uint
		valid []any
	}{
		{3, []any{uint64(0), ^uint64(0)}},
		{7, []any{bytes.Repeat([]byte{0x11}, 32)}},
		{8, []any{uint64(0), ^uint64(0)}},
		{11, []any{bytes.Repeat([]byte{0x11}, 32)}},
		{15, []any{uint64(0), uint64(1)}},
		{16, []any{[]any{address, uint64(0)},
			map[uint]any{0: address, 1: uint64(0)}}},
		{17, []any{uint64(0), ^uint64(0)}},
		{21, []any{uint64(0), ^uint64(0)}},
	} {
		t.Run(fmt.Sprintf("field_%d", field.key), func(t *testing.T) {
			for _, invalid := range []struct {
				name  string
				value any
			}{
				{"null", cbor.RawMessage{0xf6}},
				{"undefined", cbor.RawMessage{0xf7}},
				{"bool", false},
				{"text", "invalid"},
				{"negative", int64(-1)},
			} {
				t.Run(invalid.name, func(t *testing.T) {
					fields := map[uint]any{
						0: []any{}, 1: []any{}, 2: uint64(0),
						field.key: invalid.value,
					}
					body, err := cbor.Encode(fields)
					require.NoError(t, err)
					_, err = NewConwayTransactionBodyFromCbor(body)
					require.Error(t, err)
					tx, err := cbor.Encode([]any{
						cbor.RawMessage(body), map[uint]any{}, true, nil,
					})
					require.NoError(t, err)
					_, err = NewConwayTransactionFromCbor(tx)
					require.Error(t, err)
				})
			}
			for i, value := range field.valid {
				t.Run(fmt.Sprintf("valid_%d", i), func(t *testing.T) {
					body, err := cbor.Encode(map[uint]any{
						0: []any{}, 1: []any{}, 2: uint64(0), field.key: value,
					})
					require.NoError(t, err)
					decoded, err := NewConwayTransactionBodyFromCbor(body)
					require.NoError(t, err)
					require.Equal(t, body, decoded.Cbor())
					tx, err := cbor.Encode([]any{
						cbor.RawMessage(body), map[uint]any{}, true, nil,
					})
					require.NoError(t, err)
					_, err = NewConwayTransactionFromCbor(tx)
					require.NoError(t, err)
				})
			}
		})
	}
	t.Run("omitted", func(t *testing.T) {
		body, err := cbor.Encode(map[uint]any{
			0: []any{}, 1: []any{}, 2: uint64(0),
		})
		require.NoError(t, err)
		_, err = NewConwayTransactionBodyFromCbor(body)
		require.NoError(t, err)
	})
}
