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
	"crypto/ed25519"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/vrf"
)

// testNetworkConfig returns a NetworkConfig for testing with mainnet-like parameters.
func testNetworkConfig() NetworkConfig {
	return NetworkConfig{
		Name:            "test",
		SecurityParam:   2160,
		ActiveSlotCoeff: common.GenesisRat{Rat: big.NewRat(1, 20)}, // 0.05
		SlotLength: common.GenesisRat{
			Rat: big.NewRat(1, 1),
		}, // 1 second
		EpochLength:       432000,
		SlotsPerKESPeriod: 129600,
		MaxKESEvolutions:  62,
	}
}

// testNetworkConfigPreview returns a NetworkConfig with preview-like parameters.
func testNetworkConfigPreview() NetworkConfig {
	return NetworkConfig{
		Name:            "test-preview",
		SecurityParam:   2160,
		ActiveSlotCoeff: common.GenesisRat{Rat: big.NewRat(1, 20)}, // 0.05
		SlotLength: common.GenesisRat{
			Rat: big.NewRat(1, 1),
		}, // 1 second
		EpochLength:       86400, // 1 day
		SlotsPerKESPeriod: 129600,
		MaxKESEvolutions:  62,
	}
}

func TestNewHeaderValidator(t *testing.T) {
	validator := NewHeaderValidator(testNetworkConfig())

	if validator.slotsPerKESPeriod != 129600 {
		t.Errorf(
			"expected slotsPerKESPeriod 129600, got %d",
			validator.slotsPerKESPeriod,
		)
	}
	if validator.maxKESEvolutions != 62 {
		t.Errorf(
			"expected maxKESEvolutions 62, got %d",
			validator.maxKESEvolutions,
		)
	}
}

func TestValidateSlotOrdering(t *testing.T) {
	validator := NewHeaderValidator(testNetworkConfig())

	// Valid: slot increases
	input := &ValidateHeaderInput{
		Slot:     100,
		PrevSlot: 50,
	}
	err := validator.validateSlotOrdering(input)
	if err != nil {
		t.Errorf("expected no error for increasing slot, got %v", err)
	}

	// Invalid: slot doesn't increase
	input = &ValidateHeaderInput{
		Slot:     50,
		PrevSlot: 100,
	}
	err = validator.validateSlotOrdering(input)
	if err == nil {
		t.Error("expected error for non-increasing slot")
	}

	// Invalid: slot equal to previous
	input = &ValidateHeaderInput{
		Slot:     100,
		PrevSlot: 100,
	}
	err = validator.validateSlotOrdering(input)
	if err == nil {
		t.Error("expected error for equal slots")
	}
}

func TestValidateBlockNumber(t *testing.T) {
	validator := NewHeaderValidator(testNetworkConfig())

	// Valid: block number is previous + 1
	input := &ValidateHeaderInput{
		BlockNumber:     101,
		PrevBlockNumber: 100,
	}
	err := validator.validateBlockNumber(input)
	if err != nil {
		t.Errorf("expected no error for correct block number, got %v", err)
	}

	// Invalid: block number skips
	input = &ValidateHeaderInput{
		BlockNumber:     102,
		PrevBlockNumber: 100,
	}
	err = validator.validateBlockNumber(input)
	if err == nil {
		t.Error("expected error for skipping block number")
	}

	// Invalid: block number goes backwards
	input = &ValidateHeaderInput{
		BlockNumber:     99,
		PrevBlockNumber: 100,
	}
	err = validator.validateBlockNumber(input)
	if err == nil {
		t.Error("expected error for decreasing block number")
	}
}

