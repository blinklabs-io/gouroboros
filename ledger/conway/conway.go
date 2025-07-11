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

package conway

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

const (
	EraIdConway   = 6
	EraNameConway = "Conway"

	BlockTypeConway = 7

	BlockHeaderTypeConway = 6

	TxTypeConway = 6
)

var EraConway = common.Era{
	Id:   EraIdConway,
	Name: EraNameConway,
}

func init() {
	common.RegisterEra(EraConway)
}

type ConwayBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	BlockHeader            *ConwayBlockHeader
	TransactionBodies      []ConwayTransactionBody
	TransactionWitnessSets []ConwayTransactionWitnessSet
	TransactionMetadataSet map[uint]*cbor.LazyValue
	InvalidTransactions    []uint
}

func (b *ConwayBlock) UnmarshalCBOR(cborData []byte) error {
	type tConwayBlock ConwayBlock
	var tmp tConwayBlock
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*b = ConwayBlock(tmp)
	b.SetCbor(cborData)
	return nil
}

func (ConwayBlock) Type() int {
	return BlockTypeConway
}

func (b *ConwayBlock) Hash() common.Blake2b256 {
	return b.BlockHeader.Hash()
}

func (b *ConwayBlock) Header() common.BlockHeader {
	return b.BlockHeader
}

func (b *ConwayBlock) PrevHash() common.Blake2b256 {
	return b.BlockHeader.PrevHash()
}

func (b *ConwayBlock) BlockNumber() uint64 {
	return b.BlockHeader.BlockNumber()
}

func (b *ConwayBlock) SlotNumber() uint64 {
	return b.BlockHeader.SlotNumber()
}

func (b *ConwayBlock) IssuerVkey() common.IssuerVkey {
	return b.BlockHeader.IssuerVkey()
}

func (b *ConwayBlock) BlockBodySize() uint64 {
	return b.BlockHeader.BlockBodySize()
}

func (b *ConwayBlock) Era() common.Era {
	return EraConway
}

func (b *ConwayBlock) Transactions() []common.Transaction {
	invalidTxMap := make(map[uint]bool, len(b.InvalidTransactions))
	for _, invalidTxIdx := range b.InvalidTransactions {
		invalidTxMap[invalidTxIdx] = true
	}

	ret := make([]common.Transaction, len(b.TransactionBodies))
	// #nosec G115
	for idx := range b.TransactionBodies {
		ret[idx] = &ConwayTransaction{
			Body:       b.TransactionBodies[idx],
			WitnessSet: b.TransactionWitnessSets[idx],
			TxMetadata: b.TransactionMetadataSet[uint(idx)],
			TxIsValid:  !invalidTxMap[uint(idx)],
		}
	}
	return ret
}

