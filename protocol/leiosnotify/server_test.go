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
	"sync/atomic"
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

func TestNewServerPropagatesConnectionDoneChan(t *testing.T) {
	done := make(chan any)
	server := NewServer(
		protocol.ProtocolOptions{
			ConnectionId:       connection.ConnectionId{},
			ConnectionDoneChan: done,
		},
		nil,
	)
	close(done)
	select {
	case <-server.callbackContext.ConnectionDoneChan:
	default:
		t.Fatal("connection lifecycle channel did not close")
	}
}

func TestNewClientPropagatesConnectionDoneChan(t *testing.T) {
	done := make(chan any)
	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId:       connection.ConnectionId{},
			ConnectionDoneChan: done,
		},
		nil,
	)
	close(done)
	select {
	case <-client.callbackContext.ConnectionDoneChan:
	default:
		t.Fatal("connection lifecycle channel did not close")
	}
}

func TestStartedServerCallbackObservesConnectionShutdown(t *testing.T) {
	connA, connB := net.Pipe()
	defer connA.Close()
	defer connB.Close()
	m := muxer.New(connA)
	defer m.Stop()

	connectionDone := make(chan any)
	testCleanup := make(chan struct{})
	callbackEntered := make(chan struct{})
	callbackReturned := make(chan struct{})
	cfg := NewConfig(
		WithRequestNextFunc(
			func(ctx CallbackContext) (protocol.Message, error) {
				close(callbackEntered)
				select {
				case <-ctx.ConnectionDoneChan:
				case <-testCleanup:
					return nil, errors.New("test: cleanup")
				}
				close(callbackReturned)
				return nil, errors.New("test: connection shutdown")
			},
		),
	)
	server := NewServer(protocol.ProtocolOptions{
		ConnectionId:       connection.ConnectionId{},
		ConnectionDoneChan: connectionDone,
		Muxer:              m,
	}, &cfg)
	server.Start()
	defer server.Protocol.Stop()
	defer close(testCleanup)
	m.Start()

	requestData, err := cbor.Encode(NewMsgNotificationRequestNext())
	require.NoError(t, err)
	require.NoError(t, connB.SetWriteDeadline(time.Now().Add(time.Second)))
	writeLeiosNotifyTestSegment(
		t,
		connB,
		muxer.NewSegment(ProtocolId, requestData, false),
	)
	select {
	case <-callbackEntered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for started protocol callback")
	}
	close(connectionDone)
	select {
	case <-callbackReturned:
	case <-time.After(time.Second):
		t.Fatal("callback did not observe connection shutdown")
	}
	select {
	case <-server.DoneChan():
	case <-time.After(time.Second):
		t.Fatal("started protocol did not stop after callback returned")
	}
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

	response := NewMsgBlockAnnouncement(testAnnouncementEra, cbor.RawMessage{0x82, 0x01, 0x02})
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
			return NewMsgBlockAnnouncement(testAnnouncementEra, cbor.RawMessage{0x81, 0x01}), nil
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
			return NewMsgBlockAnnouncement(testAnnouncementEra, cbor.RawMessage{0x81, 0x01}), nil
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

	response := NewMsgBlockAnnouncement(testAnnouncementEra, cbor.RawMessage{0x82, 0x01, 0x02})
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
	msg := NewMsgBlockOffer(pcommon.NewPoint(12345, testPointHash(0x01)), 12345)

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

	called := make(chan struct{})
	errCh := make(chan error, 1)
	protoOptions.ErrorChan = errCh
	cfg := NewConfig(
		WithRequestNextFunc(func(ctx CallbackContext) (protocol.Message, error) {
			close(called)
			// Return an error to prevent attempting to send (which would hang without a muxer)
			return nil, errors.New("test: callback error")
		}),
	)

	server := NewServer(protoOptions, &cfg)

	msg := NewMsgNotificationRequestNext()
	// The request is answered off the receive loop, so its error is
	// reported to the connection rather than returned
	require.NoError(t, server.messageHandler(msg))

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("expected callback to be called")
	}
	select {
	case err := <-errCh:
		assert.ErrorContains(t, err, "test: callback error")
	case <-time.After(time.Second):
		t.Fatal("callback error was not reported")
	}
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
			response: NewMsgBlockAnnouncement(testAnnouncementEra, cbor.RawMessage{0x82, 0x01, 0x02}),
		},
		{
			name:     "BlockOffer response",
			response: NewMsgBlockOffer(pcommon.NewPoint(12345, testPointHash(0x01)), 12345),
		},
		{
			name:     "BlockTxsOffer response",
			response: NewMsgBlockTxsOffer(pcommon.NewPoint(12345, testPointHash(0x01))),
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

func newQuitTestServer(
	t *testing.T,
	cfg Config,
) (*Server, net.Conn) {
	t.Helper()
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		_ = connA.Close()
		_ = connB.Close()
	})
	m := muxer.New(connA)
	t.Cleanup(m.Stop)
	server := NewServer(protocol.ProtocolOptions{
		ConnectionId: connId,
		Muxer:        m,
	}, &cfg)
	server.Start()
	t.Cleanup(func() { server.ProtocolInstance().Stop() })
	m.Start()
	return server, connB
}

func readQuitTestMessage(
	t *testing.T,
	conn net.Conn,
) protocol.Message {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	segment, err := readLeiosNotifyTestSegment(t, conn)
	require.NoError(t, err)
	var fields []cbor.RawMessage
	_, err = cbor.Decode(segment.Payload, &fields)
	require.NoError(t, err)
	require.NotEmpty(t, fields)
	var tag uint
	_, err = cbor.Decode(fields[0], &tag)
	require.NoError(t, err)
	msg, err := NewMsgFromCbor(tag, segment.Payload)
	require.NoError(t, err)
	return msg
}

