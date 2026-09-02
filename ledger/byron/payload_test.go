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

package byron_test

import (
	"bytes"
	"crypto/ed25519"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

const testPayloadProtocolMagic = uint32(764824073)

// rawArray frames already-encoded fields as a definite-length CBOR array,
// leaving each field's bytes untouched. Tests assemble payload entries this
// way, rather than encoding a Go value, so they can control the exact wire
// encoding of individual fields -- which is the whole point of the
// non-shortest vectors below.
func rawArray(fields ...[]byte) cbor.RawMessage {
	count := len(fields)
	if count > 23 {
		panic("rawArray only handles arrays short enough for a 1-byte header")
	}
	out := []byte{byte(0x80 + count)}
	for _, field := range fields {
		out = append(out, field...)
	}
	return out
}

// testKeyPair returns a deterministic Byron extended verification key (the
// 32-byte Ed25519 public key followed by 32 chain-code bytes) and the
// Ed25519 private key that signs for it.
func testKeyPair(seedByte byte) ([]byte, ed25519.PrivateKey) {
	private := ed25519.NewKeyFromSeed(
		bytes.Repeat([]byte{seedByte}, ed25519.SeedSize),
	)
	verificationKey := make([]byte, byron.VerificationKeySize)
	copy(verificationKey, private.Public().(ed25519.PublicKey))
	copy(verificationKey[32:], bytes.Repeat([]byte{seedByte ^ 0xff}, 32))
	return verificationKey, private
}

// shortestEpoch and nonShortestEpoch encode the same epoch two ways. CBOR
// permits both and this decoder accepts both, but only one is what a given
// certificate's issuer signed over.
func shortestEpoch(t *testing.T, epoch uint64) []byte {
	t.Helper()
	return mustEncode(t, epoch)
}

func nonShortestEpoch(epoch byte) []byte {
	// Major type 0 with a 1-byte argument, where the shortest form would
	// have inlined the value in the initial byte.
	return []byte{0x18, epoch}
}

// shortestProposalId and nonShortestProposalId likewise encode the same
// 32-byte proposal id with a 1-byte and a 2-byte length header.
func shortestProposalId(t *testing.T, id []byte) []byte {
	t.Helper()
	return mustEncode(t, id)
}

func nonShortestProposalId(id []byte) []byte {
	out := []byte{0x59, 0x00, byte(len(id))}
	return append(out, id...)
}

// signedDelegationCertificate builds a delegation certificate over the
// caller's chosen epoch encoding, signing exactly those bytes.
func signedDelegationCertificate(
	t *testing.T,
	protocolMagic uint32,
	epochField []byte,
	issuerVK []byte,
	issuerPrivate ed25519.PrivateKey,
	delegateVK []byte,
) cbor.RawMessage {
	t.Helper()
	inner := make([]byte, 0, 2+len(delegateVK)+len(epochField))
	inner = append(inner, '0', '0')
	inner = append(inner, delegateVK...)
	inner = append(inner, epochField...)
	signed := []byte{byron.SignTagCertificate}
	signed = append(signed, mustEncode(t, protocolMagic)...)
	signed = append(signed, mustEncode(t, inner)...)
	return rawArray(
		epochField,
		mustEncode(t, issuerVK),
		mustEncode(t, delegateVK),
		mustEncode(t, ed25519.Sign(issuerPrivate, signed)),
	)
}

// signedUpdateVote builds a vote over the caller's chosen proposal id
// encoding, signing exactly those bytes.
func signedUpdateVote(
	t *testing.T,
	protocolMagic uint32,
	voterVK []byte,
	voterPrivate ed25519.PrivateKey,
	proposalIdField []byte,
) cbor.RawMessage {
	t.Helper()
	inner := []byte{0x82}
	inner = append(inner, proposalIdField...)
	inner = append(inner, 0xf5)
	signed := []byte{byron.SignTagUSVote}
	signed = append(signed, mustEncode(t, protocolMagic)...)
	signed = append(signed, inner...)
	return rawArray(
		mustEncode(t, voterVK),
		proposalIdField,
		mustEncode(t, true),
		mustEncode(t, ed25519.Sign(voterPrivate, signed)),
	)
}

// signedUpdateProposal builds a proposal whose signature covers its own
// first five fields, framed as a five-element array. attributesField lets a
// caller substitute a non-shortest encoding for one of those fields.
func signedUpdateProposal(
	t *testing.T,
	protocolMagic uint32,
	issuerVK []byte,
	issuerPrivate ed25519.PrivateKey,
	metadataField []byte,
	attributesField []byte,
) cbor.RawMessage {
	t.Helper()
	blockVersion := mustEncode(t, byron.ByronBlockVersion{Major: 1, Minor: 0})
	blockVersionMod := mustEncode(
		t,
		byron.ByronUpdateProposalBlockVersionMod{MaxTxSize: []uint64{4096}},
	)
	softwareVersion := mustEncode(
		t,
		byron.ByronSoftwareVersion{Name: "cardano-sl", Version: 1},
	)
	data := metadataField

	body := []byte{0x85}
	for _, field := range [][]byte{
		blockVersion, blockVersionMod, softwareVersion, data, attributesField,
	} {
		body = append(body, field...)
	}
	signed := []byte{byron.SignTagUSProposal}
	signed = append(signed, mustEncode(t, protocolMagic)...)
	signed = append(signed, body...)
	return rawArray(
		blockVersion,
		blockVersionMod,
		softwareVersion,
		data,
		attributesField,
		mustEncode(t, issuerVK),
		mustEncode(t, ed25519.Sign(issuerPrivate, signed)),
	)
}

// emptyMap is the canonical encoding of a map with no entries, which is
// what both an absent metadata map and the always-empty attributes field
// look like on the wire.
func emptyMap() []byte {
	return []byte{0xa0}
}

// nonCanonicalEmptyMap encodes the same empty map with a 1-byte count
// header. cardano-ledger-byron reads attributes with decodeMapLenCanonical
// and rejects this.
func nonCanonicalEmptyMap() []byte {
	return []byte{0xb8, 0x00}
}

// installerHashField builds an InstallerHash in the shape
// cardano-ledger-byron's decoder requires: a four-element array whose
// element 1 carries the blake2b-256 hash. Elements 0, 2, and 3 are the
// remains of cardano-sl's UpdateData record and are dropped by the
// reference, so their content is arbitrary here.
func installerHashField(t *testing.T) []byte {
	t.Helper()
	filler := mustEncode(t, bytes.Repeat([]byte{0x00}, common.Blake2b256Size))
	return rawArray(
		filler,
		mustEncode(t, bytes.Repeat([]byte{0x7c}, common.Blake2b256Size)),
		filler,
		filler,
	)
}

// metadataMap builds a one-entry metadata map from a system tag to an
// already-encoded installer hash.
func metadataMap(t *testing.T, tag string, installerHash []byte) []byte {
	t.Helper()
	out := []byte{0xa1}
	out = append(out, mustEncode(t, tag)...)
	return append(out, installerHash...)
}

// installerMetadata builds a metadata map of one system tag to one
// reference-shaped installer hash.
func installerMetadata(t *testing.T, tag string) []byte {
	t.Helper()
	return metadataMap(t, tag, installerHashField(t))
}

func decodeProposal(
	t *testing.T,
	raw cbor.RawMessage,
) byron.ByronUpdateProposal {
	t.Helper()
	var proposal byron.ByronUpdateProposal
	_, err := cbor.Decode(raw, &proposal)
	require.NoError(t, err)
	require.NotEmpty(t, proposal.Cbor())
	return proposal
}

// testMainBlock assembles a Byron main block body as CBOR and decodes it,
// so the block carries the preserved payload bytes that validation reads.
func testMainBlock(
	t *testing.T,
	protocolMagic uint32,
	certificates []cbor.RawMessage,
	proposals []cbor.RawMessage,
	votes []cbor.RawMessage,
) *byron.ByronMainBlock {
	t.Helper()
	rawList := func(entries []cbor.RawMessage) []byte {
		out := []byte{}
		for _, entry := range entries {
			out = append(out, entry...)
		}
		return append(
			[]byte{byte(0x80 + len(entries))}, out...,
		)
	}
	body := rawArray(
		[]byte{0x80},                             // empty tx payload
		mustEncode(t, []any{uint64(3), []any{}}), // certificates ssc payload
		rawList(certificates),
		rawArray(rawList(proposals), rawList(votes)),
	)
	var decoded byron.ByronMainBlockBody
	_, err := cbor.Decode(body, &decoded)
	require.NoError(t, err)
	return &byron.ByronMainBlock{
		BlockHeader: &byron.ByronMainBlockHeader{
			ProtocolMagic: protocolMagic,
		},
		Body: decoded,
	}
}

func TestParseDelegationCertificateValid(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x11)
	delegateVK, _ := testKeyPair(0x22)
	epochField := shortestEpoch(t, 7)
	raw := signedDelegationCertificate(
		t, testPayloadProtocolMagic, epochField, issuerVK, issuerPrivate,
		delegateVK,
	)

	certificate, err := byron.ParseDelegationCertificate(raw)
	require.NoError(t, err)
	require.Equal(t, uint64(7), certificate.Epoch)
	require.Equal(t, epochField, certificate.EpochCbor)
	require.Equal(t, issuerVK, certificate.IssuerVK)
	require.Equal(t, delegateVK, certificate.DelegateVK)
	require.NoError(t, certificate.Verify(testPayloadProtocolMagic))
}

