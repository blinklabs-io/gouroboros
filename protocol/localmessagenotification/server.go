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
	"bytes"
	"context"
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
	protocolMu      sync.RWMutex
	protoOptions    protocol.ProtocolOptions
	// stopped is set by Stop under protocolMu. initProtocol checks it under
	// the same lock, so a restart racing with Stop installs no new instance.
	stopped bool

	// Message queue management
	lock               sync.Mutex
	messageQueue       []*pcommon.DmqMessage
	acknowledgedIDs    map[string]time.Time // Maps message ID to acknowledgment timestamp for TTL-based expiration
	acknowledgedIDsTTL time.Duration        // TTL for acknowledged message IDs (default: 10 minutes)
	expirationTicker   *time.Ticker
	expirationStopChan chan struct{}
	expirationDoneChan chan struct{}
	// messageAdded is closed, and replaced, by every AddMessage. A waiter
	// takes the current channel under lock, so it wakes for any message
	// added after it last looked at the queue, and every waiter wakes, not
	// only one.
	messageAdded       chan struct{}
	done               chan struct{}
	stopOnce           sync.Once
	connectionDoneChan <-chan any
	muxerDoneChan      <-chan bool
	now                func() time.Time
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
		expirationDoneChan: make(chan struct{}),
		messageAdded:       make(chan struct{}),
		done:               make(chan struct{}),
		connectionDoneChan: protoOptions.ConnectionDoneChan,
		now:                time.Now,
		protoOptions:       protoOptions,
	}
	if protoOptions.Muxer != nil {
		s.muxerDoneChan = protoOptions.Muxer.DoneChan()
	}
	s.callbackContext = CallbackContext{
		Server:       s,
		ConnectionId: protoOptions.ConnectionId,
	}
	s.initProtocol()
	// Start background goroutine to clean up expired acknowledged IDs after Protocol is set
	s.startExpirationCleaner()
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
	s.Protocol = p
	return true
}

// ProtocolInstance returns the protocol instance of the current run. The
// server replaces it after each MsgClientDone.
func (s *Server) ProtocolInstance() *protocol.Protocol {
	s.protocolMu.RLock()
	defer s.protocolMu.RUnlock()
	return s.Protocol
}

// Start starts the protocol instance of the current run.
func (s *Server) Start() {
	s.ProtocolInstance().Start()
}

