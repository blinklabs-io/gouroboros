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

package byron

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
	EraIdByron   = 0
	EraNameByron = "Byron"

	BlockTypeByronEbb  = 0
	BlockTypeByronMain = 1

	BlockHeaderTypeByron = 0

	TxTypeByron = 0

	ByronSlotsPerEpoch = 21600
)

var EraByron = common.Era{
	Id:   EraIdByron,
	Name: EraNameByron,
}

func init() {
	common.RegisterEra(EraByron)
}

type ByronMainBlockHeader struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash          *common.Blake2b256
	ProtocolMagic uint32
	PrevBlock     common.Blake2b256
	BodyProof     any
	ConsensusData struct {
		cbor.StructAsArray
		// [slotid, pubkey, difficulty, blocksig]
		SlotId struct {
			cbor.StructAsArray
			Epoch uint64
			Slot  uint16
		}
		PubKey     []byte
		Difficulty struct {
			cbor.StructAsArray
			Value uint64
		}
		BlockSig []any
	}
	ExtraData struct {
		cbor.StructAsArray
		BlockVersion    ByronBlockVersion
		SoftwareVersion ByronSoftwareVersion
		Attributes      any
		ExtraProof      common.Blake2b256
	}
}

func (h *ByronMainBlockHeader) UnmarshalCBOR(cborData []byte) error {
	type tByronMainBlockHeader ByronMainBlockHeader
	var tmp tByronMainBlockHeader
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*h = ByronMainBlockHeader(tmp)
	h.SetCbor(cborData)
	return nil
}

func (h *ByronMainBlockHeader) Hash() common.Blake2b256 {
	if h.hash == nil {
		// Prepend bytes for CBOR list wrapper
		// The block hash is calculated with these extra bytes, so we have to add them to
		// get the correct value
		tmpHash := common.Blake2b256Hash(
			append(
				[]byte{0x82, BlockTypeByronMain},
				h.Cbor()...,
			),
		)
		h.hash = &tmpHash
	}
	return *h.hash
}

func (h *ByronMainBlockHeader) PrevHash() common.Blake2b256 {
	return h.PrevBlock
}

func (h *ByronMainBlockHeader) BlockNumber() uint64 {
	return h.ConsensusData.Difficulty.Value
}

func (h *ByronMainBlockHeader) SlotNumber() uint64 {
	return (h.ConsensusData.SlotId.Epoch * ByronSlotsPerEpoch) + uint64(
		h.ConsensusData.SlotId.Slot,
	)
}

func (h *ByronMainBlockHeader) IssuerVkey() common.IssuerVkey {
	// Byron blocks don't have an issuer
	return common.IssuerVkey{}
}

func (h *ByronMainBlockHeader) BlockBodySize() uint64 {
	// Byron doesn't include the block body size in the header
	return 0
}

func (h *ByronMainBlockHeader) Era() common.Era {
	return EraByron
}

type ByronTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash       *common.Blake2b256
	TxInputs   []ByronTransactionInput
	TxOutputs  []ByronTransactionOutput
	Attributes *cbor.LazyValue
}

func (t *ByronTransaction) UnmarshalCBOR(cborData []byte) error {
	type tByronTransaction ByronTransaction
	var tmp tByronTransaction
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*t = ByronTransaction(tmp)
	t.SetCbor(cborData)
	return nil
}

func (ByronTransaction) Type() int {
	return TxTypeByron
}

func (t *ByronTransaction) Hash() common.Blake2b256 {
	if t.hash == nil {
		tmpHash := common.Blake2b256Hash(t.Cbor())
		t.hash = &tmpHash
	}
	return *t.hash
}

func (t *ByronTransaction) Inputs() []common.TransactionInput {
	ret := []common.TransactionInput{}
	for _, input := range t.TxInputs {
		ret = append(ret, input)
	}
	return ret
}

func (t *ByronTransaction) Outputs() []common.TransactionOutput {
	ret := []common.TransactionOutput{}
	for _, output := range t.TxOutputs {
		ret = append(ret, &output)
	}
	return ret
}

func (t *ByronTransaction) Fee() uint64 {
	// The fee is implicit in Byron, and we don't have enough information here to calculate it.
	// You need to know the Lovelace in the inputs to determine the fee, and that information is
	// not provided directly in the TX
	return 0
}

