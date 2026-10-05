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

package localmessagenotification

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

type clientTestStakeAuthority struct{}

func (clientTestStakeAuthority) PoolActiveStake(
	pcommon.PoolKeyHash,
) (uint64, error) {
	return 1, nil
}

func clientTestMessage(t *testing.T, body string, expiresAt uint32) pcommon.DmqMessage {
	t.Helper()
	msg := pcommon.DmqMessage{
		Payload: pcommon.DmqMessagePayload{
			MessageBody: []byte(body),
			KESPeriod:   1,
			ExpiresAt:   expiresAt,
		},
	}
	require.NoError(t, msg.SetComputedMessageID())
	return msg
}

func clientTestAuthenticator(t *testing.T) *pcommon.MessageAuthenticator {
	t.Helper()
	auth, err := pcommon.NewMessageAuthenticator(
		pcommon.MessageAuthenticatorConfig{
			StakeAuthority: clientTestStakeAuthority{},
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
	return auth
}

func clientTestSignedMessages(
	t *testing.T,
	bodies []string,
	issueNumbers []uint64,
	expiresAt []uint32,
) ([]pcommon.DmqMessage, *pcommon.MessageAuthenticator) {
	t.Helper()
	require.Len(t, issueNumbers, len(bodies))
	require.Len(t, expiresAt, len(bodies))
	coldPub, coldPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	seed := make([]byte, kes.SeedSize)
	_, err = rand.Read(seed)
	require.NoError(t, err)
	kesKey, kesPub, err := kes.KeyGen(kes.CardanoKesDepth, seed)
	require.NoError(t, err)

	messages := make([]pcommon.DmqMessage, len(bodies))
	for i := range bodies {
		payload := pcommon.DmqMessagePayload{
			MessageBody: []byte(bodies[i]),
			KESPeriod:   1,
			ExpiresAt:   expiresAt[i],
		}
		payloadCBOR, err := cbor.Encode(payload)
		require.NoError(t, err)
		wrappedCBOR, err := cbor.Encode(payloadCBOR)
		require.NoError(t, err)
		kesSignature, err := kes.Sign(kesKey, 0, wrappedCBOR)
		require.NoError(t, err)
		signable := make([]byte, 0, len(kesPub)+16)
		signable = append(signable, kesPub...)
		issueBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(issueBytes, issueNumbers[i])
		signable = append(signable, issueBytes...)
		periodBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(periodBytes, 1)
		signable = append(signable, periodBytes...)
		messages[i] = pcommon.DmqMessage{
			Payload:      payload,
			KESSignature: kesSignature,
			OperationalCertificate: pcommon.OperationalCertificate{
				KESVerificationKey: kesPub,
				IssueNumber:        issueNumbers[i],
				KESPeriod:          1,
				ColdSignature:      ed25519.Sign(coldPriv, signable),
			},
			ColdVerificationKey: coldPub,
		}
		require.NoError(t, messages[i].SetComputedMessageID())
	}
	authenticator, err := pcommon.NewMessageAuthenticator(
		pcommon.MessageAuthenticatorConfig{
			StakeAuthority: clientTestStakeAuthority{},
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
	return messages, authenticator
}

func testReplyMessages(
	blocking bool,
	messages []pcommon.DmqMessage,
) protocol.Message {
	if blocking {
		return NewMsgReplyMessagesBlocking(messages)
	}
	return NewMsgReplyMessagesNonBlocking(messages, false)
}

func TestClientRejectsForgedAndExpiredRepliesBeforeCallback(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		name          string
		blocking      bool
		message       pcommon.DmqMessage
		authenticator *pcommon.MessageAuthenticator
	}{
		{
			name:          "forged nonblocking",
			message:       clientTestMessage(t, "forged", uint32(now.Add(time.Minute).Unix())),
			authenticator: clientTestAuthenticator(t),
		},
		{
			name:          "forged blocking",
			blocking:      true,
			message:       clientTestMessage(t, "forged", uint32(now.Add(time.Minute).Unix())),
			authenticator: clientTestAuthenticator(t),
		},
		{
			name:          "expired nonblocking",
			message:       clientTestMessage(t, "expired", uint32(now.Add(-time.Minute).Unix())),
			authenticator: pcommon.NewNoOpAuthenticator(nil),
		},
		{
			name:          "expired blocking",
			blocking:      true,
			message:       clientTestMessage(t, "expired", uint32(now.Add(-time.Minute).Unix())),
			authenticator: pcommon.NewNoOpAuthenticator(nil),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var callbacks atomic.Int32
			cfg := NewConfig(
				WithAuthenticator(test.authenticator),
				WithReplyMessagesFunc(func(CallbackContext, []pcommon.DmqMessage, bool) {
					callbacks.Add(1)
				}),
			)
			client := NewClient(protocol.ProtocolOptions{}, &cfg)

			err := client.messageHandler(testReplyMessages(
				test.blocking,
				[]pcommon.DmqMessage{test.message},
			))
			require.Error(t, err)
			require.Zero(t, callbacks.Load())
			require.Empty(t, client.replayState.acceptedIDs)
		})
	}
}

func TestClientConcurrentReplayInvokesCallbackOnce(t *testing.T) {
	const attempts = 32
	var callbacks atomic.Int32
	cfg := NewConfig(
		WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
		WithReplyMessagesFunc(func(CallbackContext, []pcommon.DmqMessage, bool) {
			callbacks.Add(1)
		}),
	)
	client := NewClient(protocol.ProtocolOptions{}, &cfg)
	msg := clientTestMessage(
		t,
		"replay",
		uint32(time.Now().Add(time.Minute).Unix()),
	)

	var wg sync.WaitGroup
	errs := make(chan error, attempts)
	for i := range attempts {
		wg.Add(1)
		go func(blocking bool) {
			defer wg.Done()
			errs <- client.messageHandler(testReplyMessages(
				blocking,
				[]pcommon.DmqMessage{msg},
			))
		}(i%2 == 0)
	}
	wg.Wait()
	close(errs)

	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, int32(1), callbacks.Load())
	require.Len(t, client.replayState.acceptedIDs, 1)
}

func TestClientBatchFailureDoesNotReserveEarlierMessages(t *testing.T) {
	var callbacks atomic.Int32
	cfg := NewConfig(
		WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
		WithReplyMessagesFunc(func(CallbackContext, []pcommon.DmqMessage, bool) {
			callbacks.Add(1)
		}),
	)
	client := NewClient(protocol.ProtocolOptions{}, &cfg)
	now := time.Now()
	valid := clientTestMessage(t, "valid", uint32(now.Add(time.Minute).Unix()))
	expired := clientTestMessage(t, "expired", uint32(now.Add(-time.Minute).Unix()))

	require.Error(t, client.messageHandler(
		NewMsgReplyMessagesNonBlocking(
			[]pcommon.DmqMessage{valid, expired},
			false,
		),
	))
	require.Zero(t, len(client.replayState.acceptedIDs))
	require.Zero(t, callbacks.Load())

	require.NoError(t, client.messageHandler(
		NewMsgReplyMessagesNonBlocking([]pcommon.DmqMessage{valid}, false),
	))
	require.Equal(t, int32(1), callbacks.Load())
}

func TestClientReplayCacheCapAndExpiration(t *testing.T) {
	cfg := NewConfig(WithMaxReplayEntries(1))
	client := NewClient(protocol.ProtocolOptions{}, &cfg)
	first := clientTestMessage(t, "first", 100)
	second := clientTestMessage(t, "second", 200)

	require.NoError(t, client.commitAcceptedMessages(
		[]pcommon.DmqMessage{first},
		time.Unix(100, 0),
		nil,
	))
	require.ErrorContains(t, client.commitAcceptedMessages(
		[]pcommon.DmqMessage{second},
		time.Unix(100, 0),
		nil,
	), "replay cache full")
	require.Len(t, client.replayState.acceptedIDs, 1)

	require.NoError(t, client.commitAcceptedMessages(
		[]pcommon.DmqMessage{second},
		time.Unix(101, 0),
		nil,
	))
	require.Len(t, client.replayState.acceptedIDs, 1)
	require.Contains(t, client.replayState.acceptedIDs, string(second.ID()))
}

func TestClientReplayCacheUsesBoundForZeroValueConfig(t *testing.T) {
	cfg := Config{}
	client := NewClient(protocol.ProtocolOptions{}, &cfg)
	msg := clientTestMessage(t, "zero value config", 100)

	require.NoError(t, client.commitAcceptedMessages(
		[]pcommon.DmqMessage{msg},
		time.Unix(100, 0),
		nil,
	))
	require.Len(t, client.replayState.acceptedIDs, 1)
}

func TestClientReplayReservationSurvivesReconstruction(t *testing.T) {
	var callbacks atomic.Int32
	cfg := NewConfig(
		WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
		WithReplyMessagesFunc(
			func(CallbackContext, []pcommon.DmqMessage, bool) {
				callbacks.Add(1)
			},
		),
	)
	msg := clientTestMessage(
		t,
		"reconnect replay",
		uint32(time.Now().Add(time.Minute).Unix()),
	)
	firstClient := NewClient(protocol.ProtocolOptions{}, &cfg)
	require.NoError(t, firstClient.messageHandler(
		NewMsgReplyMessagesNonBlocking([]pcommon.DmqMessage{msg}, false),
	))

	secondClient := NewClient(protocol.ProtocolOptions{}, &cfg)
	require.ErrorContains(t, secondClient.messageHandler(
		NewMsgReplyMessagesNonBlocking([]pcommon.DmqMessage{msg}, false),
	), "already accepted")
	require.Equal(t, int32(1), callbacks.Load())
}

func TestClientRechecksTTLAfterAuthentication(t *testing.T) {
	var callbacks atomic.Int32
	cfg := NewConfig(
		WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
		WithReplyMessagesFunc(
			func(CallbackContext, []pcommon.DmqMessage, bool) {
				callbacks.Add(1)
			},
		),
	)
	client := NewClient(protocol.ProtocolOptions{}, &cfg)
	times := []time.Time{time.Unix(100, 0), time.Unix(101, 0)}
	client.now = func() time.Time {
		now := times[0]
		times = times[1:]
		return now
	}
	msg := clientTestMessage(t, "expires during authentication", 100)

	require.ErrorContains(t, client.messageHandler(
		NewMsgReplyMessagesNonBlocking([]pcommon.DmqMessage{msg}, false),
	), "TTL validation failed")
	require.Zero(t, callbacks.Load())
	require.Empty(t, client.replayState.acceptedIDs)
}

func TestClientReplayRejectionDoesNotCommitHigherOpCert(t *testing.T) {
	messages, authenticator := clientTestSignedMessages(
		t,
		[]string{"same", "same", "later"},
		[]uint64{1, 2, 1},
		[]uint32{200, 200, 200},
	)
	cfg := NewConfig(WithAuthenticator(authenticator))
	client := NewClient(protocol.ProtocolOptions{}, &cfg)
	client.now = func() time.Time { return time.Unix(100, 0) }

	require.NoError(t, client.validateAndReserve(messages[:1]))
	require.ErrorContains(t, client.validateAndReserve(messages[1:2]), "already accepted")
	delete(client.replayState.acceptedIDs, string(messages[0].ID()))
	require.NoError(t, client.validateAndReserve(messages[2:]))
}

func TestClientExpiryRejectionDoesNotCommitHigherOpCert(t *testing.T) {
	messages, authenticator := clientTestSignedMessages(
		t,
		[]string{"expires", "later"},
		[]uint64{2, 1},
		[]uint32{100, 200},
	)
	cfg := NewConfig(WithAuthenticator(authenticator))
	client := NewClient(protocol.ProtocolOptions{}, &cfg)
	times := []time.Time{
		time.Unix(100, 0), time.Unix(101, 0),
		time.Unix(101, 0), time.Unix(101, 0),
	}
	client.now = func() time.Time {
		now := times[0]
		times = times[1:]
		return now
	}

	require.ErrorContains(t, client.validateAndReserve(messages[:1]), "TTL validation failed")
	require.NoError(t, client.validateAndReserve(messages[1:]))
}

func TestClientReplayCapRejectionDoesNotCommitHigherOpCert(t *testing.T) {
	messages, authenticator := clientTestSignedMessages(
		t,
		[]string{"first", "higher", "later"},
		[]uint64{1, 2, 1},
		[]uint32{100, 200, 200},
	)
	cfg := NewConfig(
		WithAuthenticator(authenticator),
		WithMaxReplayEntries(1),
	)
	client := NewClient(protocol.ProtocolOptions{}, &cfg)
	client.now = func() time.Time { return time.Unix(100, 0) }

	require.NoError(t, client.validateAndReserve(messages[:1]))
	require.ErrorContains(t, client.validateAndReserve(messages[1:2]), "replay cache full")
	client.now = func() time.Time { return time.Unix(101, 0) }
	require.NoError(t, client.validateAndReserve(messages[2:]))
}
