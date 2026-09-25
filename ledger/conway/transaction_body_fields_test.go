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
	"strconv"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func TestConwayTransactionBodyRequiredFields(t *testing.T) {
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
			var body ConwayTransactionBody
			err = body.UnmarshalCBOR(encoded)
			require.ErrorContains(
				t,
				err,
				"required CBOR map field",
			)
		})
	}
}

func TestConwayTransactionBodyRejectsExplicitlyEmptyFields(t *testing.T) {
	t.Parallel()

	for _, key := range []uint{4, 5, 9, 13, 14, 18, 20} {
		t.Run(strconv.FormatUint(uint64(key), 10), func(t *testing.T) {
			fields := map[uint]any{
				0: []any{},
				1: []any{},
				2: uint64(0),
			}
			if key == 5 {
				fields[key] = map[uint]any{}
			} else if key == 9 {
				fields[key] = map[uint]any{}
			} else {
				fields[key] = []any{}
			}
			encoded, err := cbor.Encode(fields)
			require.NoError(t, err)
			var body ConwayTransactionBody
			err = body.UnmarshalCBOR(encoded)
			require.Error(t, err)
		})
	}
}

func TestConwayTransactionBodyAllowsPresentEmptyOutputs(t *testing.T) {
	t.Parallel()

	encoded, err := cbor.Encode(map[uint]any{
		0: []any{},
		1: []any{},
		2: uint64(0),
	})
	require.NoError(t, err)
	var body ConwayTransactionBody
	require.NoError(t, body.UnmarshalCBOR(encoded))
}
