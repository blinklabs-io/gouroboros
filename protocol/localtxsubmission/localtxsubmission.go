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

// Package localtxsubmission implements the Ouroboros local-tx-submission protocol
package localtxsubmission

import (
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Protocol identifiers
const (
	ProtocolName        = "local-tx-submission"
	ProtocolId   uint16 = 6
)

var (
	stateIdle = protocol.NewState(1, "Idle")
	stateBusy = protocol.NewState(2, "Busy")
	stateDone = protocol.NewState(3, "Done")
)

// LocalTxSubmission protocol state machine
var StateMap = protocol.StateMap{
	stateIdle: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeSubmitTx,
				NewState: stateBusy,
			},
		},
	},
	stateBusy: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeAcceptTx,
				NewState: stateIdle,
			},
			{
				MsgType:  MessageTypeRejectTx,
				NewState: stateIdle,
			},
		},
	},
	stateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

// LocalTxSubmission is a wrapper object that holds the client and server instances
type LocalTxSubmission struct {
	Client *Client
	Server *Server
}

// Config is used to configure the LocalTxSubmission protocol instance
type Config struct {
	SubmitTxFunc SubmitTxFunc
	Timeout      time.Duration
}

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// Callback function types
type SubmitTxFunc func(CallbackContext, interface{}) error

// New returns a new LocalTxSubmission object
func New(
	protoOptions protocol.ProtocolOptions,
	cfg *Config,
) *LocalTxSubmission {
	l := &LocalTxSubmission{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return l
}

// LocalTxSubmissionOptionFunc represents a function used to modify the LocalTxSubmission protocol config
type LocalTxSubmissionOptionFunc func(*Config)

// NewConfig returns a new LocalTxSubmission config object with the provided options
func NewConfig(options ...LocalTxSubmissionOptionFunc) Config {
	c := Config{
		Timeout: 30 * time.Second,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

// WithSubmitTxFunc specifies the callback function when a TX is submitted when acting as a server
func WithSubmitTxFunc(submitTxFunc SubmitTxFunc) LocalTxSubmissionOptionFunc {
	return func(c *Config) {
		c.SubmitTxFunc = submitTxFunc
	}
}

// WithTimeout specifies the timeout for a TX submit operation when acting as a client
func WithTimeout(timeout time.Duration) LocalTxSubmissionOptionFunc {
	return func(c *Config) {
		c.Timeout = timeout
	}
}
