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

package allegra

import (
	"errors"
	"fmt"
	"math/big"

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

var EraAllegra = common.Era{
	Id:   EraIdAllegra,
	Name: EraNameAllegra,
}

func init() {
	common.RegisterEra(EraAllegra)
}

type AllegraBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	BlockHeader            *AllegraBlockHeader
	TransactionBodies      []AllegraTransactionBody
	TransactionWitnessSets []shelley.ShelleyTransactionWitnessSet
	TransactionMetadataSet common.TransactionMetadataSet
}

func (b *AllegraBlock) UnmarshalCBOR(cborData []byte) error {
	type tAllegraBlock AllegraBlock
	var tmp tAllegraBlock
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*b = AllegraBlock(tmp)
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

func (*AllegraBlock) Type() int {
	return BlockTypeAllegra
}

func (b *AllegraBlock) Hash() common.Blake2b256 {
	return b.BlockHeader.Hash()
}

func (b *AllegraBlock) Header() common.BlockHeader {
	return b.BlockHeader
}

func (b *AllegraBlock) PrevHash() common.Blake2b256 {
	return b.BlockHeader.PrevHash()
}

func (b *AllegraBlock) BlockNumber() uint64 {
	return b.BlockHeader.BlockNumber()
}

func (b *AllegraBlock) SlotNumber() uint64 {
	return b.BlockHeader.SlotNumber()
}

func (b *AllegraBlock) IssuerVkey() common.IssuerVkey {
	return b.BlockHeader.IssuerVkey()
}

func (b *AllegraBlock) BlockBodySize() uint64 {
	return b.BlockHeader.BlockBodySize()
}

func (b *AllegraBlock) Era() common.Era {
	return EraAllegra
}

func (b *AllegraBlock) Transactions() []common.Transaction {
	ret := make([]common.Transaction, len(b.TransactionBodies))
	// #nosec G115
	for idx := range b.TransactionBodies {
		tx := &AllegraTransaction{
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

func (b *AllegraBlock) Utxorpc() (*utxorpc.Block, error) {
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

func (b *AllegraBlock) BlockBodyHash() common.Blake2b256 {
	return b.Header().BlockBodyHash()
}

type AllegraBlockHeader struct {
	shelley.ShelleyBlockHeader
}

func (h *AllegraBlockHeader) Era() common.Era {
	return EraAllegra
}

type AllegraTransactionPparamUpdate struct {
	cbor.StructAsArray
	ProtocolParamUpdates map[common.Blake2b224]AllegraProtocolParameterUpdate
	Epoch                uint64
}

type AllegraTransactionBody struct {
	common.TransactionBodyBase
	TxInputs                shelley.ShelleyTransactionInputSet `cbor:"0,keyasint,omitempty"`
	TxOutputs               []shelley.ShelleyTransactionOutput `cbor:"1,keyasint,omitempty"`
	TxFee                   uint64                             `cbor:"2,keyasint,omitempty"`
	Ttl                     uint64                             `cbor:"3,keyasint,omitempty"`
	TxCertificates          []common.CertificateWrapper        `cbor:"4,keyasint,omitempty"`
	TxWithdrawals           map[*common.Address]uint64         `cbor:"5,keyasint,omitempty"`
	Update                  *AllegraTransactionPparamUpdate    `cbor:"6,keyasint,omitempty"`
	TxAuxDataHash           *common.Blake2b256                 `cbor:"7,keyasint,omitempty"`
	TxValidityIntervalStart uint64                             `cbor:"8,keyasint,omitempty"`
}

func (b *AllegraTransactionBody) UnmarshalCBOR(cborData []byte) error {
	type tAllegraTransactionBody AllegraTransactionBody
	var tmp tAllegraTransactionBody
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*b = AllegraTransactionBody(tmp)
	b.SetCbor(cborData)
	return nil
}

func (b *AllegraTransactionBody) Inputs() []common.TransactionInput {
	ret := make([]common.TransactionInput, 0, len(b.TxInputs.Items()))
	for _, input := range b.TxInputs.Items() {
		ret = append(ret, input)
	}
	return ret
}

func (b *AllegraTransactionBody) Outputs() []common.TransactionOutput {
	ret := make([]common.TransactionOutput, len(b.TxOutputs))
	for i := range b.TxOutputs {
		ret[i] = &b.TxOutputs[i]
	}
	return ret
}

func (b *AllegraTransactionBody) Fee() *big.Int {
	return new(big.Int).SetUint64(b.TxFee)
}

func (b *AllegraTransactionBody) TTL() uint64 {
	return b.Ttl
}

func (b *AllegraTransactionBody) ValidityIntervalStart() uint64 {
	return b.TxValidityIntervalStart
}

func (b *AllegraTransactionBody) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	if b.Update == nil {
		return 0, nil
	}
	updateMap := make(map[common.Blake2b224]common.ProtocolParameterUpdate)
	for k, v := range b.Update.ProtocolParamUpdates {
		updateMap[k] = &v
	}
	return b.Update.Epoch, updateMap
}

func (b *AllegraTransactionBody) Certificates() []common.Certificate {
	ret := make([]common.Certificate, len(b.TxCertificates))
	for i, cert := range b.TxCertificates {
		ret[i] = cert.Certificate
	}
	return ret
}

func (b *AllegraTransactionBody) Withdrawals() map[*common.Address]*big.Int {
	if b.TxWithdrawals == nil {
		return nil
	}
	ret := make(map[*common.Address]*big.Int)
	for addr, amount := range b.TxWithdrawals {
		ret[addr] = new(big.Int).SetUint64(amount)
	}
	return ret
}

func (b *AllegraTransactionBody) AuxDataHash() *common.Blake2b256 {
	return b.TxAuxDataHash
}

func (b *AllegraTransactionBody) Utxorpc() (*utxorpc.Tx, error) {
	return common.TransactionBodyToUtxorpc(b)
}

type AllegraTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash       *common.Blake2b256
	Body       AllegraTransactionBody
	WitnessSet shelley.ShelleyTransactionWitnessSet
	TxMetadata common.TransactionMetadatum
	auxData    common.AuxiliaryData
}

func (t *AllegraTransaction) UnmarshalCBOR(cborData []byte) error {
	// Decode as raw array to capture metadata bytes
	var txArray []cbor.RawMessage
	if _, err := cbor.Decode(cborData, &txArray); err != nil {
		return err
	}
	// Ensure we have at least 3 components (body, witness, metadata)
	if len(txArray) < 3 {
		return fmt.Errorf(
			"invalid transaction: expected at least 3 components, got %d",
			len(txArray),
		)
	}
	// Decode body and witness set
	if _, err := cbor.Decode(txArray[0], &t.Body); err != nil {
		return fmt.Errorf("failed to decode transaction body: %w", err)
	}
	if _, err := cbor.Decode(txArray[1], &t.WitnessSet); err != nil {
		return fmt.Errorf("failed to decode transaction witness set: %w", err)
	}
	// Handle metadata (component 3, index 2) - always present, but may be CBOR nil
	// Store raw auxiliary data bytes (including any scripts)
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

func (*AllegraTransaction) Type() int {
	return TxTypeAllegra
}

func (t *AllegraTransaction) Hash() common.Blake2b256 {
	return t.Id()
}

func (t *AllegraTransaction) Id() common.Blake2b256 {
	return t.Body.Id()
}

func (t *AllegraTransaction) LeiosHash() common.Blake2b256 {
	if t.hash == nil {
		tmpHash := common.Blake2b256Hash(t.Cbor())
		t.hash = &tmpHash
	}
	return *t.hash
}

func (t *AllegraTransaction) Inputs() []common.TransactionInput {
	return t.Body.Inputs()
}

func (t *AllegraTransaction) Outputs() []common.TransactionOutput {
	return t.Body.Outputs()
}

func (t *AllegraTransaction) Fee() *big.Int {
	return t.Body.Fee()
}

func (t *AllegraTransaction) TTL() uint64 {
	return t.Body.TTL()
}

func (t *AllegraTransaction) ValidityIntervalStart() uint64 {
	return t.Body.ValidityIntervalStart()
}

func (t *AllegraTransaction) ReferenceInputs() []common.TransactionInput {
	return t.Body.ReferenceInputs()
}

func (t *AllegraTransaction) Collateral() []common.TransactionInput {
	return t.Body.Collateral()
}

func (t *AllegraTransaction) CollateralReturn() common.TransactionOutput {
	return t.Body.CollateralReturn()
}

func (t *AllegraTransaction) TotalCollateral() *big.Int {
	return t.Body.TotalCollateral()
}

func (t *AllegraTransaction) Certificates() []common.Certificate {
	return t.Body.Certificates()
}

func (t *AllegraTransaction) Withdrawals() map[*common.Address]*big.Int {
	return t.Body.Withdrawals()
}

func (t *AllegraTransaction) AuxDataHash() *common.Blake2b256 {
	return t.Body.AuxDataHash()
}

func (t *AllegraTransaction) RequiredSigners() []common.Blake2b224 {
	return t.Body.RequiredSigners()
}

func (t *AllegraTransaction) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return t.Body.AssetMint()
}

func (t *AllegraTransaction) ScriptDataHash() *common.Blake2b256 {
	return t.Body.ScriptDataHash()
}

func (t *AllegraTransaction) VotingProcedures() common.VotingProcedures {
	return t.Body.VotingProcedures()
}

func (t *AllegraTransaction) ProposalProcedures() []common.ProposalProcedure {
	return t.Body.ProposalProcedures()
}

func (t *AllegraTransaction) CurrentTreasuryValue() *big.Int {
	return t.Body.CurrentTreasuryValue()
}

func (t *AllegraTransaction) Donation() *big.Int {
	return t.Body.Donation()
}

func (t *AllegraTransaction) Metadata() common.TransactionMetadatum {
	return t.TxMetadata
}

func (t *AllegraTransaction) AuxiliaryData() common.AuxiliaryData {
	return t.auxData
}

func (t *AllegraTransaction) IsValid() bool {
	return true
}

func (t *AllegraTransaction) Consumed() []common.TransactionInput {
	return t.Inputs()
}

func (t *AllegraTransaction) Produced() []common.Utxo {
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

func (t *AllegraTransaction) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	return t.Body.ProtocolParameterUpdates()
}

func (t *AllegraTransaction) Witnesses() common.TransactionWitnessSet {
	return &t.WitnessSet
}

func (t *AllegraTransaction) Utxorpc() (*utxorpc.Tx, error) {
	tx, err := t.Body.Utxorpc()
	if err != nil {
		return nil, fmt.Errorf("failed to convert Allegra transaction: %w", err)
	}
	return tx, nil
}

func (t *AllegraTransaction) MarshalCBOR() ([]byte, error) {
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
	// Delegate to MarshalCBOR which handles encoding
	cborData, err := t.MarshalCBOR()
	if err != nil {
		panic("CBOR encoding that should never fail has failed: " + err.Error())
	}
	return cborData
}

func NewAllegraBlockFromCbor(
	data []byte,
	config ...common.VerifyConfig,
) (*AllegraBlock, error) {
	var cfg common.VerifyConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	// Default: validation enabled (SkipBodyHashValidation = false)

	var allegraBlock AllegraBlock
	if _, err := cbor.Decode(data, &allegraBlock); err != nil {
		return nil, fmt.Errorf("decode Allegra block error: %w", err)
	}

	// Validate body hash during parsing if not skipped
	if !cfg.SkipBodyHashValidation {
		if allegraBlock.BlockHeader == nil {
			return nil, errors.New("allegra block header is nil")
		}
		if err := common.ValidateBlockBodyHash(
			data,
			allegraBlock.BlockHeader.BlockBodyHash(),
			EraNameAllegra,
			4, // Allegra has 4 elements: header, txs, witnesses, aux
		); err != nil {
			return nil, err
		}
	}

	return &allegraBlock, nil
}

func NewAllegraBlockHeaderFromCbor(data []byte) (*AllegraBlockHeader, error) {
	var allegraBlockHeader AllegraBlockHeader
	if _, err := cbor.Decode(data, &allegraBlockHeader); err != nil {
		return nil, fmt.Errorf("decode Allegra block header error: %w", err)
	}
	return &allegraBlockHeader, nil
}

func NewAllegraTransactionBodyFromCbor(
	data []byte,
) (*AllegraTransactionBody, error) {
	var allegraTx AllegraTransactionBody
	if _, err := cbor.Decode(data, &allegraTx); err != nil {
		return nil, fmt.Errorf("decode Allegra transaction body error: %w", err)
	}
	return &allegraTx, nil
}

func NewAllegraTransactionFromCbor(data []byte) (*AllegraTransaction, error) {
	var allegraTx AllegraTransaction
	if _, err := cbor.Decode(data, &allegraTx); err != nil {
		return nil, fmt.Errorf("decode Allegra transaction error: %w", err)
	}
	return &allegraTx, nil
}
