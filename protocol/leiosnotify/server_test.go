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
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readLeiosNotifyTestSegment(
	t *testing.T,
	conn net.Conn,
) (*muxer.Segment, error) {
	t.Helper()
	header := muxer.SegmentHeader{}
	if err := binary.Read(conn, binary.BigEndian, &header); err != nil {
		return nil, err
	}
	payload := make([]byte, header.PayloadLength)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return nil, err
	}
	return &muxer.Segment{
		SegmentHeader: header,
		Payload:       payload,
	}, nil
}

func writeLeiosNotifyTestSegment(
	t *testing.T,
	conn net.Conn,
	segment *muxer.Segment,
) {
	t.Helper()
	require.NotNil(t, segment)
	buf := &bytes.Buffer{}
	require.NoError(t, binary.Write(buf, binary.BigEndian, segment.SegmentHeader))
	_, err := buf.Write(segment.Payload)
	require.NoError(t, err)
	_, err = conn.Write(buf.Bytes())
	require.NoError(t, err)
}

type leiosNotifyFailWriteConn struct {
	net.Conn
}

var errLeiosNotifyTestWrite = errors.New(
	"test: forced transport write failure",
)

func (leiosNotifyFailWriteConn) Write([]byte) (int, error) {
	return 0, errLeiosNotifyTestWrite
}

func TestNewServer(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}

	cfg := NewConfig()
	server := NewServer(protoOptions, &cfg)

	require.NotNil(t, server)
	assert.NotNil(t, server.Protocol)
	assert.NotNil(t, server.config)
}

func TestHandleRequestNext_CallbackIsCalled(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}

	called := false

	cfg := NewConfig(
		WithRequestNextFunc(func(ctx CallbackContext) (protocol.Message, error) {
			called = true
			// Return an error to prevent attempting to send (which would hang)
			return nil, errors.New("test: stopping before send")
		}),
	)

	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: connId},
	}
	server.initProtocol()

	err := server.handleRequestNext()

	// Error is expected because we return an error from the callback
	assert.Error(t, err)
	assert.True(t, called, "expected RequestNextFunc to be called")
}

