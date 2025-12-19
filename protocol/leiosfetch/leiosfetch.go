// Copyright 2025 Blink Labs Software
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

package leiosfetch

import (
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

const (
	ProtocolName        = "leios-fetch"
	ProtocolId   uint16 = 19
)

var (
	StateIdle       = protocol.NewState(1, "Idle")
	StateBlock      = protocol.NewState(2, "Block")
	StateBlockTxs   = protocol.NewState(3, "BlockTxs")
	StateVotes      = protocol.NewState(4, "Votes")
	StateBlockRange = protocol.NewState(5, "BlockRange")
	StateDone       = protocol.NewState(6, "Done")
)

var StateMap = protocol.StateMap{
	StateIdle: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeBlockRequest,
				NewState: StateBlock,
			},
			{
				MsgType:  MessageTypeBlockTxsRequest,
				NewState: StateBlockTxs,
			},
			{
				MsgType:  MessageTypeVotesRequest,
				NewState: StateVotes,
			},
			{
				MsgType:  MessageTypeBlockRangeRequest,
				NewState: StateBlockRange,
			},
			{
				MsgType:  MessageTypeDone,
				NewState: StateDone,
			},
		},
	},
	StateBlock: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeBlock,
				NewState: StateIdle,
			},
		},
	},
	StateBlockTxs: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeBlockTxs,
				NewState: StateIdle,
			},
		},
	},
	StateVotes: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeVotes,
				NewState: StateIdle,
			},
		},
	},
	StateBlockRange: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeNextBlockAndTxsInRange,
				NewState: StateBlockRange,
			},
			{
				MsgType:  MessageTypeLastBlockAndTxsInRange,
				NewState: StateIdle,
			},
		},
	},
	StateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

type LeiosFetch struct {
	Client *Client
	Server *Server
}

type Config struct {
	BlockRequestFunc      BlockRequestFunc
	BlockTxsRequestFunc   BlockTxsRequestFunc
	VotesRequestFunc      VotesRequestFunc
	BlockRangeRequestFunc BlockRangeRequestFunc
	Timeout               time.Duration
}

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// Callback function types
type (
	BlockRequestFunc      func(CallbackContext, uint64, []byte) (protocol.Message, error)
	BlockTxsRequestFunc   func(CallbackContext, uint64, []byte, map[uint16][8]byte) (protocol.Message, error)
	VotesRequestFunc      func(CallbackContext, []MsgVotesRequestVoteId) (protocol.Message, error)
	BlockRangeRequestFunc func(CallbackContext, pcommon.Point, pcommon.Point) error
)

func New(protoOptions protocol.ProtocolOptions, cfg *Config) *LeiosFetch {
	b := &LeiosFetch{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return b
}

type LeiosFetchOptionFunc func(*Config)

func NewConfig(options ...LeiosFetchOptionFunc) Config {
	c := Config{
		Timeout: 5 * time.Second,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

// WithBlockRequestFunc specifies a callback function for BlockRequest messages when acting as a server. The callback is expected to return the proper response message and an error
func WithBlockRequestFunc(
	blockRequestFunc BlockRequestFunc,
) LeiosFetchOptionFunc {
	return func(c *Config) {
		c.BlockRequestFunc = blockRequestFunc
	}
}

// WithBlockTxsRequestFunc specifies a callback function for BlockTxsRequest messages when acting as a server. The callback is expected to return the proper response message and an error
func WithBlockTxsRequestFunc(
	blockTxsRequestFunc BlockTxsRequestFunc,
) LeiosFetchOptionFunc {
	return func(c *Config) {
		c.BlockTxsRequestFunc = blockTxsRequestFunc
	}
}

// WithVotesRequestFunc specifies a callback function for VotesRequest messages when acting as a server. The callback is expected to return the proper response message and an error
func WithVotesRequestFunc(
	votesRequestFunc VotesRequestFunc,
) LeiosFetchOptionFunc {
	return func(c *Config) {
		c.VotesRequestFunc = votesRequestFunc
	}
}

// WithBlockRangeRequestFunc specifies a callback function for BlockRangeRequest messages when acting as a server. The callback is expected to return an error and start an async process
// to generate and send LastBlockAndTxsInRange and NextBlockAndTxsInRange messages that satisfy the requested range
func WithBlockRangeRequestFunc(
	blockRangeRequestFunc BlockRangeRequestFunc,
) LeiosFetchOptionFunc {
	return func(c *Config) {
		c.BlockRangeRequestFunc = blockRangeRequestFunc
	}
}

func WithTimeout(timeout time.Duration) LeiosFetchOptionFunc {
	return func(c *Config) {
		c.Timeout = timeout
	}
}
