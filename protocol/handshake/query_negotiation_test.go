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

package handshake

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func queryTestData(magic uint32, query bool) protocol.VersionData {
	return protocol.VersionDataNtN13andUp{
		VersionDataNtN11to12: protocol.VersionDataNtN11to12{
			CborNetworkMagic: magic,
			CborQuery:        query,
		},
	}
}

// answerProposal runs a node-to-node server supporting versions 13 and 14
// with magic 42, sends it proposal, and returns the type of its answer.
func answerProposal(t *testing.T, proposal protocol.ProtocolVersionMap) uint8 {
	t.Helper()
	serverConn, peerConn := net.Pipe()
	m := muxer.New(serverConn)
	t.Cleanup(func() {
		_ = m.Stop(context.Background())
		_ = serverConn.Close()
		_ = peerConn.Close()
	})
	m.Start()
	cfg := NewConfig(
		WithProtocolVersionMap(protocol.ProtocolVersionMap{
			13: queryTestData(42, false),
			14: queryTestData(42, false),
		}),
		WithFinishedFunc(func(CallbackContext, uint16, protocol.VersionData) error {
			return nil
		}),
	)
	server := NewServer(protocol.ProtocolOptions{
		Muxer:     m,
		ErrorChan: make(chan error, 4),
		Mode:      protocol.ProtocolModeNodeToNode,
		Role:      protocol.ProtocolRoleServer,
	}, &cfg)
	server.Start()
	t.Cleanup(func() { _ = server.Stop(context.Background()) })
	data, err := cbor.Encode(NewMsgProposeVersions(proposal))
	require.NoError(t, err)
	segment := muxer.NewSegment(ProtocolId, data, false)
	if segment == nil {
		t.Fatal("failed to construct proposal segment")
	}
	buf := &bytes.Buffer{}
	require.NoError(t, binary.Write(buf, binary.BigEndian, &segment.SegmentHeader))
	buf.Write(segment.Payload)
	require.NoError(t, peerConn.SetDeadline(time.Now().Add(5*time.Second)))
	_, err = peerConn.Write(buf.Bytes())
	require.NoError(t, err)
	var header muxer.SegmentHeader
	require.NoError(t, binary.Read(peerConn, binary.BigEndian, &header))
	payload := make([]byte, header.PayloadLength)
	_, err = io.ReadFull(peerConn, payload)
	require.NoError(t, err)
	msgType, err := cbor.DecodeIdFromList(payload)
	require.NoError(t, err)
	return uint8(msgType) // #nosec G115 -- handshake message types are small
}

// TestServerAnswersQueryOnlyAfterNegotiation checks that the query flag is
// honoured only for a proposal the server would accept, as in the reference
// server (Handshake/Server.hs), which runs acceptOrRefuse first and answers
// MsgQueryReply only for accepted version data that asks for a query.
func TestServerAnswersQueryOnlyAfterNegotiation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		proposal protocol.ProtocolVersionMap
		want     uint8
	}{
		{"query", protocol.ProtocolVersionMap{14: queryTestData(42, true)}, MessageTypeQueryReply},
		{"no common version", protocol.ProtocolVersionMap{15: queryTestData(42, true)}, MessageTypeRefuse},
		{"magic mismatch", protocol.ProtocolVersionMap{14: queryTestData(43, true)}, MessageTypeRefuse},
		{"query on a version not chosen", protocol.ProtocolVersionMap{
			13: queryTestData(42, true),
			14: queryTestData(42, false),
		}, MessageTypeAcceptVersion},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, answerProposal(t, test.proposal))
		})
	}
}
