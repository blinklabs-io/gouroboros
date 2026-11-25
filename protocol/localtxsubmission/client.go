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
	"context"
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/internal/stopping"
)

// Client implements the LocalTxSubmission client
type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	busyMutex       sync.Mutex
	// stopRequested is closed when Stop is called, before Stop takes
	// busyMutex, so that a SubmitTx waiting for the server under busyMutex
	// returns instead of making Stop wait for the server's answer.
	stopRequested chan struct{}
	// abandoned records, under busyMutex, that a SubmitTx stopped waiting
	// for the server's answer. The server then holds agency, so Stop may
	// not send MsgDone and closes the protocol instead.
	abandoned        bool
	muxer            *muxer.Muxer
	submitResultChan chan error
	onceStart        sync.Once
	onceStop         sync.Once
}

// NewClient returns a new LocalTxSubmission client object
func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:           cfg,
		muxer:            protoOptions.Muxer,
		stopRequested:    make(chan struct{}),
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
			close(c.submitResultChan)
		}()
	})
}

// Stop transitions the protocol to the Done state. No more operations will
// be possible. A SubmitTx waiting for the server's answer returns
// protocol.ErrProtocolShuttingDown; the server then holds agency, so Stop
// closes the protocol without sending MsgDone and stops the muxer, which
// closes the connection: the server still owes that answer, and a client
// registered later on the same connection would read it as its own.
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
	stopMuxer := false
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
			// Stopped once busyMutex is released: both joins wait for
			// goroutines that may run callbacks.
			stopMuxer = c.muxer != nil
			return
		}
		msg := NewMsgDone()
		if err = stopping.SendDone(ctx, c.Protocol, msg); err != nil {
			return
		}
	})
	stopErr := c.Protocol.Stop(ctx)
	if stopMuxer {
		if muxerErr := c.muxer.Stop(ctx); stopErr == nil {
			stopErr = muxerErr
		}
	}
	if stopErr != nil {
		return stopErr
	}
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
	select {
	case <-c.stopRequested:
		return protocol.ErrProtocolShuttingDown
	default:
	}
	msg := NewMsgSubmitTx(eraId, tx)
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	select {
	case err, ok := <-c.submitResultChan:
		if !ok {
			return protocol.ErrProtocolShuttingDown
		}
		return err
	case <-c.stopRequested:
		c.abandoned = true
		return protocol.ErrProtocolShuttingDown
	}
}

// deliverResult hands an answer to the waiting SubmitTx, or drops it once
// Stop is called: the caller may have abandoned the wait, and DoneChan
// closes only after this receive loop returns.
func (c *Client) deliverResult(err error) {
	select {
	case c.submitResultChan <- err:
	case <-c.stopRequested:
	}
}

func (c *Client) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeAcceptTx:
		err = c.handleAcceptTx()
	case MessageTypeRejectTx:
		err = c.handleRejectTx(msg)
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
	// Check for shutdown
	select {
	case <-c.DoneChan():
		return protocol.ErrProtocolShuttingDown
	default:
	}
	c.deliverResult(nil)
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
	// Check for shutdown
	select {
	case <-c.DoneChan():
		return protocol.ErrProtocolShuttingDown
	default:
	}
	msgRejectTx := msg.(*MsgRejectTx)
	rejectErr, err := ledger.NewTxSubmitErrorFromCbor(msgRejectTx.Reason)
	if err != nil {
		return err
	}
	err = TransactionRejectedError{
		Reason:     rejectErr,
		ReasonCbor: []byte(msgRejectTx.Reason),
	}
	c.deliverResult(err)
	return nil
}