func (t *ByronTransaction) TTL() uint64 {
	// No TTL in Byron
	return 0
}

func (t *ByronTransaction) ValidityIntervalStart() uint64 {
	// No validity interval start in Byron
	return 0
}

func (t *ByronTransaction) ReferenceInputs() []common.TransactionInput {
	// No reference inputs in Byron
	return nil
}

func (t *ByronTransaction) Collateral() []common.TransactionInput {
	// No collateral in Byron
	return nil
}

func (t *ByronTransaction) CollateralReturn() common.TransactionOutput {
	// No collateral in Byron
	return nil
}

func (t *ByronTransaction) TotalCollateral() uint64 {
	// No collateral in Byron
	return 0
}

func (t *ByronTransaction) Certificates() []common.Certificate {
	// No certificates in Byron
	return nil
}

func (t *ByronTransaction) Withdrawals() map[*common.Address]uint64 {
	// No withdrawals in Byron
	return nil
}

func (t *ByronTransaction) AuxDataHash() *common.Blake2b256 {
	// No aux data hash in Byron
	return nil
}

func (t *ByronTransaction) RequiredSigners() []common.Blake2b224 {
	// No required signers in Byron
	return nil
}

func (t *ByronTransaction) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	// No asset mints in Byron
	return nil
}

func (t *ByronTransaction) ScriptDataHash() *common.Blake2b256 {
	// No script data hash in Byron
	return nil
}

func (t *ByronTransaction) VotingProcedures() common.VotingProcedures {
	// No voting procedures in Byron
	return nil
}

func (t *ByronTransaction) ProposalProcedures() []common.ProposalProcedure {
	// No proposal procedures in Byron
	return nil
}

func (t *ByronTransaction) CurrentTreasuryValue() int64 {
	// No current treasury value in Byron
	return 0
}

func (t *ByronTransaction) Donation() uint64 {
	// No donation in Byron
	return 0
}

func (t *ByronTransaction) Metadata() *cbor.LazyValue {
	return t.Attributes
}

func (t *ByronTransaction) IsValid() bool {
	return true
}

func (t *ByronTransaction) Consumed() []common.TransactionInput {
	return t.Inputs()
}

func (t *ByronTransaction) Produced() []common.Utxo {
	ret := []common.Utxo{}
	for idx, output := range t.Outputs() {
		ret = append(
			ret,
			common.Utxo{
				Id:     NewByronTransactionInput(t.Hash().String(), idx),
				Output: output,
			},
		)
	}
	return ret
}

func (t ByronTransaction) Witnesses() common.TransactionWitnessSet {
	// TODO: implement once we properly support decoding Byron TX witnesses (#853)
	return nil
}

func (t *ByronTransaction) Utxorpc() (*utxorpc.Tx, error) {
	return &utxorpc.Tx{}, nil
}

func (t *ByronTransaction) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	// No protocol parameter updates in Byron
	updateMap := make(map[common.Blake2b224]common.ProtocolParameterUpdate)
	return 0, updateMap
}

type ByronTransactionInput struct {
	cbor.StructAsArray
	TxId        common.Blake2b256
	OutputIndex uint32
}

func NewByronTransactionInput(hash string, idx int) ByronTransactionInput {
	tmpHash, err := hex.DecodeString(hash)
	if err != nil {
		panic(fmt.Sprintf("failed to decode transaction hash: %s", err))
	}
	if idx < 0 || idx > math.MaxUint32 {
		panic("index out of range")
	}
	return ByronTransactionInput{
		TxId:        common.Blake2b256(tmpHash),
		OutputIndex: uint32(idx),
	}
}

func (i *ByronTransactionInput) UnmarshalCBOR(data []byte) error {
	id, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	switch id {
	case 0:
		// Decode outer data
		var tmpData struct {
			cbor.StructAsArray
			Id   int
			Cbor []byte
		}
		if _, err := cbor.Decode(data, &tmpData); err != nil {
			return err
		}
		// Decode inner data
		type tByronTransactionInput ByronTransactionInput
		var tmp tByronTransactionInput
		if _, err := cbor.Decode(tmpData.Cbor, &tmp); err != nil {
			return err
		}
		*i = ByronTransactionInput(tmp)
	default:
		// [u8 .ne 0, encoded-cbor]
		return errors.New("can't parse yet")
	}
	return nil
}

