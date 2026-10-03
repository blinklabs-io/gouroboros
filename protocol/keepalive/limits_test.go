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

package keepalive

import (
	"bytes"
	"encoding/binary"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func writeLimitsSegments(t *testing.T, conn net.Conn, payload []byte) {
	t.Helper()
	const segmentMax = 0xffff
	for len(payload) > 0 {
		n := min(len(payload), segmentMax)
		segment := muxer.NewSegment(ProtocolId, payload[:n], false)
		buf := &bytes.Buffer{}
		require.NoError(
			t,
			binary.Write(buf, binary.BigEndian, &segment.SegmentHeader),
		)
		buf.Write(segment.Payload)
		require.NoError(
			t,
			conn.SetWriteDeadline(time.Now().Add(5*time.Second)),
		)
		if _, err := conn.Write(buf.Bytes()); err != nil {
			// The protocol may close the connection once it refuses the
			// message; the refusal is asserted through the error channel.
			return
		}
		payload = payload[n:]
	}
}

func newLimitsServer(t *testing.T) (net.Conn, chan error) {
	t.Helper()
	connA, connB := net.Pipe()
	m := muxer.New(connA)
	errs := make(chan error, 1)
	server := NewServer(
		protocol.ProtocolOptions{
			ConnectionId: connection.ConnectionId{
				LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
				RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
			},
			Muxer:     m,
			ErrorChan: errs,
			Mode:      protocol.ProtocolModeNodeToNode,
		},
		nil,
	)
	server.Start()
	m.Start()
	t.Cleanup(func() {
		server.Protocol.Stop()
		m.Stop()
		_ = connA.Close()
		_ = connB.Close()
	})
	return connB, errs
}

// TestServerRefusesOversizedKeepAlive proves a message larger than
// MaxPendingMessageBytes is refused. A keep-alive message is a few bytes, so
// anything near the limit is hostile.
func TestServerRefusesOversizedKeepAlive(t *testing.T) {
	t.Parallel()
	conn, errs := newLimitsServer(t)

	data, err := cbor.Encode([]any{
		MessageTypeKeepAlive,
		make([]byte, MaxPendingMessageBytes+1),
	})
	require.NoError(t, err)
	require.Greater(t, len(data), MaxPendingMessageBytes)
	writeLimitsSegments(t, conn, data)

	select {
	case err := <-errs:
		require.ErrorContains(t, err, "received oversized message")
		require.ErrorContains(t, err, strconv.Itoa(MaxPendingMessageBytes))
	case <-time.After(5 * time.Second):
		t.Fatal("oversized KeepAlive was not refused")
	}
}

// TestStateMapBoundsPendingMessageBytes pins the limit onto both active
// states, matching the reference implementation's byteLimitsKeepAlive.
func TestStateMapBoundsPendingMessageBytes(t *testing.T) {
	t.Parallel()
	for _, state := range []protocol.State{StateClient, StateServer} {
		entry, ok := StateMap[state]
		require.True(t, ok, "missing state %s", state)
		require.Equal(
			t,
			MaxPendingMessageBytes,
			entry.PendingMessageByteLimit,
			"state %s must bound pending message bytes",
			state,
		)
	}
}
