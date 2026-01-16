// Copyright 2024 Cardano Foundation
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

package ledger

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/kes"
)

// OpCert represents an Operational Certificate used in Cardano consensus.
// It binds a KES (Key Evolving Signature) key to a pool's cold key.
type OpCert struct {
	KesVkey       []byte // KES verification key (hot key), 32 bytes
	IssueNumber   uint64 // Certificate sequence/issue number
	KesPeriod     uint64 // KES period at certificate creation
	ColdSignature []byte // Cold key signature over the certificate, 64 bytes
}

// OpCertError represents an error during OpCert validation
type OpCertError struct {
	Field   string
	Message string
}

func (e *OpCertError) Error() string {
	return fmt.Sprintf("opcert validation failed: %s: %s", e.Field, e.Message)
}

// VerifyOpCertSignature verifies the cold key signature on an OpCert.
// The signature covers CBOR([kes_vkey, issue_number, kes_period]).
func VerifyOpCertSignature(opCert *OpCert, coldVkey []byte) error {
	if opCert == nil {
		return errors.New("opcert is nil")
	}

	// Validate field sizes
	if len(opCert.KesVkey) != kes.PublicKeySize {
		return &OpCertError{
			Field: "kes_vkey",
			Message: fmt.Sprintf(
				"must be %d bytes, got %d",
				kes.PublicKeySize,
				len(opCert.KesVkey),
			),
		}
	}
	if len(opCert.ColdSignature) != ed25519.SignatureSize {
		return &OpCertError{
			Field: "cold_signature",
			Message: fmt.Sprintf(
				"must be %d bytes, got %d",
				ed25519.SignatureSize,
				len(opCert.ColdSignature),
			),
		}
	}
	if len(coldVkey) != ed25519.PublicKeySize {
		return &OpCertError{
			Field: "cold_vkey",
			Message: fmt.Sprintf(
				"must be %d bytes, got %d",
				ed25519.PublicKeySize,
				len(coldVkey),
			),
		}
	}

	// Create the message to verify: CBOR([kes_vkey, issue_number, kes_period])
	certData := []any{
		opCert.KesVkey,
		opCert.IssueNumber,
		opCert.KesPeriod,
	}

	certCbor, err := cbor.Encode(certData)
	if err != nil {
		return fmt.Errorf("failed to encode certificate data: %w", err)
	}

	// Verify signature using cold verification key
	pubKey := ed25519.PublicKey(coldVkey)
	if !ed25519.Verify(pubKey, certCbor, opCert.ColdSignature) {
		return &OpCertError{
			Field:   "cold_signature",
			Message: "signature verification failed",
		}
	}

	return nil
}

// ValidateKesPeriod checks if the OpCert's KES period is valid for the given slot.
// It returns the evolution period (t) if valid, or an error if invalid.
//
// Parameters:
//   - opCertKesPeriod: the KES period in the OpCert
//   - currentSlot: the current slot number
//   - slotsPerKesPeriod: number of slots per KES period (e.g., 129600 on mainnet)
//   - maxKesEvolutions: maximum allowed KES evolutions (e.g., 62 on mainnet)
//
// Returns:
//   - evolutionPeriod: the number of KES evolutions from opCertKesPeriod to current period
//   - error: if the KES period is invalid
func ValidateKesPeriod(
	opCertKesPeriod uint64,
	currentSlot uint64,
	slotsPerKesPeriod uint64,
	maxKesEvolutions uint64,
) (uint64, error) {
	if slotsPerKesPeriod == 0 {
		return 0, errors.New("slotsPerKesPeriod cannot be zero")
	}
	if maxKesEvolutions == 0 {
		return 0, errors.New("maxKesEvolutions cannot be zero")
	}

	currentKesPeriod := currentSlot / slotsPerKesPeriod

	// OpCert cannot be from the future
	if currentKesPeriod < opCertKesPeriod {
		return 0, &OpCertError{
			Field: "kes_period",
			Message: fmt.Sprintf(
				"certificate KES period %d is in the future (current period: %d)",
				opCertKesPeriod,
				currentKesPeriod,
			),
		}
	}

	// Calculate evolution period
	evolutionPeriod := currentKesPeriod - opCertKesPeriod

	// Check if certificate has expired (exceeded max evolutions)
	if evolutionPeriod >= maxKesEvolutions {
		return 0, &OpCertError{
			Field: "kes_period",
			Message: fmt.Sprintf(
				"certificate has expired: evolution period %d >= maxKesEvolutions %d",
				evolutionPeriod,
				maxKesEvolutions,
			),
		}
	}

	return evolutionPeriod, nil
}

