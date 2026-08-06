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
	"crypto/sha3"
	"errors"
	"fmt"

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
// stakeholderIDFromPubkeyCbor below re-applies the same three-step
// primitive directly to the pubkey-only case ssccomm/ssccert entries carry.
// Openings and shares, by contrast, really are CBOR maps keyed directly by
// a 28-byte ID.
//
// The canonical byte encoding this type hashes accumulated entries with is
// the CBOR encoding of a genuine, definite-length map from each 28-byte
// stakeholder ID to that entry's preserved CBOR bytes, keys ascending (see
// canonicalMapHash) -- matching cardano-sl's own HashMap StakeholderId ...
// hashes, not a bespoke concatenation of raw bytes. This is confirmed for
// the empty-map case against the bundled mainnet fixture's own (empty)
// CertificatesPayload, whose header ssc_proof hash this package now
// reproduces exactly (see TestValidateBodyHashWithSscState_RealMainnetBlock
// in consensus/byron); it is not separately confirmed against a real,
// non-empty mainnet ssc_proof, since no such fixture is available.
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
			mergeStakeholderMap(&s.Commitments, primary)
		case SscTypeOpenings:
			mergeStakeholderMap(&s.Openings, primary)
		case SscTypeShares:
			mergeStakeholderMap(&s.Shares, primary)
		}
		mergeStakeholderMap(&s.VssCertificates, certs)
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
		mergeStakeholderMap(&s.VssCertificates, certs)
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
//
// expectedType is the discriminant of the block's own SscPayload (see
// decodeSscPayloadParts); the proof's declared type is required to match
// it, so a header whose ssc_proof structurally claims a different SSC type
// than the body's actual payload is rejected here rather than silently
// checked against the wrong shape of accumulated state.
func (s *ByronEpochSscState) checkProof(
	rawProof any,
	expectedType uint64,
) error {
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
	if sscType != expectedType {
		return fmt.Errorf(
			"%w: ssc proof declares type %d, but the block's own ssc "+
				"payload is type %d",
			ErrBodyProofMismatch, sscType, expectedType,
		)
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
		if len(proofSlice) != 2 {
			return fmt.Errorf(
				"%w: ssc proof type %d requires exactly 2 elements, got %d",
				ErrBodyProofMismatch, sscType, len(proofSlice),
			)
		}
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
// with stakeholderIDFromPubkeyCbor, applied directly to the field's
// preserved CBOR bytes rather than a decoded-and-re-encoded copy (see that
// function's doc comment for why). The full entry's original CBOR bytes are
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
		key := stakeholderIDFromPubkeyCbor(fields[pubkeyFieldIndex])
		result[key] = []byte(item)
	}
	return result, nil
}

// stakeholderIDFromPubkeyCbor derives a Byron StakeholderId from the
// preserved CBOR encoding of a raw public key, applying cardano-sl's
// generic `addressHash` primitive: blake2b_224(sha3_256(cbor_bytes)). This
// is the same double-hash primitive ledger/common's computeByronAddressRoot
// (verify.go) and NewByronAddressRedeem (address.go) apply to full Byron
// address-root payloads -- see the citation on computeByronAddressRoot
// ("This matches the Amaru implementation") -- just applied here to a bare
// public key rather than an address-root structure, since that's what
// ssccomm/ssccert entries carry (see ByronEpochSscState's doc comment).
//
// The caller must pass the field's original, preserved CBOR bytes, not a
// decoded-and-re-encoded copy: if the wire encoding is not canonical (e.g.
// an indefinite-length byte string, or a non-minimal length header), the
// re-encoded bytes would differ from what was actually hashed on the wire,
// silently deriving the wrong stakeholder ID.
func stakeholderIDFromPubkeyCbor(pubkeyCbor []byte) common.Blake2b224 {
	sha3Sum := sha3.Sum256(pubkeyCbor)
	return common.Blake2b224Hash(sha3Sum[:])
}

// mergeStakeholderMap folds src into *dst, with entries in src overwriting
// any existing entry for the same stakeholder.
//
// dst is a pointer to the destination map, rather than the map itself,
// because a caller may reach AccumulateBlock via a zero-value
// &ByronEpochSscState{} instead of NewByronEpochSscState(), leaving the
// destination field nil; assigning through the pointer here lazily
// initializes it in place instead of panicking with "assignment to entry in
// nil map" or silently requiring the constructor.
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
	dst *map[common.Blake2b224][]byte,
	src map[common.Blake2b224][]byte,
) {
	if *dst == nil {
		*dst = make(map[common.Blake2b224][]byte, len(src))
	}
	for k, v := range src {
		(*dst)[k] = v
	}
}

// canonicalMapHash computes this package's canonical hash of a stakeholder
// map: the blake2b-256 hash of the CBOR encoding of a genuine, definite-
// length CBOR map from each 28-byte stakeholder ID to that entry's
// preserved CBOR bytes, with keys ordered per RFC 8949's core deterministic
// encoding (ascending, since all keys here are fixed-length byte strings).
//
// This mirrors cardano-sl's own commitment/opening/share/certificate
// hashes, which are computed over the CBOR encoding of a genuine map
// (HashMap StakeholderId ...), not a bespoke concatenation of raw bytes:
// for the bundled mainnet fixture's empty VssCertificatesMap, this produces
// blake2b256(0xa0) -- the CBOR encoding of an empty map -- which matches
// the real header's ssc_proof hash for that block exactly.
func canonicalMapHash(m map[common.Blake2b224][]byte) common.Blake2b256 {
	encoded := make(map[cbor.ByteString]cbor.RawMessage, len(m))
	for k, v := range m {
		encoded[cbor.NewByteString(k.Bytes())] = cbor.RawMessage(v)
	}
	data, err := cbor.Encode(encoded)
	if err != nil {
		// encoded's keys are fixed-length byte strings and its values are
		// already-valid, previously-decoded CBOR bytes; this cannot fail.
		panic("CBOR encoding that should never fail has failed: " + err.Error())
	}
	return common.Blake2b256Hash(data)
}
