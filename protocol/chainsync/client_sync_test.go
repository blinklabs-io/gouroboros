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
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

func (h *reintersectHarness) syncAsync(points []pcommon.Point) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- h.client.Sync(points) }()
	return ch
}

// requireNoFind fails if the server sees a FindIntersect or a protocol error
// is raised during the window. Asserting that nothing arrives only gets safer
// under load.
func (h *reintersectHarness) requireNoFind(window time.Duration) {
	h.t.Helper()
	select {
	case points := <-h.finds:
		h.t.Fatalf("FindIntersect %v sent by a refused Sync", points)
	case err := <-h.errs:
		h.t.Fatalf("protocol error after a refused Sync: %v", err)
	case <-time.After(window):
	}
}

// The loop exits on the first stop signal, so the later replies' stop signals
// outlive it. A Sync called directly afterwards must not let its new loop take
// one and exit.
func TestSyncDiscardsStopSignalsOfStoppedSync(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()
	h.stopSync.Store(true)
	h.finishStoppedSync()
	h.stopSync.Store(false)

	require.NoError(t, h.client.Sync([]pcommon.Point{h.findPoint}))
	recv(t, h.finds, "Sync FindIntersect")
	require.Equal(t, 5, recv(t, h.requests, "request after the stopped sync"))
	h.release(1)
	require.Equal(t, uint64(5), recv(t, h.rollbacks, "rollback 5"))
	require.Equal(t, 6, recv(t, h.requests,
		"pipelined request from a loop not ended by a stale stop signal"))
}

// Replies after the stopping one signal ready. A new loop taking one would
// send a MsgRequestNext with no reply to justify it, so the signals must be
// gone once Sync has started its loop.
func TestSyncDiscardsReadySignalsOfStoppedSync(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()
	h.stopAtSlot.Store(2)
	h.finishStoppedSync()

	require.NoError(t, h.client.Sync([]pcommon.Point{h.findPoint}))
	recv(t, h.finds, "Sync FindIntersect")
	require.Equal(t, 5, recv(t, h.requests, "request after the stopped sync"))
	select {
	case n := <-h.requests:
		t.Fatalf("request %d sent on a stale ready signal", n)
	case err := <-h.errs:
		t.Fatalf("unexpected protocol error: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	require.Equal(t, 1, h.outstanding(),
		"a stale ready signal made the new loop send a request no reply justified")
	h.release(1)
	require.Equal(t, uint64(5), recv(t, h.rollbacks, "rollback 5"))
	require.Equal(t, 6, recv(t, h.requests, "pipelined request"))
}

func TestSyncWhileSyncingReturnsErrSyncInProgress(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()

	err := recv(t, h.syncAsync([]pcommon.Point{h.findPoint}),
		"Sync to refuse instead of sending FindIntersect")
	require.ErrorIs(t, err, ErrSyncInProgress)
	h.requireNoFind(100 * time.Millisecond)

	// The running sync is untouched.
	h.release(1)
	require.Equal(t, uint64(2), recv(t, h.rollbacks, "rollback 2"))
}

// A loop still working through its ready signals has no request outstanding
// once the server has answered them all, yet a FindIntersect would move the
// server's read pointer under it.
func TestSyncWithIdleSyncLoopReturnsErrSyncInProgress(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	var armed atomic.Bool
	parked := make(chan struct{}, 1)
	proceed := make(chan struct{})
	h.client.testSyncLoopBeforeBusyLock = func() {
		if armed.CompareAndSwap(true, false) {
			parked <- struct{}{}
			<-proceed
		}
	}
	h.startPipelined()
	armed.Store(true)
	defer close(proceed)

	h.release(3)
	recv(t, parked, "loop parked on the first signal of the batch")
	require.Eventually(t, func() bool {
		return h.outstanding() == 0
	}, reintersectTestTimeout, time.Millisecond,
		"batch was never answered while the loop was parked")

	err := recv(t, h.syncAsync([]pcommon.Point{h.findPoint}),
		"Sync to refuse instead of sending FindIntersect")
	require.ErrorIs(t, err, ErrSyncInProgress)
	h.requireNoFind(100 * time.Millisecond)
}

// A loop stopped by a callback leaves its pipelined requests unanswered, so
// the protocol is not in StIdle even though no loop is running.
func TestSyncWithUnansweredRequestNextReturnsErrSyncInProgress(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()
	h.stopSync.Store(true)
	h.release(1)
	require.Equal(t, uint64(2), recv(t, h.rollbacks, "rollback 2"))
	h.client.lifecycleMutex.Lock()
	loopDone := h.client.syncLoopDone
	h.client.lifecycleMutex.Unlock()
	recv(t, loopDone, "sync loop exit")
	require.Positive(t, h.outstanding())

	err := recv(t, h.syncAsync([]pcommon.Point{h.findPoint}),
		"Sync to refuse instead of sending FindIntersect")
	require.ErrorIs(t, err, ErrSyncInProgress)
	h.requireNoFind(100 * time.Millisecond)
}

// A refused Sync leaves the client usable: once the stopped sync has drained,
// the same call succeeds.
func TestSyncSucceedsAfterRefusalOnceStoppedSyncDrained(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()
	h.stopSync.Store(true)
	h.release(1)
	require.Equal(t, uint64(2), recv(t, h.rollbacks, "rollback 2"))
	h.client.lifecycleMutex.Lock()
	loopDone := h.client.syncLoopDone
	h.client.lifecycleMutex.Unlock()
	recv(t, loopDone, "sync loop exit")

	require.ErrorIs(t,
		recv(t, h.syncAsync([]pcommon.Point{h.findPoint}),
			"Sync to refuse instead of sending FindIntersect"),
		ErrSyncInProgress)

	h.release(2)
	require.Equal(t, 3, recv(t, h.requests, "request 3"))
	require.Equal(t, 4, recv(t, h.requests, "request 4"))
	recv(t, h.rollbacks, "drained rollback 3")
	recv(t, h.rollbacks, "drained rollback 4")
	require.Eventually(t, func() bool {
		return h.outstanding() == 0
	}, reintersectTestTimeout, time.Millisecond,
		"stopped sync never retired its replies")
	h.stopSync.Store(false)

	require.NoError(t, h.client.Sync([]pcommon.Point{h.findPoint}))
	recv(t, h.finds, "Sync FindIntersect")
	require.Equal(t, 5, recv(t, h.requests, "request after the stopped sync"))
}

func TestGetAvailableBlockRangeWhileSyncingReturnsErrSyncInProgress(
	t *testing.T,
) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()

	type rangeResult struct{ err error }
	ch := make(chan rangeResult, 1)
	go func() {
		_, _, err := h.client.GetAvailableBlockRange(
			[]pcommon.Point{h.findPoint},
		)
		ch <- rangeResult{err}
	}()
	res := recv(t, ch, "GetAvailableBlockRange to refuse a running sync")
	require.ErrorIs(t, res.err, ErrSyncInProgress)
	h.requireNoFind(100 * time.Millisecond)
}

