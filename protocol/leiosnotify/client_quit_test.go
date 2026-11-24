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
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// goroutineWaitingIn reports whether some goroutine with fn on its stack is
// parked, rather than running or runnable.
func goroutineWaitingIn(fn string) bool {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	for g := range strings.SplitSeq(string(buf[:n]), "\n\n") {
		header, _, _ := strings.Cut(g, "\n")
		if strings.Contains(g, fn) &&
			!strings.Contains(header, "[running]") &&
			!strings.Contains(header, "[runnable]") {
			return true
		}
	}
	return false
}

func newQuitTestClient(
	t *testing.T,
	cfg Config,
) (*Client, net.Conn, chan error) {
	t.Helper()
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		_ = connA.Close()
		_ = connB.Close()
	})
	m := muxer.New(connA)
	t.Cleanup(m.Stop)
	errs := make(chan error, 8)
	client := NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
			RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
		},
		Muxer:     m,
		ErrorChan: errs,
	}, &cfg)
	client.Start()
	t.Cleanup(client.Protocol.Stop)
	m.Start()
	return client, connB, errs
}

// clientMessageReader splits the client's byte stream into messages. The
// muxer may put several messages in one segment, and every message a client
// sends is a one-element list, [tag], two bytes on the wire.
type clientMessageReader struct {
	conn    net.Conn
	pending []byte
}

func (r *clientMessageReader) next(t *testing.T) uint {
	t.Helper()
	for len(r.pending) < 2 {
		require.NoError(t, r.conn.SetReadDeadline(time.Now().Add(5*time.Second)))
		segment, err := readLeiosNotifyTestSegment(t, r.conn)
		require.NoError(t, err)
		r.pending = append(r.pending, segment.Payload...)
	}
	require.Equal(t, byte(0x81), r.pending[0], "client message is not [tag]")
	tag := uint(r.pending[1])
	r.pending = r.pending[2:]
	return tag
}

func writeQuitTestReply(t *testing.T, conn net.Conn, msg protocol.Message) {
	t.Helper()
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	writeLeiosNotifyTestSegment(
		t,
		conn,
		muxer.NewSegment(ProtocolId, data, true),
	)
}

// TestStopDuringRefillSendsNoRequestAfterQuit calls Stop while the
// notification loop is part way through sending a refill of PipelineLimit
// requests. MsgQuit ends the client's part of the protocol: the reference
// client sends nothing after it, and a request behind it reaches a server
// that has already left Idle for Quit. So every request of the refill must
// be on the wire before MsgQuit.
func TestStopDuringRefillSendsNoRequestAfterQuit(t *testing.T) {
	cfg := NewConfig(
		WithPipelineLimit(MaxPipelineLimit),
		WithNotificationFunc(func(CallbackContext, protocol.Message) error {
			return nil
		}),
	)
	client, conn, _ := newQuitTestClient(t, cfg)
	reader := &clientMessageReader{conn: conn}
	require.NoError(t, client.Sync())
	require.Equal(t, uint(MessageTypeNotificationRequestNext), reader.next(t))
	writeQuitTestReply(t, conn, NewMsgBlockAnnouncement(
		testAnnouncementEra,
		cbor.RawMessage{0x82, 0x01, 0x02},
	))
	// The first request of the refill is on the wire, so the loop has decided
	// to refill. It now blocks on the send queue, because this end of the
	// pipe stops reading until Stop has either returned or parked.
	require.Equal(t, uint(MessageTypeNotificationRequestNext), reader.next(t))
	stopErr := make(chan error, 1)
	go func() { stopErr <- client.Stop() }()
	require.Eventually(t, func() bool {
		select {
		case err := <-stopErr:
			stopErr <- err
			return true
		default:
			return goroutineWaitingIn("leiosnotify.(*Client).Stop")
		}
	}, 5*time.Second, time.Millisecond, "Stop neither returned nor waited")

	// The refill sends MaxPipelineLimit requests in all; one is read.
	quitAt := -1
	for i := range MaxPipelineLimit {
		tag := reader.next(t)
		if tag == uint(MessageTypeQuit) {
			quitAt = i
			continue
		}
		require.Equal(t, uint(MessageTypeNotificationRequestNext), tag)
		require.Equal(t, -1, quitAt,
			"request %d of the refill was sent after MsgQuit", i+1)
	}
	require.Equal(t, MaxPipelineLimit-1, quitAt, "MsgQuit must follow the refill")
	require.NoError(t, <-stopErr)
}

// TestSyncAfterStopIsRefused checks that Sync sends nothing once Stop has
// sent MsgQuit: no request is legal after it.
func TestSyncAfterStopIsRefused(t *testing.T) {
	cfg := NewConfig(
		WithNotificationFunc(func(CallbackContext, protocol.Message) error {
			return nil
		}),
	)
	client, conn, _ := newQuitTestClient(t, cfg)
	stopErr := make(chan error, 1)
	go func() { stopErr <- client.Stop() }()
	require.IsType(t, &MsgQuit{}, readQuitTestMessage(t, conn))
	require.NoError(t, <-stopErr)
	require.ErrorIs(t, client.Sync(), protocol.ErrProtocolShuttingDown)
}

// TestSyncAfterStopWhileSyncingIsRefused stops a client whose notification
// loop is running. The loop stays parked until the protocol ends, so Sync
// must report the shutdown rather than a loop that is still running.
func TestSyncAfterStopWhileSyncingIsRefused(t *testing.T) {
	cfg := NewConfig(
		WithNotificationFunc(func(CallbackContext, protocol.Message) error {
			return nil
		}),
	)
	client, conn, _ := newQuitTestClient(t, cfg)
	reader := &clientMessageReader{conn: conn}
	require.NoError(t, client.Sync())
	require.Equal(t, uint(MessageTypeNotificationRequestNext), reader.next(t))
	stopErr := make(chan error, 1)
	go func() { stopErr <- client.Stop() }()
	require.Equal(t, uint(MessageTypeQuit), reader.next(t))
	require.NoError(t, <-stopErr)
	require.ErrorIs(t, client.Sync(), protocol.ErrProtocolShuttingDown)
}
