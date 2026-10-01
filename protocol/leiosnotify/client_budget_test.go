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

package leiosnotify

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func TestStateMapBoundsPendingBytesInEveryState(t *testing.T) {
	for _, cfg := range []Config{
		NewConfig(),
		NewConfig(WithPipelineLimit(MaxPipelineLimit)),
		NewConfig(WithMaxPendingBytes(2 * DefaultMaxPendingBytes)),
	} {
		protoConfig := cfg.protocolConfig(protocol.ProtocolConfig{})
		require.Len(t, protoConfig.StateMap, len(StateMap))
		for state, entry := range protoConfig.StateMap {
			require.Equal(
				t,
				cfg.pendingBytes(),
				entry.PendingMessageByteLimit,
				"state %s",
				state,
			)
		}
		require.Equal(
			t,
			cfg.pendingBytes()/MaxBlockAnnouncementBytes,
			protoConfig.RecvQueueSize,
		)
		require.GreaterOrEqual(
			t,
			cfg.pendingBytes(),
			max(cfg.PipelineLimit, 1)*MaxBlockAnnouncementBytes,
		)
	}
	for state, entry := range StateMap {
		require.Equal(
			t,
			DefaultMaxPendingBytes,
			entry.PendingMessageByteLimit,
			"state %s",
			state,
		)
	}
}

func TestConfigRejectsMaxPendingBytesBelowLimits(t *testing.T) {
	for _, cfg := range []Config{
		{MaxPendingBytes: -1},
		{MaxPendingBytes: MaxVotesOfferBytes - 1},
		{
			PipelineLimit:   MaxPipelineLimit,
			MaxPendingBytes: MaxVotesOfferBytes,
		},
	} {
		require.Error(t, cfg.validate(), "%+v", cfg)
	}
	require.NoError(t, (&Config{
		PipelineLimit:   4,
		MaxPendingBytes: MaxVotesOfferBytes,
	}).validate())
}

// The payload is not valid CBOR, so a decode attempt would fail with a
// different error.
func TestNewMsgFromCborRejectsOversizedAnnouncementBeforeDecoding(
	t *testing.T,
) {
	data := bytes.Repeat([]byte{0xff}, MaxBlockAnnouncementBytes+1)
	_, err := NewMsgFromCbor(MessageTypeBlockAnnouncement, data)
	require.ErrorContains(t, err, "block announcement size")
}

func blockAnnouncementBytes(t *testing.T, size int) []byte {
	t.Helper()
	header, err := cbor.Encode(make([]byte, size))
	require.NoError(t, err)
	data, err := cbor.Encode(NewMsgBlockAnnouncement(header))
	require.NoError(t, err)
	return data
}

// readBatchBytes is the most segment payload the protocol read loop takes
// from the muxer before scanning it.
const readBatchBytes = 10 * muxer.SegmentMaxPayloadLength

// segmentsOutsideLimits counts the whole segments the client can hold beyond
// its byte budget and the muxer's ingress queue: the muxer's delivery channel
// (10) and the segment its forwarder holds, the read loop's batch (the
// segment it waited for and the 10 it drains), the segment that overflowed
// the ingress queue, and the notification inside NotificationFunc.
const segmentsOutsideLimits = 10 + 1 + 1 + 10 + 1 + 1

// countRequests reads the client's segments and reports each
// NotificationRequestNext it carries.
func countRequests(conn net.Conn, requests chan<- struct{}) {
	header := make([]byte, 8)
	for {
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}
		payload := make([]byte, binary.BigEndian.Uint16(header[6:]))
		if _, err := io.ReadFull(conn, payload); err != nil {
			return
		}
		for len(payload) >= 2 {
			if payload[0] == 0x81 &&
				payload[1] == MessageTypeNotificationRequestNext {
				requests <- struct{}{}
			}
			payload = payload[2:]
		}
	}
}

// TestSlowConsumerBoundsRetainedNotificationBytes blocks NotificationFunc
// and has the server keep sending announcements while the client is in
// Busy. The client must stop taking them once its byte budget is held, so
// the bytes the server can write stay within the budget plus the muxer's
// ingress queue, however long the consumer stays blocked.
func TestSlowConsumerBoundsRetainedNotificationBytes(t *testing.T) {
	clientConn, peerConn := net.Pipe()
	m := muxer.New(clientConn)
	release := make(chan struct{})
	t.Cleanup(func() {
		close(release)
		m.Stop()
		_ = clientConn.Close()
		_ = peerConn.Close()
	})
	var notified atomic.Int32
	blocked := make(chan struct{})
	cfg := NewConfig(
		WithNotificationFunc(func(CallbackContext, protocol.Message) error {
			// Return from the first so the client requests more, then block.
			if notified.Add(1) == 2 {
				close(blocked)
				<-release
			}
			return nil
		}),
	)
	clientErrors := make(chan error, 10)
	client := NewClient(
		protocol.ProtocolOptions{
			Muxer:     m,
			ErrorChan: clientErrors,
			Mode:      protocol.ProtocolModeNodeToNode,
		},
		&cfg,
	)
	client.Start()
	m.Start()
	requests := make(chan struct{}, 2*MaxPipelineLimit)
	go countRequests(peerConn, requests)

	announcement := blockAnnouncementBytes(t, 60000)
	segment := muxer.NewSegment(ProtocolId, announcement, true)
	require.NotNil(t, segment)
	var wire bytes.Buffer
	require.NoError(
		t,
		binary.Write(&wire, binary.BigEndian, segment.SegmentHeader),
	)
	wire.Write(segment.Payload)
	writeAnnouncement := func() error {
		if err := peerConn.SetWriteDeadline(
			time.Now().Add(time.Second),
		); err != nil {
			return err
		}
		_, err := peerConn.Write(wire.Bytes())
		return err
	}

	// Read before the flood: an overflow stops the muxer and unregisters it.
	ingressLimit := m.IngressLimit(ProtocolId, muxer.ProtocolRoleInitiator)
	require.Positive(t, ingressLimit)
	require.LessOrEqual(
		t,
		ingressLimit,
		max(cfg.pendingBytes(), readBatchBytes),
	)

	bound := cfg.pendingBytes() + ingressLimit +
		segmentsOutsideLimits*wire.Len()
	awaitRequest := func() {
		t.Helper()
		select {
		case <-requests:
		case err := <-clientErrors:
			t.Fatalf("client error: %s", err)
		case <-time.After(2 * time.Second):
			t.Fatal("client did not request a notification")
		}
	}

	require.NoError(t, client.Sync())
	// The first notification returns and the second blocks NotificationFunc.
	// The third then blocks the receive loop handing it to the notification
	// loop, after its transition back to Idle sends the fourth request, so
	// the client stays in Busy and every later message waits in it.
	for i := range 3 {
		awaitRequest()
		require.NoError(t, writeAnnouncement())
		if i == 1 {
			select {
			case <-blocked:
			case <-time.After(2 * time.Second):
				t.Fatal(
					"NotificationFunc did not receive the second notification",
				)
			}
		}
	}
	awaitRequest()

	written := 0
	for written <= bound {
		err := writeAnnouncement()
		if errors.Is(err, os.ErrDeadlineExceeded) ||
			errors.Is(err, io.ErrClosedPipe) {
			break
		}
		require.NoError(t, err)
		written += wire.Len()
	}
	// Whatever stopped the reads, it must not be the client rejecting a
	// message: that tears the protocol down and the muxer drops the rest.
	select {
	case err := <-clientErrors:
		require.NotContains(t, err.Error(), "without peer agency")
	default:
	}
	require.LessOrEqual(t, written, bound)
}
