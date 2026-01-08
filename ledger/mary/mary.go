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
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/data"
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
	TransactionMetadataSet common.TransactionMetadataSet
}

func (b *MaryBlock) UnmarshalCBOR(cborData []byte) error {
	type tMaryBlock MaryBlock
	var tmp tMaryBlock
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*b = MaryBlock(tmp)
	b.SetCbor(cborData)

	// Extract and store CBOR for each component
	if err := common.ExtractAndSetTransactionCbor(
		cborData,
		func(i int, data []byte) { b.TransactionBodies[i].SetCbor(data) },
		func(i int, data []byte) { b.TransactionWitnessSets[i].SetCbor(data) },
		len(b.TransactionBodies),
		len(b.TransactionWitnessSets),
	); err != nil {
		return err
	}
	return nil
}

func (b *MaryBlock) MarshalCBOR() ([]byte, error) {
	// Return the stored CBOR if available
	// Note: this is a workaround for an issue described here:
	// https://github.com/blinklabs-io/gouroboros/pull/1093#discussion_r2491161964
	if b.Cbor() != nil {
		return b.Cbor(), nil
	}
	// Otherwise, encode generically
	return cbor.EncodeGeneric(b)
}

func (MaryBlock) Type() int {
	return BlockTypeMary
}

func (b *MaryBlock) Hash() common.Blake2b256 {
	return b.BlockHeader.Hash()
}

func (b *MaryBlock) Header() common.BlockHeader {
	return b.BlockHeader
}

func (b *MaryBlock) PrevHash() common.Blake2b256 {
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
		tx := &MaryTransaction{
			Body:       b.TransactionBodies[idx],
			WitnessSet: b.TransactionWitnessSets[idx],
		}
		// Populate metadata and preserve original auxiliary CBOR when present
		if metadata, ok := b.TransactionMetadataSet.GetMetadata(uint(idx)); ok {
			tx.TxMetadata = metadata
		}
		if raw, ok := b.TransactionMetadataSet.GetRawMetadata(uint(idx)); ok &&
			len(raw) > 0 {
			if aux, err := common.DecodeAuxiliaryData(raw); err == nil &&
				aux != nil {
				tx.auxData = aux
			}
		}
		ret[idx] = tx
	}
	return ret
}

