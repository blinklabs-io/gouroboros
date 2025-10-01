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
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
)

const (
	ProtocolName        = "leios-notify"
	ProtocolId   uint16 = 18
)

var (
	StateIdle = protocol.NewState(1, "Idle")
	StateBusy = protocol.NewState(2, "Busy")
	StateDone = protocol.NewState(3, "Done")
)

var StateMap = protocol.StateMap{
	StateIdle: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeNotificationRequestNext,
				NewState: StateBusy,
			},
			{
				MsgType:  MessageTypeDone,
				NewState: StateDone,
			},
		},
	},
	StateBusy: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeBlockAnnouncement,
				NewState: StateIdle,
			},
			{
				MsgType:  MessageTypeBlockOffer,
				NewState: StateIdle,
			},
			{
				MsgType:  MessageTypeBlockTxsOffer,
				NewState: StateIdle,
			},
			{
				MsgType:  MessageTypeVotesOffer,
				NewState: StateIdle,
			},
		},
	},
	StateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

type LeiosNotify struct {
	Client *Client
	Server *Server
}

type Config struct {
	RequestNextFunc RequestNextFunc
	Timeout         time.Duration
}

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// Callback function types
type (
	RequestNextFunc func(CallbackContext) (protocol.Message, error)
)

func New(protoOptions protocol.ProtocolOptions, cfg *Config) *LeiosNotify {
	b := &LeiosNotify{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return b
}

type LeiosNotifyOptionFunc func(*Config)

func NewConfig(options ...LeiosNotifyOptionFunc) Config {
	c := Config{
		Timeout: 60 * time.Second,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

func WithRequestNextFunc(requestNextFunc RequestNextFunc) LeiosNotifyOptionFunc {
	return func(c *Config) {
		c.RequestNextFunc = requestNextFunc
	}
}

func WithTimeout(timeout time.Duration) LeiosNotifyOptionFunc {
	return func(c *Config) {
		c.Timeout = timeout
	}
}