// A stop signal left by a stopped sync must not end a later range query
// before the server has answered its request.
func TestGetAvailableBlockRangeDiscardsStopSignalsOfStoppedSync(
	t *testing.T,
) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()
	h.stopSync.Store(true)
	h.finishStoppedSync()
	h.stopSync.Store(false)

	type rangeResult struct{ err error }
	ch := make(chan rangeResult, 1)
	go func() {
		_, _, err := h.client.GetAvailableBlockRange(
			[]pcommon.Point{h.findPoint},
		)
		ch <- rangeResult{err}
	}()
	recv(t, h.finds, "GetAvailableBlockRange FindIntersect")
	require.Equal(t, 5, recv(t, h.requests, "range RequestNext"))
	select {
	case res := <-ch:
		t.Fatalf("range query returned before the server replied: %v", res.err)
	case <-time.After(100 * time.Millisecond):
	}
}

// newRangeTestClient connects a Client to a Server that answers every odd
// MsgRequestNext with a rollback to the intersection and every even one with
// a block, which is the exchange GetAvailableBlockRange expects.
func newRangeTestClient(t *testing.T) (*Client, []pcommon.Point) {
	t.Helper()
	intersect := pcommon.NewPoint(10, testPointHash(0x0b))
	block := ledger.BabbageBlock{BlockHeader: &ledger.BabbageBlockHeader{}}
	block.BlockHeader.Body.BlockNumber = 50
	block.BlockHeader.Body.Slot = 50
	block.BlockHeader.Body.VrfKey = make([]byte, 32)
	block.BlockHeader.Body.VrfResult = common.VrfResult{
		Output: make([]byte, 64),
		Proof:  make([]byte, 80),
	}
	block.BlockHeader.Body.OpCert.HotVkey = make([]byte, 32)
	block.BlockHeader.Body.OpCert.Signature = make([]byte, 64)
	block.BlockHeader.Signature = make([]byte, 448)
	blockCbor, err := cbor.Encode(block)
	require.NoError(t, err)
	tip := Tip{Point: pcommon.NewPoint(1000, testPointHash(0xff))}

	clientConn, serverConn := net.Pipe()
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		clientMuxer.Stop()
		serverMuxer.Stop()
	})
	errs := make(chan error, 8)
	var requests int
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
				return points[0], tip, nil
			},
			RequestNextFunc: func(ctx CallbackContext) error {
				requests++
				if requests%2 == 1 {
					return ctx.Server.RollBackward(intersect, tip)
				}
				return ctx.Server.RollForward(
					ledger.BlockTypeBabbage, blockCbor, tip,
				)
			},
		},
	)
	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Muxer:        clientMuxer,
			ErrorChan:    errs,
			Mode:         protocol.ProtocolModeNodeToClient,
		},
		&Config{PipelineLimit: 3, SkipBlockValidation: true},
	)
	server.Start()
	client.Start()
	serverMuxer.Start()
	clientMuxer.Start()
	t.Cleanup(func() { _ = client.Stop() })
	return client, []pcommon.Point{intersect}
}

