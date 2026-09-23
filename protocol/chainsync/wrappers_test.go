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

package chainsync_test

import (
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wrappedByronHeaderCbor builds the NtN RollForward header envelope a peer
// sends for a Byron header: [era, [[type, size], #6.24(header)]].
func wrappedByronHeaderCbor(t *testing.T, byronType uint) []byte {
	t.Helper()
	data, err := cbor.Encode([]any{
		uint(ledger.BlockHeaderTypeByron),
		[]any{
			[]any{byronType, uint(1)},
			cbor.Tag{Number: 24, Content: []byte{0x80}},
		},
	})
	require.NoError(t, err)
	return data
}

// TestWrappedHeaderRejectsOutOfRangeByronType pins the legal domain of the
// Byron block type carried in peer-supplied header metadata.
//
// cardano-ledger's decCBORABlockOrBoundaryHdr (Cardano.Chain.Block.Block)
// accepts only 0 (boundary) and 1 (regular) and otherwise fails with
// "Unknown tag in encoded HeaderOrBoundary"; ouroboros-consensus'
// DecodeDiskDepIx (NestedCtxt Header) ByronBlock, which is what encodes this
// [type, size] pair on the wire, rejects the same values with
// DecoderErrorUnknownTag.
func TestWrappedHeaderRejectsOutOfRangeByronType(t *testing.T) {
	t.Parallel()
	for _, byronType := range []uint{2, 3, 255, 9999} {
		t.Run(fmt.Sprintf("type_%d", byronType), func(t *testing.T) {
			t.Parallel()
			var w chainsync.WrappedHeader
			_, err := cbor.Decode(wrappedByronHeaderCbor(t, byronType), &w)
			require.Error(t, err)
			assert.ErrorContains(t, err, "byron block type")
		})
	}
}

// TestWrappedHeaderAcceptsLegalByronTypes is the positive control: without
// it, a validator that rejected everything would pass the test above.
func TestWrappedHeaderAcceptsLegalByronTypes(t *testing.T) {
	t.Parallel()
	for _, byronType := range []uint{
		ledger.BlockTypeByronEbb,
		ledger.BlockTypeByronMain,
	} {
		t.Run(fmt.Sprintf("type_%d", byronType), func(t *testing.T) {
			t.Parallel()
			var w chainsync.WrappedHeader
			_, err := cbor.Decode(wrappedByronHeaderCbor(t, byronType), &w)
			require.NoError(t, err)
			assert.Equal(t, byronType, w.ByronType())
		})
	}
}

// TestNewWrappedHeaderRejectsOutOfRangeByronType covers the constructor,
// which assigns the same field without going through the decoder.
func TestNewWrappedHeaderRejectsOutOfRangeByronType(t *testing.T) {
	t.Parallel()
	blockCbor, err := cbor.Encode([]any{[]any{}, []any{}})
	require.NoError(t, err)

	_, err = chainsync.NewWrappedHeader(
		ledger.BlockHeaderTypeByron,
		2,
		blockCbor,
	)
	require.Error(t, err)
	assert.ErrorContains(t, err, "byron block type")

	_, err = chainsync.NewWrappedHeader(
		ledger.BlockHeaderTypeByron,
		ledger.BlockTypeByronMain,
		blockCbor,
	)
	require.NoError(t, err)
}
