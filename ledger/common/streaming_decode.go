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

package common

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// BlockWithOffsets pairs a decoded block with its pre-computed CBOR offsets.
// This enables efficient extraction of transaction/UTxO data without re-parsing.
type BlockWithOffsets struct {
	// Block is the fully decoded block
	Block Block

	// Offsets contains byte positions for all transactions and their components
	Offsets *BlockTransactionOffsets
}

// OutputLocation represents the byte location of a transaction output within block CBOR.
type OutputLocation struct {
	// TxIndex is the transaction index within the block
	TxIndex int

	// OutputIndex is the output index within the transaction
	OutputIndex int

	// Range is the byte range of the output CBOR
	Range ByteRange
}

// BlockOutputOffsets contains byte offsets for all outputs in a block.
type BlockOutputOffsets struct {
	// Outputs maps (tx_index, output_index) to byte location
	Outputs []OutputLocation
}

// StreamingBlockDecoder extracts CBOR offsets using the shared block walker.
type StreamingBlockDecoder struct {
	data   []byte
	stream *cbor.StreamDecoder
}

const (
	minBlockComponents = 2
	maxBlockComponents = 5
)

// decodeBlockArray returns the original encoding of each top-level block
// component without allowing the CBOR decoder to allocate from an untrusted
// array length. Supported blocks have two through five components.
func decodeBlockArray(
	data []byte,
	stream *cbor.StreamDecoder,
) ([]cbor.RawMessage, uint32, error) {
	componentCount, headerSize, indefinite := cbor.ArrayInfo(data)
	if componentCount < 0 {
		return nil, 0, errors.New("invalid block array header")
	}
	if !indefinite &&
		(componentCount < minBlockComponents ||
			componentCount > maxBlockComponents) {
		return nil, 0, fmt.Errorf(
			"invalid block component count %d",
			componentCount,
		)
	}
	if err := stream.Advance(int(headerSize)); err != nil {
		return nil, 0, err
	}

	capacity := componentCount
	if indefinite {
		capacity = maxBlockComponents
	}
	components := make([]cbor.RawMessage, 0, capacity)
	for indefinite || len(components) < componentCount {
		position := stream.Position()
		if position >= len(data) {
			return nil, 0, errors.New("unterminated block array")
		}
		if indefinite && data[position] == 0xff {
			if err := stream.Advance(1); err != nil {
				return nil, 0, err
			}
			break
		}
		if len(components) == maxBlockComponents {
			return nil, 0, fmt.Errorf(
				"invalid block component count: more than %d",
				maxBlockComponents,
			)
		}
		offset, length, err := stream.Skip()
		if err != nil {
			return nil, 0, err
		}
		components = append(
			components,
			cbor.RawMessage(data[offset:offset+length]),
		)
	}
	if len(components) < minBlockComponents {
		return nil, 0, fmt.Errorf(
			"invalid block component count %d",
			len(components),
		)
	}
	if stream.Position() != len(data) {
		return nil, 0, errors.New("unexpected trailing CBOR data")
	}
	return components, headerSize, nil
}

func decodeBlockArrayBytes(
	data []byte,
) ([]cbor.RawMessage, uint32, error) {
	stream, err := cbor.NewStreamDecoder(data)
	if err != nil {
		return nil, 0, err
	}
	return decodeBlockArray(data, stream)
}

// NewStreamingBlockDecoder creates a decoder for the given block CBOR.
func NewStreamingBlockDecoder(data []byte) (*StreamingBlockDecoder, error) {
	stream, err := cbor.NewStreamDecoder(data)
	if err != nil {
		return nil, fmt.Errorf("create stream decoder: %w", err)
	}

	return &StreamingBlockDecoder{data: data, stream: stream}, nil
}

// DecodeWithOffsets extracts the same transaction and component offsets as
// ExtractTransactionOffsets, including transaction validity and nested bodies.
func (d *StreamingBlockDecoder) DecodeWithOffsets() (*BlockTransactionOffsets, error) {
	blockArray, arrayHeaderSize, err := decodeBlockArray(d.data, d.stream)
	if err != nil {
		return nil, fmt.Errorf("decode block array: %w", err)
	}
	return extractTransactionOffsets(d.data, blockArray, arrayHeaderSize)
}

