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

// TestClientReannouncesUnacknowledgedTxId covers a mempool that drops a
// transaction and adds it back, as a fork switch does, while the peer has not
// yet acknowledged its first announcement. The reference outbound announces
// mempool entries after the last index it sent, and the re-added entry has a
// new index, so it announces the same id again; the reference inbound keeps
// both entries and acknowledges by count. The client must send that reply
// rather than fail the connection over it, and its window must then count
// the id twice.
func TestClientReannouncesUnacknowledgedTxId(t *testing.T) {
	ids := windowTxIdAndSizes(2)
	replies := make(chan []TxIdAndSize, 3)
	replies <- ids
	replies <- []TxIdAndSize{ids[1]}
	replies <- nil
	served := make(chan uint16, 3)
	p := newRawRequestingPeerAtVersion(t, &Config{
		RequestTxIdsFunc: func(
			_ CallbackContext,
			_ bool,
			_ uint16,
			req uint16,
		) ([]TxIdAndSize, error) {
			served <- req
			return <-replies, nil
		},
	}, 15)
	await := func(what string) {
		t.Helper()
		select {
		case <-served:
		case err := <-p.errorChan:
			t.Fatalf("%s: protocol error: %v", what, err)
		case <-time.After(pendingBytesWindow):
			t.Fatalf("%s never reached the callback", what)
		}
	}

	start := p.outboundRead.count.Load()
	p.send(t, NewMsgRequestTxIds(true, 0, 2))
	await("the blocking request")
	p.waitForOutboundMessage(t, start, NewMsgReplyTxIds(ids))

	// Acknowledge the first id; the second is still outstanding when the
	// mempool announces it again.
	start = p.outboundRead.count.Load()
	p.send(t, NewMsgRequestTxIds(false, 1, 2))
	await("the non-blocking request")
	p.waitForOutboundMessage(
		t,
		start,
		NewMsgReplyTxIds([]TxIdAndSize{ids[1]}),
	)
	p.client.unackedMu.Lock()
	require.Equal(t, []TxIdAndSize{ids[1], ids[1]}, p.client.unackedTxIds)
	p.client.unackedMu.Unlock()

	// Acknowledging both entries leaves nothing: blocking is now required.
	p.send(t, NewMsgRequestTxIds(false, 2, 1))
	select {
	case err := <-p.errorChan:
		require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
	case <-served:
		t.Fatal("a non-blocking request leaving nothing was served")
	case <-time.After(pendingBytesWindow):
		t.Fatal("a non-blocking request leaving nothing was not refused")
	}
}

// TestReconcileTxIdsRefusesDuplicateWithinReply keeps the stricter check:
// a reply read from one mempool snapshot never names an id twice, so one
// that does is refused.
func TestReconcileTxIdsRefusesDuplicateWithinReply(t *testing.T) {
	ids := windowTxIdAndSizes(1)
	_, err := reconcileTxIds(nil, 0, 2, true, []TxIdAndSize{ids[0], ids[0]})
	require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
}
