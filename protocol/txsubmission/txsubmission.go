// Copyright 2023 Blink Labs, LLC.
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

package txsubmission

import (
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
)

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

var StateMap = protocol.StateMap{
	stateInit: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeInit,
				NewState: stateIdle,
			},
		},
	},
	stateIdle: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeRequestTxIds,
				NewState: stateTxIdsBlocking,
				// Match if blocking
				MatchFunc: func(msg protocol.Message) bool {
					msgRequestTxIds := msg.(*MsgRequestTxIds)
					return msgRequestTxIds.Blocking
				},
			},
			{
				MsgType:  MessageTypeRequestTxIds,
				NewState: stateTxIdsNonblocking,
				// Metch if non-blocking
				MatchFunc: func(msg protocol.Message) bool {
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
		Agency: protocol.AgencyClient,
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
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeReplyTxIds,
				NewState: stateIdle,
			},
		},
	},
	stateTxs: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeReplyTxs,
				NewState: stateIdle,
			},
		},
	},
	stateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

type TxSubmission struct {
	Client *Client
	Server *Server
}

type Config struct {
	RequestTxIdsFunc RequestTxIdsFunc
	ReplyTxIdsFunc   ReplyTxIdsFunc
	RequestTxsFunc   RequestTxsFunc
	ReplyTxsFunc     ReplyTxsFunc
	DoneFunc         DoneFunc
	InitFunc         InitFunc
	IdleTimeout      time.Duration
}

// Callback function types
type RequestTxIdsFunc func(bool, uint16, uint16) ([]TxIdAndSize, error)
type ReplyTxIdsFunc func(interface{}) error
type RequestTxsFunc func([]TxId) ([]TxBody, error)
type ReplyTxsFunc func(interface{}) error
type DoneFunc func() error
type InitFunc func() error

func New(protoOptions protocol.ProtocolOptions, cfg *Config) *TxSubmission {
	t := &TxSubmission{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return t
}

type TxSubmissionOptionFunc func(*Config)

func NewConfig(options ...TxSubmissionOptionFunc) Config {
	c := Config{
		IdleTimeout: 300 * time.Second,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

func WithRequestTxIdsFunc(
	requestTxIdsFunc RequestTxIdsFunc,
) TxSubmissionOptionFunc {
	return func(c *Config) {
		c.RequestTxIdsFunc = requestTxIdsFunc
	}
}

func WithReplyTxIdsFunc(replyTxIdsFunc ReplyTxIdsFunc) TxSubmissionOptionFunc {
	return func(c *Config) {
		c.ReplyTxIdsFunc = replyTxIdsFunc
	}
}

func WithRequestTxsFunc(requestTxsFunc RequestTxsFunc) TxSubmissionOptionFunc {
	return func(c *Config) {
		c.RequestTxsFunc = requestTxsFunc
	}
}

func WithReplyTxsFunc(replyTxsFunc ReplyTxsFunc) TxSubmissionOptionFunc {
	return func(c *Config) {
		c.ReplyTxsFunc = replyTxsFunc
	}
}

func WithDoneFunc(doneFunc DoneFunc) TxSubmissionOptionFunc {
	return func(c *Config) {
		c.DoneFunc = doneFunc
	}
}

func WithInitFunc(initFunc InitFunc) TxSubmissionOptionFunc {
	return func(c *Config) {
		c.InitFunc = initFunc
	}
}

func WithIdleTimeout(timeout time.Duration) TxSubmissionOptionFunc {
	return func(c *Config) {
		c.IdleTimeout = timeout
	}
}
