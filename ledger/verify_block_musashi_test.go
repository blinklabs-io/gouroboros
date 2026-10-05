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

package ledger

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/stretchr/testify/require"
)

// TestDetermineBlockTypeAcceptsLiveMusashiHeader classifies the header of a
// block captured from the ouroboros-leios "musashi" testnet
// (dijkstra/testdata/musashi_dijkstra_block.hex, slot 566037, block 28091).
// Its body has the 12-field Leios-extended shape (cardano-ledger
// eras/dijkstra/impl/cddl/data/dijkstra.cddl header_body), which callers such
// as an era check must classify as Dijkstra, and the decoded header must
// re-encode to the received bytes.
func TestDetermineBlockTypeAcceptsLiveMusashiHeader(t *testing.T) {
	t.Parallel()
	hexData, err := os.ReadFile("dijkstra/testdata/musashi_dijkstra_block.hex")
	require.NoError(t, err)
	raw, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	require.NoError(t, err)

	var top []cbor.RawMessage
	_, err = cbor.Decode(raw, &top)
	require.NoError(t, err)
	require.Len(t, top, 2)
	headerCbor := []byte(top[0])

	var bodyElems []any
	var headerArray []cbor.RawMessage
	_, err = cbor.Decode(headerCbor, &headerArray)
	require.NoError(t, err)
	require.Len(t, headerArray, 2)
	_, err = cbor.Decode(headerArray[0], &bodyElems)
	require.NoError(t, err)
	require.Len(
		t,
		bodyElems,
		HeaderBodyLengthDijkstraLeiosLike,
		"fixture must still exercise the 12-field Leios-extended body",
	)

	blockType, err := DetermineBlockType(headerCbor)
	require.NoError(t, err)
	require.Equal(t, uint(BlockTypeDijkstra), blockType)

	// Round trip: decoding the header and re-encoding it must reproduce the
	// original wire bytes exactly (gouroboros preserves raw CBOR).
	var header dijkstra.DijkstraBlockHeader
	_, err = cbor.Decode(headerCbor, &header)
	require.NoError(t, err)
	reencoded, err := header.MarshalCBOR()
	require.NoError(t, err)
	require.Equal(t, headerCbor, reencoded)
}
