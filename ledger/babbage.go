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
	"encoding/json"
	"fmt"

	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"

	"github.com/blinklabs-io/gouroboros/cbor"
)

const (
	EraIdBabbage = 5

	BlockTypeBabbage = 6

	BlockHeaderTypeBabbage = 5

	TxTypeBabbage = 5
)

type BabbageBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Header                 *BabbageBlockHeader
	TransactionBodies      []BabbageTransactionBody
	TransactionWitnessSets []BabbageTransactionWitnessSet
	TransactionMetadataSet map[uint]*cbor.Value
	InvalidTransactions    []uint
}

func (b *BabbageBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *BabbageBlock) Hash() string {
	return b.Header.Hash()
}

func (b *BabbageBlock) BlockNumber() uint64 {
	return b.Header.BlockNumber()
}

func (b *BabbageBlock) SlotNumber() uint64 {
	return b.Header.SlotNumber()
}

func (b *BabbageBlock) IssuerVkey() IssuerVkey {
	return b.Header.IssuerVkey()
}

func (b *BabbageBlock) BlockBodySize() uint64 {
	return b.Header.BlockBodySize()
}

func (b *BabbageBlock) Era() Era {
	return eras[EraIdBabbage]
}

func (b *BabbageBlock) Transactions() []Transaction {
	invalidTxMap := make(map[uint]bool, len(b.InvalidTransactions))
	for _, invalidTxIdx := range b.InvalidTransactions {
		invalidTxMap[invalidTxIdx] = true
	}

	ret := make([]Transaction, len(b.TransactionBodies))
	for idx := range b.TransactionBodies {
		ret[idx] = &BabbageTransaction{
			Body:       b.TransactionBodies[idx],
			WitnessSet: b.TransactionWitnessSets[idx],
			TxMetadata: b.TransactionMetadataSet[uint(idx)],
			IsTxValid:  !invalidTxMap[uint(idx)],
		}
	}
	return ret
}

func (b *BabbageBlock) Utxorpc() *utxorpc.Block {
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

type BabbageBlockHeader struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash string
	Body struct {
		cbor.StructAsArray
		BlockNumber   uint64
		Slot          uint64
		PrevHash      Blake2b256
		IssuerVkey    IssuerVkey
		VrfKey        []byte
		VrfResult     interface{}
		BlockBodySize uint64
		BlockBodyHash Blake2b256
		OpCert        struct {
			cbor.StructAsArray
			HotVkey        []byte
			SequenceNumber uint32
			KesPeriod      uint32
			Signature      []byte
		}
		ProtoVersion struct {
			cbor.StructAsArray
			Major uint64
			Minor uint64
		}
	}
	Signature interface{}
}

func (h *BabbageBlockHeader) UnmarshalCBOR(cborData []byte) error {
	return h.UnmarshalCbor(cborData, h)
}

func (h *BabbageBlockHeader) Hash() string {
	if h.hash == "" {
		h.hash = generateBlockHeaderHash(h.Cbor(), nil)
	}
	return h.hash
}

func (h *BabbageBlockHeader) BlockNumber() uint64 {
	return h.Body.BlockNumber
}

func (h *BabbageBlockHeader) SlotNumber() uint64 {
	return h.Body.Slot
}

func (h *BabbageBlockHeader) IssuerVkey() IssuerVkey {
	return h.Body.IssuerVkey
}

func (h *BabbageBlockHeader) BlockBodySize() uint64 {
	return h.Body.BlockBodySize
}

func (h *BabbageBlockHeader) Era() Era {
	return eras[EraIdBabbage]
}

type BabbageTransactionBody struct {
	AlonzoTransactionBody
	TxOutputs []BabbageTransactionOutput `cbor:"1,keyasint,omitempty"`
	Update    struct {
		cbor.StructAsArray
		ProtocolParamUpdates map[Blake2b224]BabbageProtocolParameterUpdate
		Epoch                uint64
	} `cbor:"6,keyasint,omitempty"`
	CollateralReturn  BabbageTransactionOutput  `cbor:"16,keyasint,omitempty"`
	TotalCollateral   uint64                    `cbor:"17,keyasint,omitempty"`
	TxReferenceInputs []ShelleyTransactionInput `cbor:"18,keyasint,omitempty"`
}

