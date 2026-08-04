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

package leiosfetch

import (
	"context"
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
	// NOTE: StateBlock and StateBlockTxs intentionally do NOT get a
	// protocol-level timeout. A missing response to a BlockRequest /
	// BlockTxsRequest must fail only that individual request (bounded by the
	// caller-supplied context), never tear down the shared multiplexed
	// connection. The protocol-level timeout fires p.SendError(), which is
	// fatal to every mini-protocol on the same bearer (chainsync, blockfetch,
	// etc.), so it must not be wired for these two states.
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

// BlockRequest fetches the requested EB identified by the specified point.
//
// The wait for a response is bounded by the provided context. If the context
// is cancelled or its deadline is exceeded before a response arrives, the
// context error is returned. This failure is local to this request: it does
// NOT emit a protocol error and does NOT tear down the shared multiplexed
// connection. A response that arrives after the context is done is dropped by
// the receive path (see handleBlock/handleNoBlock).
func (c *Client) BlockRequest(
	ctx context.Context,
	point pcommon.Point,
) (protocol.Message, error) {
	msg := NewMsgBlockRequest(point)
	if err := c.SendMessage(msg); err != nil {
		return nil, err
	}
	select {
	case resp, ok := <-c.blockResultChan:
		if !ok {
			return nil, protocol.ErrProtocolShuttingDown
		}
		// The server reported the endorser block as not available
		if _, ok := resp.(*MsgNoBlock); ok {
			return nil, ErrBlockNotFound
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// BlockTxsRequest fetches the requested TXs identified by the specified point and TX bitmaps.
//
// As with BlockRequest, the wait is bounded by the provided context and a
// context cancellation/deadline returns the context error without tearing down
// the shared connection.
func (c *Client) BlockTxsRequest(
	ctx context.Context,
	point pcommon.Point,
	bitmaps map[uint16]uint64,
) (protocol.Message, error) {
	msg := NewMsgBlockTxsRequest(point, bitmaps)
	if err := c.SendMessage(msg); err != nil {
		return nil, err
	}
	select {
	case resp, ok := <-c.blockTxsResultChan:
		if !ok {
			return nil, protocol.ErrProtocolShuttingDown
		}
		// The server reported the endorser block transactions as not available
		if _, ok := resp.(*MsgNoBlockTxs); ok {
			return nil, ErrBlockTxsNotFound
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
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
		c.handleBlock(msg)
	case MessageTypeNoBlock:
		c.handleNoBlock(msg)
	case MessageTypeBlockTxs:
		c.handleBlockTxs(msg)
	case MessageTypeNoBlockTxs:
		c.handleNoBlockTxs(msg)
	case MessageTypeVotes:
		c.handleVotes(msg)
	case MessageTypeNextBlockAndTxsInRange:
		c.handleNextBlockAndTxsInRange(msg)
	case MessageTypeLastBlockAndTxsInRange:
		c.handleLastBlockAndTxsInRange(msg)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

// deliverResult delivers a response to a waiting request without blocking the
// protocol receive loop. If no caller is currently waiting on the channel (for
// example, because the request was abandoned after its context expired), the
// response is dropped. The mini-protocol state transition back to StateIdle is
// driven independently by protocol.handleMessage before this handler runs, so
// dropping the message here never leaves the mini-protocol wedged.
func (c *Client) deliverResult(
	ch chan protocol.Message,
	msg protocol.Message,
) {
	select {
	case ch <- msg:
	default:
		c.Protocol.Logger().
			Debug("dropping unawaited leios-fetch response",
				"component", "network",
				"protocol", ProtocolName,
				"role", "client",
				"connection_id", c.callbackContext.ConnectionId.String(),
				"message_type", msg.Type(),
			)
	}
}

func (c *Client) handleBlock(msg protocol.Message) {
	c.deliverResult(c.blockResultChan, msg)
}

func (c *Client) handleNoBlock(msg protocol.Message) {
	c.Protocol.Logger().
		Debug("endorser block not available",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.deliverResult(c.blockResultChan, msg)
}

func (c *Client) handleBlockTxs(msg protocol.Message) {
	c.deliverResult(c.blockTxsResultChan, msg)
}

func (c *Client) handleNoBlockTxs(msg protocol.Message) {
	c.Protocol.Logger().
		Debug("endorser block transactions not available",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.deliverResult(c.blockTxsResultChan, msg)
}

func (c *Client) handleVotes(msg protocol.Message) {
	c.votesResultChan <- msg
}

func (c *Client) handleNextBlockAndTxsInRange(msg protocol.Message) {
	c.blockRangeResultChan <- msg
}

func (c *Client) handleLastBlockAndTxsInRange(msg protocol.Message) {
	c.blockRangeResultChan <- msg
}
