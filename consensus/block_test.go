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
	"crypto/ed25519"
	"errors"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/vrf"
)

// Test seeds (32 bytes each)
var (
	testVRFSeedBlock = []byte("test_vrf_seed_for_block_build!!!")
	testKESSeed      = []byte("test_kes_seed_for_block_build!!!")
)

// SimpleKESSigner is a simple KES signer for testing
type SimpleKESSigner struct {
	sk        *kes.SecretKey
	publicKey []byte
	period    uint64
}

// testKESDepth uses a smaller depth for faster test key generation.
// Depth 2 = 4 periods (vs depth 6 = 64 periods), but much faster to generate.
const testKESDepth = 2

// NewSimpleKESSigner creates a new KES signer from a seed
func NewSimpleKESSigner(seed []byte) (*SimpleKESSigner, error) {
	sk, pk, err := kes.KeyGen(testKESDepth, seed)
	if err != nil {
		return nil, err
	}
	return &SimpleKESSigner{
		sk:        sk,
		publicKey: pk,
		period:    0,
	}, nil
}

// Sign produces a KES signature for the given message
func (s *SimpleKESSigner) Sign(message []byte) ([]byte, error) {
	return kes.Sign(s.sk, s.period, message)
}

// PublicKey returns the current KES verification key
func (s *SimpleKESSigner) PublicKey() []byte {
	return s.publicKey
}

// Period returns the current KES period
func (s *SimpleKESSigner) Period() uint64 {
	return s.period
}

func TestNewBlockBuilder(t *testing.T) {
	vrfSigner, err := NewSimpleVRFSigner(testVRFSeedBlock)
	if err != nil {
		t.Fatalf("failed to create VRF signer: %v", err)
	}
	kesSigner, err := NewSimpleKESSigner(testKESSeed)
	if err != nil {
		t.Fatalf("failed to create KES signer: %v", err)
	}

	opCert := &OperationalCert{
		HotVkey:        kesSigner.PublicKey(),
		SequenceNumber: 1,
		KesPeriod:      0,
		Signature:      make([]byte, 64),
	}

	issuerVkey := make([]byte, 32)
	poolId := make([]byte, 28)

	builder := NewBlockBuilder(
		vrfSigner,
		kesSigner,
		opCert,
		poolId,
		issuerVkey,
		big.NewRat(1, 20),
	)

	if builder == nil {
		t.Fatal("expected non-nil block builder")
	}
}

func TestBuildHeaderEligible(t *testing.T) {
	vrfSigner, err := NewSimpleVRFSigner(testVRFSeedBlock)
	if err != nil {
		t.Fatalf("failed to create VRF signer: %v", err)
	}
	kesSigner, err := NewSimpleKESSigner(testKESSeed)
	if err != nil {
		t.Fatalf("failed to create KES signer: %v", err)
	}

	opCert := &OperationalCert{
		HotVkey:        kesSigner.PublicKey(),
		SequenceNumber: 1,
		KesPeriod:      0,
		Signature:      make([]byte, 64),
	}

	issuerVkey := make([]byte, 32)
	for i := range issuerVkey {
		issuerVkey[i] = byte(i)
	}
	poolId := make([]byte, 28)

	// Use high active slot coefficient to increase chance of eligibility
	builder := NewBlockBuilder(
		vrfSigner,
		kesSigner,
		opCert,
		poolId,
		issuerVkey,
		big.NewRat(99, 100), // 99% active slots
	)

	epochNonce := make([]byte, 32)
	for i := range epochNonce {
		epochNonce[i] = byte(i)
	}

	prevHash := make([]byte, 32)
	bodyHash := make([]byte, 32)

	// Try multiple slots to find one where we're eligible
	var header *Header
	var result *LeaderElectionResult

	for slot := uint64(1); slot <= 100; slot++ {
		input := BuildHeaderInput{
			Slot:          slot,
			BlockNumber:   slot,
			PrevHash:      prevHash,
			EpochNonce:    epochNonce,
			PoolStake:     1000000000,
			TotalStake:    1000000000, // 100% stake
			BlockBodyHash: bodyHash,
			BlockBodySize: 1024,
			ProtoMajor:    9,
			ProtoMinor:    0,
		}

		header, result, err = builder.BuildHeader(input)
		if err == nil && header != nil {
			break
		}
	}

	if header == nil {
		t.Skip("no eligible slot found in range (unlikely but possible)")
	}

	// Verify header structure
	if header.Body.BlockNumber == 0 {
		t.Error("expected non-zero block number")
	}
	if len(header.Body.VrfProof) != 80 {
		t.Errorf(
			"expected 80-byte VRF proof, got %d",
			len(header.Body.VrfProof),
		)
	}
	if len(header.Body.VrfOutput) != 64 {
		t.Errorf(
			"expected 64-byte VRF output, got %d",
			len(header.Body.VrfOutput),
		)
	}
	// Signature size = 64 + depth*64 (for testKESDepth=2, size=192)
	expectedSigSize := 64 + testKESDepth*64
	if len(header.Signature) != expectedSigSize {
		t.Errorf(
			"expected %d-byte KES signature, got %d",
			expectedSigSize,
			len(header.Signature),
		)
	}
	if result == nil || !result.Eligible {
		t.Error("expected eligible result")
	}
}

