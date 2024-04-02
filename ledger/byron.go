// Copyright 2024 Blink Labs Software
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
	// Decode generically and store original CBOR
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
	return IssuerVkey{}
}

func (h *ByronMainBlockHeader) BlockBodySize() uint64 {
	// Byron doesn't include the block body size in the header
	return 0
}

func (h *ByronMainBlockHeader) Era() Era {
	return eras[EraIdByron]
}

type ByronTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash       string
	TxInputs   []ByronTransactionInput
	TxOutputs  []ByronTransactionOutput
	Attributes *cbor.Value
}

func (t *ByronTransaction) UnmarshalCBOR(data []byte) error {
	// Decode generically and store original CBOR
	return t.UnmarshalCbor(data, t)
}

func (t *ByronTransaction) Hash() string {
	if t.hash == "" {
		t.hash = generateTransactionHash(t.Cbor(), nil)
	}
	return t.hash
}

func (t *ByronTransaction) Inputs() []TransactionInput {
	ret := []TransactionInput{}
	for _, input := range t.TxInputs {
		ret = append(ret, input)
	}
	return ret
}

func (t *ByronTransaction) Outputs() []TransactionOutput {
	ret := []TransactionOutput{}
	for _, output := range t.TxOutputs {
		output := output
		ret = append(ret, &output)
	}
	return ret
}

func (t *ByronTransaction) Fee() uint64 {
	// The fee is implicit in Byron, and we don't have enough information here to calculate it.
	// You need to know the Lovelace in the inputs to determine the fee, and that information is
	// not provided directly in the TX
	return 0
}

func (t *ByronTransaction) TTL() uint64 {
	// No TTL in Byron
	return 0
}

func (t *ByronTransaction) ReferenceInputs() []TransactionInput {
	// No reference inputs in Byron
	return nil
}

func (t *ByronTransaction) Metadata() *cbor.Value {
	return t.Attributes
}

func (t *ByronTransaction) IsValid() bool {
	return true
}

func (t *ByronTransaction) Utxorpc() *utxorpc.Tx {
	return &utxorpc.Tx{}
}

type ByronTransactionInput struct {
	cbor.StructAsArray
	TxId        Blake2b256
	OutputIndex uint32
}

func (i *ByronTransactionInput) UnmarshalCBOR(data []byte) error {
	id, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	switch id {
	case 0:
		var tmpData struct {
			cbor.StructAsArray
			Id   int
			Cbor []byte
		}
		if _, err := cbor.Decode(data, &tmpData); err != nil {
			return err
		}
		if err := cbor.DecodeGeneric(tmpData.Cbor, i); err != nil {
			return err
		}
	default:
		// [u8 .ne 0, encoded-cbor]
		return fmt.Errorf("can't parse yet")
	}
	return nil
}

func (i ByronTransactionInput) Id() Blake2b256 {
	return i.TxId
}

func (i ByronTransactionInput) Index() uint32 {
	return i.OutputIndex
}

func (i ByronTransactionInput) Utxorpc() *utxorpc.TxInput {
	return &utxorpc.TxInput{
		TxHash:      i.TxId.Bytes(),
		OutputIndex: i.OutputIndex,
		// AsOutput: i.AsOutput,
		// Redeemer: i.Redeemer,
	}
}

func (i ByronTransactionInput) String() string {
	return fmt.Sprintf("%s#%d", i.TxId, i.OutputIndex)
}

func (i ByronTransactionInput) MarshalJSON() ([]byte, error) {
	return []byte("\"" + i.String() + "\""), nil
}

type ByronTransactionOutput struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	OutputAddress Address `json:"address"`
	OutputAmount  uint64  `json:"amount"`
}

func (o *ByronTransactionOutput) UnmarshalCBOR(data []byte) error {
	// Save original CBOR
	o.SetCbor(data)
	var tmpData struct {
		cbor.StructAsArray
		WrappedAddress cbor.RawMessage
		Amount         uint64
	}
	if _, err := cbor.Decode(data, &tmpData); err != nil {
		return err
	}
	o.OutputAmount = tmpData.Amount
	if _, err := cbor.Decode(tmpData.WrappedAddress, &o.OutputAddress); err != nil {
		return err
	}
	return nil
}

func (o ByronTransactionOutput) Address() Address {
	return o.OutputAddress
}

func (o ByronTransactionOutput) Amount() uint64 {
	return o.OutputAmount
}

func (o ByronTransactionOutput) Assets() *MultiAsset[MultiAssetTypeOutput] {
	return nil
}

func (o ByronTransactionOutput) DatumHash() *Blake2b256 {
	return nil
}

func (o ByronTransactionOutput) Datum() *cbor.LazyValue {
	return nil
}

func (o ByronTransactionOutput) Utxorpc() *utxorpc.TxOutput {
	return &utxorpc.TxOutput{
		Address: o.OutputAddress.Bytes(),
		Coin:    o.Amount(),
	}
}

type ByronMainBlockBody struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
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

func (b *ByronMainBlockBody) UnmarshalCBOR(data []byte) error {
	// Decode generically and store original CBOR
	return b.UnmarshalCbor(data, b)
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
	// Decode generically and store original CBOR
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
	// Byron doesn't include the block body size in the header
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
	// Decode generically and store original CBOR
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
	return uint64(len(b.Body.Cbor()))
}

func (b *ByronMainBlock) Era() Era {
	return b.Header.Era()
}

func (b *ByronMainBlock) Transactions() []Transaction {
	ret := make([]Transaction, len(b.Body.TxPayload))
	for idx, payload := range b.Body.TxPayload {
		payload := payload
		ret[idx] = &payload.Transaction
	}
	return ret
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
	// Decode generically and store original CBOR
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
	// There's not really a body for an epoch boundary block
	return 0
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
