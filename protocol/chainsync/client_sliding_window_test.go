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

package chainsync

import (
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// delayConn delivers every Write to the wrapped connection after a fixed
// one-way delay, preserving order, so a pair of them models a link with a
// round-trip time.
type delayConn struct {
	net.Conn
	delay  time.Duration
	queue  chan delayedWrite
	done   chan struct{}
	closed sync.Once
}

type delayedWrite struct {
	data []byte
	at   time.Time
}

func newDelayConn(c net.Conn, delay time.Duration) *delayConn {
	d := &delayConn{
		Conn:  c,
		delay: delay,
		queue: make(chan delayedWrite, 1<<16),
		done:  make(chan struct{}),
	}
	go func() {
		for {
			select {
			case w := <-d.queue:
				if wait := time.Until(w.at); wait > 0 {
					select {
					case <-time.After(wait):
					case <-d.done:
						return
					}
				}
				if _, err := d.Conn.Write(w.data); err != nil {
					return
				}
			case <-d.done:
				return
			}
		}
	}()
	return d
}

func (d *delayConn) Write(p []byte) (int, error) {
	select {
	case <-d.done:
		return 0, net.ErrClosed
	default:
	}
	d.queue <- delayedWrite{
		data: append([]byte(nil), p...),
		at:   time.Now().Add(d.delay),
	}
	return len(p), nil
}

func (d *delayConn) Close() error {
	d.closed.Do(func() { close(d.done) })
	return d.Conn.Close()
}

type windowEventKind int

const (
	windowForward windowEventKind = iota
	windowBackward
)

// windowEvent is one reply the scripted server gives to a RequestNext. n is
// the block number for a roll forward and the rollback target for a roll
// backward.
type windowEvent struct {
	kind windowEventKind
	n    uint64
}

// delivered is one callback invocation observed by the client.
type delivered struct {
	kind windowEventKind
	n    uint64
}

type windowHarness struct {
	client *Client
	server *Server
	// atTip counts AwaitReply callbacks.
	atTip atomic.Int32

	mu        sync.Mutex
	events    []delivered
	minInFlig int
	maxInFlig int
	served    atomic.Int64
	stop      chan struct{}
	protoErrs []error
}

func windowHash(n uint64) []byte {
	h := make([]byte, 32)
	binary.BigEndian.PutUint64(h, n)
	return h
}

func windowTip(n uint64) Tip {
	return Tip{
		Point:       pcommon.NewPoint(n, windowHash(n)),
		BlockNumber: n,
	}
}

func windowBlock(t testing.TB, n uint64) []byte {
	t.Helper()
	b, err := cbor.Encode(n)
	require.NoError(t, err)
	return b
}

// newWindowHarness connects a Client to a scripted Server over a link with the
// given round-trip time. The server answers the i-th RequestNext with
// script[i]; once the script is exhausted it answers AwaitReply. onForward, if
// non-nil, runs inside the client's roll-forward callback before the delivery
// is recorded; it may block to model a slow consumer or return an error.
func newWindowHarness(
	t testing.TB,
	limit int,
	rtt time.Duration,
	script []windowEvent,
	onForward func(n uint64) error,
) *windowHarness {
	t.Helper()
	h := &windowHarness{minInFlig: 1 << 30, stop: make(chan struct{})}
	clientRaw, serverRaw := net.Pipe()
	clientConn := newDelayConn(clientRaw, rtt/2)
	serverConn := newDelayConn(serverRaw, rtt/2)
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)

	errs := make(chan error, 8)
	go func() {
		for {
			select {
			case err := <-errs:
				h.mu.Lock()
				h.protoErrs = append(h.protoErrs, err)
				h.mu.Unlock()
			case <-h.stop:
				return
			}
		}
	}()
	var scriptIdx atomic.Int64
	server := NewServer(
		protocol.ProtocolOptions{
			Muxer:     serverMuxer,
			ErrorChan: errs,
			Mode:      protocol.ProtocolModeNodeToClient,
		},
		&Config{
			FindIntersectFunc: func(
				_ CallbackContext,
				points []pcommon.Point,
			) (pcommon.Point, Tip, error) {
				return points[0], windowTip(0), nil
			},
			RequestNextFunc: func(ctx CallbackContext) error {
				i := int(scriptIdx.Add(1) - 1)
				if i >= len(script) {
					return ctx.Server.AwaitReply()
				}
				h.served.Add(1)
				ev := script[i]
				if ev.kind == windowBackward {
					return ctx.Server.RollBackward(
						pcommon.NewPoint(
							ev.n,
							windowHash(ev.n),
						),
						windowTip(ev.n),
					)
				}
				return ctx.Server.RollForward(
					0,
					windowBlock(t, ev.n),
					windowTip(ev.n),
				)
			},
		},
	)
	h.server = server

	cfg := NewConfig(
		WithPipelineLimit(limit),
		WithRollForwardRawFunc(func(
			_ CallbackContext,
			_ uint,
			data []byte,
			_ Tip,
		) error {
			var n uint64
			if _, err := cbor.Decode(data, &n); err != nil {
				return err
			}
			h.sampleInFlight()
			if onForward != nil {
				if err := onForward(n); err != nil {
					h.record(delivered{windowForward, n})
					return err
				}
			}
			h.record(delivered{windowForward, n})
			return nil
		}),
		WithRollBackwardFunc(func(
			_ CallbackContext,
			point pcommon.Point,
			_ Tip,
		) error {
			h.sampleInFlight()
			h.record(delivered{windowBackward, point.Slot})
			return nil
		}),
		WithAwaitReplyFunc(func(CallbackContext) error {
			h.atTip.Add(1)
			return nil
		}),
	)
	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Muxer:        clientMuxer,
			ErrorChan:    errs,
			Mode:         protocol.ProtocolModeNodeToClient,
		},
		&cfg,
	)
	h.client = client

	server.Start()
	client.Start()
	serverMuxer.Start()
	clientMuxer.Start()
	t.Cleanup(func() {
		close(h.stop)
		_ = client.Stop()
		server.Stop()
		_ = clientConn.Close()
		_ = serverConn.Close()
		clientMuxer.Stop()
		serverMuxer.Stop()
	})
	return h
}

