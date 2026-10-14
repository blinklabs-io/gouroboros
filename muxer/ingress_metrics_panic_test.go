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
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

// panicMetrics records like fakeMetrics, then panics in the callbacks it
// is told to.
type panicMetrics struct {
	fakeMetrics
	depth        bool
	blocked      bool
	backpressure bool
}

func (p *panicMetrics) IngressQueueDepth(
	id uint16,
	role muxer.ProtocolRole,
	n int,
) {
	p.fakeMetrics.IngressQueueDepth(id, role, n)
	if p.depth {
		panic("injected IngressQueueDepth panic")
	}
}

func (p *panicMetrics) IngressDeliveryBlocked(
	id uint16,
	role muxer.ProtocolRole,
	d time.Duration,
) {
	p.fakeMetrics.IngressDeliveryBlocked(id, role, d)
	if p.blocked {
		panic("injected IngressDeliveryBlocked panic")
	}
}

func (p *panicMetrics) IngressBackpressure(
	id uint16,
	role muxer.ProtocolRole,
	d time.Duration,
) {
	p.fakeMetrics.IngressBackpressure(id, role, d)
	if p.backpressure {
		panic("injected IngressBackpressure panic")
	}
}

// writePanicTestSegments queues count segments of payload for protocol 1.
func writePanicTestSegments(
	t *testing.T,
	conn *mockConn,
	count int,
	payload string,
) {
	t.Helper()
	var buf bytes.Buffer
	for range count {
		seg := muxer.NewSegment(0x01, []byte(payload), false)
		require.NotNil(t, seg)
		buf.Write(createSegmentData(seg))
	}
	conn.WriteToReadBuf(buf.Bytes())
}

// requireMuxerHealthy checks that the muxer has reported no error.
func requireMuxerHealthy(t *testing.T, m *muxer.Muxer) {
	t.Helper()
	select {
	case err := <-m.ErrorChan():
		t.Fatalf("muxer failed: %v", err)
	default:
	}
}

// waitMuxerFinished waits for the muxer to finish stopping: ErrorChan is
// closed only after the read loop's cleanup, which waits for every delivery
// goroutine, has completed. DoneChan only signals that a stop was requested.
// Errors still buffered are discarded.
func waitMuxerFinished(t *testing.T, m *muxer.Muxer) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-m.ErrorChan():
			if !ok {
				return
			}
		case <-timeout:
			t.Fatal("muxer did not finish stopping")
		}
	}
}

// requireMuxerStops checks that Stop returns and the muxer finishes stopping,
// which it cannot if a callback panic left a queue locked.
func requireMuxerStops(t *testing.T, m *muxer.Muxer) {
	t.Helper()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		m.Stop()
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("muxer did not stop")
	}
	waitMuxerFinished(t, m)
}

// TestIngressMetricsPanicsAreContained checks that a panic in the queue-depth
// or delivery callback neither escapes the read loop or the forwarder nor
// stops delivery, and that every depth is still reported.
func TestIngressMetricsPanicsAreContained(t *testing.T) {
	const segments = 15
	tests := []struct {
		name    string
		metrics *panicMetrics
	}{
		{name: "queue depth", metrics: &panicMetrics{depth: true}},
		{name: "delivery blocked", metrics: &panicMetrics{blocked: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conn := newMockConn()
			m := muxer.New(conn)
			t.Cleanup(m.Stop)
			m.SetMetrics(tc.metrics)
			_, recvChan, _ := m.RegisterProtocol(
				0x01,
				muxer.ProtocolRoleResponder,
			)
			m.Start()

			// Hold the consumer so delivery waits and segments queue.
			writePanicTestSegments(t, conn, segments, "payload")
			require.Eventually(t, func() bool {
				return len(recvChan) == cap(recvChan)
			}, 2*time.Second, time.Millisecond)
			<-time.After(20 * time.Millisecond)
			for range segments {
				select {
				case <-recvChan:
				case <-time.After(2 * time.Second):
					t.Fatal("segment was not delivered")
				}
			}

			require.Eventually(t, func() bool {
				depths, _, blocked := tc.metrics.snapshot()
				return len(depths) == 2*segments && len(blocked) > 0
			}, 2*time.Second, time.Millisecond)
			requireMuxerHealthy(t, m)
			requireMuxerStops(t, m)
		})
	}
}

