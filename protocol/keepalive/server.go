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
	"fmt"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the keep-alive protocol server, responsible for handling keep-alive requests and sending responses.
type Server struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
}

// NewServer creates and returns a new keep-alive protocol server with the given options and configuration.
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	s := &Server{
		config: cfg,
	}
	s.callbackContext = CallbackContext{
		Server:       s,
		ConnectionId: protoOptions.ConnectionId,
	}
	protoConfig := protocol.ProtocolConfig{
		Name:                ProtocolName,
		ProtocolId:          ProtocolId,
		Muxer:               protoOptions.Muxer,
		Logger:              protoOptions.Logger,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        StateClient,
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

// messageHandler handles incoming protocol messages for the server.
func (s *Server) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeKeepAlive:
		err = s.handleKeepAlive(msg)
	case MessageTypeDone:
		s.handleDone()
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
func (s *Server) handleKeepAlive(msgGeneric protocol.Message) error {
	s.Protocol.Logger().
		Debug("keep alive",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	msg := msgGeneric.(*MsgKeepAlive)

	// Call optional notification callback if provided
	if s.config != nil && s.config.OnKeepAliveReceived != nil {
		s.config.OnKeepAliveReceived(s.callbackContext.ConnectionId, msg.Cookie)
	}

	// Automatically send the keep-alive response
	resp := NewMsgKeepAliveResponse(msg.Cookie)
	return s.SendMessage(resp)
}

// handleDone processes a done message from the client and performs any necessary cleanup.
func (s *Server) handleDone() {
	s.Protocol.Logger().
		Debug("done",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
}

// Stop stops the keep-alive protocol server.
func (s *Server) Stop() {
	s.Protocol.Stop()
}
