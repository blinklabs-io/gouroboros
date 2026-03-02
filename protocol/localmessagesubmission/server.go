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

package localmessagesubmission

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// Server implements the LocalMessageSubmission server
type Server struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
}

// NewServer returns a new LocalMessageSubmission server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	s := &Server{
		config: cfg,
	}
	s.callbackContext = CallbackContext{
		Server:       s,
		ConnectionId: protoOptions.ConnectionId,
	}
	protoConfig := protocol.ProtocolConfig{
		Name:                ProtocolName,
		ProtocolId:          ProtocolID,
		Muxer:               protoOptions.Muxer,
		Logger:              protoOptions.Logger,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            stateMap,
		InitialState:        protocolStateIdle,
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

func (s *Server) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeSubmitMessage:
		err = s.handleSubmitMessage(msg)
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

func (s *Server) handleSubmitMessage(msg protocol.Message) error {
	msgSubmit, ok := msg.(*MsgSubmitMessage)
	if !ok {
		err := fmt.Errorf(
			"%s: expected MsgSubmitMessage but got %T",
			ProtocolName,
			msg,
		)
		s.Protocol.Logger().
			Warn("unexpected message type in handleSubmitMessage",
				"component", "network",
				"protocol", ProtocolName,
				"role", "server",
				"connection_id", s.callbackContext.ConnectionId.String(),
				"error", err,
			)
		return err
	}

	s.Protocol.Logger().
		Debug("submit message",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)

	if s.config.SubmitMessageFunc == nil {
		err := errors.New(
			"received local-message-submission SubmitMessage message but no callback function is defined",
		)
		s.Protocol.Logger().
			Warn("submit message callback not configured",
				"component", "network",
				"protocol", ProtocolName,
				"role", "server",
				"connection_id", s.callbackContext.ConnectionId.String(),
				"error", err,
			)
		rejectMsg := NewMsgRejectMessage(
			pcommon.OtherReason{Message: err.Error()},
		)
		return s.SendMessage(rejectMsg)
	}

	// Validate inbound message before invoking handler
	if s.config.TTLValidator != nil {
		if err := s.config.TTLValidator.ValidateMessageTTL(&msgSubmit.Message); err != nil {
			s.Protocol.Logger().
				Warn("message validation failed",
					"component", "network",
					"protocol", ProtocolName,
					"role", "server",
					"connection_id", s.callbackContext.ConnectionId.String(),
					"error", err,
				)
			rejectMsg := NewMsgRejectMessage(
				pcommon.InvalidReason{Message: err.Error()},
			)
			return s.SendMessage(rejectMsg)
		}
	}
	if s.config.Authenticator != nil {
		if err := s.config.Authenticator.VerifyMessage(&msgSubmit.Message); err != nil {
			s.Protocol.Logger().
				Warn("message authentication failed",
					"component", "network",
					"protocol", ProtocolName,
					"role", "server",
					"connection_id", s.callbackContext.ConnectionId.String(),
					"error", err,
				)
			rejectMsg := NewMsgRejectMessage(
				pcommon.InvalidReason{Message: err.Error()},
			)
			return s.SendMessage(rejectMsg)
		}
	}

	// Call the user callback function and send Accept/RejectMessage based on result
	reason := s.config.SubmitMessageFunc(s.callbackContext, &msgSubmit.Message)
	if reason == nil {
		acceptMsg := NewMsgAcceptMessage()
		if err := s.SendMessage(acceptMsg); err != nil {
			return err
		}
	} else {
		// Log rejection reason for observability
		s.Protocol.Logger().
			Warn("message rejected by handler",
				"component", "network",
				"protocol", ProtocolName,
				"role", "server",
				"connection_id", s.callbackContext.ConnectionId.String(),
				"reason", reason,
			)
		rejectMsg := NewMsgRejectMessage(reason)
		if err := s.SendMessage(rejectMsg); err != nil {
			return err
		}
	}
	return nil
}

//nolint:unparam
func (s *Server) handleDone() error {
	s.Protocol.Logger().
		Debug("received done message",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	return nil
}
