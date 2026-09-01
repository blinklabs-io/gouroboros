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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamingBlockDecoder(t *testing.T) {
	// Test with a minimal valid CBOR block structure
	// Block structure: [header, tx_bodies[], witnesses[], metadata_map, invalid_txs[]]

	// This is a simplified test - in production, use real block data
	// For now, test that the decoder handles the basic structure

	t.Run("empty block array", func(t *testing.T) {
		// CBOR array with 5 empty elements: [[], [], [], {}, []]
		// 85 = array(5)
		// 80 = array(0) - header placeholder
		// 80 = array(0) - tx bodies
		// 80 = array(0) - witnesses
		// a0 = map(0) - metadata
		// 80 = array(0) - invalid txs
		data := []byte{0x85, 0x80, 0x80, 0x80, 0xa0, 0x80}

		decoder, err := common.NewStreamingBlockDecoder(data)
		require.NoError(t, err, "creating decoder should succeed")

		offsets, err := decoder.DecodeWithOffsets()
		require.NoError(t, err, "decoding should succeed")

		assert.NotNil(t, offsets, "offsets should not be nil")
		assert.Empty(t, offsets.Transactions, "should have no transactions")
	})

	t.Run("invalid CBOR", func(t *testing.T) {
		data := []byte{0xff, 0xff, 0xff}

		decoder, err := common.NewStreamingBlockDecoder(data)
		require.NoError(t, err, "creating decoder should succeed")

		_, err = decoder.DecodeWithOffsets()
		assert.Error(t, err, "decoding invalid CBOR should fail")
	})
}

func TestExtractOutputCbor(t *testing.T) {
	t.Run("valid extraction", func(t *testing.T) {
		// Create mock block data and offsets
		blockData := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09}
		offsets := &common.BlockTransactionOffsets{
			Transactions: []common.TransactionLocation{
				{
					Body: common.ByteRange{Offset: 0, Length: 4},
					Outputs: []common.ByteRange{
						{Offset: 2, Length: 2}, // bytes 0x02, 0x03
						{Offset: 4, Length: 3}, // bytes 0x04, 0x05, 0x06
					},
				},
			},
		}

		// Extract first output
		output0, err := common.ExtractOutputCbor(blockData, offsets, 0, 0)
		require.NoError(t, err)
		assert.Equal(t, []byte{0x02, 0x03}, output0)

		// Extract second output
		output1, err := common.ExtractOutputCbor(blockData, offsets, 0, 1)
		require.NoError(t, err)
		assert.Equal(t, []byte{0x04, 0x05, 0x06}, output1)
	})

	t.Run("transaction index out of range", func(t *testing.T) {
		blockData := []byte{0x00, 0x01, 0x02, 0x03}
		offsets := &common.BlockTransactionOffsets{
			Transactions: []common.TransactionLocation{},
		}

		_, err := common.ExtractOutputCbor(blockData, offsets, 0, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "transaction index")
	})

	t.Run("output index out of range", func(t *testing.T) {
		blockData := []byte{0x00, 0x01, 0x02, 0x03}
		offsets := &common.BlockTransactionOffsets{
			Transactions: []common.TransactionLocation{
				{
					Outputs: []common.ByteRange{},
				},
			},
		}

		_, err := common.ExtractOutputCbor(blockData, offsets, 0, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "output index")
	})
}

func TestExtractTransactionBodyCbor(t *testing.T) {
	blockData := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}
	offsets := &common.BlockTransactionOffsets{
		Transactions: []common.TransactionLocation{
			{
				Body: common.ByteRange{Offset: 2, Length: 4}, // bytes 0x02-0x05
			},
		},
	}

	body, err := common.ExtractTransactionBodyCbor(blockData, offsets, 0)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x02, 0x03, 0x04, 0x05}, body)
}

func TestExtractWitnessCbor(t *testing.T) {
	blockData := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}
	offsets := &common.BlockTransactionOffsets{
		Transactions: []common.TransactionLocation{
			{
				Witness: common.ByteRange{Offset: 3, Length: 3}, // bytes 0x03-0x05
			},
		},
	}

	witness, err := common.ExtractWitnessCbor(blockData, offsets, 0)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x03, 0x04, 0x05}, witness)
}

