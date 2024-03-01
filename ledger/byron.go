// Copyright 2023 Blink Labs Software
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

	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"

	"github.com/blinklabs-io/gouroboros/cbor"
)

const (
	EraIdByron = 0

	BlockTypeByronEbb  = 0
	BlockTypeByronMain = 1

	BlockHeaderTypeByron = 0

	TxTypeByron = 0

	ByronSlotsPerEpoch = 21600
)

type ByronMainBlockHeader struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash          string
	ProtocolMagic uint32
	PrevBlock     Blake2b256
	BodyProof     interface{}
	ConsensusData struct {
		cbor.StructAsArray
		// [slotid, pubkey, difficulty, blocksig]
		SlotId struct {
			cbor.StructAsArray
			Epoch uint64
			Slot  uint16
		}
		PubKey     []byte
		Difficulty struct {
			cbor.StructAsArray
			Unknown uint64
		}
		BlockSig []interface{}
	}
	ExtraData struct {
		cbor.StructAsArray
		BlockVersion struct {
			cbor.StructAsArray
			Major   uint16
			Minor   uint16
			Unknown uint8
		}
		SoftwareVersion struct {
			cbor.StructAsArray
			Name    string
			Unknown uint32
		}
		Attributes interface{}
		ExtraProof Blake2b256
	}
}

func (h *ByronMainBlockHeader) UnmarshalCBOR(cborData []byte) error {
	return h.UnmarshalCbor(cborData, h)
}

func (h *ByronMainBlockHeader) Hash() string {
	if h.hash == "" {
		// Prepend bytes for CBOR list wrapper
		// The block hash is calculated with these extra bytes, so we have to add them to
		// get the correct value
		h.hash = generateBlockHeaderHash(
			h.Cbor(),
			[]byte{0x82, BlockTypeByronMain},
		)
	}
	return h.hash
}

func (h *ByronMainBlockHeader) BlockNumber() uint64 {
	// Byron blocks don't store the block number in the block
	return 0
}

func (h *ByronMainBlockHeader) SlotNumber() uint64 {
	return uint64(
		(h.ConsensusData.SlotId.Epoch * ByronSlotsPerEpoch) + uint64(
			h.ConsensusData.SlotId.Slot,
		),
	)
}

func (h *ByronMainBlockHeader) IssuerVkey() IssuerVkey {
	// Byron blocks don't have an issuer
	return IssuerVkey([]byte{})
}

func (h *ByronMainBlockHeader) BlockBodySize() uint64 {
	// TODO: calculate this
	return 0
}

func (h *ByronMainBlockHeader) Era() Era {
	return eras[EraIdByron]
}

type ByronTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	// TODO: flesh these out
	TxInputs   []any
	TxOutputs  []any
	Attributes *cbor.Value
}

func (t *ByronTransaction) Hash() string {
	// TODO
	return ""
}

func (t *ByronTransaction) Inputs() []TransactionInput {
	// TODO
	return nil
}

func (t *ByronTransaction) Outputs() []TransactionOutput {
	// TODO
	return nil
}

func (t *ByronTransaction) Fee() uint64 {
	// TODO
	return 0
}

func (t *ByronTransaction) TTL() uint64 {
	// TODO
	return 0
}

func (t *ByronTransaction) Metadata() *cbor.Value {
	return t.Attributes
}

func (t *ByronTransaction) Utxorpc() *utxorpc.Tx {
	return &utxorpc.Tx{}
}

type ByronMainBlockBody struct {
	cbor.StructAsArray
	// TODO: split this to its own type
	TxPayload []struct {
		cbor.StructAsArray
		Transaction ByronTransaction
		// TODO: figure out what this field actually is
		Twit []cbor.Value
	}
	SscPayload cbor.Value
	DlgPayload []interface{}
	UpdPayload []interface{}
}

type ByronEpochBoundaryBlockHeader struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash          string
	ProtocolMagic uint32
	PrevBlock     Blake2b256
	BodyProof     interface{}
	ConsensusData struct {
		cbor.StructAsArray
		Epoch      uint64
		Difficulty struct {
			cbor.StructAsArray
			Value uint64
		}
	}
	ExtraData interface{}
}

func (h *ByronEpochBoundaryBlockHeader) UnmarshalCBOR(cborData []byte) error {
	return h.UnmarshalCbor(cborData, h)
}

func (h *ByronEpochBoundaryBlockHeader) Hash() string {
	if h.hash == "" {
		// Prepend bytes for CBOR list wrapper
		// The block hash is calculated with these extra bytes, so we have to add them to
		// get the correct value
		h.hash = generateBlockHeaderHash(
			h.Cbor(),
			[]byte{0x82, BlockTypeByronEbb},
		)
	}
	return h.hash
}

