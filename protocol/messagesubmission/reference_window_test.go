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
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// dmq-node's requester asks for at most 33 IDs at a time and keeps up to
// 4 * 33 = 132 unacknowledged (DMQ/Policy.hs maxSigIdsInflight,
// sigDecisionPolicy), and its outbound checks requests against the same 132.
const (
	referenceIDsPerRequest = 33
	referenceWindow        = 4 * referenceIDsPerRequest
)

// TestV2OutboundAcceptsReferenceInboundWindow drives a default-configured
// Client the way dmq-node's V2 requester fills its window: a blocking
// request, then non-blocking requests pipelined behind one another that
// acknowledge nothing, each for 33 IDs. The last one leaves 132 IDs
// outstanding, which dmq-node's own outbound accepts, so the Client must
// too.
func TestV2OutboundAcceptsReferenceInboundWindow(t *testing.T) {
	h := newClientHarness(t, protocol.ProtocolVersionDMQNtN2)
	next := 0
	reply := func(what string, blocking bool) {
		t.Helper()
		require.Equal(
			t,
			idRequest{
				blocking: blocking,
				ack:      0,
				req:      referenceIDsPerRequest,
			},
			await(t, h.idRequests, h.errs, what),
		)
		ids := make([]pcommon.MessageIDAndSize, 0, referenceIDsPerRequest)
		items := make([]byte, 0, referenceIDsPerRequest*38)
		for range referenceIDsPerRequest {
			next++
			id := bytes.Repeat([]byte{byte(next)}, 32)
			ids = append(ids, pcommon.MessageIDAndSize{
				MessageID:   id,
				SizeInBytes: 100,
			})
			items = append(items, idAndSize(id)...)
		}
		require.NoError(t, h.client.ReplyMessageIds(ids))
		// MsgReplySigIds = [2, [_ [id, 100] ...]]
		h.peer.expect(false, cat([]byte{0x82, 0x02, 0x9f}, items, []byte{0xff}))
	}

	// MsgRequestSigIds blocking, ack 0, req 33 = [1, true, 0, 33]
	h.peer.send(true, []byte{0x84, 0x01, 0xf5, 0x00, 0x18, 0x21})
	reply("the blocking request", true)

	// Three non-blocking requests, ack 0, req 33, sent back to back.
	nonBlocking := []byte{0x84, 0x01, 0xf4, 0x00, 0x18, 0x21}
	for range 3 {
		h.peer.send(true, nonBlocking)
	}
	reply("the first pipelined request", false)
	reply("the second pipelined request", false)
	reply("the third pipelined request", false)

	require.Len(t, h.client.GetPendingMessageIDs(), referenceWindow)
	select {
	case err := <-h.errs:
		t.Fatalf("protocol error: %v", err)
	default:
	}
}

// TestDefaultWindowMatchesReference pins the default window to dmq-node's,
// the window its requester fills and its outbound enforces.
func TestDefaultWindowMatchesReference(t *testing.T) {
	require.Equal(t, referenceWindow, DefaultMaxUnacknowledgedMessageIDs)
	cfg := NewConfig()
	require.Equal(t, referenceWindow, cfg.MaxUnacknowledgedMessageIDs)
}
