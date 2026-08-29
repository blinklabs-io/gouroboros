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

package common

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// DuplicateLogicalMapKeyError indicates that two CBOR map keys decoded to the
// same ledger value. This cannot be left to Go's map duplicate handling when
// the decoded map is keyed by pointers, because each wire key gets a distinct
// pointer identity.
type DuplicateLogicalMapKeyError struct {
	Field string
}

func (e DuplicateLogicalMapKeyError) Error() string {
	return "duplicate logical key in " + e.Field
}

// DuplicateCertificateError indicates that a transaction certificate set
// contains the same logical certificate more than once.
type DuplicateCertificateError struct{}

func (DuplicateCertificateError) Error() string {
	return "duplicate certificate in transaction certificate set"
}

func validateLogicalMapKeys[K any, V any](
	values map[*K]V,
	field string,
	logicalKey func(*K) (string, error),
) error {
	if len(values) < 2 {
		for key := range values {
			if key == nil {
				return fmt.Errorf("%s contains a nil key", field)
			}
		}
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	for key := range values {
		if key == nil {
			return fmt.Errorf("%s contains a nil key", field)
		}
		identity, err := logicalKey(key)
		if err != nil {
			return fmt.Errorf("%s key: %w", field, err)
		}
		if _, ok := seen[identity]; ok {
			return DuplicateLogicalMapKeyError{Field: field}
		}
		seen[identity] = struct{}{}
	}
	return nil
}

func addressLogicalKey(address *Address) (string, error) {
	addressBytes, err := address.Bytes()
	if err != nil {
		return "", err
	}
	return string(addressBytes), nil
}

func credentialLogicalKey(credential *Credential) (string, error) {
	key := make([]byte, 8+len(credential.Credential))
	binary.BigEndian.PutUint64(key, uint64(credential.CredType))
	copy(key[8:], credential.Credential[:])
	return string(key), nil
}

func voterLogicalKey(voter *Voter) (string, error) {
	key := make([]byte, 1+len(voter.Hash))
	key[0] = voter.Type
	copy(key[1:], voter.Hash[:])
	return string(key), nil
}

func govActionIdLogicalKey(actionId *GovActionId) (string, error) {
	key := make([]byte, len(actionId.TransactionId)+4)
	copy(key, actionId.TransactionId[:])
	binary.BigEndian.PutUint32(
		key[len(actionId.TransactionId):],
		actionId.GovActionIdx,
	)
	return string(key), nil
}

// ValidateWithdrawalsMap rejects nil and duplicate logical address keys while
// preserving the existing pointer-keyed public representation.
func ValidateWithdrawalsMap(withdrawals map[*Address]uint64) error {
	for address := range withdrawals {
		if address == nil {
			return errors.New("withdrawals map contains a nil address")
		}
	}
	return validateLogicalMapKeys(
		withdrawals,
		"withdrawals map",
		addressLogicalKey,
	)
}

func validateCredentialMapKeys[V any](
	values map[*Credential]V,
	field string,
) error {
	return validateLogicalMapKeys(values, field, credentialLogicalKey)
}

// ValidateCertificateSet rejects duplicate logical transaction certificates.
// EncodeGeneric deliberately ignores preserved source CBOR so equivalent
// definite, indefinite, and non-shortest encodings compare by decoded value.
func ValidateCertificateSet(certificates []CertificateWrapper) error {
	var typeCounts [CertificateTypeUpdateDrep + 1]uint
	hasRepeatedType := false
	for _, certificate := range certificates {
		if certificate.Certificate == nil {
			return errors.New(
				"transaction certificate set contains a nil certificate",
			)
		}
		certificateType := certificate.Certificate.Type()
		// Certificate is a closed interface, so every implementation has a
		// type in this range.
		if certificateType > uint(CertificateTypeUpdateDrep) {
			return fmt.Errorf("invalid certificate type: %d", certificateType)
		}
		typeCounts[certificateType]++
		if typeCounts[certificateType] == 2 {
			hasRepeatedType = true
		}
	}
	if !hasRepeatedType {
		return nil
	}
	seen := make(map[string]struct{}, len(certificates))
	for _, certificate := range certificates {
		if typeCounts[certificate.Certificate.Type()] < 2 {
			continue
		}
		encoded, err := cbor.EncodeGeneric(certificate.Certificate)
		if err != nil {
			return fmt.Errorf("encode transaction certificate: %w", err)
		}
		key := string(encoded)
		if _, ok := seen[key]; ok {
			return DuplicateCertificateError{}
		}
		seen[key] = struct{}{}
	}
	return nil
}
