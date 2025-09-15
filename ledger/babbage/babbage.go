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
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/data"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

const (
	EraIdBabbage   = 5
	EraNameBabbage = "Babbage"

	BlockTypeBabbage = 6

	BlockHeaderTypeBabbage = 5

	TxTypeBabbage = 5
)

var EraBabbage = common.Era{
	Id:   EraIdBabbage,
	Name: EraNameBabbage,
}

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
	type tBabbageBlock BabbageBlock
	var tmp tBabbageBlock
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*b = BabbageBlock(tmp)
	b.SetCbor(cborData)
	return nil
}

func (BabbageBlock) Type() int {
	return BlockTypeBabbage
}

func (b *BabbageBlock) Hash() common.Blake2b256 {
	return b.BlockHeader.Hash()
}

func (b *BabbageBlock) Header() common.BlockHeader {
	return b.BlockHeader
}

func (b *BabbageBlock) PrevHash() common.Blake2b256 {
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
			TxIsValid:  !invalidTxMap[uint(idx)],
		}
	}
	return ret
}

func (b *BabbageBlock) Utxorpc() (*utxorpc.Block, error) {
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

type BabbageBlockHeader struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash      *common.Blake2b256
	Body      BabbageBlockHeaderBody
	Signature []byte
}

type BabbageBlockHeaderBody struct {
	cbor.StructAsArray
	BlockNumber   uint64
	Slot          uint64
	PrevHash      common.Blake2b256
	IssuerVkey    common.IssuerVkey
	VrfKey        []byte
	VrfResult     common.VrfResult
	BlockBodySize uint64
	BlockBodyHash common.Blake2b256
	OpCert        BabbageOpCert
	ProtoVersion  BabbageProtoVersion
}

type BabbageOpCert struct {
	cbor.StructAsArray
	HotVkey        []byte
	SequenceNumber uint32
	KesPeriod      uint32
	Signature      []byte
}

type BabbageProtoVersion struct {
	cbor.StructAsArray
	Major uint64
	Minor uint64
}

func (h *BabbageBlockHeader) UnmarshalCBOR(cborData []byte) error {
	type tBabbageBlockHeader BabbageBlockHeader
	var tmp tBabbageBlockHeader
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*h = BabbageBlockHeader(tmp)
	h.SetCbor(cborData)
	return nil
}

func (h *BabbageBlockHeader) Hash() common.Blake2b256 {
	if h.hash == nil {
		tmpHash := common.Blake2b256Hash(h.Cbor())
		h.hash = &tmpHash
	}
	return *h.hash
}

func (h *BabbageBlockHeader) PrevHash() common.Blake2b256 {
	return h.Body.PrevHash
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

type BabbageTransactionPparamUpdate struct {
	cbor.StructAsArray
	ProtocolParamUpdates map[common.Blake2b224]BabbageProtocolParameterUpdate
	Epoch                uint64
}

type BabbageTransactionBody struct {
	common.TransactionBodyBase
	TxInputs                shelley.ShelleyTransactionInputSet            `cbor:"0,keyasint,omitempty"`
	TxOutputs               []BabbageTransactionOutput                    `cbor:"1,keyasint,omitempty"`
	TxFee                   uint64                                        `cbor:"2,keyasint,omitempty"`
	Ttl                     uint64                                        `cbor:"3,keyasint,omitempty"`
	TxCertificates          []common.CertificateWrapper                   `cbor:"4,keyasint,omitempty"`
	TxWithdrawals           map[*common.Address]uint64                    `cbor:"5,keyasint,omitempty"`
	Update                  *BabbageTransactionPparamUpdate               `cbor:"6,keyasint,omitempty"`
	TxAuxDataHash           *common.Blake2b256                            `cbor:"7,keyasint,omitempty"`
	TxValidityIntervalStart uint64                                        `cbor:"8,keyasint,omitempty"`
	TxMint                  *common.MultiAsset[common.MultiAssetTypeMint] `cbor:"9,keyasint,omitempty"`
	TxScriptDataHash        *common.Blake2b256                            `cbor:"11,keyasint,omitempty"`
	TxCollateral            cbor.SetType[shelley.ShelleyTransactionInput] `cbor:"13,keyasint,omitempty,omitzero"`
	TxRequiredSigners       cbor.SetType[common.Blake2b224]               `cbor:"14,keyasint,omitempty,omitzero"`
	NetworkId               uint8                                         `cbor:"15,keyasint,omitempty"`
	TxCollateralReturn      *BabbageTransactionOutput                     `cbor:"16,keyasint,omitempty"`
	TxTotalCollateral       uint64                                        `cbor:"17,keyasint,omitempty"`
	TxReferenceInputs       cbor.SetType[shelley.ShelleyTransactionInput] `cbor:"18,keyasint,omitempty,omitzero"`
}

func (b *BabbageTransactionBody) UnmarshalCBOR(cborData []byte) error {
	type tBabbageTransactionBody BabbageTransactionBody
	var tmp tBabbageTransactionBody
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*b = BabbageTransactionBody(tmp)
	b.SetCbor(cborData)
	return nil
}

func (b *BabbageTransactionBody) Inputs() []common.TransactionInput {
	ret := []common.TransactionInput{}
	for _, input := range b.TxInputs.Items() {
		ret = append(ret, input)
	}
	return ret
}

func (b *BabbageTransactionBody) Outputs() []common.TransactionOutput {
	ret := []common.TransactionOutput{}
	for _, output := range b.TxOutputs {
		ret = append(ret, &output)
	}
	return ret
}

func (b *BabbageTransactionBody) Fee() uint64 {
	return b.TxFee
}

func (b *BabbageTransactionBody) TTL() uint64 {
	return b.Ttl
}

func (b *BabbageTransactionBody) ValidityIntervalStart() uint64 {
	return b.TxValidityIntervalStart
}

func (b *BabbageTransactionBody) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	if b.Update == nil {
		return 0, nil
	}
	updateMap := make(map[common.Blake2b224]common.ProtocolParameterUpdate)
	for k, v := range b.Update.ProtocolParamUpdates {
		updateMap[k] = v
	}
	return b.Update.Epoch, updateMap
}

