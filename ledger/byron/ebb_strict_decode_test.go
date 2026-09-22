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
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// genesisTagAttributes is the extra data attributes map {255: "Genesis"}.
var genesisTagAttributes = append(
	[]byte{0xa1, 0x18, 0xff, 0x47},
	[]byte("Genesis")...,
)

func testnetEbbCbor(t *testing.T) []byte {
	t.Helper()
	hexData, err := os.ReadFile(filepath.Join(
		"..",
		"..",
		"protocol",
		"chainsync",
		"testdata",
		"byron_ebb_testnet_8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f.hex",
	))
	require.NoError(t, err)
	raw, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	require.NoError(t, err)
	return raw
}

func rawArrayParts(t *testing.T, raw []byte, n int) []cbor.RawMessage {
	t.Helper()
	var parts []cbor.RawMessage
	_, err := cbor.Decode(raw, &parts)
	require.NoError(t, err)
	require.Len(t, parts, n)
	// Copied into a slice made at length n because nilaway cannot see that
	// require.Len stops the test when the decode returned nothing.
	out := make([]cbor.RawMessage, n)
	copy(out, parts)
	return out
}

// definiteArray encodes parts as a definite-length CBOR array of fewer than
// 24 elements, keeping every part's bytes unchanged.
func definiteArray(parts ...cbor.RawMessage) []byte {
	out := []byte{cbor.CborTypeArray | byte(len(parts))}
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// ebbWith rebuilds the testnet EBB with block field index replaced.
func ebbWith(t *testing.T, index int, replacement []byte) []byte {
	t.Helper()
	parts := rawArrayParts(t, testnetEbbCbor(t), 3)
	parts[index] = replacement
	return definiteArray(parts...)
}

// ebbHeaderWith returns the testnet EBB header with its ExtraData replaced.
func ebbHeaderWith(t *testing.T, extraData []byte) []byte {
	t.Helper()
	blockParts := rawArrayParts(t, testnetEbbCbor(t), 3)
	headerParts := rawArrayParts(t, blockParts[0], 5)
	headerParts[4] = extraData
	return definiteArray(headerParts...)
}

func decodeEbb(raw []byte) (*byron.ByronEpochBoundaryBlock, error) {
	// The body proof is skipped so that every rejection below is the
	// decoder's own, not a proof mismatch caused by the mutation.
	return byron.NewByronEpochBoundaryBlockFromCbor(
		raw, common.VerifyConfig{SkipBodyHashValidation: true},
	)
}

func TestByronEpochBoundaryBlockTestnetFixtureDecodes(t *testing.T) {
	block, err := byron.NewByronEpochBoundaryBlockFromCbor(testnetEbbCbor(t))
	require.NoError(t, err)
	assert.False(t, block.BlockHeader.HasGenesisTag())
	assert.Equal(t, []any{map[any]any{}}, block.Extra)
}

func TestByronEpochBoundaryBlockOuterArity(t *testing.T) {
	parts := rawArrayParts(t, testnetEbbCbor(t), 3)
	for name, raw := range map[string][]byte{
		"two fields":  definiteArray(parts[0], parts[1]),
		"four fields": definiteArray(parts[0], parts[1], parts[2], parts[2]),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decodeEbb(raw)
			require.Error(t, err)
		})
	}
}

