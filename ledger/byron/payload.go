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
	"crypto/ed25519"
	"errors"
	"fmt"
	"unicode"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// ErrInvalidPayload reports a Byron main block whose delegation or update
// payload is malformed: the body proof binds the payload bytes to the
// header, but says nothing about whether those bytes decode to anything
// meaningful, so a block can pass ValidateBodyProof while carrying a
// delegation certificate with a truncated key or a vote that is not even
// an array.
var ErrInvalidPayload = errors.New("byron block payload is invalid")

// Element counts and indices for the delegation certificate wire format,
// which is [epoch, issuerVK, delegateVK, certSig].
const (
	delegationCertEpochIndex      = 0
	delegationCertIssuerIndex     = 1
	delegationCertDelegateIndex   = 2
	delegationCertSignatureIndex  = 3
	delegationCertElementCount    = 4
	updateVoteVoterIndex          = 0
	updateVoteProposalIdIndex     = 1
	updateVoteDecisionIndex       = 2
	updateVoteSignatureIndex      = 3
	updateVoteElementCount        = 4
	updateProposalElementCount    = 7
	updateProposalSignedBodyCount = 5
	updatePayloadVotesIndex       = 1
	updatePayloadElementCount     = 2
	updateProposalMetadataIndex   = 3
	updateProposalAttributesIndex = 4
	installerHashElementCount     = 4
	installerHashHashIndex        = 1
	// systemTagMaxLength is cardano-ledger-byron's systemTagMaxLength.
	systemTagMaxLength = 10
)

// DelegationCertificate is a decoded Byron heavyweight delegation
// certificate from a main block's delegation payload. The verification keys
// are the full 64-byte extended form the wire format carries; see
// VerificationKeySize.
type DelegationCertificate struct {
	Epoch uint64
	// EpochCbor is the epoch field's original wire encoding, which is what
	// the certificate's signature covers. It is kept because CBOR admits
	// non-shortest integer encodings that this decoder accepts: an epoch
	// that arrived as 0x1807 decodes to 7 and re-encodes to 0x07, and
	// verifying against the re-encoding would reject a certificate its
	// issuer signed correctly.
	EpochCbor  []byte
	IssuerVK   []byte
	DelegateVK []byte
	Signature  []byte
}

// UpdateVote is a decoded Byron update-proposal vote from a main block's
// update payload.
type UpdateVote struct {
	VoterVK []byte
	// ProposalIdCbor is the proposal id field's original wire encoding,
	// which is what the vote's signature covers. See
	// DelegationCertificate.EpochCbor: a 32-byte id that arrived as
	// 0x590020... re-encodes to 0x5820..., and the voter signed the former.
	ProposalId     []byte
	ProposalIdCbor []byte
	// Decision is always true. The reference checks that the wire field is
	// a CBOR Bool, discards its value, and verifies the signature as a
	// positive vote, so a wire false still records a positive vote.
	Decision  bool
	Signature []byte
}

// ParseDelegationCertificate decodes and structurally validates one entry
// of a Byron main block's delegation payload, from that entry's original
// CBOR.
//
// It takes raw CBOR rather than a decoded value because the certificate's
// signature covers the epoch field's wire encoding, which a decoded uint64
// cannot reproduce -- see DelegationCertificate.EpochCbor.
//
// The returned certificate's byte slices are copies: callers routinely hold
// these past the lifetime of the block they came from, and a certificate
// that aliased the decoded block would let a later mutation of one change
// the other.
func ParseDelegationCertificate(
	raw cbor.RawMessage,
) (*DelegationCertificate, error) {
	fields, err := payloadFields(
		"delegation certificate", raw, delegationCertElementCount,
	)
	if err != nil {
		return nil, err
	}
	epochCbor := fields[delegationCertEpochIndex]
	var epoch uint64
	if _, err := cbor.Decode(epochCbor, &epoch); err != nil {
		return nil, fmt.Errorf(
			"%w: delegation certificate epoch: %w", ErrInvalidPayload, err,
		)
	}
	issuerVK, err := payloadVerificationKey(
		"delegation certificate issuer verification key",
		fields[delegationCertIssuerIndex],
	)
	if err != nil {
		return nil, err
	}
	delegateVK, err := payloadVerificationKey(
		"delegation certificate delegate verification key",
		fields[delegationCertDelegateIndex],
	)
	if err != nil {
		return nil, err
	}
	signature, err := payloadBytes(
		"delegation certificate signature",
		fields[delegationCertSignatureIndex],
		ed25519.SignatureSize,
	)
	if err != nil {
		return nil, err
	}
	return &DelegationCertificate{
		Epoch:      epoch,
		EpochCbor:  append([]byte(nil), epochCbor...),
		IssuerVK:   issuerVK,
		DelegateVK: delegateVK,
		Signature:  signature,
	}, nil
}

