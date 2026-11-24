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
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	"github.com/stretchr/testify/require"
)

func TestSegmentedHandshake(t *testing.T) {
	for _, nodeToNode := range []bool{false, true} {
		mode := protocol.ProtocolModeNodeToClient
		version := uint16(10 + protocol.ProtocolVersionNtCOffset)
		if nodeToNode {
			mode = protocol.ProtocolModeNodeToNode
			version = 14
		}
		versions := protocol.GetProtocolVersionMap(
			mode,
			42,
			false,
			false,
			false,
		)
		versionData := versions[version]
		require.NotNil(t, versionData)
		for _, server := range []bool{false, true} {
			for _, split := range []bool{false, true} {
				name := fmt.Sprintf(
					"ntn=%v/server=%v/split=%v",
					nodeToNode,
					server,
					split,
				)
				t.Run(name, func(t *testing.T) {
					var message protocol.Message = handshake.NewMsgAcceptVersion(
						version, versionData,
					)
					if server {
						message = handshake.NewMsgProposeVersions(
							protocol.ProtocolVersionMap{version: versionData},
						)
					}
					wire, err := cbor.Encode(message)
					require.NoError(t, err)
					require.Greater(t, len(wire), 1)
					local, peer := net.Pipe()
					defer local.Close()
					defer peer.Close()
					require.NoError(
						t,
						peer.SetDeadline(time.Now().Add(5*time.Second)),
					)
					done := make(chan error, 1)
					go func() {
						if !server {
							if err := readHandshakeSegment(peer); err != nil {
								done <- err
								return
							}
						}
						chunks := [][]byte{wire}
						if split {
							chunks = [][]byte{wire[:1], wire[1:]}
						}
						for _, chunk := range chunks {
							if err := writeHandshakeSegment(peer, chunk, !server); err != nil {
								done <- err
								return
							}
						}
						if server {
							done <- readHandshakeSegment(peer)
						} else {
							done <- nil
						}
					}()
					conn, err := ouroboros.NewConnection(
						ouroboros.WithConnection(local),
						ouroboros.WithServer(server),
						ouroboros.WithNodeToNode(nodeToNode),
						ouroboros.WithNetworkMagic(42),
						ouroboros.WithHandshakeTimeout(2*time.Second),
						ouroboros.WithDelayProtocolStart(true),
						ouroboros.WithDelayMuxerStart(true),
					)
					require.NoError(t, err)
					defer conn.Close()
					require.NoError(t, <-done)
					negotiated, _ := conn.ProtocolVersion()
					require.Equal(t, version, negotiated)
				})
			}
		}
	}
}

func writeHandshakeSegment(conn net.Conn, payload []byte, response bool) error {
	segment := muxer.NewSegment(handshake.ProtocolId, payload, response)
	if segment == nil {
		return fmt.Errorf("handshake segment is too large")
	}
	if err := binary.Write(
		conn, binary.BigEndian, segment.SegmentHeader,
	); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

func readHandshakeSegment(conn net.Conn) error {
	var header muxer.SegmentHeader
	if err := binary.Read(conn, binary.BigEndian, &header); err != nil {
		return err
	}
	_, err := io.CopyN(io.Discard, conn, int64(header.PayloadLength))
	return err
}

func TestSegmentedHandshakeLimits(t *testing.T) {
	versions := make(map[uint64]uint64)
	for i := range uint64(2000) {
		versions[i] = 42
	}
	oversized, err := cbor.Encode([]any{uint64(0), versions})
	require.NoError(t, err)
	require.Greater(t, len(oversized), handshake.MaxPendingMessageBytes)
	for _, tc := range []struct {
		name      string
		chunks    [][]byte
		wantError string
	}{
		{"incomplete", [][]byte{{0x82}}, "handshake did not complete within"},
		{
			"oversized",
			[][]byte{oversized[:1], oversized[1:]},
			"received oversized message",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local, peer := net.Pipe()
			defer local.Close()
			defer peer.Close()
			require.NoError(
				t,
				peer.SetWriteDeadline(time.Now().Add(5*time.Second)),
			)
			done := make(chan error, 1)
			go func() {
				for _, chunk := range tc.chunks {
					if err := writeHandshakeSegment(peer, chunk, false); err != nil {
						done <- err
						return
					}
				}
				done <- nil
			}()
			_, err := ouroboros.NewConnection(
				ouroboros.WithConnection(local),
				ouroboros.WithServer(true),
				ouroboros.WithNetworkMagic(42),
				ouroboros.WithHandshakeTimeout(500*time.Millisecond),
			)
			require.ErrorContains(t, err, tc.wantError)
			require.NoError(t, <-done)
		})
	}
}
