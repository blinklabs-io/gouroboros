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

func newTestSegment(protocolId uint16) *Segment {
	return &Segment{
		SegmentHeader: SegmentHeader{
			ProtocolId:    protocolId,
			PayloadLength: 1,
		},
		Payload: []byte{1},
	}
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
	_, err := e.acquire(newTestSegment(testPraosId), nil)
	require.NoError(t, err)
	leios := queueWaiter(t, &e, newTestSegment(testLeiosId))
	praos := queueWaiter(t, &e, newTestSegment(testPraosId))

	e.release()
	requireGranted(t, praos)
	requireNotGranted(t, leios)
	e.release()
	requireGranted(t, leios)
}

func TestEgressLeiosIsNotStarved(t *testing.T) {
	t.Parallel()
	var e egress
	_, err := e.acquire(newTestSegment(testPraosId), nil)
	require.NoError(t, err)
	leios := queueWaiter(t, &e, newTestSegment(testLeiosId))

	// Praos always has another waiter; Leios must still get a turn within
	// praosBurst Praos turns.
	for range praosBurst {
		praos := queueWaiter(t, &e, newTestSegment(testPraosId))
		e.release()
		requireGranted(t, praos)
		requireNotGranted(t, leios)
	}
	queueWaiter(t, &e, newTestSegment(testPraosId))
	e.release()
	requireGranted(t, leios)
}

// Each protocol queues its next segment as soon as the previous one is
// written. A waiting Leios segment must not be passed over indefinitely by
// other Leios protocols that keep queueing newer segments.
func TestEgressLeiosSenderIsNotStarvedByOtherLeios(t *testing.T) {
	t.Parallel()
	var e egress
	_, err := e.acquire(newTestSegment(protocolIdLeiosVotes), nil)
	require.NoError(t, err)
	votes := queueWaiter(t, &e, newTestSegment(protocolIdLeiosVotes))
	queueWaiter(t, &e, newTestSegment(protocolIdLeiosNotify))
	queueWaiter(t, &e, newTestSegment(protocolIdLeiosFetch))
	for _, id := range []uint16{protocolIdLeiosNotify, protocolIdLeiosFetch} {
		e.release()
		queueWaiter(t, &e, newTestSegment(id))
		select {
		case err := <-votes:
			require.NoError(t, err)
			return
		default:
		}
	}
	e.release()
	requireGranted(t, votes)
}

func TestEgressStopReleasesWaiter(t *testing.T) {
	t.Parallel()
	var e egress
	_, err := e.acquire(newTestSegment(testPraosId), nil)
	require.NoError(t, err)
	done := make(chan bool)
	res := make(chan error, 1)
	go func() {
		_, err := e.acquire(newTestSegment(testPraosId), done)
		res <- err
	}()
	require.Eventually(t, func() bool { return e.waiting() == 1 },
		5*time.Second, time.Millisecond)
	close(done)
	require.Error(t, <-res)
	require.Zero(t, e.waiting())
}

func TestEgressDoesNotAcquireAfterStop(t *testing.T) {
	t.Parallel()
	var e egress
	done := make(chan bool)
	close(done)
	_, err := e.acquire(newTestSegment(testPraosId), done)
	require.Error(t, err)
	require.False(t, e.busy)
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
	mu    sync.Mutex
	waits map[EgressClass]int
}

type reentrantEgressMetrics struct {
	Metrics
	m    *Muxer
	done chan error
}

func (r *reentrantEgressMetrics) EgressWait(
	_ uint16,
	_ EgressClass,
	_ time.Duration,
) {
	r.done <- r.m.Send(newTestSegment(testPraosId))
}

func TestEgressMetricsCanSend(t *testing.T) {
	t.Parallel()
	conn := newGateConn(3)
	m := newTestMuxer(t, conn)
	callback := &reentrantEgressMetrics{
		m:    m,
		done: make(chan error, 1),
	}
	m.SetMetrics(callback)
	first := make(chan error, 1)
	second := make(chan error, 1)
	go func() { first <- m.Send(newTestSegment(testPraosId)) }()
	require.Eventually(t, func() bool {
		m.egress.mu.Lock()
		defer m.egress.mu.Unlock()
		return m.egress.busy
	}, 5*time.Second, time.Millisecond)
	go func() { second <- m.Send(newTestSegment(testPraosId)) }()
	require.Eventually(t, func() bool {
		return m.egress.waiting() == 1
	}, 5*time.Second, time.Millisecond)
	for range 3 {
		conn.tokens <- struct{}{}
	}
	require.NoError(t, <-first)
	select {
	case err := <-second:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("metrics callback blocked a second send")
	}
	select {
	case err := <-callback.done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("metrics callback could not send")
	}
}

func (r *egressRecorder) EgressWait(_ uint16, c EgressClass, _ time.Duration) {
	r.mu.Lock()
	r.waits[c]++
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

// A Leios segment delayed by a slow write is still sent, and the muxer keeps
// running with Praos traffic.
func TestMuxerLeiosSegmentDelayedBySlowWriteIsSent(t *testing.T) {
	t.Parallel()
	conn := newGateConn(2)
	m := newTestMuxer(t, conn)
	praosSend, _, _ := m.RegisterProtocol(testPraosId, ProtocolRoleInitiator)
	leiosSend, _, _ := m.RegisterProtocol(testLeiosId, ProtocolRoleInitiator)

	// The Praos write blocks on the connection until a token is given.
	praosSend <- NewSegment(testPraosId, []byte{1}, false)
	require.Eventually(t, func() bool {
		m.egress.mu.Lock()
		defer m.egress.mu.Unlock()
		return m.egress.busy
	}, 5*time.Second, time.Millisecond)
	// The Leios segment queues behind it.
	leiosSend <- newTestSegment(testLeiosId)
	stopped := func() bool {
		select {
		case <-m.doneChan:
			return true
		default:
			return false
		}
	}
	require.Eventually(t, func() bool {
		return m.egress.waiting() == 1 || stopped()
	}, 5*time.Second, time.Millisecond)
	require.False(t, stopped(), "muxer stopped while Leios segment waited")

	conn.tokens <- struct{}{}
	conn.tokens <- struct{}{}
	require.Eventually(t, func() bool {
		return len(conn.order()) == 2 || stopped()
	}, 5*time.Second, time.Millisecond)
	require.False(t, stopped(), "muxer stopped; written=%v", conn.order())
	require.Equal(t, []uint16{testPraosId, testLeiosId}, conn.order())
}
