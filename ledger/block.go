// Copyright 2023 Blink Labs, LLC.
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
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/blake2b"
)

type Block interface {
	BlockHeader
	Transactions() []TransactionBody
}

type BlockHeader interface {
	Hash() string
	BlockNumber() uint64
	SlotNumber() uint64
	Era() Era
	Cbor() []byte
}

func NewBlockFromCbor(blockType uint, data []byte) (Block, error) {
	switch blockType {
	case BLOCK_TYPE_BYRON_EBB:
		return NewByronEpochBoundaryBlockFromCbor(data)
	case BLOCK_TYPE_BYRON_MAIN:
		return NewByronMainBlockFromCbor(data)
	case BLOCK_TYPE_SHELLEY:
		return NewShelleyBlockFromCbor(data)
	case BLOCK_TYPE_ALLEGRA:
		return NewAllegraBlockFromCbor(data)
	case BLOCK_TYPE_MARY:
		return NewMaryBlockFromCbor(data)
	case BLOCK_TYPE_ALONZO:
		return NewAlonzoBlockFromCbor(data)
	case BLOCK_TYPE_BABBAGE:
		return NewBabbageBlockFromCbor(data)
	}
	return nil, fmt.Errorf("unknown node-to-client block type: %d", blockType)
}

// XXX: should this take the block header type instead?
func NewBlockHeaderFromCbor(blockType uint, data []byte) (BlockHeader, error) {
	switch blockType {
	case BLOCK_TYPE_BYRON_EBB:
		return NewByronEpochBoundaryBlockHeaderFromCbor(data)
	case BLOCK_TYPE_BYRON_MAIN:
		return NewByronMainBlockHeaderFromCbor(data)
	// TODO: break into separate cases and parse as specific block header types
	case BLOCK_TYPE_SHELLEY, BLOCK_TYPE_ALLEGRA, BLOCK_TYPE_MARY, BLOCK_TYPE_ALONZO:
		return NewShelleyBlockHeaderFromCbor(data)
	case BLOCK_TYPE_BABBAGE:
		return NewBabbageBlockHeaderFromCbor(data)
	}
	return nil, fmt.Errorf("unknown node-to-node block type: %d", blockType)
}

func generateBlockHeaderHash(data []byte, prefix []byte) string {
	// We can ignore the error return here because our fixed size/key arguments will
	// never trigger an error
	tmpHash, _ := blake2b.New256(nil)
	if prefix != nil {
		tmpHash.Write(prefix)
	}
	tmpHash.Write(data)
	return hex.EncodeToString(tmpHash.Sum(nil))
}
