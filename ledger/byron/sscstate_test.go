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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sscStakeholder returns a distinct 28-byte stakeholder/address ID for
// OpeningsPayload/SharesPayload test fixtures, which really are CBOR maps
// keyed by such an ID -- see decodeStakeholderMap.
func sscStakeholder(b byte) common.Blake2b224 {
	id := common.Blake2b224{}
	for i := range id {
		id[i] = b
	}
	return id
}

// sscPubkey returns a distinct raw public key for CommitmentsPayload and
// VSS certificate test fixtures. Per the Byron CDDL, ssccomm/ssccert entries
// carry the contributor's raw public key rather than a pre-hashed
// stakeholder ID -- see decodeIdentitySet.
func sscPubkey(b byte) []byte {
	pk := make([]byte, 32)
	for i := range pk {
		pk[i] = b
	}
	return pk
}

// mustEncode is a small helper to build opaque CBOR entry bytes for SSC map
// values; the content is never interpreted, only hashed.
func mustEncode(t *testing.T, v any) cbor.RawMessage {
	t.Helper()
	b, err := cbor.Encode(v)
	require.NoError(t, err)
	return cbor.RawMessage(b)
}

// mustEncodeSet CBOR-encodes items as a tag-258 set, matching ssccomms/
// ssccerts in the CDDL (#6.258([* ssccomm]) / #6.258([* ssccert])).
func mustEncodeSet(t *testing.T, items ...[]any) cbor.RawMessage {
	t.Helper()
	set := make(cbor.Set, len(items))
	for i, item := range items {
		set[i] = item
	}
	b, err := cbor.Encode(set)
	require.NoError(t, err)
	return cbor.RawMessage(b)
}

// mustEncodeUntaggedArray CBOR-encodes items as a plain, untagged array --
// the shape decodeIdentitySet must reject for ssccomms/ssccerts, which are
// always tag-258 sets on the real Byron wire, never a bare array.
func mustEncodeUntaggedArray(t *testing.T, items ...[]any) cbor.RawMessage {
	t.Helper()
	untagged := make([]any, len(items))
	for i, item := range items {
		untagged[i] = item
	}
	b, err := cbor.Encode(untagged)
	require.NoError(t, err)
	return cbor.RawMessage(b)
}

// sscCommEntry builds an ssccomm entry: [pubkey, ..., signature]. Only the
// pubkey field (index 0) is ever interpreted by decodeIdentitySet; the rest
// only needs to be valid CBOR distinguishing one entry from another.
func sscCommEntry(pubkey []byte, tag string) []any {
	return []any{pubkey, tag + "-shares", tag + "-sig"}
}

// sscCertEntry builds an ssccert entry: [vsspubkey, epochid, signature,
// pubkey]. Only the pubkey field (index 3) is ever interpreted by
// decodeIdentitySet -- see certificatePubkeyFieldIndex's doc comment for
// why this field order, confirmed against real mainnet data, differs from
// cardano-ledger's own published (but apparently incorrect, for this one
// field) Byron CDDL comment.
func sscCertEntry(pubkey []byte, tag string) []any {
	return []any{tag + "-vsspubkey", uint64(0), tag + "-sig", pubkey}
}

// toStakeholderCborMap builds a real CBOR map keyed by 28-byte IDs, matching
// sscopens/sscshares in the CDDL.
func toStakeholderCborMap(
	entries map[common.Blake2b224]cbor.RawMessage,
) map[cbor.ByteString]cbor.RawMessage {
	out := make(map[cbor.ByteString]cbor.RawMessage, len(entries))
	for k, v := range entries {
		out[cbor.NewByteString(k.Bytes())] = v
	}
	return out
}

