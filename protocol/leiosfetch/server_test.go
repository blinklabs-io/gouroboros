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
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
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

func readLeiosFetchTestSegment(
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

func writeLeiosFetchTestSegment(
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

// sendNotFoundTest drives a real server over a muxer, sending it the given
// request and returning the message type of the server's response segment. It
// is used to verify that a not-found callback signal produces the graceful
// MsgNoBlock/MsgNoBlockTxs response instead of tearing down the connection.
func sendNotFoundTest(
	t *testing.T,
	cfg Config,
	request protocol.Message,
) uint {
	t.Helper()
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	connA, connB := net.Pipe()
	defer connA.Close()
	defer connB.Close()
	m := muxer.New(connA)
	defer m.Stop()

	server := NewServer(protocol.ProtocolOptions{
		ConnectionId: connId,
		Muxer:        m,
	}, &cfg)
	server.Start()
	defer server.Protocol.Stop()
	m.Start()

	requestData, err := cbor.Encode(request)
	require.NoError(t, err)
	writeLeiosFetchTestSegment(
		t,
		connB,
		muxer.NewSegment(ProtocolId, requestData, false),
	)
	require.NoError(t, connB.SetReadDeadline(time.Now().Add(time.Second)))
	segment, err := readLeiosFetchTestSegment(t, connB)
	require.NoError(t, err)
	assert.True(t, segment.IsResponse())
	assert.Equal(t, ProtocolId, segment.GetProtocolId())

	var elems []cbor.RawMessage
	_, err = cbor.Decode(segment.Payload, &elems)
	require.NoError(t, err)
	require.NotEmpty(t, elems)
	var msgType uint
	_, err = cbor.Decode(elems[0], &msgType)
	require.NoError(t, err)
	return msgType
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

func TestHandleBlockRequest_CallbackIsCalled(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}

	called := false
	expectedSlot := uint64(12345)
	expectedHash := []byte{0x01, 0x02, 0x03, 0x04}

	cfg := NewConfig(
		WithBlockRequestFunc(func(ctx CallbackContext, point pcommon.Point) (protocol.Message, error) {
			called = true
			assert.Equal(t, expectedSlot, point.Slot)
			assert.Equal(t, expectedHash, point.Hash)
			// Return an error to prevent attempting to send (which would hang)
			return nil, errors.New("test: stopping before send")
		}),
	)

	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: connId},
	}
	server.initProtocol()

	msg := NewMsgBlockRequest(pcommon.NewPoint(expectedSlot, expectedHash))
	err := server.handleBlockRequest(msg)

	// Error is expected because we return an error from the callback
	assert.Error(t, err)
	assert.True(t, called, "expected BlockRequestFunc to be called")
}

func TestHandleBlockRequest_NilCallback(t *testing.T) {
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

	msg := NewMsgBlockRequest(pcommon.NewPoint(12345, []byte{0x01, 0x02}))
	err := server.handleBlockRequest(msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no callback function is defined")
}

func TestHandleBlockRequest_NilConfig(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}

	server := &Server{
		config:          nil,
		callbackContext: CallbackContext{ConnectionId: connId},
	}
	server.initProtocol()

	msg := NewMsgBlockRequest(pcommon.NewPoint(12345, []byte{0x01, 0x02}))
	err := server.handleBlockRequest(msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no callback function is defined")
}

func TestHandleBlockRequest_CallbackError(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}

	expectedError := errors.New("block not found")
	cfg := NewConfig(
		WithBlockRequestFunc(func(ctx CallbackContext, point pcommon.Point) (protocol.Message, error) {
			return nil, expectedError
		}),
	)

	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: connId},
	}
	server.initProtocol()

	msg := NewMsgBlockRequest(pcommon.NewPoint(12345, []byte{0x01, 0x02}))
	err := server.handleBlockRequest(msg)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestHandleBlockRequest_NilResponse(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}

	cfg := NewConfig(
		WithBlockRequestFunc(func(ctx CallbackContext, point pcommon.Point) (protocol.Message, error) {
			return nil, nil
		}),
	)

	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: connId},
	}
	server.initProtocol()

	msg := NewMsgBlockRequest(pcommon.NewPoint(12345, []byte{0x01, 0x02}))
	err := server.handleBlockRequest(msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "callback returned nil")
}

