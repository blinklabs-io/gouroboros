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
	"encoding/json"
	"fmt"
	"iter"
	"math/big"
	"slices"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/data"
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
	type tAlonzoBlock AlonzoBlock
	var tmp tAlonzoBlock
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*b = AlonzoBlock(tmp)
	b.SetCbor(cborData)
	return nil
}

func (AlonzoBlock) Type() int {
	return BlockTypeAlonzo
}

func (b *AlonzoBlock) Hash() common.Blake2b256 {
	return b.BlockHeader.Hash()
}

func (b *AlonzoBlock) Header() common.BlockHeader {
	return b.BlockHeader
}

func (b *AlonzoBlock) PrevHash() common.Blake2b256 {
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
			TxIsValid:  !invalidTxMap[uint(idx)],
		}
	}
	return ret
}

func (b *AlonzoBlock) Utxorpc() (*utxorpc.Block, error) {
	txs := []*utxorpc.Tx{}
	for _, t := range b.Transactions() {
		tx, err := t.Utxorpc()
		if err != nil {
			return nil, err
		}
		txs = append(txs, tx)
	}
	body := &utxorpc.BlockBody{
		Tx: txs,
	}
	header := &utxorpc.BlockHeader{
		Hash:   b.Hash().Bytes(),
		Height: b.BlockNumber(),
		Slot:   b.SlotNumber(),
	}
	block := &utxorpc.Block{
		Body:   body,
		Header: header,
	}
	return block, nil
}

type AlonzoBlockHeader struct {
	shelley.ShelleyBlockHeader
}

func (h *AlonzoBlockHeader) Era() common.Era {
	return EraAlonzo
}

type AlonzoTransactionPparamUpdate struct {
	cbor.StructAsArray
	ProtocolParamUpdates map[common.Blake2b224]AlonzoProtocolParameterUpdate
	Epoch                uint64
}

type AlonzoTransactionBody struct {
	common.TransactionBodyBase
	TxInputs                shelley.ShelleyTransactionInputSet            `cbor:"0,keyasint,omitempty"`
	TxOutputs               []AlonzoTransactionOutput                     `cbor:"1,keyasint,omitempty"`
	TxFee                   uint64                                        `cbor:"2,keyasint,omitempty"`
	Ttl                     uint64                                        `cbor:"3,keyasint,omitempty"`
	TxCertificates          []common.CertificateWrapper                   `cbor:"4,keyasint,omitempty"`
	TxWithdrawals           map[*common.Address]uint64                    `cbor:"5,keyasint,omitempty"`
	Update                  *AlonzoTransactionPparamUpdate                `cbor:"6,keyasint,omitempty"`
	TxAuxDataHash           *common.Blake2b256                            `cbor:"7,keyasint,omitempty"`
	TxValidityIntervalStart uint64                                        `cbor:"8,keyasint,omitempty"`
	TxMint                  *common.MultiAsset[common.MultiAssetTypeMint] `cbor:"9,keyasint,omitempty"`
	TxScriptDataHash        *common.Blake2b256                            `cbor:"11,keyasint,omitempty"`
	TxCollateral            cbor.SetType[shelley.ShelleyTransactionInput] `cbor:"13,keyasint,omitempty,omitzero"`
	TxRequiredSigners       cbor.SetType[common.Blake2b224]               `cbor:"14,keyasint,omitempty,omitzero"`
	NetworkId               uint8                                         `cbor:"15,keyasint,omitempty"`
}

func (b *AlonzoTransactionBody) UnmarshalCBOR(cborData []byte) error {
	type tAlonzoTransactionBody AlonzoTransactionBody
	var tmp tAlonzoTransactionBody
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*b = AlonzoTransactionBody(tmp)
	b.SetCbor(cborData)
	return nil
}

func (b *AlonzoTransactionBody) Inputs() []common.TransactionInput {
	ret := []common.TransactionInput{}
	for _, input := range b.TxInputs.Items() {
		ret = append(ret, input)
	}
	return ret
}

func (b *AlonzoTransactionBody) Outputs() []common.TransactionOutput {
	ret := []common.TransactionOutput{}
	for _, output := range b.TxOutputs {
		ret = append(ret, &output)
	}
	return ret
}