func TestBuildHeaderNotEligible(t *testing.T) {
	vrfSigner, err := NewSimpleVRFSigner(testVRFSeedBlock)
	if err != nil {
		t.Fatalf("failed to create VRF signer: %v", err)
	}
	kesSigner, err := NewSimpleKESSigner(testKESSeed)
	if err != nil {
		t.Fatalf("failed to create KES signer: %v", err)
	}

	opCert := &OperationalCert{
		HotVkey:        kesSigner.PublicKey(),
		SequenceNumber: 1,
		KesPeriod:      0,
		Signature:      make([]byte, 64),
	}

	issuerVkey := make([]byte, 32)
	poolId := make([]byte, 28)

	// Use very low stake to ensure ineligibility
	builder := NewBlockBuilder(
		vrfSigner,
		kesSigner,
		opCert,
		poolId,
		issuerVkey,
		big.NewRat(1, 20), // 5% active slots
	)

	epochNonce := make([]byte, 32)
	prevHash := make([]byte, 32)
	bodyHash := make([]byte, 32)

	input := BuildHeaderInput{
		Slot:          1,
		BlockNumber:   1,
		PrevHash:      prevHash,
		EpochNonce:    epochNonce,
		PoolStake:     1,             // Tiny stake
		TotalStake:    1000000000000, // vs huge total
		BlockBodyHash: bodyHash,
		BlockBodySize: 1024,
		ProtoMajor:    9,
		ProtoMinor:    0,
	}

	_, _, err = builder.BuildHeader(input)

	// Most likely not eligible with such small relative stake
	if err == nil {
		// If somehow eligible, that's fine - just skip
		t.Skip("unexpectedly eligible (very unlikely but possible)")
	}

	if !errors.Is(err, ErrNotSlotLeader) {
		t.Fatalf("expected ErrNotSlotLeader, got: %v", err)
	}
}

func TestBuildHeaderMissingInputs(t *testing.T) {
	vrfSigner, err := NewSimpleVRFSigner(testVRFSeedBlock)
	if err != nil {
		t.Fatalf("failed to create VRF signer: %v", err)
	}
	kesSigner, err := NewSimpleKESSigner(testKESSeed)
	if err != nil {
		t.Fatalf("failed to create KES signer: %v", err)
	}

	opCert := &OperationalCert{
		HotVkey:        kesSigner.PublicKey(),
		SequenceNumber: 1,
		KesPeriod:      0,
		Signature:      make([]byte, 64),
	}

	issuerVkey := make([]byte, 32)
	poolId := make([]byte, 28)

	builder := NewBlockBuilder(
		vrfSigner,
		kesSigner,
		opCert,
		poolId,
		issuerVkey,
		big.NewRat(1, 20),
	)

	epochNonce := make([]byte, 32)
	prevHash := make([]byte, 32)
	bodyHash := make([]byte, 32)

	// Test empty prevHash
	input := BuildHeaderInput{
		Slot:          1,
		BlockNumber:   1,
		PrevHash:      []byte{},
		EpochNonce:    epochNonce,
		PoolStake:     1000,
		TotalStake:    10000,
		BlockBodyHash: bodyHash,
		BlockBodySize: 1024,
	}

	_, _, err = builder.BuildHeader(input)
	if err == nil {
		t.Error("expected error for empty prevHash")
	}

	// Test empty epochNonce
	input.PrevHash = prevHash
	input.EpochNonce = []byte{}

	_, _, err = builder.BuildHeader(input)
	if err == nil {
		t.Error("expected error for empty epochNonce")
	}

	// Test empty blockBodyHash
	input.EpochNonce = epochNonce
	input.BlockBodyHash = []byte{}

	_, _, err = builder.BuildHeader(input)
	if err == nil {
		t.Error("expected error for empty blockBodyHash")
	}
}

func TestBuildHeaderNilSigners(t *testing.T) {
	kesSigner, err := NewSimpleKESSigner(testKESSeed)
	if err != nil {
		t.Fatalf("failed to create KES signer: %v", err)
	}
	vrfSigner, err := NewSimpleVRFSigner(testVRFSeedBlock)
	if err != nil {
		t.Fatalf("failed to create VRF signer: %v", err)
	}

	opCert := &OperationalCert{
		HotVkey:        kesSigner.PublicKey(),
		SequenceNumber: 1,
		KesPeriod:      0,
		Signature:      make([]byte, 64),
	}

	issuerVkey := make([]byte, 32)
	poolId := make([]byte, 28)

	// Test nil VRF signer
	builder := NewBlockBuilder(
		nil,
		kesSigner,
		opCert,
		poolId,
		issuerVkey,
		big.NewRat(1, 20),
	)

	input := BuildHeaderInput{
		Slot:          1,
		BlockNumber:   1,
		PrevHash:      make([]byte, 32),
		EpochNonce:    make([]byte, 32),
		PoolStake:     1000,
		TotalStake:    10000,
		BlockBodyHash: make([]byte, 32),
		BlockBodySize: 1024,
	}

	_, _, err = builder.BuildHeader(input)
	if err == nil {
		t.Error("expected error for nil VRF signer")
	}

	// Test nil KES signer
	builder = NewBlockBuilder(
		vrfSigner,
		nil,
		opCert,
		poolId,
		issuerVkey,
		big.NewRat(1, 20),
	)

	_, _, err = builder.BuildHeader(input)
	if err == nil {
		t.Error("expected error for nil KES signer")
	}

	// Test nil opCert
	builder = NewBlockBuilder(
		vrfSigner,
		kesSigner,
		nil,
		poolId,
		issuerVkey,
		big.NewRat(1, 20),
	)

	_, _, err = builder.BuildHeader(input)
	if err == nil {
		t.Error("expected error for nil opCert")
	}
}

