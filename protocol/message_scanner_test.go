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
		require.Equal(t, uint8(1), msg.message.Type())
	case err := <-errors:
		t.Fatalf("readLoop rejected a pipelined reply: %v", err)
	case <-time.After(time.Second):
		t.Fatal("readLoop did not enqueue the pipelined reply")
	}
}

func TestReadLoopLimitsRepliesToOutstandingPipelinedRequests(t *testing.T) {
	state := NewState(1, "Idle")
	var decodeCalls int
	p, errors := newReadLoopTestProtocol(
		t,
		state,
		AgencyClient,
		10,
		2,
		[]byte{0x81, 0x01, 0x81, 0x01, 0x81, 0x01},
		func(messageType uint, _ []byte) (Message, error) {
			decodeCalls++
			return &MessageBase{MessageType: uint8(messageType)}, nil
		},
	)
	for range 2 {
		select {
		case msg := <-p.recvQueueChan:
			require.Equal(t, uint8(1), msg.message.Type())
		case <-time.After(time.Second):
			t.Fatal("readLoop did not enqueue an accounted pipelined reply")
		}
	}
	select {
	case err := <-errors:
		require.ErrorContains(t, err, "without peer agency")
	case <-time.After(time.Second):
		t.Fatal("readLoop admitted a reply without an outstanding request")
	}
	require.Equal(t, 2, decodeCalls)
}

func TestReadLoopConsumesPeerAgencyBeforeNextCoalescedReply(t *testing.T) {
	const (
		requestType = uint8(0)
		replyType   = uint8(1)
	)
	idle := NewState(1, "Idle")
	busy := NewState(2, "Busy")
	errorChan := make(chan error, 1)
	segments := make(chan *muxer.Segment, 1)
	segment := muxer.NewSegment(
		1,
		[]byte{0x81, replyType, 0x81, replyType},
		false,
	)
	require.NotNil(t, segment)
	segments <- segment
	var decodeCalls int
	handled := make(chan Message, 1)
	allowNextRequest := make(chan struct{})
	defer close(allowNextRequest)
	var p *Protocol
	p = &Protocol{
		config: ProtocolConfig{
			Name:      "test",
			ErrorChan: errorChan,
			Role:      ProtocolRoleClient,
			MessageFromCborFunc: func(messageType uint, _ []byte) (Message, error) {
				decodeCalls++
				return &MessageBase{MessageType: uint8(messageType)}, nil
			},
			MessageHandlerFunc: func(msg Message) error {
				handled <- msg
				<-allowNextRequest
				return p.transitionState(&MessageBase{MessageType: requestType})
			},
			StateMap: StateMap{
				idle: {
					Agency: AgencyClient,
					Transitions: []StateTransition{{
						MsgType:  requestType,
						NewState: busy,
					}},
				},
				busy: {
					Agency: AgencyServer,
					Transitions: []StateTransition{{
						MsgType:  replyType,
						NewState: idle,
					}},
				},
			},
			InitialState:      busy,
			MaxReadBufferSize: 1 << 20,
		},
		doneChan:             make(chan struct{}),
		stopChan:             make(chan struct{}),
		muxerDoneChan:        make(chan bool),
		recvDoneChan:         make(chan struct{}),
		sendDoneChan:         make(chan struct{}),
		muxerRecvChan:        segments,
		recvQueueChan:        make(chan inboundMessage, 2),
		recvStateChangedChan: make(chan struct{}, 1),
		sendReadyChan:        make(chan bool, 1),
		currentState:         busy,
	}
	stateTransitions := make(chan protocolStateTransition)
	p.stateTransitionChan = stateTransitions
	go p.stateLoop(stateTransitions)
	go p.recvLoop()
	go p.readLoop()
	t.Cleanup(p.Stop)

	select {
	case msg := <-handled:
		require.Equal(t, replyType, msg.Type())
	case <-time.After(time.Second):
		t.Fatal("recvLoop did not handle the first peer reply")
	}
	select {
	case err := <-errorChan:
		require.ErrorContains(t, err, "without peer agency")
	case <-time.After(time.Second):
		t.Fatal("readLoop queued a second reply under the consumed agency grant")
	}
	require.Equal(t, 1, decodeCalls)
}

