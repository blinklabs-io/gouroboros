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

package babbage

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"

	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

const (
	EraIdBabbage   = 5
	EraNameBabbage = "Babbage"

	BlockTypeBabbage = 6

	BlockHeaderTypeBabbage = 5

	TxTypeBabbage = 5
)

var (
	EraBabbage = common.Era{
		Id:   EraIdBabbage,
		Name: EraNameBabbage,
	}
)

func init() {
	common.RegisterEra(EraBabbage)
}

type BabbageBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	BlockHeader            *BabbageBlockHeader
	TransactionBodies      []BabbageTransactionBody
	TransactionWitnessSets []BabbageTransactionWitnessSet
	TransactionMetadataSet map[uint]*cbor.LazyValue
	InvalidTransactions    []uint
}

func (b *BabbageBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (BabbageBlock) Type() int {
	return BlockTypeBabbage
}

func (b *BabbageBlock) Hash() string {
	return b.BlockHeader.Hash()
}

func (b *BabbageBlock) Header() common.BlockHeader {
	return b.BlockHeader
}

func (b *BabbageBlock) PrevHash() string {
	return b.BlockHeader.PrevHash()
}

func (b *BabbageBlock) BlockNumber() uint64 {
	return b.BlockHeader.BlockNumber()
}

func (b *BabbageBlock) SlotNumber() uint64 {
	return b.BlockHeader.SlotNumber()
}

func (b *BabbageBlock) IssuerVkey() common.IssuerVkey {
	return b.BlockHeader.IssuerVkey()
}

func (b *BabbageBlock) BlockBodySize() uint64 {
	return b.BlockHeader.BlockBodySize()
}

func (b *BabbageBlock) Era() common.Era {
	return EraBabbage
}

func (b *BabbageBlock) Transactions() []common.Transaction {
	invalidTxMap := make(map[uint]bool, len(b.InvalidTransactions))
	for _, invalidTxIdx := range b.InvalidTransactions {
		invalidTxMap[invalidTxIdx] = true
	}

	ret := make([]common.Transaction, len(b.TransactionBodies))
	// #nosec G115
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
		PrevHash      common.Blake2b256
		IssuerVkey    common.IssuerVkey
		VrfKey        []byte
		VrfResult     common.VrfResult
		BlockBodySize uint64
		BlockBodyHash common.Blake2b256
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
	Signature []byte
}

func (h *BabbageBlockHeader) UnmarshalCBOR(cborData []byte) error {
	return h.UnmarshalCbor(cborData, h)
}

func (h *BabbageBlockHeader) Hash() string {
	if h.hash == "" {
		tmpHash := common.Blake2b256Hash(h.Cbor())
		h.hash = hex.EncodeToString(tmpHash.Bytes())
	}
	return h.hash
}

func (h *BabbageBlockHeader) PrevHash() string {
	return h.Body.PrevHash.String()
}

func (h *BabbageBlockHeader) BlockNumber() uint64 {
	return h.Body.BlockNumber
}

func (h *BabbageBlockHeader) SlotNumber() uint64 {
	return h.Body.Slot
}

func (h *BabbageBlockHeader) IssuerVkey() common.IssuerVkey {
	return h.Body.IssuerVkey
}

func (h *BabbageBlockHeader) BlockBodySize() uint64 {
	return h.Body.BlockBodySize
}

func (h *BabbageBlockHeader) Era() common.Era {
	return EraBabbage
}

type BabbageTransactionBody struct {
	alonzo.AlonzoTransactionBody
	TxOutputs []BabbageTransactionOutput `cbor:"1,keyasint,omitempty"`
	Update    struct {
		cbor.StructAsArray
		ProtocolParamUpdates map[common.Blake2b224]BabbageProtocolParameterUpdate
		Epoch                uint64
	} `cbor:"6,keyasint,omitempty"`
	TxCollateralReturn *BabbageTransactionOutput         `cbor:"16,keyasint,omitempty"`
	TxTotalCollateral  uint64                            `cbor:"17,keyasint,omitempty"`
	TxReferenceInputs  []shelley.ShelleyTransactionInput `cbor:"18,keyasint,omitempty"`
}

func (b *BabbageTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *BabbageTransactionBody) Outputs() []common.TransactionOutput {
	ret := []common.TransactionOutput{}
	for _, output := range b.TxOutputs {
		output := output
		ret = append(ret, &output)
	}
	return ret
}

func (b *BabbageTransactionBody) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	updateMap := make(map[common.Blake2b224]common.ProtocolParameterUpdate)
	for k, v := range b.Update.ProtocolParamUpdates {
		updateMap[k] = v
	}
	return b.Update.Epoch, updateMap
}

