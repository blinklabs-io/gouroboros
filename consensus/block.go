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

package consensus

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/vrf"
)

// BlockBuilder constructs new blocks for the Praos consensus protocol.
type BlockBuilder struct {
	vrfSigner       VRFSigner
	kesSigner       KESSigner
	opCert          *OperationalCert
	poolId          []byte
	issuerVkey      []byte
	activeSlotCoeff *big.Rat
	mode            ConsensusMode
}

// OperationalCert represents an operational certificate for block production.
type OperationalCert struct {
	HotVkey        []byte // KES verification key (32 bytes)
	SequenceNumber uint32 // Monotonically increasing counter
	KesPeriod      uint32 // Starting KES period for this cert
	Signature      []byte // Cold key signature (64 bytes)
}

// NewBlockBuilder creates a new block builder using CPRAOS consensus
// (Babbage and later eras). For Shelley through Alonzo (TPraos) block
// building, use NewBlockBuilderWithMode with ConsensusModeTPraos.
//
// Parameters:
//   - vrfSigner: VRF signer for leader election proofs
//   - kesSigner: KES signer for block header signatures
//   - opCert: operational certificate binding KES key to cold key
//   - poolId: the pool ID (cold verification key hash)
//   - issuerVkey: the pool's cold verification key (32 bytes)
//   - activeSlotCoeff: active slot coefficient for leader election
func NewBlockBuilder(
	vrfSigner VRFSigner,
	kesSigner KESSigner,
	opCert *OperationalCert,
	poolId []byte,
	issuerVkey []byte,
	activeSlotCoeff *big.Rat,
) *BlockBuilder {
	return NewBlockBuilderWithMode(
		vrfSigner,
		kesSigner,
		opCert,
		poolId,
		issuerVkey,
		activeSlotCoeff,
		ConsensusModeCPraos,
	)
}

// NewBlockBuilderWithMode creates a new block builder for the given
// consensus mode. Use ConsensusModeTPraos for Shelley through Alonzo era
// block building and ConsensusModeCPraos for Babbage and later eras.
//
// Parameters:
//   - vrfSigner: VRF signer for leader election proofs
//   - kesSigner: KES signer for block header signatures
//   - opCert: operational certificate binding KES key to cold key
//   - poolId: the pool ID (cold verification key hash)
//   - issuerVkey: the pool's cold verification key (32 bytes)
//   - activeSlotCoeff: active slot coefficient for leader election
//   - mode: the consensus mode (CPRAOS or TPraos)
func NewBlockBuilderWithMode(
	vrfSigner VRFSigner,
	kesSigner KESSigner,
	opCert *OperationalCert,
	poolId []byte,
	issuerVkey []byte,
	activeSlotCoeff *big.Rat,
	mode ConsensusMode,
) *BlockBuilder {
	return &BlockBuilder{
		vrfSigner:       vrfSigner,
		kesSigner:       kesSigner,
		opCert:          opCert,
		poolId:          poolId,
		issuerVkey:      issuerVkey,
		activeSlotCoeff: activeSlotCoeff,
		mode:            mode,
	}
}

// HeaderBody contains the fields of a block header body.
// This is a simplified representation that can be serialized to CBOR.
//
// VrfOutput/VrfProof always carry the leader VRF certificate (bheaderL,
// seedL = mkNonceFromNumber(1) for TPraos; the single VrfResult for
// CPraos). NonceVrfOutput/NonceVrfProof additionally carry the TPraos-only
// nonce VRF certificate (bheaderEta, seedEta = mkNonceFromNumber(0)),
// used for epoch nonce evolution; they are unused (nil) for CPraos-mode
// headers, which have no separate nonce VRF field.
type HeaderBody struct {
	BlockNumber uint64
	Slot        uint64
	PrevHash    []byte // 32 bytes
	IssuerVkey  []byte // 32 bytes
	VrfKey      []byte // 32 bytes
	// NonceVrfOutput and NonceVrfProof are set only for TPraos-mode
	// headers (Shelley through Alonzo).
	NonceVrfOutput       []byte // 64 bytes (TPraos only)
	NonceVrfProof        []byte // 80 bytes (TPraos only)
	VrfOutput            []byte // 64 bytes
	VrfProof             []byte // 80 bytes
	BlockBodySize        uint64
	BlockBodyHash        []byte // 32 bytes
	OpCertHotVkey        []byte // 32 bytes
	OpCertSequenceNumber uint32
	OpCertKesPeriod      uint32
	OpCertSignature      []byte // 64 bytes
	ProtoMajor           uint64
	ProtoMinor           uint64
}

