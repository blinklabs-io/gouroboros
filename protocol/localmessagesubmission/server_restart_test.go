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

package localmessagesubmission_test

import (
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/gouroboros/protocol/localmessagesubmission"
	"github.com/stretchr/testify/require"
)

func restartTestSubmission() *localmessagesubmission.MsgSubmitMessage {
	return localmessagesubmission.NewMsgSubmitMessage(pcommon.DmqMessage{
		MessageID: []byte("restart-message-id"),
		Payload: pcommon.DmqMessagePayload{
			ExpiresAt: uint32(time.Now().Add(time.Minute).Unix()),
		},
	})
}

// TestServerAnswersSubmitSentDuringRestart sends a client's MsgSubmitMessage,
// the first message of a new run, while the server is still handling the
// preceding MsgDone, and requires the restarted server to answer it. dmq-node
// runs its node-to-client servers under the inbound governor, which restarts
// a responder that ended cleanly.
func TestServerAnswersSubmitSentDuringRestart(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, localmessagesubmission.ProtocolID)
	// No SubmitMessageFunc: the server refuses every submission, which is
	// still an answer.
	server := localmessagesubmission.NewServer(
		protocol.ProtocolOptions{
			Muxer:  h.Server,
			Logger: h.Logger("received done message"),
			Mode:   protocol.ProtocolModeNodeToClient,
		},
		nil,
	)
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	h.Send(localmessagesubmission.NewMsgDone())
	h.WaitHeld()
	h.Send(restartTestSubmission())
	h.Settle()
	h.Release()

	require.Equal(
		t,
		uint(localmessagesubmission.MessageTypeRejectMessage),
		h.RecvType("RejectMessage"),
	)
}

// TestServerAnswersBackToBackRestarts submits and ends a run repeatedly
// without waiting between a MsgDone and the next run.
func TestServerAnswersBackToBackRestarts(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, localmessagesubmission.ProtocolID)
	server := localmessagesubmission.NewServer(
		protocol.ProtocolOptions{
			Muxer: h.Server,
			Mode:  protocol.ProtocolModeNodeToClient,
		},
		nil,
	)
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	for range 50 {
		h.Send(restartTestSubmission())
		require.Equal(
			t,
			uint(localmessagesubmission.MessageTypeRejectMessage),
			h.RecvType("RejectMessage"),
		)
		h.Send(localmessagesubmission.NewMsgDone())
	}
}
