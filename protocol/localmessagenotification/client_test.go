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
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
			require.Empty(t, client.acceptedIDs)
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
	require.Len(t, client.acceptedIDs, 1)
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
	require.Zero(t, len(client.acceptedIDs))
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

	require.NoError(t, client.reserveMessageIDs(
		[]pcommon.DmqMessage{first},
		time.Unix(100, 0),
	))
	require.ErrorContains(t, client.reserveMessageIDs(
		[]pcommon.DmqMessage{second},
		time.Unix(100, 0),
	), "replay cache full")
	require.Len(t, client.acceptedIDs, 1)

	require.NoError(t, client.reserveMessageIDs(
		[]pcommon.DmqMessage{second},
		time.Unix(101, 0),
	))
	require.Len(t, client.acceptedIDs, 1)
	require.Contains(t, client.acceptedIDs, string(second.ID()))
}
