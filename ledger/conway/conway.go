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
	"iter"
	"maps"
	"slices"

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
	TransactionMetadataSet common.TransactionMetadataSet
	InvalidTransactions    []uint
}

func (b *ConwayBlock) UnmarshalCBOR(cborData []byte) error {
	// Create a temporary struct for unmarshaling with []any for InvalidTransactions
	type tmpConwayBlock struct {
		cbor.StructAsArray
		BlockHeader            *ConwayBlockHeader
		TransactionBodies      []ConwayTransactionBody
		TransactionWitnessSets []ConwayTransactionWitnessSet
		TransactionMetadataSet common.TransactionMetadataSet
		InvalidTransactions    []any
	}

	var tmp tmpConwayBlock
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}

	// Convert []any to []uint with validation
	const maxReasonableIndex = 1000000 // Reasonable upper bound for transaction indices
	result := make([]uint, 0, len(tmp.InvalidTransactions))
	for _, v := range tmp.InvalidTransactions {
		switch val := v.(type) {
		case uint:
			if val > maxReasonableIndex {
				continue // Skip unreasonably large indices
			}
			result = append(result, val)
		case uint64:
			if val > maxReasonableIndex {
				continue // Skip unreasonably large indices
			}
			result = append(result, uint(val))
		case int:
			if val < 0 || val > maxReasonableIndex {
				continue // Skip negative or unreasonably large indices
			}
			result = append(result, uint(val))
		case int64:
			if val < 0 || val > maxReasonableIndex {
				continue // Skip negative or unreasonably large indices
			}
			result = append(result, uint(val))
		default:
			// Skip invalid types (strings, floats, etc.)
			continue
		}
	}
	if len(result) == 0 {
		b.InvalidTransactions = nil
	} else {
		b.InvalidTransactions = result
	}

	// Assign the other fields
	b.BlockHeader = tmp.BlockHeader
	b.TransactionBodies = tmp.TransactionBodies
	b.TransactionWitnessSets = tmp.TransactionWitnessSets
	b.TransactionMetadataSet = tmp.TransactionMetadataSet

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

