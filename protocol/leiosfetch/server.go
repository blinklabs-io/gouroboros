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

package leiosfetch

import (
	"errors"
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/cbor"
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
		// NOTE: this MUST answer, not return nil. Returning nil left the
		// protocol in StateBlock holding server agency forever, which wedges
		// the requester's client permanently: its send loop waits for agency
		// that only this response can return, so it can never issue another
		// leios-fetch request on the connection (dingo issue #3623). The
		// MessageTypeNoBlock wire ID is a placeholder, but a configured
		// callback that reports ErrBlockNotFound already emits it below, so
		// declining here adds no wire risk that the normal path does not
		// already take.
		return s.SendMessage(NewMsgNoBlock())
	}
	msgBlockRequest := msg.(*MsgBlockRequest)
	resp, err := s.config.BlockRequestFunc(
		s.callbackContext,
		msgBlockRequest.Point,
	)
	if err != nil {
		// A not-found signal is answered with MsgNoBlock rather than being
		// propagated as a protocol violation that tears down the connection.
		if errors.Is(err, ErrBlockNotFound) {
			s.Protocol.Logger().
				Debug("endorser block not available",
					"component", "network",
					"protocol", ProtocolName,
					"role", "server",
					"connection_id", s.callbackContext.ConnectionId.String(),
					"point", fmt.Sprintf(
						"%d.%x",
						msgBlockRequest.Point.Slot,
						msgBlockRequest.Point.Hash,
					),
				)
			return s.SendMessage(NewMsgNoBlock())
		}
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
		// NOTE: as with handleBlockRequest, this MUST answer. Retaining
		// server agency in StateBlockTxs permanently desynchronises the
		// requester's leios-fetch client (dingo issue #3623), and the
		// configured not-available path below already puts this wire ID on
		// the wire.
		return s.SendMessage(NewMsgNoBlockTxs())
	}
	msgBlockTxsRequest := msg.(*MsgBlockTxsRequest)
	resp, err := s.config.BlockTxsRequestFunc(
		s.callbackContext,
		msgBlockTxsRequest.Point,
		msgBlockTxsRequest.Bitmaps,
	)
	if err != nil {
		// A not-found signal is answered with MsgNoBlockTxs rather than being
		// propagated as a protocol violation that tears down the connection.
		if errors.Is(err, ErrBlockTxsNotFound) {
			s.Protocol.Logger().
				Debug("endorser block transactions not available",
					"component", "network",
					"protocol", ProtocolName,
					"role", "server",
					"connection_id", s.callbackContext.ConnectionId.String(),
					"point", fmt.Sprintf(
						"%d.%x",
						msgBlockTxsRequest.Point.Slot,
						msgBlockTxsRequest.Point.Hash,
					),
				)
			return s.SendMessage(NewMsgNoBlockTxs())
		}
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
		return s.SendMessage(NewMsgVotes([]cbor.RawMessage{}))
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
		// NOTE: unlike Block/BlockTxs there is no absence reply for a range
		// request -- MsgLastBlockAndTxsInRange carries a mandatory block --
		// so this cannot decline gracefully. Fail the connection rather than
		// retain server agency in StateBlockRange forever: a silent hang
		// leaves the requester's leios-fetch client permanently wedged with
		// no way to detect it, while an error lets its peer governance drop
		// and replace this peer (dingo issue #3623).
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
