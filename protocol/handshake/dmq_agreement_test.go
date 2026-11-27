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
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// dmq-node agrees node-to-node data as Cardano does, except that peer
// sharing is disabled whenever the agreed diffusion mode is initiator-only
// (DMQ/NodeToNode/Version.hs acceptableVersion).
func TestServerAgreesDMQPeerSharingAsReference(t *testing.T) {
	const magic = 3141592
	for _, version := range protocol.GetProtocolVersionsDMQNtN() {
		for _, local := range []struct{ initiatorOnly, peerSharing bool }{
			{false, false}, {false, true}, {true, false}, {true, true},
		} {
			for _, remote := range []struct{ initiatorOnly, peerSharing bool }{
				{false, false}, {false, true}, {true, false}, {true, true},
			} {
				name := fmt.Sprintf("v%d local %+v remote %+v", version, local, remote)
				t.Run(name, func(t *testing.T) {
					initiatorOnly := local.initiatorOnly || remote.initiatorOnly
					wantPeerSharing := !initiatorOnly &&
						local.peerSharing && remote.peerSharing
					accept := runServerHandshake(
						t,
						protocol.ProtocolModeNodeToNode,
						protocol.ProtocolVersionMap{
							version: protocol.GetProtocolVersionMapDMQNtN(
								magic, local.initiatorOnly, local.peerSharing, false,
							)[version],
						},
						protocol.ProtocolVersionMap{
							version: protocol.GetProtocolVersionMapDMQNtN(
								magic, remote.initiatorOnly, remote.peerSharing, false,
							)[version],
						},
					)
					got, err := protocol.GetProtocolVersion(version).
						NewVersionDataFromCborFunc(accept.VersionData)
					require.NoError(t, err)
					require.Equal(t, initiatorOnly, got.DiffusionMode() == protocol.DiffusionModeInitiatorOnly)
					require.Equal(t, wantPeerSharing, got.PeerSharing())
				})
			}
		}
	}
}
