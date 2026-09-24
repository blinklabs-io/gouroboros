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

package common_test

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixtures shared with the Dijkstra era decoder tests. The block supplies a
// real header plus the leios/peras certificate fields, the transaction
// supplies a real body/witness set/auxiliary data triple.
var (
	dijkstraBlockFixturePath = filepath.Join(
		"..", "dijkstra", "testdata", "musashi_dijkstra_block.hex",
	)
	dijkstraTxFixturePath = filepath.Join(
		"..", "dijkstra", "testdata", "cardano_ledger_dijkstra_w30_tx.hex",
	)
)

func readHexFixture(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	data, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	require.NoError(t, err)
	return data
}

func encodeCbor(t *testing.T, v any) cbor.RawMessage {
	t.Helper()
	data, err := cbor.Encode(v)
	require.NoError(t, err)
	return cbor.RawMessage(data)
}

func decodeCborArray(t *testing.T, data []byte) []cbor.RawMessage {
	t.Helper()
	var items []cbor.RawMessage
	_, err := cbor.Decode(data, &items)
	require.NoError(t, err)
	return items
}

func currentDijkstraFixtureTx(t *testing.T, parts []cbor.RawMessage) []cbor.RawMessage {
	t.Helper()
	require.Len(t, parts, 3)
	var bodyFields map[uint]cbor.RawMessage
	_, err := cbor.Decode(parts[0], &bodyFields)
	require.NoError(t, err)
	delete(bodyFields, 26)
	subTxBytes, exists := bodyFields[23]
	if !exists {
		return parts
	}
	var subTxs cbor.SetType[cbor.RawMessage]
	_, err = cbor.Decode(subTxBytes, &subTxs)
	require.NoError(t, err)
	updatedSubTxs := subTxs.Items()
	for index, rawSubTx := range updatedSubTxs {
		subTxParts := decodeCborArray(t, rawSubTx)
		require.Len(t, subTxParts, 3)
		var subBodyFields map[uint]cbor.RawMessage
		_, err := cbor.Decode(subTxParts[0], &subBodyFields)
		require.NoError(t, err)
		// The source vector predates CIP-159's reward-account byte-string
		// keys for account_balance_intervals. Remove the obsolete optional
		// field from this test copy so transaction-offset coverage exercises
		// the current valid body shape.
		delete(subBodyFields, 26)
		subTxParts[0] = encodeCbor(t, subBodyFields)
		updatedSubTxs[index] = encodeCbor(t, subTxParts)
	}
	bodyFields[23] = encodeCbor(
		t,
		cbor.NewSetType(updatedSubTxs, true),
	)
	parts[0] = encodeCbor(t, bodyFields)
	return parts
}

// dijkstraFixtureParts returns the header and the (nil) leios and peras
// certificate fields of the in-tree Dijkstra block fixture, together with a
// three-element transaction taken from the in-tree Dijkstra transaction
// fixture.
func dijkstraFixtureParts(
	t *testing.T,
) (header, leios, peras cbor.RawMessage, tx3 []cbor.RawMessage) {
	t.Helper()
	blockParts := decodeCborArray(t, readHexFixture(t, dijkstraBlockFixturePath))
	require.Len(t, blockParts, 2)
	bodyParts := decodeCborArray(t, []byte(blockParts[1]))
	// The in-tree fixture is in the legacy prototype-2026w27 form:
	// [invalid_transactions, transactions, leios_certificate,
	// peras_certificate].
	require.Len(t, bodyParts, 4)
	txParts := currentDijkstraFixtureTx(
		t,
		decodeCborArray(t, readHexFixture(t, dijkstraTxFixturePath)),
	)
	require.Len(t, txParts, 3)
	return blockParts[0], bodyParts[2], bodyParts[3], txParts
}

