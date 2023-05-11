// Copyright 2023 Blink Labs, LLC.
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

	"github.com/blinklabs-io/gouroboros/protocol"
)

const (
	PROTOCOL_NAME        = "keep-alive"
	PROTOCOL_ID   uint16 = 8

	// Time between keep-alive probes, in seconds
	DEFAULT_KEEP_ALIVE_PERIOD = 60

	// Timeout for keep-alive responses, in seconds
	DEFAULT_KEEP_ALIVE_TIMEOUT = 10
)

var (
	STATE_CLIENT = protocol.NewState(1, "Client")
	STATE_SERVER = protocol.NewState(2, "Server")
	STATE_DONE   = protocol.NewState(3, "Done")
)

var StateMap = protocol.StateMap{
	STATE_CLIENT: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_KEEP_ALIVE,
				NewState: STATE_SERVER,
			},
			{
				MsgType:  MESSAGE_TYPE_DONE,
				NewState: STATE_DONE,
			},
		},
	},
	STATE_SERVER: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_KEEP_ALIVE_RESPONSE,
				NewState: STATE_CLIENT,
			},
		},
	},
	STATE_DONE: protocol.StateMapEntry{
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
}

// Callback function types
type KeepAliveFunc func(uint16) error
type KeepAliveResponseFunc func(uint16) error
type DoneFunc func() error

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
		Period:  DEFAULT_KEEP_ALIVE_PERIOD * time.Second,
		Timeout: DEFAULT_KEEP_ALIVE_TIMEOUT * time.Second,
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

func WithKeepAliveResponseFunc(keepAliveResponseFunc KeepAliveResponseFunc) KeepAliveOptionFunc {
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
