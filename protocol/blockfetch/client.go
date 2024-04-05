// Copyright 2024 Blink Labs Software
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

package blockfetch

import (
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/common"

	"github.com/blinklabs-io/gouroboros/ledger"
)

type Client struct {
	*protocol.Protocol
	config               *Config
	callbackContext      CallbackContext
	blockChan            chan ledger.Block
	startBatchResultChan chan error
	busyMutex            sync.Mutex
	blockUseCallback     bool
	onceStart            sync.Once
	onceStop             sync.Once
}

func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:               cfg,
		blockChan:            make(chan ledger.Block),
		startBatchResultChan: make(chan error),
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	// Update state map with timeouts
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[StateBusy]; ok {
		entry.Timeout = c.config.BatchStartTimeout
		stateMap[StateBusy] = entry
	}
	if entry, ok := stateMap[StateStreaming]; ok {
		entry.Timeout = c.config.BlockTimeout
		stateMap[StateStreaming] = entry
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
		InitialState:        StateIdle,
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

func (c *Client) Start() {
	c.onceStart.Do(func() {
		c.Protocol.Start()
		// Start goroutine to cleanup resources on protocol shutdown
		go func() {
			<-c.Protocol.DoneChan()
			close(c.blockChan)
			close(c.startBatchResultChan)
		}()
	})
}

func (c *Client) Stop() error {
	var err error
	c.onceStop.Do(func() {
		msg := NewMsgClientDone()
		err = c.SendMessage(msg)
	})
	return err
}

// GetBlockRange starts an async process to fetch all blocks in the specified range (inclusive)
func (c *Client) GetBlockRange(start common.Point, end common.Point) error {
	c.busyMutex.Lock()
	c.blockUseCallback = true
	msg := NewMsgRequestRange(start, end)
	if err := c.SendMessage(msg); err != nil {
		c.busyMutex.Unlock()
		return err
	}
	err, ok := <-c.startBatchResultChan
	if !ok {
		return protocol.ProtocolShuttingDownError
	}
	if err != nil {
		c.busyMutex.Unlock()
		return err
	}
	return nil
}

// GetBlock requests and returns a single block specified by the provided point
func (c *Client) GetBlock(point common.Point) (ledger.Block, error) {
	c.busyMutex.Lock()
	c.blockUseCallback = false
	msg := NewMsgRequestRange(point, point)
	if err := c.SendMessage(msg); err != nil {
		c.busyMutex.Unlock()
		return nil, err
	}
	err, ok := <-c.startBatchResultChan
	if !ok {
		return nil, protocol.ProtocolShuttingDownError
	}
	if err != nil {
		c.busyMutex.Unlock()
		return nil, err
	}
	block, ok := <-c.blockChan
	if !ok {
		return nil, protocol.ProtocolShuttingDownError
	}
	return block, nil
}

func (c *Client) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeStartBatch:
		err = c.handleStartBatch()
	case MessageTypeNoBlocks:
		err = c.handleNoBlocks()
	case MessageTypeBlock:
		err = c.handleBlock(msg)
	case MessageTypeBatchDone:
		err = c.handleBatchDone()
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (c *Client) handleStartBatch() error {
	c.startBatchResultChan <- nil
	return nil
}

func (c *Client) handleNoBlocks() error {
	err := fmt.Errorf("block(s) not found")
	c.startBatchResultChan <- err
	return nil
}

func (c *Client) handleBlock(msgGeneric protocol.Message) error {
	msg := msgGeneric.(*MsgBlock)
	// Decode only enough to get the block type value
	var wrappedBlock WrappedBlock
	if _, err := cbor.Decode(msg.WrappedBlock, &wrappedBlock); err != nil {
		return fmt.Errorf("%s: decode error: %s", ProtocolName, err)
	}
	blk, err := ledger.NewBlockFromCbor(
		wrappedBlock.Type,
		wrappedBlock.RawBlock,
	)
	if err != nil {
		return err
	}
	// We use the callback when requesting ranges and the internal channel for a single block
	if c.blockUseCallback {
		if err := c.config.BlockFunc(c.callbackContext, blk); err != nil {
			return err
		}
	} else {
		c.blockChan <- blk
	}
	return nil
}

func (c *Client) handleBatchDone() error {
	c.busyMutex.Unlock()
	return nil
}
