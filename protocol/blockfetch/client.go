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
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

type clientLifecycleState uint8

const (
	clientStateNew clientLifecycleState = iota
	clientStateStarting
	clientStateRunning
	clientStateStopped
)

// Client implements the Block Fetch protocol client, which requests blocks from a server.
type Client struct {
	*protocol.Protocol
	config               *Config           // Protocol configuration
	callbackContext      CallbackContext   // Callback context for client
	blockChan            chan ledger.Block // Channel for received blocks
	startBatchResultChan chan error        // Channel for batch start results
	busyMutex            sync.Mutex        // Mutex for busy state
	lifecycleMutex       sync.Mutex        // Mutex for lifecycle state
	lifecycleState       clientLifecycleState
	startingDone         chan struct{}
	protoOptions         protocol.ProtocolOptions
	blockUseCallback     bool // Whether to use callback for blocks
	protoStarted         bool // Whether Protocol.Start() was called
}

// NewClient creates a new Block Fetch protocol client with the given options and configuration.
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:         cfg,
		protoOptions:   protoOptions,
		lifecycleState: clientStateNew,
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	c.initProtocol()
	return c
}

func (c *Client) initProtocol() {
	// Recreate channels
	c.blockChan = make(chan ledger.Block)
	c.startBatchResultChan = make(chan error)
	c.protoStarted = false

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
		Muxer:               c.protoOptions.Muxer,
		Logger:              c.protoOptions.Logger,
		ErrorChan:           c.protoOptions.ErrorChan,
		Mode:                c.protoOptions.Mode,
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
}

// Start begins the Block Fetch client protocol. Safe to call multiple times.
func (c *Client) Start() {
	for {
		c.lifecycleMutex.Lock()

		switch c.lifecycleState {
		case clientStateRunning:
			c.lifecycleMutex.Unlock()
			return

		case clientStateStarting:
			// Another goroutine is already starting. Wait for it to complete.
			ch := c.startingDone
			c.lifecycleMutex.Unlock()
			if ch != nil {
				<-ch
			}
			// Re-check state after the in-flight start completes
			continue

		case clientStateStopped, clientStateNew:
			// We will be the goroutine that performs initialization/start.
			prevState := c.lifecycleState
			c.lifecycleState = clientStateStarting
			ch := make(chan struct{})
			c.startingDone = ch

			oldProto := c.Protocol
			oldProtoStarted := c.protoStarted
			var oldDone <-chan struct{}
			// Only wait for old protocol if it was actually started.
			// If Stop() was called during clientStateStarting before Protocol.Start(),
			// the protocol's DoneChan will never close.
			if prevState == clientStateStopped && oldProto != nil &&
				oldProtoStarted {
				oldDone = oldProto.DoneChan()
			}
			c.lifecycleMutex.Unlock()

			// If we were stopped, ensure the old instance is fully stopped before re-registering.
			if oldDone != nil {
				oldProto.Stop()
				<-oldDone
			}

			c.lifecycleMutex.Lock()
			// If we were stopped by someone else while waiting, don't continue.
			if c.lifecycleState != clientStateStarting {
				if c.startingDone == ch {
					close(ch)
					c.startingDone = nil
				}
				c.lifecycleMutex.Unlock()
				return
			}

			// Reinitialize protocol when transitioning from stopped->start (or if nil).
			if c.Protocol == nil || prevState == clientStateStopped {
				c.initProtocol()
			}

			c.Protocol.Logger().
				Debug("starting client protocol",
					"component", "network",
					"protocol", ProtocolName,
					"connection_id", c.callbackContext.ConnectionId.String(),
				)
			c.Protocol.Start()
			c.protoStarted = true
			c.lifecycleState = clientStateRunning
			if c.startingDone == ch {
				close(ch)
				c.startingDone = nil
			}
			c.lifecycleMutex.Unlock()
			return

		default:
			// Should not happen; treat as stopped.
			c.lifecycleState = clientStateStopped
			c.lifecycleMutex.Unlock()
			continue
		}
	}
}