func (b *MaryBlock) Utxorpc() (*utxorpc.Block, error) {
	tmpTxs := b.Transactions()
	txs := make([]*utxorpc.Tx, 0, len(tmpTxs))
	for _, t := range tmpTxs {
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

func (b *MaryBlock) BlockBodyHash() common.Blake2b256 {
	return b.Header().BlockBodyHash()
}

type MaryBlockHeader struct {
	shelley.ShelleyBlockHeader
}

func (h *MaryBlockHeader) Era() common.Era {
	return EraMary
}

type MaryTransactionPparamUpdate struct {
	cbor.StructAsArray
	ProtocolParamUpdates map[common.Blake2b224]MaryProtocolParameterUpdate
	Epoch                uint64
}

type MaryTransactionBody struct {
	common.TransactionBodyBase
	TxInputs                shelley.ShelleyTransactionInputSet            `cbor:"0,keyasint,omitempty"`
	TxOutputs               []MaryTransactionOutput                       `cbor:"1,keyasint,omitempty"`
	TxFee                   uint64                                        `cbor:"2,keyasint,omitempty"`
	Ttl                     uint64                                        `cbor:"3,keyasint,omitempty"`
	TxCertificates          []common.CertificateWrapper                   `cbor:"4,keyasint,omitempty"`
	TxWithdrawals           map[*common.Address]uint64                    `cbor:"5,keyasint,omitempty"`
	Update                  *MaryTransactionPparamUpdate                  `cbor:"6,keyasint,omitempty"`
	TxAuxDataHash           *common.Blake2b256                            `cbor:"7,keyasint,omitempty"`
	TxValidityIntervalStart *uint64                                       `cbor:"8,keyasint,omitempty"`
	TxMint                  *common.MultiAsset[common.MultiAssetTypeMint] `cbor:"9,keyasint,omitempty"`
}

func (b *MaryTransactionBody) UnmarshalCBOR(cborData []byte) error {
	type tMaryTransactionBody MaryTransactionBody
	var tmp tMaryTransactionBody
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*b = MaryTransactionBody(tmp)
	b.SetCbor(cborData)
	return nil
}

func (b *MaryTransactionBody) MarshalCBOR() ([]byte, error) {
	if b.Cbor() != nil {
		return b.Cbor(), nil
	}
	return cbor.EncodeGeneric(b)
}

func (b *MaryTransactionBody) Inputs() []common.TransactionInput {
	ret := make([]common.TransactionInput, 0, len(b.TxInputs.Items()))
	for _, input := range b.TxInputs.Items() {
		ret = append(ret, input)
	}
	return ret
}

func (b *MaryTransactionBody) Outputs() []common.TransactionOutput {
	ret := make([]common.TransactionOutput, len(b.TxOutputs))
	for i := range b.TxOutputs {
		ret[i] = &b.TxOutputs[i]
	}
	return ret
}

func (b *MaryTransactionBody) Fee() *big.Int {
	return new(big.Int).SetUint64(b.TxFee)
}

func (b *MaryTransactionBody) TTL() uint64 {
	return b.Ttl
}

func (b *MaryTransactionBody) ValidityIntervalStart() uint64 {
	if b.TxValidityIntervalStart == nil {
		return 0
	}
	return *b.TxValidityIntervalStart
}

func (b *MaryTransactionBody) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	if b.Update == nil {
		return 0, nil
	}
	updateMap := make(map[common.Blake2b224]common.ProtocolParameterUpdate)
	for k, v := range b.Update.ProtocolParamUpdates {
		updateMap[k] = v
	}
	return b.Update.Epoch, updateMap
}

func (b *MaryTransactionBody) Certificates() []common.Certificate {
	ret := make([]common.Certificate, len(b.TxCertificates))
	for i, cert := range b.TxCertificates {
		ret[i] = cert.Certificate
	}
	return ret
}

func (b *MaryTransactionBody) Withdrawals() map[*common.Address]*big.Int {
	if b.TxWithdrawals == nil {
		return nil
	}
	ret := make(map[*common.Address]*big.Int)
	for k, v := range b.TxWithdrawals {
		ret[k] = new(big.Int).SetUint64(v)
	}
	return ret
}

func (b *MaryTransactionBody) AuxDataHash() *common.Blake2b256 {
	return b.TxAuxDataHash
}

func (b *MaryTransactionBody) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return b.TxMint
}

func (b *MaryTransactionBody) Utxorpc() (*utxorpc.Tx, error) {
	return common.TransactionBodyToUtxorpc(b)
}

type MaryTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash       *common.Blake2b256
	Body       MaryTransactionBody
	WitnessSet shelley.ShelleyTransactionWitnessSet
	TxMetadata common.TransactionMetadatum
	auxData    common.AuxiliaryData
}

func (t *MaryTransaction) UnmarshalCBOR(cborData []byte) error {
	// Decode as raw array to preserve metadata bytes
	var txArray []cbor.RawMessage
	if _, err := cbor.Decode(cborData, &txArray); err != nil {
		return err
	}

	// Ensure we have at least 3 components (body, witness_set, metadata)
	if len(txArray) < 3 {
		return fmt.Errorf(
			"invalid transaction: expected at least 3 components, got %d",
			len(txArray),
		)
	}

	// Decode body
	if _, err := cbor.Decode([]byte(txArray[0]), &t.Body); err != nil {
		return fmt.Errorf("failed to decode transaction body: %w", err)
	}

	// Decode witness set
	if _, err := cbor.Decode([]byte(txArray[1]), &t.WitnessSet); err != nil {
		return fmt.Errorf("failed to decode transaction witness set: %w", err)
	}

	// Handle metadata (component 3, index 2) - always present, but may be CBOR nil
	// Handle metadata (component 3, index 2) - always present, but may be CBOR nil
	if len(txArray) > 2 && len(txArray[2]) > 0 &&
		txArray[2][0] != 0xF6 {
		// 0xF6 is CBOR null

		// Decode auxiliary data
		auxData, err := common.DecodeAuxiliaryData(txArray[2])
		if err == nil && auxData != nil {
			t.auxData = auxData
			// Extract metadata for backward compatibility
			metadata, _ := auxData.Metadata()
			if metadata != nil {
				t.TxMetadata = metadata
			}
		} else {
			// Fallback to old method for backward compatibility
			metadata, err := common.DecodeAuxiliaryDataToMetadata(txArray[2])
			if err == nil && metadata != nil {
				t.TxMetadata = metadata
			}
		}
	}

	t.SetCbor(cborData)
	return nil
}

