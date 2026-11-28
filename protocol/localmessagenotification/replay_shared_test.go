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

// TestClientsSharingConfigDeliverReplayOnce gives the same reply to clients
// of two connections built from one Config at the same time. The replay
// cache is the Config's, so the message reaches ReplyMessagesFunc once.
func TestClientsSharingConfigDeliverReplayOnce(t *testing.T) {
	const rounds = 64
	var delivered atomic.Int32
	cfg := NewConfig(
		WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
		WithReplyMessagesFunc(
			func(_ CallbackContext, messages []pcommon.DmqMessage, _ bool) {
				delivered.Add(int32(len(messages)))
			},
		),
	)
	expiresAt := uint32(time.Now().Add(time.Minute).Unix())
	for round := range rounds {
		msg := clientTestMessage(t, "shared replay", expiresAt+uint32(round))
		clients := []*Client{
			NewClient(protocol.ProtocolOptions{}, &cfg),
			NewClient(protocol.ProtocolOptions{}, &cfg),
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		errs := make(chan error, len(clients))
		for _, client := range clients {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs <- client.messageHandler(
					NewMsgReplyMessagesBlocking([]pcommon.DmqMessage{msg}),
				)
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
		require.Equal(t, int32(round+1), delivered.Load(), "round %d", round)
	}
}

// TestClientKeepsReplayIDUntilMessageExpires replays a delivered message at
// the last instant its TTL admits it, and just after. The cached ID must
// still be there at the last valid instant, or the replay is delivered
// again; once the message has expired the TTL check refuses it either way.
func TestClientKeepsReplayIDUntilMessageExpires(t *testing.T) {
	var delivered atomic.Int32
	cfg := NewConfig(
		WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
		WithReplyMessagesFunc(
			func(_ CallbackContext, messages []pcommon.DmqMessage, _ bool) {
				delivered.Add(int32(len(messages)))
			},
		),
	)
	expiresAt := uint32(time.Now().Add(time.Minute).Unix())
	msg := clientTestMessage(t, "expiry boundary", expiresAt)
	expiry := time.Unix(int64(expiresAt), 0)

	first := NewClient(protocol.ProtocolOptions{}, &cfg)
	first.now = func() time.Time { return expiry.Add(-time.Second) }
	require.NoError(t, first.messageHandler(
		NewMsgReplyMessagesBlocking([]pcommon.DmqMessage{msg}),
	))
	require.Equal(t, int32(1), delivered.Load())

	atExpiry := NewClient(protocol.ProtocolOptions{}, &cfg)
	atExpiry.now = func() time.Time { return expiry }
	require.NoError(t, atExpiry.messageHandler(
		NewMsgReplyMessagesBlocking([]pcommon.DmqMessage{msg}),
	))
	require.Equal(
		t, int32(1), delivered.Load(),
		"replay delivered again at the last valid instant",
	)

	afterExpiry := NewClient(protocol.ProtocolOptions{}, &cfg)
	afterExpiry.now = func() time.Time {
		return expiry.Add(500 * time.Millisecond)
	}
	_ = afterExpiry.messageHandler(
		NewMsgReplyMessagesBlocking([]pcommon.DmqMessage{msg}),
	)
	require.Equal(
		t, int32(1), delivered.Load(),
		"expired replay delivered again",
	)
}
