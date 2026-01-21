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

package byron

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// HeaderValidator validates Byron block headers using OBFT rules
type HeaderValidator struct {
	config           ByronConfig
	genesisKeyHashes map[string]bool // Set of allowed genesis delegate key hashes
}

// NewHeaderValidator creates a new Byron header validator
func NewHeaderValidator(config ByronConfig) *HeaderValidator {
	keyHashes := make(map[string]bool)
	for _, hash := range config.GenesisKeyHashes {
		keyHashes[string(hash)] = true
	}
	return &HeaderValidator{
		config:           config,
		genesisKeyHashes: keyHashes,
	}
}

// ValidateHeaderInput contains data needed to validate a Byron header
type ValidateHeaderInput struct {
	// Header fields
	Slot           uint64
	BlockNumber    uint64
	PrevHash       []byte
	ProtocolMagic  uint32
	IssuerPubKey   []byte
	BlockSignature []byte

	// For signature verification
	HeaderCbor []byte // The CBOR-encoded header body to verify signature against

	// Previous header for chain validation
	PrevSlot        uint64
	PrevBlockNumber uint64
	PrevHeaderHash  []byte

	// Block type
	IsEBB bool // Epoch Boundary Block

	// EnvelopeOnly skips signature and genesis delegate validation.
	// Use this for structural validation without cryptographic verification.
	EnvelopeOnly bool
}

// ValidateResult contains the result of header validation
type ValidateResult struct {
	Valid  bool
	Errors []error
}

// ValidateHeader validates a Byron block header using OBFT rules
//
// Validation checks for main blocks:
//  1. Slot strictly increases from previous block (or equal if prev was EBB)
//  2. Block number is previous + 1 (or equal if this is EBB)
//  3. PrevHash matches hash of previous header
//  4. Protocol magic matches expected
//  5. Block signature is valid
//  6. Issuer is a valid genesis delegate
//
// For Epoch Boundary Blocks:
//  1. Slot is at epoch boundary
//  2. Block number is previous (EBBs share block number with next block)
//  3. PrevHash matches
//  4. Protocol magic matches
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

	// 4. Validate protocol magic
	if err := v.validateProtocolMagic(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	// For EBBs or envelope-only validation, skip signature checks
	if input.IsEBB || input.EnvelopeOnly {
		return result
	}

	// 5. Validate block signature (main blocks only)
	if err := v.validateBlockSignature(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	// 6. Validate issuer is genesis delegate (main blocks only)
	if err := v.validateGenesisDelegate(input); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
	}

	return result
}

// validateSlotOrdering checks slot progression
func (v *HeaderValidator) validateSlotOrdering(
	input *ValidateHeaderInput,
) error {
	if input.IsEBB {
		// EBBs can share slot with previous block (at epoch boundary)
		if input.Slot < input.PrevSlot {
			return fmt.Errorf(
				"EBB slot must be >= previous slot: current=%d, previous=%d",
				input.Slot,
				input.PrevSlot,
			)
		}
	} else {
		// Regular blocks must strictly increase
		if input.Slot <= input.PrevSlot {
			return fmt.Errorf(
				"slot must be greater than previous slot: current=%d, previous=%d",
				input.Slot,
				input.PrevSlot,
			)
		}
	}
	return nil
}

// validateBlockNumber checks block number progression
func (v *HeaderValidator) validateBlockNumber(
	input *ValidateHeaderInput,
) error {
	if input.IsEBB {
		// EBBs have same block number as the next regular block
		// So they share block number with previous (or it's the same)
		if input.BlockNumber != input.PrevBlockNumber &&
			input.BlockNumber != input.PrevBlockNumber+1 {
			return fmt.Errorf(
				"EBB block number must match or be previous + 1: current=%d, previous=%d",
				input.BlockNumber,
				input.PrevBlockNumber,
			)
		}
	} else {
		expectedBlockNumber := input.PrevBlockNumber + 1
		if input.BlockNumber != expectedBlockNumber {
			return fmt.Errorf(
				"block number must be previous + 1: current=%d, expected=%d",
				input.BlockNumber,
				expectedBlockNumber,
			)
		}
	}
	return nil
}

// validatePrevHash checks previous hash linkage
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

// validateProtocolMagic checks the protocol magic matches expected network
func (v *HeaderValidator) validateProtocolMagic(
	input *ValidateHeaderInput,
) error {
	if input.ProtocolMagic != v.config.ProtocolMagic {
		return fmt.Errorf(
			"protocol magic mismatch: got %d, expected %d",
			input.ProtocolMagic,
			v.config.ProtocolMagic,
		)
	}
	return nil
}

