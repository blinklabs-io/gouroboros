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

package common

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"golang.org/x/crypto/blake2b"
)

// ValidationError represents a structured validation error with additional context
type ValidationError struct {
	Type    ValidationErrorType
	Message string
	Details map[string]any
	Cause   error
	// ByteOffset is the byte offset within the underlying CBOR data where the
	// problem was detected. Zero means unknown / not applicable.
	ByteOffset int
	// CborContext is a human-readable path within the CBOR structure (e.g.
	// "block/body/tx[3]/inputs"). Empty means unknown / not applicable.
	CborContext string
	// Diagnostic is a CBOR diagnostic notation snippet describing the
	// problematic element. Empty means unavailable.
	Diagnostic string
}

type ValidationErrorType string

const (
	ValidationErrorTypeBodyHash      ValidationErrorType = "body_hash"
	ValidationErrorTypeTransaction   ValidationErrorType = "transaction"
	ValidationErrorTypeStakePool     ValidationErrorType = "stake_pool"
	ValidationErrorTypeVRF           ValidationErrorType = "vrf"
	ValidationErrorTypeKES           ValidationErrorType = "kes"
	ValidationErrorTypeProtocol      ValidationErrorType = "protocol"
	ValidationErrorTypeConfiguration ValidationErrorType = "configuration"
)

func (e ValidationError) Error() string {
	var ctx string
	if e.CborContext != "" || e.ByteOffset > 0 {
		switch {
		case e.CborContext != "" && e.ByteOffset > 0:
			ctx = fmt.Sprintf(" [%s @offset %d]", e.CborContext, e.ByteOffset)
		case e.CborContext != "":
			ctx = fmt.Sprintf(" [%s]", e.CborContext)
		default:
			ctx = fmt.Sprintf(" [@offset %d]", e.ByteOffset)
		}
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s%s (%v)", e.Type, e.Message, ctx, e.Cause)
	}
	return fmt.Sprintf("%s: %s%s", e.Type, e.Message, ctx)
}

func (e ValidationError) Unwrap() error {
	return e.Cause
}

// WithDiagnostic returns Error() followed by the stored Diagnostic snippet on
// new lines. When Diagnostic is empty, it returns Error() unchanged.
func (e ValidationError) WithDiagnostic() string {
	if e.Diagnostic == "" {
		return e.Error()
	}
	return fmt.Sprintf("%s\n\nDiagnostic:\n%s", e.Error(), e.Diagnostic)
}

// NewValidationError creates a new structured validation error
func NewValidationError(
	errType ValidationErrorType,
	message string,
	details map[string]any,
	cause error,
) *ValidationError {
	return &ValidationError{
		Type:    errType,
		Message: message,
		Details: details,
		Cause:   cause,
	}
}

