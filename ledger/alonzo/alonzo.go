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

package alonzo

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

const (
	EraIdAlonzo   = 4
	EraNameAlonzo = "Alonzo"

	BlockTypeAlonzo = 5

	BlockHeaderTypeAlonzo = 4

	TxTypeAlonzo = 4
)

var EraAlonzo = common.Era{
	Id:   EraIdAlonzo,
	Name: EraNameAlonzo,
}

func init() {
	common.RegisterEra(EraAlonzo)
}

type AlonzoBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	BlockHeader            *AlonzoBlockHeader
	TransactionBodies      []AlonzoTransactionBody
	TransactionWitnessSets []AlonzoTransactionWitnessSet
	TransactionMetadataSet map[uint]*cbor.LazyValue
	InvalidTransactions    []uint
}

func (b *AlonzoBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (AlonzoBlock) Type() int {
	return BlockTypeAlonzo
}

func (b *AlonzoBlock) Hash() string {
	return b.BlockHeader.Hash()
}

func (b *AlonzoBlock) Header() common.BlockHeader {
	return b.BlockHeader
}

func (b *AlonzoBlock) PrevHash() string {
	return b.BlockHeader.PrevHash()
}

func (b *AlonzoBlock) BlockNumber() uint64 {
	return b.BlockHeader.BlockNumber()
}

func (b *AlonzoBlock) SlotNumber() uint64 {
	return b.BlockHeader.SlotNumber()
}

func (b *AlonzoBlock) IssuerVkey() common.IssuerVkey {
	return b.BlockHeader.IssuerVkey()
}

func (b *AlonzoBlock) BlockBodySize() uint64 {
	return b.BlockHeader.BlockBodySize()
}

func (b *AlonzoBlock) Era() common.Era {
	return EraAlonzo
}

func (b *AlonzoBlock) Transactions() []common.Transaction {
	invalidTxMap := make(map[uint]bool, len(b.InvalidTransactions))
	for _, invalidTxIdx := range b.InvalidTransactions {
		invalidTxMap[invalidTxIdx] = true
	}

	ret := make([]common.Transaction, len(b.TransactionBodies))
	// #nosec G115
	for idx := range b.TransactionBodies {
		ret[idx] = &AlonzoTransaction{
			Body:       b.TransactionBodies[idx],
			WitnessSet: b.TransactionWitnessSets[idx],
			TxMetadata: b.TransactionMetadataSet[uint(idx)],
			IsTxValid:  !invalidTxMap[uint(idx)],
		}
	}
	return ret
}

func (b *AlonzoBlock) Utxorpc() *utxorpc.Block {
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

type AlonzoBlockHeader struct {
	shelley.ShelleyBlockHeader
}

func (h *AlonzoBlockHeader) Era() common.Era {
	return EraAlonzo
}

type AlonzoTransactionBody struct {
	mary.MaryTransactionBody
	TxOutputs []AlonzoTransactionOutput `cbor:"1,keyasint,omitempty"`
	Update    struct {
		cbor.StructAsArray
		ProtocolParamUpdates map[common.Blake2b224]AlonzoProtocolParameterUpdate
		Epoch                uint64
	} `cbor:"6,keyasint,omitempty"`
	TxScriptDataHash  *common.Blake2b256                `cbor:"11,keyasint,omitempty"`
	TxCollateral      []shelley.ShelleyTransactionInput `cbor:"13,keyasint,omitempty"`
	TxRequiredSigners []common.Blake2b224               `cbor:"14,keyasint,omitempty"`
	NetworkId         uint8                             `cbor:"15,keyasint,omitempty"`
}

func (b *AlonzoTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *AlonzoTransactionBody) Outputs() []common.TransactionOutput {
	ret := []common.TransactionOutput{}
	for _, output := range b.TxOutputs {
		ret = append(ret, &output)
	}
	return ret
}

func (b *AlonzoTransactionBody) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	updateMap := make(map[common.Blake2b224]common.ProtocolParameterUpdate)
	for k, v := range b.Update.ProtocolParamUpdates {
		updateMap[k] = v
	}
	return b.Update.Epoch, updateMap
}

func (b *AlonzoTransactionBody) Collateral() []common.TransactionInput {
	ret := []common.TransactionInput{}
	for _, collateral := range b.TxCollateral {
		ret = append(ret, collateral)
	}
	return ret
}

func (b *AlonzoTransactionBody) RequiredSigners() []common.Blake2b224 {
	return b.TxRequiredSigners[:]
}

func (b *AlonzoTransactionBody) ScriptDataHash() *common.Blake2b256 {
	return b.TxScriptDataHash
}

type AlonzoTransactionOutput struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	OutputAddress     common.Address
	OutputAmount      mary.MaryTransactionOutputValue
	TxOutputDatumHash *common.Blake2b256
	legacyOutput      bool
}

