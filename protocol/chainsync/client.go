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

package chainsync

import (
	"encoding/hex"
	"fmt"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/common"
)

// Client implements the ChainSync client
type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext

	clientDoneChan         chan struct{}
	messageHandlerDoneChan chan struct{}
	requestHandlerDoneChan chan struct{}

	handleMessageChan     chan clientHandleMessage
	immediateStopChan     chan struct{}
	readyForNextBlockChan chan bool

	requestFindIntersectChan          chan clientFindIntersectRequest
	requestGetAvailableBlockRangeChan chan clientGetAvailableBlockRangeRequest
	requestStartSyncingChan           chan clientStartSyncingRequest
	requestStopClientChan             chan chan<- error
	wantCurrentTipChan                chan chan<- Tip
	wantFirstBlockChan                chan chan<- clientPointResult
	wantIntersectFoundChan            chan chan<- clientPointResult
	wantRollbackChan                  chan chan<- Tip
}

type clientFindIntersectRequest struct {
	intersectPoints []common.Point
	resultChan      chan<- clientPointResult
}

type clientGetAvailableBlockRangeRequest struct {
	intersectPoints []common.Point
	resultChan      chan<- clientGetAvailableBlockRangeResult
}

type clientGetAvailableBlockRangeResult struct {
	start common.Point
	end   common.Point
	error error
}

type clientHandleMessage struct {
	message   protocol.Message
	errorChan chan<- error
}

type clientPointResult struct {
	tip   Tip
	point common.Point
	error error
}

type clientStartSyncingRequest struct {
	intersectPoints []common.Point
	resultChan      chan<- error
}

// NewClient returns a new ChainSync client object
func NewClient(stateContext interface{}, protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	// Use node-to-client protocol ID
	ProtocolId := ProtocolIdNtC
	msgFromCborFunc := NewMsgFromCborNtC
	if protoOptions.Mode == protocol.ProtocolModeNodeToNode {
		// Use node-to-node protocol ID
		ProtocolId = ProtocolIdNtN
		msgFromCborFunc = NewMsgFromCborNtN
	}
	// TODO: Storing the config as a pointer is unsafe.
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}

	c := &Client{
		config: cfg,

		clientDoneChan:         make(chan struct{}),
		messageHandlerDoneChan: make(chan struct{}),
		requestHandlerDoneChan: make(chan struct{}),

		handleMessageChan: make(chan clientHandleMessage),
		immediateStopChan: make(chan struct{}, 1),
		// TODO: This channel set to 0 length, which would block message handling. Review if this is ok
		// to set to 1.
		readyForNextBlockChan: make(chan bool),

		requestFindIntersectChan:          make(chan clientFindIntersectRequest),
		requestGetAvailableBlockRangeChan: make(chan clientGetAvailableBlockRangeRequest),
		requestStartSyncingChan:           make(chan clientStartSyncingRequest),
		requestStopClientChan:             make(chan chan<- error),

		// TODO: We should only have a buffer size of 1 here, and review the protocol to make sure
		// it always responds to messages. If it doesn't, we should add a timeout to the channels
		// and error handling in case the node misbehaves.
		wantCurrentTipChan:     make(chan chan<- Tip),
		wantFirstBlockChan:     make(chan chan<- clientPointResult, 1),
		wantIntersectFoundChan: make(chan chan<- clientPointResult, 1),
		wantRollbackChan:       make(chan chan<- Tip, 1),
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
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.messageHandler,
		MessageFromCborFunc: msgFromCborFunc,
		StateMap:            stateMap,
		StateContext:        stateContext,
		InitialState:        stateIdle,
	}
	c.Protocol = protocol.New(protoConfig)

	go func() {
		select {
		case <-c.Protocol.DoneChan():
		case <-c.immediateStopChan:
		}

		// Strictly speaking, the client isn't done here as either the request or message handling
		// loops might still be running.
		close(c.clientDoneChan)
	}()
	go c.requestHandlerLoop()
	go c.messageHandlerLoop()

	return c
}

