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

// ratePool signs messages for one pool.
type ratePool struct {
	coldPub  ed25519.PublicKey
	coldPriv ed25519.PrivateKey
	kesSK    *kes.SecretKey
	kesPub   []byte
	id       PoolKeyHash
}

func newRatePool(t *testing.T, stake *stubStakeAuthority) *ratePool {
	t.Helper()
	coldPub, coldPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	seed := make([]byte, kes.SeedSize)
	_, err = rand.Read(seed)
	require.NoError(t, err)
	sk, kesPub, err := kes.KeyGen(kes.CardanoKesDepth, seed)
	require.NoError(t, err)
	id, err := poolKeyHash(coldPub)
	require.NoError(t, err)
	stake.register(id, 1000)
	return &ratePool{coldPub, coldPriv, sk, kesPub, id}
}

// message signs a distinct message (by tag) at KES period 100. A forged one
// carries a cold signature that does not verify.
func (p *ratePool) message(t *testing.T, tag byte, forged bool) DmqMessage {
	t.Helper()
	opcert := OperationalCertificate{
		KESVerificationKey: p.kesPub,
		IssueNumber:        1,
		KESPeriod:          100,
	}
	opcert.ColdSignature = ed25519.Sign(p.coldPriv, opCertSignableBytes(
		opcert.KESVerificationKey, opcert.IssueNumber, opcert.KESPeriod,
	))
	if forged {
		opcert.ColdSignature = make([]byte, ed25519.SignatureSize)
	}
	payload := DmqMessagePayload{
		MessageBody: bytes.Repeat([]byte{tag}, 100),
		KESPeriod:   100,
		ExpiresAt:   uint32(time.Now().Add(time.Hour).Unix()),
	}
	payloadCbor, err := cbor.Encode(payload)
	require.NoError(t, err)
	sig, err := kes.Sign(p.kesSK, 0, payloadCbor)
	require.NoError(t, err)
	msg := DmqMessage{
		Payload:                payload,
		KESSignature:           sig,
		OperationalCertificate: opcert,
		ColdVerificationKey:    p.coldPub,
	}
	require.NoError(t, msg.SetComputedMessageID())
	return msg
}

// rateClock is a settable clock for the authenticator.
type rateClock struct{ t time.Time }

func (c *rateClock) now() time.Time { return c.t }

func newRateAuthenticator(
	t *testing.T,
) (*MessageAuthenticator, *stubStakeAuthority, *rateClock) {
	t.Helper()
	stake := newStubStakeAuthority()
	clock := &rateClock{t: time.Unix(1_000_000, 0)}
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
		CurrentSlot: func() (uint64, error) {
			return 100 * 129600, nil
		},
		PoolOpCertIssueNumber: testPoolOpCertIssueNumber,
		Now:                   clock.now,
	})
	require.NoError(t, err)
	return auth, stake, clock
}

func verifyOne(auth *MessageAuthenticator, msg DmqMessage) error {
	return auth.VerifyMessages([]DmqMessage{msg})
}

