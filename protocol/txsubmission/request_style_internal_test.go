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

package txsubmission

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// TestClientEnforcesRequestStyle checks the client against the rules the
// reference implementation's outbound side enforces on MsgRequestTxIds
// (Ouroboros.Network.TxSubmission.Outbound): a blocking request asks for at
// least one ID and leaves nothing unacknowledged; a non-blocking request
// acknowledges or asks for something and leaves something unacknowledged.
// Each refused case has an accepted neighbour that differs in one field.
func TestClientEnforcesRequestStyle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		outstanding int
		blocking    bool
		ack, req    uint16
		refused     bool
	}{
		{"blocking for nothing", 0, true, 0, 0, true},
		{"blocking for one", 0, true, 0, 1, false},
		{"blocking with an id left", 1, true, 0, 1, true},
		{"blocking acknowledging all", 1, true, 1, 1, false},
		{"non-blocking with nothing left", 0, false, 0, 1, true},
		{"non-blocking acknowledging all", 2, false, 2, 1, true},
		{"non-blocking with an id left", 2, false, 1, 1, false},
		{"non-blocking for nothing", 1, false, 0, 0, true},
		{"non-blocking acknowledging only", 2, false, 1, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var called atomic.Bool
			p := newRawRequestingPeer(t, &Config{
				RequestTxIdsFunc: func(
					_ CallbackContext,
					_ bool,
					_ uint16,
					req uint16,
				) ([]TxIdAndSize, error) {
					called.Store(true)
					return windowTxIdAndSizes(int(req))[:min(int(req), 1)], nil
				},
			})
			setClientOutstanding(
				p.client,
				windowTxIdAndSizes(MaxUnackedTxIds)[MaxUnackedTxIds-tc.outstanding:],
			)
			p.send(t, NewMsgRequestTxIds(tc.blocking, tc.ack, tc.req))
			select {
			case err := <-p.errorChan:
				require.True(
					t,
					tc.refused,
					"a request the reference accepts was refused: %v",
					err,
				)
				require.ErrorIs(
					t,
					err,
					protocol.ErrProtocolViolationInvalidMessage,
				)
			case <-time.After(pendingBytesWindow):
				require.False(
					t,
					tc.refused,
					"a request the reference refuses was accepted",
				)
			}
			require.Equal(
				t,
				!tc.refused,
				called.Load(),
				fmt.Sprintf("callback reached: %v", called.Load()),
			)
		})
	}
}