func (o *AlonzoTransactionOutput) UnmarshalCBOR(cborData []byte) error {
	// Save original CBOR
	o.SetCbor(cborData)
	// Try to parse as legacy mary.Mary output first
	var tmpOutput mary.MaryTransactionOutput
	if _, err := cbor.Decode(cborData, &tmpOutput); err == nil {
		// Copy from temp mary.Mary output to Alonzo format
		o.OutputAddress = tmpOutput.OutputAddress
		o.OutputAmount = tmpOutput.OutputAmount
		o.legacyOutput = true
	} else {
		return cbor.DecodeGeneric(cborData, o)
	}
	return nil
}

func (o *AlonzoTransactionOutput) MarshalCBOR() ([]byte, error) {
	if o.legacyOutput {
		tmpOutput := mary.MaryTransactionOutput{
			OutputAddress: o.OutputAddress,
			OutputAmount:  o.OutputAmount,
		}
		return cbor.Encode(&tmpOutput)
	}
	return cbor.EncodeGeneric(o)
}

func (o AlonzoTransactionOutput) MarshalJSON() ([]byte, error) {
	tmpObj := struct {
		Address   common.Address                                  `json:"address"`
		Amount    uint64                                          `json:"amount"`
		Assets    *common.MultiAsset[common.MultiAssetTypeOutput] `json:"assets,omitempty"`
		DatumHash string                                          `json:"datumHash,omitempty"`
	}{
		Address: o.OutputAddress,
		Amount:  o.OutputAmount.Amount,
		Assets:  o.OutputAmount.Assets,
	}
	if o.TxOutputDatumHash != nil {
		tmpObj.DatumHash = o.TxOutputDatumHash.String()
	}
	return json.Marshal(&tmpObj)
}

func (o AlonzoTransactionOutput) Address() common.Address {
	return o.OutputAddress
}

func (o AlonzoTransactionOutput) Amount() uint64 {
	return o.OutputAmount.Amount
}

func (o AlonzoTransactionOutput) Assets() *common.MultiAsset[common.MultiAssetTypeOutput] {
	return o.OutputAmount.Assets
}

func (o AlonzoTransactionOutput) DatumHash() *common.Blake2b256 {
	return o.TxOutputDatumHash
}

func (o AlonzoTransactionOutput) Datum() *cbor.LazyValue {
	return nil
}

func (o AlonzoTransactionOutput) Utxorpc() *utxorpc.TxOutput {
	var assets []*utxorpc.Multiasset
	if o.Assets() != nil {
		tmpAssets := o.Assets()
		for _, policyId := range tmpAssets.Policies() {
			ma := &utxorpc.Multiasset{
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

	return &utxorpc.TxOutput{
		Address: o.OutputAddress.Bytes(),
		Coin:    o.Amount(),
		Assets:  assets,
		Datum: &utxorpc.Datum{
			Hash: o.TxOutputDatumHash.Bytes(),
		},
	}
}

type AlonzoRedeemer struct {
	cbor.StructAsArray
	Tag     common.RedeemerTag
	Index   uint32
	Data    cbor.LazyValue
	ExUnits common.ExUnits
}

type AlonzoRedeemers []AlonzoRedeemer

func (r AlonzoRedeemers) Indexes(tag common.RedeemerTag) []uint {
	ret := []uint{}
	for _, redeemer := range r {
		if redeemer.Tag == tag {
			ret = append(ret, uint(redeemer.Index))
		}
	}
	return ret
}

func (r AlonzoRedeemers) Value(
	index uint,
	tag common.RedeemerTag,
) (cbor.LazyValue, common.ExUnits) {
	for _, redeemer := range r {
		if redeemer.Tag == tag && uint(redeemer.Index) == index {
			return redeemer.Data, redeemer.ExUnits
		}
	}
	return cbor.LazyValue{}, common.ExUnits{}
}

type AlonzoTransactionWitnessSet struct {
	shelley.ShelleyTransactionWitnessSet
	WsPlutusV1Scripts [][]byte        `cbor:"3,keyasint,omitempty"`
	WsPlutusData      []cbor.Value    `cbor:"4,keyasint,omitempty"`
	WsRedeemers       AlonzoRedeemers `cbor:"5,keyasint,omitempty"`
}

func (w *AlonzoTransactionWitnessSet) UnmarshalCBOR(cborData []byte) error {
	return w.UnmarshalCbor(cborData, w)
}

func (w AlonzoTransactionWitnessSet) PlutusV1Scripts() [][]byte {
	return w.WsPlutusV1Scripts
}

func (w AlonzoTransactionWitnessSet) PlutusData() []cbor.Value {
	return w.WsPlutusData
}

func (w AlonzoTransactionWitnessSet) Redeemers() common.TransactionWitnessRedeemers {
	return w.WsRedeemers
}

type AlonzoTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       AlonzoTransactionBody
	WitnessSet AlonzoTransactionWitnessSet
	IsTxValid  bool
	TxMetadata *cbor.LazyValue
}

func (AlonzoTransaction) Type() int {
	return TxTypeAlonzo
}

func (t AlonzoTransaction) Hash() string {
	return t.Body.Hash()
}

func (t AlonzoTransaction) Inputs() []common.TransactionInput {
	return t.Body.Inputs()
}

func (t AlonzoTransaction) Outputs() []common.TransactionOutput {
	return t.Body.Outputs()
}

func (t AlonzoTransaction) Fee() uint64 {
	return t.Body.Fee()
}

func (t AlonzoTransaction) TTL() uint64 {
	return t.Body.TTL()
}

func (t AlonzoTransaction) ValidityIntervalStart() uint64 {
	return t.Body.ValidityIntervalStart()
}

func (t AlonzoTransaction) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	return t.Body.ProtocolParameterUpdates()
}

func (t AlonzoTransaction) ReferenceInputs() []common.TransactionInput {
	return t.Body.ReferenceInputs()
}

func (t AlonzoTransaction) Collateral() []common.TransactionInput {
	return t.Body.Collateral()
}

func (t AlonzoTransaction) CollateralReturn() common.TransactionOutput {
	return t.Body.CollateralReturn()
}

func (t AlonzoTransaction) TotalCollateral() uint64 {
	return t.Body.TotalCollateral()
}

func (t AlonzoTransaction) Certificates() []common.Certificate {
	return t.Body.Certificates()
}

func (t AlonzoTransaction) Withdrawals() map[*common.Address]uint64 {
	return t.Body.Withdrawals()
}

func (t AlonzoTransaction) AuxDataHash() *common.Blake2b256 {
	return t.Body.AuxDataHash()
}

func (t AlonzoTransaction) RequiredSigners() []common.Blake2b224 {
	return t.Body.RequiredSigners()
}

func (t AlonzoTransaction) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return t.Body.AssetMint()
}

