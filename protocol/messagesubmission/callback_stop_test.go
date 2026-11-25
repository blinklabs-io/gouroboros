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
	"context"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// TestCallbackStopsItsClient checks that a callback passing its context to
// its own client's Stop returns without waiting for the receive loop running
// it, and that the client is fully joined afterwards. The connection does
// not carry this protocol, so it is tested on a muxer pair.
func TestCallbackStopsItsClient(t *testing.T) {
	var flow *directionFlow
	stopped := make(chan error, 1)
	clientCfg := NewConfig(WithRequestMessagesFunc(func(
		ctx CallbackContext,
		_ [][]byte,
	) {
		stopped <- flow.client.Stop(ctx.Context())
	}))
	flow = newDirectionFlow(t, clientCfg, NewConfig())
	ids := [][]byte{testMessageID(1)}
	flow.server.lock.Lock()
	flow.server.pendingMessageIDs = ids
	flow.server.lock.Unlock()
	flow.client.lock.Lock()
	flow.client.pendingMessageIDs = ids
	flow.client.lock.Unlock()
	require.NoError(t, flow.server.RequestMessages(ids))
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stop from the callback did not return")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, flow.client.Stop(ctx), "client was not joined")
}

// TestEmptyIDReplyCallbackStopsItsServer checks that ReplyMessageIdsFunc,
// called for MsgReplyNoMessageIds on the server's message loop, gets a
// context marking its server: passing it to that server's Stop returns
// without waiting for the loop running the callback.
func TestEmptyIDReplyCallbackStopsItsServer(t *testing.T) {
	peer, m := newReferencePeer(t)
	errs := make(chan error, 8)
	stopped := make(chan error, 1)
	cfg := NewConfig(
		WithReplyMessageIdsFunc(func(
			ctx CallbackContext,
			_ []pcommon.MessageIDAndSize,
		) {
			stopped <- ctx.Server.Stop(ctx.Context())
		}),
		WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
	)
	server := NewServer(protocol.ProtocolOptions{
		Muxer:     m,
		ErrorChan: errs,
		Version:   protocol.ProtocolVersionDMQNtN2,
	}, &cfg)
	server.Protocol.EnsureRegistered()
	m.Start()
	server.Start()

	require.NoError(t, server.RequestMessageIdsBlocking(0, 3))
	peer.expect(true, []byte{0x84, 0x01, 0xf5, 0x00, 0x03})
	// MsgReplyNoSigIds = [3]
	peer.send(false, []byte{0x81, 0x03})
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stop from the callback did not return")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, server.Stop(ctx), "server was not joined")
}
