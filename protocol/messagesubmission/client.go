// Copyright 2026 Blink Labs Software
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
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/gouroboros/protocol/internal/stopping"
)

// Client is the side of a MessageSubmission session that has messages and
// answers the peer's requests for them. In V1 it is the connection's
// initiator; in V2 it is the responder, because there the side that wants
// messages starts the protocol.
type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	protoVersion    uint16

	// Client-side state for message ID tracking
	lock              sync.Mutex
	pendingMessageIDs [][]byte
	requestedIDs      map[string]struct{}
	pendingIDRequest  *messageIDRequest
	pendingMsgRequest *messageRequest
	requestGeneration uint64
	// runEnded is set by the V2 requester's MsgDone and cleared by whichever
	// request starts its next run; the wait for that request is not timed.
	runEnded           atomic.Bool
	testReplyPublished func()
	testReplyEnqueued  func()
	onceStart          sync.Once
	onceStop           sync.Once
	// stopErr caches the error returned by the first Stop() call so
	// subsequent Stop() calls return the same result instead of nil.
	stopErr error
}

// NewClient returns a new MessageSubmission client object
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	normalizedCfg := normalizeConfig(cfg)
	c := &Client{
		config:            &normalizedCfg,
		pendingMessageIDs: [][]byte{},
		requestedIDs:      map[string]struct{}{},
		protoVersion:      protoOptions.Version,
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}

	stateMap, initialState := stateMapFor(protoOptions.Version, c.config)
	if isMessageSubmissionV2(protoOptions.Version) {
		stateMap[protocolStateIdle] = restartOnDone(
			stateMap[protocolStateIdle],
			&c.runEnded,
		)
	}

	// Configure underlying Protocol
	protoConfig := protocol.ProtocolConfig{
		Name:               ProtocolName,
		ProtocolId:         ProtocolID,
		Muxer:              protoOptions.Muxer,
		Logger:             protoOptions.Logger,
		ErrorChan:          protoOptions.ErrorChan,
		Mode:               protoOptions.Mode,
		Role:               roleFor(protoOptions.Version, protocol.ProtocolRoleClient),
		MessageHandlerFunc: c.messageHandler,
		MessageFromCborFunc: func(msgType uint, data []byte) (protocol.Message, error) {
			return decodeMsgFromCborWithLimit(
				protoOptions.Version,
				msgType,
				data,
				c.collectionLimit(msgType),
			)
		},
		StateMap:     stateMap,
		InitialState: initialState,
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

// restartOnDone returns the V2 StIdle entry with MsgDone leading back to
// StIdle. In V2 the Client is the responder, and dmq-node runs its responders
// under the inbound governor, which restarts one that ended cleanly: its V2
// inbound sends MsgDone when the peer is demoted and requests again on the
// same connection when it is promoted. The client answers that next run as
// a restarted responder would, from StIdle with no IDs outstanding. Nothing
// bounds how long the peer stays demoted, so while runEnded is set StIdle is
// not timed.
func restartOnDone(
	entry protocol.StateMapEntry,
	runEnded *atomic.Bool,
) protocol.StateMapEntry {
	idleTimeout := entry.Timeout
	// The state loop arms the timer after handleDone has returned, so it
	// sees the flag that handler set.
	entry.TimeoutFunc = func() time.Duration {
		if runEnded.Load() {
			return 0
		}
		return idleTimeout
	}
	entry.Transitions = slices.Clone(entry.Transitions)
	for i := range entry.Transitions {
		if entry.Transitions[i].MsgType == MessageTypeV2Done {
			entry.Transitions[i].NewState = protocolStateIdle
		}
	}
	return entry
}

// collectionLimit bounds an inbound collection before it is decoded. A
// message request may only name IDs this client still holds unacknowledged.
func (c *Client) collectionLimit(msgType uint) int {
	if kind, _ := kindOf(c.protoVersion, msgType); kind != kindRequestMessages {
		return configuredMessageIDLimit(c.config)
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	return len(c.pendingMessageIDs)
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
// In V1, the client sends MsgDone. In V2, only the server can send MsgDone,
// so the client stops its protocol instance without sending anything.
//
// Stop sends the message that ends the client's run and waits up to 250ms
// for it to be written, then stops the protocol. It signals shutdown at
// once. When ctx is the Context of a CallbackContext this client passed to a
// callback, Stop returns without waiting for the protocol's goroutines,
// since the goroutine running the callback is one of them. Otherwise it
// waits until they have returned, or until ctx is done, in which case it
// returns ctx.Err() and shutdown still proceeds.
func (c *Client) Stop(ctx context.Context) error {
	c.onceStop.Do(func() {
		if isMessageSubmissionV2(c.protoVersion) {
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
		msg := NewMsgDone(c.protoVersion)
		c.stopErr = stopping.SendDone(ctx, c.Protocol, msg)
	})
	if err := c.Protocol.Stop(ctx); err != nil {
		return err
	}
	return c.stopErr
}

// Init sends the initialization message to start the protocol.
// This is only valid for V1; V2 starts directly in StIdle.
func (c *Client) Init() error {
	if isMessageSubmissionV2(c.protoVersion) {
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
	c.lock.Lock()
	request := c.pendingIDRequest
	if request == nil {
		c.lock.Unlock()
		return errors.New("no message ID request is outstanding")
	}
	previous := c.pendingMessageIDs
	next, err := reconcileMessageIDs(
		c.pendingMessageIDs,
		*request,
		messages,
		c.config.MaxUnacknowledgedMessageIDs,
	)
	if err != nil {
		c.lock.Unlock()
		return err
	}
	generation := c.requestGeneration
	previousRequested := c.requestedIDs
	c.pendingMessageIDs = next
	c.requestedIDs = forgetAcknowledgedIDs(
		c.requestedIDs,
		previous,
		request.ack,
	)
	c.pendingIDRequest = nil
	c.lock.Unlock()
	if c.testReplyPublished != nil {
		c.testReplyPublished()
	}
	if err := c.SendMessage(NewMsgReplyMessageIds(c.protoVersion, messages)); err != nil {
		c.lock.Lock()
		if c.requestGeneration == generation &&
			c.pendingIDRequest == nil && c.pendingMsgRequest == nil {
			c.pendingMessageIDs = previous
			c.requestedIDs = previousRequested
			c.pendingIDRequest = request
		}
		c.lock.Unlock()
		return err
	}
	if c.testReplyEnqueued != nil {
		c.testReplyEnqueued()
	}
	return nil
}

// ReplyNoMessageIds answers an outstanding V2 blocking request for message
// IDs with none, applying the request's acknowledgement. dmq-node's V2
// requester allows 20 seconds for a blocking answer and its own sending side
// gives up after 17, so a Client with nothing to announce should call this
// before the peer's limit rather than leave the request open.
func (c *Client) ReplyNoMessageIds() error {
	if !isMessageSubmissionV2(c.protoVersion) {
		return errors.New(
			"ReplyNoMessageIds is only supported in MessageSubmission V2",
		)
	}
	c.lock.Lock()
	request := c.pendingIDRequest
	if request == nil {
		c.lock.Unlock()
		return errors.New("no message ID request is outstanding")
	}
	next, err := acknowledgeMessageIDs(
		c.pendingMessageIDs,
		*request,
		c.config.MaxUnacknowledgedMessageIDs,
	)
	if err != nil {
		c.lock.Unlock()
		return err
	}
	previous := c.pendingMessageIDs
	generation := c.requestGeneration
	previousRequested := c.requestedIDs
	c.pendingMessageIDs = next
	c.requestedIDs = forgetAcknowledgedIDs(
		c.requestedIDs,
		previous,
		request.ack,
	)
	c.pendingIDRequest = nil
	c.lock.Unlock()
	if err := c.SendMessage(NewMsgReplyNoMessageIds()); err != nil {
		c.lock.Lock()
		if c.requestGeneration == generation &&
			c.pendingIDRequest == nil && c.pendingMsgRequest == nil {
			c.pendingMessageIDs = previous
			c.requestedIDs = previousRequested
			c.pendingIDRequest = request
		}
		c.lock.Unlock()
		return err
	}
	return nil
}

// ReplyMessages sends a reply with full messages
func (c *Client) ReplyMessages(messages []pcommon.DmqMessage) error {
	c.lock.Lock()
	request := c.pendingMsgRequest
	if request == nil {
		c.lock.Unlock()
		return errors.New("no message request is outstanding")
	}
	if err := validateMessageReply(request, messages); err != nil {
		c.lock.Unlock()
		return err
	}
	generation := c.requestGeneration
	c.pendingMsgRequest = nil
	c.lock.Unlock()
	if c.testReplyPublished != nil {
		c.testReplyPublished()
	}
	msg := NewMsgReplyMessages(c.protoVersion, messages)
	msg.legacyMessageEncoding = c.config.LegacyV1MessageEncoding &&
		!isMessageSubmissionV2(c.protoVersion)
	if err := c.SendMessage(msg); err != nil {
		c.lock.Lock()
		if c.requestGeneration == generation &&
			c.pendingIDRequest == nil && c.pendingMsgRequest == nil {
			c.pendingMsgRequest = request
		}
		c.lock.Unlock()
		return err
	}
	if c.testReplyEnqueued != nil {
		c.testReplyEnqueued()
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
	// V2 StIdle may be left by a request for IDs or for messages, an empty
	// one included, and either starts a run after MsgDone.
	if _, done := msg.(*MsgDone); !done {
		c.runEnded.Store(false)
	}
	var err error
	switch msg.(type) {
	case *MsgRequestMessageIds:
		err = c.handleRequestMessageIds(msg)
	case *MsgRequestMessages:
		err = c.handleRequestMessages(msg)
	case *MsgDone:
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

	c.lock.Lock()
	if c.pendingIDRequest != nil || c.pendingMsgRequest != nil {
		c.lock.Unlock()
		return invalidMessageSubmissionMessage("a request is already pending")
	}
	request := &messageIDRequest{
		blocking:  msgRequest.IsBlocking,
		ack:       int(msgRequest.AckCount),
		requested: int(msgRequest.RequestCount),
	}
	if err := validateMessageIDRequest(
		c.pendingMessageIDs,
		*request,
		c.config.MaxUnacknowledgedMessageIDs,
	); err != nil {
		c.lock.Unlock()
		return err
	}
	c.pendingIDRequest = request
	c.requestGeneration++
	c.lock.Unlock()

	// Invoke callback to get available message IDs
	if c.config.RequestMessageIdsFunc != nil {
		c.config.RequestMessageIdsFunc(
			c.callbackContext.withOwner(c.Protocol),
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

	c.lock.Lock()
	if c.pendingIDRequest != nil || c.pendingMsgRequest != nil {
		c.lock.Unlock()
		return invalidMessageSubmissionMessage("a request is already pending")
	}
	// V2 refuses a body asked for again while its ID is unacknowledged.
	// V1 is tx-submission, whose reference outbound refuses a repeat only
	// within one request, so a V1 peer may ask again across requests.
	alreadyRequested := c.requestedIDs
	if !isMessageSubmissionV2(c.protoVersion) {
		alreadyRequested = nil
	}
	request, err := requestedMessagesAreOutstanding(
		c.pendingMessageIDs,
		alreadyRequested,
		msgRequest.MessageIDs,
	)
	if err != nil {
		c.lock.Unlock()
		return err
	}
	c.requestedIDs = withRequestedIDs(c.requestedIDs, request)
	c.pendingMsgRequest = request
	c.requestGeneration++
	c.lock.Unlock()

	if c.config.RequestMessagesFunc != nil {
		c.config.RequestMessagesFunc(
			c.callbackContext.withOwner(c.Protocol),
			cloneMessageIDs(msgRequest.MessageIDs),
		)
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
	if isMessageSubmissionV2(c.protoVersion) {
		// The run is over; the next one starts with nothing outstanding.
		// MsgDone is legal only in StIdle, so no request is pending.
		c.lock.Lock()
		c.pendingMessageIDs = [][]byte{}
		c.requestedIDs = map[string]struct{}{}
		c.requestGeneration++
		c.lock.Unlock()
		c.runEnded.Store(true)
	}
	if c.config != nil && c.config.DoneFunc != nil {
		return c.config.DoneFunc(c.callbackContext.withOwner(c.Protocol))
	}
	return nil
}