// Verify checks the certificate's own signature against the protocol magic
// of the network it claims to belong to.
func (c *DelegationCertificate) Verify(protocolMagic uint32) error {
	if c == nil {
		return fmt.Errorf(
			"%w: delegation certificate is nil", ErrInvalidPayload,
		)
	}
	return VerifyDelegationCertificateSignature(
		protocolMagic, c.IssuerVK, c.DelegateVK, c.Signature, c.EpochCbor,
	)
}

const (
	cborFalse byte = 0xf4
	cborTrue  byte = 0xf5
)

// ParseUpdateVote decodes and structurally validates one entry of a Byron
// main block's update-payload vote list, from that entry's original CBOR.
//
// The wire format is [voterVK, proposalId, decision, signature]. As in
// cardano-ledger-byron's DecCBOR (AVote ByteSpan), the decision must be a
// CBOR Bool, but its value is discarded: Byron removed negative voting, and
// Verify checks the signature against a hardcoded True whichever value the
// wire carried.
//
// Like ParseDelegationCertificate this takes raw CBOR, because the vote's
// signature covers the proposal id field's wire encoding -- see
// UpdateVote.ProposalIdCbor.
func ParseUpdateVote(raw cbor.RawMessage) (*UpdateVote, error) {
	fields, err := payloadFields("update vote", raw, updateVoteElementCount)
	if err != nil {
		return nil, err
	}
	voterVK, err := payloadVerificationKey(
		"update vote voter verification key",
		fields[updateVoteVoterIndex],
	)
	if err != nil {
		return nil, err
	}
	proposalIdCbor := fields[updateVoteProposalIdIndex]
	proposalId, err := payloadBytes(
		"update vote proposal id", proposalIdCbor, common.Blake2b256Size,
	)
	if err != nil {
		return nil, err
	}
	// cborg's decodeBool accepts exactly 0xf4 and 0xf5. Decoding into a Go
	// bool is not equivalent: CBOR null and undefined decode into it without
	// error.
	if decision := fields[updateVoteDecisionIndex]; len(decision) != 1 ||
		(decision[0] != cborFalse && decision[0] != cborTrue) {
		return nil, fmt.Errorf(
			"%w: update vote decision is not a CBOR boolean",
			ErrInvalidPayload,
		)
	}
	signature, err := payloadBytes(
		"update vote signature",
		fields[updateVoteSignatureIndex],
		ed25519.SignatureSize,
	)
	if err != nil {
		return nil, err
	}
	return &UpdateVote{
		VoterVK:        voterVK,
		ProposalId:     proposalId,
		ProposalIdCbor: append([]byte(nil), proposalIdCbor...),
		Decision:       true,
		Signature:      signature,
	}, nil
}

