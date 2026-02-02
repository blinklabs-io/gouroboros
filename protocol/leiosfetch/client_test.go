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
			name:        "BlockTxs message",
			msg:         NewMsgBlockTxs(nil),
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
			// For block-related messages, we need to drain the channels
			// to prevent blocking. We use goroutines to consume the messages.
			done := make(chan struct{})
			go func() {
				defer close(done)
				switch tc.msg.Type() {
				case MessageTypeBlock:
					<-client.blockResultChan
				case MessageTypeBlockTxs:
					<-client.blockTxsResultChan
				case MessageTypeVotes:
					<-client.votesResultChan
				case MessageTypeNextBlockAndTxsInRange, MessageTypeLastBlockAndTxsInRange:
					<-client.blockRangeResultChan
				}
			}()

			err := client.messageHandler(tc.msg)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Wait for the channel consumer goroutine to finish
			select {
			case <-done:
			case <-time.After(100 * time.Millisecond):
				t.Fatal("timeout waiting for channel consumer")
			}
		})
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
	msg := NewMsgBlockRequest(123, []byte{0x01, 0x02})

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

	// Test transitions from Block state
	blockEntry := StateMap[StateBlock]
	require.Len(t, blockEntry.Transitions, 1)
	assert.Equal(t, uint8(MessageTypeBlock), blockEntry.Transitions[0].MsgType)
	assert.Equal(t, StateIdle, blockEntry.Transitions[0].NewState)

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
		WithBlockRequestFunc(func(ctx CallbackContext, slot uint64, hash []byte) (protocol.Message, error) {
			blockRequestCalled = true
			return nil, nil
		}),
		WithBlockTxsRequestFunc(func(ctx CallbackContext, slot uint64, hash []byte, bitmaps map[uint16][8]byte) (protocol.Message, error) {
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
	_, _ = cfg.BlockRequestFunc(CallbackContext{}, 0, nil)
	assert.True(t, blockRequestCalled)

	_, _ = cfg.BlockTxsRequestFunc(CallbackContext{}, 0, nil, nil)
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
