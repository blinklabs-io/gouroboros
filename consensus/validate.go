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
	"fmt"
	"math/big"

	"github.com/blinklabs-io/gouroboros/internal/ed25519strict"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/vrf"
)

// HeaderValidator provides consensus-level header validation.
type HeaderValidator struct {
	slotsPerKESPeriod uint64
	maxKESEvolutions  uint64
	activeSlotCoeff   *big.Rat
	mode              ConsensusMode
	tachys            *TachysLeaderSelector
}

// NewTachysHeaderValidator creates a CPraos-format header validator using a
// public CIP-0177 leader schedule. VRF proof, KES, operational certificate,
// and registered VRF key validation remain enabled.
func NewTachysHeaderValidator(
	config NetworkConfig,
	selector *TachysLeaderSelector,
) (*HeaderValidator, error) {
	if selector == nil || len(selector.intervals) == 0 ||
		selector.totalStake == 0 {
		return nil, errors.New("tachys leader selector is uninitialized")
	}
	validator := NewHeaderValidatorWithMode(config, ConsensusModeCPraos)
	validator.tachys = selector
	return validator, nil
}

// NewHeaderValidator creates a new header validator using CPRAOS consensus
// (Babbage and later eras). For Shelley through Alonzo (TPraos) headers,
// use NewHeaderValidatorWithMode with ConsensusModeTPraos.
func NewHeaderValidator(config NetworkConfig) *HeaderValidator {
	return NewHeaderValidatorWithMode(config, ConsensusModeCPraos)
}

