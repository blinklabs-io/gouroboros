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
	"errors"
	"fmt"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

const reintersectTestTimeout = 5 * time.Second

// reintersectHarness connects a real Client to a real Server over a pipe. The
// server holds every RequestNext until the test releases it, so the test
// decides how many pipelined requests are outstanding.
type reintersectHarness struct {
	t          *testing.T
	client     *Client
	clientConn net.Conn
	gate       chan struct{}
	stop       chan struct{}
	errs       chan error
	requests   chan int
	finds      chan []pcommon.Point
	rollbacks  chan uint64
	findPoint  pcommon.Point
	notFound   atomic.Bool
	stopSync   atomic.Bool

	mu     sync.Mutex
	events []string
}

func (h *reintersectHarness) log(format string, args ...any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, fmt.Sprintf(format, args...))
}

func (h *reintersectHarness) snapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.events)
}

func (h *reintersectHarness) release(n int) {
	for range n {
		h.gate <- struct{}{}
	}
}

func (h *reintersectHarness) outstanding() int {
	h.client.lifecycleMutex.Lock()
	defer h.client.lifecycleMutex.Unlock()
	return h.client.outstandingRequestNext
}

func recv[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(reintersectTestTimeout):
		t.Fatalf("timed out waiting for %s", what)
	}
	panic("unreachable")
}

func newReintersectHarness(t *testing.T) *reintersectHarness {
	t.Helper()
	h := &reintersectHarness{
		t:         t,
		gate:      make(chan struct{}, 64),
		stop:      make(chan struct{}),
		errs:      make(chan error, 8),
		requests:  make(chan int, 64),
		finds:     make(chan []pcommon.Point, 8),
		rollbacks: make(chan uint64, 64),
		findPoint: pcommon.NewPoint(10, testPointHash(0x0b)),
	}
	clientConn, serverConn := net.Pipe()
	h.clientConn = clientConn
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		clientMuxer.Stop()
		serverMuxer.Stop()
	})
	tip := Tip{Point: pcommon.NewPoint(1000, testPointHash(0xff))}

	var reqCount int
	server := NewServer(
		protocol.ProtocolOptions{
			Muxer:     serverMuxer,
			ErrorChan: h.errs,
			Mode:      protocol.ProtocolModeNodeToClient,
		},
		&Config{
			FindIntersectFunc: func(
				_ CallbackContext,
				points []pcommon.Point,
			) (pcommon.Point, Tip, error) {
				h.log("find")
				h.finds <- points
				if h.notFound.Load() {
					return pcommon.Point{}, tip, ErrIntersectNotFound
				}
				return points[0], tip, nil
			},
			RequestNextFunc: func(ctx CallbackContext) error {
				reqCount++
				n := reqCount
				h.log("req:%d", n)
				h.requests <- n
				select {
				case <-h.gate:
				case <-h.stop:
					return nil
				}
				return ctx.Server.RollBackward(
					pcommon.NewPoint(uint64(n), testPointHash(byte(n))),
					tip,
				)
			},
		},
	)
	h.client = NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Muxer:        clientMuxer,
			ErrorChan:    h.errs,
			Mode:         protocol.ProtocolModeNodeToClient,
		},
		&Config{
			PipelineLimit: 3,
			RollBackwardFunc: func(
				_ CallbackContext,
				point pcommon.Point,
				_ Tip,
			) error {
				h.log("rb:%d", point.Slot)
				h.rollbacks <- point.Slot
				if h.stopSync.Load() {
					return ErrStopSyncProcess
				}
				return nil
			},
		},
	)
	server.Start()
	h.client.Start()
	serverMuxer.Start()
	clientMuxer.Start()
	// Registered last so it runs first and frees a handler held on the gate
	// before the teardown waits on it.
	t.Cleanup(func() {
		close(h.stop)
		_ = h.client.Stop()
	})
	return h
}

// startPipelined syncs and answers the first request, leaving the client with
// the PipelineLimit requests of its next batch outstanding.
func (h *reintersectHarness) startPipelined() {
	h.t.Helper()
	require.NoError(h.t, h.client.Sync([]pcommon.Point{
		pcommon.NewPoint(5, testPointHash(0x05)),
	}))
	recv(h.t, h.finds, "initial FindIntersect")
	require.Equal(h.t, 1, recv(h.t, h.requests, "request 1"))
	h.release(1)
	require.Equal(h.t, uint64(1), recv(h.t, h.rollbacks, "rollback 1"))
	require.Equal(h.t, 2, recv(h.t, h.requests, "request 2"))
	require.Eventually(h.t, func() bool {
		return h.outstanding() == 3
	}, reintersectTestTimeout, time.Millisecond,
		"client never pipelined its request batch")
}

type reintersectResult struct {
	point pcommon.Point
	tip   Tip
	err   error
}

func (h *reintersectHarness) reintersectAsync(
	points []pcommon.Point,
) <-chan reintersectResult {
	ch := make(chan reintersectResult, 1)
	go func() {
		point, tip, err := h.client.Reintersect(points)
		ch <- reintersectResult{point, tip, err}
	}()
	return ch
}

