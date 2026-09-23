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

package shelley_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

func negativeNofKTransactionCbor(t *testing.T) []byte {
	t.Helper()
	data, err := cbor.Encode([]any{
		// Key 3 (ttl) is mandatory in the Shelley body; this fixture is
		// about the witness set, so it carries the minimum that decodes.
		map[uint64]any{3: uint64(0)},
		map[uint64]any{
			1: []any{
				[]any{uint64(3), int64(-1), []any{}},
			},
		},
		nil,
	})
	require.NoError(t, err)
	return data
}

func TestNegativeNofKThresholdIsAcceptedInShelley(t *testing.T) {
	data := negativeNofKTransactionCbor(t)

	_, err := shelley.NewShelleyTransactionFromCbor(data)
	require.NoError(t, err)
}

func TestNegativeNofKThresholdIsAcceptedFromAllegra(t *testing.T) {
	data := negativeNofKTransactionCbor(t)

	_, err := allegra.NewAllegraTransactionFromCbor(data)
	require.NoError(t, err)
}