func (b *ConwayBlock) Utxorpc() (*utxorpc.Block, error) {
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

type ConwayBlockHeader struct {
	babbage.BabbageBlockHeader
}

func (h *ConwayBlockHeader) Era() common.Era {
	return EraConway
}

type ConwayRedeemerKey struct {
	cbor.StructAsArray
	Tag   common.RedeemerTag
	Index uint32
}

type ConwayRedeemerValue struct {
	cbor.StructAsArray
	Data    cbor.LazyValue
	ExUnits common.ExUnits
}

type ConwayRedeemers struct {
	Redeemers map[ConwayRedeemerKey]ConwayRedeemerValue
	legacy    bool
}

func (r *ConwayRedeemers) UnmarshalCBOR(cborData []byte) error {
	// Try to parse as legacy redeemer first
	var tmpRedeemers []alonzo.AlonzoRedeemer
	if _, err := cbor.Decode(cborData, &tmpRedeemers); err == nil {
		// Copy data from legacy redeemer type
		r.Redeemers = make(map[ConwayRedeemerKey]ConwayRedeemerValue)
		for _, redeemer := range tmpRedeemers {
			tmpKey := ConwayRedeemerKey{
				Tag:   redeemer.Tag,
				Index: redeemer.Index,
			}
			tmpVal := ConwayRedeemerValue{
				Data:    redeemer.Data,
				ExUnits: redeemer.ExUnits,
			}
			r.Redeemers[tmpKey] = tmpVal
		}
		r.legacy = true
	} else {
		_, err := cbor.Decode(cborData, &(r.Redeemers))
		return err
	}
	return nil
}

func (r ConwayRedeemers) Indexes(tag common.RedeemerTag) []uint {
	ret := []uint{}
	for key := range r.Redeemers {
		if key.Tag == tag {
			ret = append(ret, uint(key.Index))
		}
	}
	return ret
}

func (r ConwayRedeemers) Value(
	index uint,
	tag common.RedeemerTag,
) (cbor.LazyValue, common.ExUnits) {
	redeemer, ok := r.Redeemers[ConwayRedeemerKey{
		Tag:   tag,
		Index: uint32(index), // #nosec G115
	}]
	if ok {
		return redeemer.Data, redeemer.ExUnits
	}
	return cbor.LazyValue{}, common.ExUnits{}
}

type ConwayTransactionWitnessSet struct {
	cbor.DecodeStoreCbor
	VkeyWitnesses      []common.VkeyWitness      `cbor:"0,keyasint,omitempty"`
	WsNativeScripts    []common.NativeScript     `cbor:"1,keyasint,omitempty"`
	BootstrapWitnesses []common.BootstrapWitness `cbor:"2,keyasint,omitempty"`
	WsPlutusV1Scripts  [][]byte                  `cbor:"3,keyasint,omitempty"`
	WsPlutusData       []cbor.Value              `cbor:"4,keyasint,omitempty"`
	WsRedeemers        ConwayRedeemers           `cbor:"5,keyasint,omitempty"`
	WsPlutusV2Scripts  [][]byte                  `cbor:"6,keyasint,omitempty"`
	WsPlutusV3Scripts  [][]byte                  `cbor:"7,keyasint,omitempty"`
}

func (w *ConwayTransactionWitnessSet) UnmarshalCBOR(cborData []byte) error {
	type tConwayTransactionWitnessSet ConwayTransactionWitnessSet
	var tmp tConwayTransactionWitnessSet
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*w = ConwayTransactionWitnessSet(tmp)
	w.SetCbor(cborData)
	return nil
}

func (w ConwayTransactionWitnessSet) Vkey() []common.VkeyWitness {
	return w.VkeyWitnesses
}

func (w ConwayTransactionWitnessSet) Bootstrap() []common.BootstrapWitness {
	return w.BootstrapWitnesses
}

func (w ConwayTransactionWitnessSet) NativeScripts() []common.NativeScript {
	return w.WsNativeScripts
}

func (w ConwayTransactionWitnessSet) PlutusV1Scripts() [][]byte {
	return w.WsPlutusV1Scripts
}

func (w ConwayTransactionWitnessSet) PlutusV2Scripts() [][]byte {
	return w.WsPlutusV2Scripts
}

func (w ConwayTransactionWitnessSet) PlutusV3Scripts() [][]byte {
	return w.WsPlutusV3Scripts
}

func (w ConwayTransactionWitnessSet) PlutusData() []cbor.Value {
	return w.WsPlutusData
}

func (w ConwayTransactionWitnessSet) Redeemers() common.TransactionWitnessRedeemers {
	return w.WsRedeemers
}

type ConwayTransactionInputSet struct {
	useSet bool
	items  []shelley.ShelleyTransactionInput
}

func NewConwayTransactionInputSet(
	items []shelley.ShelleyTransactionInput,
) ConwayTransactionInputSet {
	s := ConwayTransactionInputSet{
		items: items,
	}
	return s
}

func (s *ConwayTransactionInputSet) UnmarshalCBOR(data []byte) error {
	// Check if the set is wrapped in a CBOR tag
	// This is mostly needed so we can remember whether it was Set-wrapped for CBOR encoding
	var tmpTag cbor.RawTag
	if _, err := cbor.Decode(data, &tmpTag); err == nil {
		if tmpTag.Number != cbor.CborTagSet {
			return errors.New("unexpected tag type")
		}
		data = []byte(tmpTag.Content)
		s.useSet = true
	}
	var tmpData []shelley.ShelleyTransactionInput
	if _, err := cbor.Decode(data, &tmpData); err != nil {
		return err
	}
	s.items = tmpData
	return nil
}

func (s *ConwayTransactionInputSet) MarshalCBOR() ([]byte, error) {
	tmpItems := make([]any, len(s.items))
	for i, item := range s.items {
		tmpItems[i] = item
	}
	var tmpData any = tmpItems
	if s.useSet {
		tmpData = cbor.Set(tmpItems)
	}
	return cbor.Encode(tmpData)
}

func (s *ConwayTransactionInputSet) Items() []shelley.ShelleyTransactionInput {
	return s.items
}

func (s *ConwayTransactionInputSet) SetItems(
	items []shelley.ShelleyTransactionInput,
) {
	s.items = make([]shelley.ShelleyTransactionInput, len(items))
	copy(s.items, items)
}

type ConwayTransactionPparamUpdate struct {
	cbor.StructAsArray
	ProtocolParamUpdates map[common.Blake2b224]babbage.BabbageProtocolParameterUpdate
	Epoch                uint64
}

type ConwayTransactionBody struct {
	common.TransactionBodyBase
	TxInputs                ConwayTransactionInputSet                     `cbor:"0,keyasint,omitempty"`
	TxOutputs               []babbage.BabbageTransactionOutput            `cbor:"1,keyasint,omitempty"`
	TxFee                   uint64                                        `cbor:"2,keyasint,omitempty"`
	Ttl                     uint64                                        `cbor:"3,keyasint,omitempty"`
	TxCertificates          []common.CertificateWrapper                   `cbor:"4,keyasint,omitempty"`
	TxWithdrawals           map[*common.Address]uint64                    `cbor:"5,keyasint,omitempty"`
	Update                  *ConwayTransactionPparamUpdate                `cbor:"6,keyasint,omitempty"`
	TxAuxDataHash           *common.Blake2b256                            `cbor:"7,keyasint,omitempty"`
	TxValidityIntervalStart uint64                                        `cbor:"8,keyasint,omitempty"`
	TxMint                  *common.MultiAsset[common.MultiAssetTypeMint] `cbor:"9,keyasint,omitempty"`
	TxScriptDataHash        *common.Blake2b256                            `cbor:"11,keyasint,omitempty"`
	TxCollateral            cbor.SetType[shelley.ShelleyTransactionInput] `cbor:"13,keyasint,omitempty,omitzero"`
	TxRequiredSigners       cbor.SetType[common.Blake2b224]               `cbor:"14,keyasint,omitempty,omitzero"`
	NetworkId               uint8                                         `cbor:"15,keyasint,omitempty"`
	TxCollateralReturn      *babbage.BabbageTransactionOutput             `cbor:"16,keyasint,omitempty"`
	TxTotalCollateral       uint64                                        `cbor:"17,keyasint,omitempty"`
	TxReferenceInputs       cbor.SetType[shelley.ShelleyTransactionInput] `cbor:"18,keyasint,omitempty,omitzero"`
	TxVotingProcedures      common.VotingProcedures                       `cbor:"19,keyasint,omitempty"`
	TxProposalProcedures    []common.ProposalProcedure                    `cbor:"20,keyasint,omitempty"`
	TxCurrentTreasuryValue  int64                                         `cbor:"21,keyasint,omitempty"`
	TxDonation              uint64                                        `cbor:"22,keyasint,omitempty"`
}

func (b *ConwayTransactionBody) UnmarshalCBOR(cborData []byte) error {
	type tConwayTransactionBody ConwayTransactionBody
	var tmp tConwayTransactionBody
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*b = ConwayTransactionBody(tmp)
	b.SetCbor(cborData)
	return nil
}

func (b *ConwayTransactionBody) Inputs() []common.TransactionInput {
	ret := []common.TransactionInput{}
	for _, input := range b.TxInputs.Items() {
		ret = append(ret, input)
	}
	return ret
}

func (b *ConwayTransactionBody) Outputs() []common.TransactionOutput {
	ret := []common.TransactionOutput{}
	for _, output := range b.TxOutputs {
		ret = append(ret, &output)
	}
	return ret
}

func (b *ConwayTransactionBody) Fee() uint64 {
	return b.TxFee
}

func (b *ConwayTransactionBody) TTL() uint64 {
	return b.Ttl
}

func (b *ConwayTransactionBody) ValidityIntervalStart() uint64 {
	return b.TxValidityIntervalStart
}

func (b *ConwayTransactionBody) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	if b.Update == nil {
		return 0, nil
	}
	updateMap := make(map[common.Blake2b224]common.ProtocolParameterUpdate)
	for k, v := range b.Update.ProtocolParamUpdates {
		updateMap[k] = v
	}
	return b.Update.Epoch, updateMap
}