func (b *AlonzoTransactionBody) Fee() uint64 {
	return b.TxFee
}

func (b *AlonzoTransactionBody) TTL() uint64 {
	return b.Ttl
}

func (b *AlonzoTransactionBody) ValidityIntervalStart() uint64 {
	return b.TxValidityIntervalStart
}

func (b *AlonzoTransactionBody) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	if b.Update == nil {
		return 0, nil
	}
	updateMap := make(map[common.Blake2b224]common.ProtocolParameterUpdate)
	for k, v := range b.Update.ProtocolParamUpdates {
		updateMap[k] = v
	}
	return b.Update.Epoch, updateMap
}

func (b *AlonzoTransactionBody) Certificates() []common.Certificate {
	ret := make([]common.Certificate, len(b.TxCertificates))
	for i, cert := range b.TxCertificates {
		ret[i] = cert.Certificate
	}
	return ret
}

func (b *AlonzoTransactionBody) Withdrawals() map[*common.Address]uint64 {
	return b.TxWithdrawals
}

func (b *AlonzoTransactionBody) AuxDataHash() *common.Blake2b256 {
	return b.TxAuxDataHash
}

func (b *AlonzoTransactionBody) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return b.TxMint
}

func (b *AlonzoTransactionBody) Collateral() []common.TransactionInput {
	ret := []common.TransactionInput{}
	for _, collateral := range b.TxCollateral.Items() {
		ret = append(ret, collateral)
	}
	return ret
}

func (b *AlonzoTransactionBody) RequiredSigners() []common.Blake2b224 {
	return b.TxRequiredSigners.Items()
}

func (b *AlonzoTransactionBody) ScriptDataHash() *common.Blake2b256 {
	return b.TxScriptDataHash
}

func (b *AlonzoTransactionBody) Utxorpc() (*utxorpc.Tx, error) {
	return common.TransactionBodyToUtxorpc(b), nil
}

type AlonzoTransactionOutput struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	OutputAddress   common.Address
	OutputAmount    mary.MaryTransactionOutputValue
	OutputDatumHash *common.Blake2b256
	legacyOutput    bool
}

func (o *AlonzoTransactionOutput) UnmarshalCBOR(cborData []byte) error {
	// Try to parse as legacy mary.Mary output first
	var tmpOutput mary.MaryTransactionOutput
	if _, err := cbor.Decode(cborData, &tmpOutput); err == nil {
		// Copy from temp mary.Mary output to Alonzo format
		o.OutputAddress = tmpOutput.OutputAddress
		o.OutputAmount = tmpOutput.OutputAmount
		o.legacyOutput = true
	} else {
		type tAlonzoTransactionOutput AlonzoTransactionOutput
		var tmp tAlonzoTransactionOutput
		if _, err := cbor.Decode(cborData, &tmp); err != nil {
			return err
		}
		*o = AlonzoTransactionOutput(tmp)
	}
	// Save original CBOR
	o.SetCbor(cborData)
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
	tmpOutput := []any{
		o.OutputAddress,
		o.OutputAmount,
	}
	if o.OutputDatumHash != nil {
		tmpOutput = append(tmpOutput, o.OutputDatumHash)
	}
	return cbor.Encode(tmpOutput)
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
	if o.OutputDatumHash != nil {
		tmpObj.DatumHash = o.OutputDatumHash.String()
	}
	return json.Marshal(&tmpObj)
}

func (o AlonzoTransactionOutput) ToPlutusData() data.PlutusData {
	var valueData [][2]data.PlutusData
	if o.OutputAmount.Amount > 0 {
		valueData = append(
			valueData,
			[2]data.PlutusData{
				data.NewByteString(nil),
				data.NewMap(
					[][2]data.PlutusData{
						{
							data.NewByteString(nil),
							data.NewInteger(
								new(big.Int).SetUint64(o.OutputAmount.Amount),
							),
						},
					},
				),
			},
		)
	}
	if o.OutputAmount.Assets != nil {
		assetData := o.OutputAmount.Assets.ToPlutusData()
		assetDataMap, ok := assetData.(*data.Map)
		if !ok {
			return nil
		}
		valueData = append(
			valueData,
			assetDataMap.Pairs...,
		)
	}
	tmpData := data.NewConstr(
		0,
		o.OutputAddress.ToPlutusData(),
		data.NewMap(valueData),
		// Empty datum option
		// TODO: implement this
		data.NewConstr(0),
		// Empty script ref
		// TODO: implement this
		data.NewConstr(1),
	)
	return tmpData
}

