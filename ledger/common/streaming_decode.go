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

// StreamingBlockDecoder decodes blocks while capturing CBOR byte offsets.
// This is more efficient than decoding and extracting offsets separately.
type StreamingBlockDecoder struct {
	data    []byte
	stream  *cbor.StreamDecoder
	offsets *BlockTransactionOffsets
}

// NewStreamingBlockDecoder creates a decoder for the given block CBOR.
func NewStreamingBlockDecoder(data []byte) (*StreamingBlockDecoder, error) {
	stream, err := cbor.NewStreamDecoder(data)
	if err != nil {
		return nil, fmt.Errorf("create stream decoder: %w", err)
	}

	return &StreamingBlockDecoder{
		data:   data,
		stream: stream,
		offsets: &BlockTransactionOffsets{
			Transactions: nil, // Will be allocated when we know the count
		},
	}, nil
}

// DecodeWithOffsets decodes the block and extracts all CBOR offsets in a single pass.
// This is the main entry point for streaming block decoding.
//
// The function:
// 1. Parses the block array structure
// 2. Extracts transaction body, witness, and metadata positions
// 3. Extracts output positions within each transaction body
// 4. Returns offsets for efficient later extraction
//
// Note: The actual Block struct is created by calling the appropriate era-specific
// decoder on the raw data. This function focuses on offset extraction.
func (d *StreamingBlockDecoder) DecodeWithOffsets() (*BlockTransactionOffsets, error) {
	// Decode block as array of RawMessage to get component boundaries
	var blockArray []cbor.RawMessage
	blockStart, blockLen, err := d.stream.Decode(&blockArray)
	if err != nil {
		return nil, fmt.Errorf("decode block array: %w", err)
	}
	_ = blockLen // Used for validation if needed

	if len(blockArray) < 3 {
		// Byron EBB or other minimal block format
		// Return empty slice instead of nil to prevent nil pointer dereference
		d.offsets.Transactions = []TransactionLocation{}
		return d.offsets, nil
	}

	// Calculate component offsets within the block
	// Block structure: [header, tx_bodies[], witnesses[], metadata_map, invalid_txs[]]
	arrayHeaderSize := cborArrayHeaderSize(len(blockArray))

	// Track positions as we walk through the block
	// #nosec G115 -- Cardano block components are well under 4GiB
	pos := uint32(blockStart) + uint32(arrayHeaderSize)

	// Skip header (blockArray[0])
	// #nosec G115 -- block header <<4GiB
	headerLen := uint32(len(blockArray[0]))
	pos += headerLen

	// Extract transaction bodies array (blockArray[1])
	txBodiesOffset := pos
	// #nosec G115 -- tx bodies <<4GiB
	txBodiesLen := uint32(len(blockArray[1]))

	// Extract witness sets array (blockArray[2])
	witnessesOffset := txBodiesOffset + txBodiesLen
	// #nosec G115 -- witnesses <<4GiB
	witnessesLen := uint32(len(blockArray[2]))

	// Extract metadata map offset (blockArray[3] if present)
	var metadataOffset uint32
	if len(blockArray) > 3 {
		metadataOffset = witnessesOffset + witnessesLen
	}

	// Parse transaction bodies to get individual positions
	var txBodiesRaw []cbor.RawMessage
	if _, err := cbor.Decode([]byte(blockArray[1]), &txBodiesRaw); err != nil {
		return nil, fmt.Errorf("decode transaction bodies: %w", err)
	}

	// Parse witness sets
	var witnessesRaw []cbor.RawMessage
	if _, err := cbor.Decode([]byte(blockArray[2]), &witnessesRaw); err != nil {
		return nil, fmt.Errorf("decode witness sets: %w", err)
	}

	// Validate that transaction bodies and witness sets have matching lengths
	if len(txBodiesRaw) != len(witnessesRaw) {
		return nil, fmt.Errorf(
			"mismatched transaction bodies (%d) and witness sets (%d)",
			len(txBodiesRaw),
			len(witnessesRaw),
		)
	}

	// Parse metadata map for per-transaction metadata positions
	metadataByTxIdx := make(map[uint32]struct {
		offset uint32
		length uint32
	})
	if len(blockArray) > 3 && len(blockArray[3]) > 1 {
		_ = extractMetadataOffsets(
			[]byte(blockArray[3]),
			metadataOffset,
			metadataByTxIdx,
		)
	}

	// Allocate transaction locations
	d.offsets.Transactions = make([]TransactionLocation, len(txBodiesRaw))

	// Calculate individual transaction body offsets
	bodiesArrayHeader := uint32(cborArrayHeaderSize(len(txBodiesRaw)))
	bodyPos := txBodiesOffset + bodiesArrayHeader

	for i, rawBody := range txBodiesRaw {
		// #nosec G115 -- tx body <<4GiB
		bodyLen := uint32(len(rawBody))
		d.offsets.Transactions[i].Body = ByteRange{
			Offset: bodyPos,
			Length: bodyLen,
		}

		// Extract output offsets from this transaction body
		d.extractOutputOffsets(i, []byte(rawBody), bodyPos)

		bodyPos += bodyLen
	}

	// Calculate individual witness set offsets
	witnessArrayHeader := uint32(cborArrayHeaderSize(len(witnessesRaw)))
	witnessPos := witnessesOffset + witnessArrayHeader

	for i, rawWitness := range witnessesRaw {
		// #nosec G115 -- witness set <<4GiB
		witnessLen := uint32(len(rawWitness))
		if i < len(d.offsets.Transactions) {
			d.offsets.Transactions[i].Witness = ByteRange{
				Offset: witnessPos,
				Length: witnessLen,
			}

			// Extract datum, redeemer, and script offsets
			extractWitnessComponentOffsets(
				[]byte(rawWitness),
				witnessPos,
				&d.offsets.Transactions[i],
			)
		}
		witnessPos += witnessLen
	}

	// Assign metadata offsets
	for i := range d.offsets.Transactions {
		// #nosec G115 -- tx index <<4 billion
		if meta, ok := metadataByTxIdx[uint32(i)]; ok {
			d.offsets.Transactions[i].Metadata = ByteRange{
				Offset: meta.offset,
				Length: meta.length,
			}
		}
	}

	return d.offsets, nil
}