// Header represents a complete block header ready for serialization.
type Header struct {
	Body      HeaderBody
	Signature []byte // KES signature (448 bytes)
}

// BuildHeaderInput contains all information needed to build a header.
type BuildHeaderInput struct {
	// Slot is the slot number for this block
	Slot uint64
	// BlockNumber is the block height
	BlockNumber uint64
	// PrevHash is the hash of the previous block header
	PrevHash []byte
	// EpochNonce is the epoch nonce for VRF input
	EpochNonce []byte
	// PoolStake is the pool's delegated stake
	PoolStake uint64
	// TotalStake is the total active stake
	TotalStake uint64
	// BlockBodyHash is the hash of the block body
	BlockBodyHash []byte
	// BlockBodySize is the size of the block body in bytes
	BlockBodySize uint64
	// ProtoMajor is the protocol major version
	ProtoMajor uint64
	// ProtoMinor is the protocol minor version
	ProtoMinor uint64
}

// BuildHeader constructs a block header for the given parameters.
// It performs leader election and returns an error if not eligible.
//
// Returns the constructed header and leader election result.
func (b *BlockBuilder) BuildHeader(
	input BuildHeaderInput,
) (*Header, *LeaderElectionResult, error) {
	if b.vrfSigner == nil {
		return nil, nil, errors.New("vrfSigner is nil")
	}
	if b.kesSigner == nil {
		return nil, nil, errors.New("kesSigner is nil")
	}
	if b.opCert == nil {
		return nil, nil, errors.New("opCert is nil")
	}
	if len(input.PrevHash) == 0 {
		return nil, nil, errors.New("prevHash is empty")
	}
	if len(input.EpochNonce) == 0 {
		return nil, nil, errors.New("epochNonce is empty")
	}
	if len(input.BlockBodyHash) == 0 {
		return nil, nil, errors.New("blockBodyHash is empty")
	}
	if len(b.issuerVkey) != 32 {
		return nil, nil, fmt.Errorf(
			"issuerVkey: expected 32 bytes, got %d",
			len(b.issuerVkey),
		)
	}
	if len(input.PrevHash) != 32 {
		return nil, nil, fmt.Errorf(
			"prevHash: expected 32 bytes, got %d",
			len(input.PrevHash),
		)
	}
	if len(input.EpochNonce) != 32 {
		return nil, nil, fmt.Errorf(
			"epochNonce: expected 32 bytes, got %d",
			len(input.EpochNonce),
		)
	}
	if len(input.BlockBodyHash) != 32 {
		return nil, nil, fmt.Errorf(
			"blockBodyHash: expected 32 bytes, got %d",
			len(input.BlockBodyHash),
		)
	}
	if b.activeSlotCoeff == nil {
		return nil, nil, errors.New("activeSlotCoeff cannot be nil")
	}
	// Validate VRF public key size
	vrfPubKey := b.vrfSigner.PublicKey()
	if len(vrfPubKey) != vrf.PublicKeySize {
		return nil, nil, fmt.Errorf(
			"vrfSigner.PublicKey(): expected %d bytes, got %d",
			vrf.PublicKeySize,
			len(vrfPubKey),
		)
	}
	// Validate OpCert field sizes
	if len(b.opCert.HotVkey) != 32 {
		return nil, nil, fmt.Errorf(
			"opCert.HotVkey: expected 32 bytes, got %d",
			len(b.opCert.HotVkey),
		)
	}
	if len(b.opCert.Signature) != 64 {
		return nil, nil, fmt.Errorf(
			"opCert.Signature: expected 64 bytes, got %d",
			len(b.opCert.Signature),
		)
	}
	// Verify kesSigner matches opCert.HotVkey - a mismatch will produce invalid headers
	kesPublicKey := b.kesSigner.PublicKey()
	if len(kesPublicKey) != 32 {
		return nil, nil, fmt.Errorf(
			"kesSigner.PublicKey(): expected 32 bytes, got %d",
			len(kesPublicKey),
		)
	}
	if !bytes.Equal(kesPublicKey, b.opCert.HotVkey) {
		return nil, nil, errors.New(
			"kesSigner public key does not match opCert.HotVkey",
		)
	}

	// Step 1: Check leader eligibility (mode-aware: TPraos for
	// Shelley-Alonzo, CPraos for Babbage+)
	leaderResult, err := IsSlotLeaderWithMode(
		input.Slot,
		input.EpochNonce,
		input.PoolStake,
		input.TotalStake,
		b.activeSlotCoeff,
		b.vrfSigner,
		b.mode,
	)
	if err != nil {
		return nil, nil, err
	}
	if !leaderResult.Eligible {
		return nil, leaderResult, ErrNotSlotLeader
	}

	// Step 1b: TPraos headers additionally carry a nonce VRF certificate
	// (bheaderEta, seedEta = mkNonceFromNumber(0)) alongside the leader
	// VRF certificate (bheaderL, seedL = mkNonceFromNumber(1)) computed
	// above via IsSlotLeaderWithMode. The nonce VRF is independent of
	// leader eligibility and feeds epoch nonce evolution; it has no
	// equivalent field in CPraos headers.
	var nonceVrfProof, nonceVrfOutput []byte
	if b.mode == ConsensusModeTPraos {
		if input.Slot > math.MaxInt64 {
			return nil, leaderResult, fmt.Errorf(
				"slot %d exceeds maximum int64 value for VRF input",
				input.Slot,
			)
		}
		nonceVrfInput, nErr := vrf.MkSeedTPraos(
			int64(input.Slot), //nolint:gosec // G115: validated above
			input.EpochNonce,
			vrf.SeedEta(),
		)
		if nErr != nil {
			return nil, leaderResult, fmt.Errorf(
				"failed to create nonce VRF input: %w",
				nErr,
			)
		}
		nonceVrfProof, nonceVrfOutput, err = b.vrfSigner.Prove(
			nonceVrfInput,
		)
		if err != nil {
			return nil, leaderResult, fmt.Errorf(
				"failed to generate nonce VRF proof: %w",
				err,
			)
		}
	}

	// Step 2: Construct header body
	headerBody := HeaderBody{
		BlockNumber:          input.BlockNumber,
		Slot:                 input.Slot,
		PrevHash:             input.PrevHash,
		IssuerVkey:           b.issuerVkey,
		VrfKey:               b.vrfSigner.PublicKey(),
		NonceVrfOutput:       nonceVrfOutput,
		NonceVrfProof:        nonceVrfProof,
		VrfOutput:            leaderResult.Output,
		VrfProof:             leaderResult.Proof,
		BlockBodySize:        input.BlockBodySize,
		BlockBodyHash:        input.BlockBodyHash,
		OpCertHotVkey:        b.opCert.HotVkey,
		OpCertSequenceNumber: b.opCert.SequenceNumber,
		OpCertKesPeriod:      b.opCert.KesPeriod,
		OpCertSignature:      b.opCert.Signature,
		ProtoMajor:           input.ProtoMajor,
		ProtoMinor:           input.ProtoMinor,
	}

	// Step 3: Serialize header body for signing
	bodyBytes, err := b.serializeHeaderBody(&headerBody)
	if err != nil {
		return nil, leaderResult, err
	}

	// Step 4: Sign with KES
	signature, err := b.kesSigner.Sign(bodyBytes)
	if err != nil {
		return nil, leaderResult, err
	}

	header := &Header{
		Body:      headerBody,
		Signature: signature,
	}

	return header, leaderResult, nil
}

