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

	newTxPayload, err := cbor.Encode(cbor.IndefLengthList(txPayload))
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

// encodeIndefiniteWitnessList mirrors bodyproof.go's private
// encodeWitnessList: the witnesses_hash Byron carries in the header is
// computed over an indefinite-length array assembled from each
// transaction's preserved raw witness-list bytes, not a re-encoded
// structure. Reproducing the same framing here lets this test recompute a
// witnesses_hash that genuinely matches tampered bytes, rather than only
// observing a mismatch.
func encodeIndefiniteWitnessList(parts [][]byte) []byte {
	const (
		indefiniteArrayStart byte = 0x9f
		indefiniteBreak      byte = 0xff
	)
	size := 2
	for _, p := range parts {
		size += len(p)
	}
	out := make([]byte, 0, size)
	out = append(out, indefiniteArrayStart)
	for _, p := range parts {
		out = append(out, p...)
	}
	return append(out, indefiniteBreak)
}

// TestByronMainBlockRejectsMalformedExtraWitnessDespiteMatchingProof is the
// end-to-end scenario the wire-shape divergence in issue #4384 (dingo)
// describes: a transaction carries one genuine, required witness plus a
// malformed extra witness entry appended after it. The header's
// witnesses_hash is recomputed here with the exact same algorithm the
// library uses, so it matches the tampered bytes precisely -- an attacker
// who controls both the body and the header could produce exactly this, and
// the body proof alone is fully self-consistent. Decoding the block must
// still reject it: the witness proof only proves inclusion of the raw bytes,
// not that every witness they contain decodes as a valid TxInWitness.
func TestByronMainBlockRejectsMalformedExtraWitnessDespiteMatchingProof(
	t *testing.T,
) {
	original := mainnetByronBlock(t)

	var block []cbor.RawMessage
	_, err := cbor.Decode(original, &block)
	require.NoError(t, err)
	require.Len(t, block, 3, "byron main block is [header, body, extra]")

	var header []cbor.RawMessage
	_, err = cbor.Decode(block[0], &header)
	require.NoError(t, err)
	require.Len(t, header, 5, "byron main block header has 5 fields")

	var bodyProof []cbor.RawMessage
	_, err = cbor.Decode(header[2], &bodyProof)
	require.NoError(t, err)
	require.Len(t, bodyProof, 4,
		"body proof is [tx_proof, ssc_proof, dlg_proof, upd_proof]")

	var txProof []cbor.RawMessage
	_, err = cbor.Decode(bodyProof[0], &txProof)
	require.NoError(t, err)
	require.Len(t, txProof, 3,
		"tx proof is [tx_count, tx_merkle_root, witnesses_hash]")

	var body []cbor.RawMessage
	_, err = cbor.Decode(block[1], &body)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(body), 4)

	var txPayload []cbor.RawMessage
	_, err = cbor.Decode(body[0], &txPayload)
	require.NoError(t, err)
	require.Len(t, txPayload, 2, "fixture must carry two transactions")

	var tx0 []cbor.RawMessage
	_, err = cbor.Decode(txPayload[0], &tx0)
	require.NoError(t, err)
	require.Len(t, tx0, 2, "byron transaction is [body, witnesses]")

	var tx1 []cbor.RawMessage
	_, err = cbor.Decode(txPayload[1], &tx1)
	require.NoError(t, err)
	require.Len(t, tx1, 2)

	originalTwit0 := []byte(tx0[1])
	originalTwit1 := []byte(tx1[1])

	var originalWitnesses0 []cbor.RawMessage
	_, err = cbor.Decode(originalTwit0, &originalWitnesses0)
	require.NoError(t, err)
	require.NotEmpty(t, originalWitnesses0)

	// Sanity check: reproducing the real algorithm over the untouched bytes
	// must reproduce the real header's witnesses_hash exactly, so the
	// tampered value computed the same way below is genuinely correct
	// rather than coincidentally passing.
	var originalHash []byte
	_, err = cbor.Decode(txProof[2], &originalHash)
	require.NoError(t, err)
	require.Equal(
		t,
		originalHash,
		func() []byte {
			h := common.Blake2b256Hash(
				encodeIndefiniteWitnessList([][]byte{originalTwit0, originalTwit1}),
			)
			return h[:]
		}(),
		"encodeIndefiniteWitnessList must reproduce the real witnesses_hash",
	)

	// A malformed extra witness: constructor 1 (ScriptWitness) is defined by
	// the reference sum type but has no reachable decoder on any real
	// chain, so it must be rejected exactly like any other unrecognized
	// constructor.
	innerFields, err := cbor.Encode(
		[]any{[]byte{1, 2, 3, 4}, []byte{5, 6, 7, 8}},
	)
	require.NoError(t, err)
	malformedWitness, err := cbor.Encode(
		[]any{uint64(1), cbor.WrappedCbor(innerFields)},
	)
	require.NoError(t, err)

	newWitnesses0 := append(
		append([]cbor.RawMessage{}, originalWitnesses0...),
		cbor.RawMessage(malformedWitness),
	)
	newTwit0, err := cbor.Encode(newWitnesses0)
	require.NoError(t, err)

	tx0[1] = cbor.RawMessage(newTwit0)
	newTx0, err := cbor.Encode(tx0)
	require.NoError(t, err)
	txPayload[0] = cbor.RawMessage(newTx0)

	newWitnessesHash := common.Blake2b256Hash(
		encodeIndefiniteWitnessList([][]byte{newTwit0, originalTwit1}),
	)
	newHashCbor, err := cbor.Encode(newWitnessesHash[:])
	require.NoError(t, err)
	txProof[2] = cbor.RawMessage(newHashCbor)

	newTxProof, err := cbor.Encode(txProof)
	require.NoError(t, err)
	bodyProof[0] = cbor.RawMessage(newTxProof)
	newBodyProof, err := cbor.Encode(bodyProof)
	require.NoError(t, err)
	header[2] = cbor.RawMessage(newBodyProof)
	newHeader, err := cbor.Encode(header)
	require.NoError(t, err)
	block[0] = cbor.RawMessage(newHeader)

	txPayloadAny := make([]any, len(txPayload))
	for i := range txPayload {
		txPayloadAny[i] = txPayload[i]
	}
	newTxPayload, err := cbor.Encode(cbor.IndefLengthList(txPayloadAny))
	require.NoError(t, err)
	body[0] = cbor.RawMessage(newTxPayload)
	newBody, err := cbor.Encode(body)
	require.NoError(t, err)
	block[1] = cbor.RawMessage(newBody)

	tampered, err := cbor.Encode(block)
	require.NoError(t, err)
	require.NotEqual(t, len(original), len(tampered))

	_, err = byron.NewByronMainBlockFromCbor(tampered)
	require.Error(t, err,
		"a malformed extra witness must reject the block even though its "+
			"witnesses_hash was recomputed to match exactly")
	assert.ErrorContains(t, err, "TxInWitness")
}

