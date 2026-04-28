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

package leiosnotify

import (
	"net"
	"sync/atomic"
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
		WithTimeout(30 * time.Second),
	)

	client := NewClient(protoOptions, &cfg)

	require.NotNil(t, client)
	assert.Equal(t, 30*time.Second, client.config.Timeout)
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
			name:        "BlockAnnouncement message",
			msg:         NewMsgBlockAnnouncement([]byte{0x82, 0x01, 0x02}),
			expectError: false,
		},
		{
			name:        "BlockOffer message",
			msg:         NewMsgBlockOffer(pcommon.NewPoint(12345, []byte{0x01, 0x02, 0x03, 0x04}), 12345),
			expectError: false,
		},
		{
			name:        "BlockTxsOffer message",
			msg:         NewMsgBlockTxsOffer(pcommon.NewPoint(12345, []byte{0x01, 0x02, 0x03, 0x04})),
			expectError: false,
		},
		{
			name:        "VotesOffer message",
			msg:         NewMsgVotesOffer(nil),
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// For notification messages, we need to drain the channel
			// to prevent blocking. We use a goroutine to consume the message.
			done := make(chan struct{})
			go func() {
				defer close(done)
				<-client.notificationChan
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
	msg := NewMsgNotificationRequestNext()

	err := client.messageHandler(msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected message type")
}

func TestStateMap(t *testing.T) {
	// Test that all expected states exist
	assert.Equal(t, uint(1), StateIdle.Id)
	assert.Equal(t, uint(2), StateBusy.Id)
	assert.Equal(t, uint(3), StateDone.Id)

	// Test StateMap entries
	assert.Contains(t, StateMap, StateIdle)
	assert.Contains(t, StateMap, StateBusy)
	assert.Contains(t, StateMap, StateDone)

	// Test agency for each state
	assert.Equal(t, protocol.AgencyClient, StateMap[StateIdle].Agency)
	assert.Equal(t, protocol.AgencyServer, StateMap[StateBusy].Agency)
	assert.Equal(t, protocol.AgencyNone, StateMap[StateDone].Agency)
}

func TestStateTransitions(t *testing.T) {
	// Test transitions from Idle state
	idleEntry := StateMap[StateIdle]
	require.Len(t, idleEntry.Transitions, 2)

	expectedTransitions := map[uint8]protocol.State{
		MessageTypeNotificationRequestNext: StateBusy,
		MessageTypeDone:                    StateDone,
	}

	for _, trans := range idleEntry.Transitions {
		expected, ok := expectedTransitions[trans.MsgType]
		require.True(t, ok, "unexpected transition message type: %d", trans.MsgType)
		assert.Equal(t, expected, trans.NewState)
	}

	// Test transitions from Busy state
	busyEntry := StateMap[StateBusy]
	require.Len(t, busyEntry.Transitions, 4)

	expectedBusyTransitions := map[uint8]protocol.State{
		MessageTypeBlockAnnouncement: StateIdle,
		MessageTypeBlockOffer:        StateIdle,
		MessageTypeBlockTxsOffer:     StateIdle,
		MessageTypeVotesOffer:        StateIdle,
	}

	for _, trans := range busyEntry.Transitions {
		expected, ok := expectedBusyTransitions[trans.MsgType]
		require.True(t, ok, "unexpected transition message type: %d", trans.MsgType)
		assert.Equal(t, expected, trans.NewState)
	}
}

func TestConfig(t *testing.T) {
	// Test default config
	cfg := NewConfig()
	assert.Equal(t, 60*time.Second, cfg.Timeout)
	assert.Nil(t, cfg.RequestNextFunc)

	// Test config with options
	requestNextCalled := false

	cfg = NewConfig(
		WithTimeout(120*time.Second),
		WithRequestNextFunc(func(ctx CallbackContext) (protocol.Message, error) {
			requestNextCalled = true
			return nil, nil
		}),
	)

	assert.Equal(t, 120*time.Second, cfg.Timeout)
	assert.NotNil(t, cfg.RequestNextFunc)

	// Test that callback can be invoked
	_, _ = cfg.RequestNextFunc(CallbackContext{})
	assert.True(t, requestNextCalled)
}

func TestProtocolConstants(t *testing.T) {
	assert.Equal(t, "leios-notify", ProtocolName)
	assert.Equal(t, uint16(18), ProtocolId)
}

func TestClientHandleBlockAnnouncement(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}
	client := NewClient(protoOptions, nil)

	msg := NewMsgBlockAnnouncement([]byte{0x82, 0x01, 0x02})

	// Start a goroutine to receive the message
	received := make(chan protocol.Message, 1)
	go func() {
		received <- <-client.notificationChan
	}()

	client.handleBlockAnnouncement(msg)

	select {
	case receivedMsg := <-received:
		assert.Equal(t, msg, receivedMsg)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for message")
	}
}

func TestClientHandleBlockOffer(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}
	client := NewClient(protoOptions, nil)

	msg := NewMsgBlockOffer(pcommon.NewPoint(12345, []byte{0x01, 0x02, 0x03, 0x04}), 12345)

	// Start a goroutine to receive the message
	received := make(chan protocol.Message, 1)
	go func() {
		received <- <-client.notificationChan
	}()

	client.handleBlockOffer(msg)

	select {
	case receivedMsg := <-received:
		assert.Equal(t, msg, receivedMsg)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for message")
	}
}

