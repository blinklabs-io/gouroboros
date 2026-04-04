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

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// Client implements the MessageSubmission client
type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	protoVersion    uint16

	// Client-side state for message ID tracking
	lock              sync.Mutex
	pendingMessageIDs [][]byte
	onceStart         sync.Once
	onceStop          sync.Once
	// stopErr caches the error returned by the first Stop() call so
	// subsequent Stop() calls return the same result instead of nil.
	stopErr error
}

// NewClient returns a new MessageSubmission client object
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:            cfg,
		pendingMessageIDs: [][]byte{},
		protoVersion:      protoOptions.Version,
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}

	// Select state map and initial state based on negotiated version
	isV2 := protoOptions.Version >= MessageSubmissionV2MinVersion
	var baseStateMap protocol.StateMap
	var initialState protocol.State
	if isV2 {
		baseStateMap = stateMapV2
		initialState = protocolStateIdle
	} else {
		baseStateMap = stateMapV1
		initialState = protocolStateInit
	}

	// Update state map with configurable timeouts
	stateMapCopy := baseStateMap.Copy()
	if entry, ok := stateMapCopy[protocolStateInit]; ok {
		entry.Timeout = c.config.InitTimeout
		stateMapCopy[protocolStateInit] = entry
	}
	if entry, ok := stateMapCopy[protocolStateIdle]; ok {
		entry.Timeout = c.config.IdleTimeout
		stateMapCopy[protocolStateIdle] = entry
	}
	if entry, ok := stateMapCopy[protocolStateMessageIdsBlock]; ok {
		entry.Timeout = c.config.MessageIdsBlockingTimeout
		stateMapCopy[protocolStateMessageIdsBlock] = entry
	}
	if entry, ok := stateMapCopy[protocolStateMessageIdsNonBlk]; ok {
		entry.Timeout = c.config.MessageIdsNonblockingTimeout
		stateMapCopy[protocolStateMessageIdsNonBlk] = entry
	}
	if entry, ok := stateMapCopy[protocolStateMessages]; ok {
		entry.Timeout = c.config.MessagesTimeout
		stateMapCopy[protocolStateMessages] = entry
	}
	// Done state has no timeout

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
		InitialState:        initialState,
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

