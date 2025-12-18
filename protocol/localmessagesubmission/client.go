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
}

// NewClient returns a new LocalMessageSubmission client object
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
		c.stopErr = c.SendMessage(msg)
	})
	return c.stopErr
}

// SubmitMessage sends a message submission request
func (c *Client) SubmitMessage(msg *pcommon.DmqMessage) error {
	if msg == nil {
		return errors.New("message cannot be nil")
	}

	// Validate message before sending
	if c.config.TTLValidator != nil {
		if err := c.config.TTLValidator.ValidateMessageTTL(msg); err != nil {
			return err
		}
	} else if !msg.IsValid() {
		return errors.New("message has expired")
	}
	if c.config.Authenticator != nil {
		if err := c.config.Authenticator.VerifyMessage(msg); err != nil {
			return err
		}
	}

	submitMsg := NewMsgSubmitMessage(*msg)
	return c.SendMessage(submitMsg)
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
	if c.config.AcceptMessageFunc != nil {
		c.config.AcceptMessageFunc(c.callbackContext)
	}
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
	if c.config.RejectMessageFunc != nil {
		// Convert concrete RejectReasonData back to a RejectReason interface expected by the callback.
		rr := pcommon.FromRejectReasonData(msgReject.Reason)
		c.config.RejectMessageFunc(c.callbackContext, rr)
	}
	return nil
}