// TestIngressQueueDepthPanicDuringCleanup checks that stopping a muxer whose
// depth callback panics, with segments still queued, completes.
func TestIngressQueueDepthPanicDuringCleanup(t *testing.T) {
	conn := newMockConn()
	m := muxer.New(conn)
	t.Cleanup(m.Stop)
	metrics := &panicMetrics{depth: true}
	m.SetMetrics(metrics)
	_, recvChan, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	m.Start()

	writePanicTestSegments(t, conn, 15, "payload")
	require.Eventually(t, func() bool {
		_, maxDepth, _ := metrics.snapshot()
		return len(recvChan) == cap(recvChan) && maxDepth > 0
	}, 2*time.Second, time.Millisecond)
	requireMuxerStops(t, m)
}

// TestIngressBackpressurePanicIsContained checks that a panic in the
// backpressure callback does not escape the read loop, which carries on
// delivering once the held protocol drains.
func TestIngressBackpressurePanicIsContained(t *testing.T) {
	const segments = 40
	conn := newMockConn()
	m := muxer.New(conn)
	t.Cleanup(m.Stop)
	metrics := &panicMetrics{backpressure: true}
	m.SetMetrics(metrics)
	_, recvChan, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	require.True(t, m.SetIngressLimit(0x01, muxer.ProtocolRoleResponder, 100))
	require.True(
		t,
		m.SetIngressBackpressure(0x01, muxer.ProtocolRoleResponder, true),
	)
	m.Start()

	writePanicTestSegments(t, conn, segments, "0123456789")
	require.Eventually(t, func() bool {
		_, maxDepth, _ := metrics.snapshot()
		return maxDepth == 100
	}, 2*time.Second, time.Millisecond, "queue did not fill to its limit")
	for range segments {
		select {
		case <-recvChan:
		case <-time.After(2 * time.Second):
			t.Fatal("segment was not delivered")
		}
	}
	require.NotEmpty(t, metrics.backpressureWaits())
	requireMuxerHealthy(t, m)
	requireMuxerStops(t, m)
}

// unreadBytes returns how many bytes of conn's input the muxer has not read.
func unreadBytes(conn *mockConn) int {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return conn.readBuf.Len()
}

// TestIngressBackpressurePanicDuringStop checks that stopping the muxer while
// the read loop waits under backpressure completes when the backpressure
// callback, which reports that wait as the read loop exits, panics.
func TestIngressBackpressurePanicDuringStop(t *testing.T) {
	const (
		segments = 40
		payload  = "0123456789"
		limit    = 100
	)
	conn := newMockConn()
	m := muxer.New(conn)
	t.Cleanup(m.Stop)
	metrics := &panicMetrics{backpressure: true}
	m.SetMetrics(metrics)
	_, recvChan, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	require.True(t, m.SetIngressLimit(0x01, muxer.ProtocolRoleResponder, limit))
	require.True(
		t,
		m.SetIngressBackpressure(0x01, muxer.ProtocolRoleResponder, true),
	)
	m.Start()

	// Nothing drains the protocol. The receive channel, the segment its
	// forwarder holds and a full queue take the first segments, and the read
	// loop then waits for room with the next one in hand.
	seg := muxer.NewSegment(0x01, []byte(payload), false)
	require.NotNil(t, seg)
	wireLen := len(createSegmentData(seg))
	admitted := cap(recvChan) + 1 + limit/len(payload)
	writePanicTestSegments(t, conn, segments, payload)
	require.Eventually(t, func() bool {
		return unreadBytes(conn) == (segments-admitted-1)*wireLen
	}, 2*time.Second, time.Millisecond, "read loop did not reach the wait")
	// The read loop reads nothing more while it waits. Give it a moment to
	// enter the wait it is about to make.
	<-time.After(50 * time.Millisecond)
	require.Equal(t, (segments-admitted-1)*wireLen, unreadBytes(conn))

	requireMuxerStops(t, m)
	require.Eventually(t, func() bool {
		return len(metrics.backpressureWaits()) > 0
	}, 2*time.Second, time.Millisecond, "backpressure wait was not reported")
}

