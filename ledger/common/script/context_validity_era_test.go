// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package script_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

// conwayTransaction re-decodes the shared validity fixture as a Conway
// transaction. The fixture body only sets the validity interval keys, so the
// same CBOR is valid in every post-Shelley era.
func conwayTransaction(
	t *testing.T,
	fixture mockledger.ValidityIntervalFixture,
) *conway.ConwayTransaction {
	t.Helper()
	base, err := fixture.AlonzoTransaction()
	require.NoError(t, err)
	tx, err := conway.NewConwayTransactionFromCbor(base.Cbor())
	require.NoError(t, err)
	return tx
}

// dijkstraTransaction re-decodes the shared validity fixture as a Dijkstra
// transaction, for the same reason as conwayTransaction.
func dijkstraTransaction(
	t *testing.T,
	fixture mockledger.ValidityIntervalFixture,
) *dijkstra.DijkstraTransaction {
	t.Helper()
	base, err := fixture.AlonzoTransaction()
	require.NoError(t, err)
	tx, err := dijkstra.NewDijkstraTransactionFromCbor(base.Cbor())
	require.NoError(t, err)
	return tx
}

// eraTxBuilder decodes a shared validity fixture as one era's transaction
// type.
type eraTxBuilder func(
	*testing.T,
	mockledger.ValidityIntervalFixture,
) lcommon.Transaction

// validRangeBuilder builds a TxInfo of one Plutus version and renders its
// validity range.
type validRangeBuilder func(
	lcommon.SlotState,
	lcommon.Transaction,
	[]lcommon.Utxo,
) (data.PlutusData, error)

// TestValidityRangeEraIdsUnchanged pins the transaction type values used by
// the shared validity fixtures.
func TestValidityRangeEraIdsUnchanged(t *testing.T) {
	require.Equal(t, 4, alonzo.TxTypeAlonzo)
	require.Equal(t, 5, babbage.TxTypeBabbage)
	require.Equal(t, 6, conway.TxTypeConway)
	require.Equal(t, 7, dijkstra.TxTypeDijkstra)
}

// TestValidityRangeUpperBoundByEra pins the validity-interval encoding per
// era for every bound-presence combination. The upper-bound-only cases are
// the ones that differ: closed in Alonzo and Babbage, exclusive from Conway
// on.
func TestValidityRangeUpperBoundByEra(t *testing.T) {
	for _, era := range []struct {
		name string
		tx   eraTxBuilder
	}{
		{
			name: "Alonzo",
			tx: func(
				t *testing.T,
				f mockledger.ValidityIntervalFixture,
			) lcommon.Transaction {
				tx, err := f.AlonzoTransaction()
				require.NoError(t, err)
				require.Equal(t, alonzo.TxTypeAlonzo, tx.Type())
				return tx
			},
		},
		{
			name: "Babbage",
			tx: func(
				t *testing.T,
				f mockledger.ValidityIntervalFixture,
			) lcommon.Transaction {
				tx, err := f.BabbageTransaction()
				require.NoError(t, err)
				require.Equal(t, babbage.TxTypeBabbage, tx.Type())
				return tx
			},
		},
		{
			name: "Conway",
			tx: func(
				t *testing.T,
				f mockledger.ValidityIntervalFixture,
			) lcommon.Transaction {
				tx := conwayTransaction(t, f)
				require.Equal(t, conway.TxTypeConway, tx.Type())
				return tx
			},
		},
		{
			name: "Dijkstra",
			tx: func(
				t *testing.T,
				f mockledger.ValidityIntervalFixture,
			) lcommon.Transaction {
				tx := dijkstraTransaction(t, f)
				require.Equal(t, dijkstra.TxTypeDijkstra, tx.Type())
				return tx
			},
		},
	} {
		t.Run(era.name, func(t *testing.T) {
			for _, build := range []struct {
				name string
				fn   validRangeBuilder
			}{
				{
					name: "V1",
					fn: func(
						s lcommon.SlotState,
						tx lcommon.Transaction,
						u []lcommon.Utxo,
					) (data.PlutusData, error) {
						info, err := script.NewTxInfoV1FromTransaction(
							s, tx, u,
							script.StrictValidityUpperBoundForTransaction(tx),
						)
						if err != nil {
							return nil, err
						}
						return info.ValidRange.ToPlutusData(), nil
					},
				},
				{
					name: "V2",
					fn: func(
						s lcommon.SlotState,
						tx lcommon.Transaction,
						u []lcommon.Utxo,
					) (data.PlutusData, error) {
						info, err := script.NewTxInfoV2FromTransaction(
							s, tx, u,
							script.StrictValidityUpperBoundForTransaction(tx),
						)
						if err != nil {
							return nil, err
						}
						return info.ValidRange.ToPlutusData(), nil
					},
				},
				{
					name: "V3",
					fn: func(
						s lcommon.SlotState,
						tx lcommon.Transaction,
						u []lcommon.Utxo,
					) (data.PlutusData, error) {
						info, err := script.NewTxInfoV3FromTransaction(s, tx, u)
						if err != nil {
							return nil, err
						}
						return info.ValidRange.ToPlutusData(), nil
					},
				},
			} {
				t.Run(build.name, func(t *testing.T) {
					for _, fixture := range mockledger.ValidityIntervalFixtures() {
						t.Run(fixture.Name, func(t *testing.T) {
							actual, err := build.fn(
								validitySlotState{},
								era.tx(t, fixture),
								nil,
							)
							require.NoError(t, err)
							expected := expectedValidityRange(
								fixture.StartSlot,
								fixture.EndSlot,
							)
							require.True(
								t,
								expected.Equal(actual),
								"validity range mismatch:\n got: %s\nwant: %s",
								actual,
								expected,
							)
						})
					}
				})
			}
		})
	}
}
