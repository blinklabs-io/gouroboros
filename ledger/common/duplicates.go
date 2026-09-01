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
// pointer identity. Identity is the repeated logical key, so a rejected block
// can be triaged from the error alone.
type DuplicateLogicalMapKeyError struct {
	Field    string
	Identity []byte
}

func (e DuplicateLogicalMapKeyError) Error() string {
	if len(e.Identity) == 0 {
		return "duplicate logical key in " + e.Field
	}
	return fmt.Sprintf(
		"duplicate logical key in %s: %x",
		e.Field,
		e.Identity,
	)
}

// DuplicateCertificateError indicates that a transaction certificate set
// contains the same logical certificate more than once. Index and
// CertificateType name the repeat so a rejected block can be triaged from the
// error alone.
type DuplicateCertificateError struct {
	Index           int
	CertificateType uint
}

func (e DuplicateCertificateError) Error() string {
	return fmt.Sprintf(
		"duplicate certificate in transaction certificate set: index %d, type %d",
		e.Index,
		e.CertificateType,
	)
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
			return DuplicateLogicalMapKeyError{
				Field:    field,
				Identity: []byte(identity),
			}
		}
		seen[identity] = struct{}{}
	}
	return nil
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

func validateCredentialMapKeys[V any](
	values map[*Credential]V,
	field string,
) error {
	return validateLogicalMapKeys(values, field, credentialLogicalKey)
}

// appendLogicalNumber writes a fixed-width big-endian image of value. The
// conversion is deliberate: an identity needs an injective bit pattern rather
// than an arithmetic value, so a negative input is reinterpreted on purpose and
// two different inputs cannot collide.
func appendLogicalNumber[T ~int | ~int64 | ~uint | ~uint64](
	dst []byte,
	value T,
) []byte {
	var buf [8]byte
	//nolint:gosec // G115: the bit pattern is the identity, not the magnitude
	binary.BigEndian.PutUint64(buf[:], uint64(value))
	return append(dst, buf[:]...)
}

// appendLogicalBytes length-prefixes so adjacent variable-length fields cannot
// be confused with one another.
func appendLogicalBytes(dst []byte, value []byte) []byte {
	dst = appendLogicalNumber(dst, len(value))
	return append(dst, value...)
}

func appendLogicalCredential(dst []byte, credential Credential) []byte {
	dst = appendLogicalNumber(dst, credential.CredType)
	return append(dst, credential.Credential[:]...)
}

func appendLogicalDrep(dst []byte, drep Drep) []byte {
	dst = appendLogicalNumber(dst, drep.Type)
	return appendLogicalBytes(dst, drep.Credential)
}

func appendLogicalAnchor(dst []byte, anchor *GovAnchor) []byte {
	if anchor == nil {
		return append(dst, 0)
	}
	dst = append(dst, 1)
	dst = appendLogicalBytes(dst, []byte(anchor.Url))
	return append(dst, anchor.DataHash[:]...)
}

