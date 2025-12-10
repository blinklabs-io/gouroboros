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
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

func (e ValidationError) Unwrap() error {
	return e.Cause
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
	// LedgerState provides the current ledger state for transaction validation.
	// Required if SkipTransactionValidation or SkipStakePoolValidation is false.
	LedgerState LedgerState
	// ProtocolParameters provides the current protocol parameters for transaction validation.
	// Required if SkipTransactionValidation is false.
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