// serializeHeaderBody serializes the header body to CBOR for signing,
// dispatching on the builder's consensus mode. This matches the format
// expected by Cardano nodes for the corresponding era.
func (b *BlockBuilder) serializeHeaderBody(body *HeaderBody) ([]byte, error) {
	switch b.mode {
	case ConsensusModeTPraos:
		return serializeHeaderBodyTPraos(body)
	case ConsensusModeCPraos:
		return serializeHeaderBodyCPraos(body)
	default:
		return nil, fmt.Errorf("unknown consensus mode: %d", b.mode)
	}
}

// serializeHeaderBodyCPraos serializes the header body to CBOR using the
// Babbage/Conway (CPraos) wire format: a 10-element array with the
// (single) leader VRF result, OpCert, and protocol version each nested as
// their own sub-array. Matches ledger/babbage.BabbageBlockHeaderBody.
func serializeHeaderBodyCPraos(body *HeaderBody) ([]byte, error) {
	bodyArray := []any{
		body.BlockNumber,
		body.Slot,
		body.PrevHash,
		body.IssuerVkey,
		body.VrfKey,
		// VRF result is encoded as [output, proof]
		[]any{body.VrfOutput, body.VrfProof},
		body.BlockBodySize,
		body.BlockBodyHash,
		// OpCert is encoded as [hot_vkey, seq_num, kes_period, signature]
		[]any{
			body.OpCertHotVkey,
			body.OpCertSequenceNumber,
			body.OpCertKesPeriod,
			body.OpCertSignature,
		},
		// Proto version is encoded as [major, minor]
		[]any{body.ProtoMajor, body.ProtoMinor},
	}

	return cbor.Encode(bodyArray)
}

