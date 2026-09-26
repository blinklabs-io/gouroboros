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

// Package perasvotes implements the Peras ObjectDiffusion mini-protocol.
package perasvotes

import (
	"errors"
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
)

const (
	// ProtocolName is the muxer name for Peras vote diffusion.
	ProtocolName = "peras-vote-diffusion"
	// ProtocolId is the assigned node-to-node mini-protocol number.
	ProtocolId = 17

	// DefaultMaxObjectsUnacknowledged is the default ID window.
	DefaultMaxObjectsUnacknowledged uint16 = 50
	// MaxObjectsUnacknowledged bounds the configurable outstanding ID window.
	MaxObjectsUnacknowledged uint16 = 1000
	// DefaultTimeout bounds non-blocking ID and object requests.
	DefaultTimeout = 5 * time.Second
)

var (
	// StateInit is the state before the client sends MsgInit.
	StateInit = protocol.NewState(1, "Init")
	// StateIdle is where the client can request vote IDs or objects.
	StateIdle = protocol.NewState(2, "Idle")
	// StateObjectIDsBlocking waits for a non-empty ID reply.
	StateObjectIDsBlocking = protocol.NewState(3, "ObjectIDsBlocking")
	// StateObjectIDsNonBlocking waits for an ID reply that may be empty.
	StateObjectIDsNonBlocking = protocol.NewState(4, "ObjectIDsNonBlocking")
	// StateObjects waits for a reply to a vote object request.
	StateObjects = protocol.NewState(5, "Objects")
	// StateDone is the terminal state.
	StateDone = protocol.NewState(6, "Done")
)

// StateMap follows the ObjectDiffusion state machine in ouroboros-network.
var StateMap = protocol.StateMap{
	StateInit: {
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{{
			MsgType: MessageTypeInit, NewState: StateIdle,
		}},
	},
	StateIdle: {
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType: MessageTypeRequestObjectIDs, NewState: StateObjectIDsBlocking,
				MatchFunc: matchBlockingObjectIDsRequest,
			},
			{
				MsgType: MessageTypeRequestObjectIDs, NewState: StateObjectIDsNonBlocking,
				MatchFunc: matchNonBlockingObjectIDsRequest,
			},
			{
				MsgType: MessageTypeRequestObjects, NewState: StateObjects,
				MatchFunc: matchRequestObjects,
			},
			{MsgType: MessageTypeDone, NewState: StateDone},
		},
	},
	StateObjectIDsBlocking: {
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{{
			MsgType: MessageTypeReplyObjectIDs, NewState: StateIdle,
			MatchFunc: matchReplyObjectIDs,
		}},
	},
	StateObjectIDsNonBlocking: {
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{{
			MsgType: MessageTypeReplyObjectIDs, NewState: StateIdle,
			MatchFunc: matchReplyObjectIDs,
		}},
	},
	StateObjects: {
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{{
			MsgType: MessageTypeReplyObjects, NewState: StateIdle,
			MatchFunc: matchReplyObjects,
		}},
	},
	StateDone: {Agency: protocol.AgencyNone},
}

type stateContext struct {
	maxObjectsUnacknowledged uint16
	requestCount             uint16
	requestBlocking          bool
	objectIDsRequestPending  bool
	outstanding              []VoteID
	requested                map[VoteID]struct{}
	requestObjects           []VoteID
}

func newStateContext(max uint16) *stateContext {
	return &stateContext{
		maxObjectsUnacknowledged: max,
		requested:                make(map[VoteID]struct{}),
	}
}

func matchBlockingObjectIDsRequest(ctx any, msg protocol.Message) bool {
	return matchObjectIDsRequest(ctx, msg, true)
}

func matchNonBlockingObjectIDsRequest(ctx any, msg protocol.Message) bool {
	return matchObjectIDsRequest(ctx, msg, false)
}

