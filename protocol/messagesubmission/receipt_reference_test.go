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

package messagesubmission

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"
)

// receiptStake reports stake for registered pools; lookups fail while
// unavailable is set.
type receiptStake struct {
	registered  map[pcommon.PoolKeyHash]bool
	unavailable bool
}

func (s *receiptStake) PoolActiveStake(
	pool pcommon.PoolKeyHash,
) (uint64, error) {
	if s.unavailable {
		return 0, errors.New("stake distribution not loaded")
	}
	if s.registered[pool] {
		return 1, nil
	}
	return 0, nil
}

// receiptMessage describes one message of a reply: whether its pool is
// registered, whether it has expired, whether its cold signature is forged,
// and whether it claims a KES period past its opcert's window.
type receiptMessage struct {
	unregistered bool
	expired      bool
	forged       bool
	pastWindow   bool
}

// signReceiptMessage signs a message for a fresh pool, registering the pool
// with stake unless the description says otherwise.
func signReceiptMessage(
	t *testing.T,
	stake *receiptStake,
	desc receiptMessage,
	tag byte,
) pcommon.DmqMessage {
	t.Helper()
	coldPub, coldPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	seed := make([]byte, kes.SeedSize)
	_, err = rand.Read(seed)
	require.NoError(t, err)
	kesKey, kesPub, err := kes.KeyGen(kes.CardanoKesDepth, seed)
	require.NoError(t, err)
	if !desc.unregistered {
		h, err := blake2b.New(pcommon.PoolKeyHashSize, nil)
		require.NoError(t, err)
		h.Write(coldPub)
		var id pcommon.PoolKeyHash
		copy(id[:], h.Sum(nil))
		stake.registered[id] = true
	}
	expiresAt := time.Now().Add(10 * time.Minute)
	if desc.expired {
		expiresAt = time.Now().Add(-time.Minute)
	}
	kesPeriod := uint64(1)
	if desc.pastWindow {
		kesPeriod = 1 + pcommon.DefaultMaxKESEvolutions
	}
	payload := pcommon.DmqMessagePayload{
		MessageBody: bytes.Repeat([]byte{tag}, 100),
		KESPeriod:   kesPeriod,
		ExpiresAt:   uint32(expiresAt.Unix()),
	}
	payloadCbor, err := cbor.Encode(payload)
	require.NoError(t, err)
	kesSignature, err := kes.Sign(kesKey, 0, payloadCbor)
	require.NoError(t, err)
	signable := append([]byte{}, kesPub...)
	signable = binary.BigEndian.AppendUint64(signable, 1)
	signable = binary.BigEndian.AppendUint64(signable, 1)
	coldSignature := ed25519.Sign(coldPriv, signable)
	if desc.forged {
		coldSignature = make([]byte, ed25519.SignatureSize)
	}
	msg := pcommon.DmqMessage{
		Payload:      payload,
		KESSignature: kesSignature,
		OperationalCertificate: pcommon.OperationalCertificate{
			KESVerificationKey: kesPub,
			IssueNumber:        1,
			KESPeriod:          1,
			ColdSignature:      coldSignature,
		},
		ColdVerificationKey: coldPub,
	}
	require.NoError(t, msg.SetComputedMessageID())
	return msg
}

// TestServerReceivesRepliesAsReference checks what the inbound side does
// with each message of a reply, against dmq-node a267003c14e9:
//   - app/Main.hs 199-221: for a node-to-node peer, SigExpired,
//     PoolNotEligible, UnrecognizedPool, NotInitialized and SigDuplicate are
//     only traced; any other validation error is thrown, closing the
//     connection;
//   - ouroboros-network 4646039f Ouroboros/Network/TxSubmission/Mempool/
//     Simple.hs 183-245: every message is validated on its own, and the
//     valid ones are added before the invalid ones are handled, so a fatal
//     message does not cost the valid ones beside it;
//   - src/DMQ/Protocol/SigSubmission/Validate.hs 115-212: the first failing
//     check names the error, in the order expiry, KES window, pool, counter,
//     signatures, so a message both past its opcert window and from an
//     unregistered pool is fatal.
//
// dmq-node ships no vectors for this; the cases are derived from that code
// and signed here.
func TestServerReceivesRepliesAsReference(t *testing.T) {
	valid := receiptMessage{}
	cases := []struct {
		name        string
		reply       []receiptMessage
		unavailable bool
		delivered   []int
		fatal       bool
	}{
		{"all valid", []receiptMessage{valid, valid}, false, []int{0, 1}, false},
		{"an expired one is dropped", []receiptMessage{valid, {expired: true}}, false, []int{0}, false},
		{"an unregistered pool's is dropped", []receiptMessage{{unregistered: true}, valid}, false, []int{1}, false},
		{"no stake distribution yet", []receiptMessage{valid}, true, nil, false},
		{"a forged one disconnects after the valid one", []receiptMessage{valid, {forged: true}}, false, []int{0}, true},
		{"past the window from an unregistered pool", []receiptMessage{{unregistered: true, pastWindow: true}}, false, nil, true},
		{"expired and forged is only expired", []receiptMessage{{expired: true, forged: true}}, false, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stake := &receiptStake{
				registered:  make(map[pcommon.PoolKeyHash]bool),
				unavailable: tc.unavailable,
			}
			messages := make([]pcommon.DmqMessage, len(tc.reply))
			for i, desc := range tc.reply {
				messages[i] = signReceiptMessage(t, stake, desc, byte(i))
			}
			auth, err := pcommon.NewMessageAuthenticator(
				pcommon.MessageAuthenticatorConfig{
					StakeAuthority: stake,
					CurrentSlot: func() (uint64, error) {
						return 129600, nil
					},
					PoolOpCertIssueNumber: func(
						pcommon.PoolKeyHash,
					) (uint64, bool, error) {
						return 0, false, nil
					},
				},
			)
			require.NoError(t, err)
			var got [][]byte
			cfg := NewConfig(
				WithAuthenticator(auth),
				WithReplyMessagesFunc(
					func(_ CallbackContext, msgs []pcommon.DmqMessage) {
						for i := range msgs {
							got = append(got, msgs[i].ID())
						}
					},
				),
			)
			server := NewServer(protocol.ProtocolOptions{}, &cfg)
			ids := make(map[string]struct{}, len(messages))
			for i := range messages {
				ids[string(messages[i].ID())] = struct{}{}
			}
			server.pendingMsgRequest = &messageRequest{ids: ids}

			err = server.handleReplyMessages(NewMsgReplyMessages(protocol.ProtocolVersionDMQNtN1, messages))
			if tc.fatal {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			var want [][]byte
			for _, i := range tc.delivered {
				want = append(want, messages[i].ID())
			}
			require.Equal(t, want, got)
		})
	}
}
