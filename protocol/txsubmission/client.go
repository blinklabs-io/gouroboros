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

package txsubmission

import (
	"errors"
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/protocol"
)

type clientLifecycleState uint8

const (
	clientStateNew clientLifecycleState = iota
	clientStateStarting
	clientStateRunning
	clientStateStopped
)

// Client implements the TxSubmission client
type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	protoOptions    protocol.ProtocolOptions
	lifecycleMutex  sync.Mutex
	lifecycleState  clientLifecycleState
	startingDone    chan struct{}
	initSent        bool // tracks whether Init message has been sent
	protoStarted    bool // tracks whether Protocol.Start() was called
}

// NewClient returns a new TxSubmission client object
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:         cfg,
		protoOptions:   protoOptions,
		lifecycleState: clientStateNew,
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	c.initProtocol()
	return c
}

func (c *Client) initProtocol() {
	// Update state map with timeout
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[stateIdle]; ok {
		entry.Timeout = c.config.IdleTimeout
		stateMap[stateIdle] = entry
	}
	// Configure underlying Protocol
	protoConfig := protocol.ProtocolConfig{
		Name:                ProtocolName,
		ProtocolId:          ProtocolId,
		Muxer:               c.protoOptions.Muxer,
		Logger:              c.protoOptions.Logger,
		ErrorChan:           c.protoOptions.ErrorChan,
		Mode:                c.protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            stateMap,
		InitialState:        stateInit,
	}
	c.Protocol = protocol.New(protoConfig)
	c.callbackContext.DoneChan = c.DoneChan()
	// Reset state so Init() can be called again after restart
	c.initSent = false
	c.protoStarted = false
}

// Start begins the TxSubmission client protocol. Safe to call multiple times.
func (c *Client) Start() {
	for {
		c.lifecycleMutex.Lock()

		switch c.lifecycleState {
		case clientStateRunning:
			c.lifecycleMutex.Unlock()
			return

		case clientStateStarting:
			// Another goroutine is already starting. Wait for it to complete.
			ch := c.startingDone
			c.lifecycleMutex.Unlock()
			if ch != nil {
				<-ch
			}
			// Re-check state after the in-flight start completes
			continue

		case clientStateStopped, clientStateNew:
			// We will be the goroutine that performs initialization/start.
			prevState := c.lifecycleState
			c.lifecycleState = clientStateStarting
			ch := make(chan struct{})
			c.startingDone = ch

			oldProto := c.Protocol
			oldProtoStarted := c.protoStarted
			var oldDone <-chan struct{}
			// Only wait for old protocol if it was actually started.
			// If Stop() was called during clientStateStarting before Protocol.Start(),
			// the protocol's DoneChan will never close.
			if prevState == clientStateStopped && oldProto != nil &&
				oldProtoStarted {
				oldDone = oldProto.DoneChan()
			}
			c.lifecycleMutex.Unlock()

			// If we were stopped, ensure the old instance is fully stopped before re-registering.
			if oldDone != nil {
				oldProto.Stop()
				<-oldDone
			}

			c.lifecycleMutex.Lock()
			// If we were stopped by someone else while waiting, don't continue.
			if c.lifecycleState != clientStateStarting {
				if c.startingDone == ch {
					close(ch)
					c.startingDone = nil
				}
				c.lifecycleMutex.Unlock()
				return
			}

			// Reinitialize protocol when transitioning from stopped->start (or if nil).
			if c.Protocol == nil || prevState == clientStateStopped {
				c.initProtocol()
			}

			c.Protocol.Logger().
				Debug("starting client protocol",
					"component", "network",
					"protocol", ProtocolName,
					"connection_id", c.callbackContext.ConnectionId.String(),
				)
			c.Protocol.Start()
			c.protoStarted = true
			c.lifecycleState = clientStateRunning
			if c.startingDone == ch {
				close(ch)
				c.startingDone = nil
			}
			c.lifecycleMutex.Unlock()
			return

		default:
			// Should not happen; treat as stopped.
			c.lifecycleState = clientStateStopped
			c.lifecycleMutex.Unlock()
			continue
		}
	}
}