// ValidateOpCert performs full validation of an OpCert including:
//   - Cold signature verification
//   - KES period validity
//
// Parameters:
//   - opCert: the operational certificate to validate
//   - coldVkey: the pool's cold verification key
//   - currentSlot: the current slot number
//   - slotsPerKesPeriod: number of slots per KES period
//   - maxKesEvolutions: maximum allowed KES evolutions
//
// Returns:
//   - evolutionPeriod: the KES evolution period (for use in KES signature verification)
//   - error: if any validation fails
func ValidateOpCert(
	opCert *OpCert,
	coldVkey []byte,
	currentSlot uint64,
	slotsPerKesPeriod uint64,
	maxKesEvolutions uint64,
) (uint64, error) {
	// Verify the cold signature
	if err := VerifyOpCertSignature(opCert, coldVkey); err != nil {
		return 0, err
	}

	// Validate KES period
	evolutionPeriod, err := ValidateKesPeriod(
		opCert.KesPeriod,
		currentSlot,
		slotsPerKesPeriod,
		maxKesEvolutions,
	)
	if err != nil {
		return 0, err
	}

	return evolutionPeriod, nil
}

// OpCertFromBlockHeader extracts an OpCert from a block header.
// This is a convenience function for working with block headers.
type OpCertExtractor interface {
	OpCertHotVkey() []byte
	OpCertSequenceNumber() uint32
	OpCertKesPeriod() uint32
	OpCertSignature() []byte
}

// ExtractOpCert extracts an OpCert from any block header that implements OpCertExtractor.
// Returns nil if header is nil.
func ExtractOpCert(header OpCertExtractor) *OpCert {
	if header == nil {
		return nil
	}
	return &OpCert{
		KesVkey:       header.OpCertHotVkey(),
		IssueNumber:   uint64(header.OpCertSequenceNumber()),
		KesPeriod:     uint64(header.OpCertKesPeriod()),
		ColdSignature: header.OpCertSignature(),
	}
}

// CreateOpCert creates a new OpCert by signing with the cold key.
// This is used when creating blocks or issuing new operational certificates.
//
// Parameters:
//   - kesVkey: the KES verification key (32 bytes)
//   - issueNumber: the certificate issue/sequence number
//   - kesPeriod: the KES period at certificate creation
//   - coldSkey: the pool's cold signing key (64 bytes: seed + public key, or 32 bytes: seed only)
//
// Returns:
//   - opCert: the created operational certificate
//   - error: if signing fails
func CreateOpCert(
	kesVkey []byte,
	issueNumber uint64,
	kesPeriod uint64,
	coldSkey []byte,
) (*OpCert, error) {
	// Validate inputs
	if len(kesVkey) != kes.PublicKeySize {
		return nil, fmt.Errorf(
			"kesVkey must be %d bytes, got %d",
			kes.PublicKeySize,
			len(kesVkey),
		)
	}

	// Handle both 32-byte seed and 64-byte Ed25519 private key formats
	var privateKey ed25519.PrivateKey
	switch len(coldSkey) {
	case 32:
		// Seed format - derive the full private key
		privateKey = ed25519.NewKeyFromSeed(coldSkey)
	case 64:
		// Full private key format
		privateKey = coldSkey
	default:
		return nil, fmt.Errorf(
			"coldSkey must be 32 or 64 bytes, got %d",
			len(coldSkey),
		)
	}

	// Create the message to sign: CBOR([kes_vkey, issue_number, kes_period])
	certData := []any{
		kesVkey,
		issueNumber,
		kesPeriod,
	}

	certCbor, err := cbor.Encode(certData)
	if err != nil {
		return nil, fmt.Errorf("failed to encode certificate data: %w", err)
	}

	// Sign with cold key
	signature := ed25519.Sign(privateKey, certCbor)

	return &OpCert{
		KesVkey:       kesVkey,
		IssueNumber:   issueNumber,
		KesPeriod:     kesPeriod,
		ColdSignature: signature,
	}, nil
}
