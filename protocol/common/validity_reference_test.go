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
	"fmt"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/stretchr/testify/require"
)

// validityPool signs messages for one pool with any opcert counter.
type validityPool struct {
	coldPub  ed25519.PublicKey
	coldPriv ed25519.PrivateKey
	kesSK    *kes.SecretKey
	kesPub   []byte
}

func newValidityPool(t *testing.T) *validityPool {
	t.Helper()
	coldPub, coldPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	seed := make([]byte, kes.SeedSize)
	_, err = rand.Read(seed)
	require.NoError(t, err)
	sk, kesPub, err := kes.KeyGen(kes.CardanoKesDepth, seed)
	require.NoError(t, err)
	return &validityPool{coldPub, coldPriv, sk, kesPub}
}

// message signs a message at KES period 100, the opcert's start, carrying
// opcert counter issue. A forged message carries a cold signature that does
// not verify.
func (p *validityPool) message(
	t *testing.T,
	issue uint64,
	forged bool,
) DmqMessage {
	t.Helper()
	opcert := OperationalCertificate{
		KESVerificationKey: p.kesPub,
		IssueNumber:        issue,
		KESPeriod:          100,
	}
	opcert.ColdSignature = ed25519.Sign(p.coldPriv, opCertSignableBytes(
		opcert.KESVerificationKey, opcert.IssueNumber, opcert.KESPeriod,
	))
	if forged {
		opcert.ColdSignature = make([]byte, ed25519.SignatureSize)
	}
	payload := DmqMessagePayload{
		MessageBody: bytes.Repeat([]byte{byte(issue)}, 100),
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

// TestVerifyMessageCounterAndPoolAsReference walks every combination of the
// pool's recorded opcert counter (none, or 1), the message's counter (0, 1,
// 2), the pool's stake (registered or not), the cold signature (valid or
// forged) and the entry point (VerifyMessage, or the batch path through
// VerifyMessages), and checks both the verdict and the counter recorded afterwards
// against dmq-node a267003c14e9
// src/DMQ/Protocol/SigSubmission/Validate.hs:
//   - 167-177: the pool must hold stake in the current distribution;
//   - 183-197: the counter must be no lower than the last one seen for the
//     pool (equal is accepted);
//   - 203-205: the opcert's cold signature must verify;
//   - 96-100: a refused message leaves the recorded counters as they were,
//     and an accepted one records its counter.
//
// dmq-node ships no signed vectors for these rules; the cases are derived
// from that code and signed here.
func TestVerifyMessageCounterAndPoolAsReference(t *testing.T) {
	const never = -1
	for _, prior := range []int{never, 1} {
		for _, issue := range []uint64{0, 1, 2} {
			for _, staked := range []bool{true, false} {
				for _, forged := range []bool{false, true} {
					for _, batch := range []bool{false, true} {
						name := fmt.Sprintf(
							"prior=%d issue=%d staked=%t forged=%t batch=%t",
							prior, issue, staked, forged, batch,
						)
						t.Run(name, func(t *testing.T) {
							checkCounterAndPoolCell(
								t, prior, issue, staked, forged, batch,
							)
						})
					}
				}
			}
		}
	}
}

func checkCounterAndPoolCell(
	t *testing.T,
	prior int,
	issue uint64,
	staked, forged, batch bool,
) {
	stake := newStubStakeAuthority()
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
		CurrentSlot: func() (uint64, error) {
			return 100 * 129600, nil
		},
		PoolOpCertIssueNumber: testPoolOpCertIssueNumber,
	})
	require.NoError(t, err)
	pool := newValidityPool(t)
	poolID, err := poolKeyHash(pool.coldPub)
	require.NoError(t, err)
	if staked {
		stake.register(poolID, 1000)
	}
	if prior != -1 {
		auth.kesOpCertCache[poolID] = uint64(prior)
	}

	accept := staked && !forged && (prior == -1 || issue >= uint64(prior))
	msg := pool.message(t, issue, forged)
	if batch {
		err = auth.VerifyMessages([]DmqMessage{msg})
	} else {
		err = auth.VerifyMessage(&msg)
	}
	recorded, has := auth.kesOpCertCache[poolID]
	switch {
	case accept:
		require.NoError(t, err)
		require.True(t, has)
		require.Equal(t, issue, recorded)
	case prior == -1:
		require.Error(t, err)
		require.False(t, has, "a refused message recorded a counter")
	default:
		require.Error(t, err)
		require.Equal(t, uint64(prior), recorded)
	}
	if !staked {
		require.ErrorIs(t, err, ErrPoolNotInStakeDistribution)
	}
}
