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
	"math"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

// segmentCapacity is the capacity of a receiver's delivery channel. Segments
// beyond it queue in the muxer's ingress queue, which is what the ingress
// limits and the connection budget bound.
const segmentCapacity = 10

// segmentSize is the payload size of every segment these tests send.
const segmentSize = 10

// ingressSegments returns n segments of segmentSize bytes for protocolId.
func ingressSegments(t *testing.T, protocolId uint16, n int) []byte {
	t.Helper()
	var buf bytes.Buffer
	for range n {
		seg := muxer.NewSegment(
			protocolId,
			bytes.Repeat([]byte("x"), segmentSize),
			false,
		)
		require.NotNil(t, seg)
		buf.Write(createSegmentData(seg))
	}
	return buf.Bytes()
}

// stallProtocol sends a protocol enough segments to fill its delivery
// channel and leave one more held by the delivery goroutine, then waits
// until the muxer's ingress accounting is back at wantInUse. After it the
// protocol's consumer is stalled and its queue is empty, so every further
// segment stays queued and counted, and the accounting is deterministic.
func stallProtocol(
	t *testing.T,
	conn *mockConn,
	m *muxer.Muxer,
	recv chan *muxer.Segment,
	protocolId uint16,
	wantInUse int,
) {
	t.Helper()
	conn.WriteToReadBuf(ingressSegments(t, protocolId, segmentCapacity+1))
	require.Eventually(t, func() bool {
		return len(recv) == cap(recv) && m.IngressInUse() == wantInUse
	}, 5*time.Second, time.Millisecond)
}

// queueBytes sends n segments to a stalled protocol and waits until the
// muxer accounts for wantInUse bytes in total.
func queueBytes(
	t *testing.T,
	conn *mockConn,
	m *muxer.Muxer,
	protocolId uint16,
	n int,
	wantInUse int,
) {
	t.Helper()
	conn.WriteToReadBuf(ingressSegments(t, protocolId, n))
	require.Eventually(t, func() bool {
		return m.IngressInUse() == wantInUse
	}, 5*time.Second, time.Millisecond)
}

func requireNoMuxerError(t *testing.T, m *muxer.Muxer) {
	t.Helper()
	select {
	case err := <-m.ErrorChan():
		t.Fatalf("muxer failed within the budget: %v", err)
	default:
	}
}

// TestIngressBudgetBoundsAggregateAcrossProtocols checks that two protocol
// roles, each within its own ingress limit, are stopped once their queues
// together pass the connection budget, and not before.
func TestIngressBudgetBoundsAggregateAcrossProtocols(t *testing.T) {
	t.Parallel()

	conn := newMockConn()
	m := muxer.New(conn)
	defer m.Stop()

	_, recvA, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	_, recvB, _ := m.RegisterProtocol(0x02, muxer.ProtocolRoleResponder)
	require.True(t, m.SetIngressLimit(0x01, muxer.ProtocolRoleResponder, 1000))
	require.True(t, m.SetIngressLimit(0x02, muxer.ProtocolRoleResponder, 1000))
	require.Equal(t, muxer.DefaultIngressBudget, m.IngressBudget())
	m.SetIngressBudget(200)
	require.Equal(t, 200, m.IngressBudget())
	m.Start()

	stallProtocol(t, conn, m, recvA, 0x01, 0)
	queueBytes(t, conn, m, 0x01, 6, 60)
	stallProtocol(t, conn, m, recvB, 0x02, 60)
	queueBytes(t, conn, m, 0x02, 6, 120)
	// Each queue is far under its own 1000 byte limit. Exactly the budget
	// is allowed.
	queueBytes(t, conn, m, 0x02, 8, 200)
	requireNoMuxerError(t, m)

	conn.WriteToReadBuf(ingressSegments(t, 0x01, 1))
	select {
	case err := <-m.ErrorChan():
		require.ErrorIs(t, err, muxer.ErrIngressOverflow)
		require.ErrorContains(t, err, "connection budget")
	case <-time.After(5 * time.Second):
		t.Fatal("expected a connection ingress budget overflow, got none")
	}
}

