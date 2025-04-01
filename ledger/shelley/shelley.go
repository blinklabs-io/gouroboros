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

package shelley

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

const (
	EraIdShelley   = 1
	EraNameShelley = "Shelley"

	BlockTypeShelley = 2

	BlockHeaderTypeShelley = 1

	TxTypeShelley = 1
)

var EraShelley = common.Era{
	Id:   EraIdShelley,
	Name: EraNameShelley,
}

func init() {
	common.RegisterEra(EraShelley)
}

type ShelleyBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	BlockHeader            *ShelleyBlockHeader
	TransactionBodies      []ShelleyTransactionBody
	TransactionWitnessSets []ShelleyTransactionWitnessSet
	TransactionMetadataSet map[uint]*cbor.LazyValue
}

func (b *ShelleyBlock) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (ShelleyBlock) Type() int {
	return BlockTypeShelley
}

func (b *ShelleyBlock) Hash() common.Blake2b256 {
	return b.BlockHeader.Hash()
}

func (b *ShelleyBlock) Header() common.BlockHeader {
	return b.BlockHeader
}

func (b *ShelleyBlock) PrevHash() common.Blake2b256 {
	return b.BlockHeader.PrevHash()
}

func (b *ShelleyBlock) BlockNumber() uint64 {
	return b.BlockHeader.BlockNumber()
}

func (b *ShelleyBlock) SlotNumber() uint64 {
	return b.BlockHeader.SlotNumber()
}

func (b *ShelleyBlock) IssuerVkey() common.IssuerVkey {
	return b.BlockHeader.IssuerVkey()
}

func (b *ShelleyBlock) BlockBodySize() uint64 {
	return b.BlockHeader.BlockBodySize()
}

func (b *ShelleyBlock) Era() common.Era {
	return EraShelley
}

func (b *ShelleyBlock) Transactions() []common.Transaction {
	ret := make([]common.Transaction, len(b.TransactionBodies))
	// #nosec G115
	for idx := range b.TransactionBodies {
		ret[idx] = &ShelleyTransaction{
			Body:       b.TransactionBodies[idx],
			WitnessSet: b.TransactionWitnessSets[idx],
			TxMetadata: b.TransactionMetadataSet[uint(idx)],
		}
	}
	return ret
}

func (b *ShelleyBlock) Utxorpc() *utxorpc.Block {
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

type ShelleyBlockHeader struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash      *common.Blake2b256
	Body      ShelleyBlockHeaderBody
	Signature []byte
}
type ShelleyBlockHeaderBody struct {
	cbor.StructAsArray
	BlockNumber          uint64
	Slot                 uint64
	PrevHash             common.Blake2b256
	IssuerVkey           common.IssuerVkey
	VrfKey               []byte
	NonceVrf             common.VrfResult
	LeaderVrf            common.VrfResult
	BlockBodySize        uint64
	BlockBodyHash        common.Blake2b256
	OpCertHotVkey        []byte
	OpCertSequenceNumber uint32
	OpCertKesPeriod      uint32
	OpCertSignature      []byte
	ProtoMajorVersion    uint64
	ProtoMinorVersion    uint64
}

func (h *ShelleyBlockHeader) UnmarshalCBOR(cborData []byte) error {
	return h.UnmarshalCbor(cborData, h)
}

func (h *ShelleyBlockHeader) Hash() common.Blake2b256 {
	if h.hash == nil {
		tmpHash := common.Blake2b256Hash(h.Cbor())
		h.hash = &tmpHash
	}
	return *h.hash
}

func (h *ShelleyBlockHeader) PrevHash() common.Blake2b256 {
	return h.Body.PrevHash
}

func (h *ShelleyBlockHeader) BlockNumber() uint64 {
	return h.Body.BlockNumber
}

func (h *ShelleyBlockHeader) SlotNumber() uint64 {
	return h.Body.Slot
}

func (h *ShelleyBlockHeader) IssuerVkey() common.IssuerVkey {
	return h.Body.IssuerVkey
}

func (h *ShelleyBlockHeader) BlockBodySize() uint64 {
	return h.Body.BlockBodySize
}

