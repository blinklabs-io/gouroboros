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

package keepalive

import (
	"fmt"
	"sync"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
)

type Client struct {
	*protocol.Protocol
	config          *Config
	callbackContext CallbackContext
	timer           *time.Timer
	timerMutex      sync.Mutex
	onceStart       sync.Once
}

func NewClient(protoOptions protocol.ProtocolOptions, cfg *Config) *Client {
	if cfg == nil {
		tmpCfg := NewConfig()
		cfg = &tmpCfg
	}
	c := &Client{
		config: cfg,
	}
	c.callbackContext = CallbackContext{
		Client:       c,
		ConnectionId: protoOptions.ConnectionId,
	}
	// Update state map with timeout
	stateMap := StateMap.Copy()
	if entry, ok := stateMap[StateServer]; ok {
		entry.Timeout = c.config.Timeout
		stateMap[StateServer] = entry
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
		InitialState:        StateClient,
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
			// Stop any existing timer
			c.timerMutex.Lock()
			if c.timer != nil {
				c.timer.Stop()
			}
			c.timerMutex.Unlock()
		}()
		c.sendKeepAlive()
	})
}

func (c *Client) sendKeepAlive() {
	msg := NewMsgKeepAlive(c.config.Cookie)
	if err := c.SendMessage(msg); err != nil {
		c.SendError(err)
	}
	// Schedule timer
	c.startTimer()
}

func (c *Client) startTimer() {
	c.timerMutex.Lock()
	defer c.timerMutex.Unlock()
	// Stop any existing timer
	if c.timer != nil {
		c.timer.Stop()
	}
	// Create new timer
	c.timer = time.AfterFunc(c.config.Period, c.sendKeepAlive)
}

func (c *Client) messageHandler(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MessageTypeKeepAliveResponse:
		err = c.handleKeepAliveResponse(msg)
	default:
		err = fmt.Errorf(
			"%s: received unexpected message type %d",
			ProtocolName,
			msg.Type(),
		)
	}
	return err
}

func (c *Client) handleKeepAliveResponse(msgGeneric protocol.Message) error {
	msg := msgGeneric.(*MsgKeepAliveResponse)
	if msg.Cookie != c.config.Cookie {
		return fmt.Errorf(
			"%s: unexpected cookie in response, expected %d but received %d",
			ProtocolName,
			c.config.Cookie,
			msg.Cookie,
		)
	}
	if c.config != nil && c.config.KeepAliveResponseFunc != nil {
		// Call the user callback function
		return c.config.KeepAliveResponseFunc(c.callbackContext, msg.Cookie)
	}
	return nil
}
