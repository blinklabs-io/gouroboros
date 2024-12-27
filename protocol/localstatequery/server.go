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

package localstatequery

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Server implements the LocalStateQuery server
type Server struct {
	*protocol.Protocol
	config                        *Config
	callbackContext               CallbackContext
	enableGetChainBlockNo         bool
	enableGetChainPoint           bool
	enableGetRewardInfoPoolsBlock bool
}

// NewServer returns a new LocalStateQuery server object
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
		Logger:              protoOptions.Logger,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        stateIdle,
	}
	// Enable version-dependent features
	if (protoOptions.Version - protocol.ProtocolVersionNtCOffset) >= 10 {
		s.enableGetChainBlockNo = true
		s.enableGetChainPoint = true
	}
	if (protoOptions.Version - protocol.ProtocolVersionNtCOffset) >= 11 {
		s.enableGetRewardInfoPoolsBlock = true
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

func (s *Server) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeAcquire:
		err = s.handleAcquire(msg)
	case MessageTypeQuery:
		err = s.handleQuery(msg)
	case MessageTypeRelease:
		err = s.handleRelease()
	case MessageTypeReacquire:
		err = s.handleReAcquire(msg)
	case MessageTypeAcquireVolatileTip:
		err = s.handleAcquire(msg)
	case MessageTypeReacquireVolatileTip:
		err = s.handleReAcquire(msg)
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

func (s *Server) handleAcquire(msg protocol.Message) error {
	s.Protocol.Logger().
		Debug("acquire",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config.AcquireFunc == nil {
		return fmt.Errorf(
			"received local-state-query Acquire message but no callback function is defined",
		)
	}
	var acquireTarget AcquireTarget
	switch msgAcquire := msg.(type) {
	case *MsgAcquire:
		acquireTarget = AcquireSpecificPoint{
			Point: msgAcquire.Point,
		}
	case *MsgAcquireVolatileTip:
		acquireTarget = AcquireVolatileTip{}
	case *MsgAcquireImmutableTip:
		acquireTarget = AcquireImmutableTip{}
	}
	// Call the user callback function
	return s.config.AcquireFunc(s.callbackContext, acquireTarget)
}

func (s *Server) handleQuery(msg protocol.Message) error {
	s.Protocol.Logger().
		Debug("query",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config.QueryFunc == nil {
		return fmt.Errorf(
			"received local-state-query Query message but no callback function is defined",
		)
	}
	msgQuery := msg.(*MsgQuery)
	// Call the user callback function
	return s.config.QueryFunc(s.callbackContext, msgQuery.Query)
}

func (s *Server) handleRelease() error {
	s.Protocol.Logger().
		Debug("release",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config.ReleaseFunc == nil {
		return fmt.Errorf(
			"received local-state-query Release message but no callback function is defined",
		)
	}
	// Call the user callback function
	return s.config.ReleaseFunc(s.callbackContext)
}

func (s *Server) handleReAcquire(msg protocol.Message) error {
	s.Protocol.Logger().
		Debug("reacquire",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config.ReAcquireFunc == nil {
		return fmt.Errorf(
			"received local-state-query ReAcquire message but no callback function is defined",
		)
	}
	var acquireTarget AcquireTarget
	switch msgReAcquire := msg.(type) {
	case *MsgReAcquire:
		acquireTarget = AcquireSpecificPoint{
			Point: msgReAcquire.Point,
		}
	case *MsgReAcquireVolatileTip:
		acquireTarget = AcquireVolatileTip{}
	case *MsgReAcquireImmutableTip:
		acquireTarget = AcquireImmutableTip{}
	}
	// Call the user callback function
	return s.config.ReAcquireFunc(s.callbackContext, acquireTarget)
}

func (s *Server) handleDone() error {
	s.Protocol.Logger().
		Debug("done",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	if s.config.DoneFunc == nil {
		return fmt.Errorf(
			"received local-state-query Done message but no callback function is defined",
		)
	}
	// Call the user callback function
	return s.config.DoneFunc(s.callbackContext)
}