// Close immediately transitions the protocol to the Done state. No more protocol operations will be
// possible afterward.
func (c *Client) Close() error {
	select {
	case <-c.clientDoneChan:
	case c.immediateStopChan <- struct{}{}:
	}

	<-c.clientDoneChan
	<-c.messageHandlerDoneChan
	<-c.requestHandlerDoneChan
	return nil
}

// Stop gracefully transitions the protocol to the Done state. No more protocol operations will be
// possible afterward.
func (c *Client) Stop() error {
	ch := make(chan error)

	select {
	case <-c.clientDoneChan:
		return nil
	case c.requestStopClientChan <- ch:
	}

	select {
	case <-c.clientDoneChan:
		return nil
	case err := <-ch:
		<-c.clientDoneChan
		<-c.messageHandlerDoneChan
		<-c.requestHandlerDoneChan

		if err == protocol.ProtocolShuttingDownError {
			return nil
		}
		return err
	}
}

// GetCurrentTip returns the current chain tip.
func (c *Client) GetCurrentTip() (*Tip, error) {
	currentTipChan := make(chan Tip, 1)
	resultChan := make(chan clientPointResult, 1)
	request := clientFindIntersectRequest{
		intersectPoints: []common.Point{},
		resultChan:      resultChan,
	}

	select {
	case <-c.clientDoneChan:
		return nil, protocol.ProtocolShuttingDownError
	case c.wantCurrentTipChan <- currentTipChan:
		result := <-currentTipChan
		return &result, nil
	case c.requestFindIntersectChan <- request:
	}

	select {
	case <-c.clientDoneChan:
		return nil, protocol.ProtocolShuttingDownError
	case result := <-resultChan:
		if result.error != nil && result.error != IntersectNotFoundError {
			return nil, result.error
		}
		return &result.tip, nil
	}
}

// GetAvailableBlockRange returns the start and end of the range of available blocks given the provided intersect
// point(s). Empty start/end points will be returned if there are no additional blocks available.
func (c *Client) GetAvailableBlockRange(
	intersectPoints []common.Point,
) (common.Point, common.Point, error) {
	resultChan := make(chan clientGetAvailableBlockRangeResult, 1)
	request := clientGetAvailableBlockRangeRequest{
		intersectPoints: intersectPoints,
		resultChan:      resultChan,
	}

	fmt.Printf("GetAvailableBlockRange: %v\n", intersectPoints)

	select {
	case <-c.clientDoneChan:
		return common.Point{}, common.Point{}, protocol.ProtocolShuttingDownError
	case c.requestGetAvailableBlockRangeChan <- request:
	}

	fmt.Printf("GetAvailableBlockRange: waiting for result\n")

	select {
	case <-c.clientDoneChan:
		return common.Point{}, common.Point{}, protocol.ProtocolShuttingDownError
	case result := <-resultChan:
		fmt.Printf("GetAvailableBlockRange: result: %v, %v, %v\n", result.start, result.end, result.error)
		return result.start, result.end, result.error
	}
}

// Sync begins a chain-sync operation using the provided intersect point(s). Incoming blocks will be delivered
// via the RollForward callback function specified in the protocol config
func (c *Client) Sync(intersectPoints []common.Point) error {
	resultChan := make(chan error, 1)
	request := clientStartSyncingRequest{
		intersectPoints: intersectPoints,
		resultChan:      resultChan,
	}

	select {
	case <-c.clientDoneChan:
		return protocol.ProtocolShuttingDownError
	case c.requestStartSyncingChan <- request:
		return c.waitForErrorChan(resultChan)
	}
}

