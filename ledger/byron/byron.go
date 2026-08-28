// Copyright 2026 Blink Labs Software
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
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/data"
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

	// Protocol magic values for different networks
	MainnetProtocolMagic = 764824073
	TestnetProtocolMagic = 1097911063
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

// BlockBodyHashChecked returns the header's representative body hash, or an
// error wrapping ErrMalformedBodyProof when the body proof does not have
// the wire shape a hash can be read out of.
//
// Prefer this over BlockBodyHash anywhere the result is compared against a
// computed hash: BlockBodyHash has to satisfy common.BlockHeader, which
// gives it no way to report a malformed proof, so it stands in a zero hash
// instead. A caller that then compares that zero hash reports a hash
// mismatch, naming a value the block never carried, when the real fault is
// that the proof could not be parsed at all.
func (h *ByronMainBlockHeader) BlockBodyHashChecked() (
	common.Blake2b256,
	error,
) {
	// Byron BodyProof is an array: [tx_proof, ssc_proof, dlg_proof, upd_proof]
	// tx_proof is: [txCount, txBodyMerkleRoot, txWitnessMerkleRoot]
	// We return the txBodyMerkleRoot as the representative body hash.
	//
	// Both shapes are required exactly, not as minimums: the CDDL fixes
	// them, cardano-ledger reads them with enforceSize, and accepting a
	// longer array would let a header carry trailing junk that no other
	// check looks at. There is deliberately no fallback for a body proof
	// that is a bare hash -- that is the epoch boundary block's format, and
	// accepting it here would mean a main block header could pass this
	// while carrying no transaction proof at all.
	proof, ok := h.BodyProof.([]any)
	if !ok || len(proof) != bodyProofLength {
		return common.Blake2b256{}, fmt.Errorf(
			"%w: main block header body proof is not a %d-element array, "+
				"got %T with %d elements",
			ErrMalformedBodyProof, bodyProofLength, h.BodyProof, len(proof),
		)
	}
	txProof, ok := proof[bodyProofTxIndex].([]any)
	if !ok || len(txProof) != txProofLength {
		return common.Blake2b256{}, fmt.Errorf(
			"%w: main block tx proof is not a %d-element array, "+
				"got %T with %d elements",
			ErrMalformedBodyProof, txProofLength,
			proof[bodyProofTxIndex], len(txProof),
		)
	}
	merkleRoot, ok := txProof[txProofMerkleIndex].([]byte)
	if !ok || len(merkleRoot) != common.Blake2b256Size {
		return common.Blake2b256{}, fmt.Errorf(
			"%w: main block tx merkle root is not a %d-byte hash, got %T",
			ErrMalformedBodyProof, common.Blake2b256Size,
			txProof[txProofMerkleIndex],
		)
	}
	var hash common.Blake2b256
	copy(hash[:], merkleRoot)
	return hash, nil
}

// BlockBodyHash satisfies common.BlockHeader. It returns a zero hash for a
// malformed body proof rather than panicking, so a hostile block cannot
// take down the verification path; callers that act on the result should
// use BlockBodyHashChecked, which reports why instead.
func (h *ByronMainBlockHeader) BlockBodyHash() common.Blake2b256 {
	hash, err := h.BlockBodyHashChecked()
	if err != nil {
		return common.Blake2b256{}
	}
	return hash
}

type ByronTransactionBody struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash       *common.Blake2b256
	TxInputs   []ByronTransactionInput
	TxOutputs  []ByronTransactionOutput
	Attributes cbor.RawMessage
}

func (t *ByronTransactionBody) UnmarshalCBOR(cborData []byte) error {
	type tByronTransaction ByronTransactionBody
	var tmp tByronTransaction
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*t = ByronTransactionBody(tmp)
	t.SetCbor(cborData)
	return nil
}

func (t *ByronTransactionBody) Id() common.Blake2b256 {
	if t.hash == nil {
		tmpHash := common.Blake2b256Hash(t.Cbor())
		t.hash = &tmpHash
	}
	return *t.hash
}

func (t *ByronTransactionBody) Inputs() []common.TransactionInput {
	ret := make([]common.TransactionInput, 0, len(t.TxInputs))
	for _, input := range t.TxInputs {
		ret = append(ret, input)
	}
	return ret
}