// TestNewBlockBuilderWithMode verifies that the constructor stores the
// requested consensus mode and that the default constructor still
// defaults to CPRAOS (ConsensusModeCPraos is the zero value), preserving
// prior behavior for all existing callers.
func TestNewBlockBuilderWithMode(t *testing.T) {
	vrfSigner, err := NewSimpleVRFSigner(testVRFSeedBlock)
	if err != nil {
		t.Fatalf("failed to create VRF signer: %v", err)
	}
	kesSigner, err := NewSimpleKESSigner(testKESSeed)
	if err != nil {
		t.Fatalf("failed to create KES signer: %v", err)
	}
	opCert := &OperationalCert{
		HotVkey:        kesSigner.PublicKey(),
		SequenceNumber: 1,
		KesPeriod:      0,
		Signature:      make([]byte, 64),
	}
	issuerVkey := make([]byte, 32)
	poolId := make([]byte, 28)

	cpraosBuilder := NewBlockBuilder(
		vrfSigner,
		kesSigner,
		opCert,
		poolId,
		issuerVkey,
		big.NewRat(1, 20),
	)
	if cpraosBuilder.mode != ConsensusModeCPraos {
		t.Errorf(
			"expected default mode ConsensusModeCPraos, got %v",
			cpraosBuilder.mode,
		)
	}

	tpraosBuilder := NewBlockBuilderWithMode(
		vrfSigner,
		kesSigner,
		opCert,
		poolId,
		issuerVkey,
		big.NewRat(1, 20),
		ConsensusModeTPraos,
	)
	if tpraosBuilder.mode != ConsensusModeTPraos {
		t.Errorf(
			"expected mode ConsensusModeTPraos, got %v",
			tpraosBuilder.mode,
		)
	}
}

// tpraosEligibleTestSlot is a fixed slot number that is deterministically
// eligible for testVRFSeedBlock / a 32-byte epoch nonce of sequential
// bytes 0..31, with 100% relative stake and a 99/100 active slot
// coefficient. It was chosen so that leader eligibility is forced by
// construction (every one of slots 1-20 is eligible under these fixed,
// deterministic inputs), rather than by looping and skipping if no
// eligible slot is found: a genuine regression in the TPraos VRF input
// or threshold computation will make this slot ineligible and fail the
// test loudly instead of silently skipping it.
const tpraosEligibleTestSlot = 1

