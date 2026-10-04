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

package muxer

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	testPraosId = uint16(2)
	testLeiosId = protocolIdLeiosFetch
)

// waiting reports how many segments are queued for a turn.
func (e *egress) waiting() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.praos) + len(e.leios)
}

func newTestSegment(protocolId uint16, age time.Duration) *Segment {
	s := NewSegment(protocolId, []byte{1}, false)
	s.created = time.Now().Add(-age)
	return s
}

// queueWaiter starts an acquire for s and returns a channel reporting its
// result once granted. It returns after the segment is queued.
func queueWaiter(t *testing.T, e *egress, s *Segment) <-chan error {
	t.Helper()
	before := e.waiting()
	res := make(chan error, 1)
	done := make(chan bool)
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		_, err := e.acquire(s, done)
		res <- err
	}()
	// A waiter still queued when the test ends is released here, so it
	// does not outlive the test.
	t.Cleanup(func() {
		close(done)
		<-exited
	})
	require.Eventually(t, func() bool {
		return e.waiting() == before+1
	}, 5*time.Second, time.Millisecond)
	return res
}

func requireGranted(t *testing.T, res <-chan error) {
	t.Helper()
	select {
	case err := <-res:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a turn")
	}
}

func requireNotGranted(t *testing.T, res <-chan error) {
	t.Helper()
	select {
	case err := <-res:
		t.Fatalf("granted out of turn, err=%v", err)
	default:
	}
}

func TestEgressPraosGoesBeforeLeios(t *testing.T) {
	t.Parallel()
	var e egress
	_, err := e.acquire(newTestSegment(testPraosId, 0), nil)
	require.NoError(t, err)
	leios := queueWaiter(t, &e, newTestSegment(testLeiosId, 0))
	praos := queueWaiter(t, &e, newTestSegment(testPraosId, 0))

	e.release()
	requireGranted(t, praos)
	requireNotGranted(t, leios)
	e.release()
	requireGranted(t, leios)
}

func TestEgressFreshestLeiosGoesFirst(t *testing.T) {
	t.Parallel()
	var e egress
	_, err := e.acquire(newTestSegment(testPraosId, 0), nil)
	require.NoError(t, err)
	old := queueWaiter(t, &e, newTestSegment(testLeiosId, 5*time.Second))
	fresh := queueWaiter(t, &e, newTestSegment(testLeiosId, time.Second))

	e.release()
	requireGranted(t, fresh)
	requireNotGranted(t, old)
	e.release()
	requireGranted(t, old)
}

func TestEgressLeiosIsNotStarved(t *testing.T) {
	t.Parallel()
	var e egress
	_, err := e.acquire(newTestSegment(testPraosId, 0), nil)
	require.NoError(t, err)
	leios := queueWaiter(t, &e, newTestSegment(testLeiosId, 0))

	// Praos always has another waiter; Leios must still get a turn within
	// praosBurst Praos turns.
	for range praosBurst {
		praos := queueWaiter(t, &e, newTestSegment(testPraosId, 0))
		e.release()
		requireGranted(t, praos)
		requireNotGranted(t, leios)
	}
	queueWaiter(t, &e, newTestSegment(testPraosId, 0))
	e.release()
	requireGranted(t, leios)
}

func TestEgressDropsStaleLeios(t *testing.T) {
	t.Parallel()
	var e egress

	// Stale on arrival, even with the connection idle.
	_, err := e.acquire(newTestSegment(testLeiosId, 2*leiosMaxAge), nil)
	require.ErrorIs(t, err, ErrEgressStale)
	require.Zero(t, e.waiting())

	// Stale by the time its turn comes: it must not take the turn from the
	// waiter behind it.
	_, err = e.acquire(newTestSegment(testPraosId, 0), nil)
	require.NoError(t, err)
	seg := newTestSegment(testLeiosId, 0)
	staleWaiter := queueWaiter(t, &e, seg)
	fresh := queueWaiter(t, &e, newTestSegment(testLeiosId, 0))
	seg.created = time.Now().Add(-2 * leiosMaxAge)
	e.release()
	// The fresh segment is chosen first; release again to reach the stale.
	requireGranted(t, fresh)
	e.release()
	require.ErrorIs(t, <-staleWaiter, ErrEgressStale)
	// The turn was not handed to the dropped segment.
	_, err = e.acquire(newTestSegment(testPraosId, 0), make(chan bool))
	require.NoError(t, err)
}

func TestEgressStopReleasesWaiter(t *testing.T) {
	t.Parallel()
	var e egress
	_, err := e.acquire(newTestSegment(testPraosId, 0), nil)
	require.NoError(t, err)
	done := make(chan bool)
	res := make(chan error, 1)
	go func() {
		_, err := e.acquire(newTestSegment(testPraosId, 0), done)
		res <- err
	}()
	require.Eventually(t, func() bool { return e.waiting() == 1 },
		5*time.Second, time.Millisecond)
	close(done)
	require.Error(t, <-res)
	require.Zero(t, e.waiting())
}

