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
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/ouroboros-mock/fixtures"
	"github.com/stretchr/testify/require"
)

func assertCombinedOffsets(t *testing.T, block common.Block) {
	t.Helper()
	data := block.Cbor()
	if len(data) == 0 {
		t.Fatal("fixture block has no CBOR")
	}
	expected, err := common.ExtractTransactionOffsets(data)
	require.NoError(t, err)
	combined, err := ledger.NewBlockFromCborWithOffsets(
		uint(block.Type()),
		data,
	)
	require.NoError(t, err)
	require.Equal(t, expected, combined.Offsets)
	txs := combined.Block.Transactions()
	require.Len(t, combined.Offsets.Transactions, len(txs))
	flags := common.TransactionValidityFlags(
		len(txs),
		expected.InvalidTransactions,
	)
	for i, tx := range txs {
		require.Equal(t, tx.IsValid(), flags[i])
		loc := combined.Offsets.Transactions[i]
		assertTransactionRanges(
			t,
			data,
			tx.Cbor(),
			loc,
			tx.Type() == dijkstra.TxTypeDijkstra,
		)
		require.Len(t, loc.Outputs, len(tx.Outputs()))
		for j, output := range tx.Outputs() {
			require.Equal(
				t,
				output.Cbor(),
				offsetBytes(t, data, loc.Outputs[j]),
			)
		}
		if dtx, ok := tx.(*dijkstra.DijkstraTransaction); ok {
			subs := dtx.Body.TxSubTransactions.Items()
			require.Len(t, loc.SubTransactions, len(subs))
			for j, sub := range subs {
				subLoc := loc.SubTransactions[j]
				assertTransactionRanges(t, data, sub.Cbor(), subLoc, true)
				require.Len(t, subLoc.Outputs, len(sub.Body.TxOutputs))
				for k, output := range sub.Body.TxOutputs {
					require.Equal(
						t,
						output.Cbor(),
						offsetBytes(t, data, subLoc.Outputs[k]),
					)
				}
			}
		}
	}
}

func offsetBytes(t *testing.T, data []byte, loc common.ByteRange) []byte {
	t.Helper()
	end := uint64(loc.Offset) + uint64(loc.Length)
	require.LessOrEqual(t, end, uint64(len(data)))
	return data[uint64(loc.Offset):end]
}

func assertTransactionRanges(
	t *testing.T, block, tx []byte, loc common.TransactionLocation,
	dijkstraLayout bool,
) {
	t.Helper()
	var parts []cbor.RawMessage
	_, err := cbor.Decode(tx, &parts)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(parts), 3)
	require.Equal(t, []byte(parts[0]), offsetBytes(t, block, loc.Body))
	require.Equal(t, []byte(parts[1]), offsetBytes(t, block, loc.Witness))
	metadata := parts[len(parts)-1]
	if dijkstraLayout || len(parts) == 3 {
		metadata = parts[2]
	}
	if len(metadata) == 1 && metadata[0] == 0xf6 {
		require.Zero(t, loc.Metadata.Length)
	} else {
		require.Equal(t, []byte(metadata), offsetBytes(t, block, loc.Metadata))
	}
}

func TestCombinedOffsetsEmptyBlocks(t *testing.T) {
	for _, era := range fixtures.SupportedEras() {
		if era.Id == 0 {
			continue
		}
		t.Run(era.Name, func(t *testing.T) {
			block, err := fixtures.GenerateBlock(era, 1, 0)
			require.NoError(t, err)
			assertCombinedOffsets(t, block)
		})
	}
}

func TestCombinedOffsetsTransactions(t *testing.T) {
	blocks, err := fixtures.GenerateConwayChainWithTransactions(
		1, common.Blake2b256{}, 1, 1, 1,
	)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	transactions := blocks[0].Transactions()
	if len(transactions) != 1 {
		t.Fatalf("expected one fixture transaction, got %d", len(transactions))
	}
	source := transactions[0].(*conway.ConwayTransaction)
	invalid := *source
	invalid.TxIsValid = false
	invalid.SetCbor(nil)
	block, err := fixtures.NewBlockBuilder(ledger.GetEraById(6)).
		WithTransactions(source, &invalid).Build()
	require.NoError(t, err)
	t.Run("Conway", func(t *testing.T) { assertCombinedOffsets(t, block) })

	for _, eraID := range []uint8{4, 5} {
		era := ledger.GetEraById(eraID)
		t.Run(era.Name, func(t *testing.T) {
			var txs []common.Transaction
			for _, valid := range []bool{true, false} {
				inputs := shelley.NewShelleyTransactionInputSet(
					source.Body.TxInputs.Items(),
				)
				if eraID == 4 {
					output := source.Body.TxOutputs[0]
					txs = append(txs, &alonzo.AlonzoTransaction{
						Body: alonzo.AlonzoTransactionBody{
							TxInputs: inputs,
							TxOutputs: []alonzo.AlonzoTransactionOutput{{
								OutputAddress: output.OutputAddress,
								OutputAmount:  output.OutputAmount,
							}},
						},
						TxIsValid: valid,
					})
				} else {
					txs = append(txs, &babbage.BabbageTransaction{
						Body: babbage.BabbageTransactionBody{
							TxInputs: inputs, TxOutputs: source.Body.TxOutputs,
						},
						TxIsValid: valid,
					})
				}
			}
			block, err := fixtures.NewBlockBuilder(era).
				WithTransactions(txs...).Build()
			require.NoError(t, err)
			assertCombinedOffsets(t, block)
		})
	}

	metadataRaw, err := cbor.Encode(map[uint]string{0: "offset metadata"})
	require.NoError(t, err)
	metadata, err := common.DecodeAuxiliaryDataToMetadata(metadataRaw)
	require.NoError(t, err)
	output := dijkstra.DijkstraTransactionOutput{Output: source.Outputs()[0]}
	sub := dijkstra.DijkstraSubTransaction{
		Body: dijkstra.DijkstraSubTransactionBody{
			TxInputs:  source.Body.TxInputs,
			TxOutputs: []dijkstra.DijkstraTransactionOutput{output},
		},
		TxMetadata: metadata,
	}
	var dtxs []dijkstra.DijkstraTransaction
	for _, valid := range []bool{true, false} {
		tx, err := fixtures.NewDijkstraTransactionBuilder().
			WithBody(dijkstra.DijkstraTransactionBody{
				TxInputs:  source.Body.TxInputs,
				TxOutputs: []dijkstra.DijkstraTransactionOutput{output},
			}).WithSubTransactions(sub).WithMetadata(metadata).
			WithTxIsValid(valid).Build()
		require.NoError(t, err)
		dtxs = append(dtxs, *tx)
	}
	dblock, err := fixtures.NewDijkstraBlockBuilder().
		WithTransactions(dtxs...).Build()
	require.NoError(t, err)
	t.Run("Dijkstra", func(t *testing.T) { assertCombinedOffsets(t, dblock) })
}

func TestDecodeBlockWithOffsetsRejectsTransactionCountMismatch(t *testing.T) {
	blocks, err := fixtures.GenerateConwayChainWithTransactions(
		1, common.Blake2b256{}, 1, 1, 1,
	)
	require.NoError(t, err)
	empty, err := fixtures.GenerateBlock(ledger.GetEraById(6), 1, 0)
	require.NoError(t, err)
	_, err = common.DecodeBlockWithOffsets(uint(empty.Type()), empty.Cbor(),
		func(uint, []byte) (common.Block, error) { return blocks[0], nil },
	)
	require.ErrorContains(t, err, "transaction count mismatch")
}