func (b *BabbageTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *BabbageTransactionBody) Outputs() []TransactionOutput {
	ret := []TransactionOutput{}
	for _, output := range b.TxOutputs {
		output := output
		ret = append(ret, &output)
	}
	return ret
}

func (b *BabbageTransactionBody) ReferenceInputs() []TransactionInput {
	ret := []TransactionInput{}
	for _, input := range b.TxReferenceInputs {
		input := input
		ret = append(ret, &input)
	}
	return ret
}

func (b *BabbageTransactionBody) Utxorpc() *utxorpc.Tx {
	var txi, txri []*utxorpc.TxInput
	var txo []*utxorpc.TxOutput
	for _, i := range b.Inputs() {
		input := i.Utxorpc()
		txi = append(txi, input)
	}
	for _, o := range b.Outputs() {
		output := o.Utxorpc()
		txo = append(txo, output)
	}
	for _, ri := range b.ReferenceInputs() {
		input := ri.Utxorpc()
		txri = append(txri, input)
	}
	tmpHash, err := hex.DecodeString(b.Hash())
	if err != nil {
		return &utxorpc.Tx{}
	}
	tx := &utxorpc.Tx{
		Inputs:  txi,
		Outputs: txo,
		// Certificates: b.Certificates(),
		// Withdrawals:  b.Withdrawals(),
		// Mint:         b.Mint(),
		ReferenceInputs: txri,
		// Witnesses:    b.Witnesses(),
		// Collateral:   b.Collateral(),
		Fee: b.Fee(),
		// Successful:   b.Successful(),
		// Auxiliary:    b.AuxData(),
		// Validity:     b.Validity(),
		Hash: tmpHash,
	}
	return tx
}

const (
	DatumOptionTypeHash = 0
	DatumOptionTypeData = 1
)

type BabbageTransactionOutputDatumOption struct {
	hash *Blake2b256
	data *cbor.LazyValue
}

func (d *BabbageTransactionOutputDatumOption) UnmarshalCBOR(data []byte) error {
	datumOptionType, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	switch datumOptionType {
	case DatumOptionTypeHash:
		var tmpDatumHash struct {
			cbor.StructAsArray
			Type int
			Hash Blake2b256
		}
		if _, err := cbor.Decode(data, &tmpDatumHash); err != nil {
			return err
		}
		d.hash = &(tmpDatumHash.Hash)
	case DatumOptionTypeData:
		var tmpDatumData struct {
			cbor.StructAsArray
			Type     int
			DataCbor []byte
		}
		if _, err := cbor.Decode(data, &tmpDatumData); err != nil {
			return err
		}
		var datumValue cbor.LazyValue
		if _, err := cbor.Decode(tmpDatumData.DataCbor, &datumValue); err != nil {
			return err
		}
		d.data = &(datumValue)
	default:
		return fmt.Errorf("unsupported datum option type: %d", datumOptionType)
	}
	return nil
}

func (d *BabbageTransactionOutputDatumOption) MarshalCBOR() ([]byte, error) {
	var tmpObj []interface{}
	if d.hash != nil {
		tmpObj = []interface{}{DatumOptionTypeHash, d.hash}
	} else if d.data != nil {
		tmpObj = []interface{}{DatumOptionTypeData, cbor.Tag{Number: 24, Content: d.data.Cbor()}}
	} else {
		return nil, fmt.Errorf("unknown datum option type")
	}
	return cbor.Encode(&tmpObj)
}

type BabbageTransactionOutput struct {
	cbor.DecodeStoreCbor
	OutputAddress Address                              `cbor:"0,keyasint,omitempty"`
	OutputAmount  MaryTransactionOutputValue           `cbor:"1,keyasint,omitempty"`
	DatumOption   *BabbageTransactionOutputDatumOption `cbor:"2,keyasint,omitempty"`
	ScriptRef     *cbor.Tag                            `cbor:"3,keyasint,omitempty"`
	legacyOutput  bool
}