// requestHandlerLoop is the request handler loop for the client.
func (c *Client) requestHandlerLoop() {
	defer func() {
		close(c.requestHandlerDoneChan)

		select {
		case <-c.clientDoneChan:
		case c.immediateStopChan <- struct{}{}:
		}
	}()

	requestFindIntersectChan := c.requestFindIntersectChan
	requestGetAvailableBlockRangeChan := c.requestGetAvailableBlockRangeChan

	isSyncing := false
	syncPipelineCount := 0
	// syncPipelineLimit := c.config.PipelineLimit

	// REMOVE: Testing pipeline limit
	syncPipelineLimit := 10

	// TODO: Change NewClient to return errors on invalid configuration.
	if syncPipelineLimit < 1 {
		syncPipelineLimit = 1
	}

	for {
		select {
		case <-c.clientDoneChan:
			return

		case request := <-requestFindIntersectChan:
			result := c.requestFindIntersect(request.intersectPoints)
			request.resultChan <- result
			if result.error != nil && result.error != IntersectNotFoundError {
				c.SendError(result.error)
				return
			}

		case request := <-requestGetAvailableBlockRangeChan:
			result := c.requestGetAvailableBlockRange(request.intersectPoints)
			request.resultChan <- result
			if result.error != nil && result.error != IntersectNotFoundError {
				c.SendError(result.error)
				return
			}

		case request := <-c.requestStartSyncingChan:
			if isSyncing {
				// Already syncing. This should be an error(?)
				request.resultChan <- nil
				return
			}
			if syncPipelineCount != 0 {
				// TODO: Review this behavior. Should we wait for the current pipeline to finish?
				err := fmt.Errorf("sync pipeline is not empty")
				request.resultChan <- err
				c.SendError(err)
				return
			}

			// Disable requests that aren't allowed during syncing. (Review this)
			isSyncing = true
			requestFindIntersectChan = nil
			requestGetAvailableBlockRangeChan = nil

			err := c.requestSync(request.intersectPoints)
			request.resultChan <- err
			if err != nil {
				if err == IntersectNotFoundError {
					continue
				}
				c.SendError(err)
				return
			}

			for syncPipelineCount < syncPipelineLimit {
				fmt.Printf("requestNextBlockChan: %v : %v\n", syncPipelineCount, c.config.PipelineLimit)

				msg := NewMsgRequestNext()
				if err := c.SendMessage(msg); err != nil {
					c.SendError(err)
					return
				}
				syncPipelineCount++
			}

		case ch := <-c.requestStopClientChan:
			msg := NewMsgDone()
			if err := c.SendMessage(msg); err != nil && err != protocol.ProtocolShuttingDownError {
				fmt.Printf("Error sending Done message: %v\n", err)
				ch <- err
				c.SendError(err)
				return
			}

			fmt.Printf("Client done: Done message sent\n")
			ch <- nil
			return

		case ready := <-c.readyForNextBlockChan:
			if syncPipelineCount != 0 {
				syncPipelineCount--
			}

			if !isSyncing {
				// We're not syncing, so just ignore the ready signal. This can happen if the
				// protocol sends us an unexpected rollforward/rollback message.
				//
				// TODO: Should this be an error?
				fmt.Printf("readyForNextBlock received when not syncing\n")
				continue
			}
			if !ready {
				isSyncing = false
				requestFindIntersectChan = c.requestFindIntersectChan
				requestGetAvailableBlockRangeChan = c.requestGetAvailableBlockRangeChan
				continue
			}

			fmt.Printf("requestNextBlockChan: %v : %v\n", syncPipelineCount, c.config.PipelineLimit)

			msg := NewMsgRequestNext()
			if err := c.SendMessage(msg); err != nil {
				c.SendError(err)
				return
			}
			syncPipelineCount++
		}
	}
}

// messageHandler handles incoming messages from the protocol. It is called from the underlying
// protocol and is blocking.
func (c *Client) messageHandler(msg protocol.Message) error {
	errorChan := make(chan error, 1)

	select {
	case <-c.clientDoneChan:
		return protocol.ProtocolShuttingDownError
	case c.handleMessageChan <- clientHandleMessage{message: msg, errorChan: errorChan}:
		return c.waitForErrorChan(errorChan)
	}
}

