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

// Package blockfetch implements the Ouroboros Block Fetch mini-protocol.
// It provides client and server implementations for requesting and serving
// blocks over the network according to the Cardano Ouroboros specification.
package blockfetch

import (
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/pipeline"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// ProtocolName is the name of the Block Fetch protocol.
const ProtocolName = "block-fetch"

// ProtocolId is the unique protocol identifier for Block Fetch.
const ProtocolId uint16 = 3

// StateIdle represents the Idle state in the Block Fetch protocol.
var StateIdle = protocol.NewState(1, "Idle")

// StateBusy represents the Busy state in the Block Fetch protocol.
var StateBusy = protocol.NewState(2, "Busy")

// StateStreaming represents the Streaming state in the Block Fetch protocol.
var StateStreaming = protocol.NewState(3, "Streaming")

// StateDone represents the Done state in the Block Fetch protocol.
var StateDone = protocol.NewState(4, "Done")

// StateMap defines the state transitions and agency for the Block Fetch protocol.
var StateMap = protocol.StateMap{
	StateIdle: protocol.StateMapEntry{
		Agency:                  protocol.AgencyClient,
		PendingMessageByteLimit: IdleMaxPendingMessageBytes,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeRequestRange,
				NewState: StateBusy,
			},
			{
				MsgType:  MessageTypeClientDone,
				NewState: StateDone,
			},
		},
	},
	StateBusy: protocol.StateMapEntry{
		Agency:                  protocol.AgencyServer,
		PendingMessageByteLimit: BusyMaxPendingMessageBytes,
		Timeout:                 BusyTimeout, // Timeout for server to start batch or respond no blocks
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeStartBatch,
				NewState: StateStreaming,
			},
			{
				MsgType:  MessageTypeNoBlocks,
				NewState: StateIdle,
			},
		},
	},
	StateStreaming: protocol.StateMapEntry{
		Agency:                  protocol.AgencyServer,
		PendingMessageByteLimit: StreamingMaxPendingMessageBytes,
		Timeout:                 StreamingTimeout, // Timeout for server to send next block in batch
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeBlock,
				NewState: StateStreaming,
			},
			{
				MsgType:  MessageTypeBatchDone,
				NewState: StateIdle,
			},
		},
	},
	StateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

// BlockFetch provides a combined client and server for the Block Fetch protocol.
type BlockFetch struct {
	Client *Client // Block Fetch client
	Server *Server // Block Fetch server
}

// Config holds configuration options for the Block Fetch protocol.
type Config struct {
	BlockFunc           BlockFunc               // Callback for decoded blocks
	BlockRawFunc        BlockRawFunc            // Callback for raw block data
	BatchDoneFunc       BatchDoneFunc           // Callback when a batch is done
	RangeDoneFunc       RangeDoneFunc           // Callback when a pipelined range request completes
	RequestRangeFunc    RequestRangeFunc        // Callback for range requests
	BatchStartTimeout   time.Duration           // Timeout for starting a batch
	BlockTimeout        time.Duration           // Timeout for receiving a block
	RecvQueueSize       int                     // Size of the receive queue
	SkipBlockValidation bool                    // Skip block validation during parsing
	Pipeline            *pipeline.BlockPipeline // Pipeline enables the block processing pipeline for batch operations
	// RequestPipelining allows the client to keep more than one
	// MsgRequestRange outstanding through Client.RequestRange. This is
	// protocol request pipelining and is unrelated to Pipeline above, which
	// is a processing pipeline for blocks that have already been received.
	RequestPipelining bool
	// MaxInFlightBytes bounds the total expected size of the range requests
	// a pipelining client keeps outstanding. Zero means
	// DefaultMaxInFlightBytes.
	MaxInFlightBytes uint64
}

// MaxRecvQueueSize is the maximum allowed receive queue size (messages).
const MaxRecvQueueSize = 512

// DefaultRecvQueueSize is the default receive queue size.
const DefaultRecvQueueSize = 384

// StreamingMaxPendingMessageBytes is the maximum allowed pending message bytes when in the Streaming state
const StreamingMaxPendingMessageBytes = 2500000

