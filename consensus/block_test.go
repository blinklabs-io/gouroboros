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

package consensus

import (
	"bytes"
	"errors"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/kes"
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