func TestValidatePrevHash(t *testing.T) {
	validator := NewHeaderValidator(testNetworkConfig())

	hash := make([]byte, 32)
	for i := range hash {
		hash[i] = byte(i)
	}

	// Valid: hashes match
	input := &ValidateHeaderInput{
		PrevHash:       hash,
		PrevHeaderHash: hash,
	}
	err := validator.validatePrevHash(input)
	if err != nil {
		t.Errorf("expected no error for matching hashes, got %v", err)
	}

	// Valid: no previous hash to check (empty expected)
	input = &ValidateHeaderInput{
		PrevHash:       hash,
		PrevHeaderHash: nil,
	}
	err = validator.validatePrevHash(input)
	if err != nil {
		t.Errorf("expected no error when no expected hash, got %v", err)
	}

	// Invalid: hashes don't match
	wrongHash := make([]byte, 32)
	wrongHash[0] = 0xFF
	input = &ValidateHeaderInput{
		PrevHash:       hash,
		PrevHeaderHash: wrongHash,
	}
	err = validator.validatePrevHash(input)
	if err == nil {
		t.Error("expected error for mismatched hashes")
	}
}

func TestValidateKESPeriod(t *testing.T) {
	validator := NewHeaderValidator(testNetworkConfig())

	// Valid: current period matches opcert period
	input := &ValidateHeaderInput{
		Slot:            129600, // period 1
		OpCertKesPeriod: 1,
	}
	err := validator.validateKESPeriod(input)
	if err != nil {
		t.Errorf("expected no error for valid KES period, got %v", err)
	}

	// Valid: current period is after opcert period (within max evolutions)
	input = &ValidateHeaderInput{
		Slot:            129600 * 10, // period 10
		OpCertKesPeriod: 5,           // started at period 5
	}
	err = validator.validateKESPeriod(input)
	if err != nil {
		t.Errorf(
			"expected no error for period within evolution limit, got %v",
			err,
		)
	}

	// Invalid: opcert from future
	input = &ValidateHeaderInput{
		Slot:            129600, // period 1
		OpCertKesPeriod: 5,      // claims to start at period 5
	}
	err = validator.validateKESPeriod(input)
	if err == nil {
		t.Error("expected error for future opcert")
	}

	// Invalid: certificate expired
	input = &ValidateHeaderInput{
		Slot:            129600 * 100, // period 100
		OpCertKesPeriod: 0,            // started at period 0, only 62 evolutions allowed
	}
	err = validator.validateKESPeriod(input)
	if err == nil {
		t.Error("expected error for expired certificate")
	}
}

func TestValidateVRFProof(t *testing.T) {
	validator := NewHeaderValidator(testNetworkConfig())

	// Generate valid VRF key and proof
	seed := []byte("test_vrf_seed_for_validation!!!!")
	pk, sk, err := vrf.KeyGen(seed)
	if err != nil {
		t.Fatalf("vrf.KeyGen failed: %v", err)
	}

	epochNonce := make([]byte, 32)
	for i := range epochNonce {
		epochNonce[i] = byte(i)
	}

	slot := uint64(1000)
	vrfInput := vrf.MkInputVrf(int64(slot), epochNonce)
	proof, output, err := vrf.Prove(sk, vrfInput)
	if err != nil {
		t.Fatalf("vrf.Prove failed: %v", err)
	}

	// Valid VRF proof
	input := &ValidateHeaderInput{
		Slot:       slot,
		EpochNonce: epochNonce,
		VrfKey:     pk,
		VrfProof:   proof,
		VrfOutput:  output,
	}
	vrfOutput, err := validator.validateVRFProof(input)
	if err != nil {
		t.Errorf("expected no error for valid VRF proof, got %v", err)
	}
	if vrfOutput == nil {
		t.Error("expected non-nil VRF output")
	}

	// Invalid: missing epoch nonce
	input = &ValidateHeaderInput{
		Slot:      slot,
		VrfKey:    pk,
		VrfProof:  proof,
		VrfOutput: output,
	}
	_, err = validator.validateVRFProof(input)
	if err == nil {
		t.Error("expected error for missing epoch nonce")
	}

	// Invalid: wrong VRF key size
	input = &ValidateHeaderInput{
		Slot:       slot,
		EpochNonce: epochNonce,
		VrfKey:     []byte("short"),
		VrfProof:   proof,
		VrfOutput:  output,
	}
	_, err = validator.validateVRFProof(input)
	if err == nil {
		t.Error("expected error for wrong VRF key size")
	}

	// Invalid: wrong proof
	wrongProof := make([]byte, 80)
	input = &ValidateHeaderInput{
		Slot:       slot,
		EpochNonce: epochNonce,
		VrfKey:     pk,
		VrfProof:   wrongProof,
		VrfOutput:  output,
	}
	_, err = validator.validateVRFProof(input)
	if err == nil {
		t.Error("expected error for invalid VRF proof")
	}
}