// The block reply is handed to GetAvailableBlockRange before its request is
// retired. Returning on the block alone releases the client with a request
// still counted, so the next Sync or range query is refused.
func TestGetAvailableBlockRangeReturnsAfterItsRequestsAreRetired(
	t *testing.T,
) {
	t.Parallel()
	client, points := newRangeTestClient(t)
	var armed atomic.Bool
	parked := make(chan struct{}, 1)
	proceed := make(chan struct{})
	client.testRollForwardBeforeAnswered = func() {
		if armed.CompareAndSwap(true, false) {
			parked <- struct{}{}
			<-proceed
		}
	}
	armed.Store(true)

	type rangeResult struct{ err error }
	ch := make(chan rangeResult, 1)
	go func() {
		_, _, err := client.GetAvailableBlockRange(points)
		ch <- rangeResult{err}
	}()
	recv(t, parked, "block reply held before its request is retired")
	select {
	case res := <-ch:
		close(proceed)
		t.Fatalf(
			"GetAvailableBlockRange returned with its request unretired: %v",
			res.err,
		)
	case <-time.After(100 * time.Millisecond):
	}
	close(proceed)
	require.NoError(t, recv(t, ch, "GetAvailableBlockRange").err)

	_, _, err := client.GetAvailableBlockRange(points)
	require.NoError(t, err, "range query refused after the previous one")
}

// Reintersect is the way to resume a sync stopped by a callback while its
// pipelined replies are still owed: it lets them complete, then intersects
// and streams again.
func TestReintersectResumesSyncStoppedByCallback(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()
	h.stopSync.Store(true)
	h.release(1)
	require.Equal(t, uint64(2), recv(t, h.rollbacks, "rollback 2"))
	h.client.lifecycleMutex.Lock()
	loopDone := h.client.syncLoopDone
	h.client.lifecycleMutex.Unlock()
	recv(t, loopDone, "sync loop exit")
	h.stopSync.Store(false)

	done := h.reintersectAsync([]pcommon.Point{h.findPoint})
	h.requireQuiet(done, 100*time.Millisecond)
	h.release(2)
	require.Equal(t, 3, recv(t, h.requests, "request 3"))
	require.Equal(t, 4, recv(t, h.requests, "request 4"))
	require.Equal(t, uint64(3), recv(t, h.rollbacks, "drained rollback 3"))
	require.Equal(t, uint64(4), recv(t, h.rollbacks, "drained rollback 4"))
	recv(t, h.finds, "Reintersect FindIntersect after the drain")
	res := recv(t, done, "Reintersect")
	require.NoError(t, res.err)
	require.Equal(t, h.findPoint, res.point)
	require.Equal(t, 5, recv(t, h.requests, "request after resuming"))
	h.release(1)
	require.Equal(t, uint64(5), recv(t, h.rollbacks, "rollback 5"))
	require.Equal(t, 6, recv(t, h.requests, "pipelined request"))
}
