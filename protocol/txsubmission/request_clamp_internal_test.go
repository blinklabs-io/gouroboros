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

// From NodeToNodeV_16 the window is a local setting of each side, so a peer
// may ask for more transaction IDs than fit in ours. That is not a protocol
// error: the client answers with as many as fit (the reference's
// TxOutboundV_2). Before V16 the same request is refused.
func TestRequestTxIdsOverWindowIsClampedFromV16(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		version uint16
		clamped bool
	}{
		{"v15 refuses", 15, false},
		{"v16 clamps", 16, true},
		{"v17 clamps", 17, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			requested := make(chan uint16, 2)
			p := newRawRequestingPeerAtVersion(t, &Config{
				RequestTxIdsFunc: func(
					_ CallbackContext,
					_ bool,
					_ uint16,
					req uint16,
				) ([]TxIdAndSize, error) {
					requested <- req
					return windowTxIdAndSizes(int(req)), nil
				},
			}, test.version)
			outboundStart := p.outboundRead.count.Load()
			p.send(t, NewMsgRequestTxIds(true, 0, MaxUnackedTxIds+5))
			if !test.clamped {
				select {
				case err := <-p.errorChan:
					require.ErrorIs(
						t,
						err,
						protocol.ErrProtocolViolationRequestExceeded,
					)
				case req := <-requested:
					t.Fatalf("a request over the window was served: %d", req)
				case <-time.After(pendingBytesWindow):
					t.Fatal("a request over the window was not refused")
				}
				return
			}
			select {
			case req := <-requested:
				require.Equal(t, uint16(MaxUnackedTxIds), req)
			case err := <-p.errorChan:
				t.Fatalf("a request over the window was refused: %v", err)
			case <-time.After(pendingBytesWindow):
				t.Fatal("a request over the window never reached the callback")
			}
			p.waitForOutboundMessage(
				t,
				outboundStart,
				NewMsgReplyTxIds(windowTxIdAndSizes(MaxUnackedTxIds)),
			)
			// Two acknowledged leave eight outstanding, so only two fit.
			p.send(t, NewMsgRequestTxIds(false, 2, 5))
			select {
			case req := <-requested:
				require.Equal(t, uint16(2), req)
			case err := <-p.errorChan:
				t.Fatalf("a partial request was refused: %v", err)
			case <-time.After(pendingBytesWindow):
				t.Fatal("a partial request never reached the callback")
			}
		})
	}
}

// Clamping replaces only the window check: acknowledging more IDs than are
// outstanding is still a protocol error from V16.
func TestRequestTxIdsAckOverOutstandingIsRefusedFromV16(t *testing.T) {
	t.Parallel()
	requested := make(chan uint16, 1)
	p := newRawRequestingPeerAtVersion(t, &Config{
		RequestTxIdsFunc: func(
			_ CallbackContext,
			_ bool,
			_ uint16,
			req uint16,
		) ([]TxIdAndSize, error) {
			requested <- req
			return windowTxIdAndSizes(int(req)), nil
		},
	}, 16)
	p.send(t, NewMsgRequestTxIds(true, 1, MaxUnackedTxIds+5))
	select {
	case err := <-p.errorChan:
		require.ErrorIs(t, err, protocol.ErrProtocolViolationRequestExceeded)
	case req := <-requested:
		t.Fatalf("an over-acknowledging request was served: %d", req)
	case <-time.After(pendingBytesWindow):
		t.Fatal("an over-acknowledging request was not refused")
	}
}