func (b *ConwayTransactionBody) Certificates() []common.Certificate {
	ret := make([]common.Certificate, len(b.TxCertificates))
	for i, cert := range b.TxCertificates {
		ret[i] = cert.Certificate
	}
	return ret
}

func (b *ConwayTransactionBody) Withdrawals() map[*common.Address]uint64 {
	return b.TxWithdrawals
}

func (b *ConwayTransactionBody) AuxDataHash() *common.Blake2b256 {
	return b.TxAuxDataHash
}

func (b *ConwayTransactionBody) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return b.TxMint
}

func (b *ConwayTransactionBody) Collateral() []common.TransactionInput {
	ret := []common.TransactionInput{}
	for _, collateral := range b.TxCollateral.Items() {
		ret = append(ret, collateral)
	}
	return ret
}

func (b *ConwayTransactionBody) RequiredSigners() []common.Blake2b224 {
	return b.TxRequiredSigners.Items()
}

func (b *ConwayTransactionBody) ScriptDataHash() *common.Blake2b256 {
	return b.TxScriptDataHash
}

func (b *ConwayTransactionBody) ReferenceInputs() []common.TransactionInput {
	ret := []common.TransactionInput{}
	for _, input := range b.TxReferenceInputs.Items() {
		ret = append(ret, &input)
	}
	return ret
}

