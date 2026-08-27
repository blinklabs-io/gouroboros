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

// signedDelegationCertificate builds a delegation certificate in the wire
// shape the decoder produces, signed by issuerPrivate.
func signedDelegationCertificate(
	t *testing.T,
	protocolMagic uint32,
	epoch uint64,
	issuerVK []byte,
	issuerPrivate ed25519.PrivateKey,
	delegateVK []byte,
) []any {
	t.Helper()
	epochCbor, err := cbor.Encode(epoch)
	require.NoError(t, err)
	inner := make([]byte, 0, 2+len(delegateVK)+len(epochCbor))
	inner = append(inner, '0', '0')
	inner = append(inner, delegateVK...)
	inner = append(inner, epochCbor...)
	innerCbor, err := cbor.Encode(inner)
	require.NoError(t, err)
	magicCbor, err := cbor.Encode(protocolMagic)
	require.NoError(t, err)
	signed := []byte{byron.SignTagCertificate}
	signed = append(signed, magicCbor...)
	signed = append(signed, innerCbor...)
	return []any{
		epoch,
		append([]byte(nil), issuerVK...),
		append([]byte(nil), delegateVK...),
		ed25519.Sign(issuerPrivate, signed),
	}
}

// signedUpdateVote builds an update vote in the wire shape the decoder
// produces, signed by voterPrivate.
func signedUpdateVote(
	t *testing.T,
	protocolMagic uint32,
	voterVK []byte,
	voterPrivate ed25519.PrivateKey,
	proposalId []byte,
) []any {
	t.Helper()
	proposalIdCbor, err := cbor.Encode(proposalId)
	require.NoError(t, err)
	magicCbor, err := cbor.Encode(protocolMagic)
	require.NoError(t, err)
	signed := []byte{byron.SignTagUSVote}
	signed = append(signed, magicCbor...)
	signed = append(signed, 0x82)
	signed = append(signed, proposalIdCbor...)
	signed = append(signed, 0xf5)
	return []any{
		append([]byte(nil), voterVK...),
		append([]byte(nil), proposalId...),
		true,
		ed25519.Sign(voterPrivate, signed),
	}
}

// roundTripProposal encodes a proposal and decodes it back so the result
// carries preserved CBOR, which is what the signature check reads its
// signed body out of.
func roundTripProposal(
	t *testing.T,
	proposal byron.ByronUpdateProposal,
) byron.ByronUpdateProposal {
	t.Helper()
	encoded, err := cbor.Encode(&proposal)
	require.NoError(t, err)
	var decoded byron.ByronUpdateProposal
	_, err = cbor.Decode(encoded, &decoded)
	require.NoError(t, err)
	require.NotEmpty(t, decoded.Cbor())
	return decoded
}

// signedUpdateProposal builds a proposal whose signature covers its own
// first five fields, by round-tripping it once with a placeholder signature
// to recover the exact signed bytes, then again with the real one.
func signedUpdateProposal(
	t *testing.T,
	protocolMagic uint32,
	issuerVK []byte,
	issuerPrivate ed25519.PrivateKey,
) byron.ByronUpdateProposal {
	t.Helper()
	base := byron.ByronUpdateProposal{
		BlockVersion: byron.ByronBlockVersion{
			Major: 1,
			Minor: 0,
		},
		BlockVersionMod: byron.ByronUpdateProposalBlockVersionMod{
			MaxTxSize: []uint64{4096},
		},
		SoftwareVersion: byron.ByronSoftwareVersion{
			Name:    "cardano-sl",
			Version: 1,
		},
		Data:       map[string]any{},
		Attributes: map[string]any{},
		From:       append([]byte(nil), issuerVK...),
		Signature:  make([]byte, ed25519.SignatureSize),
	}
	// Recover the signed body from a placeholder round trip: the signature
	// sits outside the five fields it covers, so signing does not disturb
	// them.
	placeholder := roundTripProposal(t, base)
	var fields []cbor.RawMessage
	_, err := cbor.Decode(placeholder.Cbor(), &fields)
	require.NoError(t, err)
	require.Len(t, fields, 7)
	body := []byte{0x85}
	for _, field := range fields[:5] {
		body = append(body, field...)
	}
	magicCbor, err := cbor.Encode(protocolMagic)
	require.NoError(t, err)
	signed := []byte{byron.SignTagUSProposal}
	signed = append(signed, magicCbor...)
	signed = append(signed, body...)
	base.Signature = ed25519.Sign(issuerPrivate, signed)
	return roundTripProposal(t, base)
}

