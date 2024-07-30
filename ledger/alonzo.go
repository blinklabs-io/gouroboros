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
	EraIdAlonzo = 4

	BlockTypeAlonzo = 5

	BlockHeaderTypeAlonzo = 4

	TxTypeAlonzo = 4
)

type AlonzoBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Header                 *AlonzoBlockHeader
	TransactionBodies      []AlonzoTransactionBody
	TransactionWitnessSets []AlonzoTransactionWitnessSet
	TransactionMetadataSet map[uint]*cbor.LazyValue
	InvalidTransactions    []uint
}

func (b *AlonzoBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *AlonzoBlock) Hash() string {
	return b.Header.Hash()
}

func (b *AlonzoBlock) BlockNumber() uint64 {
	return b.Header.BlockNumber()
}

func (b *AlonzoBlock) SlotNumber() uint64 {
	return b.Header.SlotNumber()
}

func (b *AlonzoBlock) IssuerVkey() IssuerVkey {
	return b.Header.IssuerVkey()
}

func (b *AlonzoBlock) BlockBodySize() uint64 {
	return b.Header.BlockBodySize()
}

func (b *AlonzoBlock) Era() Era {
	return eras[EraIdAlonzo]
}

func (b *AlonzoBlock) Transactions() []Transaction {
	invalidTxMap := make(map[uint]bool, len(b.InvalidTransactions))
	for _, invalidTxIdx := range b.InvalidTransactions {
		invalidTxMap[invalidTxIdx] = true
	}

	ret := make([]Transaction, len(b.TransactionBodies))
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

type AlonzoBlockHeader struct {
	ShelleyBlockHeader
}

func (h *AlonzoBlockHeader) Era() Era {
	return eras[EraIdAlonzo]
}

type AlonzoTransactionBody struct {
	MaryTransactionBody
	TxOutputs []AlonzoTransactionOutput `cbor:"1,keyasint,omitempty"`
	Update    struct {
		cbor.StructAsArray
		ProtocolParamUpdates map[Blake2b224]AlonzoProtocolParameterUpdate
		Epoch                uint64
	} `cbor:"6,keyasint,omitempty"`
	TxScriptDataHash  *Blake2b256               `cbor:"11,keyasint,omitempty"`
	TxCollateral      []ShelleyTransactionInput `cbor:"13,keyasint,omitempty"`
	TxRequiredSigners []Blake2b224              `cbor:"14,keyasint,omitempty"`
	NetworkId         uint8                     `cbor:"15,keyasint,omitempty"`
}

func (b *AlonzoTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *AlonzoTransactionBody) Outputs() []TransactionOutput {
	ret := []TransactionOutput{}
	for _, output := range b.TxOutputs {
		output := output
		ret = append(ret, &output)
	}
	return ret
}

func (b *AlonzoTransactionBody) ProtocolParametersUpdate() map[Blake2b224]any {
	updateMap := make(map[Blake2b224]any)
	for k, v := range b.Update.ProtocolParamUpdates {
		updateMap[k] = v
	}
	return updateMap
}

func (b *AlonzoTransactionBody) Collateral() []TransactionInput {
	ret := []TransactionInput{}
	for _, collateral := range b.TxCollateral {
		ret = append(ret, collateral)
	}
	return ret
}

func (b *AlonzoTransactionBody) RequiredSigners() []Blake2b224 {
	return b.TxRequiredSigners[:]
}

func (b *AlonzoTransactionBody) ScriptDataHash() *Blake2b256 {
	return b.TxScriptDataHash
}

type AlonzoTransactionOutput struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	OutputAddress     Address
	OutputAmount      MaryTransactionOutputValue
	TxOutputDatumHash *Blake2b256
	legacyOutput      bool
}

