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
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
)

const (
	ProtocolName        = "keep-alive"
	ProtocolId   uint16 = 8

	// Time between keep-alive probes, in seconds
	DefaultKeepAlivePeriod = 60

	// Timeout for keep-alive responses, in seconds
	DefaultKeepAliveTimeout = 10
)

var (
	StateClient = protocol.NewState(1, "Client")
	StateServer = protocol.NewState(2, "Server")
	StateDone   = protocol.NewState(3, "Done")
)

var StateMap = protocol.StateMap{
	StateClient: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeKeepAlive,
				NewState: StateServer,
			},
			{
				MsgType:  MessageTypeDone,
				NewState: StateDone,
			},
		},
	},
	StateServer: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeKeepAliveResponse,
				NewState: StateClient,
			},
		},
	},
	StateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

type KeepAlive struct {
	Client *Client
	Server *Server
}

type Config struct {
	KeepAliveFunc         KeepAliveFunc
	KeepAliveResponseFunc KeepAliveResponseFunc
	DoneFunc              DoneFunc
	Timeout               time.Duration
	Period                time.Duration
	Cookie                uint16
}

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// Callback function types
type KeepAliveFunc func(CallbackContext, uint16) error
type KeepAliveResponseFunc func(CallbackContext, uint16) error
type DoneFunc func(CallbackContext) error

func New(protoOptions protocol.ProtocolOptions, cfg *Config) *KeepAlive {
	k := &KeepAlive{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return k
}

type KeepAliveOptionFunc func(*Config)

func NewConfig(options ...KeepAliveOptionFunc) Config {
	c := Config{
		Period:  DefaultKeepAlivePeriod * time.Second,
		Timeout: DefaultKeepAliveTimeout * time.Second,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

func WithKeepAliveFunc(keepAliveFunc KeepAliveFunc) KeepAliveOptionFunc {
	return func(c *Config) {
		c.KeepAliveFunc = keepAliveFunc
	}
}

func WithKeepAliveResponseFunc(
	keepAliveResponseFunc KeepAliveResponseFunc,
) KeepAliveOptionFunc {
	return func(c *Config) {
		c.KeepAliveResponseFunc = keepAliveResponseFunc
	}
}

func WithDoneFunc(doneFunc DoneFunc) KeepAliveOptionFunc {
	return func(c *Config) {
		c.DoneFunc = doneFunc
	}
}

func WithTimeout(timeout time.Duration) KeepAliveOptionFunc {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

func WithPeriod(period time.Duration) KeepAliveOptionFunc {
	return func(c *Config) {
		c.Period = period
	}
}

func WithCookie(cookie uint16) KeepAliveOptionFunc {
	return func(c *Config) {
		c.Cookie = cookie
	}
}