// buildDijkstraBlock assembles a Dijkstra block around numTx copies of the
// in-tree transaction fixture. When legacyBody is true the block uses the
// legacy prototype-2026w27 four-element block_body with three-element
// transactions; otherwise it uses the current three-element block_body with
// four-element block transactions carrying a trailing is_valid flag.
func buildDijkstraBlock(t *testing.T, legacyBody bool, numTx int) []byte {
	t.Helper()
	header, leios, peras, tx3 := dijkstraFixtureParts(t)
	txs := make([]cbor.RawMessage, 0, numTx)
	for i := range numTx {
		if legacyBody {
			txs = append(txs, encodeCbor(t, tx3))
			continue
		}
		// block_transaction = [body, witness_set, auxiliary_data/nil, bool].
		// Alternate is_valid so both boolean encodings are exercised.
		tx4 := make([]cbor.RawMessage, 0, 4)
		tx4 = append(tx4, tx3...)
		tx4 = append(tx4, encodeCbor(t, i%2 == 0))
		txs = append(txs, encodeCbor(t, tx4))
	}
	var body []cbor.RawMessage
	if legacyBody {
		body = []cbor.RawMessage{
			encodeCbor(t, nil), // invalid_transactions
			encodeCbor(t, txs),
			leios,
			peras,
		}
	} else {
		body = []cbor.RawMessage{
			encodeCbor(t, txs),
			leios,
			peras,
		}
	}
	return []byte(
		encodeCbor(t, []cbor.RawMessage{header, encodeCbor(t, body)}),
	)
}

// TestExtractTransactionOffsetsDijkstraBlockShapes covers both Dijkstra
// block_body arities. The legacy four-element body with three-element
// transactions is the control: it already worked. The current three-element
// body with four-element block transactions regressed to zero offsets and a
// nil error, which silently drops every transaction in the block.
func TestExtractTransactionOffsetsDijkstraBlockShapes(t *testing.T) {
	testCases := []struct {
		name       string
		legacyBody bool
		numTx      int
	}{
		{name: "legacy body, no transactions", legacyBody: true, numTx: 0},
		{name: "legacy body, one transaction", legacyBody: true, numTx: 1},
		{name: "legacy body, two transactions", legacyBody: true, numTx: 2},
		{name: "current body, no transactions", legacyBody: false, numTx: 0},
		{name: "current body, one transaction", legacyBody: false, numTx: 1},
		{name: "current body, two transactions", legacyBody: false, numTx: 2},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			blockCbor := buildDijkstraBlock(
				t,
				testCase.legacyBody,
				testCase.numTx,
			)

			// Current consensus decoding accepts only the current body shape.
			// The historical offset walker still understands the pre-respin
			// layout for archive/indexing callers.
			if !testCase.legacyBody {
				var block dijkstra.DijkstraBlock
				require.NoError(t, block.UnmarshalCBOR(blockCbor))
				require.Len(t, block.Transactions(), testCase.numTx)
			}

			offsets, err := common.ExtractTransactionOffsets(blockCbor)
			require.NoError(t, err)
			require.NotNil(t, offsets)
			require.Len(t, offsets.Transactions, testCase.numTx)

			// Every recorded range must round-trip to the exact CBOR of the
			// corresponding transaction component.
			topParts := decodeCborArray(t, blockCbor)
			bodyParts := decodeCborArray(t, []byte(topParts[1]))
			txField := 0
			if testCase.legacyBody {
				txField = 1
			}
			rawTxs := decodeCborArray(t, []byte(bodyParts[txField]))
			require.Len(t, rawTxs, testCase.numTx)
			for i, rawTx := range rawTxs {
				txParts := decodeCborArray(t, []byte(rawTx))
				if testCase.legacyBody {
					require.Len(t, txParts, 3)
				} else {
					require.Len(t, txParts, 4)
				}
				loc := offsets.Transactions[i]
				assert.Equal(
					t,
					[]byte(txParts[0]),
					blockCbor[loc.Body.Offset:loc.Body.Offset+loc.Body.Length],
					"transaction %d body bytes",
					i,
				)
				assert.Equal(
					t,
					[]byte(txParts[1]),
					blockCbor[loc.Witness.Offset:loc.Witness.Offset+loc.Witness.Length],
					"transaction %d witness bytes",
					i,
				)
				require.NotZero(t, loc.Metadata.Length)
				assert.Equal(
					t,
					[]byte(txParts[2]),
					blockCbor[loc.Metadata.Offset:loc.Metadata.Offset+loc.Metadata.Length],
					"transaction %d auxiliary data bytes",
					i,
				)
				// The trailing is_valid flag is a bool, not a byte range: no
				// recorded range may extend into it.
				if !testCase.legacyBody {
					isValidLen := uint32(len(txParts[3]))
					txEnd := loc.Metadata.Offset + loc.Metadata.Length
					assert.Equal(
						t,
						[]byte(txParts[3]),
						blockCbor[txEnd:txEnd+isValidLen],
						"transaction %d is_valid must follow the recorded ranges",
						i,
					)
				}
			}
		})
	}
}

