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
		Agency:                  protocol.AgencyClient,
		PendingMessageByteLimit: MaxPendingMessageBytes,
		Timeout:                 InitTimeout, // Timeout for client to send init message
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeInit,
				NewState: stateIdle,
			},
		},
	},
	stateIdle: protocol.StateMapEntry{
		Agency:                  protocol.AgencyServer,
		PendingMessageByteLimit: MaxPendingMessageBytes,
		Timeout:                 IdleTimeout, // Timeout for server to send tx request when idle
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeRequestTxIds,
				NewState: stateTxIdsBlocking,
				// Match if blocking
				MatchFunc: func(context any, msg protocol.Message) bool {
					msgRequestTxIds := msg.(*MsgRequestTxIds)
					return msgRequestTxIds.Blocking
				},
			},
			{
				MsgType:  MessageTypeRequestTxIds,
				NewState: stateTxIdsNonblocking,
				// Metch if non-blocking
				MatchFunc: func(context any, msg protocol.Message) bool {
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
		Agency:                  protocol.AgencyClient,
		PendingMessageByteLimit: MaxPendingMessageBytes,
		Timeout:                 TxIdsBlockingTimeout, // No timeout per spec: client blocks until tx available
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
		Agency:                  protocol.AgencyClient,
		PendingMessageByteLimit: MaxPendingMessageBytes,
		Timeout:                 TxIdsNonblockingTimeout, // Timeout for client to reply with tx IDs (non-blocking)
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeReplyTxIds,
				NewState: stateIdle,
			},
		},
	},
	stateTxs: protocol.StateMapEntry{
		Agency:                  protocol.AgencyClient,
		PendingMessageByteLimit: MaxPendingMessageBytes,
		Timeout:                 TxsTimeout, // Timeout for client to reply with full transactions
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeReplyTxs,
				NewState: stateIdle,
			},
		},
	},
	stateDone: protocol.StateMapEntry{
		Agency:                  protocol.AgencyNone,
		PendingMessageByteLimit: MaxPendingMessageBytes,
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
	DoneFunc         DoneFunc
}

// Protocol limits per Ouroboros Network Specification
const (
	MaxRequestCount     = 65535 // Max transactions per request (uint16)
	MaxAckCount         = 65535 // Max transaction acks (uint16)
	DefaultRequestLimit = 1000  // Default request limit
	DefaultAckLimit     = 1000  // Default ack limit
)

// Pending-message byte limits. Protocol.readLoop rejects an oversized single
// message and applies inbound backpressure only while a state's
// PendingMessageByteLimit is nonzero, and Protocol.SendMessage checks the
// outbound queue against it on the same condition. TxSubmission is
// node-to-node, so a zero limit leaves an untrusted peer unbounded on both
// paths.
const (
	// MaxTxSizeBytes is the largest MsgReplyTxs transaction body a peer may
	// send, matching max_TX_SIZE in the reference implementation
	// (ouroboros-network, Ouroboros.Network.TxSubmission.Inbound.V2.Policy).
	// It is deliberately larger than any Cardano protocol maxTxSize to date,
	// because a peer's reply is bounded by the wire limit rather than by the
	// era's protocol parameters.
	MaxTxSizeBytes = 65540
	// TxIdReplyEntryBytes is the wire cost of one (txId, size) pair in a
	// MsgReplyTxIds: 4 bytes of CBOR structure, a 34-byte transaction ID and
	// a 6-byte size, per the reference implementation's derivation of the
	// same limit.
	TxIdReplyEntryBytes = 44
	// MaxUnackedTxIds is the number of transaction IDs a peer may leave
	// unacknowledged, and therefore the number of transactions it can have
	// in flight. It matches txSubmissionMaxUnacked in the reference
	// implementation. MaxRequestCount and MaxAckCount are the uint16 wire
	// ranges of the count fields, not an in-flight window, so they cannot
	// serve as the multiplier here.
	MaxUnackedTxIds = 10
	// MaxPendingMessageBytes bounds pending message bytes in every
	// TxSubmission state: a full unacknowledged window of maximum-size
	// transactions plus the tx-id reply that announced them, with the
	// reference implementation's 10% safety margin. This is the value
	// cardano-node enforces as its own tx-submission mux ingress limit, so a
	// conforming peer never exceeds it.
	MaxPendingMessageBytes = MaxUnackedTxIds *
		(TxIdReplyEntryBytes + MaxTxSizeBytes) * 11 / 10
)

// Protocol state timeout constants per Ouroboros Network Specification (Table 3.11).
const (
	InitTimeout             = time.Duration(0) // No timeout per spec
	IdleTimeout             = time.Duration(0) // No timeout per spec
	TxIdsBlockingTimeout    = time.Duration(0) // No timeout per spec (blocking waits indefinitely)
	TxIdsNonblockingTimeout = 10 * time.Second // Timeout for client to reply with tx IDs (non-blocking)
	TxsTimeout              = 10 * time.Second // Timeout for client to reply with full transactions
)

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
	DoneChan     <-chan struct{}
}

// Callback function types
type (
	RequestTxIdsFunc func(CallbackContext, bool, uint16, uint16) ([]TxIdAndSize, error)
	RequestTxsFunc   func(CallbackContext, []TxId) ([]TxBody, error)
	InitFunc         func(CallbackContext) error
	DoneFunc         func(CallbackContext) error
)

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
	c := Config{}
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

// WithDoneFunc specifies the Done callback function
func WithDoneFunc(doneFunc DoneFunc) TxSubmissionOptionFunc {
	return func(c *Config) {
		c.DoneFunc = doneFunc
	}
}
