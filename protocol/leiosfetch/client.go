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
	blockSlot            requestSlot
	blockTxsSlot         requestSlot
	votesResultChan      chan protocol.Message
	blockRangeResultChan chan protocol.Message
}

// requestSlot serializes a single outstanding request/response exchange for a
// ping-pong leios-fetch state (Block or BlockTxs) and correlates the response
// with the caller that is actually waiting for it.
//
// These mini-protocol states carry no request identifier and the underlying
// protocol is strictly serialized (a new request message is not sent until the
// previous response returns the state to Idle). Correlation is therefore
// enforced structurally:
//   - a request registers its own capacity-1 delivery channel BEFORE the
//     request message is sent, so a response that arrives before the caller
//     parks cannot be dropped; and
//   - a new request blocks until any previously abandoned response has been
//     drained, so a late response to an abandoned request is never
//     mis-delivered to a subsequent request.
type requestSlot struct {
	mu        sync.Mutex
	waiter    chan protocol.Message
	busy      bool
	drainedCh chan struct{}
}

// acquire waits until the slot is free (any previously abandoned response has
// been drained), bounded by ctx and the protocol done channel, then registers
// and returns a fresh capacity-1 delivery channel for this request.
func (s *requestSlot) acquire(
	ctx context.Context,
	done <-chan struct{},
) (chan protocol.Message, error) {
	s.mu.Lock()
	for s.busy {
		drained := s.drainedCh
		s.mu.Unlock()
		select {
		case <-drained:
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-done:
			return nil, protocol.ErrProtocolShuttingDown
		}
		s.mu.Lock()
	}
	w := make(chan protocol.Message, 1)
	s.waiter = w
	s.busy = true
	s.drainedCh = make(chan struct{})
	s.mu.Unlock()
	return w, nil
}

// abandon clears the live waiter so that a late response is dropped by deliver
// rather than mis-delivered, while leaving the slot busy until that response
// arrives (or the protocol shuts down). It is used when a caller's context
// expires before a response is received.
//
// The caller passes the delivery channel it registered in acquire. The slot is
// only mutated while that channel is still the live waiter (s.waiter == w). If
// the response already arrived (deliver cleared and freed the slot) and a
// different request has since acquired the slot, s.waiter now belongs to that
// newer request, so this is a no-op and the newer waiter is left intact.
func (s *requestSlot) abandon(w chan protocol.Message) {
	s.mu.Lock()
	if s.waiter == w {
		s.waiter = nil
	}
	s.mu.Unlock()
}

// release frees the slot without a response having been received, waking any
// goroutine waiting in acquire. It is used when the request message could not
// be sent, or the protocol is shutting down, so no response will ever arrive
// to drain the slot.
//
// As with abandon, the caller passes the delivery channel it registered in
// acquire and the slot is only cleared and freed while that channel is still
// the live waiter (s.waiter == w). If the slot was already freed and reacquired
// by a newer request, this is a no-op so the newer request's registration is
// not disturbed.
func (s *requestSlot) release(w chan protocol.Message) {
	s.mu.Lock()
	if s.waiter == w {
		s.waiter = nil
		s.freeLocked()
	}
	s.mu.Unlock()
}

// deliver routes a received response to the waiting caller, if any, and frees
// the slot. It returns true when a caller received the response and false when
// the response was dropped (no caller waiting, e.g. after abandonment).
func (s *requestSlot) deliver(msg protocol.Message) bool {
	s.mu.Lock()
	w := s.waiter
	s.waiter = nil
	s.freeLocked()
	s.mu.Unlock()
	if w == nil {
		return false
	}
	// The channel has capacity 1 and receives at most one response per
	// request (the state machine rejects a second server message before it
	// reaches this handler), so this send never blocks the receive loop.
	w <- msg
	return true
}

// freeLocked marks the slot as no longer busy and wakes any goroutine waiting
// in acquire. The caller must hold s.mu.
func (s *requestSlot) freeLocked() {
	if !s.busy {
		return
	}
	s.busy = false
	if s.drainedCh != nil {
		close(s.drainedCh)
		s.drainedCh = nil
	}
}

func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:               cfg,
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
		// Start goroutine to cleanup resources on protocol shutdown.
		// The Block/BlockTxs request slots are unblocked directly by the
		// per-request select watching DoneChan (see BlockRequest /
		// BlockTxsRequest), so only the shared Votes/BlockRange channels are
		// closed here.
		go func() {
			<-c.DoneChan()
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
// the receive path (see handleBlock/handleNoBlock), and a subsequent request
// waits for that late response to drain so it can never be mis-delivered.
func (c *Client) BlockRequest(
	ctx context.Context,
	point pcommon.Point,
) (protocol.Message, error) {
	w, err := c.blockSlot.acquire(ctx, c.DoneChan())
	if err != nil {
		return nil, err
	}
	msg := NewMsgBlockRequest(point)
	if err := c.SendMessageContext(ctx, msg); err != nil {
		c.blockSlot.release(w)
		return nil, err
	}
	select {
	case resp := <-w:
		// The server reported the endorser block as not available
		if _, ok := resp.(*MsgNoBlock); ok {
			return nil, ErrBlockNotFound
		}
		return resp, nil
	case <-ctx.Done():
		c.blockSlot.abandon(w)
		return nil, ctx.Err()
	case <-c.DoneChan():
		c.blockSlot.release(w)
		return nil, protocol.ErrProtocolShuttingDown
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
	w, err := c.blockTxsSlot.acquire(ctx, c.DoneChan())
	if err != nil {
		return nil, err
	}
	msg := NewMsgBlockTxsRequest(point, bitmaps)
	if err := c.SendMessageContext(ctx, msg); err != nil {
		c.blockTxsSlot.release(w)
		return nil, err
	}
	select {
	case resp := <-w:
		// The server reported the endorser block transactions as not available
		if _, ok := resp.(*MsgNoBlockTxs); ok {
			return nil, ErrBlockTxsNotFound
		}
		return resp, nil
	case <-ctx.Done():
		c.blockTxsSlot.abandon(w)
		return nil, ctx.Err()
	case <-c.DoneChan():
		c.blockTxsSlot.release(w)
		return nil, protocol.ErrProtocolShuttingDown
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

// logDroppedResponse records that a response was dropped because no caller was
// waiting for it (for example, because the request was abandoned after its
// context expired). The mini-protocol state transition back to StateIdle is
// driven independently by protocol.handleMessage before this handler runs, so
// dropping the message here never leaves the mini-protocol wedged.
func (c *Client) logDroppedResponse(msg protocol.Message) {
	c.Protocol.Logger().
		Debug("dropping unawaited leios-fetch response",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
			"message_type", msg.Type(),
		)
}

func (c *Client) handleBlock(msg protocol.Message) {
	if !c.blockSlot.deliver(msg) {
		c.logDroppedResponse(msg)
	}
}

func (c *Client) handleNoBlock(msg protocol.Message) {
	c.Protocol.Logger().
		Debug("endorser block not available",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	if !c.blockSlot.deliver(msg) {
		c.logDroppedResponse(msg)
	}
}

func (c *Client) handleBlockTxs(msg protocol.Message) {
	if !c.blockTxsSlot.deliver(msg) {
		c.logDroppedResponse(msg)
	}
}

func (c *Client) handleNoBlockTxs(msg protocol.Message) {
	c.Protocol.Logger().
		Debug("endorser block transactions not available",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	if !c.blockTxsSlot.deliver(msg) {
		c.logDroppedResponse(msg)
	}
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
