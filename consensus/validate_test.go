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
	"crypto/ed25519"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/vrf"
	"github.com/stretchr/testify/require"
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
		BlockNumber:    2,
		PrevHash:       hash,
		PrevHeaderHash: hash,
	}
	err := validator.validatePrevHash(input)
	require.NoError(t, err)

	// Invalid: missing previous hash for non-genesis blocks
	input = &ValidateHeaderInput{
		BlockNumber:    1,
		PrevHash:       hash,
		PrevHeaderHash: nil,
	}
	err = validator.validatePrevHash(input)
	require.EqualError(
		t,
		err,
		"previous header hash is required for non-genesis blocks",
	)

	// Valid: genesis block may omit previous hash
	input = &ValidateHeaderInput{
		BlockNumber:    0,
		PrevHash:       hash,
		PrevHeaderHash: nil,
	}
	err = validator.validatePrevHash(input)
	require.NoError(t, err)

	// Invalid: hashes don't match
	wrongHash := make([]byte, 32)
	wrongHash[0] = 0xFF
	input = &ValidateHeaderInput{
		BlockNumber:    2,
		PrevHash:       hash,
		PrevHeaderHash: wrongHash,
	}
	err = validator.validatePrevHash(input)
	require.Error(t, err)
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
	vrfInput, err := vrf.MkInputVrf(int64(slot), epochNonce)
	if err != nil {
		t.Fatalf("vrf.MkInputVrf failed: %v", err)
	}
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

	// With CPRAOS, VRF output is first hashed with VrfLeaderValue (BLAKE2b-256
	// with "L" prefix) before comparison. The leader value is deterministic
	// but not directly related to the raw VRF output bytes.

	// Test with zero VRF output
	zeroOutput := make([]byte, 64)
	leaderValue := VrfLeaderValue(zeroOutput)
	leaderValueInt := new(big.Int).SetBytes(leaderValue)

	// Compute threshold for 100% stake with mainnet active slot coefficient
	activeSlotCoeff := big.NewRat(1, 20) // 0.05
	threshold := CertifiedNatThreshold(1000000000, 1000000000, activeSlotCoeff)
	expectedEligible := leaderValueInt.Cmp(threshold) < 0

	input := &ValidateHeaderInput{
		PoolStake:  1000000000,
		TotalStake: 1000000000, // 100% stake
	}
	err := validator.validateLeadership(input, zeroOutput)

	if expectedEligible {
		require.NoError(
			t,
			err,
			"expected no error for eligible VRF output with 100%% stake",
		)
	} else {
		require.Error(t, err, "expected error for non-eligible VRF output")
	}

	// Invalid: zero total stake
	input = &ValidateHeaderInput{
		PoolStake:  1000,
		TotalStake: 0,
	}
	err = validator.validateLeadership(input, zeroOutput)
	require.Error(t, err, "expected error for zero total stake")

	// Test with max VRF output - the leader value is a hash, not max value
	maxOutput := make([]byte, 64)
	for i := range maxOutput {
		maxOutput[i] = 0xFF
	}
	maxLeaderValue := VrfLeaderValue(maxOutput)
	maxLeaderValueInt := new(big.Int).SetBytes(maxLeaderValue)
	maxExpectedEligible := maxLeaderValueInt.Cmp(threshold) < 0

	input = &ValidateHeaderInput{
		PoolStake:  1000000000,
		TotalStake: 1000000000,
	}
	err = validator.validateLeadership(input, maxOutput)

	if maxExpectedEligible {
		require.NoError(t, err, "expected no error for eligible max VRF output")
	} else {
		require.Error(t, err, "expected error for non-eligible max VRF output")
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
	vrfInput, err := vrf.MkInputVrf(int64(slot), epochNonce)
	if err != nil {
		t.Fatalf("vrf.MkInputVrf failed: %v", err)
	}
	vrfProof, vrfOutput, err := vrf.Prove(vrfSk, vrfInput)
	if err != nil {
		t.Fatalf("vrf.Prove failed: %v", err)
	}

	message := []byte("test header body for full validation test!")
	kesSig, err := kes.Sign(kesSk, 0, message) // Sign at period 0
	if err != nil {
		t.Fatalf("KES sign failed: %v", err)
	}

	// Create OpCert signature: cold key signs the raw OCertSignable
	// representation (hot_vkey || sequence_number || kes_period).
	opCertSeqNum := uint32(0)
	opCertKesPeriod := uint32(0)
	opCertBody := common.OpCertSignableBytes(
		kesPk,
		uint64(opCertSeqNum),
		uint64(opCertKesPeriod),
	)
	opCertSignature := ed25519.Sign(coldPrivateKey, opCertBody)

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

	// Create valid OpCert signature over the raw OCertSignable representation
	// (hot_vkey || sequence_number || kes_period).
	opCertSeqNum := uint32(5)
	opCertKesPeriod := uint32(10)
	opCertBody := common.OpCertSignableBytes(
		kesPk,
		uint64(opCertSeqNum),
		uint64(opCertKesPeriod),
	)
	opCertSignature := ed25519.Sign(coldPrivateKey, opCertBody)

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

	// Test with no issuer key (should require IssuerVkey)
	input.IssuerVkey = nil
	err = validator.validateOpCertSignature(input)
	if err == nil {
		t.Error("expected error when IssuerVkey is empty")
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

	// Compute expected hash using the canonical ledger registration width.
	expectedHash := common.Blake2b256Hash(vrfPk)

	// Test matching hash
	input := &ValidateHeaderInput{
		VrfKey:               vrfPk,
		RegisteredVrfKeyHash: expectedHash.Bytes(),
	}

	err = validator.validateVRFKeyRegistration(input)
	if err != nil {
		t.Errorf("expected valid VRF key registration, got error: %v", err)
	}

	legacyHash := common.Blake2b224Hash(vrfPk)
	input.RegisteredVrfKeyHash = legacyHash.Bytes()
	err = validator.validateVRFKeyRegistration(input)
	if err == nil {
		t.Error("expected error for legacy 28-byte VRF key hash")
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

// TestNewHeaderValidatorWithMode verifies that the constructor stores the
// requested consensus mode and that the default constructor still defaults
// to CPRAOS (ConsensusModeCPraos is the zero value), preserving prior
// behavior for all existing callers.
func TestNewHeaderValidatorWithMode(t *testing.T) {
	cpraosValidator := NewHeaderValidator(testNetworkConfig())
	require.Equal(t, ConsensusModeCPraos, cpraosValidator.mode)

	tpraosValidator := NewHeaderValidatorWithMode(
		testNetworkConfig(),
		ConsensusModeTPraos,
	)
	require.Equal(t, ConsensusModeTPraos, tpraosValidator.mode)
}

// TestValidateVRFProofTPraos exercises the TPraos-mode VRF input
// construction (MkSeedTPraos with the seedL constant), as used by
// Shelley through Alonzo headers.
func TestValidateVRFProofTPraos(t *testing.T) {
	validator := NewHeaderValidatorWithMode(
		testNetworkConfig(),
		ConsensusModeTPraos,
	)

	seed := []byte("test_vrf_seed_for_tpraos_valid!!")
	pk, sk, err := vrf.KeyGen(seed)
	require.NoError(t, err)

	epochNonce := make([]byte, 32)
	for i := range epochNonce {
		epochNonce[i] = byte(i)
	}

	slot := uint64(1000)
	vrfInput, err := vrf.MkSeedTPraos(int64(slot), epochNonce, vrf.SeedL())
	require.NoError(t, err)
	proof, output, err := vrf.Prove(sk, vrfInput)
	require.NoError(t, err)

	// Valid: TPraos-constructed proof verifies under a TPraos validator.
	input := &ValidateHeaderInput{
		Slot:       slot,
		EpochNonce: epochNonce,
		VrfKey:     pk,
		VrfProof:   proof,
		VrfOutput:  output,
	}
	gotOutput, err := validator.validateVRFProof(input)
	require.NoError(t, err)
	require.Equal(t, output, gotOutput)

	// Invalid: the same proof does not verify under a CPRAOS validator,
	// because the CPRAOS and TPraos VRF input constructions differ.
	cpraosValidator := NewHeaderValidator(testNetworkConfig())
	_, err = cpraosValidator.validateVRFProof(input)
	require.Error(t, err)

	// Invalid: a CPRAOS-constructed proof for the same key/slot/nonce does
	// not verify under a TPraos validator.
	cpraosInput, err := vrf.MkInputVrf(int64(slot), epochNonce)
	require.NoError(t, err)
	cpraosProof, cpraosOutput, err := vrf.Prove(sk, cpraosInput)
	require.NoError(t, err)
	crossInput := &ValidateHeaderInput{
		Slot:       slot,
		EpochNonce: epochNonce,
		VrfKey:     pk,
		VrfProof:   cpraosProof,
		VrfOutput:  cpraosOutput,
	}
	_, err = validator.validateVRFProof(crossInput)
	require.Error(t, err)
}

// TestValidateLeadershipTPraos exercises the TPraos leadership threshold
// semantics: the raw 64-byte VRF output is compared directly (no
// BLAKE2b-256 "L"-prefix hashing) against a threshold derived from 2^512.
func TestValidateLeadershipTPraos(t *testing.T) {
	validator := NewHeaderValidatorWithMode(
		testNetworkConfig(),
		ConsensusModeTPraos,
	)

	activeSlotCoeff := big.NewRat(1, 20) // 0.05
	threshold, err := CertifiedNatThresholdWithMode(
		1000000000,
		1000000000,
		activeSlotCoeff,
		ConsensusModeTPraos,
	)
	require.NoError(t, err)

	// Valid: zero VRF output is always numerically below any positive
	// threshold, so it must be accepted as eligible under TPraos, where
	// the raw output is compared directly (no leader-value hashing).
	zeroOutput := make([]byte, 64)
	input := &ValidateHeaderInput{
		PoolStake:  1000000000,
		TotalStake: 1000000000, // 100% stake
	}
	err = validator.validateLeadership(input, zeroOutput)
	require.NoError(t, err)
	require.Positive(
		t,
		threshold.Sign(),
		"threshold should be positive for nonzero stake",
	)

	// Invalid: the maximum possible VRF output (2^512 - 1) can never be
	// below a threshold that is strictly less than 2^512.
	maxOutput := make([]byte, 64)
	for i := range maxOutput {
		maxOutput[i] = 0xFF
	}
	err = validator.validateLeadership(input, maxOutput)
	require.Error(t, err)

	// Invalid: zero total stake.
	input = &ValidateHeaderInput{
		PoolStake:  1000,
		TotalStake: 0,
	}
	err = validator.validateLeadership(input, zeroOutput)
	require.Error(t, err)
}

// TestValidateHeaderFullTPraosValid builds a fully valid Shelley-era
// (TPraos) header end-to-end -- VRF proof, KES signature, and OpCert
// signature -- and confirms ValidateHeader accepts it when the validator
// is configured for ConsensusModeTPraos.
func TestValidateHeaderFullTPraosValid(t *testing.T) {
	validator := NewHeaderValidatorWithMode(
		testNetworkConfig(),
		ConsensusModeTPraos,
	)

	vrfSigner, err := NewSimpleVRFSigner(
		[]byte("test_vrf_seed_for_tpraos_full!!!"),
	)
	require.NoError(t, err)

	kesSeed := []byte("test_kes_seed_for_tpraos_full!!!")
	kesSk, kesPk, err := kes.KeyGen(kes.CardanoKesDepth, kesSeed)
	require.NoError(t, err)

	coldSeed := []byte("test_cold_key_for_tpraos_full!!!")
	coldPrivateKey := ed25519.NewKeyFromSeed(coldSeed)
	coldPublicKey := coldPrivateKey.Public().(ed25519.PublicKey)

	epochNonce := make([]byte, 32)
	for i := range epochNonce {
		epochNonce[i] = byte(i)
	}

	// Use the validator's own active slot coefficient (from
	// testNetworkConfig) and 100% relative stake so the leadership check
	// performed below by validateLeadership uses the same threshold used
	// here. slot 5 is a fixed, deterministically-eligible slot for this
	// exact seed/epochNonce/stake/coefficient combination -- chosen so a
	// genuine regression in the TPraos VRF input or threshold computation
	// fails this test loudly instead of silently skipping it.
	activeSlotCoeff := validator.activeSlotCoeff
	poolStake := uint64(1000000000)
	totalStake := uint64(1000000000)
	const slot = uint64(5)

	leaderResult, err := IsSlotLeaderWithMode(
		slot,
		epochNonce,
		poolStake,
		totalStake,
		activeSlotCoeff,
		vrfSigner,
		ConsensusModeTPraos,
	)
	require.NoError(t, err)
	require.True(
		t,
		leaderResult.Eligible,
		"expected slot %d to be deterministically eligible",
		slot,
	)
	vrfProof := leaderResult.Proof
	vrfOutput := leaderResult.Output

	// The nonce VRF certificate (bheaderEta, seedEta) is independent of
	// the leader VRF (seedL) checked above and must also be present and
	// valid for a full TPraos header to validate.
	nonceVrfInput, err := vrf.MkSeedTPraos(
		int64(slot),
		epochNonce,
		vrf.SeedEta(),
	)
	require.NoError(t, err)
	nonceVrfProof, nonceVrfOutput, err := vrfSigner.Prove(nonceVrfInput)
	require.NoError(t, err)

	// slot is well within KES period 0 given the default network config's
	// SlotsPerKESPeriod, so sign at period 0 to match OpCertKesPeriod.
	message := []byte("test header body for tpraos full validation!!!")
	kesSig, err := kes.Sign(kesSk, 0, message)
	require.NoError(t, err)

	opCertSeqNum := uint32(0)
	opCertKesPeriod := uint32(0)
	opCertBody := common.OpCertSignableBytes(
		kesPk,
		uint64(opCertSeqNum),
		uint64(opCertKesPeriod),
	)
	opCertSignature := ed25519.Sign(coldPrivateKey, opCertBody)

	prevHash := make([]byte, 32)
	input := &ValidateHeaderInput{
		Slot:                 slot,
		BlockNumber:          1,
		PrevHash:             prevHash,
		IssuerVkey:           coldPublicKey,
		VrfKey:               vrfSigner.PublicKey(),
		VrfProof:             vrfProof,
		VrfOutput:            vrfOutput,
		NonceVrfProof:        nonceVrfProof,
		NonceVrfOutput:       nonceVrfOutput,
		KesSignature:         kesSig,
		HeaderBodyCbor:       message,
		OpCertHotVkey:        kesPk,
		OpCertSequenceNumber: opCertSeqNum,
		OpCertKesPeriod:      opCertKesPeriod,
		OpCertSignature:      opCertSignature,
		PrevSlot:             0,
		PrevBlockNumber:      0,
		PrevHeaderHash:       prevHash, // matches PrevHash for a valid non-genesis header
		EpochNonce:           epochNonce,
		PoolStake:            poolStake,
		TotalStake:           totalStake,
	}

	result := validator.ValidateHeader(input)
	require.True(
		t,
		result.Valid,
		"unexpected validation errors: %v",
		result.Errors,
	)
	require.Empty(t, result.Errors)
	require.Equal(t, vrfOutput, result.VrfOutput)

	// Invalid: the identical header data fails VRF verification when
	// validated under CPraos mode, because CPraos and TPraos use different
	// VRF input constructions and threshold interpretations.
	cpraosValidator := NewHeaderValidator(testNetworkConfig())
	cpraosResult := cpraosValidator.ValidateHeader(input)
	require.False(t, cpraosResult.Valid)
	require.NotEmpty(t, cpraosResult.Errors)
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
