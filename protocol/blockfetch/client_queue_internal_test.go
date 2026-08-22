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

package blockfetch

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// newQueueTestClient builds a client whose protocol was never started. The
// outstanding-request queue and the in-flight byte accounting do not touch the
// network, so they can be exercised directly.
func newQueueTestClient(cfg *Config) *Client {
	return NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{},
			RemoteAddr: &net.TCPAddr{},
		},
	}, cfg)
}

// newInMemoryPeer is the shared transport fixture for queue tests that need a
// live muxer. ouroboros-mock emits one muxer segment per message, so it cannot
// inject the multi-segment oversized messages exercised by the Idle ingress
// tests below.
func newInMemoryPeer(
	t *testing.T,
	cfg *Config,
	startMuxer bool,
	errorChan chan error,
) *idlePeer {
	t.Helper()
	localConn, peerConn := net.Pipe()
	m := muxer.New(localConn)
	if startMuxer {
		m.Start()
	}
	peerDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, peerConn)
		close(peerDone)
	}()
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
	t.Cleanup(func() {
		proto := c.ProtocolInstance()
		proto.Stop()
		select {
		case <-proto.DoneChan():
		case <-time.After(time.Second):
			t.Error("protocol did not stop")
		}
		c.failOutstanding(protocol.ErrProtocolShuttingDown)
		m.Stop()
		_ = peerConn.Close()
		select {
		case <-peerDone:
		case <-time.After(time.Second):
			t.Error("peer drain did not stop")
		}
	})
	return &idlePeer{client: c, conn: peerConn, errorChan: errorChan}
}

// newStartedQueueTestClient provides a live protocol send queue while draining
// the other side of an in-memory connection. It deliberately sends no peer
// responses, leaving accepted requests outstanding for queue assertions.
func newStartedQueueTestClient(t *testing.T, cfg *Config) *Client {
	t.Helper()
	return newInMemoryPeer(t, cfg, false, nil).client
}

func (c *Client) appendTestRequest(reservedBytes uint64) *rangeRequest {
	c.queueMutex.Lock()
	defer c.queueMutex.Unlock()
	c.nextRequestId++
	req := &rangeRequest{
		id:            c.nextRequestId,
		protocol:      c.ProtocolInstance(),
		delivery:      deliveryCallback,
		pipelined:     true,
		reservedBytes: reservedBytes,
		startChan:     make(chan error, 1),
		blockChan:     make(chan ledger.Block, 1),
		doneChan:      make(chan error, 1),
	}
	c.queue = append(c.queue, req)
	c.inFlightBytes += reservedBytes
	return req
}

// TestQueueRejectsResponseWithNoOutstandingRequest covers a peer answering
// when nothing was asked for.
func TestQueueRejectsResponseWithNoOutstandingRequest(t *testing.T) {
	c := newQueueTestClient(&Config{
		RequestPipelining: true,
		RangeDoneFunc: func(CallbackContext, error) error {
			return nil
		},
	})
	require.ErrorContains(
		t,
		c.handleStartBatch(),
		"no outstanding range request",
	)
	require.ErrorContains(
		t,
		c.handleBatchDone(),
		"no outstanding range request",
	)
	require.ErrorContains(t, c.handleNoBlocks(), "no outstanding range request")
}

// TestQueueExcessBatchDoneIsNotAppliedToNextRequest is the negative case for
// FIFO attribution: once a request is retired, a further MsgBatchDone must be
// rejected instead of completing the request behind it.
func TestQueueExcessBatchDoneIsNotAppliedToNextRequest(t *testing.T) {
	completions := make(chan uint64, 4)
	c := newQueueTestClient(&Config{
		RequestPipelining: true,
		RangeDoneFunc: func(ctx CallbackContext, err error) error {
			require.NoError(t, err)
			completions <- ctx.RequestId
			return nil
		},
	})
	first := c.appendTestRequest(1024)
	second := c.appendTestRequest(1024)

	require.NoError(t, c.handleStartBatch())
	require.True(t, first.started)
	require.False(t, second.started)
	require.NoError(t, c.handleBatchDone())
	require.Equal(t, uint64(1), <-completions)

	// The second request is now at the head but has never been started, so a
	// further BatchDone belongs to nothing.
	require.ErrorContains(t, c.handleBatchDone(), "in the wrong order")
	c.queueMutex.Lock()
	queueLen := len(c.queue)
	c.queueMutex.Unlock()
	require.Equal(t, 1, queueLen, "the queue must still hold request 2")
	require.Len(t, completions, 0, "request 2 must not be reported complete")
	select {
	case err := <-second.doneChan:
		t.Fatalf("request 2 was resolved by an excess BatchDone: %v", err)
	default:
	}
}

