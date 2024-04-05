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

// Package peersharing implements the Ouroboros PeerSharing protocol
package peersharing

import (
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Protocol identifiers
const (
	ProtocolName = "peer-sharing"
	ProtocolId   = 10
)

var (
	stateIdle = protocol.NewState(1, "Idle")
	stateBusy = protocol.NewState(2, "Busy")
	stateDone = protocol.NewState(3, "Done")
)

// PeerSharing protocol state machine
var StateMap = protocol.StateMap{
	stateIdle: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeShareRequest,
				NewState: stateBusy,
			},
			{
				MsgType:  MessageTypeDone,
				NewState: stateDone,
			},
		},
	},
	stateBusy: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeSharePeers,
				NewState: stateIdle,
			},
		},
	},
	stateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

// PeerSharing is a wrapper object that holds the client and server instances
type PeerSharing struct {
	Client *Client
	Server *Server
}

// Config is used to configure the PeerSharing protocol instance
type Config struct {
	ShareRequestFunc ShareRequestFunc
	Timeout          time.Duration
}

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// Callback function types
type ShareRequestFunc func(CallbackContext, int) ([]PeerAddress, error)

// New returns a new PeerSharing object
func New(protoOptions protocol.ProtocolOptions, cfg *Config) *PeerSharing {
	h := &PeerSharing{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return h
}

// PeerSharingOptionFunc represents a function used to modify the PeerSharing protocol config
type PeerSharingOptionFunc func(*Config)

// NewConfig returns a new PeerSharing config object with the provided options
func NewConfig(options ...PeerSharingOptionFunc) Config {
	c := Config{
		Timeout: 5 * time.Second,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

// WithShareRequestFunc specifies the ShareRequest callback function
func WithShareRequestFunc(shareRequestFunc ShareRequestFunc) PeerSharingOptionFunc {
	return func(c *Config) {
		c.ShareRequestFunc = shareRequestFunc
	}
}

// WithTimeout specifies the timeout for the handshake operation
func WithTimeout(timeout time.Duration) PeerSharingOptionFunc {
	return func(c *Config) {
		c.Timeout = timeout
	}
}
