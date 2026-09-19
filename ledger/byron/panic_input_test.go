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
	"math"

	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// nullHeaderMainBlockCbor rewrites a real Byron main block so its header is
// CBOR null, leaving the body and extra body data untouched. Splicing a known
// good block keeps every other decode check satisfied, so a decode that still
// succeeds can only have accepted the null header.
func nullHeaderMainBlockCbor(t *testing.T) []byte {
	t.Helper()
	raw, err := hex.DecodeString(strings.TrimSpace(testdata.ByronBlockHex))
	require.NoError(t, err)
	var parts []cbor.RawMessage
	_, err = cbor.Decode(raw, &parts)
	require.NoError(t, err)
	require.Len(t, parts, 3)
	parts[0] = cbor.RawMessage{0xf6}
	spliced, err := cbor.Encode(parts)
	require.NoError(t, err)
	return spliced
}

// A Byron main block arrives from a remote peer, so a header the decoder
// leaves nil is dereferenced by every accessor on the block. The
// epoch-boundary decode already rejects this shape.
func TestByronMainBlockRejectsNullHeader(t *testing.T) {
	t.Parallel()

	data := nullHeaderMainBlockCbor(t)

	var block byron.ByronMainBlock
	err := block.UnmarshalCBOR(data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing header")

	// The body-proof check stands in for the missing guard only while it
	// runs; a caller that skips it must still not receive a nil header.
	_, err = byron.NewByronMainBlockFromCbor(
		data,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing header")
}

// A valid block must keep decoding, so the guard cannot be satisfied by
// rejecting everything.
func TestByronMainBlockAcceptsRealHeader(t *testing.T) {
	t.Parallel()

	raw, err := hex.DecodeString(strings.TrimSpace(testdata.ByronBlockHex))
	require.NoError(t, err)
	block, err := byron.NewByronMainBlockFromCbor(raw)
	require.NoError(t, err)
	require.NotNil(t, block.BlockHeader)
}
func TestNewByronTransactionInputRejectsBadArguments(t *testing.T) {
	t.Parallel()

	validHash := strings.Repeat("ab", 32)

	for _, test := range []struct {
		name string
		hash string
		idx  int
	}{
		{name: "non-hex hash", hash: "not-hex", idx: 0},
		{name: "negative index", hash: validHash, idx: -1},
		{
			name: "index above uint32",
			hash: validHash,
			idx:  math.MaxUint32 + 1,
		},
	} {
		if test.idx > math.MaxInt32 && math.MaxInt == math.MaxInt32 {
			continue
		}
		require.NotPanics(t, func() {
			_, err := byron.NewByronTransactionInput(test.hash, test.idx)
			require.Error(t, err, test.name)
		}, test.name)
	}

	input, err := byron.NewByronTransactionInput(validHash, 3)
	require.NoError(t, err)
	require.Equal(t, uint32(3), input.OutputIndex)
	require.Equal(t, validHash, input.Id().String())
}

// Produced builds its inputs from the already-typed transaction hash rather
// than round-tripping it through hex, so the identity it yields is asserted
// here: a UTxO identified by the wrong hash or index is a consensus fault,
// not a formatting one.
func TestByronTransactionProducedInputIdentity(t *testing.T) {
	t.Parallel()

	raw, err := hex.DecodeString(strings.TrimSpace(testdata.ByronBlockHex))
	require.NoError(t, err)
	block, err := byron.NewByronMainBlockFromCbor(raw)
	require.NoError(t, err)

	txs := block.Transactions()
	require.NotEmpty(t, txs)
	for _, tx := range txs {
		produced := tx.Produced()
		require.Len(t, produced, len(tx.Outputs()))
		for idx, utxo := range produced {
			require.Equal(t, tx.Hash(), utxo.Id.Id())
			require.Equal(t, uint32(idx), utxo.Id.Index())
		}
	}
}