func matchObjectIDsRequest(ctx any, msg protocol.Message, blocking bool) bool {
	stateCtx, ok := ctx.(*stateContext)
	if !ok {
		return false
	}
	req, ok := msg.(*MsgRequestObjectIDs)
	if !ok || req.Blocking != blocking ||
		(blocking && req.RequestCount == 0) ||
		(!blocking && req.AckCount == 0 && req.RequestCount == 0) ||
		int(req.AckCount) > len(stateCtx.outstanding) {
		return false
	}
	remaining := len(stateCtx.outstanding) - int(req.AckCount)
	if blocking != (remaining == 0) ||
		remaining+int(req.RequestCount) > int(stateCtx.maxObjectsUnacknowledged) {
		return false
	}
	for _, id := range stateCtx.outstanding[:req.AckCount] {
		delete(stateCtx.requested, id)
	}
	stateCtx.outstanding = append(
		[]VoteID(nil),
		stateCtx.outstanding[req.AckCount:]...,
	)
	stateCtx.requestCount = req.RequestCount
	stateCtx.requestBlocking = blocking
	stateCtx.objectIDsRequestPending = true
	return true
}

func matchReplyObjectIDs(ctx any, msg protocol.Message) bool {
	stateCtx, ok := ctx.(*stateContext)
	if !ok {
		return false
	}
	reply, ok := msg.(*MsgReplyObjectIDs)
	if !ok || !stateCtx.objectIDsRequestPending ||
		len(reply.ObjectIDs) > int(stateCtx.requestCount) ||
		(stateCtx.requestBlocking && len(reply.ObjectIDs) == 0) ||
		len(reply.ObjectIDs) >
			int(stateCtx.maxObjectsUnacknowledged)-len(stateCtx.outstanding) {
		return false
	}
	seen := make(map[VoteID]struct{},
		len(stateCtx.outstanding)+len(reply.ObjectIDs),
	)
	for _, id := range stateCtx.outstanding {
		seen[id] = struct{}{}
	}
	for _, id := range reply.ObjectIDs {
		if _, exists := seen[id]; exists {
			return false
		}
		seen[id] = struct{}{}
	}
	stateCtx.outstanding = append(stateCtx.outstanding, reply.ObjectIDs...)
	stateCtx.requestCount = 0
	stateCtx.objectIDsRequestPending = false
	return true
}

func matchRequestObjects(ctx any, msg protocol.Message) bool {
	stateCtx, ok := ctx.(*stateContext)
	if !ok {
		return false
	}
	req, ok := msg.(*MsgRequestObjects)
	if !ok || len(req.ObjectIDs) == 0 {
		return false
	}
	seen := make(map[VoteID]struct{}, len(req.ObjectIDs))
	for _, id := range req.ObjectIDs {
		if _, exists := stateCtx.requested[id]; exists {
			return false
		}
		if _, exists := seen[id]; exists ||
			!containsVoteID(stateCtx.outstanding, id) {
			return false
		}
		seen[id] = struct{}{}
	}
	for id := range seen {
		stateCtx.requested[id] = struct{}{}
	}
	stateCtx.requestObjects = append([]VoteID(nil), req.ObjectIDs...)
	return true
}

func matchReplyObjects(ctx any, msg protocol.Message) bool {
	stateCtx, ok := ctx.(*stateContext)
	if !ok {
		return false
	}
	reply, ok := msg.(*MsgReplyObjects)
	if !ok || len(stateCtx.requestObjects) == 0 ||
		len(reply.Objects) > len(stateCtx.requestObjects) {
		return false
	}
	requested := make(map[VoteID]struct{}, len(stateCtx.requestObjects))
	for _, id := range stateCtx.requestObjects {
		requested[id] = struct{}{}
	}
	seen := make(map[VoteID]struct{}, len(reply.Objects))
	for _, object := range reply.Objects {
		id, err := object.VoteID()
		if err != nil {
			return false
		}
		if _, ok := requested[id]; !ok {
			return false
		}
		if _, ok := seen[id]; ok {
			return false
		}
		seen[id] = struct{}{}
	}
	stateCtx.requestObjects = nil
	return true
}

