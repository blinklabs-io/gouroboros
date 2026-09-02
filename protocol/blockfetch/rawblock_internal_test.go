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

package blockfetch

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/stretchr/testify/require"
)

func readMusashiBlock(t *testing.T) []byte {
	t.Helper()
	hexData, err := os.ReadFile(filepath.Join(
		"..", "..", "ledger", "dijkstra", "testdata",
		"musashi_dijkstra_block.hex",
	))
	require.NoError(t, err)
	raw, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	require.NoError(t, err)
	return raw
}

// TestRawBlockHeaderInfoMatchesTypedDecode is the agreement test between the
// two correlation paths. For any block the typed decoder can handle, reading
// the header directly must produce exactly the point and previous hash the
// decoded block reports, or the raw fallback would correlate ranges by a
// different rule than the normal path.
func TestRawBlockHeaderInfoMatchesTypedDecode(t *testing.T) {
	musashi := readMusashiBlock(t)
	musashiBlock, err := dijkstra.NewDijkstraBlockFromCbor(musashi)
	require.NoError(t, err)

	babbageBlock := ledger.BabbageBlock{
		BlockHeader: &ledger.BabbageBlockHeader{},
	}
	babbageBlock.BlockHeader.Body.BlockNumber = 12345
	babbageBlock.BlockHeader.Body.Slot = 23456
	babbageCbor, err := cbor.Encode(babbageBlock)
	require.NoError(t, err)
	_, err = cbor.Decode(babbageCbor, &babbageBlock)
	require.NoError(t, err)

	// An encoded zero Blake2b256 is a 32-byte bytestring, not CBOR null, so
	// the case above does not reach the origin branch. Rewrite prev_hash to
	// null to cover it.
	originCbor := withNullPrevHash(t, babbageCbor)
	var originBlock ledger.BabbageBlock
	_, err = cbor.Decode(originCbor, &originBlock)
	require.NoError(t, err)

	testCases := []struct {
		name  string
		raw   []byte
		block ledger.Block
	}{
		// Babbage exercises a 10-field header body with a present prev_hash.
		{name: "babbage", raw: babbageCbor, block: &babbageBlock},
		// The origin case: prev_hash encoded as CBOR null, which the typed
		// header decoder turns into the zero hash.
		{name: "babbage origin", raw: originCbor, block: &originBlock},
		// Musashi exercises a 12-field Leios-extended header body, the shape
		// that made the raw fallback necessary in the first place.
		{name: "musashi dijkstra", raw: musashi, block: musashiBlock},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := rawBlockHeaderInfoFromCbor(tc.raw)
			require.NoError(t, err)
			require.Equal(t, tc.block.SlotNumber(), info.point.Slot)
			require.Equal(t, tc.block.Hash().Bytes(), info.point.Hash)
			wantPrev := tc.block.PrevHash()
			require.Equal(t, wantPrev.Bytes(), info.prevHash)
		})
	}
}

// TestRawBlockHeaderInfoRejectsMalformed covers the inputs a hostile or broken
// peer can send. Each must produce an error rather than a panic or a
// zero-valued point that would correlate against an unrelated range.
func TestRawBlockHeaderInfoRejectsMalformed(t *testing.T) {
	headerBody := func(fields ...any) []byte {
		t.Helper()
		encoded, err := cbor.Encode(fields)
		require.NoError(t, err)
		return encoded
	}
	block := func(t *testing.T, header any) []byte {
		t.Helper()
		encoded, err := cbor.Encode([]any{header, []any{}})
		require.NoError(t, err)
		return encoded
	}
	shortBody := headerBody(uint64(1), uint64(2))
	badSlot := headerBody("not-a-slot", "not-a-slot", make([]byte, 32))

	testCases := []struct {
		name string
		raw  []byte
	}{
		{name: "empty", raw: []byte{}},
		{name: "not an array", raw: []byte{0x01}},
		{
			name: "empty block array",
			raw:  func() []byte { b, _ := cbor.Encode([]any{}); return b }(),
		},
		{
			name: "header not an array",
			raw:  block(t, uint64(1)),
		},
		{
			name: "header body not an array",
			raw:  block(t, []any{uint64(1), []byte{}}),
		},
		{
			name: "header body too short",
			raw:  block(t, []any{cbor.RawMessage(shortBody), []byte{}}),
		},
		{
			name: "slot not an integer",
			raw:  block(t, []any{cbor.RawMessage(badSlot), []byte{}}),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := rawBlockHeaderInfoFromCbor(tc.raw)
			require.Error(t, err)
		})
	}
}

// withNullPrevHash rewrites a block's header-body prev_hash to CBOR null,
// producing the encoding an origin header uses.
func withNullPrevHash(t *testing.T, raw []byte) []byte {
	t.Helper()
	var blockElems []cbor.RawMessage
	_, err := cbor.Decode(raw, &blockElems)
	require.NoError(t, err)
	require.NotEmpty(t, blockElems)
	var headerElems []cbor.RawMessage
	_, err = cbor.Decode(blockElems[0], &headerElems)
	require.NoError(t, err)
	require.NotEmpty(t, headerElems)
	var bodyElems []cbor.RawMessage
	_, err = cbor.Decode(headerElems[0], &bodyElems)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(bodyElems), blockHeaderBodyMinFields)
	bodyElems[2] = cbor.RawMessage{0xf6}
	headerBody, err := cbor.Encode(bodyElems)
	require.NoError(t, err)
	headerElems[0] = headerBody
	header, err := cbor.Encode(headerElems)
	require.NoError(t, err)
	blockElems[0] = header
	out, err := cbor.Encode(blockElems)
	require.NoError(t, err)
	return out
}
