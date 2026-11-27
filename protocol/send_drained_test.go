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

package protocol

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

// readSegment reads one segment from conn and returns its payload.
func readSegment(conn net.Conn) ([]byte, error) {
	var header muxer.SegmentHeader
	if err := binary.Read(conn, binary.BigEndian, &header); err != nil {
		return nil, err
	}
	payload := make([]byte, header.PayloadLength)
	_, err := io.ReadFull(conn, payload)
	return payload, err
}

// TestWaitSendQueueDrainedBlockedBearer: while the peer reads nothing, a
// net.Pipe write cannot complete, so a queued message is never drained however
// long the waiter waits; once the peer reads the segment, the waiter reports
// drained.
func TestWaitSendQueueDrainedBlockedBearer(t *testing.T) {
	p, peer := newSendBeforeStartProtocol(t)
	p.Start()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, p.SendMessageContext(ctx, &MessageBase{MessageType: 1}))

	require.False(t, p.WaitSendQueueDrained(200*time.Millisecond),
		"drained before the peer read the message")

	type read struct {
		payload []byte
		err     error
	}
	readDone := make(chan read, 1)
	go func() {
		payload, err := readSegment(peer)
		readDone <- read{payload, err}
	}()
	require.True(t, p.WaitSendQueueDrained(5*time.Second))
	got := <-readDone
	require.NoError(t, got.err)
	require.Equal(t, []byte{0x81, 0x01}, got.payload)
}

// TestWaitSendQueueDrainedReturnsOnStop: a waiter blocked on a message the
// peer never reads returns "not drained" once the protocol stops, long before
// its timeout.
func TestWaitSendQueueDrainedReturnsOnStop(t *testing.T) {
	p, _ := newSendBeforeStartProtocol(t)
	p.Start()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, p.SendMessageContext(ctx, &MessageBase{MessageType: 1}))

	result := make(chan bool, 1)
	go func() { result <- p.WaitSendQueueDrained(time.Minute) }()
	require.NoError(t, p.Stop(ctx))
	select {
	case drained := <-result:
		require.False(t, drained)
	case <-ctx.Done():
		t.Fatal("WaitSendQueueDrained did not return after Stop")
	}
}

// TestWaitSendQueueDrainedReturnsOnWriteFailure: a message whose write fails
// is never drained, and its bytes stay counted, so the waiter returns "not
// drained" once the failed write tears the connection down.
func TestWaitSendQueueDrainedReturnsOnWriteFailure(t *testing.T) {
	p, peer := newSendBeforeStartProtocol(t)
	p.Start()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, p.SendMessageContext(ctx, &MessageBase{MessageType: 1}))

	result := make(chan bool, 1)
	go func() { result <- p.WaitSendQueueDrained(time.Minute) }()
	require.NoError(t, peer.Close())
	select {
	case drained := <-result:
		require.False(t, drained)
	case <-ctx.Done():
		t.Fatal("WaitSendQueueDrained did not return after the write failed")
	}
}
