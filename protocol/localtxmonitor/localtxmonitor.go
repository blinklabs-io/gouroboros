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
	"context"
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
			{
				MsgType:   MessageTypeGetMeasures,
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
			{
				MsgType:   MessageTypeReplyGetMeasures,
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
	MessageTypeHasTx:       MessageTypeReplyHasTx,
	MessageTypeNextTx:      MessageTypeReplyNextTx,
	MessageTypeGetSizes:    MessageTypeReplyGetSizes,
	MessageTypeGetMeasures: MessageTypeReplyGetMeasures,
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
	GetMempoolFunc   GetMempoolFunc
	AwaitMempoolFunc AwaitMempoolFunc
	AcquireTimeout   time.Duration
	QueryTimeout     time.Duration
}

// Helper types
type TxAndEraId struct {
	EraId uint
	Tx    []byte
}

// MempoolMeasures is the answer to GetMeasures: the number of transactions
// and, by measure name, their size against the mempool's capacity.
type MempoolMeasures struct {
	TxCount  uint32
	Measures map[string]MeasureSizeAndCapacity
}

// MeasureTransactionBytes is the measure of transaction sizes in bytes, the
// name the reference node uses.
const MeasureTransactionBytes = "transaction_bytes"

// getMeasuresMinVersion is the first node-to-client version that has
// MsgGetMeasures and MsgReplyGetMeasures.
const getMeasuresMinVersion = 20

// supportsGetMeasures reports whether the negotiated version has
// MsgGetMeasures. A version below the node-to-client offset, as an
// unconfigured test uses, wraps around and counts as supported, the same as
// the local-state-query version checks.
func supportsGetMeasures(version uint16) bool {
	return version-protocol.ProtocolVersionNtCOffset >= getMeasuresMinVersion
}

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
	// ctx is returned by Context.
	ctx context.Context
}

// Context returns the context of this callback. A callback that stops its
// own protocol, or closes its own connection, passes it to that Stop or
// Close: the call then signals shutdown and returns without waiting for the
// goroutine running the callback. It carries no deadline and is never
// cancelled. It is context.Background for a CallbackContext that the package
// did not build.
func (c CallbackContext) Context() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

// withOwner returns c with the context of a callback that p runs.
func (c CallbackContext) withOwner(p *protocol.Protocol) CallbackContext {
	c.ctx = p.CallbackContext()
	return c
}

// Callback function types

// GetMempoolFunc returns the mempool snapshot for an Acquire: the slot
// number, the capacity in bytes, and the transactions. The server keeps
// the returned Tx slices without copying them, relaying them for NextTx and
// parsing them for HasTx while the snapshot is held, so the callback must
// not modify them after returning.
type GetMempoolFunc func(CallbackContext) (uint64, uint32, []TxAndEraId, error)

// AwaitMempoolFunc is called by the server for MsgAwaitAcquire, an acquire
// sent while the client holds a snapshot, with that snapshot's slot number
// and transactions. It blocks until the mempool differs from them, then
// returns nil and the server takes the new snapshot from GetMempoolFunc. The
// reference node waits in the same way, treating a snapshot as unchanged
// while its slot and the identity of its transactions are. ctx ends when the
// server stops or the connection closes; nothing is sent then. Any other
// error fails the connection. The specification sets no time limit on the
// wait.
//
// Without it the server answers MsgAwaitAcquire at once with the current
// mempool, which may be the snapshot the client already holds.
type AwaitMempoolFunc func(
	ctx context.Context,
	callbackContext CallbackContext,
	slotNo uint64,
	txs []TxAndEraId,
) error

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
	// No default timeouts: the specification sets none for node-to-client
	// protocols, and the reference defines no time limits for this one.
	c := Config{}
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

// WithAwaitMempoolFunc specifies the callback the server uses to wait for
// the mempool to change before answering MsgAwaitAcquire
func WithAwaitMempoolFunc(
	awaitMempoolFunc AwaitMempoolFunc,
) LocalTxMonitorOptionFunc {
	return func(c *Config) {
		c.AwaitMempoolFunc = awaitMempoolFunc
	}
}

// WithAcquireTimeout specifies the timeout for acquire operations when acting
// as a client. There is none by default: the reference sets none, and a node
// answers a re-acquire only once its mempool changes.
func WithAcquireTimeout(timeout time.Duration) LocalTxMonitorOptionFunc {
	return func(c *Config) {
		c.AcquireTimeout = timeout
	}
}

// WithQueryTimeout specifies the timeout for query operations when acting as
// a client. There is none by default, as in the reference.
func WithQueryTimeout(timeout time.Duration) LocalTxMonitorOptionFunc {
	return func(c *Config) {
		c.QueryTimeout = timeout
	}
}