// Verify checks the vote's signature, reproducing
// cardano-ledger-byron's Cardano.Chain.Update.Vote signing format:
//
//	inner  = 0x82 || proposalIdCbor || 0xf5
//	signed = 0x06 || CBOR(protocolMagic) || inner
//
// 0x82 is a two-element definite-length array header and 0xf5 is CBOR true,
// together the encoding of the (UpId, Bool) pair signatureForVote signs.
// This mirrors recoverSignedBytes, which reassembles the same two bytes
// around the proposal id's preserved encoding rather than re-encoding the
// pair -- and preserved is what it has to be, since a non-shortest id
// encoding would not survive a round trip.
func (v *UpdateVote) Verify(protocolMagic uint32) error {
	if v == nil {
		return fmt.Errorf("%w: update vote is nil", ErrInvalidPayload)
	}
	if len(v.ProposalIdCbor) == 0 {
		return fmt.Errorf(
			"%w: update vote has no preserved proposal id encoding",
			ErrInvalidPayload,
		)
	}
	const cborArrayLen2 byte = 0x82
	inner := make([]byte, 0, 2+len(v.ProposalIdCbor))
	inner = append(inner, cborArrayLen2)
	inner = append(inner, v.ProposalIdCbor...)
	inner = append(inner, cborTrue)
	signed, err := signedBytes(SignTagUSVote, protocolMagic, inner)
	if err != nil {
		return err
	}
	if !verifyEd25519(v.VoterVK, signed, v.Signature) {
		return fmt.Errorf(
			"%w: update vote for proposal %x", ErrInvalidSignature,
			v.ProposalId,
		)
	}
	return nil
}

// proposalFields returns an update proposal's seven fields as their
// original wire encodings.
func (p *ByronUpdateProposal) proposalFields() ([]cbor.RawMessage, error) {
	proposalCbor := p.Cbor()
	if len(proposalCbor) == 0 {
		return nil, fmt.Errorf(
			"%w: update proposal has no preserved CBOR", ErrInvalidPayload,
		)
	}
	return payloadFields(
		"update proposal", proposalCbor, updateProposalElementCount,
	)
}

// signedBody returns the exact bytes an update proposal's signature covers:
// a five-element array header followed by the proposal's first five fields
// as they appeared on the wire.
//
// The bytes are recovered from the proposal's preserved CBOR rather than
// re-encoded. cardano-ledger-byron does the same thing for the same reason
// (recoverProposalSignedBytes prepends "\133" -- 0x85 -- to the decoded
// body's byte span): the signature covers a seven-element proposal's first
// five fields re-framed as a five-element array, and two of those fields
// decode into `any`, so re-encoding them is not guaranteed to reproduce the
// issuer's bytes.
func signedBody(fields []cbor.RawMessage) []byte {
	const cborArrayLen5 byte = 0x85
	size := 1
	for _, field := range fields[:updateProposalSignedBodyCount] {
		size += len(field)
	}
	body := make([]byte, 0, size)
	body = append(body, cborArrayLen5)
	for _, field := range fields[:updateProposalSignedBodyCount] {
		body = append(body, field...)
	}
	return body
}

// Validate structurally validates an update proposal and verifies its
// issuer signature, reproducing cardano-ledger-byron's
// Cardano.Chain.Update.Proposal signing format:
//
//	signed = 0x04 || CBOR(protocolMagic) || signedBody()
//
// See signedBody for how the signed body is recovered.
//
// Authenticating the signed bytes is not on its own enough to accept a
// proposal: the signature covers whatever the issuer put in the metadata
// and attributes fields, so a correctly signed proposal can still carry
// values the reference decoder rejects outright -- a bare integer in place
// of the metadata map, say. validateProposalMetadata and
// validateProposalAttributes reproduce what
// cardano-ledger-byron's ProposalBody decoder enforces for those two
// fields, so this cannot accept a proposal cardano-ledger would refuse to
// decode.
func (p *ByronUpdateProposal) Validate(protocolMagic uint32) error {
	if p == nil {
		return fmt.Errorf("%w: update proposal is nil", ErrInvalidPayload)
	}
	if len(p.From) != VerificationKeySize {
		return fmt.Errorf(
			"%w: update proposal issuer key is %d bytes, expected %d",
			ErrInvalidPayload, len(p.From), VerificationKeySize,
		)
	}
	if len(p.Signature) != ed25519.SignatureSize {
		return fmt.Errorf(
			"%w: update proposal signature is %d bytes, expected %d",
			ErrInvalidPayload, len(p.Signature), ed25519.SignatureSize,
		)
	}
	fields, err := p.proposalFields()
	if err != nil {
		return err
	}
	if err := validateProposalMetadata(
		fields[updateProposalMetadataIndex],
	); err != nil {
		return err
	}
	if err := validateProposalAttributes(
		fields[updateProposalAttributesIndex],
	); err != nil {
		return err
	}
	signed, err := signedBytes(
		SignTagUSProposal, protocolMagic, signedBody(fields),
	)
	if err != nil {
		return err
	}
	if !verifyEd25519(p.From, signed, p.Signature) {
		return fmt.Errorf(
			"%w: update proposal from %x", ErrInvalidSignature, p.From[:32],
		)
	}
	return nil
}

