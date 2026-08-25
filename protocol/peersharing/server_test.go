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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func readPeerSharingTestSegment(t *testing.T, conn net.Conn) *muxer.Segment {
	t.Helper()
	header := muxer.SegmentHeader{}
	require.NoError(t, binary.Read(conn, binary.BigEndian, &header))
	payload := make([]byte, header.PayloadLength)
	_, err := io.ReadFull(conn, payload)
	require.NoError(t, err)
	return &muxer.Segment{SegmentHeader: header, Payload: payload}
}

func writePeerSharingTestSegment(t *testing.T, conn net.Conn, segment *muxer.Segment) {
	t.Helper()
	require.NotNil(t, segment)
	buf := &bytes.Buffer{}
	require.NoError(t, binary.Write(buf, binary.BigEndian, &segment.SegmentHeader))
	_, err := buf.Write(segment.Payload)
	require.NoError(t, err)
	_, err = conn.Write(buf.Bytes())
	require.NoError(t, err)
}

// TestServerHandleShareRequestWhenLocalDisabled verifies that an unexpected
// ShareRequest gets an empty response when this node advertised NoPeerSharing.
func TestServerHandleShareRequestRefusesWhenLocalDisabled(t *testing.T) {
	cfg := NewConfig(WithLocalDisabled(true))
	connA, connB := net.Pipe()
	defer connA.Close()
	defer connB.Close()
	m := muxer.New(connA)
	defer m.Stop()
	server := NewServer(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{},
			RemoteAddr: &net.TCPAddr{},
		},
		Muxer: m,
	}, &cfg)
	server.Start()
	defer server.Protocol.Stop()
	m.Start()

	data, err := cbor.Encode(NewMsgShareRequest(5))
	require.NoError(t, err)
	writePeerSharingTestSegment(t, connB, muxer.NewSegment(ProtocolId, data, false))
	segment := readPeerSharingTestSegment(t, connB)
	var elems []cbor.RawMessage
	_, err = cbor.Decode(segment.Payload, &elems)
	require.NoError(t, err)
	require.NotEmpty(t, elems)
	var msgType uint
	_, err = cbor.Decode(elems[0], &msgType)
	require.NoError(t, err)
	require.Equal(t, uint(MessageTypeSharePeers), msgType)

	// A second request proves that the responder returned to Idle and did not
	// poison the shared muxer when the optional feature was disabled.
	writePeerSharingTestSegment(t, connB, muxer.NewSegment(ProtocolId, data, false))
	segment = readPeerSharingTestSegment(t, connB)
	_, err = cbor.Decode(segment.Payload, &elems)
	require.NoError(t, err)
	require.NotEmpty(t, elems)
	_, err = cbor.Decode(elems[0], &msgType)
	require.NoError(t, err)
	require.Equal(t, uint(MessageTypeSharePeers), msgType)
}

// TestServerHandleShareRequestWithoutCallback verifies that the zero-value
// configuration returns the protocol's empty response.
func TestServerHandleShareRequestRequiresCallback(t *testing.T) {
	cfg := NewConfig() // zero-value: LocalDisabled is false
	connA, connB := net.Pipe()
	defer connA.Close()
	defer connB.Close()
	m := muxer.New(connA)
	defer m.Stop()
	server := NewServer(protocol.ProtocolOptions{
		ConnectionId: testProtocolOptions().ConnectionId,
		Muxer:        m,
	}, &cfg)
	server.Start()
	defer server.Protocol.Stop()
	m.Start()
	data, err := cbor.Encode(NewMsgShareRequest(5))
	require.NoError(t, err)
	writePeerSharingTestSegment(t, connB, muxer.NewSegment(ProtocolId, data, false))
	segment := readPeerSharingTestSegment(t, connB)
	var elems []cbor.RawMessage
	_, err = cbor.Decode(segment.Payload, &elems)
	require.NoError(t, err)
	require.NotEmpty(t, elems)
	var msgType uint
	_, err = cbor.Decode(elems[0], &msgType)
	require.NoError(t, err)
	require.Equal(t, uint(MessageTypeSharePeers), msgType)
}
