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

// Package handshake implements the Ouroboros handshake protocol
package handshake

import (
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Protocol identifiers
const (
	ProtocolName = "handshake"
	ProtocolId   = 0
)

// Protocol state timeout constants per Ouroboros Network Specification (Table 3.4).
// N2N handshake has timeouts; N2C handshake has none.
const (
	ProposeTimeout = 10 * time.Second // N2N: timeout for client to propose versions
	ConfirmTimeout = 10 * time.Second // N2N: timeout for server to accept or refuse versions
)

var (
	statePropose = protocol.NewState(1, "Propose")
	stateConfirm = protocol.NewState(2, "Confirm")
	stateDone    = protocol.NewState(3, "Done")
)

// confirmTransitions are shared between N2N and N2C state maps.
var confirmTransitions = []protocol.StateTransition{
	{
		MsgType:  MessageTypeAcceptVersion,
		NewState: stateDone,
	},
	{
		MsgType:  MessageTypeRefuse,
		NewState: stateDone,
	},
	{
		MsgType:  MessageTypeQueryReply,
		NewState: stateDone,
	},
}

// proposeTransitions are shared between N2N and N2C state maps.
var proposeTransitions = []protocol.StateTransition{
	{
		MsgType:  MessageTypeProposeVersions,
		NewState: stateConfirm,
	},
}

// StateMapNtN is the N2N handshake state machine with timeouts per spec Table 3.4.
var StateMapNtN = protocol.StateMap{
	statePropose: protocol.StateMapEntry{
		Agency:      protocol.AgencyClient,
		Timeout:     ProposeTimeout,
		Transitions: proposeTransitions,
	},
	stateConfirm: protocol.StateMapEntry{
		Agency:      protocol.AgencyServer,
		Timeout:     ConfirmTimeout,
		Transitions: confirmTransitions,
	},
	stateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

// StateMapNtC is the N2C handshake state machine with no timeouts per spec Table 3.5.
var StateMapNtC = protocol.StateMap{
	statePropose: protocol.StateMapEntry{
		Agency:      protocol.AgencyClient,
		Transitions: proposeTransitions,
	},
	stateConfirm: protocol.StateMapEntry{
		Agency:      protocol.AgencyServer,
		Transitions: confirmTransitions,
	},
	stateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

// StateMap is a copy of StateMapNtN for backward compatibility.
var StateMap = StateMapNtN.Copy()

// Handshake is a wrapper object that holds the client and server instances
type Handshake struct {
	Client *Client
	Server *Server
}

// Config is used to configure the Handshake protocol instance
type Config struct {
	ProtocolVersionMap protocol.ProtocolVersionMap
	FinishedFunc       FinishedFunc
	QueryReplyFunc     QueryReplyFunc
	Timeout            time.Duration
}

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// Callback function types
type FinishedFunc func(CallbackContext, uint16, protocol.VersionData) error

type QueryReplyFunc func(CallbackContext, protocol.ProtocolVersionMap) error

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
		Timeout: 10 * time.Second,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

// WithProtocolVersionMap specifies the supported protocol versions
func WithProtocolVersionMap(
	versionMap protocol.ProtocolVersionMap,
) HandshakeOptionFunc {
	return func(c *Config) {
		c.ProtocolVersionMap = versionMap
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

// WithQueryReplyFunc specifies the QueryReply callback function
func WithQueryReplyFunc(queryReplyFunc QueryReplyFunc) HandshakeOptionFunc {
	return func(c *Config) {
		c.QueryReplyFunc = queryReplyFunc
	}
}
