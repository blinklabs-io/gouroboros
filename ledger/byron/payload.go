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
)

// DelegationCertificate is a decoded Byron heavyweight delegation
// certificate from a main block's delegation payload. The verification keys
// are the full 64-byte extended form the wire format carries; see
// VerificationKeySize.
type DelegationCertificate struct {
	Epoch      uint64
	IssuerVK   []byte
	DelegateVK []byte
	Signature  []byte
}

// UpdateVote is a decoded Byron update-proposal vote from a main block's
// update payload.
type UpdateVote struct {
	VoterVK    []byte
	ProposalId []byte
	Decision   bool
	Signature  []byte
}

// ParseDelegationCertificate decodes and structurally validates one entry
// of a Byron main block's delegation payload, which the decoder hands back
// untyped because the payload as a whole is stored as []any.
//
// The returned certificate's byte slices are copies: callers routinely hold
// these past the lifetime of the block they came from, and a certificate
// that aliased the decoded block would let a later mutation of one change
// the other.
func ParseDelegationCertificate(raw any) (*DelegationCertificate, error) {
	fields, ok := raw.([]any)
	if !ok || len(fields) != delegationCertElementCount {
		return nil, fmt.Errorf(
			"%w: delegation certificate is not a %d-element array, "+
				"got %T with %d elements",
			ErrInvalidPayload, delegationCertElementCount, raw, len(fields),
		)
	}
	epoch, err := asUint(fields[delegationCertEpochIndex])
	if err != nil {
		return nil, fmt.Errorf(
			"%w: delegation certificate epoch: %w", ErrInvalidPayload, err,
		)
	}
	issuerVK, err := payloadBytes(
		"delegation certificate issuer verification key",
		fields[delegationCertIssuerIndex],
		VerificationKeySize,
	)
	if err != nil {
		return nil, err
	}
	delegateVK, err := payloadBytes(
		"delegation certificate delegate verification key",
		fields[delegationCertDelegateIndex],
		VerificationKeySize,
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
		protocolMagic, c.IssuerVK, c.DelegateVK, c.Signature, c.Epoch,
	)
}

