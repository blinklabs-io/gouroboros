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

package localmessagesubmission

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// startSubmissionPair connects a client built from clientOpts to a server
// that accepts every message.
func startSubmissionPair(
	t *testing.T,
	clientOpts ...LocalMessageSubmissionOptionFunc,
) *Client {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)
	serverCfg := NewConfig(
		WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
		WithTTLValidator(pcommon.NewNoOpTTLValidator(nil)),
		WithSubmitMessageFunc(func(
			CallbackContext,
			*pcommon.DmqMessage,
		) pcommon.RejectReason {
			return nil
		}),
	)
	clientCfg := NewConfig(append([]LocalMessageSubmissionOptionFunc{
		WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
		WithTTLValidator(pcommon.NewNoOpTTLValidator(nil)),
	}, clientOpts...)...)
	client := NewClient(protocol.ProtocolOptions{
		Muxer:     clientMuxer,
		ErrorChan: make(chan error, 1),
	}, &clientCfg)
	server := NewServer(protocol.ProtocolOptions{
		Muxer:     serverMuxer,
		ErrorChan: make(chan error, 1),
	}, &serverCfg)
	client.Protocol.EnsureRegistered()
	server.Protocol.EnsureRegistered()
	clientMuxer.Start()
	serverMuxer.Start()
	client.Start()
	server.Start()
	t.Cleanup(func() {
		_ = client.Stop()
		server.Stop()
		clientMuxer.Stop()
		serverMuxer.Stop()
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	return client
}

func testMessage(id string) *pcommon.DmqMessage {
	return &pcommon.DmqMessage{
		MessageID: []byte(id),
		Payload: pcommon.DmqMessagePayload{
			MessageBody: paddedTestBody(id),
			ExpiresAt:   uint32(time.Now().Add(time.Minute).Unix()),
		},
	}
}

// TestSubmitAfterStopReturnsShuttingDown checks that submissions made after
// Stop return ErrProtocolShuttingDown. Stop moves the client to StDone, where
// nothing more is sent, so a submission queued behind MsgDone could never be
// answered.
func TestSubmitAfterStopReturnsShuttingDown(t *testing.T) {
	client := startSubmissionPair(t)
	require.NoError(t, client.SubmitMessage(testMessage("before")))
	require.NoError(t, client.Stop())
	results := make(chan error, 4)
	for i := range 4 {
		go func() {
			results <- client.SubmitMessage(testMessage(string(rune('a' + i))))
		}()
	}
	for range 4 {
		select {
		case err := <-results:
			require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
		case <-time.After(5 * time.Second):
			t.Fatal("SubmitMessage after Stop did not return")
		}
	}
}

// TestCallbackMaySubmitAgain checks that an accept callback can submit the
// next message. The callback runs before the first SubmitMessage returns; if
// it ran on the receive loop while that call held the submit lock, the second
// submission would wait for the lock and its answer would wait for the loop.
func TestCallbackMaySubmitAgain(t *testing.T) {
	var (
		client   *Client
		fired    atomic.Bool
		innerErr = make(chan error, 1)
	)
	client = startSubmissionPair(t, WithAcceptMessageFunc(func(CallbackContext) {
		if fired.CompareAndSwap(false, true) {
			innerErr <- client.SubmitMessage(testMessage("inner"))
		}
	}))
	outer := make(chan error, 1)
	go func() { outer <- client.SubmitMessage(testMessage("outer")) }()
	select {
	case err := <-outer:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("SubmitMessage whose callback submits again did not return")
	}
	require.NoError(t, <-innerErr)
}