// encodeSscCommitmentsPayload builds the wire bytes of a Byron
// CommitmentsPayload SSC payload: [0, ssccomms, ssccerts], per the CDDL
// (ssccomms = #6.258([* ssccomm]), ssccerts = #6.258([* ssccert])).
func encodeSscCommitmentsPayload(
	t *testing.T,
	comms cbor.RawMessage,
	certs cbor.RawMessage,
) []byte {
	t.Helper()
	payload := []any{uint64(byron.SscTypeCommitments), comms, certs}
	b, err := cbor.Encode(payload)
	require.NoError(t, err)
	return b
}

// withSscPayloadAndProof returns a copy of a Byron main block's CBOR with its
// SSC payload (body element 1) and ssc_proof (header body-proof element 1)
// replaced, preserving every other component -- including the transaction,
// delegation, and update proofs -- byte for byte.
func withSscPayloadAndProof(
	t *testing.T,
	blockCbor []byte,
	sscPayload []byte,
	sscProof []byte,
) []byte {
	t.Helper()

	var block []cbor.RawMessage
	_, err := cbor.Decode(blockCbor, &block)
	require.NoError(t, err)
	require.Len(t, block, 3, "byron main block is [header, body, extra]")

	var header []cbor.RawMessage
	_, err = cbor.Decode(block[0], &header)
	require.NoError(t, err)
	require.Len(
		t, header, 5,
		"byron main block header is [magic, prevBlock, bodyProof, "+
			"consensusData, extraData]",
	)

	var bodyProof []cbor.RawMessage
	_, err = cbor.Decode(header[2], &bodyProof)
	require.NoError(t, err)
	require.Len(
		t, bodyProof, 4,
		"body proof is [txProof, sscProof, dlgProof, updProof]",
	)
	bodyProof[1] = sscProof
	newBodyProof, err := cbor.Encode(bodyProof)
	require.NoError(t, err)
	header[2] = newBodyProof

	newHeader, err := cbor.Encode(header)
	require.NoError(t, err)
	block[0] = newHeader

	var body []cbor.RawMessage
	_, err = cbor.Decode(block[1], &body)
	require.NoError(t, err)
	require.Len(
		t, body, 4,
		"body is [txPayload, sscPayload, dlgPayload, updPayload]",
	)
	body[1] = sscPayload
	newBody, err := cbor.Encode(body)
	require.NoError(t, err)
	block[1] = newBody

	tampered, err := cbor.Encode(block)
	require.NoError(t, err)
	return tampered
}