// TestBuildHeaderTPraosEligible builds a header using a TPraos-mode block
// builder and confirms the resulting VRF proof/output were constructed
// with the TPraos VRF input (MkSeedTPraos + seedL), not the CPraos input
// (MkInputVrf).
func TestBuildHeaderTPraosEligible(t *testing.T) {
	vrfSigner, err := NewSimpleVRFSigner(testVRFSeedBlock)
	if err != nil {
		t.Fatalf("failed to create VRF signer: %v", err)
	}
	kesSigner, err := NewSimpleKESSigner(testKESSeed)
	if err != nil {
		t.Fatalf("failed to create KES signer: %v", err)
	}

	opCert := &OperationalCert{
		HotVkey:        kesSigner.PublicKey(),
		SequenceNumber: 1,
		KesPeriod:      0,
		Signature:      make([]byte, 64),
	}

	issuerVkey := make([]byte, 32)
	for i := range issuerVkey {
		issuerVkey[i] = byte(i)
	}
	poolId := make([]byte, 28)

	// Use a high active slot coefficient and 100% relative stake so that
	// tpraosEligibleTestSlot is deterministically eligible.
	builder := NewBlockBuilderWithMode(
		vrfSigner,
		kesSigner,
		opCert,
		poolId,
		issuerVkey,
		big.NewRat(99, 100), // 99% active slots
		ConsensusModeTPraos,
	)

	epochNonce := make([]byte, 32)
	for i := range epochNonce {
		epochNonce[i] = byte(i)
	}

	prevHash := make([]byte, 32)
	bodyHash := make([]byte, 32)
	eligibleSlot := uint64(tpraosEligibleTestSlot)

	input := BuildHeaderInput{
		Slot:          eligibleSlot,
		BlockNumber:   eligibleSlot,
		PrevHash:      prevHash,
		EpochNonce:    epochNonce,
		PoolStake:     1000000000,
		TotalStake:    1000000000, // 100% stake
		BlockBodyHash: bodyHash,
		BlockBodySize: 1024,
		ProtoMajor:    2,
		ProtoMinor:    0,
	}

	header, result, err := builder.BuildHeader(input)
	if err != nil {
		t.Fatalf(
			"BuildHeader failed for deterministically eligible slot %d: %v",
			eligibleSlot,
			err,
		)
	}
	if header == nil {
		t.Fatal("expected non-nil header")
	}
	if result == nil || !result.Eligible {
		t.Fatal("expected eligible result")
	}

	// The VRF proof/output must verify under the TPraos VRF input
	// construction for the slot that was actually used.
	tpraosInput, err := vrf.MkSeedTPraos(
		int64(eligibleSlot),
		epochNonce,
		vrf.SeedL(),
	)
	if err != nil {
		t.Fatalf("vrf.MkSeedTPraos failed: %v", err)
	}
	valid, err := vrf.Verify(
		vrfSigner.PublicKey(),
		header.Body.VrfProof,
		header.Body.VrfOutput,
		tpraosInput,
	)
	if err != nil {
		t.Fatalf("vrf.Verify failed: %v", err)
	}
	if !valid {
		t.Error(
			"expected VRF proof to verify under TPraos VRF input construction",
		)
	}

	// It must NOT verify under the CPraos VRF input construction, proving
	// the TPraos-mode builder did not fall back to CPraos semantics. A
	// mismatched message can surface either as a verification failure or
	// as an error from the underlying ECVRF check, so either outcome
	// confirms the proof does not verify under CPraos semantics.
	cpraosInput, err := vrf.MkInputVrf(int64(eligibleSlot), epochNonce)
	if err != nil {
		t.Fatalf("vrf.MkInputVrf failed: %v", err)
	}
	crossValid, crossErr := vrf.Verify(
		vrfSigner.PublicKey(),
		header.Body.VrfProof,
		header.Body.VrfOutput,
		cpraosInput,
	)
	if crossErr == nil && crossValid {
		t.Error(
			"VRF proof unexpectedly verified under CPraos VRF input construction",
		)
	}

	// The nonce VRF certificate (bheaderEta, seedEta) must also be
	// present and must independently verify against the seedEta-based
	// TPraos VRF input -- it is required for epoch nonce evolution and is
	// distinct from the leader VRF certificate checked above.
	if len(header.Body.NonceVrfOutput) != 64 {
		t.Fatalf(
			"expected 64-byte nonce VRF output, got %d",
			len(header.Body.NonceVrfOutput),
		)
	}
	if len(header.Body.NonceVrfProof) != 80 {
		t.Fatalf(
			"expected 80-byte nonce VRF proof, got %d",
			len(header.Body.NonceVrfProof),
		)
	}
	if bytes.Equal(header.Body.NonceVrfOutput, header.Body.VrfOutput) {
		t.Error("expected nonce VRF output to differ from leader VRF output")
	}
	nonceVrfInput, err := vrf.MkSeedTPraos(
		int64(eligibleSlot),
		epochNonce,
		vrf.SeedEta(),
	)
	if err != nil {
		t.Fatalf("vrf.MkSeedTPraos (seedEta) failed: %v", err)
	}
	nonceValid, err := vrf.Verify(
		vrfSigner.PublicKey(),
		header.Body.NonceVrfProof,
		header.Body.NonceVrfOutput,
		nonceVrfInput,
	)
	if err != nil {
		t.Fatalf("vrf.Verify (nonce VRF) failed: %v", err)
	}
	if !nonceValid {
		t.Error(
			"expected nonce VRF proof to verify under seedEta-based TPraos VRF input",
		)
	}
}

