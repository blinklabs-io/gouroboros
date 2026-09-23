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
	"math"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// encodeByronOutput builds the wire encoding of a Byron transaction output
// carrying amount, using a real (if otherwise meaningless) Byron address so
// the address half of decoding always succeeds and only the amount bound is
// under test.
func encodeByronOutput(t *testing.T, amount uint64) []byte {
	t.Helper()
	addr, err := common.NewByronAddressFromParts(
		0,
		make([]byte, common.AddressHashSize),
		common.ByronAddressAttributes{},
	)
	require.NoError(t, err)
	addrBytes, err := addr.Bytes()
	require.NoError(t, err)
	data, err := cbor.Encode(
		[]any{cbor.RawMessage(addrBytes), amount},
	)
	require.NoError(t, err)
	return data
}

func TestByronTransactionOutputEnforcesLovelaceBounds(t *testing.T) {
	t.Run("accepts zero", func(t *testing.T) {
		out, err := byron.NewByronTransactionOutputFromCbor(
			encodeByronOutput(t, 0),
		)
		require.NoError(t, err)
		require.Equal(t, uint64(0), out.OutputAmount)
	})

	t.Run("accepts exact maximum", func(t *testing.T) {
		out, err := byron.NewByronTransactionOutputFromCbor(
			encodeByronOutput(t, byron.MaxLovelace),
		)
		require.NoError(t, err)
		require.Equal(t, byron.MaxLovelace, out.OutputAmount)
	})

	t.Run("rejects maximum plus one", func(t *testing.T) {
		_, err := byron.NewByronTransactionOutputFromCbor(
			encodeByronOutput(t, byron.MaxLovelace+1),
		)
		require.Error(t, err)
	})

	t.Run("rejects math.MaxUint64", func(t *testing.T) {
		_, err := byron.NewByronTransactionOutputFromCbor(
			encodeByronOutput(t, math.MaxUint64),
		)
		require.Error(t, err)
	})
}

// encodeByronTransaction builds the wire encoding of a Byron transaction
// carrying a single output of amount, with an empty input list, empty
// attributes, and an empty witness list. Signature and balance validation
// are out of scope for the Lovelace bound, which the decoder must enforce
// regardless of either.
func encodeByronTransaction(t *testing.T, amount uint64) []byte {
	t.Helper()
	output := encodeByronOutput(t, amount)
	body, err := cbor.Encode(
		[]any{[]any{}, []any{cbor.RawMessage(output)}, map[any]any{}},
	)
	require.NoError(t, err)
	twit, err := cbor.Encode([]any{})
	require.NoError(t, err)
	tx, err := cbor.Encode(
		[]any{cbor.RawMessage(body), cbor.RawMessage(twit)},
	)
	require.NoError(t, err)
	return tx
}

func TestByronTransactionEnforcesLovelaceBounds(t *testing.T) {
	t.Run("accepts zero", func(t *testing.T) {
		tx, err := byron.NewByronTransactionFromCbor(
			encodeByronTransaction(t, 0),
		)
		require.NoError(t, err)
		require.Equal(t, uint64(0), tx.Body.TxOutputs[0].OutputAmount)
	})

	t.Run("accepts exact maximum", func(t *testing.T) {
		tx, err := byron.NewByronTransactionFromCbor(
			encodeByronTransaction(t, byron.MaxLovelace),
		)
		require.NoError(t, err)
		require.Equal(t, byron.MaxLovelace, tx.Body.TxOutputs[0].OutputAmount)
	})

	t.Run("rejects maximum plus one", func(t *testing.T) {
		_, err := byron.NewByronTransactionFromCbor(
			encodeByronTransaction(t, byron.MaxLovelace+1),
		)
		require.Error(t, err)
	})

	t.Run("rejects math.MaxUint64", func(t *testing.T) {
		_, err := byron.NewByronTransactionFromCbor(
			encodeByronTransaction(t, math.MaxUint64),
		)
		require.Error(t, err)
	})
}