func (t *ByronTransactionBody) Outputs() []common.TransactionOutput {
	ret := make([]common.TransactionOutput, len(t.TxOutputs))
	for i := range t.TxOutputs {
		ret[i] = &t.TxOutputs[i]
	}
	return ret
}

func (t *ByronTransactionBody) Fee() *big.Int {
	// The fee is implicit in Byron, and we don't have enough information here to calculate it.
	// You need to know the Lovelace in the inputs to determine the fee, and that information is
	// not provided directly in the TX
	return big.NewInt(0)
}

func (t *ByronTransactionBody) TTL() uint64 {
	// No TTL in Byron
	return 0
}

func (t *ByronTransactionBody) ValidityIntervalStart() uint64 {
	// No validity interval start in Byron
	return 0
}

func (t *ByronTransactionBody) ReferenceInputs() []common.TransactionInput {
	// No reference inputs in Byron
	return nil
}

func (t *ByronTransactionBody) Collateral() []common.TransactionInput {
	// No collateral in Byron
	return nil
}

func (t *ByronTransactionBody) CollateralReturn() common.TransactionOutput {
	// No collateral in Byron
	return nil
}

func (t *ByronTransactionBody) TotalCollateral() *big.Int {
	// No collateral in Byron
	return nil
}

func (t *ByronTransactionBody) Certificates() []common.Certificate {
	// No certificates in Byron
	return nil
}

func (t *ByronTransactionBody) Withdrawals() map[*common.Address]*big.Int {
	// No withdrawals in Byron
	return nil
}

func (t *ByronTransactionBody) AuxDataHash() *common.Blake2b256 {
	// No aux data hash in Byron
	return nil
}

func (t *ByronTransactionBody) RequiredSigners() []common.Blake2b224 {
	// No required signers in Byron
	return nil
}

func (t *ByronTransactionBody) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	// No asset mints in Byron
	return nil
}

func (t *ByronTransactionBody) ScriptDataHash() *common.Blake2b256 {
	// No script data hash in Byron
	return nil
}

func (t *ByronTransactionBody) VotingProcedures() common.VotingProcedures {
	// No voting procedures in Byron
	return nil
}

func (t *ByronTransactionBody) ProposalProcedures() []common.ProposalProcedure {
	// No proposal procedures in Byron
	return nil
}

func (t *ByronTransactionBody) CurrentTreasuryValue() *big.Int {
	// No current treasury value in Byron
	return nil
}

func (t *ByronTransactionBody) Donation() *big.Int {
	// No donation in Byron
	return nil
}

func (t *ByronTransactionBody) Utxorpc() (*utxorpc.Tx, error) {
	return &utxorpc.Tx{}, nil
}

func (t *ByronTransactionBody) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	updateMap := make(map[common.Blake2b224]common.ProtocolParameterUpdate)
	return 0, updateMap
}

type ByronTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash       *common.Blake2b256
	Body       ByronTransactionBody
	Twit       []cbor.Value
	twitCbor   []byte // Original CBOR of witnesses for merkle tree computation
	witnessSet *ByronTransactionWitnessSet
}

func (t *ByronTransaction) UnmarshalCBOR(cborData []byte) error {
	var txArray []cbor.RawMessage
	if _, err := cbor.Decode(cborData, &txArray); err != nil {
		return err
	}

	if len(txArray) < 2 {
		return fmt.Errorf(
			"invalid byron transaction: expected at least 2 components, got %d",
			len(txArray),
		)
	}
	// Decode body
	if _, err := cbor.Decode([]byte(txArray[0]), &t.Body); err != nil {
		return fmt.Errorf("failed to decode byron transaction body: %w", err)
	}
	// Store raw witness CBOR for merkle tree computation
	t.twitCbor = []byte(txArray[1])
	// Decode witnesses (Twit)
	if _, err := cbor.Decode([]byte(txArray[1]), &t.Twit); err != nil {
		return fmt.Errorf(
			"failed to decode byron transaction witnesses: %w",
			err,
		)
	}
	t.SetCbor(cborData)
	return nil
}

