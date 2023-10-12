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

package chainsync

import (
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/common"
)

// Client implements the ChainSync client
type Client struct {
	*protocol.Protocol
	config                *Config
	busyMutex             sync.Mutex
	intersectResultChan   chan error
	readyForNextBlockChan chan bool
	wantCurrentTip        bool
	currentTipChan        chan Tip
	wantFirstBlock        bool
	firstBlockChan        chan common.Point
	onceStop              sync.Once
}

// NewClient returns a new ChainSync client object
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
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
		intersectResultChan:   make(chan error),
		readyForNextBlockChan: make(chan bool),
		currentTipChan:        make(chan Tip),
		firstBlockChan:        make(chan common.Point),
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
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.messageHandler,
		MessageFromCborFunc: msgFromCborFunc,
		StateMap:            stateMap,
		InitialState:        stateIdle,
	}
	c.Protocol = protocol.New(protoConfig)
	// Start goroutine to cleanup resources on protocol shutdown
	go func() {
		<-c.Protocol.DoneChan()
		close(c.intersectResultChan)
		close(c.readyForNextBlockChan)
		close(c.currentTipChan)
		close(c.firstBlockChan)
	}()
	return c
}

func (c *Client) messageHandler(msg protocol.Message, isResponse bool) error {
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
		err = fmt.Errorf("%s: received unexpected message type %d", ProtocolName, msg.Type())
	}
	return err
}

