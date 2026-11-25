// Copyright 2024 Blink Labs Software
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

package keepalive

import (
	"context"
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the keep-alive protocol server, responsible for handling keep-alive requests and sending responses.
type Server struct {
	*protocol.Protocol
	protocolMu      sync.RWMutex
	config          *Config
	callbackContext CallbackContext
	protoOptions    protocol.ProtocolOptions
	// stopped is set by Stop under protocolMu. A restart already past
	// reading MsgDone checks it before installing a new instance, so Stop
	// cannot be undone by a restart it raced with.
	stopped bool
	// restartHook is test-only synchronization for a Stop that lands after
	// StopForRestart and before the restart installs a new instance.
	restartHook func()
}

// NewServer creates and returns a new keep-alive protocol server with the given options and configuration.
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	s := &Server{
		config:       cfg,
		protoOptions: protoOptions,
	}
	s.callbackContext = CallbackContext{
		Server:       s,
		ConnectionId: protoOptions.ConnectionId,
	}
	s.Protocol = s.newProtocol()
	return s
}

func (s *Server) newProtocol() *protocol.Protocol {
	protoConfig := protocol.ProtocolConfig{
		Name:                ProtocolName,
		ProtocolId:          ProtocolId,
		Muxer:               s.protoOptions.Muxer,
		Logger:              s.protoOptions.Logger,
		ErrorChan:           s.protoOptions.ErrorChan,
		Mode:                s.protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        StateClient,
	}
	return protocol.New(protoConfig)
}

// ProtocolInstance returns the current protocol instance. The server replaces
// it each time the client ends the protocol with MsgDone.
func (s *Server) ProtocolInstance() *protocol.Protocol {
	s.protocolMu.RLock()
	defer s.protocolMu.RUnlock()
	return s.Protocol
}

// messageHandler handles incoming protocol messages for the server.
func (s *Server) messageHandler(msg protocol.Message) error {
	proto := s.ProtocolInstance()
	var err error
	switch msg.Type() {
	case MessageTypeKeepAlive:
		err = s.handleKeepAlive(proto, msg)
	case MessageTypeDone:
		err = s.handleDone(proto)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

// handleKeepAlive processes a keep-alive message from the client and sends a response.
func (s *Server) handleKeepAlive(
	proto *protocol.Protocol,
	msgGeneric protocol.Message,
) error {
	proto.Logger().
		Debug("keep alive",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	msg := msgGeneric.(*MsgKeepAlive)
	if s.config != nil && s.config.KeepAliveFunc != nil {
		callbackContext := s.callbackContext.withOwner(proto)
		return s.config.KeepAliveFunc(callbackContext, msg.Cookie)
	}

	// Call optional notification callback if provided
	if s.config != nil && s.config.OnKeepAliveReceived != nil {
		s.config.OnKeepAliveReceived(s.callbackContext.ConnectionId, msg.Cookie)
	}

	// Automatically send the keep-alive response
	resp := NewMsgKeepAliveResponse(msg.Cookie)
	return proto.SendMessage(resp)
}

// handleDone processes a done message from the client and restarts the
// protocol in its initial state, so the client can start a new run on the
// same connection.
func (s *Server) handleDone(proto *protocol.Protocol) error {
	proto.Logger().
		Debug("done",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config != nil && s.config.DoneFunc != nil {
		callbackContext := s.callbackContext.withOwner(proto)
		if err := s.config.DoneFunc(callbackContext); err != nil {
			return err
		}
	}
	_ = proto.StopForRestart(proto.CallbackContext())
	if s.restartHook != nil {
		s.restartHook()
	}
	s.protocolMu.Lock()
	if s.stopped {
		s.protocolMu.Unlock()
		proto.CancelRestart()
		return nil
	}
	p := s.newProtocol()
	s.Protocol = p
	s.protocolMu.Unlock()
	// Start does nothing if Stop reached p first.
	p.Start()
	return nil
}

// Stop stops the keep-alive protocol server, including a restart in progress.
//
// Stop signals shutdown at once. When ctx is the Context of a
// CallbackContext this server passed to a callback, Stop returns without
// waiting, since the goroutine running the callback is one it would wait
// for. Otherwise it waits until the protocol's goroutines have returned, or
// until ctx is done, in which case it returns ctx.Err() and shutdown still
// proceeds.
func (s *Server) Stop(ctx context.Context) error {
	s.protocolMu.Lock()
	s.stopped = true
	p := s.Protocol
	s.protocolMu.Unlock()
	return p.Stop(ctx)
}
