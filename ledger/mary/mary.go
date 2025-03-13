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

package mary

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

const (
	EraIdMary   = 3
	EraNameMary = "Mary"

	BlockTypeMary = 4

	BlockHeaderTypeMary = 3

	TxTypeMary = 3
)

var EraMary = common.Era{
	Id:   EraIdMary,
	Name: EraNameMary,
}

func init() {
	common.RegisterEra(EraMary)
}

type MaryBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	BlockHeader            *MaryBlockHeader
	TransactionBodies      []MaryTransactionBody
	TransactionWitnessSets []shelley.ShelleyTransactionWitnessSet
	TransactionMetadataSet map[uint]*cbor.LazyValue
}

func (b *MaryBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (MaryBlock) Type() int {
	return BlockTypeMary
}

func (b *MaryBlock) Hash() string {
	return b.BlockHeader.Hash()
}

func (b *MaryBlock) Header() common.BlockHeader {
	return b.BlockHeader
}

func (b *MaryBlock) PrevHash() string {
	return b.BlockHeader.PrevHash()
}

func (b *MaryBlock) BlockNumber() uint64 {
	return b.BlockHeader.BlockNumber()
}

func (b *MaryBlock) SlotNumber() uint64 {
	return b.BlockHeader.SlotNumber()
}

func (b *MaryBlock) IssuerVkey() common.IssuerVkey {
	return b.BlockHeader.IssuerVkey()
}

func (b *MaryBlock) BlockBodySize() uint64 {
	return b.BlockHeader.BlockBodySize()
}

func (b *MaryBlock) Era() common.Era {
	return EraMary
}

func (b *MaryBlock) Transactions() []common.Transaction {
	ret := make([]common.Transaction, len(b.TransactionBodies))
	// #nosec G115
	for idx := range b.TransactionBodies {
		ret[idx] = &MaryTransaction{
			Body:       b.TransactionBodies[idx],
			WitnessSet: b.TransactionWitnessSets[idx],
			TxMetadata: b.TransactionMetadataSet[uint(idx)],
		}
	}
	return ret
}

func (b *MaryBlock) Utxorpc() *utxorpc.Block {
	txs := []*utxorpc.Tx{}
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

type MaryBlockHeader struct {
	shelley.ShelleyBlockHeader
}

func (h *MaryBlockHeader) Era() common.Era {
	return EraMary
}

type MaryTransactionBody struct {
	allegra.AllegraTransactionBody
	Update struct {
		cbor.StructAsArray
		ProtocolParamUpdates map[common.Blake2b224]MaryProtocolParameterUpdate
		Epoch                uint64
	} `cbor:"6,keyasint,omitempty"`
	TxOutputs []MaryTransactionOutput                       `cbor:"1,keyasint,omitempty"`
	TxMint    *common.MultiAsset[common.MultiAssetTypeMint] `cbor:"9,keyasint,omitempty"`
}

func (b *MaryTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *MaryTransactionBody) Outputs() []common.TransactionOutput {
	ret := []common.TransactionOutput{}
	for _, output := range b.TxOutputs {
		ret = append(ret, &output)
	}
	return ret
}

func (b *MaryTransactionBody) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	updateMap := make(map[common.Blake2b224]common.ProtocolParameterUpdate)
	for k, v := range b.Update.ProtocolParamUpdates {
		updateMap[k] = v
	}
	return b.Update.Epoch, updateMap
}

func (b *MaryTransactionBody) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return b.TxMint
}

type MaryTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       MaryTransactionBody
	WitnessSet shelley.ShelleyTransactionWitnessSet
	TxMetadata *cbor.LazyValue
}

func (MaryTransaction) Type() int {
	return TxTypeMary
}

func (t MaryTransaction) Hash() string {
	return t.Body.Hash()
}

func (t MaryTransaction) Inputs() []common.TransactionInput {
	return t.Body.Inputs()
}

func (t MaryTransaction) Outputs() []common.TransactionOutput {
	return t.Body.Outputs()
}

func (t MaryTransaction) Fee() uint64 {
	return t.Body.Fee()
}

func (t MaryTransaction) TTL() uint64 {
	return t.Body.TTL()
}

func (t MaryTransaction) ValidityIntervalStart() uint64 {
	return t.Body.ValidityIntervalStart()
}

func (t MaryTransaction) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	return t.Body.ProtocolParameterUpdates()
}

func (t MaryTransaction) ReferenceInputs() []common.TransactionInput {
	return t.Body.ReferenceInputs()
}

func (t MaryTransaction) Collateral() []common.TransactionInput {
	return t.Body.Collateral()
}

func (t MaryTransaction) CollateralReturn() common.TransactionOutput {
	return t.Body.CollateralReturn()
}

func (t MaryTransaction) TotalCollateral() uint64 {
	return t.Body.TotalCollateral()
}

func (t MaryTransaction) Certificates() []common.Certificate {
	return t.Body.Certificates()
}

func (t MaryTransaction) Withdrawals() map[*common.Address]uint64 {
	return t.Body.Withdrawals()
}

