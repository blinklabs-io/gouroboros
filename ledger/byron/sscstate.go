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

// Field index of the signer's public key within each wire-level SSC entry.
// Both entry shapes carry the signer's raw public key rather than a
// pre-hashed stakeholder ID, so this package derives the certificate
// accumulator's map key itself (see decodeIdentitySet).
//
// certificatePubkeyFieldIndex is 3, not 1: cardano-ledger's own published
// Byron CDDL (eras/byron/ledger/impl/cddl-spec/byron.cddl in
// input-output-hk/cardano-ledger) states
// "ssccert = [vsspubkey, pubkey, epochid, signature]", putting pubkey at
// index 1. Real, non-empty mainnet ssccert entries (fetched directly off
// mainnet -- see sscstate_real_test.go) instead have a plain small uint
// (the shared expiry epoch) at index 1, a 64-byte signature at index 2, and
// the actual 64-byte extended signing key at index 3 -- i.e. the real wire
// order is [vsspubkey, epochid, signature, pubkey], matching cardano-sl's
// own VssCertificate record field order (vcVssKey, vcExpiryEpoch,
// vcSignature, vcSigningKey), not the CDDL comment. Confirmed by the value
// at index 3 being byte-identical to the ssccomm pubkey field of other real
// commitment entries from the same handful of genesis-era stakeholders. The
// CDDL file appears never to have been corrected for this field order
// because cardano-ledger's own EncCBOR for this payload never reconstructs
// real SSC content in the first place (see ByronEpochSscState's doc
// comment) -- nothing in that codebase ever exercises this field order
// against real data. Using index 1 here (matching the written but
// apparently incorrect CDDL) makes decodeIdentitySet reject every real,
// non-empty ssccert entry, since a small uint fails its byte-string check.
const (
	commitmentPubkeyFieldIndex = 0 // ssccomm = [pubkey, ..., signature]
	// ssccert = [vsspubkey, epochid, signature, pubkey]
	certificatePubkeyFieldIndex = 3
)

// ByronEpochSscState is a cross-block registry of the Shared Seed
// Computation (SSC) contributions -- commitments, openings, shares, and VSS
// certificates -- seen so far while walking a Byron epoch's main blocks in
// order, keyed by the contributing stakeholder's ID.
//
// This type is NOT connected to ssc_proof validation, despite its name and
// this package's original design intent for it: real ssc_proof hashes turn
// out to be entirely block-local, like every other body-proof component
// (tx_proof, dlg_proof, upd_proof) -- see checkSscProofLocal's doc comment
// (bodyproof.go) for how this was confirmed against real, non-empty
// mainnet data, and why. ValidateBodyProof validates a block's ssc_proof
// entirely from that block's own payload and never consults this type or
// AccumulateBlock. This type remains available purely as a convenience for
// callers that want a running view of an epoch's registered stakeholders
// for some other purpose (e.g. a chain follower inspecting VSS certificate
// coverage over time) -- not because anything in this package still needs
// it for proof validation.
//
// The wire shapes decoded here (below) come from cardano-ledger's own Byron
// CDDL spec (eras/byron/ledger/impl/cddl-spec/byron.cddl in
// input-output-hk/cardano-ledger), with one correction: that spec's
// "ssccert" comment orders pubkey before epochid; real mainnet data instead
// orders them as shown below, matching cardano-sl's own VssCertificate
// record field order -- see certificatePubkeyFieldIndex's doc comment for
// how this was confirmed and why the published CDDL comment is wrong here.
//
//	ssccomm  = [pubkey, [{* vsspubkey => vssenc}, vssproof], signature]
//	ssccomms = #6.258([* ssccomm])
//	sscopens = {* stakeholderid => vsssec}
//	sscshares = {* addressid => sharesmap}
//	sharesmap = {* addressid => vssdec}  ; a nested map, per
//	    cardano-ledger's dropSharesMap/dropInnerSharesMap decoder -- not the
//	    array shape an earlier version of this comment claimed
//	ssccert  = [vsspubkey, epochid, signature, pubkey]
//	ssccerts = #6.258([* ssccert])
//	ssc = [0, ssccomms, ssccerts]
//	    / [1, sscopens, ssccerts]
//	    / [2, sscshares, ssccerts]
//	    / [3, ssccerts]
//
// Commitments and VSS certificates are therefore CBOR **sets** (tag 258) of
// self-contained entries, not maps: neither ssccomm nor ssccert carries an
// explicit stakeholder ID field, only the contributor's raw public key. This
// package derives each certificate entry's map key as
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
// See canonicalMapHash's doc comment for the certificate hash's canonical
// byte encoding (a genuine CBOR map, not the wire's tag-258 set) and
// checkSscProofLocal's doc comment for how all of this is now confirmed
// against real, non-empty mainnet data.
type ByronEpochSscState struct {
	Commitments     map[common.Blake2b224][]byte
	Openings        map[common.Blake2b224][]byte
	Shares          map[common.Blake2b224][]byte
	VssCertificates map[common.Blake2b224][]byte
}