// TestQueueRejectsBlockBeforeStartBatch covers a peer streaming a block for a
// request the head entry has not started.
func TestQueueRejectsBlockBeforeStartBatch(t *testing.T) {
	c := newQueueTestClient(&Config{
		RequestPipelining: true,
		RangeDoneFunc: func(CallbackContext, error) error {
			return nil
		},
	})
	c.appendTestRequest(1024)
	require.ErrorContains(
		t,
		c.handleBlock(NewMsgBlock([]byte{0x80})),
		"in the wrong order",
	)
}

// TestQueueRetiredBytesAreReleased covers the in-flight accounting: retiring a
// request returns its reserved bytes to the budget.
func TestQueueRetiredBytesAreReleased(t *testing.T) {
	c := newQueueTestClient(&Config{
		RequestPipelining: true,
		RangeDoneFunc: func(CallbackContext, error) error {
			return nil
		},
	})
	c.appendTestRequest(4096)
	c.queueMutex.Lock()
	inFlight := c.inFlightBytes
	c.queueMutex.Unlock()
	require.Equal(t, uint64(4096), inFlight)

	require.NoError(t, c.handleStartBatch())
	require.NoError(t, c.handleBatchDone())
	c.queueMutex.Lock()
	inFlight = c.inFlightBytes
	queueLen := len(c.queue)
	c.queueMutex.Unlock()
	require.Equal(t, uint64(0), inFlight)
	require.Equal(t, 0, queueLen)
}

// TestInFlightCapacityAdmitsOversizedRequestOnEmptyQueue verifies a range
// whose expected size exceeds the whole bound is still admitted when nothing
// else is outstanding, so an oversized range cannot stall forever.
func TestInFlightCapacityAdmitsOversizedRequestOnEmptyQueue(t *testing.T) {
	c := newQueueTestClient(&Config{
		RequestPipelining: true,
		MaxInFlightBytes:  1024,
	})
	c.queueMutex.Lock()
	defer c.queueMutex.Unlock()
	require.True(t, c.hasInFlightCapacityLocked(1024*1024))
}

// TestInFlightCapacityBlocksAtBound verifies that a request which does not fit
// is held until the existing request retires.
func TestInFlightCapacityBlocksAtBound(t *testing.T) {
	c := newQueueTestClient(&Config{
		RequestPipelining: true,
		MaxInFlightBytes:  1024,
	})
	req := c.appendTestRequest(1024)
	c.queueMutex.Lock()
	require.False(t, c.hasInFlightCapacityLocked(1024))
	c.queueMutex.Unlock()

	// Releasing the outstanding request makes room again
	c.queueMutex.Lock()
	c.removeLocked(req)
	require.True(t, c.hasInFlightCapacityLocked(1024))
	c.queueMutex.Unlock()
}

// TestInFlightCapacityDefaultsBound verifies an unset MaxInFlightBytes
// resolves to the documented default rather than to zero, which would admit
// nothing.
func TestInFlightCapacityDefaultsBound(t *testing.T) {
	c := newQueueTestClient(&Config{RequestPipelining: true})
	require.Equal(t, DefaultMaxInFlightBytes, c.maxInFlightBytes())
}

