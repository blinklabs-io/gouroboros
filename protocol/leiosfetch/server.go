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
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
)

type Server struct {
	proto           *protocol.Protocol // current instance; replaced on restart, guarded by protocolMu
	protocolMu      sync.RWMutex
	config          *Config
	callbackContext CallbackContext
	protoOptions    protocol.ProtocolOptions
	// stopped is set by Stop under protocolMu. initProtocol checks it under
	// the same lock, so a restart racing with Stop installs no new instance.
	stopped bool
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

func (s *Server) initProtocol() bool {
	protoConfig := protocol.ProtocolConfig{
		Name:                ProtocolName,
		ProtocolId:          ProtocolId,
		Muxer:               s.protoOptions.Muxer,
		Logger:              s.protoOptions.Logger,
		ErrorChan:           s.protoOptions.ErrorChan,
		Mode:                s.protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.messageHandler,
		MessageFromCborFunc: serverMessageFromCbor,
		StateMap:            StateMap,
		InitialState:        StateIdle,
	}
	p := protocol.New(protoConfig)
	s.protocolMu.Lock()
	if s.stopped {
		s.protocolMu.Unlock()
		return false
	}
	s.proto = p
	s.protocolMu.Unlock()
	return true
}

func serverMessageFromCbor(
	msgType uint,
	data []byte,
) (protocol.Message, error) {
	switch msgType {
	case MessageTypeBlockRequest, MessageTypeBlockTxsRequest,
		MessageTypeVotesRequest, MessageTypeBlockRangeRequest, MessageTypeDone:
		return NewMsgFromCbor(msgType, data)
	default:
		return nil, fmt.Errorf(
			"%s: message type %d is not a client request",
			ProtocolName,
			msgType,
		)
	}
}

func (s *Server) ProtocolInstance() *protocol.Protocol {
	s.protocolMu.RLock()
	defer s.protocolMu.RUnlock()
	return s.proto
}

// Stop stops the server. A restart that is in progress when Stop is called
// does not start a new protocol instance.
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
	p := s.proto
	s.protocolMu.Unlock()
	return p.Stop(ctx)
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
	s.ProtocolInstance().Logger().
		Debug("block request",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config == nil || s.config.BlockRequestFunc == nil {
		return errors.New(
			"received leios-fetch BlockRequest message " +
				"but no callback function is defined",
		)
	}
	msgBlockRequest := msg.(*MsgBlockRequest)
	resp, err := s.config.BlockRequestFunc(
		s.callbackContext.withOwner(s.ProtocolInstance()),
		msgBlockRequest.Point,
	)
	if err != nil {
		if errors.Is(err, ErrBlockNotFound) {
			s.ProtocolInstance().Logger().
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
		}
		return err
	}
	if resp == nil {
		return errors.New(
			"received leios-fetch BlockRequest message but callback returned nil",
		)
	}
	if err := s.ProtocolInstance().SendMessage(resp); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleBlockTxsRequest(msg protocol.Message) error {
	s.ProtocolInstance().Logger().
		Debug("block Txs request",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config == nil || s.config.BlockTxsRequestFunc == nil {
		return errors.New(
			"received leios-fetch BlockTxsRequest message " +
				"but no callback function is defined",
		)
	}
	msgBlockTxsRequest := msg.(*MsgBlockTxsRequest)
	resp, err := s.config.BlockTxsRequestFunc(
		s.callbackContext.withOwner(s.ProtocolInstance()),
		msgBlockTxsRequest.Point,
		msgBlockTxsRequest.Bitmaps,
	)
	if err != nil {
		if errors.Is(err, ErrBlockTxsNotFound) {
			s.ProtocolInstance().Logger().
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
		}
		return err
	}
	if resp == nil {
		return errors.New(
			"received leios-fetch BlockTxsRequest message but callback returned nil",
		)
	}
	if err := s.ProtocolInstance().SendMessage(resp); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleVotesRequest(msg protocol.Message) error {
	s.ProtocolInstance().Logger().
		Debug("votes request",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config == nil || s.config.VotesRequestFunc == nil {
		return s.ProtocolInstance().SendMessage(NewMsgVotes([]cbor.RawMessage{}))
	}
	msgVotesRequest := msg.(*MsgVotesRequest)
	resp, err := s.config.VotesRequestFunc(
		s.callbackContext.withOwner(s.ProtocolInstance()),
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
	if err := s.ProtocolInstance().SendMessage(resp); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleBlockRangeRequest(msg protocol.Message) error {
	s.ProtocolInstance().Logger().
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
		s.callbackContext.withOwner(s.ProtocolInstance()),
		msgBlockRangeRequest.Start,
		msgBlockRangeRequest.End,
	)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) handleDone() {
	s.ProtocolInstance().Logger().
		Debug("client done",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	// Restart protocol
	old := s.ProtocolInstance()
	_ = old.StopForRestart(old.CallbackContext())
	if !s.initProtocol() {
		old.CancelRestart()
		return
	}
	s.Start()
}
