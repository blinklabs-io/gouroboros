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

package localmessagenotification_test

import (
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localmessagenotification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRestartTestServer(
	h *restartwindow.Harness,
	logger string,
) *localmessagenotification.Server {
	server := localmessagenotification.NewServer(
		protocol.ProtocolOptions{
			Muxer:  h.Server,
			Logger: h.Logger(logger),
			Mode:   protocol.ProtocolModeNodeToClient,
		},
		nil,
	)
	server.Start()
	return server
}

// TestServerAnswersRequestSentDuringRestart sends a client's non-blocking
// MsgRequestMessages, the first message of a new run, while the server is
// still handling the preceding MsgClientDone, and requires the restarted
// server to answer it. dmq-node runs its node-to-client servers under the
// inbound governor, which restarts a responder that ended cleanly.
func TestServerAnswersRequestSentDuringRestart(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, localmessagenotification.ProtocolID)
	server := newRestartTestServer(h, "received client done message")
	t.Cleanup(func() { _ = server.Stop() })
	h.Start()

	h.Send(localmessagenotification.NewMsgClientDone())
	h.WaitHeld()
	h.Send(localmessagenotification.NewMsgRequestMessages(false))
	h.Settle()
	h.Release()

	require.Equal(
		t,
		uint(localmessagenotification.MessageTypeReplyMessagesNonBlocking),
		h.RecvType("ReplyMessagesNonBlocking"),
	)
}

// TestServerAnswersBackToBackRestarts requests and ends a run repeatedly
// without waiting between a MsgClientDone and the next run.
func TestServerAnswersBackToBackRestarts(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, localmessagenotification.ProtocolID)
	server := newRestartTestServer(h, "unused hold")
	t.Cleanup(func() { _ = server.Stop() })
	h.Start()

	for range 50 {
		h.Send(localmessagenotification.NewMsgRequestMessages(false))
		require.Equal(
			t,
			uint(localmessagenotification.MessageTypeReplyMessagesNonBlocking),
			h.RecvType("ReplyMessagesNonBlocking"),
		)
		h.Send(localmessagenotification.NewMsgClientDone())
	}
}

// TestServerStopDuringRestartStopsServer stops the server while its
// MsgClientDone handler is running, before it has replaced its protocol
// instance. The restart must not install a new instance.
func TestServerStopDuringRestartStopsServer(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, localmessagenotification.ProtocolID)
	server := newRestartTestServer(h, "received client done message")
	t.Cleanup(func() { _ = server.Stop() })
	h.Start()

	h.Send(localmessagenotification.NewMsgClientDone())
	h.WaitHeld()
	stopped := server.ProtocolInstance()
	require.NoError(t, server.Stop())
	h.Release()

	restarted := func() bool { return server.ProtocolInstance() != stopped }
	assert.Never(t, restarted, 200*time.Millisecond, 5*time.Millisecond,
		"a stopped server installed a new protocol instance")
}