func TestExtractTransactionOffsetsDijkstraSubTransactionComponents(t *testing.T) {
	t.Parallel()
	header, leios, peras, _ := dijkstraFixtureParts(t)
	output := encodeCbor(t, map[uint]any{0: uint64(1)})
	subBody := encodeCbor(t, map[uint]any{
		0: []any{},
		1: []cbor.RawMessage{output},
	})
	witness := encodeCbor(t, map[uint]any{})
	metadata := encodeCbor(t, map[uint]any{0: uint64(1)})
	subTransaction := encodeCbor(t, []cbor.RawMessage{subBody, witness, metadata})
	topBody := encodeCbor(t, map[uint]any{
		0:  []any{},
		1:  []any{},
		2:  uint64(0),
		23: cbor.NewSetType([]cbor.RawMessage{subTransaction}, true),
	})
	topTransaction := encodeCbor(t, []cbor.RawMessage{
		topBody,
		witness,
		encodeCbor(t, nil),
		encodeCbor(t, true),
	})
	blockBody := encodeCbor(t, []cbor.RawMessage{
		encodeCbor(t, []cbor.RawMessage{topTransaction}),
		leios,
		peras,
	})
	block := []byte(encodeCbor(t, []cbor.RawMessage{header, blockBody}))

	offsets, err := common.ExtractTransactionOffsets(block)
	require.NoError(t, err)
	require.Len(t, offsets.Transactions, 1)
	require.Len(t, offsets.Transactions[0].SubTransactions, 1)
	subLoc := offsets.Transactions[0].SubTransactions[0]
	assert.Equal(t, []byte(subBody), block[subLoc.Body.Offset:subLoc.Body.Offset+subLoc.Body.Length])
	assert.Equal(t, []byte(witness), block[subLoc.Witness.Offset:subLoc.Witness.Offset+subLoc.Witness.Length])
	assert.Equal(t, []byte(metadata), block[subLoc.Metadata.Offset:subLoc.Metadata.Offset+subLoc.Metadata.Length])
	require.Len(t, subLoc.Outputs, 1)
	outputLoc := subLoc.Outputs[0]
	assert.Equal(t, []byte(output), block[outputLoc.Offset:outputLoc.Offset+outputLoc.Length])
}

func TestExtractTransactionOffsetsDijkstraRejectsMalformedSubTransaction(t *testing.T) {
	t.Parallel()
	header, leios, peras, _ := dijkstraFixtureParts(t)
	malformedSubTransaction := encodeCbor(t, []any{map[uint]any{}, map[uint]any{}})
	topBody := encodeCbor(t, map[uint]any{
		0:  []any{},
		1:  []any{},
		2:  uint64(0),
		23: cbor.NewSetType([]cbor.RawMessage{malformedSubTransaction}, true),
	})
	topTransaction := encodeCbor(t, []cbor.RawMessage{
		topBody,
		encodeCbor(t, map[uint]any{}),
		encodeCbor(t, nil),
		encodeCbor(t, true),
	})
	blockBody := encodeCbor(t, []cbor.RawMessage{
		encodeCbor(t, []cbor.RawMessage{topTransaction}),
		leios,
		peras,
	})
	block := []byte(encodeCbor(t, []cbor.RawMessage{header, blockBody}))

	_, err := common.ExtractTransactionOffsets(block)
	require.ErrorContains(t, err, "sub-transaction 0 has 2 components, expected 3")
}

