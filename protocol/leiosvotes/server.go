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
	"context"
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
	// stopped is set by Stop under protocolMu. initProtocol checks it under
	// the same lock, so a restart racing with Stop installs no new instance.
	stopped bool
}

func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	cfg = normalizeConfig(cfg)
	s := &Server{
		config:       cfg,
		protoOptions: protoOptions,
	}
	s.callbackContext = CallbackContext{
		Server:             s,
		ConnectionId:       protoOptions.ConnectionId,
		ConnectionDoneChan: protoOptions.ConnectionDoneChan,
	}
	s.initProtocol()
	return s
}

func (s *Server) initProtocol() bool {
	// The server holds agency in Busy, so it never times that state; the
	// client times the wait for its votes.
	stateMap := StateMap.Copy()
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
	if s.stopped {
		s.protocolMu.Unlock()
		return false
	}
	s.Protocol = p
	s.protocolMu.Unlock()
	return true
}

func (s *Server) ProtocolInstance() *protocol.Protocol {
	s.protocolMu.RLock()
	defer s.protocolMu.RUnlock()
	return s.Protocol
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
	p := s.Protocol
	s.protocolMu.Unlock()
	return p.Stop(ctx)
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
		// CIP-0164 permits only Vote while the server has agency in Busy.
		// Keep the request pending rather than inventing a response or failing
		// the shared bearer.
		return nil
	}
	votes, err := s.config.RequestNextFunc(
		s.callbackContext.withOwner(s.ProtocolInstance()),
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
	old := s.ProtocolInstance()
	_ = old.StopForRestart(old.CallbackContext())
	if !s.initProtocol() {
		old.CancelRestart()
		return
	}
	s.Start()
}
