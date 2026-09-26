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

// Client runs the inbound/client side of ObjectDiffusion.
type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	onceStart       sync.Once
	opMu            sync.Mutex
	objectIDsChan   chan objectIDsResult
	objectsChan     chan objectsResult
}

// NewClient creates a client endpoint with the supplied protocol options.
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	cfg = normalizeConfig(cfg)
	c := &Client{
		config:        cfg,
		objectIDsChan: make(chan objectIDsResult, 1),
		objectsChan:   make(chan objectsResult, 1),
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
// IDs. Blocking requests must request at least one ID and return a non-empty
// reply; non-blocking requests complete promptly and may return none.
func (c *Client) RequestObjectIDs(
	ctx context.Context,
	blocking bool,
	ackCount, requestCount uint16,
) ([]VoteID, error) {
	if ctx == nil {
		return nil, errors.New("context must not be nil")
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()
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
		c.Protocol.Stop()
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
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.requestObjects(ctx, ids)
}

func (c *Client) requestObjects(
	ctx context.Context,
	ids []VoteID,
) ([]VoteObject, error) {
	if len(ids) == 0 {
		return nil, errors.New("peras object request must contain at least one ID")
	}
	if err := c.SendMessageContext(ctx, NewMsgRequestObjects(ids)); err != nil {
		return nil, err
	}
	select {
	case result := <-c.objectsChan:
		return result.objects, result.err
	case <-ctx.Done():
		c.Protocol.Stop()
		return nil, ctx.Err()
	case <-c.DoneChan():
		return nil, protocol.ErrProtocolShuttingDown
	case <-c.callbackContext.ConnectionDoneChan:
		return nil, protocol.ErrProtocolShuttingDown
	}
}

// Sync continuously requests and acknowledges vote batches until cancellation
// or shutdown. A VoteFunc error stops the protocol because the batch is no
// longer safe to acknowledge.
func (c *Client) Sync(ctx context.Context) error {
	if c.config.VoteFunc == nil {
		return errors.New("peras vote function must be configured for sync")
	}
	if ctx == nil {
		return errors.New("context must not be nil")
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()
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
			return errors.New("blocking Peras object ID reply was empty")
		}
		objects, err := c.requestObjects(ctx, ids)
		if err != nil {
			return err
		}
		for _, object := range objects {
			id, err := object.VoteID()
			if err != nil {
				c.Protocol.Stop()
				return err
			}
			if err := c.config.VoteFunc(c.callbackContext, id, object); err != nil {
				c.Protocol.Stop()
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
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.SendMessageContextAndWait(ctx, NewMsgDone())
}

// Stop interrupts the client protocol.
func (c *Client) Stop() { c.Protocol.Stop() }

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