func TestInboundPipelinedReplySurvivesPeerToLocalStateAdvance(t *testing.T) {
	busy := NewState(1, "Busy")
	idle := NewState(2, "Idle")
	p := &Protocol{
		config: ProtocolConfig{
			Name: "test",
			Role: ProtocolRoleClient,
			StateMap: StateMap{
				busy: {Agency: AgencyServer},
				idle: {Agency: AgencyClient},
			},
		},
		stopChan:                 make(chan struct{}),
		sendDoneChan:             make(chan struct{}),
		muxerDoneChan:            make(chan bool),
		currentState:             idle,
		currentStateGeneration:   2,
		pendingPipelinedRequests: 1,
	}

	state, admission, result, stopped := p.waitForInboundAdmission(
		1,
		busy,
		1,
		true,
	)
	require.False(t, stopped)
	require.Equal(t, idle, state)
	require.Equal(t, inboundAdmissionGranted, result)
	require.True(t, admission.reservedPipelinedReply)
}

func TestInboundAdmissionWaitCannotUseLaterPipelinedRequest(t *testing.T) {
	const batchDoneType = uint8(2)
	idle := NewState(1, "Idle")
	busy := NewState(2, "Busy")
	p := &Protocol{
		config: ProtocolConfig{
			Name:         "test",
			Role:         ProtocolRoleClient,
			InitialState: busy,
			StateMap: StateMap{
				idle: {Agency: AgencyClient},
				busy: {
					Agency: AgencyServer,
					Transitions: []StateTransition{{
						MsgType:  batchDoneType,
						NewState: idle,
					}},
				},
			},
		},
		doneChan:               make(chan struct{}),
		stopChan:               make(chan struct{}),
		sendDoneChan:           make(chan struct{}),
		muxerDoneChan:          make(chan bool),
		recvStateChangedChan:   make(chan struct{}, 1),
		currentState:           busy,
		currentStateGeneration: 1,
		peerAgencyReserved:     true,
	}
	transitions := make(chan protocolStateTransition)
	p.stateTransitionChan = transitions
	go p.stateLoop(transitions)
	t.Cleanup(p.Stop)
	select {
	case <-p.recvStateChangedChan:
	case <-time.After(time.Second):
		t.Fatal("state loop did not publish its initial state")
	}

	type admissionResult struct {
		admission inboundAdmission
		result    inboundAdmissionResult
	}
	result := make(chan admissionResult, 1)
	waiting := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_, admission, got, stopped := p.waitForInboundAdmissionWithWait(
			1,
			busy,
			1,
			false,
			func() bool {
				close(waiting)
				<-release
				return true
			},
		)
		if stopped {
			result <- admissionResult{result: inboundAdmissionDenied}
			return
		}
		result <- admissionResult{admission: admission, result: got}
	}()
	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("admission did not reach the wait state")
	}
	require.NoError(t, p.transitionInboundState(
		&MessageBase{MessageType: batchDoneType},
		false,
	))
	p.addPendingPipelinedRequest()
	close(release)
	select {
	case got := <-result:
		require.Equal(t, inboundAdmissionDenied, got.result)
		require.False(t, got.admission.reservedPipelinedReply)
	case <-time.After(time.Second):
		t.Fatal("inbound admission did not retry after the deferred transition")
	}
}

func TestInboundStaleReplyCannotReserveLaterPipelinedRequest(t *testing.T) {
	busy := NewState(1, "Busy")
	idle := NewState(2, "Idle")
	p := &Protocol{
		config: ProtocolConfig{
			Name: "test",
			Role: ProtocolRoleClient,
			StateMap: StateMap{
				busy: {Agency: AgencyServer},
				idle: {Agency: AgencyClient},
			},
		},
		stopChan:                 make(chan struct{}),
		sendDoneChan:             make(chan struct{}),
		muxerDoneChan:            make(chan bool),
		currentState:             idle,
		currentStateGeneration:   3,
		pendingPipelinedRequests: 1,
	}

	_, admission, result, stopped := p.waitForInboundAdmission(1, busy, 1, false)
	require.False(t, stopped)
	require.Equal(t, inboundAdmissionDenied, result)
	require.False(t, admission.reservedPipelinedReply)
	require.Zero(t, p.pendingPipelinedReplies)
}

func TestInboundPipelinedReplyPreservesStreamingAgency(t *testing.T) {
	const replyType = uint8(1)
	streaming := NewState(1, "Streaming")
	p := &Protocol{
		config: ProtocolConfig{
			Name: "test",
			Role: ProtocolRoleClient,
			StateMap: StateMap{
				streaming: {
					Agency: AgencyServer,
					Transitions: []StateTransition{{
						MsgType:  replyType,
						NewState: streaming,
					}},
				},
			},
			InitialState: streaming,
		},
		doneChan:                 make(chan struct{}),
		stopChan:                 make(chan struct{}),
		sendReadyChan:            make(chan bool, 1),
		recvStateChangedChan:     make(chan struct{}, 1),
		currentState:             streaming,
		peerAgencyReserved:       true,
		pendingPipelinedRequests: 1,
		pendingPipelinedReplies:  1,
	}
	stateTransitions := make(chan protocolStateTransition)
	p.stateTransitionChan = stateTransitions
	go p.stateLoop(stateTransitions)
	t.Cleanup(p.Stop)

	err := p.transitionInboundState(
		&MessageBase{MessageType: replyType},
		true,
	)
	require.NoError(t, err)
	_, _, result := p.reserveInboundAdmission(
		uint(replyType),
		streaming,
		1,
		true,
	)
	require.Equal(t, inboundAdmissionGranted, result)
}