func (h *windowHarness) inFlight() int {
	h.client.lifecycleMutex.Lock()
	defer h.client.lifecycleMutex.Unlock()
	return h.client.outstandingRequestNext
}

// sampleInFlight records the unanswered RequestNext count seen from inside a
// reply callback. The reply being handled is still counted, so a value of one
// means the pipe is empty behind it.
func (h *windowHarness) sampleInFlight() {
	n := h.inFlight()
	h.mu.Lock()
	if len(h.events) == 0 {
		// Only the initial RequestNext is out before the first reply.
		h.mu.Unlock()
		return
	}
	h.minInFlig = min(h.minInFlig, n)
	h.maxInFlig = max(h.maxInFlig, n)
	h.mu.Unlock()
}

func (h *windowHarness) record(d delivered) {
	h.mu.Lock()
	h.events = append(h.events, d)
	h.mu.Unlock()
}

func (h *windowHarness) delivered() []delivered {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]delivered(nil), h.events...)
}

func (h *windowHarness) waitDelivered(t testing.TB, n int) {
	t.Helper()
	require.Eventually(t, func() bool {
		return len(h.delivered()) >= n
	}, 20*time.Second, time.Millisecond, "deliveries did not reach %d", n)
}

func forwardScript(from, to uint64) []windowEvent {
	var s []windowEvent
	for n := from; n <= to; n++ {
		s = append(s, windowEvent{windowForward, n})
	}
	return s
}