// TestDelegationCertificateNonShortestEpoch is the regression vector for
// re-encoding the epoch instead of preserving it. CBOR admits a non-shortest
// integer encoding, this decoder accepts it, and the issuer signed the bytes
// that were actually on the wire -- so verification has to use those.
func TestDelegationCertificateNonShortestEpoch(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x11)
	delegateVK, _ := testKeyPair(0x22)
	epochField := nonShortestEpoch(7)
	require.NotEqual(t, shortestEpoch(t, 7), epochField)

	raw := signedDelegationCertificate(
		t, testPayloadProtocolMagic, epochField, issuerVK, issuerPrivate,
		delegateVK,
	)
	certificate, err := byron.ParseDelegationCertificate(raw)
	require.NoError(t, err)
	// The decoded value is the same epoch either way...
	require.Equal(t, uint64(7), certificate.Epoch)
	// ...but the preserved encoding is what the signature covers.
	require.Equal(t, epochField, certificate.EpochCbor)
	require.NoError(
		t,
		certificate.Verify(testPayloadProtocolMagic),
		"a certificate signed over a non-shortest epoch encoding must verify",
	)
}

// TestDelegationCertificateEpochEncodingIsLoadBearing is the converse: a
// certificate signed over the shortest encoding must NOT verify when the
// wire carries the non-shortest one, which proves the wire bytes are what
// gets used.
func TestDelegationCertificateEpochEncodingIsLoadBearing(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x11)
	delegateVK, _ := testKeyPair(0x22)
	signedOverShortest := signedDelegationCertificate(
		t, testPayloadProtocolMagic, shortestEpoch(t, 7), issuerVK,
		issuerPrivate, delegateVK,
	)
	fields := decodeFields(t, signedOverShortest, 4)
	// Swap in the other encoding of the same epoch, keeping the signature.
	swapped := rawArray(
		nonShortestEpoch(7), fields[1], fields[2], fields[3],
	)
	certificate, err := byron.ParseDelegationCertificate(swapped)
	require.NoError(t, err)
	require.Equal(t, uint64(7), certificate.Epoch)
	require.ErrorIs(
		t,
		certificate.Verify(testPayloadProtocolMagic),
		byron.ErrInvalidSignature,
	)
}

