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

// TestServerRepliesAfterRequestNextReturnsWithPipelinedClient covers a
// server that answers MsgRequestNext with AwaitReply and sends the
// RollBackward after RequestNextFunc has returned, while the client keeps
// further RequestNext messages pipelined. Each pipelined RequestNext must
// wait for the server to return to Idle and then start its own CanAwait
// exchange; handling it while the previous request is still unanswered sends
// an AwaitReply that is not allowed in MustReply.
func TestServerRepliesAfterRequestNextReturnsWithPipelinedClient(t *testing.T) {
	const burst = 5
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)
	t.Cleanup(clientMuxer.Stop)
	t.Cleanup(serverMuxer.Stop)

	serverErrors := make(chan error, 1)
	server := NewServer(
		protocol.ProtocolOptions{
			Muxer:     serverMuxer,
			ErrorChan: serverErrors,
			Mode:      protocol.ProtocolModeNodeToClient,
		},
		&Config{
			RequestNextFunc: func(ctx CallbackContext) error {
				if err := ctx.Server.AwaitReply(); err != nil {
					return err
				}
				go func() {
					time.Sleep(100 * time.Millisecond)
					if err := ctx.Server.RollBackward(
						pcommon.NewPointOrigin(),
						Tip{},
					); err != nil {
						serverErrors <- err
					}
				}()
				return nil
			},
		},
	)

	clientErrors := make(chan error, 1)
	rollBackwards := make(chan struct{}, burst)
	client := protocol.New(protocol.ProtocolConfig{
		Name:       "chainsync-async-reply-client",
		ProtocolId: ProtocolIdNtC,
		ErrorChan:  clientErrors,
		Muxer:      clientMuxer,
		Mode:       protocol.ProtocolModeNodeToClient,
		Role:       protocol.ProtocolRoleClient,
		MessageHandlerFunc: func(msg protocol.Message) error {
			if msg.Type() == MessageTypeRollBackward {
				rollBackwards <- struct{}{}
			}
			return nil
		},
		MessageFromCborFunc: NewMsgFromCborNtC,
		StateMap:            StateMapNtC,
		InitialState:        stateIdle,
	})

	server.Start()
	t.Cleanup(server.Stop)
	client.Start()
	t.Cleanup(client.Stop)
	serverMuxer.Start()
	clientMuxer.Start()

	for range burst {
		require.NoError(t, client.SendMessage(NewMsgRequestNext()))
	}

	deadline := time.After(5 * time.Second)
	for received := 0; received < burst; {
		select {
		case <-rollBackwards:
			received++
		case err := <-serverErrors:
			t.Fatalf("server error after %d replies: %s", received, err)
		case err := <-clientErrors:
			t.Fatalf("client error after %d replies: %s", received, err)
		case <-deadline:
			t.Fatalf("received %d of %d RollBackward replies", received, burst)
		}
	}
}