// serializeHeaderBodyTPraos serializes the header body to CBOR using the
// Shelley-Alonzo (TPraos) wire format: a flat 15-element array with two
// separate VRF result sub-arrays -- NonceVrf (bheaderEta, seedEta) and
// LeaderVrf (bheaderL, seedL) -- and flat (non-nested) OpCert and
// protocol version fields. Matches
// ledger/shelley.ShelleyBlockHeaderBody's field order exactly (also used
// unmodified by allegra/mary/alonzo).
func serializeHeaderBodyTPraos(body *HeaderBody) ([]byte, error) {
	bodyArray := []any{
		body.BlockNumber,
		body.Slot,
		body.PrevHash,
		body.IssuerVkey,
		body.VrfKey,
		// NonceVrf is encoded as [output, proof]
		[]any{body.NonceVrfOutput, body.NonceVrfProof},
		// LeaderVrf is encoded as [output, proof]
		[]any{body.VrfOutput, body.VrfProof},
		body.BlockBodySize,
		body.BlockBodyHash,
		body.OpCertHotVkey,
		body.OpCertSequenceNumber,
		body.OpCertKesPeriod,
		body.OpCertSignature,
		body.ProtoMajor,
		body.ProtoMinor,
	}

	return cbor.Encode(bodyArray)
}

// CheckSlotLeadership checks if the block builder is eligible for a slot
// without constructing a full header.
func (b *BlockBuilder) CheckSlotLeadership(
	slot uint64,
	epochNonce []byte,
	poolStake uint64,
	totalStake uint64,
) (*LeaderElectionResult, error) {
	if b.activeSlotCoeff == nil {
		return nil, errors.New("activeSlotCoeff cannot be nil")
	}
	// Validate epochNonce length to prevent panic in vrf.MkInputVrf
	if len(epochNonce) != 32 {
		return nil, fmt.Errorf(
			"epochNonce must be 32 bytes, got %d",
			len(epochNonce),
		)
	}
	return IsSlotLeaderWithMode(
		slot,
		epochNonce,
		poolStake,
		totalStake,
		b.activeSlotCoeff,
		b.vrfSigner,
		b.mode,
	)
}

// ComputeBlockBodyHash computes the hash of a block body.
// The body should be the CBOR-encoded transaction data.
func ComputeBlockBodyHash(bodyBytes []byte) []byte {
	hash := common.Blake2b256Hash(bodyBytes)
	return hash[:]
}

// ComputeVRFInput creates the VRF input for a given slot and epoch nonce.
// Returns nil if epochNonce is not exactly 32 bytes or if slot overflows int64.
func ComputeVRFInput(slot uint64, epochNonce []byte) []byte {
	// Validate epochNonce length to prevent panic in vrf.MkInputVrf
	if len(epochNonce) != 32 {
		return nil
	}
	// Guard against slot overflow when converting to int64
	// While Cardano slots are far below int64 max, validate for safety
	if slot > math.MaxInt64 {
		return nil
	}
	vrfInput, err := vrf.MkInputVrf(
		int64(slot),
		epochNonce,
	) //nolint:gosec // G115: validated above
	if err != nil {
		return nil
	}
	return vrfInput
}

// Errors for block construction
var (
	ErrNotSlotLeader = errors.New("not eligible to produce block in this slot")
)
