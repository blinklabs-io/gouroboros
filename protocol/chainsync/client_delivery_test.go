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
	"context"
	"encoding/binary"
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

type doneWriteGate struct {
	net.Conn
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *doneWriteGate) Write(p []byte) (int, error) {
	c.once.Do(func() { close(c.started) })
	<-c.release
	return c.Conn.Write(p)
}

func newGatedClient(t *testing.T) (*Client, *doneWriteGate, net.Conn, *muxer.Muxer) {
	t.Helper()
	local, peer := net.Pipe()
	gate := &doneWriteGate{
		Conn: local, started: make(chan struct{}), release: make(chan struct{}),
	}
	m := muxer.New(gate)
	client := NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{},
			RemoteAddr: &net.TCPAddr{},
		},
		Muxer:     m,
		Mode:      protocol.ProtocolModeNodeToClient,
		ErrorChan: make(chan error, 8),
	}, nil)
	require.NoError(t, local.SetDeadline(time.Now().Add(3*time.Second)))
	require.NoError(t, peer.SetDeadline(time.Now().Add(3*time.Second)))
	t.Cleanup(func() {
		select {
		case <-gate.release:
		default:
			close(gate.release)
		}
		_ = local.Close()
		_ = peer.Close()
		_ = client.Stop()
		m.Stop()
	})
	m.Start()
	client.Start()
	return client, gate, peer, m
}

func TestClientStopWaitsForDoneDelivery(t *testing.T) {
	client, gate, peer, _ := newGatedClient(t)
	stopped := make(chan error, 1)
	go func() { stopped <- client.Stop() }()
	select {
	case <-gate.started:
	case <-time.After(2 * time.Second):
		t.Fatal("Done write did not start")
	}
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned before Done delivery: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(gate.release)
	var header muxer.SegmentHeader
	require.NoError(t, binary.Read(peer, binary.BigEndian, &header))
	payload := make([]byte, header.PayloadLength)
	_, err := io.ReadFull(peer, payload)
	require.NoError(t, err)
	require.Equal(t, uint16(ProtocolIdNtC), header.ProtocolId)
	msg, err := NewMsgFromCborNtC(uint(MessageTypeDone), payload)
	require.NoError(t, err)
	require.Equal(t, uint8(MessageTypeDone), msg.Type())
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not finish after Done delivery")
	}
}

func TestClientStopBoundsBlockedDoneDelivery(t *testing.T) {
	client, gate, _, _ := newGatedClient(t)
	stopped := make(chan error, 1)
	go func() { stopped <- client.Stop() }()
	select {
	case <-gate.started:
	case <-time.After(2 * time.Second):
		t.Fatal("Done write did not start")
	}
	select {
	case err := <-stopped:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(2 * time.Second):
		t.Fatal("Stop failed to bound blocked Done delivery")
	}
}
