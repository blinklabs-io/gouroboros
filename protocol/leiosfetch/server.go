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

package leiosfetch

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/protocol"
)

type Server struct {
	*protocol.Protocol
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
	s.Protocol = protocol.New(protoConfig)
}

func (s *Server) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeBlockRequest:
		err = s.handleBlockRequest(msg)
	case MessageTypeBlockTxsRequest:
		err = s.handleBlockTxsRequest(msg)
	case MessageTypeVotesRequest:
		err = s.handleVotesRequest(msg)
	case MessageTypeBlockRangeRequest:
		err = s.handleBlockRangeRequest(msg)
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

func (s *Server) handleBlockRequest(msg protocol.Message) error {
	s.Protocol.Logger().
		Debug("block request",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config == nil || s.config.BlockRequestFunc == nil {
		return errors.New(
			"received leios-fetch BlockRequest message but no callback function is defined",
		)
	}
	msgBlockRequest := msg.(*MsgBlockRequest)
	resp, err := s.config.BlockRequestFunc(
		s.callbackContext,
		msgBlockRequest.Point,
	)
	if err != nil {
		return err
	}
	if resp == nil {
		return errors.New(
			"received leios-fetch BlockRequest message but callback returned nil",
		)
	}
	if err := s.SendMessage(resp); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleBlockTxsRequest(msg protocol.Message) error {
	s.Protocol.Logger().
		Debug("block Txs request",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config == nil || s.config.BlockTxsRequestFunc == nil {
		return errors.New(
			"received leios-fetch BlockTxsRequest message but no callback function is defined",
		)
	}
	msgBlockTxsRequest := msg.(*MsgBlockTxsRequest)
	resp, err := s.config.BlockTxsRequestFunc(
		s.callbackContext,
		msgBlockTxsRequest.Point,
		msgBlockTxsRequest.Bitmaps,
	)
	if err != nil {
		return err
	}
	if resp == nil {
		return errors.New(
			"received leios-fetch BlockTxsRequest message but callback returned nil",
		)
	}
	if err := s.SendMessage(resp); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleVotesRequest(msg protocol.Message) error {
	s.Protocol.Logger().
		Debug("votes request",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config == nil || s.config.VotesRequestFunc == nil {
		return errors.New(
			"received leios-fetch VotesRequest message but no callback function is defined",
		)
	}
	msgVotesRequest := msg.(*MsgVotesRequest)
	resp, err := s.config.VotesRequestFunc(
		s.callbackContext,
		msgVotesRequest.VoteIds,
	)
	if err != nil {
		return err
	}
	if resp == nil {
		return errors.New(
			"received leios-fetch VotesRequest message but callback returned nil",
		)
	}
	if err := s.SendMessage(resp); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleBlockRangeRequest(msg protocol.Message) error {
	s.Protocol.Logger().
		Debug("block range request",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config == nil || s.config.BlockRangeRequestFunc == nil {
		return errors.New(
			"received leios-fetch BlockRangeRequest message but no callback function is defined",
		)
	}
	msgBlockRangeRequest := msg.(*MsgBlockRangeRequest)
	err := s.config.BlockRangeRequestFunc(
		s.callbackContext,
		msgBlockRangeRequest.Start,
		msgBlockRangeRequest.End,
	)
	if err != nil {
		return err
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
	// Restart protocol
	s.Stop()
	s.initProtocol()
	s.Start()
}
