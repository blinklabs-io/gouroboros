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

package protocol

import (
	"bytes"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

func TestMessageScannerMakesLinearProgressAcrossSegments(t *testing.T) {
	const payloadSize = 32 * 1024
	message := []byte{0x82, 0x01, 0x59, 0x80, 0x00}
	message = append(message, bytes.Repeat([]byte{0x42}, payloadSize)...)

	scanner := messageScanner{}
	typeChecked := false
	var result messageScanResult
	for end := 1; end <= len(message); end++ {
		previousOffset := scanner.offset
		var err error
		result, err = scanner.scan(message[:end], 0)
		require.NoError(t, err)
		require.GreaterOrEqual(t, scanner.offset, previousOffset)
		require.LessOrEqual(t, scanner.offset, end)
		if result.hasMessageType && !typeChecked {
			require.Equal(t, uint(1), result.messageType)
			typeChecked = true
			result, err = scanner.scan(message[:end], 0)
			require.NoError(t, err)
		}
		require.Equal(t, scanner.offset, scanner.processedBytes)
	}
	require.True(t, result.complete)
	require.Equal(t, len(message), result.messageLength)
	require.Equal(t, len(message), scanner.processedBytes)
}

func TestMessageScannerHandlesHighCardinalityArray(t *testing.T) {
	const itemCount = 100_001
	message := []byte{0x9a, 0x00, 0x01, 0x86, 0xa1}
	message = append(message, 0x01)
	message = append(message, bytes.Repeat([]byte{0x00}, itemCount-1)...)

	scanner := messageScanner{}
	result, err := scanner.scan(message, 0)
	require.NoError(t, err)
	require.True(t, result.hasMessageType)
	require.Equal(t, uint(1), result.messageType)
	result, err = scanner.scan(message, 0)
	require.NoError(t, err)
	require.True(t, result.complete)
	require.Equal(t, len(message), result.messageLength)
	require.Equal(t, len(message), scanner.processedBytes)
}

func TestMessageScannerHandlesIndefiniteProtocolMessage(t *testing.T) {
	message := []byte{
		0x9f, 0x01, 0x5f, 0x42, 0xaa, 0xbb, 0x41, 0xcc, 0xff, 0xff,
	}
	scanner := messageScanner{}
	result, err := scanner.scan(message, 0)
	require.NoError(t, err)
	require.True(t, result.hasMessageType)
	result, err = scanner.scan(message, 0)
	require.NoError(t, err)
	require.True(t, result.complete)
	require.Equal(t, len(message), result.messageLength)
}

func TestReadLoopRejectsDeclaredOversizedMessageBeforeTypedDecode(
	t *testing.T,
) {
	var decodeCalls int
	state := NewState(1, "Busy")
	p, errors := newReadLoopTestProtocol(
		t,
		state,
		AgencyServer,
		2,
		0,
		[]byte{0x82, 0x01, 0x58, 0x64},
		func(messageType uint, _ []byte) (Message, error) {
			decodeCalls++
			return &MessageBase{MessageType: uint8(messageType)}, nil
		},
	)
	_ = p
	select {
	case err := <-errors:
		require.ErrorContains(t, err, "oversized message")
	case <-time.After(time.Second):
		t.Fatal("readLoop did not reject the oversized message")
	}
	require.Zero(t, decodeCalls)
}

func TestReadLoopRejectsOutOfAgencyMessageBeforeTypedDecode(t *testing.T) {
	var decodeCalls int
	state := NewState(1, "Idle")
	_, errors := newReadLoopTestProtocol(
		t,
		state,
		AgencyClient,
		10,
		0,
		[]byte{0x81, 0x01},
		func(messageType uint, _ []byte) (Message, error) {
			decodeCalls++
			return &MessageBase{MessageType: uint8(messageType)}, nil
		},
	)
	select {
	case err := <-errors:
		require.ErrorContains(t, err, "without peer agency")
	case <-time.After(time.Second):
		t.Fatal("readLoop did not reject the out-of-agency message")
	}
	require.Zero(t, decodeCalls)
}

func TestReadLoopAllowsMessageForOutstandingPipelinedRequest(t *testing.T) {
	state := NewState(1, "Idle")
	p, errors := newReadLoopTestProtocol(
		t,
		state,
		AgencyClient,
		10,
		1,
		[]byte{0x81, 0x01},
		func(messageType uint, _ []byte) (Message, error) {
			return &MessageBase{MessageType: uint8(messageType)}, nil
		},
	)
	select {
	case msg := <-p.recvQueueChan:
		require.Equal(t, uint8(1), msg.Type())
	case err := <-errors:
		t.Fatalf("readLoop rejected a pipelined reply: %v", err)
	case <-time.After(time.Second):
		t.Fatal("readLoop did not enqueue the pipelined reply")
	}
}

func TestReadLoopAppliesIdleSizeLimitWithOutstandingPipelinedRequest(
	t *testing.T,
) {
	var decodeCalls int
	state := NewState(1, "Idle")
	_, errors := newReadLoopTestProtocol(
		t,
		state,
		AgencyClient,
		2,
		1,
		[]byte{0x82, 0x01, 0x58, 0x64},
		func(messageType uint, _ []byte) (Message, error) {
			decodeCalls++
			return &MessageBase{MessageType: uint8(messageType)}, nil
		},
	)
	select {
	case err := <-errors:
		require.ErrorContains(t, err, "oversized message")
	case <-time.After(time.Second):
		t.Fatal("readLoop did not reject the oversized pipelined message")
	}
	require.Zero(t, decodeCalls)
}

func newReadLoopTestProtocol(
	t *testing.T,
	state State,
	agency ProtocolStateAgency,
	messageLimit int,
	pendingPipelinedRequests int,
	message []byte,
	decode MessageFromCborFunc,
) (*Protocol, chan error) {
	t.Helper()
	errorChan := make(chan error, 1)
	segments := make(chan *muxer.Segment, 1)
	segment := muxer.NewSegment(1, message, false)
	require.NotNil(t, segment)
	segments <- segment
	p := &Protocol{
		config: ProtocolConfig{
			Name:                "test",
			ErrorChan:           errorChan,
			Role:                ProtocolRoleClient,
			MessageFromCborFunc: decode,
			StateMap: StateMap{state: {
				Agency:                  agency,
				PendingMessageByteLimit: messageLimit,
			}},
			InitialState:      state,
			MaxReadBufferSize: 1 << 20,
		},
		doneChan:                 make(chan struct{}),
		stopChan:                 make(chan struct{}),
		muxerDoneChan:            make(chan bool),
		sendDoneChan:             make(chan struct{}),
		muxerRecvChan:            segments,
		recvQueueChan:            make(chan Message, 1),
		currentState:             state,
		pendingPipelinedRequests: pendingPipelinedRequests,
	}
	t.Cleanup(p.Stop)
	go p.readLoop()
	return p, errorChan
}
