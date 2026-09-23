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
	"math/big"
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
	if err := validateProtocolParametersUpdate(fields[1]); err != nil {
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
	pairs, err := cborMapRawEntries(raw)
	if err != nil {
		return fmt.Errorf("%w: update proposal metadata: %w", ErrInvalidPayload, err)
	}
	previousTag := ""
	for index, pair := range pairs {
		if len(pair[0]) == 0 || pair[0][0]>>5 != 3 || pair[0][0]&0x1f == 31 {
			return fmt.Errorf("%w: update proposal system tag is not definite text", ErrInvalidPayload)
		}
		var tag string
		n, err := cbor.Decode(pair[0], &tag)
		if err != nil {
			return fmt.Errorf("%w: decode update proposal system tag: %w", ErrInvalidPayload, err)
		}
		if n != len(pair[0]) {
			return fmt.Errorf("%w: update proposal system tag has trailing bytes", ErrInvalidPayload)
		}
		if index > 0 && tag <= previousTag {
			return fmt.Errorf("%w: update proposal system tags are not strictly increasing", ErrInvalidPayload)
		}
		previousTag = tag
		if err := validateSystemTag(tag); err != nil {
			return err
		}
		if err := validateInstallerHash(tag, pair[1]); err != nil {
			return err
		}
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
// Elements 0, 2, and 3 are deliberately left unchecked: the reference
// discards them without interpreting them, so constraining them here could
// only reject something the reference accepts.
func validateInstallerHash(tag string, raw cbor.RawMessage) error {
	label := fmt.Sprintf(
		"update proposal installer hash for system tag %q", tag,
	)
	fields, err := payloadFields(label, raw, installerHashElementCount)
	if err != nil {
		return err
	}
	_, err = payloadBytes(
		label, fields[installerHashHashIndex], common.Blake2b256Size,
	)
	if err != nil {
		return err
	}
	for _, index := range []int{0, 2, 3} {
		if len(fields[index]) == 0 || fields[index][0]>>5 != 2 || fields[index][0]&0x1f == 31 {
			return fmt.Errorf("%w: %s field %d is not a definite byte string", ErrInvalidPayload, label, index)
		}
		var value []byte
		n, err := cbor.Decode(fields[index], &value)
		if err != nil {
			return fmt.Errorf("%w: decode %s field %d: %w", ErrInvalidPayload, label, index, err)
		}
		if n != len(fields[index]) {
			return fmt.Errorf("%w: %s field %d has trailing bytes", ErrInvalidPayload, label, index)
		}
	}
	return nil
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
// The reference reads the length with decodeMapLen, accepting non-shortest
// definite lengths while rejecting indefinite maps.
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
	return nil
}

// cborMapLen reads the entry count out of a definite-length CBOR map header,
// accepting non-shortest lengths and rejecting indefinite maps.
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

func cborMapRawEntries(raw []byte) ([][2]cbor.RawMessage, error) {
	count, err := cborMapLen(raw)
	if err != nil {
		return nil, err
	}
	argument := raw[0] & 0x1f
	headerLength := 1
	switch argument {
	case 24:
		headerLength += 1
	case 25:
		headerLength += 2
	case 26:
		headerLength += 4
	case 27:
		headerLength += 8
	}
	// The conversion is safe because len is nonnegative and bounded by MaxInt.
	if count > uint64((len(raw)-headerLength)/2) { //nolint:gosec
		return nil, errors.New("truncated map entries")
	}
	offset := headerLength
	pairs := make([][2]cbor.RawMessage, 0, (len(raw)-headerLength)/2)
	for range count {
		var pair [2]cbor.RawMessage
		for index := range pair {
			end, err := cborItemEnd(raw, offset)
			if err != nil {
				return nil, fmt.Errorf("decode map entry: %w", err)
			}
			pair[index] = append(cbor.RawMessage(nil), raw[offset:end]...)
			offset = end
		}
		pairs = append(pairs, pair)
	}
	if offset != len(raw) {
		return nil, errors.New("trailing bytes after map")
	}
	return pairs, nil
}

const maxByronLovelacePortion = uint64(1_000_000_000_000_000)

// ByronLovelacePortion is the Byron protocol's bounded stake fraction.
type ByronLovelacePortion uint64

func (p *ByronLovelacePortion) UnmarshalCBOR(raw []byte) error {
	var value uint64
	n, err := cbor.Decode(raw, &value)
	if err != nil {
		return fmt.Errorf("%w: decode LovelacePortion: %w", ErrInvalidPayload, err)
	}
	if n != len(raw) {
		return fmt.Errorf("%w: LovelacePortion has trailing bytes", ErrInvalidPayload)
	}
	if value > maxByronLovelacePortion {
		return fmt.Errorf("%w: lovelace portion exceeds maximum", ErrInvalidPayload)
	}
	*p = ByronLovelacePortion(value)
	return nil
}

// ByronSoftForkRule contains the three Byron stake portions used to adopt a
// proposed block version.
type ByronSoftForkRule struct {
	InitThreshold      ByronLovelacePortion
	MinThreshold       ByronLovelacePortion
	ThresholdDecrement ByronLovelacePortion
}

func (r *ByronSoftForkRule) UnmarshalCBOR(raw []byte) error {
	fields, err := cborRawArrayEntries(raw, true)
	if err != nil || len(fields) != 3 {
		return fmt.Errorf("%w: softfork rule must contain three portions", ErrInvalidPayload)
	}
	var decoded ByronSoftForkRule
	for index, target := range []*ByronLovelacePortion{
		&decoded.InitThreshold,
		&decoded.MinThreshold,
		&decoded.ThresholdDecrement,
	} {
		if _, err := cbor.Decode(fields[index], target); err != nil {
			return err
		}
	}
	*r = decoded
	return nil
}

// ByronTxFeePolicy is the currently defined TxSizeLinear fee policy.
type ByronTxFeePolicy struct {
	Tag            uint8
	SummandNano    *big.Int
	MultiplierNano *big.Int
}

func (p *ByronTxFeePolicy) UnmarshalCBOR(raw []byte) error {
	fields, err := cborRawArrayEntries(raw, true)
	if err != nil || len(fields) != 2 {
		return fmt.Errorf("%w: transaction fee policy must contain two fields", ErrInvalidPayload)
	}
	var tag uint8
	if _, err := cbor.Decode(fields[0], &tag); err != nil || tag != 0 {
		return fmt.Errorf("%w: transaction fee policy has an unsupported tag", ErrInvalidPayload)
	}
	encoded, err := decodeKnownCborBytes(fields[1])
	if err != nil {
		return fmt.Errorf("%w: transaction fee policy is not known-CBOR TxSizeLinear: %w", ErrInvalidPayload, err)
	}
	coefficients, err := cborRawArrayEntries(encoded, true)
	if err != nil || len(coefficients) != 2 {
		return fmt.Errorf("%w: TxSizeLinear must contain two coefficients", ErrInvalidPayload)
	}
	decoded := ByronTxFeePolicy{Tag: tag, SummandNano: new(big.Int), MultiplierNano: new(big.Int)}
	for index, target := range []*big.Int{decoded.SummandNano, decoded.MultiplierNano} {
		if n, err := cbor.Decode(coefficients[index], target); err != nil {
			return fmt.Errorf("%w: decode TxSizeLinear coefficient %d: %w", ErrInvalidPayload, index, err)
		} else if n != len(coefficients[index]) {
			return fmt.Errorf("%w: TxSizeLinear coefficient %d has trailing bytes", ErrInvalidPayload, index)
		}
	}
	roundedSummand := roundNanoToInteger(decoded.SummandNano)
	if roundedSummand.Sign() < 0 || roundedSummand.Cmp(big.NewInt(45_000_000_000_000_000)) > 0 {
		return fmt.Errorf("%w: TxSizeLinear summand is outside the Lovelace range", ErrInvalidPayload)
	}
	*p = decoded
	return nil
}

func validateLovelacePortion(raw cbor.RawMessage, label string) error {
	var value uint64
	if n, err := cbor.Decode(raw, &value); err != nil {
		return fmt.Errorf("%w: %s is not a LovelacePortion: %w", ErrInvalidPayload, label, err)
	} else if n != len(raw) {
		return fmt.Errorf("%w: %s LovelacePortion has trailing bytes", ErrInvalidPayload, label)
	}
	if value > maxByronLovelacePortion {
		return fmt.Errorf("%w: %s exceeds the LovelacePortion maximum", ErrInvalidPayload, label)
	}
	return nil
}

func validateOptionalLovelacePortion(raw cbor.RawMessage, label string) error {
	values, err := cborRawArrayEntries(raw, true)
	if err != nil {
		return fmt.Errorf("%w: %s is not an optional value list: %w", ErrInvalidPayload, label, err)
	}
	if len(values) > 1 {
		return fmt.Errorf("%w: %s has %d values, expected at most one", ErrInvalidPayload, label, len(values))
	}
	if len(values) == 1 {
		return validateLovelacePortion(values[0], label)
	}
	return nil
}

func validateProtocolParametersUpdate(raw cbor.RawMessage) error {
	fields, err := payloadFields("Byron protocol-parameters update", raw, 14)
	if err != nil {
		return err
	}
	for index, name := range map[int]string{
		6: "mpcThd", 7: "heavyDelThd", 8: "updateVoteThd", 9: "updateProposalThd",
	} {
		if err := validateOptionalLovelacePortion(fields[index], name); err != nil {
			return err
		}
	}
	softForkValues, err := cborRawArrayEntries(fields[11], true)
	if err != nil {
		return fmt.Errorf("%w: softForkRule is not an optional list: %w", ErrInvalidPayload, err)
	}
	if len(softForkValues) > 1 {
		return fmt.Errorf("%w: softForkRule has multiple values", ErrInvalidPayload)
	}
	if len(softForkValues) == 1 {
		parts, err := payloadFields("softForkRule", softForkValues[0], 3)
		if err != nil {
			return err
		}
		for _, label := range []string{"initThd", "minThd", "thdDecrement"} {
			if err := validateLovelacePortion(parts[0], label); err != nil {
				return err
			}
			parts = parts[1:]
		}
	}
	return validateTxFeePolicy(fields[12])
}

func validateTxFeePolicy(raw cbor.RawMessage) error {
	count, offset, err := cborArrayHeader(raw)
	if err != nil {
		return fmt.Errorf("%w: txFeePolicy is not an optional list: %w", ErrInvalidPayload, err)
	}
	if count > 1 {
		return fmt.Errorf("%w: txFeePolicy has multiple values", ErrInvalidPayload)
	}
	if count == 0 {
		if offset != len(raw) {
			return fmt.Errorf("%w: trailing bytes after txFeePolicy", ErrInvalidPayload)
		}
		return nil
	}
	policyRaw := raw[offset:]
	policyCount, policyOffset, err := cborArrayHeader(policyRaw)
	if err != nil || policyCount != 2 {
		return fmt.Errorf("%w: txFeePolicy must be a two-element array", ErrInvalidPayload)
	}
	var tag uint64
	firstSize, err := cbor.Decode(policyRaw[policyOffset:], &tag)
	if err != nil || tag != 0 {
		return fmt.Errorf("%w: txFeePolicy has an unsupported tag", ErrInvalidPayload)
	}
	knownRaw := policyRaw[policyOffset+firstSize:]
	encoded, err := decodeKnownCborBytes(knownRaw)
	if err != nil {
		return fmt.Errorf("%w: txFeePolicy is not a known-CBOR TxSizeLinear: %w", ErrInvalidPayload, err)
	}
	parts, err := payloadFields("TxSizeLinear", encoded, 2)
	if err != nil {
		return err
	}
	var summandNano, multiplierNano big.Int
	for index, target := range []*big.Int{&summandNano, &multiplierNano} {
		if n, err := cbor.Decode(parts[index], target); err != nil {
			return fmt.Errorf("%w: TxSizeLinear coefficient %d is not Nano: %w", ErrInvalidPayload, index, err)
		} else if n != len(parts[index]) {
			return fmt.Errorf("%w: TxSizeLinear coefficient %d has trailing bytes", ErrInvalidPayload, index)
		}
	}
	if summandNano.Sign() < 0 {
		return fmt.Errorf("%w: TxSizeLinear summand is negative", ErrInvalidPayload)
	}
	// TxSizeLinear decodes its first Nano coefficient, rounds it to
	// Lovelace, and then applies the ordinary Lovelace bound. Its multiplier
	// is an unbounded rational coefficient and has no Lovelace range check.
	rounded := roundNanoToInteger(&summandNano)
	if rounded.Cmp(big.NewInt(45_000_000_000_000_000)) > 0 {
		return fmt.Errorf("%w: TxSizeLinear summand exceeds maximum Lovelace", ErrInvalidPayload)
	}
	return nil
}

func cborArrayHeader(raw []byte) (uint64, int, error) {
	if len(raw) == 0 || raw[0]>>5 != 4 {
		return 0, 0, errors.New("expected CBOR array")
	}
	argument := raw[0] & 0x1f
	if argument < 24 {
		return uint64(argument), 1, nil
	}
	width := 1 << (argument - 24)
	if argument < 24 || argument > 27 || len(raw) < 1+width {
		return 0, 0, errors.New("invalid or truncated CBOR array header")
	}
	var count uint64
	for _, value := range raw[1 : 1+width] {
		count = count<<8 | uint64(value)
	}
	return count, 1 + width, nil
}

func decodeKnownCborBytes(raw cbor.RawMessage) ([]byte, error) {
	if len(raw) == 0 || raw[0]>>5 != 6 {
		return nil, errors.New("expected semantic tag 24")
	}
	argument := raw[0] & 0x1f
	headerLength := 1
	var tag uint64
	switch {
	case argument < 24:
		tag = uint64(argument)
	case argument == 24:
		if len(raw) < 2 {
			return nil, errors.New("truncated tag header")
		}
		tag, headerLength = uint64(raw[1]), 2
	case argument == 25:
		if len(raw) < 3 {
			return nil, errors.New("truncated tag header")
		}
		tag, headerLength = uint64(raw[1])<<8|uint64(raw[2]), 3
	case argument == 26:
		if len(raw) < 5 {
			return nil, errors.New("truncated tag header")
		}
		for _, value := range raw[1:5] {
			tag = tag<<8 | uint64(value)
		}
		headerLength = 5
	case argument == 27:
		if len(raw) < 9 {
			return nil, errors.New("truncated tag header")
		}
		for _, value := range raw[1:9] {
			tag = tag<<8 | uint64(value)
		}
		headerLength = 9
	default:
		return nil, errors.New("invalid tag header")
	}
	if tag != cbor.CborTagCbor {
		return nil, fmt.Errorf("expected semantic tag 24, got %d", tag)
	}
	var encoded []byte
	n, err := cbor.Decode(raw[headerLength:], &encoded)
	if err != nil {
		return nil, err
	}
	if n != len(raw)-headerLength {
		return nil, errors.New("trailing bytes after known-CBOR byte string")
	}
	return encoded, nil
}

func roundNanoToInteger(value *big.Int) *big.Int {
	denominator := big.NewInt(1_000_000_000)
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(value, denominator, remainder)
	twiceRemainder := new(big.Int).Lsh(remainder, 1)
	comparison := twiceRemainder.Cmp(denominator)
	if comparison < 0 {
		comparison = -comparison
	}
	if comparison > 0 || (comparison == 0 && quotient.Bit(0) == 1) {
		if value.Sign() < 0 {
			quotient.Sub(quotient, big.NewInt(1))
		} else {
			quotient.Add(quotient, big.NewInt(1))
		}
	}
	return quotient
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
		false,
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
		false,
	)
}

// ValidateUpdatePayloadStructure checks the Byron update payload's wire
// structure before state processing. Signature and authorization checks stay
// in ValidateUpdatePayload.
func (b *ByronMainBlockBody) ValidateUpdatePayloadStructure() error {
	if b == nil || len(b.updPayloadRaw) == 0 {
		return fmt.Errorf("%w: update payload has no preserved CBOR", ErrInvalidPayload)
	}
	return validateUpdatePayloadStructure(
		b.updPayloadRaw,
		b.UpdPayload.Proposals,
		b.UpdPayload.Votes,
	)
}

func validateUpdatePayloadStructure(
	raw []byte,
	decodedProposals []ByronUpdateProposal,
	decodedVotes []any,
) error {
	var parts []cbor.RawMessage
	if _, err := cbor.Decode(raw, &parts); err != nil || len(parts) != updatePayloadElementCount {
		return fmt.Errorf("%w: update payload must be a two-element array", ErrInvalidPayload)
	}
	if len(parts[updatePayloadVotesIndex]) == 0 || parts[updatePayloadVotesIndex][0] != 0x9f {
		return fmt.Errorf("%w: update votes must use indefinite-list framing", ErrInvalidPayload)
	}
	proposals, err := payloadEntries("update payload proposals", parts[0], len(decodedProposals), true)
	if err != nil {
		return err
	}
	if len(proposals) > 1 {
		return fmt.Errorf("%w: update payload contains %d proposals, expected at most one", ErrInvalidPayload, len(proposals))
	}
	for index, proposal := range proposals {
		fields, err := payloadFields("update proposal", proposal, updateProposalElementCount)
		if err != nil {
			return err
		}
		if err := validateProposalMetadata(fields[updateProposalMetadataIndex]); err != nil {
			return fmt.Errorf("update proposal %d metadata: %w", index, err)
		}
		if err := validateProposalAttributes(fields[updateProposalAttributesIndex]); err != nil {
			return fmt.Errorf("update proposal %d attributes: %w", index, err)
		}
		if err := validateProtocolParametersUpdate(fields[1]); err != nil {
			return fmt.Errorf("update proposal %d parameters: %w", index, err)
		}
	}
	votes, err := payloadEntries("update payload votes", parts[updatePayloadVotesIndex], len(decodedVotes), false)
	if err != nil {
		return err
	}
	for index, rawVote := range votes {
		if _, err := ParseUpdateVote(rawVote); err != nil {
			return fmt.Errorf("update vote %d: %w", index, err)
		}
	}
	return nil
}

func validateDelegationPayloadWire(raw []byte) error {
	if len(raw) == 0 || raw[0] != 0x9f {
		return errors.New("byron delegation certificates must use indefinite-list framing")
	}
	certificates, err := cborRawArrayEntries(raw, false)
	if err != nil {
		return fmt.Errorf("decode Byron delegation certificates: %w", err)
	}
	for i, rawCertificate := range certificates {
		if _, err := ParseDelegationCertificate(rawCertificate); err != nil {
			return fmt.Errorf("byron delegation certificate %d: %w", i, err)
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
	requireDefinite bool,
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
	entries, err := cborRawArrayEntries(raw, requireDefinite)
	if err != nil {
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
	fields, err := cborRawArrayEntries(raw, true)
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

func cborRawArrayEntries(raw []byte, requireDefinite bool) ([]cbor.RawMessage, error) {
	count, offset, indefinite, err := cborCollectionHeader(raw, 4)
	if err != nil {
		return nil, err
	}
	if indefinite && requireDefinite {
		return nil, errors.New("indefinite array is not permitted")
	}
	if indefinite {
		entries := make([]cbor.RawMessage, 0)
		for offset < len(raw) && raw[offset] != 0xff {
			end, err := cborItemEnd(raw, offset)
			if err != nil {
				return nil, err
			}
			entries = append(entries, append(cbor.RawMessage(nil), raw[offset:end]...))
			offset = end
		}
		if offset >= len(raw) || offset+1 != len(raw) {
			return nil, errors.New("malformed indefinite array")
		}
		return entries, nil
	}
	// The conversion is safe because len is nonnegative and bounded by MaxInt.
	if count > uint64(len(raw)-offset) { //nolint:gosec
		return nil, errors.New("truncated array elements")
	}
	entries := make([]cbor.RawMessage, 0, len(raw)-offset)
	for range count {
		end, err := cborItemEnd(raw, offset)
		if err != nil {
			return nil, err
		}
		entries = append(entries, append(cbor.RawMessage(nil), raw[offset:end]...))
		offset = end
	}
	if offset != len(raw) {
		return nil, errors.New("trailing bytes after array")
	}
	return entries, nil
}

func cborCollectionHeader(raw []byte, expectedMajor byte) (uint64, int, bool, error) {
	if len(raw) == 0 || raw[0]>>5 != expectedMajor {
		return 0, 0, false, errors.New("unexpected CBOR collection type")
	}
	argument := raw[0] & 0x1f
	if argument < 24 {
		return uint64(argument), 1, false, nil
	}
	if argument == 31 {
		return 0, 1, true, nil
	}
	if argument > 27 {
		return 0, 0, false, errors.New("reserved CBOR collection argument")
	}
	width := 1 << (argument - 24)
	if len(raw) < 1+width {
		return 0, 0, false, errors.New("truncated CBOR collection header")
	}
	var count uint64
	for _, value := range raw[1 : 1+width] {
		count = count<<8 | uint64(value)
	}
	return count, 1 + width, false, nil
}

func cborItemEnd(raw []byte, offset int) (int, error) {
	if offset >= len(raw) {
		return 0, errors.New("truncated CBOR item")
	}
	major := raw[offset] >> 5
	argument := raw[offset] & 0x1f
	_, headerLength, indefinite, err := cborCollectionHeader(raw[offset:], major)
	if err != nil && (major != 7 || argument < 28) {
		return 0, err
	}
	if argument < 24 {
		headerLength = 1
	} else if argument <= 27 {
		if headerLength == 0 {
			return 0, errors.New("truncated CBOR item header")
		}
	} else if argument == 31 && (major == 2 || major == 3 || major == 4 || major == 5) {
		indefinite = true
		headerLength = 1
	} else if major != 7 {
		return 0, errors.New("invalid indefinite CBOR item")
	}
	itemOffset := offset + headerLength
	argumentValue := uint64(0)
	if argument < 24 {
		argumentValue = uint64(argument)
	} else if argument <= 27 {
		for _, value := range raw[offset+1 : itemOffset] {
			argumentValue = argumentValue<<8 | uint64(value)
		}
	}
	switch major {
	case 0, 1:
		return itemOffset, nil
	case 2, 3:
		if indefinite {
			for itemOffset < len(raw) && raw[itemOffset] != 0xff {
				chunkEnd, err := cborItemEnd(raw, itemOffset)
				if err != nil || raw[itemOffset]>>5 != major || raw[itemOffset]&0x1f == 31 {
					return 0, errors.New("invalid indefinite string chunk")
				}
				itemOffset = chunkEnd
			}
			if itemOffset >= len(raw) {
				return 0, errors.New("unterminated indefinite string")
			}
			return itemOffset + 1, nil
		}
		// The conversion is safe because len is nonnegative and bounded by MaxInt.
		if argumentValue > uint64(len(raw)-itemOffset) { //nolint:gosec
			return 0, errors.New("truncated CBOR string")
		}
		return itemOffset + int(argumentValue), nil
	case 4, 5:
		items := argumentValue
		if major == 5 && !indefinite {
			// The conversion is safe because len is nonnegative and bounded by MaxInt.
			if items > uint64(len(raw)-itemOffset)/2 { //nolint:gosec
				return 0, errors.New("truncated CBOR map")
			}
			items *= 2
		}
		for index := uint64(0); indefinite || index < items; index++ {
			if indefinite && itemOffset < len(raw) && raw[itemOffset] == 0xff {
				return itemOffset + 1, nil
			}
			if itemOffset >= len(raw) || index >= uint64(len(raw)) {
				return 0, errors.New("truncated CBOR collection")
			}
			itemOffset, err = cborItemEnd(raw, itemOffset)
			if err != nil {
				return 0, err
			}
		}
		return itemOffset, nil
	case 6:
		return cborItemEnd(raw, itemOffset)
	case 7:
		if argument <= 23 {
			return itemOffset, nil
		}
		if argument >= 24 && argument <= 27 {
			return itemOffset, nil
		}
		return 0, errors.New("unexpected CBOR break")
	default:
		return 0, errors.New("unknown CBOR item type")
	}
}

// payloadBytes decodes one preserved payload field as a byte string of an
// exact length. The result is a fresh slice, so it does not alias the block
// it was decoded from.
func payloadBytes(
	label string,
	raw cbor.RawMessage,
	size int,
) ([]byte, error) {
	var value []byte
	if _, err := cbor.Decode(raw, &value); err != nil {
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
	return value, nil
}

func payloadVerificationKey(label string, raw cbor.RawMessage) ([]byte, error) {
	value, err := requireCanonicalByronByteString(raw, label)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrInvalidPayload, label, err)
	}
	if len(value) != VerificationKeySize {
		return nil, fmt.Errorf("%w: %s is %d bytes, expected %d", ErrInvalidPayload, label, len(value), VerificationKeySize)
	}
	return append([]byte(nil), value...), nil
}
