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

package localtxmonitor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/internal/stopping"
)

// Client implements the LocalTxMonitor client
type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	busyMutex       sync.Mutex
	// stopRequested is closed when Stop is called, before Stop takes
	// busyMutex, so that a call waiting for the server under busyMutex
	// returns instead of making Stop wait for the server's answer.
	stopRequested chan struct{}
	// abandoned records, under busyMutex, that a call stopped waiting for
	// the server's answer. The server then holds agency, so Stop may not
	// send MsgDone and closes the protocol instead.
	abandoned          bool
	acquired           bool
	acquiredSlot       uint64
	acquireResultChan  chan bool
	hasTxResultChan    chan bool
	nextTxResultChan   chan []byte
	getSizesResultChan chan MsgReplyGetSizesResult
	getMeasuresChan    chan MempoolMeasures
	enableGetMeasures  bool
	onceStart          sync.Once
	onceStop           sync.Once
}

// NewClient returns a new LocalTxMonitor client object
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:             cfg,
		stopRequested:      make(chan struct{}),
		acquireResultChan:  make(chan bool),
		hasTxResultChan:    make(chan bool),
		nextTxResultChan:   make(chan []byte),
		getSizesResultChan: make(chan MsgReplyGetSizesResult),
		getMeasuresChan:    make(chan MempoolMeasures),
		enableGetMeasures:  supportsGetMeasures(protoOptions.Version),
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	stateCtx := &stateContext{}
	// Update state map with timeout
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[stateAcquiring]; ok {
		// MsgAwaitAcquire waits for the mempool to change, which may take
		// any time, so only an acquire from Idle is timed.
		acquireTimeout := c.config.AcquireTimeout
		entry.TimeoutFunc = func() time.Duration {
			if stateCtx.awaiting.Load() {
				return 0
			}
			return acquireTimeout
		}
		stateMap[stateAcquiring] = entry
	}
	if entry, ok := stateMap[stateBusy]; ok {
		entry.Timeout = c.config.QueryTimeout
		stateMap[stateBusy] = entry
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
		StateContext:        stateCtx,
		StateMap:            stateMap,
		InitialState:        stateIdle,
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
	})
}