// extractOutputOffsets parses a transaction body to find output CBOR positions.
// This uses streaming decode to track positions of each output within the body.
func (d *StreamingBlockDecoder) extractOutputOffsets(
	txIndex int,
	bodyData []byte,
	bodyOffset uint32,
) {
	// Transaction body is a CBOR map with numeric keys
	// Key 1 contains the outputs array
	// We need to find the outputs array and get positions of each output

	if len(bodyData) < 2 {
		return
	}

	// Parse the transaction body map
	count, headerSize, indefinite := cborMapInfo(bodyData)
	if count < 0 && !indefinite {
		return
	}

	// Create a stream decoder for the body content (after map header)
	bodyStream, err := cbor.NewStreamDecoder(bodyData[headerSize:])
	if err != nil {
		return
	}

	// Scan through map key-value pairs looking for key 1 (outputs)
	for i := 0; indefinite || i < count; i++ {
		// Check for break byte in indefinite maps
		if indefinite {
			pos := bodyStream.Position()
			checkPos := int(headerSize) + pos
			if checkPos >= len(bodyData) || bodyData[checkPos] == 0xff {
				break
			}
		}

		// Decode map key
		var key uint64
		keyStart, keyLen, err := bodyStream.Decode(&key)
		if err != nil {
			return
		}
		_ = keyStart
		_ = keyLen

		if key == 1 {
			// Found the outputs array - decode it with position tracking
			valueStart := bodyStream.Position()

			// Decode outputs as array of RawMessage
			var outputsRaw []cbor.RawMessage
			_, _, err := bodyStream.Decode(&outputsRaw)
			if err != nil {
				return
			}

			// Calculate the absolute offset of the outputs array
			// #nosec G115 -- Cardano tx body offsets are well under 4GiB
			outputsArrayOffset := bodyOffset + uint32(headerSize) + uint32(valueStart)
			outputsArrayHeader := uint32(cborArrayHeaderSize(len(outputsRaw)))

			// Track position within outputs array
			outputPos := outputsArrayOffset + outputsArrayHeader

			// Record each output's position
			if d.offsets.Transactions[txIndex].Outputs == nil {
				d.offsets.Transactions[txIndex].Outputs = make([]ByteRange, len(outputsRaw))
			}

			for j, rawOutput := range outputsRaw {
				// #nosec G115 -- Cardano outputs are <<4GiB
				outputLen := uint32(len(rawOutput))
				d.offsets.Transactions[txIndex].Outputs[j] = ByteRange{
					Offset: outputPos,
					Length: outputLen,
				}
				outputPos += outputLen
			}

			return // Found outputs, done with this body
		}

		// Skip the value for this key
		if _, _, err := bodyStream.Skip(); err != nil {
			return
		}
	}
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
		return nil, fmt.Errorf("output index %d out of range for transaction %d", outputIndex, txIndex)
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