// TestExtractTransactionOffsetsDijkstra verifies that ExtractTransactionOffsets
// recognizes a Dijkstra (prototype-2026w27) 2-element block
// [header, block_body] and extracts one offset entry per inline transaction,
// with each extracted body/witness/metadata slice round-tripping to the exact
// CBOR of the corresponding transaction element.
func TestExtractTransactionOffsetsDijkstra(t *testing.T) {
	// tx0: no metadata; tx1: carries auxiliary_data (metadata) and an output.
	tx0 := []any{
		map[uint64]any{2: uint64(10)}, // body: {fee: 10}
		map[uint64]any{},              // empty witness set
		nil,                           // no auxiliary_data
	}
	tx1 := []any{
		map[uint64]any{ // body: {outputs: [out], fee: 20}
			1: []any{[]any{[]byte{0xde, 0xad}, uint64(5)}},
			2: uint64(20),
		},
		map[uint64]any{0: []any{}},          // witness set with empty vkey witness list
		map[uint64]any{uint64(674): "meta"}, // auxiliary_data (metadata)
	}
	// A realistic 2-element header placeholder ([body, signature]); its contents
	// are irrelevant to offset extraction, only its byte length matters.
	header := []any{[]byte{0x01, 0x02, 0x03}, []byte{0x04}}
	blockBody := []any{nil, []any{tx0, tx1}, nil, nil}
	blockCbor, err := cbor.Encode([]any{header, blockBody})
	require.NoError(t, err)

	offsets, err := common.ExtractTransactionOffsets(blockCbor)
	require.NoError(t, err)
	require.Len(t, offsets.Transactions, 2)

	// Independently decode to obtain the authoritative sub-element bytes.
	var top []cbor.RawMessage
	_, err = cbor.Decode(blockCbor, &top)
	require.NoError(t, err)
	require.Len(t, top, 2)
	var bodyParts []cbor.RawMessage
	_, err = cbor.Decode([]byte(top[1]), &bodyParts)
	require.NoError(t, err)
	require.Len(t, bodyParts, 4)
	var txs []cbor.RawMessage
	_, err = cbor.Decode([]byte(bodyParts[1]), &txs)
	require.NoError(t, err)
	require.Len(t, txs, 2)

	for i, rawTx := range txs {
		var txParts []cbor.RawMessage
		_, err = cbor.Decode([]byte(rawTx), &txParts)
		require.NoError(t, err)
		require.Len(t, txParts, 3)
		loc := offsets.Transactions[i]

		gotBody := blockCbor[loc.Body.Offset : loc.Body.Offset+loc.Body.Length]
		assert.Equal(t, []byte(txParts[0]), gotBody, "tx %d body bytes", i)

		gotWit := blockCbor[loc.Witness.Offset : loc.Witness.Offset+loc.Witness.Length]
		assert.Equal(t, []byte(txParts[1]), gotWit, "tx %d witness bytes", i)
	}

	// tx0 has no auxiliary_data -> zero Metadata range.
	assert.Zero(t, offsets.Transactions[0].Metadata.Length)
	// tx1 carries metadata; the recorded range must round-trip to its aux bytes.
	var tx1Parts []cbor.RawMessage
	_, err = cbor.Decode([]byte(txs[1]), &tx1Parts)
	require.NoError(t, err)
	require.Len(t, tx1Parts, 3)
	m := offsets.Transactions[1].Metadata
	require.NotZero(t, m.Length)
	assert.Equal(t, []byte(tx1Parts[2]), blockCbor[m.Offset:m.Offset+m.Length])

	// tx1's single output must also round-trip.
	require.Len(t, offsets.Transactions[1].Outputs, 1)
	var body1Parts map[uint64]cbor.RawMessage
	_, err = cbor.Decode([]byte(tx1Parts[0]), &body1Parts)
	require.NoError(t, err)
	var outputs []cbor.RawMessage
	_, err = cbor.Decode([]byte(body1Parts[1]), &outputs)
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	o := offsets.Transactions[1].Outputs[0]
	assert.Equal(t, []byte(outputs[0]), blockCbor[o.Offset:o.Offset+o.Length])
}

func TestExtractTransactionOffsetsDijkstraUsesWireOffsets(t *testing.T) {
	nonMinimalArray := func(items ...[]byte) []byte {
		ret := []byte{0x98, byte(len(items))}
		for _, item := range items {
			ret = append(ret, item...)
		}
		return ret
	}

	txBody := []byte{
		0xa2,             // map(2)
		0x01,             // outputs
		0x81,             // array(1)
		0x82,             // legacy output array(2)
		0x42, 0xde, 0xad, // address bytes
		0x05,       // amount
		0x02, 0x14, // fee
	}
	plutusV4Script := []byte{0x42, 0x41, 0x00}
	txWitness := append(
		[]byte{
			0xa1, // map(1)
			0x08, // Plutus V4 scripts
			0x81, // array(1)
		},
		plutusV4Script...,
	)
	txMetadata := []byte{
		0xa1,             // map(1)
		0x19, 0x02, 0xa2, // 674
		0x64, 'm', 'e', 't', 'a',
	}
	tx := nonMinimalArray(txBody, txWitness, txMetadata)
	txs := nonMinimalArray(tx)
	blockBody := nonMinimalArray([]byte{0xf6}, txs, []byte{0xf6}, []byte{0xf6})
	blockCbor := nonMinimalArray([]byte{0x80}, blockBody)

	offsets, err := common.ExtractTransactionOffsets(blockCbor)
	require.NoError(t, err)
	require.Len(t, offsets.Transactions, 1)

	loc := offsets.Transactions[0]
	assert.Equal(
		t,
		txBody,
		blockCbor[loc.Body.Offset:loc.Body.Offset+loc.Body.Length],
	)
	assert.Equal(
		t,
		txWitness,
		blockCbor[loc.Witness.Offset:loc.Witness.Offset+loc.Witness.Length],
	)
	assert.Equal(
		t,
		txMetadata,
		blockCbor[loc.Metadata.Offset:loc.Metadata.Offset+loc.Metadata.Length],
	)

	require.Len(t, loc.Outputs, 1)
	output := loc.Outputs[0]
	assert.Equal(
		t,
		txBody[3:8],
		blockCbor[output.Offset:output.Offset+output.Length],
	)

	scriptHash := common.Blake2b224Hash(
		append([]byte{common.ScriptRefTypePlutusV4}, plutusV4Script...),
	)
	scriptRange, ok := loc.Scripts[scriptHash]
	require.True(t, ok)
	assert.Equal(
		t,
		plutusV4Script,
		blockCbor[scriptRange.Offset:scriptRange.Offset+scriptRange.Length],
	)
}
