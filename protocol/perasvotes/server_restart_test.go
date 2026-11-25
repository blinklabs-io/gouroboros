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

package perasvotes_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/perasvotes"
	"github.com/stretchr/testify/require"
)

func newRestartTestServer(
	h *restartwindow.Harness,
	logger string,
) *perasvotes.Server {
	return perasvotes.NewServer(
		protocol.ProtocolOptions{
			Muxer:  h.Server,
			Logger: h.Logger(logger),
			Mode:   protocol.ProtocolModeNodeToNode,
		},
		&perasvotes.Config{
			ObjectIDsFunc: func(
				_ perasvotes.CallbackContext,
				_ uint16,
				count uint16,
			) ([]perasvotes.VoteID, error) {
				if count == 0 {
					return nil, nil
				}
				return []perasvotes.VoteID{{RoundNo: 1, SeatIndex: 1}}, nil
			},
		},
	)
}

// TestServerAnswersRequestAfterRestart ends a run with the client's MsgDone
// and starts the next one with MsgInit and a blocking MsgRequestObjectIDs.
// The reference runs the outbound side as a responder under the inbound
// governor, which restarts a responder that ended cleanly, so the request
// must be answered.
func TestServerAnswersRequestAfterRestart(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, perasvotes.ProtocolId)
	server := newRestartTestServer(h, "unused hold")
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	for range 3 {
		h.Send(perasvotes.NewMsgInit())
		h.Send(perasvotes.NewMsgRequestObjectIDs(true, 0, 1))
		require.Equal(
			t,
			uint(perasvotes.MessageTypeReplyObjectIDs),
			h.RecvType("ReplyObjectIDs"),
		)
		h.Send(perasvotes.NewMsgDone())
	}
}

// TestServerAnswersInitSentDuringRestart sends the next run's MsgInit and
// request while the server is still handling the preceding MsgDone.
func TestServerAnswersInitSentDuringRestart(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, perasvotes.ProtocolId)
	server := newRestartTestServer(h, "done")
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	h.Send(perasvotes.NewMsgInit())
	h.Send(perasvotes.NewMsgDone())
	h.WaitHeld()
	h.Send(perasvotes.NewMsgInit())
	h.Send(perasvotes.NewMsgRequestObjectIDs(true, 0, 1))
	h.Settle()
	h.Release()

	require.Equal(
		t,
		uint(perasvotes.MessageTypeReplyObjectIDs),
		h.RecvType("ReplyObjectIDs"),
	)
}