func (b *ConwayTransactionBody) CollateralReturn() common.TransactionOutput {
	// Return an actual nil if we have no value. If we return our nil pointer,
	// we get a non-nil interface containing a nil value, which is harder to
	// compare against
	if b.TxCollateralReturn == nil {
		return nil
	}
	return b.TxCollateralReturn
}

func (b *ConwayTransactionBody) TotalCollateral() uint64 {
	return b.TxTotalCollateral
}

func (b *ConwayTransactionBody) VotingProcedures() common.VotingProcedures {
	return b.TxVotingProcedures
}

func (b *ConwayTransactionBody) ProposalProcedures() []common.ProposalProcedure {
	return b.TxProposalProcedures
}

func (b *ConwayTransactionBody) CurrentTreasuryValue() int64 {
	return b.TxCurrentTreasuryValue
}

func (b *ConwayTransactionBody) Donation() uint64 {
	return b.TxDonation
}

func (b *ConwayTransactionBody) Utxorpc() (*utxorpc.Tx, error) {
	return common.TransactionBodyToUtxorpc(b), nil
}

type ConwayTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       ConwayTransactionBody
	WitnessSet ConwayTransactionWitnessSet
	TxIsValid  bool
	TxMetadata *cbor.LazyValue
}

func (t *ConwayTransaction) UnmarshalCBOR(cborData []byte) error {
	type tConwayTransaction ConwayTransaction
	var tmp tConwayTransaction
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*t = ConwayTransaction(tmp)
	t.SetCbor(cborData)
	return nil
}

func (ConwayTransaction) Type() int {
	return TxTypeConway
}

func (t ConwayTransaction) Hash() common.Blake2b256 {
	return t.Body.Hash()
}

func (t ConwayTransaction) Inputs() []common.TransactionInput {
	return t.Body.Inputs()
}

func (t ConwayTransaction) Outputs() []common.TransactionOutput {
	return t.Body.Outputs()
}

func (t ConwayTransaction) Fee() uint64 {
	return t.Body.Fee()
}

func (t ConwayTransaction) TTL() uint64 {
	return t.Body.TTL()
}

