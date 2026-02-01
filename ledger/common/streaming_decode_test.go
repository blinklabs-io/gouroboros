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
