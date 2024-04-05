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
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Client implements the LocalTxMonitor client
type Client struct {
	*protocol.Protocol
	config             *Config
	callbackContext    CallbackContext
	busyMutex          sync.Mutex
	acquired           bool
	acquiredSlot       uint64
	acquireResultChan  chan bool
	hasTxResultChan    chan bool
	nextTxResultChan   chan []byte
	getSizesResultChan chan MsgReplyGetSizesResult
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
		acquireResultChan:  make(chan bool),
		hasTxResultChan:    make(chan bool),
		nextTxResultChan:   make(chan []byte),
		getSizesResultChan: make(chan MsgReplyGetSizesResult),
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	// Update state map with timeout
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[stateAcquiring]; ok {
		entry.Timeout = c.config.AcquireTimeout
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
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleClient,
		MessageHandlerFunc:  c.messageHandler,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            stateMap,
		InitialState:        stateIdle,
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
			close(c.acquireResultChan)
			close(c.hasTxResultChan)
			close(c.nextTxResultChan)
			close(c.getSizesResultChan)
		}()
	})
}

func (c *Client) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeAcquired:
		err = c.handleAcquired(msg)
	case MessageTypeReplyHasTx:
		err = c.handleReplyHasTx(msg)
	case MessageTypeReplyNextTx:
		err = c.handleReplyNextTx(msg)
	case MessageTypeReplyGetSizes:
		err = c.handleReplyGetSizes(msg)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (c *Client) acquire() error {
	msg := NewMsgAcquire()
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	// Wait for reply
	_, ok := <-c.acquireResultChan
	if !ok {
		return protocol.ProtocolShuttingDownError
	}
	return nil
}

func (c *Client) release() error {
	msg := NewMsgRelease()
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	c.acquired = false
	return nil
}

// Acquire starts the acquire process for a current mempool snapshot
func (c *Client) Acquire() error {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	return c.acquire()
}

// Release releases the previously acquired mempool snapshot
func (c *Client) Release() error {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	return c.release()
}

// Stop transitions the protocol to the Done state. No more operations will be possible
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

// HasTx returns whether or not the specified transaction ID exists in the mempool snapshot
func (c *Client) HasTx(txId []byte) (bool, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	if !c.acquired {
		if err := c.acquire(); err != nil {
			return false, err
		}
	}
	msg := NewMsgHasTx(txId)
	if err := c.SendMessage(msg); err != nil {
		return false, err
	}
	result, ok := <-c.hasTxResultChan
	if !ok {
		return false, protocol.ProtocolShuttingDownError
	}
	return result, nil
}

// NextTx returns the next transaction in the mempool snapshot
func (c *Client) NextTx() ([]byte, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	if !c.acquired {
		if err := c.acquire(); err != nil {
			return nil, err
		}
	}
	msg := NewMsgNextTx()
	if err := c.SendMessage(msg); err != nil {
		return nil, err
	}
	tx, ok := <-c.nextTxResultChan
	if !ok {
		return nil, protocol.ProtocolShuttingDownError
	}
	return tx, nil
}

// GetSizes returns the capacity (in bytes), size (in bytes), and number of transactions in the mempool snapshot
func (c *Client) GetSizes() (uint32, uint32, uint32, error) {
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	if !c.acquired {
		if err := c.acquire(); err != nil {
			return 0, 0, 0, err
		}
	}
	msg := NewMsgGetSizes()
	if err := c.SendMessage(msg); err != nil {
		return 0, 0, 0, err
	}
	result, ok := <-c.getSizesResultChan
	if !ok {
		return 0, 0, 0, protocol.ProtocolShuttingDownError
	}
	return result.Capacity, result.Size, result.NumberOfTxs, nil
}

func (c *Client) handleAcquired(msg protocol.Message) error {
	msgAcquired := msg.(*MsgAcquired)
	c.acquired = true
	c.acquiredSlot = msgAcquired.SlotNo
	c.acquireResultChan <- true
	return nil
}

func (c *Client) handleReplyHasTx(msg protocol.Message) error {
	msgReplyHasTx := msg.(*MsgReplyHasTx)
	c.hasTxResultChan <- msgReplyHasTx.Result
	return nil
}

func (c *Client) handleReplyNextTx(msg protocol.Message) error {
	msgReplyNextTx := msg.(*MsgReplyNextTx)
	c.nextTxResultChan <- msgReplyNextTx.Transaction.Tx
	return nil
}

func (c *Client) handleReplyGetSizes(msg protocol.Message) error {
	msgReplyGetSizes := msg.(*MsgReplyGetSizes)
	c.getSizesResultChan <- msgReplyGetSizes.Result
	return nil
}