// messageHandlerLoop is responsible for handling messages from the protocol connection.
func (c *Client) messageHandlerLoop() {
	defer func() {
		close(c.messageHandlerDoneChan)

		select {
		case <-c.clientDoneChan:
		case c.immediateStopChan <- struct{}{}:
		}
	}()

	for {
		select {
		case <-c.clientDoneChan:
			return

		case msg := <-c.handleMessageChan:
			msg.errorChan <- func() error {
				switch msg.message.Type() {
				case MessageTypeAwaitReply:
					return c.handleAwaitReply()
				case MessageTypeRollForward:
					return c.handleRollForward(msg.message)
				case MessageTypeRollBackward:
					return c.handleRollBackward(msg.message)
				case MessageTypeIntersectFound:
					return c.handleIntersectFound(msg.message)
				case MessageTypeIntersectNotFound:
					return c.handleIntersectNotFound(msg.message)
				default:
					return fmt.Errorf(
						"%s: received unexpected message type %d",
						ProtocolName,
						msg.message.Type(),
					)
				}
			}()
		}
	}
}

func (c *Client) sendCurrentTip(tip Tip) {
	for {
		select {
		case ch := <-c.wantCurrentTipChan:
			fmt.Printf("sendCurrentTip: %v\n", tip)
			ch <- tip
		default:
			return
		}
	}
}

func (c *Client) sendReadyForNextBlock(ready bool) error {
	select {
	case <-c.clientDoneChan:
		return protocol.ProtocolShuttingDownError
	case c.readyForNextBlockChan <- ready:
		return nil
	}
}