// NewByronEpochSscState returns an empty per-epoch SSC registry.
func NewByronEpochSscState() *ByronEpochSscState {
	return &ByronEpochSscState{
		Commitments:     make(map[common.Blake2b224][]byte),
		Openings:        make(map[common.Blake2b224][]byte),
		Shares:          make(map[common.Blake2b224][]byte),
		VssCertificates: make(map[common.Blake2b224][]byte),
	}
}

// AccumulateBlock folds a main block's own SSC payload into the registry.
//
// This is not needed, and never consulted, when validating a block's
// ssc_proof: ValidateBodyProof validates entirely from each block's own
// payload (see ByronEpochSscState's doc comment). Calling this remains
// useful only for callers that want a running view of an epoch's
// registered stakeholders for some other purpose.
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

// checkSscProofLocal validates a block's ssc_proof (the raw, decoded proof
// entry at bodyProofSscIndex) entirely from that same block's own,
// already-decoded SscPayload parts (rest, as returned by
// decodeSscPayloadParts) -- no cross-block state needed.
//
// This is the opt-in, full-hash-comparison form (checkSscProofCore): it recomputes each hash the header claims and rejects
// the block unless the header's value matches exactly. ValidateBodyProof
// does NOT call this by default -- see checkSscProofCore's doc comment
// and common.VerifyConfig.EnableByronSscProofHashValidation for why that
// comparison is opt-in rather than applied unconditionally to every
// decoded Byron main block, and checkSscProofShape for the structural-only
// form ValidateBodyProof runs instead by default.
//
// This replaced an earlier, epoch-accumulation-based design (see
// ByronEpochSscState's doc comment) after real, non-empty mainnet vectors
// disproved its premise. Concretely, for three real, consecutive
// CommitmentsPayload blocks within the same Byron epoch (mainnet slots
// 21601, 21602, 21603), each block's own real header ssc_proof commitments
// hash matches blake2b256 of *that block's own* commitments set bytes
// alone -- not a hash of any multi-block union, and not a hash of a
// stakeholder-keyed map rebuilt from that set. The same holds for a real
// OpeningsPayload block (slot 30240) against its own openings map. A real
// CommitmentsPayload block carrying 7 VSS certificates from 7 distinct
// stakeholders (slot 129601, epoch 6) likewise matches its own certificate
// set, but that block happens to be its epoch's *first* main block, so this
// vector alone cannot distinguish "this block's own certificates" from "the
// epoch's accumulated certificates so far" for the certificates field
// specifically (they're identical at that point in the epoch). What does
// establish certificate block-locality is cardano-sl's own type
// declarations: SscPayload's constructors each carry their own
// VssCertificatesMap value directly (see e.g. `CommitmentsPayload
// !CommitmentsMap !VssCertificatesMap`), not a reference to any
// epoch-spanning accumulator, so there is no accumulated state for a later
// block's proof to depend on in the first place. See sscstate_real_test.go
// for the fixtures and assertions the mainnet-vector claims above rest on.
//
// NOTE: a discriminating real vector -- a non-first main block in an
// epoch that already has non-empty VSS certificates from an earlier block
// in that same epoch -- would settle the block-local-vs-epoch-accumulated
// question for certificates definitively (block-local predicts that
// block's own certificate set alone; epoch-accumulated predicts it
// wouldn't). A chain-sync scan from mainnet slot 129601 (epoch 6's first
// main block, the one 7-certificate vector already confirmed above)
// forward through slot 145401 -- covering the main blocks across that
// 15,801-slot range (slots 129601-145401 inclusive; Byron allows at most
// one main block per slot, so that is an upper bound on the block count,
// not the count itself), roughly the first three-quarters of epoch 6 --
// found none: every other block in that range carried an empty VSS
// certificate set. That is not itself evidence for either hypothesis (an
// epoch-accumulated implementation could just as easily produce
// all-empty certificate fields on every later block, if VSS certificates
// are only actually (re)broadcast once near each epoch's start); it only
// means the scan did not turn up a vector that could distinguish the two.
// The conclusion that certificates hash block-locally therefore still
// rests on cardano-sl's type-level SscPayload argument in the paragraph
// above alone, not on a discriminating real vector -- treat it as
// well-supported, not as settled by direct observation.
//
// The two SSC payload categories hash differently, matching cardano-sl's
// own documented behavior (input-output-hk/cardano-sl,
// docs/block-processing/types.md, on VssCertificatesHash):
//
//   - Commitments, openings, and shares hash as blake2b256 of that field's
//     own preserved wire bytes directly (a tag-258 set for commitments, a
//     genuine CBOR map for openings/shares) -- no reconstruction at all.
//   - VSS certificates hash as blake2b256 of a *rebuilt*, genuine CBOR map
//     from each entry's stakeholder ID (derived from the corrected pubkey
//     field -- see certificatePubkeyFieldIndex) to that entry's own
//     preserved CBOR bytes, even though the wire format for certificates
//     is the same kind of tag-258 set as commitments. cardano-sl's own
//     docs explain why: "hashing is done after serialization, and at some
//     point the serialization format for VssCertificatesHash was changed
//     from a map to a set. Since we can't change the protocol easily at
//     this point, for hashing we still use the map representation."
//
// SharesPayload's own hash is not independently confirmed against a real,
// non-empty example: a genuinely non-empty SharesPayload only arises when
// a commitment's own contributor fails to reveal their opening directly
// and other stakeholders instead reveal decrypted shares of it -- a
// failure-path event not found while scanning real mainnet blocks from
// genesis through slot 1,650,000 (epoch 76, ~October 2018). That scan
// covers roughly the first third of the classic-Ouroboros SSC era, not
// "essentially the entire window" an earlier version of this comment
// claimed: the OBFT hard fork (the end of that era) landed around March
// 2019, epoch 105-108, some 30 epochs later. The scanning tooling used has
// since been deleted and no log or artifact of exactly what it checked
// survives, so treat "an exhaustive scan found nothing" as an unreproduced
// claim, not an established fact.
//
// This is not purely a guess, though: SscTypeShares is handled by the
// exact same code path as SscTypeOpenings immediately below (same case
// branch, same blake2b256-of-raw-bytes computation), over the same kind
// of wire shape per the CDDL -- both sscopens and sscshares are genuine
// CBOR maps keyed by a 28-byte ID (see ByronEpochSscState's doc comment)
// -- so confirming SscTypeOpenings's hash construction, as real data now
// has, provides strong indirect evidence for SscTypeShares's too, even
// without a genuinely non-empty SharesPayload vector to confirm it
// directly. cardano-sl's own SscPayload type declares
// `SharesProof !(Hash SharesMap) !VssCertificatesHash` -- a plain hash of
// the shares map, with no map-vs-set encoding quirk of its own; that quirk
// (see the VssCertificatesHash discussion above) is specific to
// VssCertificatesMap/VssCertificatesHash, not shares.
//
// expectedType is the discriminant of the block's own SscPayload (see
// decodeSscPayloadParts); the proof's declared type is required to match
// it, so a header whose ssc_proof structurally claims a different SSC type
// than the body's actual payload is rejected here rather than silently
// checked against the wrong shape of payload.
//
// Before hashing rest[0] (the primary field: commitments, openings, or
// shares) this also decodes it with decodePrimaryEntries -- the same
// shape-validating decoder AccumulateBlock uses -- and rejects the block if
// it fails. This is not redundant with checkHash immediately below: a
// hash-only check authenticates *some* bytes, but says nothing about
// whether those bytes are the CBOR shape the Byron wire format actually
// requires (a tag-258 set for commitments per dropCommitmentsMap, a genuine
// CBOR map for openings/shares per dropOpeningsMap/dropSharesMap in
// cardano-ledger's Cardano.Chain.Ssc). Without this gate, an untagged array
// (or any other wrong-shaped value) could be paired with a freshly
// recomputed header hash and pass checkHash, accepting a payload the real
// Byron node's own decoder would reject outright at decode time. The
// certificates field (rest[1] / rest[0] for SscTypeCertificates) is already
// shape-validated this way via localCertificatesHash -> decodeIdentitySet;
// this closes the same gap for the primary field.
func checkSscProofLocal(
	rawProof any,
	expectedType uint64,
	rest []cbor.RawMessage,
) error {
	return checkSscProofCore(rawProof, expectedType, rest)
}

