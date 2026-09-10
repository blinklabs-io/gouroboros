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
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	"github.com/stretchr/testify/require"
)

func TestServerRefusalDeliveryWaitCanBeCancelled(t *testing.T) {
	serverConn, peer := net.Pipe()
	defer serverConn.Close()
	defer peer.Close()
	require.NoError(t, peer.SetDeadline(time.Now().Add(5*time.Second)))
	m := muxer.New(serverConn)
	m.Start()
	defer m.Stop()
	version := uint16(12 + protocol.ProtocolVersionNtCOffset)
	cfg := handshake.NewConfig(
		handshake.WithProtocolVersionMap(protocol.ProtocolVersionMap{version: nil}),
		handshake.WithFinishedFunc(func(handshake.CallbackContext, uint16, protocol.VersionData) error {
			t.Error("refusal must not invoke acceptance callback")
			return nil
		}),
	)
	server := handshake.NewServer(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr: serverConn.LocalAddr(), RemoteAddr: serverConn.RemoteAddr(),
		},
		Muxer: m, ErrorChan: make(chan error, 1),
		Mode: protocol.ProtocolModeNodeToClient,
	}, &cfg)
	server.Start()
	defer server.Stop()
	proposal, err := cbor.Encode(handshake.NewMsgProposeVersions(
		protocol.ProtocolVersionMap{version: protocol.VersionDataNtC9to14(42)},
	))
	require.NoError(t, err)
	segment := muxer.NewSegment(handshake.ProtocolId, proposal, false)
	var wire bytes.Buffer
	require.NoError(t, binary.Write(&wire, binary.BigEndian, segment.SegmentHeader))
	wire.Write(segment.Payload)
	_, err = peer.Write(wire.Bytes())
	require.NoError(t, err)
	// Read only the response header. net.Pipe keeps the write blocked on the
	// remaining payload, so cancellation is exercised during actual delivery.
	header := make([]byte, 8)
	_, err = io.ReadFull(peer, header)
	require.NoError(t, err)
	require.Positive(t, binary.BigEndian.Uint16(header[6:]))
	server.Stop()
	select {
	case <-server.DoneChan():
	case <-time.After(5 * time.Second):
		t.Fatal("refusal delivery wait prevented protocol cancellation")
	}
}

func TestServerEmptyVersionDataRefusalReachesClientAsText(t *testing.T) {
	version := uint16(12 + protocol.ProtocolVersionNtCOffset)
	finished := func(handshake.CallbackContext, uint16, protocol.VersionData) error {
		t.Error("refused handshake must not invoke acceptance callback")
		return nil
	}
	serverCfg := handshake.NewConfig(
		handshake.WithProtocolVersionMap(protocol.ProtocolVersionMap{version: nil}),
		handshake.WithFinishedFunc(finished),
	)
	clientCfg := handshake.NewConfig(
		handshake.WithProtocolVersionMap(protocol.ProtocolVersionMap{
			version: protocol.VersionDataNtC9to14(42),
		}),
		handshake.WithFinishedFunc(finished),
	)
	clientErrors := startHandshakeWirePair(t, &serverCfg, &clientCfg)
	select {
	case err := <-clientErrors:
		var refusal *handshake.DecodeError
		require.ErrorAs(t, err, &refusal)
		require.Equal(t, version, refusal.Version)
		require.Equal(t,
			"handshake failed: refused due to empty version data", refusal.Message)
	case <-time.After(5 * time.Second):
		t.Fatal("client did not receive server refusal")
	}
}

func TestServerQueryReplyReachesClientBeforeTermination(t *testing.T) {
	version := uint16(15 + protocol.ProtocolVersionNtCOffset)
	data := protocol.VersionDataNtC15andUp{CborNetworkMagic: 42}
	versions := protocol.ProtocolVersionMap{version: data}
	finished := func(handshake.CallbackContext, uint16, protocol.VersionData) error {
		t.Error("query must not invoke acceptance callback")
		return nil
	}
	serverCfg := handshake.NewConfig(
		handshake.WithProtocolVersionMap(versions),
		handshake.WithFinishedFunc(finished),
	)
	data.CborQuery = true
	replies := make(chan protocol.ProtocolVersionMap, 1)
	clientCfg := handshake.NewConfig(
		handshake.WithProtocolVersionMap(protocol.ProtocolVersionMap{version: data}),
		handshake.WithQueryReplyFunc(func(_ handshake.CallbackContext, reply protocol.ProtocolVersionMap) error {
			replies <- reply
			return nil
		}),
	)
	clientErrors := startHandshakeWirePair(t, &serverCfg, &clientCfg)
	select {
	case reply := <-replies:
		require.Equal(t, versions, reply)
	case err := <-clientErrors:
		t.Fatalf("client failed before query reply: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("client did not receive query reply")
	}
}

func startHandshakeWirePair(t *testing.T, serverCfg, clientCfg *handshake.Config) <-chan error {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	serverMux := muxer.New(serverConn)
	clientMux := muxer.New(clientConn)
	serverMux.Start()
	clientMux.Start()
	serverErrors := make(chan error, 1)
	clientErrors := make(chan error, 1)
	server := handshake.NewServer(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr: serverConn.LocalAddr(), RemoteAddr: serverConn.RemoteAddr(),
		},
		Muxer: serverMux, ErrorChan: serverErrors,
		Mode: protocol.ProtocolModeNodeToClient,
	}, serverCfg)
	client := handshake.NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr: clientConn.LocalAddr(), RemoteAddr: clientConn.RemoteAddr(),
		},
		Muxer: clientMux, ErrorChan: clientErrors,
		Mode: protocol.ProtocolModeNodeToClient,
	}, clientCfg)
	t.Cleanup(func() {
		server.Stop()
		client.Stop()
		serverMux.Stop()
		clientMux.Stop()
		_ = serverConn.Close()
		_ = clientConn.Close()
		for _, done := range []<-chan struct{}{server.DoneChan(), client.DoneChan()} {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("handshake protocol did not stop")
			}
		}
	})
	server.Start()
	client.Start()
	return clientErrors
}