// decodeWithSscPayload decodes a mainnet Byron main block after replacing
// its SSC payload and ssc_proof with the given raw bytes. It uses a
// placeholder ssc_proof, and so decodes with decode-time body-proof
// validation (which now fully validates ssc_proof -- see
// ValidateBodyProof's doc comment) explicitly disabled, so callers that
// need a specific header proof value can patch it in afterwards with
// withSscPayloadAndProof and decode again with validation enabled.
func decodeWithSscPayload(
	t *testing.T,
	sscPayload []byte,
) *byron.ByronMainBlock {
	t.Helper()
	placeholderProof := mustEncode(t, uint64(0))
	tampered := withSscPayloadAndProof(
		t, mainnetByronBlock(t), sscPayload, placeholderProof,
	)
	block, err := byron.NewByronMainBlockFromCbor(
		tampered, common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	return block
}

// TestByronEpochSscStateAccumulatesRealMainnetBlock is the regression for
// the wire-shape bug this type shipped with initially: it decoded VSS
// certificate sets as CBOR maps, when the real, bundled mainnet fixture's
// own (empty) CertificatesPayload carries its VSS certificates as a CBOR
// tag-258 set, per cardano-ledger's Byron CDDL. Accumulating the real,
// completely unmodified fixture must succeed.
func TestByronEpochSscStateAccumulatesRealMainnetBlock(t *testing.T) {
	block, err := byron.NewByronMainBlockFromCbor(mainnetByronBlock(t))
	require.NoError(t, err)

	sscState := byron.NewByronEpochSscState()
	require.NoError(t, sscState.AccumulateBlock(block))
	assert.Empty(t, sscState.VssCertificates)
	assert.Empty(t, sscState.Commitments)
	assert.Empty(t, sscState.Openings)
	assert.Empty(t, sscState.Shares)
}

// TestByronEpochSscStateValidatesBlockLocalProof builds a single block's
// CommitmentsPayload contributed by two distinct stakeholders and confirms
// its ssc_proof, computed entirely from that block's own payload (see
// checkSscProofLocal's doc comment), validates via the block-local
// ValidateBodyProof. ByronEpochSscState is used here only to compute the
// expected VSS certificates hash for the synthetic proof this test builds
// -- it is not consulted by ValidateBodyProof itself (see
// ByronEpochSscState's doc comment).
func TestByronEpochSscStateValidatesBlockLocalProof(t *testing.T) {
	pubkeyA := sscPubkey(0xaa)
	pubkeyB := sscPubkey(0xbb)

	comms := mustEncodeSet(
		t,
		sscCommEntry(pubkeyA, "commitment-a"),
		sscCommEntry(pubkeyB, "commitment-b"),
	)
	certs := mustEncodeSet(
		t,
		sscCertEntry(pubkeyA, "cert-a"),
		sscCertEntry(pubkeyB, "cert-b"),
	)
	payload := encodeSscCommitmentsPayload(t, comms, certs)

	certState := byron.NewByronEpochSscState()
	require.NoError(
		t,
		certState.AccumulateBlock(decodeWithSscPayload(t, payload)),
	)
	require.Len(t, certState.VssCertificates, 2)

	realProof, err := cbor.Encode([]any{
		uint64(byron.SscTypeCommitments),
		common.Blake2b256Hash(comms).Bytes(),
		certState.CertificatesHash().Bytes(),
	})
	require.NoError(t, err)

	blockCbor := withSscPayloadAndProof(
		t, mainnetByronBlock(t), payload, realProof,
	)
	block, err := byron.NewByronMainBlockFromCbor(blockCbor)
	require.NoError(t, err)

	assert.NoError(t, block.ValidateBodyProof())
}

// TestByronEpochSscStateTamperingIsBlockLocal demonstrates that a real
// ssc_proof binds only to its own block's payload: tampering a *different*
// block's commitment has no way to reach this block's own proof at all --
// ValidateBodyProof takes no cross-block state as input in the first place
// -- while tampering *this* block's own commitment is still detected. This
// contrasts with the epoch-wide accumulation this package originally
// assumed ssc_proof required; see checkSscProofLocal's doc comment for the
// real mainnet vectors that disproved it.
func TestByronEpochSscStateTamperingIsBlockLocal(t *testing.T) {
	pubkeyB := sscPubkey(0xbb)

	comms := mustEncodeSet(t, sscCommEntry(pubkeyB, "commitment-b"))
	certs := mustEncodeSet(t, sscCertEntry(pubkeyB, "cert-b"))
	payload := encodeSscCommitmentsPayload(t, comms, certs)

	certState := byron.NewByronEpochSscState()
	require.NoError(
		t,
		certState.AccumulateBlock(decodeWithSscPayload(t, payload)),
	)

	realProof, err := cbor.Encode([]any{
		uint64(byron.SscTypeCommitments),
		common.Blake2b256Hash(comms).Bytes(),
		certState.CertificatesHash().Bytes(),
	})
	require.NoError(t, err)

	blockCbor := withSscPayloadAndProof(
		t, mainnetByronBlock(t), payload, realProof,
	)
	block, err := byron.NewByronMainBlockFromCbor(blockCbor)
	require.NoError(t, err)
	require.NoError(t, block.ValidateBodyProof())

	// Tampering *this* block's own commitment is detected: rebuild the same
	// block with an altered commitment entry but the same (now stale) real
	// proof.
	tamperedPayload := encodeSscCommitmentsPayload(
		t,
		mustEncodeSet(t, sscCommEntry(pubkeyB, "commitment-b-tampered")),
		certs,
	)
	tamperedBlockCbor := withSscPayloadAndProof(
		t, mainnetByronBlock(t), tamperedPayload, realProof,
	)
	tamperedBlock, err := byron.NewByronMainBlockFromCbor(
		tamperedBlockCbor, common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	err = tamperedBlock.ValidateBodyProof()
	require.Error(t, err)
	assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)
}

// TestByronEpochSscStateCertificatesOnly exercises the CertificatesProof
// shape, which carries a single hash rather than two, using the same
// tag-258 set encoding as the real mainnet fixture's own (empty)
// CertificatesPayload.
func TestByronEpochSscStateCertificatesOnly(t *testing.T) {
	pubkey := sscPubkey(0xcc)

	payload, err := cbor.Encode([]any{
		uint64(byron.SscTypeCertificates),
		mustEncodeSet(t, sscCertEntry(pubkey, "cert-c")),
	})
	require.NoError(t, err)

	block := decodeWithSscPayload(t, payload)

	sscState := byron.NewByronEpochSscState()
	require.NoError(t, sscState.AccumulateBlock(block))
	// The accumulator key is derived internally from the pubkey (see
	// stakeholderIDFromPubkeyCbor); this test doesn't assume any particular
	// derivation, only that accumulating one entry produces exactly one key.
	require.Len(t, sscState.VssCertificates, 1)

	realProof, err := cbor.Encode([]any{
		uint64(byron.SscTypeCertificates),
		sscState.CertificatesHash().Bytes(),
	})
	require.NoError(t, err)

	blockCbor := withSscPayloadAndProof(
		t, mainnetByronBlock(t), payload, realProof,
	)
	realBlock, err := byron.NewByronMainBlockFromCbor(blockCbor)
	require.NoError(t, err)

	assert.NoError(t, realBlock.ValidateBodyProof())

	// Tampering this block's own certificate entry, keeping the same (now
	// stale) real proof, is detected.
	tamperedPayload, err := cbor.Encode([]any{
		uint64(byron.SscTypeCertificates),
		mustEncodeSet(t, sscCertEntry(pubkey, "cert-c-tampered")),
	})
	require.NoError(t, err)
	tamperedBlockCbor := withSscPayloadAndProof(
		t, mainnetByronBlock(t), tamperedPayload, realProof,
	)
	tamperedBlock, err := byron.NewByronMainBlockFromCbor(
		tamperedBlockCbor, common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	err = tamperedBlock.ValidateBodyProof()
	require.Error(t, err)
	assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)

	// Tampering specifically the certificate's pubkey field (index 3 --
	// see certificatePubkeyFieldIndex's doc comment for why this field
	// order, not the CDDL comment's, is the real one) is also detected.
	// This is a regression check for the bug this package originally
	// shipped with: using field index 1 (matching the incorrect published
	// CDDL comment) instead of index 3 caused decodeIdentitySet to reject
	// every real, non-empty ssccert entry outright, so a test that only
	// tampers other fields could pass even with that bug present.
	tamperedPubkeyEntry := []any{
		"cert-c-vsspubkey", uint64(0), "cert-c-sig",
		sscPubkey(0xcc + 1),
	}
	tamperedPubkeyPayload, err := cbor.Encode([]any{
		uint64(byron.SscTypeCertificates),
		mustEncodeSet(t, tamperedPubkeyEntry),
	})
	require.NoError(t, err)
	tamperedPubkeyBlockCbor := withSscPayloadAndProof(
		t, mainnetByronBlock(t), tamperedPubkeyPayload, realProof,
	)
	tamperedPubkeyBlock, err := byron.NewByronMainBlockFromCbor(
		tamperedPubkeyBlockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	err = tamperedPubkeyBlock.ValidateBodyProof()
	require.Error(t, err)
	assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)
}

// TestByronEpochSscStateOpeningsAndShares exercises OpeningsPayload and
// SharesPayload, whose primary field really is a CBOR map keyed by a
// 28-byte ID (unlike commitments/certificates, which are tag-258 sets).
func TestByronEpochSscStateOpeningsAndShares(t *testing.T) {
	stakeholder := sscStakeholder(0xdd)

	openingsPayload, err := cbor.Encode([]any{
		uint64(byron.SscTypeOpenings),
		toStakeholderCborMap(map[common.Blake2b224]cbor.RawMessage{
			stakeholder: mustEncode(t, "opening-d"),
		}),
		mustEncodeSet(t, sscCertEntry(sscPubkey(0xee), "cert-e")),
	})
	require.NoError(t, err)

	block := decodeWithSscPayload(t, openingsPayload)

	sscState := byron.NewByronEpochSscState()
	require.NoError(t, sscState.AccumulateBlock(block))
	require.Contains(t, sscState.Openings, stakeholder)
	assert.NotEmpty(t, sscState.VssCertificates)

	sharesPayload, err := cbor.Encode([]any{
		uint64(byron.SscTypeShares),
		toStakeholderCborMap(map[common.Blake2b224]cbor.RawMessage{
			stakeholder: mustEncode(t, []any{
				stakeholder.Bytes(), []any{"share-1"},
			}),
		}),
		mustEncodeSet(t, sscCertEntry(sscPubkey(0xee), "cert-e")),
	})
	require.NoError(t, err)

	sharesBlock := decodeWithSscPayload(t, sharesPayload)
	require.NoError(t, sscState.AccumulateBlock(sharesBlock))
	require.Contains(t, sscState.Shares, stakeholder)
}

// TestByronEpochSscStateZeroValueAccumulateBlockIsSafe confirms that
// AccumulateBlock does not panic when called on a zero-value
// &byron.ByronEpochSscState{} rather than one built via
// byron.NewByronEpochSscState(): the exported maps must be lazily
// initialized by the merge, not assumed non-nil.
func TestByronEpochSscStateZeroValueAccumulateBlockIsSafe(t *testing.T) {
	pubkey := sscPubkey(0xff)

	payload, err := cbor.Encode([]any{
		uint64(byron.SscTypeCertificates),
		mustEncodeSet(t, sscCertEntry(pubkey, "cert-zero")),
	})
	require.NoError(t, err)

	block := decodeWithSscPayload(t, payload)

	sscState := &byron.ByronEpochSscState{}
	require.NotPanics(t, func() {
		require.NoError(t, sscState.AccumulateBlock(block))
	})
	require.Len(t, sscState.VssCertificates, 1)
}

// TestByronEpochSscStateCertificatesProofRejectsMalformedLength confirms
// that a CertificatesProof with more than its required 2 elements is
// rejected, matching the exact-length strictness other proof shapes
// already have.
func TestByronEpochSscStateCertificatesProofRejectsMalformedLength(
	t *testing.T,
) {
	pubkey := sscPubkey(0xcd)

	payload, err := cbor.Encode([]any{
		uint64(byron.SscTypeCertificates),
		mustEncodeSet(t, sscCertEntry(pubkey, "cert-malformed")),
	})
	require.NoError(t, err)

	block := decodeWithSscPayload(t, payload)

	sscState := byron.NewByronEpochSscState()
	require.NoError(t, sscState.AccumulateBlock(block))

	// A malformed 3-element CertificatesProof: [type, hash, extra].
	malformedProof, err := cbor.Encode([]any{
		uint64(byron.SscTypeCertificates),
		sscState.CertificatesHash().Bytes(),
		sscState.CertificatesHash().Bytes(),
	})
	require.NoError(t, err)

	blockCbor := withSscPayloadAndProof(
		t, mainnetByronBlock(t), payload, malformedProof,
	)
	malformedBlock, err := byron.NewByronMainBlockFromCbor(
		blockCbor, common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	err = malformedBlock.ValidateBodyProof()
	require.Error(t, err)
	assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)
}

// TestByronEpochSscStateRejectsMalformedPubkeyField confirms that a set
// entry whose public-key field is not a CBOR byte string is rejected by
// decodeIdentitySet rather than silently accepted and hashed into the
// accumulator as if it were a valid public key.
//
// The array-of-uints case exercises fxamacker/cbor's permissive
// decode-into-[]byte behavior directly: parseArrayToSlice happily converts a
// CBOR array of small (0-255) unsigned integers into a Go []byte, so a naive
// "does this field decode into []byte" check would accept a major-type-4
// array as if it were a major-type-2 byte string. The array-of-strings case
// is kept alongside it since it exercises the same "field is an array, not a
// byte string" shape via a different, incidental failure mode (each element
// fails to decode as a uint8).
func TestByronEpochSscStateRejectsMalformedPubkeyField(t *testing.T) {
	tests := []struct {
		name          string
		malformedFits []any
	}{
		{
			name:          "array of text strings",
			malformedFits: []any{"not", "a", "pubkey"},
		},
		{
			name:          "array of small unsigned integers",
			malformedFits: []any{uint64(1), uint64(2), uint64(3)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A malformed ssccert entry: [vsspubkey, epochid, signature,
			// pubkey], with the pubkey field (index 3) encoded as a
			// CBOR array instead of a byte string.
			malformedEntry := []any{
				"vsspubkey", uint64(0), "sig", tt.malformedFits,
			}

			payload, err := cbor.Encode([]any{
				uint64(byron.SscTypeCertificates),
				mustEncodeSet(t, malformedEntry),
			})
			require.NoError(t, err)

			block := decodeWithSscPayload(t, payload)

			sscState := byron.NewByronEpochSscState()
			err = sscState.AccumulateBlock(block)
			require.Error(t, err)
			assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)
			assert.Empty(t, sscState.VssCertificates)
		})
	}
}

// TestByronEpochSscStateRejectsEmptyPubkeyField confirms that a set entry
// whose public-key field is a genuine, well-formed CBOR byte string but has
// zero length (wire encoding 0x40) is rejected by decodeIdentitySet, rather
// than being accepted as a valid stakeholder public key. This is distinct
// from TestByronEpochSscStateRejectsMalformedPubkeyField: a zero-length byte
// string passes the major-type check there (it genuinely is major type 2),
// so it needs its own, explicit length check after decoding.
func TestByronEpochSscStateRejectsEmptyPubkeyField(t *testing.T) {
	// A malformed ssccert entry: [vsspubkey, epochid, signature, pubkey],
	// with the pubkey field (index 3) encoded as an empty CBOR byte
	// string ([]byte{} encodes to the wire bytes 0x40).
	malformedEntry := []any{
		"vsspubkey", uint64(0), "sig", []byte{},
	}

	payload, err := cbor.Encode([]any{
		uint64(byron.SscTypeCertificates),
		mustEncodeSet(t, malformedEntry),
	})
	require.NoError(t, err)

	block := decodeWithSscPayload(t, payload)

	sscState := byron.NewByronEpochSscState()
	err = sscState.AccumulateBlock(block)
	require.Error(t, err)
	assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)
	assert.Empty(t, sscState.VssCertificates)
}