func (t *ByronTransaction) MarshalCBOR() ([]byte, error) {
	cborData := t.DecodeStoreCbor.Cbor()
	if cborData != nil {
		return cborData, nil
	}
	type tmpTx struct {
		cbor.StructAsArray
		Body ByronTransactionBody
		Twit cbor.IndefLengthList
	}
	var twit cbor.IndefLengthList
	if t.Twit != nil {
		twit = make(cbor.IndefLengthList, len(t.Twit))
		for i := range t.Twit {
			twit[i] = t.Twit[i]
		}
	}
	return cbor.Encode(&tmpTx{
		Body: t.Body,
		Twit: twit,
	})
}

func (t *ByronTransaction) Cbor() []byte {
	cborData := t.DecodeStoreCbor.Cbor()
	if cborData != nil {
		return cborData[:]
	}
	if t.Body.Cbor() == nil {
		return nil
	}
	cborData, err := cbor.Encode(t)
	if err != nil {
		panic("CBOR encoding that should never fail has failed: " + err.Error())
	}
	return cborData
}

func (ByronTransaction) Type() int {
	return TxTypeByron
}

func (t *ByronTransaction) Hash() common.Blake2b256 {
	return t.Id()
}

func (t *ByronTransaction) Id() common.Blake2b256 {
	return t.Body.Id()
}

func (t *ByronTransaction) Inputs() []common.TransactionInput {
	return t.Body.Inputs()
}

func (t *ByronTransaction) Outputs() []common.TransactionOutput {
	return t.Body.Outputs()
}

func (t *ByronTransaction) Fee() *big.Int {
	return t.Body.Fee()
}

func (t *ByronTransaction) TTL() uint64 {
	return t.Body.TTL()
}

func (t *ByronTransaction) ValidityIntervalStart() uint64 {
	return t.Body.ValidityIntervalStart()
}

func (t *ByronTransaction) ReferenceInputs() []common.TransactionInput {
	return t.Body.ReferenceInputs()
}

func (t *ByronTransaction) Collateral() []common.TransactionInput {
	return t.Body.Collateral()
}

func (t *ByronTransaction) CollateralReturn() common.TransactionOutput {
	return t.Body.CollateralReturn()
}

func (t *ByronTransaction) TotalCollateral() *big.Int {
	return t.Body.TotalCollateral()
}

func (t *ByronTransaction) Certificates() []common.Certificate {
	return t.Body.Certificates()
}

func (t *ByronTransaction) Withdrawals() map[*common.Address]*big.Int {
	return t.Body.Withdrawals()
}

func (t *ByronTransaction) AuxDataHash() *common.Blake2b256 {
	return t.Body.AuxDataHash()
}

func (t *ByronTransaction) RequiredSigners() []common.Blake2b224 {
	return t.Body.RequiredSigners()
}

func (t *ByronTransaction) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return t.Body.AssetMint()
}

func (t *ByronTransaction) ScriptDataHash() *common.Blake2b256 {
	return t.Body.ScriptDataHash()
}

func (t *ByronTransaction) VotingProcedures() common.VotingProcedures {
	return t.Body.VotingProcedures()
}

func (t *ByronTransaction) ProposalProcedures() []common.ProposalProcedure {
	return t.Body.ProposalProcedures()
}

func (t *ByronTransaction) CurrentTreasuryValue() *big.Int {
	return t.Body.CurrentTreasuryValue()
}

func (t *ByronTransaction) Donation() *big.Int {
	return t.Body.Donation()
}

func (t *ByronTransaction) Metadata() common.TransactionMetadatum {
	return nil
}

func (t *ByronTransaction) AuxiliaryData() common.AuxiliaryData {
	return nil
}

func (t *ByronTransaction) LeiosHash() common.Blake2b256 {
	if t.hash == nil {
		tmpHash := common.Blake2b256Hash(t.Cbor())
		t.hash = &tmpHash
	}
	return *t.hash
}

func (t *ByronTransaction) IsValid() bool {
	return true
}

func (t *ByronTransaction) Consumed() []common.TransactionInput {
	return t.Inputs()
}