func (b *BabbageTransactionBody) ReferenceInputs() []common.TransactionInput {
	ret := []common.TransactionInput{}
	for _, input := range b.TxReferenceInputs {
		input := input
		ret = append(ret, &input)
	}
	return ret
}

func (b *BabbageTransactionBody) CollateralReturn() common.TransactionOutput {
	// Return an actual nil if we have no value. If we return our nil pointer,
	// we get a non-nil interface containing a nil value, which is harder to
	// compare against
	if b.TxCollateralReturn == nil {
		return nil
	}
	return b.TxCollateralReturn
}

func (b *BabbageTransactionBody) TotalCollateral() uint64 {
	return b.TxTotalCollateral
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
	hash *common.Blake2b256
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
			Hash common.Blake2b256
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
	OutputAddress common.Address                       `cbor:"0,keyasint,omitempty"`
	OutputAmount  mary.MaryTransactionOutputValue      `cbor:"1,keyasint,omitempty"`
	DatumOption   *BabbageTransactionOutputDatumOption `cbor:"2,keyasint,omitempty"`
	ScriptRef     *cbor.Tag                            `cbor:"3,keyasint,omitempty"`
	legacyOutput  bool
}

func (o *BabbageTransactionOutput) UnmarshalCBOR(cborData []byte) error {
	// Save original CBOR
	o.SetCbor(cborData)
	// Try to parse as legacy output first
	var tmpOutput alonzo.AlonzoTransactionOutput
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

func (o *BabbageTransactionOutput) MarshalCBOR() ([]byte, error) {
	if o.legacyOutput {
		tmpOutput := alonzo.AlonzoTransactionOutput{
			OutputAddress: o.OutputAddress,
			OutputAmount:  o.OutputAmount,
		}
		return cbor.Encode(&tmpOutput)
	}
	return cbor.EncodeGeneric(o)
}

func (o BabbageTransactionOutput) MarshalJSON() ([]byte, error) {
	tmpObj := struct {
		Address   common.Address                                  `json:"address"`
		Amount    uint64                                          `json:"amount"`
		Assets    *common.MultiAsset[common.MultiAssetTypeOutput] `json:"assets,omitempty"`
		Datum     *cbor.LazyValue                                 `json:"datum,omitempty"`
		DatumHash string                                          `json:"datumHash,omitempty"`
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

func (o BabbageTransactionOutput) Address() common.Address {
	return o.OutputAddress
}

func (o BabbageTransactionOutput) Amount() uint64 {
	return o.OutputAmount.Amount
}

func (o BabbageTransactionOutput) Assets() *common.MultiAsset[common.MultiAssetTypeOutput] {
	return o.OutputAmount.Assets
}

func (o BabbageTransactionOutput) DatumHash() *common.Blake2b256 {
	if o.DatumOption != nil {
		return o.DatumOption.hash
	}
	return &common.Blake2b256{}
}

func (o BabbageTransactionOutput) Datum() *cbor.LazyValue {
	if o.DatumOption != nil {
		return o.DatumOption.data
	}
	return nil
}

func (o BabbageTransactionOutput) Utxorpc() *utxorpc.TxOutput {
	var address []byte
	if o.OutputAddress.Bytes() == nil {
		address = []byte{}
	} else {
		address = o.OutputAddress.Bytes()
	}

	var assets []*utxorpc.Multiasset
	if o.Assets() != nil {
		tmpAssets := o.Assets()
		for _, policyId := range tmpAssets.Policies() {
			var ma = &utxorpc.Multiasset{
				PolicyId: policyId.Bytes(),
			}
			for _, assetName := range tmpAssets.Assets(policyId) {
				amount := tmpAssets.Asset(policyId, assetName)
				asset := &utxorpc.Asset{
					Name:       assetName,
					OutputCoin: amount,
				}
				ma.Assets = append(ma.Assets, asset)
			}
			assets = append(assets, ma)
		}
	}

	var datumHash []byte
	if o.DatumHash() == nil {
		datumHash = []byte{}
	} else {
		datumHash = o.DatumHash().Bytes()
	}

	return &utxorpc.TxOutput{
		Address: address,
		Coin:    o.Amount(),
		Assets:  assets,
		Datum: &utxorpc.Datum{
			Hash: datumHash,
			// OriginalCbor: o.Datum().Cbor(),
		},
		// Script:    o.ScriptRef,
	}
}

type BabbageTransactionWitnessSet struct {
	alonzo.AlonzoTransactionWitnessSet
	WsPlutusV2Scripts [][]byte `cbor:"6,keyasint,omitempty"`
}

func (w *BabbageTransactionWitnessSet) UnmarshalCBOR(cborData []byte) error {
	return w.UnmarshalCbor(cborData, w)
}

func (w BabbageTransactionWitnessSet) PlutusV2Scripts() [][]byte {
	return w.WsPlutusV2Scripts
}

type BabbageTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       BabbageTransactionBody
	WitnessSet BabbageTransactionWitnessSet
	IsTxValid  bool
	TxMetadata *cbor.LazyValue
}

func (BabbageTransaction) Type() int {
	return TxTypeBabbage
}

func (t BabbageTransaction) Hash() string {
	return t.Body.Hash()
}

func (t BabbageTransaction) Inputs() []common.TransactionInput {
	return t.Body.Inputs()
}

func (t BabbageTransaction) Outputs() []common.TransactionOutput {
	return t.Body.Outputs()
}

func (t BabbageTransaction) Fee() uint64 {
	return t.Body.Fee()
}

func (t BabbageTransaction) TTL() uint64 {
	return t.Body.TTL()
}

func (t BabbageTransaction) ValidityIntervalStart() uint64 {
	return t.Body.ValidityIntervalStart()
}

func (t BabbageTransaction) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	return t.Body.ProtocolParameterUpdates()
}

func (t BabbageTransaction) ReferenceInputs() []common.TransactionInput {
	return t.Body.ReferenceInputs()
}

func (t BabbageTransaction) Collateral() []common.TransactionInput {
	return t.Body.Collateral()
}

func (t BabbageTransaction) CollateralReturn() common.TransactionOutput {
	return t.Body.CollateralReturn()
}

func (t BabbageTransaction) TotalCollateral() uint64 {
	return t.Body.TotalCollateral()
}

func (t BabbageTransaction) Certificates() []common.Certificate {
	return t.Body.Certificates()
}

func (t BabbageTransaction) Withdrawals() map[*common.Address]uint64 {
	return t.Body.Withdrawals()
}

func (t BabbageTransaction) AuxDataHash() *common.Blake2b256 {
	return t.Body.AuxDataHash()
}

func (t BabbageTransaction) ScriptDataHash() *common.Blake2b256 {
	return t.Body.ScriptDataHash()
}

func (t BabbageTransaction) VotingProcedures() common.VotingProcedures {
	return t.Body.VotingProcedures()
}

func (t BabbageTransaction) RequiredSigners() []common.Blake2b224 {
	return t.Body.RequiredSigners()
}

func (t BabbageTransaction) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return t.Body.AssetMint()
}

