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

package chainsync

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/common"
)

// Client implements the ChainSync client
type Client struct {
	*protocol.Protocol
	config                   *Config
	callbackContext          CallbackContext
	busyMutex                sync.Mutex
	readyForNextBlockChan    chan bool
	onceStart                sync.Once
	onceStop                 sync.Once
	syncPipelinedRequestNext int

	// waitingForCurrentTipChan will process all the requests for the current tip until the channel
	// is empty.
	//
	// want* only processes one request per message reply received from the server. If the message
	// request fails, it is the responsibility of the caller to clear the channel.
	waitingForCurrentTipChan chan chan<- Tip
	wantCurrentTipChan       chan chan<- Tip
	wantFirstBlockChan       chan chan<- clientPointResult
	wantIntersectFoundChan   chan chan<- clientPointResult
}

type clientPointResult struct {
	tip   Tip
	point common.Point
	error error
}

// NewClient returns a new ChainSync client object
func NewClient(
	stateContext interface{},
	protoOptions protocol.ProtocolOptions,
	cfg *Config,
) *Client {
	// Use node-to-client protocol ID
	ProtocolId := ProtocolIdNtC
	msgFromCborFunc := NewMsgFromCborNtC
	if protoOptions.Mode == protocol.ProtocolModeNodeToNode {
		// Use node-to-node protocol ID
		ProtocolId = ProtocolIdNtN
		msgFromCborFunc = NewMsgFromCborNtN
	}
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:                cfg,
		readyForNextBlockChan: make(chan bool),

		waitingForCurrentTipChan: make(chan chan<- Tip, 20),
		wantCurrentTipChan:       make(chan chan<- Tip, 1),
		wantFirstBlockChan:       make(chan chan<- clientPointResult, 1),
		wantIntersectFoundChan:   make(chan chan<- clientPointResult, 1),
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	// Update state map with timeouts
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[stateIntersect]; ok {
		entry.Timeout = c.config.IntersectTimeout
		stateMap[stateIntersect] = entry
	}
	for _, state := range []protocol.State{stateCanAwait, stateMustReply} {
		if entry, ok := stateMap[state]; ok {
			entry.Timeout = c.config.BlockTimeout
			stateMap[state] = entry
		}
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
		MessageFromCborFunc: msgFromCborFunc,
		StateMap:            stateMap,
		StateContext:        stateContext,
		InitialState:        stateIdle,
	}
	if c.config != nil {
		protoConfig.RecvQueueSize = c.config.RecvQueueSize
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
			<-c.Protocol.DoneChan()
			close(c.readyForNextBlockChan)
		}()
	})
}