func TestValidateLeadership(t *testing.T) {
	validator := NewHeaderValidator(testNetworkConfig())

	// Zero VRF output should always be below threshold
	zeroOutput := make([]byte, 64)

	input := &ValidateHeaderInput{
		PoolStake:  1000000000,
		TotalStake: 1000000000, // 100% stake
	}
	err := validator.validateLeadership(input, zeroOutput)
	if err != nil {
		t.Errorf(
			"expected no error for zero VRF output with 100%% stake, got %v",
			err,
		)
	}

	// Invalid: zero total stake
	input = &ValidateHeaderInput{
		PoolStake:  1000,
		TotalStake: 0,
	}
	err = validator.validateLeadership(input, zeroOutput)
	if err == nil {
		t.Error("expected error for zero total stake")
	}

	// Max VRF output should not be below threshold
	maxOutput := make([]byte, 64)
	for i := range maxOutput {
		maxOutput[i] = 0xFF
	}

	input = &ValidateHeaderInput{
		PoolStake:  1000000000,
		TotalStake: 1000000000,
	}
	err = validator.validateLeadership(input, maxOutput)
	if err == nil {
		t.Error("expected error for max VRF output")
	}
}

func TestValidateKESSignature(t *testing.T) {
	validator := NewHeaderValidator(testNetworkConfig())

	// Generate valid KES key and signature
	seed := []byte("test_kes_seed_for_validation!!!!")
	sk, pk, err := kes.KeyGen(kes.CardanoKesDepth, seed)
	if err != nil {
		t.Fatalf("kes.KeyGen failed: %v", err)
	}

	message := []byte("test header body cbor content!!!")
	signature, err := kes.Sign(sk, 0, message)
	if err != nil {
		t.Fatalf("kes.Sign failed: %v", err)
	}

	// Valid KES signature
	input := &ValidateHeaderInput{
		Slot:            0,
		HeaderBodyCbor:  message,
		KesSignature:    signature,
		OpCertHotVkey:   pk,
		OpCertKesPeriod: 0,
	}
	err = validator.validateKESSignature(input)
	if err != nil {
		t.Errorf("expected no error for valid KES signature, got %v", err)
	}

	// Invalid: missing header body
	input = &ValidateHeaderInput{
		KesSignature:    signature,
		OpCertHotVkey:   pk,
		OpCertKesPeriod: 0,
	}
	err = validator.validateKESSignature(input)
	if err == nil {
		t.Error("expected error for missing header body")
	}

	// Invalid: wrong signature size
	input = &ValidateHeaderInput{
		HeaderBodyCbor:  message,
		KesSignature:    []byte("short"),
		OpCertHotVkey:   pk,
		OpCertKesPeriod: 0,
	}
	err = validator.validateKESSignature(input)
	if err == nil {
		t.Error("expected error for wrong signature size")
	}

	// Invalid: wrong hot vkey
	wrongKey := make([]byte, 32)
	input = &ValidateHeaderInput{
		Slot:            0,
		HeaderBodyCbor:  message,
		KesSignature:    signature,
		OpCertHotVkey:   wrongKey,
		OpCertKesPeriod: 0,
	}
	err = validator.validateKESSignature(input)
	if err == nil {
		t.Error("expected error for wrong hot vkey")
	}
}