// certificateLogicalKey builds a certificate identity from its decoded fields,
// so two encodings of the same certificate compare equal without re-encoding
// it. Types that aggregate over slices or maps fall back to a canonical
// re-encoding, which keeps the reflection cost off the ordinary decode path.
func certificateLogicalKey(certificate Certificate) (string, error) {
	key := appendLogicalNumber(
		make([]byte, 0, 64),
		certificate.Type(),
	)
	switch c := certificate.(type) {
	case *StakeRegistrationCertificate:
		key = appendLogicalCredential(key, c.StakeCredential)
	case *StakeDeregistrationCertificate:
		key = appendLogicalCredential(key, c.StakeCredential)
	case *StakeDelegationCertificate:
		if c.StakeCredential == nil {
			return "", errors.New(
				"stake delegation certificate has a nil stake credential",
			)
		}
		key = appendLogicalCredential(key, *c.StakeCredential)
		key = append(key, c.PoolKeyHash[:]...)
	case *PoolRetirementCertificate:
		key = append(key, c.PoolKeyHash[:]...)
		key = appendLogicalNumber(key, c.Epoch)
	case *GenesisKeyDelegationCertificate:
		key = appendLogicalBytes(key, c.GenesisHash)
		key = appendLogicalBytes(key, c.GenesisDelegateHash)
		key = append(key, c.VrfKeyHash[:]...)
	case *RegistrationCertificate:
		key = appendLogicalCredential(key, c.StakeCredential)
		key = appendLogicalNumber(key, c.Amount)
	case *DeregistrationCertificate:
		key = appendLogicalCredential(key, c.StakeCredential)
		key = appendLogicalNumber(key, c.Amount)
	case *VoteDelegationCertificate:
		key = appendLogicalCredential(key, c.StakeCredential)
		key = appendLogicalDrep(key, c.Drep)
	case *StakeVoteDelegationCertificate:
		key = appendLogicalCredential(key, c.StakeCredential)
		key = append(key, c.PoolKeyHash[:]...)
		key = appendLogicalDrep(key, c.Drep)
	case *StakeRegistrationDelegationCertificate:
		key = appendLogicalCredential(key, c.StakeCredential)
		key = append(key, c.PoolKeyHash[:]...)
		key = appendLogicalNumber(key, c.Amount)
	case *VoteRegistrationDelegationCertificate:
		key = appendLogicalCredential(key, c.StakeCredential)
		key = appendLogicalDrep(key, c.Drep)
		key = appendLogicalNumber(key, c.Amount)
	case *StakeVoteRegistrationDelegationCertificate:
		key = appendLogicalCredential(key, c.StakeCredential)
		key = append(key, c.PoolKeyHash[:]...)
		key = appendLogicalDrep(key, c.Drep)
		key = appendLogicalNumber(key, c.Amount)
	case *AuthCommitteeHotCertificate:
		key = appendLogicalCredential(key, c.ColdCredential)
		key = appendLogicalCredential(key, c.HotCredential)
	case *ResignCommitteeColdCertificate:
		key = appendLogicalCredential(key, c.ColdCredential)
		key = appendLogicalAnchor(key, c.Anchor)
	case *RegistrationDrepCertificate:
		key = appendLogicalCredential(key, c.DrepCredential)
		key = appendLogicalNumber(key, c.Amount)
		key = appendLogicalAnchor(key, c.Anchor)
	case *DeregistrationDrepCertificate:
		key = appendLogicalCredential(key, c.DrepCredential)
		key = appendLogicalNumber(key, c.Amount)
	case *UpdateDrepCertificate:
		key = appendLogicalCredential(key, c.DrepCredential)
		key = appendLogicalAnchor(key, c.Anchor)
	default:
		// PoolRegistrationCertificate and MoveInstantaneousRewardsCertificate
		// aggregate over slices and maps, and a certificate type added later
		// has no enumerated identity here yet. A canonical re-encoding is a
		// correct identity for all of them, so a new certificate type keeps
		// working without touching this file.
		encoded, err := cbor.EncodeGeneric(certificate)
		if err != nil {
			return "", fmt.Errorf("encode transaction certificate: %w", err)
		}
		key = appendLogicalBytes(key, encoded)
	}
	return string(key), nil
}

// ValidateCertificateSet rejects duplicate logical transaction certificates.
// The identity is built from decoded fields, so equivalent definite,
// indefinite, and non-shortest encodings compare equal.
func ValidateCertificateSet(certificates []CertificateWrapper) error {
	typeCounts := make(map[uint]int, len(certificates))
	hasRepeatedType := false
	for _, certificate := range certificates {
		if certificate.Certificate == nil {
			return errors.New(
				"transaction certificate set contains a nil certificate",
			)
		}
		certificateType := certificate.Certificate.Type()
		typeCounts[certificateType]++
		if typeCounts[certificateType] == 2 {
			hasRepeatedType = true
		}
	}
	if !hasRepeatedType {
		return nil
	}
	seen := make(map[string]struct{}, len(certificates))
	for index, certificate := range certificates {
		certificateType := certificate.Certificate.Type()
		if typeCounts[certificateType] < 2 {
			continue
		}
		key, err := certificateLogicalKey(certificate.Certificate)
		if err != nil {
			return err
		}
		if _, ok := seen[key]; ok {
			return DuplicateCertificateError{
				Index:           index,
				CertificateType: certificateType,
			}
		}
		seen[key] = struct{}{}
	}
	return nil
}
