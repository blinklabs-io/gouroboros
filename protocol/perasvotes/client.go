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

package perasvotes

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
)

type objectIDsResult struct {
	ids []VoteID
	err error
}

type objectsResult struct {
	objects []VoteObject
	err     error
}

// ErrCalledFromVoteFunc is returned by a client method called, while VoteFunc
// runs, with the Context of the CallbackContext the client passed that
// VoteFunc call.
var ErrCalledFromVoteFunc = errors.New(
	"peras votes client called from its own VoteFunc during Sync",
)

// Client runs the inbound/client side of ObjectDiffusion.
type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	onceStart       sync.Once
	// opSem serializes whole request/reply exchanges, so a reply belongs to
	// the one caller waiting for it. It is a channel rather than a mutex so
	// that a caller waiting for it can give up when its context ends.
	opSem         chan struct{}
	objectIDsChan chan objectIDsResult
	objectsChan   chan objectsResult
}

// NewClient creates a client endpoint with the supplied protocol options.
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	cfg = normalizeConfig(cfg)
	c := &Client{
		config:        cfg,
		objectIDsChan: make(chan objectIDsResult, 1),
		objectsChan:   make(chan objectsResult, 1),
		opSem:         make(chan struct{}, 1),
	}
	c.callbackContext = CallbackContext{
		Client: c, ConnectionId: protoOptions.ConnectionId,
		ConnectionDoneChan: protoOptions.ConnectionDoneChan,
	}
	stateMap := clientStateMap(cfg)
	c.Protocol = protocol.New(protocol.ProtocolConfig{
		Name: ProtocolName, ProtocolId: ProtocolId,
		Muxer: protoOptions.Muxer, Logger: protoOptions.Logger,
		ErrorChan: protoOptions.ErrorChan, Mode: protoOptions.Mode,
		Role: protocol.ProtocolRoleClient, MessageHandlerFunc: c.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateContext:        newStateContext(cfg.MaxObjectsUnacknowledged),
		StateMap:            stateMap, InitialState: StateInit,
	})
	return c
}

func clientStateMap(cfg *Config) protocol.StateMap {
	stateMap := StateMap.Copy()
	for _, state := range []protocol.State{
		StateObjectIDsNonBlocking,
		StateObjects,
	} {
		entry := stateMap[state]
		entry.Timeout = cfg.Timeout
		stateMap[state] = entry
	}
	for state, timeout := range map[protocol.State]time.Duration{
		StateObjectIDsBlocking:          CanAwaitTimeout,
		StateObjectIDsBlockingMustReply: MustReplyTimeout,
	} {
		entry := stateMap[state]
		entry.Timeout = timeout
		stateMap[state] = entry
	}
	limit := int(cfg.MaxObjectsUnacknowledged)*1100 + 256
	for state, entry := range stateMap {
		if state != StateDone {
			entry.PendingMessageByteLimit = limit
			stateMap[state] = entry
		}
	}
	return stateMap
}

// Start starts the client and sends the initialization message.
func (c *Client) Start() {
	c.onceStart.Do(func() {
		c.Protocol.Start()
		if err := c.SendMessageAndWait(NewMsgInit()); err != nil {
			c.SendError(err)
		}
	})
}

// RequestObjectIDs acknowledges prior IDs and requests up to requestCount new
// IDs. Blocking requests must request at least one ID. A blocking request
// returns a non-empty reply, or no IDs and no error when the server answered
// MsgAwaitReply and then MsgServerIdle; the acknowledgement was applied either
// way. Non-blocking requests complete promptly and may return none.
func (c *Client) RequestObjectIDs(
	ctx context.Context,
	blocking bool,
	ackCount, requestCount uint16,
) ([]VoteID, error) {
	if ctx == nil {
		return nil, errors.New("context must not be nil")
	}
	if err := c.lockOp(ctx); err != nil {
		return nil, err
	}
	defer c.unlockOp()
	return c.requestObjectIDs(ctx, blocking, ackCount, requestCount)
}

func (c *Client) requestObjectIDs(
	ctx context.Context,
	blocking bool,
	ackCount, requestCount uint16,
) ([]VoteID, error) {
	if blocking && requestCount == 0 {
		return nil, errors.New(
			"blocking Peras object ID request count must be positive",
		)
	}
	if err := c.SendMessageContext(
		ctx,
		NewMsgRequestObjectIDs(blocking, ackCount, requestCount),
	); err != nil {
		return nil, err
	}
	select {
	case result := <-c.objectIDsChan:
		return result.ids, result.err
	case <-ctx.Done():
		_ = c.Protocol.Stop(ctx)
		return nil, ctx.Err()
	case <-c.DoneChan():
		return nil, protocol.ErrProtocolShuttingDown
	case <-c.callbackContext.ConnectionDoneChan:
		return nil, protocol.ErrProtocolShuttingDown
	}
}

// RequestObjects requests payloads for outstanding IDs. A peer may omit an
// object that became unavailable after it announced the ID.
func (c *Client) RequestObjects(
	ctx context.Context,
	ids []VoteID,
) ([]VoteObject, error) {
	if ctx == nil {
		return nil, errors.New("context must not be nil")
	}
	if err := c.lockOp(ctx); err != nil {
		return nil, err
	}
	defer c.unlockOp()
	return c.requestObjects(ctx, ids)
}

