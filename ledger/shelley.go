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
	EraIdShelley = 1

	BlockTypeShelley = 2

	BlockHeaderTypeShelley = 1

	TxTypeShelley = 1
)

type ShelleyBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Header                 *ShelleyBlockHeader
	TransactionBodies      []ShelleyTransactionBody
	TransactionWitnessSets []ShelleyTransactionWitnessSet
	TransactionMetadataSet map[uint]*cbor.Value
}

func (b *ShelleyBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *ShelleyBlock) Hash() string {
	return b.Header.Hash()
}

func (b *ShelleyBlock) BlockNumber() uint64 {
	return b.Header.BlockNumber()
}

func (b *ShelleyBlock) SlotNumber() uint64 {
	return b.Header.SlotNumber()
}

func (b *ShelleyBlock) IssuerVkey() IssuerVkey {
	return b.Header.IssuerVkey()
}

func (b *ShelleyBlock) BlockBodySize() uint64 {
	return b.Header.BlockBodySize()
}

func (b *ShelleyBlock) Era() Era {
	return eras[EraIdShelley]
}

func (b *ShelleyBlock) Transactions() []Transaction {
	ret := []Transaction{}
	for idx := range b.TransactionBodies {
		tmpTransaction := ShelleyTransaction{
			Body:       b.TransactionBodies[idx],
			WitnessSet: b.TransactionWitnessSets[idx],
			TxMetadata: b.TransactionMetadataSet[uint(idx)],
		}
		ret = append(ret, &tmpTransaction)
	}
	return ret
}

func (b *ShelleyBlock) Utxorpc() *utxorpc.Block {
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

type ShelleyBlockHeader struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash string
	Body struct {
		cbor.StructAsArray
		BlockNumber          uint64
		Slot                 uint64
		PrevHash             Blake2b256
		IssuerVkey           IssuerVkey
		VrfKey               []byte
		NonceVrf             interface{}
		LeaderVrf            interface{}
		BlockBodySize        uint64
		BlockBodyHash        Blake2b256
		OpCertHotVkey        []byte
		OpCertSequenceNumber uint32
		OpCertKesPeriod      uint32
		OpCertSignature      []byte
		ProtoMajorVersion    uint64
		ProtoMinorVersion    uint64
	}
	Signature interface{}
}

func (h *ShelleyBlockHeader) UnmarshalCBOR(cborData []byte) error {
	return h.UnmarshalCbor(cborData, h)
}

func (h *ShelleyBlockHeader) Hash() string {
	if h.hash == "" {
		h.hash = generateBlockHeaderHash(h.Cbor(), nil)
	}
	return h.hash
}

func (h *ShelleyBlockHeader) BlockNumber() uint64 {
	return h.Body.BlockNumber
}

func (h *ShelleyBlockHeader) SlotNumber() uint64 {
	return h.Body.Slot
}

func (h *ShelleyBlockHeader) IssuerVkey() IssuerVkey {
	return h.Body.IssuerVkey
}

func (h *ShelleyBlockHeader) BlockBodySize() uint64 {
	return h.Body.BlockBodySize
}

func (h *ShelleyBlockHeader) Era() Era {
	return eras[EraIdShelley]
}

type ShelleyTransactionBody struct {
	cbor.DecodeStoreCbor
	hash      string
	TxInputs  []ShelleyTransactionInput  `cbor:"0,keyasint,omitempty"`
	TxOutputs []ShelleyTransactionOutput `cbor:"1,keyasint,omitempty"`
	TxFee     uint64                     `cbor:"2,keyasint,omitempty"`
	Ttl       uint64                     `cbor:"3,keyasint,omitempty"`
	// TODO: figure out how to parse properly
	Certificates cbor.RawMessage `cbor:"4,keyasint,omitempty"`
	// TODO: figure out how to parse this correctly
	// We keep the raw CBOR because it can contain a map with []byte keys, which
	// Go does not allow
	Withdrawals cbor.Value `cbor:"5,keyasint,omitempty"`
	Update      struct {
		cbor.StructAsArray
		ProtocolParamUpdates cbor.Value
		Epoch                uint64
	} `cbor:"6,keyasint,omitempty"`
	MetadataHash Blake2b256 `cbor:"7,keyasint,omitempty"`
}

func (b *ShelleyTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *ShelleyTransactionBody) Hash() string {
	if b.hash == "" {
		b.hash = generateTransactionHash(b.Cbor(), nil)
	}
	return b.hash
}

func (b *ShelleyTransactionBody) Inputs() []TransactionInput {
	ret := []TransactionInput{}
	for _, input := range b.TxInputs {
		ret = append(ret, input)
	}
	return ret
}

func (b *ShelleyTransactionBody) Outputs() []TransactionOutput {
	ret := []TransactionOutput{}
	for _, output := range b.TxOutputs {
		output := output
		ret = append(ret, &output)
	}
	return ret
}

func (b *ShelleyTransactionBody) Fee() uint64 {
	return b.TxFee
}

func (b *ShelleyTransactionBody) TTL() uint64 {
	return b.Ttl
}