// NewHeaderValidatorWithMode creates a new header validator for the given
// consensus mode. Use ConsensusModeTPraos for Shelley through Alonzo era
// headers and ConsensusModeCPraos for Babbage and later era headers.
func NewHeaderValidatorWithMode(
	config NetworkConfig,
	mode ConsensusMode,
) *HeaderValidator {
	return &HeaderValidator{
		slotsPerKESPeriod: config.SlotsPerKESPeriod,
		maxKESEvolutions:  config.MaxKESEvolutions,
		activeSlotCoeff:   config.ActiveSlotCoeffRat(),
		mode:              mode,
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

	// NonceVrfProof and NonceVrfOutput carry the TPraos-only nonce VRF
	// certificate (bheaderEta, seedEta = mkNonceFromNumber(0)), which
	// drives epoch nonce evolution and is independent of the leader VRF
	// (VrfProof/VrfOutput, seedL) verified above. They are TPraos-only:
	// leave unset (nil) for ConsensusModeCPraos headers, which have no
	// separate nonce VRF field; validateNonceVRFProof is a no-op in that
	// mode.
	NonceVrfProof  []byte
	NonceVrfOutput []byte

	// KesPeriod is reserved for future validation. Currently unused because
	// KES period is computed from Slot / SlotsPerKESPeriod in validation.
	// If headers expose a KesPeriod() method, this could be used to verify
	// the header's claimed period matches the computed value.
	KesPeriod uint64

	// OpCert fields
	OpCertHotVkey        []byte
	OpCertSequenceNumber uint64
	OpCertKesPeriod      uint64
	OpCertSignature      []byte

	// OpCertCounterState is the authoritative counter for IssuerVkey
	// before this header. Use a pointer to zero when the issuer has no entry.
	// Header validation fails when the state is unavailable.
	OpCertCounterState *uint64

	// Previous header for chain validation
	PrevSlot        uint64
	PrevBlockNumber uint64
	PrevHeaderHash  []byte

	// Epoch nonce for VRF verification
	EpochNonce []byte

	// Stake information for leadership check
	PoolStake  uint64
	TotalStake uint64

	// RegisteredVrfKeyHash is the VRF hash in the active leader distribution
	// for Slot. It must come from the same snapshot as PoolStake and TotalStake,
	// not the current pool registration. Required for full header validation,
	// except in a TPraos overlay slot, which no pool may lead.
	RegisteredVrfKeyHash []byte

	// Envelope carries the protocol version limit and the header and body
	// size limits. Required in every mode.
	Envelope *EnvelopeInput

	// Overlay carries the TPraos overlay schedule inputs. Required for
	// ConsensusModeTPraos; ignored for ConsensusModeCPraos.
	Overlay *TPraosOverlayInput
}

// ValidateResult contains the result of header validation.
type ValidateResult struct {
	Valid             bool
	VrfOutput         []byte
	NextOpCertCounter *uint64
	Errors            []error
}

// ValidateHeader performs full consensus validation of a block header.
//
// If an input the mode requires is missing (Envelope always, Overlay for
// ConsensusModeTPraos), the result holds a single error wrapping
// ErrMissingHeaderInput and no other check runs.
//
// Otherwise every check runs and every failure is returned in Errors, in
// the order the reference validator checks them, so the first error is the
// one the reference would report:
//
//  1. The envelope: block number is previous + 1, slot strictly increases,
//     PrevHash matches the previous header's hash, and the protocol
//     version, header size and body size are within their limits.
//  2. CPraos: the KES side, then the VRF side. TPraos: the VRF side, then
//     the KES side.
//
// The KES side checks that the KES period is within the operational
// certificate's validity, the cold key's signature over the operational
// certificate, the KES signature, and the operational certificate counter
// against authoritative pool state. The VRF side checks that the VRF key
// matches the active leader snapshot hash, the TPraos-only nonce VRF proof,
// the leader VRF proof, and finally that the verified output satisfies
// finite E34 eligibility and pool policy. Eligibility is not checked when
// the VRF key or the leader VRF proof failed.
//
// TPraos only: a header in a non-active overlay slot fails with
// ErrNotActiveSlot and its VRF side is not checked. In an active overlay
// slot the VRF side checks that the issuer is the scheduled genesis
// delegate and the VRF key is that delegate's, then both VRF proofs; it
// checks no eligibility.
func (v *HeaderValidator) ValidateHeader(
	input *ValidateHeaderInput,
) *ValidateResult {
	if err := v.checkRequiredInputs(input); err != nil {
		return &ValidateResult{Errors: []error{err}}
	}
	result := &ValidateResult{
		Valid:  true,
		Errors: make([]error, 0),
	}
	failed := func(err error) bool {
		if err == nil {
			return false
		}
		result.Valid = false
		result.Errors = append(result.Errors, err)
		return true
	}

	failed(v.validateBlockNumber(input))
	failed(v.validateSlotOrdering(input))
	failed(v.validatePrevHash(input))

	for _, err := range validateEnvelopeLimits(input.Envelope) {
		failed(err)
	}

	overlay := v.overlaySlotFor(input)
	var nextOpCertSequence uint64
	kesSide := func() {
		failed(v.validateKESPeriod(input))
		failed(v.validateOpCertSignature(input))
		failed(v.validateKESSignature(input))
		next, err := v.validateOpCertSequence(input)
		if !failed(err) {
			nextOpCertSequence = next
		}
	}
	// A non-active overlay slot fails outright and runs no VRF check; an
	// active one belongs to a genesis delegate, which has no stake, so
	// leadership is not checked (Rules/Overlay.hs overlayTransition).
	vrfSide := func() {
		var keyFailed bool
		switch overlay.kind {
		case overlaySlotNonActive:
			failed(fmt.Errorf("%w: slot %d", ErrNotActiveSlot, input.Slot))
			return
		case overlaySlotActive:
			failed(validateGenesisColdKey(input, overlay.delegation))
			keyFailed = failed(
				validateGenesisVRFKey(input, overlay.delegation),
			)
		case overlaySlotPraos:
			keyFailed = failed(v.validateVRFKeyRegistration(input))
		}
		failed(v.validateNonceVRFProof(input))
		vrfOutput, err := v.validateVRFProof(input)
		if failed(err) {
			return
		}
		result.VrfOutput = vrfOutput
		if !keyFailed && overlay.kind == overlaySlotPraos {
			failed(v.validateLeadership(input, vrfOutput))
		}
	}
	// The reference's order differs by protocol: Praos validates the KES
	// side first (PolyPraos.updateChainDepStatePolyPraos), TPraos the VRF
	// side, in the OVERLAY rule, before the OCERT rule.
	if v.mode == ConsensusModeTPraos {
		vrfSide()
		kesSide()
	} else {
		kesSide()
		vrfSide()
	}

	if result.Valid {
		result.NextOpCertCounter = &nextOpCertSequence
	}

	return result
}

func (v *HeaderValidator) validateOpCertSequence(
	input *ValidateHeaderInput,
) (uint64, error) {
	if input.OpCertCounterState == nil {
		return 0, errors.New(
			"previous operational certificate counter is required for header validation",
		)
	}
	previous := *input.OpCertCounterState
	current := input.OpCertSequenceNumber
	if current < previous {
		return 0, fmt.Errorf(
			"operational certificate counter rolled back: current=%d, previous=%d",
			current,
			previous,
		)
	}
	if v.mode == ConsensusModeCPraos &&
		current > previous && current-previous > 1 {
		return 0, fmt.Errorf(
			"operational certificate counter skipped ahead: current=%d, previous=%d",
			current,
			previous,
		)
	}
	return current, nil
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
	if input.BlockNumber > 0 && len(input.PrevHeaderHash) == 0 {
		return errors.New(
			"previous header hash is required for non-genesis blocks",
		)
	}
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

// verifyCertifiedVRF verifies a VRF certificate (proof + output) against the
// given VRF key, slot, and epoch nonce, deriving the VRF input from seed.
// It holds the plumbing shared by the leader VRF check (validateVRFProof,
// seed = vrf.SeedL()) and the TPraos-only nonce VRF check
// (validateNonceVRFProof, seed = vrf.SeedEta()): epoch-nonce length
// validation, VRF key/proof/output size validation, VRF input construction,
// and vrf.Verify.
//
// label is prepended to the VRF-specific error messages so nonce VRF
// failures can be distinguished from leader VRF failures (e.g. "nonce ");
// pass "" for the leader VRF check. checkOutputSize controls whether the
// output length is validated up front: validateVRFProof historically relies
// on vrf.Verify's constant-time comparison to reject a size-mismatched
// output (returning the generic "verification returned false" error), so it
// passes false to preserve that behavior; validateNonceVRFProof passes true.
func (v *HeaderValidator) verifyCertifiedVRF(
	slot uint64,
	epochNonce []byte,
	vrfKey []byte,
	proof []byte,
	output []byte,
	seed []byte,
	checkOutputSize bool,
	label string,
) ([]byte, error) {
	// Validate EpochNonce is exactly 32 bytes to prevent panic in
	// vrf.MkInputVrf / vrf.MkSeedTPraos.
	if len(epochNonce) != 32 {
		return nil, fmt.Errorf(
			"epoch nonce must be 32 bytes, got %d",
			len(epochNonce),
		)
	}

	if len(vrfKey) != vrf.PublicKeySize {
		return nil, fmt.Errorf(
			"invalid VRF key size: expected %d, got %d",
			vrf.PublicKeySize,
			len(vrfKey),
		)
	}

	if len(proof) != vrf.ProofSize {
		return nil, fmt.Errorf(
			"invalid %sVRF proof size: expected %d, got %d",
			label,
			vrf.ProofSize,
			len(proof),
		)
	}

	if checkOutputSize && len(output) != vrf.OutputSize {
		return nil, fmt.Errorf(
			"invalid %sVRF output size: expected %d, got %d",
			label,
			vrf.OutputSize,
			len(output),
		)
	}

	// Compute VRF input message. TPraos (Shelley-Alonzo) and CPraos
	// (Babbage+) use different VRF input constructions: TPraos XORs the
	// base hash with the seed constant (Cardano.Protocol.TPraos.Rules.
	// Overlay.vrfChecks), while CPraos uses the raw
	// blake2b-256(slot || nonce) (Ouroboros.Consensus.Protocol.Praos.VRF.
	// mkInputVRF).
	// Slot numbers in Cardano are far below int64 max (mainnet ~100M, max ~9.2 quintillion)
	var vrfInput []byte
	var err error
	switch v.mode {
	case ConsensusModeTPraos:
		vrfInput, err = vrf.MkSeedTPraos(
			int64(slot), //nolint:gosec
			epochNonce,
			seed,
		)
	case ConsensusModeCPraos:
		vrfInput, err = vrf.MkInputVrf(
			int64(slot), //nolint:gosec
			epochNonce,
		)
	default:
		return nil, fmt.Errorf("unknown consensus mode: %d", v.mode)
	}
	if err != nil {
		return nil, fmt.Errorf(
			"invalid %sVRF input parameters at slot %d: %w",
			label,
			slot,
			err,
		)
	}

	// Verify VRF proof
	valid, err := vrf.Verify(vrfKey, proof, output, vrfInput)
	if err != nil {
		return nil, fmt.Errorf(
			"%sVRF verification failed at slot %d: %w",
			label,
			slot,
			err,
		)
	}

	if !valid {
		return nil, fmt.Errorf(
			"%sVRF proof verification returned false at slot %d",
			label,
			slot,
		)
	}

	return output, nil
}

// validateVRFProof verifies the VRF proof.
func (v *HeaderValidator) validateVRFProof(
	input *ValidateHeaderInput,
) ([]byte, error) {
	return v.verifyCertifiedVRF(
		input.Slot,
		input.EpochNonce,
		input.VrfKey,
		input.VrfProof,
		input.VrfOutput,
		vrf.SeedL(),
		false,
		"",
	)
}

// validateNonceVRFProof verifies the TPraos-only nonce VRF proof
// (bheaderEta, seedEta = mkNonceFromNumber(0)). This certificate drives
// epoch nonce evolution and is independent of the leader VRF (bheaderL,
// seedL) verified by validateVRFProof above. CPraos headers carry no
// separate nonce VRF field, so this is a no-op for ConsensusModeCPraos.
//
// Without this check, a TPraos header with a missing or forged nonce VRF
// certificate would be accepted as structurally valid, causing a real
// validator to diverge from the chain once the epoch nonce is derived from
// unverified nonce VRF outputs.
func (v *HeaderValidator) validateNonceVRFProof(
	input *ValidateHeaderInput,
) error {
	// Nonce VRF has no equivalent in CPraos headers.
	if v.mode != ConsensusModeTPraos {
		return nil
	}

	_, err := v.verifyCertifiedVRF(
		input.Slot,
		input.EpochNonce,
		input.VrfKey,
		input.NonceVrfProof,
		input.NonceVrfOutput,
		vrf.SeedEta(),
		true,
		"nonce ",
	)
	return err
}

// validateLeadership applies the pool policy and finite E34 leader predicate.
func (v *HeaderValidator) validateLeadership(
	input *ValidateHeaderInput,
	vrfOutput []byte,
) error {
	if v.tachys != nil {
		authorized, err := v.tachys.AuthorizeIssuer(
			input.EpochNonce,
			input.Slot,
			input.IssuerVkey,
		)
		if err != nil {
			return err
		}
		if !authorized {
			return fmt.Errorf(
				"issuer is not the scheduled Tachys leader at slot %d",
				input.Slot,
			)
		}
		return nil
	}
	if input.TotalStake == 0 {
		return errors.New("total stake cannot be zero")
	}

	if input.PoolStake == 0 {
		if err := validateConsensusMode(v.mode); err != nil {
			return err
		}
		if err := validateActiveSlotCoefficient(v.activeSlotCoeff); err != nil {
			return err
		}
		return errors.New("pool stake cannot be zero")
	}
	evaluator, err := NewLeaderEligibility(
		input.PoolStake, input.TotalStake, v.activeSlotCoeff, v.mode,
	)
	if err != nil {
		return err
	}
	eligible, err := evaluator.CheckVRFOutput(vrfOutput)
	if err != nil {
		return err
	}
	if !eligible {
		return fmt.Errorf(
			"VRF output does not satisfy leader eligibility at slot %d",
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
	opCertKESPeriod := input.OpCertKesPeriod

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

	// A certificate that starts after the current period is rejected by
	// validateKESPeriod. As in the reference, the signature is still
	// verified, at evolution 0, so that a bad signature is also reported
	// and the period error is not reported twice.
	currentKESPeriod := input.Slot / v.slotsPerKESPeriod
	var evolutionPeriod uint64
	if currentKESPeriod >= input.OpCertKesPeriod {
		evolutionPeriod = currentKESPeriod - input.OpCertKesPeriod
	}

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
	// IssuerVkey is required for OpCert validation
	if len(input.IssuerVkey) == 0 {
		return errors.New("IssuerVkey is required for OpCert validation")
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

	// The OpCert signature is over the raw OCertSignable representation
	// (hot_vkey || sequence_number || kes_period), not a CBOR encoding.
	opCertBody := common.OpCertSignableBytes(
		input.OpCertHotVkey,
		input.OpCertSequenceNumber,
		input.OpCertKesPeriod,
	)

	// Verify Ed25519 signature
	valid := ed25519strict.Verify(
		input.IssuerVkey,
		opCertBody,
		input.OpCertSignature,
	)
	if !valid {
		return errors.New("OpCert signature verification failed")
	}

	return nil
}

// validateVRFKeyRegistration verifies the VRF key matches the pool's registered key hash.
func (v *HeaderValidator) validateVRFKeyRegistration(
	input *ValidateHeaderInput,
) error {
	if len(input.RegisteredVrfKeyHash) == 0 {
		return errors.New(
			"registered VRF key hash is required for header validation",
		)
	}

	if len(input.VrfKey) != vrf.PublicKeySize {
		return fmt.Errorf(
			"invalid VRF key size for registration check: expected %d, got %d",
			vrf.PublicKeySize,
			len(input.VrfKey),
		)
	}

	// Pool registrations store the canonical Blake2b-256 hash of the VRF key.
	vrfKeyHash := common.Blake2b256Hash(input.VrfKey)

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