func (o *AlonzoTransactionOutput) UnmarshalCBOR(cborData []byte) error {
	// Save original CBOR
	o.SetCbor(cborData)
	// Try to parse as legacy Mary output first
	var tmpOutput MaryTransactionOutput
	if _, err := cbor.Decode(cborData, &tmpOutput); err == nil {
		// Copy from temp Mary output to Alonzo format
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
		tmpOutput := MaryTransactionOutput{
			OutputAddress: o.OutputAddress,
			OutputAmount:  o.OutputAmount,
		}
		return cbor.Encode(&tmpOutput)
	}
	return cbor.EncodeGeneric(o)
}

func (o AlonzoTransactionOutput) MarshalJSON() ([]byte, error) {
	tmpObj := struct {
		Address   Address                           `json:"address"`
		Amount    uint64                            `json:"amount"`
		Assets    *MultiAsset[MultiAssetTypeOutput] `json:"assets,omitempty"`
		DatumHash string                            `json:"datumHash,omitempty"`
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

func (o AlonzoTransactionOutput) Address() Address {
	return o.OutputAddress
}

func (o AlonzoTransactionOutput) Amount() uint64 {
	return o.OutputAmount.Amount
}

func (o AlonzoTransactionOutput) Assets() *MultiAsset[MultiAssetTypeOutput] {
	return o.OutputAmount.Assets
}

func (o AlonzoTransactionOutput) DatumHash() *Blake2b256 {
	return o.TxOutputDatumHash
}

func (o AlonzoTransactionOutput) Datum() *cbor.LazyValue {
	return nil
}

func (o AlonzoTransactionOutput) Utxorpc() *utxorpc.TxOutput {
	var assets []*utxorpc.Multiasset
	if o.Assets() != nil {
		for policyId, policyData := range o.Assets().data {
			var ma = &utxorpc.Multiasset{
				PolicyId: policyId.Bytes(),
			}
			for assetName, amount := range policyData {
				asset := &utxorpc.Asset{
					Name:       assetName.Bytes(),
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
	Tag     uint8
	Index   uint32
	Data    cbor.RawMessage
	ExUnits RedeemerExUnits
}

type RedeemerExUnits struct {
	cbor.StructAsArray
	Memory uint64
	Steps  uint64
}

type AlonzoTransactionWitnessSet struct {
	ShelleyTransactionWitnessSet
	PlutusScripts []cbor.RawMessage `cbor:"3,keyasint,omitempty"`
	PlutusData    []cbor.RawMessage `cbor:"4,keyasint,omitempty"`
	Redeemers     []AlonzoRedeemer  `cbor:"5,keyasint,omitempty"`
}

func (t *AlonzoTransactionWitnessSet) UnmarshalCBOR(cborData []byte) error {
	return t.UnmarshalCbor(cborData, t)
}

type AlonzoTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       AlonzoTransactionBody
	WitnessSet AlonzoTransactionWitnessSet
	IsTxValid  bool
	TxMetadata *cbor.LazyValue
}

func (t AlonzoTransaction) Hash() string {
	return t.Body.Hash()
}

func (t AlonzoTransaction) Inputs() []TransactionInput {
	return t.Body.Inputs()
}

func (t AlonzoTransaction) Outputs() []TransactionOutput {
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

func (t AlonzoTransaction) ProtocolParametersUpdate() map[Blake2b224]any {
	return t.Body.ProtocolParametersUpdate()
}

func (t AlonzoTransaction) ReferenceInputs() []TransactionInput {
	return t.Body.ReferenceInputs()
}

func (t AlonzoTransaction) Collateral() []TransactionInput {
	return t.Body.Collateral()
}

func (t AlonzoTransaction) CollateralReturn() TransactionOutput {
	return t.Body.CollateralReturn()
}

func (t AlonzoTransaction) TotalCollateral() uint64 {
	return t.Body.TotalCollateral()
}

func (t AlonzoTransaction) Certificates() []Certificate {
	return t.Body.Certificates()
}

func (t AlonzoTransaction) Withdrawals() map[*Address]uint64 {
	return t.Body.Withdrawals()
}

func (t AlonzoTransaction) AuxDataHash() *Blake2b256 {
	return t.Body.AuxDataHash()
}

func (t AlonzoTransaction) RequiredSigners() []Blake2b224 {
	return t.Body.RequiredSigners()
}

func (t AlonzoTransaction) AssetMint() *MultiAsset[MultiAssetTypeMint] {
	return t.Body.AssetMint()
}

func (t AlonzoTransaction) ScriptDataHash() *Blake2b256 {
	return t.Body.ScriptDataHash()
}

func (t AlonzoTransaction) VotingProcedures() VotingProcedures {
	return t.Body.VotingProcedures()
}

func (t AlonzoTransaction) ProposalProcedures() []ProposalProcedure {
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

func (t AlonzoTransaction) Consumed() []TransactionInput {
	if t.IsValid() {
		return t.Inputs()
	} else {
		return t.Collateral()
	}
}

func (t AlonzoTransaction) Produced() []Utxo {
	if t.IsValid() {
		var ret []Utxo
		for idx, output := range t.Outputs() {
			ret = append(
				ret,
				Utxo{
					Id:     NewShelleyTransactionInput(t.Hash(), idx),
					Output: output,
				},
			)
		}
		return ret
	} else {
		// No collateral return in Alonzo
		return []Utxo{}
	}
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

type ExUnit struct {
	cbor.StructAsArray
	Mem   uint
	Steps uint
}

type ExUnitPrice struct {
	cbor.StructAsArray
	MemPrice  uint
	StepPrice uint
}

type AlonzoProtocolParameters struct {
	MaryProtocolParameters
	MinPoolCost          uint
	AdaPerUtxoByte       uint
	CostModels           uint
	ExecutionCosts       uint
	MaxTxExUnits         uint
	MaxBlockExUnits      uint
	MaxValueSize         uint
	CollateralPercentage uint
	MaxCollateralInputs  uint
}

type AlonzoProtocolParameterUpdate struct {
	MaryProtocolParameterUpdate
	MinPoolCost          uint            `cbor:"16,keyasint"`
	AdaPerUtxoByte       uint            `cbor:"17,keyasint"`
	CostModels           map[uint][]uint `cbor:"18,keyasint"`
	ExecutionCosts       *ExUnitPrice    `cbor:"19,keyasint"`
	MaxTxExUnits         *ExUnit         `cbor:"20,keyasint"`
	MaxBlockExUnits      *ExUnit         `cbor:"21,keyasint"`
	MaxValueSize         uint            `cbor:"22,keyasint"`
	CollateralPercentage uint            `cbor:"23,keyasint"`
	MaxCollateralInputs  uint            `cbor:"24,keyasint"`
}

func NewAlonzoBlockFromCbor(data []byte) (*AlonzoBlock, error) {
	var alonzoBlock AlonzoBlock
	if _, err := cbor.Decode(data, &alonzoBlock); err != nil {
		return nil, fmt.Errorf("Alonzo block decode error: %s", err)
	}
	return &alonzoBlock, nil
}

func NewAlonzoTransactionBodyFromCbor(
	data []byte,
) (*AlonzoTransactionBody, error) {
	var alonzoTx AlonzoTransactionBody
	if _, err := cbor.Decode(data, &alonzoTx); err != nil {
		return nil, fmt.Errorf("Alonzo transaction body decode error: %s", err)
	}
	return &alonzoTx, nil
}

func NewAlonzoTransactionFromCbor(data []byte) (*AlonzoTransaction, error) {
	var alonzoTx AlonzoTransaction
	if _, err := cbor.Decode(data, &alonzoTx); err != nil {
		return nil, fmt.Errorf("Alonzo transaction decode error: %s", err)
	}
	return &alonzoTx, nil
}

func NewAlonzoTransactionOutputFromCbor(
	data []byte,
) (*AlonzoTransactionOutput, error) {
	var alonzoTxOutput AlonzoTransactionOutput
	if _, err := cbor.Decode(data, &alonzoTxOutput); err != nil {
		return nil, fmt.Errorf(
			"Alonzo transaction output decode error: %s",
			err,
		)
	}
	return &alonzoTxOutput, nil
}