func (h *ShelleyBlockHeader) Era() common.Era {
	return EraShelley
}

type ShelleyTransactionBody struct {
	cbor.DecodeStoreCbor
	hash           *common.Blake2b256
	TxInputs       ShelleyTransactionInputSet  `cbor:"0,keyasint,omitempty"`
	TxOutputs      []ShelleyTransactionOutput  `cbor:"1,keyasint,omitempty"`
	TxFee          uint64                      `cbor:"2,keyasint,omitempty"`
	Ttl            uint64                      `cbor:"3,keyasint,omitempty"`
	TxCertificates []common.CertificateWrapper `cbor:"4,keyasint,omitempty"`
	TxWithdrawals  map[*common.Address]uint64  `cbor:"5,keyasint,omitempty"`
	Update         struct {
		cbor.StructAsArray
		ProtocolParamUpdates map[common.Blake2b224]ShelleyProtocolParameterUpdate
		Epoch                uint64
	} `cbor:"6,keyasint,omitempty"`
	TxAuxDataHash *common.Blake2b256 `cbor:"7,keyasint,omitempty"`
}

func (b *ShelleyTransactionBody) UnmarshalCBOR(cborData []byte) error {
	return b.UnmarshalCbor(cborData, b)
}

func (b *ShelleyTransactionBody) Hash() common.Blake2b256 {
	if b.hash == nil {
		tmpHash := common.Blake2b256Hash(b.Cbor())
		b.hash = &tmpHash
	}
	return *b.hash
}

func (b *ShelleyTransactionBody) Inputs() []common.TransactionInput {
	ret := []common.TransactionInput{}
	for _, input := range b.TxInputs.Items() {
		ret = append(ret, input)
	}
	return ret
}

func (b *ShelleyTransactionBody) Outputs() []common.TransactionOutput {
	ret := []common.TransactionOutput{}
	for _, output := range b.TxOutputs {
		ret = append(ret, &output)
	}
	return ret
}

func (b *ShelleyTransactionBody) Fee() uint64 {
	return b.TxFee
}

func (b *ShelleyTransactionBody) TTL() uint64 {
	return b.Ttl
}

func (b *ShelleyTransactionBody) ValidityIntervalStart() uint64 {
	// No validity interval start in Shelley
	return 0
}

func (b *ShelleyTransactionBody) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	updateMap := make(map[common.Blake2b224]common.ProtocolParameterUpdate)
	for k, v := range b.Update.ProtocolParamUpdates {
		updateMap[k] = v
	}
	return b.Update.Epoch, updateMap
}

func (b *ShelleyTransactionBody) ReferenceInputs() []common.TransactionInput {
	return []common.TransactionInput{}
}

func (b *ShelleyTransactionBody) Collateral() []common.TransactionInput {
	// No collateral in Shelley
	return nil
}

func (b *ShelleyTransactionBody) CollateralReturn() common.TransactionOutput {
	// No collateral in Shelley
	return nil
}

func (b *ShelleyTransactionBody) TotalCollateral() uint64 {
	// No collateral in Shelley
	return 0
}

func (b *ShelleyTransactionBody) Certificates() []common.Certificate {
	ret := make([]common.Certificate, len(b.TxCertificates))
	for i, cert := range b.TxCertificates {
		ret[i] = cert.Certificate
	}
	return ret
}

func (b *ShelleyTransactionBody) Withdrawals() map[*common.Address]uint64 {
	return b.TxWithdrawals
}

func (b *ShelleyTransactionBody) AuxDataHash() *common.Blake2b256 {
	return b.TxAuxDataHash
}

func (b *ShelleyTransactionBody) RequiredSigners() []common.Blake2b224 {
	// No required signers in Shelley
	return nil
}

func (b *ShelleyTransactionBody) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	// No asset minting in Shelley
	return nil
}

func (b *ShelleyTransactionBody) ScriptDataHash() *common.Blake2b256 {
	// No script data hash in Shelley
	return nil
}