// DecodeBlockWithOffsets decodes a block and extracts CBOR offsets.
// This performs two passes: one for offset extraction, one for block decoding.
//
// Parameters:
//   - blockType: The block type ID (e.g., BlockTypeConway)
//   - data: Raw block CBOR bytes
//   - decodeBlock: Function to decode the block into its typed struct
//
// Returns:
//   - BlockWithOffsets containing both the decoded block and offset information
//   - error if decoding fails
func DecodeBlockWithOffsets(
	blockType uint,
	data []byte,
	decodeBlock func(uint, []byte) (Block, error),
) (*BlockWithOffsets, error) {
	// First, extract offsets using streaming decoder
	decoder, err := NewStreamingBlockDecoder(data)
	if err != nil {
		return nil, fmt.Errorf("create streaming decoder: %w", err)
	}

	offsets, err := decoder.DecodeWithOffsets()
	if err != nil {
		return nil, fmt.Errorf("extract offsets: %w", err)
	}

	// Then decode the block using the era-specific decoder
	// This is still needed because the streaming decoder doesn't fully
	// populate the block struct - it focuses on offset extraction
	block, err := decodeBlock(blockType, data)
	if err != nil {
		return nil, fmt.Errorf("decode block: %w", err)
	}

	if count := len(block.Transactions()); count != len(offsets.Transactions) {
		return nil, fmt.Errorf(
			"transaction count mismatch: decoded %d transactions, extracted %d offsets",
			count,
			len(offsets.Transactions),
		)
	}

	return &BlockWithOffsets{
		Block:   block,
		Offsets: offsets,
	}, nil
}

// ExtractOutputCbor extracts the raw CBOR for a specific output from block data.
// This uses pre-computed offsets for efficient extraction without re-parsing.
func ExtractOutputCbor(
	blockData []byte,
	offsets *BlockTransactionOffsets,
	txIndex int,
	outputIndex int,
) ([]byte, error) {
	// Validate inputs
	if offsets == nil {
		return nil, errors.New("offsets is nil")
	}
	if txIndex < 0 {
		return nil, fmt.Errorf("transaction index %d is negative", txIndex)
	}
	if outputIndex < 0 {
		return nil, fmt.Errorf("output index %d is negative", outputIndex)
	}
	if txIndex >= len(offsets.Transactions) {
		return nil, fmt.Errorf("transaction index %d out of range", txIndex)
	}

	txLoc := offsets.Transactions[txIndex]
	if outputIndex >= len(txLoc.Outputs) {
		return nil, fmt.Errorf(
			"output index %d out of range for transaction %d",
			outputIndex,
			txIndex,
		)
	}

	outputRange := txLoc.Outputs[outputIndex]

	// Check for overflow in offset + length calculation
	end := uint64(outputRange.Offset) + uint64(outputRange.Length)
	if end > uint64(len(blockData)) {
		return nil, errors.New("output range exceeds block data")
	}

	return blockData[outputRange.Offset : outputRange.Offset+outputRange.Length], nil
}

// ExtractTransactionBodyCbor extracts the raw CBOR for a transaction body.
func ExtractTransactionBodyCbor(
	blockData []byte,
	offsets *BlockTransactionOffsets,
	txIndex int,
) ([]byte, error) {
	// Validate inputs
	if offsets == nil {
		return nil, errors.New("offsets is nil")
	}
	if txIndex < 0 {
		return nil, fmt.Errorf("transaction index %d is negative", txIndex)
	}
	if txIndex >= len(offsets.Transactions) {
		return nil, fmt.Errorf("transaction index %d out of range", txIndex)
	}

	bodyRange := offsets.Transactions[txIndex].Body

	// Check for overflow in offset + length calculation
	end := uint64(bodyRange.Offset) + uint64(bodyRange.Length)
	if end > uint64(len(blockData)) {
		return nil, errors.New("body range exceeds block data")
	}

	return blockData[bodyRange.Offset : bodyRange.Offset+bodyRange.Length], nil
}

// ExtractWitnessCbor extracts the raw CBOR for a transaction witness set.
func ExtractWitnessCbor(
	blockData []byte,
	offsets *BlockTransactionOffsets,
	txIndex int,
) ([]byte, error) {
	// Validate inputs
	if offsets == nil {
		return nil, errors.New("offsets is nil")
	}
	if txIndex < 0 {
		return nil, fmt.Errorf("transaction index %d is negative", txIndex)
	}
	if txIndex >= len(offsets.Transactions) {
		return nil, fmt.Errorf("transaction index %d out of range", txIndex)
	}

	witnessRange := offsets.Transactions[txIndex].Witness

	// Check for overflow in offset + length calculation
	end := uint64(witnessRange.Offset) + uint64(witnessRange.Length)
	if end > uint64(len(blockData)) {
		return nil, errors.New("witness range exceeds block data")
	}

	return blockData[witnessRange.Offset : witnessRange.Offset+witnessRange.Length], nil
}
