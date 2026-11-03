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

// Package localtxmonitor implements the Ouroboros local-tx-monitor protocol
package localtxmonitor

import (
	"sync/atomic"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Protocol identifiers
const (
	ProtocolName        = "local-tx-monitor"
	ProtocolId   uint16 = 9
)

var (
	stateIdle      = protocol.NewState(1, "Idle")
	stateAcquiring = protocol.NewState(2, "Acquiring")
	stateAcquired  = protocol.NewState(3, "Acquired")
	stateBusy      = protocol.NewState(4, "Busy")
	stateDone      = protocol.NewState(5, "Done")
)

// LocalTxMonitor protocol state machine
var StateMap = protocol.StateMap{
	stateIdle: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeAcquire,
				NewState: stateAcquiring,
			},
			{
				MsgType:  MessageTypeDone,
				NewState: stateDone,
			},
		},
	},
	stateAcquiring: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeAcquired,
				NewState: stateAcquired,
			},
		},
	},
	stateAcquired: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeAcquire,
				NewState: stateAcquiring,
			},
			{
				MsgType:  MessageTypeRelease,
				NewState: stateIdle,
			},
			{
				MsgType:   MessageTypeHasTx,
				NewState:  stateBusy,
				MatchFunc: matchQuery,
			},
			{
				MsgType:   MessageTypeNextTx,
				NewState:  stateBusy,
				MatchFunc: matchQuery,
			},
			{
				MsgType:   MessageTypeGetSizes,
				NewState:  stateBusy,
				MatchFunc: matchQuery,
			},
		},
	},
	stateBusy: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:   MessageTypeReplyHasTx,
				NewState:  stateAcquired,
				MatchFunc: matchReply,
			},
			{
				MsgType:   MessageTypeReplyNextTx,
				NewState:  stateAcquired,
				MatchFunc: matchReply,
			},
			{
				MsgType:   MessageTypeReplyGetSizes,
				NewState:  stateAcquired,
				MatchFunc: matchReply,
			},
		},
	},
	stateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

// queryReplyTypes maps each query that hands agency to the server to the
// only reply that may answer it.
var queryReplyTypes = map[uint8]uint8{
	MessageTypeHasTx:    MessageTypeReplyHasTx,
	MessageTypeNextTx:   MessageTypeReplyNextTx,
	MessageTypeGetSizes: MessageTypeReplyGetSizes,
}

// stateContext records the query in flight so that Busy accepts only its
// reply. Each client and server needs its own.
type stateContext struct {
	query atomic.Uint32
}

// matchQuery records the query before the transition grants the server
// agency. Recording the same query again is harmless, so the framework may
// evaluate the transition more than once.
func matchQuery(ctx any, msg protocol.Message) bool {
	stateCtx, ok := ctx.(*stateContext)
	if !ok {
		return false
	}
	stateCtx.query.Store(uint32(msg.Type()))
	return true
}

// matchReply accepts only the reply to the query in flight, so a mismatched
// reply is refused before the transition to Acquired.
func matchReply(ctx any, msg protocol.Message) bool {
	stateCtx, ok := ctx.(*stateContext)
	if !ok {
		return false
	}
	query := uint8(stateCtx.query.Load()) //nolint:gosec // stores a uint8
	replyType, ok := queryReplyTypes[query]
	return ok && replyType == msg.Type()
}

// LocalTxMonitor is a wrapper object that holds the client and server instances
type LocalTxMonitor struct {
	Client *Client
	Server *Server
}

// Config is used to configure the LocalTxMonitor protocol instance
type Config struct {
	GetMempoolFunc GetMempoolFunc
	AcquireTimeout time.Duration
	QueryTimeout   time.Duration
}

// Helper types
type TxAndEraId struct {
	EraId uint
	Tx    []byte
}

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// Callback function types

// GetMempoolFunc returns the mempool snapshot for an Acquire: the slot
// number, the capacity in bytes, and the transactions. The server keeps
// the returned Tx slices without copying them, relaying them for NextTx and
// parsing them for HasTx while the snapshot is held, so the callback must
// not modify them after returning.
type GetMempoolFunc func(CallbackContext) (uint64, uint32, []TxAndEraId, error)

// New returns a new LocalTxMonitor object
func New(protoOptions protocol.ProtocolOptions, cfg *Config) *LocalTxMonitor {
	l := &LocalTxMonitor{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return l
}

// LocalTxMonitorOptionFunc represents a function used to modify the LocalTxMonitor protocol config
type LocalTxMonitorOptionFunc func(*Config)

// NewConfig returns a new LocalTxMonitor config object with the provided options
func NewConfig(options ...LocalTxMonitorOptionFunc) Config {
	c := Config{
		AcquireTimeout: 5 * time.Second,
		QueryTimeout:   30 * time.Second,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

// WithGetMempoolFunc specifies the callback function for retrieving the mempool
func WithGetMempoolFunc(
	getMempoolFunc GetMempoolFunc,
) LocalTxMonitorOptionFunc {
	return func(c *Config) {
		c.GetMempoolFunc = getMempoolFunc
	}
}

// WithAcquireTimeout specifies the timeout for acquire operations when acting as a client
func WithAcquireTimeout(timeout time.Duration) LocalTxMonitorOptionFunc {
	return func(c *Config) {
		c.AcquireTimeout = timeout
	}
}

// WithQueryTimeout specifies the timeout for query operations when acting as a client
func WithQueryTimeout(timeout time.Duration) LocalTxMonitorOptionFunc {
	return func(c *Config) {
		c.QueryTimeout = timeout
	}
}
