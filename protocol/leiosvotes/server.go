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
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/protocol"
)

type Server struct {
	*protocol.Protocol
	callbackContext CallbackContext
	config          *Config
	protocolMu      sync.RWMutex
	protoOptions    protocol.ProtocolOptions
}

func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	cfg = normalizeConfig(cfg)
	s := &Server{
		config:       cfg,
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
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[StateBusy]; ok {
		timeout := DefaultTimeout
		if s.config != nil && s.config.Timeout != 0 {
			timeout = s.config.Timeout
		}
		entry.Timeout = timeout
		stateMap[StateBusy] = entry
	}
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
		StateContext:        &stateContext{},
		StateMap:            stateMap,
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
	case MessageTypeVotesRequestNext:
		err = s.handleRequestNext(msg)
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

func (s *Server) handleRequestNext(msg protocol.Message) error {
	msgRequestNext := msg.(*MsgVotesRequestNext)
	s.Protocol.Logger().
		Debug("votes request next",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
			"count", msgRequestNext.Count,
		)
	if msgRequestNext.Count == 0 || msgRequestNext.Count > MaxRequestNextCount {
		return fmt.Errorf(
			"received leios-votes VotesRequestNext message with invalid count %d",
			msgRequestNext.Count,
		)
	}
	if s.config == nil || s.config.RequestNextFunc == nil {
		return errors.New(
			"received leios-votes VotesRequestNext message but no callback function is defined",
		)
	}
	votes, err := s.config.RequestNextFunc(
		s.callbackContext,
		msgRequestNext.Count,
	)
	if err != nil {
		return err
	}
	if len(votes) != int(msgRequestNext.Count) {
		return fmt.Errorf(
			"received leios-votes VotesRequestNext message for %d votes but callback returned %d",
			msgRequestNext.Count,
			len(votes),
		)
	}
	for _, vote := range votes {
		if err := s.SendMessage(NewMsgVote(vote)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) handleDone() {
	s.Protocol.Logger().
		Debug("client done",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	s.Stop()
	s.initProtocol()
	s.Start()
}
