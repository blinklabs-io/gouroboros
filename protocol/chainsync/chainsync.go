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

// Package chainsync implements the Ouroboros chain-sync protocol
package chainsync

import (
	"fmt"
	"sync"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/common"
)

// Protocol identifiers
const (
	ProtocolName         = "chain-sync"
	ProtocolIdNtN uint16 = 2
	ProtocolIdNtC uint16 = 5
)

var (
	stateIdle      = protocol.NewState(1, "Idle")
	stateCanAwait  = protocol.NewState(2, "CanAwait")
	stateMustReply = protocol.NewState(3, "MustReply")
	stateIntersect = protocol.NewState(4, "Intersect")
	stateDone      = protocol.NewState(5, "Done")
)

// ChainSync protocol state machine
var StateMap = protocol.StateMap{
	stateIdle: protocol.StateMapEntry{
		Agency:                  protocol.AgencyClient,
		PendingMessageByteLimit: MaxPendingMessageBytes,
		Timeout:                 IdleTimeout, // Timeout for client to send next request
		Transitions: []protocol.StateTransition{
			{
				MsgType:   MessageTypeRequestNext,
				NewState:  stateCanAwait,
				MatchFunc: IncrementPipelineCount,
			},
			{
				MsgType:  MessageTypeFindIntersect,
				NewState: stateIntersect,
			},
			{
				MsgType:  MessageTypeDone,
				NewState: stateDone,
			},
		},
	},
	stateCanAwait: protocol.StateMapEntry{
		Agency:                  protocol.AgencyServer,
		PendingMessageByteLimit: MaxPendingMessageBytes,
		Timeout:                 CanAwaitTimeout, // Timeout for server to provide next block or await
		Transitions: []protocol.StateTransition{
			{
				MsgType:   MessageTypeRequestNext,
				NewState:  stateCanAwait,
				MatchFunc: IncrementPipelineCount,
			},
			{
				MsgType:  MessageTypeAwaitReply,
				NewState: stateMustReply,
			},
			{
				MsgType:   MessageTypeRollForward,
				NewState:  stateIdle,
				MatchFunc: DecrementPipelineCountAndIsEmpty,
			},
			{
				MsgType:   MessageTypeRollForward,
				NewState:  stateCanAwait,
				MatchFunc: DecrementPipelineCountAndIsNotEmpty,
			},
			{
				MsgType:   MessageTypeRollBackward,
				NewState:  stateIdle,
				MatchFunc: DecrementPipelineCountAndIsEmpty,
			},
			{
				MsgType:   MessageTypeRollBackward,
				NewState:  stateCanAwait,
				MatchFunc: DecrementPipelineCountAndIsNotEmpty,
			},
		},
	},
	stateIntersect: protocol.StateMapEntry{
		Agency:                  protocol.AgencyServer,
		PendingMessageByteLimit: MaxPendingMessageBytes,
		Timeout:                 IntersectTimeout, // Timeout for server to respond to intersect request
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeIntersectFound,
				NewState: stateIdle,
			},
			{
				MsgType:  MessageTypeIntersectNotFound,
				NewState: stateIdle,
			},
		},
	},
	stateMustReply: protocol.StateMapEntry{
		Agency:                  protocol.AgencyServer,
		PendingMessageByteLimit: MaxPendingMessageBytes,
		Timeout:                 MustReplyTimeout, // Timeout for server to provide next block
		Transitions: []protocol.StateTransition{
			{
				MsgType:   MessageTypeRollForward,
				NewState:  stateIdle,
				MatchFunc: DecrementPipelineCountAndIsEmpty,
			},
			{
				MsgType:   MessageTypeRollForward,
				NewState:  stateCanAwait,
				MatchFunc: DecrementPipelineCountAndIsNotEmpty,
			},
			{
				MsgType:   MessageTypeRollBackward,
				NewState:  stateIdle,
				MatchFunc: DecrementPipelineCountAndIsEmpty,
			},
			{
				MsgType:   MessageTypeRollBackward,
				NewState:  stateCanAwait,
				MatchFunc: DecrementPipelineCountAndIsNotEmpty,
			},
		},
	},
	stateDone: protocol.StateMapEntry{
		Agency:                  protocol.AgencyNone,
		PendingMessageByteLimit: MaxPendingMessageBytes,
	},
}

