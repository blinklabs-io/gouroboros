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
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mainnetByronBlock returns the CBOR of the bundled mainnet Byron main block,
// which carries two transactions and therefore exercises merkle branch
// combination rather than only the single-leaf case.
func mainnetByronBlock(t *testing.T) []byte {
	t.Helper()
	raw, err := hex.DecodeString(strings.TrimSpace(testdata.ByronBlockHex))
	require.NoError(t, err)
	return raw
}

// withTxPayload re-encodes a Byron main block with its transaction payload
// replaced, preserving every other component byte-for-byte. This models a
// hostile archive returning a genuine header with a substituted body.
func withTxPayload(t *testing.T, blockCbor []byte, txPayload []any) []byte {
	t.Helper()
	var block []cbor.RawMessage
	_, err := cbor.Decode(blockCbor, &block)
	require.NoError(t, err)
	require.Len(t, block, 3, "byron main block is [header, body, extra]")

	var body []cbor.RawMessage
	_, err = cbor.Decode(block[1], &body)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(body), 4)

	newTxPayload, err := cbor.Encode(txPayload)
	require.NoError(t, err)
	body[0] = newTxPayload

	newBody, err := cbor.Encode(body)
	require.NoError(t, err)
	block[1] = newBody

	tampered, err := cbor.Encode(block)
	require.NoError(t, err)
	return tampered
}

// TestByronMainBlockBodyProofValidates checks the real mainnet block against
// its own body proof, so the recomputation is pinned to a block produced by
// the reference implementation rather than to our own encoder.
func TestByronMainBlockBodyProofValidates(t *testing.T) {
	block, err := byron.NewByronMainBlockFromCbor(mainnetByronBlock(t))
	require.NoError(t, err)
	require.Len(t, block.Body.TxPayload, 2,
		"fixture must carry two transactions to exercise a merkle branch")
	require.NoError(t, block.ValidateBodyProof())
}

// TestByronMainBlockRejectsSubstitutedBody is the regression for a hostile
// archive returning the requested header with a different body. Decoding must
// fail rather than hand back a block whose contents were never checked.
func TestByronMainBlockRejectsSubstitutedBody(t *testing.T) {
	original := mainnetByronBlock(t)

	tests := []struct {
		name      string
		txPayload []any
	}{
		{name: "transactions removed", txPayload: []any{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tampered := withTxPayload(t, original, tc.txPayload)
			require.NotEqual(t, len(original), len(tampered),
				"tampering must actually change the encoding")

			_, err := byron.NewByronMainBlockFromCbor(tampered)
			require.Error(t, err,
				"a substituted body must not decode successfully")
			assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)
		})
	}
}

// TestByronMainBlockSkipBodyHashValidation confirms the escape hatch used by
// callers that already validated the bytes upstream, matching how the
// Shelley-and-later constructors treat the same option.
func TestByronMainBlockSkipBodyHashValidation(t *testing.T) {
	tampered := withTxPayload(t, mainnetByronBlock(t), []any{})

	_, err := byron.NewByronMainBlockFromCbor(
		tampered,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err,
		"skipping validation must still decode a structurally valid block")
}

// testnetByronEbb returns the CBOR of the bundled testnet Byron epoch
// boundary block.
func testnetByronEbb(t *testing.T) []byte {
	t.Helper()
	ebbPath := filepath.Join(
		"..", "..", "protocol", "chainsync", "testdata",
		"byron_ebb_testnet_8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f.hex",
	)
	hexData, err := os.ReadFile(ebbPath)
	require.NoError(t, err)
	raw, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	require.NoError(t, err)
	return raw
}

// withEbbBodyProof re-encodes an EBB with its header body-proof field
// replaced by an arbitrary raw CBOR value, preserving every other component
// byte-for-byte. This models the header field the reference decoder decodes
// but never interprets or compares against the body.
func withEbbBodyProof(
	t *testing.T,
	blockCbor []byte,
	newProof cbor.RawMessage,
) []byte {
	t.Helper()
	var block []cbor.RawMessage
	_, err := cbor.Decode(blockCbor, &block)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(block), 1, "byron EBB is [header, body, extra]")

	var header []cbor.RawMessage
	_, err = cbor.Decode(block[0], &header)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(header), 3,
		"byron EBB header is [protocol_magic, prev_block, body_proof, consensus_data, extra_data]")

	header[2] = newProof
	newHeader, err := cbor.Encode(header)
	require.NoError(t, err)
	block[0] = newHeader

	tampered, err := cbor.Encode(block)
	require.NoError(t, err)
	return tampered
}

// TestByronEpochBoundaryBlockBodyProofValidates checks a real testnet EBB
// decodes and validates against its own header body-proof field.
func TestByronEpochBoundaryBlockBodyProofValidates(t *testing.T) {
	block, err := byron.NewByronEpochBoundaryBlockFromCbor(testnetByronEbb(t))
	require.NoError(t, err)
	require.NoError(t, block.ValidateBodyProof())
}

