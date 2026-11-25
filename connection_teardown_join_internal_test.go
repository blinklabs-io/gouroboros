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
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/keepalive"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

const teardownJoinTimeout = 5 * time.Second

// newKeepAliveCallbackConnection starts a node-to-node server connection
// whose peer sends one keep-alive request after the handshake, so the
// keep-alive server runs onKeepAlive on its receive loop.
func newKeepAliveCallbackConnection(
	t *testing.T,
	onKeepAlive func(c *Connection, ctx keepalive.CallbackContext),
) net.Conn {
	t.Helper()
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleServer,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestOutput,
			ouroboros_mock.ConversationEntryHandshakeNtNResponseInput,
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: keepalive.ProtocolId,
				Messages: []protocol.Message{
					keepalive.NewMsgKeepAlive(7),
				},
			},
		},
	)
	var conn *Connection
	_, err := NewConnection(
		WithConnection(mockConn),
		WithNodeToNode(true),
		WithServer(true),
		WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		WithKeepAliveConfig(keepalive.NewConfig()),
		func(c *Connection) {
			conn = c
			c.keepAliveConfig.KeepAliveFunc = func(
				ctx keepalive.CallbackContext,
				_ uint16,
			) error {
				onKeepAlive(conn, ctx)
				return nil
			}
		},
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = mockConn.Close() })
	return mockConn
}

// TestCloseWaitsForRunningCallback checks that Close returns only after a
// callback already running on a protocol loop has returned, so no callback
// runs once Close has returned. The reference mux waits for every
// mini-protocol thread before its run returns (network-mux Mux.hs:280-287,
// Control/Concurrent/JobPool.hs:105-109).
func TestCloseWaitsForRunningCallback(t *testing.T) {
	defer goleak.VerifyNone(t)
	entered := make(chan *Connection, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseCallback := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseCallback)
	newKeepAliveCallbackConnection(t, func(c *Connection, _ keepalive.CallbackContext) {
		entered <- c
		<-release
	})
	var conn *Connection
	select {
	case conn = <-entered:
	case <-time.After(teardownJoinTimeout):
		t.Fatal("keep-alive callback did not run")
	}
	closed := make(chan struct{})
	go func() {
		_ = conn.Close(context.Background())
		close(closed)
	}()
	// Close cannot return while the callback is held; this bounds how long
	// the test gives it to do so wrongly.
	select {
	case <-closed:
		t.Fatal("Close returned while a protocol callback was running")
	case <-time.After(500 * time.Millisecond):
	}
	releaseCallback()
	select {
	case <-closed:
	case <-time.After(teardownJoinTimeout):
		t.Fatal("Close did not return after the callback returned")
	}
}

// TestCallbackMayCloseItsConnection checks that a callback passing its
// context to Close on its own connection does not wait for its own
// protocol loop.
func TestCallbackMayCloseItsConnection(t *testing.T) {
	defer goleak.VerifyNone(t)
	returned := make(chan *Connection, 1)
	newKeepAliveCallbackConnection(t, func(c *Connection, ctx keepalive.CallbackContext) {
		_ = c.Close(ctx.Context())
		returned <- c
	})
	select {
	case conn := <-returned:
		select {
		case _, open := <-conn.ErrorChan():
			require.False(t, open, "Close returned before teardown finished")
		default:
			t.Fatal("Close returned before teardown finished")
		}
	case <-time.After(teardownJoinTimeout):
		t.Fatal("Close from a protocol callback did not return")
	}
}

// TestConcurrentCloseCallbacksReturn checks that callbacks of two
// protocol instances closing their connection at the same time both
// return: neither waits for the other, whose callback is itself closing.
// A later Close still waits for both. The reference mux tears down once
// and releases every thread that asked (network-mux Mux.hs:280-287).
func TestConcurrentCloseCallbacksReturn(t *testing.T) {
	defer goleak.VerifyNone(t)
	entered := make(chan *Connection, 1)
	bothEntered := make(chan struct{})
	returned := make(chan struct{}, 2)
	newKeepAliveCallbackConnection(t, func(c *Connection, ctx keepalive.CallbackContext) {
		entered <- c
		<-bothEntered
		_ = c.Close(ctx.Context())
		returned <- struct{}{}
	})
	var conn *Connection
	select {
	case conn = <-entered:
	case <-time.After(teardownJoinTimeout):
		t.Fatal("keep-alive callback did not run")
	}
	// A second protocol instance's loop, running a callback that closes
	// the connection with its own context.
	other := &protocol.Protocol{}
	otherCtx := other.CallbackContext()
	conn.setupMu.Lock()
	m := conn.muxer
	conn.setupMu.Unlock()
	require.True(t, m.StartProtocolLoops(other, func() {
		<-bothEntered
		_ = conn.Close(otherCtx)
		returned <- struct{}{}
	}))
	close(bothEntered)
	for range 2 {
		select {
		case <-returned:
		case <-time.After(teardownJoinTimeout):
			t.Fatal("Close waited for another callback that was closing")
		}
	}
	closed := make(chan struct{})
	go func() {
		_ = conn.Close(context.Background())
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(teardownJoinTimeout):
		t.Fatal("Close did not return after both callbacks returned")
	}
}