func testMainBlock(
	protocolMagic uint32,
	dlgPayload []any,
	updPayload byron.ByronUpdatePayload,
) *byron.ByronMainBlock {
	return &byron.ByronMainBlock{
		BlockHeader: &byron.ByronMainBlockHeader{
			ProtocolMagic: protocolMagic,
		},
		Body: byron.ByronMainBlockBody{
			DlgPayload: dlgPayload,
			UpdPayload: updPayload,
		},
	}
}

func TestParseDelegationCertificateValid(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x11)
	delegateVK, _ := testKeyPair(0x22)
	raw := signedDelegationCertificate(
		t, testPayloadProtocolMagic, 7, issuerVK, issuerPrivate, delegateVK,
	)

	certificate, err := byron.ParseDelegationCertificate(raw)
	require.NoError(t, err)
	require.Equal(t, uint64(7), certificate.Epoch)
	require.Equal(t, issuerVK, certificate.IssuerVK)
	require.Equal(t, delegateVK, certificate.DelegateVK)
	require.NoError(t, certificate.Verify(testPayloadProtocolMagic))
}

func TestParseDelegationCertificateMalformed(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x11)
	delegateVK, _ := testKeyPair(0x22)
	valid := signedDelegationCertificate(
		t, testPayloadProtocolMagic, 7, issuerVK, issuerPrivate, delegateVK,
	)

	testCases := []struct {
		name string
		raw  any
	}{
		{
			name: "not an array",
			raw:  []byte{0x01, 0x02},
		},
		{
			name: "too few elements",
			raw:  valid[:3],
		},
		{
			name: "too many elements",
			raw:  append(append([]any{}, valid...), uint64(0)),
		},
		{
			name: "negative epoch",
			raw: []any{
				int64(-1), valid[1], valid[2], valid[3],
			},
		},
		{
			name: "issuer key not bytes",
			raw: []any{
				valid[0], "not-a-key", valid[2], valid[3],
			},
		},
		{
			name: "issuer key truncated",
			raw: []any{
				valid[0],
				issuerVK[:32],
				valid[2],
				valid[3],
			},
		},
		{
			name: "delegate key truncated",
			raw: []any{
				valid[0], valid[1], delegateVK[:16], valid[3],
			},
		},
		{
			name: "signature truncated",
			raw: []any{
				valid[0],
				valid[1],
				valid[2],
				make([]byte, ed25519.SignatureSize-1),
			},
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

	t.Run("signed by another key", func(t *testing.T) {
		raw := signedDelegationCertificate(
			t, testPayloadProtocolMagic, 7, issuerVK, issuerPrivate,
			delegateVK,
		)
		// Keep the signature, swap the key it claims to come from.
		raw[1] = impostorVK
		certificate, err := byron.ParseDelegationCertificate(raw)
		require.NoError(t, err)
		require.ErrorIs(
			t,
			certificate.Verify(testPayloadProtocolMagic),
			byron.ErrInvalidSignature,
		)
	})

	t.Run("delegate substituted", func(t *testing.T) {
		raw := signedDelegationCertificate(
			t, testPayloadProtocolMagic, 7, issuerVK, issuerPrivate,
			delegateVK,
		)
		raw[2] = impostorVK
		certificate, err := byron.ParseDelegationCertificate(raw)
		require.NoError(t, err)
		require.ErrorIs(
			t,
			certificate.Verify(testPayloadProtocolMagic),
			byron.ErrInvalidSignature,
		)
	})

	t.Run("epoch substituted", func(t *testing.T) {
		raw := signedDelegationCertificate(
			t, testPayloadProtocolMagic, 7, issuerVK, issuerPrivate,
			delegateVK,
		)
		raw[0] = uint64(8)
		certificate, err := byron.ParseDelegationCertificate(raw)
		require.NoError(t, err)
		require.ErrorIs(
			t,
			certificate.Verify(testPayloadProtocolMagic),
			byron.ErrInvalidSignature,
		)
	})

	t.Run("wrong network", func(t *testing.T) {
		raw := signedDelegationCertificate(
			t, testPayloadProtocolMagic, 7, issuerVK, issuerPrivate,
			delegateVK,
		)
		certificate, err := byron.ParseDelegationCertificate(raw)
		require.NoError(t, err)
		require.ErrorIs(
			t,
			certificate.Verify(testPayloadProtocolMagic+1),
			byron.ErrInvalidSignature,
		)
	})
}

func TestParseDelegationCertificateDoesNotAliasInput(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x11)
	delegateVK, _ := testKeyPair(0x22)
	raw := signedDelegationCertificate(
		t, testPayloadProtocolMagic, 7, issuerVK, issuerPrivate, delegateVK,
	)

	certificate, err := byron.ParseDelegationCertificate(raw)
	require.NoError(t, err)

	// Scribble over the decoded payload the certificate came from. A
	// certificate that aliased it would start verifying against bytes its
	// issuer never signed.
	for _, index := range []int{1, 2, 3} {
		field, ok := raw[index].([]byte)
		require.True(t, ok)
		for i := range field {
			field[i] = 0xff
		}
	}
	require.NoError(t, certificate.Verify(testPayloadProtocolMagic))
}