// TestBuildHeaderTPraosSerializesFlat15ElementShape confirms that
// BuildHeader with ConsensusModeTPraos serializes the header body as the
// flat 15-element array used by ledger/shelley.ShelleyBlockHeaderBody
// (also shared unmodified by allegra/mary/alonzo), with two distinct
// nested [output, proof] VRF result sub-arrays (NonceVrf, then
// LeaderVrf) rather than the Babbage/Conway single-VRF 10-element shape.
func TestBuildHeaderTPraosSerializesFlat15ElementShape(t *testing.T) {
	vrfSigner, err := NewSimpleVRFSigner(testVRFSeedBlock)
	if err != nil {
		t.Fatalf("failed to create VRF signer: %v", err)
	}
	kesSigner, err := NewSimpleKESSigner(testKESSeed)
	if err != nil {
		t.Fatalf("failed to create KES signer: %v", err)
	}
	opCert := &OperationalCert{
		HotVkey:        kesSigner.PublicKey(),
		SequenceNumber: 1,
		KesPeriod:      0,
		Signature:      make([]byte, 64),
	}
	issuerVkey := make([]byte, 32)
	for i := range issuerVkey {
		issuerVkey[i] = byte(i)
	}
	poolId := make([]byte, 28)

	builder := NewBlockBuilderWithMode(
		vrfSigner,
		kesSigner,
		opCert,
		poolId,
		issuerVkey,
		big.NewRat(99, 100),
		ConsensusModeTPraos,
	)

	epochNonce := make([]byte, 32)
	for i := range epochNonce {
		epochNonce[i] = byte(i)
	}
	prevHash := make([]byte, 32)
	bodyHash := make([]byte, 32)

	header, _, err := builder.BuildHeader(BuildHeaderInput{
		Slot:          tpraosEligibleTestSlot,
		BlockNumber:   1,
		PrevHash:      prevHash,
		EpochNonce:    epochNonce,
		PoolStake:     1000000000,
		TotalStake:    1000000000,
		BlockBodyHash: bodyHash,
		BlockBodySize: 1024,
		ProtoMajor:    2,
		ProtoMinor:    0,
	})
	if err != nil {
		t.Fatalf("BuildHeader failed: %v", err)
	}

	bodyBytes, err := builder.serializeHeaderBody(&header.Body)
	if err != nil {
		t.Fatalf("serializeHeaderBody failed: %v", err)
	}

	var decoded []any
	if _, err := cbor.Decode(bodyBytes, &decoded); err != nil {
		t.Fatalf("failed to decode serialized TPraos header body: %v", err)
	}
	if len(decoded) != 15 {
		t.Fatalf(
			"expected flat 15-element TPraos header body array, got %d elements",
			len(decoded),
		)
	}

	// Element 5 (index 5, 0-based) is NonceVrf, element 6 is LeaderVrf.
	// Each must be its own 2-element [output, proof] sub-array, and the
	// two must be distinct VRF results (not the same field encoded
	// twice).
	nonceVrf, ok := decoded[5].([]any)
	if !ok || len(nonceVrf) != 2 {
		t.Fatalf(
			"expected NonceVrf as a 2-element sub-array, got %#v",
			decoded[5],
		)
	}
	leaderVrf, ok := decoded[6].([]any)
	if !ok || len(leaderVrf) != 2 {
		t.Fatalf(
			"expected LeaderVrf as a 2-element sub-array, got %#v",
			decoded[6],
		)
	}
	nonceOutput, ok := nonceVrf[0].([]byte)
	if !ok {
		t.Fatalf(
			"expected NonceVrf output to decode as bytes, got %#v",
			nonceVrf[0],
		)
	}
	leaderOutput, ok := leaderVrf[0].([]byte)
	if !ok {
		t.Fatalf(
			"expected LeaderVrf output to decode as bytes, got %#v",
			leaderVrf[0],
		)
	}
	if bytes.Equal(nonceOutput, leaderOutput) {
		t.Error("expected NonceVrf and LeaderVrf outputs to be distinct")
	}

	// OpCert and protocol version fields must be flat (not nested
	// sub-arrays), matching ShelleyBlockHeaderBody's field layout.
	for _, idx := range []int{9, 10, 11, 12, 13, 14} {
		if _, isArray := decoded[idx].([]any); isArray {
			t.Errorf(
				"expected element %d to be a flat field, got a nested array",
				idx,
			)
		}
	}
}

// TestBuildHeaderCPraosSerializesUnchanged10ElementShape is a regression
// test confirming that CPraos-mode (Babbage+) block building still
// produces the pre-existing 10-element header body shape with a single
// nested VRF result, unaffected by the TPraos-mode changes above.
func TestBuildHeaderCPraosSerializesUnchanged10ElementShape(t *testing.T) {
	vrfSigner, err := NewSimpleVRFSigner(testVRFSeedBlock)
	if err != nil {
		t.Fatalf("failed to create VRF signer: %v", err)
	}
	kesSigner, err := NewSimpleKESSigner(testKESSeed)
	if err != nil {
		t.Fatalf("failed to create KES signer: %v", err)
	}
	opCert := &OperationalCert{
		HotVkey:        kesSigner.PublicKey(),
		SequenceNumber: 1,
		KesPeriod:      0,
		Signature:      make([]byte, 64),
	}
	issuerVkey := make([]byte, 32)
	for i := range issuerVkey {
		issuerVkey[i] = byte(i)
	}
	poolId := make([]byte, 28)

	// Default constructor: CPraos mode.
	builder := NewBlockBuilder(
		vrfSigner,
		kesSigner,
		opCert,
		poolId,
		issuerVkey,
		big.NewRat(99, 100),
	)

	epochNonce := make([]byte, 32)
	for i := range epochNonce {
		epochNonce[i] = byte(i)
	}
	prevHash := make([]byte, 32)
	bodyHash := make([]byte, 32)

	header, _, err := builder.BuildHeader(BuildHeaderInput{
		Slot:          tpraosEligibleTestSlot,
		BlockNumber:   1,
		PrevHash:      prevHash,
		EpochNonce:    epochNonce,
		PoolStake:     1000000000,
		TotalStake:    1000000000,
		BlockBodyHash: bodyHash,
		BlockBodySize: 1024,
		ProtoMajor:    9,
		ProtoMinor:    0,
	})
	if err != nil {
		t.Fatalf("BuildHeader failed: %v", err)
	}

	// CPraos headers carry no nonce VRF certificate.
	if header.Body.NonceVrfOutput != nil || header.Body.NonceVrfProof != nil {
		t.Error("expected no nonce VRF fields for a CPraos-mode header")
	}

	bodyBytes, err := builder.serializeHeaderBody(&header.Body)
	if err != nil {
		t.Fatalf("serializeHeaderBody failed: %v", err)
	}

	var decoded []any
	if _, err := cbor.Decode(bodyBytes, &decoded); err != nil {
		t.Fatalf("failed to decode serialized CPraos header body: %v", err)
	}
	if len(decoded) != 10 {
		t.Fatalf(
			"expected unchanged 10-element CPraos header body array, got %d elements",
			len(decoded),
		)
	}
	vrfResult, ok := decoded[5].([]any)
	if !ok || len(vrfResult) != 2 {
		t.Fatalf(
			"expected a single 2-element VRF result sub-array at index 5, got %#v",
			decoded[5],
		)
	}
}

