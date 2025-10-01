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
)

const (
	ProtocolName        = "leios-fetch"
	ProtocolId   uint16 = 999 // NOTE: this is a dummy value and will need to be changed
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
				NewState: StateVote,
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

// TODO
type Config struct {
	RequestNextFunc RequestNextFunc
	Timeout         time.Duration
}

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// TODO
// Callback function types
type (
	RequestNextFunc func(CallbackContext) (protocol.Message, error)
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

// TODO
func WithRequestNextFunc(requestNextFunc RequestNextFunc) LeiosFetchOptionFunc {
	return func(c *Config) {
		c.RequestNextFunc = requestNextFunc
	}
}

func WithTimeout(timeout time.Duration) LeiosFetchOptionFunc {
	return func(c *Config) {
		c.Timeout = timeout
	}
}
