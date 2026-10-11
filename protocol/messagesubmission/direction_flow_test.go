// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package messagesubmission

import (
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

type directionFlow struct {
	client       *Client
	server       *Server
	clientMuxer  *muxer.Muxer
	serverMuxer  *muxer.Muxer
	clientConn   net.Conn
	serverConn   net.Conn
	serverErrors chan error
}

func newDirectionFlow(
	t *testing.T,
	clientCfg Config,
	serverCfg Config,
) *directionFlow {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	f := &directionFlow{
		clientMuxer:  muxer.New(clientConn),
		serverMuxer:  muxer.New(serverConn),
		clientConn:   clientConn,
		serverConn:   serverConn,
		serverErrors: make(chan error, 4),
	}
	f.client = NewClient(protocol.ProtocolOptions{
		Muxer:     f.clientMuxer,
		ErrorChan: make(chan error, 4),
		Version:   MessageSubmissionV2MinVersion,
	}, &clientCfg)
	f.server = NewServer(protocol.ProtocolOptions{
		Muxer:     f.serverMuxer,
		ErrorChan: f.serverErrors,
		Version:   MessageSubmissionV2MinVersion,
	}, &serverCfg)
	f.client.Protocol.EnsureRegistered()
	f.server.Protocol.EnsureRegistered()
	f.clientMuxer.Start()
	f.serverMuxer.Start()
	f.client.Start()
	f.server.Start()
	t.Cleanup(func() {
		f.client.Protocol.Stop()
		f.server.Protocol.Stop()
		f.clientMuxer.Stop()
		f.serverMuxer.Stop()
		_ = f.clientConn.Close()
		_ = f.serverConn.Close()
	})
	return f
}

func TestMessageIDReplyUsesRequestedCountThroughProtocol(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
		valid bool
	}{
		{name: "at requested count", count: 2, valid: true},
		{name: "above requested count", count: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requestReceived := make(chan struct{})
			replyReceived := make(chan int, 1)
			clientCfg := NewConfig(WithRequestMessageIdsFunc(func(
				_ CallbackContext,
				_ bool,
				_ uint16,
				_ uint16,
			) {
				close(requestReceived)
			}))
			serverCfg := NewConfig(WithReplyMessageIdsFunc(func(
				_ CallbackContext,
				ids []pcommon.MessageIDAndSize,
			) {
				replyReceived <- len(ids)
			}))
			flow := newDirectionFlow(t, clientCfg, serverCfg)
			require.NoError(t, flow.server.RequestMessageIdsBlocking(0, 2))
			select {
			case <-requestReceived:
			case <-time.After(time.Second):
				t.Fatal("message ID request did not reach the client")
			}
			entries := make([]pcommon.MessageIDAndSize, tc.count)
			for idx := range entries {
				entries[idx].MessageID = []byte{byte(idx)}
			}
			require.NoError(t, flow.client.SendMessage(NewMsgReplyMessageIds(entries)))
			if tc.valid {
				select {
				case count := <-replyReceived:
					require.Equal(t, 2, count)
				case <-time.After(time.Second):
					t.Fatal("message ID reply did not reach the server")
				}
				return
			}
			select {
			case err := <-flow.serverErrors:
				require.ErrorContains(t, err, "maximum is 2")
			case <-time.After(time.Second):
				t.Fatal("oversized message ID reply was not rejected")
			}
		})
	}
}

func TestMessageReplyUsesRequestedCountThroughProtocol(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
		valid bool
	}{
		{name: "at requested count", count: 2, valid: true},
		{name: "above requested count", count: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requestReceived := make(chan struct{})
			replyReceived := make(chan int, 1)
			clientCfg := NewConfig(WithRequestMessagesFunc(func(
				_ CallbackContext,
				_ [][]byte,
			) {
				close(requestReceived)
			}))
			serverCfg := NewConfig(WithReplyMessagesFunc(func(
				_ CallbackContext,
				messages []pcommon.DmqMessage,
			) {
				replyReceived <- len(messages)
			}))
			serverCfg.Authenticator = pcommon.NewNoOpAuthenticator(nil)
			serverCfg.TTLValidator = pcommon.NewNoOpTTLValidator(nil)
			flow := newDirectionFlow(t, clientCfg, serverCfg)
			flow.client.lock.Lock()
			flow.client.pendingMessageIDs = [][]byte{{1}, {2}}
			flow.client.lock.Unlock()
			require.NoError(t, flow.server.RequestMessages(
				[][]byte{{1}, {2}},
			))
			select {
			case <-requestReceived:
			case <-time.After(time.Second):
				t.Fatal("message request did not reach the client")
			}
			require.NoError(t, flow.client.SendMessage(
				NewMsgReplyMessages(make([]pcommon.DmqMessage, tc.count)),
			))
			if tc.valid {
				select {
				case count := <-replyReceived:
					require.Equal(t, 2, count)
				case <-time.After(time.Second):
					t.Fatal("message reply did not reach the server")
				}
				return
			}
			select {
			case err := <-flow.serverErrors:
				require.ErrorContains(t, err, "maximum is 2")
			case <-time.After(time.Second):
				t.Fatal("oversized message reply was not rejected")
			}
		})
	}
}
