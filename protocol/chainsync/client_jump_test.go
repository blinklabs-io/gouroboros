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
	"bytes"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

type jumpResult struct {
	point pcommon.Point
	tip   Tip
	found bool
	err   error
}

func (h *reintersectHarness) jumpAsync(
	points []pcommon.Point,
) <-chan jumpResult {
	ch := make(chan jumpResult, 1)
	go func() {
		point, tip, found, err := h.client.Jump(points)
		ch <- jumpResult{point, tip, found, err}
	}()
	return ch
}

// requireNoStreaming fails if the server sees a RequestNext or a protocol
// error is raised during the window. Asserting that nothing arrives only gets
// safer under load.
func (h *reintersectHarness) requireNoStreaming(window time.Duration) {
	h.t.Helper()
	select {
	case n := <-h.requests:
		h.t.Fatalf("RequestNext %d sent by a client that only jumped", n)
	case err := <-h.errs:
		h.t.Fatalf("unexpected protocol error: %v", err)
	case <-time.After(window):
	}
	require.Equal(h.t, 0, h.outstanding(),
		"RequestNext outstanding on a client that only jumped")
}

func jumpTestTip() Tip {
	return Tip{Point: pcommon.NewPoint(1000, testPointHash(0xff))}
}

func TestJumpWithoutSyncFindsIntersectionsAndDoesNotStream(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	first := pcommon.NewPoint(20, testPointHash(0x14))
	second := pcommon.NewPoint(40, testPointHash(0x28))

	point, tip, found, err := h.client.Jump([]pcommon.Point{first})
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, first, point)
	require.Equal(t, jumpTestTip(), tip)
	require.Equal(t, []pcommon.Point{first}, recv(t, h.finds, "first jump"))

	point, tip, found, err = h.client.Jump(
		[]pcommon.Point{second, first},
	)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, second, point)
	require.Equal(t, jumpTestTip(), tip)
	require.Equal(t, []pcommon.Point{second, first},
		recv(t, h.finds, "second jump"))

	h.requireNoStreaming(300 * time.Millisecond)
}

func TestJumpIntersectNotFoundLeavesProtocolIdle(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.notFound.Store(true)

	point, tip, found, err := h.client.Jump([]pcommon.Point{h.findPoint})
	require.NoError(t, err, "an absent intersection is a result, not an error")
	require.False(t, found)
	require.Equal(t, pcommon.Point{}, point)
	require.Equal(t, jumpTestTip(), tip)
	recv(t, h.finds, "jump")

	// The protocol is back in StIdle, so another FindIntersect is legal.
	h.notFound.Store(false)
	point, _, found, err = h.client.Jump([]pcommon.Point{h.findPoint})
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, h.findPoint, point)
	recv(t, h.finds, "retried jump")
	h.requireNoStreaming(100 * time.Millisecond)
}

func TestJumpThenSyncStreamsFromGivenPoint(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	for _, slot := range []uint64{20, 40} {
		jumpPoint := pcommon.NewPoint(slot, testPointHash(byte(slot)))
		_, _, found, err := h.client.Jump([]pcommon.Point{jumpPoint})
		require.NoError(t, err)
		require.True(t, found)
		recv(t, h.finds, "jump")
	}

	syncPoint := pcommon.NewPoint(60, testPointHash(0x3c))
	require.NoError(t, h.client.Sync([]pcommon.Point{syncPoint}))
	require.Equal(t, []pcommon.Point{syncPoint},
		recv(t, h.finds, "Sync FindIntersect"))
	require.Equal(t, 1, recv(t, h.requests, "first RequestNext"))
	h.release(1)
	require.Equal(t, uint64(1), recv(t, h.rollbacks, "rollback 1"))
	require.Equal(t, 2, recv(t, h.requests, "pipelined RequestNext"))
}

