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
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// TestCallbackStopsItsServer checks that a callback passing its context to
// its own server's Stop returns without waiting for the receive loop running
// it, and that the server is fully joined afterwards. A connection's
// handshake callbacks are its own, so this is tested without a connection.
func TestCallbackStopsItsServer(t *testing.T) {
	defer goleak.VerifyNone(t)
	serverConn, peerConn := net.Pipe()
	m := muxer.New(serverConn)
	versions := protocol.ProtocolVersionMap{
		10: protocol.VersionDataNtN7to10{
			CborNetworkMagic: ouroboros_mock.MockNetworkMagic,
		},
	}
	var s *handshake.Server
	stopped := make(chan error, 1)
	cfg := handshake.NewConfig(
		handshake.WithProtocolVersionMap(versions),
		handshake.WithFinishedFunc(
			func(ctx handshake.CallbackContext, _ uint16, _ protocol.VersionData) error {
				stopped <- s.Stop(ctx.Context())
				return nil
			},
		),
	)
	s = handshake.NewServer(protocol.ProtocolOptions{
		Muxer:     m,
		ErrorChan: make(chan error, 4),
		Mode:      protocol.ProtocolModeNodeToNode,
		Role:      protocol.ProtocolRoleServer,
	}, &cfg)
	s.Start()
	m.Start()
	defer func() {
		_ = peerConn.Close()
		_ = m.Stop(context.Background())
		for range m.ErrorChan() {
		}
	}()
	payload, err := cbor.Encode(handshake.NewMsgProposeVersions(versions))
	require.NoError(t, err)
	writeSegment(t, peerConn, muxer.NewSegment(handshake.ProtocolId, payload, false))
	// FinishedFunc runs once AcceptVersion is written.
	_ = readSegment(t, peerConn)
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stop from the callback did not return")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, s.Stop(ctx), "server was not joined")
}