// IdleMaxPendingMessageBytes is the maximum allowed pending message bytes
// in the Idle state. Only control messages (MsgRequestRange, MsgClientDone)
// are sent in this state, so the limit can be small.
const IdleMaxPendingMessageBytes = 65535

// PipelinedIdleMaxPendingMessageBytes is the pending message byte limit used
// for the Idle state by a client with RequestPipelining enabled. With more
// than one request outstanding, the peer's MsgBlock for the next request can
// arrive while the local state machine is momentarily back in Idle, between
// the previous MsgBatchDone and the deferred transition for the next
// MsgRequestRange. The Idle limit therefore has to admit a block, for the
// same reason BusyMaxPendingMessageBytes does.
const PipelinedIdleMaxPendingMessageBytes = StreamingMaxPendingMessageBytes

// BusyMaxPendingMessageBytes is the maximum allowed pending message bytes
// in the Busy state. This must match StreamingMaxPendingMessageBytes because
// MsgBlock can arrive while the protocol state machine is still in Busy
// (before recvLoop processes MsgStartBatch to transition to Streaming),
// creating a race between muxerRecvLoop limit checks and state transitions.
const BusyMaxPendingMessageBytes = StreamingMaxPendingMessageBytes

// DefaultRequestExpectedBytes is the size assumed for a range request whose
// caller did not estimate one. It is one maximum-size mainnet block body
// (maxBlockBodySize = 88 KiB), which makes DefaultMaxInFlightBytes degrade to
// cardano-node's blockFetchPipeliningMax of 100 outstanding requests for
// callers that cannot estimate.
const DefaultRequestExpectedBytes uint64 = 88 * 1024

// DefaultMaxInFlightBytes is the default bound on the total expected size of
// outstanding pipelined range requests. It matches the ingress allowance
// cardano-node sizes for block-fetch in blockFetchProtocolLimits:
// blockFetchPipeliningMax (100) times the maximum block body size (88 KiB).
const DefaultMaxInFlightBytes uint64 = 100 * DefaultRequestExpectedBytes

// BusyTimeout is the timeout for the server to start a batch or respond no blocks.
const BusyTimeout = 60 * time.Second

// StreamingTimeout is the timeout for the server to send the next block in a batch.
const StreamingTimeout = 60 * time.Second

// CallbackContext provides context for Block Fetch callbacks.
type CallbackContext struct {
	ConnectionId connection.ConnectionId // Connection ID
	Client       *Client                 // Client instance (if applicable)
	Server       *Server                 // Server instance (if applicable)
	// RequestId identifies the client range request that produced this
	// callback. Requests are numbered from 1 in the order they were sent,
	// and block-fetch responses are ordered, so this attributes each block
	// and completion to the MsgRequestRange that asked for it. It is zero
	// for server-side callbacks.
	RequestId uint64
}

// BlockFunc is a callback for handling decoded blocks.
type BlockFunc func(CallbackContext, uint, ledger.Block) error

// BlockRawFunc is a callback for handling raw block data.
type BlockRawFunc func(CallbackContext, uint, []byte) error

// BatchDoneFunc is a callback invoked when a batch is complete. It is not
// used for requests made through Client.RequestRange, which report
// completion through RangeDoneFunc instead.
type BatchDoneFunc func(CallbackContext) error

// RangeDoneFunc is a callback invoked exactly once for each request made
// through Client.RequestRange. The error is nil when the peer completed the
// batch, and otherwise reports why the request will not be completed: the
// range was unavailable, the peer violated the protocol, or the protocol shut
// down with the request still outstanding.
type RangeDoneFunc func(CallbackContext, error) error

// RequestRangeFunc is a callback for handling block range requests.
type RequestRangeFunc func(CallbackContext, pcommon.Point, pcommon.Point) error