// checkSscProofShape validates a block's ssc_proof as far as
// cardano-ledger's dropSscProof does, and validates the separately decoded
// SSC payload as far as dropSscPayload does. This is what
// ValidateBodyProof runs by default for every decoded Byron main block
// (see NewByronMainBlockFromCbor), so anything it rejects is a whole-block
// decode failure; checkSscProofLocal's full hash comparison, and the
// stricter wire shapes it needs to compute those hashes, are opt-in via
// common.VerifyConfig.EnableByronSscProofHashValidation, exposed through
// ByronMainBlock.ValidateSscProof.
//
// dropSscProof reads a list length, then a Word8 tag, then matchSize
// against 3 for tags 0-2 and 2 for tag 3, then a dropBytes per hash slot,
// and errors on any other tag (Cardano/Chain/Ssc.hs:169-188). dropBytes is
// void decodeBytes: a byte string of any length, never interpreted
// (Cardano/Ledger/Binary/Decoding/Drop.hs:28-29). So the hash slots are
// checked for being byte strings and nothing more.
//
// dropSscPayload is a separate decoder run over a separate field and never
// sees the proof's tag (Cardano/Chain/Ssc.hs:75-90), so the payload's own
// arity and nested wire shapes are checked against the payload's own type
// here rather than against the proof's, and the two types are not compared.
func checkSscProofShape(
	rawProof any,
	expectedType uint64,
	rest []cbor.RawMessage,
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
	proofHashes := 0
	switch sscType {
	case SscTypeCommitments, SscTypeOpenings, SscTypeShares:
		proofHashes = 2
	case SscTypeCertificates:
		proofHashes = 1
	default:
		return fmt.Errorf(
			"%w: unknown ssc proof type %d", ErrBodyProofMismatch, sscType,
		)
	}
	if len(proofSlice) != proofHashes+1 {
		return fmt.Errorf(
			"%w: ssc proof type %d requires %d elements, got %d",
			ErrBodyProofMismatch, sscType, proofHashes+1, len(proofSlice),
		)
	}
	for i := 1; i <= proofHashes; i++ {
		if _, ok := proofSlice[i].([]byte); !ok {
			return fmt.Errorf(
				"%w: ssc proof element %d is not a byte string, got %T",
				ErrBodyProofMismatch, i, proofSlice[i],
			)
		}
	}
	if err := validateSscPayloadShape(expectedType, rest); err != nil {
		return fmt.Errorf("%w: ssc payload shape: %w", ErrBodyProofMismatch, err)
	}
	return nil
}

