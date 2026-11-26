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

package handshake_test

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	"github.com/stretchr/testify/require"
)

// serverAnswer sends a raw MsgProposeVersions payload to a node-to-node
// server with table local. It returns the server's answer, or the error
// the server reported without answering.
func serverAnswer(
	t *testing.T,
	local protocol.ProtocolVersionMap,
	payload []byte,
) (protocol.Message, error) {
	t.Helper()
	serverConn, peerConn := net.Pipe()
	deadline := time.Now().Add(2 * time.Second)
	require.NoError(t, serverConn.SetDeadline(deadline))
	require.NoError(t, peerConn.SetDeadline(deadline))
	m := muxer.New(serverConn)
	errorChan := make(chan error, 1)
	cfg := handshake.NewConfig(
		handshake.WithProtocolVersionMap(local),
		handshake.WithFinishedFunc(
			func(handshake.CallbackContext, uint16, protocol.VersionData) error {
				return nil
			},
		),
	)
	s := handshake.NewServer(protocol.ProtocolOptions{
		Muxer:     m,
		ErrorChan: errorChan,
		Mode:      protocol.ProtocolModeNodeToNode,
		Role:      protocol.ProtocolRoleServer,
	}, &cfg)
	s.Start()
	defer func() {
		_ = peerConn.Close()
		_ = serverConn.Close()
		_ = s.Stop(context.Background())
		_ = m.Stop(context.Background())
	}()
	m.Start()
	writeSegment(t, peerConn, muxer.NewSegment(handshake.ProtocolId, payload, false))
	segments := make(chan *muxer.Segment, 1)
	go func() {
		defer close(segments)
		var header muxer.SegmentHeader
		if err := binary.Read(peerConn, binary.BigEndian, &header); err != nil {
			return
		}
		payload := make([]byte, header.PayloadLength)
		if _, err := io.ReadFull(peerConn, payload); err != nil {
			return
		}
		segments <- &muxer.Segment{SegmentHeader: header, Payload: payload}
	}()
	select {
	case seg, ok := <-segments:
		require.True(t, ok, "connection closed without an answer")
		msgType := uint(seg.Payload[1])
		msg, err := handshake.NewMsgFromCbor(msgType, seg.Payload)
		require.NoError(t, err)
		return msg, nil
	case err := <-errorChan:
		return nil, err
	}
}

// A proposal may carry version numbers the receiver does not know: a newer
// release's, or another protocol family's. The reference skips them
// (Codec.hs decodeVersions) and negotiates among the rest; gOuroboros
// ignores them the same way, wherever they are.
func TestServerIgnoresUnknownProposedVersions(t *testing.T) {
	const magic = 764824073
	local := protocol.GetProtocolVersionMap(
		protocol.ProtocolModeNodeToNode, magic,
		protocol.DiffusionModeInitiatorAndResponder, false, false,
	)
	for _, test := range []struct {
		name  string
		table []byte
	}{
		// {14: data, 100: false}
		{"newer version after", []byte{0xa2, 0x0e, 0x84, 0x1a, 0x2d, 0x96, 0x4a, 0x09, 0xf4, 0x00, 0xf4, 0x18, 0x64, 0xf4}},
		// {100: false, 14: data}
		{"newer version first", []byte{0xa2, 0x18, 0x64, 0xf4, 0x0e, 0x84, 0x1a, 0x2d, 0x96, 0x4a, 0x09, 0xf4, 0x00, 0xf4}},
		// {14: data, 0x8017: false}: a node-to-client version on a node-to-node server
		{"other family", []byte{0xa2, 0x0e, 0x84, 0x1a, 0x2d, 0x96, 0x4a, 0x09, 0xf4, 0x00, 0xf4, 0x19, 0x80, 0x17, 0xf4}},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := append([]byte{0x82, 0x00}, test.table...)
			msg, err := serverAnswer(t, local, payload)
			require.NoError(t, err)
			accept, ok := msg.(*handshake.MsgAcceptVersion)
			require.True(t, ok, "got %T", msg)
			require.Equal(t, uint16(14), accept.Version)
		})
	}
}
