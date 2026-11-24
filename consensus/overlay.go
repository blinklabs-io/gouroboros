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
	"math/big"
	"slices"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// Errors reported by ValidateHeader for the envelope and overlay checks.
var (
	// ErrMissingHeaderInput reports that the caller left out an input the
	// header's era requires. It is a caller error, not a verdict on the
	// header, and is the only error in the result.
	ErrMissingHeaderInput = errors.New(
		"required header validation input missing",
	)

	// ErrObsoleteNode reports a protocol major version above the node's
	// maximum (the reference's ObsoleteNode).
	ErrObsoleteNode = errors.New("protocol version is above the node's maximum")

	// ErrHeaderSizeTooLarge reports a header larger than the protocol
	// parameter maxBlockHeaderSize.
	ErrHeaderSizeTooLarge = errors.New("block header size too large")

	// ErrBlockSizeTooLarge reports a declared block body size larger than
	// the protocol parameter maxBlockBodySize.
	ErrBlockSizeTooLarge = errors.New("block body size too large")

	// ErrNotActiveSlot reports a header in a non-active slot of the TPraos
	// overlay schedule, where no block may be made.
	ErrNotActiveSlot = errors.New("header in a non-active overlay slot")

	// ErrWrongGenesisColdKey reports a header in an active overlay slot
	// whose issuer is not the scheduled genesis key's delegate.
	ErrWrongGenesisColdKey = errors.New(
		"issuer is not the delegate of the genesis key scheduled for the slot",
	)

	// ErrWrongGenesisVRFKey reports a header in an active overlay slot
	// whose VRF key is not the one in the scheduled genesis delegation.
	ErrWrongGenesisVRFKey = errors.New(
		"VRF key does not match the scheduled genesis delegation",
	)
)

// EnvelopeInput carries the inputs of the envelope checks every Shelley and
// later header must pass: the node's protocol version limit and the
// protocol parameters' header and body size limits.
type EnvelopeInput struct {
	// ProtocolMajorVersion is compared with MaxMajorProtocolVersion. For
	// ConsensusModeTPraos it is the major version the header declares; for
	// ConsensusModeCPraos it is the major version of the protocol parameters
	// in effect at the header's slot.
	ProtocolMajorVersion uint64
	// MaxMajorProtocolVersion is the highest protocol major version this
	// node supports. A header that needs a higher one is rejected with
	// ErrObsoleteNode.
	MaxMajorProtocolVersion uint64
	// HeaderSize is the size in bytes of the header's original CBOR,
	// signature included.
	HeaderSize uint64
	// BlockBodySize is the block body size the header declares.
	BlockBodySize uint64
	// MaxBlockHeaderSize is the protocol parameter maxBlockHeaderSize in
	// effect at the header's slot.
	MaxBlockHeaderSize uint64
	// MaxBlockBodySize is the protocol parameter maxBlockBodySize in effect
	// at the header's slot.
	MaxBlockBodySize uint64
}

// GenesisDelegation is the delegation of one genesis key: the cold key hash
// of its delegate and the hash of the delegate's VRF key.
type GenesisDelegation struct {
	Delegate   common.Blake2b224
	VrfKeyHash common.Blake2b256
}

// TPraosOverlayInput carries the inputs of the TPraos overlay schedule,
// which ConsensusModeTPraos (Shelley through Alonzo) headers require.
type TPraosOverlayInput struct {
	// Decentralization is the protocol parameter d in effect at the
	// header's slot, between 0 and 1. With d = 0 there is no overlay.
	Decentralization *big.Rat
	// EpochFirstSlot is the first slot of the epoch that contains the
	// header's slot.
	EpochFirstSlot uint64
	// GenesisDelegates maps each genesis key hash to its delegation, as the
	// ledger has it at the header's slot: the Shelley genesis delegations,
	// updated by genesis key delegation certificates. It may be empty only
	// when Decentralization is 0.
	GenesisDelegates map[common.Blake2b224]GenesisDelegation
}