func validateSscPayloadShape(
	sscType uint64,
	rest []cbor.RawMessage,
) error {
	switch sscType {
	case SscTypeCommitments:
		if len(rest) != 2 {
			return fmt.Errorf(
				"ssc payload type %d requires 2 elements after the type, got %d",
				sscType, len(rest),
			)
		}
		if err := validateSscSet(rest[0], validateSignedCommitment); err != nil {
			return fmt.Errorf("commitments: %w", err)
		}
		return validateSscSet(rest[1], validateVssCertificate)
	case SscTypeOpenings:
		if len(rest) != 2 {
			return fmt.Errorf(
				"ssc payload type %d requires 2 elements after the type, got %d",
				sscType, len(rest),
			)
		}
		if err := validateSscByteStringMap(
			rest[0], validateSscByteString, "openings",
		); err != nil {
			return err
		}
		return validateSscSet(rest[1], validateVssCertificate)
	case SscTypeShares:
		if len(rest) != 2 {
			return fmt.Errorf(
				"ssc payload type %d requires 2 elements after the type, got %d",
				sscType, len(rest),
			)
		}
		if err := validateSscSharesMap(rest[0]); err != nil {
			return err
		}
		return validateSscSet(rest[1], validateVssCertificate)
	case SscTypeCertificates:
		if len(rest) != 1 {
			return fmt.Errorf(
				"ssc payload type %d requires 1 element after the type, got %d",
				sscType, len(rest),
			)
		}
		return validateSscSet(rest[0], validateVssCertificate)
	default:
		return fmt.Errorf("unknown ssc payload type %d", sscType)
	}
}

func validateSscSet(
	raw cbor.RawMessage,
	validateEntry func(cbor.RawMessage) error,
) error {
	if len(raw) == 0 || raw[0]&cbor.CborTypeMask != cbor.CborTypeTag {
		return errors.New("expected a tagged set")
	}
	var tag cbor.RawTag
	if _, err := cbor.Decode(raw, &tag); err != nil {
		return fmt.Errorf("decode set tag: %w", err)
	}
	if len(tag.Content) == 0 ||
		tag.Content[0]&cbor.CborTypeMask != cbor.CborTypeArray {
		return errors.New("set tag content must be a CBOR list")
	}
	itemCount, _, indefinite := cbor.ArrayInfo(tag.Content)
	if itemCount < 0 || indefinite {
		return errors.New("set tag content must be a definite-length CBOR list")
	}
	var entries []cbor.RawMessage
	if _, err := cbor.Decode(tag.Content, &entries); err != nil {
		return fmt.Errorf("decode set entries: %w", err)
	}
	if len(entries) != itemCount {
		return fmt.Errorf(
			"set has %d decoded entries, expected %d", len(entries), itemCount,
		)
	}
	for idx, entry := range entries {
		if err := validateEntry(entry); err != nil {
			return fmt.Errorf("set entry %d: %w", idx, err)
		}
	}
	return nil
}