// ParseUpdateVote decodes and structurally validates one entry of a Byron
// main block's update-payload vote list.
//
// The wire format is [voterVK, proposalId, decision, signature]. Byron only
// ever recorded positive votes -- cardano-ledger-byron drops the decision
// bit on decode and re-encodes a hardcoded True -- so a false decision is
// something no real block carries, and is rejected here rather than
// silently accepted as a vote whose signature covers the opposite value.
func ParseUpdateVote(raw any) (*UpdateVote, error) {
	fields, ok := raw.([]any)
	if !ok || len(fields) != updateVoteElementCount {
		return nil, fmt.Errorf(
			"%w: update vote is not a %d-element array, got %T with %d elements",
			ErrInvalidPayload, updateVoteElementCount, raw, len(fields),
		)
	}
	voterVK, err := payloadBytes(
		"update vote voter verification key",
		fields[updateVoteVoterIndex],
		VerificationKeySize,
	)
	if err != nil {
		return nil, err
	}
	proposalId, err := payloadBytes(
		"update vote proposal id",
		fields[updateVoteProposalIdIndex],
		common.Blake2b256Size,
	)
	if err != nil {
		return nil, err
	}
	decision, ok := fields[updateVoteDecisionIndex].(bool)
	if !ok {
		return nil, fmt.Errorf(
			"%w: update vote decision is not a boolean, got %T",
			ErrInvalidPayload, fields[updateVoteDecisionIndex],
		)
	}
	if !decision {
		return nil, fmt.Errorf(
			"%w: update vote decision is false, which Byron never records",
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
		VoterVK:    voterVK,
		ProposalId: proposalId,
		Decision:   decision,
		Signature:  signature,
	}, nil
}

// Verify checks the vote's signature, reproducing
// cardano-ledger-byron's Cardano.Chain.Update.Vote signing format:
//
//	inner  = 0x82 || CBOR(proposalId) || 0xf5
//	signed = 0x06 || CBOR(protocolMagic) || inner
//
// 0x82 is a two-element definite-length array header and 0xf5 is CBOR true,
// together the encoding of the (UpId, Bool) pair signatureForVote signs.
// This mirrors recoverSignedBytes, which reassembles the same two bytes
// around the proposal id's preserved encoding rather than re-encoding the
// pair.
func (v *UpdateVote) Verify(protocolMagic uint32) error {
	if v == nil {
		return fmt.Errorf("%w: update vote is nil", ErrInvalidPayload)
	}
	const (
		cborArrayLen2 byte = 0x82
		cborTrue      byte = 0xf5
	)
	proposalIdCbor, err := cbor.Encode(v.ProposalId)
	if err != nil {
		return fmt.Errorf("encode update vote proposal id: %w", err)
	}
	inner := make([]byte, 0, 2+len(proposalIdCbor))
	inner = append(inner, cborArrayLen2)
	inner = append(inner, proposalIdCbor...)
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
func (p *ByronUpdateProposal) signedBody() ([]byte, error) {
	const cborArrayLen5 byte = 0x85
	proposalCbor := p.Cbor()
	if len(proposalCbor) == 0 {
		return nil, fmt.Errorf(
			"%w: update proposal has no preserved CBOR", ErrInvalidPayload,
		)
	}
	var fields []cbor.RawMessage
	if _, err := cbor.Decode(proposalCbor, &fields); err != nil {
		return nil, fmt.Errorf(
			"%w: decode update proposal fields: %w", ErrInvalidPayload, err,
		)
	}
	if len(fields) != updateProposalElementCount {
		return nil, fmt.Errorf(
			"%w: update proposal is not a %d-element array, got %d elements",
			ErrInvalidPayload, updateProposalElementCount, len(fields),
		)
	}
	size := 1
	for _, field := range fields[:updateProposalSignedBodyCount] {
		size += len(field)
	}
	body := make([]byte, 0, size)
	body = append(body, cborArrayLen5)
	for _, field := range fields[:updateProposalSignedBodyCount] {
		body = append(body, field...)
	}
	return body, nil
}

// Validate structurally validates an update proposal and verifies its
// issuer signature, reproducing cardano-ledger-byron's
// Cardano.Chain.Update.Proposal signing format:
//
//	signed = 0x04 || CBOR(protocolMagic) || signedBody()
//
// See signedBody for how the signed body is recovered.
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
	body, err := p.signedBody()
	if err != nil {
		return err
	}
	signed, err := signedBytes(SignTagUSProposal, protocolMagic, body)
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

// ValidateDelegationPayload structurally validates every heavyweight
// delegation certificate in a Byron main block's delegation payload and
// verifies each certificate's signature.
//
// The block's own header protocol magic is used for domain separation, so a
// certificate signed for another network fails here.
func (b *ByronMainBlock) ValidateDelegationPayload() error {
	if b == nil || b.BlockHeader == nil {
		return fmt.Errorf(
			"%w: block or block header is nil", ErrInvalidPayload,
		)
	}
	protocolMagic := b.BlockHeader.ProtocolMagic
	for i, rawCertificate := range b.Body.DlgPayload {
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
	for i, rawVote := range b.Body.UpdPayload.Votes {
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

// ValidatePayloads runs ValidateDelegationPayload and
// ValidateUpdatePayload. This is what ValidateBodyProof calls when a caller
// opts in via common.VerifyConfig.EnableByronPayloadValidation.
func (b *ByronMainBlock) ValidatePayloads() error {
	if err := b.ValidateDelegationPayload(); err != nil {
		return err
	}
	return b.ValidateUpdatePayload()
}

// payloadBytes asserts that a decoded payload field is a byte string of an
// exact length, returning a copy so the result does not alias the block it
// was decoded from.
func payloadBytes(label string, value any, size int) ([]byte, error) {
	raw, ok := value.([]byte)
	if !ok || len(raw) != size {
		return nil, fmt.Errorf(
			"%w: %s is not %d bytes, got %T with length %d",
			ErrInvalidPayload, label, size, value, len(raw),
		)
	}
	return append([]byte(nil), raw...), nil
}
