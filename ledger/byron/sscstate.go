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

package byron

import (
	"bytes"
	"crypto/sha3"
	"errors"
	"fmt"
	"sort"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// SSC (Shared Seed Computation) payload discriminants, per Byron's body
// proof/payload sum type: sscpayload = [0, ...] / [1, ...] / [2, ...] /
// [3, ...].
const (
	SscTypeCommitments  = 0 // CommitmentsPayload: commitments + VSS certificates
	SscTypeOpenings     = 1 // OpeningsPayload: openings + VSS certificates
	SscTypeShares       = 2 // SharesPayload: shares + VSS certificates
	SscTypeCertificates = 3 // CertificatesPayload: VSS certificates only
)

// Field index of the signer's public key within each wire-level SSC entry,
// per the CDDL below. Both entry shapes carry the signer's raw public key
// rather than a pre-hashed stakeholder ID, so this package derives the
// accumulator's map key itself (see decodeIdentitySet).
const (
	commitmentPubkeyFieldIndex  = 0 // ssccomm  = [pubkey, ..., signature]
	certificatePubkeyFieldIndex = 1 // ssccert  = [vsspubkey, pubkey, ...]
)

// ByronEpochSscState accumulates the Shared Seed Computation (SSC) payload
// contributions -- commitments, openings, shares, and VSS certificates --
// carried by main blocks within a single Byron epoch, keyed by the
// contributing stakeholder's ID.
//
// NOTE: ValidateBodyProof deliberately only checks the SSC proof
// structurally (see its own NOTE) because a block's ssc_proof hashes are not
// reproducible from that block's payload in isolation: cardano-sl's SSC
// payload for a commitments/openings/shares block carries a VSS certificate
// set that is a running snapshot of every certificate seen so far in the
// epoch, so validating the proof for real requires folding every prior
// block of the epoch into state before checking any one block. This type is
// that per-epoch accumulator; ValidateBodyProofWithSscState is the validator
// that consumes it.
//
// The wire shapes decoded here (below) come from cardano-ledger's own Byron
// CDDL spec (eras/byron/ledger/impl/cddl-spec/byron.cddl in
// input-output-hk/cardano-ledger):
//
//	ssccomm  = [pubkey, [{* vsspubkey => vssenc}, vssproof], signature]
//	ssccomms = #6.258([* ssccomm])
//	sscopens = {* stakeholderid => vsssec}
//	sscshares = {* addressid => [addressid, [* vssdec]]}
//	ssccert  = [vsspubkey, pubkey, epochid, signature]
//	ssccerts = #6.258([* ssccert])
//	ssc = [0, ssccomms, ssccerts]
//	    / [1, sscopens, ssccerts]
//	    / [2, sscshares, ssccerts]
//	    / [3, ssccerts]
//
// Commitments and VSS certificates are therefore CBOR **sets** (tag 258) of
// self-contained entries, not maps: neither ssccomm nor ssccert carries an
// explicit stakeholder ID field, only the contributor's raw public key. This
// package derives each entry's accumulator key as
// Blake2b224Hash(sha3_256(cbor_serialize(pubkey))) -- the same double-hash
// primitive cardano-sl's generic `addressHash` applies to turn a
// serializable value into a StakeholderId, and the same primitive this
// package's own siblings computeByronAddressRoot (ledger/common/verify.go)
// and NewByronAddressRedeem (ledger/common/address.go) already apply to
// full Byron address-root payloads. Those two helpers are scoped to that
// specific address-root CBOR structure, not a bare public key, so
// stakeholderIDFromPubkey below re-applies the same three-step primitive
// directly to the pubkey-only case ssccomm/ssccert entries carry. Openings
// and shares, by contrast, really are CBOR maps keyed directly by a
// 28-byte ID.
//
// The canonical byte encoding this type hashes accumulated entries with
// (stakeholder IDs sorted ascending, each entry's preserved CBOR bytes
// concatenated in that order) is otherwise defined by this package and is
// internally consistent for this validator, but is not proven to reproduce
// cardano-ledger's own historical mainnet encoding of these sets/maps
// byte-for-byte.
type ByronEpochSscState struct {
	Commitments     map[common.Blake2b224][]byte
	Openings        map[common.Blake2b224][]byte
	Shares          map[common.Blake2b224][]byte
	VssCertificates map[common.Blake2b224][]byte
}

// NewByronEpochSscState returns an empty per-epoch SSC accumulator.
func NewByronEpochSscState() *ByronEpochSscState {
	return &ByronEpochSscState{
		Commitments:     make(map[common.Blake2b224][]byte),
		Openings:        make(map[common.Blake2b224][]byte),
		Shares:          make(map[common.Blake2b224][]byte),
		VssCertificates: make(map[common.Blake2b224][]byte),
	}
}

// AccumulateBlock folds a main block's own SSC payload into the epoch
// state. Callers must invoke this for every main block of the epoch, in
// block order, before validating that block or any later block's ssc_proof
// with ValidateBodyProofWithSscState against this state.
//
// See ByronEpochSscState's doc comment for the wire shapes this decodes,
// per cardano-ledger's Byron CDDL spec.
func (s *ByronEpochSscState) AccumulateBlock(block *ByronMainBlock) error {
	if block == nil {
		return fmt.Errorf("%w: block is nil", ErrBodyProofMismatch)
	}
	sscType, rest, err := decodeSscPayloadParts(block.Body.SscPayload)
	if err != nil {
		return fmt.Errorf("%w: ssc payload: %w", ErrBodyProofMismatch, err)
	}
	switch sscType {
	case SscTypeCommitments, SscTypeOpenings, SscTypeShares:
		if len(rest) != 2 {
			return fmt.Errorf(
				"%w: ssc payload type %d requires 2 elements after the "+
					"type, got %d",
				ErrBodyProofMismatch, sscType, len(rest),
			)
		}
		primary, err := decodePrimaryEntries(sscType, rest[0])
		if err != nil {
			return fmt.Errorf(
				"%w: ssc payload primary entries: %w",
				ErrBodyProofMismatch, err,
			)
		}
		certs, err := decodeIdentitySet(rest[1], certificatePubkeyFieldIndex)
		if err != nil {
			return fmt.Errorf(
				"%w: ssc payload vss certificates set: %w",
				ErrBodyProofMismatch, err,
			)
		}
		switch sscType {
		case SscTypeCommitments:
			mergeStakeholderMap(s.Commitments, primary)
		case SscTypeOpenings:
			mergeStakeholderMap(s.Openings, primary)
		case SscTypeShares:
			mergeStakeholderMap(s.Shares, primary)
		}
		mergeStakeholderMap(s.VssCertificates, certs)
	case SscTypeCertificates:
		if len(rest) != 1 {
			return fmt.Errorf(
				"%w: ssc payload type %d requires 1 element after the "+
					"type, got %d",
				ErrBodyProofMismatch, sscType, len(rest),
			)
		}
		certs, err := decodeIdentitySet(rest[0], certificatePubkeyFieldIndex)
		if err != nil {
			return fmt.Errorf(
				"%w: ssc payload vss certificates set: %w",
				ErrBodyProofMismatch, err,
			)
		}
		mergeStakeholderMap(s.VssCertificates, certs)
	default:
		return fmt.Errorf(
			"%w: unknown ssc payload type %d", ErrBodyProofMismatch, sscType,
		)
	}
	return nil
}

// decodePrimaryEntries decodes the type-specific primary field of an SSC
// payload: a CBOR set of ssccomm entries for CommitmentsPayload, or a real
// CBOR map for OpeningsPayload/SharesPayload -- see ByronEpochSscState's
// doc comment for the CDDL this follows.
func decodePrimaryEntries(
	sscType uint64,
	raw cbor.RawMessage,
) (map[common.Blake2b224][]byte, error) {
	if sscType == SscTypeCommitments {
		return decodeIdentitySet(raw, commitmentPubkeyFieldIndex)
	}
	return decodeStakeholderMap(raw)
}

// checkProof validates a block's ssc_proof (the raw, decoded proof entry at
// bodyProofSscIndex) against this epoch state's currently accumulated
// hashes.
func (s *ByronEpochSscState) checkProof(rawProof any) error {
	proofSlice, ok := rawProof.([]any)
	if !ok || len(proofSlice) < 2 {
		return fmt.Errorf(
			"%w: ssc proof is not an array of at least 2 elements, got %T",
			ErrBodyProofMismatch, rawProof,
		)
	}
	sscType, err := asUint(proofSlice[0])
	if err != nil {
		return fmt.Errorf("%w: ssc proof type: %w", ErrBodyProofMismatch, err)
	}
	switch sscType {
	case SscTypeCommitments, SscTypeOpenings, SscTypeShares:
		if len(proofSlice) < 3 {
			return fmt.Errorf(
				"%w: ssc proof type %d requires 3 elements, got %d",
				ErrBodyProofMismatch, sscType, len(proofSlice),
			)
		}
		var primaryHash common.Blake2b256
		switch sscType {
		case SscTypeCommitments:
			primaryHash = s.CommitmentsHash()
		case SscTypeOpenings:
			primaryHash = s.OpeningsHash()
		case SscTypeShares:
			primaryHash = s.SharesHash()
		}
		if err := checkHash(
			"ssc primary hash", proofSlice[1], primaryHash,
		); err != nil {
			return err
		}
		return checkHash(
			"ssc vss certificates hash",
			proofSlice[2],
			s.CertificatesHash(),
		)
	case SscTypeCertificates:
		return checkHash(
			"ssc vss certificates hash",
			proofSlice[1],
			s.CertificatesHash(),
		)
	default:
		return fmt.Errorf(
			"%w: unknown ssc proof type %d", ErrBodyProofMismatch, sscType,
		)
	}
}

// CommitmentsHash returns this state's canonical hash of every stakeholder's
// currently accumulated commitment.
func (s *ByronEpochSscState) CommitmentsHash() common.Blake2b256 {
	return canonicalMapHash(s.Commitments)
}

// OpeningsHash returns this state's canonical hash of every stakeholder's
// currently accumulated opening.
func (s *ByronEpochSscState) OpeningsHash() common.Blake2b256 {
	return canonicalMapHash(s.Openings)
}

// SharesHash returns this state's canonical hash of every stakeholder's
// currently accumulated shares.
func (s *ByronEpochSscState) SharesHash() common.Blake2b256 {
	return canonicalMapHash(s.Shares)
}

// CertificatesHash returns this state's canonical hash of every
// stakeholder's currently accumulated VSS certificate.
func (s *ByronEpochSscState) CertificatesHash() common.Blake2b256 {
	return canonicalMapHash(s.VssCertificates)
}

// decodeSscPayloadParts decodes a block's SSC payload from its preserved
// CBOR bytes into its discriminant and remaining elements, without losing
// the original per-element encoding of each remaining part.
func decodeSscPayloadParts(
	payload cbor.Value,
) (uint64, []cbor.RawMessage, error) {
	raw := payload.Cbor()
	if len(raw) == 0 {
		return 0, nil, errors.New("ssc payload has no preserved CBOR")
	}
	var parts []cbor.RawMessage
	if _, err := cbor.Decode(raw, &parts); err != nil {
		return 0, nil, fmt.Errorf("decoding ssc payload array: %w", err)
	}
	if len(parts) < 1 {
		return 0, nil, errors.New("ssc payload array is empty")
	}
	var sscType uint64
	if _, err := cbor.Decode(parts[0], &sscType); err != nil {
		return 0, nil, fmt.Errorf("decoding ssc payload type: %w", err)
	}
	return sscType, parts[1:], nil
}

// decodeStakeholderMap decodes a real CBOR map from 28-byte IDs (a
// stakeholderid or addressid, per the CDDL) to opaque entries, preserving
// each entry's original CBOR bytes. This is the wire shape of
// OpeningsPayload's and SharesPayload's primary field (sscopens/sscshares).
func decodeStakeholderMap(
	raw cbor.RawMessage,
) (map[common.Blake2b224][]byte, error) {
	var decoded map[cbor.ByteString]cbor.RawMessage
	if _, err := cbor.Decode(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decoding stakeholder map: %w", err)
	}
	result := make(map[common.Blake2b224][]byte, len(decoded))
	for key, value := range decoded {
		keyBytes := key.Bytes()
		if len(keyBytes) != common.Blake2b224Size {
			return nil, fmt.Errorf(
				"stakeholder ID has wrong length: expected %d bytes, got %d",
				common.Blake2b224Size, len(keyBytes),
			)
		}
		result[common.NewBlake2b224(keyBytes)] = []byte(value)
	}
	return result, nil
}

// decodeIdentitySet decodes a CBOR tag-258 set of self-contained entries
// (ssccomm or ssccert, per the CDDL), each a CBOR array whose element at
// pubkeyFieldIndex is the contributor's raw public key. Since neither entry
// shape carries a pre-hashed stakeholder ID, the accumulator key is derived
// with stakeholderIDFromPubkey. The full entry's original CBOR bytes are
// preserved as the map value.
func decodeIdentitySet(
	raw cbor.RawMessage,
	pubkeyFieldIndex int,
) (map[common.Blake2b224][]byte, error) {
	var set cbor.SetType[cbor.RawMessage]
	if _, err := cbor.Decode(raw, &set); err != nil {
		return nil, fmt.Errorf("decoding identity set: %w", err)
	}
	items := set.Items()
	result := make(map[common.Blake2b224][]byte, len(items))
	for i, item := range items {
		var fields []cbor.RawMessage
		if _, err := cbor.Decode(item, &fields); err != nil {
			return nil, fmt.Errorf("decoding set entry %d: %w", i, err)
		}
		if pubkeyFieldIndex >= len(fields) {
			return nil, fmt.Errorf(
				"set entry %d has %d fields, need index %d for the public key",
				i, len(fields), pubkeyFieldIndex,
			)
		}
		var pubkeyBytes []byte
		if _, err := cbor.Decode(
			fields[pubkeyFieldIndex], &pubkeyBytes,
		); err != nil {
			return nil, fmt.Errorf(
				"decoding set entry %d public key: %w", i, err,
			)
		}
		key, err := stakeholderIDFromPubkey(pubkeyBytes)
		if err != nil {
			return nil, fmt.Errorf(
				"deriving stakeholder ID for set entry %d: %w", i, err,
			)
		}
		result[key] = []byte(item)
	}
	return result, nil
}

// stakeholderIDFromPubkey derives a Byron StakeholderId from a raw public
// key using cardano-sl's generic `addressHash` primitive:
// blake2b_224(sha3_256(cbor_serialize(x))). This is the same double-hash
// primitive ledger/common's computeByronAddressRoot (verify.go) and
// NewByronAddressRedeem (address.go) apply to full Byron address-root
// payloads -- see the citation on computeByronAddressRoot ("This matches
// the Amaru implementation") -- just applied here to a bare public key
// rather than an address-root structure, since that's what ssccomm/ssccert
// entries carry (see ByronEpochSscState's doc comment).
func stakeholderIDFromPubkey(pubkey []byte) (common.Blake2b224, error) {
	encoded, err := cbor.Encode(pubkey)
	if err != nil {
		return common.Blake2b224{}, fmt.Errorf(
			"cbor-encoding public key: %w", err,
		)
	}
	sha3Sum := sha3.Sum256(encoded)
	return common.Blake2b224Hash(sha3Sum[:]), nil
}

// mergeStakeholderMap folds src into dst, with entries in src overwriting
// any existing entry for the same stakeholder.
//
// Whether a given block's own SSC payload carries only its newly-seen
// entries (a delta) or a full running snapshot of everything seen so far in
// the epoch is not settled by this package -- see ByronEpochSscState's NOTE
// on the open question of matching cardano-ledger's real accumulation
// semantics byte-for-byte. Overwrite-on-merge is correct either way: under a
// delta model it simply folds in what's new, and under a snapshot model a
// later, more complete snapshot's entries supersede the same stakeholder's
// earlier ones with no other effect.
func mergeStakeholderMap(
	dst, src map[common.Blake2b224][]byte,
) {
	for k, v := range src {
		dst[k] = v
	}
}

// canonicalMapHash computes this package's canonical hash of a stakeholder
// map: the blake2b-256 hash of each entry's stakeholder ID followed by its
// preserved CBOR bytes, concatenated in ascending stakeholder ID order.
func canonicalMapHash(m map[common.Blake2b224][]byte) common.Blake2b256 {
	keys := make([]common.Blake2b224, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i].Bytes(), keys[j].Bytes()) < 0
	})
	var buf bytes.Buffer
	for _, k := range keys {
		buf.Write(k.Bytes())
		buf.Write(m[k])
	}
	return common.Blake2b256Hash(buf.Bytes())
}
