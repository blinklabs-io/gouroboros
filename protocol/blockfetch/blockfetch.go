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
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/common"

	"github.com/blinklabs-io/gouroboros/ledger"
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
		Agency: protocol.AgencyClient,
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
		Agency: protocol.AgencyServer,
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
		Agency: protocol.AgencyServer,
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

type BlockFetch struct {
	Client *Client
	Server *Server
}

type Config struct {
	BlockFunc         BlockFunc
	RequestRangeFunc  RequestRangeFunc
	BatchStartTimeout time.Duration
	BlockTimeout      time.Duration
}

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// Callback function types
type BlockFunc func(CallbackContext, ledger.Block) error
type RequestRangeFunc func(CallbackContext, common.Point, common.Point) error

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
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

func WithBlockFunc(blockFunc BlockFunc) BlockFetchOptionFunc {
	return func(c *Config) {
		c.BlockFunc = blockFunc
	}
}

func WithRequestRangeFunc(requestRangeFunc RequestRangeFunc) BlockFetchOptionFunc {
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