func validateSscByteString(raw cbor.RawMessage) error {
	if len(raw) == 0 || raw[0]&cbor.CborTypeMask != cbor.CborTypeByteString {
		return errors.New("expected a CBOR byte string")
	}
	return nil
}

func validateSscByteStringMap(
	raw cbor.RawMessage,
	validateValue func(cbor.RawMessage) error,
	name string,
) error {
	return validateSscMap(
		raw, validateSscByteString, validateValue, name,
	)
}

func validateSscMap(
	raw cbor.RawMessage,
	validateKey func(cbor.RawMessage) error,
	validateValue func(cbor.RawMessage) error,
	name string,
) error {
	decoder, err := cbor.NewStreamDecoder(raw)
	if err != nil {
		return fmt.Errorf("decode %s map: %w", name, err)
	}
	pairCount, _, _, err := decoder.DecodeMapHeader()
	if err != nil {
		return fmt.Errorf("%s must be a definite-length CBOR map: %w", name, err)
	}
	for idx := 0; idx < pairCount; idx++ {
		keyOffset, keyLength, err := decoder.Skip()
		if err != nil {
			return fmt.Errorf("decode %s map key %d: %w", name, idx, err)
		}
		key := cbor.RawMessage(decoder.RawBytes(keyOffset, keyLength))
		if err := validateKey(key); err != nil {
			return fmt.Errorf("%s map key %d: %w", name, idx, err)
		}
		valueOffset, valueLength, err := decoder.Skip()
		if err != nil {
			return fmt.Errorf("decode %s map value %d: %w", name, idx, err)
		}
		value := cbor.RawMessage(decoder.RawBytes(valueOffset, valueLength))
		if err := validateValue(value); err != nil {
			return fmt.Errorf("%s map value %d: %w", name, idx, err)
		}
	}
	if !decoder.EOF() {
		return fmt.Errorf("%s map has trailing CBOR data", name)
	}
	return nil
}

func validateSscSharesMap(raw cbor.RawMessage) error {
	return validateSscByteStringMap(raw, func(value cbor.RawMessage) error {
		return validateSscByteStringMap(value, func(inner cbor.RawMessage) error {
			return validateSscByteStringList(inner, "share value")
		}, "inner shares")
	}, "shares")
}

func validateSscByteStringList(raw cbor.RawMessage, name string) error {
	itemCount, _, indefinite := cbor.ArrayInfo(raw)
	if itemCount < 0 || !indefinite {
		return fmt.Errorf("%s must be an indefinite-length CBOR list", name)
	}
	var items []cbor.RawMessage
	if _, err := cbor.Decode(raw, &items); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	for idx, item := range items {
		if err := validateSscByteString(item); err != nil {
			return fmt.Errorf("%s item %d: %w", name, idx, err)
		}
	}
	return nil
}

func validateSscArray(
	raw cbor.RawMessage,
	length int,
	name string,
) ([]cbor.RawMessage, error) {
	itemCount, _, indefinite := cbor.ArrayInfo(raw)
	if itemCount < 0 || indefinite {
		return nil, fmt.Errorf("%s must be a definite-length CBOR list", name)
	}
	if itemCount != length {
		return nil, fmt.Errorf(
			"%s has %d fields, expected %d", name, itemCount, length,
		)
	}
	var fields []cbor.RawMessage
	if _, err := cbor.Decode(raw, &fields); err != nil {
		return nil, fmt.Errorf("decode %s: %w", name, err)
	}
	if len(fields) != itemCount {
		return nil, fmt.Errorf(
			"%s has %d decoded fields, expected %d",
			name, len(fields), itemCount,
		)
	}
	return fields, nil
}

func validateSignedCommitment(raw cbor.RawMessage) error {
	fields, err := validateSscArray(raw, 3, "signed commitment")
	if err != nil {
		return err
	}
	if err := validateSscByteString(fields[0]); err != nil {
		return fmt.Errorf("verification key: %w", err)
	}
	if err := validateCommitment(fields[1]); err != nil {
		return err
	}
	return validateSscByteString(fields[2])
}

func validateCommitment(raw cbor.RawMessage) error {
	fields, err := validateSscArray(raw, 2, "commitment")
	if err != nil {
		return err
	}
	if err := validateSscByteStringMap(
		fields[0], func(value cbor.RawMessage) error {
			return validateSscByteStringList(value, "commitment shares")
		}, "commitment shares",
	); err != nil {
		return err
	}
	return validateSecretProof(fields[1])
}

