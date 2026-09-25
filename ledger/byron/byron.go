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
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/bits"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/ed25519byron"
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

var (
	// ErrByronSlotNumberOverflow indicates that the absolute slot cannot fit in
	// a uint64.
	ErrByronSlotNumberOverflow = errors.New(
		"byron absolute slot overflows uint64",
	)
	// ErrByronSlotsPerEpochZero indicates invalid epoch-length configuration.
	ErrByronSlotsPerEpochZero = errors.New(
		"byron slots per epoch must be nonzero",
	)
)

// SlotNumberFromEpochAndSlot converts Byron's raw epoch and within-epoch slot
// counts using the epoch length configured for the network.
func SlotNumberFromEpochAndSlot(
	epoch uint64,
	slot uint64,
	slotsPerEpoch uint64,
) (uint64, error) {
	if slotsPerEpoch == 0 {
		return 0, ErrByronSlotsPerEpochZero
	}
	hi, lo := bits.Mul64(epoch, slotsPerEpoch)
	if hi != 0 {
		return 0, ErrByronSlotNumberOverflow
	}
	result, carry := bits.Add64(lo, slot, 0)
	if carry != 0 {
		return 0, ErrByronSlotNumberOverflow
	}
	return result, nil
}

// SlotNumberFromHeader converts a Byron-capable block header using the
// configured epoch length. A zero epoch length selects the legacy mainnet
// length; headers without the conversion capability are accepted only for
// that legacy length.
func SlotNumberFromHeader(
	header interface{ SlotNumber() uint64 },
	slotsPerEpoch uint64,
) (uint64, error) {
	if slotsPerEpoch == 0 {
		slotsPerEpoch = ByronSlotsPerEpoch
	}
	if converter, ok := header.(interface {
		SlotNumberWithEpochLength(uint64) (uint64, error)
	}); ok {
		return converter.SlotNumberWithEpochLength(slotsPerEpoch)
	}
	if slotsPerEpoch != ByronSlotsPerEpoch {
		return 0, fmt.Errorf(
			"header does not support configured Byron epoch length %d",
			slotsPerEpoch,
		)
	}
	return header.SlotNumber(), nil
}

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
	hash          common.Blake2b256Cache
	ProtocolMagic uint32
	PrevBlock     common.Blake2b256
	BodyProof     any
	ConsensusData struct {
		cbor.StructAsArray
		// [slotid, pubkey, difficulty, blocksig]
		SlotId struct {
			cbor.StructAsArray
			Epoch uint64
			// Slot is the slot count within the epoch. cardano-ledger
			// types it as SlotCount, a Word64 newtype with a derived
			// DecCBOR (Cardano/Chain/Slotting/SlotCount.hs:15-19,
			// EpochAndSlotCount.hs:38-41), and toSlotNumber adds it to
			// the flattened epoch with no bound of its own.
			Slot uint64
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
		// ExtraProof is dropped by the reference decoder:
		// decCBORBlockVersions ends with dropBytes, which accepts a
		// byte string of any length and never interprets it
		// (Cardano/Chain/Block/Header.hs:392-395).
		ExtraProof []byte
	}
}

func (h *ByronMainBlockHeader) SetCbor(cborData []byte) {
	// Callers must externally synchronize this with Hash and other mutations.
	h.DecodeStoreCbor.SetCbor(cborData)
	h.hash.Reset()
}

func (h *ByronMainBlockHeader) SetCborReference(cborData []byte) {
	// Callers must externally synchronize this with Hash and other mutations.
	h.DecodeStoreCbor.SetCborReference(cborData)
	h.hash.Reset()
}

func (h *ByronMainBlockHeader) UnmarshalCBOR(cborData []byte) error {
	if err := validateByronDefiniteStrings(cborData); err != nil {
		return fmt.Errorf("invalid Byron main block header string framing: %w", err)
	}
	rawParts, err := byronArrayFields(cborData, "byron main block header")
	if err != nil {
		return err
	}
	if len(rawParts) != 5 {
		return fmt.Errorf(
			"byron main block header has %d fields, expected 5",
			len(rawParts),
		)
	}
	extraData, err := byronArrayFields(rawParts[4], "byron main block extra data")
	if err != nil {
		return fmt.Errorf("decode byron main block extra data: %w", err)
	}
	if len(extraData) != 4 {
		return fmt.Errorf(
			"byron main block extra data has %d fields, expected 4",
			len(extraData),
		)
	}
	consensusData, err := byronArrayFields(rawParts[3], "byron main block consensus data")
	if err != nil {
		return err
	}
	if len(consensusData) != 4 {
		return fmt.Errorf("byron main block consensus data has %d fields, expected 4", len(consensusData))
	}
	bodyProof, err := byronArrayFields(rawParts[2], "byron main block body proof")
	if err != nil {
		return err
	}
	if len(bodyProof) != bodyProofLength {
		return fmt.Errorf("byron main block body proof has %d fields, expected %d", len(bodyProof), bodyProofLength)
	}
	if err := requireByronArrayLength(bodyProof[bodyProofTxIndex], "byron transaction proof", txProofLength); err != nil {
		return err
	}
	if err := requireByronArrayLength(consensusData[0], "byron slot identifier", 2); err != nil {
		return err
	}
	if err := requireByronVerificationKey(consensusData[1], "Byron main header verification key"); err != nil {
		return err
	}
	if err := requireByronArrayLength(consensusData[2], "byron chain difficulty", 1); err != nil {
		return err
	}
	if err := requireByronArrayLength(extraData[0], "byron block version", 3); err != nil {
		return err
	}
	softwareVersion, err := byronArrayFields(extraData[1], "byron software version")
	if err != nil {
		return err
	}
	if len(softwareVersion) != 2 {
		return fmt.Errorf("byron software version has %d fields, expected 2", len(softwareVersion))
	}
	if err := requireByronTextString(softwareVersion[0], "Byron application name"); err != nil {
		return err
	}
	// The reference's decCBORBlockVersions requires this field's map to be
	// empty (Cardano.Chain.Common.Attributes.dropEmptyAttributes), even
	// though it never interprets the map's contents otherwise. It is not
	// covered by any signature or hash check that treats it as opaque, so a
	// mutated non-empty map here would decode without detection
	// (blinklabs-io/gouroboros#2340).
	if err := requireEmptyCborMap(
		extraData[2], "byron main block header attributes",
	); err != nil {
		return err
	}
	if err := requireByronByteString(
		extraData[3], "byron main block extra data proof",
	); err != nil {
		return err
	}
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
	return h.hash.Get(func() common.Blake2b256 {
		return common.Blake2b256Hash(
			append(
				[]byte{0x82, BlockTypeByronMain},
				h.Cbor()...,
			),
		)
	})
}

