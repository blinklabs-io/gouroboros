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

package messagesubmission

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// Server implements the MessageSubmission server
type Server struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext

	// Server-side state for message queue management
	lock              sync.Mutex
	messageQueue      []*pcommon.DmqMessage
	pendingMessageIDs [][]byte
	requestInFlight   bool // Protects against concurrent request modifications (TOCTOU)
}

// NewServer returns a new MessageSubmission server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	s := &Server{
		config:            cfg,
		messageQueue:      make([]*pcommon.DmqMessage, 0),
		pendingMessageIDs: [][]byte{},
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
		InitialState:        protocolStateInit,
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

// AddMessage adds a message to the outbound queue
func (s *Server) AddMessage(msg *pcommon.DmqMessage) error {
	// Validate message before adding to queue
	if s.config.TTLValidator != nil {
		if err := s.config.TTLValidator.ValidateMessageTTL(msg); err != nil {
			return err
		}
	}
	if s.config.Authenticator != nil {
		if err := s.config.Authenticator.VerifyMessage(msg); err != nil {
			return err
		}
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	// Check queue size limit
	if len(s.messageQueue) >= s.config.MaxQueueSize {
		return errors.New("message queue full")
	}

	s.messageQueue = append(s.messageQueue, msg)

	s.Protocol.Logger().
		Debug("message added to queue",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
			"message_id", string(msg.Payload.MessageID),
			"queue_size", len(s.messageQueue),
		)

	return nil
}

// RequestMessageIdsBlocking sends a blocking request for message IDs
func (s *Server) RequestMessageIdsBlocking(
	ackCount, requestCount uint16,
) error {
	s.lock.Lock()

	// Protocol invariant: blocking request must be done if and only if buffer of unacknowledged ids is empty
	if len(s.pendingMessageIDs) > 0 {
		s.lock.Unlock()
		return errors.New(
			"cannot send blocking request when pending message IDs exist",
		)
	}

	// Prevent concurrent requests
	if s.requestInFlight {
		s.lock.Unlock()
		return errors.New("a request is already in flight")
	}

	if requestCount == 0 {
		s.lock.Unlock()
		return errors.New("cannot request 0 message IDs")
	}

	// Mark request as in-flight before releasing lock.
	s.requestInFlight = true
	// Prepare message
	msg := NewMsgRequestMessageIds(true, ackCount, requestCount)
	// Release lock before performing network I/O
	s.lock.Unlock()
	err := s.SendMessage(msg)
	// Re-acquire lock to update in-flight flag on error
	s.lock.Lock()
	if err != nil {
		s.requestInFlight = false
	}
	s.lock.Unlock()

	return err
}

// RequestMessageIdsNonBlocking sends a non-blocking request for message IDs
func (s *Server) RequestMessageIdsNonBlocking(
	ackCount, requestCount uint16,
) error {
	s.lock.Lock()

	// Protocol invariant: cannot request if buffer of unacknowledged ids is empty
	if len(s.pendingMessageIDs) == 0 {
		s.lock.Unlock()
		return errors.New(
			"cannot send non-blocking request when pending message IDs buffer is empty",
		)
	}

	// Prevent concurrent requests
	if s.requestInFlight {
		s.lock.Unlock()
		return errors.New("a request is already in flight")
	}

	if requestCount == 0 {
		s.lock.Unlock()
		return errors.New("cannot request 0 message IDs")
	}

	// Mark request as in-flight while holding the lock.
	s.requestInFlight = true
	// Prepare message
	msg := NewMsgRequestMessageIds(false, ackCount, requestCount)
	// Release lock before performing network I/O
	s.lock.Unlock()
	err := s.SendMessage(msg)
	// Re-acquire lock to update in-flight flag on error
	s.lock.Lock()
	if err != nil {
		s.requestInFlight = false
	}
	s.lock.Unlock()

	return err
}

// RequestMessages sends a request for specific messages by their IDs
func (s *Server) RequestMessages(messageIDs [][]byte) error {
	msg := NewMsgRequestMessages(messageIDs)
	return s.SendMessage(msg)
}

// GetAvailableMessages returns messages from the outbound queue up to maxCount
func (s *Server) GetAvailableMessages(maxCount int) []pcommon.DmqMessage {
	s.lock.Lock()
	defer s.lock.Unlock()

	var available []pcommon.DmqMessage
	now := time.Now().Unix()

	// Prune expired and gather up to maxCount
	filtered := make([]*pcommon.DmqMessage, 0, len(s.messageQueue))
	for _, msg := range s.messageQueue {
		if msg == nil {
			continue
		}
		// #nosec G115 -- Unix timestamp will not overflow uint32 until year 2106
		if uint32(now) <= msg.Payload.ExpiresAt {
			if len(available) < maxCount {
				available = append(available, *msg)
			} else {
				filtered = append(filtered, msg)
			}
		}
	}
	s.messageQueue = filtered

	return available
}

// GetAvailableMessageIDs returns message IDs from the queue up to count
func (s *Server) GetAvailableMessageIDs(count int) []pcommon.MessageIDAndSize {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Guard: prevent overwriting pending IDs if a request is already in flight
	if s.requestInFlight {
		s.Protocol.Logger().
			Debug("ignoring GetAvailableMessageIDs; request already in flight",
				"component", "network",
				"protocol", ProtocolName,
				"role", "server",
				"connection_id", s.callbackContext.ConnectionId.String(),
			)
		return []pcommon.MessageIDAndSize{}
	}

	ids := make([]pcommon.MessageIDAndSize, 0, count)
	now := time.Now().Unix()

	// Prune expired messages and collect IDs
	filtered := make([]*pcommon.DmqMessage, 0, len(s.messageQueue))
	for _, msg := range s.messageQueue {
		if msg == nil {
			continue
		}
		// #nosec G115 -- Unix timestamp will not overflow uint32 until year 2106
		if uint32(now) > msg.Payload.ExpiresAt {
			continue // skip expired messages, don't add to filtered
		}
		// #nosec G115 -- message body size bounded by protocol limits
		if len(ids) < count {
			ids = append(ids, pcommon.MessageIDAndSize{
				MessageID:   msg.Payload.MessageID,
				SizeInBytes: uint32(len(msg.Payload.MessageBody)),
			})
		}
		filtered = append(filtered, msg)
	}
	s.messageQueue = filtered

	// Append new pending IDs instead of overwriting to avoid dropping unacknowledged IDs
	if len(ids) > 0 {
		// Build a set of existing pending IDs to prevent duplicates across multiple calls
		existing := make(map[string]struct{}, len(s.pendingMessageIDs))
		for _, b := range s.pendingMessageIDs {
			existing[string(b)] = struct{}{}
		}
		for _, id := range ids {
			key := string(id.MessageID)
			if _, seen := existing[key]; !seen {
				s.pendingMessageIDs = append(s.pendingMessageIDs, id.MessageID)
				existing[key] = struct{}{}
			}
		}
	}

	return ids
}

// GetMessagesByIDs returns messages matching the provided IDs
func (s *Server) GetMessagesByIDs(ids [][]byte) []pcommon.DmqMessage {
	s.lock.Lock()
	defer s.lock.Unlock()

	messages := make([]pcommon.DmqMessage, 0, len(ids))
	idSet := make(map[string]struct{}, len(ids))
	now := time.Now().Unix()

	// Build a set of IDs for O(1) lookup
	for _, id := range ids {
		idSet[string(id)] = struct{}{}
	}

	// Check all messages once, return those matching the ID set
	for _, msg := range s.messageQueue {
		if msg == nil {
			continue
		}
		// #nosec G115 -- Unix timestamp will not overflow uint32 until year 2106
		if uint32(now) > msg.Payload.ExpiresAt {
			continue
		}
		if _, exists := idSet[string(msg.Payload.MessageID)]; exists {
			messages = append(messages, *msg)
		}
	}

	return messages
}

func (s *Server) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeInit:
		err = s.handleInit()
	case MessageTypeReplyMessageIds:
		err = s.handleReplyMessageIds(msg)
	case MessageTypeReplyMessages:
		err = s.handleReplyMessages(msg)
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

//nolint:unparam
func (s *Server) handleInit() error {
	s.Protocol.Logger().
		Debug("received init message",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	return nil
}

//nolint:unparam
func (s *Server) handleReplyMessageIds(msg protocol.Message) error {
	msgReply := msg.(*MsgReplyMessageIds)

	s.Protocol.Logger().
		Debug("received reply message IDs",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
			"message_count", len(msgReply.Messages),
		)

	s.lock.Lock()
	// Clear the in-flight flag now that response is received
	s.requestInFlight = false
	// Remove the replied IDs from pendingMessageIDs to mark them as acknowledged
	repliedIDs := make(map[string]struct{}, len(msgReply.Messages))
	for _, entry := range msgReply.Messages {
		repliedIDs[string(entry.MessageID)] = struct{}{}
	}
	filtered := make([][]byte, 0, len(s.pendingMessageIDs))
	for _, id := range s.pendingMessageIDs {
		if _, replied := repliedIDs[string(id)]; !replied {
			filtered = append(filtered, id)
		}
	}
	s.pendingMessageIDs = filtered
	s.lock.Unlock()

	// Invoke callback
	if s.config.ReplyMessageIdsFunc != nil {
		s.config.ReplyMessageIdsFunc(s.callbackContext, msgReply.Messages)
	}

	return nil
}

func (s *Server) handleReplyMessages(msg protocol.Message) error {
	msgReply := msg.(*MsgReplyMessages)

	s.Protocol.Logger().
		Debug("received reply messages",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
			"message_count", len(msgReply.Messages),
		)

	// Validate messages before invoking callback
	for i := range msgReply.Messages {
		if s.config.TTLValidator != nil {
			if err := s.config.TTLValidator.ValidateMessageTTL(&msgReply.Messages[i]); err != nil {
				s.Protocol.Logger().
					Warn("message validation failed",
						"component", "network",
						"protocol", ProtocolName,
						"role", "server",
						"connection_id", s.callbackContext.ConnectionId.String(),
						"error", err,
					)
				return err
			}
		}
		if s.config.Authenticator != nil {
			if err := s.config.Authenticator.VerifyMessage(&msgReply.Messages[i]); err != nil {
				s.Protocol.Logger().
					Warn("message authentication failed",
						"component", "network",
						"protocol", ProtocolName,
						"role", "server",
						"connection_id", s.callbackContext.ConnectionId.String(),
						"error", err,
					)
				return err
			}
		}
	}

	// Invoke callback
	if s.config.ReplyMessagesFunc != nil {
		s.config.ReplyMessagesFunc(s.callbackContext, msgReply.Messages)
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