func validateSecretProof(raw cbor.RawMessage) error {
	fields, err := validateSscArray(raw, 4, "secret proof")
	if err != nil {
		return err
	}
	for idx := 0; idx < 3; idx++ {
		if err := validateSscByteString(fields[idx]); err != nil {
			return fmt.Errorf("secret proof field %d: %w", idx, err)
		}
	}
	return validateSscByteStringList(
		fields[3], "secret proof commitments",
	)
}

func validateVssCertificate(raw cbor.RawMessage) error {
	fields, err := validateSscArray(raw, 4, "VSS certificate")
	if err != nil {
		return err
	}
	if err := validateSscByteString(fields[0]); err != nil {
		return fmt.Errorf("verification key: %w", err)
	}
	if len(fields[1]) == 0 || fields[1][0]&cbor.CborTypeMask != 0x00 {
		return errors.New("epoch must be an unsigned integer")
	}
	var epoch uint64
	if _, err := cbor.Decode(fields[1], &epoch); err != nil {
		return fmt.Errorf("decode epoch: %w", err)
	}
	for idx := 2; idx < 4; idx++ {
		if err := validateSscByteString(fields[idx]); err != nil {
			return fmt.Errorf("certificate field %d: %w", idx, err)
		}
	}
	return nil
}

// checkSscProofCore implements checkSscProofLocal: it validates the proof
// type against the payload's, the element counts, the wire shape of every
// field it hashes, and the hash values themselves. checkSscProofShape does
// not share it -- that function is bounded by cardano-ledger's dropSscProof
// and cannot use any of the shapes below.
//
// This is a separate function from checkSscProofShape because,
// unlike tx_proof/dlg_proof/upd_proof, ssc_proof has no upstream reference
// implementation to cross-check this package's own hash construction
// against: modern cardano-ledger decodes SscProof as a unit type and
// re-encodes a hardcoded placeholder regardless of the block's actual SSC
// content, so cardano-node itself would accept a block whose ssc_proof
// this comparison might reject. The construction below is confirmed
// against a handful of real, non-empty mainnet blocks (see
// checkSscProofLocal's doc comment, and sscstate_real_test.go) --
// roughly 4-5 blocks, covering only two of the four SSC payload types
// (CommitmentsPayload and OpeningsPayload) but exercising three of the
// four distinct hash constructions those payloads use (the
// commitments-field, openings-field, and certificates-field hashes; the
// CommitmentsPayload vector happens to also carry a non-empty certificate
// set). SharesPayload's hash construction has no confirmed non-empty
// vector -- see checkSscProofLocal's doc comment -- out of Byron's
// ~5,000,000 total blocks and 208 epochs -- and two real encoding bugs
// were found and fixed in this construction during the same review round
// that produced those vectors, which is evidence the construction is
// subtle rather than settled. Making the hash comparison decode-gating by
// default (as an earlier version of this function did) means any
// undiscovered edge case in this construction turns into every real,
// unrelated caller of NewByronMainBlockFromCbor failing to decode an
// otherwise-genuine mainnet block. The structural checks this function
// always performs (proof type and element counts matching the payload,
// and the wire shape -- tag-258 set vs. genuine CBOR map -- of every field
// the proof would hash) carry no such risk, since they only ever reject
// input that the real Byron wire format could not have produced in the
// first place; only the value comparison is gated.
func checkSscProofCore(
	rawProof any,
	expectedType uint64,
	rest []cbor.RawMessage,
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
		if len(rest) != 2 {
			return fmt.Errorf(
				"%w: ssc payload type %d requires 2 elements after the "+
					"type, got %d",
				ErrBodyProofMismatch, sscType, len(rest),
			)
		}
		if _, err := decodePrimaryEntries(sscType, rest[0]); err != nil {
			return fmt.Errorf(
				"%w: ssc payload primary entries have the wrong wire "+
					"shape for type %d (expected a tag-258 set for "+
					"commitments, or a CBOR map for openings/shares): %w",
				ErrBodyProofMismatch, sscType, err,
			)
		}
		if err := checkHash(
			"ssc primary hash",
			proofSlice[1],
			common.Blake2b256Hash(rest[0]),
		); err != nil {
			return err
		}
		certsHash, err := localCertificatesHash(rest[1])
		if err != nil {
			return fmt.Errorf(
				"%w: ssc payload vss certificates set: %w",
				ErrBodyProofMismatch, err,
			)
		}
		return checkHash(
			"ssc vss certificates hash", proofSlice[2], certsHash,
		)
	case SscTypeCertificates:
		if len(proofSlice) != 2 {
			return fmt.Errorf(
				"%w: ssc proof type %d requires exactly 2 elements, got %d",
				ErrBodyProofMismatch, sscType, len(proofSlice),
			)
		}
		if len(rest) != 1 {
			return fmt.Errorf(
				"%w: ssc payload type %d requires 1 element after the "+
					"type, got %d",
				ErrBodyProofMismatch, sscType, len(rest),
			)
		}
		certsHash, err := localCertificatesHash(rest[0])
		if err != nil {
			return fmt.Errorf(
				"%w: ssc payload vss certificates set: %w",
				ErrBodyProofMismatch, err,
			)
		}
		return checkHash(
			"ssc vss certificates hash", proofSlice[1], certsHash,
		)
	default:
		return fmt.Errorf(
			"%w: unknown ssc proof type %d", ErrBodyProofMismatch, sscType,
		)
	}
}