// validateProposalMetadata enforces the shape of an update proposal's
// metadata field, which cardano-ledger-byron types as
// Map SystemTag InstallerHash (Cardano.Chain.Update.Proposal's ProposalBody,
// via Cardano.Chain.Update.SystemTag and .InstallerHash):
//
//   - the field is a CBOR map;
//   - each key is a text string of at most systemTagMaxLength characters,
//     all ASCII -- SystemTag's checkSystemTag;
//   - each value is a four-element array whose element 1 is a 32-byte hash
//     -- InstallerHash's enforceSize "InstallerHash" 4, which drops
//     elements 0, 2, and 3 and reads the hash out of element 1.
func validateProposalMetadata(raw cbor.RawMessage) error {
	length, headerSize, indefinite := cbor.MapInfo(raw)
	if indefinite || length < 0 {
		return fmt.Errorf("%w: update proposal metadata must be a definite map", ErrInvalidPayload)
	}
	pos := int(headerSize)
	var previous string
	for i := 0; i < length; i++ {
		keyStart := pos
		var err error
		pos, err = scanByronItem(raw, pos, 0)
		if err != nil {
			return fmt.Errorf("%w: update proposal metadata key %d: %w", ErrInvalidPayload, i, err)
		}
		keyRaw := raw[keyStart:pos]
		if err := requireByronTextString(keyRaw, "update proposal system tag"); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidPayload, err)
		}
		var tag string
		if consumed, err := cbor.Decode(keyRaw, &tag); err != nil || consumed != len(keyRaw) {
			return fmt.Errorf("%w: invalid update proposal system tag", ErrInvalidPayload)
		}
		if i > 0 && tag <= previous {
			return fmt.Errorf("%w: update proposal system tags must be strictly increasing", ErrInvalidPayload)
		}
		previous = tag
		valueStart := pos
		pos, err = scanByronItem(raw, pos, 0)
		if err != nil {
			return fmt.Errorf("%w: update proposal installer hash for %q: %w", ErrInvalidPayload, tag, err)
		}
		if err := validateSystemTag(tag); err != nil {
			return err
		}
		if err := validateInstallerHash(tag, raw[valueStart:pos]); err != nil {
			return err
		}
	}
	if pos != len(raw) {
		return fmt.Errorf("%w: update proposal metadata has trailing CBOR data", ErrInvalidPayload)
	}
	return nil
}

// validateSystemTag reproduces cardano-ledger-byron's checkSystemTag.
func validateSystemTag(tag string) error {
	for i := range len(tag) {
		if tag[i] > unicode.MaxASCII {
			return fmt.Errorf(
				"%w: update proposal system tag %q is not ASCII",
				ErrInvalidPayload, tag,
			)
		}
	}
	// Every byte is ASCII by this point, so byte length is character
	// length, which is what checkSystemTag bounds.
	if len(tag) > systemTagMaxLength {
		return fmt.Errorf(
			"%w: update proposal system tag %q is %d characters, at most %d allowed",
			ErrInvalidPayload, tag, len(tag), systemTagMaxLength,
		)
	}
	return nil
}