func TestJumpWhileSyncingReturnsErrSyncInProgress(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()

	res := recv(t, h.jumpAsync([]pcommon.Point{h.findPoint}),
		"Jump to refuse instead of waiting behind the sync")
	require.ErrorIs(t, res.err, ErrSyncInProgress)
	require.False(t, res.found)
	select {
	case points := <-h.finds:
		t.Fatalf("FindIntersect %v sent while syncing", points)
	case err := <-h.errs:
		t.Fatalf("protocol error after a refused jump: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	// The running sync is untouched.
	h.release(1)
	require.Equal(t, uint64(2), recv(t, h.rollbacks, "rollback 2"))
}

// A loop still working through its ready signals has no request outstanding
// once the server has answered them all, yet a FindIntersect would move the
// server's read pointer under it.
func TestJumpWithIdleSyncLoopReturnsErrSyncInProgress(t *testing.T) {
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

	_, _, _, err := h.client.Jump([]pcommon.Point{h.findPoint})
	require.ErrorIs(t, err, ErrSyncInProgress)
}

// A loop stopped by a callback leaves its pipelined requests unanswered, so
// the protocol is not in StIdle even though no loop is running.
func TestJumpWithUnansweredRequestNextReturnsErrSyncInProgress(t *testing.T) {
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

	res := recv(t, h.jumpAsync([]pcommon.Point{h.findPoint}),
		"Jump to refuse instead of waiting behind unanswered RequestNext")
	require.ErrorIs(t, res.err, ErrSyncInProgress)
	select {
	case points := <-h.finds:
		t.Fatalf("FindIntersect %v sent with replies outstanding", points)
	case <-time.After(100 * time.Millisecond):
	}
}

// finishStoppedSync answers the pipelined batch of a sync whose callback has
// been told to stop, and returns once the loop has exited and every reply has
// been retired. Replies enqueue their ready signal before they are retired,
// so all the signals the old loop left behind are buffered by then.
func (h *reintersectHarness) finishStoppedSync() {
	h.t.Helper()
	h.client.lifecycleMutex.Lock()
	loopDone := h.client.syncLoopDone
	h.client.lifecycleMutex.Unlock()
	h.release(3)
	require.Equal(h.t, 3, recv(h.t, h.requests, "request 3"))
	require.Equal(h.t, 4, recv(h.t, h.requests, "request 4"))
	for want := uint64(2); want <= 4; want++ {
		require.Equal(h.t, want, recv(h.t, h.rollbacks, "drained rollback"))
	}
	recv(h.t, loopDone, "sync loop exit")
	require.Eventually(h.t, func() bool {
		return h.outstanding() == 0
	}, reintersectTestTimeout, time.Millisecond,
		"stopped sync never retired its replies")
}

// The loop exits on the first stop signal, so the later replies' stop signals
// outlive it. A new loop must not take one for its own and exit.
func TestJumpDiscardsStopSignalsOfStoppedSync(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()
	h.stopSync.Store(true)
	h.finishStoppedSync()
	h.stopSync.Store(false)

	_, _, found, err := h.client.Jump([]pcommon.Point{h.findPoint})
	require.NoError(t, err)
	require.True(t, found)
	recv(t, h.finds, "jump")

	require.NoError(t, h.client.Sync([]pcommon.Point{h.findPoint}))
	recv(t, h.finds, "Sync FindIntersect")
	require.Equal(t, 5, recv(t, h.requests, "request after jump"))
	h.release(1)
	require.Equal(t, uint64(5), recv(t, h.rollbacks, "rollback 5"))
	require.Equal(t, 6, recv(t, h.requests,
		"pipelined request from a loop not ended by a stale stop signal"))
}

// Replies after the stopping one signal ready. A new loop taking one would
// send a MsgRequestNext with no reply to justify it, so the signals must be
// gone when Jump returns.
func TestJumpDiscardsReadySignalsOfStoppedSync(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()
	h.stopAtSlot.Store(2)
	h.finishStoppedSync()

	_, _, found, err := h.client.Jump([]pcommon.Point{h.findPoint})
	require.NoError(t, err)
	require.True(t, found)
	recv(t, h.finds, "jump")

	h.client.lifecycleMutex.Lock()
	readyChan := h.client.readyForNextBlockChan
	h.client.lifecycleMutex.Unlock()
	require.Empty(t, readyChan, "stale ready signals survived Jump")

	require.NoError(t, h.client.Sync([]pcommon.Point{h.findPoint}))
	recv(t, h.finds, "Sync FindIntersect")
	require.Equal(t, 5, recv(t, h.requests, "request after jump"))
	h.release(1)
	require.Equal(t, uint64(5), recv(t, h.rollbacks, "rollback 5"))
	require.Equal(t, 6, recv(t, h.requests, "pipelined request"))
}

func TestJumpConnectionCloseDuringJump(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.holdFind.Store(true)

	done := h.jumpAsync([]pcommon.Point{h.findPoint})
	recv(t, h.finds, "held jump")
	require.NoError(t, h.clientConn.Close())
	res := recv(t, done, "Jump result after close")
	require.ErrorIs(t, res.err, protocol.ErrProtocolShuttingDown)
	require.False(t, res.found)
}

func TestJumpStopDuringJump(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	// The held jump owns busyMutex; do not make Stop wait its full default.
	h.client.testStopBusyLockTimeout = 50 * time.Millisecond
	h.holdFind.Store(true)

	done := h.jumpAsync([]pcommon.Point{h.findPoint})
	recv(t, h.finds, "held jump")
	stopped := make(chan error, 1)
	go func() { stopped <- h.client.Stop() }()
	res := recv(t, done, "Jump result after Stop")
	require.ErrorIs(t, res.err, protocol.ErrProtocolShuttingDown)
	recv(t, stopped, "Stop")
}

func TestJumpAfterStopReturnsErrProtocolShuttingDown(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	require.NoError(t, h.client.Stop())

	_, _, _, err := h.client.Jump([]pcommon.Point{h.findPoint})
	require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
}

// Concurrent jumps are serialized: each waits for the previous reply, so the
// server never sees a FindIntersect outside StIdle.
func TestJumpConcurrentCallsAreSerialized(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	const jumps = 6
	var wg sync.WaitGroup
	errs := make(chan error, jumps)
	for i := range jumps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := pcommon.NewPoint(uint64(i+1), testPointHash(byte(i+1)))
			got, _, found, err := h.client.Jump([]pcommon.Point{p})
			if err == nil && (!found || got.Slot != p.Slot ||
				!bytes.Equal(got.Hash, p.Hash)) {
				err = ErrIntersectNotFound
			}
			errs <- err
		}()
	}
	wg.Wait()
	for range jumps {
		require.NoError(t, <-errs)
		recv(t, h.finds, "jump")
	}
	h.requireNoStreaming(100 * time.Millisecond)
}
