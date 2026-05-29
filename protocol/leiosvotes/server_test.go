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

package leiosvotes

import (
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	cfg := NewConfig()
	server := NewServer(
		protocol.ProtocolOptions{ConnectionId: testConnectionId()},
		&cfg,
	)

	require.NotNil(t, server)
	assert.NotNil(t, server.Protocol)
	assert.NotNil(t, server.config)
}

func TestHandleRequestNextCallbackIsCalled(t *testing.T) {
	called := false
	cfg := NewConfig(
		WithRequestNextFunc(func(ctx CallbackContext, count uint64) ([]Vote, error) {
			called = true
			assert.Equal(t, uint64(2), count)
			return nil, errors.New("test: stopping before send")
		}),
	)
	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: testConnectionId()},
	}
	server.initProtocol()

	err := server.handleRequestNext(NewMsgVotesRequestNext(2))

	assert.Error(t, err)
	assert.True(t, called)
}

func TestHandleRequestNextNilCallback(t *testing.T) {
	cfg := NewConfig()
	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: testConnectionId()},
	}
	server.initProtocol()

	err := server.handleRequestNext(NewMsgVotesRequestNext(1))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no callback function is defined")
}

func TestHandleRequestNextNilConfig(t *testing.T) {
	server := &Server{
		config:          nil,
		callbackContext: CallbackContext{ConnectionId: testConnectionId()},
	}
	server.initProtocol()

	err := server.handleRequestNext(NewMsgVotesRequestNext(1))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no callback function is defined")
}

func TestHandleRequestNextInvalidCount(t *testing.T) {
	cfg := NewConfig()
	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: testConnectionId()},
	}
	server.initProtocol()

	err := server.handleRequestNext(NewMsgVotesRequestNext(0))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid count")
}

func TestHandleRequestNextWrongVoteCount(t *testing.T) {
	cfg := NewConfig(
		WithRequestNextFunc(func(ctx CallbackContext, count uint64) ([]Vote, error) {
			return []Vote{testVote()}, nil
		}),
	)
	server := &Server{
		config:          &cfg,
		callbackContext: CallbackContext{ConnectionId: testConnectionId()},
	}
	server.initProtocol()

	err := server.handleRequestNext(NewMsgVotesRequestNext(2))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "callback returned 1")
}

func TestServerMessageHandlerUnexpectedType(t *testing.T) {
	cfg := NewConfig()
	server := NewServer(
		protocol.ProtocolOptions{ConnectionId: testConnectionId()},
		&cfg,
	)

	err := server.messageHandler(NewMsgVote(testVote()))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected message type")
}

func TestServerMessageHandlerRequestNext(t *testing.T) {
	called := false
	cfg := NewConfig(
		WithRequestNextFunc(func(ctx CallbackContext, count uint64) ([]Vote, error) {
			called = true
			return nil, errors.New("test: callback error")
		}),
	)
	server := NewServer(
		protocol.ProtocolOptions{ConnectionId: testConnectionId()},
		&cfg,
	)

	err := server.messageHandler(NewMsgVotesRequestNext(1))

	assert.Error(t, err)
	assert.True(t, called)
}

func TestNew(t *testing.T) {
	cfg := NewConfig()
	leiosVotes := New(
		protocol.ProtocolOptions{ConnectionId: testConnectionId()},
		&cfg,
	)

	require.NotNil(t, leiosVotes)
	assert.NotNil(t, leiosVotes.Client)
	assert.NotNil(t, leiosVotes.Server)
}

func TestNewNormalizesZeroValueConfig(t *testing.T) {
	cfg := Config{}
	leiosVotes := New(
		protocol.ProtocolOptions{ConnectionId: testConnectionId()},
		&cfg,
	)

	require.NotNil(t, leiosVotes)
	assert.Equal(t, DefaultRequestNextCount, leiosVotes.Client.config.RequestNextCount)
	assert.Equal(t, DefaultRequestNextCount, leiosVotes.Server.config.RequestNextCount)
}

func TestNewPanicsOnInvalidConfig(t *testing.T) {
	cfg := Config{
		PipelineLimit:    MaxPipelineLimit + 1,
		RequestNextCount: DefaultRequestNextCount,
	}

	assert.Panics(t, func() {
		New(protocol.ProtocolOptions{ConnectionId: testConnectionId()}, &cfg)
	})
}

func TestServerCallbackContext(t *testing.T) {
	cfg := NewConfig()
	server := NewServer(
		protocol.ProtocolOptions{ConnectionId: testConnectionId()},
		&cfg,
	)

	assert.Equal(t, testConnectionId(), server.callbackContext.ConnectionId)
	assert.Equal(t, server, server.callbackContext.Server)
}
