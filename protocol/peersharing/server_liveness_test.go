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
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func writeLivenessSegment(conn net.Conn, segment *muxer.Segment) error {
	buf := bytes.NewBuffer(nil)
	if err := binary.Write(buf, binary.BigEndian, segment.SegmentHeader); err != nil {
		return err
	}
	if _, err := buf.Write(segment.Payload); err != nil {
		return err
	}
	data := buf.Bytes()
	for len(data) > 0 {
		n, err := conn.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

func TestRepeatedDoneDoesNotStallSharedMuxer(t *testing.T) {
	const (
		segmentCount    = 20
		messagesPerSeg  = 2000
		probeProtocolId = 42
	)

	localConn, remoteConn := net.Pipe()
	m := muxer.New(localConn)
	protocolErrors := make(chan error, 1)
	cfg := NewConfig(WithLocalDisabled(true))
	server := NewServer(
		protocol.ProtocolOptions{
			ConnectionId: connection.ConnectionId{
				LocalAddr:  localConn.LocalAddr(),
				RemoteAddr: localConn.RemoteAddr(),
			},
			ErrorChan: protocolErrors,
			Muxer:     m,
		},
		&cfg,
	)
	_, probeRecv, _ := m.RegisterProtocol(
		probeProtocolId,
		muxer.ProtocolRoleResponder,
	)
	server.Start()
	m.Start()
	t.Cleanup(func() {
		m.Stop()
		_ = remoteConn.Close()
		_ = localConn.Close()
		server.ProtocolInstance().Stop()
	})

	doneCbor, err := cbor.Encode(NewMsgDone())
	require.NoError(t, err)
	donePayload := bytes.Repeat(doneCbor, messagesPerSeg)
	writeDone := make(chan error, 1)
	go func() {
		for range segmentCount {
			segment := muxer.NewSegment(ProtocolId, donePayload, false)
			if segment == nil {
				writeDone <- fmt.Errorf("failed to create peer-sharing segment")
				return
			}
			if err := writeLivenessSegment(remoteConn, segment); err != nil {
				writeDone <- err
				return
			}
		}
		probe := muxer.NewSegment(probeProtocolId, []byte{0x81, 0x00}, false)
		if probe == nil {
			writeDone <- fmt.Errorf("failed to create probe segment")
			return
		}
		writeDone <- writeLivenessSegment(remoteConn, probe)
	}()

	probeDelivered := false
	connectionClosed := false
	select {
	case segment, ok := <-probeRecv:
		require.True(t, ok, "probe receiver closed before delivery")
		require.Equal(t, uint16(probeProtocolId), segment.GetProtocolId())
		probeDelivered = true
	case err := <-protocolErrors:
		require.NoError(t, err)
	case err := <-m.ErrorChan():
		require.EqualError(
			t,
			err,
			"received message for unknown protocol ID 10",
		)
		connectionClosed = true
	case <-time.After(2 * time.Second):
		t.Fatal("shared muxer stalled after repeated peer-sharing Done messages")
	}
	writerErr := <-writeDone
	if probeDelivered {
		require.NoError(t, writerErr)
		select {
		case err := <-protocolErrors:
			require.NoError(t, err)
		default:
		}
	} else if connectionClosed {
		require.Error(t, writerErr)
	}
}
