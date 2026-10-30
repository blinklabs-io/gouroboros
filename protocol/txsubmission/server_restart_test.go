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

package txsubmission_test

import (
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
	"github.com/stretchr/testify/require"
)

// TestServerHandlesInitSentDuringRestart sends a client's MsgInit, the first
// message of a restarted TxSubmission, while the server is still handling the
// preceding MsgDone, and requires the restarted server to handle it.
func TestServerHandlesInitSentDuringRestart(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, txsubmission.ProtocolId)
	inits := make(chan struct{}, 2)
	server := txsubmission.NewServer(
		protocol.ProtocolOptions{
			Muxer:  h.Server,
			Logger: h.Logger("done"),
			Mode:   protocol.ProtocolModeNodeToNode,
		},
		&txsubmission.Config{
			InitFunc: func(ctx txsubmission.CallbackContext) error {
				inits <- struct{}{}
				// A blocking request is outstanding when the client
				// answers with MsgDone. It returns once the Done is
				// handled, so it must not run on the protocol's receive
				// loop.
				go func() {
					_, _ = ctx.Server.RequestTxIds(true, 1)
				}()
				return nil
			},
		},
	)
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	waitInit := func(what string) {
		t.Helper()
		select {
		case <-inits:
		case <-time.After(restartwindow.Timeout):
			t.Fatalf("server did not handle %s", what)
		}
	}

	h.Send(txsubmission.NewMsgInit())
	waitInit("the first MsgInit")
	// The blocking RequestTxIds answered by the client's MsgDone.
	require.Equal(
		t,
		uint(txsubmission.MessageTypeRequestTxIds),
		h.RecvType("RequestTxIds"),
	)
	h.Send(txsubmission.NewMsgDone())
	h.WaitHeld()
	h.Send(txsubmission.NewMsgInit())
	h.Settle()
	h.Release()

	waitInit("the MsgInit sent during the restart")
}