// TestBuildHeaderTPraosRoundTripsWithHeaderValidator builds a header with
// a TPraos-mode BlockBuilder and feeds the result directly into a
// TPraos-mode HeaderValidator, proving the block-building and
// header-validation halves of the TPraos support agree with each other
// (not just each independently exercising plausible-looking TPraos
// logic). This covers the full pipeline: leader VRF construction
// (seedL), the flat 15-element TPraos wire shape used for KES signing,
// the KES signature itself, and the OpCert signature.
func TestBuildHeaderTPraosRoundTripsWithHeaderValidator(t *testing.T) {
	vrfSigner, err := NewSimpleVRFSigner(testVRFSeedBlock)
	if err != nil {
		t.Fatalf("failed to create VRF signer: %v", err)
	}
	// Use the full Cardano KES depth here (unlike other BlockBuilder
	// tests, which use a shrunk test depth for speed): the validator
	// side enforces the real 448-byte Cardano KES signature size, so the
	// round trip needs a signer that actually produces that size.
	kesSk, kesPk, err := kes.KeyGen(
		kes.CardanoKesDepth,
		[]byte("test_kes_seed_for_tpraos_rtrip!!"),
	)
	if err != nil {
		t.Fatalf("kes.KeyGen failed: %v", err)
	}
	kesSigner := &SimpleKESSigner{sk: kesSk, publicKey: kesPk, period: 0}

	// Cold key signs the OpCert (hot_vkey || seq_num || kes_period).
	coldSeed := []byte("test_cold_key_for_tpraos_rtrip!!")
	coldPrivateKey := ed25519.NewKeyFromSeed(coldSeed)
	coldPublicKey := coldPrivateKey.Public().(ed25519.PublicKey)

	opCertSeqNum := uint32(1)
	opCertKesPeriod := uint32(0)
	opCertBody := common.OpCertSignableBytes(
		kesSigner.PublicKey(),
		uint64(opCertSeqNum),
		uint64(opCertKesPeriod),
	)
	opCert := &OperationalCert{
		HotVkey:        kesSigner.PublicKey(),
		SequenceNumber: opCertSeqNum,
		KesPeriod:      opCertKesPeriod,
		Signature:      ed25519.Sign(coldPrivateKey, opCertBody),
	}

	// Must match the validator's active slot coefficient below so both
	// sides agree on the leadership threshold.
	activeSlotCoeff := big.NewRat(99, 100)
	poolStake := uint64(1000000000)
	totalStake := uint64(1000000000)

	builder := NewBlockBuilderWithMode(
		vrfSigner,
		kesSigner,
		opCert,
		make([]byte, 28), // poolId
		coldPublicKey,    // issuerVkey
		activeSlotCoeff,
		ConsensusModeTPraos,
	)

	epochNonce := make([]byte, 32)
	for i := range epochNonce {
		epochNonce[i] = byte(i)
	}
	prevHash := make([]byte, 32)
	bodyHash := make([]byte, 32)

	header, leaderResult, err := builder.BuildHeader(BuildHeaderInput{
		Slot:          tpraosEligibleTestSlot,
		BlockNumber:   1,
		PrevHash:      prevHash,
		EpochNonce:    epochNonce,
		PoolStake:     poolStake,
		TotalStake:    totalStake,
		BlockBodyHash: bodyHash,
		BlockBodySize: 1024,
		ProtoMajor:    2,
		ProtoMinor:    0,
	})
	if err != nil {
		t.Fatalf(
			"BuildHeader failed for deterministically eligible slot %d: %v",
			tpraosEligibleTestSlot,
			err,
		)
	}
	if leaderResult == nil || !leaderResult.Eligible {
		t.Fatal("expected eligible leader result")
	}

	// Recompute the exact bytes that were KES-signed, the same way
	// BuildHeader does internally, so the validator can check the KES
	// signature against them.
	headerBodyCbor, err := builder.serializeHeaderBody(&header.Body)
	if err != nil {
		t.Fatalf("serializeHeaderBody failed: %v", err)
	}

	validatorConfig := NetworkConfig{
		ActiveSlotCoeff:   common.GenesisRat{Rat: activeSlotCoeff},
		SlotsPerKESPeriod: 129600,
		MaxKESEvolutions:  62,
	}
	validator := NewHeaderValidatorWithMode(
		validatorConfig,
		ConsensusModeTPraos,
	)

	input := &ValidateHeaderInput{
		Slot:                 header.Body.Slot,
		BlockNumber:          header.Body.BlockNumber,
		PrevHash:             header.Body.PrevHash,
		IssuerVkey:           header.Body.IssuerVkey,
		VrfKey:               header.Body.VrfKey,
		VrfProof:             header.Body.VrfProof,
		VrfOutput:            header.Body.VrfOutput,
		KesSignature:         header.Signature,
		HeaderBodyCbor:       headerBodyCbor,
		OpCertHotVkey:        header.Body.OpCertHotVkey,
		OpCertSequenceNumber: header.Body.OpCertSequenceNumber,
		OpCertKesPeriod:      header.Body.OpCertKesPeriod,
		OpCertSignature:      header.Body.OpCertSignature,
		PrevSlot:             0,
		PrevBlockNumber:      0,
		PrevHeaderHash:       prevHash,
		EpochNonce:           epochNonce,
		PoolStake:            poolStake,
		TotalStake:           totalStake,
	}

	result := validator.ValidateHeader(input)
	if !result.Valid {
		t.Fatalf(
			"expected BuildHeader's TPraos output to validate successfully under HeaderValidator's TPraos path, got errors: %v",
			result.Errors,
		)
	}
	if !bytes.Equal(result.VrfOutput, header.Body.VrfOutput) {
		t.Error(
			"expected returned VRF output to match the built header's leader VRF output",
		)
	}

	// Cross-check: the same header must NOT validate under a CPraos
	// validator, confirming build and validate are consistently
	// TPraos-specific on both sides, not accidentally cross-compatible.
	cpraosValidator := NewHeaderValidator(validatorConfig)
	cpraosResult := cpraosValidator.ValidateHeader(input)
	if cpraosResult.Valid {
		t.Error(
			"expected TPraos-built header to fail validation under a CPraos validator",
		)
	}
}

