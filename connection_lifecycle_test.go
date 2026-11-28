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

package ouroboros_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/gouroboros/protocol/keepalive"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// These tests run two real Connections over a loopback TCP pair and tear
// them down from each source, checking on both ends what ErrorChan carries
// and when it closes.

const lifecycleTimeout = 5 * time.Second

const lifecycleMagic = 42

// loopbackPair returns both ends of a TCP connection on 127.0.0.1.
func loopbackPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			close(accepted)
			return
		}
		accepted <- conn
	}()
	client, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	server, ok := <-accepted
	require.True(t, ok, "accept failed")
	return client, server
}

// connectPair sets up a client and a server Connection over a loopback pair
// and waits for both handshakes.
func connectLifecyclePair(
	t *testing.T,
	clientOpts []ouroboros.ConnectionOptionFunc,
	serverOpts []ouroboros.ConnectionOptionFunc,
) (*ouroboros.Connection, *ouroboros.Connection) {
	t.Helper()
	clientRaw, serverRaw := loopbackPair(t)
	type result struct {
		conn *ouroboros.Connection
		err  error
	}
	clientResult := make(chan result, 1)
	serverResult := make(chan result, 1)
	go func() {
		conn, err := ouroboros.NewConnection(append([]ouroboros.ConnectionOptionFunc{
			ouroboros.WithConnection(clientRaw),
			ouroboros.WithNetworkMagic(lifecycleMagic),
		}, clientOpts...)...)
		clientResult <- result{conn, err}
	}()
	go func() {
		conn, err := ouroboros.NewConnection(append([]ouroboros.ConnectionOptionFunc{
			ouroboros.WithConnection(serverRaw),
			ouroboros.WithNetworkMagic(lifecycleMagic),
			ouroboros.WithServer(true),
		}, serverOpts...)...)
		serverResult <- result{conn, err}
	}()
	var client, server *ouroboros.Connection
	for client == nil || server == nil {
		select {
		case got := <-clientResult:
			require.NoError(t, got.err)
			client = got.conn
		case got := <-serverResult:
			require.NoError(t, got.err)
			server = got.conn
		case <-time.After(lifecycleTimeout):
			t.Fatal("handshake did not finish")
		}
	}
	return client, server
}

// drainErrors returns every error a Connection-owned ErrorChan carries until
// it closes.
func drainErrors(t *testing.T, errs <-chan error) []error {
	t.Helper()
	var got []error
	deadline := time.After(lifecycleTimeout)
	for {
		select {
		case err, ok := <-errs:
			if !ok {
				return got
			}
			got = append(got, err)
		case <-deadline:
			t.Fatalf("ErrorChan did not close; carried %v", got)
		}
	}
}

func closeBoth(client, server *ouroboros.Connection) {
	_ = client.Close(context.Background())
	_ = server.Close(context.Background())
}

// A local Close is a quiet teardown on its own end, and on the other end the
// peer's EOF arrives while every protocol is idle, which is also quiet.
func TestConnectionLifecycleLocalClose(t *testing.T) {
	defer goleak.VerifyNone(t)
	client, server := connectLifecyclePair(t, nil, nil)
	defer closeBoth(client, server)

	require.NoError(t, client.Close(context.Background()))
	require.Empty(t, drainErrors(t, client.ErrorChan()),
		"local Close delivered an error")
	require.Empty(t, drainErrors(t, server.ErrorChan()),
		"peer EOF with every protocol idle delivered an error")
}

// busyServer returns server options whose chain-sync FindIntersect callback
// signals entered and then blocks until release closes, leaving the server
// owing a reply.
func busyServer(
	entered chan<- struct{},
	release <-chan struct{},
) []ouroboros.ConnectionOptionFunc {
	cfg := chainsync.NewConfig(
		chainsync.WithFindIntersectFunc(
			func(
				chainsync.CallbackContext,
				[]pcommon.Point,
			) (pcommon.Point, chainsync.Tip, error) {
				entered <- struct{}{}
				<-release
				return pcommon.Point{}, chainsync.Tip{}, chainsync.ErrIntersectNotFound
			},
		),
	)
	return []ouroboros.ConnectionOptionFunc{ouroboros.WithChainSyncConfig(cfg)}
}

// requireOneErrorThenOpen checks that a Connection-owned ErrorChan has
// delivered exactly one error and stays open while a callback still runs:
// shutdown closes it only after joining the callbacks.
func requireOneErrorThenOpen(t *testing.T, errs <-chan error) error {
	t.Helper()
	var first error
	select {
	case err, ok := <-errs:
		require.True(t, ok, "ErrorChan closed before delivering the error")
		first = err
	case <-time.After(lifecycleTimeout):
		t.Fatal("no error delivered")
	}
	select {
	case err, ok := <-errs:
		if ok {
			t.Fatalf("second error delivered: %v", err)
		}
		t.Fatal("ErrorChan closed while a callback was still running")
	case <-time.After(50 * time.Millisecond):
	}
	return first
}