// AddMessage adds a message to the notification queue
func (s *Server) AddMessage(msg *pcommon.DmqMessage) error {
	if msg == nil {
		return errors.New("message is nil")
	}
	if s.config.TTLValidator == nil {
		return errors.New("dmq: TTL validator not configured")
	}
	if err := s.config.TTLValidator.ValidateMessageTTLAt(msg, s.now()); err != nil {
		return err
	}
	if s.config.Authenticator == nil {
		return errors.New("dmq: message authenticator not configured")
	}
	queued := cloneDmqMessage(*msg)
	commitAuthentication, err := s.config.Authenticator.PrepareMessages(
		[]pcommon.DmqMessage{queued},
	)
	if err != nil {
		return err
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	// Check if already acknowledged (non-zero timestamp means it was acknowledged)
	msgID := string(queued.ID())
	if _, acknowledged := s.acknowledgedIDs[msgID]; acknowledged {
		return errors.New("message already acknowledged")
	}
	for _, queued := range s.messageQueue {
		if string(queued.ID()) == msgID {
			return errors.New("message already queued")
		}
	}

	// Check queue size limit
	if len(s.messageQueue) >= s.config.MaxQueueSize {
		return errors.New("message queue full")
	}
	if err := s.config.TTLValidator.ValidateMessageTTLAt(&queued, s.now()); err != nil {
		return err
	}
	if err := commitAuthentication(); err != nil {
		return err
	}

	s.messageQueue = append(s.messageQueue, &queued)

	close(s.messageAdded)
	s.messageAdded = make(chan struct{})

	s.ProtocolInstance().Logger().
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

func cloneDmqMessage(msg pcommon.DmqMessage) pcommon.DmqMessage {
	msg.MessageID = bytes.Clone(msg.MessageID)
	msg.Payload.MessageID = bytes.Clone(msg.Payload.MessageID)
	msg.Payload.MessageBody = bytes.Clone(msg.Payload.MessageBody)
	msg.KESSignature = bytes.Clone(msg.KESSignature)
	msg.OperationalCertificate.KESVerificationKey = bytes.Clone(
		msg.OperationalCertificate.KESVerificationKey,
	)
	msg.OperationalCertificate.ColdSignature = bytes.Clone(
		msg.OperationalCertificate.ColdSignature,
	)
	msg.ColdVerificationKey = bytes.Clone(msg.ColdVerificationKey)
	return msg
}

// WaitForMessage blocks until a message is queued or timeout occurs. It
// returns at once if a message is already queued. A timeout of zero waits
// indefinitely. The client ending a run does not end the wait: the server
// restarts and keeps its queue. Stop, or the connection closing, ends it
// with protocol.ErrProtocolShuttingDown.
func (s *Server) WaitForMessage(timeout time.Duration) error {
	s.lock.Lock()
	if len(s.messageQueue) > 0 {
		s.lock.Unlock()
		return nil
	}
	added := s.messageAdded
	s.lock.Unlock()
	return s.waitForMessageAdded(added, timeout, nil)
}

// waitForMessageAdded waits until added is closed by AddMessage, the
// timeout passes, the server shuts down, or runStop is closed. A blocking
// request passes its run's stop channel; WaitForMessage passes nil, since
// a restart closes that channel and the wait belongs to the server.
func (s *Server) waitForMessageAdded(
	added <-chan struct{},
	timeout time.Duration,
	runStop <-chan struct{},
) error {
	var timerChan <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		timerChan = timer.C
	}
	select {
	case <-added:
		return nil
	case <-timerChan:
		return errors.New("timeout waiting for message")
	case <-s.done:
		return protocol.ErrProtocolShuttingDown
	case <-runStop:
		return protocol.ErrProtocolShuttingDown
	case <-s.connectionDoneChan:
		return protocol.ErrProtocolShuttingDown
	case <-s.muxerDoneChan:
		return protocol.ErrProtocolShuttingDown
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

	s.ProtocolInstance().Logger().
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
	messages := s.drainValidMessagesLocked(time.Now())
	s.lock.Unlock()

	// After clearing the queue, hasMore should always be false
	replyMsg := NewMsgReplyMessagesNonBlocking(messages, false)
	return s.ProtocolInstance().SendMessage(replyMsg)
}

func (s *Server) handleBlockingRequest() error {
	// For blocking requests, wait until at least one message is available
	runStop := s.ProtocolInstance().StopChan()
	for {
		s.lock.Lock()
		messages := s.drainValidMessagesLocked(time.Now())
		if len(messages) > 0 {
			s.lock.Unlock()
			replyMsg := NewMsgReplyMessagesBlocking(messages)
			return s.ProtocolInstance().SendMessage(replyMsg)
		}
		// Take the channel before releasing the lock, so a message added
		// between the drain and the wait still wakes this request.
		added := s.messageAdded
		s.lock.Unlock()

		if err := s.waitForMessageAdded(added, 0, runStop); err != nil {
			return err
		}
	}
}

func (s *Server) drainValidMessagesLocked(now time.Time) []pcommon.DmqMessage {
	messages := make([]pcommon.DmqMessage, 0, len(s.messageQueue))
	for _, msg := range s.messageQueue {
		if msg == nil || !msg.IsValidAt(now) {
			continue
		}
		messages = append(messages, *msg)
		s.acknowledgedIDs[string(msg.ID())] = now
	}
	s.messageQueue = s.messageQueue[:0]
	return messages
}

// handleClientDone restarts the server in its initial state. dmq-node runs
// its node-to-client servers under the inbound governor, which restarts a
// responder that ended cleanly, so the client may start a new run on the
// connection. The queue, the acknowledged IDs and the expiration cleaner
// belong to the connection and outlast the run; only Stop ends them.
//
//nolint:unparam
func (s *Server) handleClientDone() error {
	old := s.ProtocolInstance()
	old.Logger().
		Debug("received client done message",
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

// Stop terminates the server and releases background resources. It is safe to
// call before Start when protocol registration or construction fails. A
// restart that is in progress when Stop is called does not start a new
// protocol instance.
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
	s.stop()
	return p.Stop(ctx)
}

func (s *Server) stop() {
	s.stopOnce.Do(func() {
		close(s.done)
		s.stopExpirationCleaner()
	})
}

// startExpirationCleaner starts a background goroutine that periodically cleans up expired acknowledged IDs
func (s *Server) startExpirationCleaner() {
	s.expirationTicker = time.NewTicker(1 * time.Minute) // Check every 1 minute
	go func() {
		defer close(s.expirationDoneChan)
		defer s.expirationTicker.Stop()
		for {
			select {
			case <-s.expirationTicker.C:
				s.cleanupExpiredAcknowledgedIDs()
			// The protocol instance's stop and done channels are not
			// watched: a restart closes them, and the cleaner serves every
			// run. Stop ends it through expirationStopChan.
			case <-s.expirationStopChan:
				return
			case <-s.connectionDoneChan:
				return
			case <-s.muxerDoneChan:
				return
			}
		}
	}()
}

// stopExpirationCleaner stops the background expiration cleanup goroutine
func (s *Server) stopExpirationCleaner() {
	s.lock.Lock()
	select {
	case <-s.expirationStopChan:
		// Already closed
	default:
		close(s.expirationStopChan)
	}
	s.lock.Unlock()
	<-s.expirationDoneChan
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
		s.ProtocolInstance().Logger().
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
