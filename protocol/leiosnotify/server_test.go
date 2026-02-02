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
	"errors"
	"net"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
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

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no callback function is defined")
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

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no callback function is defined")
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
	msg := NewMsgBlockOffer(12345, []byte{0x01, 0x02})

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
			response: NewMsgBlockOffer(12345, []byte{0x01, 0x02, 0x03, 0x04}),
		},
		{
			name:     "BlockTxsOffer response",
			response: NewMsgBlockTxsOffer(12345, []byte{0x01, 0x02, 0x03, 0x04}),
		},
		{
			name: "VotesOffer response",
			response: NewMsgVotesOffer([]MsgVotesOfferVote{
				{Slot: 100, VoteIssuerId: []byte{0x01, 0x02, 0x03, 0x04}},
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
