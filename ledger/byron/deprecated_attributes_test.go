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
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/stretchr/testify/require"
)

// nonEmptyCborMap is a 1-pair CBOR map (key 0, value 0), standing in for
// any non-empty deprecated attributes map. The reference rejects any
// non-zero-length map here (Cardano.Chain.Common.Attributes.dropEmptyAttributes:
// "decodeMapLen; unless (len == 0) $ cborError ...").
var nonEmptyCborMap = []byte{0xa1, 0x00, 0x00}

// realByronBlockCbor returns the real mainnet Byron block fixture
// (internal/testdata.ByronBlockHex), decoded to raw bytes.
func realByronBlockCbor(t *testing.T) []byte {
	t.Helper()
	data, err := hex.DecodeString(strings.TrimSpace(testdata.ByronBlockHex))
	require.NoError(t, err)
	return data
}

// spliceArray reconstructs a CBOR array's raw bytes with parts[index]
// replaced by replacement and every other part's bytes unchanged, prefixed
// by the array's own original length-header byte.
func spliceArray(
	header byte,
	parts []cbor.RawMessage,
	index int,
	replacement []byte,
) []byte {
	out := []byte{header}
	for i, p := range parts {
		if i == index {
			out = append(out, replacement...)
			continue
		}
		out = append(out, p...)
	}
	return out
}

// byronBlockParts decodes raw as a top-level Byron block array
// [header, body, extra] and returns its raw sub-messages.
func byronBlockParts(t *testing.T, raw []byte) []cbor.RawMessage {
	t.Helper()
	var parts []cbor.RawMessage
	_, err := cbor.Decode(raw, &parts)
	require.NoError(t, err)
	require.Len(t, parts, 3)
	return parts
}

// byronHeaderExtraDataParts decodes headerRaw as a Byron main header array
// and returns its ExtraData field's raw sub-messages
// [blockVersion, softwareVersion, attributes, extraProof].
func byronHeaderExtraDataParts(
	t *testing.T,
	headerRaw cbor.RawMessage,
) (extraDataRaw cbor.RawMessage, parts []cbor.RawMessage) {
	t.Helper()
	var headerParts []cbor.RawMessage
	_, err := cbor.Decode(headerRaw, &headerParts)
	require.NoError(t, err)
	require.Len(t, headerParts, 5)
	extraDataRaw = headerParts[4]
	_, err = cbor.Decode(extraDataRaw, &parts)
	require.NoError(t, err)
	require.Len(t, parts, 4)
	return extraDataRaw, parts
}

// mutateHeaderExtraData rebuilds the full block CBOR with the header's
// ExtraData field's raw sub-message at index replaced by replacement, and
// every other byte (including the body and the block-level extra) unchanged.
func mutateHeaderExtraData(
	t *testing.T,
	raw []byte,
	index int,
	replacement []byte,
) []byte {
	t.Helper()
	blockParts := byronBlockParts(t, raw)
	headerRaw := blockParts[0]

	var headerParts []cbor.RawMessage
	_, err := cbor.Decode(headerRaw, &headerParts)
	require.NoError(t, err)
	require.Len(t, headerParts, 5)

	var extraDataParts []cbor.RawMessage
	_, err = cbor.Decode(headerParts[4], &extraDataParts)
	require.NoError(t, err)
	require.Len(t, extraDataParts, 4)

	newExtraData := spliceArray(
		headerParts[4][0],
		extraDataParts,
		index,
		replacement,
	)
	newHeader := spliceArray(headerRaw[0], headerParts, 4, newExtraData)
	return spliceArray(raw[0], blockParts, 0, newHeader)
}

// mutateBlockExtra rebuilds the full block CBOR with the top-level Extra
// (ExtraBodyData) field's raw sub-message at index replaced by replacement.
func mutateBlockExtra(
	t *testing.T,
	raw []byte,
	index int,
	replacement []byte,
) []byte {
	t.Helper()
	blockParts := byronBlockParts(t, raw)
	extraRaw := blockParts[2]

	var extraParts []cbor.RawMessage
	_, err := cbor.Decode(extraRaw, &extraParts)
	require.NoError(t, err)
	require.Len(t, extraParts, 1)

	newExtra := spliceArray(extraRaw[0], extraParts, index, replacement)
	return spliceArray(raw[0], blockParts, 2, newExtra)
}

// replaceBlockExtraWhole rebuilds the full block CBOR with the top-level
// Extra field's whole raw sub-message (array header included) replaced.
func replaceBlockExtraWhole(t *testing.T, raw []byte, replacement []byte) []byte {
	t.Helper()
	blockParts := byronBlockParts(t, raw)
	return spliceArray(raw[0], blockParts, 2, replacement)
}

