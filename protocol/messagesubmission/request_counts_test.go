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

package messagesubmission

import (
	"math"
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// TestRequestCountsAtWireRange checks requests whose counts sit at the top
// of their uint16 wire range: each is refused as exceeding the window or the
// IDs outstanding, never accepted through overflow, on the side that
// receives it (the Client) and on the side that sends it (the Server).
func TestRequestCountsAtWireRange(t *testing.T) {
	const top = math.MaxUint16
	for _, tc := range []struct {
		name        string
		outstanding int
		blocking    bool
		ack, req    uint16
		window      int
	}{
		{"ack past the ids held", 0, false, top, 1, top},
		{"ack and request at the top", 1, false, top, top, top},
		{"blocking request at the top", 0, true, 0, top, top - 1},
		{"non-blocking request over a full wire window", 1, false, 0, top, top},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outstanding := make([][]byte, tc.outstanding)
			for i := range outstanding {
				outstanding[i] = testMessageID(byte(i + 1))
			}
			cfg := NewConfig(WithMaxUnacknowledgedMessageIDs(tc.window))
			client := NewClient(
				newTestProtoOptions(protocol.ProtocolVersionDMQNtN2),
				&cfg,
			)
			client.pendingMessageIDs = outstanding
			err := client.handleRequestMessageIds(
				NewMsgRequestMessageIds(
					client.protoVersion,
					tc.blocking,
					tc.ack,
					tc.req,
				),
			)
			require.ErrorIs(t, err, protocol.ErrProtocolViolationRequestExceeded)
			require.Nil(t, client.pendingIDRequest)

			server := NewServer(
				newTestProtoOptions(protocol.ProtocolVersionDMQNtN2),
				&cfg,
			)
			server.pendingMessageIDs = outstanding
			err = server.requestMessageIDs(tc.blocking, tc.ack, tc.req)
			require.ErrorIs(t, err, protocol.ErrProtocolViolationRequestExceeded)
			require.Nil(t, server.pendingIDRequest)
		})
	}
}
