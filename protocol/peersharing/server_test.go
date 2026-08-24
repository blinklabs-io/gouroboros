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
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func readPeerSharingSegment(t *testing.T, conn net.Conn) *muxer.Segment {
	t.Helper()
	header := muxer.SegmentHeader{}
	require.NoError(t, binary.Read(conn, binary.BigEndian, &header))
	payload := make([]byte, header.PayloadLength)
	_, err := io.ReadFull(conn, payload)
	require.NoError(t, err)
	return &muxer.Segment{SegmentHeader: header, Payload: payload}
}

func writePeerSharingSegment(t *testing.T, conn net.Conn, segment *muxer.Segment) {
	t.Helper()
	buf := &bytes.Buffer{}
	require.NoError(t, binary.Write(buf, binary.BigEndian, &segment.SegmentHeader))
	_, err := buf.Write(segment.Payload)
	require.NoError(t, err)
	_, err = conn.Write(buf.Bytes())
	require.NoError(t, err)
}

func sendPeerSharingRequest(t *testing.T, conn net.Conn, amount uint8) {
	t.Helper()
	data, err := cbor.Encode(NewMsgShareRequest(amount))
	require.NoError(t, err)
	writePeerSharingSegment(t, conn, muxer.NewSegment(ProtocolId, data, false))
}

func responsePeerCount(t *testing.T, segment *muxer.Segment) int {
	t.Helper()
	var values []cbor.RawMessage
	_, err := cbor.Decode(segment.Payload, &values)
	require.NoError(t, err)
	require.Len(t, values, 2)
	var peers []PeerAddress
	_, err = cbor.Decode(values[1], &peers)
	require.NoError(t, err)
	return len(peers)
}

func testPeerSharingServer(t *testing.T, cfg *Config) net.Conn {
	t.Helper()
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	connA, connB := net.Pipe()
	m := muxer.New(connA)
	server := NewServer(protocol.ProtocolOptions{ConnectionId: connId, Muxer: m}, cfg)
	server.Start()
	m.Start()
	t.Cleanup(func() {
		server.Stop()
		m.Stop()
		connA.Close()
		connB.Close()
	})
	return connB
}

func TestServerDisabledRequestReturnsEmptyResponseAndRemainsUsable(t *testing.T) {
	called := false
	cfg := NewConfig(
		WithLocalDisabled(true),
		WithShareRequestFunc(func(CallbackContext, int) ([]PeerAddress, error) {
			called = true
			return nil, nil
		}),
	)
	connB := testPeerSharingServer(t, &cfg)

	for range 2 {
		sendPeerSharingRequest(t, connB, 5)
		require.NoError(t, connB.SetReadDeadline(time.Now().Add(time.Second)))
		segment := readPeerSharingSegment(t, connB)
		require.True(t, segment.IsResponse())
		require.Equal(t, uint16(ProtocolId), segment.GetProtocolId())
		require.Equal(t, 0, responsePeerCount(t, segment))
	}
	require.False(t, called, "disabled peer sharing must not invoke the callback")
}

func TestServerUnconfiguredRequestReturnsEmptyResponse(t *testing.T) {
	cfg := NewConfig()
	connB := testPeerSharingServer(t, &cfg)
	sendPeerSharingRequest(t, connB, 5)
	require.NoError(t, connB.SetReadDeadline(time.Now().Add(time.Second)))
	segment := readPeerSharingSegment(t, connB)
	require.Equal(t, 0, responsePeerCount(t, segment))
}
