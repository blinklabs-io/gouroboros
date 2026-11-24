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

package chainsync

import (
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// TestStopWithRequestUnansweredReportsError stops a client at the chain tip,
// where the server has answered MsgRequestNext with MsgAwaitReply and holds
// its roll. MsgDone cannot be sent before that roll, so Stop gives up without
// it and the server is left owing a reply. The client must report
// ErrStoppedWithoutDone so its connection is closed: a client started again
// on the same connection would receive the old reply in StIdle.
func TestStopWithRequestUnansweredReportsError(t *testing.T) {
	t.Parallel()
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)
	t.Cleanup(clientMuxer.Stop)
	t.Cleanup(serverMuxer.Stop)

	requested := make(chan struct{}, 1)
	release := make(chan struct{})
	serverErrors := make(chan error, 4)
	serverCfg := NewConfig(
		WithFindIntersectFunc(func(
			CallbackContext, []pcommon.Point,
		) (pcommon.Point, Tip, error) {
			return pcommon.NewPointOrigin(), Tip{}, nil
		}),
		WithRequestNextFunc(func(ctx CallbackContext) error {
			if err := ctx.Server.AwaitReply(); err != nil {
				return err
			}
			requested <- struct{}{}
			go func() {
				<-release
				_ = ctx.Server.RollBackward(pcommon.NewPointOrigin(), Tip{})
			}()
			return nil
		}),
	)
	server := NewServer(protocol.ProtocolOptions{
		Muxer:     serverMuxer,
		ErrorChan: serverErrors,
		Mode:      protocol.ProtocolModeNodeToClient,
	}, &serverCfg)

	clientErrors := make(chan error, 4)
	clientCfg := NewConfig(
		WithPipelineLimit(1),
		WithRollBackwardFunc(func(CallbackContext, pcommon.Point, Tip) error {
			return nil
		}),
	)
	client := NewClient(protocol.ProtocolOptions{
		Muxer:     clientMuxer,
		ErrorChan: clientErrors,
		Mode:      protocol.ProtocolModeNodeToClient,
	}, &clientCfg)

	server.Start()
	t.Cleanup(server.Stop)
	client.Start()
	t.Cleanup(func() { _ = client.Stop() })
	serverMuxer.Start()
	clientMuxer.Start()
	t.Cleanup(func() { close(release) })

	require.NoError(t, client.Sync([]pcommon.Point{pcommon.NewPointOrigin()}))
	select {
	case <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("server never received MsgRequestNext")
	}

	require.Error(t, client.Stop(), "Stop delivered MsgDone with a request unanswered")
	select {
	case err := <-clientErrors:
		require.ErrorIs(t, err, ErrStoppedWithoutDone)
	case <-time.After(time.Second):
		t.Fatal("Stop left the server owing a reply without reporting an error")
	}
}