func (t *ByronTransaction) Produced() []common.Utxo {
	outputs := t.Outputs()
	ret := make([]common.Utxo, 0, len(outputs))
	for idx, output := range outputs {
		ret = append(
			ret,
			common.Utxo{
				Id:     NewByronTransactionInput(t.Id().String(), idx),
				Output: output,
			},
		)
	}
	return ret
}

func (t *ByronTransaction) Witnesses() common.TransactionWitnessSet {
	if t.witnessSet == nil {
		t.witnessSet = NewByronTransactionWitnessSet(t.Twit)
	}
	return *t.witnessSet
}

// WitnessesCbor returns the raw CBOR bytes of the transaction witnesses.
// This is used for merkle tree computation in body hash validation.
func (t *ByronTransaction) WitnessesCbor() []byte {
	return t.twitCbor
}

type ByronTransactionWitnessSet struct {
	vkey      []common.VkeyWitness
	bootstrap []common.BootstrapWitness
}

func NewByronTransactionWitnessSet(
	twit []cbor.Value,
) *ByronTransactionWitnessSet {
	ws := &ByronTransactionWitnessSet{}
	for i := range twit {
		if vw, bw, ok := decodeByronWitness(twit[i]); ok {
			if vw != nil {
				ws.vkey = append(ws.vkey, *vw)
			}
			if bw != nil {
				ws.bootstrap = append(ws.bootstrap, *bw)
			}
		}
	}
	return ws
}

func (w ByronTransactionWitnessSet) Vkey() []common.VkeyWitness {
	return w.vkey
}

func (ByronTransactionWitnessSet) NativeScripts() []common.NativeScript {
	return nil
}

func (w ByronTransactionWitnessSet) Bootstrap() []common.BootstrapWitness {
	return w.bootstrap
}

func (ByronTransactionWitnessSet) PlutusData() []common.Datum {
	return nil
}

func (ByronTransactionWitnessSet) PlutusV1Scripts() []common.PlutusV1Script {
	return nil
}

func (ByronTransactionWitnessSet) PlutusV2Scripts() []common.PlutusV2Script {
	return nil
}

func (ByronTransactionWitnessSet) PlutusV3Scripts() []common.PlutusV3Script {
	return nil
}

func (ByronTransactionWitnessSet) Redeemers() common.TransactionWitnessRedeemers {
	return nil
}

func decodeByronWitness(
	v cbor.Value,
) (vkey *common.VkeyWitness, bootstrap *common.BootstrapWitness, ok bool) {
	switch w := v.Value().(type) {
	case cbor.ConstructorDecoder:
		fields, err := w.ParsedFields()
		if err != nil {
			return nil, nil, false
		}
		return decodeByronWitnessFromConstructor(uint64(w.Tag()), fields)
	case []any:
		if len(w) == 0 {
			return nil, nil, false
		}
		if ctor, ok2 := asUint64(w[0]); ok2 {
			return decodeByronWitnessFromConstructor(ctor, w[1:])
		}
		return decodeByronWitnessFromFields(w)
	default:
		return nil, nil, false
	}
}

func decodeByronWitnessFromConstructor(
	ctor uint64,
	fields []any,
) (vkey *common.VkeyWitness, bootstrap *common.BootstrapWitness, ok bool) {
	switch ctor {
	case 0, 2:
		if len(fields) != 2 {
			return nil, nil, false
		}
		pk, okPk := asBytes(fields[0])
		sig, okSig := asBytes(fields[1])
		if !okPk || !okSig {
			return nil, nil, false
		}
		return &common.VkeyWitness{Vkey: pk, Signature: sig}, nil, true
	case 3:
		if len(fields) != 4 {
			return nil, nil, false
		}
		pk, okPk := asBytes(fields[0])
		sig, okSig := asBytes(fields[1])
		chainCode, okCc := asBytes(fields[2])
		attrs, okAttrs := asBytes(fields[3])
		if !okPk || !okSig || !okCc || !okAttrs {
			return nil, nil, false
		}
		return nil, &common.BootstrapWitness{
			PublicKey:  pk,
			Signature:  sig,
			ChainCode:  chainCode,
			Attributes: attrs,
		}, true
	default:
		return decodeByronWitnessFromFields(fields)
	}
}