func TestInFlightCapacityDoesNotOverflow(t *testing.T) {
	c := newQueueTestClient(&Config{
		RequestPipelining: true,
		MaxInFlightBytes:  math.MaxUint64,
	})
	c.appendTestRequest(math.MaxUint64)
	c.queueMutex.Lock()
	defer c.queueMutex.Unlock()
	require.False(t, c.hasInFlightCapacityLocked(1))
}

// TestConcurrentAdmissionRespectsInFlightBound pins the recheck under the send
// token: two callers released into the admission path together both pass the
// capacity check made before the token, so without the recheck the second is
// admitted past MaxInFlightBytes.
func TestConcurrentAdmissionRespectsInFlightBound(t *testing.T) {
	c := newStartedQueueTestClient(t, &Config{
		RequestPipelining: true,
		MaxInFlightBytes:  1,
		RangeDoneFunc: func(CallbackContext, error) error {
			return nil
		},
	})

	var arrived sync.WaitGroup
	arrived.Add(2)
	release := make(chan struct{})
	c.beforeRequestAdmission = func() {
		arrived.Done()
		<-release
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := c.RequestRange(ctx, RangeRequest{ExpectedBytes: 1})
			results <- err
		}()
	}
	arrived.Wait()
	close(release)

	// Both callers reached the admission boundary while the queue was empty.
	// Exactly one may reserve the entire bound; the other must be held back by
	// the recheck under the same lock as reservation.
	select {
	case err := <-results:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("no request was admitted")
	}
	select {
	case err := <-results:
		t.Fatalf("second request was admitted past the bound: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	c.queueMutex.Lock()
	require.Len(t, c.queue, 1)
	require.Equal(t, uint64(1), c.inFlightBytes)
	c.queueMutex.Unlock()
	cancel()
	select {
	case err := <-results:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("waiting RequestRange did not return after cancellation")
	}
	c.queueMutex.Lock()
	defer c.queueMutex.Unlock()
	require.Len(t, c.queue, 1)
	require.Equal(t, uint64(1), c.inFlightBytes)
}

// TestInFlightWaitDoesNotBlockOtherCallers covers the wedge: a pipelined
// caller parked on the in-flight byte bound must not hold the send token,
// because every other request path shares it. GetBlockRange and GetBlock pass
// context.Background(), so a caller wedged behind the parked request would
// only be released by protocol shutdown.
func TestInFlightWaitDoesNotBlockOtherCallers(t *testing.T) {
	c := newStartedQueueTestClient(t, &Config{
		RequestPipelining: true,
		MaxInFlightBytes:  1,
		RangeDoneFunc: func(CallbackContext, error) error {
			return nil
		},
	})
	waiting := make(chan struct{})
	var waitingOnce sync.Once
	c.beforeInFlightWait = func() {
		waitingOnce.Do(func() { close(waiting) })
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The first request takes the whole bound. The test peer never answers,
	// so nothing retires it and nothing releases capacity.
	_, err := c.RequestRange(ctx, RangeRequest{ExpectedBytes: 1})
	require.NoError(t, err)

	// The second request cannot be admitted and parks on the byte bound.
	blocked := make(chan error, 1)
	go func() {
		_, err := c.RequestRange(ctx, RangeRequest{ExpectedBytes: 1})
		blocked <- err
	}()
	select {
	case <-waiting:
	case err := <-blocked:
		t.Fatalf("second request was not held by the byte bound: %v", err)
	case <-time.After(time.Second):
		t.Fatal("second request never reached the in-flight wait")
	}

	// A concurrent GetBlockRange must still reach the wire. It blocks
	// afterwards waiting for MsgStartBatch, which the test peer never sends,
	// so the observable is its queue entry rather than its return.
	legacyDone := make(chan error, 1)
	go func() {
		legacyDone <- c.GetBlockRange(pcommon.Point{}, pcommon.Point{})
	}()
	require.Eventually(t, func() bool {
		c.queueMutex.Lock()
		defer c.queueMutex.Unlock()
		for _, queued := range c.queue {
			if !queued.pipelined {
				return true
			}
		}
		return false
	}, time.Second, time.Millisecond,
		"GetBlockRange never queued its request: it is wedged behind a RequestRange parked on the in-flight byte bound",
	)

	// The parked request is still parked, holding nothing.
	select {
	case err := <-blocked:
		t.Fatalf("second request was admitted past the bound: %v", err)
	default:
	}
	cancel()
	select {
	case err := <-blocked:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("parked RequestRange did not return after cancellation")
	}
	c.failOutstanding(protocol.ErrProtocolShuttingDown)
	select {
	case err := <-legacyDone:
		require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	case <-time.After(time.Second):
		t.Fatal("GetBlockRange was not released by shutdown")
	}
}

func TestRequestRangeContextCancelsBlockedSend(t *testing.T) {
	c := newQueueTestClient(&Config{
		RequestPipelining: true,
		MaxInFlightBytes:  1,
		RangeDoneFunc: func(CallbackContext, error) error {
			return nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan error, 1)
	go func() {
		_, err := c.RequestRange(ctx, RangeRequest{ExpectedBytes: 1})
		results <- err
	}()

	// A protocol that has not started has no send queue, so the request is
	// guaranteed to be blocked after reservation rather than already sent.
	require.Eventually(t, func() bool {
		c.queueMutex.Lock()
		defer c.queueMutex.Unlock()
		return len(c.queue) == 1 && c.inFlightBytes == 1
	}, time.Second, time.Millisecond)
	cancel()
	select {
	case err := <-results:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("RequestRange remained blocked after cancellation")
	}
	c.queueMutex.Lock()
	defer c.queueMutex.Unlock()
	require.Empty(t, c.queue)
	require.Zero(t, c.inFlightBytes)
}

func TestOldProtocolShutdownDoesNotFailRestartedRequests(t *testing.T) {
	var completions []uint64
	c := newQueueTestClient(&Config{
		RequestPipelining: true,
		RangeDoneFunc: func(ctx CallbackContext, _ error) error {
			completions = append(completions, ctx.RequestId)
			return nil
		},
	})
	oldProto := c.ProtocolInstance()
	c.initProtocol()
	newProto := c.ProtocolInstance()
	req := c.appendTestRequest(1)

	c.failOutstandingForProtocol(
		oldProto,
		protocol.ErrProtocolShuttingDown,
	)
	c.queueMutex.Lock()
	require.Len(t, c.queue, 1)
	require.Same(t, req, c.queue[0])
	require.Equal(t, uint64(1), c.inFlightBytes)
	c.queueMutex.Unlock()
	require.Empty(t, completions)
	select {
	case err := <-req.doneChan:
		t.Fatalf("old protocol shutdown resolved restarted request: %v", err)
	default:
	}

	c.failOutstandingForProtocol(
		newProto,
		protocol.ErrProtocolShuttingDown,
	)
	require.Equal(t, []uint64{req.id}, completions)
	c.queueMutex.Lock()
	defer c.queueMutex.Unlock()
	require.Empty(t, c.queue)
	require.Zero(t, c.inFlightBytes)
}

// idleLimitObservationWindow bounds both halves of the Idle-state ingress
// limit pair below: the non-pipelining client must report the oversized
// message within it, and the pipelining client must not.
const idleLimitObservationWindow = time.Second

// idlePeer drives a client attached to a real muxer over an in-memory
// connection. The ouroboros-mock conversation harness cannot express these
// cases: it puts each message in a single muxer segment, and a segment
// payload is capped at muxer.SegmentMaxPayloadLength, so it cannot deliver a
// mainnet-sized block at all.
type idlePeer struct {
	client    *Client
	conn      net.Conn
	errorChan chan error
}

func newIdlePeer(t *testing.T, cfg *Config) *idlePeer {
	t.Helper()
	errorChan := make(chan error, 10)
	return newInMemoryPeer(t, cfg, true, errorChan)
}

// send writes a message to the client as raw muxer segments, splitting it
// across segments the way Protocol.sendLoop does for an oversized payload.
// net.Pipe is unbuffered, so this returns only once the client's muxer has
// read every byte.
func (p *idlePeer) send(t *testing.T, msg protocol.Message) {
	t.Helper()
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	for len(data) > 0 {
		chunkLen := min(len(data), muxer.SegmentMaxPayloadLength)
		segment := muxer.NewSegment(ProtocolId, data[:chunkLen], true)
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

// completeOneEmptyBatch drives a single pipelined request to completion so the
// client's state machine returns to Idle through MsgBatchDone, which is the
// state a peer's next MsgBlock can arrive in.
func (p *idlePeer) completeOneEmptyBatch(t *testing.T, done chan error) {
	t.Helper()
	_, err := p.client.RequestRange(
		context.Background(),
		RangeRequest{ExpectedBytes: 1},
	)
	require.NoError(t, err)
	p.send(t, NewMsgStartBatch())
	p.send(t, NewMsgBatchDone())
	select {
	case err := <-done:
		require.NoError(t, err)
	case err := <-p.errorChan:
		t.Fatalf("unexpected protocol error before the batch completed: %s", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the first range request never completed")
	}
}

// bigBlockMsg returns a MsgBlock whose encoded size exceeds
// IdleMaxPendingMessageBytes. Its body is one maximum-size mainnet block body,
// which is what a real peer streams during catch-up sync.
func bigBlockMsg(t *testing.T) protocol.Message {
	t.Helper()
	msg := NewMsgBlock(make([]byte, DefaultRequestExpectedBytes))
	encoded, err := cbor.Encode(msg)
	require.NoError(t, err)
	require.Greater(t, len(encoded), IdleMaxPendingMessageBytes)
	require.LessOrEqual(t, len(encoded), PipelinedIdleMaxPendingMessageBytes)
	return msg
}

// TestPipelinedIdleLimitAdmitsMainnetSizedBlock covers the Idle-state ingress
// limit for a pipelining client. With more than one request outstanding, the
// peer's MsgBlock for the next request can arrive while the state machine is
// momentarily back in Idle after a MsgBatchDone, so the Idle limit has to
// admit a block. A limit of IdleMaxPendingMessageBytes tears the connection
// down instead, which only shows up against a real peer because the blocks in
// the mock conversations are tiny.
func TestPipelinedIdleLimitAdmitsMainnetSizedBlock(t *testing.T) {
	done := make(chan error, 4)
	p := newIdlePeer(t, &Config{
		RequestPipelining: true,
		RangeDoneFunc: func(_ CallbackContext, err error) error {
			done <- err
			return nil
		},
	})
	p.completeOneEmptyBatch(t, done)

	// The state machine is back in Idle and the block is larger than the
	// unpipelined Idle limit. It must be admitted rather than rejected as
	// oversized; nothing consumes it, because the client has agency in Idle.
	p.send(t, bigBlockMsg(t))
	select {
	case err := <-p.errorChan:
		t.Fatalf(
			"a mainnet-sized block was rejected while the state machine was in Idle: %s",
			err,
		)
	case <-p.client.ProtocolInstance().DoneChan():
		t.Fatal("the protocol was torn down by a mainnet-sized block in Idle")
	case <-time.After(idleLimitObservationWindow):
	}
}

// TestUnpipelinedIdleLimitRejectsMainnetSizedBlock is the paired positive
// case. It keeps the unpipelined Idle limit honest, and it proves the
// observation window above is long enough to see the rejection when the limit
// does produce one.
func TestUnpipelinedIdleLimitRejectsMainnetSizedBlock(t *testing.T) {
	p := newIdlePeer(t, &Config{})
	p.send(t, bigBlockMsg(t))
	select {
	case err := <-p.errorChan:
		require.ErrorContains(t, err, "received oversized message")
		require.ErrorContains(
			t,
			err,
			fmt.Sprintf("exceeding limit (%d bytes)", IdleMaxPendingMessageBytes),
		)
	case <-time.After(idleLimitObservationWindow):
		t.Fatal("an oversized block in Idle was not rejected")
	}
}
