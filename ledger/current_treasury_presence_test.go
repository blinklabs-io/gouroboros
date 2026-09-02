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
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/stretchr/testify/require"
)

type currentTreasuryPresenceBody interface {
	common.TransactionBody
	UnmarshalCBOR([]byte) error
	MarshalCBOR() ([]byte, error)
	CurrentTreasuryValuePresent() bool
	SetCurrentTreasuryValuePresence(bool)
	ValidityIntervalUpperBound() (uint64, bool)
}

type currentTreasuryPresenceTestCase struct {
	name    string
	newBody func(uint64) currentTreasuryPresenceBody
}

func currentTreasuryPresenceTestCases() []currentTreasuryPresenceTestCase {
	return []currentTreasuryPresenceTestCase{
		{
			name: "Conway",
			newBody: func(value uint64) currentTreasuryPresenceBody {
				return &conway.ConwayTransactionBody{
					TxCurrentTreasuryValue: int64(value), // #nosec G115 -- test values are bounded
				}
			},
		},
		{
			name: "Dijkstra",
			newBody: func(value uint64) currentTreasuryPresenceBody {
				return &dijkstra.DijkstraTransactionBody{
					TxCurrentTreasuryValue: value,
				}
			},
		},
		{
			name: "Dijkstra sub-transaction",
			newBody: func(value uint64) currentTreasuryPresenceBody {
				return &dijkstra.DijkstraSubTransactionBody{
					TxCurrentTreasuryValue: value,
				}
			},
		},
	}
}

func TestCurrentTreasuryValueDecodedPresenceExactCBOR(t *testing.T) {
	tests := []struct {
		name             string
		cbor             []byte
		wantTreasury     *big.Int
		wantUpperBound   uint64
		wantUpperPresent bool
	}{
		{
			name: "absent",
			cbor: []byte{0xa0},
		},
		{
			name:         "explicit treasury zero",
			cbor:         []byte{0xa1, 0x15, 0x00},
			wantTreasury: big.NewInt(0),
		},
		{
			name:             "explicit upper and treasury zero",
			cbor:             []byte{0xa2, 0x03, 0x00, 0x15, 0x00},
			wantTreasury:     big.NewInt(0),
			wantUpperPresent: true,
		},
		{
			name:             "nonzero treasury and explicit upper zero",
			cbor:             []byte{0xa2, 0x03, 0x00, 0x15, 0x18, 0x2a},
			wantTreasury:     big.NewInt(42),
			wantUpperPresent: true,
		},
		{
			name:             "nonzero fields",
			cbor:             []byte{0xa2, 0x03, 0x18, 0x2b, 0x15, 0x18, 0x2a},
			wantTreasury:     big.NewInt(42),
			wantUpperBound:   43,
			wantUpperPresent: true,
		},
	}
	for _, bodyTest := range currentTreasuryPresenceTestCases() {
		t.Run(bodyTest.name, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					body := bodyTest.newBody(0)
					require.NoError(t, body.UnmarshalCBOR(test.cbor))
					require.Equal(
						t,
						test.wantTreasury != nil,
						body.CurrentTreasuryValuePresent(),
					)
					require.Equal(t, test.wantTreasury, body.CurrentTreasuryValue())
					upperBound, upperPresent := body.ValidityIntervalUpperBound()
					require.Equal(t, test.wantUpperBound, upperBound)
					require.Equal(t, test.wantUpperPresent, upperPresent)

					reencoded, err := body.MarshalCBOR()
					require.NoError(t, err)
					require.Equal(t, test.cbor, reencoded)
				})
			}
		})
	}
}

func TestCurrentTreasuryValueConstructedPresenceExactCBOR(t *testing.T) {
	for _, bodyTest := range currentTreasuryPresenceTestCases() {
		t.Run(bodyTest.name, func(t *testing.T) {
			absent := bodyTest.newBody(0)
			require.False(t, absent.CurrentTreasuryValuePresent())
			require.Nil(t, absent.CurrentTreasuryValue())
			encoded, err := absent.MarshalCBOR()
			require.NoError(t, err)
			require.Equal(t, []byte{0xa1, 0x00, 0x80}, encoded)

			explicitZero := bodyTest.newBody(0)
			explicitZero.SetCurrentTreasuryValuePresence(true)
			require.True(t, explicitZero.CurrentTreasuryValuePresent())
			require.Equal(t, big.NewInt(0), explicitZero.CurrentTreasuryValue())
			encoded, err = explicitZero.MarshalCBOR()
			require.NoError(t, err)
			require.Equal(t, []byte{0xa2, 0x00, 0x80, 0x15, 0x00}, encoded)

			nonzero := bodyTest.newBody(42)
			require.True(
				t,
				nonzero.CurrentTreasuryValuePresent(),
				"a nonzero typed value must imply presence",
			)
			require.Equal(t, big.NewInt(42), nonzero.CurrentTreasuryValue())
			encoded, err = nonzero.MarshalCBOR()
			require.NoError(t, err)
			require.Equal(
				t,
				[]byte{0xa2, 0x00, 0x80, 0x15, 0x18, 0x2a},
				encoded,
			)
		})
	}
}

func TestCurrentTreasuryValuePresenceRejectsTruncatedCBOR(t *testing.T) {
	truncated := []byte{0xa1, 0x15}
	for _, bodyTest := range currentTreasuryPresenceTestCases() {
		t.Run(bodyTest.name, func(t *testing.T) {
			body := bodyTest.newBody(0)
			require.Error(t, body.UnmarshalCBOR(truncated))
		})
	}
}
