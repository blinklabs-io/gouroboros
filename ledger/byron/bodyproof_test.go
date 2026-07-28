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

// TestByronEpochBoundaryBlockBodyProofValidates checks a real testnet EBB
// against its own body hash. EBBs carry no transactions, so the whole body is
// covered by a single hash rather than a merkle root.
func TestByronEpochBoundaryBlockBodyProofValidates(t *testing.T) {
	ebbPath := filepath.Join(
		"..", "..", "protocol", "chainsync", "testdata",
		"byron_ebb_testnet_8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f.hex",
	)
	hexData, err := os.ReadFile(ebbPath)
	require.NoError(t, err)
	raw, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	require.NoError(t, err)

	block, err := byron.NewByronEpochBoundaryBlockFromCbor(raw)
	require.NoError(t, err)
	require.NoError(t, block.ValidateBodyProof())
}

// TestByronEpochBoundaryBlockRejectsSubstitutedBody covers the same
// substitution attack for epoch boundary blocks.
func TestByronEpochBoundaryBlockRejectsSubstitutedBody(t *testing.T) {
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

	// Replace the stakeholder list with an empty one.
	emptyBody, err := cbor.Encode([]any{})
	require.NoError(t, err)
	block[1] = emptyBody
	tampered, err := cbor.Encode(block)
	require.NoError(t, err)

	_, err = byron.NewByronEpochBoundaryBlockFromCbor(tampered)
	require.Error(t, err)
	assert.ErrorIs(t, err, byron.ErrBodyProofMismatch)
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
