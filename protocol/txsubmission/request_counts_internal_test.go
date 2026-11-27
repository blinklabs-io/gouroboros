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
	"math"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// TestServerRequestCountsAtWireRange checks Server.RequestTxIds at and past
// the uint16 range of the request field. Every count is refused before it
// is converted to uint16 or sent, and the refusal leaves no request pending,
// so the next valid request goes out. The server has no running protocol, so
// a count that is not refused waits for a reply that never comes; the call
// is bounded so that shows as a failure rather than a hang.
func TestServerRequestCountsAtWireRange(t *testing.T) {
	for _, tc := range []struct {
		name     string
		reqCount int
	}{
		{"negative", -1},
		{"over the window", MaxUnackedTxIds + 1},
		{"top of the wire range", math.MaxUint16},
		{"past the wire range", math.MaxUint16 + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer(protocol.ProtocolOptions{}, &Config{})
			refused := make(chan error, 1)
			go func() {
				_, err := s.RequestTxIds(true, tc.reqCount)
				refused <- err
			}()
			select {
			case err := <-refused:
				require.ErrorIs(
					t,
					err,
					protocol.ErrProtocolViolationRequestExceeded,
				)
			case <-time.After(time.Second):
				t.Fatalf("RequestTxIds(%d) was not refused", tc.reqCount)
			}
			s.stateMu.Lock()
			defer s.stateMu.Unlock()
			require.Nil(t, s.pendingTxIds)
			require.Zero(t, s.ackCount)
		})
	}
}
