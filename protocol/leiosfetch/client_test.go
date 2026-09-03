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

package leiosfetch

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}

	client := NewClient(protoOptions, nil)

	require.NotNil(t, client)
	assert.NotNil(t, client.Protocol)
	assert.NotNil(t, client.config)
}

func TestNewClientWithConfig(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}
	cfg := NewConfig(
		WithTimeout(10 * time.Second),
	)

	client := NewClient(protoOptions, &cfg)

	require.NotNil(t, client)
	assert.Equal(t, 10*time.Second, client.config.Timeout)
}

func TestClientMessageHandler(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}
	client := NewClient(protoOptions, nil)

	testCases := []struct {
		name        string
		msg         protocol.Message
		expectError bool
	}{
		{
			name:        "Block message",
			msg:         NewMsgBlock([]byte{0x82, 0x01, 0x02}),
			expectError: false,
		},
		{
			name:        "NoBlock message",
			msg:         NewMsgNoBlock(),
			expectError: false,
		},
		{
			name:        "BlockTxs message",
			msg:         NewMsgBlockTxs(nil),
			expectError: false,
		},
		{
			name:        "NoBlockTxs message",
			msg:         NewMsgNoBlockTxs(),
			expectError: false,
		},
		{
			name:        "Votes message",
			msg:         NewMsgVotes(nil),
			expectError: false,
		},
		{
			name:        "NextBlockAndTxsInRange message",
			msg:         NewMsgNextBlockAndTxsInRange(nil, nil),
			expectError: false,
		},
		{
			name:        "LastBlockAndTxsInRange message",
			msg:         NewMsgLastBlockAndTxsInRange(nil, nil),
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// The Block/BlockTxs handlers deliver results to a per-request slot
			// (a non-blocking send), so register a waiter first. The
			// Every response handler delivers through the request slot. Register
			// a waiter before invoking the handler so delivery is correlated with
			// the request under test.
			var deliverCh chan protocol.Message
			switch tc.msg.Type() {
			case MessageTypeBlock, MessageTypeNoBlock, MessageTypeBlockTxs, MessageTypeNoBlockTxs,
				MessageTypeVotes, MessageTypeNextBlockAndTxsInRange, MessageTypeLastBlockAndTxsInRange:
				w, err := client.blockRequestSlot.acquire(
					context.Background(), client.DoneChan(),
				)
				require.NoError(t, err)
				deliverCh = w
			}
			errCh := make(chan error, 1)
			go func() {
				errCh <- client.messageHandler(tc.msg)
			}()

			// Receive the routed message under a bounded deadline.
			select {
			case m := <-deliverCh:
				assert.Equal(t, tc.msg, m)
			case <-time.After(time.Second):
				t.Fatal("handler did not route message")
			}

			// Confirm the handler returned (and its error expectation) under a
			// bounded deadline so the helper goroutine cannot dangle.
			select {
			case err := <-errCh:
				if tc.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			case <-time.After(time.Second):
				t.Fatal("handler did not return")
			}
		})
	}
}

// TestRequestSlotAbandonAfterDeliverKeepsNextWaiter reproduces the
// deliver-vs-context-cancel race between two requests on the same slot.
//
// Request A registers its waiter and its response is delivered (freeing the
// slot). A's context also fires, so A's select may pick the cancellation branch
// and call abandon with its own delivery channel. Before that abandon runs,
// request B acquires the freed slot and registers its own waiter. abandon must
// only clear the waiter it still owns: passing A's channel while B owns the slot
// must be a no-op, so B's response is delivered to B rather than dropped.
//
// The ordering (deliver A, acquire B, then abandon A) is driven explicitly so
// the regression is exercised deterministically rather than via scheduler luck.
func TestRequestSlotAbandonAfterDeliverKeepsNextWaiter(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	client := NewClient(protocol.ProtocolOptions{ConnectionId: connId}, nil)

	// Request A acquires the slot and registers its delivery channel.
	wA, err := client.blockRequestSlot.acquire(context.Background(), client.DoneChan())
	require.NoError(t, err)

	// A's response arrives: delivered to wA and the slot is freed.
	msgA := NewMsgBlock([]byte{0xaa, 0xbb})
	require.True(t, client.blockRequestSlot.deliver(msgA))

	// Request B acquires the now-free slot and registers its own channel.
	wB, err := client.blockRequestSlot.acquire(context.Background(), client.DoneChan())
	require.NoError(t, err)

	// A's late context cancellation abandons using A's own channel. B owns the
	// slot now, so this must not disturb B's registration.
	client.blockRequestSlot.abandon(wA)

	// B's response must be delivered to B (not dropped).
	msgB := NewMsgBlock([]byte{0xcc, 0xdd})
	require.True(
		t,
		client.blockRequestSlot.deliver(msgB),
		"B's response was dropped: abandon(wA) erased B's waiter",
	)

	// B receives its own payload, not A's.
	select {
	case got := <-wB:
		gotBlock, ok := got.(*MsgBlock)
		require.True(t, ok)
		assert.Equal(t, []byte{0xcc, 0xdd}, []byte(gotBlock.BlockRaw))
	case <-time.After(time.Second):
		t.Fatal("request B did not receive its response")
	}

	// A's earlier response is still buffered on its own channel, unaffected.
	select {
	case got := <-wA:
		gotBlock, ok := got.(*MsgBlock)
		require.True(t, ok)
		assert.Equal(t, []byte{0xaa, 0xbb}, []byte(gotBlock.BlockRaw))
	case <-time.After(time.Second):
		t.Fatal("request A's buffered response went missing")
	}
}

