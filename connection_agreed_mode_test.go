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

package ouroboros_test

import (
	"net"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	"github.com/blinklabs-io/gouroboros/protocol/keepalive"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// TestServerEndsWithAgreedDiffusionMode covers an initiator-only server
// answering a full-duplex client. The reference server ends the handshake
// with the agreed data (Handshake/Server.hs 42-44), which is initiator-only
// when either side is, so the server runs as a responder only and refuses a
// segment sent by a responder, as the client's own initiator-only view
// requires.
func TestServerEndsWithAgreedDiffusionMode(t *testing.T) {
	t.Cleanup(func() { goleak.VerifyNone(t) })
	serverPipe, clientPipe := net.Pipe()
	t.Cleanup(func() {
		_ = serverPipe.Close()
		_ = clientPipe.Close()
	})
	const magic = 42

	type serverResult struct {
		conn *ouroboros.Connection
		err  error
	}
	serverChan := make(chan serverResult, 1)
	go func() {
		c, err := ouroboros.NewConnection(
			ouroboros.WithConnection(serverPipe),
			ouroboros.WithServer(true),
			ouroboros.WithNodeToNode(true),
			ouroboros.WithNetworkMagic(magic),
			ouroboros.WithFullDuplex(false),
		)
		serverChan <- serverResult{conn: c, err: err}
	}()

	clientMuxer := muxer.New(clientPipe)
	t.Cleanup(clientMuxer.Stop)
	finished := make(chan protocol.VersionData, 1)
	clientErrors := make(chan error, 1)
	config := handshake.NewConfig(
		handshake.WithProtocolVersionMap(protocol.GetProtocolVersionMap(
			protocol.ProtocolModeNodeToNode,
			magic,
			protocol.DiffusionModeInitiatorAndResponder,
			false,
			false,
		)),
		handshake.WithFinishedFunc(func(
			_ handshake.CallbackContext,
			_ uint16,
			data protocol.VersionData,
		) error {
			finished <- data
			return nil
		}),
	)
	client := handshake.NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{},
			RemoteAddr: &net.TCPAddr{},
		},
		Muxer:     clientMuxer,
		ErrorChan: clientErrors,
		Mode:      protocol.ProtocolModeNodeToNode,
	}, &config)
	t.Cleanup(client.Stop)
	client.Start()
	clientMuxer.StartOnce()

	var server serverResult
	select {
	case server = <-serverChan:
	case <-time.After(5 * time.Second):
		t.Fatal("server handshake did not finish")
	}
	require.NoError(t, server.err)
	t.Cleanup(func() { _ = server.conn.Close() })
	select {
	case data := <-finished:
		require.Equal(t, protocol.DiffusionModeInitiatorOnly, data.DiffusionMode())
	case err := <-clientErrors:
		t.Fatalf("client handshake failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("client handshake did not finish")
	}
	_, serverData := server.conn.ProtocolVersion()
	require.Equal(
		t,
		protocol.DiffusionModeInitiatorOnly,
		serverData.DiffusionMode(),
	)

	// A segment from a responder reaches a server whose muxer also runs
	// as an initiator; a responder-only muxer refuses it on arrival.
	segment := muxer.NewSegment(
		keepalive.ProtocolId,
		[]byte{0x82, 0x01, 0x00},
		true,
	)
	require.NotNil(t, segment)
	require.NoError(t, clientMuxer.Send(segment))
	select {
	case err := <-server.conn.ErrorChan():
		require.ErrorContains(t, err, "not configured as an initiator")
	case <-time.After(5 * time.Second):
		t.Fatal("server accepted a segment from a responder")
	}
}
