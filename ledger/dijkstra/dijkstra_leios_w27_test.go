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

package dijkstra

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// prototype-2026w27 (IntersectMBO/cardano-ledger #5872, #5889):
//
//	leios_certificate  = [ signers : bytes, aggregated_signature : bytes .size 48 ]
//	leios_announcement = [ announced_eb : hash32, announced_eb_size : uint .size 4 ]
//	header extension   = [ leios_certified : bool, leios_announcement / nil ]

func TestDijkstraLeiosCertificateRoundTrip(t *testing.T) {
	signers := []byte{0xf0, 0x0f}
	sig := make([]byte, common.LeiosBlsSignatureSize)
	for i := range sig {
		sig[i] = byte(i)
	}
	certCbor, err := cbor.Encode([]any{signers, sig})
	require.NoError(t, err)
	var cert DijkstraLeiosCertificate
	_, err = cbor.Decode(certCbor, &cert)
	require.NoError(t, err)
	assert.Equal(t, signers, cert.Signers)
	assert.Equal(t, sig, cert.AggregatedSignature)
	out, err := cert.MarshalCBOR()
	require.NoError(t, err)
	assert.Equal(t, certCbor, out)
}

func TestDijkstraLeiosCertificateRejectsBadSignatureLength(t *testing.T) {
	shortSig := make([]byte, common.LeiosBlsSignatureSize-1)
	certCbor, err := cbor.Encode([]any{[]byte{0x01}, shortSig})
	require.NoError(t, err)
	var cert DijkstraLeiosCertificate
	_, err = cbor.Decode(certCbor, &cert)
	require.Error(t, err)
}

func TestDijkstraLeiosCertificateRejectsWrongFieldCount(t *testing.T) {
	sig := make([]byte, common.LeiosBlsSignatureSize)
	certCbor, err := cbor.Encode([]any{[]byte{0x01}, sig, uint64(3)})
	require.NoError(t, err)
	var cert DijkstraLeiosCertificate
	_, err = cbor.Decode(certCbor, &cert)
	require.Error(t, err)
}

func encodeAnnouncement(t *testing.T, hash []byte, size uint64) cbor.RawMessage {
	t.Helper()
	raw, err := cbor.Encode([]any{hash, size})
	require.NoError(t, err)
	return cbor.RawMessage(raw)
}

func encodeBool(t *testing.T, v bool) cbor.RawMessage {
	t.Helper()
	raw, err := cbor.Encode(v)
	require.NoError(t, err)
	return cbor.RawMessage(raw)
}

func TestDijkstraLeiosHeaderExtensionAccessors(t *testing.T) {
	hash := make([]byte, common.Blake2b256Size)
	for i := range hash {
		hash[i] = byte(0xa0 + i%16)
	}
	const size = uint64(4096)

	// Certified block that also announces an endorser block.
	h := &DijkstraBlockHeader{
		LeiosHeaderExtension: []cbor.RawMessage{
			encodeBool(t, true),
			encodeAnnouncement(t, hash, size),
		},
	}
	certified, present := h.LeiosCertified()
	assert.True(t, present)
	assert.True(t, certified)
	gotHash, gotSize, ok := h.LeiosAnnouncement()
	assert.True(t, ok)
	assert.Equal(t, hash, gotHash.Bytes())
	assert.Equal(t, size, gotSize)

	// Not certified, no announcement (leios_announcement = nil).
	hNil := &DijkstraBlockHeader{
		LeiosHeaderExtension: []cbor.RawMessage{
			encodeBool(t, false),
			cbor.RawMessage([]byte{0xf6}), // CBOR null
		},
	}
	certified, present = hNil.LeiosCertified()
	assert.True(t, present)
	assert.False(t, certified)
	_, _, ok = hNil.LeiosAnnouncement()
	assert.False(t, ok)

	// Pre-Leios-extension header: no extension at all.
	hLegacy := &DijkstraBlockHeader{}
	_, present = hLegacy.LeiosCertified()
	assert.False(t, present)
	_, _, ok = hLegacy.LeiosAnnouncement()
	assert.False(t, ok)
}

