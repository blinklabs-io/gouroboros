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
	"net"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// The reference client ends with acceptVersion vData vData', its own
// agreement of its data with the data in MsgAcceptVersion (Client.hs), not
// the server's data as sent. A server that sends its own data, as
// gOuroboros servers did before they sent the agreed data, must not make an
// initiator-only client without peer sharing end as full duplex with peer
// sharing.
func TestClientEndsWithItsAgreement(t *testing.T) {
	const magic = 764824073
	own := protocol.GetProtocolVersionMapWithPerasSupport(
		protocol.ProtocolModeNodeToNode, magic,
		protocol.DiffusionModeInitiatorOnly, false, false, false,
	)
	serverOwn := protocol.GetProtocolVersionMapWithPerasSupport(
		protocol.ProtocolModeNodeToNode, magic,
		protocol.DiffusionModeInitiatorAndResponder, true, true, true,
	)
	for _, version := range []uint16{14, 15, 16} {
		var got protocol.VersionData
		cfg := NewConfig(
			WithProtocolVersionMap(own),
			WithFinishedFunc(
				func(_ CallbackContext, _ uint16, data protocol.VersionData) error {
					got = data
					return nil
				},
			),
		)
		client := NewClient(protocol.ProtocolOptions{
			ConnectionId: connection.ConnectionId{
				LocalAddr:  &net.TCPAddr{},
				RemoteAddr: &net.TCPAddr{},
			},
			Mode: protocol.ProtocolModeNodeToNode,
		}, &cfg)
		data := serverOwn[version]
		encoded, err := cbor.Encode(&data)
		require.NoError(t, err)
		require.NoError(t, client.handleAcceptVersion(&MsgAcceptVersion{
			MessageBase: protocol.MessageBase{MessageType: MessageTypeAcceptVersion},
			Version:     version,
			VersionData: encoded,
		}))
		require.NotNil(t, got)
		require.Equal(t, protocol.DiffusionModeInitiatorOnly, got.DiffusionMode(), "version %d", version)
		require.False(t, got.PeerSharing(), "version %d", version)
		require.True(t, got.Query(), "version %d: query is set if either side asks", version)
		if peras, ok := got.(interface{ PerasSupported() bool }); ok {
			require.False(t, peras.PerasSupported(), "version %d", version)
		}
	}
}