func decodeFields(
	t *testing.T,
	raw cbor.RawMessage,
	count int,
) []cbor.RawMessage {
	t.Helper()
	var decoded []cbor.RawMessage
	_, err := cbor.Decode(raw, &decoded)
	require.NoError(t, err)
	require.Len(t, decoded, count)
	fields := make([]cbor.RawMessage, count)
	copy(fields, decoded)
	return fields
}

func TestParseDelegationCertificateMalformed(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x11)
	delegateVK, _ := testKeyPair(0x22)
	valid := decodeFields(t, signedDelegationCertificate(
		t, testPayloadProtocolMagic, shortestEpoch(t, 7), issuerVK,
		issuerPrivate, delegateVK,
	), 4)

	testCases := []struct {
		name string
		raw  cbor.RawMessage
	}{
		{
			name: "empty",
			raw:  nil,
		},
		{
			name: "not an array",
			raw:  mustEncode(t, []byte{0x01, 0x02}),
		},
		{
			name: "too few elements",
			raw:  rawArray(valid[0], valid[1], valid[2]),
		},
		{
			name: "too many elements",
			raw: rawArray(
				valid[0], valid[1], valid[2], valid[3],
				mustEncode(t, uint64(0)),
			),
		},
		{
			name: "negative epoch",
			raw: rawArray(
				mustEncode(t, int64(-1)), valid[1], valid[2], valid[3],
			),
		},
		{
			name: "issuer key not bytes",
			raw: rawArray(
				valid[0], mustEncode(t, "not-a-key"), valid[2], valid[3],
			),
		},
		{
			name: "issuer key truncated",
			raw: rawArray(
				valid[0], mustEncode(t, issuerVK[:32]), valid[2], valid[3],
			),
		},
		{
			name: "delegate key truncated",
			raw: rawArray(
				valid[0], valid[1], mustEncode(t, delegateVK[:16]), valid[3],
			),
		},
		{
			name: "signature truncated",
			raw: rawArray(
				valid[0], valid[1], valid[2],
				mustEncode(t, make([]byte, ed25519.SignatureSize-1)),
			),
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := byron.ParseDelegationCertificate(testCase.raw)
			require.ErrorIs(t, err, byron.ErrInvalidPayload)
		})
	}
}

