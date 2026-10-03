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

package txsubmission

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// newHookPair connects a client and a server over an in-memory pipe. The
// client returns an empty reply to every txid request.
func newHookPair(
	t *testing.T,
	clientCfg *Config,
	serverCfg *Config,
) (*Client, *Server, connection.ConnectionId, chan error) {
	t.Helper()
	localConn, remoteConn := net.Pipe()
	t.Cleanup(func() {
		require.NoError(t, localConn.Close())
		require.NoError(t, remoteConn.Close())
	})
	serverMuxer := muxer.New(localConn)
	clientMuxer := muxer.New(remoteConn)
	serverMuxer.Start()
	clientMuxer.Start()
	t.Cleanup(serverMuxer.Stop)
	t.Cleanup(clientMuxer.Stop)
	connId := connection.ConnectionId{
		LocalAddr:  &net.UnixAddr{Name: "local", Net: "unix"},
		RemoteAddr: &net.UnixAddr{Name: "remote", Net: "unix"},
	}
	serverErrors := make(chan error, 10)
	client := NewClient(
		protocol.ProtocolOptions{Muxer: clientMuxer, ConnectionId: connId},
		clientCfg,
	)
	server := NewServer(
		protocol.ProtocolOptions{
			Muxer:        serverMuxer,
			ConnectionId: connId,
			ErrorChan:    serverErrors,
		},
		serverCfg,
	)
	server.Start()
	client.Start()
	return client, server, connId, serverErrors
}

func TestOnInitNotifiesServerWithConnectionId(t *testing.T) {
	t.Parallel()
	got := make(chan connection.ConnectionId, 1)
	client, _, connId, _ := newHookPair(
		t,
		&Config{},
		&Config{OnInit: func(id connection.ConnectionId) { got <- id }},
	)
	client.Init()
	select {
	case id := <-got:
		require.Equal(t, connId, id)
	case <-time.After(5 * time.Second):
		t.Fatal("OnInit was not called")
	}
}

func TestInitWithoutCallbackIsAccepted(t *testing.T) {
	t.Parallel()
	client, server, _, serverErrors := newHookPair(
		t,
		&Config{RequestTxIdsFunc: func(
			CallbackContext, bool, uint16, uint16,
		) ([]TxIdAndSize, error) {
			return nil, nil
		}},
		&Config{},
	)
	client.Init()
	// A request only succeeds once the server has accepted Init.
	result, err := server.RequestTxIds(false, 1)
	require.NoError(t, err)
	require.Empty(t, result)
	select {
	case err := <-serverErrors:
		t.Fatalf("server rejected Init: %s", err)
	default:
	}
}

func TestLegacyInitFuncStillCalledAlongsideOnInit(t *testing.T) {
	t.Parallel()
	legacy := make(chan struct{}, 1)
	notified := make(chan struct{}, 1)
	client, _, _, _ := newHookPair(
		t,
		&Config{},
		&Config{
			InitFunc: func(CallbackContext) error {
				legacy <- struct{}{}
				return nil
			},
			OnInit: func(connection.ConnectionId) { notified <- struct{}{} },
		},
	)
	client.Init()
	for _, ch := range []chan struct{}{legacy, notified} {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("an Init callback was not called")
		}
	}
}

func TestOnDoneNotifiesServerWithConnectionId(t *testing.T) {
	t.Parallel()
	got := make(chan connection.ConnectionId, 1)
	client, server, connId, _ := newHookPair(
		t,
		&Config{RequestTxIdsFunc: func(
			CallbackContext, bool, uint16, uint16,
		) ([]TxIdAndSize, error) {
			return nil, ErrStopServerProcess
		}},
		&Config{OnDone: func(id connection.ConnectionId) { got <- id }},
	)
	client.Init()
	_, err := server.RequestTxIds(true, 1)
	require.ErrorIs(t, err, ErrStopServerProcess)
	select {
	case id := <-got:
		require.Equal(t, connId, id)
	case <-time.After(5 * time.Second):
		t.Fatal("OnDone was not called")
	}
}

func TestLegacyInitFuncErrorFailsProtocolAfterOnInit(t *testing.T) {
	t.Parallel()
	initErr := errors.New("init refused")
	notified := make(chan struct{}, 1)
	client, _, _, serverErrors := newHookPair(
		t,
		&Config{},
		&Config{
			InitFunc: func(CallbackContext) error { return initErr },
			OnInit:   func(connection.ConnectionId) { notified <- struct{}{} },
		},
	)
	client.Init()
	select {
	case <-notified:
	case <-time.After(5 * time.Second):
		t.Fatal("OnInit was not called")
	}
	select {
	case err := <-serverErrors:
		require.ErrorIs(t, err, initErr)
	case <-time.After(5 * time.Second):
		t.Fatal("InitFunc error did not fail the protocol")
	}
}