func TestByronMainBlockRejectsMalformedUpdateVote(t *testing.T) {
	var blockFields []cbor.RawMessage
	_, err := cbor.Decode(mainnetByronBlock(t), &blockFields)
	require.NoError(t, err)
	if blockFields == nil {
		t.Fatal("expected Byron block fields")
	}
	var bodyFields []cbor.RawMessage
	_, err = cbor.Decode(blockFields[1], &bodyFields)
	require.NoError(t, err)
	if bodyFields == nil {
		t.Fatal("expected Byron block body fields")
	}
	var updateFields []cbor.RawMessage
	_, err = cbor.Decode(bodyFields[3], &updateFields)
	require.NoError(t, err)
	if updateFields == nil {
		t.Fatal("expected Byron update fields")
	}
	updateFields[1] = cbor.RawMessage{0x9f, 0x00, 0xff}
	bodyFields[3], err = cbor.Encode(updateFields)
	require.NoError(t, err)
	blockFields[1], err = cbor.Encode(bodyFields)
	require.NoError(t, err)
	mutatedBlock, err := cbor.Encode(blockFields)
	require.NoError(t, err)

	var decoded byron.ByronMainBlock
	_, err = cbor.Decode(mutatedBlock, &decoded)
	require.ErrorContains(t, err, "update vote 0")
}

func TestByronMainBlockRejectsMalformedUpdateProposal(t *testing.T) {
	var blockFields []cbor.RawMessage
	_, err := cbor.Decode(mainnetByronBlock(t), &blockFields)
	require.NoError(t, err)
	if blockFields == nil {
		t.Fatal("expected Byron block fields")
	}
	var bodyFields []cbor.RawMessage
	_, err = cbor.Decode(blockFields[1], &bodyFields)
	require.NoError(t, err)
	if bodyFields == nil {
		t.Fatal("expected Byron block body fields")
	}
	var updateFields []cbor.RawMessage
	_, err = cbor.Decode(bodyFields[3], &updateFields)
	require.NoError(t, err)
	if updateFields == nil {
		t.Fatal("expected Byron update fields")
	}
	updateFields[0] = cbor.RawMessage{0x81, 0x00}
	bodyFields[3], err = cbor.Encode(updateFields)
	require.NoError(t, err)
	blockFields[1], err = cbor.Encode(bodyFields)
	require.NoError(t, err)
	mutatedBlock, err := cbor.Encode(blockFields)
	require.NoError(t, err)

	var decoded byron.ByronMainBlock
	_, err = cbor.Decode(mutatedBlock, &decoded)
	require.Error(t, err)
}

func TestByronMainBlockBodyRequiresIndefiniteLegacyPayloadLists(t *testing.T) {
	for _, tc := range []struct {
		name      string
		listIndex int
		want      string
	}{
		{name: "delegation certificates", listIndex: 2, want: "delegation certificates must use indefinite-list framing"},
		{name: "update votes", listIndex: 3, want: "update votes must use indefinite-list framing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var blockFields []cbor.RawMessage
			_, err := cbor.Decode(mainnetByronBlock(t), &blockFields)
			require.NoError(t, err)
			if blockFields == nil {
				t.Fatal("expected Byron block fields")
			}
			var bodyFields []cbor.RawMessage
			_, err = cbor.Decode(blockFields[1], &bodyFields)
			require.NoError(t, err)
			if bodyFields == nil {
				t.Fatal("expected Byron block body fields")
			}
			if tc.listIndex == 3 {
				var updateFields []cbor.RawMessage
				_, err = cbor.Decode(bodyFields[3], &updateFields)
				require.NoError(t, err)
				if updateFields == nil {
					t.Fatal("expected Byron update fields")
				}
				updateFields[1] = cbor.RawMessage{0x80}
				bodyFields[3], err = cbor.Encode(updateFields)
				require.NoError(t, err)
			} else {
				bodyFields[2] = cbor.RawMessage{0x80}
			}
			mutatedBody, err := cbor.Encode(bodyFields)
			require.NoError(t, err)

			var decoded byron.ByronMainBlockBody
			_, err = cbor.Decode(mutatedBody, &decoded)
			require.ErrorContains(t, err, tc.want)
		})
	}
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

	// Replace the stakeholder list with an empty one. The reference only
	// accepts the indefinite-length form, so a definite empty list would be
	// rejected at decode before the proof is checked.
	block[1] = cbor.RawMessage{0x9f, 0xff}
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