// TestSyncKeepsWindowFullAtLatency checks that the pipe never runs empty
// while the server still has blocks: each consumed reply is replaced by a new
// RequestNext. A batch-and-wait client drops to one unanswered request (the
// reply being handled) at the end of every batch.
func TestSyncKeepsWindowFullAtLatency(t *testing.T) {
	t.Parallel()
	const (
		limit = 10
		total = 200
	)
	h := newWindowHarness(
		t, limit, 20*time.Millisecond, forwardScript(1, total), nil,
	)
	require.NoError(t, h.client.Sync([]pcommon.Point{pcommon.NewPointOrigin()}))
	h.waitDelivered(t, total)

	h.mu.Lock()
	defer h.mu.Unlock()
	require.GreaterOrEqual(
		t, h.minInFlig, limit/2,
		"unanswered RequestNext count fell to %d mid-sync (window %d)",
		h.minInFlig, limit,
	)
	require.LessOrEqual(t, h.maxInFlig, limit)
}

func (h *windowHarness) requireNoProtocolError(t testing.TB) {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	require.Empty(t, h.protoErrs)
}

func requireConsecutiveForwards(t testing.TB, got []delivered, first uint64) {
	t.Helper()
	for i, d := range got {
		require.Equal(
			t, delivered{windowForward, first + uint64(i)}, d,
			"delivery %d out of sequence", i,
		)
	}
}

// TestSyncRollbackWithWindowOutstanding checks that a RollBackward arriving
// while further RequestNext messages are outstanding is delivered in order
// between the roll forwards around it.
func TestSyncRollbackWithWindowOutstanding(t *testing.T) {
	t.Parallel()
	const limit = 10
	script := forwardScript(1, 8)
	script = append(script, windowEvent{windowBackward, 5})
	script = append(script, forwardScript(106, 125)...)
	h := newWindowHarness(t, limit, 10*time.Millisecond, script, nil)
	require.NoError(t, h.client.Sync([]pcommon.Point{pcommon.NewPointOrigin()}))
	h.waitDelivered(t, len(script))

	want := make([]delivered, 0, len(script))
	for _, ev := range script {
		want = append(want, delivered{ev.kind, ev.n})
	}
	require.Equal(t, want, h.delivered())
	h.requireNoProtocolError(t)
}

// TestSyncAtTipKeepsWindowBounded checks the window at the chain tip: the
// server answers one request with AwaitReply and holds the rest, the client
// keeps exactly PipelineLimit requests outstanding without adding more, and a
// block that arrives later is delivered and replaced by one new request.
func TestSyncAtTipKeepsWindowBounded(t *testing.T) {
	t.Parallel()
	const limit = 10
	h := newWindowHarness(
		t, limit, 10*time.Millisecond, forwardScript(1, 30), nil,
	)
	require.NoError(t, h.client.Sync([]pcommon.Point{pcommon.NewPointOrigin()}))
	h.waitDelivered(t, 30)
	require.Eventually(t, func() bool {
		return h.atTip.Load() == 1 && h.inFlight() == limit
	}, 5*time.Second, time.Millisecond, "window did not settle at tip")
	require.Never(t, func() bool {
		return h.inFlight() != limit || h.atTip.Load() != 1
	}, 200*time.Millisecond, time.Millisecond)

	require.NoError(
		t,
		h.server.RollForward(0, windowBlock(t, 31), windowTip(31)),
	)
	h.waitDelivered(t, 31)
	require.Eventually(t, func() bool {
		return h.atTip.Load() == 2 && h.inFlight() == limit
	}, 5*time.Second, time.Millisecond, "window did not refill after tip block")
	require.Never(t, func() bool {
		return h.inFlight() > limit
	}, 200*time.Millisecond, time.Millisecond)
	requireConsecutiveForwards(t, h.delivered(), 1)
	h.requireNoProtocolError(t)
}