// The peer EOF arrives while the server owes a reply and its callback is
// still running. The server delivers exactly one ConnectionClosedError at
// once, and closes ErrorChan only after the callback returns. The client,
// which began the teardown with Close, delivers nothing.
func TestConnectionLifecyclePeerEOFWhileBusy(t *testing.T) {
	defer goleak.VerifyNone(t)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	client, server := connectLifecyclePair(t, nil, busyServer(entered, release))
	defer closeBoth(client, server)
	releaseOnce := sync.OnceFunc(func() { close(release) })
	defer releaseOnce()

	tipDone := make(chan struct{})
	go func() {
		defer close(tipDone)
		_, _ = client.ChainSync().Client.GetCurrentTip()
	}()
	select {
	case <-entered:
	case <-time.After(lifecycleTimeout):
		t.Fatal("FindIntersect callback did not run")
	}

	require.NoError(t, client.Close(context.Background()))
	<-tipDone
	require.Empty(t, drainErrors(t, client.ErrorChan()),
		"local Close delivered an error")

	err := requireOneErrorThenOpen(t, server.ErrorChan())
	var closed *muxer.ConnectionClosedError
	require.ErrorAs(t, err, &closed)
	releaseOnce()
	require.Empty(t, drainErrors(t, server.ErrorChan()),
		"a second error followed the first")
}

// A protocol error (the client's intersect timeout) delivers exactly one
// protocol error on the client; the server, still owing the reply, sees the
// client's close as exactly one ConnectionClosedError.
func TestConnectionLifecycleProtocolError(t *testing.T) {
	defer goleak.VerifyNone(t)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	clientCfg := chainsync.NewConfig(
		chainsync.WithIntersectTimeout(100 * time.Millisecond),
	)
	// Only node-to-node chain-sync has an intersect timeout.
	client, server := connectLifecyclePair(t,
		[]ouroboros.ConnectionOptionFunc{
			ouroboros.WithNodeToNode(true),
			ouroboros.WithChainSyncConfig(clientCfg),
		},
		append(
			busyServer(entered, release),
			ouroboros.WithNodeToNode(true),
		),
	)
	defer closeBoth(client, server)
	releaseOnce := sync.OnceFunc(func() { close(release) })
	defer releaseOnce()

	go func() { _, _ = client.ChainSync().Client.GetCurrentTip() }()
	select {
	case <-entered:
	case <-time.After(lifecycleTimeout):
		t.Fatal("FindIntersect callback did not run")
	}

	clientErrs := drainErrors(t, client.ErrorChan())
	require.Len(t, clientErrs, 1, "client errors: %v", clientErrs)
	require.Contains(t, clientErrs[0].Error(), "protocol error")

	err := requireOneErrorThenOpen(t, server.ErrorChan())
	var closed *muxer.ConnectionClosedError
	require.ErrorAs(t, err, &closed)
	releaseOnce()
	require.Empty(t, drainErrors(t, server.ErrorChan()))
}

// A keep-alive timeout is a protocol error on the client. The server never
// starts its muxer after the handshake, so it never answers.
func TestConnectionLifecycleKeepAliveTimeout(t *testing.T) {
	defer goleak.VerifyNone(t)
	keepAliveCfg := keepalive.NewConfig(
		keepalive.WithPeriod(20*time.Millisecond),
		keepalive.WithTimeout(100*time.Millisecond),
	)
	client, server := connectLifecyclePair(t,
		[]ouroboros.ConnectionOptionFunc{
			ouroboros.WithNodeToNode(true),
			ouroboros.WithKeepAlive(true),
			ouroboros.WithKeepAliveConfig(keepAliveCfg),
		},
		[]ouroboros.ConnectionOptionFunc{
			ouroboros.WithNodeToNode(true),
			ouroboros.WithDelayMuxerStart(true),
		},
	)
	defer closeBoth(client, server)

	clientErrs := drainErrors(t, client.ErrorChan())
	require.Len(t, clientErrs, 1, "client errors: %v", clientErrs)
	require.Contains(t, clientErrs[0].Error(), "protocol error")

	require.NoError(t, server.Close(context.Background()))
	require.Empty(t, drainErrors(t, server.ErrorChan()))
}

// A handshake timeout is returned by Dial, and ErrorChan carries nothing:
// setup began the teardown itself.
func TestConnectionLifecycleHandshakeTimeout(t *testing.T) {
	defer goleak.VerifyNone(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	silent := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			silent <- conn
		}
	}()

	conn, err := ouroboros.NewConnection(
		ouroboros.WithNetworkMagic(lifecycleMagic),
		ouroboros.WithHandshakeTimeout(100*time.Millisecond),
	)
	require.NoError(t, err)
	err = conn.DialTimeout("tcp", listener.Addr().String(), lifecycleTimeout)
	require.Error(t, err)
	require.True(t,
		strings.Contains(err.Error(), "handshake did not complete"),
		"unexpected Dial error: %v", err)
	require.Empty(t, drainErrors(t, conn.ErrorChan()))
	require.NoError(t, conn.Close(context.Background()))
	peer := <-silent
	_ = peer.Close()
}

// Close is idempotent and returns at once on a closed connection, from any
// goroutine.
func TestConnectionLifecycleCloseIdempotent(t *testing.T) {
	defer goleak.VerifyNone(t)
	client, server := connectLifecyclePair(t, nil, nil)
	defer closeBoth(client, server)

	results := make(chan error, 3)
	for range 3 {
		go func() { results <- client.Close(context.Background()) }()
	}
	for range 3 {
		select {
		case err := <-results:
			require.NoError(t, err)
		case <-time.After(lifecycleTimeout):
			t.Fatal("Close did not return")
		}
	}
	require.NoError(t, client.Close(context.Background()))
	require.Empty(t, drainErrors(t, client.ErrorChan()))
	require.Empty(t, drainErrors(t, server.ErrorChan()))
	require.False(t, errors.Is(server.Close(context.Background()), context.Canceled))
}
