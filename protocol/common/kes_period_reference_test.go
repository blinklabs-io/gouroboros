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

// kesPeriodMessage builds a message whose opcert starts at certPeriod, whose
// payload claims claimedPeriod, and whose KES signature is made with the key
// evolved signedEvolution times. An honest signer uses
// signedEvolution = claimedPeriod - certPeriod.
func kesPeriodMessage(
	t *testing.T,
	certPeriod, claimedPeriod, signedEvolution uint64,
) *DmqMessage {
	t.Helper()
	coldPub, coldPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	seed := make([]byte, kes.SeedSize)
	_, err = rand.Read(seed)
	require.NoError(t, err)
	sk, kesPub, err := kes.KeyGen(kes.CardanoKesDepth, seed)
	require.NoError(t, err)
	for range signedEvolution {
		sk, err = kes.Update(sk)
		require.NoError(t, err)
	}
	opcert := OperationalCertificate{
		KESVerificationKey: kesPub,
		IssueNumber:        1,
		KESPeriod:          certPeriod,
	}
	opcert.ColdSignature = ed25519.Sign(coldPriv, opCertSignableBytes(
		opcert.KESVerificationKey,
		opcert.IssueNumber,
		opcert.KESPeriod,
	))
	payload := DmqMessagePayload{
		MessageBody: bytes.Repeat([]byte{0x42}, 100),
		KESPeriod:   claimedPeriod,
		ExpiresAt:   uint32(time.Now().Add(time.Hour).Unix()),
	}
	payloadCbor, err := cbor.Encode(payload)
	require.NoError(t, err)
	sig, err := kes.Sign(sk, signedEvolution, payloadCbor)
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

// TestVerifyMessageKESPeriodAsReference checks which KES evolution a message's
// signature is verified at, against dmq-node a267003c14e9
// src/DMQ/Protocol/SigSubmission/Validate.hs: the claimed period must lie in
// [ocertKESPeriod, ocertKESPeriod + maxKESEvo) (lines 156-161) and the
// signature is verified at sigKESPeriod - ocertKESPeriod (lines 207-212). The
// node's current slot plays no part there. dmq-node ships no signed vectors
// for this rule, so the cases are derived from that code and signed here.
//
// Two rows are gOuroboros's own, stricter checks: it also refuses a message
// whose opcert has not started, or has ended, at the node's current KES
// period. The reference accepts both.
func TestVerifyMessageKESPeriodAsReference(t *testing.T) {
	const (
		maxEvo = 3
		cert   = 100
	)
	cases := []struct {
		name      string
		claimed   uint64
		evolution uint64
		current   uint64
		accept    bool
	}{
		{"signed at claimed period, same period now", 101, 1, 101, true},
		{"signed at claimed period, next period now", 101, 1, 102, true},
		{"signed at claimed period, earlier period now", 101, 1, 100, true},
		{"first period of the opcert", 100, 0, 100, true},
		{"last period of the opcert", 102, 2, 102, true},
		{"signed at current period, claims an earlier one", 101, 2, 102, false},
		{"signed at an evolution matching neither", 101, 0, 102, false},
		{"claimed period before the opcert", 99, 0, 100, false},
		{"claimed period after the opcert", 103, 3, 102, false},
		// gOuroboros only: the opcert has ended at the current period.
		{"opcert ended at the current period", 102, 2, 103, false},
		// gOuroboros only: the opcert has not started at the current period.
		{"opcert not started at the current period", 100, 0, 99, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stake := newStubStakeAuthority()
			auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
				StakeAuthority: stake,
				CurrentSlot: func() (uint64, error) {
					return tc.current * 129600, nil
				},
				PoolOpCertIssueNumber: testPoolOpCertIssueNumber,
				MaxKESEvolutions:      maxEvo,
			})
			require.NoError(t, err)
			msg := kesPeriodMessage(t, cert, tc.claimed, tc.evolution)
			poolID, err := poolKeyHash(msg.ColdVerificationKey)
			require.NoError(t, err)
			stake.register(poolID, 1000)
			err = auth.VerifyMessage(msg)
			if tc.accept {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
