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

package ledger_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

type marshalCBORer interface {
	MarshalCBOR() ([]byte, error)
}

// TestZeroValueTransactionBodyEncodesRequiredKeys proves that a freshly
// constructed (never-decoded) transaction body still emits the map keys the
// CDDL marks mandatory (no "?"), even when their Go zero values would
// otherwise be omitted by "omitempty": transaction_body/sub_transaction_body
// keys 0 (inputs), 1 (outputs), and (except for a Dijkstra sub-transaction,
// which has no fee field) 2 (fee) in every era from Shelley through Dijkstra,
// per cardano-ledger's eras/*/impl/cddl*/*.cddl transaction_body definitions.
// Shelley's key 3 (ttl) is also mandatory there and is asserted separately.
func TestZeroValueTransactionBodyEncodesRequiredKeys(t *testing.T) {
	tests := []struct {
		name          string
		body          marshalCBORer
		requiredKeys  []uint
		forbiddenKeys []uint
	}{
		{
			name:         "Shelley",
			body:         &shelley.ShelleyTransactionBody{},
			requiredKeys: []uint{0, 1, 2, 3},
		},
		{
			name:         "Allegra",
			body:         &allegra.AllegraTransactionBody{},
			requiredKeys: []uint{0, 1, 2},
			// Allegra makes ttl (key 3) optional; a zero-value body carries
			// no explicit upper bound, so key 3 must stay absent.
			forbiddenKeys: []uint{3},
		},
		{
			name:          "Mary",
			body:          &mary.MaryTransactionBody{},
			requiredKeys:  []uint{0, 1, 2},
			forbiddenKeys: []uint{3},
		},
		{
			name:          "Alonzo",
			body:          &alonzo.AlonzoTransactionBody{},
			requiredKeys:  []uint{0, 1, 2},
			forbiddenKeys: []uint{3},
		},
		{
			name:          "Babbage",
			body:          &babbage.BabbageTransactionBody{},
			requiredKeys:  []uint{0, 1, 2},
			forbiddenKeys: []uint{3},
		},
		{
			name:          "Conway",
			body:          &conway.ConwayTransactionBody{},
			requiredKeys:  []uint{0, 1, 2},
			forbiddenKeys: []uint{3},
		},
		{
			name:          "Dijkstra",
			body:          &dijkstra.DijkstraTransactionBody{},
			requiredKeys:  []uint{0, 1, 2},
			forbiddenKeys: []uint{3},
		},
		{
			name:         "Dijkstra sub-transaction",
			body:         &dijkstra.DijkstraSubTransactionBody{},
			requiredKeys: []uint{0, 1},
			// Sub-transactions carry no fee field at all (not merely
			// optional): cardano-ledger's sub_transaction_body has no key 2.
			forbiddenKeys: []uint{2, 3},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			encoded, err := test.body.MarshalCBOR()
			require.NoError(t, err)

			var fields map[uint]cbor.RawMessage
			_, err = cbor.Decode(encoded, &fields)
			require.NoError(t, err)

			for _, key := range test.requiredKeys {
				require.Containsf(
					t,
					fields,
					key,
					"required transaction-body key %d must be present even when its Go zero value is empty",
					key,
				)
			}
			for _, key := range test.forbiddenKeys {
				require.NotContainsf(
					t,
					fields,
					key,
					"optional/nonexistent transaction-body key %d must stay absent",
					key,
				)
			}
		})
	}
}