func TestDelegationCertificateSignatureMismatch(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x11)
	delegateVK, _ := testKeyPair(0x22)
	impostorVK, _ := testKeyPair(0x33)
	valid := decodeFields(t, signedDelegationCertificate(
		t, testPayloadProtocolMagic, shortestEpoch(t, 7), issuerVK,
		issuerPrivate, delegateVK,
	), 4)

	testCases := []struct {
		name  string
		raw   cbor.RawMessage
		magic uint32
	}{
		{
			name: "signed by another key",
			raw: rawArray(
				valid[0], mustEncode(t, impostorVK), valid[2], valid[3],
			),
			magic: testPayloadProtocolMagic,
		},
		{
			name: "delegate substituted",
			raw: rawArray(
				valid[0], valid[1], mustEncode(t, impostorVK), valid[3],
			),
			magic: testPayloadProtocolMagic,
		},
		{
			name: "epoch substituted",
			raw: rawArray(
				shortestEpoch(t, 8), valid[1], valid[2], valid[3],
			),
			magic: testPayloadProtocolMagic,
		},
		{
			name:  "wrong network",
			raw:   rawArray(valid[0], valid[1], valid[2], valid[3]),
			magic: testPayloadProtocolMagic + 1,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			certificate, err := byron.ParseDelegationCertificate(testCase.raw)
			require.NoError(t, err)
			require.ErrorIs(
				t,
				certificate.Verify(testCase.magic),
				byron.ErrInvalidSignature,
			)
		})
	}
}

func TestParseDelegationCertificateDoesNotAliasInput(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x11)
	delegateVK, _ := testKeyPair(0x22)
	raw := signedDelegationCertificate(
		t, testPayloadProtocolMagic, shortestEpoch(t, 7), issuerVK,
		issuerPrivate, delegateVK,
	)

	certificate, err := byron.ParseDelegationCertificate(raw)
	require.NoError(t, err)

	// Scribble over the buffer the certificate came from. A certificate
	// that aliased it would start verifying against bytes its issuer never
	// signed.
	for i := range raw {
		raw[i] = 0xff
	}
	require.NoError(t, certificate.Verify(testPayloadProtocolMagic))
}

func TestParseUpdateVoteValid(t *testing.T) {
	voterVK, voterPrivate := testKeyPair(0x44)
	proposalId := bytes.Repeat([]byte{0x5a}, common.Blake2b256Size)
	idField := shortestProposalId(t, proposalId)
	raw := signedUpdateVote(
		t, testPayloadProtocolMagic, voterVK, voterPrivate, idField,
	)

	vote, err := byron.ParseUpdateVote(raw)
	require.NoError(t, err)
	require.Equal(t, voterVK, vote.VoterVK)
	require.Equal(t, proposalId, vote.ProposalId)
	require.Equal(t, idField, vote.ProposalIdCbor)
	require.True(t, vote.Decision)
	require.NoError(t, vote.Verify(testPayloadProtocolMagic))
}

// TestUpdateVoteNonShortestProposalId is the regression vector the reviewer
// called out: a 32-byte proposal id encoded with a 2-byte length header
// (0x59 0x00 0x20) re-encodes to 0x58 0x20, so verifying against the
// re-encoding would reject a vote its voter signed correctly.
func TestUpdateVoteNonShortestProposalId(t *testing.T) {
	voterVK, voterPrivate := testKeyPair(0x44)
	proposalId := bytes.Repeat([]byte{0x5a}, common.Blake2b256Size)
	idField := nonShortestProposalId(proposalId)
	require.NotEqual(t, shortestProposalId(t, proposalId), idField)
	require.Equal(t, []byte{0x59, 0x00, 0x20}, idField[:3])

	raw := signedUpdateVote(
		t, testPayloadProtocolMagic, voterVK, voterPrivate, idField,
	)
	vote, err := byron.ParseUpdateVote(raw)
	require.NoError(t, err)
	require.Equal(t, proposalId, vote.ProposalId)
	require.Equal(t, idField, vote.ProposalIdCbor)
	require.NoError(
		t,
		vote.Verify(testPayloadProtocolMagic),
		"a vote signed over a non-shortest proposal id must verify",
	)
}