// TestCheckSlotLeadershipModeAffectsThreshold verifies that CPraos and
// TPraos modes compute leadership thresholds on different scales (2^256
// vs 2^512), confirming BlockBuilder threads its consensus mode through
// to CheckSlotLeadership.
func TestCheckSlotLeadershipModeAffectsThreshold(t *testing.T) {
	vrfSigner, err := NewSimpleVRFSigner(testVRFSeedBlock)
	if err != nil {
		t.Fatalf("failed to create VRF signer: %v", err)
	}
	kesSigner, err := NewSimpleKESSigner(testKESSeed)
	if err != nil {
		t.Fatalf("failed to create KES signer: %v", err)
	}
	opCert := &OperationalCert{
		HotVkey:        kesSigner.PublicKey(),
		SequenceNumber: 1,
		KesPeriod:      0,
		Signature:      make([]byte, 64),
	}
	issuerVkey := make([]byte, 32)
	poolId := make([]byte, 28)

	cpraosBuilder := NewBlockBuilder(
		vrfSigner,
		kesSigner,
		opCert,
		poolId,
		issuerVkey,
		big.NewRat(1, 20),
	)
	tpraosBuilder := NewBlockBuilderWithMode(
		vrfSigner,
		kesSigner,
		opCert,
		poolId,
		issuerVkey,
		big.NewRat(1, 20),
		ConsensusModeTPraos,
	)

	epochNonce := make([]byte, 32)

	cpraosResult, err := cpraosBuilder.CheckSlotLeadership(
		1000,
		epochNonce,
		500000000,
		1000000000,
	)
	if err != nil {
		t.Fatalf("CheckSlotLeadership (CPraos) failed: %v", err)
	}
	tpraosResult, err := tpraosBuilder.CheckSlotLeadership(
		1000,
		epochNonce,
		500000000,
		1000000000,
	)
	if err != nil {
		t.Fatalf("CheckSlotLeadership (TPraos) failed: %v", err)
	}

	if cpraosResult.Threshold.BitLen() > 256 {
		t.Errorf(
			"expected CPraos threshold to fit within 256 bits, got %d bits",
			cpraosResult.Threshold.BitLen(),
		)
	}
	if tpraosResult.Threshold.BitLen() <= 256 {
		t.Errorf(
			"expected TPraos threshold to exceed 256 bits, got %d bits",
			tpraosResult.Threshold.BitLen(),
		)
	}
}