// validateInstallerHash reproduces cardano-ledger-byron's InstallerHash
// decoder, which enforces a four-element array, drops elements 0, 2, and 3,
// and reads the Hash Raw out of element 1. The four elements are the
// remains of cardano-sl's UpdateData record, of which only one hash
// survived into cardano-ledger-byron.
//
// The reference discards fields 0, 2, and 3 after decoding them as bytes.
func validateInstallerHash(tag string, raw cbor.RawMessage) error {
	label := fmt.Sprintf(
		"update proposal installer hash for system tag %q", tag,
	)
	fields, err := payloadFields(label, raw, installerHashElementCount)
	if err != nil {
		return err
	}
	for _, index := range []int{0, 2, 3} {
		if err := requireByronByteString(fields[index], label); err != nil {
			return fmt.Errorf("%w: %s dropped field %d must be bytes: %w", ErrInvalidPayload, label, index, err)
		}
	}
	_, err = payloadBytes(
		label, fields[installerHashHashIndex], common.Blake2b256Size,
	)
	return err
}

// validateProposalAttributes enforces that an update proposal's attributes
// field is an empty map, which is what cardano-ledger-byron's
// dropEmptyAttributes requires: it reads a definite map length and errors
// with "Found unexpected attributes!" on anything other than zero.
//
// The check reads the map header directly rather than decoding into a Go
// map, because the field's key type is not fixed -- the reference drops it
// without ever decoding the keys -- and a decode would have to guess one.
//
// The reference reads an ordinary definite map length, so non-shortest
// encodings of the empty map remain valid.
func validateProposalAttributes(raw cbor.RawMessage) error {
	length, err := cborMapLen(raw)
	if err != nil {
		return fmt.Errorf(
			"%w: update proposal attributes: %w", ErrInvalidPayload, err,
		)
	}
	if length != 0 {
		return fmt.Errorf(
			"%w: update proposal carries %d attributes, expected none",
			ErrInvalidPayload, length,
		)
	}
	if _, headerSize, indefinite := cbor.MapInfo(raw); indefinite || int(headerSize) != len(raw) {
		return fmt.Errorf("%w: update proposal attributes have trailing CBOR data", ErrInvalidPayload)
	}
	return nil
}

// cborMapLen reads the entry count out of a definite-length CBOR map
// header, rejecting indefinite-length maps while accepting non-shortest
// definite encodings.
func cborMapLen(raw []byte) (uint64, error) {
	const (
		majorTypeMap       = 5
		majorTypeShift     = 5
		inlineArgumentMax  = 23
		argumentOneByte    = 24
		argumentEightByte  = 27
		argumentIndefinite = 31
		argumentMask       = 0x1f
	)
	if len(raw) == 0 {
		return 0, errors.New("empty encoding")
	}
	if raw[0]>>majorTypeShift != majorTypeMap {
		return 0, fmt.Errorf("not a CBOR map, initial byte is 0x%02x", raw[0])
	}
	argument := raw[0] & argumentMask
	if argument <= inlineArgumentMax {
		return uint64(argument), nil
	}
	if argument == argumentIndefinite {
		return 0, errors.New("indefinite-length map is not permitted")
	}
	if argument < argumentOneByte || argument > argumentEightByte {
		return 0, fmt.Errorf("reserved map header argument %d", argument)
	}
	width := 1 << (argument - argumentOneByte)
	if len(raw) < 1+width {
		return 0, fmt.Errorf(
			"truncated map header: need %d bytes, have %d", 1+width, len(raw),
		)
	}
	var length uint64
	for _, b := range raw[1 : 1+width] {
		length = length<<8 | uint64(b)
	}
	return length, nil
}