// Stop transitions the protocol to the Done state. No more protocol operations will be possible afterward
func (c *Client) Stop() error {
	var err error
	c.onceStop.Do(func() {
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
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	c.wantCurrentTip = true
	msg := NewMsgFindIntersect([]common.Point{})
	if err := c.SendMessage(msg); err != nil {
		return nil, err
	}
	tip := <-c.currentTipChan
	// Clear out intersect result channel to prevent blocking
	<-c.intersectResultChan
	c.wantCurrentTip = false
	return &tip, nil
}

// GetAvailableBlockRange returns the start and end of the range of available blocks given the provided intersect
// point(s).
func (c *Client) GetAvailableBlockRange(intersectPoints []common.Point) (common.Point, common.Point, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	c.wantCurrentTip = true
	c.wantFirstBlock = true
	var start, end common.Point
	msgFindIntersect := NewMsgFindIntersect(intersectPoints)
	if err := c.SendMessage(msgFindIntersect); err != nil {
		return start, end, err
	}
	select {
	case tip := <-c.currentTipChan:
		end = tip.Point
		// Clear out intersect result channel to prevent blocking
		<-c.intersectResultChan
	case err := <-c.intersectResultChan:
		return start, end, err
	}
	c.wantCurrentTip = false
	// Request the next block. This should result in a rollback
	msgRequestNext := NewMsgRequestNext()
	if err := c.SendMessage(msgRequestNext); err != nil {
		return start, end, err
	}
	for {
		select {
		case point := <-c.firstBlockChan:
			start = point
			c.wantFirstBlock = false
		case <-c.readyForNextBlockChan:
			// Request the next block
			msg := NewMsgRequestNext()
			if err := c.SendMessage(msg); err != nil {
				return start, end, err
			}
		}
		if !c.wantFirstBlock {
			break
		}
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
	msg := NewMsgFindIntersect(intersectPoints)
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	if err := <-c.intersectResultChan; err != nil {
		return err
	}
	// Pipeline the initial block requests to speed things up a bit
	// Using a value higher than 10 seems to cause problems with NtN
	for i := 0; i <= c.config.PipelineLimit; i++ {
		msg := NewMsgRequestNext()
		if err := c.SendMessage(msg); err != nil {
			return err
		}
	}
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
		// Request the next block
		// In practice we already have multiple block requests pipelined
		// and this just adds another one to the pile
		msg := NewMsgRequestNext()
		if err := c.SendMessage(msg); err != nil {
			c.SendError(err)
			return
		}
		c.busyMutex.Unlock()
	}
}

func (c *Client) handleAwaitReply() error {
	return nil
}

func (c *Client) handleRollForward(msgGeneric protocol.Message) error {
	if c.config.RollForwardFunc == nil && !c.wantFirstBlock {
		return fmt.Errorf("received chain-sync RollForward message but no callback function is defined")
	}
	var callbackErr error
	if c.Mode() == protocol.ProtocolModeNodeToNode {
		msg := msgGeneric.(*MsgRollForwardNtN)
		var blockHeader ledger.BlockHeader
		var blockType uint
		blockEra := msg.WrappedHeader.Era
		switch blockEra {
		case ledger.BlockHeaderTypeByron:
			blockType = msg.WrappedHeader.ByronType()
			var err error
			blockHeader, err = ledger.NewBlockHeaderFromCbor(blockType, msg.WrappedHeader.HeaderCbor())
			if err != nil {
				return err
			}
		default:
			// Map block header types to block types
			blockTypeMap := map[uint]uint{
				ledger.BlockHeaderTypeShelley: ledger.BlockTypeShelley,
				ledger.BlockHeaderTypeAllegra: ledger.BlockTypeAllegra,
				ledger.BlockHeaderTypeMary:    ledger.BlockTypeMary,
				ledger.BlockHeaderTypeAlonzo:  ledger.BlockTypeAlonzo,
				ledger.BlockHeaderTypeBabbage: ledger.BlockTypeBabbage,
			}
			blockType = blockTypeMap[blockEra]
			var err error
			blockHeader, err = ledger.NewBlockHeaderFromCbor(blockType, msg.WrappedHeader.HeaderCbor())
			if err != nil {
				return err
			}
		}
		if c.wantFirstBlock {
			blockHash, err := hex.DecodeString(blockHeader.Hash())
			if err != nil {
				return err
			}
			point := common.NewPoint(blockHeader.SlotNumber(), blockHash)
			c.firstBlockChan <- point
			return nil
		}
		// Call the user callback function
		callbackErr = c.config.RollForwardFunc(blockType, blockHeader, msg.Tip)
	} else {
		msg := msgGeneric.(*MsgRollForwardNtC)
		blk, err := ledger.NewBlockFromCbor(msg.BlockType(), msg.BlockCbor())
		if err != nil {
			return err
		}
		if c.wantFirstBlock {
			blockHash, err := hex.DecodeString(blk.Hash())
			if err != nil {
				return err
			}
			point := common.NewPoint(blk.SlotNumber(), blockHash)
			c.firstBlockChan <- point
			return nil
		}
		// Call the user callback function
		callbackErr = c.config.RollForwardFunc(msg.BlockType(), blk, msg.Tip)
	}
	if callbackErr != nil {
		if callbackErr == StopSyncProcessError {
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

func (c *Client) handleRollBackward(msgGeneric protocol.Message) error {
	if !c.wantFirstBlock {
		if c.config.RollBackwardFunc == nil {
			return fmt.Errorf("received chain-sync RollBackward message but no callback function is defined")
		}
		msg := msgGeneric.(*MsgRollBackward)
		// Call the user callback function
		if callbackErr := c.config.RollBackwardFunc(msg.Point, msg.Tip); callbackErr != nil {
			if callbackErr == StopSyncProcessError {
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

func (c *Client) handleIntersectFound(msgGeneric protocol.Message) error {
	if c.wantCurrentTip {
		msgIntersectFound := msgGeneric.(*MsgIntersectFound)
		c.currentTipChan <- msgIntersectFound.Tip
	}
	c.intersectResultChan <- nil
	return nil
}

func (c *Client) handleIntersectNotFound(msgGeneric protocol.Message) error {
	if c.wantCurrentTip {
		msgIntersectNotFound := msgGeneric.(*MsgIntersectNotFound)
		c.currentTipChan <- msgIntersectNotFound.Tip
	}
	c.intersectResultChan <- IntersectNotFoundError{}
	return nil
}
