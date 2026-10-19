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
	"sync"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// Server implements the LocalMessageSubmission server
type Server struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	now             func() time.Time
}

var (
	errInvalidReplayCacheCapacity = errors.New(
		"dmq: MaxReplayEntries must be greater than zero",
	)
	errReplayCacheCapacityExceeded = errors.New(
		"dmq: local submission replay cache capacity exceeded",
	)
)

type submissionReplayState struct {
	mu          sync.Mutex
	acceptedIDs map[string]replayEntry
	nextToken   uint64
}

// replayEntry records a reserved message ID. The token identifies the holder,
// so a release by a holder whose entry expired and was pruned cannot remove a
// newer holder's reservation of the same ID.
type replayEntry struct {
	expiresAt uint32
	token     uint64
}

func newSubmissionReplayState() *submissionReplayState {
	return &submissionReplayState{acceptedIDs: make(map[string]replayEntry)}
}

// reserve claims id until expiresAt. It returns false without error when the
// ID is already reserved. A reservation is held while the submission callback
// runs and is kept only if the callback accepts the message; otherwise the
// caller must release it with the returned token.
func (r *submissionReplayState) reserve(
	id []byte,
	expiresAt uint32,
	now time.Time,
	maxEntries int,
	commitAuthentication func() error,
) (uint64, bool, error) {
	if maxEntries < 1 {
		return 0, false, errInvalidReplayCacheCapacity
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for acceptedID, entry := range r.acceptedIDs {
		if now.Unix() > int64(entry.expiresAt) {
			delete(r.acceptedIDs, acceptedID)
		}
	}
	key := string(id)
	if _, exists := r.acceptedIDs[key]; exists {
		return 0, false, nil
	}
	if len(r.acceptedIDs) >= maxEntries {
		return 0, false, errReplayCacheCapacityExceeded
	}
	if commitAuthentication != nil {
		if err := commitAuthentication(); err != nil {
			return 0, false, fmt.Errorf(
				"commit message authentication: %w",
				err,
			)
		}
	}
	r.nextToken++
	r.acceptedIDs[key] = replayEntry{expiresAt: expiresAt, token: r.nextToken}
	return r.nextToken, true, nil
}

// release drops the reservation of id made with token, if it is still held.
func (r *submissionReplayState) release(id []byte, token uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := string(id)
	if entry, exists := r.acceptedIDs[key]; exists && entry.token == token {
		delete(r.acceptedIDs, key)
	}
}

// NewServer returns a new LocalMessageSubmission server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	normalizedCfg := normalizeConfig(cfg)
	s := &Server{
		config: &normalizedCfg,
		now:    time.Now,
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
		return s.refuse(msgSubmit, pcommon.OtherReason{Message: err.Error()})
	}

	// Validate inbound message before invoking handler
	if err := s.config.TTLValidator.ValidateMessageTTLAt(
		&msgSubmit.Message,
		s.now(),
	); err != nil {
		s.Protocol.Logger().
			Warn("message validation failed",
				"component", "network",
				"protocol", ProtocolName,
				"role", "server",
				"connection_id", s.callbackContext.ConnectionId.String(),
				"error", err,
			)
		return s.refuse(msgSubmit, ttlValidationRejectReason(err))
	}
	if s.config.Authenticator == nil {
		return errors.New("dmq: message authenticator not configured")
	}
	commitAuthentication, err := s.config.Authenticator.PrepareMessages(
		[]pcommon.DmqMessage{msgSubmit.Message},
	)
	if err != nil {
		s.Protocol.Logger().
			Warn("message authentication failed",
				"component", "network",
				"protocol", ProtocolName,
				"role", "server",
				"connection_id", s.callbackContext.ConnectionId.String(),
				"error", err,
			)
		return s.refuse(msgSubmit, pcommon.InvalidReason{Message: err.Error()})
	}
	now := s.now()
	if err := s.config.TTLValidator.ValidateMessageTTLAt(
		&msgSubmit.Message,
		now,
	); err != nil {
		return s.refuse(msgSubmit, ttlValidationRejectReason(err))
	}
	token, reserved, err := s.config.replayState.reserve(
		msgSubmit.Message.ID(),
		msgSubmit.Message.Payload.ExpiresAt,
		now,
		s.config.MaxReplayEntries,
		commitAuthentication,
	)
	if errors.Is(err, errReplayCacheCapacityExceeded) ||
		errors.Is(err, errInvalidReplayCacheCapacity) {
		return s.refuse(msgSubmit, pcommon.OtherReason{Message: err.Error()})
	}
	if err != nil {
		return s.refuse(msgSubmit, pcommon.InvalidReason{Message: err.Error()})
	}
	if !reserved {
		return s.refuse(msgSubmit, pcommon.AlreadyReceivedReason{})
	}

	// Only an accepted message keeps its ID reserved. A rejected one, such as
	// a message refused for a full pool or a KES period that has not begun,
	// must be admissible again when resubmitted. The release is deferred so
	// that a panicking callback does not strand the ID until it expires.
	accepted := false
	defer func() {
		if !accepted {
			s.config.replayState.release(msgSubmit.Message.ID(), token)
		}
	}()
	reason := s.config.SubmitMessageFunc(s.callbackContext, &msgSubmit.Message)
	if reason != nil {
		s.Protocol.Logger().
			Warn("message rejected by handler",
				"component", "network",
				"protocol", ProtocolName,
				"role", "server",
				"connection_id", s.callbackContext.ConnectionId.String(),
				"reason", reason,
			)
		return s.sendReject(reason)
	}
	accepted = true
	return s.SendMessage(NewMsgAcceptMessage())
}

// refuse rejects a submission before it reaches SubmitMessageFunc, reporting
// it to SubmitRefusedFunc first so the refusal is observable by the time the
// client sees it.
func (s *Server) refuse(
	msg *MsgSubmitMessage,
	reason pcommon.RejectReason,
) error {
	if s.config.SubmitRefusedFunc != nil {
		s.config.SubmitRefusedFunc(s.callbackContext, &msg.Message, reason)
	}
	return s.sendReject(reason)
}

func (s *Server) sendReject(reason pcommon.RejectReason) error {
	rejectMsg, err := NewMsgRejectMessage(reason)
	if err != nil {
		return err
	}
	return s.SendMessage(rejectMsg)
}

func ttlValidationRejectReason(err error) pcommon.RejectReason {
	if errors.Is(err, pcommon.ErrMessageExpired) {
		return pcommon.ExpiredReason{}
	}
	return pcommon.InvalidReason{Message: err.Error()}
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
