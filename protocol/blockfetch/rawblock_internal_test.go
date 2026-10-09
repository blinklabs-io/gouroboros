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
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger"
	ledgerbyron "github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/stretchr/testify/require"
)

// TestRawBlockHeaderInfoMatchesTypedDecode is the agreement test between the
// two correlation paths. For any block the typed decoder can handle, reading
// the header directly must produce exactly the point and previous hash the
// decoded block reports, or the raw fallback would correlate ranges by a
// different rule than the normal path.
func TestRawBlockHeaderInfoMatchesTypedDecode(t *testing.T) {
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

	type testCase struct {
		name          string
		raw           []byte
		block         ledger.Block
		slotsPerEpoch uint64
	}
	testCases := []testCase{
		// Babbage exercises a 10-field header body with a present prev_hash.
		{name: "babbage", raw: babbageCbor, block: &babbageBlock},
		// The origin case: prev_hash encoded as CBOR null, which the typed
		// header decoder turns into the zero hash.
		{name: "babbage origin", raw: originCbor, block: &originBlock},
	}
	for _, fixture := range testdata.GetTestBlocks() {
		if fixture.Name != "Byron" {
			continue
		}
		block, err := ledger.NewBlockFromCbor(fixture.BlockType, fixture.Cbor)
		require.NoError(t, err)
		testCases = append(testCases, testCase{
			name: "byron main", raw: fixture.Cbor, block: block,
		})
	}
	ebbHex, err := os.ReadFile(filepath.Join(
		"..", "chainsync", "testdata",
		"byron_ebb_testnet_8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f.hex",
	))
	require.NoError(t, err)
	ebbCbor, err := hex.DecodeString(strings.TrimSpace(string(ebbHex)))
	require.NoError(t, err)
	ebb, err := ledger.NewBlockFromCbor(ledger.BlockTypeByronEbb, ebbCbor)
	require.NoError(t, err)
	testCases = append(testCases, testCase{
		name:          "byron epoch boundary",
		raw:           ebbCbor,
		block:         ebb,
		slotsPerEpoch: 600,
	})
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := rawBlockHeaderInfoFromCbor(
				tc.raw,
				tc.slotsPerEpoch,
			)
			require.NoError(t, err)
			expectedSlot, err := ledgerbyron.SlotNumberFromBlockHeader(
				tc.block.Header(),
				tc.slotsPerEpoch,
			)
			require.NoError(t, err)
			require.Equal(t, expectedSlot, info.point.Slot)
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
			_, err := rawBlockHeaderInfoFromCbor(tc.raw, 0)
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