func (b *ShelleyTransactionBody) VotingProcedures() common.VotingProcedures {
	// No voting procedures in Shelley
	return nil
}

func (b *ShelleyTransactionBody) ProposalProcedures() []common.ProposalProcedure {
	// No proposal procedures in Shelley
	return nil
}

func (b *ShelleyTransactionBody) CurrentTreasuryValue() int64 {
	// No current treasury value in Shelley
	return 0
}

func (b *ShelleyTransactionBody) Donation() uint64 {
	// No donation in Shelley
	return 0
}

func (b *ShelleyTransactionBody) Utxorpc() *utxorpc.Tx {
	txi := []*utxorpc.TxInput{}
	txo := []*utxorpc.TxOutput{}
	for _, i := range b.Inputs() {
		input := i.Utxorpc()
		txi = append(txi, input)
	}
	for _, o := range b.Outputs() {
		output := o.Utxorpc()
		txo = append(txo, output)
	}
	tx := &utxorpc.Tx{
		Inputs:  txi,
		Outputs: txo,
		// Certificates: b.Certificates(),
		// Withdrawals:  b.Withdrawals(),
		// Witnesses:    b.Witnesses(),
		Fee: b.Fee(),
		// Validity:     b.Validity(),
		// Successful:   b.Successful(),
		// Auxiliary:    b.AuxData(),
		Hash: b.Hash().Bytes(),
	}
	return tx
}

type ShelleyTransactionInputSet struct {
	items []ShelleyTransactionInput
}

func NewShelleyTransactionInputSet(
	items []ShelleyTransactionInput,
) ShelleyTransactionInputSet {
	s := ShelleyTransactionInputSet{
		items: items,
	}
	return s
}

func (s *ShelleyTransactionInputSet) UnmarshalCBOR(data []byte) error {
	// Make sure this isn't a tag-wrapped set
	// This is needed to prevent Conway+ TXs from being decoded as an earlier type
	var tmpTag cbor.RawTag
	if _, err := cbor.Decode(data, &tmpTag); err == nil {
		return errors.New("did not expect CBOR tag")
	}
	var tmpData []ShelleyTransactionInput
	if _, err := cbor.Decode(data, &tmpData); err != nil {
		return err
	}
	s.items = tmpData
	return nil
}

func (s *ShelleyTransactionInputSet) Items() []ShelleyTransactionInput {
	return s.items
}

func (s *ShelleyTransactionInputSet) SetItems(items []ShelleyTransactionInput) {
	s.items = make([]ShelleyTransactionInput, len(items))
	copy(s.items, items)
}

type ShelleyTransactionInput struct {
	cbor.StructAsArray
	TxId        common.Blake2b256
	OutputIndex uint32
}

func NewShelleyTransactionInput(hash string, idx int) ShelleyTransactionInput {
	tmpHash, err := hex.DecodeString(hash)
	if err != nil {
		panic(fmt.Sprintf("failed to decode transaction hash: %s", err))
	}
	if idx < 0 || idx > math.MaxUint32 {
		panic("index out of range")
	}
	return ShelleyTransactionInput{
		TxId:        common.Blake2b256(tmpHash),
		OutputIndex: uint32(idx),
	}
}

func (i ShelleyTransactionInput) Id() common.Blake2b256 {
	return i.TxId
}

func (i ShelleyTransactionInput) Index() uint32 {
	return i.OutputIndex
}

func (i ShelleyTransactionInput) Utxorpc() *utxorpc.TxInput {
	return &utxorpc.TxInput{
		TxHash:      i.TxId.Bytes(),
		OutputIndex: i.OutputIndex,
	}
}

func (i ShelleyTransactionInput) String() string {
	return fmt.Sprintf("%s#%d", i.TxId, i.OutputIndex)
}

func (i ShelleyTransactionInput) MarshalJSON() ([]byte, error) {
	return []byte("\"" + i.String() + "\""), nil
}

type ShelleyTransactionOutput struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	OutputAddress common.Address `json:"address"`
	OutputAmount  uint64         `json:"amount"`
}

