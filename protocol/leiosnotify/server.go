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
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/protocol"
)

type Server struct {
	*protocol.Protocol
	protocolMu      sync.RWMutex
	config          *Config
	callbackContext CallbackContext
	protoOptions    protocol.ProtocolOptions
}

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
		MessageHandlerFunc:  s.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        StateIdle,
	}
	p := protocol.New(protoConfig)
	s.protocolMu.Lock()
	s.Protocol = p
	s.protocolMu.Unlock()
}

func (s *Server) ProtocolInstance() *protocol.Protocol {
	s.protocolMu.RLock()
	defer s.protocolMu.RUnlock()
	return s.Protocol
}

func (s *Server) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeNotificationRequestNext:
		err = s.handleRequestNext()
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

func (s *Server) handleRequestNext() error {
	s.Protocol.Logger().
		Debug("notificiation request next",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config == nil || s.config.RequestNextFunc == nil {
		// No notification source is wired. This happens when a node negotiates
		// an N2N version that includes the Leios mini-protocols but does not
		// actively serve Leios notifications (e.g. a node following the chain
		// over chainsync/blockfetch without participating in Leios diffusion).
		// Leave the request pending in the Busy state instead of erroring:
		// leios-notify has no response timeout and no await message, so a
		// server with nothing to announce legitimately holds agency until it
		// has a notification. Erroring here would tear down the whole
		// connection -- including chainsync/blockfetch -- over an unanswered
		// notification request.
		return nil
	}
	resp, err := s.config.RequestNextFunc(
		s.callbackContext,
	)
	if err != nil {
		return err
	}
	if resp == nil {
		return errors.New(
			"received leios-notify NotificationRequestNext message but callback returned nil",
		)
	}
	sendErr := s.SendMessageAndWait(resp)
	if s.config.ResponseSentFunc != nil {
		s.config.ResponseSentFunc(s.callbackContext, resp, sendErr)
	}
	return sendErr
}

func (s *Server) handleDone() {
	s.Protocol.Logger().
		Debug("client done",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	// Restart protocol
	s.Stop()
	s.initProtocol()
	s.Start()
}
