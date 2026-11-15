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
	"sync/atomic"
	"testing"
	"time"

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
