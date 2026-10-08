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

package messagesubmission

import (
	"net"
	"testing"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconcileMessageIDsBindsReplyToRequest(t *testing.T) {
	idA := testMessageID(0xa1)
	idB := testMessageID(0xb2)
	idC := testMessageID(0xc3)
	tests := []struct {
		name    string
		reply   []pcommon.MessageIDAndSize
		wantErr error
	}{
		{
			name: "excess",
			reply: []pcommon.MessageIDAndSize{
				{MessageID: idB},
				{MessageID: idC},
			},
			wantErr: protocol.ErrProtocolViolationRequestExceeded,
		},
		{
			name: "duplicate",
			reply: []pcommon.MessageIDAndSize{
				{MessageID: idA},
			},
			wantErr: protocol.ErrProtocolViolationInvalidMessage,
		},
		{
			name: "malformed ID",
			reply: []pcommon.MessageIDAndSize{
				{MessageID: []byte("short")},
			},
			wantErr: protocol.ErrProtocolViolationInvalidMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reconcileMessageIDs(
				[][]byte{idA},
				messageIDRequest{requested: 1},
				tt.reply,
				DefaultMaxUnacknowledgedMessageIDs,
			)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestReconcileMessageIDsCommitsAcknowledgementAndReply(t *testing.T) {
	idA := testMessageID(0xa1)
	idB := testMessageID(0xb2)
	idC := testMessageID(0xc3)

	next, err := reconcileMessageIDs(
		[][]byte{idA, idB},
		messageIDRequest{ack: 1, requested: 1},
		[]pcommon.MessageIDAndSize{{MessageID: idC}},
		DefaultMaxUnacknowledgedMessageIDs,
	)

	require.NoError(t, err)
	assert.Equal(t, [][]byte{idB, idC}, next)
	idB[0] = 0xff
	idC[0] = 0xff
	assert.Equal(t, byte(0xb2), next[0][0])
	assert.Equal(t, byte(0xc3), next[1][0])
}

func TestRequestedMessagesMustBeOutstandingAndUnique(t *testing.T) {
	idA := testMessageID(0xa1)
	idB := testMessageID(0xb2)
	idC := testMessageID(0xc3)
	tests := []struct {
		name      string
		requested [][]byte
	}{
		{name: "unannounced", requested: [][]byte{idC}},
		{name: "duplicate", requested: [][]byte{idA, idA}},
		{name: "malformed", requested: [][]byte{[]byte("short")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := requestedMessagesAreOutstanding(
				[][]byte{idA, idB},
				tt.requested,
			)
			require.ErrorIs(
				t,
				err,
				protocol.ErrProtocolViolationInvalidMessage,
			)
		})
	}
}

func TestMessageReplyAllowsOmissionAndRequestIndependentOrdering(t *testing.T) {
	idA := testMessageID(0xa1)
	idB := testMessageID(0xb2)
	idC := testMessageID(0xc3)
	request, err := requestedMessagesAreOutstanding(
		[][]byte{idA, idB, idC},
		[][]byte{idA, idB, idC},
	)
	require.NoError(t, err)

	err = validateMessageReply(request, []pcommon.DmqMessage{
		{MessageID: idC},
		{MessageID: idA},
	})

	require.NoError(t, err)
}

func TestMessageIDReplyAllowsFewerEntriesThanRequested(t *testing.T) {
	next, err := reconcileMessageIDs(
		[][]byte{},
		messageIDRequest{blocking: true, requested: 3},
		[]pcommon.MessageIDAndSize{{MessageID: testMessageID(0xa1)}},
		DefaultMaxUnacknowledgedMessageIDs,
	)

	require.NoError(t, err)
	require.Len(t, next, 1)
}

func TestMessageIDRequestCountRules(t *testing.T) {
	id := testMessageID(0xa1)
	tests := []struct {
		name        string
		outstanding [][]byte
		request     messageIDRequest
		wantErr     bool
	}{
		{
			name:    "blocking request requires a positive count",
			request: messageIDRequest{blocking: true},
			wantErr: true,
		},
		{
			name:    "non-blocking zero counts are invalid",
			request: messageIDRequest{},
			wantErr: true,
		},
		{
			name:    "non-blocking request can request first IDs",
			request: messageIDRequest{requested: 1},
		},
		{
			name:        "non-blocking request can only acknowledge IDs",
			outstanding: [][]byte{id},
			request:     messageIDRequest{ack: 1},
		},
		{
			name:    "negative request count is invalid",
			request: messageIDRequest{requested: -1},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMessageIDRequest(
				tt.outstanding,
				tt.request,
				DefaultMaxUnacknowledgedMessageIDs,
			)
			if tt.wantErr {
				require.ErrorIs(t, err, protocol.ErrProtocolViolationRequestExceeded)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestServerRejectsUnrequestedMessageBeforeAuthenticationAndCallback(
	t *testing.T,
) {
	requestedID := testMessageID(0xa1)
	callbackCalled := false
	cfg := NewConfig(WithReplyMessagesFunc(func(
		_ CallbackContext,
		_ []pcommon.DmqMessage,
	) {
		callbackCalled = true
	}))
	server := NewServer(
		newTestProtoOptions(MessageSubmissionV2MinVersion),
		&cfg,
	)
	server.pendingMsgRequest = &messageRequest{
		ids: map[string]struct{}{string(requestedID): {}},
	}

	err := server.handleReplyMessages(NewMsgReplyMessages(
		[]pcommon.DmqMessage{{MessageID: testMessageID(0xb2)}},
	))

	require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
	assert.False(t, callbackCalled)
	assert.NotNil(t, server.pendingMsgRequest)
}

func TestClientRejectsUnannouncedMessageRequestBeforeCallback(t *testing.T) {
	callbackCalled := false
	cfg := NewConfig(WithRequestMessagesFunc(func(
		_ CallbackContext,
		_ [][]byte,
	) {
		callbackCalled = true
	}))
	client := NewClient(
		newTestProtoOptions(MessageSubmissionV2MinVersion),
		&cfg,
	)
	client.pendingMessageIDs = [][]byte{testMessageID(0xa1)}

	err := client.handleRequestMessages(
		NewMsgRequestMessages([][]byte{testMessageID(0xb2)}),
	)

	require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
	assert.False(t, callbackCalled)
	assert.Nil(t, client.pendingMsgRequest)
}

func TestClientRejectsMessageRequestAboveOutstandingWindowBeforeCallback(
	t *testing.T,
) {
	callbackCalled := false
	cfg := NewConfig(WithRequestMessagesFunc(func(
		_ CallbackContext,
		_ [][]byte,
	) {
		callbackCalled = true
	}))
	client := NewClient(
		newTestProtoOptions(MessageSubmissionV2MinVersion),
		&cfg,
	)
	client.pendingMessageIDs = [][]byte{testMessageID(0xa1)}

	err := client.handleRequestMessages(NewMsgRequestMessages([][]byte{
		testMessageID(0xa1),
		testMessageID(0xb2),
	}))

	require.ErrorIs(t, err, protocol.ErrProtocolViolationRequestExceeded)
	assert.False(t, callbackCalled)
	assert.Nil(t, client.pendingMsgRequest)
}

func TestClientRejectsSubstitutedMessageReplyBeforeSend(t *testing.T) {
	requestedID := testMessageID(0xa1)
	client := newStartedMessageSubmissionClient(t)
	client.pendingMsgRequest = &messageRequest{
		ids: map[string]struct{}{string(requestedID): {}},
	}

	err := client.ReplyMessages(
		[]pcommon.DmqMessage{{MessageID: testMessageID(0xb2)}},
	)

	require.ErrorIs(t, err, protocol.ErrProtocolViolationInvalidMessage)
	assert.NotNil(t, client.pendingMsgRequest)
}

func TestClientRejectsExcessMessageIDReplyBeforeSend(t *testing.T) {
	client := newStartedMessageSubmissionClient(t)
	client.pendingIDRequest = &messageIDRequest{
		blocking:  true,
		requested: 1,
	}

	err := client.ReplyMessageIds([]pcommon.MessageIDAndSize{
		{MessageID: testMessageID(0xa1)},
		{MessageID: testMessageID(0xb2)},
	})

	require.ErrorIs(t, err, protocol.ErrProtocolViolationRequestExceeded)
	assert.NotNil(t, client.pendingIDRequest)
}

func TestClientAdmitsNextIDRequestAfterReplyEnqueue(t *testing.T) {
	client := newStartedMessageSubmissionClient(t)
	client.pendingIDRequest = &messageIDRequest{
		blocking:  true,
		requested: 1,
	}
	var nextRequestErr error
	client.testReplyEnqueued = func() {
		nextRequestErr = client.handleRequestMessageIds(
			NewMsgRequestMessageIds(false, 0, 1),
		)
	}

	err := client.ReplyMessageIds([]pcommon.MessageIDAndSize{
		{MessageID: testMessageID(0xa1)},
	})

	require.NoError(t, err)
	require.NoError(t, nextRequestErr)
	require.NotNil(t, client.pendingIDRequest)
}

func TestClientAdmitsNextRequestAfterMessageReplyEnqueue(t *testing.T) {
	client := newStartedMessageSubmissionClient(t)
	id := testMessageID(0xa1)
	client.pendingMessageIDs = [][]byte{id}
	client.pendingMsgRequest = &messageRequest{
		ids: map[string]struct{}{string(id): {}},
	}
	var nextRequestErr error
	client.testReplyEnqueued = func() {
		nextRequestErr = client.handleRequestMessageIds(
			NewMsgRequestMessageIds(false, 0, 1),
		)
	}

	err := client.ReplyMessages([]pcommon.DmqMessage{{MessageID: id}})

	require.NoError(t, err)
	require.NoError(t, nextRequestErr)
	require.NotNil(t, client.pendingIDRequest)
}

func TestClientRestoresIDRequestAfterEnqueueFailure(t *testing.T) {
	client := NewClient(
		newTestProtoOptions(MessageSubmissionV2MinVersion),
		nil,
	)
	request := &messageIDRequest{blocking: true, requested: 1}
	client.pendingIDRequest = request
	client.Protocol.Stop()

	err := client.ReplyMessageIds([]pcommon.MessageIDAndSize{
		{MessageID: testMessageID(0xa1)},
	})

	require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	assert.Same(t, request, client.pendingIDRequest)
	assert.Empty(t, client.pendingMessageIDs)
}

func TestClientRestoresMessageRequestAfterEnqueueFailure(t *testing.T) {
	client := NewClient(
		newTestProtoOptions(MessageSubmissionV2MinVersion),
		nil,
	)
	id := testMessageID(0xa1)
	request := &messageRequest{
		ids: map[string]struct{}{string(id): {}},
	}
	client.pendingMsgRequest = request
	client.Protocol.Stop()

	err := client.ReplyMessages([]pcommon.DmqMessage{{MessageID: id}})

	require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	assert.Same(t, request, client.pendingMsgRequest)
}

func TestClientEnqueueFailureDoesNotOverwriteNewerRequest(t *testing.T) {
	client := NewClient(
		newTestProtoOptions(MessageSubmissionV2MinVersion),
		nil,
	)
	client.pendingIDRequest = &messageIDRequest{
		blocking:  true,
		requested: 1,
	}
	var nextRequest *messageIDRequest
	client.testReplyPublished = func() {
		require.NoError(t, client.handleRequestMessageIds(
			NewMsgRequestMessageIds(false, 0, 1),
		))
		nextRequest = client.pendingIDRequest
	}
	client.Protocol.Stop()

	err := client.ReplyMessageIds([]pcommon.MessageIDAndSize{
		{MessageID: testMessageID(0xa1)},
	})

	require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	assert.Same(t, nextRequest, client.pendingIDRequest)
	assert.Equal(t, [][]byte{testMessageID(0xa1)}, client.pendingMessageIDs)
}

func TestClientMessageEnqueueFailureDoesNotOverwriteNewerRequest(t *testing.T) {
	client := NewClient(
		newTestProtoOptions(MessageSubmissionV2MinVersion),
		nil,
	)
	id := testMessageID(0xa1)
	client.pendingMessageIDs = [][]byte{id}
	client.pendingMsgRequest = &messageRequest{
		ids: map[string]struct{}{string(id): {}},
	}
	var nextRequest *messageIDRequest
	client.testReplyPublished = func() {
		require.NoError(t, client.handleRequestMessageIds(
			NewMsgRequestMessageIds(false, 0, 1),
		))
		nextRequest = client.pendingIDRequest
	}
	client.Protocol.Stop()

	err := client.ReplyMessages([]pcommon.DmqMessage{{MessageID: id}})

	require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	assert.Same(t, nextRequest, client.pendingIDRequest)
	assert.Nil(t, client.pendingMsgRequest)
}

func TestServerRejectsExcessMessageIDsBeforeCallback(t *testing.T) {
	callbackCalled := false
	cfg := NewConfig(WithReplyMessageIdsFunc(func(
		_ CallbackContext,
		_ []pcommon.MessageIDAndSize,
	) {
		callbackCalled = true
	}))
	server := NewServer(
		newTestProtoOptions(MessageSubmissionV2MinVersion),
		&cfg,
	)
	server.pendingIDRequest = &messageIDRequest{
		blocking:  true,
		requested: 1,
	}

	err := server.handleReplyMessageIds(NewMsgReplyMessageIds(
		[]pcommon.MessageIDAndSize{
			{MessageID: testMessageID(0xa1)},
			{MessageID: testMessageID(0xb2)},
		},
	))

	require.ErrorIs(t, err, protocol.ErrProtocolViolationRequestExceeded)
	assert.False(t, callbackCalled)
	assert.NotNil(t, server.pendingIDRequest)
}

func testMessageID(fill byte) []byte {
	return []byte{
		fill, fill, fill, fill, fill, fill, fill, fill,
		fill, fill, fill, fill, fill, fill, fill, fill,
		fill, fill, fill, fill, fill, fill, fill, fill,
		fill, fill, fill, fill, fill, fill, fill, fill,
	}
}

func newStartedMessageSubmissionClient(t *testing.T) *Client {
	t.Helper()
	localConn, peerConn := net.Pipe()
	m := muxer.New(localConn)
	options := newTestProtoOptions(MessageSubmissionV2MinVersion)
	options.Muxer = m
	options.ErrorChan = make(chan error, 1)
	client := NewClient(options, nil)
	client.Protocol.EnsureRegistered()
	m.Start()
	client.Start()
	t.Cleanup(func() {
		client.Protocol.Stop()
		m.Stop()
		_ = peerConn.Close()
		_ = localConn.Close()
	})
	return client
}

func newStartedMessageSubmissionServer(t *testing.T) *Server {
	t.Helper()
	localConn, peerConn := net.Pipe()
	m := muxer.New(localConn)
	options := newTestProtoOptions(MessageSubmissionV2MinVersion)
	options.Muxer = m
	options.ErrorChan = make(chan error, 1)
	server := NewServer(options, nil)
	server.Protocol.EnsureRegistered()
	m.Start()
	server.Start()
	t.Cleanup(func() {
		server.Protocol.Stop()
		m.Stop()
		_ = peerConn.Close()
		_ = localConn.Close()
	})
	return server
}

func TestServerRequestAPIsValidateAndTrackRequests(t *testing.T) {
	id := testMessageID(0xa1)
	tests := []struct {
		name       string
		prepare    func(*Server)
		request    func(*Server) error
		wantErr    bool
		wantIDReq  bool
		wantMsgReq bool
	}{
		{
			name: "blocking ID request accepted",
			request: func(server *Server) error {
				return server.RequestMessageIdsBlocking(0, 1)
			},
			wantIDReq: true,
		},
		{
			name: "blocking zero request rejected",
			request: func(server *Server) error {
				return server.RequestMessageIdsBlocking(0, 0)
			},
			wantErr: true,
		},
		{
			name: "blocking request above window rejected",
			request: func(server *Server) error {
				return server.RequestMessageIdsBlocking(
					0,
					uint16(DefaultMaxUnacknowledgedMessageIDs+1),
				)
			},
			wantErr: true,
		},
		{
			name: "non-blocking ID request accepted",
			request: func(server *Server) error {
				return server.RequestMessageIdsNonBlocking(0, 1)
			},
			wantIDReq: true,
		},
		{
			name: "non-blocking zero request rejected",
			request: func(server *Server) error {
				return server.RequestMessageIdsNonBlocking(0, 0)
			},
			wantErr: true,
		},
		{
			name: "non-blocking request above window rejected",
			request: func(server *Server) error {
				return server.RequestMessageIdsNonBlocking(
					0,
					uint16(DefaultMaxUnacknowledgedMessageIDs+1),
				)
			},
			wantErr: true,
		},
		{
			name:    "non-blocking acknowledgement accepted",
			prepare: func(server *Server) { server.pendingMessageIDs = [][]byte{id} },
			request: func(server *Server) error {
				return server.RequestMessageIdsNonBlocking(1, 0)
			},
			wantIDReq: true,
		},
		{
			name:    "message request accepted",
			prepare: func(server *Server) { server.pendingMessageIDs = [][]byte{id} },
			request: func(server *Server) error {
				return server.RequestMessages([][]byte{id})
			},
			wantMsgReq: true,
		},
		{
			name: "unannounced message request rejected",
			request: func(server *Server) error {
				return server.RequestMessages([][]byte{id})
			},
			wantErr: true,
		},
		{
			name: "malformed message ID rejected",
			prepare: func(server *Server) {
				server.pendingMessageIDs = [][]byte{[]byte("short")}
			},
			request: func(server *Server) error {
				return server.RequestMessages([][]byte{[]byte("short")})
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newStartedMessageSubmissionServer(t)
			if tt.prepare != nil {
				tt.prepare(server)
			}
			err := tt.request(server)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantIDReq, server.pendingIDRequest != nil)
			assert.Equal(t, tt.wantMsgReq, server.pendingMsgRequest != nil)
		})
	}
}
