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

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// Client implements the LocalMessageSubmission client
type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	onceStart       sync.Once
	onceStop        sync.Once
	stopErr         error
	// busyMutex holds one submission from its send until its answer, so
	// the answer that arrives is the answer to the holder's message.
	busyMutex        sync.Mutex
	submitResultChan chan error
	// stopMutex orders a submission's queueing against Stop's MsgDone: a
	// submission queued after MsgDone would never be sent, since StDone
	// sends nothing, so it waits forever for an answer.
	stopMutex sync.Mutex
	stopping  bool
}

// NewClient returns a new LocalMessageSubmission client object
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	normalizedCfg := normalizeConfig(cfg)
	c := &Client{
		config:           &normalizedCfg,
		submitResultChan: make(chan error),
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	// Update state map with timeout
	stateMapCopy := stateMap.Copy()
	if entry, ok := stateMapCopy[protocolStateBusy]; ok {
		entry.Timeout = c.config.Timeout
		stateMapCopy[protocolStateBusy] = entry
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
		// The receive loop, the only sender, has ended once DoneChan
		// closes, so closing here cannot race a send.
		go func() {
			<-c.DoneChan()
			close(c.submitResultChan)
		}()
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
		msg := NewMsgDone()
		c.stopMutex.Lock()
		c.stopping = true
		c.stopErr = c.SendMessage(msg)
		c.stopMutex.Unlock()
	})
	return c.stopErr
}

// SubmitMessage submits msg and waits for the node's answer, as the
// reference client's submit does. It returns nil when the node accepts the
// message and a MessageRejectedError when it rejects it. Concurrent calls
// are sent one at a time, and each returns the answer to its own message.
// It returns ErrProtocolShuttingDown, without sending, once Stop has been
// called. AcceptMessageFunc and RejectMessageFunc, when set, run on the
// calling goroutine before the call returns, after the next submission may
// start, so a callback may itself call SubmitMessage.
func (c *Client) SubmitMessage(msg *pcommon.DmqMessage) error {
	if msg == nil {
		return errors.New("message cannot be nil")
	}

	// Validate message before sending
	if err := c.config.TTLValidator.ValidateMessageTTL(msg); err != nil {
		return err
	}
	if c.config.Authenticator == nil {
		return errors.New("dmq: message authenticator not configured")
	}
	if err := c.config.Authenticator.VerifyMessage(msg); err != nil {
		return err
	}

	submitMsg := NewMsgSubmitMessage(*msg)
	c.busyMutex.Lock()
	c.stopMutex.Lock()
	if c.stopping {
		c.stopMutex.Unlock()
		c.busyMutex.Unlock()
		return protocol.ErrProtocolShuttingDown
	}
	err := c.SendMessage(submitMsg)
	c.stopMutex.Unlock()
	if err != nil {
		c.busyMutex.Unlock()
		return err
	}
	err, ok := <-c.submitResultChan
	c.busyMutex.Unlock()
	if !ok {
		return protocol.ErrProtocolShuttingDown
	}
	// The callbacks run here rather than on the receive loop: a callback
	// that submits again needs the receive loop free to read its answer.
	var rejected MessageRejectedError
	switch {
	case err == nil:
		if c.config.AcceptMessageFunc != nil {
			c.config.AcceptMessageFunc(c.callbackContext)
		}
	case errors.As(err, &rejected):
		if c.config.RejectMessageFunc != nil {
			c.config.RejectMessageFunc(c.callbackContext, rejected.Reason)
		}
	}
	return err
}

func (c *Client) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeAcceptMessage:
		err = c.handleAcceptMessage()
	case MessageTypeRejectMessage:
		err = c.handleRejectMessage(msg)
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
func (c *Client) handleAcceptMessage() error {
	c.Protocol.Logger().
		Debug("message accepted",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.submitResultChan <- nil
	return nil
}

func (c *Client) handleRejectMessage(msg protocol.Message) error {
	msgReject, ok := msg.(*MsgRejectMessage)
	if !ok {
		err := fmt.Errorf(
			"%s: expected MsgRejectMessage but got %T",
			ProtocolName,
			msg,
		)
		c.Protocol.Logger().
			Warn("unexpected message type in handleRejectMessage",
				"component", "network",
				"protocol", ProtocolName,
				"role", "client",
				"connection_id", c.callbackContext.ConnectionId.String(),
				"error", err,
			)
		return err
	}
	c.Protocol.Logger().
		Warn("message rejected",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
			"reason", msgReject.Reason,
		)
	rr, err := pcommon.FromRejectReasonData(msgReject.Reason)
	if err != nil {
		return err
	}
	c.submitResultChan <- MessageRejectedError{Reason: rr}
	return nil
}

// MessageRejectedError is the error SubmitMessage returns when the node
// rejects the message. Reason is the node's reason.
type MessageRejectedError struct {
	Reason pcommon.RejectReason
}

func (e MessageRejectedError) Error() string {
	return fmt.Sprintf("%s: message rejected: %+v", ProtocolName, e.Reason)
}