// TestStopDuringWindowDeliversNothingAfterReturn checks that Stop with a full
// window in flight returns with no callback running afterwards and that no
// block was delivered twice or skipped.
func TestStopDuringWindowDeliversNothingAfterReturn(t *testing.T) {
	t.Parallel()
	h := newWindowHarness(
		t, 20, 10*time.Millisecond, forwardScript(1, 100000), nil,
	)
	require.NoError(t, h.client.Sync([]pcommon.Point{pcommon.NewPointOrigin()}))
	h.waitDelivered(t, 100)
	_ = h.client.Stop()
	after := len(h.delivered())
	require.Never(t, func() bool {
		return len(h.delivered()) != after
	}, 200*time.Millisecond, time.Millisecond)
	requireConsecutiveForwards(t, h.delivered(), 1)
}

// TestReintersectDrainsFullWindow checks that Reintersect called with a full
// window of requests in flight drains every reply exactly once, then resumes
// with a refilled window.
func TestReintersectDrainsFullWindow(t *testing.T) {
	t.Parallel()
	const (
		limit = 10
		total = 200
	)
	h := newWindowHarness(
		t, limit, 10*time.Millisecond, forwardScript(1, total), nil,
	)
	require.NoError(t, h.client.Sync([]pcommon.Point{pcommon.NewPointOrigin()}))
	h.waitDelivered(t, 30)
	_, _, err := h.client.Reintersect([]pcommon.Point{pcommon.NewPointOrigin()})
	require.NoError(t, err)
	h.waitDelivered(t, total)
	requireConsecutiveForwards(t, h.delivered(), 1)
	require.Eventually(t, func() bool {
		return h.inFlight() == limit
	}, 5*time.Second, time.Millisecond)
	h.requireNoProtocolError(t)
}

// TestSyncBlockedConsumerCapsWindow checks backpressure: with the callback
// blocked, the replies the server has sent stay within PipelineLimit of the
// block being handled, because a new request is issued only after a reply has
// been consumed.
func TestSyncBlockedConsumerCapsWindow(t *testing.T) {
	t.Parallel()
	const (
		limit     = 10
		blockedAt = 3
	)
	reached := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseFn := func() { releaseOnce.Do(func() { close(release) }) }
	h := newWindowHarness(
		t, limit, 10*time.Millisecond, forwardScript(1, 1000),
		func(n uint64) error {
			if n == blockedAt {
				close(reached)
				<-release
			}
			return nil
		},
	)
	// Runs before the harness cleanup so a failing test cannot leave the
	// callback blocking Stop.
	t.Cleanup(releaseFn)
	require.NoError(t, h.client.Sync([]pcommon.Point{pcommon.NewPointOrigin()}))
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("consumer never reached the blocking block")
	}
	// Requests sent: the initial one, a full window after reply 1, and one
	// for each further consumed reply before the blocked one.
	const wantServed = 1 + limit + (blockedAt - 2)
	require.Eventually(t, func() bool {
		return h.served.Load() == wantServed
	}, 5*time.Second, time.Millisecond,
		"served %d replies, want the window to fill to %d",
		h.served.Load(), wantServed)
	require.Never(t, func() bool {
		return h.served.Load() > wantServed || h.inFlight() > limit
	}, 300*time.Millisecond, time.Millisecond)
	releaseFn()
	h.waitDelivered(t, 50)
	h.requireNoProtocolError(t)
}

// BenchmarkSyncThroughput reports headers per second against a link with a
// round-trip time and a consumer that spends perHeader on each reply.
func BenchmarkSyncThroughput(b *testing.B) {
	const (
		limit     = 75
		rtt       = 180 * time.Millisecond
		perHeader = 700 * time.Microsecond
	)
	total := 1500
	for range b.N {
		h := newWindowHarness(
			b, limit, rtt, forwardScript(1, uint64(total)),
			func(uint64) error {
				time.Sleep(perHeader)
				return nil
			},
		)
		start := time.Now()
		if err := h.client.Sync(
			[]pcommon.Point{pcommon.NewPointOrigin()},
		); err != nil {
			b.Fatal(err)
		}
		h.waitDelivered(b, total)
		b.ReportMetric(float64(total)/time.Since(start).Seconds(), "headers/s")
	}
}
