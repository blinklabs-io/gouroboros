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

package blockfetch

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

// ErrNoBlocks is the error reported for a range the peer answered with
// MsgNoBlocks: through RangeDoneFunc for a request made with RequestRange,
// and as the return value of GetBlock. Consumers match on it with errors.Is.
// The message is unchanged because it was previously the only thing to match
// on.
var ErrNoBlocks = errors.New("block(s) not found")

// ErrRequestPipeliningDisabled is returned by RequestRange when the client
// was not configured with RequestPipelining.
var ErrRequestPipeliningDisabled = errors.New(
	"block-fetch request pipelining is not enabled on this client",
)

// requestDelivery selects how a queued range request delivers its blocks.
type requestDelivery uint8

const (
	// deliveryCallback delivers blocks through the configured block
	// callbacks or the block processing pipeline (GetBlockRange,
	// RequestRange).
	deliveryCallback requestDelivery = iota
	// deliveryChannel delivers a single decoded block through the request's
	// own channel (GetBlock).
	deliveryChannel
)

// rangeRequest tracks one MsgRequestRange that has been sent and not yet
// resolved. Block-fetch responses are ordered, so an entry's position in the
// client's FIFO queue identifies the response it will receive; the head entry
// owns the next MsgStartBatch, MsgNoBlocks, MsgBlock, and MsgBatchDone.
type rangeRequest struct {
	id             uint64
	protocol       *protocol.Protocol
	delivery       requestDelivery
	pipelined      bool
	reservedBytes  uint64
	busyToken      uint64
	hasBusyToken   bool
	started        bool
	blockDelivered bool
	// startChan carries the MsgStartBatch outcome to a synchronous caller.
	startChan chan error
	// blockChan carries the single block of a deliveryChannel request.
	blockChan chan ledger.Block
	// doneChan carries the terminal outcome of the request.
	doneChan chan error
	// resolveOnce ensures a request is resolved exactly once, whichever of
	// the response path and the shutdown path reaches it first.
	resolveOnce sync.Once
}

// RangeRequest describes a single block range request queued through
// Client.RequestRange.
type RangeRequest struct {
	Start pcommon.Point // Start point of the range (inclusive)
	End   pcommon.Point // End point of the range (inclusive)
	// ExpectedBytes is the caller's estimate of the total serialized size of
	// the blocks in this range, used for the client's in-flight byte bound.
	// A caller driving block-fetch from chain-sync headers has this from the
	// header block body sizes. Zero means DefaultRequestExpectedBytes.
	ExpectedBytes uint64
}

// Client implements the Block Fetch protocol client, which requests blocks from a server.
type Client struct {
	*protocol.Protocol
	protocolMu      sync.RWMutex
	config          *Config         // Protocol configuration
	callbackContext CallbackContext // Callback context for client
	busyMutex       sync.Mutex      // Mutex for busy state
	busyStateMutex  sync.Mutex      // Protects busy lock ownership
	busyLocked      bool
	busyToken       uint64
	busyDoneChan    chan struct{}
	lifecycleMutex  sync.Mutex // Mutex for lifecycle state
	lifecycleState  clientLifecycleState
	startingDone    chan struct{}
	protoOptions    protocol.ProtocolOptions
	protoStarted    bool // Whether Protocol.Start() was called
	// queueMutex protects the outstanding request queue and the in-flight
	// byte accounting.
	queueMutex    sync.Mutex
	queue         []*rangeRequest
	nextRequestId uint64
	inFlightBytes uint64
	// bytesFreed is closed and replaced every time in-flight bytes are
	// released, to broadcast to callers waiting for admission.
	bytesFreed chan struct{}
	// rangeSendToken serializes queue append and MsgRequestRange enqueue so
	// FIFO queue order always matches wire order. Unlike a mutex, acquiring
	// the token can be canceled by RequestRange's context.
	rangeSendToken chan struct{}
	// beforeRequestReservation is a test synchronization point immediately
	// before the atomic admission and reservation step.
	beforeRequestReservation func()
	// beforeInFlightWait is a test synchronization point immediately before a
	// pipelined caller parks on the in-flight byte bound. The park is
	// otherwise unobservable, and the ordering it establishes is what proves
	// a parked caller holds nothing another caller needs.
	beforeInFlightWait func()
}