// With the window full, a non-blocking request is clamped to zero IDs and is
// answered with an empty reply, which the reference allows: its TxOutboundV_2
// replies with "only as many txids as fit, which can be none for a
// non-blocking request" (Ouroboros.Network.TxSubmission.Outbound). The
// callback still runs, with a count of zero, because it is how the caller
// learns what the peer acknowledged; returning any ID for that count is
// refused.
func TestRequestTxIdsNonBlockingClampedToZeroFromV16(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		extra int
	}{
		{"empty reply", 0},
		{"reply past the clamp refused", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			type call struct{ ack, req uint16 }
			calls := make(chan call, 2)
			p := newRawRequestingPeerAtVersion(t, &Config{
				RequestTxIdsFunc: func(
					_ CallbackContext,
					blocking bool,
					ack uint16,
					req uint16,
				) ([]TxIdAndSize, error) {
					calls <- call{ack, req}
					if blocking {
						return windowTxIdAndSizes(int(req)), nil
					}
					return windowTxIdAndSizes(int(req) + test.extra)[:test.extra], nil
				},
			}, 16)
			outboundStart := p.outboundRead.count.Load()
			p.send(t, NewMsgRequestTxIds(true, 0, MaxUnackedTxIds))
			require.Equal(t, call{0, MaxUnackedTxIds}, <-calls)
			p.waitForOutboundMessage(
				t,
				outboundStart,
				NewMsgReplyTxIds(windowTxIdAndSizes(MaxUnackedTxIds)),
			)
			outboundStart = p.outboundRead.count.Load()
			p.send(t, NewMsgRequestTxIds(false, 0, 3))
			// The callback runs before any refusal of its reply, so wait for
			// it alone; a refusal of the request itself never reaches it.
			select {
			case got := <-calls:
				require.Equal(t, call{0, 0}, got)
			case <-time.After(pendingBytesWindow):
				t.Fatal("a request on a full window never reached the callback")
			}
			if test.extra == 0 {
				p.waitForOutboundMessage(
					t,
					outboundStart,
					NewMsgReplyTxIds([]TxIdAndSize{}),
				)
				return
			}
			select {
			case err := <-p.errorChan:
				require.ErrorIs(
					t,
					err,
					protocol.ErrProtocolViolationRequestExceeded,
				)
			case <-time.After(pendingBytesWindow):
				t.Fatal("a reply past the clamped count was not refused")
			}
		})
	}
}

// A blocking request is never clamped to zero: the reference refuses a
// blocking request that leaves IDs unacknowledged (ProtocolErrorRequestBlocking)
// or asks for none (ProtocolErrorRequestedNothing) before it clamps, and the
// specification forbids an empty reply to a blocking request
// (miniprotocols.tex, tx-submission "MsgRequestTxIds"; the reference codec
// refuses to decode one). Without that refusal a full window would clamp a
// blocking request to zero and answer it with an empty reply.
func TestRequestTxIdsBlockingIsNotClampedToZero(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		version uint16
		ack     uint16
		req     uint16
	}{
		{"v16 full window, nothing acknowledged", 16, 0, 3},
		{"v16 ids left after the acknowledgement", 16, 4, 3},
		{"v16 asks for nothing", 16, MaxUnackedTxIds, 0},
		{"v15 ids left after the acknowledgement", 15, 4, 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			calls := make(chan uint16, 2)
			p := newRawRequestingPeerAtVersion(t, &Config{
				RequestTxIdsFunc: func(
					_ CallbackContext,
					_ bool,
					_ uint16,
					req uint16,
				) ([]TxIdAndSize, error) {
					calls <- req
					return windowTxIdAndSizes(int(req)), nil
				},
			}, test.version)
			outboundStart := p.outboundRead.count.Load()
			p.send(t, NewMsgRequestTxIds(true, 0, MaxUnackedTxIds))
			require.Equal(t, uint16(MaxUnackedTxIds), <-calls)
			p.waitForOutboundMessage(
				t,
				outboundStart,
				NewMsgReplyTxIds(windowTxIdAndSizes(MaxUnackedTxIds)),
			)
			p.send(t, NewMsgRequestTxIds(true, test.ack, test.req))
			select {
			case err := <-p.errorChan:
				require.ErrorIs(
					t,
					err,
					protocol.ErrProtocolViolationInvalidMessage,
				)
			case req := <-calls:
				t.Fatalf("an illegal blocking request was served: %d", req)
			case <-time.After(pendingBytesWindow):
				t.Fatal("an illegal blocking request was not refused")
			}
		})
	}
}