// TestByronMainBlockRejectsOversizedOutputAtDecode proves that an oversized
// output amount is rejected at decode time even in a block whose transaction
// and witness proofs have been correctly recomputed over the exact
// (tampered) body, exactly as a real block producer would build them --
// so proof validation alone, which runs after decoding in
// NewByronMainBlockFromCbor, never gets the chance to be the thing that
// repairs a missing range check.
//
// It starts from a real mainnet block, replaces transaction 0's first
// output amount with a same-CBOR-width value, and recomputes the header's
// tx_proof Merkle root over the resulting bodies using the same MerkleRoot
// function ValidateBodyProof itself uses -- so the two blocks this test
// builds are byte-for-byte self-consistent except for the one amount and
// its dependent root.
func TestByronMainBlockRejectsOversizedOutputAtDecode(t *testing.T) {
	blockBytes, err := hex.DecodeString(
		strings.TrimSpace(testdata.ByronBlockHex),
	)
	require.NoError(t, err)

	var original byron.ByronMainBlock
	require.NoError(t, original.UnmarshalCBOR(blockBytes))
	require.NotEmpty(t, original.Body.TxPayload)
	require.NotEmpty(t, original.Body.TxPayload[0].Body.TxOutputs)

	tx0Body := original.Body.TxPayload[0].Body.Cbor()
	require.NotEmpty(t, tx0Body)
	bodyOffset := bytes.Index(blockBytes, tx0Body)
	require.GreaterOrEqual(t, bodyOffset, 0)

	originalAmount := original.Body.TxPayload[0].Body.TxOutputs[0].OutputAmount
	originalAmountEnc, err := cbor.Encode(originalAmount)
	require.NoError(t, err)
	amountOffsetInBody := bytes.Index(tx0Body, originalAmountEnc)
	require.GreaterOrEqual(t, amountOffsetInBody, 0)

	originalBodies := make([][]byte, len(original.Body.TxPayload))
	for i, tx := range original.Body.TxPayload {
		originalBodies[i] = tx.Body.Cbor()
	}
	oldRoot := byron.MerkleRoot(originalBodies)
	rootOffset := bytes.Index(blockBytes, oldRoot.Bytes())
	require.GreaterOrEqual(t, rootOffset, 0)

	// buildBlock replaces output 0's amount with newAmount and recomputes
	// the tx_proof Merkle root over the resulting bodies, returning a
	// fully self-consistent block.
	buildBlock := func(t *testing.T, newAmount uint64) []byte {
		t.Helper()
		newAmountEnc, err := cbor.Encode(newAmount)
		require.NoError(t, err)
		require.Len(
			t,
			newAmountEnc,
			len(originalAmountEnc),
			"replacement amount must keep the same CBOR width",
		)

		tampered := append([]byte{}, blockBytes...)
		copy(
			tampered[bodyOffset+amountOffsetInBody:],
			newAmountEnc,
		)

		bodies := make([][]byte, len(originalBodies))
		copy(bodies, originalBodies)
		bodies[0] = tampered[bodyOffset : bodyOffset+len(tx0Body)]
		newRoot := byron.MerkleRoot(bodies)
		copy(tampered[rootOffset:], newRoot.Bytes())
		return tampered
	}

	t.Run("proof-consistent block at the maximum decodes and validates", func(t *testing.T) {
		block, err := byron.NewByronMainBlockFromCbor(
			buildBlock(t, byron.MaxLovelace),
		)
		require.NoError(t, err)
		require.Equal(
			t,
			byron.MaxLovelace,
			block.Body.TxPayload[0].Body.TxOutputs[0].OutputAmount,
		)
	})

	t.Run("proof-consistent block over the maximum is rejected at decode", func(t *testing.T) {
		_, err := byron.NewByronMainBlockFromCbor(
			buildBlock(t, byron.MaxLovelace+1),
		)
		require.Error(t, err)
		require.NotErrorIs(t, err, byron.ErrBodyProofMismatch)
		require.Contains(t, err.Error(), "exceeds maximum Lovelace value")
	})
}
