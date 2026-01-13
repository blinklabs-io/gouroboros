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
	"crypto/ed25519"
	"errors"
	"fmt"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/vrf"
)

// HeaderValidator provides consensus-level header validation.
type HeaderValidator struct {
	slotsPerKESPeriod uint64
	maxKESEvolutions  uint64
	activeSlotCoeff   *big.Rat
}

// NewHeaderValidator creates a new header validator.
func NewHeaderValidator(config NetworkConfig) *HeaderValidator {
	return &HeaderValidator{
		slotsPerKESPeriod: config.SlotsPerKESPeriod,
		maxKESEvolutions:  config.MaxKESEvolutions,
		activeSlotCoeff:   config.ActiveSlotCoeffRat(),
	}
}

// ValidateHeaderInput contains all information needed to validate a header.
type ValidateHeaderInput struct {
	// Header fields
	Slot           uint64
	BlockNumber    uint64
	PrevHash       []byte
	IssuerVkey     []byte
	VrfKey         []byte
	VrfProof       []byte
	VrfOutput      []byte
	KesSignature   []byte
	HeaderBodyCbor []byte

	// KesPeriod is reserved for future validation. Currently unused because
	// KES period is computed from Slot / SlotsPerKESPeriod in validation.
	// If headers expose a KesPeriod() method, this could be used to verify
	// the header's claimed period matches the computed value.
	KesPeriod uint64

	// OpCert fields
	OpCertHotVkey        []byte
	OpCertSequenceNumber uint32
	OpCertKesPeriod      uint32
	OpCertSignature      []byte

	// Previous header for chain validation
	PrevSlot        uint64
	PrevBlockNumber uint64
	PrevHeaderHash  []byte

	// Epoch nonce for VRF verification
	EpochNonce []byte

	// Stake information for leadership check
	PoolStake  uint64
	TotalStake uint64

	// Optional: registered VRF key hash for verification against pool registration
	// If provided, validates that VrfKey hashes to this value
	RegisteredVrfKeyHash []byte
}

// ValidateResult contains the result of header validation.
type ValidateResult struct {
	Valid     bool
	VrfOutput []byte
	Errors    []error
}