func TestServerAnswersQuitWithDone(t *testing.T) {
	_, conn := newQuitTestServer(t, NewConfig())

	quitData, err := cbor.Encode(NewMsgQuit())
	require.NoError(t, err)
	writeLeiosNotifyTestSegment(
		t,
		conn,
		muxer.NewSegment(ProtocolId, quitData, false),
	)
	msg := readQuitTestMessage(t, conn)
	require.IsType(t, &MsgDone{}, msg)
}

func writeQuitTestMessage(t *testing.T, conn net.Conn, msg protocol.Message) {
	t.Helper()
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	writeLeiosNotifyTestSegment(
		t,
		conn,
		muxer.NewSegment(ProtocolId, data, false),
	)
}

func TestServerAnswersQuitAfterReply(t *testing.T) {
	_, conn := newQuitTestServer(t, NewConfig(
		WithRequestNextFunc(func(CallbackContext) (protocol.Message, error) {
			return NewMsgBlockAnnouncement(testAnnouncementEra,
				cbor.RawMessage{0x82, 0x01, 0x02},
			), nil
		}),
	))

	writeQuitTestMessage(t, conn, NewMsgNotificationRequestNext())
	require.IsType(t, &MsgBlockAnnouncement{}, readQuitTestMessage(t, conn))
	writeQuitTestMessage(t, conn, NewMsgQuit())
	require.IsType(t, &MsgDone{}, readQuitTestMessage(t, conn))
}

// A reference-node client pipelines MsgQuit behind its outstanding requests,
// and the server must answer each of them before it acknowledges the quit,
// even while the notification source is still waiting.
func TestServerCancelsRequestOutstandingAtQuit(t *testing.T) {
	sourceStarted := make(chan struct{})
	sawQuit := make(chan struct{})
	_, conn := newQuitTestServer(t, NewConfig(
		WithRequestNextFunc(func(ctx CallbackContext) (protocol.Message, error) {
			close(sourceStarted)
			select {
			case <-ctx.QuitChan:
				close(sawQuit)
			case <-time.After(5 * time.Second):
			}
			return nil, errors.New("canceled")
		}),
	))

	writeQuitTestMessage(t, conn, NewMsgNotificationRequestNext())
	<-sourceStarted
	writeQuitTestMessage(t, conn, NewMsgQuit())
	require.IsType(t, &MsgCanceled{}, readQuitTestMessage(t, conn))
	require.IsType(t, &MsgDone{}, readQuitTestMessage(t, conn))
	select {
	case <-sawQuit:
	case <-time.After(time.Second):
		t.Fatal("source was not told the client quit")
	}
}

func TestServerCancelsEveryPipelinedRequest(t *testing.T) {
	const pipelined = 1000
	var sourceCalls atomic.Int32
	sourceStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	_, conn := newQuitTestServer(t, NewConfig(
		WithRequestNextFunc(func(CallbackContext) (protocol.Message, error) {
			sourceCalls.Add(1)
			sourceStarted <- struct{}{}
			<-release
			return nil, errors.New("released")
		}),
	))

	go func() {
		for range pipelined {
			writeQuitTestMessage(t, conn, NewMsgNotificationRequestNext())
		}
		writeQuitTestMessage(t, conn, NewMsgQuit())
	}()
	for i := range pipelined {
		require.IsType(
			t,
			&MsgCanceled{},
			readQuitTestMessage(t, conn),
			"request %d",
			i,
		)
	}
	require.IsType(t, &MsgDone{}, readQuitTestMessage(t, conn))
	require.Equal(t, int32(1), sourceCalls.Load())
}

func TestServerCancelsRequestWithoutSource(t *testing.T) {
	_, conn := newQuitTestServer(t, NewConfig())

	writeQuitTestMessage(t, conn, NewMsgNotificationRequestNext())
	writeQuitTestMessage(t, conn, NewMsgQuit())
	require.IsType(t, &MsgCanceled{}, readQuitTestMessage(t, conn))
	require.IsType(t, &MsgDone{}, readQuitTestMessage(t, conn))
}

// A source that ignores the quit and hands back a notification later must be
// told it was not sent.
func TestServerReleasesNotificationReturnedAfterQuit(t *testing.T) {
	sourceStarted := make(chan struct{})
	release := make(chan struct{})
	released := make(chan error, 1)
	response := NewMsgBlockAnnouncement(testAnnouncementEra, cbor.RawMessage{0x82, 0x01, 0x02})
	_, conn := newQuitTestServer(t, NewConfig(
		WithRequestNextFunc(func(CallbackContext) (protocol.Message, error) {
			close(sourceStarted)
			<-release
			return response, nil
		}),
		WithResponseSentFunc(func(
			_ CallbackContext,
			msg protocol.Message,
			err error,
		) {
			require.Same(t, response, msg)
			released <- err
		}),
	))

	writeQuitTestMessage(t, conn, NewMsgNotificationRequestNext())
	<-sourceStarted
	writeQuitTestMessage(t, conn, NewMsgQuit())
	require.IsType(t, &MsgCanceled{}, readQuitTestMessage(t, conn))
	require.IsType(t, &MsgDone{}, readQuitTestMessage(t, conn))
	close(release)
	select {
	case err := <-released:
		require.ErrorIs(t, err, ErrRequestCanceled)
	case <-time.After(time.Second):
		t.Fatal("late notification was not released")
	}
}
