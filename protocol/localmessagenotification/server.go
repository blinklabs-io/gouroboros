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

package localmessagenotification

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// Server implements the LocalMessageNotification server
type Server struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext

	// Message queue management
	lock                   sync.Mutex
	messageQueue           []*pcommon.DmqMessage
	acknowledgedIDs        map[string]time.Time // Maps message ID to acknowledgment timestamp for TTL-based expiration
	acknowledgedIDsTTL     time.Duration        // TTL for acknowledged message IDs (default: 10 minutes)
	expirationTicker       *time.Ticker
	expirationStopChan     chan struct{}
	newMessageSignal       chan struct{}
	newMessageSignalClosed bool
	done                   chan struct{}
}

// NewServer returns a new LocalMessageNotification server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	s := &Server{
		config:             cfg,
		messageQueue:       make([]*pcommon.DmqMessage, 0),
		acknowledgedIDs:    make(map[string]time.Time),
		acknowledgedIDsTTL: 10 * time.Minute, // TTL for acknowledged message IDs
		expirationStopChan: make(chan struct{}),
		newMessageSignal:   make(chan struct{}, 1),
		done:               make(chan struct{}),
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
	// Start background goroutine to clean up expired acknowledged IDs after Protocol is set
	s.startExpirationCleaner()
	return s
}

// AddMessage adds a message to the notification queue
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

	// Check if already acknowledged (non-zero timestamp means it was acknowledged)
	msgID := string(msg.Payload.MessageID)
	if _, acknowledged := s.acknowledgedIDs[msgID]; acknowledged {
		return errors.New("message already acknowledged")
	}

	// Check queue size limit
	if len(s.messageQueue) >= s.config.MaxQueueSize {
		return errors.New("message queue full")
	}

	s.messageQueue = append(s.messageQueue, msg)

	// Signal that a new message is available (non-blocking).
	// Check the closed flag under the lock to avoid sending to a closed channel.
	if !s.newMessageSignalClosed {
		select {
		case s.newMessageSignal <- struct{}{}:
		default:
		}
	}

	s.Protocol.Logger().
		Debug("message added to queue",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
			"message_id", msgID,
			"queue_size", len(s.messageQueue),
		)

	return nil
}

// WaitForMessage blocks until a message is available or timeout occurs
func (s *Server) WaitForMessage(timeout time.Duration) error {
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case _, ok := <-s.newMessageSignal:
			if !ok {
				return errors.New("server shutting down")
			}
			return nil
		case <-timer.C:
			return errors.New("timeout waiting for message")
		case <-s.done:
			return errors.New("server shutting down")
		}
	}
	// Wait indefinitely
	select {
	case _, ok := <-s.newMessageSignal:
		if !ok {
			return errors.New("server shutting down")
		}
		return nil
	case <-s.done:
		return errors.New("server shutting down")
	}
}

func (s *Server) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeRequestMessages:
		err = s.handleRequestMessages(msg)
	case MessageTypeClientDone:
		err = s.handleClientDone()
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (s *Server) handleRequestMessages(msg protocol.Message) error {
	msgRequest := msg.(*MsgRequestMessages)

	s.Protocol.Logger().
		Debug("request messages",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
			"is_blocking", msgRequest.IsBlocking,
		)

	if msgRequest.IsBlocking {
		return s.handleBlockingRequest()
	}
	return s.handleNonBlockingRequest()
}

func (s *Server) handleNonBlockingRequest() error {
	s.lock.Lock()

	messages := make([]pcommon.DmqMessage, 0, len(s.messageQueue))
	for _, msg := range s.messageQueue {
		messages = append(messages, *msg)
	}

	// Mark all collected messages as acknowledged with current timestamp and clear the queue
	now := time.Now()
	for _, msg := range s.messageQueue {
		s.acknowledgedIDs[string(msg.Payload.MessageID)] = now
	}
	s.messageQueue = s.messageQueue[:0]

	s.lock.Unlock()

	// After clearing the queue, hasMore should always be false
	replyMsg := NewMsgReplyMessagesNonBlocking(messages, false)
	return s.SendMessage(replyMsg)
}

func (s *Server) handleBlockingRequest() error {
	// For blocking requests, wait until at least one message is available
	for {
		s.lock.Lock()
		hasMessages := len(s.messageQueue) > 0
		if hasMessages {
			break
		}
		s.lock.Unlock()

		// Wait for a message to arrive (no timeout - blocking indefinitely)
		if err := s.WaitForMessage(0); err != nil {
			// If we get an error (e.g., shutting down), return empty list
			replyMsg := NewMsgReplyMessagesBlocking([]pcommon.DmqMessage{})
			return s.SendMessage(replyMsg)
		}
	}

	// At this point, lock is held and hasMessages is true

	messages := make([]pcommon.DmqMessage, 0, len(s.messageQueue))
	for _, msg := range s.messageQueue {
		messages = append(messages, *msg)
	}

	// Mark all collected messages as acknowledged with current timestamp and clear the queue
	now := time.Now()
	for _, msg := range s.messageQueue {
		s.acknowledgedIDs[string(msg.Payload.MessageID)] = now
	}
	s.messageQueue = s.messageQueue[:0]

	s.lock.Unlock()

	replyMsg := NewMsgReplyMessagesBlocking(messages)
	return s.SendMessage(replyMsg)
}

//nolint:unparam
func (s *Server) handleClientDone() error {
	s.Protocol.Logger().
		Debug("received client done message",
			"component", "network",
			"protocol", ProtocolName,
			"role", "server",
			"connection_id", s.callbackContext.ConnectionId.String(),
		)
	// Close channels and signal shutdown
	s.lock.Lock()
	if !s.newMessageSignalClosed {
		// Close newMessageSignal to wake up any goroutines waiting on it
		close(s.newMessageSignal)
		// Close done channel to signal protocol shutdown
		close(s.done)
		s.newMessageSignalClosed = true
	}
	s.lock.Unlock()

	// Stop the expiration cleaner
	s.stopExpirationCleaner()
	return nil
}

// startExpirationCleaner starts a background goroutine that periodically cleans up expired acknowledged IDs
func (s *Server) startExpirationCleaner() {
	s.expirationTicker = time.NewTicker(1 * time.Minute) // Check every 1 minute
	go func() {
		for {
			select {
			case <-s.expirationTicker.C:
				s.cleanupExpiredAcknowledgedIDs()
			case <-s.expirationStopChan:
				s.expirationTicker.Stop()
				return
			case <-s.DoneChan():
				s.expirationTicker.Stop()
				return
			}
		}
	}()
}

// stopExpirationCleaner stops the background expiration cleanup goroutine
func (s *Server) stopExpirationCleaner() {
	s.lock.Lock()
	defer s.lock.Unlock()

	select {
	case <-s.expirationStopChan:
		// Already closed
	default:
		close(s.expirationStopChan)
	}
}

// cleanupExpiredAcknowledgedIDs removes acknowledged IDs that have exceeded their TTL
func (s *Server) cleanupExpiredAcknowledgedIDs() {
	s.lock.Lock()
	defer s.lock.Unlock()

	now := time.Now()
	expiredCount := 0

	for msgID, ackTime := range s.acknowledgedIDs {
		if now.Sub(ackTime) > s.acknowledgedIDsTTL {
			delete(s.acknowledgedIDs, msgID)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		s.Protocol.Logger().
			Debug("cleaned up expired acknowledged IDs",
				"component", "network",
				"protocol", ProtocolName,
				"role", "server",
				"connection_id", s.callbackContext.ConnectionId.String(),
				"count", expiredCount,
				"remaining", len(s.acknowledgedIDs),
			)
	}
}