// VerifyConfig holds runtime verification toggles.
// Default values favor safety; tests or specific flows can opt out.
type VerifyConfig struct {
	// SkipBodyHashValidation disables body hash verification in VerifyBlock().
	// When false (default), full block CBOR must be available for validation.
	// Useful for scenarios where full block CBOR is unavailable.
	SkipBodyHashValidation bool
	// SkipTransactionValidation disables transaction validation in VerifyBlock().
	// When false (default), LedgerState and ProtocolParameters must be set.
	SkipTransactionValidation bool
	// SkipStakePoolValidation disables stake pool registration validation in VerifyBlock().
	// When false (default), LedgerState must be set.
	SkipStakePoolValidation bool
	// SkipBlockLimitsValidation disables block-wide execution-unit and
	// serialized-size limit enforcement in VerifyBlock() (the BBODY
	// block-level checks: sum of transaction ExUnits against
	// ppMaxBlockExUnits, and block body/header size against
	// ppMaxBlockBodySize/ppMaxBlockHeaderSize). Unlike the other Skip*
	// flags, this check does not require LedgerState and runs independently
	// of SkipTransactionValidation: it only needs ProtocolParameters (for
	// the limits) and, for the size checks, the block's raw CBOR. It is a
	// no-op when ProtocolParameters is nil or the block has neither
	// transactions nor raw CBOR available.
	SkipBlockLimitsValidation bool
	// EnableByronSscProofHashValidation opts into recomputing and comparing
	// a Byron main block's ssc_proof hashes against its header, in addition
	// to the always-on structural check of that field (proof type, element
	// counts, and the wire shape -- tag-258 set vs. genuine CBOR map -- of
	// every field it hashes).
	//
	// Default false: unlike tx_proof/dlg_proof/upd_proof, ssc_proof has no
	// upstream reference implementation to cross-check this package's own
	// hash construction against. Modern cardano-ledger decodes SscProof as
	// a unit type and re-encodes a hardcoded placeholder regardless of the
	// block's actual SSC content, so cardano-node itself would accept a
	// block whose ssc_proof this check might reject -- the "mainnet has run
	// on cardano-node for years" safety net that justifies enforcing
	// tx_proof/dlg_proof/upd_proof unconditionally does not apply here. The
	// hash construction itself (see ledger/byron's checkSscProofCore) is
	// confirmed against a small number of real mainnet blocks -- roughly
	// 4-5 blocks, covering only two of the four SSC payload types
	// (CommitmentsPayload and OpeningsPayload) but exercising three of the
	// four distinct hash constructions those payloads use (the
	// commitments-field, openings-field, and certificates-field hashes;
	// the CommitmentsPayload vector happens to also carry a non-empty
	// certificate set). SharesPayload's hash construction has no
	// confirmed non-empty vector and is inferred only by code-path
	// identity with the proven Openings case -- see ledger/byron's
	// checkSscProofLocal doc comment -- out of Byron's
	// ~5,000,000 total blocks and 208 epochs -- and two real encoding bugs
	// were found and fixed in it during the same development round that
	// produced those vectors, which is evidence the construction is subtle
	// rather than settled. Making that comparison decode-gating by default
	// would mean any undiscovered edge case turns into a real, unrelated
	// mainnet block failing to decode. Set this to true to opt into the
	// full hash comparison once broader evidence justifies treating a
	// mismatch as fatal for real blocks; until then, decode-gating callers
	// (e.g. NewByronMainBlockFromCbor) leave it off and rely on the
	// structural check alone.
	EnableByronSscProofHashValidation bool
	// EnableByronPayloadValidation opts into structurally validating a
	// Byron main block's delegation and update payloads and verifying the
	// signatures they carry -- heavyweight delegation certificates, update
	// proposals, and update votes -- in addition to the always-on check
	// that those payloads' bytes hash to the dlg_proof/upd_proof in the
	// header.
	//
	// Those two checks answer different questions. dlg_proof and upd_proof
	// bind the payload bytes to the header, so a block cannot carry a
	// payload its issuer did not commit to; they say nothing about whether
	// those bytes decode to a well-formed certificate, or whether the
	// signature inside one verifies. A block whose delegation payload holds
	// a certificate with a truncated verification key, or a vote signed by
	// a key that did not sign it, passes the proof check unchanged.
	//
	// Default false, for the same reason as
	// EnableByronSscProofHashValidation: the update proposal and vote
	// signing formats reproduced here (see ledger/byron's
	// ByronUpdateProposal.Validate and UpdateVote.Verify) are taken from
	// cardano-ledger-byron's Cardano.Chain.Update sources rather than
	// confirmed against real mainnet blocks carrying those payloads, which
	// are rare enough that this repository has no such vector. Making them
	// decode-gating by default would risk an undiscovered edge case turning
	// into a real mainnet block that fails to decode. The delegation
	// certificate half is on firmer ground -- its signing format is the
	// same one consensus/byron already verifies against a real mainnet
	// certificate on every proxy-signed header -- but the flag covers both
	// so that opting in is a single, coherent decision.
	//
	// Callers who want only the delegation half without the update half can
	// call byron.ByronMainBlock.ValidateDelegationPayload directly.
	EnableByronPayloadValidation bool
	// LedgerState provides the current ledger state for transaction validation.
	// Required if SkipTransactionValidation or SkipStakePoolValidation is false.
	LedgerState LedgerState
	// ProtocolParameters provides the current protocol parameters for
	// transaction validation and block-limits validation.
	// Required if SkipTransactionValidation is false. When set and
	// SkipBlockLimitsValidation is false, also enables block-wide
	// execution-unit and size validation (see SkipBlockLimitsValidation).
	ProtocolParameters ProtocolParameters
}