// lockCheckMetrics calls back into the muxer from the depth callback, which
// waits on the queue's lock and so deadlocks if the callback holds it.
type lockCheckMetrics struct {
	fakeMetrics
	m *muxer.Muxer
}

func (l *lockCheckMetrics) IngressQueueDepth(
	id uint16,
	role muxer.ProtocolRole,
	n int,
) {
	l.fakeMetrics.IngressQueueDepth(id, role, n)
	_ = l.m.IngressLimit(id, role)
}

// TestIngressQueueDepthRunsOutsideQueueLock checks that the depth callback
// does not hold the queue's lock, and that the depths it receives follow the
// queue's changes in order: each differs from the last by one segment, and
// the last is zero.
func TestIngressQueueDepthRunsOutsideQueueLock(t *testing.T) {
	const segments = 15
	conn := newMockConn()
	m := muxer.New(conn)
	t.Cleanup(m.Stop)
	metrics := &lockCheckMetrics{m: m}
	m.SetMetrics(metrics)
	_, recvChan, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	m.Start()

	writePanicTestSegments(t, conn, segments, "payload")
	for range segments {
		select {
		case <-recvChan:
		case <-time.After(2 * time.Second):
			t.Fatal("segment was not delivered")
		}
	}
	require.Eventually(t, func() bool {
		depths, _, _ := metrics.snapshot()
		return len(depths) == 2*segments
	}, 2*time.Second, time.Millisecond)

	depths, _, _ := metrics.snapshot()
	previous := 0
	for i, depth := range depths {
		step := depth - previous
		require.True(
			t,
			step == len("payload") || step == -len("payload"),
			"depth %d of %v", i, depths,
		)
		previous = depth
	}
	require.Zero(t, previous)
}

// unregisterMetrics unregisters another protocol and registers a new one
// from its first depth report, both of which the Metrics contract allows.
type unregisterMetrics struct {
	fakeMetrics
	m    *muxer.Muxer
	once sync.Once
	done chan struct{}
}

func (u *unregisterMetrics) IngressQueueDepth(
	id uint16,
	role muxer.ProtocolRole,
	n int,
) {
	u.fakeMetrics.IngressQueueDepth(id, role, n)
	if id != 0x01 {
		return
	}
	u.once.Do(func() {
		u.m.UnregisterProtocol(0x02, muxer.ProtocolRoleResponder)
		u.m.UnregisterProtocol(0x01, muxer.ProtocolRoleInitiator)
		u.m.RegisterProtocol(0x03, muxer.ProtocolRoleResponder)
		close(u.done)
	})
}

// TestIngressQueueDepthMayUnregisterOtherRoles checks the supported side of
// the Metrics contract: a depth callback may unregister a different protocol
// or role, and register one, without deadlocking the muxer.
func TestIngressQueueDepthMayUnregisterOtherRoles(t *testing.T) {
	conn := newMockConn()
	m := muxer.New(conn)
	t.Cleanup(m.Stop)
	metrics := &unregisterMetrics{m: m, done: make(chan struct{})}
	m.SetMetrics(metrics)
	_, recvChan, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	_, _, _ = m.RegisterProtocol(0x01, muxer.ProtocolRoleInitiator)
	_, _, _ = m.RegisterProtocol(0x02, muxer.ProtocolRoleResponder)
	m.Start()

	writePanicTestSegments(t, conn, 1, "payload")
	select {
	case <-metrics.done:
	case <-time.After(5 * time.Second):
		t.Fatal("depth callback did not return from unregistering")
	}
	select {
	case got := <-recvChan:
		require.Equal(t, []byte("payload"), got.Payload)
	case <-time.After(5 * time.Second):
		t.Fatal("segment was not delivered")
	}
	requireMuxerHealthy(t, m)
	requireMuxerStops(t, m)
}