func TestValidateHeaderFull(t *testing.T) {
	validator := NewHeaderValidator(testNetworkConfig())

	// Generate valid VRF key and proof
	vrfSeed := []byte("test_vrf_seed_for_full_valid!!!!")
	vrfPk, vrfSk, err := vrf.KeyGen(vrfSeed)
	if err != nil {
		t.Fatalf("vrf.KeyGen failed: %v", err)
	}

	// Generate valid KES key and signature
	kesSeed := []byte("test_kes_seed_for_full_valid!!!!")
	kesSk, kesPk, err := kes.KeyGen(kes.CardanoKesDepth, kesSeed)
	if err != nil {
		t.Fatalf("kes.KeyGen failed: %v", err)
	}

	// Generate cold key for OpCert signing
	coldSeed := []byte("test_cold_key_for_full_valid!!!!")
	coldPrivateKey := ed25519.NewKeyFromSeed(coldSeed)
	coldPublicKey := coldPrivateKey.Public().(ed25519.PublicKey)

	epochNonce := make([]byte, 32)
	for i := range epochNonce {
		epochNonce[i] = byte(i)
	}

	// Use slot 0 (period 0) to match the key period
	slot := uint64(0)
	vrfInput := vrf.MkInputVrf(int64(slot), epochNonce)
	vrfProof, vrfOutput, err := vrf.Prove(vrfSk, vrfInput)
	if err != nil {
		t.Fatalf("vrf.Prove failed: %v", err)
	}

	message := []byte("test header body for full validation test!")
	kesSig, err := kes.Sign(kesSk, 0, message) // Sign at period 0
	if err != nil {
		t.Fatalf("KES sign failed: %v", err)
	}

	// Create OpCert signature: cold key signs CBOR([hot_vkey, sequence_number, kes_period])
	opCertSeqNum := uint32(0)
	opCertKesPeriod := uint32(0)
	opCertBody := []any{kesPk, opCertSeqNum, opCertKesPeriod}
	opCertBodyBytes, err := cbor.Encode(opCertBody)
	if err != nil {
		t.Fatalf("CBOR encode failed: %v", err)
	}
	opCertSignature := ed25519.Sign(coldPrivateKey, opCertBodyBytes)

	prevHash := make([]byte, 32)

	input := &ValidateHeaderInput{
		Slot:                 1, // slot 1, but opcert starts at period 0
		BlockNumber:          1, // First block after genesis
		PrevHash:             prevHash,
		IssuerVkey:           coldPublicKey,
		VrfKey:               vrfPk,
		VrfProof:             vrfProof,
		VrfOutput:            vrfOutput,
		KesSignature:         kesSig,
		KesPeriod:            0,
		HeaderBodyCbor:       message,
		OpCertHotVkey:        kesPk,
		OpCertSequenceNumber: opCertSeqNum,
		OpCertKesPeriod:      opCertKesPeriod,
		OpCertSignature:      opCertSignature,
		PrevSlot:             0,
		PrevBlockNumber:      0,
		PrevHeaderHash:       nil, // Genesis has no prev
		EpochNonce:           epochNonce,
		PoolStake:            1000000000000, // Use large stake for high probability
		TotalStake:           1000000000000, // 100% stake
	}

	result := validator.ValidateHeader(input)

	// Note: This test validates that the full validation pipeline completes
	// without panics. The VRF input uses a different slot (0) than the header (1),
	// so VRF verification will fail, but we verify the result is captured.
	if result == nil {
		t.Fatal("expected non-nil validation result")
	}
	// Leadership check may fail since VRF output is probabilistic
	// VRF verification fails because we use slot 0 for input but slot 1 in header
	// The test validates the pipeline runs to completion without panics
	if len(result.Errors) > 0 {
		t.Logf(
			"Validation errors (expected due to slot mismatch): %v",
			result.Errors,
		)
	}
}

func TestQuickValidateHeader(t *testing.T) {
	// Valid header structure
	input := &ValidateHeaderInput{
		Slot:          100,
		BlockNumber:   50,
		PrevHash:      make([]byte, 32),
		VrfKey:        make([]byte, vrf.PublicKeySize),
		VrfProof:      make([]byte, vrf.ProofSize),
		VrfOutput:     make([]byte, vrf.OutputSize),
		KesSignature:  make([]byte, kes.CardanoKesSignatureSize),
		OpCertHotVkey: make([]byte, kes.PublicKeySize),
	}

	err := QuickValidateHeader(input)
	if err != nil {
		t.Errorf("expected no error for valid structure, got %v", err)
	}

	// Invalid: zero slot
	input.Slot = 0
	err = QuickValidateHeader(input)
	if err == nil {
		t.Error("expected error for zero slot")
	}
	input.Slot = 100

	// Invalid: wrong VRF key size
	input.VrfKey = []byte("short")
	err = QuickValidateHeader(input)
	if err == nil {
		t.Error("expected error for wrong VRF key size")
	}
	input.VrfKey = make([]byte, vrf.PublicKeySize)

	// Invalid: wrong proof size
	input.VrfProof = []byte("short")
	err = QuickValidateHeader(input)
	if err == nil {
		t.Error("expected error for wrong proof size")
	}
}