func TestHandleBlockRequestNotFoundSendsMsgNoBlock(t *testing.T) {
	cfg := NewConfig(
		WithBlockRequestFunc(func(CallbackContext, pcommon.Point) (protocol.Message, error) {
			return nil, ErrBlockNotFound
		}),
	)
	msgType := sendNotFoundTest(
		t,
		cfg,
		NewMsgBlockRequest(pcommon.NewPoint(12345, []byte{0x01, 0x02})),
	)
	assert.Equal(t, uint(MessageTypeNoBlock), msgType)
}

func TestHandleBlockRequestWrappedNotFoundSendsMsgNoBlock(t *testing.T) {
	cfg := NewConfig(
		WithBlockRequestFunc(func(_ CallbackContext, point pcommon.Point) (protocol.Message, error) {
			return nil, fmt.Errorf(
				"cache miss for %d: %w",
				point.Slot,
				ErrBlockNotFound,
			)
		}),
	)
	msgType := sendNotFoundTest(
		t,
		cfg,
		NewMsgBlockRequest(pcommon.NewPoint(12345, []byte{0x01, 0x02})),
	)
	assert.Equal(t, uint(MessageTypeNoBlock), msgType)
}

func TestHandleBlockTxsRequestNotFoundSendsMsgNoBlockTxs(t *testing.T) {
	cfg := NewConfig(
		WithBlockTxsRequestFunc(func(CallbackContext, pcommon.Point, map[uint16]uint64) (protocol.Message, error) {
			return nil, ErrBlockTxsNotFound
		}),
	)
	msgType := sendNotFoundTest(
		t,
		cfg,
		NewMsgBlockTxsRequest(pcommon.NewPoint(12345, []byte{0x01, 0x02}), nil),
	)
	assert.Equal(t, uint(MessageTypeNoBlockTxs), msgType)
}

func TestHandleBlockRequest_NonNotFoundErrorPropagates(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}

	expectedError := errors.New("some other failure")
	cfg := NewConfig(
		WithBlockRequestFunc(func(CallbackContext, pcommon.Point) (protocol.Message, error) {
			return nil, expectedError
		}),
	)

	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: connId},
	}
	server.initProtocol()

	msg := NewMsgBlockRequest(pcommon.NewPoint(12345, []byte{0x01, 0x02}))
	err := server.handleBlockRequest(msg)

	// A non-not-found error is still propagated as a protocol violation
	assert.Equal(t, expectedError, err)
	assert.NotErrorIs(t, err, ErrBlockNotFound)
}

func TestHandleBlockTxsRequest_CallbackIsCalled(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}

	called := false
	expectedSlot := uint64(12345)
	expectedHash := []byte{0x01, 0x02, 0x03, 0x04}
	expectedBitmaps := map[uint16]uint64{
		0: 0xff00000000000000,
	}

	cfg := NewConfig(
		WithBlockTxsRequestFunc(func(ctx CallbackContext, point pcommon.Point, bitmaps map[uint16]uint64) (protocol.Message, error) {
			called = true
			assert.Equal(t, expectedSlot, point.Slot)
			assert.Equal(t, expectedHash, point.Hash)
			assert.Equal(t, expectedBitmaps, bitmaps)
			// Return an error to prevent attempting to send (which would hang)
			return nil, errors.New("test: stopping before send")
		}),
	)

	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: connId},
	}
	server.initProtocol()

	msg := NewMsgBlockTxsRequest(pcommon.NewPoint(expectedSlot, expectedHash), expectedBitmaps)
	err := server.handleBlockTxsRequest(msg)

	// Error is expected because we return an error from the callback
	assert.Error(t, err)
	assert.True(t, called, "expected BlockTxsRequestFunc to be called")
}

