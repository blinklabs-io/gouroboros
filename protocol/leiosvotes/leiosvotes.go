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

package leiosvotes

import (
	"errors"
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
)

const (
	ProtocolName        = "leios-votes"
	ProtocolId   uint16 = 20
)

const (
	MaxRequestNextCount     uint64 = 1000
	DefaultRequestNextCount uint64 = 1000
	MaxPipelineLimit               = 100
	DefaultPipelineLimit           = 1
	DefaultTimeout                 = 60 * time.Second
)

var (
	StateIdle = protocol.NewState(1, "Idle")
	StateBusy = protocol.NewState(2, "Busy")
	StateDone = protocol.NewState(3, "Done")
)

var StateMap = protocol.StateMap{
	StateIdle: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:   MessageTypeVotesRequestNext,
				NewState:  StateBusy,
				MatchFunc: matchRequestNext,
			},
			{
				MsgType:  MessageTypeDone,
				NewState: StateDone,
			},
		},
	},
	StateBusy: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:   MessageTypeVote,
				NewState:  StateBusy,
				MatchFunc: matchVoteWithMorePending,
			},
			{
				MsgType:   MessageTypeVote,
				NewState:  StateIdle,
				MatchFunc: matchFinalVote,
			},
		},
	},
	StateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

type stateContext struct {
	tokens uint64
}

func matchRequestNext(ctx any, msg protocol.Message) bool {
	stateCtx, ok := ctx.(*stateContext)
	if !ok {
		return false
	}
	req, ok := msg.(*MsgVotesRequestNext)
	if !ok || req.Count == 0 || req.Count > MaxRequestNextCount {
		return false
	}
	stateCtx.tokens = req.Count
	return true
}

func matchVoteWithMorePending(ctx any, msg protocol.Message) bool {
	stateCtx, ok := ctx.(*stateContext)
	if !ok || msg.Type() != MessageTypeVote || stateCtx.tokens <= 1 {
		return false
	}
	stateCtx.tokens--
	return true
}

func matchFinalVote(ctx any, msg protocol.Message) bool {
	stateCtx, ok := ctx.(*stateContext)
	if !ok || msg.Type() != MessageTypeVote || stateCtx.tokens != 1 {
		return false
	}
	stateCtx.tokens = 0
	return true
}

type LeiosVotes struct {
	Client *Client
	Server *Server
}

type Config struct {
	PipelineLimit    int
	RequestNextCount uint64
	RequestNextFunc  RequestNextFunc
	Timeout          time.Duration
	VoteFunc         VoteFunc
}

type CallbackContext struct {
	ConnectionId connection.ConnectionId
	Client       *Client
	Server       *Server
}

type (
	RequestNextFunc func(CallbackContext, uint64) ([]Vote, error)
	VoteFunc        func(CallbackContext, Vote) error
)

func New(protoOptions protocol.ProtocolOptions, cfg *Config) *LeiosVotes {
	cfg = normalizeConfig(cfg)
	v := &LeiosVotes{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return v
}

type LeiosVotesOptionFunc func(*Config)

func normalizeConfig(cfg *Config) *Config {
	if cfg == nil {
		tmpCfg := NewConfig()
		return &tmpCfg
	}
	ret := *cfg
	if ret.PipelineLimit == 0 {
		ret.PipelineLimit = DefaultPipelineLimit
	}
	if ret.RequestNextCount == 0 {
		ret.RequestNextCount = DefaultRequestNextCount
	}
	if ret.Timeout == 0 {
		ret.Timeout = DefaultTimeout
	}
	if err := ret.validate(); err != nil {
		panic("invalid LeiosVotes configuration: " + err.Error())
	}
	return &ret
}

func NewConfig(options ...LeiosVotesOptionFunc) Config {
	c := Config{
		PipelineLimit:    DefaultPipelineLimit,
		RequestNextCount: DefaultRequestNextCount,
		Timeout:          DefaultTimeout,
	}
	for _, option := range options {
		option(&c)
	}
	if err := c.validate(); err != nil {
		panic("invalid LeiosVotes configuration: " + err.Error())
	}
	return c
}

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
	if c.RequestNextCount == 0 {
		return errors.New("RequestNextCount must be positive")
	}
	if c.RequestNextCount > MaxRequestNextCount {
		return fmt.Errorf(
			"RequestNextCount %d exceeds maximum allowed %d",
			c.RequestNextCount,
			MaxRequestNextCount,
		)
	}
	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	return nil
}

func WithPipelineLimit(limit int) LeiosVotesOptionFunc {
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
					"PipelineLimit %d exceeds maximum allowed %d",
					limit,
					MaxPipelineLimit,
				),
			)
		}
		c.PipelineLimit = limit
	}
}

func WithRequestNextCount(count uint64) LeiosVotesOptionFunc {
	return func(c *Config) {
		if count == 0 {
			panic("RequestNextCount must be positive")
		}
		if count > MaxRequestNextCount {
			panic(
				fmt.Sprintf(
					"RequestNextCount %d exceeds maximum allowed %d",
					count,
					MaxRequestNextCount,
				),
			)
		}
		c.RequestNextCount = count
	}
}

func WithRequestNextFunc(
	requestNextFunc RequestNextFunc,
) LeiosVotesOptionFunc {
	return func(c *Config) {
		c.RequestNextFunc = requestNextFunc
	}
}

func WithTimeout(timeout time.Duration) LeiosVotesOptionFunc {
	return func(c *Config) {
		if timeout <= 0 {
			panic("Timeout must be positive")
		}
		c.Timeout = timeout
	}
}

func WithVoteFunc(voteFunc VoteFunc) LeiosVotesOptionFunc {
	return func(c *Config) {
		c.VoteFunc = voteFunc
	}
}
