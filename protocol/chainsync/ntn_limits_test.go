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

package chainsync

import (
	"context"
	"net"
	"testing"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// TestNtNIngressLimitHoldsTheIngressQueue checks that a node-to-node client
// and server still hold at least the specification's 462000-byte chain-sync
// ingress queue, which a full pipelined window of headers needs, although
// the per-state message limit is far smaller.
func TestNtNIngressLimitHoldsTheIngressQueue(t *testing.T) {
	t.Parallel()
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)
	t.Cleanup(func() { _ = clientMuxer.Stop(context.Background()) })
	t.Cleanup(func() { _ = serverMuxer.Stop(context.Background()) })

	cfg := NewConfig()
	client := NewClient(protocol.ProtocolOptions{
		Muxer:     clientMuxer,
		ErrorChan: make(chan error, 1),
		Mode:      protocol.ProtocolModeNodeToNode,
	}, &cfg)
	server := NewServer(protocol.ProtocolOptions{
		Muxer:     serverMuxer,
		ErrorChan: make(chan error, 1),
		Mode:      protocol.ProtocolModeNodeToNode,
	}, &cfg)
	client.Start()
	t.Cleanup(func() { _ = client.Stop(context.Background()) })
	server.Start()
	t.Cleanup(func() { _ = server.Stop(context.Background()) })

	require.GreaterOrEqual(t,
		clientMuxer.IngressLimit(ProtocolIdNtN, muxer.ProtocolRoleInitiator),
		MaxPendingMessageBytes)
	require.GreaterOrEqual(t,
		serverMuxer.IngressLimit(ProtocolIdNtN, muxer.ProtocolRoleResponder),
		MaxPendingMessageBytes)
}
