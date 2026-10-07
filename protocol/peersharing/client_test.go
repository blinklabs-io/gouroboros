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

package peersharing

import (
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

type notifyingLocker struct {
	sync.Locker
	attempts chan<- struct{}
}

func (l *notifyingLocker) Lock() {
	l.attempts <- struct{}{}
	l.Locker.Lock()
}

func notifyExchangeLockAttempts(client *Client) <-chan struct{} {
	attempts := make(chan struct{}, 2)
	client.exchangeLock = &notifyingLocker{
		Locker:   client.exchangeLock,
		attempts: attempts,
	}
	return attempts
}

func waitForExchangeLockAttempt(t *testing.T, attempts <-chan struct{}) {
	t.Helper()
	select {
	case <-attempts:
	case <-time.After(5 * time.Second):
		t.Fatal("GetPeers did not attempt to lock the exchange")
	}
}

func requirePeerAddressesEqual(
	t *testing.T,
	expected []PeerAddress,
	actual []PeerAddress,
) {
	t.Helper()
	require.Len(t, actual, len(expected))
	for i := range expected {
		require.True(t, expected[i].IP.Equal(actual[i].IP), "peer %d IP", i)
		require.Equal(t, expected[i].Port, actual[i].Port, "peer %d port", i)
	}
}

func TestClientGetPeersSerializesConcurrentExchanges(t *testing.T) {
	client, connB, errs := testPeerSharingClient(t)
	attempts := notifyExchangeLockAttempts(client)

	first := startGetPeers(t, client, connB, 5)
	waitForExchangeLockAttempt(t, attempts)
	second := make(chan peersResult, 1)
	go func() {
		peers, err := client.GetPeers(1)
		second <- peersResult{peers: peers, err: err}
	}()
	waitForExchangeLockAttempt(t, attempts)

	firstPeers := smallPeerAddresses(5)
	writeSharePeers(t, connB, firstPeers)
	select {
	case result := <-first:
		require.NoError(t, result.err)
		requirePeerAddressesEqual(t, firstPeers, result.peers)
	case <-time.After(5 * time.Second):
		t.Fatal("first GetPeers did not receive its reply")
	}

	readShareRequest(t, connB, 1)
	secondPeers := []PeerAddress{{IP: net.IPv4(192, 0, 2, 1), Port: 3002}}
	writeSharePeers(t, connB, secondPeers)
	select {
	case result := <-second:
		require.NoError(t, result.err)
		requirePeerAddressesEqual(t, secondPeers, result.peers)
	case <-time.After(5 * time.Second):
		t.Fatal("second GetPeers did not receive its reply")
	}
	select {
	case err := <-errs:
		t.Fatalf("protocol reported an error for serialized exchanges: %v", err)
	default:
	}
}

func TestClientGetPeersConcurrentRequestCannotWidenLimit(t *testing.T) {
	client, connB, errs := testPeerSharingClient(t)
	attempts := notifyExchangeLockAttempts(client)

	first := startGetPeers(t, client, connB, 1)
	waitForExchangeLockAttempt(t, attempts)
	second := make(chan peersResult, 1)
	go func() {
		peers, err := client.GetPeers(5)
		second <- peersResult{peers: peers, err: err}
	}()
	waitForExchangeLockAttempt(t, attempts)

	writeSharePeers(t, connB, smallPeerAddresses(2))
	select {
	case err := <-errs:
		require.ErrorIs(t, err, ErrTooManyPeersShared)
		require.ErrorContains(t, err, "requested 1, received 2")
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent request widened the active reply limit")
	}
	for name, resultChan := range map[string]<-chan peersResult{
		"first":  first,
		"second": second,
	} {
		select {
		case result := <-resultChan:
			require.ErrorIs(t, result.err, protocol.ErrProtocolShuttingDown, name)
		case <-time.After(5 * time.Second):
			t.Fatalf("%s GetPeers stayed blocked after protocol rejection", name)
		}
	}
}

func testProtocolOptions() protocol.ProtocolOptions {
	return protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{},
			RemoteAddr: &net.TCPAddr{},
		},
	}
}

// TestClientGetPeersRefusesWhenRemoteDisabled verifies that GetPeers returns
// ErrRemotePeerSharingDisabled without sending any message when the handshake
// recorded that the remote advertised NoPeerSharing.
func TestClientGetPeersRefusesWhenRemoteDisabled(t *testing.T) {
	cfg := NewConfig(WithRemoteDisabled(true))
	client := NewClient(testProtocolOptions(), &cfg)

	peers, err := client.GetPeers(5)
	require.ErrorIs(t, err, ErrRemotePeerSharingDisabled)
	require.Nil(t, peers)
}