func TestExtractTransactionOffsetsDijkstraReturnsInvalidTransactions(t *testing.T) {
	header, leios, peras, tx3 := dijkstraFixtureParts(t)
	txs := []cbor.RawMessage{
		encodeCbor(t, tx3),
		encodeCbor(t, tx3),
	}
	body := []cbor.RawMessage{
		encodeCbor(t, []uint{1}), // invalid_transactions
		encodeCbor(t, txs),
		leios,
		peras,
	}
	blockCbor := []byte(encodeCbor(t, []cbor.RawMessage{
		header,
		encodeCbor(t, body),
	}))

	offsets, err := common.ExtractTransactionOffsets(blockCbor)
	require.NoError(t, err)
	require.Len(t, offsets.Transactions, 2)
	require.Equal(t, []uint{1}, offsets.InvalidTransactions)
}

func TestExtractTransactionOffsetsDijkstraRejectsDuplicateInvalidTransactions(
	t *testing.T,
) {
	header, leios, peras, tx3 := dijkstraFixtureParts(t)
	txs := []cbor.RawMessage{
		encodeCbor(t, tx3),
		encodeCbor(t, tx3),
	}
	body := []cbor.RawMessage{
		encodeCbor(t, cbor.NewSetType([]uint{1, 1}, false)),
		encodeCbor(t, txs),
		leios,
		peras,
	}
	blockCbor := []byte(encodeCbor(t, []cbor.RawMessage{
		header,
		encodeCbor(t, body),
	}))

	offsets, err := common.ExtractTransactionOffsets(blockCbor)
	require.ErrorContains(t, err, "duplicate member in set")
	require.Nil(t, offsets)
}

// TestExtractTransactionOffsetsDijkstraMalformed verifies that a block which
// can only be a Dijkstra block but whose shape is not understood fails loudly.
// Returning an empty offset set with a nil error lets a consumer store a block
// with none of its transactions indexed.
func TestExtractTransactionOffsetsDijkstraMalformed(t *testing.T) {
	header, leios, peras, tx3 := dijkstraFixtureParts(t)
	tx5 := make([]cbor.RawMessage, 0, 5)
	tx5 = append(tx5, tx3...)
	tx5 = append(tx5, encodeCbor(t, true), encodeCbor(t, true))
	oneTx5 := encodeCbor(t, []cbor.RawMessage{encodeCbor(t, tx5)})

	testCases := []struct {
		name string
		body []cbor.RawMessage
	}{
		{
			name: "two element block body",
			body: []cbor.RawMessage{
				encodeCbor(t, []cbor.RawMessage{encodeCbor(t, tx3)}),
				leios,
			},
		},
		{
			name: "five element block body",
			body: []cbor.RawMessage{
				encodeCbor(t, nil),
				encodeCbor(t, []cbor.RawMessage{encodeCbor(t, tx3)}),
				leios,
				peras,
				encodeCbor(t, nil),
			},
		},
		{
			name: "current body with five element transaction",
			body: []cbor.RawMessage{oneTx5, leios, peras},
		},
		{
			name: "legacy body with five element transaction",
			body: []cbor.RawMessage{encodeCbor(t, nil), oneTx5, leios, peras},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			blockCbor := []byte(encodeCbor(t, []cbor.RawMessage{
				header,
				encodeCbor(t, testCase.body),
			}))
			offsets, err := common.ExtractTransactionOffsets(blockCbor)
			require.Error(t, err)
			assert.Nil(t, offsets)
		})
	}
}

// TestExtractTransactionOffsetsTwoElementNonDijkstra pins the fall-through for
// two-element values that are not Dijkstra blocks: their second element does
// not hold a transactions array, so they are not misclassified and keep
// returning an empty offset set.
func TestExtractTransactionOffsetsTwoElementNonDijkstra(t *testing.T) {
	testCases := []struct {
		name  string
		block []any
	}{
		{
			name:  "body is not an array",
			block: []any{[]byte{0x01}, []byte{0x02, 0x03}},
		},
		{
			name:  "body holds bytestrings, not transactions",
			block: []any{[]byte{0x01}, []any{[]byte{0x02}, []byte{0x03}, []byte{0x04}}},
		},
		{
			name:  "malformed body with unrelated header",
			block: []any{[]byte{0x01}, []any{[]any{}, nil}},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			blockCbor, err := cbor.Encode(testCase.block)
			require.NoError(t, err)
			offsets, err := common.ExtractTransactionOffsets(blockCbor)
			require.NoError(t, err)
			require.NotNil(t, offsets)
			assert.Empty(t, offsets.Transactions)
		})
	}
}
