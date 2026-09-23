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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func TestValidateMapFields(t *testing.T) {
	t.Run("required key distinguishes missing from zero value", func(t *testing.T) {
		data, err := cbor.Encode(map[uint64]any{0: uint64(0)})
		require.NoError(t, err)
		require.NoError(t, cbor.ValidateMapFields(data, []uint64{0}, nil))
		require.ErrorContains(t, cbor.ValidateMapFields(data, []uint64{1}, nil), "field 1 is missing")
	})

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
