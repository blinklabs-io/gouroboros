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
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// Server implements the LocalMessageSubmission server
type Server struct {
	proto           *protocol.Protocol // current instance; replaced on restart, guarded by protocolMu
	config          *Config
	callbackContext CallbackContext
	now             func() time.Time
	protocolMu      sync.RWMutex
	protoOptions    protocol.ProtocolOptions
	// stopped is set by Stop under protocolMu. initProtocol checks it under
	// the same lock, so a restart racing with Stop installs no new instance.
	stopped bool
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
		config:       &normalizedCfg,
		now:          time.Now,
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
		ProtocolId:          ProtocolID,
		Muxer:               s.protoOptions.Muxer,
		Logger:              s.protoOptions.Logger,
		ErrorChan:           s.protoOptions.ErrorChan,
		Mode:                s.protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            stateMap,
		InitialState:        protocolStateIdle,
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

// Start starts the protocol instance of the current run.
func (s *Server) Start() {
	s.ProtocolInstance().Start()
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
		s.ProtocolInstance().Logger().
			Warn("unexpected message type in handleSubmitMessage",
				"component", "network",
				"protocol", ProtocolName,
				"role", "server",
				"connection_id", s.callbackContext.ConnectionId.String(),
				"error", err,
			)
		return err
	}

	s.ProtocolInstance().Logger().
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
		s.ProtocolInstance().Logger().
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
		s.ProtocolInstance().Logger().
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
		s.ProtocolInstance().Logger().
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
	// that a panicking callback does not strand the ID until it expires, and
	// is repeated before the rejection is sent so a client that resubmits as
	// soon as it sees the reply cannot find the ID still reserved.
	accepted := false
	defer func() {
		if !accepted {
			s.config.replayState.release(msgSubmit.Message.ID(), token)
		}
	}()
	reason := s.config.SubmitMessageFunc(s.callbackContext.withOwner(s.ProtocolInstance()), &msgSubmit.Message)
	if reason != nil {
		s.ProtocolInstance().Logger().
			Warn("message rejected by handler",
				"component", "network",
				"protocol", ProtocolName,
				"role", "server",
				"connection_id", s.callbackContext.ConnectionId.String(),
				"reason", reason,
			)
		s.config.replayState.release(msgSubmit.Message.ID(), token)
		return s.sendReject(reason)
	}
	accepted = true
	return s.ProtocolInstance().SendMessage(NewMsgAcceptMessage())
}

// refuse rejects a submission before it reaches SubmitMessageFunc, reporting
// it to SubmitRefusedFunc first so the refusal is observable by the time the
// client sees it.
func (s *Server) refuse(
	msg *MsgSubmitMessage,
	reason pcommon.RejectReason,
) error {
	if s.config.SubmitRefusedFunc != nil {
		s.config.SubmitRefusedFunc(s.callbackContext.withOwner(s.ProtocolInstance()), &msg.Message, reason)
	}
	return s.sendReject(reason)
}

func (s *Server) sendReject(reason pcommon.RejectReason) error {
	rejectMsg, err := NewMsgRejectMessage(reason)
	if err != nil {
		return err
	}
	return s.ProtocolInstance().SendMessage(rejectMsg)
}

func ttlValidationRejectReason(err error) pcommon.RejectReason {
	if errors.Is(err, pcommon.ErrMessageExpired) {
		return pcommon.ExpiredReason{}
	}
	return pcommon.InvalidReason{Message: err.Error()}
}

// handleDone restarts the server in its initial state. dmq-node runs its
// node-to-client servers under the inbound governor, which restarts a
// responder that ended cleanly, so the client may start a new run on the
// connection. The replay cache lives in the config and outlasts the run.
//
//nolint:unparam
func (s *Server) handleDone() error {
	old := s.ProtocolInstance()
	old.Logger().
		Debug("received done message",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	_ = old.StopForRestart(old.CallbackContext())
	if !s.initProtocol() {
		old.CancelRestart()
		return nil
	}
	s.Start()
	return nil
}