func TestPipelinedRequestTransitionConsumesAdmissionSlot(t *testing.T) {
	const (
		requestType = uint8(0)
		replyType   = uint8(1)
	)
	idle := NewState(1, "Idle")
	busy := NewState(2, "Busy")
	for _, test := range []struct {
		name                  string
		pendingPipelinedReply bool
		wantFirst             inboundAdmissionResult
	}{
		{
			name:      "new reply uses current grant",
			wantFirst: inboundAdmissionGranted,
		},
		{
			name:                  "reserved reply owns current grant",
			pendingPipelinedReply: true,
			wantFirst:             inboundAdmissionWait,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			pendingReplies := 0
			if test.pendingPipelinedReply {
				pendingReplies = 1
			}
			p := &Protocol{
				config: ProtocolConfig{
					Name: "test",
					Role: ProtocolRoleClient,
					StateMap: StateMap{
						idle: {
							Agency: AgencyClient,
							Transitions: []StateTransition{{
								MsgType:  requestType,
								NewState: busy,
							}},
						},
						busy: {Agency: AgencyServer},
					},
					InitialState: idle,
				},
				doneChan:                 make(chan struct{}),
				stopChan:                 make(chan struct{}),
				sendReadyChan:            make(chan bool, 1),
				recvStateChangedChan:     make(chan struct{}, 1),
				currentState:             idle,
				pendingPipelinedRequests: 1,
				pendingPipelinedReplies:  pendingReplies,
			}
			stateTransitions := make(chan protocolStateTransition)
			p.stateTransitionChan = stateTransitions
			go p.stateLoop(stateTransitions)
			t.Cleanup(p.Stop)

			err := p.transitionPipelinedRequestState(
				&MessageBase{MessageType: requestType},
			)
			require.NoError(t, err)
			_, _, result := p.reserveInboundAdmission(
				uint(replyType),
				busy,
				1,
				false,
			)
			require.Equal(t, test.wantFirst, result)
			if result == inboundAdmissionGranted {
				_, _, result = p.reserveInboundAdmission(
					uint(replyType),
					busy,
					1,
					false,
				)
				require.Equal(t, inboundAdmissionWait, result)
			}
		})
	}
}

func TestReadLoopUsesDeclaredPipelinedMessageTypes(t *testing.T) {
	for _, test := range []struct {
		name       string
		message    byte
		wantAccept bool
	}{
		{name: "declared message", message: 2, wantAccept: true},
		{name: "undeclared message", message: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := NewState(1, "Busy")
			var decodeCalls int
			p, errors := newReadLoopTestProtocol(
				t,
				state,
				AgencyClient,
				10,
				0,
				[]byte{0x81, test.message},
				func(messageType uint, _ []byte) (Message, error) {
					decodeCalls++
					return &MessageBase{MessageType: uint8(messageType)}, nil
				},
				2,
			)
			if test.wantAccept {
				select {
				case msg := <-p.recvQueueChan:
					require.Equal(t, test.message, msg.message.Type())
					require.Equal(t, 1, decodeCalls)
				case err := <-errors:
					t.Fatalf("readLoop rejected a declared pipelined message: %v", err)
				case <-time.After(time.Second):
					t.Fatal("readLoop did not enqueue the declared pipelined message")
				}
				return
			}
			select {
			case err := <-errors:
				require.ErrorContains(t, err, "without peer agency")
			case <-time.After(time.Second):
				t.Fatal("readLoop accepted an undeclared pipelined message")
			}
			require.Zero(t, decodeCalls)
		})
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
	pipelinedMessageTypes ...uint8,
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
				PipelinedMessageTypes:   pipelinedMessageTypes,
			}},
			InitialState:      state,
			MaxReadBufferSize: 1 << 20,
		},
		doneChan:                 make(chan struct{}),
		stopChan:                 make(chan struct{}),
		muxerDoneChan:            make(chan bool),
		sendDoneChan:             make(chan struct{}),
		muxerRecvChan:            segments,
		recvQueueChan:            make(chan inboundMessage, 1),
		currentState:             state,
		pendingPipelinedRequests: pendingPipelinedRequests,
	}
	t.Cleanup(p.Stop)
	go p.readLoop()
	return p, errorChan
}
