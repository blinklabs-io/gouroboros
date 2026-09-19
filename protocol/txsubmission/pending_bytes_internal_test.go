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

// send writes msg to the server as raw muxer segments, split the way
// Protocol.sendLoop splits an oversized payload. net.Pipe is unbuffered, so
// this returns only once the server's muxer has read every byte.
func (p *rawPeer) send(t *testing.T, msg protocol.Message) {
	t.Helper()
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	for len(data) > 0 {
		chunkLen := min(len(data), muxer.SegmentMaxPayloadLength)
		segment := muxer.NewSegment(ProtocolId, data[:chunkLen], false)
		require.NotNil(t, segment)
		require.NoError(
			t,
			binary.Write(p.conn, binary.BigEndian, segment.SegmentHeader),
		)
		_, err := p.conn.Write(segment.Payload)
		require.NoError(t, err)
		data = data[chunkLen:]
	}
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