func TestParseUpdateVoteValid(t *testing.T) {
	voterVK, voterPrivate := testKeyPair(0x44)
	proposalId := bytes.Repeat([]byte{0x5a}, common.Blake2b256Size)
	raw := signedUpdateVote(
		t, testPayloadProtocolMagic, voterVK, voterPrivate, proposalId,
	)

	vote, err := byron.ParseUpdateVote(raw)
	require.NoError(t, err)
	require.Equal(t, voterVK, vote.VoterVK)
	require.Equal(t, proposalId, vote.ProposalId)
	require.True(t, vote.Decision)
	require.NoError(t, vote.Verify(testPayloadProtocolMagic))
}

func TestParseUpdateVoteMalformed(t *testing.T) {
	voterVK, voterPrivate := testKeyPair(0x44)
	proposalId := bytes.Repeat([]byte{0x5a}, common.Blake2b256Size)
	valid := signedUpdateVote(
		t, testPayloadProtocolMagic, voterVK, voterPrivate, proposalId,
	)

	testCases := []struct {
		name string
		raw  any
	}{
		{
			name: "not an array",
			raw:  uint64(3),
		},
		{
			name: "too few elements",
			raw:  valid[:3],
		},
		{
			name: "voter key truncated",
			raw:  []any{voterVK[:32], valid[1], valid[2], valid[3]},
		},
		{
			name: "proposal id wrong length",
			raw:  []any{valid[0], proposalId[:16], valid[2], valid[3]},
		},
		{
			name: "decision not a boolean",
			raw:  []any{valid[0], valid[1], uint64(1), valid[3]},
		},
		{
			name: "decision false",
			raw:  []any{valid[0], valid[1], false, valid[3]},
		},
		{
			name: "signature truncated",
			raw: []any{
				valid[0], valid[1], valid[2], make([]byte, 8),
			},
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

	t.Run("voter substituted", func(t *testing.T) {
		raw := signedUpdateVote(
			t, testPayloadProtocolMagic, voterVK, voterPrivate, proposalId,
		)
		raw[0] = impostorVK
		vote, err := byron.ParseUpdateVote(raw)
		require.NoError(t, err)
		require.ErrorIs(
			t,
			vote.Verify(testPayloadProtocolMagic),
			byron.ErrInvalidSignature,
		)
	})

	t.Run("proposal id substituted", func(t *testing.T) {
		raw := signedUpdateVote(
			t, testPayloadProtocolMagic, voterVK, voterPrivate, proposalId,
		)
		raw[1] = bytes.Repeat([]byte{0x01}, common.Blake2b256Size)
		vote, err := byron.ParseUpdateVote(raw)
		require.NoError(t, err)
		require.ErrorIs(
			t,
			vote.Verify(testPayloadProtocolMagic),
			byron.ErrInvalidSignature,
		)
	})

	t.Run("wrong network", func(t *testing.T) {
		raw := signedUpdateVote(
			t, testPayloadProtocolMagic, voterVK, voterPrivate, proposalId,
		)
		vote, err := byron.ParseUpdateVote(raw)
		require.NoError(t, err)
		require.ErrorIs(
			t,
			vote.Verify(testPayloadProtocolMagic+1),
			byron.ErrInvalidSignature,
		)
	})
}

func TestUpdateProposalValid(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x66)
	proposal := signedUpdateProposal(
		t, testPayloadProtocolMagic, issuerVK, issuerPrivate,
	)
	require.NoError(t, proposal.Validate(testPayloadProtocolMagic))
}

func TestUpdateProposalMalformed(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x66)
	valid := signedUpdateProposal(
		t, testPayloadProtocolMagic, issuerVK, issuerPrivate,
	)

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
	valid := signedUpdateProposal(
		t, testPayloadProtocolMagic, issuerVK, issuerPrivate,
	)

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
		other := signedUpdateProposal(
			t, testPayloadProtocolMagic+1, issuerVK, issuerPrivate,
		)
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
	block := testMainBlock(
		testPayloadProtocolMagic, nil, byron.ByronUpdatePayload{},
	)
	require.NoError(t, block.ValidatePayloads())
}