// Stop stops the TxSubmission client protocol.
// Note: Unlike other protocols, TxSubmission can only send Done from the TxIdsBlocking
// state per protocol spec. This method stops the protocol without sending Done.
// To send Done gracefully, use the callback mechanism with ErrStopServerProcess.
func (c *Client) Stop() error {
	c.lifecycleMutex.Lock()
	defer c.lifecycleMutex.Unlock()

	switch c.lifecycleState {
	case clientStateNew, clientStateStopped:
		return nil
	case clientStateStarting:
		// Mark as stopped so Start() will abort when it re-checks state
		c.lifecycleState = clientStateStopped
		// Unblock Start() if it's waiting
		if c.startingDone != nil {
			close(c.startingDone)
			c.startingDone = nil
		}
		return nil
	case clientStateRunning:
		// Continue with normal stop logic below
	}

	c.Protocol.Logger().
		Debug("stopping client protocol",
			"component", "network",
			"protocol", ProtocolName,
			"connection_id", c.callbackContext.ConnectionId.String(),
		)

	// Stop/unregister the underlying protocol instance.
	c.Protocol.Stop()
	c.lifecycleState = clientStateStopped
	// Unblock any goroutine waiting for an in-progress start.
	if c.startingDone != nil {
		close(c.startingDone)
		c.startingDone = nil
	}
	return nil
}

// Init tells the server to begin asking us for transactions
func (c *Client) Init() {
	c.lifecycleMutex.Lock()
	defer c.lifecycleMutex.Unlock()

	if c.initSent {
		return
	}

	c.Protocol.Logger().
		Debug("calling Init()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	// Send our Init message
	msg := NewMsgInit()
	_ = c.SendMessage(msg)
	c.initSent = true
}

func (c *Client) messageHandler(msg protocol.Message) error {
	c.Protocol.Logger().
		Debug(fmt.Sprintf("%s: client message for %+v", ProtocolName, c.callbackContext.ConnectionId.RemoteAddr))
	var err error
	switch msg.Type() {
	case MessageTypeRequestTxIds:
		err = c.handleRequestTxIds(msg)
	case MessageTypeRequestTxs:
		err = c.handleRequestTxs(msg)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (c *Client) handleRequestTxIds(msg protocol.Message) error {
	c.Protocol.Logger().
		Debug("requesting tx ids",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	if c.config.RequestTxIdsFunc == nil {
		return errors.New(
			"received tx-submission RequestTxIds message but no callback function is defined",
		)
	}
	msgRequestTxIds := msg.(*MsgRequestTxIds)

	// Validate request counts
	if msgRequestTxIds.Ack > MaxAckCount {
		c.Protocol.Logger().
			Error("TxSubmission ack count exceeded", "ack", msgRequestTxIds.Ack, "limit", MaxAckCount)
		return protocol.ErrProtocolViolationRequestExceeded
	}
	if msgRequestTxIds.Req > MaxRequestCount {
		c.Protocol.Logger().
			Error("TxSubmission request count exceeded", "req", msgRequestTxIds.Req, "limit", MaxRequestCount)
		return protocol.ErrProtocolViolationRequestExceeded
	}

	// Call the user callback function
	txIds, err := c.config.RequestTxIdsFunc(
		c.callbackContext,
		msgRequestTxIds.Blocking,
		msgRequestTxIds.Ack,
		msgRequestTxIds.Req,
	)
	if err != nil {
		if !errors.Is(err, ErrStopServerProcess) {
			return err
		}
		if !msgRequestTxIds.Blocking {
			return errors.New(
				"cannot stop server process during a non-blocking operation",
			)
		}
		resp := NewMsgDone()
		if err := c.SendMessage(resp); err != nil {
			return err
		}
		return nil
	}
	resp := NewMsgReplyTxIds(txIds)
	if err := c.SendMessage(resp); err != nil {
		return err
	}
	return nil
}

func (c *Client) handleRequestTxs(msg protocol.Message) error {
	c.Protocol.Logger().
		Debug("requesting txs",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	if c.config.RequestTxsFunc == nil {
		return errors.New(
			"received tx-submission RequestTxs message but no callback function is defined",
		)
	}
	msgRequestTxs := msg.(*MsgRequestTxs)
	// Call the user callback function
	txs, err := c.config.RequestTxsFunc(c.callbackContext, msgRequestTxs.TxIds)
	if err != nil {
		return err
	}
	resp := NewMsgReplyTxs(txs)
	if err := c.SendMessage(resp); err != nil {
		return err
	}
	return nil
}
