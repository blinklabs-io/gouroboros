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

// Client implements the TxSubmission client
type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	onceInit        sync.Once
}

// NewClient returns a new TxSubmission client object
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
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[stateIdle]; ok {
		entry.Timeout = c.config.IdleTimeout
		stateMap[stateIdle] = entry
	}
	// Configure underlying Protocol
	protoConfig := protocol.ProtocolConfig{
		Name:                ProtocolName,
		ProtocolId:          ProtocolId,
		Muxer:               protoOptions.Muxer,
		Logger:              protoOptions.Logger,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            stateMap,
		InitialState:        stateInit,
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

// Init tells the server to begin asking us for transactions
func (c *Client) Init() {
	c.onceInit.Do(func() {
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
	})
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