func TestHandleBlockTxsRequest_NilCallback(t *testing.T) {
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

	msg := NewMsgBlockTxsRequest(pcommon.NewPoint(12345, []byte{0x01, 0x02}), nil)
	err := server.handleBlockTxsRequest(msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no callback function is defined")
}

func TestHandleVotesRequest_CallbackIsCalled(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}

	called := false
	expectedVoteIds := []MsgVotesRequestVoteId{
		{SlotNo: 100, VoterId: 1},
	}

	cfg := NewConfig(
		WithVotesRequestFunc(func(ctx CallbackContext, voteIds []MsgVotesRequestVoteId) (protocol.Message, error) {
			called = true
			assert.Equal(t, expectedVoteIds, voteIds)
			// Return an error to prevent attempting to send (which would hang)
			return nil, errors.New("test: stopping before send")
		}),
	)

	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: connId},
	}
	server.initProtocol()

	msg := NewMsgVotesRequest(expectedVoteIds)
	err := server.handleVotesRequest(msg)

	// Error is expected because we return an error from the callback
	assert.Error(t, err)
	assert.True(t, called, "expected VotesRequestFunc to be called")
}

func TestHandleVotesRequest_NilCallback(t *testing.T) {
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

	msg := NewMsgVotesRequest(nil)
	err := server.handleVotesRequest(msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no callback function is defined")
}

func TestHandleBlockRangeRequest_Callback(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}

	called := false
	expectedStart := pcommon.NewPoint(100, []byte{0x01, 0x02, 0x03, 0x04})
	expectedEnd := pcommon.NewPoint(200, []byte{0x05, 0x06, 0x07, 0x08})

	cfg := NewConfig(
		WithBlockRangeRequestFunc(func(ctx CallbackContext, start, end pcommon.Point) error {
			called = true
			assert.Equal(t, expectedStart, start)
			assert.Equal(t, expectedEnd, end)
			return nil
		}),
	)

	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: connId},
	}
	server.initProtocol()

	msg := NewMsgBlockRangeRequest(expectedStart, expectedEnd)
	err := server.handleBlockRangeRequest(msg)

	assert.NoError(t, err)
	assert.True(t, called, "expected BlockRangeRequestFunc to be called")
}

func TestHandleBlockRangeRequest_NilCallback(t *testing.T) {
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

	msg := NewMsgBlockRangeRequest(pcommon.Point{}, pcommon.Point{})
	err := server.handleBlockRangeRequest(msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no callback function is defined")
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
	msg := NewMsgBlock([]byte{0x82, 0x01, 0x02})

	err := server.messageHandler(msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected message type")
}

func TestServerMessageHandler_AllTypes(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: connId,
	}

	// Create a config with all callbacks that return errors to prevent send attempts
	cfg := NewConfig(
		WithBlockRequestFunc(func(ctx CallbackContext, point pcommon.Point) (protocol.Message, error) {
			return nil, errors.New("test: callback error")
		}),
		WithBlockTxsRequestFunc(func(ctx CallbackContext, point pcommon.Point, bitmaps map[uint16]uint64) (protocol.Message, error) {
			return nil, errors.New("test: callback error")
		}),
		WithVotesRequestFunc(func(ctx CallbackContext, voteIds []MsgVotesRequestVoteId) (protocol.Message, error) {
			return nil, errors.New("test: callback error")
		}),
		WithBlockRangeRequestFunc(func(ctx CallbackContext, start, end pcommon.Point) error {
			return errors.New("test: callback error")
		}),
	)

	server := NewServer(protoOptions, &cfg)

	testCases := []struct {
		name string
		msg  protocol.Message
	}{
		{
			name: "BlockRequest",
			msg:  NewMsgBlockRequest(pcommon.NewPoint(12345, []byte{0x01, 0x02})),
		},
		{
			name: "BlockTxsRequest",
			msg:  NewMsgBlockTxsRequest(pcommon.NewPoint(12345, []byte{0x01, 0x02}), nil),
		},
		{
			name: "VotesRequest",
			msg:  NewMsgVotesRequest(nil),
		},
		{
			name: "BlockRangeRequest",
			msg:  NewMsgBlockRangeRequest(pcommon.Point{}, pcommon.Point{}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := server.messageHandler(tc.msg)
			// These will return errors from the callback
			// but we're testing that the message handler routes to the correct handler
			assert.Error(t, err)
			assert.NotContains(t, err.Error(), "unexpected message type")
		})
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

	leiosFetch := New(protoOptions, &cfg)

	require.NotNil(t, leiosFetch)
	assert.NotNil(t, leiosFetch.Client)
	assert.NotNil(t, leiosFetch.Server)
}