func (c *Client) requestObjects(
	ctx context.Context,
	ids []VoteID,
) ([]VoteObject, error) {
	if len(ids) == 0 {
		return nil, errors.New(
			"peras object request must contain at least one ID",
		)
	}
	if err := c.SendMessageContext(ctx, NewMsgRequestObjects(ids)); err != nil {
		return nil, err
	}
	select {
	case result := <-c.objectsChan:
		return result.objects, result.err
	case <-ctx.Done():
		_ = c.Protocol.Stop(ctx)
		return nil, ctx.Err()
	case <-c.DoneChan():
		return nil, protocol.ErrProtocolShuttingDown
	case <-c.callbackContext.ConnectionDoneChan:
		return nil, protocol.ErrProtocolShuttingDown
	}
}

// Sync continuously requests and acknowledges vote batches until cancellation
// or shutdown. A VoteFunc error stops the protocol because the batch is no
// longer safe to acknowledge. VoteFunc runs while Sync holds the request
// exchange: RequestObjectIDs, RequestObjects or Done called from VoteFunc
// with the Context of its CallbackContext returns ErrCalledFromVoteFunc at
// once, and with any other context waits for Sync to end or for that
// context. To end the sync from VoteFunc, return an error. Once VoteFunc
// has returned, its context marks nothing, and a call with it is an
// ordinary call.
func (c *Client) Sync(ctx context.Context) error {
	if c.config.VoteFunc == nil {
		return errors.New("peras vote function must be configured for sync")
	}
	if ctx == nil {
		return errors.New("context must not be nil")
	}
	if err := c.lockOp(ctx); err != nil {
		return err
	}
	defer c.unlockOp()
	var ackCount uint16
	for {
		ids, err := c.requestObjectIDs(
			ctx,
			true,
			ackCount,
			c.config.MaxObjectsUnacknowledged,
		)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			// MsgServerIdle: the request's acknowledgement was applied, and
			// the reference expects the client to ask again at once.
			ackCount = 0
			continue
		}
		objects, err := c.requestObjects(ctx, ids)
		if err != nil {
			return err
		}
		for _, object := range objects {
			id, err := object.VoteID()
			if err != nil {
				_ = c.Protocol.Stop(ctx)
				return err
			}
			if err := c.callVoteFunc(id, object); err != nil {
				_ = c.Protocol.Stop(ctx)
				return fmt.Errorf("process Peras vote %v: %w", id, err)
			}
		}
		// #nosec G115 -- the state machine caps IDs at MaxObjectsUnacknowledged.
		ackCount = uint16(len(ids))
	}
}

// Done sends the graceful termination message.
func (c *Client) Done(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context must not be nil")
	}
	if err := c.lockOp(ctx); err != nil {
		return err
	}
	defer c.unlockOp()
	return c.SendMessageContextAndWait(ctx, NewMsgDone())
}

// voteFuncOwnerKey marks the context the client passes VoteFunc, with the
// *voteFuncCall as its value.
type voteFuncOwnerKey struct{}

// voteFuncCall identifies one VoteFunc call. active is cleared when the
// call returns, so a context VoteFunc keeps marks nothing afterwards.
type voteFuncCall struct {
	client *Client
	active atomic.Bool
}

// callVoteFunc runs VoteFunc with a context that marks this call.
func (c *Client) callVoteFunc(id VoteID, object VoteObject) error {
	call := &voteFuncCall{client: c}
	call.active.Store(true)
	defer call.active.Store(false)
	cbCtx := c.callbackContext.withOwner(c.Protocol)
	cbCtx.ctx = context.WithValue(cbCtx.ctx, voteFuncOwnerKey{}, call)
	return c.config.VoteFunc(cbCtx, id, object)
}

// lockOp takes the request exchange. VoteFunc runs inside Sync, which holds
// the exchange until VoteFunc returns, so a call that carries this client's
// VoteFunc context could never get it: it fails at once instead. The
// reference inbound side issues each request from its one protocol thread,
// so it has no such re-entrant request either.
func (c *Client) lockOp(ctx context.Context) error {
	if call, _ := ctx.Value(voteFuncOwnerKey{}).(*voteFuncCall); call != nil &&
		call.client == c && call.active.Load() {
		return ErrCalledFromVoteFunc
	}
	select {
	case c.opSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) unlockOp() { <-c.opSem }

// Stop interrupts the client protocol.
//
// Stop signals shutdown at once. When ctx is the Context of a
// CallbackContext this client passed to a callback, Stop returns without
// waiting, since the goroutine running the callback is one it would wait
// for. Otherwise it waits until the protocol's goroutines have returned, or
// until ctx is done, in which case it returns ctx.Err() and shutdown still
// proceeds.
func (c *Client) Stop(ctx context.Context) error { return c.Protocol.Stop(ctx) }

func (c *Client) messageHandler(msg protocol.Message) error {
	switch msg.Type() {
	case MessageTypeReplyObjectIDs:
		select {
		case c.objectIDsChan <- objectIDsResult{
			ids: msg.(*MsgReplyObjectIDs).ObjectIDs,
		}:
			return nil
		default:
			return errors.New("unexpected Peras object ID reply")
		}
	case MessageTypeAwaitReply:
		return nil
	case MessageTypeServerIdle:
		select {
		case c.objectIDsChan <- objectIDsResult{}:
			return nil
		default:
			return errors.New("unexpected Peras server idle")
		}
	case MessageTypeReplyObjects:
		select {
		case c.objectsChan <- objectsResult{objects: msg.(*MsgReplyObjects).Objects}:
			return nil
		default:
			return errors.New("unexpected Peras object reply")
		}
	default:
		return fmt.Errorf(
			"%s: client received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
}