func TestCheckSlotLeadership(t *testing.T) {
	vrfSigner, err := NewSimpleVRFSigner(testVRFSeedBlock)
	if err != nil {
		t.Fatalf("failed to create VRF signer: %v", err)
	}
	kesSigner, err := NewSimpleKESSigner(testKESSeed)
	if err != nil {
		t.Fatalf("failed to create KES signer: %v", err)
	}

	opCert := &OperationalCert{
		HotVkey:        kesSigner.PublicKey(),
		SequenceNumber: 1,
		KesPeriod:      0,
		Signature:      make([]byte, 64),
	}

	issuerVkey := make([]byte, 32)
	poolId := make([]byte, 28)

	builder := NewBlockBuilder(
		vrfSigner,
		kesSigner,
		opCert,
		poolId,
		issuerVkey,
		big.NewRat(1, 20),
	)

	epochNonce := make([]byte, 32)

	result, err := builder.CheckSlotLeadership(
		1000,
		epochNonce,
		500000000,
		1000000000,
	)
	if err != nil {
		t.Fatalf("CheckSlotLeadership failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Threshold == nil {
		t.Error("expected non-nil threshold")
	}
}

func TestComputeBlockBodyHash(t *testing.T) {
	body := []byte("test block body content")
	hash := ComputeBlockBodyHash(body)

	if len(hash) != 32 {
		t.Errorf("expected 32-byte hash, got %d", len(hash))
	}

	// Same input should produce same hash
	hash2 := ComputeBlockBodyHash(body)
	if !bytes.Equal(hash, hash2) {
		t.Error("hash should be deterministic")
	}

	// Different input should produce different hash
	hash3 := ComputeBlockBodyHash([]byte("different body"))
	if bytes.Equal(hash, hash3) {
		t.Error("different inputs should produce different hashes")
	}
}

func TestComputeVRFInput(t *testing.T) {
	epochNonce := make([]byte, 32)
	for i := range epochNonce {
		epochNonce[i] = byte(i)
	}

	input1 := ComputeVRFInput(1000, epochNonce)
	if len(input1) != 32 {
		t.Errorf("expected 32-byte VRF input, got %d", len(input1))
	}

	// Same parameters should produce same input
	input2 := ComputeVRFInput(1000, epochNonce)
	if !bytes.Equal(input1, input2) {
		t.Error("VRF input should be deterministic")
	}

	// Different slot should produce different input
	input3 := ComputeVRFInput(1001, epochNonce)
	if bytes.Equal(input1, input3) {
		t.Error("different slots should produce different VRF inputs")
	}
}

func TestHeaderBodySerialization(t *testing.T) {
	vrfSigner, err := NewSimpleVRFSigner(testVRFSeedBlock)
	if err != nil {
		t.Fatalf("failed to create VRF signer: %v", err)
	}
	kesSigner, err := NewSimpleKESSigner(testKESSeed)
	if err != nil {
		t.Fatalf("failed to create KES signer: %v", err)
	}

	opCert := &OperationalCert{
		HotVkey:        kesSigner.PublicKey(),
		SequenceNumber: 1,
		KesPeriod:      0,
		Signature:      make([]byte, 64),
	}

	issuerVkey := make([]byte, 32)
	poolId := make([]byte, 28)

	builder := NewBlockBuilder(
		vrfSigner,
		kesSigner,
		opCert,
		poolId,
		issuerVkey,
		big.NewRat(1, 20),
	)

	headerBody := &HeaderBody{
		BlockNumber:          100,
		Slot:                 1000,
		PrevHash:             make([]byte, 32),
		IssuerVkey:           issuerVkey,
		VrfKey:               vrfSigner.PublicKey(),
		VrfOutput:            make([]byte, 64),
		VrfProof:             make([]byte, 80),
		BlockBodySize:        2048,
		BlockBodyHash:        make([]byte, 32),
		OpCertHotVkey:        kesSigner.PublicKey(),
		OpCertSequenceNumber: 1,
		OpCertKesPeriod:      0,
		OpCertSignature:      make([]byte, 64),
		ProtoMajor:           9,
		ProtoMinor:           0,
	}

	encoded, err := builder.serializeHeaderBody(headerBody)
	if err != nil {
		t.Fatalf("serialization failed: %v", err)
	}

	if len(encoded) == 0 {
		t.Error("expected non-empty serialized header body")
	}

	// Should produce consistent output
	encoded2, err := builder.serializeHeaderBody(headerBody)
	if err != nil {
		t.Fatalf("second serialization failed: %v", err)
	}
	if !bytes.Equal(encoded, encoded2) {
		t.Error("serialization should be deterministic")
	}
}

func TestOperationalCertFields(t *testing.T) {
	kesSigner, err := NewSimpleKESSigner(testKESSeed)
	if err != nil {
		t.Fatalf("failed to create KES signer: %v", err)
	}

	opCert := &OperationalCert{
		HotVkey:        kesSigner.PublicKey(),
		SequenceNumber: 42,
		KesPeriod:      5,
		Signature:      make([]byte, 64),
	}

	if len(opCert.HotVkey) != 32 {
		t.Errorf("expected 32-byte hot vkey, got %d", len(opCert.HotVkey))
	}
	if opCert.SequenceNumber != 42 {
		t.Errorf("expected sequence number 42, got %d", opCert.SequenceNumber)
	}
	if opCert.KesPeriod != 5 {
		t.Errorf("expected KES period 5, got %d", opCert.KesPeriod)
	}
	if len(opCert.Signature) != 64 {
		t.Errorf("expected 64-byte signature, got %d", len(opCert.Signature))
	}
}
