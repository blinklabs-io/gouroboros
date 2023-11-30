// Copyright 2023 Blink Labs Software
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

package chainsync

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the ChainSync server
type Server struct {
	*protocol.Protocol
	config *Config
}

// NewServer returns a new ChainSync server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	// Use node-to-client protocol ID
	ProtocolId := ProtocolIdNtC
	msgFromCborFunc := NewMsgFromCborNtC
	if protoOptions.Mode == protocol.ProtocolModeNodeToNode {
		// Use node-to-node protocol ID
		ProtocolId = ProtocolIdNtN
		msgFromCborFunc = NewMsgFromCborNtN
	}
	s := &Server{
		config: cfg,
	}
	protoConfig := protocol.ProtocolConfig{
		Name:                ProtocolName,
		ProtocolId:          ProtocolId,
		Muxer:               protoOptions.Muxer,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.messageHandler,
		MessageFromCborFunc: msgFromCborFunc,
		StateMap:            StateMap,
		InitialState:        stateIdle,
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

func (s *Server) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeRequestNext:
		err = s.handleRequestNext(msg)
	case MessageTypeFindIntersect:
		err = s.handleFindIntersect(msg)
	case MessageTypeDone:
		err = s.handleDone()
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
	if s.config == nil || s.config.RequestNextFunc == nil {
		return fmt.Errorf(
			"received chain-sync RequestNext message but no callback function is defined",
		)
	}
	msgResp, err := s.config.RequestNextFunc()
	if err != nil {
		return err
	}
	if err := s.SendMessage(msgResp); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleFindIntersect(msg protocol.Message) error {
	if s.config == nil || s.config.FindIntersectFunc == nil {
		return fmt.Errorf(
			"received chain-sync FindIntersect message but no callback function is defined",
		)
	}
	msgFindIntersect := msg.(*MsgFindIntersect)
	point, tip, err := s.config.FindIntersectFunc(msgFindIntersect.Points)
	if err != nil {
		if err == IntersectNotFoundError {
			msgResp := NewMsgIntersectNotFound(tip)
			if err := s.SendMessage(msgResp); err != nil {
				return err
			}
		}
	}
	msgResp := NewMsgIntersectFound(point, tip)
	if err := s.SendMessage(msgResp); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleDone() error {
	return nil
}