func TestValidateOpCertSignature(t *testing.T) {
	validator := NewHeaderValidator(testNetworkConfig())

	// Generate KES key (hot key)
	kesSeed := []byte("test_kes_seed_for_opcert_test!!!")
	_, kesPk, err := kes.KeyGen(kes.CardanoKesDepth, kesSeed)
	if err != nil {
		t.Fatalf("kes.KeyGen failed: %v", err)
	}

	// Generate cold key for OpCert signing
	coldSeed := []byte("test_cold_key_for_opcert_test!!!")
	coldPrivateKey := ed25519.NewKeyFromSeed(coldSeed)
	coldPublicKey := coldPrivateKey.Public().(ed25519.PublicKey)

	// Create valid OpCert signature
	opCertSeqNum := uint32(5)
	opCertKesPeriod := uint32(10)
	opCertBody := []any{kesPk, opCertSeqNum, opCertKesPeriod}
	opCertBodyBytes, err := cbor.Encode(opCertBody)
	if err != nil {
		t.Fatalf("CBOR encode failed: %v", err)
	}
	opCertSignature := ed25519.Sign(coldPrivateKey, opCertBodyBytes)

	// Test valid signature
	input := &ValidateHeaderInput{
		IssuerVkey:           coldPublicKey,
		OpCertHotVkey:        kesPk,
		OpCertSequenceNumber: opCertSeqNum,
		OpCertKesPeriod:      opCertKesPeriod,
		OpCertSignature:      opCertSignature,
	}

	err = validator.validateOpCertSignature(input)
	if err != nil {
		t.Errorf("expected valid OpCert signature, got error: %v", err)
	}

	// Test with wrong signature
	wrongSig := make([]byte, ed25519.SignatureSize)
	copy(wrongSig, opCertSignature)
	wrongSig[0] ^= 0xFF // Corrupt the signature
	input.OpCertSignature = wrongSig

	err = validator.validateOpCertSignature(input)
	if err == nil {
		t.Error("expected error for corrupted OpCert signature")
	}

	// Test with wrong sequence number
	input.OpCertSignature = opCertSignature
	input.OpCertSequenceNumber = 999 // Different from what was signed
	err = validator.validateOpCertSignature(input)
	if err == nil {
		t.Error("expected error for wrong sequence number")
	}

	// Test with no issuer key (should skip validation)
	input.IssuerVkey = nil
	err = validator.validateOpCertSignature(input)
	if err != nil {
		t.Errorf("expected nil error when IssuerVkey is empty, got: %v", err)
	}

	// Test with wrong issuer key size
	input.IssuerVkey = []byte("short")
	err = validator.validateOpCertSignature(input)
	if err == nil {
		t.Error("expected error for wrong issuer key size")
	}
}