// validateBlockSignature verifies the block signature
func (v *HeaderValidator) validateBlockSignature(
	input *ValidateHeaderInput,
) error {
	if len(input.IssuerPubKey) != ed25519.PublicKeySize {
		return fmt.Errorf(
			"invalid issuer public key size: got %d, expected %d",
			len(input.IssuerPubKey),
			ed25519.PublicKeySize,
		)
	}

	if len(input.BlockSignature) != ed25519.SignatureSize {
		return fmt.Errorf(
			"invalid block signature size: got %d, expected %d",
			len(input.BlockSignature),
			ed25519.SignatureSize,
		)
	}

	if len(input.HeaderCbor) == 0 {
		return errors.New("header CBOR is required for signature verification")
	}

	// Verify Ed25519 signature
	valid := ed25519.Verify(
		input.IssuerPubKey,
		input.HeaderCbor,
		input.BlockSignature,
	)
	if !valid {
		return fmt.Errorf(
			"block signature verification failed at slot %d, block %d",
			input.Slot,
			input.BlockNumber,
		)
	}

	return nil
}

// validateGenesisDelegate checks if the issuer is a valid genesis delegate
func (v *HeaderValidator) validateGenesisDelegate(
	input *ValidateHeaderInput,
) error {
	// If no genesis keys configured, skip this check
	if len(v.genesisKeyHashes) == 0 {
		return nil
	}

	// Hash the issuer public key using the common Blake2b224Hash function
	keyHash := common.Blake2b224Hash(input.IssuerPubKey)

	if !v.genesisKeyHashes[string(keyHash.Bytes())] {
		return fmt.Errorf(
			"issuer is not a valid genesis delegate: key hash %s",
			keyHash.String(),
		)
	}

	return nil
}

// ValidateByronBlockHeader validates a Byron block header (main block or EBB).
// Set isEBB to true for Epoch Boundary Blocks, false for main blocks.
//
// Note: This function performs envelope validation only. For main blocks (isEBB=false),
// full validation including block signature and genesis delegate verification requires
// using HeaderValidator directly with IssuerPubKey, BlockSignature, and HeaderCbor fields.
// This wrapper is suitable for structural validation but not cryptographic verification.
//
// IMPORTANT: Protocol magic validation is skipped in this wrapper because the header
// interface does not expose the protocol magic field. The protocol magic is taken from
// the config parameter, so validation would compare config against itself. For full
// protocol magic validation, use HeaderValidator directly with ValidateHeaderInput
// populated from the actual header fields.
func ValidateByronBlockHeader(
	header interface {
		SlotNumber() uint64
		BlockNumber() uint64
		PrevHash() common.Blake2b256
	},
	prevHeader interface {
		SlotNumber() uint64
		BlockNumber() uint64
		Hash() common.Blake2b256
	},
	config ByronConfig,
	isEBB bool,
) error {
	validator := NewHeaderValidator(config)

	input := &ValidateHeaderInput{
		Slot:            header.SlotNumber(),
		BlockNumber:     header.BlockNumber(),
		PrevHash:        header.PrevHash().Bytes(),
		ProtocolMagic:   config.ProtocolMagic, // Note: uses config, not header (see doc above)
		PrevSlot:        prevHeader.SlotNumber(),
		PrevBlockNumber: prevHeader.BlockNumber(),
		PrevHeaderHash:  prevHeader.Hash().Bytes(),
		IsEBB:           isEBB,
		EnvelopeOnly:    true, // This wrapper performs envelope validation only
	}

	result := validator.ValidateHeader(input)
	if !result.Valid && len(result.Errors) > 0 {
		return result.Errors[0]
	}
	return nil
}

// ValidateByronMainBlockHeader validates a ByronMainBlockHeader.
// This is a convenience wrapper for ValidateByronBlockHeader with isEBB=false.
func ValidateByronMainBlockHeader(
	header interface {
		SlotNumber() uint64
		BlockNumber() uint64
		PrevHash() common.Blake2b256
	},
	prevHeader interface {
		SlotNumber() uint64
		BlockNumber() uint64
		Hash() common.Blake2b256
	},
	config ByronConfig,
) error {
	return ValidateByronBlockHeader(header, prevHeader, config, false)
}

// ValidateByronEBBHeader validates a ByronEpochBoundaryBlockHeader.
// This is a convenience wrapper for ValidateByronBlockHeader with isEBB=true.
func ValidateByronEBBHeader(
	header interface {
		SlotNumber() uint64
		BlockNumber() uint64
		PrevHash() common.Blake2b256
	},
	prevHeader interface {
		SlotNumber() uint64
		BlockNumber() uint64
		Hash() common.Blake2b256
	},
	config ByronConfig,
) error {
	return ValidateByronBlockHeader(header, prevHeader, config, true)
}
