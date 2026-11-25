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

package localstatequery_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	"github.com/stretchr/testify/require"
)

func newRestartTestServer(
	h *restartwindow.Harness,
	opts protocol.ProtocolOptions,
) *localstatequery.Server {
	opts.Muxer = h.Server
	opts.Mode = protocol.ProtocolModeNodeToClient
	return localstatequery.NewServer(opts, &localstatequery.Config{
		AcquireFunc: func(
			localstatequery.CallbackContext,
			localstatequery.AcquireTarget,
			bool,
		) error {
			return nil
		},
		ReleaseFunc: func(localstatequery.CallbackContext) error {
			return nil
		},
	})
}

// TestServerAnswersAcquireSentDuringRestart sends a client's MsgAcquire, the
// first message of a new run, while the server is still handling the
// preceding MsgDone, and requires the restarted server to answer it. The
// node-to-client server of the reference runs under the inbound governor,
// which restarts a responder that ended cleanly.
func TestServerAnswersAcquireSentDuringRestart(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, localstatequery.ProtocolId)
	server := newRestartTestServer(h, protocol.ProtocolOptions{
		Logger: h.Logger("done"),
	})
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	h.Send(localstatequery.NewMsgDone())
	h.WaitHeld()
	h.Send(localstatequery.NewMsgAcquireVolatileTip())
	h.Settle()
	h.Release()

	require.Equal(
		t,
		uint(localstatequery.MessageTypeAcquired),
		h.RecvType("Acquired"),
	)
}

// TestServerAnswersBackToBackRestarts runs acquire, release and MsgDone
// without waiting between a MsgDone and the next run, repeatedly.
func TestServerAnswersBackToBackRestarts(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, localstatequery.ProtocolId)
	server := newRestartTestServer(h, protocol.ProtocolOptions{})
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	for range 200 {
		h.Send(localstatequery.NewMsgAcquireVolatileTip())
		require.Equal(
			t,
			uint(localstatequery.MessageTypeAcquired),
			h.RecvType("Acquired"),
		)
		h.Send(localstatequery.NewMsgRelease())
		h.Send(localstatequery.NewMsgDone())
	}
}