// NewClient creates a new Block Fetch protocol client with the given options and configuration.
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg, _ := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:         cfg,
		protoOptions:   protoOptions,
		lifecycleState: clientStateNew,
		bytesFreed:     make(chan struct{}),
		rangeSendToken: make(chan struct{}, 1),
	}
	c.rangeSendToken <- struct{}{}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	c.initProtocol()
	return c
}

func (c *Client) initProtocol() {
	c.protoStarted = false

	// Update state map with timeouts
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[StateBusy]; ok {
		entry.Timeout = c.config.BatchStartTimeout
		if c.config.RequestPipelining {
			// The client may send the next MsgRequestRange while the peer is
			// still answering the previous one.
			entry.AllowPipelinedSend = true
		}
		stateMap[StateBusy] = entry
	}
	if entry, ok := stateMap[StateStreaming]; ok {
		entry.Timeout = c.config.BlockTimeout
		if c.config.RequestPipelining {
			entry.AllowPipelinedSend = true
		}
		stateMap[StateStreaming] = entry
	}
	if c.config.RequestPipelining {
		if entry, ok := stateMap[StateIdle]; ok {
			entry.PendingMessageByteLimit = PipelinedIdleMaxPendingMessageBytes
			stateMap[StateIdle] = entry
		}
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
	p := protocol.New(protoConfig)
	c.protocolMu.Lock()
	c.Protocol = p
	c.protocolMu.Unlock()
}

func (c *Client) ProtocolInstance() *protocol.Protocol {
	c.protocolMu.RLock()
	defer c.protocolMu.RUnlock()
	return c.Protocol
}

func (c *Client) acquireBusy() (uint64, <-chan struct{}) {
	c.busyMutex.Lock()

	c.busyStateMutex.Lock()
	defer c.busyStateMutex.Unlock()
	c.busyToken++
	c.busyLocked = true
	c.busyDoneChan = make(chan struct{})
	return c.busyToken, c.busyDoneChan
}

func (c *Client) releaseBusy(token uint64) {
	var doneChan chan struct{}
	c.busyStateMutex.Lock()
	if c.busyLocked && c.busyToken == token {
		c.busyLocked = false
		doneChan = c.busyDoneChan
		c.busyDoneChan = nil
	}
	c.busyStateMutex.Unlock()

	if doneChan != nil {
		close(doneChan)
		c.busyMutex.Unlock()
	}
}

func (c *Client) releaseCurrentBusy() {
	var doneChan chan struct{}
	c.busyStateMutex.Lock()
	if c.busyLocked {
		c.busyLocked = false
		doneChan = c.busyDoneChan
		c.busyDoneChan = nil
	}
	c.busyStateMutex.Unlock()

	if doneChan != nil {
		close(doneChan)
		c.busyMutex.Unlock()
	}
}

func (c *Client) releaseBusyOnProtocolDone(
	token uint64,
	busyDone <-chan struct{},
	protocolDone <-chan struct{},
) {
	select {
	case <-protocolDone:
		c.releaseBusy(token)
	case <-busyDone:
	}
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
			proto := c.ProtocolInstance()
			proto.Start()
			c.protoStarted = true
			c.lifecycleState = clientStateRunning
			// Resolve any request left outstanding when the protocol shuts
			// down, so no caller and no callback consumer is left waiting.
			go c.failOutstandingOnProtocolDone(proto)
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
	c.lifecycleMutex.Lock()
	defer c.lifecycleMutex.Unlock()

	switch c.lifecycleState {
	case clientStateNew, clientStateStopped:
		c.failOutstanding(protocol.ErrProtocolShuttingDown)
		c.releaseCurrentBusy()
		return nil
	case clientStateStarting:
		// Mark as stopped so Start() will abort when it re-checks state
		c.lifecycleState = clientStateStopped
		// Unblock Start() if it's waiting
		if c.startingDone != nil {
			close(c.startingDone)
			c.startingDone = nil
		}
		c.failOutstanding(protocol.ErrProtocolShuttingDown)
		c.releaseCurrentBusy()
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
		if errors.Is(sendErr, protocol.ErrProtocolShuttingDown) {
			sendErr = nil
		}
		_ = c.WaitSendQueueDrained(250 * time.Millisecond)
	}

	// Stop/unregister the underlying protocol instance first, then wait for
	// message handlers to finish before resolving outstanding requests.
	doneChan := c.DoneChan()
	c.Protocol.Stop()
	c.lifecycleState = clientStateStopped

	// Release lock while waiting for protocol shutdown to avoid deadlock.
	c.lifecycleMutex.Unlock()
	<-doneChan
	// Every queued request is resolved with a terminal error. Each request
	// owns its own buffered channels, so a handler that is still running
	// cannot send on a closed channel and no waiter is stranded.
	c.failOutstanding(protocol.ErrProtocolShuttingDown)
	c.releaseCurrentBusy()

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
//
// Only one GetBlockRange or GetBlock call is in progress at a time; a second
// call blocks until the first batch completes. Use RequestRange to keep
// multiple requests outstanding.
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
	token, busyDone := c.acquireBusy()
	protocolDone := c.DoneChan()
	req, err := c.sendRequestRange(
		context.Background(),
		start,
		end,
		deliveryCallback,
		false,
		0,
		token,
	)
	if err != nil {
		c.releaseBusy(token)
		return err
	}
	// Wait for batch start
	if err := c.waitForBatchStart(req, protocolDone); err != nil {
		c.releaseBusy(token)
		return err
	}
	go c.releaseBusyOnProtocolDone(token, busyDone, protocolDone)
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
	token, _ := c.acquireBusy()
	protocolDone := c.DoneChan()
	req, err := c.sendRequestRange(
		context.Background(),
		point,
		point,
		deliveryChannel,
		false,
		0,
		token,
	)
	if err != nil {
		c.releaseBusy(token)
		return nil, err
	}
	// Wait for batch start
	if err := c.waitForBatchStart(req, protocolDone); err != nil {
		c.releaseBusy(token)
		return nil, err
	}
	// Wait for the block. Both the block and the completion can already be
	// buffered by the time we get here, and a select over two ready cases
	// picks at random, so a completed batch is only believed after checking
	// for a delivered block.
	var block ledger.Block
	var batchDone error
	var batchIsDone bool
	select {
	case b := <-req.blockChan:
		block = b
	case err := <-req.doneChan:
		batchDone, batchIsDone = err, true
		select {
		case b := <-req.blockChan:
			block = b
		default:
		}
	case <-protocolDone:
		c.releaseBusy(token)
		return nil, protocol.ErrProtocolShuttingDown
	}
	if batchIsDone {
		c.releaseBusy(token)
		if batchDone != nil {
			return nil, batchDone
		}
		if block == nil {
			// The peer completed the batch without sending a block
			return nil, ErrNoBlocks
		}
		return block, nil
	}
	// Wait for BatchDone before returning to ensure the protocol state machine
	// completes the batch properly (transitions back to Idle state).
	select {
	case err := <-req.doneChan:
		c.releaseBusy(token)
		if err != nil {
			return nil, err
		}
		return block, nil
	case <-protocolDone:
		// Shutdown while waiting for BatchDone
		c.releaseBusy(token)
		return nil, protocol.ErrProtocolShuttingDown
	}
}

// RequestRange queues a request for the given block range without waiting for
// previously queued requests to complete, so the peer always has work in hand
// at a batch boundary. It returns the request ID, which the client reports in
// CallbackContext.RequestId for every block of the range and in the
// RangeDoneFunc call that completes it. Blocks are delivered through the same
// callbacks GetBlockRange uses.
//
// It blocks while the expected size of the outstanding requests would exceed
// the configured MaxInFlightBytes, and returns the context's error if the
// caller gives up first. A request larger than the whole bound is admitted
// once the queue is empty, so an oversized range cannot stall forever.
//
// The client must be configured with RequestPipelining and a RangeDoneFunc.
func (c *Client) RequestRange(
	ctx context.Context,
	req RangeRequest,
) (uint64, error) {
	if !c.config.RequestPipelining {
		return 0, ErrRequestPipeliningDisabled
	}
	if c.config.RangeDoneFunc == nil {
		return 0, errors.New(
			"block-fetch RequestRange requires a RangeDoneFunc callback",
		)
	}
	c.Protocol.Logger().
		Debug(
			fmt.Sprintf("calling RequestRange(start: {Slot: %d, Hash: %x}, end: {Slot: %d, Hash: %x})",
				req.Start.Slot,
				req.Start.Hash,
				req.End.Slot,
				req.End.Hash,
			),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	expectedBytes := req.ExpectedBytes
	if expectedBytes == 0 {
		expectedBytes = DefaultRequestExpectedBytes
	}
	if c.beforeRequestReservation != nil {
		c.beforeRequestReservation()
	}
	sent, err := c.sendRequestRange(
		ctx,
		req.Start,
		req.End,
		deliveryCallback,
		true,
		expectedBytes,
		0,
	)
	if err != nil {
		return 0, err
	}
	return sent.id, nil
}

// maxInFlightBytes returns the configured in-flight byte bound, treating an
// unset value as the default. Config is built directly by some consumers, so
// the default cannot only be applied in NewConfig.
func (c *Client) maxInFlightBytes() uint64 {
	if c.config.MaxInFlightBytes == 0 {
		return DefaultMaxInFlightBytes
	}
	return c.config.MaxInFlightBytes
}

// hasInFlightCapacityLocked reports whether a pipelined request can reserve
// expectedBytes. Subtraction avoids wrapping at the top of the uint64 range.
// The caller must hold queueMutex.
func (c *Client) hasInFlightCapacityLocked(expectedBytes uint64) bool {
	if len(c.queue) == 0 {
		// An empty queue always admits, so a range larger than the whole
		// bound still makes progress instead of deadlocking.
		return true
	}
	limit := c.maxInFlightBytes()
	return c.inFlightBytes <= limit &&
		expectedBytes <= limit-c.inFlightBytes
}

// waitForInFlightCapacity blocks until the in-flight byte budget can admit
// expectedBytes. It is called without rangeSendToken held, so a caller parked
// on the byte bound never delays another caller's queue append and send.
func (c *Client) waitForInFlightCapacity(
	ctx context.Context,
	protocolDone <-chan struct{},
	expectedBytes uint64,
) error {
	for {
		c.queueMutex.Lock()
		if c.hasInFlightCapacityLocked(expectedBytes) {
			c.queueMutex.Unlock()
			return nil
		}
		waitChan := c.bytesFreed
		c.queueMutex.Unlock()
		if c.beforeInFlightWait != nil {
			c.beforeInFlightWait()
		}
		select {
		case <-waitChan:
		case <-ctx.Done():
			return ctx.Err()
		case <-protocolDone:
			return protocol.ErrProtocolShuttingDown
		}
	}
}

// sendRequestRange appends a request to the outstanding queue and sends its
// MsgRequestRange. A context-aware send token covers only the queue append and
// the MsgRequestRange enqueue, so FIFO queue order always matches wire order
// without holding queueMutex while a full protocol send queue blocks. Waiting
// for in-flight capacity happens outside the token; the reservation is
// rechecked under it, and the token is handed back before parking again.
func (c *Client) sendRequestRange(
	ctx context.Context,
	start pcommon.Point,
	end pcommon.Point,
	delivery requestDelivery,
	pipelined bool,
	expectedBytes uint64,
	busyToken uint64,
) (*rangeRequest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	proto := c.ProtocolInstance()
	protocolDone := proto.DoneChan()
	var req *rangeRequest
	for {
		if pipelined {
			if err := c.waitForInFlightCapacity(
				ctx,
				protocolDone,
				expectedBytes,
			); err != nil {
				return nil, err
			}
		}
		select {
		case <-c.rangeSendToken:
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-protocolDone:
			return nil, protocol.ErrProtocolShuttingDown
		}
		c.queueMutex.Lock()
		if pipelined && !c.hasInFlightCapacityLocked(expectedBytes) {
			// Another caller took the capacity between the wait and the
			// token. Hand the token back before parking again so no other
			// caller is held behind this one.
			c.queueMutex.Unlock()
			c.rangeSendToken <- struct{}{}
			continue
		}
		c.nextRequestId++
		req = &rangeRequest{
			id:            c.nextRequestId,
			protocol:      proto,
			delivery:      delivery,
			pipelined:     pipelined,
			reservedBytes: expectedBytes,
			busyToken:     busyToken,
			hasBusyToken:  busyToken != 0,
			startChan:     make(chan error, 1),
			blockChan:     make(chan ledger.Block, 1),
			doneChan:      make(chan error, 1),
		}
		c.queue = append(c.queue, req)
		c.inFlightBytes += expectedBytes
		c.queueMutex.Unlock()
		break
	}
	// The token is held from the queue append through the MsgRequestRange
	// enqueue, which is what keeps queue order equal to wire order.
	defer func() {
		c.rangeSendToken <- struct{}{}
	}()
	err := proto.SendMessageContext(ctx, NewMsgRequestRange(start, end))
	if err != nil {
		// The request never reached the wire, so it can be removed without
		// disturbing the position of any other entry.
		c.queueMutex.Lock()
		c.removeLocked(req)
		c.queueMutex.Unlock()
		return nil, err
	}
	return req, nil
}

// waitForBatchStart waits for the peer to accept a request, for the
// synchronous GetBlockRange and GetBlock paths.
func (c *Client) waitForBatchStart(
	req *rangeRequest,
	protocolDone <-chan struct{},
) error {
	select {
	case err := <-req.startChan:
		return err
	case <-protocolDone:
		return protocol.ErrProtocolShuttingDown
	}
}

// removeLocked removes a request from the queue and releases its reserved
// bytes. The caller must hold queueMutex.
func (c *Client) removeLocked(req *rangeRequest) {
	for i, queued := range c.queue {
		if queued == req {
			c.queue = slices.Delete(c.queue, i, i+1)
			c.releaseBytesLocked(req.reservedBytes)
			return
		}
	}
}

// releaseBytesLocked returns reserved bytes to the in-flight budget and wakes
// every caller waiting for admission. The caller must hold queueMutex.
func (c *Client) releaseBytesLocked(reserved uint64) {
	if reserved > c.inFlightBytes {
		c.inFlightBytes = 0
	} else {
		c.inFlightBytes -= reserved
	}
	if c.bytesFreed != nil {
		close(c.bytesFreed)
	}
	c.bytesFreed = make(chan struct{})
}

// headForResponse returns the queue entry a response belongs to. Block-fetch
// responses are ordered, so that is always the head entry, and it must be in
// the state the response implies: streaming for MsgBlock and MsgBatchDone,
// not yet started for MsgStartBatch and MsgNoBlocks. Anything else is a peer
// that is talking out of turn, and is reported rather than applied to the
// following entry. The caller must hold queueMutex.
func (c *Client) headForResponse(
	msgName string,
	wantStarted bool,
) (*rangeRequest, error) {
	if len(c.queue) == 0 {
		return nil, fmt.Errorf(
			"%s: received %s with no outstanding range request",
			ProtocolName,
			msgName,
		)
	}
	req := c.queue[0]
	if req.started != wantStarted {
		return nil, fmt.Errorf(
			"%s: received %s for range request %d in the wrong order",
			ProtocolName,
			msgName,
			req.id,
		)
	}
	return req, nil
}

// drainLocked empties the queue and clears the in-flight byte accounting,
// returning the entries the caller must resolve. The caller must hold
// queueMutex.
func (c *Client) drainLocked() []*rangeRequest {
	pending := c.queue
	c.queue = nil
	if len(pending) > 0 {
		c.inFlightBytes = 0
		close(c.bytesFreed)
		c.bytesFreed = make(chan struct{})
	}
	return pending
}

// resolve delivers a request's terminal outcome. It must be called without
// queueMutex held, because it can invoke a user callback.
func (c *Client) resolve(req *rangeRequest, err error) error {
	var callbackErr error
	req.resolveOnce.Do(func() {
		if err != nil {
			// Release a caller that is still waiting for the batch to start.
			select {
			case req.startChan <- err:
			default:
			}
		}
		select {
		case req.doneChan <- err:
		default:
		}
		if req.hasBusyToken {
			c.releaseBusy(req.busyToken)
		}
		if req.pipelined && c.config.RangeDoneFunc != nil {
			callbackErr = c.config.RangeDoneFunc(
				c.callbackContextFor(req),
				err,
			)
		}
	})
	return callbackErr
}

// failOutstanding resolves every queued request with the given error.
func (c *Client) failOutstanding(err error) {
	c.queueMutex.Lock()
	pending := c.drainLocked()
	c.queueMutex.Unlock()
	for _, req := range pending {
		if resolveErr := c.resolve(req, err); resolveErr != nil {
			c.Protocol.Logger().
				Warn("range done callback failed during shutdown",
					"component", "network",
					"protocol", ProtocolName,
					"role", "client",
					"connection_id", c.callbackContext.ConnectionId.String(),
					"error", resolveErr.Error(),
				)
		}
	}
}

// failOutstandingForProtocol resolves only requests owned by proto. A delayed
// shutdown watcher from an earlier client start must not resolve requests
// queued after the client has restarted with a new protocol instance.
func (c *Client) failOutstandingForProtocol(
	proto *protocol.Protocol,
	err error,
) {
	c.queueMutex.Lock()
	pending := make([]*rangeRequest, 0)
	remaining := make([]*rangeRequest, 0, len(c.queue))
	for _, req := range c.queue {
		if req.protocol == proto {
			pending = append(pending, req)
			c.releaseBytesLocked(req.reservedBytes)
			continue
		}
		remaining = append(remaining, req)
	}
	c.queue = remaining
	c.queueMutex.Unlock()
	for _, req := range pending {
		if resolveErr := c.resolve(req, err); resolveErr != nil {
			proto.Logger().
				Warn("range done callback failed during shutdown",
					"component", "network",
					"protocol", ProtocolName,
					"role", "client",
					"connection_id", c.callbackContext.ConnectionId.String(),
					"error", resolveErr.Error(),
				)
		}
	}
}

// failOutstandingOnProtocolDone resolves outstanding requests once the
// protocol shuts down.
func (c *Client) failOutstandingOnProtocolDone(proto *protocol.Protocol) {
	<-proto.DoneChan()
	c.failOutstandingForProtocol(proto, protocol.ErrProtocolShuttingDown)
}

// callbackContextFor returns the callback context for a request, carrying its
// FIFO identity.
func (c *Client) callbackContextFor(req *rangeRequest) CallbackContext {
	ctx := c.callbackContext
	ctx.RequestId = req.id
	return ctx
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
	c.queueMutex.Lock()
	req, err := c.headForResponse("StartBatch", false)
	if err != nil {
		c.queueMutex.Unlock()
		return err
	}
	req.started = true
	c.queueMutex.Unlock()
	select {
	case req.startChan <- nil:
	default:
	}
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
	c.queueMutex.Lock()
	req, err := c.headForResponse("NoBlocks", false)
	if err != nil {
		c.queueMutex.Unlock()
		return err
	}
	c.removeLocked(req)
	c.queueMutex.Unlock()
	return c.resolve(req, ErrNoBlocks)
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
	msg, ok := msgGeneric.(*MsgBlock)
	if !ok {
		return fmt.Errorf("%s: unexpected message type", ProtocolName)
	}
	c.queueMutex.Lock()
	req, err := c.headForResponse("Block", true)
	if err != nil {
		c.queueMutex.Unlock()
		return err
	}
	if req.delivery == deliveryChannel {
		if req.blockDelivered {
			c.queueMutex.Unlock()
			return fmt.Errorf(
				"%s: received more than one block for single-block request %d",
				ProtocolName,
				req.id,
			)
		}
		req.blockDelivered = true
	}
	c.queueMutex.Unlock()
	// Decode only enough to get the block type value
	var wrappedBlock WrappedBlock
	if _, err := cbor.Decode(msg.WrappedBlock, &wrappedBlock); err != nil {
		return c.failRequest(
			req,
			fmt.Errorf("%s: decode error: %w", ProtocolName, err),
		)
	}
	// If pipeline is configured, submit to pipeline
	// Only use pipeline if we are in callback mode (GetBlockRange, RequestRange),
	// preserving GetBlock functionality.
	if c.config.Pipeline != nil && req.delivery == deliveryCallback {
		// Check for shutdown
		select {
		case <-c.DoneChan():
			return c.failRequest(req, protocol.ErrProtocolShuttingDown)
		default:
		}

		tip := pcommon.Tip{} // BlockFetch doesn't provide tip
		// Create a context that cancels when the protocol shuts down.
		// This prevents Submit from blocking indefinitely if the pipeline is
		// full (backpressure) and DoneChan fires before pipeline.Stop().
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			select {
			case <-c.DoneChan():
				cancel()
			case <-ctx.Done():
			}
		}()
		err := c.config.Pipeline.Submit(
			ctx,
			wrappedBlock.Type,
			wrappedBlock.RawBlock,
			tip,
		)
		cancel() // Ensure goroutine exits promptly
		if err != nil {
			return c.failRequest(req, err)
		}
		return nil
	}
	var block ledger.Block
	if req.delivery == deliveryChannel || c.config.BlockFunc != nil {
		var err error
		block, err = ledger.NewBlockFromCbor(
			wrappedBlock.Type,
			wrappedBlock.RawBlock,
			lcommon.VerifyConfig{
				SkipBodyHashValidation: c.config.SkipBlockValidation,
			},
		)
		if err != nil {
			return c.failRequest(req, err)
		}
	}
	// Check for shutdown
	select {
	case <-c.DoneChan():
		return c.failRequest(req, protocol.ErrProtocolShuttingDown)
	default:
	}
	// We use the callbacks when requesting ranges and the request's own
	// channel for a single block
	if req.delivery == deliveryCallback {
		cbCtx := c.callbackContextFor(req)
		switch {
		case c.config.BlockRawFunc != nil:
			if err := c.config.BlockRawFunc(cbCtx, wrappedBlock.Type, wrappedBlock.RawBlock); err != nil {
				return c.failRequest(req, err)
			}
		case c.config.BlockFunc != nil:
			if err := c.config.BlockFunc(cbCtx, wrappedBlock.Type, block); err != nil {
				return c.failRequest(req, err)
			}
		default:
			return c.failRequest(
				req,
				errors.New(
					"received block-fetch Block message but no callback function is defined",
				),
			)
		}
		return nil
	}
	select {
	case req.blockChan <- block:
	default:
	}
	return nil
}

// failRequest retires a request that cannot be completed and returns the
// error so the protocol is torn down, matching the previous behavior of
// releasing the busy lock before returning a handler error.
func (c *Client) failRequest(req *rangeRequest, err error) error {
	c.queueMutex.Lock()
	retired := len(c.queue) > 0 && c.queue[0] == req
	if retired {
		c.removeLocked(req)
	}
	c.queueMutex.Unlock()
	if retired {
		if resolveErr := c.resolve(req, err); resolveErr != nil {
			return errors.Join(err, resolveErr)
		}
	}
	return err
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
	c.queueMutex.Lock()
	req, err := c.headForResponse("BatchDone", true)
	if err != nil {
		c.queueMutex.Unlock()
		return err
	}
	c.removeLocked(req)
	c.queueMutex.Unlock()
	// Notify the user if requested. A pipelined request reports completion
	// through RangeDoneFunc in resolve() instead, so it gets exactly one
	// completion callback.
	if !req.pipelined && req.delivery == deliveryCallback &&
		c.config.BatchDoneFunc != nil {
		if err := c.config.BatchDoneFunc(c.callbackContextFor(req)); err != nil {
			// The request is already retired; still resolve it so no caller
			// is left waiting for a batch that will not be reported.
			if resolveErr := c.resolve(req, err); resolveErr != nil {
				return errors.Join(err, resolveErr)
			}
			return err
		}
	}
	return c.resolve(req, nil)
}
