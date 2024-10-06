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

package allegra

import (
	"encoding/hex"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"

	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

const (
	EraIdAllegra   = 2
	EraNameAllegra = "Allegra"

	BlockTypeAllegra = 3

	BlockHeaderTypeAllegra = 2

	TxTypeAllegra = 2
)

var (
	EraAllegra = common.Era{
		Id:   EraIdAllegra,
		Name: EraNameAllegra,
	}
)

func init() {
	common.RegisterEra(EraAllegra)
}

type AllegraBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Header                 *AllegraBlockHeader
	TransactionBodies      []AllegraTransactionBody
	TransactionWitnessSets []shelley.ShelleyTransactionWitnessSet
	TransactionMetadataSet map[uint]*cbor.LazyValue
}

func (b *AllegraBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (AllegraBlock) Type() int {
	return BlockTypeAllegra
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

func (b *AllegraBlock) IssuerVkey() common.IssuerVkey {
	return b.Header.IssuerVkey()
}

func (b *AllegraBlock) BlockBodySize() uint64 {
	return b.Header.BlockBodySize()
}

func (b *AllegraBlock) Era() common.Era {
	return EraAllegra
}

func (b *AllegraBlock) Transactions() []common.Transaction {
	ret := make([]common.Transaction, len(b.TransactionBodies))
	for idx := range b.TransactionBodies {
		ret[idx] = &AllegraTransaction{
			Body:       b.TransactionBodies[idx],
			WitnessSet: b.TransactionWitnessSets[idx],
			TxMetadata: b.TransactionMetadataSet[uint(idx)],
		}
	}
	return ret
}

func (b *AllegraBlock) Utxorpc() *utxorpc.Block {
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

type AllegraBlockHeader struct {
	shelley.ShelleyBlockHeader
}

func (h *AllegraBlockHeader) Era() common.Era {
	return EraAllegra
}

type AllegraTransactionBody struct {
	shelley.ShelleyTransactionBody
	Update struct {
		cbor.StructAsArray
		ProtocolParamUpdates map[common.Blake2b224]AllegraProtocolParameterUpdate
		Epoch                uint64
	} `cbor:"6,keyasint,omitempty"`
	TxValidityIntervalStart uint64 `cbor:"8,keyasint,omitempty"`
}

func (b *AllegraTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *AllegraTransactionBody) ValidityIntervalStart() uint64 {
	return b.TxValidityIntervalStart
}

func (b *AllegraTransactionBody) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	updateMap := make(map[common.Blake2b224]common.ProtocolParameterUpdate)
	for k, v := range b.Update.ProtocolParamUpdates {
		updateMap[k] = v
	}
	return b.Update.Epoch, updateMap
}

type AllegraTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       AllegraTransactionBody
	WitnessSet shelley.ShelleyTransactionWitnessSet
	TxMetadata *cbor.LazyValue
}

func (AllegraTransaction) Type() int {
	return TxTypeAllegra
}

func (t AllegraTransaction) Hash() string {
	return t.Body.Hash()
}

func (t AllegraTransaction) Inputs() []common.TransactionInput {
	return t.Body.Inputs()
}

func (t AllegraTransaction) Outputs() []common.TransactionOutput {
	return t.Body.Outputs()
}

func (t AllegraTransaction) Fee() uint64 {
	return t.Body.Fee()
}

func (t AllegraTransaction) TTL() uint64 {
	return t.Body.TTL()
}

func (t AllegraTransaction) ValidityIntervalStart() uint64 {
	return t.Body.ValidityIntervalStart()
}

func (t AllegraTransaction) ReferenceInputs() []common.TransactionInput {
	return t.Body.ReferenceInputs()
}

func (t AllegraTransaction) Collateral() []common.TransactionInput {
	return t.Body.Collateral()
}

func (t AllegraTransaction) CollateralReturn() common.TransactionOutput {
	return t.Body.CollateralReturn()
}

func (t AllegraTransaction) TotalCollateral() uint64 {
	return t.Body.TotalCollateral()
}

func (t AllegraTransaction) Certificates() []common.Certificate {
	return t.Body.Certificates()
}

func (t AllegraTransaction) Withdrawals() map[*common.Address]uint64 {
	return t.Body.Withdrawals()
}

func (t AllegraTransaction) AuxDataHash() *common.Blake2b256 {
	return t.Body.AuxDataHash()
}

func (t AllegraTransaction) RequiredSigners() []common.Blake2b224 {
	return t.Body.RequiredSigners()
}

func (t AllegraTransaction) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return t.Body.AssetMint()
}

func (t AllegraTransaction) ScriptDataHash() *common.Blake2b256 {
	return t.Body.ScriptDataHash()
}

func (t AllegraTransaction) VotingProcedures() common.VotingProcedures {
	return t.Body.VotingProcedures()
}

func (t AllegraTransaction) ProposalProcedures() []common.ProposalProcedure {
	return t.Body.ProposalProcedures()
}

func (t AllegraTransaction) CurrentTreasuryValue() int64 {
	return t.Body.CurrentTreasuryValue()
}

func (t AllegraTransaction) Donation() uint64 {
	return t.Body.Donation()
}

func (t AllegraTransaction) Metadata() *cbor.LazyValue {
	return t.TxMetadata
}

func (t AllegraTransaction) Utxorpc() *utxorpc.Tx {
	return t.Body.Utxorpc()
}

func (t AllegraTransaction) IsValid() bool {
	return true
}

func (t AllegraTransaction) Consumed() []common.TransactionInput {
	return t.Inputs()
}

func (t AllegraTransaction) Produced() []common.Utxo {
	var ret []common.Utxo
	for idx, output := range t.Outputs() {
		ret = append(
			ret,
			common.Utxo{
				Id:     shelley.NewShelleyTransactionInput(t.Hash(), idx),
				Output: output,
			},
		)
	}
	return ret
}

func (t AllegraTransaction) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	return t.Body.ProtocolParameterUpdates()
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
