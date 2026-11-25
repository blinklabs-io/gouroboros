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

package messagesubmission_test

import (
	"context"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/messagesubmission"
	"github.com/stretchr/testify/require"
)

// TestServerRequestsAfterRestart ends a V1 run with the client's MsgDone,
// the answer to a blocking ID request, and starts the next run with MsgInit.
// The restarted server must handle MsgInit and send its next request. dmq-node
// runs its node-to-node responders under the inbound governor, which
// restarts a responder that ended cleanly.
// requestAfterDone makes a blocking ID request once the server has handled
// the client's MsgDone. Until then the run's request is still outstanding
// and the call is refused without sending anything.
func requestAfterDone(t *testing.T, server *messagesubmission.Server) {
	t.Helper()
	require.Eventually(
		t,
		func() bool { return server.RequestMessageIdsBlocking(0, 1) == nil },
		restartwindow.Timeout,
		time.Millisecond,
		"the server never accepted a request for the next run",
	)
}

func TestServerRequestsAfterRestart(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, messagesubmission.ProtocolID)
	server := messagesubmission.NewServer(
		protocol.ProtocolOptions{
			Muxer:  h.Server,
			Logger: h.Logger("unused hold"),
			Mode:   protocol.ProtocolModeNodeToNode,
		},
		nil,
	)
	server.Start()
	t.Cleanup(func() { _ = server.Stop(context.Background()) })
	h.Start()

	for range 3 {
		h.Send(messagesubmission.NewMsgInit())
		requestAfterDone(t, server)
		require.Equal(
			t,
			uint(messagesubmission.MessageTypeV1RequestMessageIds),
			h.RecvType("RequestMessageIds"),
		)
		h.Send(messagesubmission.NewMsgDone(0))
	}
}

// TestServerHandlesInitSentDuringRestart sends the next run's MsgInit while
// the server is still handling the preceding MsgDone.
func TestServerHandlesInitSentDuringRestart(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, messagesubmission.ProtocolID)
	server := messagesubmission.NewServer(
		protocol.ProtocolOptions{
			Muxer:  h.Server,
			Logger: h.Logger("received done message"),
			Mode:   protocol.ProtocolModeNodeToNode,
		},
		nil,
	)
	server.Start()
	t.Cleanup(func() { _ = server.Stop(context.Background()) })
	h.Start()

	h.Send(messagesubmission.NewMsgInit())
	require.NoError(t, server.RequestMessageIdsBlocking(0, 1))
	require.Equal(
		t,
		uint(messagesubmission.MessageTypeV1RequestMessageIds),
		h.RecvType("RequestMessageIds"),
	)
	h.Send(messagesubmission.NewMsgDone(0))
	h.WaitHeld()
	h.Send(messagesubmission.NewMsgInit())
	h.Settle()
	h.Release()

	requestAfterDone(t, server)
	require.Equal(
		t,
		uint(messagesubmission.MessageTypeV1RequestMessageIds),
		h.RecvType("RequestMessageIds after restart"),
	)
}