func (b *BabbageTransactionBody) Certificates() []common.Certificate {
	ret := make([]common.Certificate, len(b.TxCertificates))
	for i, cert := range b.TxCertificates {
		ret[i] = cert.Certificate
	}
	return ret
}

func (b *BabbageTransactionBody) Withdrawals() map[*common.Address]uint64 {
	return b.TxWithdrawals
}

func (b *BabbageTransactionBody) AuxDataHash() *common.Blake2b256 {
	return b.TxAuxDataHash
}

func (b *BabbageTransactionBody) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return b.TxMint
}

func (b *BabbageTransactionBody) Collateral() []common.TransactionInput {
	ret := []common.TransactionInput{}
	for _, collateral := range b.TxCollateral.Items() {
		ret = append(ret, collateral)
	}
	return ret
}

func (b *BabbageTransactionBody) RequiredSigners() []common.Blake2b224 {
	return b.TxRequiredSigners.Items()
}

func (b *BabbageTransactionBody) ScriptDataHash() *common.Blake2b256 {
	return b.TxScriptDataHash
}

func (b *BabbageTransactionBody) ReferenceInputs() []common.TransactionInput {
	ret := []common.TransactionInput{}
	for _, input := range b.TxReferenceInputs.Items() {
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

func (b *BabbageTransactionBody) Utxorpc() (*utxorpc.Tx, error) {
	return common.TransactionBodyToUtxorpc(b)
}

const (
	DatumOptionTypeHash = 0
	DatumOptionTypeData = 1
)

type BabbageTransactionOutputDatumOption struct {
	hash *common.Blake2b256
	data *common.Datum
}

func (d *BabbageTransactionOutputDatumOption) UnmarshalCBOR(
	cborData []byte,
) error {
	datumOptionType, err := cbor.DecodeIdFromList(cborData)
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
		if _, err := cbor.Decode(cborData, &tmpDatumHash); err != nil {
			return err
		}
		d.hash = &(tmpDatumHash.Hash)
	case DatumOptionTypeData:
		var tmpDatumData struct {
			cbor.StructAsArray
			Type     int
			DataCbor []byte
		}
		if _, err := cbor.Decode(cborData, &tmpDatumData); err != nil {
			return err
		}
		var datumValue common.Datum
		if _, err := cbor.Decode(tmpDatumData.DataCbor, &datumValue); err != nil {
			return err
		}
		d.data = &datumValue
	default:
		return fmt.Errorf("unsupported datum option type: %d", datumOptionType)
	}
	return nil
}

func (d *BabbageTransactionOutputDatumOption) MarshalCBOR() ([]byte, error) {
	var tmpObj []any
	if d.hash != nil {
		tmpObj = []any{DatumOptionTypeHash, d.hash}
	} else if d.data != nil {
		tmpContent, err := cbor.Encode(d.data)
		if err != nil {
			return nil, err
		}
		tmpObj = []any{DatumOptionTypeData, cbor.Tag{Number: 24, Content: tmpContent}}
	} else {
		return nil, errors.New("unknown datum option type")
	}
	return cbor.Encode(&tmpObj)
}

type BabbageTransactionOutput struct {
	cbor.DecodeStoreCbor
	OutputAddress  common.Address                       `cbor:"0,keyasint,omitempty"`
	OutputAmount   mary.MaryTransactionOutputValue      `cbor:"1,keyasint,omitempty"`
	DatumOption    *BabbageTransactionOutputDatumOption `cbor:"2,keyasint,omitempty"`
	TxOutScriptRef *common.ScriptRef                    `cbor:"3,keyasint,omitempty"`
	legacyOutput   bool
}

func (o *BabbageTransactionOutput) UnmarshalCBOR(cborData []byte) error {
	// Try to parse as legacy output first
	var tmpOutput alonzo.AlonzoTransactionOutput
	if _, err := cbor.Decode(cborData, &tmpOutput); err == nil {
		// Copy from temp legacy object to Babbage format
		o.OutputAddress = tmpOutput.OutputAddress
		o.OutputAmount = tmpOutput.OutputAmount
		o.legacyOutput = true
	} else {
		type tBabbageTransactionOutput BabbageTransactionOutput
		var tmp tBabbageTransactionOutput
		if _, err := cbor.Decode(cborData, &tmp); err != nil {
			return err
		}
		*o = BabbageTransactionOutput(tmp)
	}
	// Save original CBOR
	o.SetCbor(cborData)
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
		Datum     *common.Datum                                   `json:"datum,omitempty"`
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

func (o BabbageTransactionOutput) ToPlutusData() data.PlutusData {
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
	var datumOptionPd data.PlutusData
	switch {
	case o.DatumOption == nil:
		datumOptionPd = data.NewConstr(0)
	case o.DatumOption.hash != nil:
		datumOptionPd = data.NewConstr(
			1,
			data.NewByteString(o.DatumOption.hash.Bytes()),
		)
	case o.DatumOption.data != nil:
		datumOptionPd = data.NewConstr(
			2,
			o.DatumOption.data.Data,
		)
	}
	var scriptRefPd data.PlutusData
	if o.TxOutScriptRef == nil {
		scriptRefPd = data.NewConstr(1)
	} else {
		scriptRefPd = data.NewConstr(
			0,
			data.NewByteString(
				o.TxOutScriptRef.Script.Hash().Bytes(),
			),
		)
	}
	tmpData := data.NewConstr(
		0,
		o.OutputAddress.ToPlutusData(),
		data.NewMap(valueData),
		datumOptionPd,
		scriptRefPd,
	)
	return tmpData
}

func (o BabbageTransactionOutput) Address() common.Address {
	return o.OutputAddress
}

func (o BabbageTransactionOutput) ScriptRef() common.Script {
	if o.TxOutScriptRef == nil {
		return nil
	}
	return o.TxOutScriptRef.Script
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

func (o BabbageTransactionOutput) Datum() *common.Datum {
	if o.DatumOption != nil {
		return o.DatumOption.data
	}
	return nil
}

func (o BabbageTransactionOutput) Utxorpc() (*utxorpc.TxOutput, error) {
	// Handle address bytes
	addressBytes, err := o.OutputAddress.Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to get output address bytes: %w", err)
	}
	var address []byte
	if addressBytes == nil {
		address = []byte{}
	} else {
		address = addressBytes
	}

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
		},
		nil
}

func (o BabbageTransactionOutput) String() string {
	assets := ""
	if o.OutputAmount.Assets != nil {
		if as := o.OutputAmount.Assets.String(); as != "[]" {
			assets = " assets=" + as
		}
	}
	return fmt.Sprintf(
		"(BabbageTransactionOutput address=%s amount=%d%s)",
		o.OutputAddress.String(),
		o.OutputAmount.Amount,
		assets,
	)
}

type BabbageTransactionWitnessSet struct {
	cbor.DecodeStoreCbor
	VkeyWitnesses      []common.VkeyWitness      `cbor:"0,keyasint,omitempty"`
	WsNativeScripts    []common.NativeScript     `cbor:"1,keyasint,omitempty"`
	BootstrapWitnesses []common.BootstrapWitness `cbor:"2,keyasint,omitempty"`
	WsPlutusV1Scripts  []common.PlutusV1Script   `cbor:"3,keyasint,omitempty"`
	WsPlutusData       []common.Datum            `cbor:"4,keyasint,omitempty"`
	WsRedeemers        alonzo.AlonzoRedeemers    `cbor:"5,keyasint,omitempty"`
	WsPlutusV2Scripts  []common.PlutusV2Script   `cbor:"6,keyasint,omitempty"`
}

func (w *BabbageTransactionWitnessSet) UnmarshalCBOR(cborData []byte) error {
	type tBabbageTransactionWitnessSet BabbageTransactionWitnessSet
	var tmp tBabbageTransactionWitnessSet
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*w = BabbageTransactionWitnessSet(tmp)
	w.SetCbor(cborData)
	return nil
}

func (w BabbageTransactionWitnessSet) Vkey() []common.VkeyWitness {
	return w.VkeyWitnesses
}

func (w BabbageTransactionWitnessSet) Bootstrap() []common.BootstrapWitness {
	return w.BootstrapWitnesses
}

func (w BabbageTransactionWitnessSet) NativeScripts() []common.NativeScript {
	return w.WsNativeScripts
}

func (w BabbageTransactionWitnessSet) PlutusV1Scripts() []common.PlutusV1Script {
	return w.WsPlutusV1Scripts
}

func (w BabbageTransactionWitnessSet) PlutusV2Scripts() []common.PlutusV2Script {
	return w.WsPlutusV2Scripts
}

func (w BabbageTransactionWitnessSet) PlutusV3Scripts() []common.PlutusV3Script {
	// No plutus v3 scripts in Babbage
	return nil
}

func (w BabbageTransactionWitnessSet) PlutusData() []common.Datum {
	return w.WsPlutusData
}

func (w BabbageTransactionWitnessSet) Redeemers() common.TransactionWitnessRedeemers {
	return w.WsRedeemers
}

type BabbageTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       BabbageTransactionBody
	WitnessSet BabbageTransactionWitnessSet
	TxIsValid  bool
	TxMetadata *cbor.LazyValue
}

