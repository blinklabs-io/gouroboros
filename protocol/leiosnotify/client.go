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

package leiosnotify

import (
	"errors"
	"fmt"
	"sync"

	"github.com/blinklabs-io/gouroboros/protocol"
)

type Client struct {
	*protocol.Protocol
	config               *Config
	callbackContext      CallbackContext
	busyMutex            sync.Mutex
	notificationChan     chan protocol.Message
	pipelinedRequestNext int
	onceStart            sync.Once
	onceStop             sync.Once
}

func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config:           cfg,
		notificationChan: make(chan protocol.Message),
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
			close(c.notificationChan)
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

// Sync starts an async process to fetch Leios notifications. Notification messages will be delivered via the
// Notification callback function specified in the protocol config
func (c *Client) Sync() error {
	if c.config == nil || c.config.NotificationFunc == nil {
		return errors.New("you must configure NotificationFunc to receive notifications")
	}
	c.busyMutex.Lock()
	defer c.busyMutex.Unlock()
	msg := NewMsgNotificationRequestNext()
	if err := c.SendMessage(msg); err != nil {
		return err
	}
	// Reset pipelined message counter
	c.pipelinedRequestNext = 0
	// Start notification loop
	go c.notificationLoop()
	return nil
}

func (c *Client) notificationLoop() {
	for {
		// Wait for a notification to be received
		select {
		case msg, ok := <-c.notificationChan:
			if !ok {
				// Channel is closed, which means we're shutting down
				return
			}
			err := c.config.NotificationFunc(c.callbackContext, msg)
			if err != nil {
				if errors.Is(err, ErrStopNotificationProcess) {
					return
				}
				c.SendError(err)
				return
			}
		case <-c.DoneChan():
			// Protocol is shutting down
			return
		}
		c.busyMutex.Lock()
		// Wait for next notification if we have pipelined messages
		if c.pipelinedRequestNext > 0 {
			c.pipelinedRequestNext--
			c.busyMutex.Unlock()
			continue
		}
		// Request the next notification(s)
		msgCount := max(c.config.PipelineLimit, 1)
		for range msgCount {
			msg := NewMsgNotificationRequestNext()
			if err := c.SendMessage(msg); err != nil {
				c.SendError(err)
				c.busyMutex.Unlock()
				return
			}
		}
		c.pipelinedRequestNext = msgCount - 1
		c.busyMutex.Unlock()
	}
}

func (c *Client) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeBlockAnnouncement:
		c.handleBlockAnnouncement(msg)
	case MessageTypeBlockOffer:
		c.handleBlockOffer(msg)
	case MessageTypeBlockTxsOffer:
		c.handleBlockTxsOffer(msg)
	case MessageTypeVotesOffer:
		c.handleVotesOffer(msg)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (c *Client) handleBlockAnnouncement(msg protocol.Message) {
	select {
	case <-c.DoneChan():
	case c.notificationChan <- msg:
	}
}

func (c *Client) handleBlockOffer(msg protocol.Message) {
	select {
	case <-c.DoneChan():
	case c.notificationChan <- msg:
	}
}

func (c *Client) handleBlockTxsOffer(msg protocol.Message) {
	select {
	case <-c.DoneChan():
	case c.notificationChan <- msg:
	}
}

func (c *Client) handleVotesOffer(msg protocol.Message) {
	select {
	case <-c.DoneChan():
	case c.notificationChan <- msg:
	}
}