type StateContext struct {
	mu            sync.Mutex
	pipelineCount int
}

var IncrementPipelineCount = func(context any, msg protocol.Message) bool {
	s := context.(*StateContext)
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pipelineCount++
	return true
}

var DecrementPipelineCountAndIsEmpty = func(context any, msg protocol.Message) bool {
	s := context.(*StateContext)
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pipelineCount == 1 {
		s.pipelineCount--
		return true
	}
	return false
}

var DecrementPipelineCountAndIsNotEmpty = func(context any, msg protocol.Message) bool {
	s := context.(*StateContext)
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pipelineCount > 1 {
		s.pipelineCount--
		return true
	}
	return false
}

var PipelineIsEmtpy = func(context any, msg protocol.Message) bool {
	s := context.(*StateContext)
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.pipelineCount == 0
}

var PipelineIsNotEmpty = func(context any, msg protocol.Message) bool {
	s := context.(*StateContext)
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.pipelineCount > 0
}

// ChainSync is a wrapper object that holds the client and server instances
type ChainSync struct {
	Client *Client
	Server *Server
}

// Config is used to configure the ChainSync protocol instance
type Config struct {
	RollBackwardFunc   RollBackwardFunc
	RollForwardFunc    RollForwardFunc
	RollForwardRawFunc RollForwardRawFunc
	FindIntersectFunc  FindIntersectFunc
	RequestNextFunc    RequestNextFunc
	IntersectTimeout   time.Duration
	BlockTimeout       time.Duration
	PipelineLimit      int
	RecvQueueSize      int
}

// Protocol limits per Ouroboros Network Specification
const (
	MaxPipelineLimit       = 100    // Max pipelined requests
	MaxRecvQueueSize       = 100    // Max receive queue size (messages)
	DefaultPipelineLimit   = 75     // Default pipeline limit
	DefaultRecvQueueSize   = 75     // Default queue size
	MaxPendingMessageBytes = 102400 // Max pending message bytes (100KB)
)

// Protocol state timeout constants per network specification
const (
	IdleTimeout      = 60 * time.Second  // Timeout for client to send next request
	CanAwaitTimeout  = 300 * time.Second // Timeout for server to provide next block or await
	IntersectTimeout = 5 * time.Second   // Timeout for server to respond to intersect request
	MustReplyTimeout = 300 * time.Second // Timeout for server to provide next block
)

// Callback context
type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

// Callback function types
type (
	RollBackwardFunc   func(CallbackContext, common.Point, Tip) error
	RollForwardFunc    func(CallbackContext, uint, any, Tip) error
	RollForwardRawFunc func(CallbackContext, uint, []byte, Tip) error
)

type (
	FindIntersectFunc func(CallbackContext, []common.Point) (common.Point, Tip, error)
	RequestNextFunc   func(CallbackContext) error
)

// New returns a new ChainSync object
func New(protoOptions protocol.ProtocolOptions, cfg *Config) *ChainSync {
	stateContext := &StateContext{}

	c := &ChainSync{
		Client: NewClient(stateContext, protoOptions, cfg),
		Server: NewServer(stateContext, protoOptions, cfg),
	}
	return c
}

// ChainSyncOptionFunc represents a function used to modify the ChainSync protocol config
type ChainSyncOptionFunc func(*Config)