// Stop transitions the protocol to the Done state.
// In V1, the client sends MsgDone. In V2, only the server can send MsgDone;
// the client-side Stop is a no-op.
func (c *Client) Stop() error {
	c.onceStop.Do(func() {
		if c.protoVersion >= MessageSubmissionV2MinVersion {
			// V2: client cannot send MsgDone; server controls termination
			c.Protocol.Logger().
				Debug("V2 client Stop() is a no-op; server sends MsgDone",
					"component", "network",
					"protocol", ProtocolName,
					"connection_id", c.callbackContext.ConnectionId.String(),
				)
			return
		}
		c.Protocol.Logger().
			Debug("stopping client protocol",
				"component", "network",
				"protocol", ProtocolName,
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
		msg := NewMsgDone()
		c.stopErr = c.SendMessage(msg)
	})
	return c.stopErr
}

// Init sends the initialization message to start the protocol.
// This is only valid for V1; V2 starts directly in StIdle.
func (c *Client) Init() error {
	if c.protoVersion >= MessageSubmissionV2MinVersion {
		return errors.New(
			"Init is not supported in MessageSubmission V2; protocol starts in StIdle",
		)
	}
	msg := NewMsgInit()
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	return nil
}

// ReplyMessageIds sends a reply with message IDs and sizes
func (c *Client) ReplyMessageIds(messages []pcommon.MessageIDAndSize) error {
	// Prepare the outgoing message first and attempt send before mutating internal state.
	msg := NewMsgReplyMessageIds(messages)
	if err := c.SendMessage(msg); err != nil {
		return err
	}

	// On successful send, update pendingMessageIDs under lock. Deep-copy inner byte slices
	// to avoid external mutation affecting internal state.
	ids := make([][]byte, len(messages))
	for i, m := range messages {
		if m.MessageID == nil {
			ids[i] = nil
			continue
		}
		ids[i] = make([]byte, len(m.MessageID))
		copy(ids[i], m.MessageID)
	}

	c.lock.Lock()
	c.pendingMessageIDs = ids
	c.lock.Unlock()

	return nil
}

// ReplyMessages sends a reply with full messages
func (c *Client) ReplyMessages(messages []pcommon.DmqMessage) error {
	msg := NewMsgReplyMessages(messages)
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	return nil
}

// GetPendingMessageIDs returns the list of message IDs that are currently pending
func (c *Client) GetPendingMessageIDs() [][]byte {
	c.lock.Lock()
	defer c.lock.Unlock()
	// Return a deep copy to prevent external modification of inner byte slices
	ids := make([][]byte, len(c.pendingMessageIDs))
	for i, b := range c.pendingMessageIDs {
		if b == nil {
			ids[i] = nil
			continue
		}
		ids[i] = make([]byte, len(b))
		copy(ids[i], b)
	}
	return ids
}

func (c *Client) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeRequestMessageIds:
		err = c.handleRequestMessageIds(msg)
	case MessageTypeRequestMessages:
		err = c.handleRequestMessages(msg)
	case MessageTypeDone:
		err = c.handleDone()
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (c *Client) handleRequestMessageIds(msg protocol.Message) error {
	msgRequest, ok := msg.(*MsgRequestMessageIds)
	if !ok {
		return fmt.Errorf("%s: unexpected message type %T", ProtocolName, msg)
	}

	c.Protocol.Logger().
		Debug("request message IDs",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
			"is_blocking", msgRequest.IsBlocking,
			"ack_count", msgRequest.AckCount,
			"request_count", msgRequest.RequestCount,
		)

	// Acknowledge previously sent IDs
	c.lock.Lock()
	if msgRequest.AckCount > 0 {
		if int(msgRequest.AckCount) <= len(c.pendingMessageIDs) {
			c.pendingMessageIDs = c.pendingMessageIDs[int(msgRequest.AckCount):]
		} else {
			c.Protocol.Logger().Warn("AckCount greater than pendingMessageIDs; clearing all",
				"ack_count", msgRequest.AckCount,
				"pending", len(c.pendingMessageIDs))
			c.pendingMessageIDs = nil
		}
	}

	// Validate blocking/non-blocking protocol invariants
	if msgRequest.IsBlocking && len(c.pendingMessageIDs) > 0 {
		c.lock.Unlock()
		return errors.New(
			"cannot accept blocking request when pending IDs exist",
		)
	}
	if !msgRequest.IsBlocking && len(c.pendingMessageIDs) == 0 {
		c.lock.Unlock()
		return errors.New(
			"cannot accept non-blocking request when no pending IDs",
		)
	}
	c.lock.Unlock()

	// Invoke callback to get available message IDs
	if c.config.RequestMessageIdsFunc != nil {
		c.config.RequestMessageIdsFunc(
			c.callbackContext,
			msgRequest.IsBlocking,
			msgRequest.AckCount,
			msgRequest.RequestCount,
		)
	}

	return nil
}

func (c *Client) handleRequestMessages(msg protocol.Message) error {
	msgRequest, ok := msg.(*MsgRequestMessages)
	if !ok {
		return fmt.Errorf("%s: unexpected message type %T", ProtocolName, msg)
	}

	c.Protocol.Logger().
		Debug("request messages",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
			"message_count", len(msgRequest.MessageIDs),
		)

	// Invoke callback to get the requested messages
	if c.config.RequestMessagesFunc != nil {
		c.config.RequestMessagesFunc(c.callbackContext, msgRequest.MessageIDs)
	}

	return nil
}

func (c *Client) handleDone() error {
	c.Protocol.Logger().
		Debug("received done message",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	if c.config != nil && c.config.DoneFunc != nil {
		return c.config.DoneFunc(c.callbackContext)
	}
	return nil
}
