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
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// pendingBytesWindow bounds how long the at-limit case waits for a rejection
// that must not arrive. The over-limit case below uses the same window and
// does get its rejection inside it, which is what keeps this long enough to
// be meaningful.
const pendingBytesWindow = time.Second

// rawPeer drives a TxSubmission server over a real muxer on an in-memory
// connection, writing raw muxer segments as an initiating peer would. The
// ouroboros-mock conversation harness cannot express these cases: it puts
// each message in a single muxer segment, and a segment payload is capped at
// muxer.SegmentMaxPayloadLength, so it cannot deliver a message anywhere near
// the pending-message byte limit.
type rawPeer struct {
	server    *Server
	conn      net.Conn
	errorChan chan error
}

func newRawPeer(t *testing.T) *rawPeer {
	t.Helper()
	localConn, peerConn := net.Pipe()
	m := muxer.New(localConn)
	m.Start()
	peerDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, peerConn)
		close(peerDone)
	}()
	errorChan := make(chan error, 10)
	s := NewServer(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  localConn.LocalAddr(),
			RemoteAddr: localConn.RemoteAddr(),
		},
		Muxer:     m,
		ErrorChan: errorChan,
		Mode:      protocol.ProtocolModeNodeToNode,
	}, &Config{})
	s.Start()
	t.Cleanup(func() {
		proto := s.ProtocolInstance()
		proto.Stop()
		select {
		case <-proto.DoneChan():
		case <-time.After(5 * time.Second):
			t.Error("protocol did not stop")
		}
		m.Stop()
		_ = peerConn.Close()
		select {
		case <-peerDone:
		case <-time.After(5 * time.Second):
			t.Error("peer drain did not stop")
		}
	})
	return &rawPeer{server: s, conn: peerConn, errorChan: errorChan}
}

// sendAsSegments writes msg to conn as raw muxer segments, split the way
// Protocol.sendLoop splits an oversized payload. net.Pipe is unbuffered, so
// this returns only once the reader's muxer has read every byte. isResponse
// selects which side the muxer routes the segment to: a responder's segment
// reaches the local initiator (client) and an initiator's reaches the local
// responder (server).
func sendAsSegments(
	t *testing.T,
	conn net.Conn,
	msg protocol.Message,
	isResponse bool,
) {
	t.Helper()
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	for len(data) > 0 {
		chunkLen := min(len(data), muxer.SegmentMaxPayloadLength)
		segment := muxer.NewSegment(ProtocolId, data[:chunkLen], isResponse)
		require.NotNil(t, segment)
		require.NoError(
			t,
			binary.Write(conn, binary.BigEndian, segment.SegmentHeader),
		)
		_, err := conn.Write(segment.Payload)
		require.NoError(t, err)
		data = data[chunkLen:]
	}
}

func (p *rawPeer) send(t *testing.T, msg protocol.Message) {
	t.Helper()
	sendAsSegments(t, p.conn, msg, false)
}

// replyTxsEncodedTo returns a MsgReplyTxs whose CBOR encoding is exactly size
// bytes: MaxUnackedTxIds maximum-size transaction bodies -- the window
// MaxPendingMessageBytes is derived from -- plus one trailing body trimmed so
// the encoded message lands on size exactly. Hitting the boundary exactly is
// the point: MaxPendingMessageBytes itself must be admitted and
// MaxPendingMessageBytes+1 rejected.
func replyTxsEncodedTo(t *testing.T, size int) protocol.Message {
	t.Helper()
	bodies := make([]TxBody, 0, MaxUnackedTxIds+1)
	for range MaxUnackedTxIds {
		bodies = append(
			bodies,
			TxBody{EraId: 6, TxBody: make([]byte, MaxTxSizeBytes)},
		)
	}
	bodies = append(bodies, TxBody{EraId: 6})
	tailLen := 0
	for range 8 {
		bodies[MaxUnackedTxIds].TxBody = make([]byte, tailLen)
		msg := NewMsgReplyTxs(bodies)
		encoded, err := cbor.Encode(msg)
		require.NoError(t, err)
		if len(encoded) == size {
			return msg
		}
		tailLen += size - len(encoded)
		require.Positive(
			t,
			tailLen,
			"cannot shrink a MsgReplyTxs to %d bytes",
			size,
		)
	}
	t.Fatalf("could not build a MsgReplyTxs of exactly %d bytes", size)
	return nil
}

