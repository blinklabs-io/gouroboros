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
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

type validityUpperBoundBody interface {
	common.TransactionBody
	UnmarshalCBOR([]byte) error
	MarshalCBOR() ([]byte, error)
	SetCbor([]byte)
	SetValidityIntervalUpperBound(uint64)
	ClearValidityIntervalUpperBound()
}

type validityUpperBoundTestCase struct {
	name    string
	newBody func() validityUpperBoundBody
	newTx   func(validityUpperBoundBody) common.TransactionBody
}

func validityUpperBoundTestCases() []validityUpperBoundTestCase {
	return []validityUpperBoundTestCase{
		{
			name: "Allegra",
			newBody: func() validityUpperBoundBody {
				return &allegra.AllegraTransactionBody{}
			},
			newTx: func(body validityUpperBoundBody) common.TransactionBody {
				return &allegra.AllegraTransaction{
					Body: *(body.(*allegra.AllegraTransactionBody)),
				}
			},
		},
		{
			name: "Mary",
			newBody: func() validityUpperBoundBody {
				return &mary.MaryTransactionBody{}
			},
			newTx: func(body validityUpperBoundBody) common.TransactionBody {
				return &mary.MaryTransaction{
					Body: *(body.(*mary.MaryTransactionBody)),
				}
			},
		},
		{
			name: "Alonzo",
			newBody: func() validityUpperBoundBody {
				return &alonzo.AlonzoTransactionBody{}
			},
			newTx: func(body validityUpperBoundBody) common.TransactionBody {
				return &alonzo.AlonzoTransaction{
					Body: *(body.(*alonzo.AlonzoTransactionBody)),
				}
			},
		},
		{
			name: "Babbage",
			newBody: func() validityUpperBoundBody {
				return &babbage.BabbageTransactionBody{}
			},
			newTx: func(body validityUpperBoundBody) common.TransactionBody {
				return &babbage.BabbageTransaction{
					Body: *(body.(*babbage.BabbageTransactionBody)),
				}
			},
		},
		{
			name: "Conway",
			newBody: func() validityUpperBoundBody {
				return &conway.ConwayTransactionBody{}
			},
			newTx: func(body validityUpperBoundBody) common.TransactionBody {
				return &conway.ConwayTransaction{
					Body: *(body.(*conway.ConwayTransactionBody)),
				}
			},
		},
		{
			name: "Dijkstra",
			newBody: func() validityUpperBoundBody {
				return &dijkstra.DijkstraTransactionBody{}
			},
			newTx: func(body validityUpperBoundBody) common.TransactionBody {
				return &dijkstra.DijkstraTransaction{
					Body: *(body.(*dijkstra.DijkstraTransactionBody)),
				}
			},
		},
		{
			name: "Dijkstra sub-transaction",
			newBody: func() validityUpperBoundBody {
				return &dijkstra.DijkstraSubTransactionBody{}
			},
		},
	}
}

func TestValidityIntervalUpperBoundDecodedPresence(t *testing.T) {
	for _, test := range validityUpperBoundTestCases() {
		t.Run(test.name, func(t *testing.T) {
			body := test.newBody()
			require.NoError(t, body.UnmarshalCBOR([]byte{0xa1, 0x03, 0x00}))
			requireValidityUpperBound(t, body, 0, true)
			if test.newTx != nil {
				requireValidityUpperBound(t, test.newTx(body), 0, true)
			}

			// Presence is decoded state, not a query-time heuristic over stored
			// CBOR. Clearing stored bytes must not lose an explicit zero.
			body.SetCbor(nil)
			requireValidityUpperBound(t, body, 0, true)

			// Reusing a receiver must not retain presence from the prior body.
			require.NoError(t, body.UnmarshalCBOR([]byte{0xa0}))
			requireValidityUpperBound(t, body, 0, false)
		})
	}
}

func TestValidityIntervalUpperBoundConstructedZeroRoundTrip(t *testing.T) {
	for _, test := range validityUpperBoundTestCases() {
		t.Run(test.name, func(t *testing.T) {
			body := test.newBody()
			requireValidityUpperBound(t, body, 0, false)

			body.SetValidityIntervalUpperBound(0)
			requireValidityUpperBound(t, body, 0, true)
			if test.newTx != nil {
				tx := test.newTx(body)
				requireValidityUpperBound(t, tx, 0, true)
				txMarshaler := tx.(interface {
					MarshalCBOR() ([]byte, error)
				})
				encodedTx, err := txMarshaler.MarshalCBOR()
				require.NoError(t, err)
				var txFields []cbor.RawMessage
				_, err = cbor.Decode(encodedTx, &txFields)
				require.NoError(t, err)
				require.NotEmpty(t, txFields)
				var txBodyFields map[uint]cbor.RawMessage
				_, err = cbor.Decode(txFields[0], &txBodyFields)
				require.NoError(t, err)
				require.Contains(t, txBodyFields, uint(3))
			}

			encoded, err := body.MarshalCBOR()
			require.NoError(t, err)
			var fields map[uint]cbor.RawMessage
			_, err = cbor.Decode(encoded, &fields)
			require.NoError(t, err)
			require.Contains(t, fields, uint(3))

			body.ClearValidityIntervalUpperBound()
			requireValidityUpperBound(t, body, 0, false)
			encoded, err = body.MarshalCBOR()
			require.NoError(t, err)
			fields = nil
			_, err = cbor.Decode(encoded, &fields)
			require.NoError(t, err)
			require.NotContains(t, fields, uint(3))
		})
	}
}

func TestTransactionValidityIntervalUpperBoundLegacyFallback(t *testing.T) {
	body := &shelley.ShelleyTransactionBody{}
	requireValidityUpperBound(t, body, 0, false)

	body.Ttl = 42
	requireValidityUpperBound(t, body, 42, true)
}

func requireValidityUpperBound(
	t *testing.T,
	tx common.TransactionBody,
	wantUpperBound uint64,
	wantPresent bool,
) {
	t.Helper()
	upperBound, present := common.TransactionValidityIntervalUpperBound(tx)
	require.Equal(t, wantUpperBound, upperBound)
	require.Equal(t, wantPresent, present)
}