func containsVoteID(ids []VoteID, target VoteID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

// PerasVotes contains the client and server protocol endpoints.
type PerasVotes struct {
	Client *Client
	Server *Server
}

// Config holds ObjectDiffusion limits and application callbacks.
type Config struct {
	MaxObjectsUnacknowledged uint16
	Timeout                  time.Duration
	ObjectIDsFunc            ObjectIDsFunc
	ObjectsFunc              ObjectsFunc
	VoteFunc                 VoteFunc
}

// CallbackContext provides protocol and connection lifecycle information.
type CallbackContext struct {
	ConnectionId       connection.ConnectionId
	ConnectionDoneChan <-chan any
	Client             *Client
	Server             *Server
}

type (
	// ObjectIDsFunc supplies IDs in response to a client request.
	ObjectIDsFunc func(CallbackContext, uint16, uint16) ([]VoteID, error)
	// ObjectsFunc supplies requested vote payloads; unavailable votes may be
	// omitted.
	ObjectsFunc func(CallbackContext, []VoteID) ([]VoteObject, error)
	// VoteFunc processes each vote received by Client.Sync.
	VoteFunc func(CallbackContext, VoteID, VoteObject) error
)

// PerasVotesOptionFunc modifies PerasVotes configuration.
type PerasVotesOptionFunc func(*Config)

// New creates both Peras vote-diffusion endpoints with shared options.
func New(protoOptions protocol.ProtocolOptions, cfg *Config) *PerasVotes {
	cfg = normalizeConfig(cfg)
	return &PerasVotes{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
}

// Start starts the server and sends MsgInit from the client endpoint.
func (p *PerasVotes) Start() {
	if p.Server != nil {
		p.Server.Start()
	}
	if p.Client != nil {
		p.Client.Start()
	}
}

// Stop immediately stops both endpoints.
func (p *PerasVotes) Stop() {
	if p.Client != nil {
		p.Client.Stop()
	}
	if p.Server != nil {
		p.Server.Stop()
	}
}

func normalizeConfig(cfg *Config) *Config {
	if cfg == nil {
		value := NewConfig()
		return &value
	}
	ret := *cfg
	if ret.MaxObjectsUnacknowledged == 0 {
		ret.MaxObjectsUnacknowledged = DefaultMaxObjectsUnacknowledged
	}
	if ret.Timeout == 0 {
		ret.Timeout = DefaultTimeout
	}
	if err := ret.validate(); err != nil {
		panic("invalid PerasVotes configuration: " + err.Error())
	}
	return &ret
}

// NewConfig returns validated defaults modified by the supplied options.
func NewConfig(options ...PerasVotesOptionFunc) Config {
	cfg := Config{
		MaxObjectsUnacknowledged: DefaultMaxObjectsUnacknowledged,
		Timeout:                  DefaultTimeout,
	}
	for _, option := range options {
		option(&cfg)
	}
	if err := cfg.validate(); err != nil {
		panic("invalid PerasVotes configuration: " + err.Error())
	}
	return cfg
}

func (c *Config) validate() error {
	if c.MaxObjectsUnacknowledged == 0 ||
		c.MaxObjectsUnacknowledged > MaxObjectsUnacknowledged {
		return fmt.Errorf(
			"MaxObjectsUnacknowledged must be between 1 and %d",
			MaxObjectsUnacknowledged,
		)
	}
	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	return nil
}

// WithMaxObjectsUnacknowledged sets the outstanding vote ID limit.
func WithMaxObjectsUnacknowledged(limit uint16) PerasVotesOptionFunc {
	return func(c *Config) { c.MaxObjectsUnacknowledged = limit }
}

// WithTimeout sets the non-blocking request timeout.
func WithTimeout(timeout time.Duration) PerasVotesOptionFunc {
	return func(c *Config) { c.Timeout = timeout }
}

// WithObjectIDsFunc sets the server's ID announcement callback.
func WithObjectIDsFunc(fn ObjectIDsFunc) PerasVotesOptionFunc {
	return func(c *Config) { c.ObjectIDsFunc = fn }
}

// WithObjectsFunc sets the server's object lookup callback.
func WithObjectsFunc(fn ObjectsFunc) PerasVotesOptionFunc {
	return func(c *Config) { c.ObjectsFunc = fn }
}

// WithVoteFunc sets the client's vote processing callback for Sync.
func WithVoteFunc(fn VoteFunc) PerasVotesOptionFunc {
	return func(c *Config) { c.VoteFunc = fn }
}
