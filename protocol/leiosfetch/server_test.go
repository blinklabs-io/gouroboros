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
	"errors"
	"net"
	"testing"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		WithBlockRequestFunc(func(ctx CallbackContext, slot uint64, hash []byte) (protocol.Message, error) {
			called = true
			assert.Equal(t, expectedSlot, slot)
			assert.Equal(t, expectedHash, hash)
			// Return an error to prevent attempting to send (which would hang)
			return nil, errors.New("test: stopping before send")
		}),
	)

	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: connId},
	}
	server.initProtocol()

	msg := NewMsgBlockRequest(expectedSlot, expectedHash)
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

	msg := NewMsgBlockRequest(12345, []byte{0x01, 0x02})
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

	msg := NewMsgBlockRequest(12345, []byte{0x01, 0x02})
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
		WithBlockRequestFunc(func(ctx CallbackContext, slot uint64, hash []byte) (protocol.Message, error) {
			return nil, expectedError
		}),
	)

	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: connId},
	}
	server.initProtocol()

	msg := NewMsgBlockRequest(12345, []byte{0x01, 0x02})
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
		WithBlockRequestFunc(func(ctx CallbackContext, slot uint64, hash []byte) (protocol.Message, error) {
			return nil, nil
		}),
	)

	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: connId},
	}
	server.initProtocol()

	msg := NewMsgBlockRequest(12345, []byte{0x01, 0x02})
	err := server.handleBlockRequest(msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "callback returned nil")
}

func TestHandleBlockTxsRequest_CallbackIsCalled(t *testing.T) {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}

	called := false
	expectedSlot := uint64(12345)
	expectedHash := []byte{0x01, 0x02, 0x03, 0x04}
	expectedBitmaps := map[uint16][8]byte{
		0: {0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}

	cfg := NewConfig(
		WithBlockTxsRequestFunc(func(ctx CallbackContext, slot uint64, hash []byte, bitmaps map[uint16][8]byte) (protocol.Message, error) {
			called = true
			assert.Equal(t, expectedSlot, slot)
			assert.Equal(t, expectedHash, hash)
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

	msg := NewMsgBlockTxsRequest(expectedSlot, expectedHash, expectedBitmaps)
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

	msg := NewMsgBlockTxsRequest(12345, []byte{0x01, 0x02}, nil)
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
		{Slot: 100, VoteIssuerId: []byte{0x01, 0x02, 0x03, 0x04}},
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
		WithBlockRequestFunc(func(ctx CallbackContext, slot uint64, hash []byte) (protocol.Message, error) {
			return nil, errors.New("test: callback error")
		}),
		WithBlockTxsRequestFunc(func(ctx CallbackContext, slot uint64, hash []byte, bitmaps map[uint16][8]byte) (protocol.Message, error) {
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
			msg:  NewMsgBlockRequest(12345, []byte{0x01, 0x02}),
		},
		{
			name: "BlockTxsRequest",
			msg:  NewMsgBlockTxsRequest(12345, []byte{0x01, 0x02}, nil),
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