// gateConn lets a test pace the connection: each Write waits for a token and
// is then recorded by protocol ID.
type gateConn struct {
	net.Conn
	tokens  chan struct{}
	closed  chan struct{}
	once    sync.Once
	mu      sync.Mutex
	written []uint16
}

func newGateConn(tokens int) *gateConn {
	return &gateConn{
		tokens: make(chan struct{}, tokens),
		closed: make(chan struct{}),
	}
}

func (c *gateConn) Write(b []byte) (int, error) {
	select {
	case <-c.tokens:
	case <-c.closed:
		return 0, net.ErrClosed
	}
	var h SegmentHeader
	_ = binary.Read(bytes.NewReader(b), binary.BigEndian, &h)
	c.mu.Lock()
	c.written = append(c.written, h.GetProtocolId())
	c.mu.Unlock()
	return len(b), nil
}

func (c *gateConn) SetWriteDeadline(time.Time) error { return nil }
func (c *gateConn) SetReadDeadline(time.Time) error  { return nil }
func (c *gateConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *gateConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func (c *gateConn) order() []uint16 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]uint16(nil), c.written...)
}

// newTestMuxer returns a muxer that is stopped, and has finished stopping,
// when the test ends.
func newTestMuxer(t *testing.T, conn net.Conn) *Muxer {
	t.Helper()
	m := New(conn)
	t.Cleanup(func() {
		m.Stop()
		for range m.ErrorChan() {
		}
	})
	return m
}

type egressRecorder struct {
	Metrics
	mu     sync.Mutex
	waits  map[EgressClass]int
	stales int
}

func (r *egressRecorder) EgressWait(_ uint16, c EgressClass, _ time.Duration) {
	r.mu.Lock()
	r.waits[c]++
	r.mu.Unlock()
}

func (r *egressRecorder) EgressStaleDropped(uint16) {
	r.mu.Lock()
	r.stales++
	r.mu.Unlock()
}

// A Praos segment queued behind saturated Leios senders is written after at
// most the segment already on the wire, and the Leios senders still finish.
func TestMuxerPraosLatencyBoundedUnderLeiosSaturation(t *testing.T) {
	t.Parallel()
	conn := newGateConn(1024)
	m := newTestMuxer(t, conn)
	rec := &egressRecorder{waits: map[EgressClass]int{}}
	m.SetMetrics(rec)
	const perLeios = 40
	leiosIds := []uint16{
		protocolIdLeiosNotify, protocolIdLeiosFetch, protocolIdLeiosVotes,
	}
	var send []chan *Segment
	for _, id := range leiosIds {
		ch, _, _ := m.RegisterProtocol(id, ProtocolRoleInitiator)
		send = append(send, ch)
	}
	praosSend, _, _ := m.RegisterProtocol(testPraosId, ProtocolRoleInitiator)
	for i, ch := range send {
		go func() {
			for range perLeios {
				ch <- NewSegment(leiosIds[i], []byte{1}, false)
			}
		}()
	}
	// Wait until every Leios sender has a segment on the wire or queued.
	require.Eventually(t, func() bool {
		return m.egress.waiting() == len(leiosIds)-1
	}, 5*time.Second, time.Millisecond)

	praosSend <- NewSegment(testPraosId, []byte{1}, false)
	require.Eventually(t, func() bool {
		return m.egress.waiting() == len(leiosIds)
	}, 5*time.Second, time.Millisecond)

	// Pace the connection one segment at a time. The first token completes
	// the write already in progress; Praos must be the very next write.
	conn.tokens <- struct{}{}
	conn.tokens <- struct{}{}
	require.Eventually(t, func() bool {
		return len(conn.order()) == 2
	}, 5*time.Second, time.Millisecond)
	require.Equal(t, testPraosId, conn.order()[1],
		"Praos segment was written behind Leios backlog: %v", conn.order())

	// No starvation: the rest of the Leios traffic drains.
	for range 1024 - 2 {
		conn.tokens <- struct{}{}
	}
	require.Eventually(t, func() bool {
		return len(conn.order()) == perLeios*len(leiosIds)+1
	}, 5*time.Second, time.Millisecond)
	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.Positive(t, rec.waits[EgressClassLeios])
	require.Positive(t, rec.waits[EgressClassPraos])
}

func TestMuxerReportsStaleLeiosDrop(t *testing.T) {
	t.Parallel()
	// The token lets a wrongly written segment finish instead of blocking.
	conn := newGateConn(1)
	conn.tokens <- struct{}{}
	m := newTestMuxer(t, conn)
	rec := &egressRecorder{waits: map[EgressClass]int{}}
	m.SetMetrics(rec)

	err := m.Send(newTestSegment(testLeiosId, 2*leiosMaxAge))
	require.ErrorIs(t, err, ErrEgressStale)
	require.Empty(t, conn.order())
	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.Equal(t, 1, rec.stales)
}