// localCertificatesHash decodes raw as a tag-258 VSS certificate set and
// returns cardano-sl's VssCertificatesHash of it: canonicalMapHash of a
// fresh map from each entry's own stakeholder ID to that entry's preserved
// CBOR bytes, built from raw alone -- see checkSscProofLocal's doc comment
// for why certificates hash via this rebuilt-map representation while
// commitments/openings/shares hash via their preserved wire bytes
// directly.
func localCertificatesHash(raw cbor.RawMessage) (common.Blake2b256, error) {
	certs, err := decodeIdentitySet(raw, certificatePubkeyFieldIndex)
	if err != nil {
		return common.Blake2b256{}, err
	}
	return canonicalMapHash(certs), nil
}

// CommitmentsHash returns this registry's canonical hash of every
// stakeholder's currently accumulated commitment.
//
// This is never consulted by ValidateBodyProof (see ByronEpochSscState's
// doc comment) -- it remains available only for callers with some other
// use for a multi-block commitments view.
func (s *ByronEpochSscState) CommitmentsHash() common.Blake2b256 {
	return canonicalMapHash(s.Commitments)
}

// OpeningsHash returns this registry's canonical hash of every
// stakeholder's currently accumulated opening.
//
// This is never consulted by ValidateBodyProof (see ByronEpochSscState's
// doc comment) -- it remains available only for callers with some other
// use for a multi-block openings view.
func (s *ByronEpochSscState) OpeningsHash() common.Blake2b256 {
	return canonicalMapHash(s.Openings)
}

// SharesHash returns this registry's canonical hash of every stakeholder's
// currently accumulated shares.
//
// This is never consulted by ValidateBodyProof (see ByronEpochSscState's
// doc comment) -- it remains available only for callers with some other
// use for a multi-block shares view.
func (s *ByronEpochSscState) SharesHash() common.Blake2b256 {
	return canonicalMapHash(s.Shares)
}