// TestClientGetPeersReturnsAfterBusyTimeout verifies that the protocol's
// Busy-state timeout releases a caller when the peer accepts the request but
// never sends SharePeers.
func TestClientGetPeersReturnsAfterBusyTimeout(t *testing.T) {
	localConn, remoteConn := net.Pipe()
	m := muxer.New(localConn)
	errorChan := make(chan error, 1)
	peerDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, remoteConn)
		close(peerDone)
	}()

	cfg := NewConfig(WithTimeout(10 * time.Millisecond))
	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: connection.ConnectionId{
				LocalAddr:  localConn.LocalAddr(),
				RemoteAddr: localConn.RemoteAddr(),
			},
			Muxer:     m,
			ErrorChan: errorChan,
			Mode:      protocol.ProtocolModeNodeToNode,
		},
		&cfg,
	)
	client.Start()
	m.Start()
	t.Cleanup(func() {
		client.Protocol.Stop()
		m.Stop()
		_ = remoteConn.Close()
		select {
		case <-peerDone:
		case <-time.After(time.Second):
			t.Error("peer drain did not stop")
		}
	})

	resultChan := make(chan error, 1)
	go func() {
		_, err := client.GetPeers(5)
		resultChan <- err
	}()

	select {
	case err := <-errorChan:
		require.ErrorContains(t, err, "timeout waiting on transition")
	case <-time.After(time.Second):
		t.Fatal("protocol did not report the Busy-state timeout")
	}

	select {
	case err := <-resultChan:
		require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	case <-time.After(time.Second):
		// Release the pre-fix caller so a fail-before run does not leave a
		// goroutine behind after reporting the regression.
		select {
		case client.sharePeersChan <- nil:
		case <-time.After(time.Second):
			t.Fatal("GetPeers remained blocked and could not be released")
		}
		<-resultChan
		t.Fatal("GetPeers remained blocked after the protocol timed out")
	}

	lateHandlerDone := make(chan struct{})
	go func() {
		client.handleSharePeers(NewMsgSharePeers(nil))
		close(lateHandlerDone)
	}()
	select {
	case <-lateHandlerDone:
	case <-time.After(time.Second):
		t.Fatal("late SharePeers handler remained blocked after shutdown")
	}
}

// TestClientGetPeersReturnsAfterBusyTimeoutWithFullErrorChannel verifies that
// best-effort error reporting cannot prevent the timeout from stopping the
// protocol and releasing the caller.
func TestClientGetPeersReturnsAfterBusyTimeoutWithFullErrorChannel(t *testing.T) {
	localConn, remoteConn := net.Pipe()
	m := muxer.New(localConn)
	errorChan := make(chan error, 1)
	errorChan <- errors.New("undrained error")
	peerDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, remoteConn)
		close(peerDone)
	}()

	cfg := NewConfig(WithTimeout(10 * time.Millisecond))
	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: connection.ConnectionId{
				LocalAddr:  localConn.LocalAddr(),
				RemoteAddr: localConn.RemoteAddr(),
			},
			Muxer:     m,
			ErrorChan: errorChan,
			Mode:      protocol.ProtocolModeNodeToNode,
		},
		&cfg,
	)
	client.Start()
	m.Start()
	t.Cleanup(func() {
		client.Protocol.Stop()
		m.Stop()
		_ = remoteConn.Close()
		select {
		case <-peerDone:
		case <-time.After(time.Second):
			t.Error("peer drain did not stop")
		}
	})

	resultChan := make(chan error, 1)
	go func() {
		_, err := client.GetPeers(5)
		resultChan <- err
	}()

	select {
	case err := <-resultChan:
		require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	case <-time.After(time.Second):
		client.Protocol.Stop()
		select {
		case <-resultChan:
		case <-time.After(time.Second):
			t.Fatal("GetPeers remained blocked after explicit protocol stop")
		}
		t.Fatal("a full error channel prevented timeout shutdown")
	}
}

// TestConfigDefaultsArePermissive guards the inverted-flag contract: the zero
// value of Config preserves legacy behaviour, so direct callers of New that
// do not perform a handshake are not silently broken. The connection layer
// is the only place that should set these flags to true (via the handshake
// outcome).
func TestConfigDefaultsArePermissive(t *testing.T) {
	cfg := NewConfig()
	require.False(t, cfg.LocalDisabled, "default Config.LocalDisabled must be false")
	require.False(t, cfg.RemoteDisabled, "default Config.RemoteDisabled must be false")
}