func (o AlonzoTransactionOutput) Address() common.Address {
	return o.OutputAddress
}

func (o AlonzoTransactionOutput) ScriptRef() common.Script {
	return nil
}

func (o AlonzoTransactionOutput) Amount() uint64 {
	return o.OutputAmount.Amount
}

func (o AlonzoTransactionOutput) Assets() *common.MultiAsset[common.MultiAssetTypeOutput] {
	return o.OutputAmount.Assets
}

func (o AlonzoTransactionOutput) DatumHash() *common.Blake2b256 {
	return o.OutputDatumHash
}

func (o AlonzoTransactionOutput) Datum() *common.Datum {
	return nil
}

func (o AlonzoTransactionOutput) Utxorpc() (*utxorpc.TxOutput, error) {
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

	addressBytes, err := o.OutputAddress.Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to get address bytes: %w", err)
	}

	// Datum hash handling
	var datumHash []byte
	if o.OutputDatumHash != nil {
		datumHash = o.OutputDatumHash.Bytes()
	}

	return &utxorpc.TxOutput{
			Address: addressBytes,
			Coin:    o.Amount(),
			Assets:  assets,
			Datum: &utxorpc.Datum{
				Hash: datumHash,
			},
		},
		nil
}

type AlonzoRedeemer struct {
	cbor.StructAsArray
	Tag     common.RedeemerTag
	Index   uint32
	Data    common.Datum
	ExUnits common.ExUnits
}

type AlonzoRedeemers []AlonzoRedeemer

func (r AlonzoRedeemers) Iter() iter.Seq2[common.RedeemerKey, common.RedeemerValue] {
	return func(yield func(common.RedeemerKey, common.RedeemerValue) bool) {
		// Sort redeemers
		sorted := make([]AlonzoRedeemer, len(r))
		copy(sorted, r)
		slices.SortFunc(
			sorted,
			func(a, b AlonzoRedeemer) int {
				if a.Tag < b.Tag || (a.Tag == b.Tag && a.Index < b.Index) {
					return -1
				}
				if a.Tag > b.Tag || (a.Tag == b.Tag && a.Index > b.Index) {
					return 1
				}
				return 0
			},
		)
		// Yield keys
		for _, redeemer := range sorted {
			tmpKey := common.RedeemerKey{
				Tag:   redeemer.Tag,
				Index: redeemer.Index,
			}
			tmpVal := common.RedeemerValue{
				Data:    redeemer.Data,
				ExUnits: redeemer.ExUnits,
			}
			if !yield(tmpKey, tmpVal) {
				return
			}
		}
	}
}

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
) common.RedeemerValue {
	for _, redeemer := range r {
		if redeemer.Tag == tag && uint(redeemer.Index) == index {
			return common.RedeemerValue{
				Data:    redeemer.Data,
				ExUnits: redeemer.ExUnits,
			}
		}
	}
	return common.RedeemerValue{}
}

type AlonzoTransactionWitnessSet struct {
	cbor.DecodeStoreCbor
	VkeyWitnesses      []common.VkeyWitness      `cbor:"0,keyasint,omitempty"`
	WsNativeScripts    []common.NativeScript     `cbor:"1,keyasint,omitempty"`
	BootstrapWitnesses []common.BootstrapWitness `cbor:"2,keyasint,omitempty"`
	WsPlutusV1Scripts  [][]byte                  `cbor:"3,keyasint,omitempty"`
	WsPlutusData       []common.Datum            `cbor:"4,keyasint,omitempty"`
	WsRedeemers        AlonzoRedeemers           `cbor:"5,keyasint,omitempty"`
}

func (w *AlonzoTransactionWitnessSet) UnmarshalCBOR(cborData []byte) error {
	type tAlonzoTransactionWitnessSet AlonzoTransactionWitnessSet
	var tmp tAlonzoTransactionWitnessSet
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*w = AlonzoTransactionWitnessSet(tmp)
	w.SetCbor(cborData)
	return nil
}

