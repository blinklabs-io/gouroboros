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

// A loop that has taken a ready signal but not yet busyMutex when Reintersect
// stops it must still exit; Reintersect must not hold busyMutex while waiting.
func TestReintersectLoopBetweenSignalAndBusyLock(t *testing.T) {
	t.Parallel()
	h := newReintersectHarness(t)
	var armed atomic.Bool
	arrived := make(chan struct{}, 1)
	proceed := make(chan struct{})
	h.client.testSyncLoopBeforeBusyLock = func() {
		if armed.CompareAndSwap(true, false) {
			arrived <- struct{}{}
			<-proceed
		}
	}
	h.startPipelined()

	armed.Store(true)
	h.release(1)
	require.Equal(t, uint64(2), recv(t, h.rollbacks, "rollback 2"))
	recv(t, arrived, "loop parked before busyMutex")

	done := h.reintersectAsync([]pcommon.Point{h.findPoint})
	// Let Reintersect close the stop channel and take busyMutex.
	time.Sleep(200 * time.Millisecond)
	close(proceed)
	h.release(2)
	recv(t, h.finds, "FindIntersect after drain")
	res := recv(t, done, "Reintersect result")
	require.NoError(t, res.err)
}

// Stop abandons the pipelined requests of the stopped protocol instance. A
// restarted client must not wait for their replies.
func TestReintersectAfterStopStartDoesNotWaitForAbandonedRequests(
	t *testing.T,
) {
	t.Parallel()
	h := newReintersectHarness(t)
	h.startPipelined()

	// MsgDone cannot be delivered while requests are outstanding.
	_ = h.client.Stop()
	h.client.Start()
	require.Equal(t, 0, h.outstanding(),
		"requests of the stopped protocol instance still counted")
}
