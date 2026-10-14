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

package muxer_test

import (
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

// deadlineConn wraps a connection, records every read deadline the muxer
// installs, and fails SetReadDeadline with err when it is set.
type deadlineConn struct {
	net.Conn
	err       error
	reads     atomic.Int32
	mu        sync.Mutex
	deadlines []time.Time
}

func (c *deadlineConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	c.deadlines = append(c.deadlines, t)
	c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	return c.Conn.SetReadDeadline(t)
}

func (c *deadlineConn) Read(b []byte) (int, error) {
	c.reads.Add(1)
	return c.Conn.Read(b)
}

func (c *deadlineConn) recordedDeadlines() []time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Time(nil), c.deadlines...)
}

// newDeadlineConn returns one end of a pipe wrapped to fail read deadlines
// with err, and the peer end.
func newDeadlineConn(t *testing.T, err error) (*deadlineConn, net.Conn) {
	t.Helper()
	local, peer := net.Pipe()
	t.Cleanup(func() {
		local.Close()
		peer.Close()
	})
	return &deadlineConn{Conn: local, err: err}, peer
}

// requireMuxerError waits for the muxer to report an error and finish
// stopping, which closes ErrorChan once its read loop has cleaned up.
func requireMuxerError(t *testing.T, m *muxer.Muxer) error {
	t.Helper()
	select {
	case err := <-m.ErrorChan():
		waitMuxerFinished(t, m)
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("muxer reported no error")
	}
	return nil
}

// TestReadLoopStopsWhenDeadlineCannotBeSet checks that a connection which
// refuses the read deadline stops the muxer with that error before any
// read, whether the deadline is being installed or, with the timeout
// disabled, cleared.
func TestReadLoopStopsWhenDeadlineCannotBeSet(t *testing.T) {
	refused := errors.New("deadline refused")
	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{name: "install", timeout: time.Minute},
		{name: "clear", timeout: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conn, _ := newDeadlineConn(t, refused)
			m := muxer.NewWithSegmentReadTimeout(conn, tc.timeout)
			t.Cleanup(m.Stop)
			m.Start()

			err := requireMuxerError(t, m)
			require.ErrorIs(t, err, refused)
			require.ErrorContains(t, err, "set read deadline")
			require.Zero(t, conn.reads.Load(), "read without a deadline")
		})
	}
}

// TestReadLoopDeadlineOnClosedConnection checks that a deadline refused
// because the connection is closed is reported as a closed connection, as
// a read on it would be.
func TestReadLoopDeadlineOnClosedConnection(t *testing.T) {
	conn, _ := newDeadlineConn(t, io.ErrClosedPipe)
	m := muxer.New(conn)
	t.Cleanup(m.Stop)
	m.Start()

	var closedErr *muxer.ConnectionClosedError
	require.ErrorAs(t, requireMuxerError(t, m), &closedErr)
	require.NotNil(t, closedErr)
	require.Equal(t, "setting read deadline", closedErr.Context)
	require.Zero(t, conn.reads.Load())
}

// TestReadLoopDeadlineLocalCloseIsNotPeerClose checks that net.ErrClosed, which
// a connection closed on this side returns from both Read and SetReadDeadline,
// is reported unchanged rather than as the peer closing the connection.
func TestReadLoopDeadlineLocalCloseIsNotPeerClose(t *testing.T) {
	conn, _ := newDeadlineConn(t, net.ErrClosed)
	m := muxer.New(conn)
	t.Cleanup(m.Stop)
	m.Start()

	err := requireMuxerError(t, m)
	require.ErrorIs(t, err, net.ErrClosed)
	var closedErr *muxer.ConnectionClosedError
	require.False(t, errors.As(err, &closedErr), "reported as a peer close")
}

// TestReadLoopDeadlineDisabledClearsDeadline checks that a disabled timeout
// still clears the deadline before each read and delivers segments.
func TestReadLoopDeadlineDisabledClearsDeadline(t *testing.T) {
	conn, peer := newDeadlineConn(t, nil)
	m := muxer.NewWithSegmentReadTimeout(conn, 0)
	t.Cleanup(m.Stop)
	_, recvChan, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	m.Start()

	seg := muxer.NewSegment(0x01, []byte("payload"), false)
	require.NotNil(t, seg)
	_, err := peer.Write(createSegmentData(seg))
	require.NoError(t, err)
	select {
	case got := <-recvChan:
		require.Equal(t, []byte("payload"), got.Payload)
	case err := <-m.ErrorChan():
		t.Fatalf("muxer failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("segment was not delivered")
	}
	// The first deadline is set before the segment is read; the read loop
	// sets a second as it starts the next read.
	require.Eventually(t, func() bool {
		return len(conn.recordedDeadlines()) >= 2
	}, 5*time.Second, time.Millisecond, "read loop did not start another read")
	for _, deadline := range conn.recordedDeadlines() {
		require.True(t, deadline.IsZero(), "deadline %v", deadline)
	}
}