func (o *ShelleyTransactionOutput) UnmarshalCBOR(data []byte) error {
	return o.UnmarshalCbor(data, o)
}

func (o ShelleyTransactionOutput) Address() common.Address {
	return o.OutputAddress
}

func (o ShelleyTransactionOutput) Amount() uint64 {
	return o.OutputAmount
}

func (o ShelleyTransactionOutput) Assets() *common.MultiAsset[common.MultiAssetTypeOutput] {
	return nil
}

func (o ShelleyTransactionOutput) DatumHash() *common.Blake2b256 {
	return nil
}

func (o ShelleyTransactionOutput) Datum() *cbor.LazyValue {
	return nil
}

func (o ShelleyTransactionOutput) Utxorpc() *utxorpc.TxOutput {
	return &utxorpc.TxOutput{
		Address: o.OutputAddress.Bytes(),
		Coin:    o.Amount(),
	}
}

type ShelleyTransactionWitnessSet struct {
	cbor.DecodeStoreCbor
	VkeyWitnesses      []common.VkeyWitness      `cbor:"0,keyasint,omitempty"`
	WsNativeScripts    []common.NativeScript     `cbor:"1,keyasint,omitempty"`
	BootstrapWitnesses []common.BootstrapWitness `cbor:"2,keyasint,omitempty"`
}

func (t *ShelleyTransactionWitnessSet) UnmarshalCBOR(cborData []byte) error {
	return t.UnmarshalCbor(cborData, t)
}

func (w ShelleyTransactionWitnessSet) Vkey() []common.VkeyWitness {
	return w.VkeyWitnesses
}

func (w ShelleyTransactionWitnessSet) Bootstrap() []common.BootstrapWitness {
	return w.BootstrapWitnesses
}

func (w ShelleyTransactionWitnessSet) NativeScripts() []common.NativeScript {
	return w.WsNativeScripts
}

func (w ShelleyTransactionWitnessSet) PlutusData() []cbor.Value {
	// No plutus data in Shelley
	return nil
}

func (w ShelleyTransactionWitnessSet) PlutusV1Scripts() [][]byte {
	// No plutus v1 scripts in Shelley
	return nil
}

func (w ShelleyTransactionWitnessSet) PlutusV2Scripts() [][]byte {
	// No plutus v2 scripts in Shelley
	return nil
}

func (w ShelleyTransactionWitnessSet) PlutusV3Scripts() [][]byte {
	// No plutus v3 scripts in Shelley
	return nil
}

func (w ShelleyTransactionWitnessSet) Redeemers() common.TransactionWitnessRedeemers {
	// No redeemers in Shelley
	return nil
}

type ShelleyTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       ShelleyTransactionBody
	WitnessSet ShelleyTransactionWitnessSet
	TxMetadata *cbor.LazyValue
}

func (t *ShelleyTransaction) UnmarshalCBOR(data []byte) error {
	return t.UnmarshalCbor(data, t)
}

func (ShelleyTransaction) Type() int {
	return TxTypeShelley
}

func (t ShelleyTransaction) Hash() common.Blake2b256 {
	return t.Body.Hash()
}

func (t ShelleyTransaction) Inputs() []common.TransactionInput {
	return t.Body.Inputs()
}

func (t ShelleyTransaction) Outputs() []common.TransactionOutput {
	return t.Body.Outputs()
}

func (t ShelleyTransaction) Fee() uint64 {
	return t.Body.Fee()
}

func (t ShelleyTransaction) TTL() uint64 {
	return t.Body.TTL()
}

func (t ShelleyTransaction) ValidityIntervalStart() uint64 {
	return t.Body.ValidityIntervalStart()
}

func (t ShelleyTransaction) ReferenceInputs() []common.TransactionInput {
	return t.Body.ReferenceInputs()
}

func (t ShelleyTransaction) Collateral() []common.TransactionInput {
	return t.Body.Collateral()
}

func (t ShelleyTransaction) CollateralReturn() common.TransactionOutput {
	return t.Body.CollateralReturn()
}

func (t ShelleyTransaction) TotalCollateral() uint64 {
	return t.Body.TotalCollateral()
}

