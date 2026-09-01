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

// buildHashMismatchedCommitmentsBlock returns the raw CBOR of a mainnet
// fixture whose CommitmentsPayload (body) and ssc_proof (header) are both
// well-formed -- a genuine tag-258 set for both the commitments and VSS
// certificates fields -- but whose ssc_proof commitments hash does NOT
// match the real hash of the block's own commitments field. It also
// returns that mismatched hash's genuinely-real counterpart, so a caller
// can assert they differ.
func buildHashMismatchedCommitmentsBlock(
	t *testing.T,
) (tamperedCbor []byte, realHash common.Blake2b256) {
	t.Helper()

	pubkeyA := sscPubkey(0xa1)
	pubkeyB := sscPubkey(0xb2)
	comms := mustEncodeSet(t, sscCommEntry(pubkeyA, "commitment-mismatch-a"))
	certs := mustEncodeSet(t, sscCertEntry(pubkeyB, "cert-mismatch-b"))
	payload := encodeSscCommitmentsPayload(t, comms, certs)

	certState := byron.NewByronEpochSscState()
	require.NoError(
		t, certState.AccumulateBlock(decodeWithSscPayload(t, payload)),
	)

	realCommsHash := common.Blake2b256Hash(comms)
	wrongCommsHash := realCommsHash
	wrongCommsHash[0] ^= 0xff
	require.NotEqual(t, realCommsHash, wrongCommsHash)

	wrongProof, err := cbor.Encode([]any{
		uint64(byron.SscTypeCommitments),
		wrongCommsHash.Bytes(),
		certState.CertificatesHash().Bytes(),
	})
	require.NoError(t, err)

	tampered := withSscPayloadAndProof(
		t, mainnetByronBlock(t), payload, wrongProof,
	)
	return tampered, realCommsHash
}

// TestByronMainBlockDefaultLeniencyOnHashMismatch confirms that, under the
// default (zero-value) common.VerifyConfig, a structurally-valid ssc_proof
// whose commitments hash does not match the block's own real commitments
// hash still decodes successfully via NewByronMainBlockFromCbor and passes
// ValidateBodyProof with no config -- i.e. the lenient default genuinely
// tolerates a hash mismatch rather than only appearing to because no test
// ever exercised it. See common.VerifyConfig.EnableByronSscProofHashValidation
// and ValidateBodyProof's doc comment for why this leniency is intentional.
func TestByronMainBlockDefaultLeniencyOnHashMismatch(t *testing.T) {
	tampered, realHash := buildHashMismatchedCommitmentsBlock(t)

	block, err := byron.NewByronMainBlockFromCbor(tampered)
	require.NoError(
		t, err,
		"a hash-wrong but structurally-valid ssc_proof must still decode "+
			"under the default, structural-only check",
	)

	assert.NoError(t, block.ValidateBodyProof())

	// Sanity check that the proof this test built really is hash-wrong,
	// not accidentally correct.
	decodedProof, ok := block.BlockHeader.BodyProof.([]any)
	require.True(t, ok)
	sscProof, ok := decodedProof[1].([]any)
	require.True(t, ok)
	proofHash, ok := sscProof[1].([]byte)
	require.True(t, ok)
	assert.NotEqual(t, realHash.Bytes(), proofHash)
}

// TestByronMainBlockOptInRejectsHashMismatch confirms that the same
// hash-wrong ssc_proof from TestByronMainBlockDefaultLeniencyOnHashMismatch
// is rejected -- both at decode time and via an explicit ValidateBodyProof
// call -- once the caller opts in via
// common.VerifyConfig{EnableByronSscProofHashValidation: true}, proving the
// flag genuinely re-enables the full hash comparison rather than being
// inert.
func TestByronMainBlockOptInRejectsHashMismatch(t *testing.T) {
	tampered, _ := buildHashMismatchedCommitmentsBlock(t)

	optInCfg := common.VerifyConfig{EnableByronSscProofHashValidation: true}
	_, err := byron.NewByronMainBlockFromCbor(tampered, optInCfg)
	require.Error(
		t, err,
		"a hash-wrong ssc_proof must fail to decode once the opt-in hash "+
			"comparison is enabled",
	)

	block, err := byron.NewByronMainBlockFromCbor(
		tampered, common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	err = block.ValidateBodyProof(optInCfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)

	err = block.ValidateSscProof()
	require.Error(t, err)
	assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)
}

// TestByronMainBlockValidateSscProofShapeRejectsMalformedShape directly
// exercises ValidateSscProofShape (rather than going through the top-level
// ValidateBodyProof, as TestByronEpochSscStateRejectsUntaggedCommitmentsSet
// already does) to confirm it, alone, still rejects a structurally-invalid
// payload -- a plain, untagged CBOR array in place of the required tag-258
// commitments set -- under the default/no-config path. This pins down that
// the "shape is always checked" claim in ValidateBodyProof's and
// ValidateSscProofShape's doc comments is genuinely backed by a test that
// calls ValidateSscProofShape itself, not only indirectly through a
// higher-level function that might stop delegating to it in the future
// without any test noticing.
func TestByronMainBlockValidateSscProofShapeRejectsMalformedShape(
	t *testing.T,
) {
	pubkey := sscPubkey(0xc3)
	certPubkey := sscPubkey(0xd4)
	untaggedComms := mustEncodeUntaggedArray(
		t, sscCommEntry(pubkey, "commitment-shape-untagged"),
	)
	certs := mustEncodeSet(t, sscCertEntry(certPubkey, "cert-shape-d4"))
	payload := encodeSscCommitmentsPayload(t, untaggedComms, certs)

	// Placeholder proof: this test is about ValidateSscProofShape's
	// wire-shape check, which never compares hash values, so the proof's
	// actual hash bytes are irrelevant -- only its structural shape (type,
	// element count) needs to be internally consistent.
	placeholderProof, err := cbor.Encode([]any{
		uint64(byron.SscTypeCommitments),
		make([]byte, common.Blake2b256Size),
		make([]byte, common.Blake2b256Size),
	})
	require.NoError(t, err)

	tampered := withSscPayloadAndProof(
		t, mainnetByronBlock(t), payload, placeholderProof,
	)
	block, err := byron.NewByronMainBlockFromCbor(
		tampered, common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	err = block.ValidateSscProofShape()
	require.Error(
		t, err,
		"ValidateSscProofShape must reject an untagged commitments field",
	)
	assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)
}
