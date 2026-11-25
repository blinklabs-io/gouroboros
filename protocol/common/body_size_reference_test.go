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
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/stretchr/testify/require"
)

// bodySizeMessage signs an otherwise valid message whose body is n bytes.
func bodySizeMessage(t *testing.T, n int) *DmqMessage {
	t.Helper()
	coldPub, coldPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	seed := make([]byte, kes.SeedSize)
	_, err = rand.Read(seed)
	require.NoError(t, err)
	sk, kesPub, err := kes.KeyGen(kes.CardanoKesDepth, seed)
	require.NoError(t, err)
	opcert := OperationalCertificate{
		KESVerificationKey: kesPub,
		IssueNumber:        1,
		KESPeriod:          100,
	}
	opcert.ColdSignature = ed25519.Sign(coldPriv, opCertSignableBytes(
		opcert.KESVerificationKey, opcert.IssueNumber, opcert.KESPeriod,
	))
	payload := DmqMessagePayload{
		MessageBody: bytes.Repeat([]byte{0x42}, n),
		KESPeriod:   100,
		ExpiresAt:   uint32(time.Now().Add(time.Hour).Unix()),
	}
	payloadCbor, err := cbor.Encode(payload)
	require.NoError(t, err)
	sig, err := kes.Sign(sk, 0, payloadCbor)
	require.NoError(t, err)
	msg := &DmqMessage{
		Payload:                payload,
		KESSignature:           sig,
		OperationalCertificate: opcert,
		ColdVerificationKey:    coldPub,
	}
	require.NoError(t, msg.SetComputedMessageID())
	return msg
}

// TestVerifyMessageBodySizeAsReference checks the message body size bounds:
// CIP-0137 (CIPs db48da14bff1, CIP-0137/README.md line 155) gives
// messageBody = bstr .size (90..2000), and dmq-node a267003c14e9 refuses
// any other size when it decodes a message
// (src/DMQ/Protocol/SigSubmission/Codec.hs 193-202, src/DMQ/Policy.hs 42-49).
// The cases are those bounds and their neighbours.
func TestVerifyMessageBodySizeAsReference(t *testing.T) {
	cases := []struct {
		size   int
		accept bool
	}{
		{0, false},
		{89, false},
		{90, true},
		{2000, true},
		{2001, false},
	}
	for _, tc := range cases {
		msg := bodySizeMessage(t, tc.size)
		stake := newStubStakeAuthority()
		auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
			StakeAuthority: stake,
			CurrentSlot: func() (uint64, error) {
				return 100 * 129600, nil
			},
			PoolOpCertIssueNumber: testPoolOpCertIssueNumber,
		})
		require.NoError(t, err)
		poolID, err := poolKeyHash(msg.ColdVerificationKey)
		require.NoError(t, err)
		stake.register(poolID, 1000)
		err = auth.VerifyMessage(msg)
		if tc.accept {
			require.NoError(t, err, "body of %d bytes", tc.size)
		} else {
			require.ErrorIs(
				t, err, ErrMessageBodySize, "body of %d bytes", tc.size,
			)
		}
	}
}

// TestDmqMessageDecodeRefusesBodySize checks that a message whose body is
// outside 90..2000 bytes fails to decode, as in dmq-node a267003c14e9
// src/DMQ/Protocol/SigSubmission/Codec.hs 193-202, where decodeBody fails
// before validateSig runs any check. The message here has also expired: the
// decoding failure still comes first, so a node-to-node receiver closes the
// connection instead of dropping it as expired.
func TestDmqMessageDecodeRefusesBodySize(t *testing.T) {
	for _, tc := range []struct {
		size   int
		decode bool
	}{
		{0, false},
		{89, false},
		{90, true},
		{2000, true},
		{2001, false},
	} {
		msg := bodySizeMessage(t, tc.size)
		msg.Payload.ExpiresAt = uint32(time.Now().Add(-time.Hour).Unix())
		require.NoError(t, msg.SetComputedMessageID())
		for _, legacy := range []bool{false, true} {
			var wire []byte
			var err error
			if legacy {
				wire, err = MarshalDmqMessageLegacyCBOR(*msg)
			} else {
				wire, err = cbor.Encode(msg)
			}
			require.NoError(t, err)
			var decoded DmqMessage
			_, err = cbor.Decode(wire, &decoded)
			if tc.decode {
				require.NoError(t, err, "body of %d bytes", tc.size)
			} else {
				require.ErrorIs(
					t, err, ErrMessageBodySize,
					"body of %d bytes, legacy %t", tc.size, legacy,
				)
			}
		}
	}
}