func (t ShelleyTransaction) Certificates() []common.Certificate {
	return t.Body.Certificates()
}

func (t ShelleyTransaction) Withdrawals() map[*common.Address]uint64 {
	return t.Body.Withdrawals()
}

func (t ShelleyTransaction) AuxDataHash() *common.Blake2b256 {
	return t.Body.AuxDataHash()
}

func (t ShelleyTransaction) RequiredSigners() []common.Blake2b224 {
	return t.Body.RequiredSigners()
}

func (t ShelleyTransaction) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return t.Body.AssetMint()
}

func (t ShelleyTransaction) ScriptDataHash() *common.Blake2b256 {
	return t.Body.ScriptDataHash()
}

func (t ShelleyTransaction) VotingProcedures() common.VotingProcedures {
	return t.Body.VotingProcedures()
}

func (t ShelleyTransaction) ProposalProcedures() []common.ProposalProcedure {
	return t.Body.ProposalProcedures()
}

func (t ShelleyTransaction) CurrentTreasuryValue() int64 {
	return t.Body.CurrentTreasuryValue()
}

func (t ShelleyTransaction) Donation() uint64 {
	return t.Body.Donation()
}

func (t ShelleyTransaction) Metadata() *cbor.LazyValue {
	return t.TxMetadata
}

func (t ShelleyTransaction) IsValid() bool {
	return true
}

func (t ShelleyTransaction) Consumed() []common.TransactionInput {
	return t.Inputs()
}

func (t ShelleyTransaction) Produced() []common.Utxo {
	ret := []common.Utxo{}
	for idx, output := range t.Outputs() {
		ret = append(
			ret,
			common.Utxo{
				Id:     NewShelleyTransactionInput(t.Hash().String(), idx),
				Output: output,
			},
		)
	}
	return ret
}

func (t ShelleyTransaction) Witnesses() common.TransactionWitnessSet {
	return t.WitnessSet
}

func (t ShelleyTransaction) Utxorpc() *utxorpc.Tx {
	return t.Body.Utxorpc()
}

func (t *ShelleyTransaction) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	return t.Body.ProtocolParameterUpdates()
}

func (t *ShelleyTransaction) Cbor() []byte {
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

func NewShelleyBlockFromCbor(data []byte) (*ShelleyBlock, error) {
	var shelleyBlock ShelleyBlock
	if _, err := cbor.Decode(data, &shelleyBlock); err != nil {
		return nil, fmt.Errorf("Shelley block decode error: %w", err)
	}
	return &shelleyBlock, nil
}

func NewShelleyBlockHeaderFromCbor(data []byte) (*ShelleyBlockHeader, error) {
	var shelleyBlockHeader ShelleyBlockHeader
	if _, err := cbor.Decode(data, &shelleyBlockHeader); err != nil {
		return nil, fmt.Errorf("Shelley block header decode error: %w", err)
	}
	return &shelleyBlockHeader, nil
}

func NewShelleyTransactionBodyFromCbor(
	data []byte,
) (*ShelleyTransactionBody, error) {
	var shelleyTx ShelleyTransactionBody
	if _, err := cbor.Decode(data, &shelleyTx); err != nil {
		return nil, fmt.Errorf("Shelley transaction body decode error: %w", err)
	}
	return &shelleyTx, nil
}

func NewShelleyTransactionFromCbor(data []byte) (*ShelleyTransaction, error) {
	var shelleyTx ShelleyTransaction
	if _, err := cbor.Decode(data, &shelleyTx); err != nil {
		return nil, fmt.Errorf("Shelley transaction decode error: %w", err)
	}
	return &shelleyTx, nil
}

func NewShelleyTransactionOutputFromCbor(
	data []byte,
) (*ShelleyTransactionOutput, error) {
	var shelleyTxOutput ShelleyTransactionOutput
	if _, err := cbor.Decode(data, &shelleyTxOutput); err != nil {
		return nil, fmt.Errorf(
			"Shelley transaction output decode error: %w",
			err,
		)
	}
	return &shelleyTxOutput, nil
}
