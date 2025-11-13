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

package localtxsubmission

import (
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Client implements the LocalTxSubmission client
type Client struct {
	*protocol.Protocol
	config                *Config
	callbackContext       CallbackContext
	busyMutex             sync.Mutex
	submitResultChan      chan error
	onceStart             sync.Once
	onceStop              sync.Once
	stateMutex            sync.Mutex
	started               bool
	closeSubmitResultOnce sync.Once
}

// NewClient returns a new LocalTxSubmission client object
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:           cfg,
		submitResultChan: make(chan error),
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	// Update state map with timeout
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[stateBusy]; ok {
		entry.Timeout = c.config.Timeout
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
		StateMap:            stateMap,
		InitialState:        stateIdle,
	}
	c.Protocol = protocol.New(protoConfig)
	return c
}

func (c *Client) Start() {
	c.onceStart.Do(func() {
		c.stateMutex.Lock()
		defer c.stateMutex.Unlock()

		c.Protocol.Logger().
			Debug("starting client protocol",
				"component", "network",
				"protocol", ProtocolName,
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
		c.started = true
		c.Protocol.Start()
	})
}

// Stop transitions the protocol to the Done state. No more operations will be possible
func (c *Client) Stop() error {
	var err error
	c.onceStop.Do(func() {
		c.stateMutex.Lock()
		defer c.stateMutex.Unlock()

		c.Protocol.Logger().
			Debug("stopping client protocol",
				"component", "network",
				"protocol", ProtocolName,
				"connection_id", c.callbackContext.ConnectionId.String(),
			)
		c.busyMutex.Lock()
		defer c.busyMutex.Unlock()
		msg := NewMsgDone()
		if sendErr := c.SendMessage(msg); sendErr != nil {
			err = sendErr
		}
		// Always attempt to stop the protocol, even if SendMessage failed
		_ = c.Protocol.Stop() // Error ignored - method returns SendMessage error if any
		// Defer closing channel until protocol fully shuts down (only if started)
		if c.started {
			go func() {
				<-c.DoneChan()
				c.closeSubmitResultChan()
			}()
		} else {
			// If protocol was never started, close channel immediately
			c.closeSubmitResultChan()
		}
	})
	return err
}

// SubmitTx submits a transaction using the specified transaction era ID and TX payload
func (c *Client) SubmitTx(eraId uint16, tx []byte) error {
	c.Protocol.Logger().
		Debug(fmt.Sprintf("calling SubmitTx(eraId: %d, tx: %x)", eraId, tx),
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	msg := NewMsgSubmitTx(eraId, tx)
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	err, ok := <-c.submitResultChan
	if !ok {
		return protocol.ErrProtocolShuttingDown
	}
	return err
}

func (c *Client) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeAcceptTx:
		err = c.handleAcceptTx()
	case MessageTypeRejectTx:
		err = c.handleRejectTx(msg)
	case MessageTypeDone:
		err = c.handleDone()
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (c *Client) handleAcceptTx() error {
	c.Protocol.Logger().
		Debug("accept tx",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	select {
	case <-c.DoneChan():
		return protocol.ErrProtocolShuttingDown
	case c.submitResultChan <- nil:
	}
	return nil
}

func (c *Client) handleRejectTx(msg protocol.Message) error {
	c.Protocol.Logger().
		Debug("reject tx",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	msgRejectTx := msg.(*MsgRejectTx)
	rejectErr, err := ledger.NewTxSubmitErrorFromCbor(msgRejectTx.Reason)
	if err != nil {
		return err
	}
	err = TransactionRejectedError{
		Reason:     rejectErr,
		ReasonCbor: []byte(msgRejectTx.Reason),
	}
	select {
	case <-c.DoneChan():
		return protocol.ErrProtocolShuttingDown
	case c.submitResultChan <- err:
	}
	return nil
}

func (c *Client) handleDone() error {
	c.Protocol.Logger().
		Debug("received done from server",
			"component", "network",
			"protocol", ProtocolName,
			"role", "client",
			"connection_id", c.callbackContext.ConnectionId.String(),
		)
	// Server is shutting down, close the result channel to unblock any waiting operations
	c.closeSubmitResultChan()
	return nil
}

func (c *Client) closeSubmitResultChan() {
	c.closeSubmitResultOnce.Do(func() {
		close(c.submitResultChan)
	})
}