func TestClientHandleBlockTxsOffer(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}
	client := NewClient(protoOptions, nil)

	msg := NewMsgBlockTxsOffer(pcommon.NewPoint(12345, []byte{0x01, 0x02, 0x03, 0x04}))

	// Start a goroutine to receive the message
	received := make(chan protocol.Message, 1)
	go func() {
		received <- <-client.notificationChan
	}()

	client.handleBlockTxsOffer(msg)

	select {
	case receivedMsg := <-received:
		assert.Equal(t, msg, receivedMsg)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for message")
	}
}

func TestClientHandleVotesOffer(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}
	client := NewClient(protoOptions, nil)

	msg := NewMsgVotesOffer([]MsgVotesOfferVote{
		{Slot: 100, VoteIssuerId: []byte{0x01, 0x02, 0x03, 0x04}},
	})

	// Start a goroutine to receive the message
	received := make(chan protocol.Message, 1)
	go func() {
		received <- <-client.notificationChan
	}()

	client.handleVotesOffer(msg)

	select {
	case receivedMsg := <-received:
		assert.Equal(t, msg, receivedMsg)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for message")
	}
}

func TestSyncRequiresNotificationFunc(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}
	client := NewClient(protoOptions, nil)

	err := client.Sync()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NotificationFunc")
}

func TestSyncPreventsMultipleCalls(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}
	cfg := NewConfig(
		WithNotificationFunc(func(_ CallbackContext, _ protocol.Message) error {
			return nil
		}),
	)
	client := NewClient(protoOptions, &cfg)

	// Simulate that notification loop is already running
	client.busyMutex.Lock()
	client.notificationRunning = true
	client.busyMutex.Unlock()

	err := client.Sync()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}

func TestNotificationLoopDrainsOnStop(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}
	var callCount atomic.Int32
	cfg := NewConfig(
		WithNotificationFunc(func(_ CallbackContext, _ protocol.Message) error {
			callCount.Add(1)
			return ErrStopNotificationProcess
		}),
		WithPipelineLimit(5),
	)
	client := NewClient(protoOptions, &cfg)

	// Simulate pipelined state: 3 more responses expected
	client.busyMutex.Lock()
	client.pipelinedRequestNext = 3
	client.notificationRunning = true
	client.busyMutex.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		client.notificationLoop()
	}()

	// Send the first message (triggers ErrStopNotificationProcess)
	client.notificationChan <- NewMsgBlockAnnouncement([]byte{0x01})

	// Send 3 remaining pipelined messages (should be drained without calling NotificationFunc)
	for i := 0; i < 3; i++ {
		select {
		case client.notificationChan <- NewMsgBlockAnnouncement([]byte{byte(i + 2)}):
		case <-time.After(time.Second):
			t.Fatalf("timeout sending pipelined message %d - drain blocked", i+1)
		}
	}

	// notificationLoop should exit after draining
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for notificationLoop to exit")
	}

	assert.Equal(t, int32(1), callCount.Load(), "NotificationFunc should be called exactly once")

	// Verify notificationRunning was reset
	client.busyMutex.Lock()
	running := client.notificationRunning
	client.busyMutex.Unlock()
	assert.False(t, running, "notificationRunning should be reset after loop exit")
}

func TestNotificationLoopExitsOnChannelClose(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}
	cfg := NewConfig(
		WithNotificationFunc(func(_ CallbackContext, _ protocol.Message) error {
			return nil
		}),
	)
	client := NewClient(protoOptions, &cfg)

	client.busyMutex.Lock()
	client.notificationRunning = true
	client.busyMutex.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		client.notificationLoop()
	}()

	// Close the channel to simulate shutdown
	close(client.notificationChan)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for notificationLoop to exit on channel close")
	}

	client.busyMutex.Lock()
	running := client.notificationRunning
	client.busyMutex.Unlock()
	assert.False(t, running, "notificationRunning should be reset after loop exit")
}
