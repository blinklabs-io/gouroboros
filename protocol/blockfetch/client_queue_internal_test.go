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
	"net"
	"testing"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
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

func (c *Client) appendTestRequest(reservedBytes uint64) *rangeRequest {
	c.queueMutex.Lock()
	defer c.queueMutex.Unlock()
	c.nextRequestId++
	req := &rangeRequest{
		id:            c.nextRequestId,
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

// TestAwaitInFlightCapacityAdmitsOversizedRequestOnEmptyQueue verifies a range
// whose expected size exceeds the whole bound is still admitted when nothing
// else is outstanding, so an oversized range cannot stall forever.
func TestAwaitInFlightCapacityAdmitsOversizedRequestOnEmptyQueue(t *testing.T) {
	c := newQueueTestClient(&Config{
		RequestPipelining: true,
		MaxInFlightBytes:  1024,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, c.awaitInFlightCapacity(ctx, 1024*1024))
}

// TestAwaitInFlightCapacityBlocksAtBound verifies that a request which does
// not fit is held, and that the wait is released by the caller's context
// rather than being admitted anyway.
func TestAwaitInFlightCapacityBlocksAtBound(t *testing.T) {
	c := newQueueTestClient(&Config{
		RequestPipelining: true,
		MaxInFlightBytes:  1024,
	})
	req := c.appendTestRequest(1024)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, c.awaitInFlightCapacity(ctx, 1024), context.Canceled)

	// Releasing the outstanding request makes room again
	c.queueMutex.Lock()
	c.removeLocked(req)
	c.queueMutex.Unlock()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	require.NoError(t, c.awaitInFlightCapacity(ctx2, 1024))
}

// TestAwaitInFlightCapacityDefaultsBound verifies an unset MaxInFlightBytes
// resolves to the documented default rather than to zero, which would admit
// nothing.
func TestAwaitInFlightCapacityDefaultsBound(t *testing.T) {
	c := newQueueTestClient(&Config{RequestPipelining: true})
	require.Equal(t, DefaultMaxInFlightBytes, c.maxInFlightBytes())
}