func (o *BabbageTransactionOutput) UnmarshalCBOR(cborData []byte) error {
	// Save original CBOR
	o.SetCbor(cborData)
	// Try to parse as legacy output first
	var tmpOutput AlonzoTransactionOutput
	if _, err := cbor.Decode(cborData, &tmpOutput); err == nil {
		// Copy from temp legacy object to Babbage format
		o.OutputAddress = tmpOutput.OutputAddress
		o.OutputAmount = tmpOutput.OutputAmount
		o.legacyOutput = true
	} else {
		return cbor.DecodeGeneric(cborData, o)
	}
	return nil
}

func (o BabbageTransactionOutput) MarshalJSON() ([]byte, error) {
	tmpObj := struct {
		Address   Address                           `json:"address"`
		Amount    uint64                            `json:"amount"`
		Assets    *MultiAsset[MultiAssetTypeOutput] `json:"assets,omitempty"`
		Datum     *cbor.LazyValue                   `json:"datum,omitempty"`
		DatumHash string                            `json:"datumHash,omitempty"`
	}{
		Address: o.OutputAddress,
		Amount:  o.OutputAmount.Amount,
		Assets:  o.OutputAmount.Assets,
	}
	if o.DatumOption != nil {
		if o.DatumOption.hash != nil {
			tmpObj.DatumHash = o.DatumOption.hash.String()
		}
		if o.DatumOption.data != nil {
			tmpObj.Datum = o.DatumOption.data
		}
	}
	return json.Marshal(&tmpObj)
}

func (o BabbageTransactionOutput) Address() Address {
	return o.OutputAddress
}

func (o BabbageTransactionOutput) Amount() uint64 {
	return o.OutputAmount.Amount
}

func (o BabbageTransactionOutput) Assets() *MultiAsset[MultiAssetTypeOutput] {
	return o.OutputAmount.Assets
}

func (o BabbageTransactionOutput) DatumHash() *Blake2b256 {
	if o.DatumOption != nil {
		return o.DatumOption.hash
	}
	return &Blake2b256{}
}

func (o BabbageTransactionOutput) Datum() *cbor.LazyValue {
	if o.DatumOption != nil {
		return o.DatumOption.data
	}
	return nil
}

func (o BabbageTransactionOutput) Utxorpc() *utxorpc.TxOutput {
	return &utxorpc.TxOutput{
		Address: o.OutputAddress.Bytes(),
		Coin:    o.Amount(),
		// Assets: o.Assets(),
		// Datum:     o.Datum(),
		DatumHash: o.DatumHash().Bytes(),
		// Script:    o.ScriptRef,
	}
}

type BabbageTransactionWitnessSet struct {
	AlonzoTransactionWitnessSet
	PlutusV2Scripts []cbor.RawMessage `cbor:"6,keyasint,omitempty"`
}

func (t *BabbageTransactionWitnessSet) UnmarshalCBOR(cborData []byte) error {
	return t.UnmarshalCbor(cborData, t)
}

type BabbageTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       BabbageTransactionBody
	WitnessSet BabbageTransactionWitnessSet
	IsTxValid  bool
	TxMetadata *cbor.Value
}

func (t BabbageTransaction) Hash() string {
	return t.Body.Hash()
}

func (t BabbageTransaction) Inputs() []TransactionInput {
	return t.Body.Inputs()
}

func (t BabbageTransaction) Outputs() []TransactionOutput {
	return t.Body.Outputs()
}

func (t BabbageTransaction) Fee() uint64 {
	return t.Body.Fee()
}

func (t BabbageTransaction) TTL() uint64 {
	return t.Body.TTL()
}

func (t BabbageTransaction) ReferenceInputs() []TransactionInput {
	return t.Body.ReferenceInputs()
}

func (t BabbageTransaction) Metadata() *cbor.Value {
	return t.TxMetadata
}

func (t BabbageTransaction) IsValid() bool {
	return t.IsTxValid
}