func decodeByronWitnessFromFields(
	fields []any,
) (vkey *common.VkeyWitness, bootstrap *common.BootstrapWitness, ok bool) {
	if len(fields) == 2 {
		pk, okPk := asBytes(fields[0])
		sig, okSig := asBytes(fields[1])
		if okPk && okSig {
			return &common.VkeyWitness{Vkey: pk, Signature: sig}, nil, true
		}
	}
	if len(fields) == 4 {
		pk, okPk := asBytes(fields[0])
		sig, okSig := asBytes(fields[1])
		chainCode, okCc := asBytes(fields[2])
		attrs, okAttrs := asBytes(fields[3])
		if okPk && okSig && okCc && okAttrs {
			return nil, &common.BootstrapWitness{
				PublicKey:  pk,
				Signature:  sig,
				ChainCode:  chainCode,
				Attributes: attrs,
			}, true
		}
	}
	return nil, nil, false
}

func asUint64(v any) (uint64, bool) {
	switch x := v.(type) {
	case uint64:
		return x, true
	case uint32:
		return uint64(x), true
	case uint:
		return uint64(x), true
	case int:
		if x < 0 {
			return 0, false
		}
		return uint64(x), true
	case int64:
		if x < 0 {
			return 0, false
		}
		return uint64(x), true
	default:
		return 0, false
	}
}

func asBytes(v any) ([]byte, bool) {
	switch x := v.(type) {
	case []byte:
		return x, true
	case cbor.ByteString:
		return x.Bytes(), true
	default:
		return nil, false
	}
}

func (t *ByronTransaction) Utxorpc() (*utxorpc.Tx, error) {
	return t.Body.Utxorpc()
}

