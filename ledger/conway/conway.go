// Copyright 2024 Blink Labs Software
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
	"encoding/hex"
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

var (
	EraConway = common.Era{
		Id:   EraIdConway,
		Name: EraNameConway,
	}
)

func init() {
	common.RegisterEra(EraConway)
}

type ConwayBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Header                 *ConwayBlockHeader
	TransactionBodies      []ConwayTransactionBody
	TransactionWitnessSets []ConwayTransactionWitnessSet
	TransactionMetadataSet map[uint]*cbor.LazyValue
	InvalidTransactions    []uint
}

func (b *ConwayBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *ConwayBlock) Hash() string {
	return b.Header.Hash()
}

func (b *ConwayBlock) BlockNumber() uint64 {
	return b.Header.BlockNumber()
}

func (b *ConwayBlock) SlotNumber() uint64 {
	return b.Header.SlotNumber()
}

func (b *ConwayBlock) IssuerVkey() common.IssuerVkey {
	return b.Header.IssuerVkey()
}

func (b *ConwayBlock) BlockBodySize() uint64 {
	return b.Header.BlockBodySize()
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
	for idx := range b.TransactionBodies {
		ret[idx] = &ConwayTransaction{
			Body:       b.TransactionBodies[idx],
			WitnessSet: b.TransactionWitnessSets[idx],
			TxMetadata: b.TransactionMetadataSet[uint(idx)],
			IsTxValid:  !invalidTxMap[uint(idx)],
		}
	}
	return ret
}