// requireQuiet fails if the server sees a FindIntersect, a protocol error is
// raised, or the Reintersect call returns during the window. Asserting that
// nothing arrives only gets safer under load.
func (h *reintersectHarness) requireQuiet(
	done <-chan reintersectResult,
	window time.Duration,
) {
	h.t.Helper()
	select {
	case points := <-h.finds:
		h.t.Fatalf(
			"FindIntersect %v sent while replies were outstanding",
			points,
		)
	case err := <-h.errs:
		h.t.Fatalf("protocol error while replies were outstanding: %v", err)
	case res := <-done:
		h.t.Fatalf("Reintersect returned early: %+v", res)
	case <-time.After(window):
	}
}

func TestReintersectDrainsPipelinedRepliesBeforeFindIntersect(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()

	done := h.reintersectAsync([]pcommon.Point{h.findPoint})
	h.requireQuiet(done, 300*time.Millisecond)

	// Replies 2..4 are delivered to the callbacks, in order.
	h.release(3)
	for want := uint64(2); want <= 4; want++ {
		require.Equal(t, want, recv(t, h.rollbacks, "drained rollback"))
	}
	points := recv(t, h.finds, "FindIntersect")
	require.Equal(t, []pcommon.Point{h.findPoint}, points)
	res := recv(t, done, "Reintersect result")
	require.NoError(t, res.err)
	require.Equal(t, h.findPoint, res.point)

	events := h.snapshot()
	findIdx := slices.Index(events[1:], "find") + 1
	require.Positive(t, findIdx)
	require.Contains(t, events[:findIdx], "rb:4")
	// No RequestNext beyond the pipelined four precedes FindIntersect.
	require.NotContains(t, events[:findIdx], "req:5")

	// Syncing resumes from the new intersection with a single request, not
	// the old loop's batch plus a new one.
	require.Equal(t, 3, recv(t, h.requests, "request 3"))
	require.Equal(t, 4, recv(t, h.requests, "request 4"))
	require.Equal(t, 5, recv(t, h.requests, "request after intersect"))
	require.Equal(t, 1, h.outstanding(),
		"more RequestNext outstanding than the one resumed request")
	h.release(1)
	require.Equal(t, uint64(5), recv(t, h.rollbacks, "rollback 5"))
	require.Equal(t, 6, recv(t, h.requests, "pipelined request"))
	require.Eventually(t, func() bool {
		return h.outstanding() == 3
	}, reintersectTestTimeout, time.Millisecond)
	select {
	case err := <-h.errs:
		t.Fatalf("unexpected protocol error: %v", err)
	default:
	}
}

func TestReintersectStopDuringDrain(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()

	done := h.reintersectAsync([]pcommon.Point{h.findPoint})
	h.requireQuiet(done, 100*time.Millisecond)

	stopped := make(chan error, 1)
	go func() { stopped <- h.client.Stop() }()
	res := recv(t, done, "Reintersect result after Stop")
	require.ErrorIs(t, res.err, protocol.ErrProtocolShuttingDown)
	recv(t, stopped, "Stop")
	select {
	case points := <-h.finds:
		t.Fatalf("FindIntersect %v sent after Stop", points)
	default:
	}
}

func TestReintersectConnectionCloseDuringDrain(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()

	done := h.reintersectAsync([]pcommon.Point{h.findPoint})
	h.requireQuiet(done, 100*time.Millisecond)

	require.NoError(t, h.clientConn.Close())
	res := recv(t, done, "Reintersect result after close")
	require.Error(t, res.err)
	require.True(t,
		errors.Is(res.err, protocol.ErrProtocolShuttingDown),
		"got %v", res.err,
	)
}

func TestReintersectIntersectNotFoundLeavesProtocolIdle(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()

	h.notFound.Store(true)
	done := h.reintersectAsync([]pcommon.Point{h.findPoint})
	h.release(3)
	recv(t, h.finds, "FindIntersect")
	res := recv(t, done, "Reintersect result")
	require.ErrorIs(t, res.err, ErrIntersectNotFound)
	require.Equal(t, 0, h.outstanding())

	// The protocol is idle and no loop is running, so a retry succeeds.
	h.notFound.Store(false)
	point, _, err := h.client.Reintersect([]pcommon.Point{h.findPoint})
	require.NoError(t, err)
	require.Equal(t, h.findPoint, point)
}

func TestReintersectStopSyncCallbackDuringDrain(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()

	h.stopSync.Store(true)
	done := h.reintersectAsync([]pcommon.Point{h.findPoint})
	h.release(3)
	res := recv(t, done, "Reintersect result")
	require.ErrorIs(t, res.err, ErrStopSyncProcess)
	select {
	case points := <-h.finds:
		t.Fatalf(
			"FindIntersect %v sent after a callback stopped the sync",
			points,
		)
	default:
	}
}
