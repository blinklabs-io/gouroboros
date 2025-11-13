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

// Package keepalive implements the Ouroboros KeepAlive mini-protocol, which is used to detect and maintain liveness between nodes in a network.
package keepalive

import (
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
)

const (
	// ProtocolName is the name of the keep-alive protocol.
	ProtocolName = "keep-alive"
	// ProtocolId is the unique protocol identifier for the keep-alive protocol.
	ProtocolId uint16 = 8
	// DefaultKeepAlivePeriod is the default interval between keep-alive probes, in seconds.
	DefaultKeepAlivePeriod = 60
	// DefaultKeepAliveTimeout is the default timeout for keep-alive responses, in seconds.
	DefaultKeepAliveTimeout = 10
)

// Protocol state timeout constants as specified by the network protocol.
const (
	// ClientTimeout is the maximum time the server waits to receive the next keep-alive ping from the client (while in the "Client" protocol state).
	ClientTimeout = 60 * time.Second
	// ServerTimeout is the maximum time the client waits for a keep-alive pong from the server (while in the "Server" protocol state).
	ServerTimeout = 10 * time.Second
)

var (
	// StateClient is the protocol state for the client.
	StateClient = protocol.NewState(1, "Client")
	// StateServer is the protocol state for the server.
	StateServer = protocol.NewState(2, "Server")
	// StateDone is the protocol state indicating completion.
	StateDone = protocol.NewState(3, "Done")
)

// StateMap defines the valid state transitions for the keep-alive protocol.
var StateMap = protocol.StateMap{
	StateClient: protocol.StateMapEntry{
		Agency:  protocol.AgencyClient,
		Timeout: ClientTimeout, // Timeout for server waiting for client keep-alive ping
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
		Agency:  protocol.AgencyServer,
		Timeout: ServerTimeout, // Timeout for client waiting for server keep-alive pong
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeKeepAliveResponse,
				NewState: StateClient,
			},
			{
				MsgType:  MessageTypeDone,
				NewState: StateDone,
			},
		},
	},
	StateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

// KeepAlive provides both client and server implementations of the keep-alive protocol.
type KeepAlive struct {
	Client *Client
	Server *Server
}

// Config contains configuration options for the keep-alive protocol, including callbacks and timing parameters.
type Config struct {
	KeepAliveFunc         KeepAliveFunc
	KeepAliveResponseFunc KeepAliveResponseFunc
	DoneFunc              DoneFunc
	Timeout               time.Duration
	Period                time.Duration
	Cookie                uint16
}

// CallbackContext provides context information to keep-alive protocol callbacks, including connection and role references.
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// KeepAliveFunc is a callback function type for handling keep-alive messages.
type KeepAliveFunc func(CallbackContext, uint16) error

// KeepAliveResponseFunc is a callback function type for handling keep-alive response messages.
type KeepAliveResponseFunc func(CallbackContext, uint16) error

// DoneFunc is a callback function type for handling done messages.
type DoneFunc func(CallbackContext) error

// New creates and returns a new KeepAlive protocol instance using the provided protocol options and configuration.
func New(protoOptions protocol.ProtocolOptions, cfg *Config) *KeepAlive {
	k := &KeepAlive{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return k
}

// KeepAliveOptionFunc is a function that modifies a Config.
type KeepAliveOptionFunc func(*Config)

// NewConfig creates a new Config with default values, applying any provided option functions.
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

// WithKeepAliveFunc sets the KeepAliveFunc callback in the Config.
func WithKeepAliveFunc(keepAliveFunc KeepAliveFunc) KeepAliveOptionFunc {
	return func(c *Config) {
		c.KeepAliveFunc = keepAliveFunc
	}
}

// WithKeepAliveResponseFunc sets the KeepAliveResponseFunc callback in the Config.
func WithKeepAliveResponseFunc(
	keepAliveResponseFunc KeepAliveResponseFunc,
) KeepAliveOptionFunc {
	return func(c *Config) {
		c.KeepAliveResponseFunc = keepAliveResponseFunc
	}
}

// WithDoneFunc sets the DoneFunc callback in the Config.
func WithDoneFunc(doneFunc DoneFunc) KeepAliveOptionFunc {
	return func(c *Config) {
		c.DoneFunc = doneFunc
	}
}

// WithTimeout sets the timeout duration in the Config.
func WithTimeout(timeout time.Duration) KeepAliveOptionFunc {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

// WithPeriod sets the keep-alive period duration in the Config.
func WithPeriod(period time.Duration) KeepAliveOptionFunc {
	return func(c *Config) {
		c.Period = period
	}
}

// WithCookie sets the cookie value in the Config.
func WithCookie(cookie uint16) KeepAliveOptionFunc {
	return func(c *Config) {
		c.Cookie = cookie
	}
}
