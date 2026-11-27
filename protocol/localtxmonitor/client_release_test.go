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

package localtxmonitor

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// startIdleTestClient starts a client on one end of a pipe without
// acquiring anything, returning the client, the peer end and the client's
// error channel.
func startIdleTestClient(t *testing.T) (*Client, net.Conn, chan error) {
	t.Helper()
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})
	m := muxer.New(connA)
	t.Cleanup(func() { _ = m.Stop(context.Background()) })
	errorChan := make(chan error, 10)
	client := NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{},
			RemoteAddr: &net.TCPAddr{},
		},
		Muxer:     m,
		ErrorChan: errorChan,
		Mode:      protocol.ProtocolModeNodeToClient,
	}, nil)
	client.Start()
	m.Start()
	t.Cleanup(func() { _ = client.Protocol.Stop(context.Background()) })
	return client, connB, errorChan
}

// requireNothingSent checks that the client writes nothing to the peer and
// reports no protocol error within a short window.
func requireNothingSent(t *testing.T, conn net.Conn, errorChan chan error) {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(100*time.Millisecond)))
	_, err := conn.Read(make([]byte, 1))
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		var netErr net.Error
		if !errors.As(err, &netErr) || netErr == nil || !netErr.Timeout() {
			t.Fatalf("client sent a message: %v", err)
		}
	}
	select {
	case err := <-errorChan:
		t.Fatalf("protocol error: %v", err)
	default:
	}
}

// TestReleaseWithNothingAcquired checks that Release from Idle returns
// ErrNotAcquired and sends nothing: MsgRelease is legal only in Acquired,
// and the send loop would refuse it and tear the connection down. The
// client stays usable: a following Acquire is answered.
func TestReleaseWithNothingAcquired(t *testing.T) {
	client, conn, errorChan := startIdleTestClient(t)
	require.ErrorIs(t, client.Release(), ErrNotAcquired)
	requireNothingSent(t, conn, errorChan)

	acquired := make(chan error, 1)
	go func() { acquired <- client.Acquire() }()
	requireQueryTestMessageType(t, conn, MessageTypeAcquire)
	writeQueryTestMessage(t, conn, NewMsgAcquired(1))
	select {
	case err := <-acquired:
		require.NoError(t, err)
	case err := <-errorChan:
		t.Fatalf("protocol error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Acquire did not complete")
	}

	require.NoError(t, client.Release())
	requireQueryTestMessageType(t, conn, MessageTypeRelease)
	require.ErrorIs(t, client.Release(), ErrNotAcquired)
	requireNothingSent(t, conn, errorChan)
}
