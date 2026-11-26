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
	"io"
	"net"
	"testing"
	"time"

	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// silentPeer is the far end of a connection whose peer reads the
// handshake proposal and never answers.
type silentPeer struct {
	conn net.Conn
	// proposed is closed once the first bytes of the proposal arrive,
	// which happens only after setup has started the handshake protocol.
	proposed chan struct{}
}

// dialSilentPeer runs setupConnection against a silentPeer. It returns
// the connection when setup is about to create its handshake protocol,
// the peer, and a channel that receives setup's result.
func dialSilentPeer(
	t *testing.T,
	timeout time.Duration,
	hook func(c *Connection),
) (*Connection, *silentPeer, <-chan error) {
	t.Helper()
	local, peerConn := net.Pipe()
	peer := &silentPeer{conn: peerConn, proposed: make(chan struct{})}
	go func() {
		buf := make([]byte, 1)
		if _, err := io.ReadFull(peerConn, buf); err != nil {
			return
		}
		close(peer.proposed)
		_, _ = io.Copy(io.Discard, peerConn)
	}()
	created := make(chan *Connection, 1)
	result := make(chan error, 1)
	go func() {
		_, err := New(
			WithConnection(local),
			WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
			WithHandshakeTimeout(timeout),
			func(c *Connection) {
				c.beforeHandshakeHook = func() {
					if hook != nil {
						hook(c)
					}
					created <- c
				}
			},
		)
		result <- err
	}()
	select {
	case c := <-created:
		return c, peer, result
	case err := <-result:
		t.Fatalf("setup returned before creating the handshake: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("setup did not reach the handshake")
	}
	return nil, nil, nil
}

// TestHandshakeTimeoutTearsDownOnce checks the handshake timer case: it
// closes the connection, Dial returns only after the teardown has finished,
// and no mini-protocol was created.
func TestHandshakeTimeoutTearsDownOnce(t *testing.T) {
	defer goleak.VerifyNone(t)
	c, peer, result := dialSilentPeer(t, 50*time.Millisecond, nil)
	defer func() { _ = peer.conn.Close() }()
	var err error
	select {
	case err = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("handshake timeout did not end setup")
	}
	require.ErrorContains(t, err, "handshake did not complete within")
	select {
	case <-c.connClosedChan:
	default:
		t.Fatal("Dial returned before the teardown finished")
	}
	c.protocolMu.RLock()
	defer c.protocolMu.RUnlock()
	require.False(t, c.protocolsReady, "mini-protocols set up after a handshake timeout")
	require.Nil(t, c.keepAlive)
	require.Nil(t, c.chainSync)
	// A second Close finds everything torn down and returns at once.
	require.NoError(t, c.Close(context.Background()))
}

// TestCloseBeforeHandshakeCreatesNone checks that a Close landing after the
// muxer is created and before the handshake protocol is tears the
// connection down without it, and setup does not create and start one
// afterwards. Shutdown reads c.handshake under protocolMu; a handshake
// published without it, after shutdown has read none, is a data race and
// an instance nothing stops.
func TestCloseBeforeHandshakeCreatesNone(t *testing.T) {
	defer goleak.VerifyNone(t)
	c, peer, result := dialSilentPeer(t, time.Minute, func(c *Connection) {
		require.NoError(t, c.Close(context.Background()))
	})
	defer func() { _ = peer.conn.Close() }()
	var err error
	select {
	case err = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("setup did not return after Close")
	}
	require.ErrorIs(t, err, io.EOF, "setup did not report the shutdown: %v", err)
	c.protocolMu.RLock()
	defer c.protocolMu.RUnlock()
	require.Nil(t, c.handshake, "setup created a handshake protocol after the teardown")
}
