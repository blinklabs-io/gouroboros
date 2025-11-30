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

package blockfetch

import (
	"errors"
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/common"
)

// Client implements the Block Fetch protocol client, which requests blocks from a server.
type Client struct {
	*protocol.Protocol
	config               *Config           // Protocol configuration
	callbackContext      CallbackContext   // Callback context for client
	blockChan            chan ledger.Block // Channel for received blocks
	startBatchResultChan chan error        // Channel for batch start results
	busyMutex            sync.Mutex        // Mutex for busy state
	blockUseCallback     bool              // Whether to use callback for blocks
	onceStart            sync.Once         // Ensures Start is only called once
	onceStop             sync.Once         // Ensures Stop is only called once
}

// NewClient creates a new Block Fetch protocol client with the given options and configuration.
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
		Logger:              protoOptions.Logger,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            stateMap,
		InitialState:        StateIdle,
	}
	if c.config != nil {
		protoConfig.RecvQueueSize = c.config.RecvQueueSize
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

// Start begins the Block Fetch client protocol. Safe to call multiple times.
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
			close(c.blockChan)
			close(c.startBatchResultChan)
		}()
	})
}

// Stop stops the Block Fetch client protocol and sends a ClientDone message.
func (c *Client) Stop() error {
	var err error
	c.onceStop.Do(func() {
		c.Protocol.Logger().
			Debug("stopping client protocol",
				"component", "network",
				"protocol", ProtocolName,
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
		if !c.IsDone() {
			msg := NewMsgClientDone()
			if err = c.SendMessage(msg); err != nil {
				return
			}
		}
	})
	return err
}

// GetBlockRange starts an async process to fetch all blocks in the specified range (inclusive).
// The provided callbacks are used for each block and when the batch is done.
func (c *Client) GetBlockRange(start common.Point, end common.Point) error {
	c.Protocol.Logger().
		Debug(
			fmt.Sprintf("calling GetBlockRange(start: {Slot: %d, Hash: %x}, end: {Slot: %d, Hash: %x})",
				start.Slot,
				start.Hash,
				end.Slot,
				end.Hash,
			),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	// NOTE: this will be unlocked on BatchDone
	c.busyMutex.Lock()
	c.blockUseCallback = true
	msg := NewMsgRequestRange(start, end)
	if err := c.SendMessage(msg); err != nil {
		c.busyMutex.Unlock()
		return err
	}
	err, ok := <-c.startBatchResultChan
	if !ok {
		c.busyMutex.Unlock()
		return protocol.ErrProtocolShuttingDown
	}
	if err != nil {
		c.busyMutex.Unlock()
		return err
	}
	return nil
}

// GetBlock requests and returns a single block specified by the provided point.
// This is a synchronous call that returns the block or an error.
func (c *Client) GetBlock(point common.Point) (ledger.Block, error) {
	c.Protocol.Logger().
		Debug(
			fmt.Sprintf("calling GetBlock(point: {Slot: %d, Hash: %x})", point.Slot, point.Hash),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	// NOTE: this will be unlocked on BatchDone
	c.busyMutex.Lock()
	c.blockUseCallback = false
	msg := NewMsgRequestRange(point, point)
	if err := c.SendMessage(msg); err != nil {
		c.busyMutex.Unlock()
		return nil, err
	}
	err, ok := <-c.startBatchResultChan
	if !ok {
		c.busyMutex.Unlock()
		return nil, protocol.ErrProtocolShuttingDown
	}
	if err != nil {
		c.busyMutex.Unlock()
		return nil, err
	}
	block, ok := <-c.blockChan
	if !ok {
		c.busyMutex.Unlock()
		return nil, protocol.ErrProtocolShuttingDown
	}
	return block, nil
}

// messageHandler handles incoming protocol messages for the client.
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

// handleStartBatch handles the StartBatch message from the server.
func (c *Client) handleStartBatch() error {
	c.Protocol.Logger().
		Debug("starting batch",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	// Check for shutdown
	select {
	case <-c.DoneChan():
		return protocol.ErrProtocolShuttingDown
	default:
	}
	c.startBatchResultChan <- nil
	return nil
}

// handleNoBlocks handles the NoBlocks message from the server.
func (c *Client) handleNoBlocks() error {
	c.Protocol.Logger().
		Debug("no blocks returned",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	// Check for shutdown
	select {
	case <-c.DoneChan():
		return protocol.ErrProtocolShuttingDown
	default:
	}
	err := errors.New("block(s) not found")
	c.startBatchResultChan <- err
	return nil
}

// handleBlock handles the Block message from the server.
func (c *Client) handleBlock(msgGeneric protocol.Message) error {
	c.Protocol.Logger().
		Debug("block returned",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	msg := msgGeneric.(*MsgBlock)
	// Decode only enough to get the block type value
	var wrappedBlock WrappedBlock
	if _, err := cbor.Decode(msg.WrappedBlock, &wrappedBlock); err != nil {
		return fmt.Errorf("%s: decode error: %w", ProtocolName, err)
	}
	var block ledger.Block
	if !c.blockUseCallback || c.config.BlockFunc != nil {
		var err error
		block, err = ledger.NewBlockFromCbor(
			wrappedBlock.Type,
			wrappedBlock.RawBlock,
		)
		if err != nil {
			return err
		}
	}
	// Check for shutdown
	select {
	case <-c.DoneChan():
		return protocol.ErrProtocolShuttingDown
	default:
	}
	// We use the callback when requesting ranges and the internal channel for a single block
	if c.blockUseCallback {
		if c.config.BlockRawFunc != nil {
			if err := c.config.BlockRawFunc(c.callbackContext, wrappedBlock.Type, wrappedBlock.RawBlock); err != nil {
				return err
			}
		} else if c.config.BlockFunc != nil {
			if err := c.config.BlockFunc(c.callbackContext, wrappedBlock.Type, block); err != nil {
				return err
			}
		} else {
			return errors.New("received block-fetch Block message but no callback function is defined")
		}
	} else {
		c.blockChan <- block
	}
	return nil
}

// handleBatchDone handles the BatchDone message from the server.
func (c *Client) handleBatchDone() error {
	c.Protocol.Logger().
		Debug("batch done",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	// Notify the user if requested
	if c.blockUseCallback && c.config.BatchDoneFunc != nil {
		if err := c.config.BatchDoneFunc(c.callbackContext); err != nil {
			return err
		}
	}
	c.busyMutex.Unlock()
	return nil
}