// CertificatesHash returns this registry's canonical hash of every
// stakeholder's currently accumulated VSS certificate.
//
// This is never consulted by ValidateBodyProof (see ByronEpochSscState's
// doc comment) -- it remains available only for callers with some other
// use for a multi-block certificates view.
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
	itemCount, _, indefinite := cbor.ArrayInfo(raw)
	if itemCount < 0 || indefinite {
		return 0, nil, errors.New(
			"ssc payload must be a definite-length CBOR list",
		)
	}
	var parts []cbor.RawMessage
	if _, err := cbor.Decode(raw, &parts); err != nil {
		return 0, nil, fmt.Errorf("decoding ssc payload array: %w", err)
	}
	if len(parts) < 1 {
		return 0, nil, errors.New("ssc payload array is empty")
	}
	if len(parts) != itemCount {
		return 0, nil, fmt.Errorf(
			"ssc payload has %d decoded fields, expected %d",
			len(parts), itemCount,
		)
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
//
// NOTE: this only validates the map's container shape (a genuine CBOR map
// keyed by a 28-byte ID) and each value's preserved bytes as an opaque
// blob -- it does not decode or validate a value's own inner structure.
// In particular, for SharesPayload, sscshares' values are themselves a
// nested map (sharesmap = {* addressid => vssdec}, per cardano-ledger's
// dropSharesMap/dropInnerSharesMap decoder -- see the CDDL comment on
// ByronEpochSscState above), and this function never looks inside a value
// to confirm it actually is one; an opening/share value of the wrong inner
// shape (e.g. a text string where dropOpeningsMap/dropInnerSharesMap
// requires bytes or a nested map) is accepted here as long as the outer
// map's keys are well-formed. This is a deliberate, not yet closed, gap
// shared by both the proof-check path (checkSscProofLocal) and the
// accumulator path (AccumulateBlock); exploitability is near-zero since
// forging a real Byron block still requires a valid slot-leader signature
// over an immutable historical chain.
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
// shape carries a pre-hashed stakeholder ID, the map key is derived with
// stakeholderIDFromPubkeyCbor, applied directly to the field's preserved
// CBOR bytes rather than a decoded-and-re-encoded copy (see that function's
// doc comment for why). The full entry's original CBOR bytes are preserved
// as the map value.
//
// The tag-258 wrapper is required, not optional: cbor.SetType's own
// UnmarshalCBOR deliberately tolerates a plain, untagged array too (so that
// pre-Dijkstra callers of that generic type are unaffected -- see its doc
// comment in cbor/tags.go), but ssccomms/ssccerts are always tag-258 sets on
// the real Byron wire (#6.258([* ssccomm]) / #6.258([* ssccert])), never a
// bare array. Without this check, a malformed SSC body carrying an untagged
// array in place of the required set would still decode here and hash
// identically to a well-formed one, letting checkSscProofLocal validate a
// block whose body does not actually conform to the Byron wire format.
//
// NOTE: this validates each entry's outer shape only as far as "decodes
// into a CBOR array with a byte-string public key at pubkeyFieldIndex" --
// it does not enforce the entry's full field count or the type of any
// other field (e.g. dropSignedCommitment's exact-3-field ssccomm shape, or
// ssccert's epochid/signature fields). A 1-field or 5-field entry, or one
// carrying junk strings in its non-pubkey fields, decodes and hashes here
// exactly as a well-formed entry would, even though cardano-ledger's own
// decoder (dropSignedCommitment/dropCertificate) would reject it outright.
// This is a deliberate, not yet closed, gap shared by both the proof-check
// path (checkSscProofLocal) and the accumulator path (AccumulateBlock);
// exploitability is near-zero since forging a real Byron block still
// requires a valid slot-leader signature over an immutable historical
// chain.
func decodeIdentitySet(
	raw cbor.RawMessage,
	pubkeyFieldIndex int,
) (map[common.Blake2b224][]byte, error) {
	var tag cbor.RawTag
	if _, err := cbor.Decode(raw, &tag); err != nil {
		return nil, fmt.Errorf(
			"decoding identity set: expected a tag-258 wrapped set: %w", err,
		)
	}
	if tag.Number != cbor.CborTagSet {
		return nil, fmt.Errorf(
			"decoding identity set: expected tag %d, got tag %d",
			cbor.CborTagSet, tag.Number,
		)
	}
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
		pubkeyField := fields[pubkeyFieldIndex]
		if len(pubkeyField) == 0 {
			return nil, fmt.Errorf(
				"set entry %d has empty public key field", i,
			)
		}
		// Enforce the field's actual CBOR major type is byte string
		// (major type 2), not just "decodes into something
		// []byte-shaped": decoding straight into a Go []byte is too
		// permissive here, since fxamacker/cbor also routes a CBOR
		// array of small (0-255) unsigned integers through
		// parseArrayToSlice into a []byte, which would silently
		// accept a wrong-shaped field (e.g. major-type-4 array
		// [1, 2, 3]) as if it were a genuine byte string.
		if pubkeyField[0]&cbor.CborTypeMask != cbor.CborTypeByteString {
			return nil, fmt.Errorf(
				"set entry %d public key field is not a CBOR byte string (major type 0x%x)",
				i,
				pubkeyField[0]&cbor.CborTypeMask,
			)
		}
		var pubkeyBytes []byte
		if _, err := cbor.Decode(pubkeyField, &pubkeyBytes); err != nil {
			return nil, fmt.Errorf(
				"decoding set entry %d public key: %w", i, err,
			)
		}
		if len(pubkeyBytes) == 0 {
			return nil, fmt.Errorf(
				"set entry %d has empty public key", i,
			)
		}
		key := stakeholderIDFromPubkeyCbor(pubkeyField)
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
// This is cardano-sl's VssCertificatesHash construction specifically (see
// checkSscProofLocal's doc comment for the cardano-sl documentation quote
// explaining why certificates hash this way, unlike
// commitments/openings/shares): the CBOR encoding of a genuine map (HashMap
// StakeholderId VssCertificate), not the tag-258 set the certificates are
// actually transmitted as, and not a bespoke concatenation of raw bytes.
// For the bundled mainnet fixture's empty VssCertificatesMap, this produces
// blake2b256(0xa0) -- the CBOR encoding of an empty map -- matching that
// block's real header ssc_proof hash exactly; for real, non-empty,
// multi-stakeholder VSS certificate sets (mainnet slot 129601, epoch 6, 7
// distinct stakeholders), it matches the real header hash too -- see
// TestByronEpochSscStateRealMainnetCertificates in sscstate_real_test.go.
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