func (t BabbageTransaction) ProposalProcedures() []common.ProposalProcedure {
	return t.Body.ProposalProcedures()
}

func (t BabbageTransaction) CurrentTreasuryValue() int64 {
	return t.Body.CurrentTreasuryValue()
}

func (t BabbageTransaction) Donation() uint64 {
	return t.Body.Donation()
}

func (t BabbageTransaction) Metadata() *cbor.LazyValue {
	return t.TxMetadata
}

func (t BabbageTransaction) IsValid() bool {
	return t.IsTxValid
}

func (t BabbageTransaction) Consumed() []common.TransactionInput {
	if t.IsValid() {
		return t.Inputs()
	} else {
		return t.Collateral()
	}
}

func (t BabbageTransaction) Produced() []common.Utxo {
	if t.IsValid() {
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
	} else {
		if t.CollateralReturn() == nil {
			return []common.Utxo{}
		}
		return []common.Utxo{
			{
				Id:     shelley.NewShelleyTransactionInput(t.Hash(), len(t.Outputs())),
				Output: t.CollateralReturn(),
			},
		}
	}
}

func (t BabbageTransaction) Witnesses() common.TransactionWitnessSet {
	return t.WitnessSet
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

func NewBabbageTransactionOutputFromCbor(
	data []byte,
) (*BabbageTransactionOutput, error) {
	var babbageTxOutput BabbageTransactionOutput
	if _, err := cbor.Decode(data, &babbageTxOutput); err != nil {
		return nil, fmt.Errorf(
			"Babbage transaction output decode error: %s",
			err,
		)
	}
	return &babbageTxOutput, nil
}
