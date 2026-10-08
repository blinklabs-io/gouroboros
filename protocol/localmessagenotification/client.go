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

// Client implements the LocalMessageNotification client
type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	onceStart       sync.Once
	onceStop        sync.Once
	stopErr         error
	replayState     *messageReplayState
	now             func() time.Time
}

type messageReplayState struct {
	mu          sync.Mutex
	acceptedIDs map[string]uint32
}

var (
	replayStateInitMu              sync.Mutex
	errReplayCacheCapacityExceeded = errors.New("dmq: replay cache capacity exceeded")
)

func newMessageReplayState() *messageReplayState {
	return &messageReplayState{acceptedIDs: make(map[string]uint32)}
}

func replayStateForConfig(cfg *Config) *messageReplayState {
	replayStateInitMu.Lock()
	defer replayStateInitMu.Unlock()
	if cfg.replayState == nil {
		cfg.replayState = newMessageReplayState()
	}
	return cfg.replayState
}

// NewClient returns a new LocalMessageNotification client object
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:      cfg,
		replayState: replayStateForConfig(cfg),
		now:         time.Now,
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	maxReplyMessages := cfg.MaxReplayEntries
	if maxReplyMessages <= 0 {
		maxReplyMessages = defaultMaxReplayEntries
	}

	// Update state map with timeouts for blocking requests
	stateMapCopy := stateMap.Copy()
	if entry, ok := stateMapCopy[protocolStateBusyBlock]; ok {
		// Set timeout for blocking request state to the configured timeout
		entry.Timeout = cfg.BlockingRequestTimeout
		stateMapCopy[protocolStateBusyBlock] = entry
	}

	// Configure underlying Protocol
	protoConfig := protocol.ProtocolConfig{
		Name:               ProtocolName,
		ProtocolId:         ProtocolID,
		Muxer:              protoOptions.Muxer,
		Logger:             protoOptions.Logger,
		ErrorChan:          protoOptions.ErrorChan,
		Mode:               protoOptions.Mode,
		Role:               protocol.ProtocolRoleClient,
		MessageHandlerFunc: c.messageHandler,
		MessageFromCborFunc: func(msgType uint, data []byte) (protocol.Message, error) {
			return newMsgFromCborWithLimit(msgType, data, maxReplyMessages)
		},
		StateMap:     stateMapCopy,
		InitialState: protocolStateIdle,
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

// Start begins protocol operation
func (c *Client) Start() {
	c.onceStart.Do(func() {
		c.Protocol.Logger().
			Debug("starting client protocol",
				"component", "network",
				"protocol", ProtocolName,
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
		c.Protocol.Start()
	})
}

// Stop transitions the protocol to the Done state
func (c *Client) Stop() error {
	c.onceStop.Do(func() {
		c.Protocol.Logger().
			Debug("stopping client protocol",
				"component", "network",
				"protocol", ProtocolName,
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
		msg := NewMsgClientDone()
		c.stopErr = c.SendMessage(msg)
	})
	return c.stopErr
}

// RequestMessagesNonBlocking sends a non-blocking request for messages
func (c *Client) RequestMessagesNonBlocking() error {
	if err := c.ensureReplayCapacity(); err != nil {
		return err
	}
	msg := NewMsgRequestMessages(false)
	return c.SendMessage(msg)
}

// RequestMessagesBlocking sends a blocking request for messages
func (c *Client) RequestMessagesBlocking() error {
	if err := c.ensureReplayCapacity(); err != nil {
		return err
	}
	msg := NewMsgRequestMessages(true)
	return c.SendMessage(msg)
}

// RequestMessagesBlockingValidateTimeout sends a blocking request for messages and validates
// that the provided timeout matches the pre-configured BlockingRequestTimeout.
// This is a validation helper, not a dynamic timeout API. The actual timeout is controlled
// by the protocol state machine and configured via WithBlockingRequestTimeout.
// Dynamic timeout changes after client creation are not supported.
// Pass the expected timeout to verify configuration matches expectations, or use
// RequestMessagesBlocking() if validation is not needed.
func (c *Client) RequestMessagesBlockingValidateTimeout(
	expectedTimeout time.Duration,
) error {
	// If expectedTimeout is 0, validation is skipped and only a blocking request is sent.
	// This allows callers to bypass validation when they do not wish to assert a specific timeout.
	// Validate that the caller's expected timeout matches the configured BlockingRequestTimeout
	// to catch configuration mismatches. This prevents silent timeout misalignments that could
	// lead to hard-to-debug protocol hangs or unexpected behavior.
	if expectedTimeout != 0 && c.config != nil &&
		c.config.BlockingRequestTimeout != 0 &&
		expectedTimeout != c.config.BlockingRequestTimeout {
		return fmt.Errorf(
			"timeout mismatch: configured=%s, expected=%s; dynamic timeout changes are not supported",
			c.config.BlockingRequestTimeout.String(),
			expectedTimeout.String(),
		)
	}
	if err := c.ensureReplayCapacity(); err != nil {
		return err
	}

	msg := NewMsgRequestMessages(true)
	return c.SendMessage(msg)
}

func (c *Client) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeReplyMessagesNonBlocking:
		err = c.handleReplyMessagesNonBlocking(msg)
	case MessageTypeReplyMessagesBlocking:
		err = c.handleReplyMessagesBlocking(msg)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (c *Client) handleReplyMessagesNonBlocking(msg protocol.Message) error {
	msgReply, ok := msg.(*MsgReplyMessagesNonBlocking)
	if !ok {
		return fmt.Errorf("%s: unexpected message type %T", ProtocolName, msg)
	}
	c.Protocol.Logger().
		Debug("received non-blocking reply",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
			"message_count", len(msgReply.Messages),
			"has_more", msgReply.HasMore,
		)
	messages, err := c.validateAndReserve(msgReply.Messages)
	if err != nil {
		return err
	}
	if c.config.ReplyMessagesFunc != nil &&
		(len(msgReply.Messages) == 0 || len(messages) > 0) {
		c.config.ReplyMessagesFunc(c.callbackContext, messages, msgReply.HasMore)
	}
	return nil
}

func (c *Client) handleReplyMessagesBlocking(msg protocol.Message) error {
	msgReply, ok := msg.(*MsgReplyMessagesBlocking)
	if !ok {
		return fmt.Errorf("%s: unexpected message type %T", ProtocolName, msg)
	}
	c.Protocol.Logger().
		Debug("received blocking reply",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
			"message_count", len(msgReply.Messages),
		)
	messages, err := c.validateAndReserve(msgReply.Messages)
	if err != nil {
		return err
	}
	if c.config.ReplyMessagesFunc != nil &&
		(len(msgReply.Messages) == 0 || len(messages) > 0) {
		// For blocking replies, hasMore is always false (waiting until at least one message available)
		c.config.ReplyMessagesFunc(c.callbackContext, messages, false)
	}
	return nil
}

// validateAndReserve returns the messages in a reply that were not already
// delivered. A server restarts from the beginning of its queue on every new
// connection, so a replayed ID is expected and is dropped rather than treated
// as a protocol violation. Replays are dropped before authentication because a
// message accepted earlier can carry an operational certificate the chain has
// since superseded.
func (c *Client) validateAndReserve(
	messages []pcommon.DmqMessage,
) ([]pcommon.DmqMessage, error) {
	if c.config.TTLValidator == nil {
		return nil, errors.New("dmq: TTL validator not configured")
	}
	if c.config.Authenticator == nil {
		return nil, errors.New("dmq: message authenticator not configured")
	}
	maxReplayEntries := c.config.MaxReplayEntries
	if maxReplayEntries == 0 {
		maxReplayEntries = defaultMaxReplayEntries
	}
	if maxReplayEntries < 0 {
		return nil, errors.New("dmq: MaxReplayEntries must be greater than zero")
	}
	now := c.now()
	c.replayState.mu.Lock()
	defer c.replayState.mu.Unlock()
	c.replayState.pruneExpiredLocked(now)
	messages, capacityExceeded := c.replayState.admitLocked(
		messages,
		maxReplayEntries,
	)
	if capacityExceeded {
		return nil, errReplayCacheCapacityExceeded
	}
	if len(messages) == 0 {
		return messages, nil
	}
	for i := range messages {
		if err := c.config.TTLValidator.ValidateMessageTTLAt(&messages[i], now); err != nil {
			return nil, fmt.Errorf("message %d TTL validation failed: %w", i, err)
		}
	}
	commitAuthentication, err := c.config.Authenticator.PrepareMessages(messages)
	if err != nil {
		return nil, err
	}
	now = c.now()
	for i := range messages {
		if err := c.config.TTLValidator.ValidateMessageTTLAt(&messages[i], now); err != nil {
			return nil, fmt.Errorf("message %d TTL validation failed: %w", i, err)
		}
	}
	if commitAuthentication != nil {
		if err := commitAuthentication(); err != nil {
			return nil, err
		}
	}
	for i := range messages {
		c.replayState.acceptedIDs[string(messages[i].ID())] = messages[i].Payload.ExpiresAt
	}
	return messages, nil
}

func (r *messageReplayState) pruneExpiredLocked(now time.Time) {
	nowUnix := now.Unix()
	for id, expiresAt := range r.acceptedIDs {
		if nowUnix > int64(expiresAt) {
			delete(r.acceptedIDs, id)
		}
	}
}

// admitLocked returns the fresh messages when the complete reply fits without
// discarding replay protection for any unexpired accepted ID.
func (r *messageReplayState) admitLocked(
	messages []pcommon.DmqMessage,
	maxEntries int,
) ([]pcommon.DmqMessage, bool) {
	seen := make(map[string]struct{}, len(messages))
	ret := make([]pcommon.DmqMessage, 0, min(len(messages), maxEntries))
	for i := range messages {
		id := string(messages[i].ID())
		if _, accepted := r.acceptedIDs[id]; accepted {
			continue
		}
		if _, repeated := seen[id]; repeated {
			continue
		}
		seen[id] = struct{}{}
		if len(ret) == maxEntries {
			return nil, true
		}
		ret = append(ret, messages[i])
	}
	if len(r.acceptedIDs)+len(ret) > maxEntries {
		return nil, true
	}
	return ret, false
}

func (c *Client) ensureReplayCapacity() error {
	maxReplayEntries := c.config.MaxReplayEntries
	if maxReplayEntries == 0 {
		maxReplayEntries = defaultMaxReplayEntries
	}
	if maxReplayEntries < 0 {
		return errors.New("dmq: MaxReplayEntries must be greater than zero")
	}
	c.replayState.mu.Lock()
	defer c.replayState.mu.Unlock()
	c.replayState.pruneExpiredLocked(c.now())
	if len(c.replayState.acceptedIDs) >= maxReplayEntries {
		return errReplayCacheCapacityExceeded
	}
	return nil
}
