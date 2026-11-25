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

package common

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"
)

// TestVerifyMessageHashesPayloadBytesAsReceived builds a message whose
// payload encodes its KES period with a non-minimal width (0x1b and eight
// bytes for 100), signs those exact bytes and takes the message ID over them.
// dmq-node a267003c14e9 accepts it: src/DMQ/Protocol/SigSubmission/Codec.hs
// 169-181 cuts sigSignedBytes from the received message between two decoder
// offsets, Validate.hs 60-64 hashes those bytes for the ID, and 207-212
// verifies the KES signature over them; the KES period is read with cborg's
// decodeWord, which accepts a non-minimal width. Re-encoding the payload
// instead produces the minimal form, a different ID and different signed
// bytes. The message must also encode back to the bytes it was decoded from,
// so that relaying it does not change its ID.
func TestVerifyMessageHashesPayloadBytesAsReceived(t *testing.T) {
	coldPub, coldPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	seed := make([]byte, kes.SeedSize)
	_, err = rand.Read(seed)
	require.NoError(t, err)
	sk, kesPub, err := kes.KeyGen(kes.CardanoKesDepth, seed)
	require.NoError(t, err)

	body := bytes.Repeat([]byte{0x42}, 100)
	expiresAt := uint32(time.Now().Add(time.Hour).Unix())
	payload := []byte{0x83, 0x58, byte(len(body))}
	payload = append(payload, body...)
	payload = append(payload, 0x1b)
	payload = binary.BigEndian.AppendUint64(payload, 100)
	payload = append(payload, 0x1a)
	payload = binary.BigEndian.AppendUint32(payload, expiresAt)

	sig, err := kes.Sign(sk, 0, payload)
	require.NoError(t, err)
	id := blake2b.Sum256(payload)
	opcert := OperationalCertificate{
		KESVerificationKey: kesPub,
		IssueNumber:        1,
		KESPeriod:          100,
	}
	opcert.ColdSignature = ed25519.Sign(coldPriv, opCertSignableBytes(
		opcert.KESVerificationKey, opcert.IssueNumber, opcert.KESPeriod,
	))
	opcertCbor, err := cbor.Encode(opcert)
	require.NoError(t, err)
	idCbor, err := cbor.Encode(id[:])
	require.NoError(t, err)
	sigCbor, err := cbor.Encode(sig)
	require.NoError(t, err)
	coldCbor, err := cbor.Encode([]byte(coldPub))
	require.NoError(t, err)
	wire := []byte{0x85}
	wire = append(wire, idCbor...)
	wire = append(wire, payload...)
	wire = append(wire, sigCbor...)
	wire = append(wire, opcertCbor...)
	wire = append(wire, coldCbor...)

	var msg DmqMessage
	_, err = cbor.Decode(wire, &msg)
	require.NoError(t, err)
	require.Equal(t, uint64(100), msg.Payload.KESPeriod)

	stake := newStubStakeAuthority()
	poolID, err := poolKeyHash(coldPub)
	require.NoError(t, err)
	stake.register(poolID, 1000)
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
		CurrentSlot: func() (uint64, error) {
			return 100 * 129600, nil
		},
		PoolOpCertIssueNumber: testPoolOpCertIssueNumber,
	})
	require.NoError(t, err)
	require.NoError(t, auth.VerifyMessage(&msg))

	computed, err := ComputeDmqMessageID(msg.Payload)
	require.NoError(t, err)
	require.Equal(t, id[:], computed)
	reencoded, err := cbor.Encode(msg)
	require.NoError(t, err)
	require.Equal(t, wire, reencoded)
}