// TestByronEpochSscStateRejectsProofPayloadTypeMismatch confirms that a
// header ssc_proof whose declared SSC type does not match the type of SSC
// payload the block's own body actually carries is rejected, rather than
// being checked against the wrong shape of accumulated state.
func TestByronEpochSscStateRejectsProofPayloadTypeMismatch(t *testing.T) {
	pubkey := sscPubkey(0xce)

	// The block's own body carries a CertificatesPayload.
	payload, err := cbor.Encode([]any{
		uint64(byron.SscTypeCertificates),
		mustEncodeSet(t, sscCertEntry(pubkey, "cert-mismatch")),
	})
	require.NoError(t, err)

	block := decodeWithSscPayload(t, payload)

	sscState := byron.NewByronEpochSscState()
	require.NoError(t, sscState.AccumulateBlock(block))

	// The header's ssc_proof structurally claims CommitmentsProof (type 0),
	// a different SSC type than the body's own CertificatesPayload (type
	// 3).
	mismatchedProof, err := cbor.Encode([]any{
		uint64(byron.SscTypeCommitments),
		sscState.CommitmentsHash().Bytes(),
		sscState.CertificatesHash().Bytes(),
	})
	require.NoError(t, err)

	blockCbor := withSscPayloadAndProof(
		t, mainnetByronBlock(t), payload, mismatchedProof,
	)
	mismatchedBlock, err := byron.NewByronMainBlockFromCbor(
		blockCbor, common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	err = mismatchedBlock.ValidateBodyProof()
	require.Error(t, err)
	assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)
}