// NewConfig returns a new ChainSync config object with the provided options
func NewConfig(options ...ChainSyncOptionFunc) Config {
	c := Config{
		PipelineLimit:    DefaultPipelineLimit,
		RecvQueueSize:    DefaultRecvQueueSize,
		IntersectTimeout: 5 * time.Second,
		// We should really use something more useful like 30-60s, but we've seen 55s between blocks
		// in the preview network and almost 240s in preprod
		// https://preview.cexplorer.io/block/cb08a386363a946d2606e912fcd81ffed2bf326cdbc4058297b14471af4f67e9
		// https://preview.cexplorer.io/block/86806dca4ba735b233cbeee6da713bdece36fd41fb5c568f9ef5a3f5cbf572a3
		BlockTimeout: 300 * time.Second,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}

	// Validate configuration against protocol limits
	if err := c.validate(); err != nil {
		panic("invalid ChainSync configuration: " + err.Error())
	}

	return c
}

// validate checks that the configuration values are within protocol limits
func (c *Config) validate() error {
	if c.PipelineLimit < 0 {
		return fmt.Errorf(
			"PipelineLimit %d must be non-negative",
			c.PipelineLimit,
		)
	}
	if c.PipelineLimit > MaxPipelineLimit {
		return fmt.Errorf(
			"PipelineLimit %d exceeds maximum allowed %d",
			c.PipelineLimit,
			MaxPipelineLimit,
		)
	}
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

// WithRollBackwardFunc specifies the RollBackward callback function
func WithRollBackwardFunc(
	rollBackwardFunc RollBackwardFunc,
) ChainSyncOptionFunc {
	return func(c *Config) {
		c.RollBackwardFunc = rollBackwardFunc
	}
}

// WithRollForwardFunc specifies the RollForward callback function. This will provided a parsed header or block
func WithRollForwardFunc(rollForwardFunc RollForwardFunc) ChainSyncOptionFunc {
	return func(c *Config) {
		c.RollForwardFunc = rollForwardFunc
	}
}

// WithRollForwardRawFunc specifies the RollForwardRaw callback function. This will provide the raw header or block
func WithRollForwardRawFunc(
	rollForwardRawFunc RollForwardRawFunc,
) ChainSyncOptionFunc {
	return func(c *Config) {
		c.RollForwardRawFunc = rollForwardRawFunc
	}
}

// WithFindIntersectFunc specifies the FindIntersect callback function
func WithFindIntersectFunc(
	findIntersectFunc FindIntersectFunc,
) ChainSyncOptionFunc {
	return func(c *Config) {
		c.FindIntersectFunc = findIntersectFunc
	}
}

// WithRequestNextFunc specifies the RequestNext callback function
func WithRequestNextFunc(requestNextFunc RequestNextFunc) ChainSyncOptionFunc {
	return func(c *Config) {
		c.RequestNextFunc = requestNextFunc
	}
}

// WithIntersectTimeout specifies the timeout for intersect operations
func WithIntersectTimeout(timeout time.Duration) ChainSyncOptionFunc {
	return func(c *Config) {
		c.IntersectTimeout = timeout
	}
}

// WithBlockTimeout specifies the timeout for block fetch operations
func WithBlockTimeout(timeout time.Duration) ChainSyncOptionFunc {
	return func(c *Config) {
		c.BlockTimeout = timeout
	}
}

// WithPipelineLimit specifies the maximum number of block requests to pipeline
func WithPipelineLimit(limit int) ChainSyncOptionFunc {
	return func(c *Config) {
		if limit < 0 {
			panic(
				fmt.Sprintf(
					"PipelineLimit %d must be non-negative",
					limit,
				),
			)
		}
		if limit > MaxPipelineLimit {
			panic(
				fmt.Sprintf(
					"PipelineLimit %d exceeds maximum %d",
					limit,
					MaxPipelineLimit,
				),
			)
		}
		c.PipelineLimit = limit
	}
}

// WithRecvQueueSize specifies the size of the received messages queue
func WithRecvQueueSize(size int) ChainSyncOptionFunc {
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