// TestCloseBeforeDialTearsDown checks that a Close made before Dial leaves
// nothing to set up: the later Dial fails without connecting.
func TestCloseBeforeDialTearsDown(t *testing.T) {
	defer goleak.VerifyNone(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	conn, err := NewConnection(
		WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		WithHandshakeTimeout(time.Second),
	)
	require.NoError(t, err)
	require.NoError(t, conn.Close(context.Background()))
	require.Error(t, conn.Dial("tcp", listener.Addr().String()))
	tcpListener, ok := listener.(*net.TCPListener)
	require.True(t, ok)
	require.NoError(
		t,
		tcpListener.SetDeadline(time.Now().Add(200*time.Millisecond)),
	)
	if accepted, err := listener.Accept(); err == nil {
		_ = accepted.Close()
		t.Fatal("Dial connected after Close")
	}
}

// TestErrorChanClosesAfterRunningCallback checks the join on the shutdown
// path that no Close starts: the peer goes away while a callback runs, and
// the connection reports itself closed (ErrorChan closes) only after the
// callback has returned.
func TestErrorChanClosesAfterRunningCallback(t *testing.T) {
	defer goleak.VerifyNone(t)
	entered := make(chan *Connection, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseCallback := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseCallback)
	mockConn := newKeepAliveCallbackConnection(t, func(c *Connection, _ keepalive.CallbackContext) {
		entered <- c
		<-release
	})
	var conn *Connection
	select {
	case conn = <-entered:
	case <-time.After(teardownJoinTimeout):
		t.Fatal("keep-alive callback did not run")
	}
	_ = mockConn.Close()
	errorChanClosed := make(chan struct{})
	go func() {
		for range conn.ErrorChan() {
		}
		close(errorChanClosed)
	}()
	select {
	case <-errorChanClosed:
		t.Fatal("ErrorChan closed while a protocol callback was running")
	case <-time.After(500 * time.Millisecond):
	}
	releaseCallback()
	select {
	case <-errorChanClosed:
	case <-time.After(teardownJoinTimeout):
		t.Fatal("ErrorChan did not close after the callback returned")
	}
}

// TestCloseWaitsForCallbackThatClosed checks that a callback closing its own
// connection is excused only from its own Close: another goroutine's Close
// still waits for that callback to return.
func TestCloseWaitsForCallbackThatClosed(t *testing.T) {
	defer goleak.VerifyNone(t)
	closedByCallback := make(chan *Connection, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseCallback := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseCallback)
	newKeepAliveCallbackConnection(t, func(c *Connection, ctx keepalive.CallbackContext) {
		_ = c.Close(ctx.Context())
		closedByCallback <- c
		<-release
	})
	var conn *Connection
	select {
	case conn = <-closedByCallback:
	case <-time.After(teardownJoinTimeout):
		t.Fatal("Close from a protocol callback did not return")
	}
	closed := make(chan struct{})
	go func() {
		_ = conn.Close(context.Background())
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("Close returned while a protocol callback was running")
	case <-time.After(500 * time.Millisecond):
	}
	releaseCallback()
	select {
	case <-closed:
	case <-time.After(teardownJoinTimeout):
		t.Fatal("Close did not return after the callback returned")
	}
}

// TestCloseFromAnotherConnectionWaits checks that Close
// compares the callback's owner by identity: a callback of connection B
// closing connection A with B's context still waits for A's running
// callback.
func TestCloseFromAnotherConnectionWaits(t *testing.T) {
	defer goleak.VerifyNone(t)
	enteredA := make(chan *Connection, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseCallback := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseCallback)
	newKeepAliveCallbackConnection(t, func(c *Connection, _ keepalive.CallbackContext) {
		enteredA <- c
		<-release
	})
	var connA *Connection
	select {
	case connA = <-enteredA:
	case <-time.After(teardownJoinTimeout):
		t.Fatal("connection A's callback did not run")
	}
	closedA := make(chan struct{})
	enteredB := make(chan *Connection, 1)
	newKeepAliveCallbackConnection(t, func(c *Connection, ctx keepalive.CallbackContext) {
		enteredB <- c
		_ = connA.Close(ctx.Context())
		close(closedA)
	})
	var connB *Connection
	select {
	case connB = <-enteredB:
	case <-time.After(teardownJoinTimeout):
		t.Fatal("connection B's callback did not run")
	}
	select {
	case <-closedA:
		t.Fatal("another connection's context excused A's running callback")
	case <-time.After(500 * time.Millisecond):
	}
	releaseCallback()
	select {
	case <-closedA:
	case <-time.After(teardownJoinTimeout):
		t.Fatal("Close did not return after A's callback returned")
	}
	require.NoError(t, connB.Close(context.Background()))
}