// TestRequestSlotAbandonWakesExistingAcquirer verifies that an acquirer which
// was already waiting for a non-abandoned request to drain observes a later
// abandonment and applies the bounded grace period. Otherwise it remains
// blocked on the old drain channel until a response arrives, bypassing
// ErrRequestSlotAbandoned entirely.
func TestRequestSlotAbandonWakesExistingAcquirer(t *testing.T) {
	client := NewClient(protocol.ProtocolOptions{}, nil)
	wA, err := client.blockRequestSlot.acquire(context.Background(), client.DoneChan())
	require.NoError(t, err)

	bWaiting := make(chan struct{})
	client.blockRequestSlot.beforeDrainWait = func() {
		close(bWaiting)
	}
	bResult := make(chan error, 1)
	ctxB, cancelB := context.WithTimeout(
		context.Background(),
		2*abandonedRequestWait,
	)
	defer cancelB()
	go func() {
		_, err := client.blockRequestSlot.acquire(ctxB, client.DoneChan())
		bResult <- err
	}()
	<-bWaiting

	client.blockRequestSlot.abandon(wA)
	require.ErrorIs(t, <-bResult, ErrRequestSlotAbandoned)
}

// TestRequestSlotReleaseAfterReacquireKeepsNextWaiter is the release-path
// analogue of the abandon race: once the slot has been freed and reacquired by
// a newer request, calling release with the previous request's channel must be
// a no-op and must not free the slot out from under the newer waiter.
func TestRequestSlotReleaseAfterReacquireKeepsNextWaiter(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	client := NewClient(protocol.ProtocolOptions{ConnectionId: connId}, nil)

	wA, err := client.blockRequestSlot.acquire(context.Background(), client.DoneChan())
	require.NoError(t, err)
	// Free the slot via a delivery, then reacquire for request B.
	require.True(t, client.blockRequestSlot.deliver(NewMsgBlock([]byte{0x01})))
	<-wA
	wB, err := client.blockRequestSlot.acquire(context.Background(), client.DoneChan())
	require.NoError(t, err)

	// A stale release using A's channel must not touch B's registration.
	client.blockRequestSlot.release(wA)

	msgB := NewMsgBlock([]byte{0x02})
	require.True(
		t,
		client.blockRequestSlot.deliver(msgB),
		"B's response was dropped: release(wA) erased B's waiter",
	)
	select {
	case got := <-wB:
		gotBlock, ok := got.(*MsgBlock)
		require.True(t, ok)
		assert.Equal(t, []byte{0x02}, []byte(gotBlock.BlockRaw))
	case <-time.After(time.Second):
		t.Fatal("request B did not receive its response")
	}
}

func TestClientMessageHandlerUnexpectedType(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}
	client := NewClient(protoOptions, nil)

	// Create a message with an unexpected type
	msg := NewMsgBlockRequest(pcommon.NewPoint(123, []byte{0x01, 0x02}))

	err := client.messageHandler(msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected message type")
}

func TestStateMap(t *testing.T) {
	// Test that all expected states exist
	assert.Equal(t, uint(1), StateIdle.Id)
	assert.Equal(t, uint(2), StateBlock.Id)
	assert.Equal(t, uint(3), StateBlockTxs.Id)
	assert.Equal(t, uint(4), StateVotes.Id)
	assert.Equal(t, uint(5), StateBlockRange.Id)
	assert.Equal(t, uint(6), StateDone.Id)

	// Test StateMap entries
	assert.Contains(t, StateMap, StateIdle)
	assert.Contains(t, StateMap, StateBlock)
	assert.Contains(t, StateMap, StateBlockTxs)
	assert.Contains(t, StateMap, StateVotes)
	assert.Contains(t, StateMap, StateBlockRange)
	assert.Contains(t, StateMap, StateDone)

	// Test agency for each state
	assert.Equal(t, protocol.AgencyClient, StateMap[StateIdle].Agency)
	assert.Equal(t, protocol.AgencyServer, StateMap[StateBlock].Agency)
	assert.Equal(t, protocol.AgencyServer, StateMap[StateBlockTxs].Agency)
	assert.Equal(t, protocol.AgencyServer, StateMap[StateVotes].Agency)
	assert.Equal(t, protocol.AgencyServer, StateMap[StateBlockRange].Agency)
	assert.Equal(t, protocol.AgencyNone, StateMap[StateDone].Agency)
}