// TestReplyTxsOverPendingByteLimitIsRejected proves TxSubmission -- a
// node-to-node protocol, so an untrusted-peer path -- rejects a single
// inbound message larger than its state's pending-message byte limit.
// Protocol.readLoop applies that rejection only when the limit is nonzero,
// so a StateMap that leaves it at zero has no rejection and no inbound
// backpressure at all.
func TestReplyTxsOverPendingByteLimitIsRejected(t *testing.T) {
	t.Parallel()
	p := newRawPeer(t)
	p.send(t, replyTxsEncodedTo(t, MaxPendingMessageBytes+1))
	select {
	case err := <-p.errorChan:
		require.ErrorContains(t, err, "received oversized message")
		require.ErrorContains(
			t,
			err,
			fmt.Sprintf("exceeding limit (%d bytes)", MaxPendingMessageBytes),
		)
	case <-time.After(pendingBytesWindow):
		t.Fatal(
			"a tx-submission message over the pending-message" +
				" byte limit was not rejected",
		)
	}
}

// TestReplyTxsAtPendingByteLimitIsAdmitted is the paired negative case: a
// message of exactly MaxPendingMessageBytes is within the limit and must not
// be rejected as oversized. It is still a MsgReplyTxs arriving in Init, so
// the state machine rejects it on its own terms; only an oversized-message
// rejection is a failure here.
func TestReplyTxsAtPendingByteLimitIsAdmitted(t *testing.T) {
	t.Parallel()
	p := newRawPeer(t)
	p.send(t, replyTxsEncodedTo(t, MaxPendingMessageBytes))
	select {
	case err := <-p.errorChan:
		require.NotContains(
			t,
			err.Error(),
			"received oversized message",
		)
	case <-time.After(pendingBytesWindow):
	}
}

// TestStateMapBoundsPendingMessageBytes keeps every TxSubmission state
// bounded. Protocol.readLoop and Protocol.SendMessage both skip their
// pending-byte accounting when a state's limit is zero, so one unbounded
// state is enough to reopen the hole on an untrusted node-to-node path.
func TestStateMapBoundsPendingMessageBytes(t *testing.T) {
	t.Parallel()
	require.NotEmpty(t, StateMap)
	for state, entry := range StateMap {
		require.Equal(
			t,
			MaxPendingMessageBytes,
			entry.PendingMessageByteLimit,
			"state %s has no pending-message byte limit",
			state,
		)
	}
}

// windowTxIds returns n distinct transaction IDs, as a peer's MsgRequestTxs
// carries them.
func windowTxIds(n int) []TxId {
	txIds := make([]TxId, 0, n)
	for i := range n {
		txId := TxId{EraId: 6}
		txId.TxId[0] = byte(i)
		txIds = append(txIds, txId)
	}
	return txIds
}

// TestRequestTxsOverUnackedWindowIsRefused proves the client refuses a
// request for more transactions than the unacknowledged window before the
// callback assembles any body. MaxPendingMessageBytes is derived from that
// window, so without this refusal a peer can make the client materialize and
// encode an unbounded reply that SendMessage then rejects as a violation of
// our own.
func TestRequestTxsOverUnackedWindowIsRefused(t *testing.T) {
	t.Parallel()
	var called atomic.Bool
	c := NewClient(
		protocol.ProtocolOptions{Mode: protocol.ProtocolModeNodeToNode},
		&Config{
			RequestTxsFunc: func(CallbackContext, []TxId) ([]TxBody, error) {
				called.Store(true)
				return []TxBody{{EraId: 6}}, nil
			},
		},
	)
	errChan := make(chan error, 1)
	go func() {
		errChan <- c.handleRequestTxs(
			NewMsgRequestTxs(windowTxIds(MaxUnackedTxIds + 1)),
		)
	}()
	select {
	case err := <-errChan:
		require.ErrorIs(t, err, protocol.ErrProtocolViolationRequestExceeded)
	case <-time.After(pendingBytesWindow):
		t.Fatal(
			"the client began replying to a request beyond the" +
				" unacknowledged window instead of refusing it",
		)
	}
	require.False(
		t,
		called.Load(),
		"the client assembled a reply for a request beyond the window",
	)
}