// ValidateHeader performs full consensus validation of a block header.
//
// Validation checks:
//  1. Slot strictly increases from previous block
//  2. Block number is previous + 1
//  3. PrevHash matches hash of previous header
//  4. VRF proof is valid
//  5. VRF output satisfies leadership threshold
//  6. KES period is within valid range
//  7. KES signature is valid
//  8. OpCert signature is valid (cold key signed the hot key)
//  9. VRF key matches pool registration (if RegisteredVrfKeyHash provided)
func (v *HeaderValidator) ValidateHeader(
	input *ValidateHeaderInput,
) *ValidateResult {
	result := &ValidateResult{
		Valid:  true,
		Errors: make([]error, 0),
	}

	// 1. Validate slot ordering
	if err := v.validateSlotOrdering(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	// 2. Validate block number
	if err := v.validateBlockNumber(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	// 3. Validate prev hash
	if err := v.validatePrevHash(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	// 4. Validate VRF proof
	vrfOutput, err := v.validateVRFProof(input)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	} else {
		result.VrfOutput = vrfOutput
	}

	// 5. Validate leadership eligibility
	if vrfOutput != nil {
		if err := v.validateLeadership(input, vrfOutput); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, err)
		}
	}

	// 6. Validate KES period
	if err := v.validateKESPeriod(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	// 7. Validate KES signature
	if err := v.validateKESSignature(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	// 8. Validate OpCert signature (cold key signed the hot key)
	if err := v.validateOpCertSignature(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	// 9. Validate VRF key matches pool registration (if provided)
	if err := v.validateVRFKeyRegistration(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	return result
}

// validateSlotOrdering checks that the slot strictly increases.
func (v *HeaderValidator) validateSlotOrdering(
	input *ValidateHeaderInput,
) error {
	if input.Slot <= input.PrevSlot {
		return fmt.Errorf(
			"slot must be greater than previous slot: current=%d, previous=%d",
			input.Slot,
			input.PrevSlot,
		)
	}
	return nil
}

// validateBlockNumber checks that block number is previous + 1.
func (v *HeaderValidator) validateBlockNumber(
	input *ValidateHeaderInput,
) error {
	expectedBlockNumber := input.PrevBlockNumber + 1
	if input.BlockNumber != expectedBlockNumber {
		return fmt.Errorf(
			"block number must be previous + 1: current=%d, expected=%d",
			input.BlockNumber,
			expectedBlockNumber,
		)
	}
	return nil
}

// validatePrevHash checks that prev hash matches.
func (v *HeaderValidator) validatePrevHash(input *ValidateHeaderInput) error {
	if len(input.PrevHeaderHash) > 0 &&
		!bytes.Equal(input.PrevHash, input.PrevHeaderHash) {
		return fmt.Errorf(
			"previous hash does not match: got %x, expected %x",
			input.PrevHash,
			input.PrevHeaderHash,
		)
	}
	return nil
}

// validateVRFProof verifies the VRF proof.
func (v *HeaderValidator) validateVRFProof(
	input *ValidateHeaderInput,
) ([]byte, error) {
	// Validate EpochNonce is exactly 32 bytes to prevent panic in vrf.MkInputVrf
	if len(input.EpochNonce) != 32 {
		return nil, fmt.Errorf(
			"epoch nonce must be 32 bytes, got %d",
			len(input.EpochNonce),
		)
	}

	if len(input.VrfKey) != vrf.PublicKeySize {
		return nil, fmt.Errorf(
			"invalid VRF key size: expected %d, got %d",
			vrf.PublicKeySize,
			len(input.VrfKey),
		)
	}

	if len(input.VrfProof) != vrf.ProofSize {
		return nil, fmt.Errorf(
			"invalid VRF proof size: expected %d, got %d",
			vrf.ProofSize,
			len(input.VrfProof),
		)
	}

	// Compute VRF input message
	// Slot numbers in Cardano are far below int64 max (mainnet ~100M, max ~9.2 quintillion)
	vrfInput := vrf.MkInputVrf(int64(input.Slot), input.EpochNonce) //nolint:gosec // G115: slot values safe

	// Verify VRF proof
	valid, err := vrf.Verify(
		input.VrfKey,
		input.VrfProof,
		input.VrfOutput,
		vrfInput,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"VRF verification failed at slot %d: %w",
			input.Slot,
			err,
		)
	}

	if !valid {
		return nil, fmt.Errorf(
			"VRF proof verification returned false at slot %d",
			input.Slot,
		)
	}

	return input.VrfOutput, nil
}

// validateLeadership checks if the VRF output satisfies the leadership threshold.
func (v *HeaderValidator) validateLeadership(
	input *ValidateHeaderInput,
	vrfOutput []byte,
) error {
	if input.TotalStake == 0 {
		return errors.New("total stake cannot be zero")
	}

	threshold := CertifiedNatThreshold(
		input.PoolStake,
		input.TotalStake,
		v.activeSlotCoeff,
	)
	if !IsVRFOutputBelowThreshold(vrfOutput, threshold) {
		return fmt.Errorf(
			"VRF output does not satisfy leadership threshold at slot %d",
			input.Slot,
		)
	}

	return nil
}

// validateKESPeriod checks if the KES period is valid.
func (v *HeaderValidator) validateKESPeriod(input *ValidateHeaderInput) error {
	if v.slotsPerKESPeriod == 0 {
		return errors.New("slotsPerKESPeriod cannot be zero")
	}

	currentKESPeriod := input.Slot / v.slotsPerKESPeriod
	opCertKESPeriod := uint64(input.OpCertKesPeriod)

	// OpCert cannot be from the future
	if currentKESPeriod < opCertKESPeriod {
		return fmt.Errorf(
			"operational certificate KES period is in the future: current=%d, opcert=%d",
			currentKESPeriod,
			opCertKESPeriod,
		)
	}

	// Check if certificate has expired
	evolutionPeriod := currentKESPeriod - opCertKESPeriod
	if evolutionPeriod >= v.maxKESEvolutions {
		return fmt.Errorf(
			"operational certificate has expired: evolution=%d, max=%d",
			evolutionPeriod,
			v.maxKESEvolutions,
		)
	}

	return nil
}

// validateKESSignature verifies the KES signature.
func (v *HeaderValidator) validateKESSignature(
	input *ValidateHeaderInput,
) error {
	if v.slotsPerKESPeriod == 0 {
		return errors.New("slotsPerKESPeriod cannot be zero")
	}

	if len(input.HeaderBodyCbor) == 0 {
		return errors.New("header body CBOR is required for KES verification")
	}

	if len(input.KesSignature) != kes.CardanoKesSignatureSize {
		return fmt.Errorf(
			"invalid KES signature size: expected %d, got %d",
			kes.CardanoKesSignatureSize,
			len(input.KesSignature),
		)
	}

	if len(input.OpCertHotVkey) != kes.PublicKeySize {
		return fmt.Errorf(
			"invalid KES hot vkey size: expected %d, got %d",
			kes.PublicKeySize,
			len(input.OpCertHotVkey),
		)
	}

	// Calculate evolution period
	currentKESPeriod := input.Slot / v.slotsPerKESPeriod
	opCertKESPeriod := uint64(input.OpCertKesPeriod)
	// Guard against underflow if OpCert KES period is in the future
	if currentKESPeriod < opCertKESPeriod {
		return fmt.Errorf(
			"operational certificate KES period is in the future: current=%d, opcert=%d",
			currentKESPeriod,
			opCertKESPeriod,
		)
	}
	evolutionPeriod := currentKESPeriod - opCertKESPeriod

	// Verify KES signature
	valid := kes.VerifySignedKES(
		input.OpCertHotVkey,
		evolutionPeriod,
		input.HeaderBodyCbor,
		input.KesSignature,
	)

	if !valid {
		return fmt.Errorf(
			"KES signature verification failed at slot %d",
			input.Slot,
		)
	}

	return nil
}

// validateOpCertSignature verifies the operational certificate signature.
// The cold key (IssuerVkey) must have signed the OpCert data (hot key, sequence, kes period).
func (v *HeaderValidator) validateOpCertSignature(
	input *ValidateHeaderInput,
) error {
	// Skip if no issuer key provided (validation may be done elsewhere)
	if len(input.IssuerVkey) == 0 {
		return nil
	}

	if len(input.IssuerVkey) != ed25519.PublicKeySize {
		return fmt.Errorf(
			"invalid issuer vkey size: expected %d, got %d",
			ed25519.PublicKeySize,
			len(input.IssuerVkey),
		)
	}

	if len(input.OpCertSignature) != ed25519.SignatureSize {
		return fmt.Errorf(
			"invalid OpCert signature size: expected %d, got %d",
			ed25519.SignatureSize,
			len(input.OpCertSignature),
		)
	}

	// The OpCert signature is over CBOR encoding of: [hot_vkey, sequence_number, kes_period]
	opCertBody := []any{
		input.OpCertHotVkey,
		input.OpCertSequenceNumber,
		input.OpCertKesPeriod,
	}

	opCertBodyBytes, err := cbor.Encode(opCertBody)
	if err != nil {
		return fmt.Errorf("failed to encode OpCert body: %w", err)
	}

	// Verify Ed25519 signature
	valid := ed25519.Verify(
		input.IssuerVkey,
		opCertBodyBytes,
		input.OpCertSignature,
	)
	if !valid {
		return errors.New("OpCert signature verification failed")
	}

	return nil
}

// validateVRFKeyRegistration verifies the VRF key matches the pool's registered key hash.
// This check is optional and only performed if RegisteredVrfKeyHash is provided.
func (v *HeaderValidator) validateVRFKeyRegistration(
	input *ValidateHeaderInput,
) error {
	// Skip if no registered hash provided
	if len(input.RegisteredVrfKeyHash) == 0 {
		return nil
	}

	if len(input.VrfKey) != vrf.PublicKeySize {
		return fmt.Errorf(
			"invalid VRF key size for registration check: expected %d, got %d",
			vrf.PublicKeySize,
			len(input.VrfKey),
		)
	}

	// Compute hash of the VRF key (Blake2b-224)
	vrfKeyHash := common.Blake2b224Hash(input.VrfKey)

	if !bytes.Equal(vrfKeyHash.Bytes(), input.RegisteredVrfKeyHash) {
		return fmt.Errorf(
			"VRF key does not match registered key hash: got %s, expected %x",
			vrfKeyHash.String(),
			input.RegisteredVrfKeyHash,
		)
	}

	return nil
}

// QuickValidateHeader performs a quick validation of header structure.
// This does not verify cryptographic proofs, only structural validity.
func QuickValidateHeader(input *ValidateHeaderInput) error {
	// Check required fields
	if input.Slot == 0 {
		return errors.New("slot cannot be zero")
	}
	if len(input.PrevHash) == 0 && input.BlockNumber > 1 {
		return errors.New("prev hash is required for blocks after genesis")
	}
	if len(input.VrfKey) != vrf.PublicKeySize {
		return fmt.Errorf(
			"vrf key must be %d bytes, got %d",
			vrf.PublicKeySize,
			len(input.VrfKey),
		)
	}
	if len(input.VrfProof) != vrf.ProofSize {
		return fmt.Errorf(
			"vrf proof must be %d bytes, got %d",
			vrf.ProofSize,
			len(input.VrfProof),
		)
	}
	if len(input.VrfOutput) != vrf.OutputSize {
		return fmt.Errorf(
			"vrf output must be %d bytes, got %d",
			vrf.OutputSize,
			len(input.VrfOutput),
		)
	}
	if len(input.KesSignature) != kes.CardanoKesSignatureSize {
		return fmt.Errorf(
			"kes signature must be %d bytes, got %d",
			kes.CardanoKesSignatureSize,
			len(input.KesSignature),
		)
	}
	if len(input.OpCertHotVkey) != kes.PublicKeySize {
		return fmt.Errorf(
			"opcert hot vkey must be %d bytes, got %d",
			kes.PublicKeySize,
			len(input.OpCertHotVkey),
		)
	}

	return nil
}
