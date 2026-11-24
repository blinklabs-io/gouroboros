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
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	"github.com/stretchr/testify/require"
)

// The server sends the negotiated version data in MsgAcceptVersion, as the
// reference server does (acceptableVersion local remote): the diffusion mode
// is initiator-only if either side is, peer sharing and Peras support need
// both sides, and query is set if either side asks.
func TestServerAcceptsWithNegotiatedVersionData(t *testing.T) {
	const magic = 764824073
	const ntc uint16 = protocol.ProtocolVersionNtCOffset
	for _, test := range []struct {
		name   string
		mode   protocol.ProtocolMode
		ver    uint16
		server protocol.VersionData
		client protocol.VersionData
		want   protocol.VersionData
	}{
		{
			name: "node-to-node v16 every field",
			mode: protocol.ProtocolModeNodeToNode,
			ver:  16,
			server: protocol.VersionDataNtN16andUp{
				CborNetworkMagic: magic,
				CborPeerSharing:  protocol.PeerSharingModePeerSharingPublic,
				CborPerasSupport: true,
			},
			client: protocol.VersionDataNtN16andUp{
				CborNetworkMagic:                       magic,
				CborInitiatorAndResponderDiffusionMode: protocol.DiffusionModeInitiatorOnly,
			},
			want: protocol.VersionDataNtN16andUp{
				CborNetworkMagic:                       magic,
				CborInitiatorAndResponderDiffusionMode: protocol.DiffusionModeInitiatorOnly,
			},
		},
		{
			name: "node-to-node v16 both offer peer sharing and Peras",
			mode: protocol.ProtocolModeNodeToNode,
			ver:  16,
			server: protocol.VersionDataNtN16andUp{
				CborNetworkMagic:                       magic,
				CborInitiatorAndResponderDiffusionMode: protocol.DiffusionModeInitiatorOnly,
				CborPeerSharing:                        protocol.PeerSharingModePeerSharingPublic,
				CborPerasSupport:                       true,
			},
			client: protocol.VersionDataNtN16andUp{
				CborNetworkMagic: magic,
				CborPeerSharing:  protocol.PeerSharingModePeerSharingPublic,
				CborPerasSupport: true,
			},
			want: protocol.VersionDataNtN16andUp{
				CborNetworkMagic:                       magic,
				CborInitiatorAndResponderDiffusionMode: protocol.DiffusionModeInitiatorOnly,
				CborPeerSharing:                        protocol.PeerSharingModePeerSharingPublic,
				CborPerasSupport:                       true,
			},
		},
		{
			name: "node-to-node v14",
			mode: protocol.ProtocolModeNodeToNode,
			ver:  14,
			server: protocol.VersionDataNtN13andUp{
				VersionDataNtN11to12: protocol.VersionDataNtN11to12{
					CborNetworkMagic: magic,
					CborPeerSharing:  protocol.PeerSharingModePeerSharingPublic,
				},
			},
			client: protocol.VersionDataNtN13andUp{
				VersionDataNtN11to12: protocol.VersionDataNtN11to12{
					CborNetworkMagic:                       magic,
					CborInitiatorAndResponderDiffusionMode: protocol.DiffusionModeInitiatorOnly,
				},
			},
			want: protocol.VersionDataNtN13andUp{
				VersionDataNtN11to12: protocol.VersionDataNtN11to12{
					CborNetworkMagic:                       magic,
					CborInitiatorAndResponderDiffusionMode: protocol.DiffusionModeInitiatorOnly,
				},
			},
		},
		{
			name: "node-to-node v11 peer sharing levels",
			mode: protocol.ProtocolModeNodeToNode,
			ver:  11,
			server: protocol.VersionDataNtN11to12{
				CborNetworkMagic: magic,
				CborPeerSharing:  protocol.PeerSharingModeV11PeerSharingPublic,
			},
			client: protocol.VersionDataNtN11to12{
				CborNetworkMagic: magic,
				CborPeerSharing:  protocol.PeerSharingModeV11PeerSharingPrivate,
			},
			want: protocol.VersionDataNtN11to12{
				CborNetworkMagic: magic,
				CborPeerSharing:  protocol.PeerSharingModeV11PeerSharingPrivate,
			},
		},
		{
			name: "node-to-node v10",
			mode: protocol.ProtocolModeNodeToNode,
			ver:  10,
			server: protocol.VersionDataNtN7to10{
				CborNetworkMagic: magic,
			},
			client: protocol.VersionDataNtN7to10{
				CborNetworkMagic:                       magic,
				CborInitiatorAndResponderDiffusionMode: protocol.DiffusionModeInitiatorOnly,
			},
			want: protocol.VersionDataNtN7to10{
				CborNetworkMagic:                       magic,
				CborInitiatorAndResponderDiffusionMode: protocol.DiffusionModeInitiatorOnly,
			},
		},
		{
			name: "node-to-client v16",
			mode: protocol.ProtocolModeNodeToClient,
			ver:  16 + ntc,
			server: protocol.VersionDataNtC15andUp{
				CborNetworkMagic: magic,
			},
			client: protocol.VersionDataNtC15andUp{
				CborNetworkMagic: magic,
			},
			want: protocol.VersionDataNtC15andUp{
				CborNetworkMagic: magic,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			accept := runServerHandshake(
				t,
				test.mode,
				protocol.ProtocolVersionMap{test.ver: test.server},
				protocol.ProtocolVersionMap{test.ver: test.client},
			)
			require.Equal(t, test.ver, accept.Version)
			got, err := protocol.GetProtocolVersion(test.ver).
				NewVersionDataFromCborFunc(accept.VersionData)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

// runServerHandshake sends proposal to a server with table local and returns
// the MsgAcceptVersion it answers with.
func runServerHandshake(
	t *testing.T,
	mode protocol.ProtocolMode,
	local protocol.ProtocolVersionMap,
	proposal protocol.ProtocolVersionMap,
) *handshake.MsgAcceptVersion {
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
		Mode:      mode,
		Role:      protocol.ProtocolRoleServer,
	}, &cfg)
	s.Start()
	defer func() {
		_ = peerConn.Close()
		_ = serverConn.Close()
		s.Stop()
		m.Stop()
	}()
	m.Start()

	payload, err := cbor.Encode(handshake.NewMsgProposeVersions(proposal))
	require.NoError(t, err)
	writeSegment(
		t,
		peerConn,
		muxer.NewSegment(handshake.ProtocolId, payload, false),
	)
	segment := readSegment(t, peerConn)
	require.True(t, segment.IsResponse())
	msg, err := handshake.NewMsgFromCbor(
		handshake.MessageTypeAcceptVersion,
		segment.Payload,
	)
	require.NoError(t, err)
	accept, ok := msg.(*handshake.MsgAcceptVersion)
	require.True(t, ok)
	return accept
}