// TestRequestTxsAtUnackedWindowReachesCallback is the paired boundary case:
// a full window must still be served, so the refusal above cannot be the
// reason this request fails.
func TestRequestTxsAtUnackedWindowReachesCallback(t *testing.T) {
	t.Parallel()
	errCallbackReached := errors.New("callback reached")
	c := NewClient(
		protocol.ProtocolOptions{Mode: protocol.ProtocolModeNodeToNode},
		&Config{
			RequestTxsFunc: func(CallbackContext, []TxId) ([]TxBody, error) {
				return nil, errCallbackReached
			},
		},
	)
	err := c.handleRequestTxs(
		NewMsgRequestTxs(windowTxIds(MaxUnackedTxIds)),
	)
	require.ErrorIs(t, err, errCallbackReached)
}

// TestServerRequestTxsOverUnackedWindowIsRefused is the sending half: our
// own server must not put a request beyond the window on the wire, where a
// conforming peer is entitled to refuse it.
func TestServerRequestTxsOverUnackedWindowIsRefused(t *testing.T) {
	t.Parallel()
	s := NewServer(
		protocol.ProtocolOptions{Mode: protocol.ProtocolModeNodeToNode},
		&Config{},
	)
	type requestTxsResult struct {
		txs []TxBody
		err error
	}
	resultChan := make(chan requestTxsResult, 1)
	go func() {
		txs, err := s.RequestTxs(windowTxIds(MaxUnackedTxIds + 1))
		resultChan <- requestTxsResult{txs: txs, err: err}
	}()
	select {
	case result := <-resultChan:
		require.ErrorIs(
			t,
			result.err,
			protocol.ErrProtocolViolationRequestExceeded,
		)
		require.Nil(t, result.txs)
	case <-time.After(pendingBytesWindow):
		t.Fatal(
			"the server queued a request beyond the unacknowledged" +
				" window instead of refusing it",
		)
	}
}

// rawRequestingPeer drives a TxSubmission client over a real muxer on an
// in-memory connection, writing raw muxer segments as the requesting peer
// would. The client is the side that holds a mempool and answers
// MsgRequestTxIds, so this is the path an untrusted peer's request reaches.
type rawRequestingPeer struct {
	client    *Client
	conn      net.Conn
	errorChan chan error
}

func newRawRequestingPeer(t *testing.T, cfg *Config) *rawRequestingPeer {
	t.Helper()
	localConn, peerConn := net.Pipe()
	m := muxer.New(localConn)
	m.Start()
	peerDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, peerConn)
		close(peerDone)
	}()
	errorChan := make(chan error, 10)
	c := NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  localConn.LocalAddr(),
			RemoteAddr: localConn.RemoteAddr(),
		},
		Muxer:     m,
		ErrorChan: errorChan,
		Mode:      protocol.ProtocolModeNodeToNode,
	}, cfg)
	c.Start()
	// Init leaves the Init state for Idle, which is where a peer's
	// MsgRequestTxIds is accepted.
	c.Init()
	t.Cleanup(func() {
		proto := c.ProtocolInstance()
		proto.Stop()
		select {
		case <-proto.DoneChan():
		case <-time.After(5 * time.Second):
			t.Error("protocol did not stop")
		}
		m.Stop()
		_ = peerConn.Close()
		select {
		case <-peerDone:
		case <-time.After(5 * time.Second):
			t.Error("peer drain did not stop")
		}
	})
	return &rawRequestingPeer{client: c, conn: peerConn, errorChan: errorChan}
}

func (p *rawRequestingPeer) send(t *testing.T, msg protocol.Message) {
	t.Helper()
	sendAsSegments(t, p.conn, msg, true)
}