// TestByronMainBlockRejectsNonEmptyExtraBodyDataAttributes is the
// blinklabs-io/gouroboros#2340 regression for the "concrete divergence" the
// issue names directly: [header, body, [emptyAttributes]] mutated to
// [header, body, [nonEmptyAttributes]] with nothing else touched. The
// reference rejects this at Block.hs's "enforceSize \"ExtraBodyData\" 1 >>
// dropEmptyAttributes"; gouroboros must too.
func TestByronMainBlockRejectsNonEmptyExtraBodyDataAttributes(t *testing.T) {
	raw := realByronBlockCbor(t)

	t.Run("canonical block still decodes", func(t *testing.T) {
		var block byron.ByronMainBlock
		require.NoError(t, block.UnmarshalCBOR(raw))
	})

	mutated := mutateBlockExtra(t, raw, 0, nonEmptyCborMap)

	t.Run("non-empty attributes rejected", func(t *testing.T) {
		var block byron.ByronMainBlock
		require.Error(t, block.UnmarshalCBOR(mutated))
	})

	t.Run("block identity is unchanged by the mutation", func(t *testing.T) {
		// ExtraBodyData lives outside the header entirely, so mutating it
		// must never change the header's own bytes, and therefore never its
		// hash, the PBFT signature it covers, or the previous-block link.
		originalParts := byronBlockParts(t, raw)
		mutatedParts := byronBlockParts(t, mutated)
		require.Equal(t, []byte(originalParts[0]), []byte(mutatedParts[0]))
		require.Equal(t, []byte(originalParts[1]), []byte(mutatedParts[1]))

		var originalHeader, mutatedHeader byron.ByronMainBlockHeader
		require.NoError(t, originalHeader.UnmarshalCBOR(originalParts[0]))
		require.NoError(t, mutatedHeader.UnmarshalCBOR(mutatedParts[0]))
		require.Equal(t, originalHeader.Hash(), mutatedHeader.Hash())
	})
}

// TestByronMainBlockRejectsMalformedExtraBodyData covers the remaining
// shapes the acceptance criteria names: wrong element count and a
// wrong-typed single element.
func TestByronMainBlockRejectsMalformedExtraBodyData(t *testing.T) {
	raw := realByronBlockCbor(t)

	tests := []struct {
		name        string
		replacement []byte
	}{
		{"zero elements", []byte{0x80}},
		{"two elements", []byte{0x82, 0xa0, 0xa0}},
		{"single non-map element", []byte{0x81, 0x00}},
		// CBOR null/undefined both decode into a nil map[any]any with no
		// error, and len(nil) == 0, so a naive length-only check would
		// accept either in place of a real empty map.
		{"null instead of map", []byte{0x81, 0xf6}},
		{"undefined instead of map", []byte{0x81, 0xf7}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mutated := replaceBlockExtraWhole(t, raw, tc.replacement)
			var block byron.ByronMainBlock
			require.Error(t, block.UnmarshalCBOR(mutated))
		})
	}
}

// TestByronMainBlockHeaderRejectsNonEmptyBlockVersionsAttributes is the
// blinklabs-io/gouroboros#2340 regression for the header side: a validly
// signed header (real mainnet signature bytes, left untouched) with its
// deprecated BlockVersions attributes made non-empty. Decode-time rejection
// must happen before any signature check even runs
// (Header.hs's decCBORBlockVersions: "enforceSize \"BlockVersions\" 4 >>
// ... <* dropEmptyAttributes <* dropBytes").
func TestByronMainBlockHeaderRejectsNonEmptyBlockVersionsAttributes(t *testing.T) {
	raw := realByronBlockCbor(t)
	blockParts := byronBlockParts(t, raw)

	t.Run("canonical header still decodes", func(t *testing.T) {
		var header byron.ByronMainBlockHeader
		require.NoError(t, header.UnmarshalCBOR(blockParts[0]))
	})

	_, extraDataParts := byronHeaderExtraDataParts(t, blockParts[0])
	require.Equal(t, []byte{0xa0}, []byte(extraDataParts[2]))

	tests := []struct {
		name        string
		replacement []byte
	}{
		{"non-empty map", nonEmptyCborMap},
		// CBOR null/undefined both decode into a nil map[any]any with no
		// error, and len(nil) == 0, so a naive length-only check would
		// accept either in place of a real empty map.
		{"null instead of map", []byte{0xf6}},
		{"undefined instead of map", []byte{0xf7}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mutated := mutateHeaderExtraData(t, raw, 2, tc.replacement)
			mutatedBlockParts := byronBlockParts(t, mutated)

			var header byron.ByronMainBlockHeader
			require.Error(t, header.UnmarshalCBOR(mutatedBlockParts[0]))
		})
	}
}

// TestByronMainBlockHeaderPreservesExtraProofOfArbitraryLength confirms the
// acceptance criteria's other half: ExtraProof (the header's deprecated
// extra-body-data hash placeholder) must keep accepting a byte string of
// any length, unaffected by the new attributes check.
// Header.hs's decCBORBlockVersions ends with a bare "dropBytes" for this
// field -- any byte string, never interpreted.
func TestByronMainBlockHeaderPreservesExtraProofOfArbitraryLength(t *testing.T) {
	raw := realByronBlockCbor(t)

	tests := []struct {
		name        string
		replacement []byte
	}{
		{"shorter than 32 bytes", append([]byte{0x44}, []byte{0x01, 0x02, 0x03, 0x04}...)},
		{"longer than 32 bytes", append([]byte{0x58, 0x28}, make([]byte, 40)...)},
		{"empty byte string", []byte{0x40}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mutated := mutateHeaderExtraData(t, raw, 3, tc.replacement)
			blockParts := byronBlockParts(t, mutated)
			var header byron.ByronMainBlockHeader
			require.NoError(t, header.UnmarshalCBOR(blockParts[0]))
		})
	}
}