// Stop transitions the protocol to the Done state. No more operations will
// be possible. A call waiting for the server's answer returns
// protocol.ErrProtocolShuttingDown; the server then holds agency, so Stop
// closes the protocol without sending MsgDone.
//
// Stop sends the message that ends the client's run and waits up to 250ms
// for it to be written, then stops the protocol. It signals shutdown at
// once. When ctx is the Context of a CallbackContext this client passed to a
// callback, Stop returns without waiting for the protocol's goroutines,
// since the goroutine running the callback is one of them. Otherwise it
// waits until they have returned, or until ctx is done, in which case it
// returns ctx.Err() and shutdown still proceeds.
func (c *Client) Stop(ctx context.Context) error {
	var err error
	c.onceStop.Do(func() {
		c.Protocol.Logger().
			Debug("stopping client protocol",
				"component", "network",
				"protocol", ProtocolName,
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
		close(c.stopRequested)
		c.busyMutex.Lock()
		defer c.busyMutex.Unlock()
		if c.abandoned {
			return
		}
		msg := NewMsgDone()
		if err = stopping.SendDone(ctx, c.Protocol, msg); err != nil {
			return
		}
	})
	if err := c.Protocol.Stop(ctx); err != nil {
		return err
	}
	return err
}

// Acquire starts the acquire process for a current mempool snapshot
func (c *Client) Acquire() error {
	c.Protocol.Logger().
		Debug("calling Acquire()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	if c.stopping() {
		return protocol.ErrProtocolShuttingDown
	}
	return c.acquire()
}

// Release releases the previously acquired mempool snapshot. It returns
// ErrNotAcquired, without sending anything, when no snapshot is acquired.
func (c *Client) Release() error {
	c.Protocol.Logger().
		Debug("calling Release()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	if c.stopping() {
		return protocol.ErrProtocolShuttingDown
	}
	return c.release()
}

// HasTx cannot query an untagged transaction ID because the node-to-client
// protocol requires the transaction's era.
//
// Deprecated: use HasTxForEra.
func (c *Client) HasTx(txId []byte) (bool, error) {
	return false, errHasTxEraRequired
}

// HasTxForEra returns whether the specified era-tagged transaction ID exists
// in the mempool snapshot.
func (c *Client) HasTxForEra(eraId uint8, txId []byte) (bool, error) {
	c.Protocol.Logger().
		Debug(fmt.Sprintf("calling HasTx(eraId: %d, txId: %x)", eraId, txId),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	if err := validateTxId(txId); err != nil {
		return false, err
	}
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	if c.stopping() {
		return false, protocol.ErrProtocolShuttingDown
	}
	if !c.acquired {
		if err := c.acquire(); err != nil {
			return false, err
		}
	}
	msg := NewMsgHasTxForEra(eraId, txId)
	if err := c.SendMessage(msg); err != nil {
		return false, err
	}
	select {
	case result := <-c.hasTxResultChan:
		return result, nil
	case <-c.stopRequested:
		c.abandoned = true
		return false, protocol.ErrProtocolShuttingDown
	case <-c.DoneChan():
		return false, protocol.ErrProtocolShuttingDown
	}
}

// NextTx returns the next transaction in the mempool snapshot
func (c *Client) NextTx() ([]byte, error) {
	c.Protocol.Logger().
		Debug("calling NextTx()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	if c.stopping() {
		return nil, protocol.ErrProtocolShuttingDown
	}
	if !c.acquired {
		if err := c.acquire(); err != nil {
			return nil, err
		}
	}
	msg := NewMsgNextTx()
	if err := c.SendMessage(msg); err != nil {
		return nil, err
	}
	select {
	case tx := <-c.nextTxResultChan:
		return tx, nil
	case <-c.stopRequested:
		c.abandoned = true
		return nil, protocol.ErrProtocolShuttingDown
	case <-c.DoneChan():
		return nil, protocol.ErrProtocolShuttingDown
	}
}

// GetSizes returns the capacity (in bytes), size (in bytes), and number of transactions in the mempool snapshot
func (c *Client) GetSizes() (uint32, uint32, uint32, error) {
	c.Protocol.Logger().
		Debug("calling GetSizes()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	if c.stopping() {
		return 0, 0, 0, protocol.ErrProtocolShuttingDown
	}
	if !c.acquired {
		if err := c.acquire(); err != nil {
			return 0, 0, 0, err
		}
	}
	msg := NewMsgGetSizes()
	if err := c.SendMessage(msg); err != nil {
		return 0, 0, 0, err
	}
	select {
	case result := <-c.getSizesResultChan:
		return result.Capacity, result.Size, result.NumberOfTxs, nil
	case <-c.stopRequested:
		c.abandoned = true
		return 0, 0, 0, protocol.ErrProtocolShuttingDown
	case <-c.DoneChan():
		return 0, 0, 0, protocol.ErrProtocolShuttingDown
	}
}

// ErrNotAcquired is returned by Release when no mempool snapshot is
// acquired. Nothing is sent: MsgRelease is legal only while a snapshot is
// held, and sending it from Idle would close the connection.
var ErrNotAcquired = errors.New("local-tx-monitor: no mempool snapshot acquired")

// ErrGetMeasuresUnsupported is returned by GetMeasures when the negotiated
// node-to-client version is older than 20, which has no MsgGetMeasures.
var ErrGetMeasuresUnsupported = errors.New(
	"local-tx-monitor: GetMeasures requires node-to-client version 20 or later",
)

// GetMeasures returns the number of transactions in the mempool snapshot and
// their measures (such as MeasureTransactionBytes) against the mempool's
// capacity. It needs node-to-client version 20 or later and otherwise
// returns ErrGetMeasuresUnsupported without sending anything, since an older
// node cannot decode the request.
func (c *Client) GetMeasures() (MempoolMeasures, error) {
	c.Protocol.Logger().
		Debug("calling GetMeasures()",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	if !c.enableGetMeasures {
		return MempoolMeasures{}, ErrGetMeasuresUnsupported
	}
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	if !c.acquired {
		if err := c.acquire(); err != nil {
			return MempoolMeasures{}, err
		}
	}
	if err := c.SendMessage(NewMsgGetMeasures()); err != nil {
		return MempoolMeasures{}, err
	}
	select {
	case result := <-c.getMeasuresChan:
		return result, nil
	case <-c.stopRequested:
		c.abandoned = true
		return MempoolMeasures{}, protocol.ErrProtocolShuttingDown
	case <-c.DoneChan():
		return MempoolMeasures{}, protocol.ErrProtocolShuttingDown
	}
}

func (c *Client) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeAcquired:
		c.handleAcquired(msg)
	case MessageTypeReplyHasTx:
		c.handleReplyHasTx(msg)
	case MessageTypeReplyNextTx:
		c.handleReplyNextTx(msg)
	case MessageTypeReplyGetSizes:
		c.handleReplyGetSizes(msg)
	case MessageTypeReplyGetMeasures:
		c.handleReplyGetMeasures(msg)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (c *Client) handleAcquired(msg protocol.Message) {
	c.Protocol.Logger().
		Debug("acquired",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	msgAcquired := msg.(*MsgAcquired)
	c.acquired = true
	c.acquiredSlot = msgAcquired.SlotNo
	deliverResult(c, c.acquireResultChan, true)
}

func (c *Client) handleReplyHasTx(msg protocol.Message) {
	c.Protocol.Logger().
		Debug("reply has tx",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	msgReplyHasTx := msg.(*MsgReplyHasTx)
	deliverResult(c, c.hasTxResultChan, msgReplyHasTx.Result)
}

func (c *Client) handleReplyNextTx(msg protocol.Message) {
	c.Protocol.Logger().
		Debug("reply next tx",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	msgReplyNextTx := msg.(*MsgReplyNextTx)
	deliverResult(c, c.nextTxResultChan, msgReplyNextTx.Transaction.Tx)
}

func (c *Client) handleReplyGetSizes(msg protocol.Message) {
	c.Protocol.Logger().
		Debug("reply get sizes",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	msgReplyGetSizes := msg.(*MsgReplyGetSizes)
	deliverResult(c, c.getSizesResultChan, msgReplyGetSizes.Result)
}

func (c *Client) handleReplyGetMeasures(msg protocol.Message) {
	c.Protocol.Logger().
		Debug("reply get measures",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	reply := msg.(*MsgReplyGetMeasures)
	deliverResult(c, c.getMeasuresChan, MempoolMeasures{
		TxCount:  reply.TxCount,
		Measures: reply.Measures,
	})
}

// deliverResult hands a reply to the waiting caller, or gives up once the
// protocol shuts down so the receive loop is never left blocked. It also
// gives up once Stop is called: the caller may have abandoned the wait, and
// DoneChan closes only after this receive loop returns.
func deliverResult[T any](c *Client, resultChan chan<- T, result T) {
	select {
	case resultChan <- result:
	case <-c.stopRequested:
	case <-c.DoneChan():
	}
}

func (c *Client) stopping() bool {
	select {
	case <-c.stopRequested:
		return true
	default:
		return false
	}
}

func (c *Client) acquire() error {
	msg := NewMsgAcquire()
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	// Wait for reply
	select {
	case <-c.acquireResultChan:
		return nil
	case <-c.stopRequested:
		c.abandoned = true
		return protocol.ErrProtocolShuttingDown
	case <-c.DoneChan():
		return protocol.ErrProtocolShuttingDown
	}
}

func (c *Client) release() error {
	if !c.acquired {
		return ErrNotAcquired
	}
	msg := NewMsgRelease()
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	c.acquired = false
	return nil
}