// Stop transitions the protocol to the Done state. No more protocol operations will be possible afterward
func (c *Client) Stop() error {
	var err error
	c.onceStop.Do(func() {
		c.Protocol.Logger().
			Debug("stopping client protocol",
				"component", "network",
				"protocol", ProtocolName,
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
		c.busyMutex.Lock()
		defer c.busyMutex.Unlock()
		msg := NewMsgDone()
		if err = c.SendMessage(msg); err != nil {
			return
		}
	})
	return err
}

// GetCurrentTip returns the current chain tip
func (c *Client) GetCurrentTip() (*Tip, error) {
	c.Protocol.Logger().
		Debug("calling GetCurrentTip()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	done := atomic.Bool{}
	requestResultChan := make(chan Tip, 1)
	requestErrorChan := make(chan error, 1)

	go func() {
		c.busyMutex.Lock()
		defer c.busyMutex.Unlock()

		if done.Load() {
			return
		}

		currentTipChan, cancelCurrentTip := c.wantCurrentTip()
		msg := NewMsgFindIntersect([]common.Point{})
		if err := c.SendMessage(msg); err != nil {
			cancelCurrentTip()
			requestErrorChan <- err
			return
		}
		select {
		case <-c.Protocol.DoneChan():
		case tip := <-currentTipChan:
			requestResultChan <- tip
		}
	}()

	waitingResultChan := make(chan Tip, 1)
	waitingForCurrentTipChan := c.waitingForCurrentTipChan

	for {
		select {
		case <-c.Protocol.DoneChan():
			done.Store(true)
			return nil, protocol.ProtocolShuttingDownError
		case waitingForCurrentTipChan <- waitingResultChan:
			// The request is being handled by another request, wait for the result.
			waitingForCurrentTipChan = nil
		case tip := <-waitingResultChan:
			c.Protocol.Logger().
				Debug(
					fmt.Sprintf("received tip results {Slot: %d, Hash: %x, BlockNumber: %d}", tip.Point.Slot, tip.Point.Hash, tip.BlockNumber),
					"component", "network",
					"protocol", ProtocolName,
					"role", "client",
					"connection_id", c.callbackContext.ConnectionId.String(),
				)
			// The result from the other request is ready.
			done.Store(true)
			return &tip, nil
		case tip := <-requestResultChan:
			c.Protocol.Logger().
				Debug(
					fmt.Sprintf("received tip results {Slot: %d, Hash: %x, BlockNumber: %d}", tip.Point.Slot, tip.Point.Hash, tip.BlockNumber),
					"component", "network",
					"protocol", ProtocolName,
					"role", "client",
					"connection_id", c.callbackContext.ConnectionId.String(),
				)
			// If waitingForCurrentTipChan is full, the for loop that empties it might finish the
			// loop before the select statement that writes to it is triggered. For that reason we
			// require requestResultChan here.
			return &tip, nil
		case err := <-requestErrorChan:
			return nil, err
		}
	}
}

// GetAvailableBlockRange returns the start and end of the range of available blocks given the provided intersect
// point(s). Empty start/end points will be returned if there are no additional blocks available.
func (c *Client) GetAvailableBlockRange(
	intersectPoints []common.Point,
) (common.Point, common.Point, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()

	// Use origin if no intersect points were specified
	if len(intersectPoints) == 0 {
		intersectPoints = []common.Point{common.NewPointOrigin()}
	}

	// Debug logging
	switch len(intersectPoints) {
	case 1:
		c.Protocol.Logger().
			Debug(
				fmt.Sprintf(
					"calling GetAvailableBlockRange(intersectPoints: []{Slot: %d, Hash: %x})",
					intersectPoints[0].Slot,
					intersectPoints[0].Hash,
				),
				"component", "network",
				"protocol", ProtocolName,
				"role", "client",
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
	case 2:
		c.Protocol.Logger().
			Debug(
				fmt.Sprintf(
					"calling GetAvailableBlockRange(intersectPoints: []{Slot: %d, Hash: %x},{Slot: %d, Hash: %x})",
					intersectPoints[0].Slot,
					intersectPoints[0].Hash,
					intersectPoints[1].Slot,
					intersectPoints[1].Hash,
				),
				"component", "network",
				"protocol", ProtocolName,
				"role", "client",
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
	default:
		c.Protocol.Logger().
			Debug(
				fmt.Sprintf(
					"calling GetAvailableBlockRange(intersectPoints: %+v)",
					intersectPoints,
				),
				"component", "network",
				"protocol", ProtocolName,
				"role", "client",
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
	}

	// Find our chain intersection
	result := c.requestFindIntersect(intersectPoints)
	if result.error != nil {
		return common.Point{}, common.Point{}, result.error
	}
	start := result.point
	end := result.tip.Point

	// If we're already at the chain tip, return an empty range
	if start.Slot >= end.Slot {
		return common.Point{}, common.Point{}, nil
	}

	// Request the next block to get the first block after the intersect point. This should result in a rollback
	currentTipChan, cancelCurrentTip := c.wantCurrentTip()
	firstBlockChan, cancelFirstBlock := c.wantFirstBlock()
	defer func() {
		if currentTipChan != nil {
			cancelCurrentTip()
		}
		if firstBlockChan != nil {
			cancelFirstBlock()
		}
	}()

	msgRequestNext := NewMsgRequestNext()
	if err := c.SendMessage(msgRequestNext); err != nil {
		return start, end, err
	}
	for {
		select {
		case <-c.DoneChan():
			return start, end, protocol.ProtocolShuttingDownError
		case tip := <-currentTipChan:
			currentTipChan = nil
			end = tip.Point
		case firstBlock := <-firstBlockChan:
			firstBlockChan = nil
			if firstBlock.error != nil {
				return start, end, fmt.Errorf(
					"failed to get first block: %w",
					firstBlock.error,
				)
			}
			start = firstBlock.point
		case <-c.readyForNextBlockChan:
			// Request the next block
			msg := NewMsgRequestNext()
			if err := c.SendMessage(msg); err != nil {
				return start, end, err
			}
		}
		if currentTipChan == nil && firstBlockChan == nil {
			break
		}
	}
	// If we're already at the chain tip, return an empty range
	if start.Slot >= end.Slot {
		return common.Point{}, common.Point{}, nil
	}
	return start, end, nil
}

// Sync begins a chain-sync operation using the provided intersect point(s). Incoming blocks will be delivered
// via the RollForward callback function specified in the protocol config
func (c *Client) Sync(intersectPoints []common.Point) error {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()

	// Use origin if no intersect points were specified
	if len(intersectPoints) == 0 {
		intersectPoints = []common.Point{common.NewPointOrigin()}
	}

	// Debug logging
	switch len(intersectPoints) {
	case 1:
		c.Protocol.Logger().
			Debug(
				fmt.Sprintf(
					"calling Sync(intersectPoints: []{Slot: %d, Hash: %x})",
					intersectPoints[0].Slot,
					intersectPoints[0].Hash,
				),
				"component", "network",
				"protocol", ProtocolName,
				"role", "client",
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
	case 2:
		c.Protocol.Logger().
			Debug(
				fmt.Sprintf(
					"calling Sync(intersectPoints: []{Slot: %d, Hash: %x},{Slot: %d, Hash: %x})",
					intersectPoints[0].Slot,
					intersectPoints[0].Hash,
					intersectPoints[1].Slot,
					intersectPoints[1].Hash,
				),
				"component", "network",
				"protocol", ProtocolName,
				"role", "client",
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
	default:
		c.Protocol.Logger().
			Debug(
				fmt.Sprintf(
					"calling Sync(intersectPoints: %+v)",
					intersectPoints,
				),
				"component", "network",
				"protocol", ProtocolName,
				"role", "client",
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
	}

	intersectResultChan, cancel := c.wantIntersectFound()
	msgFindIntersect := NewMsgFindIntersect(intersectPoints)
	if err := c.SendMessage(msgFindIntersect); err != nil {
		cancel()
		return err
	}
	select {
	case <-c.Protocol.DoneChan():
		return protocol.ProtocolShuttingDownError
	case result := <-intersectResultChan:
		if result.error != nil {
			return result.error
		}
	}

	// Send initial RequestNext
	msgRequestNext := NewMsgRequestNext()
	if err := c.SendMessage(msgRequestNext); err != nil {
		return err
	}
	// Reset pipelined message counter
	c.syncPipelinedRequestNext = 0
	// Start sync loop
	go c.syncLoop()
	return nil
}

func (c *Client) syncLoop() {
	for {
		// Wait for a block to be received
		if ready, ok := <-c.readyForNextBlockChan; !ok {
			// Channel is closed, which means we're shutting down
			return
		} else if !ready {
			// Sync was cancelled
			return
		}
		c.busyMutex.Lock()
		// Wait for next block if we have pipelined messages
		if c.syncPipelinedRequestNext > 0 {
			c.syncPipelinedRequestNext--
			c.busyMutex.Unlock()
			continue
		}
		// Request the next block(s)
		msgCount := max(c.config.PipelineLimit, 1)
		for i := 0; i < msgCount; i++ {
			msg := NewMsgRequestNext()
			if err := c.SendMessage(msg); err != nil {
				c.SendError(err)
				c.busyMutex.Unlock()
				return
			}
		}
		c.syncPipelinedRequestNext = msgCount - 1
		c.busyMutex.Unlock()
	}
}

func (c *Client) sendCurrentTip(tip Tip) {
	// Sends to the requester.
	select {
	case ch := <-c.wantCurrentTipChan:
		ch <- tip
	default:
	}

	// Sends to all passive listeners that are in the queue.
	for {
		select {
		case ch := <-c.waitingForCurrentTipChan:
			ch <- tip
		default:
			return
		}
	}
}

// wantCurrentTip returns a channel that will receive the current tip, and a function that can be
// used to clear the channel if sending the request message fails.
func (c *Client) wantCurrentTip() (<-chan Tip, func()) {
	ch := make(chan Tip, 1)

	select {
	case <-c.Protocol.DoneChan():
		return nil, func() {}
	case c.wantCurrentTipChan <- ch:
		return ch, func() {
			select {
			case <-c.wantCurrentTipChan:
			default:
			}
		}
	}
}

// wantFirstBlock returns a channel that will receive the first block after the current tip, and a
// function that can be used to clear the channel if sending the request message fails.
func (c *Client) wantFirstBlock() (<-chan clientPointResult, func()) {
	ch := make(chan clientPointResult, 1)

	select {
	case <-c.Protocol.DoneChan():
		return nil, func() {}
	case c.wantFirstBlockChan <- ch:
		return ch, func() {
			select {
			case <-c.wantFirstBlockChan:
			default:
			}
		}
	}
}

// wantIntersectFound returns a channel that will receive the result of the next intersect request,
// and a function that can be used to clear the channel if sending the request message fails.
func (c *Client) wantIntersectFound() (<-chan clientPointResult, func()) {
	ch := make(chan clientPointResult, 1)

	select {
	case <-c.Protocol.DoneChan():
		return nil, func() {}
	case c.wantIntersectFoundChan <- ch:
		return ch, func() {
			select {
			case <-c.wantIntersectFoundChan:
			default:
			}
		}
	}
}

func (c *Client) requestFindIntersect(
	intersectPoints []common.Point,
) clientPointResult {
	resultChan, cancel := c.wantIntersectFound()
	msg := NewMsgFindIntersect(intersectPoints)
	if err := c.SendMessage(msg); err != nil {
		cancel()
		return clientPointResult{error: err}
	}

	select {
	case <-c.Protocol.DoneChan():
		return clientPointResult{error: protocol.ProtocolShuttingDownError}
	case result := <-resultChan:
		return result
	}
}

func (c *Client) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeAwaitReply:
		err = c.handleAwaitReply()
	case MessageTypeRollForward:
		err = c.handleRollForward(msg)
	case MessageTypeRollBackward:
		err = c.handleRollBackward(msg)
	case MessageTypeIntersectFound:
		err = c.handleIntersectFound(msg)
	case MessageTypeIntersectNotFound:
		err = c.handleIntersectNotFound(msg)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (c *Client) handleAwaitReply() error {
	c.Protocol.Logger().
		Debug("waiting for next reply",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	return nil
}

func (c *Client) handleRollForward(msgGeneric protocol.Message) error {
	c.Protocol.Logger().
		Debug("roll forward",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	firstBlockChan := func() chan<- clientPointResult {
		select {
		case ch := <-c.wantFirstBlockChan:
			return ch
		default:
			return nil
		}
	}()
	if firstBlockChan == nil &&
		(c.config == nil || (c.config.RollForwardFunc == nil && c.config.RollForwardRawFunc == nil)) {
		return errors.New(
			"received chain-sync RollForward message but no callback function is defined",
		)
	}
	var callbackErr error
	if c.Mode() == protocol.ProtocolModeNodeToNode {
		msg := msgGeneric.(*MsgRollForwardNtN)
		c.sendCurrentTip(msg.Tip)

		var blockHeader ledger.BlockHeader
		var blockHeaderBytes []byte
		var blockType uint
		blockEra := msg.WrappedHeader.Era

		switch blockEra {
		case ledger.BlockHeaderTypeByron:
			blockType = msg.WrappedHeader.ByronType()
			blockHeaderBytes = msg.WrappedHeader.HeaderCbor()
		default:
			// Map block header type to block type
			blockType = ledger.BlockHeaderToBlockTypeMap[blockEra]
			blockHeaderBytes = msg.WrappedHeader.HeaderCbor()
		}
		if firstBlockChan != nil || c.config.RollForwardFunc != nil {
			// Decode header
			var err error
			blockHeader, err = ledger.NewBlockHeaderFromCbor(
				blockType,
				msg.WrappedHeader.HeaderCbor(),
			)
			if err != nil {
				if firstBlockChan != nil {
					firstBlockChan <- clientPointResult{error: err}
				}
				return err
			}
		}
		if firstBlockChan != nil {
			if blockHeader == nil {
				err := errors.New("missing block header")
				firstBlockChan <- clientPointResult{error: err}
				return err
			}
			blockHash, err := hex.DecodeString(blockHeader.Hash())
			if err != nil {
				firstBlockChan <- clientPointResult{error: err}
				return err
			}
			point := common.NewPoint(blockHeader.SlotNumber(), blockHash)
			firstBlockChan <- clientPointResult{tip: msg.Tip, point: point}
			return nil
		}
		// Call the user callback function
		if c.config.RollForwardRawFunc != nil {
			callbackErr = c.config.RollForwardRawFunc(
				c.callbackContext,
				blockType,
				blockHeaderBytes,
				msg.Tip,
			)
		} else {
			callbackErr = c.config.RollForwardFunc(
				c.callbackContext,
				blockType,
				blockHeader,
				msg.Tip,
			)
		}
	} else {
		msg := msgGeneric.(*MsgRollForwardNtC)
		c.sendCurrentTip(msg.Tip)

		var block ledger.Block

		if firstBlockChan != nil || c.config.RollForwardFunc != nil {
			var err error
			block, err = ledger.NewBlockFromCbor(msg.BlockType(), msg.BlockCbor())
			if err != nil {
				if firstBlockChan != nil {
					firstBlockChan <- clientPointResult{error: err}
				}
				return err
			}
		}
		if firstBlockChan != nil {
			if block == nil {
				err := errors.New("missing block")
				firstBlockChan <- clientPointResult{error: err}
				return err
			}
			blockHash, err := hex.DecodeString(block.Hash())
			if err != nil {
				firstBlockChan <- clientPointResult{error: err}
				return err
			}
			point := common.NewPoint(block.SlotNumber(), blockHash)
			firstBlockChan <- clientPointResult{tip: msg.Tip, point: point}
			return nil
		}
		// Call the user callback function
		if c.config.RollForwardRawFunc != nil {
			callbackErr = c.config.RollForwardRawFunc(
				c.callbackContext,
				msg.BlockType(),
				msg.BlockCbor(),
				msg.Tip,
			)
		} else {
			callbackErr = c.config.RollForwardFunc(
				c.callbackContext,
				msg.BlockType(),
				block,
				msg.Tip,
			)
		}
	}
	if callbackErr != nil {
		if errors.Is(callbackErr, StopSyncProcessError) {
			// Signal that we're cancelling the sync
			c.readyForNextBlockChan <- false
			return nil
		} else {
			return callbackErr
		}
	}
	// Signal that we're ready for the next block
	c.readyForNextBlockChan <- true
	return nil
}

func (c *Client) handleRollBackward(msg protocol.Message) error {
	c.Protocol.Logger().
		Debug("roll backward",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	msgRollBackward := msg.(*MsgRollBackward)
	c.sendCurrentTip(msgRollBackward.Tip)
	if len(c.wantFirstBlockChan) == 0 {
		if c.config.RollBackwardFunc == nil {
			return errors.New(
				"received chain-sync RollBackward message but no callback function is defined",
			)
		}
		// Call the user callback function
		if callbackErr := c.config.RollBackwardFunc(c.callbackContext, msgRollBackward.Point, msgRollBackward.Tip); callbackErr != nil {
			if errors.Is(callbackErr, StopSyncProcessError) {
				// Signal that we're cancelling the sync
				c.readyForNextBlockChan <- false
				return nil
			} else {
				return callbackErr
			}
		}
	}
	// Signal that we're ready for the next block
	c.readyForNextBlockChan <- true
	return nil
}

func (c *Client) handleIntersectFound(msg protocol.Message) error {
	c.Protocol.Logger().
		Debug("chain intersect found",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	msgIntersectFound := msg.(*MsgIntersectFound)
	c.sendCurrentTip(msgIntersectFound.Tip)

	select {
	case ch := <-c.wantIntersectFoundChan:
		ch <- clientPointResult{tip: msgIntersectFound.Tip, point: msgIntersectFound.Point}
	default:
	}
	return nil
}

func (c *Client) handleIntersectNotFound(msgGeneric protocol.Message) error {
	c.Protocol.Logger().
		Debug("chain intersect not found",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	msgIntersectNotFound := msgGeneric.(*MsgIntersectNotFound)
	c.sendCurrentTip(msgIntersectNotFound.Tip)

	select {
	case ch := <-c.wantIntersectFoundChan:
		ch <- clientPointResult{tip: msgIntersectNotFound.Tip, error: IntersectNotFoundError}
	default:
	}
	return nil
}