// Stop stops the Block Fetch client protocol and sends a ClientDone message.
func (c *Client) Stop() error {
	const busyLockTimeout = 5 * time.Second
	deadline := time.Now().Add(busyLockTimeout)
	busyLocked := false
	for {
		if c.busyMutex.TryLock() {
			busyLocked = true
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	c.lifecycleMutex.Lock()
	defer c.lifecycleMutex.Unlock()

	switch c.lifecycleState {
	case clientStateNew, clientStateStopped:
		if busyLocked {
			c.busyMutex.Unlock()
		}
		return nil
	case clientStateStarting:
		// Mark as stopped so Start() will abort when it re-checks state
		c.lifecycleState = clientStateStopped
		// Unblock Start() if it's waiting
		if c.startingDone != nil {
			close(c.startingDone)
			c.startingDone = nil
		}
		if busyLocked {
			c.busyMutex.Unlock()
		}
		return nil
	case clientStateRunning:
		// Continue with normal stop logic below
	}

	c.Protocol.Logger().
		Debug("stopping client protocol",
			"component", "network",
			"protocol", ProtocolName,
			"connection_id", c.callbackContext.ConnectionId.String(),
		)

	var sendErr error
	// Check if protocol is already done before sending ClientDone message
	if !c.IsDone() {
		msg := NewMsgClientDone()
		sendErr = c.SendMessage(msg)
		_ = c.WaitSendQueueDrained(250 * time.Millisecond)
	}
	if busyLocked {
		c.busyMutex.Unlock()
	}

	// Stop/unregister the underlying protocol instance first, then wait for
	// message handlers to finish before closing channels to avoid send-on-closed panics.
	doneChan := c.DoneChan()
	c.Protocol.Stop()
	c.lifecycleState = clientStateStopped

	// Capture channel references before releasing lock to avoid closing
	// channels recreated by a concurrent Start().
	blockChanToClose := c.blockChan
	startBatchResultChanToClose := c.startBatchResultChan
	c.blockChan = nil
	c.startBatchResultChan = nil

	// Release lock while waiting for protocol shutdown to avoid deadlock
	c.lifecycleMutex.Unlock()
	<-doneChan

	// Now safe to close captured channels - message handlers have stopped
	if blockChanToClose != nil {
		close(blockChanToClose)
	}
	if startBatchResultChanToClose != nil {
		close(startBatchResultChanToClose)
	}
	c.lifecycleMutex.Lock()
	// Unblock any goroutine waiting for an in-progress start.
	if c.startingDone != nil {
		close(c.startingDone)
		c.startingDone = nil
	}
	return sendErr
}

// GetBlockRange starts an async process to fetch all blocks in the specified range (inclusive).
// The provided callbacks are used for each block and when the batch is done.
func (c *Client) GetBlockRange(start pcommon.Point, end pcommon.Point) error {
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
	// Wait for batch start
	select {
	case err, ok := <-c.startBatchResultChan:
		if !ok {
			c.busyMutex.Unlock()
			return protocol.ErrProtocolShuttingDown
		}
		if err != nil {
			c.busyMutex.Unlock()
			return err
		}
	case <-c.DoneChan():
		c.busyMutex.Unlock()
		return protocol.ErrProtocolShuttingDown
	}
	return nil
}

// GetBlock requests and returns a single block specified by the provided point.
// This is a synchronous call that returns the block or an error.
func (c *Client) GetBlock(point pcommon.Point) (ledger.Block, error) {
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
	// Wait for batch start
	select {
	case err, ok := <-c.startBatchResultChan:
		if !ok {
			c.busyMutex.Unlock()
			return nil, protocol.ErrProtocolShuttingDown
		}
		if err != nil {
			c.busyMutex.Unlock()
			return nil, err
		}
	case <-c.DoneChan():
		c.busyMutex.Unlock()
		return nil, protocol.ErrProtocolShuttingDown
	}
	// Wait for block
	select {
	case block, ok := <-c.blockChan:
		if !ok {
			c.busyMutex.Unlock()
			return nil, protocol.ErrProtocolShuttingDown
		}
		return block, nil
	case <-c.DoneChan():
		c.busyMutex.Unlock()
		return nil, protocol.ErrProtocolShuttingDown
	}
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
			lcommon.VerifyConfig{
				SkipBodyHashValidation: c.config.SkipBlockValidation,
			},
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
