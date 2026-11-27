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
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Each case races two causes and repeats, because which cause wins is up to
// the scheduler and the bad outcome needs a particular one.
const errorOnceRounds = 200

// errorOnceHold is how long TestConnectionErrorArrivesAfterShutdownBegins
// holds shutdown's start open while watching for an early error.
const errorOnceHold = 250 * time.Millisecond

// newErrorOncePair returns a node-to-client client Connection whose
// handshake with a server Connection over net.Pipe has finished. No
// mini-protocol sends anything afterwards.
func newErrorOncePair(t *testing.T) (*Connection, *Connection) {
	t.Helper()
	clientRaw, serverRaw := net.Pipe()
	type result struct {
		conn *Connection
		err  error
	}
	clientResult := make(chan result, 1)
	serverResult := make(chan result, 1)
	go func() {
		c, err := NewConnection(
			WithConnection(clientRaw), WithNetworkMagic(42),
		)
		clientResult <- result{c, err}
	}()
	go func() {
		c, err := NewConnection(
			WithConnection(serverRaw), WithNetworkMagic(42), WithServer(true),
		)
		serverResult <- result{c, err}
	}()
	var client, server *Connection
	for client == nil || server == nil {
		select {
		case got := <-clientResult:
			require.NoError(t, got.err)
			client = got.conn
		case got := <-serverResult:
			require.NoError(t, got.err)
			server = got.conn
		case <-time.After(5 * time.Second):
			t.Fatal("handshake did not finish")
		}
	}
	return client, server
}

// closeAndCount closes c and returns how many errors its owned ErrorChan
// carried before closing.
func closeAndCount(t *testing.T, c *Connection) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, c.Close(ctx))
	count := 0
	for range c.ErrorChan() {
		count++
	}
	return count
}

// A muxer error and a protocol error raised together end one connection;
// both forwarders take theirs, and only the first may be delivered.
func TestConnectionDeliversOneErrorForRacingCauses(t *testing.T) {
	for round := range errorOnceRounds {
		client, server := newErrorOncePair(t)
		// The muxer error goes first: once a forwarder begins shutdown the
		// muxer closes its error channel, and protoErrorChan is never closed.
		client.muxer.ErrorChan() <- errors.New("injected muxer error")
		client.protoErrorChan <- errors.New("injected protocol error")
		// Let a forwarder begin shutdown, so Close does not win the race.
		select {
		case <-client.doneChan:
		case <-time.After(5 * time.Second):
			t.Fatal("no forwarder began shutdown")
		}
		got := closeAndCount(t, client)
		_ = server.Close(context.Background())
		require.LessOrEqual(t, got, 1, "round %d: %d errors delivered", round, got)
		require.Equal(t, 1, got, "round %d: the first cause was lost", round)
	}
}

// A muxer error that its forwarder receives after Close has begun shutdown
// is a consequence of the Close, and is not delivered.
func TestConnectionDeliversNoErrorAfterClose(t *testing.T) {
	for round := range errorOnceRounds {
		client, server := newErrorOncePair(t)
		client.muxer.ErrorChan() <- errors.New("injected muxer error")
		won := false
		client.onceClose.Do(func() {
			close(client.doneChan)
			won = true
		})
		got := closeAndCount(t, client)
		_ = server.Close(context.Background())
		if won {
			require.Zero(t, got, "round %d: error delivered after Close", round)
		}
	}
}

// The error that begins shutdown reaches ErrorChan only once doneChan, which
// every callback sees as ConnectionDoneChan, is closed: a consumer woken by
// the error can rely on the connection's callbacks seeing the shutdown. The
// test holds onceClose with doneChan still open, so any error the forwarder
// sends before shutdown begins is received while doneChan is open. Delivery
// gated on shutdown sends nothing until the hold ends, and nothing after it,
// since the test's hold is the one that began shutdown.
func TestConnectionErrorArrivesAfterShutdownBegins(t *testing.T) {
	client, server := newErrorOncePair(t)
	defer func() { _ = server.Close(context.Background()) }()
	held := make(chan struct{})
	release := make(chan struct{})
	go client.onceClose.Do(func() {
		close(held)
		<-release
		close(client.doneChan)
	})
	<-held
	client.muxer.ErrorChan() <- errors.New("injected muxer error")
	open := false
	select {
	case <-client.ErrorChan():
		select {
		case <-client.doneChan:
		default:
			open = true
		}
	// Bounds the hold only: a forwarder that would send before shutdown
	// has sent within it, and one that would not sends nothing either way.
	case <-time.After(errorOnceHold):
	}
	close(release)
	require.Zero(t, closeAndCount(t, client),
		"error delivered after the hold began shutdown")
	require.False(t, open, "error delivered before shutdown began")
}
