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

package blockfetch

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
)

type Server struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
}

func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
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
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        StateIdle,
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

func (s *Server) NoBlocks() error {
	msg := NewMsgNoBlocks()
	return s.SendMessage(msg)
}

func (s *Server) StartBatch() error {
	msg := NewMsgStartBatch()
	return s.SendMessage(msg)
}

func (s *Server) Block(blockType uint, blockData []byte) error {
	wrappedBlock := WrappedBlock{
		Type:     blockType,
		RawBlock: blockData,
	}
	wrappedBlockData, err := cbor.Encode(&wrappedBlock)
	if err != nil {
		return err
	}
	msg := NewMsgBlock(wrappedBlockData)
	return s.SendMessage(msg)
}

func (s *Server) BatchDone() error {
	msg := NewMsgBatchDone()
	return s.SendMessage(msg)
}

func (s *Server) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeRequestRange:
		err = s.handleRequestRange(msg)
	case MessageTypeClientDone:
		err = s.handleClientDone()
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (s *Server) handleRequestRange(msg protocol.Message) error {
	if s.config == nil || s.config.RequestRangeFunc == nil {
		return fmt.Errorf(
			"received block-fetch RequestRange message but no callback function is defined",
		)
	}
	msgRequestRange := msg.(*MsgRequestRange)
	return s.config.RequestRangeFunc(s.callbackContext, msgRequestRange.Start, msgRequestRange.End)
}

func (s *Server) handleClientDone() error {
	return nil
}