func (b *ConwayBlock) MarshalCBOR() ([]byte, error) {
	// Return stored CBOR if available
	if b.Cbor() != nil {
		return b.Cbor(), nil
	}

	// Ensure InvalidTransactions is encoded as empty array if nil
	invalidTxs := b.InvalidTransactions
	if invalidTxs == nil {
		invalidTxs = []uint{}
	}

	return cbor.Encode([]any{
		b.BlockHeader,
		b.TransactionBodies,
		b.TransactionWitnessSets,
		b.TransactionMetadataSet,
		invalidTxs,
	})
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
		tx := &ConwayTransaction{
			Body:       b.TransactionBodies[idx],
			WitnessSet: b.TransactionWitnessSets[idx],
			TxIsValid:  !invalidTxMap[uint(idx)],
		}
		if metadata, ok := b.TransactionMetadataSet.GetMetadata(uint(idx)); ok {
			tx.TxMetadata = metadata
		}
		ret[idx] = tx
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

func (b *ConwayBlock) BlockBodyHash() common.Blake2b256 {
	return b.Header().BlockBodyHash()
}

type ConwayBlockHeader struct {
	babbage.BabbageBlockHeader
}

func (h *ConwayBlockHeader) Era() common.Era {
	return EraConway
}

type ConwayRedeemers struct {
	Redeemers       map[common.RedeemerKey]common.RedeemerValue
	legacyRedeemers alonzo.AlonzoRedeemers
	legacy          bool
}

func (r *ConwayRedeemers) UnmarshalCBOR(cborData []byte) error {
	// Try to parse as legacy redeemer first
	if _, err := cbor.Decode(cborData, &(r.legacyRedeemers)); err == nil {
		r.legacy = true
	} else {
		_, err := cbor.Decode(cborData, &(r.Redeemers))
		return err
	}
	return nil
}

func (r *ConwayRedeemers) MarshalCBOR() ([]byte, error) {
	if r.legacy {
		return cbor.Encode(r.legacyRedeemers)
	}
	return cbor.Encode(r.Redeemers)
}

func (r ConwayRedeemers) Iter() iter.Seq2[common.RedeemerKey, common.RedeemerValue] {
	return func(yield func(common.RedeemerKey, common.RedeemerValue) bool) {
		// Sort redeemers
		sorted := slices.Collect(maps.Keys(r.Redeemers))
		slices.SortFunc(
			sorted,
			func(a, b common.RedeemerKey) int {
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
		for _, redeemerKey := range sorted {
			tmpVal := r.Redeemers[redeemerKey]
			if !yield(redeemerKey, tmpVal) {
				return
			}
		}
	}
}

func (r ConwayRedeemers) Indexes(tag common.RedeemerTag) []uint {
	if r.legacy {
		return r.legacyRedeemers.Indexes(tag)
	}
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
) common.RedeemerValue {
	if r.legacy {
		return r.legacyRedeemers.Value(index, tag)
	}
	redeemerVal, ok := r.Redeemers[common.RedeemerKey{
		Tag:   tag,
		Index: uint32(index), // #nosec G115
	}]
	if ok {
		return redeemerVal
	}
	return common.RedeemerValue{}
}

type ConwayTransactionWitnessSet struct {
	cbor.DecodeStoreCbor
	VkeyWitnesses      cbor.SetType[common.VkeyWitness]      `cbor:"0,keyasint,omitempty,omitzero"`
	WsNativeScripts    cbor.SetType[common.NativeScript]     `cbor:"1,keyasint,omitempty,omitzero"`
	BootstrapWitnesses cbor.SetType[common.BootstrapWitness] `cbor:"2,keyasint,omitempty,omitzero"`
	WsPlutusV1Scripts  cbor.SetType[common.PlutusV1Script]   `cbor:"3,keyasint,omitempty,omitzero"`
	WsPlutusData       cbor.SetType[common.Datum]            `cbor:"4,keyasint,omitempty,omitzero"`
	WsRedeemers        ConwayRedeemers                       `cbor:"5,keyasint,omitempty,omitzero"`
	WsPlutusV2Scripts  cbor.SetType[common.PlutusV2Script]   `cbor:"6,keyasint,omitempty,omitzero"`
	WsPlutusV3Scripts  cbor.SetType[common.PlutusV3Script]   `cbor:"7,keyasint,omitempty,omitzero"`
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
	return w.VkeyWitnesses.Items()
}

func (w ConwayTransactionWitnessSet) Bootstrap() []common.BootstrapWitness {
	return w.BootstrapWitnesses.Items()
}

func (w ConwayTransactionWitnessSet) NativeScripts() []common.NativeScript {
	return w.WsNativeScripts.Items()
}

func (w ConwayTransactionWitnessSet) PlutusV1Scripts() []common.PlutusV1Script {
	return w.WsPlutusV1Scripts.Items()
}

func (w ConwayTransactionWitnessSet) PlutusV2Scripts() []common.PlutusV2Script {
	return w.WsPlutusV2Scripts.Items()
}

func (w ConwayTransactionWitnessSet) PlutusV3Scripts() []common.PlutusV3Script {
	return w.WsPlutusV3Scripts.Items()
}

func (w ConwayTransactionWitnessSet) PlutusData() []common.Datum {
	return w.WsPlutusData.Items()
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
	TxProposalProcedures    []ConwayProposalProcedure                     `cbor:"20,keyasint,omitempty"`
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
	ret := make([]common.TransactionOutput, len(b.TxOutputs))
	for i := range b.TxOutputs {
		ret[i] = &b.TxOutputs[i]
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
	items := b.TxReferenceInputs.Items()
	ret := make([]common.TransactionInput, len(items))
	for i := range items {
		ret[i] = &items[i]
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
	ret := make([]common.ProposalProcedure, len(b.TxProposalProcedures))
	for i, item := range b.TxProposalProcedures {
		ret[i] = item
	}
	return ret
}

func (b *ConwayTransactionBody) CurrentTreasuryValue() int64 {
	return b.TxCurrentTreasuryValue
}

func (b *ConwayTransactionBody) Donation() uint64 {
	return b.TxDonation
}

func (b *ConwayTransactionBody) Utxorpc() (*utxorpc.Tx, error) {
	return common.TransactionBodyToUtxorpc(b)
}

type ConwayTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash       *common.Blake2b256
	Body       ConwayTransactionBody
	WitnessSet ConwayTransactionWitnessSet
	TxIsValid  bool
	TxMetadata common.TransactionMetadatum
}

func (t *ConwayTransaction) UnmarshalCBOR(cborData []byte) error {
	// Decode as raw array to preserve metadata bytes
	var txArray []cbor.RawMessage
	if _, err := cbor.Decode(cborData, &txArray); err != nil {
		return err
	}

	// Ensure we have exactly 4 components (body, witness, isValid, metadata)
	if len(txArray) != 4 {
		return fmt.Errorf(
			"invalid transaction: expected exactly 4 components, got %d",
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

	// Decode TxIsValid flag
	if _, err := cbor.Decode([]byte(txArray[2]), &t.TxIsValid); err != nil {
		return fmt.Errorf("failed to decode TxIsValid: %w", err)
	}

	// Handle metadata (component 4, always present - either data or CBOR nil)
	// DecodeAuxiliaryDataToMetadata already preserves raw bytes via DecodeMetadatumRaw
	if len(txArray) > 3 && len(txArray[3]) > 0 {
		metadata, err := common.DecodeAuxiliaryDataToMetadata(txArray[3])
		if err == nil && metadata != nil {
			t.TxMetadata = metadata
		}
	}

	t.SetCbor(cborData)
	return nil
}

func (t *ConwayTransaction) Metadata() common.TransactionMetadatum {
	return t.TxMetadata
}

func (ConwayTransaction) Type() int {
	return TxTypeConway
}

func (t ConwayTransaction) Hash() common.Blake2b256 {
	return t.Id()
}

func (t ConwayTransaction) Id() common.Blake2b256 {
	return t.Body.Id()
}

func (t ConwayTransaction) LeiosHash() common.Blake2b256 {
	if t.hash == nil {
		tmpHash := common.Blake2b256Hash(t.Cbor())
		t.hash = &tmpHash
	}
	return *t.hash
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

func (t *ConwayTransaction) MarshalCBOR() ([]byte, error) {
	// If we have stored CBOR (from decode), return it to preserve metadata bytes
	cborData := t.DecodeStoreCbor.Cbor()
	if cborData != nil {
		return cborData, nil
	}
	// Otherwise, construct and encode
	tmpObj := []any{
		t.Body,
		t.WitnessSet,
		t.TxIsValid,
	}
	if t.TxMetadata != nil {
		tmpObj = append(tmpObj, cbor.RawMessage(t.TxMetadata.Cbor()))
	} else {
		tmpObj = append(tmpObj, nil)
	}
	return cbor.Encode(tmpObj)
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
	// Delegate to MarshalCBOR which handles encoding
	cborData, err := t.MarshalCBOR()
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

func NewConwayBlockFromCbor(
	data []byte,
	config ...common.VerifyConfig,
) (*ConwayBlock, error) {
	var cfg common.VerifyConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	// Default: validation enabled (SkipBodyHashValidation = false)

	var conwayBlock ConwayBlock
	if _, err := cbor.Decode(data, &conwayBlock); err != nil {
		return nil, fmt.Errorf("decode Conway block error: %w", err)
	}

	// Validate body hash during parsing if not skipped
	if !cfg.SkipBodyHashValidation {
		if conwayBlock.BlockHeader == nil {
			return nil, errors.New("conway block header is nil")
		}
		if err := common.ValidateBlockBodyHash(
			data,
			conwayBlock.BlockHeader.BlockBodyHash(),
			EraNameConway,
			5, // Conway has 5 elements: header, txs, witnesses, aux, invalid
		); err != nil {
			return nil, err
		}
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
