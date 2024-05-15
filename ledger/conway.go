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
	"fmt"

	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"

	"github.com/blinklabs-io/gouroboros/cbor"
)

const (
	EraIdConway = 6

	BlockTypeConway = 7

	BlockHeaderTypeConway = 6

	TxTypeConway = 6
)

type ConwayBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Header                 *ConwayBlockHeader
	TransactionBodies      []ConwayTransactionBody
	TransactionWitnessSets []BabbageTransactionWitnessSet
	TransactionMetadataSet map[uint]*cbor.Value
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

func (b *ConwayBlock) IssuerVkey() IssuerVkey {
	return b.Header.IssuerVkey()
}

func (b *ConwayBlock) BlockBodySize() uint64 {
	return b.Header.BlockBodySize()
}

func (b *ConwayBlock) Era() Era {
	return eras[EraIdConway]
}

func (b *ConwayBlock) Transactions() []Transaction {
	invalidTxMap := make(map[uint]bool, len(b.InvalidTransactions))
	for _, invalidTxIdx := range b.InvalidTransactions {
		invalidTxMap[invalidTxIdx] = true
	}

	ret := make([]Transaction, len(b.TransactionBodies))
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
	BabbageBlockHeader
}

func (h *ConwayBlockHeader) Era() Era {
	return eras[EraIdConway]
}

type ConwayTransactionBody struct {
	BabbageTransactionBody
	TxVotingProcedures   VotingProcedures    `cbor:"19,keyasint,omitempty"`
	TxProposalProcedures []ProposalProcedure `cbor:"20,keyasint,omitempty"`
	CurrentTreasuryValue int64               `cbor:"21,keyasint,omitempty"`
	Donation             uint64              `cbor:"22,keyasint,omitempty"`
}

func (b *ConwayTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *ConwayTransactionBody) VotingProcedures() VotingProcedures {
	return b.TxVotingProcedures
}

func (b *ConwayTransactionBody) ProposalProcedures() []ProposalProcedure {
	return b.TxProposalProcedures
}

// VotingProcedures is a convenience type to avoid needing to duplicate the full type definition everywhere
type VotingProcedures map[*Voter]map[*GovActionId]VotingProcedure

const (
	VoterTypeConstitutionalCommitteeHotKeyHash    uint8 = 0
	VoterTypeConstitutionalCommitteeHotScriptHash uint8 = 1
	VoterTypeDRepKeyHash                          uint8 = 2
	VoterTypeDRepScriptHash                       uint8 = 3
	VoterTypeStakingPoolKeyHash                   uint8 = 4
)

type Voter struct {
	cbor.StructAsArray
	Type uint8
	Hash [28]byte
}

const (
	GovVoteNo      uint8 = 0
	GovVoteYes     uint8 = 1
	GovVoteAbstain uint8 = 2
)

type VotingProcedure struct {
	cbor.StructAsArray
	Vote   uint8
	Anchor *GovAnchor
}

type GovAnchor struct {
	cbor.StructAsArray
	Url      string
	DataHash [32]byte
}

type GovActionId struct {
	cbor.StructAsArray
	TransactionId [32]byte
	GovActionIdx  uint32
}

type ProposalProcedure struct {
	cbor.StructAsArray
	Deposit       uint64
	RewardAccount Address
	GovAction     GovActionWrapper
	Anchor        GovAnchor
}

const (
	GovActionTypeParameterChange    = 0
	GovActionTypeHardForkInitiation = 1
	GovActionTypeTreasuryWithdrawal = 2
	GovActionTypeNoConfidence       = 3
	GovActionTypeUpdateCommittee    = 4
	GovActionTypeNewConstitution    = 5
	GovActionTypeInfo               = 6
)

type GovActionWrapper struct {
	Type   uint
	Action GovAction
}

func (g *GovActionWrapper) UnmarshalCBOR(data []byte) error {
	// Determine action type
	actionType, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	var tmpAction GovAction
	switch actionType {
	case GovActionTypeParameterChange:
		tmpAction = &ParameterChangeGovAction{}
	case GovActionTypeHardForkInitiation:
		tmpAction = &HardForkInitiationGovAction{}
	case GovActionTypeTreasuryWithdrawal:
		tmpAction = &TreasuryWithdrawalGovAction{}
	case GovActionTypeNoConfidence:
		tmpAction = &NoConfidenceGovAction{}
	case GovActionTypeUpdateCommittee:
		tmpAction = &UpdateCommitteeGovAction{}
	case GovActionTypeNewConstitution:
		tmpAction = &NewConstitutionGovAction{}
	case GovActionTypeInfo:
		tmpAction = &InfoGovAction{}
	default:
		return fmt.Errorf("unknown governance action type: %d", actionType)
	}
	// Decode action
	if _, err := cbor.Decode(data, tmpAction); err != nil {
		return err
	}
	g.Type = uint(actionType)
	g.Action = tmpAction
	return nil
}