func (t MaryTransaction) AuxDataHash() *common.Blake2b256 {
	return t.Body.AuxDataHash()
}

func (t MaryTransaction) RequiredSigners() []common.Blake2b224 {
	return t.Body.RequiredSigners()
}

func (t MaryTransaction) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return t.Body.AssetMint()
}

func (t MaryTransaction) ScriptDataHash() *common.Blake2b256 {
	return t.Body.ScriptDataHash()
}

func (t MaryTransaction) VotingProcedures() common.VotingProcedures {
	return t.Body.VotingProcedures()
}

func (t MaryTransaction) ProposalProcedures() []common.ProposalProcedure {
	return t.Body.ProposalProcedures()
}

func (t MaryTransaction) CurrentTreasuryValue() int64 {
	return t.Body.CurrentTreasuryValue()
}

func (t MaryTransaction) Donation() uint64 {
	return t.Body.Donation()
}

func (t MaryTransaction) Metadata() *cbor.LazyValue {
	return t.TxMetadata
}

func (t MaryTransaction) IsValid() bool {
	return true
}

func (t MaryTransaction) Consumed() []common.TransactionInput {
	return t.Inputs()
}

func (t MaryTransaction) Produced() []common.Utxo {
	ret := []common.Utxo{}
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

func (t MaryTransaction) Witnesses() common.TransactionWitnessSet {
	return t.WitnessSet
}

func (t *MaryTransaction) Cbor() []byte {
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

func (t *MaryTransaction) Utxorpc() *utxorpc.Tx {
	return t.Body.Utxorpc()
}

type MaryTransactionOutput struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	OutputAddress common.Address
	OutputAmount  MaryTransactionOutputValue
}

func (o MaryTransactionOutput) MarshalJSON() ([]byte, error) {
	tmpObj := struct {
		Address common.Address                                  `json:"address"`
		Amount  uint64                                          `json:"amount"`
		Assets  *common.MultiAsset[common.MultiAssetTypeOutput] `json:"assets,omitempty"`
	}{
		Address: o.OutputAddress,
		Amount:  o.OutputAmount.Amount,
		Assets:  o.OutputAmount.Assets,
	}
	return json.Marshal(&tmpObj)
}

func (o MaryTransactionOutput) Address() common.Address {
	return o.OutputAddress
}

func (o MaryTransactionOutput) Amount() uint64 {
	return o.OutputAmount.Amount
}

func (o MaryTransactionOutput) Assets() *common.MultiAsset[common.MultiAssetTypeOutput] {
	return o.OutputAmount.Assets
}

func (o MaryTransactionOutput) DatumHash() *common.Blake2b256 {
	return nil
}

func (o MaryTransactionOutput) Datum() *cbor.LazyValue {
	return nil
}

func (o MaryTransactionOutput) Utxorpc() *utxorpc.TxOutput {
	return &utxorpc.TxOutput{
		Address: o.OutputAddress.Bytes(),
		Coin:    o.Amount(),
		// Assets: o.Assets,
	}
}

type MaryTransactionOutputValue struct {
	cbor.StructAsArray
	Amount uint64
	// We use a pointer here to allow it to be nil
	Assets *common.MultiAsset[common.MultiAssetTypeOutput]
}

func (v *MaryTransactionOutputValue) UnmarshalCBOR(data []byte) error {
	if _, err := cbor.Decode(data, &(v.Amount)); err == nil {
		return nil
	}
	if err := cbor.DecodeGeneric(data, v); err != nil {
		return err
	}
	return nil
}

func (v *MaryTransactionOutputValue) MarshalCBOR() ([]byte, error) {
	if v.Assets == nil {
		return cbor.Encode(v.Amount)
	} else {
		return cbor.EncodeGeneric(v)
	}
}

func NewMaryBlockFromCbor(data []byte) (*MaryBlock, error) {
	var maryBlock MaryBlock
	if _, err := cbor.Decode(data, &maryBlock); err != nil {
		return nil, fmt.Errorf("Mary block decode error: %w", err)
	}
	return &maryBlock, nil
}

func NewMaryTransactionBodyFromCbor(data []byte) (*MaryTransactionBody, error) {
	var maryTx MaryTransactionBody
	if _, err := cbor.Decode(data, &maryTx); err != nil {
		return nil, fmt.Errorf("Mary transaction body decode error: %w", err)
	}
	return &maryTx, nil
}

func NewMaryTransactionFromCbor(data []byte) (*MaryTransaction, error) {
	var maryTx MaryTransaction
	if _, err := cbor.Decode(data, &maryTx); err != nil {
		return nil, fmt.Errorf("Mary transaction decode error: %w", err)
	}
	return &maryTx, nil
}

func NewMaryTransactionOutputFromCbor(
	data []byte,
) (*MaryTransactionOutput, error) {
	var maryTxOutput MaryTransactionOutput
	if _, err := cbor.Decode(data, &maryTxOutput); err != nil {
		return nil, fmt.Errorf("Mary transaction output decode error: %w", err)
	}
	return &maryTxOutput, nil
}
