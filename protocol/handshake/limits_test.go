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
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

const limitsTestVersion uint16 = 13

func limitsTestVersionData() protocol.VersionDataNtN13andUp {
	return protocol.VersionDataNtN13andUp{
		VersionDataNtN11to12: protocol.VersionDataNtN11to12{
			CborNetworkMagic: 42,
		},
	}
}

// proposeWithVersions encodes a ProposeVersions message carrying count
// version entries, including one supported version. Version numbers are
// distinct, so the map decodes to count entries.
func proposeWithVersions(t *testing.T, count int) []byte {
	t.Helper()
	versions := make(map[uint16]cbor.RawMessage, count)
	for i := range count {
		versions[uint16(i)] = cbor.RawMessage{0x80} // #nosec G115
	}
	versionData, err := cbor.Encode(limitsTestVersionData())
	require.NoError(t, err)
	versions[limitsTestVersion] = versionData
	data, err := cbor.Encode(&MsgProposeVersions{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeProposeVersions,
		},
		VersionMap: versions,
	})
	require.NoError(t, err)
	return data
}

func newLimitsServer(t *testing.T) (net.Conn, chan error, chan struct{}) {
	t.Helper()
	connA, connB := net.Pipe()
	m := muxer.New(connA)
	errs := make(chan error, 1)
	finished := make(chan struct{}, 1)
	cfg := NewConfig(
		WithProtocolVersionMap(protocol.ProtocolVersionMap{
			limitsTestVersion: limitsTestVersionData(),
		}),
		WithFinishedFunc(
			func(CallbackContext, uint16, protocol.VersionData) error {
				finished <- struct{}{}
				return nil
			},
		),
	)
	server := NewServer(
		protocol.ProtocolOptions{
			ConnectionId: connection.ConnectionId{
				LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
				RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
			},
			Muxer:     m,
			ErrorChan: errs,
			Mode:      protocol.ProtocolModeNodeToNode,
		},
		&cfg,
	)
	server.Start()
	m.Start()
	t.Cleanup(func() {
		server.Protocol.Stop()
		m.Stop()
		_ = connA.Close()
		_ = connB.Close()
	})
	return connB, errs, finished
}

func writeSegment(t *testing.T, conn net.Conn, payload []byte) {
	t.Helper()
	segment := muxer.NewSegment(ProtocolId, payload, false)
	if segment == nil {
		t.Fatal("failed to construct muxer segment")
	}
	buf := &bytes.Buffer{}
	require.NoError(
		t,
		binary.Write(buf, binary.BigEndian, &segment.SegmentHeader),
	)
	buf.Write(segment.Payload)
	require.NoError(t, conn.SetWriteDeadline(time.Now().Add(5*time.Second)))
	_, err := conn.Write(buf.Bytes())
	require.NoError(t, err)
}

func readSegment(t *testing.T, conn net.Conn) {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	var header muxer.SegmentHeader
	require.NoError(t, binary.Read(conn, binary.BigEndian, &header))
	payload := make([]byte, int(header.PayloadLength))
	_, err := io.ReadFull(conn, payload)
	require.NoError(t, err)
}

// TestServerRefusesOversizedPropose proves a ProposeVersions message larger
// than MaxPendingMessageBytes is refused before it is decoded: a peer that has
// not yet completed the handshake cannot make the server buffer and decode a
// message the size of the read-buffer cap.
func TestServerRefusesOversizedPropose(t *testing.T) {
	t.Parallel()
	conn, errs, _ := newLimitsServer(t)

	data := proposeWithVersions(t, 4000)
	require.Greater(t, len(data), MaxPendingMessageBytes)
	// Segments are limited to 65535 bytes; the message fits in one.
	require.LessOrEqual(t, len(data), 0xffff)
	writeSegment(t, conn, data)

	select {
	case err := <-errs:
		require.ErrorContains(t, err, "received oversized message")
		require.ErrorContains(t, err, strconv.Itoa(MaxPendingMessageBytes))
	case <-time.After(5 * time.Second):
		t.Fatal("oversized ProposeVersions was not refused")
	}
}

// TestServerAcceptsProposeWithinLimit is the control for the refusal above:
// the same message shape within MaxPendingMessageBytes reaches the handler.
func TestServerAcceptsProposeWithinLimit(t *testing.T) {
	t.Parallel()
	conn, _, finished := newLimitsServer(t)

	data := proposeWithVersions(t, 1000)
	require.LessOrEqual(t, len(data), MaxPendingMessageBytes)
	writeSegment(t, conn, data)
	readSegment(t, conn)

	select {
	case <-finished:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("in-limit ProposeVersions did not reach FinishedFunc")
	}
}

// TestStateMapsBoundPendingMessageBytes pins the limit onto both active
// states of both state maps, matching the reference implementation's
// byteLimitsHandshake.
func TestStateMapsBoundPendingMessageBytes(t *testing.T) {
	t.Parallel()
	for name, stateMap := range map[string]protocol.StateMap{
		"NtN": StateMapNtN,
		"NtC": StateMapNtC,
	} {
		for _, state := range []protocol.State{statePropose, stateConfirm} {
			entry, ok := stateMap[state]
			require.True(t, ok, "%s missing state %s", name, state)
			require.Equal(
				t,
				MaxPendingMessageBytes,
				entry.PendingMessageByteLimit,
				"%s state %s must bound pending message bytes",
				name,
				state,
			)
		}
	}
}