// ValidateDelegationPayload structurally validates every heavyweight
// delegation certificate in a Byron main block's delegation payload and
// verifies each certificate's signature.
//
// The block's own header protocol magic is used for domain separation, so a
// certificate signed for another network fails here.
//
// Validation walks the payload's preserved CBOR rather than its decoded
// []any, because each certificate's signature covers its epoch field's wire
// encoding. A block whose delegation payload was not decoded from CBOR --
// one assembled in Go -- therefore cannot be checked, and is rejected
// rather than verified against re-encoded bytes.
func (b *ByronMainBlock) ValidateDelegationPayload() error {
	if b == nil || b.BlockHeader == nil {
		return fmt.Errorf(
			"%w: block or block header is nil", ErrInvalidPayload,
		)
	}
	certificates, err := payloadEntries(
		"delegation payload",
		b.Body.DlgPayloadCbor(),
		len(b.Body.DlgPayload),
	)
	if err != nil {
		return err
	}
	protocolMagic := b.BlockHeader.ProtocolMagic
	for i, rawCertificate := range certificates {
		certificate, err := ParseDelegationCertificate(rawCertificate)
		if err != nil {
			return fmt.Errorf("delegation certificate %d: %w", i, err)
		}
		if err := certificate.Verify(protocolMagic); err != nil {
			return fmt.Errorf("delegation certificate %d: %w", i, err)
		}
	}
	return nil
}

// ValidateUpdatePayload structurally validates every update proposal and
// vote in a Byron main block's update payload and verifies their
// signatures, using the block's own header protocol magic for domain
// separation.
//
// Like ValidateDelegationPayload, the votes are read out of the payload's
// preserved CBOR: a vote's signature covers its proposal id field's wire
// encoding. Proposals carry their own preserved CBOR already.
func (b *ByronMainBlock) ValidateUpdatePayload() error {
	if b == nil || b.BlockHeader == nil {
		return fmt.Errorf(
			"%w: block or block header is nil", ErrInvalidPayload,
		)
	}
	protocolMagic := b.BlockHeader.ProtocolMagic
	for i := range b.Body.UpdPayload.Proposals {
		if err := b.Body.UpdPayload.Proposals[i].Validate(
			protocolMagic,
		); err != nil {
			return fmt.Errorf("update proposal %d: %w", i, err)
		}
	}
	votes, err := b.updateVotesCbor()
	if err != nil {
		return err
	}
	for i, rawVote := range votes {
		vote, err := ParseUpdateVote(rawVote)
		if err != nil {
			return fmt.Errorf("update vote %d: %w", i, err)
		}
		if err := vote.Verify(protocolMagic); err != nil {
			return fmt.Errorf("update vote %d: %w", i, err)
		}
	}
	return nil
}

// updateVotesCbor returns the preserved CBOR of each vote in the block's
// update payload, which is the second element of the payload's
// [proposals, votes] array.
func (b *ByronMainBlock) updateVotesCbor() ([]cbor.RawMessage, error) {
	voteCount := len(b.Body.UpdPayload.Votes)
	updCbor := b.Body.UpdPayloadCbor()
	if len(updCbor) == 0 {
		if voteCount == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf(
			"%w: update payload carries %d votes but has no preserved CBOR",
			ErrInvalidPayload, voteCount,
		)
	}
	var parts []cbor.RawMessage
	if _, err := cbor.Decode(updCbor, &parts); err != nil {
		return nil, fmt.Errorf(
			"%w: decode update payload: %w", ErrInvalidPayload, err,
		)
	}
	if len(parts) != updatePayloadElementCount {
		return nil, fmt.Errorf(
			"%w: update payload is not a %d-element array, got %d elements",
			ErrInvalidPayload, updatePayloadElementCount, len(parts),
		)
	}
	return payloadEntries(
		"update payload votes",
		parts[updatePayloadVotesIndex],
		voteCount,
	)
}

func validateUpdateVotesWire(raw []byte) error {
	parts, err := byronArrayFields(raw, "byron update payload")
	if err != nil {
		return err
	}
	if len(parts) != updatePayloadElementCount {
		return fmt.Errorf("byron update payload has %d fields, expected %d", len(parts), updatePayloadElementCount)
	}
	if len(parts[updatePayloadVotesIndex]) == 0 || parts[updatePayloadVotesIndex][0] != 0x9f {
		return errors.New("Byron update votes must use indefinite-list framing")
	}
	var votes []cbor.RawMessage
	if _, err := cbor.Decode(parts[updatePayloadVotesIndex], &votes); err != nil {
		return fmt.Errorf("decode Byron update payload votes: %w", err)
	}
	for i, rawVote := range votes {
		if _, err := ParseUpdateVote(rawVote); err != nil {
			return fmt.Errorf("Byron update vote %d: %w", i, err)
		}
	}
	return nil
}

