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
	"encoding/hex"
	"fmt"

	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"

	"github.com/blinklabs-io/gouroboros/cbor"
)

const (
	EraIdAllegra = 2

	BlockTypeAllegra = 3

	BlockHeaderTypeAllegra = 2

	TxTypeAllegra = 2
)

type AllegraBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Header                 *AllegraBlockHeader
	TransactionBodies      []AllegraTransactionBody
	TransactionWitnessSets []ShelleyTransactionWitnessSet
	TransactionMetadataSet map[uint]*cbor.Value
}

func (b *AllegraBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *AllegraBlock) Hash() string {
	return b.Header.Hash()
}

func (b *AllegraBlock) BlockNumber() uint64 {
	return b.Header.BlockNumber()
}

func (b *AllegraBlock) SlotNumber() uint64 {
	return b.Header.SlotNumber()
}

func (b *AllegraBlock) IssuerVkey() IssuerVkey {
	return b.Header.IssuerVkey()
}

func (b *AllegraBlock) BlockBodySize() uint64 {
	return b.Header.BlockBodySize()
}

func (b *AllegraBlock) Era() Era {
	return eras[EraIdAllegra]
}

func (b *AllegraBlock) Transactions() []Transaction {
	ret := []Transaction{}
	for idx := range b.TransactionBodies {
		tmpTransaction := AllegraTransaction{
			Body:       b.TransactionBodies[idx],
			WitnessSet: b.TransactionWitnessSets[idx],
			TxMetadata: b.TransactionMetadataSet[uint(idx)],
		}
		ret = append(ret, &tmpTransaction)
	}
	return ret
}

func (b *AllegraBlock) Utxorpc() *utxorpc.Block {
	var block *utxorpc.Block
	var body *utxorpc.BlockBody
	var header *utxorpc.BlockHeader
	var txs []*utxorpc.Tx
	header.Slot = b.SlotNumber()
	tmpHash, _ := hex.DecodeString(b.Hash())
	header.Hash = tmpHash
	header.Height = b.BlockNumber()
	for _, t := range b.Transactions() {
		tx := t.Utxorpc()
		txs = append(txs, tx)
	}
	body.Tx = txs
	block.Body = body
	block.Header = header
	return block
}

type AllegraBlockHeader struct {
	ShelleyBlockHeader
}

func (h *AllegraBlockHeader) Era() Era {
	return eras[EraIdAllegra]
}

type AllegraTransactionBody struct {
	ShelleyTransactionBody
	ValidityIntervalStart uint64 `cbor:"8,keyasint,omitempty"`
}

func (b *AllegraTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

type AllegraTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       AllegraTransactionBody
	WitnessSet ShelleyTransactionWitnessSet
	TxMetadata *cbor.Value
}

func (t AllegraTransaction) Hash() string {
	return t.Body.Hash()
}

func (t AllegraTransaction) Inputs() []TransactionInput {
	return t.Body.Inputs()
}

func (t AllegraTransaction) Outputs() []TransactionOutput {
	return t.Body.Outputs()
}

func (t AllegraTransaction) Fee() uint64 {
	return t.Body.Fee()
}

func (t AllegraTransaction) TTL() uint64 {
	return t.Body.TTL()
}

func (t AllegraTransaction) Metadata() *cbor.Value {
	return t.TxMetadata
}

func (t AllegraTransaction) Utxorpc() *utxorpc.Tx {
	return t.Body.Utxorpc()
}

func (t *AllegraTransaction) Cbor() []byte {
	// Return stored CBOR if we have any
	cborData := t.DecodeStoreCbor.Cbor()
	if cborData != nil {
		return cborData[:]
	}
	// Return immediately if the body CBOR is also empty, which implies an empty TX object
	if t.Body.Cbor() == nil {
		return nil
	}
	// Generate our own CBOR
	// This is necessary when a transaction is put together from pieces stored separately in a block
	tmpObj := []any{
		cbor.RawMessage(t.Body.Cbor()),
		cbor.RawMessage(t.WitnessSet.Cbor()),
	}
	if t.TxMetadata != nil {
		tmpObj = append(tmpObj, cbor.RawMessage(t.TxMetadata.Cbor()))
	} else {
		tmpObj = append(tmpObj, nil)
	}
	// This should never fail, since we're only encoding a list and a bool value
	cborData, _ = cbor.Encode(&tmpObj)
	return cborData
}

func NewAllegraBlockFromCbor(data []byte) (*AllegraBlock, error) {
	var allegraBlock AllegraBlock
	if _, err := cbor.Decode(data, &allegraBlock); err != nil {
		return nil, fmt.Errorf("Allegra block decode error: %s", err)
	}
	return &allegraBlock, nil
}

func NewAllegraTransactionBodyFromCbor(
	data []byte,
) (*AllegraTransactionBody, error) {
	var allegraTx AllegraTransactionBody
	if _, err := cbor.Decode(data, &allegraTx); err != nil {
		return nil, fmt.Errorf("Allegra transaction body decode error: %s", err)
	}
	return &allegraTx, nil
}

func NewAllegraTransactionFromCbor(data []byte) (*AllegraTransaction, error) {
	var allegraTx AllegraTransaction
	if _, err := cbor.Decode(data, &allegraTx); err != nil {
		return nil, fmt.Errorf("Allegra transaction decode error: %s", err)
	}
	return &allegraTx, nil
}
