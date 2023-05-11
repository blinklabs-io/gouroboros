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

// Package handshake implements the Ouroboros handshake protocol
package handshake

import (
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Protocol identifiers
const (
	protocolName = "handshake"
	protocolId   = 0
)

// Diffusion modes
const (
	DiffusionModeInitiatorOnly         = false
	DiffusionModeInitiatorAndResponder = true
)

var (
	statePropose = protocol.NewState(1, "Propose")
	stateConfirm = protocol.NewState(2, "Confirm")
	stateDone    = protocol.NewState(3, "Done")
)

// Handshake protocol state machine
var StateMap = protocol.StateMap{
	statePropose: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeProposeVersions,
				NewState: stateConfirm,
			},
		},
	},
	stateConfirm: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeAcceptVersion,
				NewState: stateDone,
			},
			{
				MsgType:  MessageTypeRefuse,
				NewState: stateDone,
			},
		},
	},
	stateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

// Handshake is a wrapper object that holds the client and server instances
type Handshake struct {
	Client *Client
	Server *Server
}

// Config is used to configure the Handshake protocol instance
type Config struct {
	ProtocolVersions []uint16
	NetworkMagic     uint32
	ClientFullDuplex bool
	FinishedFunc     FinishedFunc
	Timeout          time.Duration
}

// Callback function types
type FinishedFunc func(uint16, bool) error

// New returns a new Handshake object
func New(protoOptions protocol.ProtocolOptions, cfg *Config) *Handshake {
	h := &Handshake{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return h
}

// HandshakeOptionFunc represents a function used to modify the Handshake protocol config
type HandshakeOptionFunc func(*Config)

// NewConfig returns a new Handshake config object with the provided options
func NewConfig(options ...HandshakeOptionFunc) Config {
	c := Config{
		Timeout: 5 * time.Second,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

// WithProtocolVersions specifies the supported protocol versions
func WithProtocolVersions(versions []uint16) HandshakeOptionFunc {
	return func(c *Config) {
		c.ProtocolVersions = versions
	}
}

// WithNetworkMagic specifies the network magic value
func WithNetworkMagic(networkMagic uint32) HandshakeOptionFunc {
	return func(c *Config) {
		c.NetworkMagic = networkMagic
	}
}

// WithClientFullDuplex specifies whether to request full duplex mode when acting as a client
func WithClientFullDuplex(fullDuplex bool) HandshakeOptionFunc {
	return func(c *Config) {
		c.ClientFullDuplex = fullDuplex
	}
}

// WithFinishedFunc specifies the Finished callback function
func WithFinishedFunc(finishedFunc FinishedFunc) HandshakeOptionFunc {
	return func(c *Config) {
		c.FinishedFunc = finishedFunc
	}
}

// WithTimeout specifies the timeout for the handshake operation
func WithTimeout(timeout time.Duration) HandshakeOptionFunc {
	return func(c *Config) {
		c.Timeout = timeout
	}
}
