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

package localtxsubmission_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
	"github.com/stretchr/testify/require"
)

func newRestartTestServer(
	h *restartwindow.Harness,
	opts protocol.ProtocolOptions,
) *localtxsubmission.Server {
	opts.Muxer = h.Server
	opts.Mode = protocol.ProtocolModeNodeToClient
	return localtxsubmission.NewServer(opts, &localtxsubmission.Config{
		SubmitTxFunc: func(
			localtxsubmission.CallbackContext,
			localtxsubmission.MsgSubmitTxTransaction,
		) error {
			return nil
		},
	})
}

// TestServerAnswersSubmitSentDuringRestart sends a client's MsgSubmitTx,
// the first message of a new run, while the server is still handling the
// preceding MsgDone, and requires the restarted server to answer it. The
// reference runs the node-to-client server under an inbound governor, which
// restarts a responder that ended cleanly.
func TestServerAnswersSubmitSentDuringRestart(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, localtxsubmission.ProtocolId)
	server := newRestartTestServer(h, protocol.ProtocolOptions{
		Logger: h.Logger("done"),
	})
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	h.Send(localtxsubmission.NewMsgDone())
	h.WaitHeld()
	h.Send(localtxsubmission.NewMsgSubmitTx(6, []byte{0x80}))
	h.Settle()
	h.Release()

	require.Equal(
		t,
		uint(localtxsubmission.MessageTypeAcceptTx),
		h.RecvType("AcceptTx"),
	)
}

// TestServerAnswersBackToBackRuns submits and ends a run repeatedly without
// waiting between a MsgDone and the next run.
func TestServerAnswersBackToBackRuns(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, localtxsubmission.ProtocolId)
	server := newRestartTestServer(h, protocol.ProtocolOptions{})
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	for range 200 {
		h.Send(localtxsubmission.NewMsgSubmitTx(6, []byte{0x80}))
		require.Equal(
			t,
			uint(localtxsubmission.MessageTypeAcceptTx),
			h.RecvType("AcceptTx"),
		)
		h.Send(localtxsubmission.NewMsgDone())
	}
}
