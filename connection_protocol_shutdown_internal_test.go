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

package ouroboros

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/gouroboros/protocol/keepalive"
	"github.com/blinklabs-io/gouroboros/protocol/localmessagenotification"
	"github.com/blinklabs-io/gouroboros/protocol/localmessagesubmission"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestConnectionShutdownStopsConstructedDMQProtocols(t *testing.T) {
	defer goleak.VerifyNone(t)
	for range 10 {
		conn, err := New()
		require.NoError(t, err)
		protoOptions := protocol.ProtocolOptions{}
		conn.localMessageSubmission = localmessagesubmission.New(
			protoOptions,
			nil,
		)
		conn.localMessageNotification = localmessagenotification.New(
			protoOptions,
			nil,
		)
		conn.protocolsReady = true

		protocolDone := []<-chan struct{}{
			conn.localMessageSubmission.Client.DoneChan(),
			conn.localMessageSubmission.Server.DoneChan(),
			conn.localMessageNotification.Client.DoneChan(),
			conn.localMessageNotification.Server.DoneChan(),
		}
		conn.shutdown()

		for _, done := range protocolDone {
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal(
					"constructed protocol did not complete connection shutdown",
				)
			}
		}
	}
}

func TestConnectionRefusesProtocolSetupAfterShutdownBegins(t *testing.T) {
	conn, err := New()
	require.NoError(t, err)
	conn.doneChan = make(chan any)
	close(conn.doneChan)

	require.ErrorContains(
		t,
		conn.lockProtocolSetup(),
		"connection shutting down",
	)
	require.True(
		t,
		conn.protocolMu.TryLock(),
		"protocol setup left its lock held",
	)
	conn.protocolMu.Unlock()
}

// TestConnectionShutdownStopsRestartingServer runs stopProtocols while the
// chain-sync server is handling the client's MsgDone, before it has replaced
// its protocol instance. The server must stay stopped: no new instance, and
// no application callback for the client's next request.
func TestConnectionShutdownStopsRestartingServer(t *testing.T) {
	h := restartwindow.New(t, chainsync.ProtocolIdNtC)
	var calls atomic.Int32
	server := chainsync.NewServer(
		protocol.ProtocolOptions{
			Muxer:  h.Server,
			Logger: h.Logger("done"),
			Mode:   protocol.ProtocolModeNodeToClient,
		},
		&chainsync.Config{
			FindIntersectFunc: func(
				chainsync.CallbackContext,
				[]pcommon.Point,
			) (pcommon.Point, chainsync.Tip, error) {
				calls.Add(1)
				return pcommon.NewPointOrigin(), chainsync.Tip{}, nil
			},
		},
	)
	conn, err := New()
	require.NoError(t, err)
	conn.chainSync = &chainsync.ChainSync{Server: server}
	conn.protocolsReady = true
	server.Start()
	t.Cleanup(func() { _ = server.Stop(context.Background()) })
	h.Start()

	h.Send(chainsync.NewMsgDone())
	h.WaitHeld()
	stopped := server.ProtocolInstance()
	conn.stopProtocols()
	h.Release()

	restarted := func() bool { return server.ProtocolInstance() != stopped }
	require.Never(t, restarted, 200*time.Millisecond, 5*time.Millisecond,
		"a server stopped by connection shutdown installed a new instance")
	h.Send(chainsync.NewMsgFindIntersect(
		[]pcommon.Point{pcommon.NewPointOrigin()},
	))
	h.Settle()
	require.Never(t, func() bool { return calls.Load() > 0 },
		200*time.Millisecond, 5*time.Millisecond,
		"a server stopped by connection shutdown ran an application callback")
}

// TestConnectionShutdownStopsRestartingKeepAliveServer is the keep-alive
// counterpart: stopProtocols during the server's MsgDone handling must leave
// no new instance running.
func TestConnectionShutdownStopsRestartingKeepAliveServer(t *testing.T) {
	h := restartwindow.New(t, keepalive.ProtocolId)
	server := keepalive.NewServer(
		protocol.ProtocolOptions{
			Muxer:  h.Server,
			Logger: h.Logger("done"),
			Mode:   protocol.ProtocolModeNodeToNode,
		},
		nil,
	)
	conn, err := New()
	require.NoError(t, err)
	conn.keepAlive = &keepalive.KeepAlive{Server: server}
	conn.protocolsReady = true
	server.Start()
	t.Cleanup(func() { _ = server.Stop(context.Background()) })
	h.Start()

	h.Send(keepalive.NewMsgDone())
	h.WaitHeld()
	stopped := server.ProtocolInstance()
	conn.stopProtocols()
	h.Release()

	restarted := func() bool { return server.ProtocolInstance() != stopped }
	require.Never(t, restarted, 200*time.Millisecond, 5*time.Millisecond,
		"a server stopped by connection shutdown installed a new instance")
}