type overlaySlotKind int

const (
	// Not in the overlay schedule: the ordinary Praos checks apply.
	overlaySlotPraos overlaySlotKind = iota
	// In the overlay schedule, assigned to a genesis key.
	overlaySlotActive
	// In the overlay schedule, assigned to no one.
	overlaySlotNonActive
)

// overlaySlot is a slot's place in the overlay schedule. delegation is set
// only for an active slot.
type overlaySlot struct {
	kind       overlaySlotKind
	delegation GenesisDelegation
}

// ceilRat returns ceiling(n * r) for n >= 0 and r >= 0.
func ceilRat(n uint64, r *big.Rat) *big.Int {
	num := new(big.Int).Mul(new(big.Int).SetUint64(n), r.Num())
	den := r.Denom()
	num.Add(num, den)
	num.Sub(num, big.NewInt(1))
	return num.Quo(num, den)
}

// classifyOverlaySlot places slot in the overlay schedule of its epoch, as
// isOverlaySlot (Cardano.Ledger.Slot) and classifyOverlaySlot
// (Cardano.Protocol.TPraos.Rules.Overlay) do. The slot at offset s is in the
// schedule when ceiling(s*d) < ceiling((s+1)*d); it is active when that
// position is a multiple of floor(1/f), and then belongs to the genesis key
// at index (position / floor(1/f)) mod |keys|, in byte order of the key
// hashes.
func classifyOverlaySlot(
	slot uint64,
	overlay *TPraosOverlayInput,
	activeSlotCoeff *big.Rat,
) overlaySlot {
	d := overlay.Decentralization
	offset := slot - overlay.EpochFirstSlot
	position := ceilRat(offset, d)
	if position.Cmp(ceilRat(offset+1, d)) >= 0 {
		return overlaySlot{kind: overlaySlotPraos}
	}
	ascInv := new(big.Int).Quo(activeSlotCoeff.Denom(), activeSlotCoeff.Num())
	index, rem := new(big.Int).QuoRem(position, ascInv, new(big.Int))
	if rem.Sign() != 0 {
		return overlaySlot{kind: overlaySlotNonActive}
	}
	keys := make([]common.Blake2b224, 0, len(overlay.GenesisDelegates))
	for key := range overlay.GenesisDelegates {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b common.Blake2b224) int {
		return bytes.Compare(a[:], b[:])
	})
	index.Mod(index, big.NewInt(int64(len(keys))))
	return overlaySlot{
		kind:       overlaySlotActive,
		delegation: overlay.GenesisDelegates[keys[index.Int64()]],
	}
}

// checkRequiredInputs reports a missing or unusable input that the
// validator's mode requires, wrapped in ErrMissingHeaderInput.
func (v *HeaderValidator) checkRequiredInputs(
	input *ValidateHeaderInput,
) error {
	missing := func(format string, args ...any) error {
		return fmt.Errorf(
			"%w: %s",
			ErrMissingHeaderInput,
			fmt.Sprintf(format, args...),
		)
	}
	env := input.Envelope
	switch {
	case env == nil:
		return missing("Envelope is nil")
	case env.ProtocolMajorVersion == 0:
		return missing("Envelope.ProtocolMajorVersion is zero")
	case env.MaxMajorProtocolVersion == 0:
		return missing("Envelope.MaxMajorProtocolVersion is zero")
	case env.HeaderSize == 0:
		return missing("Envelope.HeaderSize is zero")
	case env.BlockBodySize == 0:
		return missing("Envelope.BlockBodySize is zero")
	case env.MaxBlockHeaderSize == 0:
		return missing("Envelope.MaxBlockHeaderSize is zero")
	case env.MaxBlockBodySize == 0:
		return missing("Envelope.MaxBlockBodySize is zero")
	}
	if v.mode != ConsensusModeTPraos {
		return nil
	}
	overlay := input.Overlay
	switch {
	case overlay == nil:
		return missing("Overlay is nil for a TPraos header")
	case overlay.Decentralization == nil:
		return missing("Overlay.Decentralization is nil")
	case overlay.Decentralization.Sign() < 0 ||
		overlay.Decentralization.Cmp(big.NewRat(1, 1)) > 0:
		return missing(
			"Overlay.Decentralization %s is outside [0, 1]",
			overlay.Decentralization.RatString(),
		)
	case overlay.GenesisDelegates == nil:
		return missing("Overlay.GenesisDelegates is nil")
	case overlay.EpochFirstSlot > input.Slot:
		return missing(
			"Overlay.EpochFirstSlot %d is after slot %d",
			overlay.EpochFirstSlot,
			input.Slot,
		)
	}
	if overlay.Decentralization.Sign() == 0 {
		return nil
	}
	if len(overlay.GenesisDelegates) == 0 {
		return missing("Overlay.GenesisDelegates is empty and d > 0")
	}
	if err := validateActiveSlotCoefficient(v.activeSlotCoeff); err != nil {
		return missing("active slot coefficient: %v", err)
	}
	return nil
}