// TestByronEpochBoundaryBlockAcceptsSubstitutedStakeholderList is the
// reference-parity regression for #2341, from the body side rather than the
// header's proof field: the reference never binds an EBB's body-proof to
// its body, so replacing the stakeholder list outright must still decode
// and validate, provided the replacement is otherwise well-formed (the
// indefinite-length form #2493 requires).
func TestByronEpochBoundaryBlockAcceptsSubstitutedStakeholderList(t *testing.T) {
	ebbPath := filepath.Join(
		"..", "..", "protocol", "chainsync", "testdata",
		"byron_ebb_testnet_8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f.hex",
	)
	hexData, err := os.ReadFile(ebbPath)
	require.NoError(t, err)
	original, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	require.NoError(t, err)

	var block []cbor.RawMessage
	_, err = cbor.Decode(original, &block)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(block), 2)

	// Replace the stakeholder list with an empty one, using the
	// indefinite-length form #2493 requires so this substitution survives
	// structural decode and exercises body-proof handling specifically.
	block[1] = cbor.RawMessage{0x9f, 0xff}
	tampered, err := cbor.Encode(block)
	require.NoError(t, err)

	tamperedBlock, err := byron.NewByronEpochBoundaryBlockFromCbor(tampered)
	require.NoError(t, err)
	require.NoError(t, tamperedBlock.ValidateBodyProof())
}

// TestByronEpochBoundaryBlockAcceptsShortProof is the reference-parity
// regression for #2341: the Byron reference decodes an EBB's body-proof
// field as a byte string with no length restriction, so a proof shorter
// than the usual 32-byte hash must still decode and validate.
func TestByronEpochBoundaryBlockAcceptsShortProof(t *testing.T) {
	shortProof := bytes.Repeat([]byte{0xAB}, 31)
	encodedProof, err := cbor.Encode(shortProof)
	require.NoError(t, err)

	tampered := withEbbBodyProof(t, testnetByronEbb(t), encodedProof)
	block, err := byron.NewByronEpochBoundaryBlockFromCbor(tampered)
	require.NoError(t, err)
	require.NoError(t, block.ValidateBodyProof())
}

// TestByronEpochBoundaryBlockAcceptsWrongValueProof is the reference-parity
// regression for #2341: the reference never compares the EBB body-proof
// value against the body, so a 32-byte proof that does not match
// blake2b256(bodyCbor) must still decode and validate.
func TestByronEpochBoundaryBlockAcceptsWrongValueProof(t *testing.T) {
	genuine := testnetByronEbb(t)
	genuineBlock, err := byron.NewByronEpochBoundaryBlockFromCbor(genuine)
	require.NoError(t, err)
	genuineProof, ok := genuineBlock.BlockHeader.BodyProof.([]byte)
	require.True(t, ok)

	wrongProof := bytes.Clone(genuineProof)
	wrongProof[0] ^= 0xFF
	require.NotEqual(t, genuineProof, wrongProof)
	encodedProof, err := cbor.Encode(wrongProof)
	require.NoError(t, err)

	tampered := withEbbBodyProof(t, genuine, encodedProof)
	block, err := byron.NewByronEpochBoundaryBlockFromCbor(tampered)
	require.NoError(t, err)
	require.NoError(t, block.ValidateBodyProof())
}

// TestByronEpochBoundaryBlockRejectsNonByteStringProof covers the one shape
// the reference does reject: a body-proof field that is not a byte string at
// all.
func TestByronEpochBoundaryBlockRejectsNonByteStringProof(t *testing.T) {
	encodedProof, err := cbor.Encode(uint64(1234))
	require.NoError(t, err)

	tampered := withEbbBodyProof(t, testnetByronEbb(t), encodedProof)
	_, err = byron.NewByronEpochBoundaryBlockFromCbor(tampered)
	require.Error(t, err)
	assert.ErrorIs(t, err, byron.ErrMalformedBodyProof)
}

// TestByronMainBlockHeaderUnchangedByTampering documents why the body proof is
// needed at all: the header, and therefore the block hash and slot, survive a
// body substitution untouched.
func TestByronMainBlockHeaderUnchangedByTampering(t *testing.T) {
	original := mainnetByronBlock(t)
	tampered := withTxPayload(t, original, []any{})

	genuine, err := byron.NewByronMainBlockFromCbor(original)
	require.NoError(t, err)
	substituted, err := byron.NewByronMainBlockFromCbor(
		tampered,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	assert.Equal(t, genuine.Hash(), substituted.Hash(),
		"block hash is derived from the header and cannot detect this")
	assert.Equal(t, genuine.SlotNumber(), substituted.SlotNumber())
	assert.NotEqual(t, len(genuine.Cbor()), len(substituted.Cbor()),
		"the bytes genuinely differ")
}
