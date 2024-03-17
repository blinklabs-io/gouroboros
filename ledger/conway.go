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
	EraIdConway = 6

	BlockTypeConway = 7

	BlockHeaderTypeConway = 6

	TxTypeConway = 6
)

type ConwayBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Header                 *ConwayBlockHeader
	TransactionBodies      []ConwayTransactionBody
	TransactionWitnessSets []BabbageTransactionWitnessSet
	TransactionMetadataSet map[uint]*cbor.Value
	InvalidTransactions    []uint
}

func (b *ConwayBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *ConwayBlock) Hash() string {
	return b.Header.Hash()
}

func (b *ConwayBlock) BlockNumber() uint64 {
	return b.Header.BlockNumber()
}

func (b *ConwayBlock) SlotNumber() uint64 {
	return b.Header.SlotNumber()
}

func (b *ConwayBlock) IssuerVkey() IssuerVkey {
	return b.Header.IssuerVkey()
}

func (b *ConwayBlock) BlockBodySize() uint64 {
	return b.Header.BlockBodySize()
}

func (b *ConwayBlock) Era() Era {
	return eras[EraIdConway]
}

func (b *ConwayBlock) Transactions() []Transaction {
	invalidTxMap := make(map[uint]bool, len(b.InvalidTransactions))
	for _, invalidTxIdx := range b.InvalidTransactions {
		invalidTxMap[invalidTxIdx] = true
	}

	ret := make([]Transaction, len(b.TransactionBodies))
	for idx := range b.TransactionBodies {
		ret[idx] = &ConwayTransaction{
			Body:       b.TransactionBodies[idx],
			WitnessSet: b.TransactionWitnessSets[idx],
			TxMetadata: b.TransactionMetadataSet[uint(idx)],
			IsTxValid:  !invalidTxMap[uint(idx)],
		}
	}
	return ret
}

func (b *ConwayBlock) Utxorpc() *utxorpc.Block {
	var txs []*utxorpc.Tx
	tmpHash, _ := hex.DecodeString(b.Hash())
	for _, t := range b.Transactions() {
		tx := t.Utxorpc()
		txs = append(txs, tx)
	}
	body := &utxorpc.BlockBody{
		Tx: txs,
	}
	header := &utxorpc.BlockHeader{
		Hash:   tmpHash,
		Height: b.BlockNumber(),
		Slot:   b.SlotNumber(),
	}
	block := &utxorpc.Block{
		Body:   body,
		Header: header,
	}
	return block
}

type ConwayBlockHeader struct {
	BabbageBlockHeader
}

func (h *ConwayBlockHeader) Era() Era {
	return eras[EraIdConway]
}

type ConwayTransactionBody struct {
	BabbageTransactionBody
	VotingProcedures     *cbor.Value `cbor:"19,keyasint,omitempty"`
	ProposalProcedures   *cbor.Value `cbor:"20,keyasint,omitempty"`
	CurrentTreasuryValue int64       `cbor:"21,keyasint,omitempty"`
	Donation             uint64      `cbor:"22,keyasint,omitempty"`
}

func (b *ConwayTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

type ConwayTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       ConwayTransactionBody
	WitnessSet BabbageTransactionWitnessSet
	IsTxValid  bool
	TxMetadata *cbor.Value
}

func (t ConwayTransaction) Hash() string {
	return t.Body.Hash()
}

func (t ConwayTransaction) Inputs() []TransactionInput {
	return t.Body.Inputs()
}

func (t ConwayTransaction) Outputs() []TransactionOutput {
	return t.Body.Outputs()
}

func (t ConwayTransaction) Fee() uint64 {
	return t.Body.Fee()
}

func (t ConwayTransaction) TTL() uint64 {
	return t.Body.TTL()
}

func (t ConwayTransaction) ReferenceInputs() []TransactionInput {
	return t.Body.ReferenceInputs()
}

func (t ConwayTransaction) Metadata() *cbor.Value {
	return t.TxMetadata
}

func (t ConwayTransaction) IsValid() bool {
	return t.IsTxValid
}

func (t *ConwayTransaction) Cbor() []byte {
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
		t.IsValid,
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

func (t *ConwayTransaction) Utxorpc() *utxorpc.Tx {
	return t.Body.Utxorpc()
}

func NewConwayBlockFromCbor(data []byte) (*ConwayBlock, error) {
	var conwayBlock ConwayBlock
	if _, err := cbor.Decode(data, &conwayBlock); err != nil {
		return nil, fmt.Errorf("Conway block decode error: %s", err)
	}
	return &conwayBlock, nil
}

func NewConwayBlockHeaderFromCbor(data []byte) (*ConwayBlockHeader, error) {
	var conwayBlockHeader ConwayBlockHeader
	if _, err := cbor.Decode(data, &conwayBlockHeader); err != nil {
		return nil, fmt.Errorf("Conway block header decode error: %s", err)
	}
	return &conwayBlockHeader, nil
}

func NewConwayTransactionBodyFromCbor(
	data []byte,
) (*ConwayTransactionBody, error) {
	var conwayTx ConwayTransactionBody
	if _, err := cbor.Decode(data, &conwayTx); err != nil {
		return nil, fmt.Errorf("Conway transaction body decode error: %s", err)
	}
	return &conwayTx, nil
}

func NewConwayTransactionFromCbor(data []byte) (*ConwayTransaction, error) {
	var conwayTx ConwayTransaction
	if _, err := cbor.Decode(data, &conwayTx); err != nil {
		return nil, fmt.Errorf("Conway transaction decode error: %s", err)
	}
	return &conwayTx, nil
}
