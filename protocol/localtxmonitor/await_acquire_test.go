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
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func requireAcquireResult(t *testing.T, result <-chan error, errorChan <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		require.NoError(t, err)
	case err := <-errorChan:
		t.Fatalf("protocol error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Acquire did not complete")
	}
}

// TestAwaitAcquireIsNotTimed re-acquires from Acquired, which sends
// MsgAwaitAcquire. A node answers it only once its mempool differs from the
// held snapshot, which may take any time, so AcquireTimeout must not tear
// the connection down while the client waits.
func TestAwaitAcquireIsNotTimed(t *testing.T) {
	const acquireTimeout = 50 * time.Millisecond
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})
	m := muxer.New(connA)
	t.Cleanup(func() { _ = m.Stop(context.Background()) })
	errorChan := make(chan error, 10)
	cfg := NewConfig(WithAcquireTimeout(acquireTimeout))
	client := NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{},
			RemoteAddr: &net.TCPAddr{},
		},
		Muxer:     m,
		ErrorChan: errorChan,
		Mode:      protocol.ProtocolModeNodeToClient,
	}, &cfg)
	client.Start()
	m.Start()
	t.Cleanup(func() { _ = client.Protocol.Stop(context.Background()) })

	acquired := make(chan error, 1)
	go func() { acquired <- client.Acquire() }()
	requireQueryTestMessageType(t, connB, MessageTypeAcquire)
	writeQueryTestMessage(t, connB, NewMsgAcquired(1))
	requireAcquireResult(t, acquired, errorChan)

	go func() { acquired <- client.Acquire() }()
	requireQueryTestMessageType(t, connB, MessageTypeAcquire)
	select {
	case err := <-errorChan:
		t.Fatalf("MsgAwaitAcquire timed out: %v", err)
	case <-time.After(6 * acquireTimeout):
	}
	writeQueryTestMessage(t, connB, NewMsgAcquired(2))
	requireAcquireResult(t, acquired, errorChan)
}

// TestAcquireFromIdleIsTimed checks that an acquire from Idle, which the
// node answers at once, still honours AcquireTimeout.
func TestAcquireFromIdleIsTimed(t *testing.T) {
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})
	m := muxer.New(connA)
	t.Cleanup(func() { _ = m.Stop(context.Background()) })
	errorChan := make(chan error, 10)
	cfg := NewConfig(WithAcquireTimeout(50 * time.Millisecond))
	client := NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{},
			RemoteAddr: &net.TCPAddr{},
		},
		Muxer:     m,
		ErrorChan: errorChan,
		Mode:      protocol.ProtocolModeNodeToClient,
	}, &cfg)
	client.Start()
	m.Start()
	t.Cleanup(func() { _ = client.Protocol.Stop(context.Background()) })

	go func() { _ = client.Acquire() }()
	requireQueryTestMessageType(t, connB, MessageTypeAcquire)
	select {
	case err := <-errorChan:
		require.ErrorContains(t, err, "timeout waiting on transition from protocol state Acquiring")
	case <-time.After(5 * time.Second):
		t.Fatal("acquire from Idle was not timed")
	}
}
