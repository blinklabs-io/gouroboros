// Copyright 2023 Blink Labs, LLC.
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
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/protocol"
)

type Client struct {
	*protocol.Protocol
	config   *Config
	onceInit sync.Once
}

func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config: cfg,
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
		// Send our Init message
		msg := NewMsgInit()
		_ = c.SendMessage(msg)
	})
}

func (c *Client) messageHandler(msg protocol.Message, isResponse bool) error {
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
	if c.config.RequestTxIdsFunc == nil {
		return fmt.Errorf(
			"received tx-submission RequestTxIds message but no callback function is defined",
		)
	}
	msgRequestTxIds := msg.(*MsgRequestTxIds)
	// Call the user callback function
	txIds, err := c.config.RequestTxIdsFunc(
		msgRequestTxIds.Blocking,
		msgRequestTxIds.Ack,
		msgRequestTxIds.Req,
	)
	if err != nil {
		return err
	}
	resp := NewMsgReplyTxIds(txIds)
	if err := c.SendMessage(resp); err != nil {
		return err
	}
	return nil
}

func (c *Client) handleRequestTxs(msg protocol.Message) error {
	if c.config.RequestTxsFunc == nil {
		return fmt.Errorf(
			"received tx-submission RequestTxs message but no callback function is defined",
		)
	}
	msgRequestTxs := msg.(*MsgRequestTxs)
	// Call the user callback function
	txs, err := c.config.RequestTxsFunc(msgRequestTxs.TxIds)
	if err != nil {
		return err
	}
	resp := NewMsgReplyTxs(txs)
	if err := c.SendMessage(resp); err != nil {
		return err
	}
	return nil
}
