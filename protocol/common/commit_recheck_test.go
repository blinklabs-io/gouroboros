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

// recheckPool signs messages for one pool: one cold key and one KES key,
// with any opcert counter.
type recheckPool struct {
	coldPub  ed25519.PublicKey
	coldPriv ed25519.PrivateKey
	kesSK    *kes.SecretKey
	kesPub   []byte
}

func newRecheckPool(t *testing.T) *recheckPool {
	t.Helper()
	coldPub, coldPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	seed := make([]byte, kes.SeedSize)
	_, err = rand.Read(seed)
	require.NoError(t, err)
	sk, kesPub, err := kes.KeyGen(kes.CardanoKesDepth, seed)
	require.NoError(t, err)
	return &recheckPool{coldPub, coldPriv, sk, kesPub}
}

// message signs a message at KES period 100 (the opcert's start) carrying
// opcert counter issue; tag makes each message's ID distinct.
func (p *recheckPool) message(t *testing.T, issue uint64, tag byte) DmqMessage {
	t.Helper()
	opcert := OperationalCertificate{
		KESVerificationKey: p.kesPub,
		IssueNumber:        issue,
		KESPeriod:          100,
	}
	opcert.ColdSignature = ed25519.Sign(p.coldPriv, opCertSignableBytes(
		opcert.KESVerificationKey, opcert.IssueNumber, opcert.KESPeriod,
	))
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

// TestPrepareMessagesCommitRechecksEveryCounter prepares a batch carrying a
// pool's opcert counters 1 then 2, records counter 2 for that pool from
// another message, and then commits the batch. dmq-node a267003c14e9
// src/DMQ/Protocol/SigSubmission/Validate.hs 183-197 checks each message's
// counter against the counters recorded when it is validated, so a counter-1
// message after counter 2 is recorded is refused (InvalidOCertCounter). The
// commit must refuse the batch rather than accept the counter-1 message.
func TestPrepareMessagesCommitRechecksEveryCounter(t *testing.T) {
	stake := newStubStakeAuthority()
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
		CurrentSlot: func() (uint64, error) {
			return 100 * 129600, nil
		},
		PoolOpCertIssueNumber: testPoolOpCertIssueNumber,
	})
	require.NoError(t, err)
	pool := newRecheckPool(t)
	poolID, err := poolKeyHash(pool.coldPub)
	require.NoError(t, err)
	stake.register(poolID, 1000)

	commit, err := auth.PrepareMessages([]DmqMessage{
		pool.message(t, 1, 0x01),
		pool.message(t, 2, 0x02),
	})
	require.NoError(t, err)

	require.NoError(t, auth.VerifyMessages([]DmqMessage{
		pool.message(t, 2, 0x03),
	}))

	require.ErrorContains(t, commit(), "went backwards")
	require.Equal(t, uint64(2), auth.kesOpCertCache[poolID])

	// A batch whose every counter is still current commits.
	commit, err = auth.PrepareMessages([]DmqMessage{
		pool.message(t, 2, 0x04),
		pool.message(t, 3, 0x05),
	})
	require.NoError(t, err)
	require.NoError(t, commit())
	require.Equal(t, uint64(3), auth.kesOpCertCache[poolID])
}