func (t *BabbageTransaction) UnmarshalCBOR(cborData []byte) error {
	type tBabbageTransaction BabbageTransaction
	var tmp tBabbageTransaction
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*t = BabbageTransaction(tmp)
	t.SetCbor(cborData)
	return nil
}

func (BabbageTransaction) Type() int {
	return TxTypeBabbage
}

func (t BabbageTransaction) Hash() common.Blake2b256 {
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
	return t.TxIsValid
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
		if t.CollateralReturn() == nil {
			return []common.Utxo{}
		}
		return []common.Utxo{
			{
				Id:     shelley.NewShelleyTransactionInput(t.Hash().String(), len(t.Outputs())),
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

func (t *BabbageTransaction) Utxorpc() (*utxorpc.Tx, error) {
	tx, err := t.Body.Utxorpc()
	if err != nil {
		return nil, fmt.Errorf("failed to convert Babbage transaction: %w", err)
	}
	return tx, nil
}

func NewBabbageBlockFromCbor(data []byte) (*BabbageBlock, error) {
	var babbageBlock BabbageBlock
	if _, err := cbor.Decode(data, &babbageBlock); err != nil {
		return nil, fmt.Errorf("decode Babbage block error: %w", err)
	}
	return &babbageBlock, nil
}

func NewBabbageBlockHeaderFromCbor(data []byte) (*BabbageBlockHeader, error) {
	var babbageBlockHeader BabbageBlockHeader
	if _, err := cbor.Decode(data, &babbageBlockHeader); err != nil {
		return nil, fmt.Errorf("decode Babbage block header error: %w", err)
	}
	return &babbageBlockHeader, nil
}

func NewBabbageTransactionBodyFromCbor(
	data []byte,
) (*BabbageTransactionBody, error) {
	var babbageTx BabbageTransactionBody
	if _, err := cbor.Decode(data, &babbageTx); err != nil {
		return nil, fmt.Errorf("decode Babbage transaction body error: %w", err)
	}
	return &babbageTx, nil
}

func NewBabbageTransactionFromCbor(data []byte) (*BabbageTransaction, error) {
	var babbageTx BabbageTransaction
	if _, err := cbor.Decode(data, &babbageTx); err != nil {
		return nil, fmt.Errorf("decode Babbage transaction error: %w", err)
	}
	return &babbageTx, nil
}

func NewBabbageTransactionOutputFromCbor(
	data []byte,
) (*BabbageTransactionOutput, error) {
	var babbageTxOutput BabbageTransactionOutput
	if _, err := cbor.Decode(data, &babbageTxOutput); err != nil {
		return nil, fmt.Errorf(
			"decode Babbage transaction output error: %w",
			err,
		)
	}
	return &babbageTxOutput, nil
}