// TestUpdateVoteProposalIdEncodingIsLoadBearing is the converse of the
// above: swapping the wire encoding while keeping the signature must fail.
func TestUpdateVoteProposalIdEncodingIsLoadBearing(t *testing.T) {
	voterVK, voterPrivate := testKeyPair(0x44)
	proposalId := bytes.Repeat([]byte{0x5a}, common.Blake2b256Size)
	signedOverShortest := signedUpdateVote(
		t, testPayloadProtocolMagic, voterVK, voterPrivate,
		shortestProposalId(t, proposalId),
	)
	fields := decodeFields(t, signedOverShortest, 4)
	swapped := rawArray(
		fields[0], nonShortestProposalId(proposalId), fields[2], fields[3],
	)
	vote, err := byron.ParseUpdateVote(swapped)
	require.NoError(t, err)
	require.Equal(t, proposalId, vote.ProposalId)
	require.ErrorIs(
		t,
		vote.Verify(testPayloadProtocolMagic),
		byron.ErrInvalidSignature,
	)
}

func TestParseUpdateVoteMalformed(t *testing.T) {
	voterVK, voterPrivate := testKeyPair(0x44)
	proposalId := bytes.Repeat([]byte{0x5a}, common.Blake2b256Size)
	valid := decodeFields(t, signedUpdateVote(
		t, testPayloadProtocolMagic, voterVK, voterPrivate,
		shortestProposalId(t, proposalId),
	), 4)

	testCases := []struct {
		name string
		raw  cbor.RawMessage
	}{
		{
			name: "empty",
			raw:  nil,
		},
		{
			name: "not an array",
			raw:  mustEncode(t, uint64(3)),
		},
		{
			name: "too few elements",
			raw:  rawArray(valid[0], valid[1], valid[2]),
		},
		{
			name: "voter key truncated",
			raw: rawArray(
				mustEncode(t, voterVK[:32]), valid[1], valid[2], valid[3],
			),
		},
		{
			name: "proposal id wrong length",
			raw: rawArray(
				valid[0], mustEncode(t, proposalId[:16]), valid[2], valid[3],
			),
		},
		{
			name: "decision not a boolean",
			raw: rawArray(
				valid[0], valid[1], mustEncode(t, uint64(1)), valid[3],
			),
		},
		{
			name: "decision false",
			raw: rawArray(
				valid[0], valid[1], mustEncode(t, false), valid[3],
			),
		},
		{
			name: "signature truncated",
			raw: rawArray(
				valid[0], valid[1], valid[2], mustEncode(t, make([]byte, 8)),
			),
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := byron.ParseUpdateVote(testCase.raw)
			require.ErrorIs(t, err, byron.ErrInvalidPayload)
		})
	}
}

func TestUpdateVoteSignatureMismatch(t *testing.T) {
	voterVK, voterPrivate := testKeyPair(0x44)
	impostorVK, _ := testKeyPair(0x55)
	proposalId := bytes.Repeat([]byte{0x5a}, common.Blake2b256Size)
	valid := decodeFields(t, signedUpdateVote(
		t, testPayloadProtocolMagic, voterVK, voterPrivate,
		shortestProposalId(t, proposalId),
	), 4)

	testCases := []struct {
		name  string
		raw   cbor.RawMessage
		magic uint32
	}{
		{
			name: "voter substituted",
			raw: rawArray(
				mustEncode(t, impostorVK), valid[1], valid[2], valid[3],
			),
			magic: testPayloadProtocolMagic,
		},
		{
			name: "proposal id substituted",
			raw: rawArray(
				valid[0],
				mustEncode(
					t, bytes.Repeat([]byte{0x01}, common.Blake2b256Size),
				),
				valid[2],
				valid[3],
			),
			magic: testPayloadProtocolMagic,
		},
		{
			name:  "wrong network",
			raw:   rawArray(valid[0], valid[1], valid[2], valid[3]),
			magic: testPayloadProtocolMagic + 1,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			vote, err := byron.ParseUpdateVote(testCase.raw)
			require.NoError(t, err)
			require.ErrorIs(
				t,
				vote.Verify(testCase.magic),
				byron.ErrInvalidSignature,
			)
		})
	}
}

func TestUpdateProposalValid(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x66)
	proposal := decodeProposal(t, signedUpdateProposal(
		t, testPayloadProtocolMagic, issuerVK, issuerPrivate,
		emptyMap(), emptyMap(),
	))
	require.NoError(t, proposal.Validate(testPayloadProtocolMagic))
}

