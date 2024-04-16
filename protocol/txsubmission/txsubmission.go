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

// Package txsubmission implements the Ouroboros TxSubmission protocol
package txsubmission

import (
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Protocol identifiers
const (
	ProtocolName        = "tx-submission"
	ProtocolId   uint16 = 4
)

var (
	stateInit             = protocol.NewState(1, "Init")
	stateIdle             = protocol.NewState(2, "Idle")
	stateTxIdsBlocking    = protocol.NewState(3, "TxIdsBlocking")
	stateTxIdsNonblocking = protocol.NewState(4, "TxIdsNonBlocking")
	stateTxs              = protocol.NewState(5, "Txs")
	stateDone             = protocol.NewState(6, "Done")
)

// TxSubmission protocol state machine
var StateMap = protocol.StateMap{
	stateInit: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeInit,
				NewState: stateIdle,
			},
		},
	},
	stateIdle: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeRequestTxIds,
				NewState: stateTxIdsBlocking,
				// Match if blocking
				MatchFunc: func(context interface{}, msg protocol.Message) bool {
					msgRequestTxIds := msg.(*MsgRequestTxIds)
					return msgRequestTxIds.Blocking
				},
			},
			{
				MsgType:  MessageTypeRequestTxIds,
				NewState: stateTxIdsNonblocking,
				// Metch if non-blocking
				MatchFunc: func(context interface{}, msg protocol.Message) bool {
					msgRequestTxIds := msg.(*MsgRequestTxIds)
					return !msgRequestTxIds.Blocking
				},
			},
			{
				MsgType:  MessageTypeRequestTxs,
				NewState: stateTxs,
			},
		},
	},
	stateTxIdsBlocking: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeReplyTxIds,
				NewState: stateIdle,
			},
			{
				MsgType:  MessageTypeDone,
				NewState: stateDone,
			},
		},
	},
	stateTxIdsNonblocking: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeReplyTxIds,
				NewState: stateIdle,
			},
		},
	},
	stateTxs: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeReplyTxs,
				NewState: stateIdle,
			},
		},
	},
	stateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

// TxSubmission is a wrapper object that holds the client and server instances
type TxSubmission struct {
	Client *Client
	Server *Server
}

// Config is used to configure the TxSubmission protocol instance
type Config struct {
	RequestTxIdsFunc RequestTxIdsFunc
	RequestTxsFunc   RequestTxsFunc
	InitFunc         InitFunc
	IdleTimeout      time.Duration
}

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// Callback function types
type RequestTxIdsFunc func(CallbackContext, bool, uint16, uint16) ([]TxIdAndSize, error)
type RequestTxsFunc func(CallbackContext, []TxId) ([]TxBody, error)
type InitFunc func(CallbackContext) error

// New returns a new TxSubmission object
func New(protoOptions protocol.ProtocolOptions, cfg *Config) *TxSubmission {
	t := &TxSubmission{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return t
}

// TxSubmissionOptionFunc represents a function used to modify the TxSubmission protocol config
type TxSubmissionOptionFunc func(*Config)

// NewConfig returns a new TxSubmission config object with the provided options
func NewConfig(options ...TxSubmissionOptionFunc) Config {
	c := Config{
		IdleTimeout: 300 * time.Second,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

// WithRequestTxIdsFunc specifies the RequestTxIds callback function
func WithRequestTxIdsFunc(
	requestTxIdsFunc RequestTxIdsFunc,
) TxSubmissionOptionFunc {
	return func(c *Config) {
		c.RequestTxIdsFunc = requestTxIdsFunc
	}
}

// WithRequestTxsFunc specifies the RequestTxs callback function
func WithRequestTxsFunc(requestTxsFunc RequestTxsFunc) TxSubmissionOptionFunc {
	return func(c *Config) {
		c.RequestTxsFunc = requestTxsFunc
	}
}

// WithInitFunc specifies the Init callback function
func WithInitFunc(initFunc InitFunc) TxSubmissionOptionFunc {
	return func(c *Config) {
		c.InitFunc = initFunc
	}
}

// WithIdleTimeout specifies the timeout for waiting for new transactions from the remote node's mempool
func WithIdleTimeout(timeout time.Duration) TxSubmissionOptionFunc {
	return func(c *Config) {
		c.IdleTimeout = timeout
	}
}