func TestValidatePayloadsValid(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x11)
	delegateVK, _ := testKeyPair(0x22)
	voterVK, voterPrivate := testKeyPair(0x44)
	proposerVK, proposerPrivate := testKeyPair(0x66)

	block := testMainBlock(
		testPayloadProtocolMagic,
		[]any{
			signedDelegationCertificate(
				t, testPayloadProtocolMagic, 7, issuerVK, issuerPrivate,
				delegateVK,
			),
		},
		byron.ByronUpdatePayload{
			Proposals: []byron.ByronUpdateProposal{
				signedUpdateProposal(
					t, testPayloadProtocolMagic, proposerVK, proposerPrivate,
				),
			},
			Votes: []any{
				signedUpdateVote(
					t,
					testPayloadProtocolMagic,
					voterVK,
					voterPrivate,
					bytes.Repeat([]byte{0x5a}, common.Blake2b256Size),
				),
			},
		},
	)
	require.NoError(t, block.ValidatePayloads())
}

func TestValidatePayloadsReportsOffendingIndex(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x11)
	delegateVK, _ := testKeyPair(0x22)
	good := signedDelegationCertificate(
		t, testPayloadProtocolMagic, 7, issuerVK, issuerPrivate, delegateVK,
	)

	block := testMainBlock(
		testPayloadProtocolMagic,
		[]any{good, []any{uint64(1)}},
		byron.ByronUpdatePayload{},
	)
	err := block.ValidateDelegationPayload()
	require.ErrorIs(t, err, byron.ErrInvalidPayload)
	require.Contains(t, err.Error(), "delegation certificate 1")
}

func TestValidatePayloadsWrongNetwork(t *testing.T) {
	issuerVK, issuerPrivate := testKeyPair(0x11)
	delegateVK, _ := testKeyPair(0x22)
	// The certificate is signed for one network; the block claims another.
	block := testMainBlock(
		testPayloadProtocolMagic+1,
		[]any{
			signedDelegationCertificate(
				t, testPayloadProtocolMagic, 7, issuerVK, issuerPrivate,
				delegateVK,
			),
		},
		byron.ByronUpdatePayload{},
	)
	require.ErrorIs(
		t,
		block.ValidateDelegationPayload(),
		byron.ErrInvalidSignature,
	)
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