// windowTxIdAndSizes returns n distinct advertised transaction IDs, each
// declaring the largest body a peer may send, as a MsgReplyTxIds carries them.
func windowTxIdAndSizes(n int) []TxIdAndSize {
	txIds := make([]TxIdAndSize, 0, n)
	for i, txId := range windowTxIds(n) {
		txIds = append(txIds, TxIdAndSize{
			TxId: txId,
			Size: uint32(MaxTxSizeBytes + i%2),
		})
	}
	return txIds
}

// TestRequestTxIdsOverWindowIsRefusedBeforeTheReply proves a peer cannot use
// the uint16 request field to make the client build a reply larger than its
// own outbound queue limit. MsgRequestTxIds carries a 65,535 request count,
// and a MsgReplyTxIds answering it encodes to roughly 2.6 MB against a
// MaxPendingMessageBytes of 721,424: Protocol.enqueueMessage then returns
// ErrProtocolViolationQueueExceeded and calls SendError, tearing the
// connection down over our own limit. The request is refused on the peer's
// terms instead, before the callback assembles anything.
func TestRequestTxIdsOverWindowIsRefusedBeforeTheReply(t *testing.T) {
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
			return windowTxIdAndSizes(int(req)), nil
		},
	})
	p.send(t, NewMsgRequestTxIds(false, 0, MaxRequestCount))
	select {
	case err := <-p.errorChan:
		require.ErrorIs(
			t,
			err,
			protocol.ErrProtocolViolationRequestExceeded,
		)
		require.NotErrorIs(
			t,
			err,
			protocol.ErrProtocolViolationQueueExceeded,
		)
	case <-time.After(pendingBytesWindow):
		t.Fatal(
			"a request beyond the unacknowledged window was not refused",
		)
	}
	require.False(
		t,
		called.Load(),
		"the client assembled a reply for a request beyond the window",
	)
}

// TestRequestTxIdsAtWindowReachesCallback is the paired boundary case: a
// request for a full window must still be served, so the refusal above
// cannot be the reason this request fails.
func TestRequestTxIdsAtWindowReachesCallback(t *testing.T) {
	t.Parallel()
	requested := make(chan uint16, 1)
	p := newRawRequestingPeer(t, &Config{
		RequestTxIdsFunc: func(
			_ CallbackContext,
			_ bool,
			_ uint16,
			req uint16,
		) ([]TxIdAndSize, error) {
			requested <- req
			return windowTxIdAndSizes(int(req)), nil
		},
	})
	p.send(t, NewMsgRequestTxIds(false, 0, MaxUnackedTxIds))
	select {
	case req := <-requested:
		require.Equal(t, uint16(MaxUnackedTxIds), req)
	case err := <-p.errorChan:
		t.Fatalf("a request inside the window was refused: %v", err)
	case <-time.After(pendingBytesWindow):
		t.Fatal("a request inside the window never reached the callback")
	}
}

// TestRequestTxIdsWindowClosesAsIdsGoUnacknowledged proves the bound is the
// outstanding window and not a bare per-message cap: a request that would be
// admitted on its own is refused once an unacknowledged reply already fills
// the window. This is the reference implementation's
// unackedNo - ackNo + reqNo > maxUnacked condition
// (Ouroboros.Network.TxSubmission.Outbound).
func TestRequestTxIdsWindowClosesAsIdsGoUnacknowledged(t *testing.T) {
	t.Parallel()
	requested := make(chan uint16, 2)
	p := newRawRequestingPeer(t, &Config{
		RequestTxIdsFunc: func(
			_ CallbackContext,
			_ bool,
			_ uint16,
			req uint16,
		) ([]TxIdAndSize, error) {
			requested <- req
			return windowTxIdAndSizes(int(req)), nil
		},
	})
	p.send(t, NewMsgRequestTxIds(false, 0, MaxUnackedTxIds))
	select {
	case req := <-requested:
		require.Equal(t, uint16(MaxUnackedTxIds), req)
	case <-time.After(pendingBytesWindow):
		t.Fatal("the first full-window request never reached the callback")
	}
	// Messages are handled in order on one receive loop, so the reply above
	// is already accounted for by the time this request is handled.
	p.send(t, NewMsgRequestTxIds(false, 0, 1))
	select {
	case err := <-p.errorChan:
		require.ErrorIs(
			t,
			err,
			protocol.ErrProtocolViolationRequestExceeded,
		)
	case req := <-requested:
		t.Fatalf(
			"a request for %d more IDs was served with a full window",
			req,
		)
	case <-time.After(pendingBytesWindow):
		t.Fatal("a request past a full window was not refused")
	}
}