func (t *ByronTransaction) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	return t.Body.ProtocolParameterUpdates()
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
	// Compare the upper bound via int64 so this builds on 32-bit GOARCHs, where
	// int is 32-bit and the untyped math.MaxUint32 constant would overflow the
	// int comparison type. On 32-bit a positive int can never exceed MaxUint32.
	if idx < 0 || int64(idx) > math.MaxUint32 {
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

func (i ByronTransactionInput) ToPlutusData() data.PlutusData {
	// This will never actually get called, but it's identical to Shelley
	return data.NewConstr(
		0,
		data.NewByteString(i.TxId.Bytes()),
		data.NewInteger(big.NewInt(int64(i.OutputIndex))),
	)
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

func (o ByronTransactionOutput) ToPlutusData() data.PlutusData {
	var valueData [][2]data.PlutusData
	if o.OutputAmount > 0 {
		valueData = append(
			valueData,
			[2]data.PlutusData{
				data.NewByteString(nil),
				data.NewMap(
					[][2]data.PlutusData{
						{
							data.NewByteString(nil),
							data.NewInteger(
								new(big.Int).SetUint64(o.OutputAmount),
							),
						},
					},
				),
			},
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

func (o ByronTransactionOutput) Address() common.Address {
	return o.OutputAddress
}

func (o ByronTransactionOutput) ScriptRef() common.Script {
	return nil
}

func (o ByronTransactionOutput) Amount() *big.Int {
	return new(big.Int).SetUint64(o.OutputAmount)
}

func (o ByronTransactionOutput) Assets() *common.MultiAsset[common.MultiAssetTypeOutput] {
	return nil
}

func (o ByronTransactionOutput) DatumHash() *common.Blake2b256 {
	return nil
}

func (o ByronTransactionOutput) Datum() *common.Datum {
	return nil
}

func (o ByronTransactionOutput) Utxorpc() (*utxorpc.TxOutput, error) {
	addressBytes, err := o.OutputAddress.Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to get address bytes: %w", err)
	}
	return &utxorpc.TxOutput{
			Address: addressBytes,
			Coin:    common.BigIntToUtxorpcBigInt(o.Amount()),
		},
		nil
}

func (o ByronTransactionOutput) String() string {
	return fmt.Sprintf(
		"(ByronTransactionOutput address=%s amount=%d)",
		o.OutputAddress.String(),
		o.OutputAmount,
	)
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
	TxPayload     []ByronTransaction
	SscPayload    cbor.Value
	DlgPayload    []any
	UpdPayload    ByronUpdatePayload
	dlgPayloadRaw []byte // Original CBOR for hash validation
	updPayloadRaw []byte // Original CBOR for hash validation
}

func (b *ByronMainBlockBody) UnmarshalCBOR(cborData []byte) error {
	// First, decode the body as raw messages to preserve original CBOR
	var rawParts []cbor.RawMessage
	if _, err := cbor.Decode(cborData, &rawParts); err != nil {
		return err
	}
	if len(rawParts) >= 4 {
		b.dlgPayloadRaw = []byte(rawParts[2])
		b.updPayloadRaw = []byte(rawParts[3])
	}

	// Then decode the full structure
	type tByronMainBlockBody ByronMainBlockBody
	var tmp tByronMainBlockBody
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*b = ByronMainBlockBody(tmp)
	// Restore the raw CBOR fields that were lost in the copy
	if len(rawParts) >= 4 {
		b.dlgPayloadRaw = []byte(rawParts[2])
		b.updPayloadRaw = []byte(rawParts[3])
	}
	b.SetCbor(cborData)
	return nil
}

// DlgPayloadCbor returns the original CBOR bytes of the delegation payload.
// This is used for hash validation.
func (b *ByronMainBlockBody) DlgPayloadCbor() []byte {
	return b.dlgPayloadRaw
}

// UpdPayloadCbor returns the original CBOR bytes of the update payload.
// This is used for hash validation.
func (b *ByronMainBlockBody) UpdPayloadCbor() []byte {
	return b.updPayloadRaw
}

func (b *ByronMainBlockBody) MarshalCBOR() ([]byte, error) {
	// Return stored CBOR if available
	if b.Cbor() != nil {
		return b.Cbor(), nil
	}
	type tmpBody struct {
		cbor.StructAsArray
		cbor.DecodeStoreCbor
		TxPayload  cbor.IndefLengthList
		SscPayload cbor.Value
		DlgPayload []any
		UpdPayload ByronUpdatePayload
	}
	var txPayload cbor.IndefLengthList
	if b.TxPayload != nil {
		txPayload = make(cbor.IndefLengthList, len(b.TxPayload))
		for i := range b.TxPayload {
			txPayload[i] = &b.TxPayload[i]
		}
	}
	temp := tmpBody{
		TxPayload:  txPayload,
		SscPayload: b.SscPayload,
		DlgPayload: b.DlgPayload,
		UpdPayload: b.UpdPayload,
	}
	return cbor.Encode(&temp)
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
	return common.IssuerVkey{}
}

func (h *ByronEpochBoundaryBlockHeader) BlockBodySize() uint64 {
	// Byron doesn't include the block body size in the header
	return 0
}

func (h *ByronEpochBoundaryBlockHeader) Era() common.Era {
	return EraByron
}

// BlockBodyHashChecked returns the EBB header's body hash, or an error
// wrapping ErrMalformedBodyProof when the body proof is not a 32-byte hash.
// See ByronMainBlockHeader.BlockBodyHashChecked for why this exists
// alongside BlockBodyHash.
func (h *ByronEpochBoundaryBlockHeader) BlockBodyHashChecked() (
	common.Blake2b256,
	error,
) {
	// BodyProof is the hash of the block body, encoded as bytes in CBOR
	if bodyProofBytes, ok := h.BodyProof.([]byte); ok &&
		len(bodyProofBytes) == common.Blake2b256Size {
		var hash common.Blake2b256
		copy(hash[:], bodyProofBytes)
		return hash, nil
	}
	return common.Blake2b256{}, fmt.Errorf(
		"%w: epoch boundary block header body proof is %T, expected a "+
			"%d-byte hash",
		ErrMalformedBodyProof, h.BodyProof, common.Blake2b256Size,
	)
}

// BlockBodyHash satisfies common.BlockHeader. See
// ByronMainBlockHeader.BlockBodyHash for why a malformed proof yields a
// zero hash here rather than an error.
func (h *ByronEpochBoundaryBlockHeader) BlockBodyHash() common.Blake2b256 {
	hash, err := h.BlockBodyHashChecked()
	if err != nil {
		return common.Blake2b256{}
	}
	return hash
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
	for idx := range b.Body.TxPayload {
		ret[idx] = &b.Body.TxPayload[idx]
	}
	return ret
}

func (b *ByronMainBlock) Utxorpc() (*utxorpc.Block, error) {
	return &utxorpc.Block{}, nil
}

func (b *ByronMainBlock) BlockBodyHash() common.Blake2b256 {
	return b.Header().BlockBodyHash()
}

// BlockBodyHashChecked reports a malformed body proof instead of standing in
// a zero hash. See ByronMainBlockHeader.BlockBodyHashChecked.
func (b *ByronMainBlock) BlockBodyHashChecked() (common.Blake2b256, error) {
	if b == nil || b.BlockHeader == nil {
		return common.Blake2b256{}, fmt.Errorf(
			"%w: block or block header is nil", ErrMalformedBodyProof,
		)
	}
	return b.BlockHeader.BlockBodyHashChecked()
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
	if tmp.BlockHeader == nil {
		return errors.New("byron EBB block missing header")
	}
	*b = ByronEpochBoundaryBlock(tmp)
	b.SetCbor(cborData)
	return nil
}

// BodyCbor returns the original CBOR bytes of the epoch boundary block body,
// sliced out of the block CBOR that DecodeStoreCbor already preserves. The
// header's proof hashes exactly these bytes, so they are read back rather than
// re-encoded.
func (b *ByronEpochBoundaryBlock) BodyCbor() []byte {
	var parts []cbor.RawMessage
	if _, err := cbor.Decode(b.Cbor(), &parts); err != nil {
		return nil
	}
	if len(parts) < 2 {
		return nil
	}
	return []byte(parts[1])
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

func (b *ByronEpochBoundaryBlock) BlockBodyHash() common.Blake2b256 {
	return b.Header().BlockBodyHash()
}

// BlockBodyHashChecked reports a malformed body proof instead of standing in
// a zero hash. See ByronMainBlockHeader.BlockBodyHashChecked.
func (b *ByronEpochBoundaryBlock) BlockBodyHashChecked() (
	common.Blake2b256,
	error,
) {
	if b == nil || b.BlockHeader == nil {
		return common.Blake2b256{}, fmt.Errorf(
			"%w: block or block header is nil", ErrMalformedBodyProof,
		)
	}
	return b.BlockHeader.BlockBodyHashChecked()
}

func NewByronEpochBoundaryBlockFromCbor(
	data []byte, config ...common.VerifyConfig,
) (*ByronEpochBoundaryBlock, error) {
	var cfg common.VerifyConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	// Default: validation enabled (SkipBodyHashValidation = false)

	var byronEbbBlock ByronEpochBoundaryBlock
	if _, err := cbor.Decode(data, &byronEbbBlock); err != nil {
		return nil, fmt.Errorf("decode Byron EBB block error: %w", err)
	}
	// Bind the body to the header. Without this the header, and so the
	// block hash, can be genuine while the body has been substituted.
	if !cfg.SkipBodyHashValidation {
		if err := byronEbbBlock.ValidateBodyProof(); err != nil {
			return nil, err
		}
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

func NewByronMainBlockFromCbor(
	data []byte,
	config ...common.VerifyConfig,
) (*ByronMainBlock, error) {
	var cfg common.VerifyConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	// Default: validation enabled (SkipBodyHashValidation = false)

	var byronMainBlock ByronMainBlock
	if _, err := cbor.Decode(data, &byronMainBlock); err != nil {
		return nil, fmt.Errorf("decode Byron main block error: %w", err)
	}
	// Bind the body to the header. Without this the header, and so the
	// block hash, can be genuine while the body has been substituted.
	//
	// cfg is forwarded as-is: ValidateBodyProof checks
	// cfg.EnableByronSscProofHashValidation itself, so a caller who opts
	// into the full ssc_proof hash comparison via this same VerifyConfig
	// gets it at decode time too, while the default (that flag unset)
	// leaves ssc_proof checked only structurally -- see ValidateBodyProof's
	// doc comment (bodyproof.go) and
	// common.VerifyConfig.EnableByronSscProofHashValidation's.
	if !cfg.SkipBodyHashValidation {
		if err := byronMainBlock.ValidateBodyProof(cfg); err != nil {
			return nil, err
		}
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