func (g *GovActionWrapper) MarshalCBOR() ([]byte, error) {
	return cbor.Encode(g.Action)
}

type GovAction interface {
	isGovAction()
}

type ParameterChangeGovAction struct {
	cbor.StructAsArray
	Type        uint
	ActionId    *GovActionId
	ParamUpdate BabbageProtocolParameterUpdate // TODO: use Conway params update type
	PolicyHash  []byte
}

func (a ParameterChangeGovAction) isGovAction() {}

type HardForkInitiationGovAction struct {
	cbor.StructAsArray
	Type            uint
	ActionId        *GovActionId
	ProtocolVersion struct {
		cbor.StructAsArray
		Major uint
		Minor uint
	}
}

func (a HardForkInitiationGovAction) isGovAction() {}

type TreasuryWithdrawalGovAction struct {
	cbor.StructAsArray
	Type        uint
	Withdrawals map[*Address]uint64
	PolicyHash  []byte
}

func (a TreasuryWithdrawalGovAction) isGovAction() {}

type NoConfidenceGovAction struct {
	cbor.StructAsArray
	Type     uint
	ActionId *GovActionId
}

func (a NoConfidenceGovAction) isGovAction() {}

type UpdateCommitteeGovAction struct {
	cbor.StructAsArray
	Type        uint
	ActionId    *GovActionId
	Credentials []StakeCredential
	CredEpochs  map[*StakeCredential]uint
	Unknown     cbor.Rat
}

func (a UpdateCommitteeGovAction) isGovAction() {}

type NewConstitutionGovAction struct {
	cbor.StructAsArray
	Type         uint
	ActionId     *GovActionId
	Constitution struct {
		cbor.StructAsArray
		Anchor     GovAnchor
		ScriptHash []byte
	}
}

func (a NewConstitutionGovAction) isGovAction() {}

type InfoGovAction struct {
	cbor.StructAsArray
	Type uint
}

func (a InfoGovAction) isGovAction() {}

type ConwayTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       ConwayTransactionBody
	WitnessSet BabbageTransactionWitnessSet
	IsTxValid  bool
	TxMetadata *cbor.Value
}

func (t ConwayTransaction) Hash() string {
	return t.Body.Hash()
}

func (t ConwayTransaction) Inputs() []TransactionInput {
	return t.Body.Inputs()
}

func (t ConwayTransaction) Outputs() []TransactionOutput {
	return t.Body.Outputs()
}

func (t ConwayTransaction) Fee() uint64 {
	return t.Body.Fee()
}

func (t ConwayTransaction) TTL() uint64 {
	return t.Body.TTL()
}

func (t ConwayTransaction) ReferenceInputs() []TransactionInput {
	return t.Body.ReferenceInputs()
}

func (t ConwayTransaction) Collateral() []TransactionInput {
	return t.Body.Collateral()
}

func (t ConwayTransaction) CollateralReturn() TransactionOutput {
	return t.Body.CollateralReturn()
}

func (t ConwayTransaction) TotalCollateral() uint64 {
	return t.Body.TotalCollateral()
}

func (t ConwayTransaction) Certificates() []Certificate {
	return t.Body.Certificates()
}

func (t ConwayTransaction) Withdrawals() map[*Address]uint64 {
	return t.Body.Withdrawals()
}

func (t ConwayTransaction) RequiredSigners() []Blake2b224 {
	return t.Body.RequiredSigners()
}

func (t ConwayTransaction) AssetMint() *MultiAsset[MultiAssetTypeMint] {
	return t.Body.AssetMint()
}

func (t ConwayTransaction) VotingProcedures() VotingProcedures {
	return t.Body.VotingProcedures()
}

func (t ConwayTransaction) ProposalProcedures() []ProposalProcedure {
	return t.Body.ProposalProcedures()
}

func (t ConwayTransaction) Metadata() *cbor.Value {
	return t.TxMetadata
}

func (t ConwayTransaction) IsValid() bool {
	return t.IsTxValid
}

func (t ConwayTransaction) Consumed() []TransactionInput {
	if t.IsValid() {
		return t.Inputs()
	} else {
		return t.Collateral()
	}
}

func (t ConwayTransaction) Produced() []TransactionOutput {
	if t.IsValid() {
		return t.Outputs()
	} else {
		if t.CollateralReturn() == nil {
			return []TransactionOutput{}
		}
		return []TransactionOutput{t.CollateralReturn()}
	}
}

func (t *ConwayTransaction) ProtocolParametersUpdate() map[Blake2b224]any {
	updateMap := make(map[Blake2b224]any)
	for k, v := range t.Body.Update.ProtocolParamUpdates {
		updateMap[k] = v
	}
	return updateMap
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