func (i ByronTransactionInput) Id() common.Blake2b256 {
	return i.TxId
}

func (i ByronTransactionInput) Index() uint32 {
	return i.OutputIndex
}

func (i ByronTransactionInput) Utxorpc() (*utxorpc.TxInput, error) {
	return &utxorpc.TxInput{
		TxHash:      i.TxId.Bytes(),
		OutputIndex: i.OutputIndex,
		// AsOutput: i.AsOutput,
		// Redeemer: i.Redeemer,
	}, nil
}

func (i ByronTransactionInput) String() string {
	return fmt.Sprintf("%s#%d", i.TxId, i.OutputIndex)
}

func (i ByronTransactionInput) MarshalJSON() ([]byte, error) {
	return []byte("\"" + i.String() + "\""), nil
}

type ByronTransactionOutput struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	OutputAddress common.Address `json:"address"`
	OutputAmount  uint64         `json:"amount"`
}

func (o *ByronTransactionOutput) UnmarshalCBOR(data []byte) error {
	// Save original CBOR
	o.SetCbor(data)
	var tmpData struct {
		cbor.StructAsArray
		WrappedAddress cbor.RawMessage
		Amount         uint64
	}
	if _, err := cbor.Decode(data, &tmpData); err != nil {
		return err
	}
	o.OutputAmount = tmpData.Amount
	if _, err := cbor.Decode(tmpData.WrappedAddress, &o.OutputAddress); err != nil {
		return err
	}
	return nil
}

func (o ByronTransactionOutput) Address() common.Address {
	return o.OutputAddress
}

func (o ByronTransactionOutput) Amount() uint64 {
	return o.OutputAmount
}

func (o ByronTransactionOutput) Assets() *common.MultiAsset[common.MultiAssetTypeOutput] {
	return nil
}

func (o ByronTransactionOutput) DatumHash() *common.Blake2b256 {
	return nil
}

func (o ByronTransactionOutput) Datum() *cbor.LazyValue {
	return nil
}

func (o ByronTransactionOutput) Utxorpc() (*utxorpc.TxOutput, error) {
	addressBytes, err := o.OutputAddress.Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to get address bytes: %w", err)
	}
	return &utxorpc.TxOutput{
			Address: addressBytes,
			Coin:    o.Amount(),
		},
		nil
}

type ByronBlockVersion struct {
	cbor.StructAsArray
	Major   uint16
	Minor   uint16
	Unknown uint8
}

type ByronSoftwareVersion struct {
	cbor.StructAsArray
	Name    string
	Version uint32
}

type ByronUpdatePayload struct {
	cbor.StructAsArray
	Proposals []ByronUpdateProposal
	Votes     []any
}

type ByronUpdateProposal struct {
	cbor.DecodeStoreCbor
	cbor.StructAsArray
	BlockVersion    ByronBlockVersion
	BlockVersionMod ByronUpdateProposalBlockVersionMod
	SoftwareVersion ByronSoftwareVersion
	Data            any
	Attributes      any
	From            []byte
	Signature       []byte
}

func (p *ByronUpdateProposal) UnmarshalCBOR(cborData []byte) error {
	type tByronUpdateProposal ByronUpdateProposal
	var tmp tByronUpdateProposal
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*p = ByronUpdateProposal(tmp)
	p.SetCbor(cborData)
	return nil
}

type ByronUpdateProposalBlockVersionMod struct {
	cbor.StructAsArray
	ScriptVersion     []uint16
	SlotDuration      []uint64
	MaxBlockSize      []uint64
	MaxHeaderSize     []uint64
	MaxTxSize         []uint64
	MaxProposalSize   []uint64
	MpcThd            []uint64
	HeavyDelThd       []uint64
	UpdateVoteThd     []uint64
	UpdateProposalThd []uint64
	UpdateImplicit    []uint64
	SoftForkRule      []any
	TxFeePolicy       []any
	UnlockStakeEpoch  []uint64
}

type ByronMainBlockBody struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	// TODO: split this to its own type (#853)
	TxPayload []struct {
		cbor.StructAsArray
		Transaction ByronTransaction
		// TODO: properly decode TX witnesses (#853)
		Twit []cbor.Value
	}
	SscPayload cbor.Value
	DlgPayload []any
	UpdPayload ByronUpdatePayload
}

