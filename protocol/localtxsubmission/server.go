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

package localtxsubmission

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the LocalTxSubmission server
type Server struct {
	proto        *protocol.Protocol // current instance; replaced on restart, guarded by protocolMu
	protocolMu   sync.RWMutex
	protoOptions protocol.ProtocolOptions
	// stopped is set by Stop under protocolMu. initProtocol checks it under
	// the same lock, so a restart racing with Stop installs no new instance.
	stopped         bool
	config          *Config
	callbackContext CallbackContext
}

// NewServer returns a new Server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
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

// initProtocol installs a new protocol instance for the next run, unless the
// server was stopped. It reports whether it installed one.
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
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        stateIdle,
	}
	p := protocol.New(protoConfig)
	s.protocolMu.Lock()
	defer s.protocolMu.Unlock()
	if s.stopped {
		return false
	}
	s.proto = p
	return true
}

// ProtocolInstance returns the protocol instance of the current run. The
// server replaces it after each MsgDone.
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
	case MessageTypeSubmitTx:
		err = s.handleSubmitTx(msg)
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

func (s *Server) handleSubmitTx(msg protocol.Message) error {
	s.ProtocolInstance().Logger().
		Debug("submit tx",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config.SubmitTxFunc == nil {
		return errors.New(
			"received local-tx-submission SubmitTx message but no callback function is defined",
		)
	}
	msgSubmitTx := msg.(*MsgSubmitTx)
	// Call the user callback function and send Accept/RejectTx based on result
	err := s.config.SubmitTxFunc(s.callbackContext.withOwner(s.ProtocolInstance()), msgSubmitTx.Transaction)
	if err == nil {
		newMsg := NewMsgAcceptTx()
		if err := s.ProtocolInstance().SendMessage(newMsg); err != nil {
			return err
		}
	} else {
		errCbor, encErr := encodeRejectReason(err)
		if encErr != nil {
			return encErr
		}
		newMsg := NewMsgRejectTx(errCbor)
		if err := s.ProtocolInstance().SendMessage(newMsg); err != nil {
			return err
		}
	}
	return nil
}

// handleDone restarts the server in Idle, as the reference's inbound
// governor restarts a responder that ended cleanly, so the client can start
// another run on the same connection.
func (s *Server) handleDone() {
	p := s.ProtocolInstance()
	p.Logger().
		Debug("done",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	_ = p.StopForRestart(p.CallbackContext())
	if !s.initProtocol() {
		p.CancelRestart()
		return
	}
	s.ProtocolInstance().Start()
}
