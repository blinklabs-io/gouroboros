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

package leiosnotify

import (
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/protocol"
)

type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	requestNextChan chan protocol.Message
	onceStart       sync.Once
	onceStop        sync.Once
	stateMutex      sync.Mutex
	started         bool
	stopped         bool
}

func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:          cfg,
		requestNextChan: make(chan protocol.Message),
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	// Update state map with timeout
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[StateBusy]; ok {
		entry.Timeout = c.config.Timeout
		stateMap[StateBusy] = entry
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
		c.stateMutex.Lock()
		defer c.stateMutex.Unlock()

		if c.stopped {
			return
		}

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
		c.stopped = true
		msg := NewMsgDone()
		if c.started {
			if sendErr := c.SendMessage(msg); sendErr != nil {
				err = sendErr
			}
		}
		// Defer closing channel until protocol fully shuts down (only if started)
		if c.started {
			go func() {
				<-c.DoneChan()
				close(c.requestNextChan)
			}()
		} else {
			// If protocol was never started, close channel immediately
			close(c.requestNextChan)
		}
	})
	return err
}

// RequestNext fetches the next available notification. This function will block until a notification is received from the peer.
func (c *Client) RequestNext() (protocol.Message, error) {
	msg := NewMsgNotificationRequestNext()
	if err := c.SendMessage(msg); err != nil {
		return nil, err
	}
	resp, ok := <-c.requestNextChan
	if !ok {
		return nil, protocol.ErrProtocolShuttingDown
	}
	return resp, nil
}

func (c *Client) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeBlockAnnouncement:
		err = c.handleBlockAnnouncement(msg)
	case MessageTypeBlockOffer:
		err = c.handleBlockOffer(msg)
	case MessageTypeBlockTxsOffer:
		err = c.handleBlockTxsOffer(msg)
	case MessageTypeVotesOffer:
		err = c.handleVotesOffer(msg)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (c *Client) handleBlockAnnouncement(msg protocol.Message) error {
	select {
	case <-c.DoneChan():
		return protocol.ErrProtocolShuttingDown
	case c.requestNextChan <- msg:
	}
	return nil
}

func (c *Client) handleBlockOffer(msg protocol.Message) error {
	select {
	case <-c.DoneChan():
		return protocol.ErrProtocolShuttingDown
	case c.requestNextChan <- msg:
	}
	return nil
}

func (c *Client) handleBlockTxsOffer(msg protocol.Message) error {
	select {
	case <-c.DoneChan():
		return protocol.ErrProtocolShuttingDown
	case c.requestNextChan <- msg:
	}
	return nil
}

func (c *Client) handleVotesOffer(msg protocol.Message) error {
	select {
	case <-c.DoneChan():
		return protocol.ErrProtocolShuttingDown
	case c.requestNextChan <- msg:
	}
	return nil
}