func TestHandleRequestNextReportsSuccessfulSend(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	connA, connB := net.Pipe()
	defer connA.Close()
	defer connB.Close()
	m := muxer.New(connA)
	defer m.Stop()

	response := NewMsgBlockAnnouncement(cbor.RawMessage{0x82, 0x01, 0x02})
	type sendResult struct {
		msg protocol.Message
		err error
	}
	resultCh := make(chan sendResult, 1)
	cfg := NewConfig(
		WithRequestNextFunc(func(CallbackContext) (protocol.Message, error) {
			return response, nil
		}),
		WithResponseSentFunc(func(
			_ CallbackContext,
			msg protocol.Message,
			err error,
		) {
			resultCh <- sendResult{msg: msg, err: err}
		}),
	)
	server := NewServer(protocol.ProtocolOptions{
		ConnectionId: connId,
		Muxer:        m,
	}, &cfg)
	server.Start()
	defer server.Protocol.Stop()
	m.Start()

	requestData, err := cbor.Encode(NewMsgNotificationRequestNext())
	require.NoError(t, err)
	writeLeiosNotifyTestSegment(
		t,
		connB,
		muxer.NewSegment(ProtocolId, requestData, false),
	)
	require.NoError(t, connB.SetReadDeadline(time.Now().Add(time.Second)))
	_, err = readLeiosNotifyTestSegment(t, connB)
	require.NoError(t, err)

	select {
	case result := <-resultCh:
		require.Same(t, response, result.msg)
		require.NoError(t, result.err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for response send callback")
	}
}

func TestHandleRequestNextReportsFailedSend(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	resultCh := make(chan error, 1)
	connA, connB := net.Pipe()
	defer connA.Close()
	defer connB.Close()
	m := muxer.New(leiosNotifyFailWriteConn{Conn: connA})
	defer m.Stop()
	cfg := NewConfig(
		WithRequestNextFunc(func(CallbackContext) (protocol.Message, error) {
			return NewMsgBlockAnnouncement(cbor.RawMessage{0x81, 0x01}), nil
		}),
		WithResponseSentFunc(func(
			_ CallbackContext,
			_ protocol.Message,
			err error,
		) {
			resultCh <- err
		}),
	)
	server := NewServer(protocol.ProtocolOptions{
		ConnectionId: connId,
		Muxer:        m,
	}, &cfg)
	server.Start()
	defer server.Protocol.Stop()
	m.Start()

	requestData, err := cbor.Encode(NewMsgNotificationRequestNext())
	require.NoError(t, err)
	writeLeiosNotifyTestSegment(
		t,
		connB,
		muxer.NewSegment(ProtocolId, requestData, false),
	)

	select {
	case sendErr := <-resultCh:
		require.ErrorIs(t, sendErr, errLeiosNotifyTestWrite)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for failed response send callback")
	}
}

func TestHandleRequestNextReportsShutdownBeforeDelivery(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	connA, connB := net.Pipe()
	defer connA.Close()
	defer connB.Close()
	m := muxer.New(connA)
	defer m.Stop()

	requestCalled := make(chan struct{})
	releaseRequest := make(chan struct{})
	resultCh := make(chan error, 1)
	cfg := NewConfig(
		WithRequestNextFunc(func(CallbackContext) (protocol.Message, error) {
			close(requestCalled)
			// Keep the response from being queued until shutdown is observable.
			<-releaseRequest
			return NewMsgBlockAnnouncement(cbor.RawMessage{0x81, 0x01}), nil
		}),
		WithResponseSentFunc(func(
			_ CallbackContext,
			_ protocol.Message,
			err error,
		) {
			resultCh <- err
		}),
	)
	server := NewServer(protocol.ProtocolOptions{
		ConnectionId: connId,
		Muxer:        m,
	}, &cfg)
	server.Start()
	m.Start()

	requestData, err := cbor.Encode(NewMsgNotificationRequestNext())
	require.NoError(t, err)
	writeLeiosNotifyTestSegment(
		t,
		connB,
		muxer.NewSegment(ProtocolId, requestData, false),
	)
	select {
	case <-requestCalled:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for request callback")
	}
	server.Protocol.Stop()
	close(releaseRequest)

	select {
	case sendErr := <-resultCh:
		require.ErrorIs(t, sendErr, protocol.ErrProtocolShuttingDown)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for shutdown delivery callback")
	}
}

func TestHandleRequestNext_NilCallback(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}

	cfg := NewConfig()
	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: connId},
	}
	server.initProtocol()

	err := server.handleRequestNext()

	// With no callback wired, the request is left pending (the server holds
	// agency until it has a notification) rather than tearing down the
	// connection. leios-notify has no response timeout or await message, so
	// this is valid protocol behavior for a node that does not serve
	// notifications.
	assert.NoError(t, err)
	assert.False(t, server.IsDone(), "nil callback must not stop the protocol")
	assert.Equal(t, connId, server.callbackContext.ConnectionId)
}

func TestNilCallbackRequestStaysPendingAndConnectionUsable(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}

	connA, connB := net.Pipe()
	defer connA.Close()
	defer connB.Close()

	m := muxer.New(connA)
	defer m.Stop()

	cfg := NewConfig()
	server := NewServer(
		protocol.ProtocolOptions{
			ConnectionId: connId,
			Muxer:        m,
		},
		&cfg,
	)
	server.Start()
	defer server.Protocol.Stop()
	m.Start()

	requestData, err := cbor.Encode(NewMsgNotificationRequestNext())
	require.NoError(t, err)
	writeLeiosNotifyTestSegment(
		t,
		connB,
		muxer.NewSegment(ProtocolId, requestData, false),
	)

	require.Eventually(t, func() bool {
		return !server.IsInTerminalOrIdleState()
	}, time.Second, 10*time.Millisecond, "server should remain in Busy")
	assert.False(t, server.IsDone(), "pending request must not close protocol")
	assert.Equal(t, connId, server.callbackContext.ConnectionId)
	assert.Equal(t, server, server.callbackContext.Server)

	response := NewMsgBlockAnnouncement(cbor.RawMessage{0x82, 0x01, 0x02})
	require.NoError(t, server.SendMessage(response))
	require.NoError(t, connB.SetReadDeadline(time.Now().Add(time.Second)))
	segment, err := readLeiosNotifyTestSegment(t, connB)
	require.NoError(t, err)
	assert.True(t, segment.IsResponse())
	assert.Equal(t, ProtocolId, segment.GetProtocolId())

	msg, err := NewMsgFromCbor(MessageTypeBlockAnnouncement, segment.Payload)
	require.NoError(t, err)
	if msg == nil {
		t.Fatal("expected block announcement message")
	}
	assert.Equal(t, response.Type(), msg.Type())
}