// TestVerifyMessageRateLimitAsReference checks the per-pool rate limit
// against dmq-node a267003c14e9:
//   - src/DMQ/Protocol/SigSubmission/Validate.hs 124-145: a message is
//     refused (SigTooFrequent) when less than vcMinSigDelay has passed since
//     the pool's last accepted message; the time is recorded only when the
//     message is accepted, since a refused message leaves the validation
//     state unchanged (96-100);
//   - src/DMQ/Policy.hs 52-55: vcMinSigDelay is 60 s;
//   - ouroboros-network 4646039f Ouroboros/Network/TxSubmission/Mempool/
//     Simple.hs 183-245: one timestamp per batch, and a message already
//     accepted is a duplicate that is not validated again, so the limit
//     never refuses it.
//
// dmq-node ships no vectors for this rule; the cases are derived from that
// code and signed here.
func TestVerifyMessageRateLimitAsReference(t *testing.T) {
	t.Run("a second message within 60 s is refused", func(t *testing.T) {
		auth, stake, clock := newRateAuthenticator(t)
		pool := newRatePool(t, stake)
		require.NoError(t, verifyOne(auth, pool.message(t, 1, false)))
		clock.t = clock.t.Add(60*time.Second - time.Millisecond)
		require.ErrorIs(
			t, verifyOne(auth, pool.message(t, 2, false)), ErrMessageTooFrequent,
		)
		clock.t = clock.t.Add(time.Millisecond)
		require.NoError(t, verifyOne(auth, pool.message(t, 3, false)))
	})
	t.Run("a refused message records no time", func(t *testing.T) {
		auth, stake, clock := newRateAuthenticator(t)
		pool := newRatePool(t, stake)
		require.Error(t, verifyOne(auth, pool.message(t, 1, true)))
		clock.t = clock.t.Add(time.Second)
		require.NoError(t, verifyOne(auth, pool.message(t, 2, false)))
		clock.t = clock.t.Add(30 * time.Second)
		require.ErrorIs(
			t, verifyOne(auth, pool.message(t, 3, false)), ErrMessageTooFrequent,
		)
		clock.t = clock.t.Add(30 * time.Second)
		require.NoError(t, verifyOne(auth, pool.message(t, 4, false)))
	})
	t.Run("an accepted message verifies again", func(t *testing.T) {
		auth, stake, clock := newRateAuthenticator(t)
		pool := newRatePool(t, stake)
		first := pool.message(t, 1, false)
		require.NoError(t, verifyOne(auth, first))
		clock.t = clock.t.Add(time.Second)
		require.NoError(t, verifyOne(auth, first))
		require.NoError(t, auth.VerifyMessage(&first))
		// Verifying it again did not move the pool's last time.
		clock.t = clock.t.Add(59 * time.Second)
		require.NoError(t, verifyOne(auth, pool.message(t, 2, false)))
	})
	t.Run("pools are limited separately", func(t *testing.T) {
		auth, stake, _ := newRateAuthenticator(t)
		a := newRatePool(t, stake)
		b := newRatePool(t, stake)
		require.NoError(t, verifyOne(auth, a.message(t, 1, false)))
		require.NoError(t, verifyOne(auth, b.message(t, 1, false)))
	})
	t.Run("two messages of one pool in one batch", func(t *testing.T) {
		auth, stake, clock := newRateAuthenticator(t)
		pool := newRatePool(t, stake)
		require.ErrorIs(t, auth.VerifyMessages([]DmqMessage{
			pool.message(t, 1, false),
			pool.message(t, 2, false),
		}), ErrMessageTooFrequent)
		clock.t = clock.t.Add(time.Second)
		require.NoError(t, verifyOne(auth, pool.message(t, 3, false)))
	})
	t.Run("concurrent submissions of one pool", func(t *testing.T) {
		auth, stake, _ := newRateAuthenticator(t)
		pool := newRatePool(t, stake)
		commitA, err := auth.PrepareMessages(
			[]DmqMessage{pool.message(t, 1, false)},
		)
		require.NoError(t, err)
		commitB, err := auth.PrepareMessages(
			[]DmqMessage{pool.message(t, 2, false)},
		)
		require.NoError(t, err)
		require.NoError(t, commitA())
		require.ErrorIs(t, commitB(), ErrMessageTooFrequent)
	})
	t.Run("concurrent submissions of one message", func(t *testing.T) {
		auth, stake, clock := newRateAuthenticator(t)
		pool := newRatePool(t, stake)
		msg := pool.message(t, 1, false)
		commitA, err := auth.PrepareMessages([]DmqMessage{msg})
		require.NoError(t, err)
		commitB, err := auth.PrepareMessages([]DmqMessage{msg})
		require.NoError(t, err)
		require.NoError(t, commitA())
		require.NoError(t, commitB())
		clock.t = clock.t.Add(60 * time.Second)
		require.NoError(t, verifyOne(auth, pool.message(t, 2, false)))
	})
}

// TestVerifyMessageRateLimitOff checks that a negative MinSigDelay turns the
// limit off everywhere, as a vcMinSigDelay of zero does in dmq-node
// (Validate.hs 124-145): a batch prepared before another message of its pool
// was accepted still commits, although its time precedes that message's.
func TestVerifyMessageRateLimitOff(t *testing.T) {
	stake := newStubStakeAuthority()
	clock := &rateClock{t: time.Unix(1_000_000, 0)}
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
		CurrentSlot: func() (uint64, error) {
			return 100 * 129600, nil
		},
		PoolOpCertIssueNumber: testPoolOpCertIssueNumber,
		MinSigDelay:           -1,
		Now:                   clock.now,
	})
	require.NoError(t, err)
	pool := newRatePool(t, stake)
	commit, err := auth.PrepareMessages(
		[]DmqMessage{pool.message(t, 1, false)},
	)
	require.NoError(t, err)
	clock.t = clock.t.Add(time.Second)
	require.NoError(t, verifyOne(auth, pool.message(t, 2, false)))
	require.NoError(t, commit())
}
