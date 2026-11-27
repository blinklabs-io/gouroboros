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
					defer conn.Close(t.Context())
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

func TestHandshakeQueryReplyBeforePeerClose(t *testing.T) {
	versions := protocol.GetProtocolVersionMap(
		protocol.ProtocolModeNodeToClient, 42, false, false, false,
	)
	reply, err := cbor.Encode(handshake.NewMsgQueryReply(versions))
	require.NoError(t, err)
	for _, split := range []bool{false, true} {
		t.Run(fmt.Sprintf("split=%v", split), func(t *testing.T) {
			for range 30 {
				local, peer := net.Pipe()
				sent := make(chan error, 1)
				go func() {
					defer peer.Close()
					if err := readHandshakeSegment(peer); err != nil {
						sent <- err
						return
					}
					chunks := [][]byte{reply}
					if split {
						chunks = [][]byte{reply[:1], reply[1:]}
					}
					for _, chunk := range chunks {
						if err := writeHandshakeSegment(peer, chunk, true); err != nil {
							sent <- err
							return
						}
					}
					sent <- nil
				}()
				conn, err := ouroboros.NewConnection(
					ouroboros.WithConnection(local),
					ouroboros.WithNetworkMagic(42),
					ouroboros.WithHandshakeTimeout(time.Second),
					ouroboros.WithQueryMode(true),
				)
				if conn != nil {
					defer conn.Close(t.Context())
				}
				require.NoError(t, <-sent)
				require.NoError(t, err)
				require.Len(t, conn.QueryReplyVersionMap(), len(versions))
				require.NoError(t, conn.Close(t.Context()))
			}
		})
	}
}

func TestIncompleteHandshakeQueryReplyBeforePeerClose(t *testing.T) {
	versions := protocol.GetProtocolVersionMap(
		protocol.ProtocolModeNodeToClient, 42, false, false, false,
	)
	reply, err := cbor.Encode(handshake.NewMsgQueryReply(versions))
	require.NoError(t, err)
	require.Greater(t, len(reply), 1)
	local, peer := net.Pipe()
	defer local.Close()
	sent := make(chan error, 1)
	go func() {
		defer peer.Close()
		if err := readHandshakeSegment(peer); err != nil {
			sent <- err
			return
		}
		sent <- writeHandshakeSegment(peer, reply[:len(reply)-1], true)
	}()
	conn, err := ouroboros.NewConnection(
		ouroboros.WithConnection(local),
		ouroboros.WithNetworkMagic(42),
		ouroboros.WithHandshakeTimeout(2*time.Second),
		ouroboros.WithQueryMode(true),
	)
	if conn != nil {
		defer conn.Close(t.Context())
	}
	require.NoError(t, <-sent)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "handshake did not complete within")
}

// A peer can follow the final handshake message with the start of another
// one in the same segment. The leftover is incomplete for the rest of the
// connection, and it must not keep the muxer reading before Start.
func TestHandshakeTrailingBytesDoNotOpenMuxer(t *testing.T) {
	t.Parallel()
	const (
		extraSegments = 8
		// A segment the muxer does not read leaves the peer's write
		// blocked, so a missed segment is never delivered late.
		blockedWrite = 150 * time.Millisecond
		// The peer can write a segment the muxer took before the final
		// message was recorded as handled; two or more mean the muxer is
		// reading on its own.
		allowedExtra = 1
	)
	for _, server := range []bool{false, true} {
		t.Run(fmt.Sprintf("server=%v", server), func(t *testing.T) {
			t.Parallel()
			versions := protocol.GetProtocolVersionMap(
				protocol.ProtocolModeNodeToNode, 42, false, false, false,
			)
			var message protocol.Message = handshake.NewMsgAcceptVersion(
				14, versions[14],
			)
			if server {
				message = handshake.NewMsgProposeVersions(versions)
			}
			wire, err := cbor.Encode(message)
			require.NoError(t, err)
			// An indefinite-length array start never completes.
			wire = append(wire, 0x9f)
			local, peer := net.Pipe()
			defer local.Close()
			defer peer.Close()
			require.NoError(
				t, peer.SetDeadline(time.Now().Add(10*time.Second)),
			)
			admitted := make(chan int, 1)
			go func() {
				if !server {
					if err := readHandshakeSegment(peer); err != nil {
						admitted <- -1
						return
					}
				}
				if err := writeHandshakeSegment(peer, wire, !server); err != nil {
					admitted <- -1
					return
				}
				if server {
					if err := readHandshakeSegment(peer); err != nil {
						admitted <- -1
						return
					}
				}
				count := 0
				for range extraSegments {
					_ = peer.SetWriteDeadline(time.Now().Add(blockedWrite))
					if err := writeHandshakeSegment(
						peer, make([]byte, 1000), !server,
					); err != nil {
						break
					}
					count++
				}
				admitted <- count
			}()
			conn, err := ouroboros.NewConnection(
				ouroboros.WithConnection(local),
				ouroboros.WithServer(server),
				ouroboros.WithNodeToNode(true),
				ouroboros.WithNetworkMagic(42),
				ouroboros.WithHandshakeTimeout(5*time.Second),
				ouroboros.WithDelayProtocolStart(true),
				ouroboros.WithDelayMuxerStart(true),
			)
			require.NoError(t, err)
			defer conn.Close(t.Context())
			require.LessOrEqual(
				t,
				<-admitted,
				allowedExtra,
				"muxer read segments after the handshake finished, before Start",
			)
		})
	}
}
