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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func TestDijkstraTransactionDecodeRejectsInvalidBodyAndWitnessFields(t *testing.T) {
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
				cbor.RawMessage(body), test.witness, nil,
			})
			require.NoError(t, err)
			_, err = NewDijkstraTransactionFromCbor(tx)
			require.ErrorContains(t, err, test.wantText)
		})
	}
}

func TestDijkstraWitnessSetRejectsLegacyEmptyRedeemerList(t *testing.T) {
	encoded, err := cbor.Encode(map[uint]any{5: []any{}})
	require.NoError(t, err)

	var witnesses DijkstraTransactionWitnessSet
	require.Error(t, witnesses.UnmarshalCBOR(encoded))
}