// New creates a new BlockFetch instance with the given protocol options and configuration.
func New(protoOptions protocol.ProtocolOptions, cfg *Config) *BlockFetch {
	b := &BlockFetch{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return b
}

// BlockFetchOptionFunc is a function that modifies a BlockFetch Config.
type BlockFetchOptionFunc func(*Config)

// NewConfig creates a new Config for Block Fetch, applying any provided option functions.
// It returns an error if the resulting configuration is invalid.
func NewConfig(options ...BlockFetchOptionFunc) (Config, error) {
	c := Config{
		BatchStartTimeout: 5 * time.Second,
		BlockTimeout:      60 * time.Second,
		RecvQueueSize:     DefaultRecvQueueSize,
		MaxInFlightBytes:  DefaultMaxInFlightBytes,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}

	// Validate configuration against protocol limits
	if err := c.validate(); err != nil {
		return Config{}, fmt.Errorf("invalid BlockFetch configuration: %w", err)
	}

	return c, nil
}

// validate checks that the configuration values are within protocol limits.
func (c *Config) validate() error {
	if c.RecvQueueSize < 0 {
		return fmt.Errorf(
			"RecvQueueSize %d must be non-negative",
			c.RecvQueueSize,
		)
	}
	if c.RecvQueueSize > MaxRecvQueueSize {
		return fmt.Errorf(
			"RecvQueueSize %d exceeds maximum allowed %d",
			c.RecvQueueSize,
			MaxRecvQueueSize,
		)
	}
	return nil
}

// WithBlockFunc sets the BlockFunc callback in the Config.
func WithBlockFunc(blockFunc BlockFunc) BlockFetchOptionFunc {
	return func(c *Config) {
		c.BlockFunc = blockFunc
	}
}

// WithBlockRawFunc sets the BlockRawFunc callback in the Config.
func WithBlockRawFunc(blockRawFunc BlockRawFunc) BlockFetchOptionFunc {
	return func(c *Config) {
		c.BlockRawFunc = blockRawFunc
	}
}

// WithRangeDoneFunc sets the RangeDoneFunc callback in the Config.
func WithRangeDoneFunc(rangeDoneFunc RangeDoneFunc) BlockFetchOptionFunc {
	return func(c *Config) {
		c.RangeDoneFunc = rangeDoneFunc
	}
}

// WithRequestPipelining enables or disables client request pipelining, which
// allows Client.RequestRange to keep multiple MsgRequestRange outstanding.
func WithRequestPipelining(enabled bool) BlockFetchOptionFunc {
	return func(c *Config) {
		c.RequestPipelining = enabled
	}
}

// WithMaxInFlightBytes sets the bound on the total expected size of
// outstanding pipelined range requests. Zero selects
// DefaultMaxInFlightBytes.
func WithMaxInFlightBytes(maxBytes uint64) BlockFetchOptionFunc {
	return func(c *Config) {
		c.MaxInFlightBytes = maxBytes
	}
}

// WithBatchDoneFunc sets the BatchDoneFunc callback in the Config.
func WithBatchDoneFunc(batchDoneFunc BatchDoneFunc) BlockFetchOptionFunc {
	return func(c *Config) {
		c.BatchDoneFunc = batchDoneFunc
	}
}

// WithRequestRangeFunc sets the RequestRangeFunc callback in the Config.
func WithRequestRangeFunc(
	requestRangeFunc RequestRangeFunc,
) BlockFetchOptionFunc {
	return func(c *Config) {
		c.RequestRangeFunc = requestRangeFunc
	}
}

// WithBatchStartTimeout sets the batch start timeout in the Config.
func WithBatchStartTimeout(timeout time.Duration) BlockFetchOptionFunc {
	return func(c *Config) {
		c.BatchStartTimeout = timeout
	}
}

// WithBlockTimeout sets the block timeout in the Config.
func WithBlockTimeout(timeout time.Duration) BlockFetchOptionFunc {
	return func(c *Config) {
		c.BlockTimeout = timeout
	}
}

// WithRecvQueueSize specifies the size of the received messages queue in the Config.
// Validation is deferred to NewConfig; invalid values are caught there.
func WithRecvQueueSize(size int) BlockFetchOptionFunc {
	return func(c *Config) {
		c.RecvQueueSize = size
	}
}

// WithPipeline sets the block processing pipeline in the Config.
// When a pipeline is configured, received blocks are submitted to the pipeline
// for parallel decoding, validation, and ordered application instead of being
// processed synchronously through callbacks.
func WithPipeline(p *pipeline.BlockPipeline) BlockFetchOptionFunc {
	return func(c *Config) {
		c.Pipeline = p
	}
}