func TestByronEpochBoundaryBlockBodyShape(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		accept bool
	}{
		{"definite empty list", "80", false},
		{"definite list of bytes", "814100", false},
		{"indefinite empty list", "9fff", true},
		{"indefinite list holding a list", "9f41004200019fff", false},
		{"indefinite list of definite bytes", "9f4100420001ff", true},
		{"indefinite list with an integer", "9f00ff", false},
		{"indefinite list with text", "9f6161ff", false},
		{"indefinite list with chunked bytes", "9f5f4100ffff", false},
		{"not a list", "a0", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, err := hex.DecodeString(tc.body)
			require.NoError(t, err)
			_, err = decodeEbb(ebbWith(t, 1, body))
			if tc.accept {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// extraDataCases covers the [attributes] shape shared by the extra header
// data and the extra body data. Key-order cases differ between the two and
// are tested separately.
var extraDataCases = []struct {
	name   string
	extra  string
	accept bool
}{
	{"bare integer", "00", false},
	{"bare empty map", "a0", false},
	{"empty list", "80", false},
	{"list holding an integer", "8100", false},
	{"two empty maps", "82a0a0", false},
	{"indefinite list", "9fa0ff", false},
	{"list holding null", "81f6", false},
	{"indefinite empty map", "81bfff", false},
	{"empty attributes", "81a0", true},
	{"unknown attribute", "81a1074100", true},
	{"non-shortest Word8 key", "81a1180540", true},
	{"eight-byte-wide Word8 key", "81a11b000000000000000540", true},
	{"key 255", "81a118ff40", true},
	{"key 256", "81a119010040", false},
	{"negative key", "81a12040", false},
	{"text key", "81a1616140", false},
	{"bytes key", "81a1410040", false},
	{"integer value", "81a10000", false},
	{"text value", "81a1006161", false},
	{"chunked bytes value", "81a1005f4100ff", false},
	{"map value", "81a100a0", false},
}

func TestByronEpochBoundaryBlockExtraBodyDataShape(t *testing.T) {
	cases := append([]struct {
		name   string
		extra  string
		accept bool
	}{
		// dropAttributes is dropMap, which checks neither order nor
		// duplicates.
		{"descending keys", "81a2054001 40", true},
		{"duplicate keys", "81a2014001 40", true},
	}, extraDataCases...)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			extra, err := hex.DecodeString(
				strings.ReplaceAll(tc.extra, " ", ""),
			)
			require.NoError(t, err)
			_, err = decodeEbb(ebbWith(t, 2, extra))
			if tc.accept {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestByronEpochBoundaryBlockExtraHeaderDataShape(t *testing.T) {
	cases := append([]struct {
		name   string
		extra  string
		accept bool
	}{
		// decCBORAttributes decodes through decodeMapSkel, which requires
		// strictly ascending keys.
		{"ascending keys", "81a2014005 40", true},
		{"descending keys", "81a2054001 40", false},
		{"duplicate keys", "81a2014001 40", false},
	}, extraDataCases...)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			extra, err := hex.DecodeString(
				strings.ReplaceAll(tc.extra, " ", ""),
			)
			require.NoError(t, err)
			header := ebbHeaderWith(t, extra)
			_, headerErr := byron.NewByronEpochBoundaryBlockHeaderFromCbor(
				header,
			)
			_, blockErr := decodeEbb(ebbWith(t, 0, header))
			if tc.accept {
				require.NoError(t, headerErr)
				require.NoError(t, blockErr)
			} else {
				require.Error(t, headerErr)
				require.Error(t, blockErr)
			}
		})
	}
}

func TestByronEpochBoundaryBlockHeaderGenesisTag(t *testing.T) {
	tests := []struct {
		name  string
		attrs []byte
		want  bool
	}{
		{"no attributes", []byte{0xa0}, false},
		{"genesis tag", genesisTagAttributes, true},
		{
			"genesis tag after an unknown attribute",
			append([]byte{0xa2, 0x07, 0x40}, genesisTagAttributes[1:]...),
			true,
		},
		{
			"key 255 with another value",
			append([]byte{0xa1, 0x18, 0xff, 0x46}, []byte("Genesi")...),
			false,
		},
		{
			"key 254 with the genesis value",
			append([]byte{0xa1, 0x18, 0xfe, 0x47}, []byte("Genesis")...),
			false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			header := ebbHeaderWith(t, definiteArray(tc.attrs))
			decoded, err := byron.NewByronEpochBoundaryBlockHeaderFromCbor(
				header,
			)
			require.NoError(t, err)
			assert.Equal(t, tc.want, decoded.HasGenesisTag())

			block, err := decodeEbb(ebbWith(t, 0, header))
			require.NoError(t, err)
			assert.Equal(t, tc.want, block.BlockHeader.HasGenesisTag())
		})
	}
}

func TestByronEpochBoundaryBlockHeaderOuterArity(t *testing.T) {
	blockParts := rawArrayParts(t, testnetEbbCbor(t), 3)
	headerParts := rawArrayParts(t, blockParts[0], 5)
	for name, raw := range map[string][]byte{
		"four fields": definiteArray(headerParts[:4]...),
		"six fields":  definiteArray(append(headerParts, headerParts[4])...),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := byron.NewByronEpochBoundaryBlockHeaderFromCbor(raw)
			require.Error(t, err)
		})
	}
}

// indefiniteArray encodes parts as an indefinite-length CBOR array.
func indefiniteArray(parts ...cbor.RawMessage) []byte {
	out := []byte{0x9f}
	for _, p := range parts {
		out = append(out, p...)
	}
	return append(out, 0xff)
}

// TestByronEpochBoundaryBlockRejectsIndefiniteLists covers the lists the
// reference reads with enforceSize, which only accepts a definite length.
func TestByronEpochBoundaryBlockRejectsIndefiniteLists(t *testing.T) {
	blockParts := rawArrayParts(t, testnetEbbCbor(t), 3)
	headerParts := rawArrayParts(t, blockParts[0], 5)
	consensusParts := rawArrayParts(t, headerParts[3], 2)
	difficultyParts := rawArrayParts(t, consensusParts[1], 1)

	withConsensus := func(consensus []byte) []byte {
		parts := rawArrayParts(t, blockParts[0], 5)
		parts[3] = consensus
		return definiteArray(parts...)
	}
	headers := map[string][]byte{
		"header": indefiniteArray(headerParts...),
		"consensus data": withConsensus(
			indefiniteArray(consensusParts...),
		),
		"chain difficulty": withConsensus(definiteArray(
			consensusParts[0], indefiniteArray(difficultyParts...),
		)),
	}
	for name, header := range headers {
		t.Run(name, func(t *testing.T) {
			_, err := byron.NewByronEpochBoundaryBlockHeaderFromCbor(header)
			require.Error(t, err)
			_, err = decodeEbb(
				definiteArray(header, blockParts[1], blockParts[2]),
			)
			require.Error(t, err)
		})
	}
	t.Run("block", func(t *testing.T) {
		_, err := decodeEbb(indefiniteArray(blockParts...))
		require.Error(t, err)
	})
}
