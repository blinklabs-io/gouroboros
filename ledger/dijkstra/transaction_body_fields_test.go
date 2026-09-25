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

package dijkstra

import (
	"strconv"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func TestDijkstraTransactionBodyRequiredFields(t *testing.T) {
	t.Parallel()

	for _, key := range []uint{0, 1, 2} {
		t.Run(strconv.FormatUint(uint64(key), 10), func(t *testing.T) {
			fields := map[uint]any{
				0: []any{},
				1: []any{},
				2: uint64(0),
			}
			delete(fields, key)
			encoded, err := cbor.Encode(fields)
			require.NoError(t, err)
			var body DijkstraTransactionBody
			err = body.UnmarshalCBOR(encoded)
			require.ErrorContains(t, err, "required CBOR map field")
		})
	}
}

func TestDijkstraSubTransactionBodyRequiredFields(t *testing.T) {
	t.Parallel()

	for _, key := range []uint{0, 1} {
		t.Run(strconv.FormatUint(uint64(key), 10), func(t *testing.T) {
			fields := map[uint]any{0: []any{}, 1: []any{}}
			delete(fields, key)
			encoded, err := cbor.Encode(fields)
			require.NoError(t, err)
			var body DijkstraSubTransactionBody
			err = body.UnmarshalCBOR(encoded)
			require.ErrorContains(t, err, "required CBOR map field")
		})
	}
}

func TestDijkstraTransactionBodiesRejectExplicitlyEmptyFields(t *testing.T) {
	t.Parallel()

	for _, field := range []struct {
		name  string
		key   uint
		value any
		body  func([]byte) error
	}{
		{"top certificates", 4, []any{}, func(b []byte) error {
			return new(DijkstraTransactionBody).UnmarshalCBOR(b)
		}},
		{"top withdrawals", 5, map[uint]any{}, func(b []byte) error {
			return new(DijkstraTransactionBody).UnmarshalCBOR(b)
		}},
		{"top mint", 9, map[uint]any{}, func(b []byte) error {
			return new(DijkstraTransactionBody).UnmarshalCBOR(b)
		}},
		{"top collateral", 13, []any{}, func(b []byte) error {
			return new(DijkstraTransactionBody).UnmarshalCBOR(b)
		}},
		{"top guards", 14, []any{}, func(b []byte) error {
			return new(DijkstraTransactionBody).UnmarshalCBOR(b)
		}},
		{"top reference inputs", 18, []any{}, func(b []byte) error {
			return new(DijkstraTransactionBody).UnmarshalCBOR(b)
		}},
		{"top proposals", 20, []any{}, func(b []byte) error {
			return new(DijkstraTransactionBody).UnmarshalCBOR(b)
		}},
		{"top subtransactions", 23, []any{}, func(b []byte) error {
			return new(DijkstraTransactionBody).UnmarshalCBOR(b)
		}},
		{"sub certificates", 4, []any{}, func(b []byte) error {
			return new(DijkstraSubTransactionBody).UnmarshalCBOR(b)
		}},
		{"sub withdrawals", 5, map[uint]any{}, func(b []byte) error {
			return new(DijkstraSubTransactionBody).UnmarshalCBOR(b)
		}},
		{"sub mint", 9, map[uint]any{}, func(b []byte) error {
			return new(DijkstraSubTransactionBody).UnmarshalCBOR(b)
		}},
		{"sub reference inputs", 18, []any{}, func(b []byte) error {
			return new(DijkstraSubTransactionBody).UnmarshalCBOR(b)
		}},
		{"sub proposals", 20, []any{}, func(b []byte) error {
			return new(DijkstraSubTransactionBody).UnmarshalCBOR(b)
		}},
	} {
		t.Run(field.name, func(t *testing.T) {
			fields := map[uint]any{0: []any{}, 1: []any{}, 2: uint64(0)}
			fields[field.key] = field.value
			encoded, err := cbor.Encode(fields)
			require.NoError(t, err)
			require.Error(t, field.body(encoded))
		})
	}
}

func TestDijkstraSubTransactionBodyAllowsPresentEmptyOutputs(t *testing.T) {
	t.Parallel()

	encoded, err := cbor.Encode(map[uint]any{0: []any{}, 1: []any{}})
	require.NoError(t, err)
	var body DijkstraSubTransactionBody
	require.NoError(t, body.UnmarshalCBOR(encoded))
}

func TestDijkstraTransactionBodyAllowsPresentEmptyOutputs(t *testing.T) {
	t.Parallel()

	encoded, err := cbor.Encode(map[uint]any{
		0: []any{},
		1: []any{},
		2: uint64(0),
	})
	require.NoError(t, err)
	var body DijkstraTransactionBody
	require.NoError(t, body.UnmarshalCBOR(encoded))
}