func (h *ByronMainBlockHeader) PrevHash() common.Blake2b256 {
	return h.PrevBlock
}

func (h *ByronMainBlockHeader) BlockNumber() uint64 {
	return h.ConsensusData.Difficulty.Value
}

// SlotNumber returns the mainnet absolute slot for legacy callers. Use
// SlotNumberWithEpochLength when interpreting a network-configured header.
func (h *ByronMainBlockHeader) SlotNumber() uint64 {
	return (h.ConsensusData.SlotId.Epoch * ByronSlotsPerEpoch) +
		h.ConsensusData.SlotId.Slot
}

// SlotNumberWithEpochLength converts the preserved epoch and slot counts to
// an absolute slot using the network's configured epoch length.
func (h *ByronMainBlockHeader) SlotNumberWithEpochLength(
	slotsPerEpoch uint64,
) (uint64, error) {
	return SlotNumberFromEpochAndSlot(
		h.ConsensusData.SlotId.Epoch,
		h.ConsensusData.SlotId.Slot,
		slotsPerEpoch,
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
	hash       common.Blake2b256Cache
	TxInputs   []ByronTransactionInput
	TxOutputs  []ByronTransactionOutput
	Attributes cbor.RawMessage
}

func (t *ByronTransactionBody) SetCbor(cborData []byte) {
	// Replacing CBOR invalidates the hash memo; callers must not mutate the
	// body concurrently with Id or this setter.
	t.DecodeStoreCbor.SetCbor(cborData)
	t.hash.Reset()
}

func (t *ByronTransactionBody) SetCborReference(cborData []byte) {
	// Replacing CBOR invalidates the hash memo; callers must not mutate the
	// body concurrently with Id or this setter.
	t.DecodeStoreCbor.SetCborReference(cborData)
	t.hash.Reset()
}

func (t *ByronTransactionBody) UnmarshalCBOR(cborData []byte) error {
	if err := validateByronDefiniteStrings(cborData); err != nil {
		return fmt.Errorf("invalid Byron transaction body string framing: %w", err)
	}
	if err := requireByronArrayLength(cborData, "byron transaction body", 3); err != nil {
		return err
	}
	attributes, err := byronArrayField(cborData, "byron transaction body", 2, 3)
	if err != nil {
		return err
	}
	if err := validateTransactionAttributes(attributes); err != nil {
		return fmt.Errorf("invalid Byron transaction attributes: %w", err)
	}
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
	return t.hash.Get(func() common.Blake2b256 {
		attributes := make(map[uint64][]byte)
		if len(t.Attributes) > 0 {
			if _, err := cbor.Decode(t.Attributes, &attributes); err != nil {
				panic("CBOR attributes that should never fail to decode have failed: " + err.Error())
			}
		}
		inputs := make(cbor.IndefLengthList, len(t.TxInputs))
		for i := range t.TxInputs {
			inputs[i] = t.TxInputs[i]
		}
		outputs := make(cbor.IndefLengthList, len(t.TxOutputs))
		for i := range t.TxOutputs {
			outputs[i] = t.TxOutputs[i]
		}
		type canonicalBody struct {
			cbor.StructAsArray
			Inputs     cbor.IndefLengthList
			Outputs    cbor.IndefLengthList
			Attributes map[uint64][]byte
		}
		encoded, err := cbor.Encode(&canonicalBody{
			Inputs:     inputs,
			Outputs:    outputs,
			Attributes: attributes,
		})
		if err != nil {
			panic("CBOR encoding that should never fail has failed: " + err.Error())
		}
		return common.Blake2b256Hash(encoded)
	})
}

// WireId hashes the original annotated transaction-body bytes used by Byron
// witness signing and transaction Merkle proofs.
func (t *ByronTransactionBody) WireId() common.Blake2b256 {
	return common.Blake2b256Hash(t.Cbor())
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
	Body             ByronTransactionBody
	Twit             []cbor.Value
	twitCbor         []byte // Original CBOR of witnesses for merkle tree computation
	witnessSet       *ByronTransactionWitnessSet
	protocolMagic    uint32
	hasProtocolMagic bool
}

func (t *ByronTransaction) UnmarshalCBOR(cborData []byte) error {
	if err := validateByronDefiniteStrings(cborData); err != nil {
		return fmt.Errorf("invalid Byron transaction string framing: %w", err)
	}
	txArray, err := byronArrayFields(cborData, "byron transaction")
	if err != nil {
		return err
	}

	if len(txArray) != 2 {
		return fmt.Errorf(
			"invalid byron transaction: expected 2 components, got %d",
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
	// Every element of Twit must decode as a recognized TxInWitness
	// variant. The reference decoder has no catch-all case, so a witness
	// that decodeByronWitness cannot recognize (missing tag 24, wrong
	// field count, unknown constructor, ...) must fail the whole
	// transaction rather than being silently dropped from the exposed
	// witness set.
	for idx, witness := range t.Twit {
		if _, _, ok := decodeByronWitness(witness); !ok {
			return fmt.Errorf(
				"failed to decode byron transaction witness %d: unrecognized TxInWitness encoding",
				idx,
			)
		}
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

// WireId returns the hash of the original transaction-body bytes.
func (t *ByronTransaction) WireId() common.Blake2b256 {
	return t.Body.WireId()
}

// ValidateVKeyWitnesses verifies transaction witnesses using Byron's
// protocol-magic and constructor-specific signing domains.
func (t *ByronTransaction) ValidateVKeyWitnesses(protocolMagic uint32) error {
	for idx, witness := range t.Twit {
		outer, ok := witness.Value().([]any)
		if !ok || len(outer) != 2 {
			return fmt.Errorf("invalid Byron transaction witness %d", idx)
		}
		constructor, ok := asUint64(outer[0])
		if !ok || (constructor != 0 && constructor != 2) {
			continue
		}
		wrapped, ok := outer[1].(cbor.WrappedCbor)
		if !ok {
			return fmt.Errorf("invalid Byron transaction witness %d payload", idx)
		}
		var fields []any
		consumed, err := cbor.Decode(wrapped.Bytes(), &fields)
		if err != nil || consumed != len(wrapped.Bytes()) || len(fields) != 2 {
			return fmt.Errorf("invalid Byron transaction witness %d fields", idx)
		}
		publicKey, ok := asBytes(fields[0])
		expectedKeySize := ed25519.PublicKeySize
		if constructor == 0 {
			expectedKeySize = VerificationKeySize
		}
		if !ok || len(publicKey) != expectedKeySize {
			return fmt.Errorf("invalid Byron transaction witness %d public key", idx)
		}
		signature, ok := asBytes(fields[1])
		if !ok {
			return fmt.Errorf("invalid Byron transaction witness %d signature", idx)
		}
		tag := byte(0x01)
		if constructor == 2 {
			tag = 0x02
		}
		magic, err := cbor.Encode(protocolMagic)
		if err != nil {
			return fmt.Errorf("encode Byron protocol magic: %w", err)
		}
		txId := t.Body.WireId()
		txPayload, err := cbor.Encode(txId[:])
		if err != nil {
			return fmt.Errorf("encode Byron transaction signing payload: %w", err)
		}
		message := append([]byte{tag}, magic...)
		message = append(message, txPayload...)
		if !ed25519byron.Verify(
			publicKey[:ed25519.PublicKeySize], message, signature,
		) {
			return fmt.Errorf("invalid Byron transaction witness %d signature", idx)
		}
	}
	return nil
}

func (t *ByronTransaction) ValidateByronVKeyWitnesses() error {
	if len(t.Witnesses().Vkey()) == 0 {
		return nil
	}
	if !t.hasProtocolMagic {
		return errors.New("byron protocol magic is required to verify transaction witnesses")
	}
	return t.ValidateVKeyWitnesses(t.protocolMagic)
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

// LeiosHash returns the Blake2b-256 hash of the transaction's CBOR. The value
// is recomputed on every call: it is not memoized on the transaction, because
// era transaction types are copied by value and an in-struct cache cannot be
// populated safely from a shared receiver.
func (t *ByronTransaction) LeiosHash() common.Blake2b256 {
	return common.Blake2b256Hash(t.Cbor())
}

func (t *ByronTransaction) IsValid() bool {
	return true
}

func (t *ByronTransaction) Consumed() []common.TransactionInput {
	return t.Inputs()
}

func (t *ByronTransaction) Produced() []common.Utxo {
	outputs := t.Outputs()
	txId := t.Id()
	ret := make([]common.Utxo, 0, len(outputs))
	for idx, output := range outputs {
		ret = append(
			ret,
			common.Utxo{
				Id: ByronTransactionInput{
					TxId: txId,
					// The output count is bounded by the Byron
					// transaction size limit, orders of magnitude
					// below MaxUint32.
					//nolint:gosec // G115: see above
					OutputIndex: uint32(idx),
				},
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

// decodeByronWitness decodes a single Byron TxInWitness value:
// [ctor, #6.24(bytes .cbor payload)]. The reference decoder
// (decodeKnownCborDataItem) requires the semantic tag 24 wrapper around the
// nested payload; an untagged array carrying the same fields is not a valid
// witness encoding and must be rejected rather than silently accepted.
func decodeByronWitness(
	v cbor.Value,
) (vkey *common.VkeyWitness, bootstrap *common.BootstrapWitness, ok bool) {
	w, isArray := v.Value().([]any)
	if !isArray || len(w) != 2 {
		return nil, nil, false
	}
	ctor, ok2 := asUint64(w[0])
	if !ok2 {
		return nil, nil, false
	}
	wrapped, isWrapped := w[1].(cbor.WrappedCbor)
	if !isWrapped {
		return nil, nil, false
	}
	var fields []any
	wrappedBytes := wrapped.Bytes()
	consumed, err := cbor.Decode(wrappedBytes, &fields)
	if err != nil || consumed != len(wrappedBytes) {
		return nil, nil, false
	}
	return decodeByronWitnessFromConstructor(ctor, fields)
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
		expectedKeySize := ed25519.PublicKeySize
		if ctor == 0 {
			expectedKeySize = VerificationKeySize
		}
		if !okPk || len(pk) != expectedKeySize ||
			!okSig || len(sig) != ed25519.SignatureSize {
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
		// The reference decoder's TxInWitness sum type has no catch-all
		// case: an unrecognized constructor is invalid regardless of
		// whether its field count happens to match a known variant.
		return nil, nil, false
	}
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

// NewByronTransactionInput builds a transaction input from a hex-encoded
// 32-byte transaction hash and an output index.
//
// It returns an error rather than panicking, so a caller passing a value it
// did not produce itself -- a hash off the wire, out of an API request, or
// out of a config file -- can reject it. A hash shorter than 32 bytes would
// otherwise panic in the slice-to-array conversion below, before any check
// on it ran.
func NewByronTransactionInput(
	hash string,
	idx int,
) (ByronTransactionInput, error) {
	tmpHash, err := hex.DecodeString(hash)
	if err != nil {
		return ByronTransactionInput{}, fmt.Errorf(
			"decode transaction hash: %w", err,
		)
	}
	if len(tmpHash) != common.Blake2b256Size {
		return ByronTransactionInput{}, fmt.Errorf(
			"transaction hash is %d bytes, expected %d",
			len(tmpHash), common.Blake2b256Size,
		)
	}
	// Compare the upper bound via int64 so this builds on 32-bit GOARCHs, where
	// int is 32-bit and the untyped math.MaxUint32 constant would overflow the
	// int comparison type. On 32-bit a positive int can never exceed MaxUint32.
	if idx < 0 || int64(idx) > math.MaxUint32 {
		return ByronTransactionInput{}, fmt.Errorf(
			"output index %d out of range", idx,
		)
	}
	return ByronTransactionInput{
		TxId:        common.Blake2b256(tmpHash),
		OutputIndex: uint32(idx),
	}, nil
}

func (i *ByronTransactionInput) UnmarshalCBOR(data []byte) error {
	if err := validateByronDefiniteStrings(data); err != nil {
		return err
	}
	if err := requireByronArrayLength(data, "byron transaction input", 2); err != nil {
		return err
	}
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
			Cbor cbor.WrappedCbor
		}
		if _, err := cbor.Decode(data, &tmpData); err != nil {
			return err
		}
		if err := requireByronArrayLength(tmpData.Cbor, "byron transaction input reference", 2); err != nil {
			return err
		}
		// Decode inner data
		type tByronTransactionInput ByronTransactionInput
		var tmp tByronTransactionInput
		innerBytes := tmpData.Cbor.Bytes()
		consumed, err := cbor.Decode(innerBytes, &tmp)
		if err != nil {
			return err
		}
		if consumed != len(innerBytes) {
			return fmt.Errorf(
				"byron TxInUtxo tag 24 payload has %d trailing byte(s)",
				len(innerBytes)-consumed,
			)
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

// MaxLovelace is the largest value the Byron reference decoder accepts for
// a single Lovelace amount (Cardano.Chain.Common.Lovelace.maxLovelaceVal),
// the total supply of ADA expressed in Lovelace. It bounds each individual
// transaction-output amount at decode time, independent of any later
// aggregate balance or fee check.
const MaxLovelace uint64 = 45_000_000_000_000_000

type ByronTransactionOutput struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	OutputAddress common.Address `json:"address"`
	OutputAmount  uint64         `json:"amount"`
}

func (o *ByronTransactionOutput) UnmarshalCBOR(data []byte) error {
	if err := validateByronDefiniteStrings(data); err != nil {
		return err
	}
	if err := requireByronArrayLength(data, "byron transaction output", 2); err != nil {
		return err
	}
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
	if tmpData.Amount > MaxLovelace {
		return fmt.Errorf(
			"byron transaction output amount %d exceeds maximum Lovelace value %d",
			tmpData.Amount,
			MaxLovelace,
		)
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

func (p *ByronUpdatePayload) UnmarshalCBOR(raw []byte) error {
	fields, err := cborRawArrayEntries(raw, true)
	if err != nil || len(fields) != updatePayloadElementCount {
		return fmt.Errorf("%w: update payload must be a two-element array", ErrInvalidPayload)
	}
	proposalEntries, err := cborRawArrayEntries(fields[0], false)
	if err != nil {
		return fmt.Errorf("%w: decode update proposals: %w", ErrInvalidPayload, err)
	}
	if len(proposalEntries) > 1 {
		return fmt.Errorf("%w: update payload contains %d proposals, expected at most one", ErrInvalidPayload, len(proposalEntries))
	}
	var tmp ByronUpdatePayload
	if _, err := cbor.Decode(fields[0], &tmp.Proposals); err != nil {
		return fmt.Errorf("%w: decode update proposal: %w", ErrInvalidPayload, err)
	}
	if _, err := cbor.Decode(fields[1], &tmp.Votes); err != nil {
		return fmt.Errorf("%w: decode update votes: %w", ErrInvalidPayload, err)
	}
	if err := validateUpdatePayloadStructure(raw, tmp.Proposals, tmp.Votes); err != nil {
		return err
	}
	*p = tmp
	return nil
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
	if err := validateByronDefiniteStrings(cborData); err != nil {
		return fmt.Errorf("invalid Byron update proposal string framing: %w", err)
	}
	if err := requireByronArrayLength(cborData, "byron update proposal", updateProposalElementCount); err != nil {
		return err
	}
	verificationKey, err := byronArrayField(cborData, "byron update proposal", 5, updateProposalElementCount)
	if err != nil {
		return err
	}
	if err := requireByronVerificationKey(verificationKey, "Byron update proposal verification key"); err != nil {
		return err
	}
	signatureRaw, err := byronArrayField(cborData, "byron update proposal", 6, updateProposalElementCount)
	if err != nil {
		return err
	}
	signature, err := decodeByronByteString(signatureRaw, false)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("byron update proposal signature must be a 64-byte string")
	}
	type tByronUpdateProposal ByronUpdateProposal
	var tmp tByronUpdateProposal
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*p = ByronUpdateProposal(tmp)
	p.SetCbor(cborData)
	return nil
}

// ByronUpdateProposalBlockVersionMod is cardano-ledger's
// ProtocolParametersUpdate: fourteen optional fields, each encoded as a
// zero- or one-element list, decoded by fourteen bare decCBORs
// (Cardano/Chain/Update/ProtocolParametersUpdate.hs:36-42, :135-152).
//
// The five size and duration fields are Natural there, an unbounded
// non-negative integer, so they are big.Int here rather than uint64.
// decodeNatural accepts any integer and rejects only negatives
// (Cardano/Ledger/Binary/Decoding/Decoder.hs:1463-1469); that is the one
// bound this type still carries, enforced in UnmarshalCBOR.
type ByronUpdateProposalBlockVersionMod struct {
	cbor.StructAsArray
	ScriptVersion     []uint16
	SlotDuration      []*big.Int
	MaxBlockSize      []*big.Int
	MaxHeaderSize     []*big.Int
	MaxTxSize         []*big.Int
	MaxProposalSize   []*big.Int
	MpcThd            []ByronLovelacePortion
	HeavyDelThd       []ByronLovelacePortion
	UpdateVoteThd     []ByronLovelacePortion
	UpdateProposalThd []ByronLovelacePortion
	UpdateImplicit    []uint64
	SoftForkRule      []ByronSoftForkRule
	TxFeePolicy       []ByronTxFeePolicy
	UnlockStakeEpoch  []uint64
}

func (m *ByronUpdateProposalBlockVersionMod) UnmarshalCBOR(
	cborData []byte,
) error {
	if err := validateByronDefiniteStrings(cborData); err != nil {
		return fmt.Errorf("invalid Byron protocol parameter update string framing: %w", err)
	}
	if err := requireByronArrayLength(cborData, "byron protocol parameter update", 14); err != nil {
		return err
	}
	type tByronUpdateProposalBlockVersionMod ByronUpdateProposalBlockVersionMod
	var tmp tByronUpdateProposalBlockVersionMod
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	for _, field := range []struct {
		name   string
		length int
	}{
		{name: "scriptVersion", length: len(tmp.ScriptVersion)},
		{name: "slotDuration", length: len(tmp.SlotDuration)},
		{name: "maxBlockSize", length: len(tmp.MaxBlockSize)},
		{name: "maxHeaderSize", length: len(tmp.MaxHeaderSize)},
		{name: "maxTxSize", length: len(tmp.MaxTxSize)},
		{name: "maxProposalSize", length: len(tmp.MaxProposalSize)},
		{name: "mpcThd", length: len(tmp.MpcThd)},
		{name: "heavyDelThd", length: len(tmp.HeavyDelThd)},
		{name: "updateVoteThd", length: len(tmp.UpdateVoteThd)},
		{name: "updateProposalThd", length: len(tmp.UpdateProposalThd)},
		{name: "updateImplicit", length: len(tmp.UpdateImplicit)},
		{name: "softForkRule", length: len(tmp.SoftForkRule)},
		{name: "txFeePolicy", length: len(tmp.TxFeePolicy)},
		{name: "unlockStakeEpoch", length: len(tmp.UnlockStakeEpoch)},
	} {
		if field.length > 1 {
			return fmt.Errorf(
				"byron update proposal %s has %d values, expected at most 1",
				field.name, field.length,
			)
		}
	}
	for _, field := range []struct {
		name   string
		values []*big.Int
	}{
		{name: "slotDuration", values: tmp.SlotDuration},
		{name: "maxBlockSize", values: tmp.MaxBlockSize},
		{name: "maxHeaderSize", values: tmp.MaxHeaderSize},
		{name: "maxTxSize", values: tmp.MaxTxSize},
		{name: "maxProposalSize", values: tmp.MaxProposalSize},
	} {
		for _, value := range field.values {
			if value == nil {
				return fmt.Errorf(
					"byron update proposal %s is null",
					field.name,
				)
			}
			if value.Sign() < 0 {
				return fmt.Errorf(
					"byron update proposal %s is negative: %s",
					field.name,
					value.String(),
				)
			}
		}
	}
	*m = ByronUpdateProposalBlockVersionMod(tmp)
	return nil
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
	if err := validateByronDefiniteStrings(cborData); err != nil {
		return fmt.Errorf("invalid Byron main block body string framing: %w", err)
	}
	// Decode the fixed record as raw messages to preserve original CBOR.
	rawParts, err := byronArrayFields(cborData, "byron main block body")
	if err != nil {
		return err
	}
	if len(rawParts) != 4 {
		return fmt.Errorf("byron main block body has %d fields, expected 4", len(rawParts))
	}
	if len(rawParts[0]) == 0 || rawParts[0][0] != 0x9f {
		return errors.New("byron transaction payload must use indefinite-list framing")
	}
	if err := validateDelegationPayloadWire(rawParts[2]); err != nil {
		return err
	}
	updateParts, err := byronArrayFields(rawParts[3], "byron update payload")
	if err != nil {
		return err
	}
	if len(updateParts) != updatePayloadElementCount || len(updateParts[updatePayloadVotesIndex]) == 0 || updateParts[updatePayloadVotesIndex][0] != 0x9f {
		return errors.New("byron update votes must use indefinite-list framing")
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
	hash common.Blake2b256Cache
	// ProtocolMagic is dropped by the reference decoder:
	// decCBORABoundaryHeader opens with dropInt32, which accepts the
	// full signed 32-bit range and never interprets the value
	// (Cardano/Chain/Block/Header.hs:613-616).
	ProtocolMagic int32
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
	// genesisTag records the deprecated 255 => "Genesis" extra header data
	// attribute; see HasGenesisTag.
	genesisTag bool
}

func (h *ByronEpochBoundaryBlockHeader) SetCbor(cborData []byte) {
	// Callers must externally synchronize this with Hash and other mutations.
	h.DecodeStoreCbor.SetCbor(cborData)
	h.hash.Reset()
}

func (h *ByronEpochBoundaryBlockHeader) SetCborReference(cborData []byte) {
	// Callers must externally synchronize this with Hash and other mutations.
	h.DecodeStoreCbor.SetCborReference(cborData)
	h.hash.Reset()
}

func (h *ByronEpochBoundaryBlockHeader) UnmarshalCBOR(cborData []byte) error {
	if err := validateByronDefiniteStrings(cborData); err != nil {
		return fmt.Errorf("invalid Byron epoch boundary header string framing: %w", err)
	}
	// decCBORABoundaryHeader uses enforceSize for the header, consensus data,
	// and chain difficulty, so each must use definite-length array framing.
	rawParts, err := decodeDefiniteCborList(cborData, 5, "byron EBB header")
	if err != nil {
		return err
	}
	consensusParts, err := decodeDefiniteCborList(
		rawParts[3], 2, "byron EBB header consensus data",
	)
	if err != nil {
		return err
	}
	if _, err := decodeDefiniteCborList(
		consensusParts[1], 1, "byron EBB header chain difficulty",
	); err != nil {
		return err
	}
	// The reference reads ExtraData with
	// dropBoundaryExtraHeaderDataRetainGenesisTag: enforceSize 1, then
	// decCBORAttributes (Cardano/Chain/Block/Boundary.hs).
	attrs, err := decodeByronExtraDataAttributes(
		rawParts[4], "byron EBB extra header data", true,
	)
	if err != nil {
		return err
	}
	type tByronEpochBoundaryBlockHeader ByronEpochBoundaryBlockHeader
	var tmp tByronEpochBoundaryBlockHeader
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*h = ByronEpochBoundaryBlockHeader(tmp)
	for _, attr := range attrs {
		if attr.key == byronGenesisTagKey &&
			string(attr.value) == byronGenesisTagValue {
			h.genesisTag = true
		}
	}
	h.SetCbor(cborData)
	return nil
}

// HasGenesisTag reports whether the header's extra data carries the
// deprecated 255 => "Genesis" attribute. The reference interprets the
// previous hash of an EBB as a genesis hash, not a header hash, when the
// epoch is zero or this tag is present (decCBORABoundaryHeader in
// Cardano/Chain/Block/Header.hs).
func (h *ByronEpochBoundaryBlockHeader) HasGenesisTag() bool {
	return h.genesisTag
}

func (h *ByronEpochBoundaryBlockHeader) Hash() common.Blake2b256 {
	return h.hash.Get(func() common.Blake2b256 {
		return common.Blake2b256Hash(
			append(
				[]byte{0x82, BlockTypeByronEbb},
				h.Cbor()...,
			),
		)
	})
}

func (h *ByronEpochBoundaryBlockHeader) PrevHash() common.Blake2b256 {
	return h.PrevBlock
}

func (h *ByronEpochBoundaryBlockHeader) BlockNumber() uint64 {
	return h.ConsensusData.Difficulty.Value
}

// SlotNumber returns the mainnet absolute slot for legacy callers. Use
// SlotNumberWithEpochLength when interpreting a network-configured header.
func (h *ByronEpochBoundaryBlockHeader) SlotNumber() uint64 {
	return h.ConsensusData.Epoch * ByronSlotsPerEpoch
}

// SlotNumberWithEpochLength converts the preserved epoch count to an absolute
// slot using the network's configured epoch length.
func (h *ByronEpochBoundaryBlockHeader) SlotNumberWithEpochLength(
	slotsPerEpoch uint64,
) (uint64, error) {
	return SlotNumberFromEpochAndSlot(h.ConsensusData.Epoch, 0, slotsPerEpoch)
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

// BlockBodyHashChecked returns the zero hash for an EBB because its body
// proof is an opaque byte string that the reference decoder discards. A
// non-byte-string proof is malformed. The EBB body proof must not bind the
// block body to the header; see #2341.
func (h *ByronEpochBoundaryBlockHeader) BlockBodyHashChecked() (
	common.Blake2b256,
	error,
) {
	if _, ok := h.BodyProof.([]byte); ok {
		return common.Blake2b256{}, nil
	}
	return common.Blake2b256{}, fmt.Errorf(
		"%w: epoch boundary block header body proof is %T, expected a byte string",
		ErrMalformedBodyProof, h.BodyProof,
	)
}

// BlockBodyHash satisfies common.BlockHeader. EBB body proofs have no hash
// semantics, so this returns zero for every well-formed proof.
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
	if err := validateByronDefiniteStrings(cborData); err != nil {
		return fmt.Errorf("invalid Byron main block string framing: %w", err)
	}
	rawParts, err := byronArrayFields(cborData, "byron main block")
	if err != nil {
		return err
	}
	if len(rawParts) != 3 {
		return fmt.Errorf(
			"byron main block has %d fields, expected 3",
			len(rawParts),
		)
	}
	// The reference's decCBORABlock requires this field to be exactly
	// [Attributes], and Attributes must be empty
	// (Cardano.Chain.Block.Block.hs: "enforceSize \"ExtraBodyData\" 1 >>
	// dropEmptyAttributes"). ExtraBodyData sits outside the header entirely,
	// so mutating it changes neither the header hash, the PBFT signature, nor
	// the body proofs, and would decode undetected otherwise
	// (blinklabs-io/gouroboros#2340).
	var extra []cbor.RawMessage
	if _, err := cbor.Decode(rawParts[2], &extra); err != nil {
		return fmt.Errorf("decode byron main block extra body data: %w", err)
	}
	if len(extra) != 1 {
		return fmt.Errorf(
			"byron main block extra body data has %d fields, expected 1",
			len(extra),
		)
	}
	if err := requireEmptyCborMap(
		extra[0], "byron main block extra body data attributes",
	); err != nil {
		return err
	}

	type tByronMainBlock ByronMainBlock
	var tmp tByronMainBlock
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	// A CBOR null header decodes into a nil pointer without error, and every
	// accessor on the block dereferences it. Rejecting it here keeps the
	// non-nil invariant whatever VerifyConfig a caller passes, matching the
	// epoch boundary block's own check.
	if tmp.BlockHeader == nil {
		return errors.New("byron main block missing header")
	}
	if err := tmp.Body.ValidateUpdatePayloadStructure(); err != nil {
		return fmt.Errorf("decode byron update payload: %w", err)
	}
	*b = ByronMainBlock(tmp)
	for idx := range b.Body.TxPayload {
		b.Body.TxPayload[idx].protocolMagic = b.BlockHeader.ProtocolMagic
		b.Body.TxPayload[idx].hasProtocolMagic = true
	}
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

// SlotNumberWithEpochLength converts the block's slot with a configured epoch
// length and reports overflow instead of wrapping.
func (b *ByronMainBlock) SlotNumberWithEpochLength(
	slotsPerEpoch uint64,
) (uint64, error) {
	return b.BlockHeader.SlotNumberWithEpochLength(slotsPerEpoch)
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
	// Body entries are dropped by the reference decoder:
	// dropBoundaryBody is dropList dropBytes, so each entry is a byte
	// string of any length that is never interpreted
	// (Cardano/Chain/Block/Boundary.hs:73-74).
	Body  [][]byte
	Extra []any
}

func (b *ByronEpochBoundaryBlock) UnmarshalCBOR(cborData []byte) error {
	if err := validateByronDefiniteStrings(cborData); err != nil {
		return fmt.Errorf("invalid Byron epoch boundary block string framing: %w", err)
	}
	// decCBORABoundaryBlock requires a definite three-field outer array.
	rawParts, err := decodeDefiniteCborList(cborData, 3, "byron EBB")
	if err != nil {
		return err
	}
	// dropBoundaryBody is dropList dropBytes, and dropList opens with
	// decodeListLenIndef, so a definite-length body is rejected even when
	// its entries are valid.
	if _, _, indefinite := cbor.ArrayInfo(rawParts[1]); !indefinite {
		return errors.New(
			"byron EBB body must be an indefinite-length CBOR list",
		)
	}
	var body []cbor.RawMessage
	if _, err := cbor.Decode(rawParts[1], &body); err != nil {
		return fmt.Errorf("decode byron EBB body: %w", err)
	}
	for idx, entry := range body {
		if err := requireCborByteString(
			entry, fmt.Sprintf("byron EBB body entry %d", idx),
		); err != nil {
			return err
		}
	}
	// dropBoundaryExtraBodyData is enforceSize 1 >> dropAttributes.
	extraAttrs, err := decodeByronExtraDataAttributes(
		rawParts[2], "byron EBB extra body data", false,
	)
	if err != nil {
		return err
	}
	var header *ByronEpochBoundaryBlockHeader
	if _, err := cbor.Decode(rawParts[0], &header); err != nil {
		return err
	}
	if header == nil {
		return errors.New("byron EBB block missing header")
	}
	var bodyEntries [][]byte
	if _, err := cbor.Decode(rawParts[1], &bodyEntries); err != nil {
		return fmt.Errorf("decode byron EBB body: %w", err)
	}
	// Extra is built from the validated attributes rather than decoded
	// generically: dropMap accepts duplicate keys, which the shared decode
	// mode's DupMapKeyEnforcedAPF would otherwise reject. The last duplicate
	// wins, as the reference never reads the values.
	extraMap := make(map[any]any, len(extraAttrs))
	for _, attr := range extraAttrs {
		extraMap[uint64(attr.key)] = attr.value
	}
	*b = ByronEpochBoundaryBlock{
		BlockHeader: header,
		Body:        bodyEntries,
		Extra:       []any{extraMap},
	}
	b.SetCbor(cborData)
	return nil
}

// requireCborByteString matches cborg's decodeBytes, which accepts only a
// definite-length byte string: the indefinite-length header 0x5f is decoded
// by the separate decodeBytesIndef and fails here.
func requireCborByteString(raw cbor.RawMessage, field string) error {
	if len(raw) == 0 || raw[0]&cbor.CborTypeMask != cbor.CborTypeByteString ||
		raw[0] == cbor.CborTypeByteString|0x1f {
		return fmt.Errorf(
			"%s must be a definite-length CBOR byte string", field,
		)
	}
	return nil
}

// decodeDefiniteCborList matches the reference's enforceSize: a
// definite-length list of exactly n elements.
func decodeDefiniteCborList(
	raw []byte, n int, field string,
) ([]cbor.RawMessage, error) {
	if _, _, indefinite := cbor.ArrayInfo(raw); indefinite {
		return nil, fmt.Errorf(
			"%s must be a definite-length CBOR list", field,
		)
	}
	var parts []cbor.RawMessage
	if _, err := cbor.Decode(raw, &parts); err != nil {
		return nil, fmt.Errorf("decode %s: %w", field, err)
	}
	if len(parts) != n {
		return nil, fmt.Errorf(
			"%s has %d fields, expected %d", field, len(parts), n,
		)
	}
	return parts, nil
}

const (
	byronGenesisTagKey   uint8 = 255
	byronGenesisTagValue       = "Genesis"
)

type byronAttribute struct {
	key   uint8
	value []byte
}

// decodeByronExtraDataAttributes enforces the reference's [Attributes]
// shape: a definite-length list of exactly one element, holding a
// definite-length map from Word8 keys to byte strings. Unknown keys are
// allowed.
//
// strictKeyOrder must be set where the reference decodes the map with
// decCBORAttributes rather than dropping it with dropAttributes. The former
// goes through the Byron-version Map decoder (decodeMapSkel), which rejects
// any key not strictly greater than the one before it; dropMap checks
// neither order nor duplicates.
func decodeByronExtraDataAttributes(
	raw cbor.RawMessage, field string, strictKeyOrder bool,
) ([]byronAttribute, error) {
	parts, err := decodeDefiniteCborList(raw, 1, field)
	if err != nil {
		return nil, err
	}
	field += " attributes"
	decoder, err := cbor.NewStreamDecoder(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", field, err)
	}
	pairCount, _, _, err := decoder.DecodeMapHeader()
	if err != nil {
		return nil, fmt.Errorf(
			"%s must be a definite-length CBOR map: %w", field, err,
		)
	}
	attrs := make([]byronAttribute, 0, min(pairCount, len(parts[0])))
	for idx := range pairCount {
		keyOffset, keyLength, err := decoder.Skip()
		if err != nil {
			return nil, fmt.Errorf("decode %s key %d: %w", field, idx, err)
		}
		key, err := decodeCborWord8(
			decoder.RawBytes(keyOffset, keyLength),
		)
		if err != nil {
			return nil, fmt.Errorf("%s key %d: %w", field, idx, err)
		}
		if strictKeyOrder && idx > 0 && key <= attrs[idx-1].key {
			return nil, fmt.Errorf(
				"%s key %d (%d) is not greater than the previous key",
				field, idx, key,
			)
		}
		valueOffset, valueLength, err := decoder.Skip()
		if err != nil {
			return nil, fmt.Errorf("decode %s value %d: %w", field, idx, err)
		}
		rawValue := cbor.RawMessage(
			decoder.RawBytes(valueOffset, valueLength),
		)
		if err := requireCborByteString(
			rawValue, fmt.Sprintf("%s value %d", field, idx),
		); err != nil {
			return nil, err
		}
		var value []byte
		if _, err := cbor.Decode(rawValue, &value); err != nil {
			return nil, fmt.Errorf("decode %s value %d: %w", field, idx, err)
		}
		attrs = append(attrs, byronAttribute{key: key, value: value})
	}
	if !decoder.EOF() {
		return nil, fmt.Errorf("%s has trailing CBOR data", field)
	}
	return attrs, nil
}

// decodeCborWord8 matches cborg's decodeWord8: an unsigned integer of any
// encoded width whose value fits in 8 bits. Checking the major type first
// rules out negative integers and tagged bignums.
func decodeCborWord8(raw cbor.RawMessage) (uint8, error) {
	if len(raw) == 0 || raw[0]&cbor.CborTypeMask != 0 {
		return 0, errors.New("must be a CBOR unsigned integer")
	}
	var value uint64
	if _, err := cbor.Decode(raw, &value); err != nil {
		return 0, err
	}
	if value > math.MaxUint8 {
		return 0, fmt.Errorf("value %d does not fit in a Word8", value)
	}
	return uint8(value), nil
}

// requireEmptyCborMap enforces the reference's dropEmptyAttributes check: the
// value must be a CBOR map, and it must have zero entries. Decoding into
// map[any]any rejects the type mismatch and reads the length regardless of
// whether it was encoded in shortest form, matching decodeMapLen's own
// length-only check.
func requireEmptyCborMap(raw cbor.RawMessage, field string) error {
	// CBOR null (0xf6) and undefined (0xf7) both decode into a nil
	// map[any]any with no error, and len(nil) == 0, so the length check
	// below would otherwise accept either in place of a real empty map.
	// The reference's decodeMapLen requires an actual map header.
	if len(raw) == 0 || raw[0]&cbor.CborTypeMask != cbor.CborTypeMap {
		return fmt.Errorf("%s must be a CBOR map", field)
	}
	var m map[any]any
	if _, err := cbor.Decode(raw, &m); err != nil {
		return fmt.Errorf("%s must be an empty CBOR map: %w", field, err)
	}
	if len(m) != 0 {
		return fmt.Errorf(
			"%s must be empty, got %d entries",
			field,
			len(m),
		)
	}
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

// SlotNumberWithEpochLength converts the block's slot with a configured epoch
// length and reports overflow instead of wrapping.
func (b *ByronEpochBoundaryBlock) SlotNumberWithEpochLength(
	slotsPerEpoch uint64,
) (uint64, error) {
	return b.BlockHeader.SlotNumberWithEpochLength(slotsPerEpoch)
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
	// Check the header's body-proof field is a well-formed byte string.
	// This does not bind the body to the header -- the reference decoder
	// does not either; see ValidateBodyProof's own doc comment.
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