func (t ConwayTransaction) ValidityIntervalStart() uint64 {
	return t.Body.ValidityIntervalStart()
}

func (t ConwayTransaction) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	return t.Body.ProtocolParameterUpdates()
}

func (t ConwayTransaction) ReferenceInputs() []common.TransactionInput {
	return t.Body.ReferenceInputs()
}

func (t ConwayTransaction) Collateral() []common.TransactionInput {
	return t.Body.Collateral()
}

func (t ConwayTransaction) CollateralReturn() common.TransactionOutput {
	return t.Body.CollateralReturn()
}

func (t ConwayTransaction) TotalCollateral() uint64 {
	return t.Body.TotalCollateral()
}

func (t ConwayTransaction) Certificates() []common.Certificate {
	return t.Body.Certificates()
}

func (t ConwayTransaction) Withdrawals() map[*common.Address]uint64 {
	return t.Body.Withdrawals()
}

func (t ConwayTransaction) AuxDataHash() *common.Blake2b256 {
	return t.Body.AuxDataHash()
}

func (t ConwayTransaction) RequiredSigners() []common.Blake2b224 {
	return t.Body.RequiredSigners()
}

func (t ConwayTransaction) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return t.Body.AssetMint()
}

func (t ConwayTransaction) ScriptDataHash() *common.Blake2b256 {
	return t.Body.ScriptDataHash()
}

func (t ConwayTransaction) VotingProcedures() common.VotingProcedures {
	return t.Body.VotingProcedures()
}

func (t ConwayTransaction) ProposalProcedures() []common.ProposalProcedure {
	return t.Body.ProposalProcedures()
}

func (t ConwayTransaction) CurrentTreasuryValue() int64 {
	return t.Body.CurrentTreasuryValue()
}

func (t ConwayTransaction) Donation() uint64 {
	return t.Body.Donation()
}

func (t ConwayTransaction) Metadata() *cbor.LazyValue {
	return t.TxMetadata
}

func (t ConwayTransaction) IsValid() bool {
	return t.TxIsValid
}

func (t ConwayTransaction) Consumed() []common.TransactionInput {
	if t.IsValid() {
		return t.Inputs()
	} else {
		return t.Collateral()
	}
}

func (t ConwayTransaction) Produced() []common.Utxo {
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

func (t ConwayTransaction) Witnesses() common.TransactionWitnessSet {
	return t.WitnessSet
}

func (t *ConwayTransaction) Cbor() []byte {
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

func (t *ConwayTransaction) Utxorpc() (*utxorpc.Tx, error) {
	tx, err := t.Body.Utxorpc()
	if err != nil {
		return nil, fmt.Errorf("failed to convert Conway transaction: %w", err)
	}
	return tx, nil
}

func NewConwayBlockFromCbor(data []byte) (*ConwayBlock, error) {
	var conwayBlock ConwayBlock
	if _, err := cbor.Decode(data, &conwayBlock); err != nil {
		return nil, fmt.Errorf("decode Conway block error: %w", err)
	}
	return &conwayBlock, nil
}

func NewConwayBlockHeaderFromCbor(data []byte) (*ConwayBlockHeader, error) {
	var conwayBlockHeader ConwayBlockHeader
	if _, err := cbor.Decode(data, &conwayBlockHeader); err != nil {
		return nil, fmt.Errorf("decode Conway block header error: %w", err)
	}
	return &conwayBlockHeader, nil
}

func NewConwayTransactionBodyFromCbor(
	data []byte,
) (*ConwayTransactionBody, error) {
	var conwayTx ConwayTransactionBody
	if _, err := cbor.Decode(data, &conwayTx); err != nil {
		return nil, fmt.Errorf("decode Conway transaction body error: %w", err)
	}
	return &conwayTx, nil
}

func NewConwayTransactionFromCbor(data []byte) (*ConwayTransaction, error) {
	var conwayTx ConwayTransaction
	if _, err := cbor.Decode(data, &conwayTx); err != nil {
		return nil, fmt.Errorf("decode Conway transaction error: %w", err)
	}
	return &conwayTx, nil
}
