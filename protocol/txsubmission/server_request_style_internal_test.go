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
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// TestServerSendsOnlyRequestsOfTheRightStyle checks that the server never
// puts on the wire a MsgRequestTxIds the reference implementation's
// outbound side refuses (Ouroboros.Network.TxSubmission.Outbound): a
// blocking request must ask for at least one ID and leave nothing
// unacknowledged, and a non-blocking one must leave something
// unacknowledged. The server acknowledges every ID it holds on each
// request, so through RequestTxIds alone a non-blocking request is never
// legal; the last two cases set the held IDs directly to show the check
// is not unconditional.
func TestServerSendsOnlyRequestsOfTheRightStyle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		announced int
		held      int
		ackCount  int
		blocking  bool
		req       int
		refused   bool
	}{
		{"non-blocking at the start", 0, 0, 0, false, 1, true},
		{"blocking for nothing", 0, 0, 0, true, 0, true},
		{"blocking at the start", 0, 0, 0, true, 1, false},
		{"non-blocking after a reply", 2, 0, 0, false, 1, true},
		{"blocking after a reply", 2, 0, 0, true, 1, false},
		{"non-blocking with an id left", 0, 2, 1, false, 1, false},
		{"blocking with an id left", 0, 2, 1, true, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			peer := newReplyValidationPeer(t)
			if tc.announced > 0 {
				peer.announceTxIds(t, smallTxIdAndSizes(tc.announced))
			}
			if tc.held > 0 {
				peer.server.stateMu.Lock()
				peer.server.outstandingTxIds = smallTxIdAndSizes(tc.held)
				peer.server.ackCount = tc.ackCount
				peer.server.stateMu.Unlock()
			}
			errChan := make(chan error, 1)
			go func() {
				_, err := peer.server.RequestTxIds(tc.blocking, tc.req)
				errChan <- err
			}()
			if !tc.refused {
				peer.waitForRequest(t)
				return
			}
			select {
			case err := <-errChan:
				require.ErrorIs(
					t,
					err,
					protocol.ErrProtocolViolationInvalidMessage,
				)
			case <-peer.outbound:
				t.Fatal("the server sent a request the reference refuses")
			case <-time.After(pendingBytesWindow):
				t.Fatal("RequestTxIds neither refused nor sent the request")
			}
		})
	}
}

// smallTxIdAndSizes returns n distinct announced IDs with a size well inside
// the limit.
func smallTxIdAndSizes(n int) []TxIdAndSize {
	txIds := windowTxIdAndSizes(n)
	for i := range txIds {
		txIds[i].Size = 100
	}
	return txIds
}