func TestStateTransitions(t *testing.T) {
	// Test transitions from Idle state
	idleEntry := StateMap[StateIdle]
	require.Len(t, idleEntry.Transitions, 5)

	expectedTransitions := map[uint8]protocol.State{
		MessageTypeBlockRequest:      StateBlock,
		MessageTypeBlockTxsRequest:   StateBlockTxs,
		MessageTypeVotesRequest:      StateVotes,
		MessageTypeBlockRangeRequest: StateBlockRange,
		MessageTypeDone:              StateDone,
	}

	for _, trans := range idleEntry.Transitions {
		expected, ok := expectedTransitions[trans.MsgType]
		require.True(t, ok, "unexpected transition message type: %d", trans.MsgType)
		assert.Equal(t, expected, trans.NewState)
	}

	// Test transitions from Block state: both Block and NoBlock return to Idle
	blockEntry := StateMap[StateBlock]
	require.Len(t, blockEntry.Transitions, 2)
	blockTransitions := map[uint8]protocol.State{}
	for _, trans := range blockEntry.Transitions {
		blockTransitions[trans.MsgType] = trans.NewState
	}
	assert.Equal(t, StateIdle, blockTransitions[MessageTypeBlock])
	assert.Equal(t, StateIdle, blockTransitions[MessageTypeNoBlock])

	// Test transitions from BlockTxs state: both BlockTxs and NoBlockTxs return to Idle
	blockTxsEntry := StateMap[StateBlockTxs]
	require.Len(t, blockTxsEntry.Transitions, 2)
	blockTxsTransitions := map[uint8]protocol.State{}
	for _, trans := range blockTxsEntry.Transitions {
		blockTxsTransitions[trans.MsgType] = trans.NewState
	}
	assert.Equal(t, StateIdle, blockTxsTransitions[MessageTypeBlockTxs])
	assert.Equal(t, StateIdle, blockTxsTransitions[MessageTypeNoBlockTxs])

	// Test transitions from BlockRange state
	blockRangeEntry := StateMap[StateBlockRange]
	require.Len(t, blockRangeEntry.Transitions, 2)
}

func TestConfig(t *testing.T) {
	// Test default config
	cfg := NewConfig()
	assert.Equal(t, 5*time.Second, cfg.Timeout)
	assert.Nil(t, cfg.BlockRequestFunc)
	assert.Nil(t, cfg.BlockTxsRequestFunc)
	assert.Nil(t, cfg.VotesRequestFunc)
	assert.Nil(t, cfg.BlockRangeRequestFunc)

	// Test config with options
	blockRequestCalled := false
	blockTxsRequestCalled := false
	votesRequestCalled := false
	blockRangeRequestCalled := false

	cfg = NewConfig(
		WithTimeout(30*time.Second),
		WithBlockRequestFunc(func(ctx CallbackContext, point pcommon.Point) (protocol.Message, error) {
			blockRequestCalled = true
			return nil, nil
		}),
		WithBlockTxsRequestFunc(func(ctx CallbackContext, point pcommon.Point, bitmaps map[uint16]uint64) (protocol.Message, error) {
			blockTxsRequestCalled = true
			return nil, nil
		}),
		WithVotesRequestFunc(func(ctx CallbackContext, voteIds []MsgVotesRequestVoteId) (protocol.Message, error) {
			votesRequestCalled = true
			return nil, nil
		}),
		WithBlockRangeRequestFunc(func(ctx CallbackContext, start, end pcommon.Point) error {
			blockRangeRequestCalled = true
			return nil
		}),
	)

	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.NotNil(t, cfg.BlockRequestFunc)
	assert.NotNil(t, cfg.BlockTxsRequestFunc)
	assert.NotNil(t, cfg.VotesRequestFunc)
	assert.NotNil(t, cfg.BlockRangeRequestFunc)

	// Test that callbacks can be invoked
	_, _ = cfg.BlockRequestFunc(CallbackContext{}, pcommon.NewPoint(0, nil))
	assert.True(t, blockRequestCalled)

	_, _ = cfg.BlockTxsRequestFunc(CallbackContext{}, pcommon.NewPoint(0, nil), nil)
	assert.True(t, blockTxsRequestCalled)

	_, _ = cfg.VotesRequestFunc(CallbackContext{}, nil)
	assert.True(t, votesRequestCalled)

	_ = cfg.BlockRangeRequestFunc(CallbackContext{}, pcommon.Point{}, pcommon.Point{})
	assert.True(t, blockRangeRequestCalled)
}

func TestProtocolConstants(t *testing.T) {
	assert.Equal(t, "leios-fetch", ProtocolName)
	assert.Equal(t, uint16(19), ProtocolId)
}
