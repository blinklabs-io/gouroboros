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

package leiosfetch

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// TestRangeDeliveryEndsWhenCallerLeavesAfterLastReply covers a range whose
// last reply arrived while the caller still had replies to read. The last
// reply frees the slot and clears its waiter at once, so when the caller then
// leaves, through its context (abandon) or the protocol ending (release),
// neither matches the waiter. The delivery goroutine must still end rather
// than block forever on the full one-slot channel, holding the queued
// replies.
func TestRangeDeliveryEndsWhenCallerLeavesAfterLastReply(t *testing.T) {
	for _, tc := range []struct {
		name  string
		leave func(*requestSlot, chan protocol.Message)
	}{
		{"abandon", (*requestSlot).abandon},
		{"release", (*requestSlot).release},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
			slot := &requestSlot{}
			w, err := slot.acquireWithRangeLimits(
				context.Background(),
				make(chan struct{}),
				testRangeRequest(),
				DefaultMaxBlockRangeResponses,
				DefaultMaxBlockRangeResponseBytes,
			)
			require.NoError(t, err)
			slot.mu.Lock()
			d := slot.delivery
			slot.mu.Unlock()

			ebs := []testEB{
				testEndorserBlock(t, 1),
				testEndorserBlock(t, 2),
				testEndorserBlock(t, 3),
			}
			for i, eb := range ebs {
				var msg protocol.Message = NewMsgNextBlockAndTxsInRange(
					eb.raw, eb.txs,
				)
				last := i == len(ebs)-1
				if last {
					msg = NewMsgLastBlockAndTxsInRange(eb.raw, eb.txs)
				}
				delivered, err := slot.deliverRange(msg, last)
				require.NoError(t, err)
				require.True(t, delivered)
			}
			// The caller reads the first reply; the second then fills the
			// channel and the last waits behind it.
			<-w

			tc.leave(slot, w)

			select {
			case <-d.stopped:
			default:
				t.Fatal("delivery still running after the caller left")
			}
		})
	}
}