func (w AlonzoTransactionWitnessSet) Vkey() []common.VkeyWitness {
	return w.VkeyWitnesses
}

func (w AlonzoTransactionWitnessSet) Bootstrap() []common.BootstrapWitness {
	return w.BootstrapWitnesses
}

func (w AlonzoTransactionWitnessSet) NativeScripts() []common.NativeScript {
	return w.WsNativeScripts
}

func (w AlonzoTransactionWitnessSet) PlutusV1Scripts() [][]byte {
	return w.WsPlutusV1Scripts
}

func (w AlonzoTransactionWitnessSet) PlutusV2Scripts() [][]byte {
	// No plutus v2 scripts in Alonzo
	return nil
}

func (w AlonzoTransactionWitnessSet) PlutusV3Scripts() [][]byte {
	// No plutus v3 scripts in Alonzo
	return nil
}

func (w AlonzoTransactionWitnessSet) PlutusData() []common.Datum {
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
	TxIsValid  bool
	TxMetadata *cbor.LazyValue
}

func (t *AlonzoTransaction) UnmarshalCBOR(cborData []byte) error {
	type tAlonzoTransaction AlonzoTransaction
	var tmp tAlonzoTransaction
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*t = AlonzoTransaction(tmp)
	t.SetCbor(cborData)
	return nil
}

func (AlonzoTransaction) Type() int {
	return TxTypeAlonzo
}

func (t AlonzoTransaction) Hash() common.Blake2b256 {
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
	return t.TxIsValid
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
					Id: shelley.NewShelleyTransactionInput(
						t.Hash().String(),
						idx,
					),
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
	if len(cborData) > 0 {
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
		t.TxIsValid,
	}
	if t.TxMetadata != nil {
		tmpObj = append(tmpObj, cbor.RawMessage(t.TxMetadata.Cbor()))
	} else {
		tmpObj = append(tmpObj, nil)
	}
	// This should never fail, since we're only encoding a list and a bool value
	cborData, err := cbor.Encode(&tmpObj)
	if err != nil {
		panic("CBOR encoding that should never fail has failed: " + err.Error())
	}
	return cborData
}

func (t *AlonzoTransaction) Utxorpc() (*utxorpc.Tx, error) {
	tx, err := t.Body.Utxorpc()
	if err != nil {
		return nil, fmt.Errorf("failed to convert Alonzo transaction: %w", err)
	}
	return tx, nil
}

func NewAlonzoBlockFromCbor(data []byte) (*AlonzoBlock, error) {
	var alonzoBlock AlonzoBlock
	if _, err := cbor.Decode(data, &alonzoBlock); err != nil {
		return nil, fmt.Errorf("decode Alonzo block error: %w", err)
	}
	return &alonzoBlock, nil
}

func NewAlonzoBlockHeaderFromCbor(data []byte) (*AlonzoBlockHeader, error) {
	var alonzoBlockHeader AlonzoBlockHeader
	if _, err := cbor.Decode(data, &alonzoBlockHeader); err != nil {
		return nil, fmt.Errorf("decode Alonzo block header error: %w", err)
	}
	return &alonzoBlockHeader, nil
}

func NewAlonzoTransactionBodyFromCbor(
	data []byte,
) (*AlonzoTransactionBody, error) {
	var alonzoTx AlonzoTransactionBody
	if _, err := cbor.Decode(data, &alonzoTx); err != nil {
		return nil, fmt.Errorf("decode Alonzo transaction body error: %w", err)
	}
	return &alonzoTx, nil
}

func NewAlonzoTransactionFromCbor(data []byte) (*AlonzoTransaction, error) {
	var alonzoTx AlonzoTransaction
	if _, err := cbor.Decode(data, &alonzoTx); err != nil {
		return nil, fmt.Errorf("decode Alonzo transaction error: %w", err)
	}
	return &alonzoTx, nil
}

func NewAlonzoTransactionOutputFromCbor(
	data []byte,
) (*AlonzoTransactionOutput, error) {
	var alonzoTxOutput AlonzoTransactionOutput
	if _, err := cbor.Decode(data, &alonzoTxOutput); err != nil {
		return nil, fmt.Errorf(
			"decode Alonzo transaction output error: %w",
			err,
		)
	}
	return &alonzoTxOutput, nil
}
