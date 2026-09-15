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

package blockfetch_test

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/stretchr/testify/require"
)

type stopWriteGate struct {
	net.Conn
	started     chan struct{}
	release     chan struct{}
	once        sync.Once
	releaseOnce sync.Once
}

func (c *stopWriteGate) Write(p []byte) (int, error) {
	c.once.Do(func() { close(c.started) })
	<-c.release
	return c.Conn.Write(p)
}

func (c *stopWriteGate) unblock() {
	c.releaseOnce.Do(func() { close(c.release) })
}

func newStopDeliveryClient(t *testing.T) (
	*blockfetch.Client, *stopWriteGate, net.Conn,
) {
	t.Helper()
	local, peer := net.Pipe()
	gate := &stopWriteGate{
		Conn: local, started: make(chan struct{}), release: make(chan struct{}),
	}
	m := muxer.New(gate)
	c := blockfetch.NewClient(protocol.ProtocolOptions{
		Muxer: m, ErrorChan: make(chan error, 8),
	}, nil)
	t.Cleanup(func() {
		gate.unblock()
		_ = local.Close()
		_ = peer.Close()
		_ = c.Stop()
		m.Stop()
	})
	require.NoError(t, local.SetDeadline(time.Now().Add(3*time.Second)))
	require.NoError(t, peer.SetDeadline(time.Now().Add(3*time.Second)))
	m.Start()
	c.Start()
	return c, gate, peer
}

func TestClientStopWaitsForClientDoneDelivery(t *testing.T) {
	c, gate, peer := newStopDeliveryClient(t)
	stopped := make(chan error, 1)
	go func() { stopped <- c.Stop() }()
	select {
	case <-gate.started:
	case <-time.After(2 * time.Second):
		t.Fatal("ClientDone write did not start")
	}
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned before ClientDone delivery: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	gate.unblock()
	var header muxer.SegmentHeader
	require.NoError(t, binary.Read(peer, binary.BigEndian, &header))
	payload := make([]byte, header.PayloadLength)
	_, err := io.ReadFull(peer, payload)
	require.NoError(t, err)
	require.Equal(t, uint16(blockfetch.ProtocolId), header.ProtocolId)
	require.Equal(t, []byte{0x81, blockfetch.MessageTypeClientDone}, payload)
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not finish after ClientDone delivery")
	}
}

func TestClientStopBoundsBlockedClientDoneDelivery(t *testing.T) {
	c, gate, _ := newStopDeliveryClient(t)
	stopped := make(chan error, 1)
	go func() { stopped <- c.Stop() }()
	select {
	case <-gate.started:
	case <-time.After(2 * time.Second):
		t.Fatal("ClientDone write did not start")
	}
	// The write remains gated: Stop must report its delivery deadline rather
	// than claim success or require the remote reader to make progress.
	select {
	case err := <-stopped:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(2 * time.Second):
		t.Fatal("Stop failed to bound blocked delivery")
	}
}
