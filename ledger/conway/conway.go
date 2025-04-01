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
	return b.UnmarshalCbor(cborData, b)
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

func (b *ConwayBlock) Utxorpc() *utxorpc.Block {
	txs := []*utxorpc.Tx{}
	for _, t := range b.Transactions() {
		tx := t.Utxorpc()
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
	return block
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
	babbage.BabbageTransactionWitnessSet
	WsRedeemers       ConwayRedeemers `cbor:"5,keyasint,omitempty"`
	WsPlutusV3Scripts [][]byte        `cbor:"7,keyasint,omitempty"`
}

func (w *ConwayTransactionWitnessSet) UnmarshalCBOR(cborData []byte) error {
	return w.UnmarshalCbor(cborData, w)
}

func (w ConwayTransactionWitnessSet) PlutusV3Scripts() [][]byte {
	return w.WsPlutusV3Scripts
}

func (w ConwayTransactionWitnessSet) Redeemers() common.TransactionWitnessRedeemers {
	return w.WsRedeemers
}

type ConwayTransactionInputSet struct {
	items []shelley.ShelleyTransactionInput
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
	// This overrides the Shelley behavior that explicitly disallowed tag-wrapped sets
	var tmpData []shelley.ShelleyTransactionInput
	if _, err := cbor.Decode(data, &tmpData); err != nil {
		return err
	}
	s.items = tmpData
	return nil
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

type ConwayTransactionBody struct {
	babbage.BabbageTransactionBody
	TxInputs               ConwayTransactionInputSet  `cbor:"0,keyasint,omitempty"`
	TxVotingProcedures     common.VotingProcedures    `cbor:"19,keyasint,omitempty"`
	TxProposalProcedures   []common.ProposalProcedure `cbor:"20,keyasint,omitempty"`
	TxCurrentTreasuryValue int64                      `cbor:"21,keyasint,omitempty"`
	TxDonation             uint64                     `cbor:"22,keyasint,omitempty"`
}

func (b *ConwayTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *ConwayTransactionBody) Inputs() []common.TransactionInput {
	ret := []common.TransactionInput{}
	for _, input := range b.TxInputs.Items() {
		ret = append(ret, input)
	}
	return ret
}

func (b *ConwayTransactionBody) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	updateMap := make(map[common.Blake2b224]common.ProtocolParameterUpdate)
	for k, v := range b.Update.ProtocolParamUpdates {
		updateMap[k] = v
	}
	return b.Update.Epoch, updateMap
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

type ConwayTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       ConwayTransactionBody
	WitnessSet ConwayTransactionWitnessSet
	TxIsValid  bool
	TxMetadata *cbor.LazyValue
}

func (t *ConwayTransaction) UnmarshalCBOR(data []byte) error {
	return t.UnmarshalCbor(data, t)
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
					Id:     shelley.NewShelleyTransactionInput(t.Hash().String(), idx),
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

func (t *ConwayTransaction) Utxorpc() *utxorpc.Tx {
	return t.Body.Utxorpc()
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