func (t *MaryTransaction) Metadata() common.TransactionMetadatum {
	return t.TxMetadata
}

func (t *MaryTransaction) AuxiliaryData() common.AuxiliaryData {
	return t.auxData
}

func (MaryTransaction) Type() int {
	return TxTypeMary
}

func (t MaryTransaction) Hash() common.Blake2b256 {
	return t.Id()
}

func (t MaryTransaction) Id() common.Blake2b256 {
	return t.Body.Id()
}

func (t MaryTransaction) LeiosHash() common.Blake2b256 {
	if t.hash == nil {
		tmpHash := common.Blake2b256Hash(t.Cbor())
		t.hash = &tmpHash
	}
	return *t.hash
}

func (t MaryTransaction) Inputs() []common.TransactionInput {
	return t.Body.Inputs()
}

func (t MaryTransaction) Outputs() []common.TransactionOutput {
	return t.Body.Outputs()
}

func (t MaryTransaction) Fee() *big.Int {
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

func (t MaryTransaction) TotalCollateral() *big.Int {
	return t.Body.TotalCollateral()
}

func (t MaryTransaction) Certificates() []common.Certificate {
	return t.Body.Certificates()
}

func (t MaryTransaction) Withdrawals() map[*common.Address]*big.Int {
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

func (t MaryTransaction) CurrentTreasuryValue() *big.Int {
	return t.Body.CurrentTreasuryValue()
}

func (t MaryTransaction) Donation() *big.Int {
	return t.Body.Donation()
}

func (t MaryTransaction) IsValid() bool {
	return true
}

func (t MaryTransaction) Consumed() []common.TransactionInput {
	return t.Inputs()
}

func (t MaryTransaction) Produced() []common.Utxo {
	outputs := t.Outputs()
	ret := make([]common.Utxo, 0, len(outputs))
	for idx, output := range outputs {
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
}

func (t MaryTransaction) Witnesses() common.TransactionWitnessSet {
	return t.WitnessSet
}

func (t *MaryTransaction) MarshalCBOR() ([]byte, error) {
	// If we have stored CBOR (from decode), return it to preserve metadata bytes
	cborData := t.DecodeStoreCbor.Cbor()
	if cborData != nil {
		return cborData, nil
	}
	// Otherwise, construct and encode
	tmpObj := []any{
		t.Body,
		t.WitnessSet,
	}
	if t.TxMetadata != nil {
		tmpObj = append(tmpObj, cbor.RawMessage(t.TxMetadata.Cbor()))
	} else {
		tmpObj = append(tmpObj, nil)
	}
	return cbor.Encode(tmpObj)
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
	// Delegate to MarshalCBOR which handles encoding
	cborData, err := cbor.Encode(t)
	if err != nil {
		panic("CBOR encoding that should never fail has failed: " + err.Error())
	}
	return cborData
}

func (t *MaryTransaction) Utxorpc() (*utxorpc.Tx, error) {
	tx, err := t.Body.Utxorpc()
	if err != nil {
		return nil, fmt.Errorf("failed to convert Mary transaction: %w", err)
	}
	return tx, nil
}

type MaryTransactionOutput struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	OutputAddress common.Address
	OutputAmount  MaryTransactionOutputValue
}

func (o *MaryTransactionOutput) UnmarshalCBOR(cborData []byte) error {
	type tMaryTransactionOutput MaryTransactionOutput
	var tmp tMaryTransactionOutput
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*o = MaryTransactionOutput(tmp)
	o.SetCbor(cborData)
	return nil
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

func (o MaryTransactionOutput) ToPlutusData() data.PlutusData {
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
		data.NewConstr(0),
		// Empty script ref
		data.NewConstr(1),
	)
	return tmpData
}

func (o MaryTransactionOutput) Address() common.Address {
	return o.OutputAddress
}

func (txo MaryTransactionOutput) ScriptRef() common.Script {
	return nil
}

func (o MaryTransactionOutput) Amount() *big.Int {
	return new(big.Int).SetUint64(o.OutputAmount.Amount)
}

func (o MaryTransactionOutput) Assets() *common.MultiAsset[common.MultiAssetTypeOutput] {
	return o.OutputAmount.Assets
}

func (o MaryTransactionOutput) DatumHash() *common.Blake2b256 {
	return nil
}

func (o MaryTransactionOutput) Datum() *common.Datum {
	return nil
}

func (o MaryTransactionOutput) Utxorpc() (*utxorpc.TxOutput, error) {
	addressBytes, err := o.OutputAddress.Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to get address bytes: %w", err)
	}
	return &utxorpc.TxOutput{
			Address: addressBytes,
			Coin:    common.BigIntToUtxorpcBigInt(o.Amount()),
			// Assets: o.Assets,
		},
		err
}

func (o MaryTransactionOutput) String() string {
	assets := ""
	if o.OutputAmount.Assets != nil {
		if as := o.OutputAmount.Assets.String(); as != "[]" {
			assets = " assets=" + as
		}
	}
	return fmt.Sprintf(
		"(MaryTransactionOutput address=%s amount=%d%s)",
		o.OutputAddress.String(),
		o.OutputAmount.Amount,
		assets,
	)
}

type MaryTransactionOutputValue struct {
	cbor.StructAsArray
	Amount uint64
	// We use a pointer here to allow it to be nil
	Assets *common.MultiAsset[common.MultiAssetTypeOutput]
}

func (v *MaryTransactionOutputValue) UnmarshalCBOR(data []byte) error {
	// Try to decode as simple amount first
	if _, err := cbor.Decode(data, &(v.Amount)); err == nil {
		return nil
	}
	type tMaryTransactionOutputValue MaryTransactionOutputValue
	var tmp tMaryTransactionOutputValue
	if _, err := cbor.Decode(data, &tmp); err != nil {
		return err
	}
	*v = MaryTransactionOutputValue(tmp)
	return nil
}

func (v *MaryTransactionOutputValue) MarshalCBOR() ([]byte, error) {
	if v.Assets == nil {
		return cbor.Encode(v.Amount)
	} else {
		return cbor.EncodeGeneric(v)
	}
}

func NewMaryBlockFromCbor(
	data []byte,
	config ...common.VerifyConfig,
) (*MaryBlock, error) {
	var cfg common.VerifyConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	// Default: validation enabled (SkipBodyHashValidation = false)

	var maryBlock MaryBlock
	if _, err := cbor.Decode(data, &maryBlock); err != nil {
		return nil, fmt.Errorf("decode Mary block error: %w", err)
	}

	// Validate body hash during parsing if not skipped
	if !cfg.SkipBodyHashValidation {
		if maryBlock.BlockHeader == nil {
			return nil, errors.New("mary block header is nil")
		}
		if err := common.ValidateBlockBodyHash(
			data,
			maryBlock.BlockHeader.BlockBodyHash(),
			EraNameMary,
			4, // Mary has 4 elements: header, txs, witnesses, aux
		); err != nil {
			return nil, err
		}
	}

	return &maryBlock, nil
}

func NewMaryBlockHeaderFromCbor(data []byte) (*MaryBlockHeader, error) {
	var maryBlockHeader MaryBlockHeader
	if _, err := cbor.Decode(data, &maryBlockHeader); err != nil {
		return nil, fmt.Errorf("decode Mary block header error: %w", err)
	}
	return &maryBlockHeader, nil
}

func NewMaryTransactionBodyFromCbor(data []byte) (*MaryTransactionBody, error) {
	var maryTx MaryTransactionBody
	if _, err := cbor.Decode(data, &maryTx); err != nil {
		return nil, fmt.Errorf("decode Mary transaction body error: %w", err)
	}
	return &maryTx, nil
}

func NewMaryTransactionFromCbor(data []byte) (*MaryTransaction, error) {
	var maryTx MaryTransaction
	if _, err := cbor.Decode(data, &maryTx); err != nil {
		return nil, fmt.Errorf("decode Mary transaction error: %w", err)
	}
	return &maryTx, nil
}

func NewMaryTransactionOutputFromCbor(
	data []byte,
) (*MaryTransactionOutput, error) {
	var maryTxOutput MaryTransactionOutput
	if _, err := cbor.Decode(data, &maryTxOutput); err != nil {
		return nil, fmt.Errorf("decode Mary transaction output error: %w", err)
	}
	return &maryTxOutput, nil
}