// TestIngressBudgetIsReturnedAsQueuesDrain checks the budget is a bound on
// bytes held, not on bytes ever received: a consumer that keeps up never
// exhausts it, and the accounting returns to zero.
func TestIngressBudgetIsReturnedAsQueuesDrain(t *testing.T) {
	t.Parallel()

	conn := newMockConn()
	m := muxer.New(conn)
	defer m.Stop()

	_, recv, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	require.True(t, m.SetIngressLimit(0x01, muxer.ProtocolRoleResponder, 1000))
	m.SetIngressBudget(120)
	m.Start()

	// Fifty segments, 500 bytes, are sent in batches of ten: the budget
	// is only ever exceeded by bytes that were already delivered.
	const batches = 5
	for range batches {
		conn.WriteToReadBuf(ingressSegments(t, 0x01, 10))
		for range 10 {
			select {
			case <-recv:
			case err := <-m.ErrorChan():
				t.Fatalf("draining consumer hit the budget: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("segment not delivered")
			}
		}
	}
	require.Eventually(t, func() bool {
		return m.IngressInUse() == 0
	}, 5*time.Second, time.Millisecond)
}

// TestIngressBudgetIsReturnedWhenProtocolUnregisters checks the queued bytes
// of a protocol that is unregistered stop counting against the connection.
func TestIngressBudgetIsReturnedWhenProtocolUnregisters(t *testing.T) {
	t.Parallel()

	conn := newMockConn()
	m := muxer.New(conn)
	defer m.Stop()

	_, recv, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	require.True(t, m.SetIngressLimit(0x01, muxer.ProtocolRoleResponder, 1000))
	m.SetIngressBudget(200)
	m.Start()

	stallProtocol(t, conn, m, recv, 0x01, 0)
	queueBytes(t, conn, m, 0x01, 6, 60)
	m.UnregisterProtocol(0x01, muxer.ProtocolRoleResponder)
	require.Eventually(t, func() bool {
		return m.IngressInUse() == 0
	}, 5*time.Second, time.Millisecond)
}

// TestIngressBudgetBackpressureWaitsInsteadOfFailing checks a protocol role
// with backpressure enabled pauses the read loop at the connection budget
// and resumes when its consumer takes segments, rather than failing the
// muxer.
func TestIngressBudgetBackpressureWaitsInsteadOfFailing(t *testing.T) {
	t.Parallel()

	conn := newMockConn()
	m := muxer.New(conn)
	defer m.Stop()

	_, recv, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	require.True(t, m.SetIngressLimit(0x01, muxer.ProtocolRoleResponder, 1000))
	require.True(
		t,
		m.SetIngressBackpressure(0x01, muxer.ProtocolRoleResponder, true),
	)
	m.SetIngressBudget(50)
	m.Start()

	const total = 40
	conn.WriteToReadBuf(ingressSegments(t, 0x01, total))
	for range total {
		select {
		case <-recv:
		case err := <-m.ErrorChan():
			t.Fatalf("backpressured protocol failed the muxer: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("segment not delivered under backpressure")
		}
	}
}

// TestIngressBudgetBackpressureAdmitsWhenOwnQueueEmpty checks a backpressured
// role whose own queue is empty still admits a segment when other roles hold
// the whole budget, since no dequeue of its own could ever make room.
func TestIngressBudgetBackpressureAdmitsWhenOwnQueueEmpty(t *testing.T) {
	t.Parallel()

	conn := newMockConn()
	m := muxer.New(conn)
	defer m.Stop()

	_, recvHolder, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	_, recvBP, _ := m.RegisterProtocol(0x02, muxer.ProtocolRoleResponder)
	require.True(t, m.SetIngressLimit(0x01, muxer.ProtocolRoleResponder, 1000))
	require.True(t, m.SetIngressLimit(0x02, muxer.ProtocolRoleResponder, 1000))
	require.True(
		t,
		m.SetIngressBackpressure(0x02, muxer.ProtocolRoleResponder, true),
	)
	m.SetIngressBudget(120)
	m.Start()

	// The holder's stalled queue takes the whole budget.
	stallProtocol(t, conn, m, recvHolder, 0x01, 0)
	queueBytes(t, conn, m, 0x01, 12, 120)

	conn.WriteToReadBuf(ingressSegments(t, 0x02, 1))
	select {
	case seg := <-recvBP:
		require.Len(t, seg.Payload, segmentSize)
	case err := <-m.ErrorChan():
		t.Fatalf("backpressured role failed the muxer: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("backpressured role with an empty queue was not admitted")
	}
}

// A release by another role must wake a read loop waiting on the shared
// budget even when the waiting role's own queue has not changed.
func TestIngressBudgetBackpressureWakesOnOtherRoleRelease(t *testing.T) {
	t.Parallel()

	conn := newMockConn()
	m := muxer.New(conn)
	defer m.Stop()

	_, holder, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	_, waiting, _ := m.RegisterProtocol(0x02, muxer.ProtocolRoleResponder)
	require.True(t, m.SetIngressLimit(0x01, muxer.ProtocolRoleResponder, 1000))
	require.True(t, m.SetIngressLimit(0x02, muxer.ProtocolRoleResponder, 1000))
	require.True(t, m.SetIngressBackpressure(0x02, muxer.ProtocolRoleResponder, true))
	m.SetIngressBudget(120)
	m.Start()

	stallProtocol(t, conn, m, waiting, 0x02, 0)
	stallProtocol(t, conn, m, holder, 0x01, 0)
	queueBytes(t, conn, m, 0x01, 12, 120)
	queueBytes(t, conn, m, 0x02, 1, 130)
	conn.WriteToReadBuf(ingressSegments(t, 0x02, 1))
	require.Eventually(t, func() bool {
		conn.mu.Lock()
		defer conn.mu.Unlock()
		return conn.readBuf.Len() == 0
	}, 5*time.Second, time.Millisecond)
	require.Never(t, func() bool {
		return m.IngressInUse() != 130
	}, 100*time.Millisecond, time.Millisecond)

	// The second waiting-role segment cannot fit until holder drains.
	for range 2 {
		select {
		case <-holder:
		case err := <-m.ErrorChan():
			t.Fatalf("muxer failed while waiting: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("holder did not drain")
		}
	}
	require.Eventually(t, func() bool {
		return m.IngressInUse() == 120
	}, 5*time.Second, time.Millisecond)
	for range segmentCapacity + 3 {
		select {
		case <-waiting:
		case err := <-m.ErrorChan():
			t.Fatalf("muxer failed after budget release: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("other role's release did not wake ingress")
		}
	}
}

// TestIngressBudgetExtensionAdmitsSolicitedIngress checks a protocol role
// that extends the budget may queue past the base budget, up to the extended
// total and no further, and that unregistering it ends the extension.
func TestIngressBudgetExtensionAdmitsSolicitedIngress(t *testing.T) {
	t.Parallel()

	conn := newMockConn()
	m := muxer.New(conn)
	defer m.Stop()

	_, recv, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	require.True(t, m.SetIngressLimit(0x01, muxer.ProtocolRoleResponder, 1000))
	m.SetIngressBudget(200)
	require.True(
		t,
		m.SetIngressBudgetExtension(0x01, muxer.ProtocolRoleResponder, 300),
	)
	require.False(
		t,
		m.SetIngressBudgetExtension(0x09, muxer.ProtocolRoleResponder, 300),
	)
	require.Equal(t, 500, m.IngressBudget())
	m.Start()

	stallProtocol(t, conn, m, recv, 0x01, 0)
	// Exactly the extended budget is admitted.
	queueBytes(t, conn, m, 0x01, 50, 500)
	requireNoMuxerError(t, m)

	conn.WriteToReadBuf(ingressSegments(t, 0x01, 1))
	select {
	case err := <-m.ErrorChan():
		require.ErrorIs(t, err, muxer.ErrIngressOverflow)
		require.ErrorContains(t, err, "connection budget")
	case <-time.After(5 * time.Second):
		t.Fatal("expected a connection ingress budget overflow, got none")
	}

	m.UnregisterProtocol(0x01, muxer.ProtocolRoleResponder)
	require.Equal(t, 200, m.IngressBudget())
}

// TestIngressBudgetExtensionIsReplacedAndCleared checks each call replaces
// the role's extension and that zero clears it.
func TestIngressBudgetExtensionIsReplacedAndCleared(t *testing.T) {
	t.Parallel()

	m := muxer.New(newMockConn())
	defer m.Stop()
	m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	m.RegisterProtocol(0x02, muxer.ProtocolRoleResponder)
	m.SetIngressBudget(100)

	extend := func(protocolId uint16, extra int) {
		t.Helper()
		require.True(
			t,
			m.SetIngressBudgetExtension(
				protocolId,
				muxer.ProtocolRoleResponder,
				extra,
			),
		)
	}
	extend(0x01, 40)
	extend(0x02, 7)
	require.Equal(t, 147, m.IngressBudget())
	extend(0x01, 10)
	require.Equal(t, 117, m.IngressBudget())
	extend(0x01, 0)
	require.Equal(t, 107, m.IngressBudget())
	extend(0x02, math.MaxInt)
	require.Greater(t, m.IngressBudget(), 107)
}

func TestIngressBudgetExtensionsCannotOverflowAcrossRoles(t *testing.T) {
	t.Parallel()

	m := muxer.New(newMockConn())
	defer m.Stop()
	m.SetIngressBudget(100)
	previous := m.IngressBudget()
	for id := uint16(1); id <= 65; id++ {
		m.RegisterProtocol(id, muxer.ProtocolRoleResponder)
		require.True(t, m.SetIngressBudgetExtension(id, muxer.ProtocolRoleResponder, math.MaxInt))
		current := m.IngressBudget()
		require.Greater(t, current, previous)
		previous = current
	}
}
