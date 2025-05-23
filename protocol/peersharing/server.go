// Copyright 2025 Blink Labs Software
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

package peersharing

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the PeerSharing server
type Server struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	protoOptions    protocol.ProtocolOptions
}

// NewServer returns a new PeerSharing server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	s := &Server{
		config: cfg,
		// Save this for re-use later
		protoOptions: protoOptions,
	}
	s.callbackContext = CallbackContext{
		Server:       s,
		ConnectionId: protoOptions.ConnectionId,
	}
	s.initProtocol()
	return s
}

func (s *Server) initProtocol() {
	protoConfig := protocol.ProtocolConfig{
		Name:                ProtocolName,
		ProtocolId:          ProtocolId,
		Muxer:               s.protoOptions.Muxer,
		Logger:              s.protoOptions.Logger,
		ErrorChan:           s.protoOptions.ErrorChan,
		Mode:                s.protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.handleMessage,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        stateIdle,
	}
	s.Protocol = protocol.New(protoConfig)
}

func (s *Server) handleMessage(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeShareRequest:
		err = s.handleShareRequest(msg)
	case MessageTypeDone:
		err = s.handleDone(msg)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (s *Server) handleShareRequest(msg protocol.Message) error {
	s.Protocol.Logger().
		Debug("share request",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config == nil || s.config.ShareRequestFunc == nil {
		return errors.New(
			"received peer-sharing ShareRequest message but no callback function is defined",
		)
	}
	msgShareRequest := msg.(*MsgShareRequest)
	peers, err := s.config.ShareRequestFunc(
		s.callbackContext,
		int(msgShareRequest.Amount),
	)
	if err != nil {
		return err
	}
	msgResp := NewMsgSharePeers(peers)
	if err := s.SendMessage(msgResp); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleDone(msg protocol.Message) error {
	s.Protocol.Logger().
		Debug("done",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	// Restart protocol
	s.Stop()
	s.initProtocol()
	s.Start()
	return nil
}