// wantFirstBlock returns a channel that will receive the first block after the current tip, and a
// function that can be used to clear the channel if sending the request message fails.
func (c *Client) wantFirstBlock() (<-chan clientPointResult, func()) {
	ch := make(chan clientPointResult, 1)

	select {
	case <-c.clientDoneChan:
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
	case <-c.clientDoneChan:
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

// wantRollback returns a channel that will receive the result of the next rollback request, and a
// function that can be used to clear the channel if sending the request message fails.
func (c *Client) wantRollback() (<-chan Tip, func()) {
	ch := make(chan Tip, 1)

	select {
	case <-c.clientDoneChan:
		return nil, func() {}
	case c.wantRollbackChan <- ch:
		return ch, func() {
			select {
			case <-c.wantRollbackChan:
			default:
			}
		}
	}
}

func (c *Client) waitForErrorChan(ch <-chan error) error {
	select {
	case <-c.clientDoneChan:
		return protocol.ProtocolShuttingDownError
	case err := <-ch:
		return err
	}
}

func (c *Client) requestFindIntersect(intersectPoints []common.Point) clientPointResult {
	resultChan, cancel := c.wantIntersectFound()
	if resultChan == nil {
		return clientPointResult{error: protocol.ProtocolShuttingDownError}
	}

	msg := NewMsgFindIntersect(intersectPoints)
	if err := c.SendMessage(msg); err != nil {
		fmt.Printf("requestFindIntersect: error sending message: %v\n", err)
		cancel()
		return clientPointResult{error: err}
	}

	select {
	case <-c.clientDoneChan:
		return clientPointResult{error: protocol.ProtocolShuttingDownError}
	case result := <-resultChan:
		fmt.Printf("requestFindIntersect: received intersect: %+v --- %+v --- %+v --- %v\n", intersectPoints, result.tip, result.point, result.error)
		return result
	}
}

func (c *Client) requestGetAvailableBlockRange(
	intersectPoints []common.Point,
) clientGetAvailableBlockRangeResult {
	fmt.Printf("requestGetAvailableBlockRange: waiting for intersect result for: %+v\n", intersectPoints)

	result := c.requestFindIntersect(intersectPoints)
	if result.error != nil {
		return clientGetAvailableBlockRangeResult{error: result.error}
	}
	start := result.point
	end := result.tip.Point

	// If we're already at the chain tip, return an empty range
	if start.Slot >= end.Slot {
		return clientGetAvailableBlockRangeResult{}
	}

	fmt.Printf("requestGetAvailableBlockRange: start=%v, end=%v\n", start, end)

	// Request the next block to get the first block after the intersect point. This should result
	// in a rollback.
	//
	// TODO: Verify that the rollback always happends, if not review the code here.
	rollbackChan, cancelRollback := c.wantRollback()
	if rollbackChan == nil {
		return clientGetAvailableBlockRangeResult{error: protocol.ProtocolShuttingDownError}
	}
	firstBlockChan, cancelFirstBlock := c.wantFirstBlock()
	if firstBlockChan == nil {
		return clientGetAvailableBlockRangeResult{error: protocol.ProtocolShuttingDownError}
	}
	defer func() {
		if rollbackChan != nil {
			cancelRollback()
		}
		if firstBlockChan != nil {
			cancelFirstBlock()
		}
	}()

	// TODO: Recommended behavior on error should be to send an empty range.

	fmt.Printf("requestGetAvailableBlockRange: requesting next block\n")

	msgRequestNext := NewMsgRequestNext()
	if err := c.SendMessage(msgRequestNext); err != nil {
		return clientGetAvailableBlockRangeResult{start: start, end: end, error: err}
	}

	fmt.Printf("requestGetAvailableBlockRange: waiting for rollback\n")

	for {
		select {
		case <-c.clientDoneChan:
			return clientGetAvailableBlockRangeResult{start: start, end: end, error: protocol.ProtocolShuttingDownError}
		case tip := <-rollbackChan:
			rollbackChan = nil
			end = tip.Point

			fmt.Printf("requestGetAvailableBlockRange: rollback received: %v\n", tip)

		case firstBlock := <-firstBlockChan:
			firstBlockChan = nil

			fmt.Printf("requestGetAvailableBlockRange: first block received: %v\n", firstBlock)

			if firstBlock.error != nil {
				return clientGetAvailableBlockRangeResult{
					start: start,
					end:   end,
					error: fmt.Errorf("failed to get first block: %w", firstBlock.error),
				}
			}
			start = firstBlock.point
		case <-c.readyForNextBlockChan:
			// TODO: This doesn't check for true/false, verify if it should?

			fmt.Printf("requestGetAvailableBlockRange: ready for next block received\n")

			// Request the next block
			msg := NewMsgRequestNext()
			if err := c.SendMessage(msg); err != nil {
				return clientGetAvailableBlockRangeResult{start: start, end: end, error: err}
			}
		}
		if firstBlockChan == nil && rollbackChan == nil {
			break
		}
	}

	fmt.Println("GetAvailableBlockRange: done")

	// If we're already at the chain tip, return an empty range
	if start.Slot >= end.Slot {
		return clientGetAvailableBlockRangeResult{}
	}

	return clientGetAvailableBlockRangeResult{start: start, end: end}
}

func (c *Client) requestSync(intersectPoints []common.Point) error {
	// TODO: Check if we're already syncing, if so return an error or cancel the current sync
	// operation. Use a channel for this.

	// Use origin if no intersect points were specified
	if len(intersectPoints) == 0 {
		intersectPoints = []common.Point{common.NewPointOrigin()}
	}

	fmt.Printf("Sync: intersectPoints=%v\n", func() string {
		var s string
		for _, p := range intersectPoints {
			s += fmt.Sprintf("%v ", p.Slot)
		}
		return s
	}())

	intersectResultChan, cancel := c.wantIntersectFound()
	if intersectResultChan == nil {
		return protocol.ProtocolShuttingDownError
	}

	msg := NewMsgFindIntersect(intersectPoints)
	if err := c.SendMessage(msg); err != nil {
		cancel()
		return err
	}

	select {
	case <-c.clientDoneChan:
		return protocol.ProtocolShuttingDownError
	case result := <-intersectResultChan:
		if result.error != nil {
			return result.error
		}
	}

	fmt.Println("Sync: starting sync loop")

	return nil
}

func (c *Client) handleAwaitReply() error {
	return nil
}

func (c *Client) handleRollForward(msgGeneric protocol.Message) error {
	firstBlockChan := func() chan<- clientPointResult {
		select {
		case ch := <-c.wantFirstBlockChan:
			return ch
		default:
			return nil
		}
	}()
	if firstBlockChan == nil && (c.config == nil || c.config.RollForwardFunc == nil) {
		return fmt.Errorf(
			"received chain-sync RollForward message but no callback function is defined",
		)
	}

	var callbackErr error
	if c.Mode() == protocol.ProtocolModeNodeToNode {
		msg := msgGeneric.(*MsgRollForwardNtN)
		c.sendCurrentTip(msg.Tip)

		var blockHeader ledger.BlockHeader
		var blockType uint
		blockEra := msg.WrappedHeader.Era

		switch blockEra {
		case ledger.BlockHeaderTypeByron:
			blockType = msg.WrappedHeader.ByronType()
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
		default:
			// Map block header type to block type
			blockType = ledger.BlockHeaderToBlockTypeMap[blockEra]
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
		callbackErr = c.config.RollForwardFunc(c.callbackContext, blockType, blockHeader, msg.Tip)
	} else {
		msg := msgGeneric.(*MsgRollForwardNtC)
		c.sendCurrentTip(msg.Tip)

		blk, err := ledger.NewBlockFromCbor(msg.BlockType(), msg.BlockCbor())
		if err != nil {
			if firstBlockChan != nil {
				firstBlockChan <- clientPointResult{error: err}
			}
			return err
		}

		if firstBlockChan != nil {
			blockHash, err := hex.DecodeString(blk.Hash())
			if err != nil {
				firstBlockChan <- clientPointResult{error: err}
				return err
			}
			point := common.NewPoint(blk.SlotNumber(), blockHash)
			firstBlockChan <- clientPointResult{tip: msg.Tip, point: point}
			return nil
		}

		// Call the user callback function
		callbackErr = c.config.RollForwardFunc(c.callbackContext, msg.BlockType(), blk, msg.Tip)
	}
	if callbackErr != nil {
		if callbackErr == StopSyncProcessError {
			// Signal that we're cancelling the sync
			return c.sendReadyForNextBlock(false)
		} else {
			return callbackErr
		}
	}

	// Signal that we're ready for the next block
	return c.sendReadyForNextBlock(true)
}

func (c *Client) handleRollBackward(msg protocol.Message) error {
	msgRollBackward := msg.(*MsgRollBackward)

	fmt.Printf("handleRolling back to %v\n", msgRollBackward.Point)

	c.sendCurrentTip(msgRollBackward.Tip)

	select {
	case ch := <-c.wantRollbackChan:
		ch <- msgRollBackward.Tip
	default:
	}

	if len(c.wantFirstBlockChan) == 0 {
		if c.config.RollBackwardFunc == nil {
			return fmt.Errorf(
				"received chain-sync RollBackward message but no callback function is defined",
			)
		}
		// Call the user callback function
		if callbackErr := c.config.RollBackwardFunc(c.callbackContext, msgRollBackward.Point, msgRollBackward.Tip); callbackErr != nil {
			if callbackErr == StopSyncProcessError {
				// Signal that we're cancelling the sync
				return c.sendReadyForNextBlock(false)
			} else {
				return callbackErr
			}
		}
	} else {
		fmt.Printf("handleRolling firstBlockchan\n")
	}

	return c.sendReadyForNextBlock(true)
}

func (c *Client) handleIntersectFound(msg protocol.Message) error {
	msgIntersectFound := msg.(*MsgIntersectFound)

	fmt.Printf("handleIntersect found: %v\n", msgIntersectFound.Point)

	c.sendCurrentTip(msgIntersectFound.Tip)

	select {
	case ch := <-c.wantIntersectFoundChan:
		ch <- clientPointResult{tip: msgIntersectFound.Tip, point: msgIntersectFound.Point}
	default:
	}

	return nil
}

func (c *Client) handleIntersectNotFound(msgGeneric protocol.Message) error {
	msgIntersectNotFound := msgGeneric.(*MsgIntersectNotFound)

	fmt.Printf("handleIntersect not found\n")

	c.sendCurrentTip(msgIntersectNotFound.Tip)

	select {
	case ch := <-c.wantIntersectFoundChan:
		ch <- clientPointResult{tip: msgIntersectNotFound.Tip, error: IntersectNotFoundError}
	default:
	}

	return nil
}