// prototype-2026w27 (IntersectMBO/cardano-ledger a24a2d69b) defines
// peras_certificate as an opaque byte string (peras_certificate = bytes;
// peras_certificate / nil), replacing the earlier empty-list placeholder.
func TestDijkstraPerasCertificateRoundTrip(t *testing.T) {
	// Present: an opaque byte string round-trips verbatim.
	body := DijkstraBlockBody{
		Transactions:     []DijkstraTransaction{},
		PerasCertificate: []byte{0xde, 0xad, 0xbe, 0xef},
	}
	raw, err := body.MarshalCBOR()
	require.NoError(t, err)
	var decoded DijkstraBlockBody
	require.NoError(t, decoded.UnmarshalCBOR(raw))
	assert.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, decoded.PerasCertificate)

	// Absent: a nil peras_certificate encodes as CBOR null and decodes to nil.
	bodyNil := DijkstraBlockBody{Transactions: []DijkstraTransaction{}}
	rawNil, err := bodyNil.MarshalCBOR()
	require.NoError(t, err)
	var items []cbor.RawMessage
	_, err = cbor.Decode(rawNil, &items)
	require.NoError(t, err)
	require.Len(t, items, 4)
	assert.Equal(t, []byte{0xf6}, []byte(items[3])) // peras field is CBOR null
	var decodedNil DijkstraBlockBody
	require.NoError(t, decodedNil.UnmarshalCBOR(rawNil))
	assert.Nil(t, decodedNil.PerasCertificate)
}

// TestDijkstraDecodeRealMusashiBlock decodes a block captured live from the
// respun ouroboros-leios prototype-2026w27 "musashi" testnet (network magic
// 164, fetched over node-to-node from leios-node.play.dev.cardano.org:3001 at
// slot 566037 / block 28091). It exercises the full Dijkstra wire format
// against real bytes: the two-element [header, block_body] envelope, the
// four-field block_body, and the 12-field Leios-extended header body.
//
// NewDijkstraBlockFromCbor verifies the block body hash against the header
// during parsing, so a successful decode proves the body-hash computation is
// correct for a real block. Musashi carries no transaction or endorser-block
// activity yet (txsProcessedNum stays 0 and no leios_certificate has appeared
// on-chain), so this representative block has no transactions and null
// certificate slots.
func TestDijkstraDecodeRealMusashiBlock(t *testing.T) {
	hexData, err := os.ReadFile("testdata/musashi_dijkstra_block.hex")
	require.NoError(t, err)
	raw, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	require.NoError(t, err)

	blk, err := NewDijkstraBlockFromCbor(raw)
	require.NoError(t, err)

	// Two-element [header, block_body] envelope with a four-field body.
	var top []cbor.RawMessage
	_, err = cbor.Decode(raw, &top)
	require.NoError(t, err)
	require.Len(t, top, 2)
	var body []cbor.RawMessage
	_, err = cbor.Decode(top[1], &body)
	require.NoError(t, err)
	require.Len(t, body, 4)

	// Header identity + Leios extension present (12-field header body).
	assert.Equal(t, uint64(566037), blk.SlotNumber())
	assert.Equal(t, uint64(28091), blk.BlockNumber())
	_, present := blk.BlockHeader.LeiosCertified()
	assert.True(t, present, "musashi headers carry the 12-field Leios extension")

	// Representative of the current txless, endorser-block-less chain.
	assert.Empty(t, blk.Transactions())
	assert.Nil(t, blk.BlockBody.LeiosCertificate)
	assert.Nil(t, blk.BlockBody.PerasCertificate)

	// Re-encoding a decoded block reproduces the exact wire bytes.
	remar, err := blk.MarshalCBOR()
	require.NoError(t, err)
	assert.Equal(t, raw, remar)
}