func (h *ByronEpochBoundaryBlockHeader) BlockNumber() uint64 {
	// Byron blocks don't store the block number in the block
	return 0
}

func (h *ByronEpochBoundaryBlockHeader) SlotNumber() uint64 {
	return uint64(h.ConsensusData.Epoch * ByronSlotsPerEpoch)
}

func (h *ByronEpochBoundaryBlockHeader) IssuerVkey() IssuerVkey {
	// Byron blocks don't have an issuer
	return IssuerVkey([]byte{})
}

func (h *ByronEpochBoundaryBlockHeader) BlockBodySize() uint64 {
	// TODO: calculate this
	return 0
}

func (h *ByronEpochBoundaryBlockHeader) Era() Era {
	return eras[EraIdByron]
}

type ByronMainBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Header *ByronMainBlockHeader
	Body   ByronMainBlockBody
	Extra  []interface{}
}

func (b *ByronMainBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *ByronMainBlock) Hash() string {
	return b.Header.Hash()
}

func (b *ByronMainBlock) BlockNumber() uint64 {
	return b.Header.BlockNumber()
}

func (b *ByronMainBlock) SlotNumber() uint64 {
	return b.Header.SlotNumber()
}

func (b *ByronMainBlock) IssuerVkey() IssuerVkey {
	return b.Header.IssuerVkey()
}

func (b *ByronMainBlock) BlockBodySize() uint64 {
	return b.Header.BlockBodySize()
}

func (b *ByronMainBlock) Era() Era {
	return b.Header.Era()
}

func (b *ByronMainBlock) Transactions() []Transaction {
	// TODO
	return nil
}

func (b *ByronMainBlock) Utxorpc() *utxorpc.Block {
	return &utxorpc.Block{}
}

type ByronEpochBoundaryBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Header *ByronEpochBoundaryBlockHeader
	Body   []Blake2b224
	Extra  []interface{}
}

func (b *ByronEpochBoundaryBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *ByronEpochBoundaryBlock) Hash() string {
	return b.Header.Hash()
}

func (b *ByronEpochBoundaryBlock) BlockNumber() uint64 {
	return b.Header.BlockNumber()
}

func (b *ByronEpochBoundaryBlock) SlotNumber() uint64 {
	return b.Header.SlotNumber()
}

func (b *ByronEpochBoundaryBlock) IssuerVkey() IssuerVkey {
	return b.Header.IssuerVkey()
}

func (b *ByronEpochBoundaryBlock) BlockBodySize() uint64 {
	return b.Header.BlockBodySize()
}

func (b *ByronEpochBoundaryBlock) Era() Era {
	return b.Header.Era()
}

func (b *ByronEpochBoundaryBlock) Transactions() []Transaction {
	// Boundary blocks don't have transactions
	return nil
}

func (b *ByronEpochBoundaryBlock) Utxorpc() *utxorpc.Block {
	return &utxorpc.Block{}
}

func NewByronEpochBoundaryBlockFromCbor(
	data []byte,
) (*ByronEpochBoundaryBlock, error) {
	var byronEbbBlock ByronEpochBoundaryBlock
	if _, err := cbor.Decode(data, &byronEbbBlock); err != nil {
		return nil, fmt.Errorf("Byron EBB block decode error: %s", err)
	}
	return &byronEbbBlock, nil
}

func NewByronEpochBoundaryBlockHeaderFromCbor(
	data []byte,
) (*ByronEpochBoundaryBlockHeader, error) {
	var byronEbbBlockHeader ByronEpochBoundaryBlockHeader
	if _, err := cbor.Decode(data, &byronEbbBlockHeader); err != nil {
		return nil, fmt.Errorf("Byron EBB block header decode error: %s", err)
	}
	return &byronEbbBlockHeader, nil
}

func NewByronMainBlockFromCbor(data []byte) (*ByronMainBlock, error) {
	var byronMainBlock ByronMainBlock
	if _, err := cbor.Decode(data, &byronMainBlock); err != nil {
		return nil, fmt.Errorf("Byron main block decode error: %s", err)
	}
	return &byronMainBlock, nil
}

func NewByronMainBlockHeaderFromCbor(
	data []byte,
) (*ByronMainBlockHeader, error) {
	var byronMainBlockHeader ByronMainBlockHeader
	if _, err := cbor.Decode(data, &byronMainBlockHeader); err != nil {
		return nil, fmt.Errorf("Byron main block header decode error: %s", err)
	}
	return &byronMainBlockHeader, nil
}

func NewByronTransactionFromCbor(data []byte) (*ByronTransaction, error) {
	var byronTx ByronTransaction
	if _, err := cbor.Decode(data, &byronTx); err != nil {
		return nil, fmt.Errorf("Byron transaction decode error: %s", err)
	}
	return &byronTx, nil
}