func TestHandleRequestNext_NilConfig(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}

	server := &Server{
		config:          nil,
		callbackContext: CallbackContext{ConnectionId: connId},
	}
	server.initProtocol()

	err := server.handleRequestNext()

	// A nil config likewise leaves the request pending rather than erroring.
	assert.NoError(t, err)
}

func TestHandleRequestNext_CallbackError(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}

	expectedError := errors.New("no notifications available")
	cfg := NewConfig(
		WithRequestNextFunc(func(ctx CallbackContext) (protocol.Message, error) {
			return nil, expectedError
		}),
	)

	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: connId},
	}
	server.initProtocol()

	err := server.handleRequestNext()

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestHandleRequestNext_NilResponse(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}

	cfg := NewConfig(
		WithRequestNextFunc(func(ctx CallbackContext) (protocol.Message, error) {
			return nil, nil
		}),
	)

	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: connId},
	}
	server.initProtocol()

	err := server.handleRequestNext()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "callback returned nil")
}

func TestServerMessageHandler_UnexpectedType(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}

	cfg := NewConfig()
	server := NewServer(protoOptions, &cfg)

	// Create a message with an unexpected type for the server
	msg := NewMsgBlockOffer(pcommon.NewPoint(12345, []byte{0x01, 0x02}), 12345)

	err := server.messageHandler(msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected message type")
}

func TestServerMessageHandler_RequestNext(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}

	called := false
	cfg := NewConfig(
		WithRequestNextFunc(func(ctx CallbackContext) (protocol.Message, error) {
			called = true
			// Return an error to prevent attempting to send (which would hang without a muxer)
			return nil, errors.New("test: callback error")
		}),
	)

	server := NewServer(protoOptions, &cfg)

	msg := NewMsgNotificationRequestNext()
	err := server.messageHandler(msg)

	// Error is expected because callback returns error
	assert.Error(t, err)
	assert.True(t, called, "expected callback to be called")
}

func TestNew(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}
	cfg := NewConfig()

	leiosNotify := New(protoOptions, &cfg)

	require.NotNil(t, leiosNotify)
	assert.NotNil(t, leiosNotify.Client)
	assert.NotNil(t, leiosNotify.Server)
}

func TestServerCallbackContext(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}
	cfg := NewConfig()

	server := NewServer(protoOptions, &cfg)

	// Verify callback context is set correctly
	assert.Equal(t, connId, server.callbackContext.ConnectionId)
	assert.Equal(t, server, server.callbackContext.Server)
}

func TestCallbackResponseTypes(t *testing.T) {
	// Test that the config callback types work correctly
	// We test that the callbacks can be set and invoked properly

	testCases := []struct {
		name     string
		response protocol.Message
	}{
		{
			name:     "BlockAnnouncement response",
			response: NewMsgBlockAnnouncement(cbor.RawMessage{0x82, 0x01, 0x02}),
		},
		{
			name:     "BlockOffer response",
			response: NewMsgBlockOffer(pcommon.NewPoint(12345, []byte{0x01, 0x02, 0x03, 0x04}), 12345),
		},
		{
			name:     "BlockTxsOffer response",
			response: NewMsgBlockTxsOffer(pcommon.NewPoint(12345, []byte{0x01, 0x02, 0x03, 0x04})),
		},
		{
			name: "VotesOffer response",
			response: NewMsgVotesOffer([]MsgVotesOfferVote{
				{SlotNo: 100, VoterId: 1},
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := NewConfig(
				WithRequestNextFunc(func(ctx CallbackContext) (protocol.Message, error) {
					return tc.response, nil
				}),
			)

			// Test that we can invoke the callback and get the expected response
			resp, err := cfg.RequestNextFunc(CallbackContext{})
			require.NoError(t, err)
			assert.Equal(t, tc.response, resp)
		})
	}
}
