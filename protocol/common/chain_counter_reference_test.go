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

// chainCounterMessage signs a message for a fresh pool, at KES period 100,
// carrying opcert counter issue.
func chainCounterMessage(t *testing.T, issue uint64) *DmqMessage {
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
		IssueNumber:        issue,
		KESPeriod:          100,
	}
	opcert.ColdSignature = ed25519.Sign(coldPriv, opCertSignableBytes(
		opcert.KESVerificationKey, opcert.IssueNumber, opcert.KESPeriod,
	))
	payload := DmqMessagePayload{
		MessageBody: bytes.Repeat([]byte{0x42}, 100),
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

// TestVerifyMessageIgnoresChainCounter checks that the opcert counter is
// compared only with counters seen in earlier messages, as in dmq-node
// a267003c14e9 src/DMQ/Protocol/SigSubmission/Validate.hs 183-197: the first
// message from a pool is accepted whatever its counter, even one below the
// pool's counter in the chain. CIP-0137 (CIPs db48da14bff1,
// CIP-0137/README.md 397-402) does not require a chain comparison; its note at
// 405-411 lists the chain as one possible source. A configured
// PoolOpCertIssueNumber is not consulted.
func TestVerifyMessageIgnoresChainCounter(t *testing.T) {
	stake := newStubStakeAuthority()
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
		CurrentSlot: func() (uint64, error) {
			return 100 * 129600, nil
		},
		PoolOpCertIssueNumber: func(PoolKeyHash) (uint64, bool, error) {
			return 5, true, nil
		},
	})
	require.NoError(t, err)
	msg := chainCounterMessage(t, 1)
	poolID, err := poolKeyHash(msg.ColdVerificationKey)
	require.NoError(t, err)
	stake.register(poolID, 1000)
	require.NoError(t, auth.VerifyMessage(msg))
	require.Equal(t, uint64(1), auth.kesOpCertCache[poolID])

	// Without a chain lookup configured the authenticator is still usable.
	auth, err = NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
		CurrentSlot: func() (uint64, error) {
			return 100 * 129600, nil
		},
	})
	require.NoError(t, err)
	require.NoError(t, auth.VerifyMessage(msg))
}
