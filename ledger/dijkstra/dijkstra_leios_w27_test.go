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
