// Copyright 2025 Blink Labs Software
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
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// Compatibility aliases
type Block = common.Block
type BlockHeader = common.BlockHeader

func NewBlockFromCbor(blockType uint, data []byte) (Block, error) {
	switch blockType {
	case BlockTypeByronEbb:
		return NewByronEpochBoundaryBlockFromCbor(data)
	case BlockTypeByronMain:
		return NewByronMainBlockFromCbor(data)
	case BlockTypeShelley:
		return NewShelleyBlockFromCbor(data)
	case BlockTypeAllegra:
		return NewAllegraBlockFromCbor(data)
	case BlockTypeMary:
		return NewMaryBlockFromCbor(data)
	case BlockTypeAlonzo:
		return NewAlonzoBlockFromCbor(data)
	case BlockTypeBabbage:
		return NewBabbageBlockFromCbor(data)
	case BlockTypeConway:
		return NewConwayBlockFromCbor(data)
	}
	return nil, fmt.Errorf("unknown node-to-client block type: %d", blockType)
}

func NewBlockHeaderFromCbor(blockType uint, data []byte) (BlockHeader, error) {
	switch blockType {
	case BlockTypeByronEbb:
		return NewByronEpochBoundaryBlockHeaderFromCbor(data)
	case BlockTypeByronMain:
		return NewByronMainBlockHeaderFromCbor(data)
	// TODO: break into separate cases and parse as specific block header types (#844)
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
	default:
		return nil, fmt.Errorf("unknown node-to-node block type: %d", blockType)
	}
}

func DetermineBlockType(data []byte) (uint, error) {
	if _, err := NewByronEpochBoundaryBlockFromCbor(data); err == nil {
		return BlockTypeByronEbb, nil
	}
	if _, err := NewByronMainBlockFromCbor(data); err == nil {
		return BlockTypeByronMain, nil
	}
	if _, err := NewShelleyBlockFromCbor(data); err == nil {
		return BlockTypeShelley, nil
	}
	if _, err := NewAllegraBlockFromCbor(data); err == nil {
		return BlockTypeAllegra, nil
	}
	if _, err := NewMaryBlockFromCbor(data); err == nil {
		return BlockTypeMary, nil
	}
	if _, err := NewAlonzoBlockFromCbor(data); err == nil {
		return BlockTypeAlonzo, nil
	}
	if _, err := NewBabbageBlockFromCbor(data); err == nil {
		return BlockTypeBabbage, nil
	}
	if _, err := NewConwayBlockFromCbor(data); err == nil {
		return BlockTypeConway, nil
	}
	return 0, errors.New("unknown block type")
}
