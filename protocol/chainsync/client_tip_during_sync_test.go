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
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// tipDuringSyncHarness connects a Client to a Server that rolls forward
// through blocks 1..blocks, then answers every further MsgRequestNext with
// MsgAwaitReply and no block, as a node-to-client server at its tip does
// until its chain grows. Each roll carries the server's tip, block blocks.
type tipDuringSyncHarness struct {
	client *Client
	// tipQueries counts MsgFindIntersect messages with no points, which only
	// GetCurrentTip sends.
	tipQueries atomic.Int32
	atTip      atomic.Int32
	delivered  atomic.Uint64

	mu        sync.Mutex
	protoErrs []error
}

func newTipDuringSyncHarness(
	t *testing.T,
	limit int,
	blocks uint64,
) *tipDuringSyncHarness {
	t.Helper()
	h := &tipDuringSyncHarness{}
	clientConn, serverConn := net.Pipe()
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)
	errs := make(chan error, 8)
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case err := <-errs:
				h.mu.Lock()
				h.protoErrs = append(h.protoErrs, err)
				h.mu.Unlock()
			case <-stop:
				return
			}
		}
	}()
	serverTip := windowTip(blocks)
	var served atomic.Uint64
	server := NewServer(protocol.ProtocolOptions{
		Muxer:     serverMuxer,
		ErrorChan: errs,
		Mode:      protocol.ProtocolModeNodeToClient,
	}, &Config{
		FindIntersectFunc: func(
			_ CallbackContext, points []pcommon.Point,
		) (pcommon.Point, Tip, error) {
			if len(points) == 0 {
				h.tipQueries.Add(1)
				return pcommon.Point{}, serverTip, ErrIntersectNotFound
			}
			return points[0], serverTip, nil
		},
		RequestNextFunc: func(ctx CallbackContext) error {
			n := served.Add(1)
			if n > blocks {
				h.atTip.Add(1)
				return ctx.Server.AwaitReply()
			}
			return ctx.Server.RollForward(0, windowBlock(t, n), serverTip)
		},
	})
	cfg := NewConfig(
		WithPipelineLimit(limit),
		WithRollForwardRawFunc(func(
			_ CallbackContext, _ uint, data []byte, _ Tip,
		) error {
			h.delivered.Add(1)
			return nil
		}),
	)
	h.client = NewClient(protocol.ProtocolOptions{
		ConnectionId: testConnectionId(),
		Muxer:        clientMuxer,
		ErrorChan:    errs,
		Mode:         protocol.ProtocolModeNodeToClient,
	}, &cfg)
	server.Start()
	h.client.Start()
	serverMuxer.Start()
	clientMuxer.Start()
	t.Cleanup(func() {
		_ = h.client.Stop(context.Background())
		_ = server.Stop(context.Background())
		_ = clientConn.Close()
		_ = serverConn.Close()
		_ = clientMuxer.Stop(context.Background())
		_ = serverMuxer.Stop(context.Background())
		close(stop)
	})
	return h
}

func (h *tipDuringSyncHarness) requireNoProtocolError(t *testing.T) {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	require.Empty(t, h.protoErrs)
}

// getCurrentTipWithin calls GetCurrentTip from a goroutine that is not a
// callback and fails the test if it has not returned within d.
func getCurrentTipWithin(t *testing.T, c *Client, d time.Duration) Tip {
	t.Helper()
	type result struct {
		tip *Tip
		err error
	}
	done := make(chan result, 1)
	go func() {
		tip, err := c.GetCurrentTip()
		done <- result{tip, err}
	}()
	select {
	case res := <-done:
		require.NoError(t, res.err)
		require.NotNil(t, res.tip)
		return *res.tip
	case <-time.After(d):
		t.Fatalf("GetCurrentTip did not return within %s during a sync", d)
		return Tip{}
	}
}