func (t AlonzoTransaction) ScriptDataHash() *common.Blake2b256 {
	return t.Body.ScriptDataHash()
}

func (t AlonzoTransaction) VotingProcedures() common.VotingProcedures {
	return t.Body.VotingProcedures()
}

func (t AlonzoTransaction) ProposalProcedures() []common.ProposalProcedure {
	return t.Body.ProposalProcedures()
}

func (t AlonzoTransaction) CurrentTreasuryValue() int64 {
	return t.Body.CurrentTreasuryValue()
}

func (t AlonzoTransaction) Donation() uint64 {
	return t.Body.Donation()
}

func (t AlonzoTransaction) Metadata() *cbor.LazyValue {
	return t.TxMetadata
}

func (t AlonzoTransaction) IsValid() bool {
	return t.IsTxValid
}

func (t AlonzoTransaction) Consumed() []common.TransactionInput {
	if t.IsValid() {
		return t.Inputs()
	} else {
		return t.Collateral()
	}
}

func (t AlonzoTransaction) Produced() []common.Utxo {
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
		// No collateral return in Alonzo
		return []common.Utxo{}
	}
}

func (t AlonzoTransaction) Witnesses() common.TransactionWitnessSet {
	return t.WitnessSet
}

func (t *AlonzoTransaction) Cbor() []byte {
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

func (t *AlonzoTransaction) Utxorpc() *utxorpc.Tx {
	return t.Body.Utxorpc()
}

func NewAlonzoBlockFromCbor(data []byte) (*AlonzoBlock, error) {
	var alonzoBlock AlonzoBlock
	if _, err := cbor.Decode(data, &alonzoBlock); err != nil {
		return nil, fmt.Errorf("Alonzo block decode error: %w", err)
	}
	return &alonzoBlock, nil
}

func NewAlonzoTransactionBodyFromCbor(
	data []byte,
) (*AlonzoTransactionBody, error) {
	var alonzoTx AlonzoTransactionBody
	if _, err := cbor.Decode(data, &alonzoTx); err != nil {
		return nil, fmt.Errorf("Alonzo transaction body decode error: %w", err)
	}
	return &alonzoTx, nil
}

func NewAlonzoTransactionFromCbor(data []byte) (*AlonzoTransaction, error) {
	var alonzoTx AlonzoTransaction
	if _, err := cbor.Decode(data, &alonzoTx); err != nil {
		return nil, fmt.Errorf("Alonzo transaction decode error: %w", err)
	}
	return &alonzoTx, nil
}

func NewAlonzoTransactionOutputFromCbor(
	data []byte,
) (*AlonzoTransactionOutput, error) {
	var alonzoTxOutput AlonzoTransactionOutput
	if _, err := cbor.Decode(data, &alonzoTxOutput); err != nil {
		return nil, fmt.Errorf(
			"Alonzo transaction output decode error: %w",
			err,
		)
	}
	return &alonzoTxOutput, nil
}