// TestUpdateProposalShapes covers the metadata and attributes fields, which
// the signature covers but does not constrain: a correctly signed proposal
// can still carry values cardano-ledger-byron's ProposalBody decoder
// refuses, and Validate has to refuse them too.
func TestUpdateProposalShapes(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x66)

	accepted := []struct {
		name       string
		metadata   []byte
		attributes []byte
	}{
		{
			name:       "empty metadata and attributes",
			metadata:   emptyMap(),
			attributes: emptyMap(),
		},
		{
			name:       "one installer",
			metadata:   installerMetadata(t, "linux"),
			attributes: emptyMap(),
		},
		{
			name:       "system tag at the length limit",
			metadata:   installerMetadata(t, "0123456789"),
			attributes: emptyMap(),
		},
		{
			// Elements 0, 2 and 3 are dropped by the reference without
			// being interpreted, so their content must not be constrained.
			name: "installer hash with arbitrary dropped elements",
			metadata: metadataMap(t, "linux", rawArray(
				mustEncode(t, uint64(1)),
				mustEncode(t, bytes.Repeat([]byte{0x7c}, 32)),
				mustEncode(t, "anything"),
				mustEncode(t, []any{}),
			)),
			attributes: emptyMap(),
		},
	}
	for _, testCase := range accepted {
		t.Run("accepts "+testCase.name, func(t *testing.T) {
			proposal := decodeProposal(t, signedUpdateProposal(
				t, testPayloadProtocolMagic, issuerVK, issuerPrivate,
				testCase.metadata, testCase.attributes,
			))
			require.NoError(t, proposal.Validate(testPayloadProtocolMagic))
		})
	}

	rejected := []struct {
		name       string
		metadata   []byte
		attributes []byte
	}{
		{
			// The case from review: both fields decode into `any`, so an
			// integer in either one used to sail through on a good
			// signature.
			name:       "integer metadata and attributes",
			metadata:   mustEncode(t, uint64(99)),
			attributes: mustEncode(t, uint64(99)),
		},
		{
			name:       "integer metadata",
			metadata:   mustEncode(t, uint64(99)),
			attributes: emptyMap(),
		},
		{
			name:       "integer attributes",
			metadata:   emptyMap(),
			attributes: mustEncode(t, uint64(99)),
		},
		{
			name:       "array metadata",
			metadata:   rawArray(mustEncode(t, uint64(1))),
			attributes: emptyMap(),
		},
		{
			name:       "non-empty attributes",
			metadata:   emptyMap(),
			attributes: installerMetadata(t, "linux"),
		},
		{
			name:       "non-canonical empty attributes",
			metadata:   emptyMap(),
			attributes: nonCanonicalEmptyMap(),
		},
		{
			name:       "system tag over the length limit",
			metadata:   installerMetadata(t, "01234567890"),
			attributes: emptyMap(),
		},
		{
			name:       "non-ascii system tag",
			metadata:   installerMetadata(t, "linu\u00fe"),
			attributes: emptyMap(),
		},
		{
			name: "installer hash not wrapped in an array",
			metadata: metadataMap(t, "linux", mustEncode(
				t, bytes.Repeat([]byte{0x7c}, common.Blake2b256Size),
			)),
			attributes: emptyMap(),
		},
		{
			// The pre-cardano-ledger shape: a one-element array. The
			// reference enforces four.
			name: "installer hash with one element",
			metadata: metadataMap(t, "linux", rawArray(
				mustEncode(t, bytes.Repeat([]byte{0x7c}, common.Blake2b256Size)),
			)),
			attributes: emptyMap(),
		},
		{
			name: "installer hash with five elements",
			metadata: metadataMap(t, "linux", rawArray(
				mustEncode(t, bytes.Repeat([]byte{0x00}, 32)),
				mustEncode(t, bytes.Repeat([]byte{0x7c}, 32)),
				mustEncode(t, bytes.Repeat([]byte{0x00}, 32)),
				mustEncode(t, bytes.Repeat([]byte{0x00}, 32)),
				mustEncode(t, bytes.Repeat([]byte{0x00}, 32)),
			)),
			attributes: emptyMap(),
		},
		{
			name: "installer hash wrong length",
			metadata: metadataMap(t, "linux", rawArray(
				mustEncode(t, bytes.Repeat([]byte{0x00}, 32)),
				mustEncode(t, bytes.Repeat([]byte{0x7c}, 16)),
				mustEncode(t, bytes.Repeat([]byte{0x00}, 32)),
				mustEncode(t, bytes.Repeat([]byte{0x00}, 32)),
			)),
			attributes: emptyMap(),
		},
	}
	for _, testCase := range rejected {
		t.Run("rejects "+testCase.name, func(t *testing.T) {
			raw := signedUpdateProposal(
				t, testPayloadProtocolMagic, issuerVK, issuerPrivate,
				testCase.metadata, testCase.attributes,
			)
			proposal := decodeProposal(t, raw)
			// The signature is genuine; only the shape is wrong.
			require.ErrorIs(
				t,
				proposal.Validate(testPayloadProtocolMagic),
				byron.ErrInvalidPayload,
			)
		})
	}
}

