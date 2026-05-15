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
	"net"
	"testing"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func testProtocolOptions() protocol.ProtocolOptions {
	return protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{},
			RemoteAddr: &net.TCPAddr{},
		},
	}
}

// TestClientGetPeersRefusesWhenRemoteDisabled verifies that GetPeers returns
// ErrRemotePeerSharingDisabled without sending any message when the handshake
// recorded that the remote advertised NoPeerSharing.
func TestClientGetPeersRefusesWhenRemoteDisabled(t *testing.T) {
	cfg := NewConfig(WithRemoteDisabled(true))
	client := NewClient(testProtocolOptions(), &cfg)

	peers, err := client.GetPeers(5)
	require.ErrorIs(t, err, ErrRemotePeerSharingDisabled)
	require.Nil(t, peers)
}

// TestConfigDefaultsArePermissive guards the inverted-flag contract: the zero
// value of Config preserves legacy behaviour, so direct callers of New that
// do not perform a handshake are not silently broken. The connection layer
// is the only place that should set these flags to true (via the handshake
// outcome).
func TestConfigDefaultsArePermissive(t *testing.T) {
	cfg := NewConfig()
	require.False(t, cfg.LocalDisabled, "default Config.LocalDisabled must be false")
	require.False(t, cfg.RemoteDisabled, "default Config.RemoteDisabled must be false")
}
