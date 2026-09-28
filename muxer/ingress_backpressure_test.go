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
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

// TestReadLoopDoesNotBlockOnFullProtocolChannel covers
// blinklabs-io/gouroboros#2598: the muxer read loop is shared by every
// mini-protocol multiplexed over one connection, so it must never block
// delivering to one protocol's full receive channel. A slow consumer on one
// protocol (never drained here) previously stalled the shared read loop once
// its fixed-size receive channel filled, starving every other protocol -
// keep-alive in particular - behind it.
func TestReadLoopDoesNotBlockOnFullProtocolChannel(t *testing.T) {
	t.Parallel()

	conn := newMockConn()
	m := muxer.New(conn)
	defer m.Stop()

	// slowRecv is deliberately never read from.
	_, slowRecv, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	_, fastRecv, _ := m.RegisterProtocol(0x02, muxer.ProtocolRoleResponder)

	m.Start()

	var buf bytes.Buffer
	// Flood the slow protocol with more segments than its receive channel's
	// capacity (10), so the old fixed-buffer channel would already be full
	// by the time the fast protocol's segment arrives.
	for range 50 {
		seg := muxer.NewSegment(0x01, []byte("slow payload"), false)
		require.NotNil(t, seg)
		buf.Write(createSegmentData(seg))
	}
	fastSeg := muxer.NewSegment(0x02, []byte("fast payload"), false)
	require.NotNil(t, fastSeg)
	buf.Write(createSegmentData(fastSeg))
	conn.WriteToReadBuf(buf.Bytes())

	select {
	case seg := <-fastRecv:
		require.Equal(t, []byte("fast payload"), seg.Payload)
	case err := <-m.ErrorChan():
		t.Fatalf("unexpected muxer error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal(
			"fast protocol starved behind the slow protocol's full receive channel",
		)
	}

	_ = slowRecv // intentionally never drained
}

// TestIngressOverflowIsAProtocolViolation checks that ingress past one
// protocol role's own limit stops the muxer with ErrIngressOverflow, and that
// the limit is per protocol: another protocol with a larger limit, fed the
// same bytes first, is unaffected.
func TestIngressOverflowIsAProtocolViolation(t *testing.T) {
	t.Parallel()

	conn := newMockConn()
	m := muxer.New(conn)
	defer m.Stop()

	_, roomyRecv, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	_, tightRecv, _ := m.RegisterProtocol(0x02, muxer.ProtocolRoleResponder)
	require.True(t, m.SetIngressLimit(0x01, muxer.ProtocolRoleResponder, 1000))
	require.True(t, m.SetIngressLimit(0x02, muxer.ProtocolRoleResponder, 32))
	require.Equal(t, 1000, m.IngressLimit(0x01, muxer.ProtocolRoleResponder))
	require.Equal(t, 32, m.IngressLimit(0x02, muxer.ProtocolRoleResponder))
	require.False(t, m.SetIngressLimit(0x03, muxer.ProtocolRoleResponder, 32))
	require.Zero(t, m.IngressLimit(0x03, muxer.ProtocolRoleResponder))
	m.Start()

	// Neither receiver is drained. Each takes 10 segments into its receive
	// channel before anything is counted against its queue, then 20 more
	// bytes of 10-byte segments.
	var buf bytes.Buffer
	for range 13 {
		seg := muxer.NewSegment(0x01, []byte("0123456789"), false)
		require.NotNil(t, seg)
		buf.Write(createSegmentData(seg))
	}
	conn.WriteToReadBuf(buf.Bytes())
	require.Eventually(t, func() bool {
		return len(roomyRecv) == cap(roomyRecv)
	}, 2*time.Second, time.Millisecond)
	select {
	case err := <-m.ErrorChan():
		t.Fatalf("protocol within its own limit failed the muxer: %v", err)
	default:
	}

	buf.Reset()
	for range 15 {
		seg := muxer.NewSegment(0x02, []byte("0123456789"), false)
		require.NotNil(t, seg)
		buf.Write(createSegmentData(seg))
	}
	conn.WriteToReadBuf(buf.Bytes())
	select {
	case err := <-m.ErrorChan():
		require.ErrorIs(t, err, muxer.ErrIngressOverflow)
		require.ErrorContains(t, err, "protocol 2:")
	case <-time.After(2 * time.Second):
		t.Fatal("expected an ingress overflow protocol error, got neither")
	}
	_ = tightRecv
}

// fakeMetrics records Metrics callback invocations for assertions.
type fakeMetrics struct {
	mu       sync.Mutex
	depths   []int
	maxDepth int
	blocked  []time.Duration
}

func (f *fakeMetrics) IngressQueueDepth(_ uint16, _ muxer.ProtocolRole, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.depths = append(f.depths, n)
	f.maxDepth = max(f.maxDepth, n)
}

func (f *fakeMetrics) IngressDeliveryBlocked(
	_ uint16,
	_ muxer.ProtocolRole,
	d time.Duration,
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blocked = append(f.blocked, d)
}

func (f *fakeMetrics) snapshot() (
	depths []int,
	maxDepth int,
	blocked []time.Duration,
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.depths...),
		f.maxDepth,
		append([]time.Duration(nil), f.blocked...)
}

// TestMuxerReportsIngressMetrics holds a consumer long enough for segments
// to queue behind its full receive channel, then drains it. The depth must
// be reported as the queue fills and again as it empties, ending at zero,
// and the time delivery waited on the held consumer must be reported.
func TestMuxerReportsIngressMetrics(t *testing.T) {
	t.Parallel()

	conn := newMockConn()
	m := muxer.New(conn)
	defer m.Stop()

	metrics := &fakeMetrics{}
	m.SetMetrics(metrics)

	_, recvChan, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)
	m.Start()

	const segments = 15
	var buf bytes.Buffer
	for range segments {
		seg := muxer.NewSegment(0x01, []byte("payload"), false)
		require.NotNil(t, seg)
		buf.Write(createSegmentData(seg))
	}
	conn.WriteToReadBuf(buf.Bytes())

	// 10 segments fill the receive channel and one more is held by the
	// delivery waiting for room, so the rest stay queued.
	require.Eventually(t, func() bool {
		_, maxDepth, _ := metrics.snapshot()
		return len(recvChan) == cap(recvChan) &&
			maxDepth >= (segments-cap(recvChan)-1)*len("payload")
	}, 2*time.Second, time.Millisecond)
	held := time.Now()
	<-time.After(40 * time.Millisecond)
	heldFor := time.Since(held)

	for range segments {
		select {
		case <-recvChan:
		case <-time.After(2 * time.Second):
			t.Fatal("did not receive segment")
		}
	}
	require.Eventually(t, func() bool {
		depths, _, _ := metrics.snapshot()
		return len(depths) == 2*segments
	}, 2*time.Second, time.Millisecond,
		"expected a depth report for every enqueue and every dequeue")
	require.Eventually(t, func() bool {
		_, _, blocked := metrics.snapshot()
		return len(blocked) > 0
	}, 2*time.Second, time.Millisecond,
		"expected the delivery wait on the held consumer to be reported")

	depths, _, blocked := metrics.snapshot()
	require.Zero(t, depths[len(depths)-1], "depth after draining: %v", depths)
	longest := slices.Max(blocked)
	// The delivery can start waiting a moment after the hold is observed,
	// so only half the hold is asserted.
	require.GreaterOrEqual(
		t,
		longest,
		heldFor/2,
		"delivery wait on the held consumer: %v", blocked,
	)
}
