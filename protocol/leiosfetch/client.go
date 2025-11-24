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

package leiosfetch

import (
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

type Client struct {
	*protocol.Protocol
	config               *Config
	callbackContext      CallbackContext
	onceStart            sync.Once
	onceStop             sync.Once
	blockResultChan      chan protocol.Message
	blockTxsResultChan   chan protocol.Message
	votesResultChan      chan protocol.Message
	blockRangeResultChan chan protocol.Message
}

func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:               cfg,
		blockResultChan:      make(chan protocol.Message),
		blockTxsResultChan:   make(chan protocol.Message),
		votesResultChan:      make(chan protocol.Message),
		blockRangeResultChan: make(chan protocol.Message),
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	// Update state map with timeout
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[StateBlock]; ok {
		entry.Timeout = c.config.Timeout
		stateMap[StateBlock] = entry
	}
	if entry, ok := stateMap[StateBlockTxs]; ok {
		entry.Timeout = c.config.Timeout
		stateMap[StateBlockTxs] = entry
	}
	if entry, ok := stateMap[StateVotes]; ok {
		entry.Timeout = c.config.Timeout
		stateMap[StateVotes] = entry
	}
	if entry, ok := stateMap[StateBlockRange]; ok {
		entry.Timeout = c.config.Timeout
		stateMap[StateBlockRange] = entry
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
		InitialState:        StateIdle,
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

func (c *Client) Start() {
	c.onceStart.Do(func() {
		c.Protocol.Logger().
			Debug("starting client protocol",
				"component", "network",
				"protocol", ProtocolName,
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
		c.Protocol.Start()
		// Start goroutine to cleanup resources on protocol shutdown
		go func() {
			<-c.DoneChan()
			close(c.blockResultChan)
			close(c.blockTxsResultChan)
			close(c.votesResultChan)
			close(c.blockRangeResultChan)
		}()
	})
}

func (c *Client) Stop() error {
	var err error
	c.onceStop.Do(func() {
		c.Protocol.Logger().
			Debug("stopping client protocol",
				"component", "network",
				"protocol", ProtocolName,
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
		msg := NewMsgDone()
		err = c.SendMessage(msg)
	})
	return err
}

// BlockRequest fetches the requested EB identified by the slot and Leios hash
func (c *Client) BlockRequest(
	slot uint64,
	hash []byte,
) (protocol.Message, error) {
	msg := NewMsgBlockRequest(slot, hash)
	if err := c.SendMessage(msg); err != nil {
		return nil, err
	}
	resp, ok := <-c.blockResultChan
	if !ok {
		return nil, protocol.ErrProtocolShuttingDown
	}
	return resp, nil
}

// BlockTxsRequest fetches the requested TXs identified by the slot, Leios hash, and TX bitmap
func (c *Client) BlockTxsRequest(
	slot uint64,
	hash []byte,
	txBitmap [8]byte,
) (protocol.Message, error) {
	msg := NewMsgBlockTxsRequest(slot, hash, txBitmap)
	if err := c.SendMessage(msg); err != nil {
		return nil, err
	}
	resp, ok := <-c.blockTxsResultChan
	if !ok {
		return nil, protocol.ErrProtocolShuttingDown
	}
	return resp, nil
}

// VotesRequest fetches the requested votes
func (c *Client) VotesRequest(
	voteIds []MsgVotesRequestVoteId,
) (protocol.Message, error) {
	msg := NewMsgVotesRequest(voteIds)
	if err := c.SendMessage(msg); err != nil {
		return nil, err
	}
	resp, ok := <-c.votesResultChan
	if !ok {
		return nil, protocol.ErrProtocolShuttingDown
	}
	return resp, nil
}

// BlockRangeRequest fetches a range of EBs and their TXs that are certified by RBs within the provided range.
// This function will block until all EBs and TXs in the requested range have been received
func (c *Client) BlockRangeRequest(
	start pcommon.Point,
	end pcommon.Point,
) ([]protocol.Message, error) {
	msg := NewMsgBlockRangeRequest(start, end)
	if err := c.SendMessage(msg); err != nil {
		return nil, err
	}
	ret := make([]protocol.Message, 0, 20)
	for {
		resp, ok := <-c.blockRangeResultChan
		if !ok {
			return nil, protocol.ErrProtocolShuttingDown
		}
		ret = append(ret, resp)
		if _, ok := resp.(*MsgLastBlockAndTxsInRange); ok {
			break
		}
	}
	return ret, nil
}

func (c *Client) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeBlock:
		err = c.handleBlock(msg)
	case MessageTypeBlockTxs:
		err = c.handleBlockTxs(msg)
	case MessageTypeVotes:
		err = c.handleVotes(msg)
	case MessageTypeNextBlockAndTxsInRange:
		err = c.handleNextBlockAndTxsInRange(msg)
	case MessageTypeLastBlockAndTxsInRange:
		err = c.handleLastBlockAndTxsInRange(msg)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (c *Client) handleBlock(msg protocol.Message) error {
	c.blockResultChan <- msg
	return nil
}

func (c *Client) handleBlockTxs(msg protocol.Message) error {
	c.blockTxsResultChan <- msg
	return nil
}

func (c *Client) handleVotes(msg protocol.Message) error {
	c.votesResultChan <- msg
	return nil
}

func (c *Client) handleNextBlockAndTxsInRange(msg protocol.Message) error {
	c.blockRangeResultChan <- msg
	return nil
}

func (c *Client) handleLastBlockAndTxsInRange(msg protocol.Message) error {
	c.blockRangeResultChan <- msg
	return nil
}
