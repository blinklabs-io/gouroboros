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
	"sync"
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

// readCounter reports the bytes read from the wrapped connection, so a test
// can tell when the peer's muxer has taken a segment off the wire.
type readCounter struct {
	net.Conn
	mu     sync.Mutex
	total  int
	wakeup chan struct{}
}

func (c *readCounter) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.mu.Lock()
	c.total += n
	c.mu.Unlock()
	select {
	case c.wakeup <- struct{}{}:
	default:
	}
	return n, err
}

func (c *readCounter) waitFor(t *testing.T, total int) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		c.mu.Lock()
		got := c.total
		c.mu.Unlock()
		if got >= total {
			return
		}
		select {
		case <-c.wakeup:
		case <-deadline:
			t.Fatalf("server read %d bytes, want %d", got, total)
		}
	}
}

// TestServerAdmitsRequestNextPipelinedIntoMustReply sends the second
// RequestNext only after the client has received AwaitReply, and holds the
// server's RollBackward until the server has read it, so the request can
// only arrive while the server is in MustReply. That is where a client's
// pipelined RequestNext lands whenever the server has already answered with
// AwaitReply.
func TestServerAdmitsRequestNextPipelinedIntoMustReply(t *testing.T) {
	for _, test := range []struct {
		name         string
		mode         protocol.ProtocolMode
		protocolId   uint16
		stateMap     protocol.StateMap
		fromCborFunc protocol.MessageFromCborFunc
	}{
		{
			name:         "NtN",
			mode:         protocol.ProtocolModeNodeToNode,
			protocolId:   ProtocolIdNtN,
			stateMap:     StateMapNtN,
			fromCborFunc: NewMsgFromCborNtN,
		},
		{
			name:         "NtC",
			mode:         protocol.ProtocolModeNodeToClient,
			protocolId:   ProtocolIdNtC,
			stateMap:     StateMapNtC,
			fromCborFunc: NewMsgFromCborNtC,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			// A RequestNext segment is an 8-byte header and a 2-byte payload.
			const requestNextSegmentBytes = 10
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() {
				_ = clientConn.Close()
				_ = serverConn.Close()
			})
			serverRead := &readCounter{
				Conn:   serverConn,
				wakeup: make(chan struct{}, 1),
			}
			clientMuxer := muxer.New(clientConn)
			serverMuxer := muxer.New(serverRead)
			t.Cleanup(clientMuxer.Stop)
			t.Cleanup(serverMuxer.Stop)

			serverErrors := make(chan error, 2)
			releaseReply := make(chan struct{})
			server := NewServer(
				protocol.ProtocolOptions{
					Muxer:     serverMuxer,
					ErrorChan: serverErrors,
					Mode:      test.mode,
				},
				&Config{
					RequestNextFunc: func(ctx CallbackContext) error {
						if err := ctx.Server.AwaitReply(); err != nil {
							return err
						}
						go func() {
							<-releaseReply
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

			// The client may pipeline RequestNext in MustReply here, so the
			// test can place a request there deterministically.
			clientStateMap := test.stateMap.Copy()
			mustReply := clientStateMap[stateMustReply]
			mustReply.AllowPipelinedSend = true
			mustReply.PipelinedMessageTypes = []uint8{MessageTypeRequestNext}
			clientStateMap[stateMustReply] = mustReply
			clientErrors := make(chan error, 1)
			replies := make(chan uint8, 4)
			client := protocol.New(protocol.ProtocolConfig{
				Name:       "chainsync-must-reply-client",
				ProtocolId: test.protocolId,
				ErrorChan:  clientErrors,
				Muxer:      clientMuxer,
				Mode:       test.mode,
				Role:       protocol.ProtocolRoleClient,
				MessageHandlerFunc: func(msg protocol.Message) error {
					replies <- msg.Type()
					return nil
				},
				MessageFromCborFunc: test.fromCborFunc,
				StateMap:            clientStateMap,
				InitialState:        stateIdle,
			})

			server.Start()
			t.Cleanup(server.Stop)
			client.Start()
			t.Cleanup(client.Stop)
			serverMuxer.Start()
			clientMuxer.Start()

			expectReply := func(want uint8) {
				t.Helper()
				select {
				case got := <-replies:
					require.Equal(t, want, got)
				case err := <-serverErrors:
					t.Fatalf("server error: %s", err)
				case err := <-clientErrors:
					t.Fatalf("client error: %s", err)
				case <-time.After(2 * time.Second):
					t.Fatalf("no reply of type %d", want)
				}
			}

			require.NoError(t, client.SendMessage(NewMsgRequestNext()))
			expectReply(MessageTypeAwaitReply)
			require.NoError(t, client.SendMessage(NewMsgRequestNext()))
			serverRead.waitFor(t, 2*requestNextSegmentBytes)
			// The server stays in MustReply until releaseReply is closed, so
			// an admission failure is reported within this window.
			select {
			case err := <-serverErrors:
				t.Fatalf("server rejected RequestNext in MustReply: %s", err)
			case <-time.After(100 * time.Millisecond):
			}
			close(releaseReply)
			expectReply(MessageTypeRollBackward)
			expectReply(MessageTypeAwaitReply)
			expectReply(MessageTypeRollBackward)
		})
	}
}