// TestGetCurrentTipDuringSyncAtTip calls GetCurrentTip while a sync has
// reached the server's tip, so its MsgRequestNext is answered with
// MsgAwaitReply and nothing more until the chain grows. MsgFindIntersect
// is legal only once that request is answered, so a GetCurrentTip that
// sends one waits for the next block, for ever on a chain that does not
// grow. The server's tip came with every roll, so GetCurrentTip returns it.
func TestGetCurrentTipDuringSyncAtTip(t *testing.T) {
	t.Parallel()
	const blocks = 30
	for _, limit := range []int{1, 10} {
		t.Run(windowName(limit), func(t *testing.T) {
			t.Parallel()
			h := newTipDuringSyncHarness(t, limit, blocks)
			require.NoError(t, h.client.Sync(
				[]pcommon.Point{pcommon.NewPointOrigin()},
			))
			require.Eventually(t, func() bool {
				return h.atTip.Load() > 0 && h.delivered.Load() == blocks
			}, 10*time.Second, time.Millisecond, "sync did not reach the tip")

			tip := getCurrentTipWithin(t, h.client, 5*time.Second)
			require.Equal(t, windowTip(blocks), tip)
			require.Zero(t, h.tipQueries.Load(),
				"GetCurrentTip sent MsgFindIntersect during the sync")
			h.requireNoProtocolError(t)
		})
	}
}

// TestGetCurrentTipDuringStreamingSync calls GetCurrentTip while a sync is
// still streaming blocks. It must return, and the sync must carry on to the
// tip afterwards.
func TestGetCurrentTipDuringStreamingSync(t *testing.T) {
	t.Parallel()
	const blocks = 3000
	for _, limit := range []int{1, 10} {
		t.Run(windowName(limit), func(t *testing.T) {
			t.Parallel()
			h := newTipDuringSyncHarness(t, limit, blocks)
			require.NoError(t, h.client.Sync(
				[]pcommon.Point{pcommon.NewPointOrigin()},
			))
			require.Eventually(t, func() bool {
				return h.delivered.Load() >= 20
			}, 10*time.Second, time.Millisecond, "sync did not start")

			tip := getCurrentTipWithin(t, h.client, 5*time.Second)
			require.Equal(t, windowTip(blocks), tip)
			require.Eventually(t, func() bool {
				return h.delivered.Load() == blocks
			}, 20*time.Second, time.Millisecond,
				"sync did not reach the tip after GetCurrentTip")
			h.requireNoProtocolError(t)
		})
	}
}

// TestGetCurrentTipRechecksSyncHoldingBusyMutex finds no MsgRequestNext
// unanswered when GetCurrentTip is called, then lets the sync loop send one,
// at the tip, before the worker takes busyMutex. The worker must check again
// and return the last reply's tip: a MsgFindIntersect queued behind that
// request would wait until the chain grows.
func TestGetCurrentTipRechecksSyncHoldingBusyMutex(t *testing.T) {
	t.Parallel()
	h := newTipDuringSyncHarness(t, 1, 1)
	loopWaiting := make(chan struct{})
	releaseLoop := make(chan struct{})
	loopSent := make(chan struct{})
	var waitOnce, sentOnce sync.Once
	h.client.testSyncLoopBeforeBusyLock = func() {
		waitOnce.Do(func() {
			close(loopWaiting)
			<-releaseLoop
		})
	}
	h.client.testSyncLoopAfterRequestNext = func() {
		sentOnce.Do(func() { close(loopSent) })
	}
	require.NoError(t, h.client.Sync(
		[]pcommon.Point{pcommon.NewPointOrigin()},
	))
	select {
	case <-loopWaiting:
	case <-time.After(5 * time.Second):
		t.Fatal("sync loop did not take the first roll's ready signal")
	}
	// The roll is retired after its ready signal is queued.
	require.Eventually(t, func() bool {
		h.client.lifecycleMutex.Lock()
		defer h.client.lifecycleMutex.Unlock()
		return h.client.outstandingRequestNext == 0
	}, 5*time.Second, time.Millisecond, "the first roll was not retired")

	h.client.testTipAfterSyncCheck = func() {
		close(releaseLoop)
		<-loopSent
	}
	tip := getCurrentTipWithin(t, h.client, 5*time.Second)
	require.Equal(t, windowTip(1), tip)
	require.Zero(t, h.tipQueries.Load(),
		"GetCurrentTip sent MsgFindIntersect behind the sync's request")
	h.requireNoProtocolError(t)
}

func windowName(limit int) string {
	if limit == 1 {
		return "window=1"
	}
	return "window=10"
}