// TestByronEpochSscStateRejectsUntaggedCertificateSet confirms that a
// CertificatesPayload whose VSS certificate field is a plain, untagged CBOR
// array -- rather than the required tag-258 set (ssccerts =
// #6.258([* ssccert]) per the CDDL) -- is rejected by ValidateBodyProof, even
// when its ssc_proof is recomputed to match that malformed array exactly.
//
// This is a regression for a real gap: decodeIdentitySet decoded via
// cbor.SetType, which deliberately also accepts a plain untagged array (see
// its own doc comment in cbor/tags.go, added for pre-Dijkstra callers of
// that generic type) -- so a malformed body using an untagged array would
// previously decode, hash, and validate successfully here, exactly as if it
// were the well-formed tag-258 set the wire format actually requires.
func TestByronEpochSscStateRejectsUntaggedCertificateSet(t *testing.T) {
	pubkey := sscPubkey(0xf0)
	untaggedCerts := mustEncodeUntaggedArray(
		t, sscCertEntry(pubkey, "cert-untagged"),
	)

	payload, err := cbor.Encode([]any{
		uint64(byron.SscTypeCertificates),
		untaggedCerts,
	})
	require.NoError(t, err)

	// A proof recomputed directly over the malformed (untagged) array, as an
	// attacker who controls both body and header would do, rather than the
	// canonical rebuilt-map hash a well-formed tag-258 set would need.
	forgedProof, err := cbor.Encode([]any{
		uint64(byron.SscTypeCertificates),
		common.Blake2b256Hash(untaggedCerts).Bytes(),
	})
	require.NoError(t, err)

	blockCbor := withSscPayloadAndProof(
		t, mainnetByronBlock(t), payload, forgedProof,
	)
	block, err := byron.NewByronMainBlockFromCbor(
		blockCbor, common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	err = block.ValidateBodyProof()
	require.Error(t, err)
	assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)

	// The same malformed shape is also rejected by AccumulateBlock, which
	// shares decodeIdentitySet with the proof-validation path.
	sscState := byron.NewByronEpochSscState()
	err = sscState.AccumulateBlock(block)
	require.Error(t, err)
	assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)
}

// TestByronEpochSscStateRejectsUntaggedCommitmentsSet confirms the same
// tag-258 enforcement applies to CommitmentsPayload's commitments field
// (ssccomms = #6.258([* ssccomm]) per the CDDL), not just VSS certificates:
// decodeIdentitySet is the shared decode point for both fields, so a fix
// scoped there covers commitments and certificates alike rather than
// needing a duplicated per-caller check.
func TestByronEpochSscStateRejectsUntaggedCommitmentsSet(t *testing.T) {
	pubkey := sscPubkey(0xf1)
	untaggedComms := mustEncodeUntaggedArray(
		t, sscCommEntry(pubkey, "commitment-untagged"),
	)
	certs := mustEncodeSet(t, sscCertEntry(sscPubkey(0xf2), "cert-f2"))

	payload := encodeSscCommitmentsPayload(t, untaggedComms, certs)
	block := decodeWithSscPayload(t, payload)

	sscState := byron.NewByronEpochSscState()
	err := sscState.AccumulateBlock(block)
	require.Error(t, err)
	assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)
	assert.Empty(t, sscState.Commitments)
}