// ValidateBlockBodyHash validates the block body hash during parsing.
// It takes the raw CBOR data, expected body hash, and era-specific parameters.
func ValidateBlockBodyHash(
	data []byte,
	expectedBodyHash Blake2b256,
	eraName string,
	minRawLength int,
) error {
	var raw []cbor.RawMessage
	if _, err := cbor.Decode(data, &raw); err != nil {
		return NewValidationError(
			ValidationErrorTypeBodyHash,
			"failed to decode block CBOR for body hash validation",
			map[string]any{
				"era": eraName,
			},
			err,
		)
	}
	if len(raw) < minRawLength {
		return NewValidationError(
			ValidationErrorTypeBodyHash,
			fmt.Sprintf(
				"invalid %s block CBOR structure for body hash validation",
				eraName,
			),
			map[string]any{
				"era":           eraName,
				"expected_min":  minRawLength,
				"actual_length": len(raw),
			},
			nil,
		)
	}
	// Compute body hash as per Cardano spec: blake2b_256(hash_tx || hash_wit || hash_aux [|| hash_invalid])
	var bodyHashes []byte
	for i := 1; i < minRawLength; i++ {
		tmpHash := blake2b.Sum256(raw[i])
		bodyHashes = append(bodyHashes, tmpHash[:]...)
	}

	actualBodyHash := blake2b.Sum256(bodyHashes)
	if !bytes.Equal(actualBodyHash[:], expectedBodyHash.Bytes()) {
		return NewValidationError(
			ValidationErrorTypeBodyHash,
			eraName+" block body hash mismatch during parsing",
			map[string]any{
				"era":           eraName,
				"expected_hash": expectedBodyHash.String(),
				"actual_hash":   hex.EncodeToString(actualBodyHash[:]),
			},
			nil,
		)
	}
	return nil
}

// BlockBodySizeFromCbor returns the serialized size, in bytes, of a block's
// body from the block's original raw CBOR: the sum of the raw CBOR lengths
// of every top-level array element after the header (element 0).
//
// This matches cardano-ledger's default blockBodySize, which for
// pre-Dijkstra blocks is the concatenation of transaction_bodies,
// transaction_witness_sets, auxiliary_data_set, and (Alonzo+)
// invalid_transactions, each encoded as normal top-level CBOR values with no
// extra wrapping array around the group; and for Dijkstra (a 2-element
// [header, block_body] block) is simply the single block_body element.
// Summing raw[1:] covers both shapes without era-specific branching.
//
// NOTE: this re-decodes the entire block CBOR into []cbor.RawMessage just to
// sum the lengths of the top-level elements, which is redundant with the
// decode VerifyBlock's caller has typically already done (or will do) to
// obtain the block/transactions in the first place. On the sync-pipeline hot
// path this is an extra full re-tokenization of the block. A proper fix
// would thread already-decoded top-level element boundaries (or their
// lengths) through from the call site instead of handing this function raw
// bytes, but that requires a larger refactor of how VerifyBlock threads CBOR
// through the pipeline. As a low-risk mitigation, the call in VerifyBlock
// only invokes this when MaxBlockBodySize > 0 (i.e. the limit is actually
// enabled), so the redundant decode is skipped entirely when the check is a
// no-op. Tracked as a known follow-up rather than addressed here.
func BlockBodySizeFromCbor(rawCbor []byte) (uint64, error) {
	var raw []cbor.RawMessage
	if _, err := cbor.Decode(rawCbor, &raw); err != nil {
		return 0, fmt.Errorf(
			"failed to decode block CBOR for body size calculation: %w",
			err,
		)
	}
	if len(raw) < 2 {
		return 0, fmt.Errorf(
			"invalid block CBOR structure for body size calculation: expected at least 2 elements, got %d",
			len(raw),
		)
	}
	var size uint64
	for _, item := range raw[1:] {
		size += uint64(len(item))
	}
	return size, nil
}