func (b *ShelleyTransactionBody) Utxorpc() *utxorpc.Tx {
	var tx *utxorpc.Tx
	var txi []*utxorpc.TxInput
	for _, i := range b.Inputs() {
		input := i.Utxorpc()
		txi = append(txi, input)
	}
	tx.Inputs = txi
	var txo []*utxorpc.TxOutput
	for _, o := range b.Outputs() {
		output := o.Utxorpc()
		txo = append(txo, output)
	}
	tx.Outputs = txo
	tx.Fee = b.Fee()
	// tx.Validity = b.Validity()
	// tx.Certificates = b.Certificates()
	// tx.Withdrawals = b.Withdrawals()
	// tx.Witnesses = b.Witnesses()
	// tx.Successful = b.Successful()
	// tx.Auxiliary = b.AuxData()
	tmpHash, _ := hex.DecodeString(b.Hash())
	tx.Hash = tmpHash
	return tx
}

type ShelleyTransactionInput struct {
	cbor.StructAsArray
	TxId        Blake2b256
	OutputIndex uint32
}

func (i ShelleyTransactionInput) Id() Blake2b256 {
	return i.TxId
}

func (i ShelleyTransactionInput) Index() uint32 {
	return i.OutputIndex
}

func (i ShelleyTransactionInput) Utxorpc() *utxorpc.TxInput {
	return &utxorpc.TxInput{
		TxHash:      i.TxId.Bytes(),
		OutputIndex: i.OutputIndex,
		// AsOutput: i.AsOutput,
		// Redeemer: i.Redeemer,
	}
}

func (i ShelleyTransactionInput) String() string {
	return fmt.Sprintf("%s#%d", i.TxId, i.OutputIndex)
}

func (i ShelleyTransactionInput) MarshalJSON() ([]byte, error) {
	return []byte("\"" + i.String() + "\""), nil
}

type ShelleyTransactionOutput struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	OutputAddress Address `json:"address"`
	OutputAmount  uint64  `json:"amount"`
}

func (o ShelleyTransactionOutput) Address() Address {
	return o.OutputAddress
}

func (o ShelleyTransactionOutput) Amount() uint64 {
	return o.OutputAmount
}

func (o ShelleyTransactionOutput) Assets() *MultiAsset[MultiAssetTypeOutput] {
	return nil
}

func (o ShelleyTransactionOutput) DatumHash() *Blake2b256 {
	return nil
}

func (o ShelleyTransactionOutput) Datum() *cbor.LazyValue {
	return nil
}

func (o ShelleyTransactionOutput) Utxorpc() *utxorpc.TxOutput {
	return &utxorpc.TxOutput{
		Address: o.OutputAddress.Bytes(),
		Coin:    o.Amount(),
	}
}

type ShelleyTransactionWitnessSet struct {
	cbor.DecodeStoreCbor
	VkeyWitnesses      []interface{} `cbor:"0,keyasint,omitempty"`
	MultisigScripts    []interface{} `cbor:"1,keyasint,omitempty"`
	BootstrapWitnesses []interface{} `cbor:"2,keyasint,omitempty"`
}

func (t *ShelleyTransactionWitnessSet) UnmarshalCBOR(cborData []byte) error {
	return t.UnmarshalCbor(cborData, t)
}

type ShelleyTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       ShelleyTransactionBody
	WitnessSet ShelleyTransactionWitnessSet
	TxMetadata *cbor.Value
}

func (t ShelleyTransaction) Hash() string {
	return t.Body.Hash()
}

func (t ShelleyTransaction) Inputs() []TransactionInput {
	return t.Body.Inputs()
}

func (t ShelleyTransaction) Outputs() []TransactionOutput {
	return t.Body.Outputs()
}

func (t ShelleyTransaction) Fee() uint64 {
	return t.Body.Fee()
}

func (t ShelleyTransaction) TTL() uint64 {
	return t.Body.TTL()
}

func (t ShelleyTransaction) Metadata() *cbor.Value {
	return t.TxMetadata
}

func (t ShelleyTransaction) Utxorpc() *utxorpc.Tx {
	return t.Body.Utxorpc()
}

func (t *ShelleyTransaction) Cbor() []byte {
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

func NewShelleyBlockFromCbor(data []byte) (*ShelleyBlock, error) {
	var shelleyBlock ShelleyBlock
	if _, err := cbor.Decode(data, &shelleyBlock); err != nil {
		return nil, fmt.Errorf("Shelley block decode error: %s", err)
	}
	return &shelleyBlock, nil
}

func NewShelleyBlockHeaderFromCbor(data []byte) (*ShelleyBlockHeader, error) {
	var shelleyBlockHeader ShelleyBlockHeader
	if _, err := cbor.Decode(data, &shelleyBlockHeader); err != nil {
		return nil, fmt.Errorf("Shelley block header decode error: %s", err)
	}
	return &shelleyBlockHeader, nil
}

func NewShelleyTransactionBodyFromCbor(
	data []byte,
) (*ShelleyTransactionBody, error) {
	var shelleyTx ShelleyTransactionBody
	if _, err := cbor.Decode(data, &shelleyTx); err != nil {
		return nil, fmt.Errorf("Shelley transaction body decode error: %s", err)
	}
	return &shelleyTx, nil
}

func NewShelleyTransactionFromCbor(data []byte) (*ShelleyTransaction, error) {
	var shelleyTx ShelleyTransaction
	if _, err := cbor.Decode(data, &shelleyTx); err != nil {
		return nil, fmt.Errorf("Shelley transaction decode error: %s", err)
	}
	return &shelleyTx, nil
}

func NewShelleyTransactionOutputFromCbor(data []byte) (*ShelleyTransactionOutput, error) {
	var shelleyTxOutput ShelleyTransactionOutput
	if _, err := cbor.Decode(data, &shelleyTxOutput); err != nil {
		return nil, fmt.Errorf("Shelley transaction output decode error: %s", err)
	}
	return &shelleyTxOutput, nil
}