func (b *ConwayBlock) Utxorpc() *utxorpc.Block {
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

type ConwayBlockHeader struct {
	babbage.BabbageBlockHeader
}

func (h *ConwayBlockHeader) Era() common.Era {
	return EraConway
}

type ConwayRedeemerKey struct {
	cbor.StructAsArray
	Tag   uint8
	Index uint32
}

type ConwayRedeemerValue struct {
	cbor.StructAsArray
	Data    cbor.RawMessage
	ExUnits common.RedeemerExUnits
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

type ConwayTransactionWitnessSet struct {
	babbage.BabbageTransactionWitnessSet
	Redeemers       ConwayRedeemers   `cbor:"5,keyasint,omitempty"`
	PlutusV3Scripts []cbor.RawMessage `cbor:"7,keyasint,omitempty"`
}

func (t *ConwayTransactionWitnessSet) UnmarshalCBOR(cborData []byte) error {
	return t.UnmarshalCbor(cborData, t)
}

type ConwayTransactionBody struct {
	babbage.BabbageTransactionBody
	TxVotingProcedures     common.VotingProcedures    `cbor:"19,keyasint,omitempty"`
	TxProposalProcedures   []common.ProposalProcedure `cbor:"20,keyasint,omitempty"`
	TxCurrentTreasuryValue int64                      `cbor:"21,keyasint,omitempty"`
	TxDonation             uint64                     `cbor:"22,keyasint,omitempty"`
}

func (b *ConwayTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *ConwayTransactionBody) ProtocolParametersUpdate() map[common.Blake2b224]any {
	updateMap := make(map[common.Blake2b224]any)
	for k, v := range b.Update.ProtocolParamUpdates {
		updateMap[k] = v
	}
	return updateMap
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

type ConwayProtocolParameters struct {
	cbor.StructAsArray
	MinFeeA            uint
	MinFeeB            uint
	MaxBlockBodySize   uint
	MaxTxSize          uint
	MaxBlockHeaderSize uint
	KeyDeposit         uint
	PoolDeposit        uint
	MaxEpoch           uint
	NOpt               uint
	A0                 *cbor.Rat
	Rho                *cbor.Rat
	Tau                *cbor.Rat
	Protocol           struct {
		cbor.StructAsArray
		Major uint
		Minor uint
	}
	MinPoolCost                uint
	AdaPerUtxoByte             uint
	CostModels                 map[uint][]int
	ExecutionUnitPrices        []*cbor.Rat // [priceMemory priceSteps]
	MaxTxExecutionUnits        []uint
	MaxBlockExecutionUnits     []uint
	MaxValueSize               uint
	CollateralPercentage       uint
	MaxCollateralInputs        uint
	PoolVotingThresholds       PoolVotingThresholds
	DRepVotingThresholds       DRepVotingThresholds
	MinCommitteeSize           uint
	CommitteeTermLimit         uint64
	GovActionValidityPeriod    uint64
	GovActionDeposit           uint64
	DRepDeposit                uint64
	DRepInactivityPeriod       uint64
	MinFeeRefScriptCostPerByte *cbor.Rat
}

type ConwayProtocolParameterUpdate struct {
	babbage.BabbageProtocolParameterUpdate
	PoolVotingThresholds       PoolVotingThresholds `cbor:"25,keyasint"`
	DRepVotingThresholds       DRepVotingThresholds `cbor:"26,keyasint"`
	MinCommitteeSize           uint                 `cbor:"27,keyasint"`
	CommitteeTermLimit         uint64               `cbor:"28,keyasint"`
	GovActionValidityPeriod    uint64               `cbor:"29,keyasint"`
	GovActionDeposit           uint64               `cbor:"30,keyasint"`
	DRepDeposit                uint64               `cbor:"31,keyasint"`
	DRepInactivityPeriod       uint64               `cbor:"32,keyasint"`
	MinFeeRefScriptCostPerByte *cbor.Rat            `cbor:"33,keyasint"`
}

type PoolVotingThresholds struct {
	cbor.StructAsArray
	MotionNoConfidence                       cbor.Rat
	CommitteeNormal                          cbor.Rat
	CommitteeNoConfidence                    cbor.Rat
	HardForkInitiation                       cbor.Rat
	SecurityRelevantParameterVotingThreshold cbor.Rat
}

type DRepVotingThresholds struct {
	cbor.StructAsArray
	MotionNoConfidence    cbor.Rat
	CommitteeNormal       cbor.Rat
	CommitteeNoConfidence cbor.Rat
	UpdateConstitution    cbor.Rat
	HardForkInitiation    cbor.Rat
	PPNetworkGroup        cbor.Rat
	PPEconomicGroup       cbor.Rat
	PPTechnicalGroup      cbor.Rat
	PPGovGroup            cbor.Rat
	TreasureWithdrawal    cbor.Rat
}

type ConwayTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       ConwayTransactionBody
	WitnessSet ConwayTransactionWitnessSet
	IsTxValid  bool
	TxMetadata *cbor.LazyValue
}

func (t ConwayTransaction) Hash() string {
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

func (t ConwayTransaction) ProtocolParametersUpdate() map[common.Blake2b224]any {
	return t.Body.ProtocolParametersUpdate()
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
	return t.IsTxValid
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

func (t *ConwayTransaction) Utxorpc() *utxorpc.Tx {
	return t.Body.Utxorpc()
}

func NewConwayBlockFromCbor(data []byte) (*ConwayBlock, error) {
	var conwayBlock ConwayBlock
	if _, err := cbor.Decode(data, &conwayBlock); err != nil {
		return nil, fmt.Errorf("Conway block decode error: %s", err)
	}
	return &conwayBlock, nil
}

func NewConwayBlockHeaderFromCbor(data []byte) (*ConwayBlockHeader, error) {
	var conwayBlockHeader ConwayBlockHeader
	if _, err := cbor.Decode(data, &conwayBlockHeader); err != nil {
		return nil, fmt.Errorf("Conway block header decode error: %s", err)
	}
	return &conwayBlockHeader, nil
}

func NewConwayTransactionBodyFromCbor(
	data []byte,
) (*ConwayTransactionBody, error) {
	var conwayTx ConwayTransactionBody
	if _, err := cbor.Decode(data, &conwayTx); err != nil {
		return nil, fmt.Errorf("Conway transaction body decode error: %s", err)
	}
	return &conwayTx, nil
}

func NewConwayTransactionFromCbor(data []byte) (*ConwayTransaction, error) {
	var conwayTx ConwayTransaction
	if _, err := cbor.Decode(data, &conwayTx); err != nil {
		return nil, fmt.Errorf("Conway transaction decode error: %s", err)
	}
	return &conwayTx, nil
}