func TestUpdateProposalMalformed(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x66)
	valid := decodeProposal(t, signedUpdateProposal(
		t, testPayloadProtocolMagic, issuerVK, issuerPrivate,
		emptyMap(), emptyMap(),
	))

	t.Run("issuer key truncated", func(t *testing.T) {
		proposal := valid
		proposal.From = issuerVK[:32]
		require.ErrorIs(
			t,
			proposal.Validate(testPayloadProtocolMagic),
			byron.ErrInvalidPayload,
		)
	})

	t.Run("signature truncated", func(t *testing.T) {
		proposal := valid
		proposal.Signature = make([]byte, 16)
		require.ErrorIs(
			t,
			proposal.Validate(testPayloadProtocolMagic),
			byron.ErrInvalidPayload,
		)
	})

	t.Run("no preserved cbor", func(t *testing.T) {
		proposal := byron.ByronUpdateProposal{
			From:      append([]byte(nil), issuerVK...),
			Signature: make([]byte, ed25519.SignatureSize),
		}
		require.ErrorIs(
			t,
			proposal.Validate(testPayloadProtocolMagic),
			byron.ErrInvalidPayload,
		)
	})

	t.Run("nil proposal", func(t *testing.T) {
		var proposal *byron.ByronUpdateProposal
		require.ErrorIs(
			t,
			proposal.Validate(testPayloadProtocolMagic),
			byron.ErrInvalidPayload,
		)
	})
}

func TestUpdateProposalSignatureMismatch(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x66)
	impostorVK, _ := testKeyPair(0x77)
	valid := decodeProposal(t, signedUpdateProposal(
		t, testPayloadProtocolMagic, issuerVK, issuerPrivate,
		emptyMap(), emptyMap(),
	))

	t.Run("issuer substituted", func(t *testing.T) {
		proposal := valid
		proposal.From = append([]byte(nil), impostorVK...)
		require.ErrorIs(
			t,
			proposal.Validate(testPayloadProtocolMagic),
			byron.ErrInvalidSignature,
		)
	})

	t.Run("signature from another proposal", func(t *testing.T) {
		other := decodeProposal(t, signedUpdateProposal(
			t, testPayloadProtocolMagic+1, issuerVK, issuerPrivate,
			emptyMap(), emptyMap(),
		))
		proposal := valid
		proposal.Signature = append([]byte(nil), other.Signature...)
		require.ErrorIs(
			t,
			proposal.Validate(testPayloadProtocolMagic),
			byron.ErrInvalidSignature,
		)
	})

	t.Run("wrong network", func(t *testing.T) {
		require.ErrorIs(
			t,
			valid.Validate(testPayloadProtocolMagic+1),
			byron.ErrInvalidSignature,
		)
	})
}

func TestValidatePayloadsEmpty(t *testing.T) {
	block := testMainBlock(t, testPayloadProtocolMagic, nil, nil, nil)
	require.NoError(t, block.ValidatePayloads())
}

func TestValidatePayloadsValid(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x11)
	delegateVK, _ := testKeyPair(0x22)
	voterVK, voterPrivate := testKeyPair(0x44)
	proposerVK, proposerPrivate := testKeyPair(0x66)

	block := testMainBlock(
		t,
		testPayloadProtocolMagic,
		[]cbor.RawMessage{
			signedDelegationCertificate(
				t, testPayloadProtocolMagic, shortestEpoch(t, 7), issuerVK,
				issuerPrivate, delegateVK,
			),
		},
		[]cbor.RawMessage{
			signedUpdateProposal(
				t, testPayloadProtocolMagic, proposerVK, proposerPrivate,
				emptyMap(), emptyMap(),
			),
		},
		[]cbor.RawMessage{
			signedUpdateVote(
				t, testPayloadProtocolMagic, voterVK, voterPrivate,
				shortestProposalId(
					t, bytes.Repeat([]byte{0x5a}, common.Blake2b256Size),
				),
			),
		},
	)
	require.NoError(t, block.ValidatePayloads())
}