func validateDelegationPayloadWire(raw []byte) error {
	if len(raw) == 0 || raw[0] != 0x9f {
		return errors.New("Byron delegation certificates must use indefinite-list framing")
	}
	var certificates []cbor.RawMessage
	if _, err := cbor.Decode(raw, &certificates); err != nil {
		return fmt.Errorf("decode Byron delegation certificates: %w", err)
	}
	for i, rawCertificate := range certificates {
		if _, err := ParseDelegationCertificate(rawCertificate); err != nil {
			return fmt.Errorf("Byron delegation certificate %d: %w", i, err)
		}
	}
	return nil
}

// ValidatePayloads runs ValidateDelegationPayload and
// ValidateUpdatePayload. This is what ValidateBodyProof calls when a caller
// opts in via common.VerifyConfig.EnableByronPayloadValidation.
func (b *ByronMainBlock) ValidatePayloads() error {
	if err := b.ValidateDelegationPayload(); err != nil {
		return err
	}
	return b.ValidateUpdatePayload()
}

// payloadEntries decodes a payload list into its per-entry preserved CBOR,
// cross-checking the count against what the typed decode produced so the
// two views of the same bytes cannot silently diverge.
func payloadEntries(
	label string,
	raw []byte,
	decodedCount int,
) ([]cbor.RawMessage, error) {
	if len(raw) == 0 {
		if decodedCount == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf(
			"%w: %s carries %d entries but has no preserved CBOR",
			ErrInvalidPayload, label, decodedCount,
		)
	}
	var entries []cbor.RawMessage
	if _, err := cbor.Decode(raw, &entries); err != nil {
		return nil, fmt.Errorf(
			"%w: decode %s: %w", ErrInvalidPayload, label, err,
		)
	}
	if len(entries) != decodedCount {
		return nil, fmt.Errorf(
			"%w: %s decodes to %d entries, preserved CBOR holds %d",
			ErrInvalidPayload, label, decodedCount, len(entries),
		)
	}
	return entries, nil
}

// payloadFields decodes one payload entry into its per-field preserved
// CBOR, asserting the entry is an array of exactly count elements.
func payloadFields(
	label string,
	raw cbor.RawMessage,
	count int,
) ([]cbor.RawMessage, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf(
			"%w: %s has no preserved CBOR", ErrInvalidPayload, label,
		)
	}
	fields, err := byronArrayFields(raw, label)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: %s is not a %d-element array: %w",
			ErrInvalidPayload, label, count, err,
		)
	}
	if len(fields) != count {
		return nil, fmt.Errorf(
			"%w: %s is not a %d-element array, got %d elements",
			ErrInvalidPayload, label, count, len(fields),
		)
	}
	return fields, nil
}

// payloadBytes decodes one preserved payload field as a byte string of an
// exact length. The result is a fresh slice, so it does not alias the block
// it was decoded from.
func payloadBytes(
	label string,
	raw cbor.RawMessage,
	size int,
) ([]byte, error) {
	value, err := decodeByronByteString(raw, false)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: %s is not a %d-byte string: %w",
			ErrInvalidPayload, label, size, err,
		)
	}
	if len(value) != size {
		return nil, fmt.Errorf(
			"%w: %s is not %d bytes, got %d",
			ErrInvalidPayload, label, size, len(value),
		)
	}
	return append([]byte(nil), value...), nil
}

func payloadVerificationKey(label string, raw cbor.RawMessage) ([]byte, error) {
	value, err := requireCanonicalByronByteString(raw, label)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrInvalidPayload, label, err)
	}
	if len(value) != VerificationKeySize {
		return nil, fmt.Errorf(
			"%w: %s is %d bytes, expected %d",
			ErrInvalidPayload, label, len(value), VerificationKeySize,
		)
	}
	return append([]byte(nil), value...), nil
}
