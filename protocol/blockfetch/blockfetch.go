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

package blockfetch

import (
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/common"
)

const (
	ProtocolName        = "block-fetch"
	ProtocolId   uint16 = 3
)

var (
	StateIdle      = protocol.NewState(1, "Idle")
	StateBusy      = protocol.NewState(2, "Busy")
	StateStreaming = protocol.NewState(3, "Streaming")
	StateDone      = protocol.NewState(4, "Done")
)

var StateMap = protocol.StateMap{
	StateIdle: protocol.StateMapEntry{
		Agency:                  protocol.AgencyClient,
		PendingMessageByteLimit: MaxPendingMessageBytes,
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
		PendingMessageByteLimit: MaxPendingMessageBytes,
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
		PendingMessageByteLimit: MaxPendingMessageBytes,
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
		Agency:                  protocol.AgencyNone,
		PendingMessageByteLimit: MaxPendingMessageBytes,
	},
}

type BlockFetch struct {
	Client *Client
	Server *Server
}

type Config struct {
	BlockFunc         BlockFunc
	BlockRawFunc      BlockRawFunc
	BatchDoneFunc     BatchDoneFunc
	RequestRangeFunc  RequestRangeFunc
	BatchStartTimeout time.Duration
	BlockTimeout      time.Duration
	RecvQueueSize     int
}

// Protocol limits per Ouroboros Network Specification
const (
	MaxRecvQueueSize       = 512     // Max receive queue size (messages)
	DefaultRecvQueueSize   = 256     // Default queue size
	MaxPendingMessageBytes = 5242880 // Max pending message bytes (5MB)
)

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// Callback function types
type (
	BlockFunc        func(CallbackContext, uint, ledger.Block) error
	BlockRawFunc     func(CallbackContext, uint, []byte) error
	BatchDoneFunc    func(CallbackContext) error
	RequestRangeFunc func(CallbackContext, common.Point, common.Point) error
)

func New(protoOptions protocol.ProtocolOptions, cfg *Config) *BlockFetch {
	b := &BlockFetch{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return b
}

type BlockFetchOptionFunc func(*Config)

func NewConfig(options ...BlockFetchOptionFunc) Config {
	c := Config{
		BatchStartTimeout: 5 * time.Second,
		BlockTimeout:      60 * time.Second,
		RecvQueueSize:     DefaultRecvQueueSize,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}

	// Validate configuration against protocol limits
	if err := c.validate(); err != nil {
		panic("invalid BlockFetch configuration: " + err.Error())
	}

	return c
}

// validate checks that the configuration values are within protocol limits
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

func WithBlockFunc(blockFunc BlockFunc) BlockFetchOptionFunc {
	return func(c *Config) {
		c.BlockFunc = blockFunc
	}
}

func WithBlockRawFunc(blockRawFunc BlockRawFunc) BlockFetchOptionFunc {
	return func(c *Config) {
		c.BlockRawFunc = blockRawFunc
	}
}

func WithBatchDoneFunc(batchDoneFunc BatchDoneFunc) BlockFetchOptionFunc {
	return func(c *Config) {
		c.BatchDoneFunc = batchDoneFunc
	}
}

func WithRequestRangeFunc(
	requestRangeFunc RequestRangeFunc,
) BlockFetchOptionFunc {
	return func(c *Config) {
		c.RequestRangeFunc = requestRangeFunc
	}
}

func WithBatchStartTimeout(timeout time.Duration) BlockFetchOptionFunc {
	return func(c *Config) {
		c.BatchStartTimeout = timeout
	}
}

func WithBlockTimeout(timeout time.Duration) BlockFetchOptionFunc {
	return func(c *Config) {
		c.BlockTimeout = timeout
	}
}

// WithRecvQueueSize specifies the size of the received messages queue
func WithRecvQueueSize(size int) BlockFetchOptionFunc {
	return func(c *Config) {
		if size < 0 {
			panic(
				fmt.Sprintf(
					"RecvQueueSize %d must be non-negative",
					size,
				),
			)
		}
		if size > MaxRecvQueueSize {
			panic(
				fmt.Sprintf(
					"RecvQueueSize %d exceeds maximum %d",
					size,
					MaxRecvQueueSize,
				),
			)
		}
		c.RecvQueueSize = size
	}
}