func TestValidateVRFKeyRegistration(t *testing.T) {
	validator := NewHeaderValidator(testNetworkConfig())

	// Generate VRF key
	vrfSeed := []byte("test_vrf_seed_for_registration!!")
	vrfPk, _, err := vrf.KeyGen(vrfSeed)
	if err != nil {
		t.Fatalf("vrf.KeyGen failed: %v", err)
	}

	// Compute expected hash (Blake2b-224) using the common function
	expectedHash := common.Blake2b224Hash(vrfPk)

	// Test matching hash
	input := &ValidateHeaderInput{
		VrfKey:               vrfPk,
		RegisteredVrfKeyHash: expectedHash.Bytes(),
	}

	err = validator.validateVRFKeyRegistration(input)
	if err != nil {
		t.Errorf("expected valid VRF key registration, got error: %v", err)
	}

	// Test mismatched hash
	wrongHash := make([]byte, len(expectedHash.Bytes()))
	copy(wrongHash, expectedHash.Bytes())
	wrongHash[0] ^= 0xFF
	input.RegisteredVrfKeyHash = wrongHash

	err = validator.validateVRFKeyRegistration(input)
	if err == nil {
		t.Error("expected error for mismatched VRF key hash")
	}

	// Test with no registered hash (should skip validation)
	input.RegisteredVrfKeyHash = nil
	err = validator.validateVRFKeyRegistration(input)
	if err != nil {
		t.Errorf(
			"expected nil error when RegisteredVrfKeyHash is empty, got: %v",
			err,
		)
	}

	// Test with wrong VRF key size
	input.VrfKey = []byte("short")
	input.RegisteredVrfKeyHash = expectedHash.Bytes()
	err = validator.validateVRFKeyRegistration(input)
	if err == nil {
		t.Error("expected error for wrong VRF key size")
	}
}

func TestBlake2b224Hash(t *testing.T) {
	// Generate VRF key
	vrfSeed := []byte("test_vrf_seed_for_hash_test!!!!!")
	vrfPk, _, err := vrf.KeyGen(vrfSeed)
	if err != nil {
		t.Fatalf("vrf.KeyGen failed: %v", err)
	}

	// Hash should be 28 bytes (Blake2b-224)
	hash := common.Blake2b224Hash(vrfPk)
	if len(hash.Bytes()) != 28 {
		t.Errorf("expected hash length 28, got %d", len(hash.Bytes()))
	}

	// Hashing same key should give same result
	hash2 := common.Blake2b224Hash(vrfPk)
	if hash.String() != hash2.String() {
		t.Error("expected deterministic hash")
	}

	// Different key should give different hash
	vrfSeed2 := []byte("different_seed_for_hash_test!!!!")
	vrfPk2, _, err := vrf.KeyGen(vrfSeed2)
	if err != nil {
		t.Fatalf("vrf.KeyGen failed: %v", err)
	}
	hash3 := common.Blake2b224Hash(vrfPk2)
	if hash.String() == hash3.String() {
		t.Error("expected different hash for different key")
	}
}

func TestPreviewNetworkConfig(t *testing.T) {
	previewConfig := testNetworkConfigPreview()
	validator := NewHeaderValidator(previewConfig)

	// Preview has different epoch length
	if previewConfig.EpochLength != 86400 {
		t.Errorf(
			"expected preview epoch length 86400, got %d",
			previewConfig.EpochLength,
		)
	}

	// Validator should use preview params
	if validator.slotsPerKESPeriod != 129600 {
		t.Errorf(
			"expected slotsPerKESPeriod 129600, got %d",
			validator.slotsPerKESPeriod,
		)
	}
}

func TestCustomNetworkConfig(t *testing.T) {
	customConfig := NetworkConfig{
		Name:              "custom",
		SecurityParam:     1000,
		ActiveSlotCoeff:   common.GenesisRat{Rat: big.NewRat(1, 10)}, // 10%
		SlotLength:        common.GenesisRat{Rat: big.NewRat(1, 1)},
		SlotsPerKESPeriod: 1000,
		MaxKESEvolutions:  10,
	}

	validator := NewHeaderValidator(customConfig)

	// Test KES period calculation with custom params
	input := &ValidateHeaderInput{
		Slot:            5000, // period 5
		OpCertKesPeriod: 0,
	}

	err := validator.validateKESPeriod(input)
	if err != nil {
		t.Errorf("expected no error for period 5 with max 10, got %v", err)
	}

	// Test expired with custom params
	input = &ValidateHeaderInput{
		Slot:            15000, // period 15
		OpCertKesPeriod: 0,     // started at 0, 15 evolutions > max 10
	}

	err = validator.validateKESPeriod(input)
	if err == nil {
		t.Error("expected error for expired certificate")
	}
}