// TestValidatePayloadsNonShortestEncodings runs a whole block whose
// delegation certificate, vote, and proposal all use non-shortest field
// encodings. This is the end-to-end form of the regression: before
// preserving the wire bytes, every one of these would have been rejected.
func TestValidatePayloadsNonShortestEncodings(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x11)
	delegateVK, _ := testKeyPair(0x22)
	voterVK, voterPrivate := testKeyPair(0x44)
	proposerVK, proposerPrivate := testKeyPair(0x66)

	block := testMainBlock(
		t,
		testPayloadProtocolMagic,
		[]cbor.RawMessage{
			signedDelegationCertificate(
				t, testPayloadProtocolMagic, nonShortestEpoch(7), issuerVK,
				issuerPrivate, delegateVK,
			),
		},
		[]cbor.RawMessage{
			signedUpdateProposal(
				t, testPayloadProtocolMagic, proposerVK, proposerPrivate,
				emptyMap(), emptyMap(),
			),
		},
		[]cbor.RawMessage{
			signedUpdateVote(
				t, testPayloadProtocolMagic, voterVK, voterPrivate,
				nonShortestProposalId(
					bytes.Repeat([]byte{0x5a}, common.Blake2b256Size),
				),
			),
		},
	)
	require.NoError(t, block.ValidatePayloads())
}

func TestValidatePayloadsReportsOffendingIndex(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x11)
	delegateVK, _ := testKeyPair(0x22)
	good := signedDelegationCertificate(
		t, testPayloadProtocolMagic, shortestEpoch(t, 7), issuerVK,
		issuerPrivate, delegateVK,
	)

	block := testMainBlock(
		t,
		testPayloadProtocolMagic,
		[]cbor.RawMessage{good, rawArray(mustEncode(t, uint64(1)))},
		nil,
		nil,
	)
	err := block.ValidateDelegationPayload()
	require.ErrorIs(t, err, byron.ErrInvalidPayload)
	require.ErrorContains(t, err, "delegation certificate 1")
}

func TestValidatePayloadsWrongNetwork(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x11)
	delegateVK, _ := testKeyPair(0x22)
	// The certificate is signed for one network; the block claims another.
	block := testMainBlock(
		t,
		testPayloadProtocolMagic+1,
		[]cbor.RawMessage{
			signedDelegationCertificate(
				t, testPayloadProtocolMagic, shortestEpoch(t, 7), issuerVK,
				issuerPrivate, delegateVK,
			),
		},
		nil,
		nil,
	)
	require.ErrorIs(
		t,
		block.ValidateDelegationPayload(),
		byron.ErrInvalidSignature,
	)
}

// TestValidatePayloadsRequiresPreservedCbor pins that a block assembled in
// Go, with no preserved payload bytes, is rejected rather than verified
// against re-encoded ones.
func TestValidatePayloadsRequiresPreservedCbor(t *testing.T) {
	block := &byron.ByronMainBlock{
		BlockHeader: &byron.ByronMainBlockHeader{
			ProtocolMagic: testPayloadProtocolMagic,
		},
		Body: byron.ByronMainBlockBody{
			DlgPayload: []any{[]any{uint64(7)}},
		},
	}
	err := block.ValidateDelegationPayload()
	require.ErrorIs(t, err, byron.ErrInvalidPayload)
	require.ErrorContains(t, err, "no preserved CBOR")

	votes := &byron.ByronMainBlock{
		BlockHeader: &byron.ByronMainBlockHeader{
			ProtocolMagic: testPayloadProtocolMagic,
		},
		Body: byron.ByronMainBlockBody{
			UpdPayload: byron.ByronUpdatePayload{Votes: []any{[]any{}}},
		},
	}
	err = votes.ValidateUpdatePayload()
	require.ErrorIs(t, err, byron.ErrInvalidPayload)
	require.ErrorContains(t, err, "no preserved CBOR")
}

func TestValidatePayloadsNilBlock(t *testing.T) {
	var block *byron.ByronMainBlock
	require.ErrorIs(
		t,
		block.ValidateDelegationPayload(),
		byron.ErrInvalidPayload,
	)
	require.ErrorIs(t, block.ValidateUpdatePayload(), byron.ErrInvalidPayload)

	headerless := &byron.ByronMainBlock{}
	require.ErrorIs(
		t,
		headerless.ValidateDelegationPayload(),
		byron.ErrInvalidPayload,
	)
	require.ErrorIs(
		t,
		headerless.ValidateUpdatePayload(),
		byron.ErrInvalidPayload,
	)
}