func (b *ByronMainBlockBody) UnmarshalCBOR(cborData []byte) error {
	type tByronMainBlockBody ByronMainBlockBody
	var tmp tByronMainBlockBody
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*b = ByronMainBlockBody(tmp)
	b.SetCbor(cborData)
	return nil
}

type ByronEpochBoundaryBlockHeader struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash          *common.Blake2b256
	ProtocolMagic uint32
	PrevBlock     common.Blake2b256
	BodyProof     any
	ConsensusData struct {
		cbor.StructAsArray
		Epoch      uint64
		Difficulty struct {
			cbor.StructAsArray
			Value uint64
		}
	}
	ExtraData any
}

func (h *ByronEpochBoundaryBlockHeader) UnmarshalCBOR(cborData []byte) error {
	type tByronEpochBoundaryBlockHeader ByronEpochBoundaryBlockHeader
	var tmp tByronEpochBoundaryBlockHeader
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*h = ByronEpochBoundaryBlockHeader(tmp)
	h.SetCbor(cborData)
	return nil
}

func (h *ByronEpochBoundaryBlockHeader) Hash() common.Blake2b256 {
	if h.hash == nil {
		// Prepend bytes for CBOR list wrapper
		// The block hash is calculated with these extra bytes, so we have to add them to
		// get the correct value
		tmpHash := common.Blake2b256Hash(
			append(
				[]byte{0x82, BlockTypeByronEbb},
				h.Cbor()...,
			),
		)
		h.hash = &tmpHash
	}
	return *h.hash
}

func (h *ByronEpochBoundaryBlockHeader) PrevHash() common.Blake2b256 {
	return h.PrevBlock
}

func (h *ByronEpochBoundaryBlockHeader) BlockNumber() uint64 {
	return h.ConsensusData.Difficulty.Value
}

func (h *ByronEpochBoundaryBlockHeader) SlotNumber() uint64 {
	return h.ConsensusData.Epoch * ByronSlotsPerEpoch
}

func (h *ByronEpochBoundaryBlockHeader) IssuerVkey() common.IssuerVkey {
	// Byron blocks don't have an issuer
	return common.IssuerVkey([]byte{})
}

func (h *ByronEpochBoundaryBlockHeader) BlockBodySize() uint64 {
	// Byron doesn't include the block body size in the header
	return 0
}

func (h *ByronEpochBoundaryBlockHeader) Era() common.Era {
	return EraByron
}

type ByronMainBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	BlockHeader *ByronMainBlockHeader
	Body        ByronMainBlockBody
	Extra       []any
}

func (b *ByronMainBlock) UnmarshalCBOR(cborData []byte) error {
	type tByronMainBlock ByronMainBlock
	var tmp tByronMainBlock
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*b = ByronMainBlock(tmp)
	b.SetCbor(cborData)
	return nil
}

func (ByronMainBlock) Type() int {
	return BlockTypeByronMain
}

func (b *ByronMainBlock) Hash() common.Blake2b256 {
	return b.BlockHeader.Hash()
}

func (b *ByronMainBlock) Header() common.BlockHeader {
	return b.BlockHeader
}

func (b *ByronMainBlock) PrevHash() common.Blake2b256 {
	return b.BlockHeader.PrevHash()
}

func (b *ByronMainBlock) BlockNumber() uint64 {
	return b.BlockHeader.BlockNumber()
}

func (b *ByronMainBlock) SlotNumber() uint64 {
	return b.BlockHeader.SlotNumber()
}

func (b *ByronMainBlock) IssuerVkey() common.IssuerVkey {
	return b.BlockHeader.IssuerVkey()
}

func (b *ByronMainBlock) BlockBodySize() uint64 {
	return uint64(len(b.Body.Cbor()))
}

func (b *ByronMainBlock) Era() common.Era {
	return b.BlockHeader.Era()
}

func (b *ByronMainBlock) Transactions() []common.Transaction {
	ret := make([]common.Transaction, len(b.Body.TxPayload))
	for idx, payload := range b.Body.TxPayload {
		ret[idx] = &payload.Transaction
	}
	return ret
}

func (b *ByronMainBlock) Utxorpc() (*utxorpc.Block, error) {
	return &utxorpc.Block{}, nil
}

type ByronEpochBoundaryBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	BlockHeader *ByronEpochBoundaryBlockHeader
	Body        []common.Blake2b224
	Extra       []any
}

func (b *ByronEpochBoundaryBlock) UnmarshalCBOR(cborData []byte) error {
	type tByronEpochBoundaryBlock ByronEpochBoundaryBlock
	var tmp tByronEpochBoundaryBlock
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*b = ByronEpochBoundaryBlock(tmp)
	b.SetCbor(cborData)
	return nil
}

func (ByronEpochBoundaryBlock) Type() int {
	return BlockTypeByronEbb
}

func (b *ByronEpochBoundaryBlock) Hash() common.Blake2b256 {
	return b.BlockHeader.Hash()
}

func (b *ByronEpochBoundaryBlock) Header() common.BlockHeader {
	return b.BlockHeader
}

func (b *ByronEpochBoundaryBlock) PrevHash() common.Blake2b256 {
	return b.BlockHeader.PrevHash()
}

func (b *ByronEpochBoundaryBlock) BlockNumber() uint64 {
	return b.BlockHeader.BlockNumber()
}

func (b *ByronEpochBoundaryBlock) SlotNumber() uint64 {
	return b.BlockHeader.SlotNumber()
}

func (b *ByronEpochBoundaryBlock) IssuerVkey() common.IssuerVkey {
	return b.BlockHeader.IssuerVkey()
}

func (b *ByronEpochBoundaryBlock) BlockBodySize() uint64 {
	// There's not really a body for an epoch boundary block
	return 0
}

func (b *ByronEpochBoundaryBlock) Era() common.Era {
	return b.BlockHeader.Era()
}

func (b *ByronEpochBoundaryBlock) Transactions() []common.Transaction {
	// Boundary blocks don't have transactions
	return nil
}

func (b *ByronEpochBoundaryBlock) Utxorpc() (*utxorpc.Block, error) {
	return &utxorpc.Block{}, nil
}

func NewByronEpochBoundaryBlockFromCbor(
	data []byte,
) (*ByronEpochBoundaryBlock, error) {
	var byronEbbBlock ByronEpochBoundaryBlock
	if _, err := cbor.Decode(data, &byronEbbBlock); err != nil {
		return nil, fmt.Errorf("decode Byron EBB block error: %w", err)
	}
	return &byronEbbBlock, nil
}

func NewByronEpochBoundaryBlockHeaderFromCbor(
	data []byte,
) (*ByronEpochBoundaryBlockHeader, error) {
	var byronEbbBlockHeader ByronEpochBoundaryBlockHeader
	if _, err := cbor.Decode(data, &byronEbbBlockHeader); err != nil {
		return nil, fmt.Errorf("decode Byron EBB block header error: %w", err)
	}
	return &byronEbbBlockHeader, nil
}

func NewByronMainBlockFromCbor(data []byte) (*ByronMainBlock, error) {
	var byronMainBlock ByronMainBlock
	if _, err := cbor.Decode(data, &byronMainBlock); err != nil {
		return nil, fmt.Errorf("decode Byron main block error: %w", err)
	}
	return &byronMainBlock, nil
}

func NewByronMainBlockHeaderFromCbor(
	data []byte,
) (*ByronMainBlockHeader, error) {
	var byronMainBlockHeader ByronMainBlockHeader
	if _, err := cbor.Decode(data, &byronMainBlockHeader); err != nil {
		return nil, fmt.Errorf("decode Byron main block header error: %w", err)
	}
	return &byronMainBlockHeader, nil
}

func NewByronTransactionFromCbor(data []byte) (*ByronTransaction, error) {
	var byronTx ByronTransaction
	if _, err := cbor.Decode(data, &byronTx); err != nil {
		return nil, fmt.Errorf("decode Byron transaction error: %w", err)
	}
	return &byronTx, nil
}

func NewByronTransactionOutputFromCbor(
	data []byte,
) (*ByronTransactionOutput, error) {
	var byronTxOutput ByronTransactionOutput
	if _, err := cbor.Decode(data, &byronTxOutput); err != nil {
		return nil, fmt.Errorf("decode Byron transaction output error: %w", err)
	}
	return &byronTxOutput, nil
}