// TestRequestTxIdsWindowReopensOnAcknowledgement is the paired case: the same
// request is served once the peer acknowledges what it was holding, so
// TestRequestTxIdsWindowClosesAsIdsGoUnacknowledged cannot be passing because
// a second request is refused unconditionally.
func TestRequestTxIdsWindowReopensOnAcknowledgement(t *testing.T) {
	t.Parallel()
	requested := make(chan uint16, 2)
	p := newRawRequestingPeer(t, &Config{
		RequestTxIdsFunc: func(
			_ CallbackContext,
			_ bool,
			_ uint16,
			req uint16,
		) ([]TxIdAndSize, error) {
			requested <- req
			return windowTxIdAndSizes(int(req)), nil
		},
	})
	p.send(t, NewMsgRequestTxIds(false, 0, MaxUnackedTxIds))
	select {
	case req := <-requested:
		require.Equal(t, uint16(MaxUnackedTxIds), req)
	case <-time.After(pendingBytesWindow):
		t.Fatal("the first full-window request never reached the callback")
	}
	p.send(t, NewMsgRequestTxIds(false, MaxUnackedTxIds, MaxUnackedTxIds))
	select {
	case req := <-requested:
		require.Equal(t, uint16(MaxUnackedTxIds), req)
	case err := <-p.errorChan:
		t.Fatalf("a request inside the reopened window was refused: %v", err)
	case <-time.After(pendingBytesWindow):
		t.Fatal(
			"a request inside the reopened window never reached the callback",
		)
	}
}

// TestRequestTxIdsAckOverOutstandingIsRefused covers the other half of the
// reference condition: acknowledging more IDs than were ever sent
// (ProtocolErrorAckedTooManyTxids) would otherwise credit the window with
// capacity the peer never earned.
func TestRequestTxIdsAckOverOutstandingIsRefused(t *testing.T) {
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
			return windowTxIdAndSizes(int(req)), nil
		},
	})
	p.send(t, NewMsgRequestTxIds(false, 1, 1))
	select {
	case err := <-p.errorChan:
		require.ErrorIs(
			t,
			err,
			protocol.ErrProtocolViolationRequestExceeded,
		)
	case <-time.After(pendingBytesWindow):
		t.Fatal(
			"an acknowledgement of IDs that were never sent was accepted",
		)
	}
	require.False(
		t,
		called.Load(),
		"the client served a request carrying an unearned acknowledgement",
	)
}

// TestServerRequestTxIdsOverUnackedWindowIsRefused is the sending half: our
// own server must not put a txid request beyond the window on the wire, where
// a conforming peer is entitled to refuse it.
func TestServerRequestTxIdsOverUnackedWindowIsRefused(t *testing.T) {
	t.Parallel()
	s := NewServer(
		protocol.ProtocolOptions{Mode: protocol.ProtocolModeNodeToNode},
		&Config{},
	)
	type requestTxIdsOutcome struct {
		txIds []TxIdAndSize
		err   error
	}
	resultChan := make(chan requestTxIdsOutcome, 1)
	go func() {
		txIds, err := s.RequestTxIds(false, MaxUnackedTxIds+1)
		resultChan <- requestTxIdsOutcome{txIds: txIds, err: err}
	}()
	select {
	case result := <-resultChan:
		require.ErrorIs(
			t,
			result.err,
			protocol.ErrProtocolViolationRequestExceeded,
		)
		require.Nil(t, result.txIds)
	case <-time.After(pendingBytesWindow):
		t.Fatal(
			"the server queued a txid request beyond the unacknowledged" +
				" window instead of refusing it",
		)
	}
}
