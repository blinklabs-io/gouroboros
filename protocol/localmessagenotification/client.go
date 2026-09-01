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
	"fmt"
	"sync"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Client implements the LocalMessageNotification client
type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	onceStart       sync.Once
	onceStop        sync.Once
	stopErr         error
}

// NewClient returns a new LocalMessageNotification client object
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config: cfg,
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
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
		Name:                ProtocolName,
		ProtocolId:          ProtocolID,
		Muxer:               protoOptions.Muxer,
		Logger:              protoOptions.Logger,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            stateMapCopy,
		InitialState:        protocolStateIdle,
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
	msg := NewMsgRequestMessages(false)
	return c.SendMessage(msg)
}

// RequestMessagesBlocking sends a blocking request for messages
func (c *Client) RequestMessagesBlocking() error {
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
	if c.config.ReplyMessagesFunc != nil {
		c.config.ReplyMessagesFunc(
			c.callbackContext,
			msgReply.Messages,
			msgReply.HasMore,
		)
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
	if c.config.ReplyMessagesFunc != nil {
		// For blocking replies, hasMore is always false (waiting until at least one message available)
		c.config.ReplyMessagesFunc(c.callbackContext, msgReply.Messages, false)
	}
	return nil
}