// overlaySlotFor returns the header's place in the overlay schedule. Only
// TPraos has an overlay; checkRequiredInputs has already vetted the inputs.
func (v *HeaderValidator) overlaySlotFor(
	input *ValidateHeaderInput,
) overlaySlot {
	if v.mode != ConsensusModeTPraos ||
		input.Overlay.Decentralization.Sign() == 0 {
		return overlaySlot{kind: overlaySlotPraos}
	}
	return classifyOverlaySlot(input.Slot, input.Overlay, v.activeSlotCoeff)
}

// validateEnvelopeLimits makes the reference's extra envelope checks
// (EnvelopeChecks.envelopeCheck): protocol version, header size, body size.
func validateEnvelopeLimits(env *EnvelopeInput) []error {
	var errs []error
	if env.ProtocolMajorVersion > env.MaxMajorProtocolVersion {
		errs = append(errs, fmt.Errorf(
			"%w: version=%d, max=%d",
			ErrObsoleteNode,
			env.ProtocolMajorVersion,
			env.MaxMajorProtocolVersion,
		))
	}
	if env.HeaderSize > env.MaxBlockHeaderSize {
		errs = append(errs, fmt.Errorf(
			"%w: size=%d, max=%d",
			ErrHeaderSizeTooLarge,
			env.HeaderSize,
			env.MaxBlockHeaderSize,
		))
	}
	if env.BlockBodySize > env.MaxBlockBodySize {
		errs = append(errs, fmt.Errorf(
			"%w: size=%d, max=%d",
			ErrBlockSizeTooLarge,
			env.BlockBodySize,
			env.MaxBlockBodySize,
		))
	}
	return errs
}

// validateGenesisColdKey checks that the issuer of a header in an active
// overlay slot is the scheduled genesis key's delegate.
func validateGenesisColdKey(
	input *ValidateHeaderInput,
	delegation GenesisDelegation,
) error {
	issuer := common.Blake2b224Hash(input.IssuerVkey)
	if issuer != delegation.Delegate {
		return fmt.Errorf(
			"%w: issuer %s, delegate %s",
			ErrWrongGenesisColdKey,
			issuer.String(),
			delegation.Delegate.String(),
		)
	}
	return nil
}

// validateGenesisVRFKey checks that the VRF key of a header in an active
// overlay slot is the one in the scheduled genesis delegation.
func validateGenesisVRFKey(
	input *ValidateHeaderInput,
	delegation GenesisDelegation,
) error {
	vrfKeyHash := common.Blake2b256Hash(input.VrfKey)
	if vrfKeyHash != delegation.VrfKeyHash {
		return fmt.Errorf(
			"%w: got %s, expected %s",
			ErrWrongGenesisVRFKey,
			vrfKeyHash.String(),
			delegation.VrfKeyHash.String(),
		)
	}
	return nil
}
