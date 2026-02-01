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

package ledger

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// Compatibility aliases
type (
	Block       = common.Block
	BlockHeader = common.BlockHeader
)

func NewBlockFromCbor(
	blockType uint,
	data []byte,
	config ...common.VerifyConfig,
) (Block, error) {
	var cfg common.VerifyConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	// Default: validation enabled (SkipBodyHashValidation = false)

	switch blockType {
	case BlockTypeByronEbb:
		return NewByronEpochBoundaryBlockFromCbor(data, cfg)
	case BlockTypeByronMain:
		return NewByronMainBlockFromCbor(data, cfg)
	case BlockTypeShelley:
		return NewShelleyBlockFromCbor(data, cfg)
	case BlockTypeAllegra:
		return NewAllegraBlockFromCbor(data, cfg)
	case BlockTypeMary:
		return NewMaryBlockFromCbor(data, cfg)
	case BlockTypeAlonzo:
		return NewAlonzoBlockFromCbor(data, cfg)
	case BlockTypeBabbage:
		return NewBabbageBlockFromCbor(data, cfg)
	case BlockTypeConway:
		return NewConwayBlockFromCbor(data, cfg)
	case BlockTypeLeiosEndorser:
		return NewLeiosEndorserBlockFromCbor(data, cfg)
	case BlockTypeLeiosRanking:
		return NewLeiosRankingBlockFromCbor(data, cfg)
	}
	return nil, fmt.Errorf("unknown node-to-client block type: %d", blockType)
}

func NewBlockHeaderFromCbor(blockType uint, data []byte) (BlockHeader, error) {
	switch blockType {
	case BlockTypeByronEbb:
		return NewByronEpochBoundaryBlockHeaderFromCbor(data)
	case BlockTypeByronMain:
		return NewByronMainBlockHeaderFromCbor(data)
	case BlockTypeShelley:
		return NewShelleyBlockHeaderFromCbor(data)
	case BlockTypeAllegra:
		return NewAllegraBlockHeaderFromCbor(data)
	case BlockTypeMary:
		return NewMaryBlockHeaderFromCbor(data)
	case BlockTypeAlonzo:
		return NewAlonzoBlockHeaderFromCbor(data)
	case BlockTypeBabbage:
		return NewBabbageBlockHeaderFromCbor(data)
	case BlockTypeConway:
		return NewConwayBlockHeaderFromCbor(data)
	case BlockTypeLeiosRanking:
		return NewLeiosBlockHeaderFromCbor(data)
	default:
		return nil, fmt.Errorf("unknown node-to-node block type: %d", blockType)
	}
}

// Type aliases for offset structures
type (
	BlockWithOffsets        = common.BlockWithOffsets
	BlockTransactionOffsets = common.BlockTransactionOffsets
	TransactionLocation     = common.TransactionLocation
	ByteRange               = common.ByteRange
)

// NewBlockFromCborWithOffsets decodes a block and extracts CBOR byte offsets.
// This is the recommended way to decode blocks when offset information is needed for
// efficient CBOR extraction (e.g., for storing offset references instead of copying CBOR).
//
// The function performs two passes over the block data:
// 1. Streaming pass: Extracts byte offsets for all transactions, outputs, witnesses, and metadata
// 2. Era-specific decode: Decodes the block using the appropriate era decoder
//
// Both passes are fast since the block data is in CPU cache. This approach maintains
// clean separation between offset extraction (era-agnostic) and block decoding (era-specific).
//
// Parameters:
//   - blockType: The block type ID (e.g., BlockTypeConway)
//   - data: Raw block CBOR bytes
//   - config: Optional verification config (e.g., to skip body hash validation)
//
// Returns:
//   - BlockWithOffsets containing both the decoded block and offset information
//   - error if decoding fails
func NewBlockFromCborWithOffsets(
	blockType uint,
	data []byte,
	config ...common.VerifyConfig,
) (*BlockWithOffsets, error) {
	// Use the streaming decoder to extract offsets
	decoder, err := common.NewStreamingBlockDecoder(data)
	if err != nil {
		return nil, fmt.Errorf("create streaming decoder: %w", err)
	}

	offsets, err := decoder.DecodeWithOffsets()
	if err != nil {
		return nil, fmt.Errorf("extract offsets: %w", err)
	}

	// Decode the block using the standard decoder
	block, err := NewBlockFromCbor(blockType, data, config...)
	if err != nil {
		return nil, fmt.Errorf("decode block: %w", err)
	}

	return &BlockWithOffsets{
		Block:   block,
		Offsets: offsets,
	}, nil
}

// ExtractTransactionOffsets extracts byte offsets for all transactions in a block.
// This is a convenience alias for common.ExtractTransactionOffsets.
//
// Use NewBlockFromCborWithOffsets when you also need to decode the block,
// as it is more efficient than decoding and extracting offsets separately.
func ExtractTransactionOffsets(cborData []byte) (*BlockTransactionOffsets, error) {
	return common.ExtractTransactionOffsets(cborData)
}