func (t *BabbageTransaction) Cbor() []byte {
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

func (t *BabbageTransaction) Utxorpc() *utxorpc.Tx {
	return t.Body.Utxorpc()
}

// BabbageProtocolParameters represents the current Babbage protocol parameters as seen in local-state-query
type BabbageProtocolParameters struct {
	cbor.StructAsArray
	MinFeeA                uint
	MinFeeB                uint
	MaxBlockBodySize       uint
	MaxTxSize              uint
	MaxBlockHeaderSize     uint
	KeyDeposit             uint
	PoolDeposit            uint
	MaxEpoch               uint
	NOpt                   uint
	A0                     *cbor.Rat
	Rho                    *cbor.Rat
	Tau                    *cbor.Rat
	ProtocolMajor          uint
	ProtocolMinor          uint
	MinPoolCost            uint
	AdaPerUtxoByte         uint
	CostModels             map[uint][]int
	ExecutionUnitPrices    []*cbor.Rat // [priceMemory priceSteps]
	MaxTxExecutionUnits    []uint
	MaxBlockExecutionUnits []uint
	MaxValueSize           uint
	CollateralPercentage   uint
	MaxCollateralInputs    uint
}

type BabbageProtocolParameterUpdate struct {
	MinFeeA            uint      `cbor:"0,keyasint"`
	MinFeeB            uint      `cbor:"1,keyasint"`
	MaxBlockBodySize   uint      `cbor:"2,keyasint"`
	MaxTxSize          uint      `cbor:"3,keyasint"`
	MaxBlockHeaderSize uint      `cbor:"4,keyasint"`
	KeyDeposit         uint      `cbor:"5,keyasint"`
	PoolDeposit        uint      `cbor:"6,keyasint"`
	MaxEpoch           uint      `cbor:"7,keyasint"`
	NOpt               uint      `cbor:"8,keyasint"`
	A0                 *cbor.Rat `cbor:"9,keyasint"`
	Rho                *cbor.Rat `cbor:"10,keyasint"`
	Tau                *cbor.Rat `cbor:"11,keyasint"`
	ProtocolVersion    struct {
		cbor.StructAsArray
		Major uint
		Minor uint
	} `cbor:"14,keyasint"`
	MinPoolCost            uint           `cbor:"16,keyasint"`
	AdaPerUtxoByte         uint           `cbor:"17,keyasint"`
	CostModels             map[uint][]int `cbor:"18,keyasint"`
	ExecutionUnitPrices    []*cbor.Rat    `cbor:"19,keyasint"`
	MaxTxExecutionUnits    []uint         `cbor:"20,keyasint"`
	MaxBlockExecutionUnits []uint         `cbor:"21,keyasint"`
	MaxValueSize           uint           `cbor:"22,keyasint"`
	CollateralPercentage   uint           `cbor:"23,keyasint"`
	MaxCollateralInputs    uint           `cbor:"24,keyasint"`
}

func NewBabbageBlockFromCbor(data []byte) (*BabbageBlock, error) {
	var babbageBlock BabbageBlock
	if _, err := cbor.Decode(data, &babbageBlock); err != nil {
		return nil, fmt.Errorf("Babbage block decode error: %s", err)
	}
	return &babbageBlock, nil
}

func NewBabbageBlockHeaderFromCbor(data []byte) (*BabbageBlockHeader, error) {
	var babbageBlockHeader BabbageBlockHeader
	if _, err := cbor.Decode(data, &babbageBlockHeader); err != nil {
		return nil, fmt.Errorf("Babbage block header decode error: %s", err)
	}
	return &babbageBlockHeader, nil
}

func NewBabbageTransactionBodyFromCbor(
	data []byte,
) (*BabbageTransactionBody, error) {
	var babbageTx BabbageTransactionBody
	if _, err := cbor.Decode(data, &babbageTx); err != nil {
		return nil, fmt.Errorf("Babbage transaction body decode error: %s", err)
	}
	return &babbageTx, nil
}

func NewBabbageTransactionFromCbor(data []byte) (*BabbageTransaction, error) {
	var babbageTx BabbageTransaction
	if _, err := cbor.Decode(data, &babbageTx); err != nil {
		return nil, fmt.Errorf("Babbage transaction decode error: %s", err)
	}
	return &babbageTx, nil
}

func NewBabbageTransactionOutputFromCbor(data []byte) (*BabbageTransactionOutput, error) {
	var babbageTxOutput BabbageTransactionOutput
	if _, err := cbor.Decode(data, &babbageTxOutput); err != nil {
		return nil, fmt.Errorf("Babbage transaction output decode error: %s", err)
	}
	return &babbageTxOutput, nil
}
