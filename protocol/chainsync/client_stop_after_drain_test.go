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
	"context"
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// doneSpyHandler reports every server-side "done" log record, which the
// server emits when it receives MsgDone.
type doneSpyHandler struct {
	slog.Handler
	done chan struct{}
}

func (h doneSpyHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h doneSpyHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Message == "done" {
		select {
		case h.done <- struct{}{}:
		default:
		}
	}
	return nil
}

func (h doneSpyHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h doneSpyHandler) WithGroup(string) slog.Handler      { return h }

// stopDrainHarness is a reintersectHarness whose server also reports MsgDone.
type stopDrainHarness struct {
	*reintersectHarness
	serverDone chan struct{}
}

func newStopDrainHarness(t *testing.T) *stopDrainHarness {
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
	sd := &stopDrainHarness{
		reintersectHarness: h,
		serverDone:         make(chan struct{}, 8),
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
			Logger: slog.New(
				doneSpyHandler{done: sd.serverDone},
			),
		},
		&Config{
			FindIntersectFunc: func(
				_ CallbackContext,
				points []pcommon.Point,
			) (pcommon.Point, Tip, error) {
				h.log("find")
				h.finds <- points
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
				return nil
			},
		},
	)
	server.Start()
	h.client.Start()
	serverMuxer.Start()
	clientMuxer.Start()
	t.Cleanup(func() {
		close(h.stop)
		_ = h.client.Stop()
	})
	return sd
}

func (h *stopDrainHarness) stopAfterDrainAsync(
	ctx context.Context,
) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- h.client.StopAfterDrain(ctx) }()
	return ch
}

// requireQuietStop fails if MsgDone reaches the server, a protocol error is
// raised, or the stop returns during the window.
func (h *stopDrainHarness) requireQuietStop(
	done <-chan error,
	window time.Duration,
) {
	h.t.Helper()
	select {
	case <-h.serverDone:
		h.t.Fatal("MsgDone sent while replies were outstanding")
	case err := <-h.errs:
		h.t.Fatalf("protocol error while replies were outstanding: %v", err)
	case err := <-done:
		h.t.Fatalf("StopAfterDrain returned early: %v", err)
	case <-time.After(window):
	}
}

func TestStopAfterDrainSendsDoneAfterRepliesAndAllowsRestart(t *testing.T) {
	t.Parallel()
	h := newStopDrainHarness(t)
	h.startPipelined()

	done := h.stopAfterDrainAsync(context.Background())
	h.requireQuietStop(done, 300*time.Millisecond)

	h.release(3)
	for want := uint64(2); want <= 4; want++ {
		require.Equal(t, want, recv(t, h.rollbacks, "drained rollback"))
	}
	recv(t, h.serverDone, "MsgDone at server")
	require.NoError(t, recv(t, done, "StopAfterDrain result"))
	events := h.snapshot()
	require.Contains(t, events, "rb:4")
	require.NotContains(t, events, "req:5", "RequestNext sent while stopping")

	// Restart on the same muxer: FindIntersect is legal only if the server
	// received MsgDone and returned to StIdle.
	h.client.Start()
	require.NoError(t, h.client.Sync([]pcommon.Point{h.findPoint}))
	require.Equal(t, []pcommon.Point{h.findPoint}, recv(t, h.finds, "FindIntersect after restart"))
	require.Equal(t, 3, recv(t, h.requests, "request 3"))
	require.Equal(t, 4, recv(t, h.requests, "request 4"))
	require.Equal(t, 5, recv(t, h.requests, "request after restart"))
	select {
	case err := <-h.errs:
		t.Fatalf("unexpected protocol error: %v", err)
	default:
	}
}

func TestStopAfterDrainContextExpiry(t *testing.T) {
	t.Parallel()
	h := newStopDrainHarness(t)
	h.startPipelined()

	ctx, cancel := context.WithTimeout(
		context.Background(), 100*time.Millisecond,
	)
	defer cancel()
	err := recv(t, h.stopAfterDrainAsync(ctx), "StopAfterDrain result")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	select {
	case <-h.serverDone:
		t.Fatal("MsgDone sent after the context expired")
	case <-time.After(200 * time.Millisecond):
	}

	// The client keeps running with its requests still outstanding, and a
	// second call with a live context resumes the wait.
	require.Equal(t, 3, h.outstanding())
	done := h.stopAfterDrainAsync(context.Background())
	h.release(3)
	recv(t, h.serverDone, "MsgDone at server")
	require.NoError(t, recv(t, done, "second StopAfterDrain result"))
}

func TestStopAfterDrainConnectionCloseDuringDrain(t *testing.T) {
	t.Parallel()
	h := newStopDrainHarness(t)
	h.startPipelined()

	done := h.stopAfterDrainAsync(context.Background())
	h.requireQuietStop(done, 100*time.Millisecond)
	require.NoError(t, h.clientConn.Close())
	err := recv(t, done, "StopAfterDrain result after close")
	require.True(t,
		errors.Is(err, protocol.ErrProtocolShuttingDown),
		"got %v", err,
	)
}

func TestStopAfterDrainWhenNotRunning(t *testing.T) {
	t.Parallel()
	h := newStopDrainHarness(t)
	require.NoError(t, h.client.Stop())
	require.NoError(t, h.client.StopAfterDrain(context.Background()))
}
