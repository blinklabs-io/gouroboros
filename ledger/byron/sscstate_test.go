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

// sscCommEntry builds an ssccomm entry: [pubkey, ..., signature]. Only the
// pubkey field (index 0) is ever interpreted by decodeIdentitySet; the rest
// only needs to be valid CBOR distinguishing one entry from another.
func sscCommEntry(pubkey []byte, tag string) []any {
	return []any{pubkey, tag + "-shares", tag + "-sig"}
}

// sscCertEntry builds an ssccert entry: [vsspubkey, pubkey, epochid,
// signature]. Only the pubkey field (index 1) is ever interpreted by
// decodeIdentitySet.
func sscCertEntry(pubkey []byte, tag string) []any {
	return []any{tag + "-vsspubkey", pubkey, uint64(0), tag + "-sig"}
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
// placeholder ssc_proof to get past decode-time structural validation
// (which never inspects that entry -- see ValidateBodyProof's NOTE), so
// callers that need a specific header proof value must patch it in
// afterwards with withSscPayloadAndProof.
func decodeWithSscPayload(
	t *testing.T,
	sscPayload []byte,
) *byron.ByronMainBlock {
	t.Helper()
	placeholderProof := mustEncode(t, uint64(0))
	tampered := withSscPayloadAndProof(
		t, mainnetByronBlock(t), sscPayload, placeholderProof,
	)
	block, err := byron.NewByronMainBlockFromCbor(tampered)
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

// TestByronEpochSscStateValidatesAccumulatedProof builds a small two-block
// mock epoch: an earlier block contributing stakeholder A's commitment and
// VSS certificate, and a later block additionally contributing stakeholder
// B's. The later block's ssc_proof is computed over the state accumulated
// through both blocks, matching how a real ssc_proof depends on the whole
// epoch rather than a single block.
func TestByronEpochSscStateValidatesAccumulatedProof(t *testing.T) {
	pubkeyA := sscPubkey(0xaa)
	pubkeyB := sscPubkey(0xbb)

	prevPayload := encodeSscCommitmentsPayload(
		t,
		mustEncodeSet(t, sscCommEntry(pubkeyA, "commitment-a")),
		mustEncodeSet(t, sscCertEntry(pubkeyA, "cert-a")),
	)
	currPayload := encodeSscCommitmentsPayload(
		t,
		mustEncodeSet(t, sscCommEntry(pubkeyB, "commitment-b")),
		mustEncodeSet(t, sscCertEntry(pubkeyB, "cert-b")),
	)

	prevBlock := decodeWithSscPayload(t, prevPayload)
	currBlockPlaceholder := decodeWithSscPayload(t, currPayload)

	// Accumulate both blocks of the mock epoch, in order, to learn the
	// hashes the later block's real ssc_proof must carry.
	expectedState := byron.NewByronEpochSscState()
	require.NoError(t, expectedState.AccumulateBlock(prevBlock))
	require.NoError(t, expectedState.AccumulateBlock(currBlockPlaceholder))

	realProof, err := cbor.Encode([]any{
		uint64(byron.SscTypeCommitments),
		expectedState.CommitmentsHash().Bytes(),
		expectedState.CertificatesHash().Bytes(),
	})
	require.NoError(t, err)

	currBlockCbor := withSscPayloadAndProof(
		t, mainnetByronBlock(t), currPayload, realProof,
	)
	currBlock, err := byron.NewByronMainBlockFromCbor(currBlockCbor)
	require.NoError(t, err)

	// A fresh accumulation, folding the same two blocks in order, must
	// validate the later block's ssc_proof.
	sscState := byron.NewByronEpochSscState()
	require.NoError(t, sscState.AccumulateBlock(prevBlock))
	require.NoError(t, sscState.AccumulateBlock(currBlock))
	assert.NoError(t, currBlock.ValidateBodyProofWithSscState(sscState))

	// The block-local structural validator alone must still pass: it never
	// looks at ssc_proof's hashes.
	assert.NoError(t, currBlock.ValidateBodyProof())
}

// TestByronEpochSscStateRejectsTamperedEarlierCommitment demonstrates the
// epoch-wide dependency the issue describes: tampering with an *earlier*
// block's contribution is only detectable by a *later* block's ssc_proof,
// because that proof is computed over state accumulated across the epoch,
// not over the later block's own payload alone.
func TestByronEpochSscStateRejectsTamperedEarlierCommitment(t *testing.T) {
	pubkeyA := sscPubkey(0xaa)
	pubkeyB := sscPubkey(0xbb)

	prevPayload := encodeSscCommitmentsPayload(
		t,
		mustEncodeSet(t, sscCommEntry(pubkeyA, "commitment-a")),
		mustEncodeSet(t, sscCertEntry(pubkeyA, "cert-a")),
	)
	currPayload := encodeSscCommitmentsPayload(
		t,
		mustEncodeSet(t, sscCommEntry(pubkeyB, "commitment-b")),
		mustEncodeSet(t, sscCertEntry(pubkeyB, "cert-b")),
	)

	prevBlock := decodeWithSscPayload(t, prevPayload)
	currBlockPlaceholder := decodeWithSscPayload(t, currPayload)

	expectedState := byron.NewByronEpochSscState()
	require.NoError(t, expectedState.AccumulateBlock(prevBlock))
	require.NoError(t, expectedState.AccumulateBlock(currBlockPlaceholder))

	realProof, err := cbor.Encode([]any{
		uint64(byron.SscTypeCommitments),
		expectedState.CommitmentsHash().Bytes(),
		expectedState.CertificatesHash().Bytes(),
	})
	require.NoError(t, err)

	currBlockCbor := withSscPayloadAndProof(
		t, mainnetByronBlock(t), currPayload, realProof,
	)
	currBlock, err := byron.NewByronMainBlockFromCbor(currBlockCbor)
	require.NoError(t, err)

	// Rebuild the earlier block with a different commitment for the same
	// contributor -- a substitution confined entirely to a block earlier in
	// the epoch than the one we are about to validate.
	tamperedPrevPayload := encodeSscCommitmentsPayload(
		t,
		mustEncodeSet(t, sscCommEntry(pubkeyA, "commitment-a-tampered")),
		mustEncodeSet(t, sscCertEntry(pubkeyA, "cert-a")),
	)
	tamperedPrevBlock := decodeWithSscPayload(t, tamperedPrevPayload)

	sscState := byron.NewByronEpochSscState()
	require.NoError(t, sscState.AccumulateBlock(tamperedPrevBlock))
	require.NoError(t, sscState.AccumulateBlock(currBlock))

	err = currBlock.ValidateBodyProofWithSscState(sscState)
	require.Error(t, err)
	assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)

	// The block-local structural validator cannot see this: the tampering
	// is entirely in a different, earlier block.
	assert.NoError(t, currBlock.ValidateBodyProof())
}

// TestByronEpochSscStateRequiresState confirms the stateful validator
// refuses to silently skip the ssc_proof check when no state is supplied.
func TestByronEpochSscStateRequiresState(t *testing.T) {
	block, err := byron.NewByronMainBlockFromCbor(mainnetByronBlock(t))
	require.NoError(t, err)

	err = block.ValidateBodyProofWithSscState(nil)
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
	// derivation, only that accumulating one entry produces exactly one
	// key, which is what gets tampered with below.
	require.Len(t, sscState.VssCertificates, 1)
	var certKey common.Blake2b224
	for k := range sscState.VssCertificates {
		certKey = k
	}

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

	freshState := byron.NewByronEpochSscState()
	require.NoError(t, freshState.AccumulateBlock(realBlock))
	assert.NoError(t, realBlock.ValidateBodyProofWithSscState(freshState))

	// Tamper the accumulated certificate directly and confirm detection.
	freshState.VssCertificates[certKey] = []byte(
		mustEncode(t, "cert-c-tampered"),
	)
	err = realBlock.ValidateBodyProofWithSscState(freshState)
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
	malformedBlock, err := byron.NewByronMainBlockFromCbor(blockCbor)
	require.NoError(t, err)

	err = malformedBlock.ValidateBodyProofWithSscState(sscState)
	require.Error(t, err)
	assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)
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
	mismatchedBlock, err := byron.NewByronMainBlockFromCbor(blockCbor)
	require.NoError(t, err)

	err = mismatchedBlock.ValidateBodyProofWithSscState(sscState)
	require.Error(t, err)
	assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)
}
