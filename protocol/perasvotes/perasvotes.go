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
	"context"
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
	// DefaultTimeout bounds non-blocking ID and object requests. It is the
	// reference's shortWait for those states.
	DefaultTimeout = 10 * time.Second
	// CanAwaitTimeout bounds the wait for the first answer to a blocking ID
	// request: identifiers or MsgAwaitReply. It is the reference's shortWait.
	CanAwaitTimeout = 10 * time.Second
	// MustReplyTimeout bounds the wait, after MsgAwaitReply, for identifiers
	// or MsgServerIdle. It is the reference's longWait.
	MustReplyTimeout = 60 * time.Second
	// DefaultAwaitTimeout is how long a server waits for new identifiers
	// after MsgAwaitReply before it sends MsgServerIdle. It stays well inside
	// MustReplyTimeout so a reference client does not time the server out.
	DefaultAwaitTimeout = 30 * time.Second
)

var (
	// StateInit is the state before the client sends MsgInit.
	StateInit = protocol.NewState(1, "Init")
	// StateIdle is where the client can request vote IDs or objects.
	StateIdle = protocol.NewState(2, "Idle")
	// StateObjectIDsBlocking waits for the first answer to a blocking ID
	// request: a non-empty ID reply, or MsgAwaitReply. It is the reference's
	// blocking CanAwait phase.
	StateObjectIDsBlocking = protocol.NewState(3, "ObjectIDsBlocking")
	// StateObjectIDsNonBlocking waits for an ID reply that may be empty.
	StateObjectIDsNonBlocking = protocol.NewState(4, "ObjectIDsNonBlocking")
	// StateObjects waits for a reply to a vote object request.
	StateObjects = protocol.NewState(5, "Objects")
	// StateDone is the terminal state.
	StateDone = protocol.NewState(6, "Done")
	// StateObjectIDsBlockingMustReply follows MsgAwaitReply: the server
	// still has agency and must send a non-empty ID reply or MsgServerIdle.
	StateObjectIDsBlockingMustReply = protocol.NewState(
		7,
		"ObjectIDsBlockingMustReply",
	)
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
		Transitions: []protocol.StateTransition{
			{
				MsgType: MessageTypeReplyObjectIDs, NewState: StateIdle,
				MatchFunc: matchReplyObjectIDs,
			},
			{
				MsgType:  MessageTypeAwaitReply,
				NewState: StateObjectIDsBlockingMustReply,
			},
		},
	},
	StateObjectIDsBlockingMustReply: {
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType: MessageTypeReplyObjectIDs, NewState: StateIdle,
				MatchFunc: matchReplyObjectIDs,
			},
			{
				MsgType: MessageTypeServerIdle, NewState: StateIdle,
				MatchFunc: matchServerIdle,
			},
		},
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
		remaining+int(
			req.RequestCount,
		) > int(
			stateCtx.maxObjectsUnacknowledged,
		) {
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

// matchServerIdle ends the blocking request without identifiers. Its
// acknowledgement was applied when the request was sent, so the next request
// acknowledges nothing from it.
func matchServerIdle(ctx any, msg protocol.Message) bool {
	stateCtx, ok := ctx.(*stateContext)
	if !ok {
		return false
	}
	if _, ok := msg.(*MsgServerIdle); !ok ||
		!stateCtx.objectIDsRequestPending {
		return false
	}
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
	// AwaitTimeout bounds a server's wait for new identifiers after it sent
	// MsgAwaitReply; it must be shorter than MustReplyTimeout.
	AwaitTimeout       time.Duration
	ObjectIDsFunc      ObjectIDsFunc
	AwaitObjectIDsFunc AwaitObjectIDsFunc
	ObjectsFunc        ObjectsFunc
	VoteFunc           VoteFunc
}

// CallbackContext provides protocol and connection lifecycle information.
type CallbackContext struct {
	ConnectionId       connection.ConnectionId
	ConnectionDoneChan <-chan any
	Client             *Client
	Server             *Server
	// ctx is returned by Context.
	ctx context.Context
}

// Context returns the context of this callback. A callback that stops its
// own protocol, or closes its own connection, passes it to that Stop or
// Close: the call then signals shutdown and returns without waiting for the
// goroutine running the callback. It carries no deadline and is never
// cancelled. It is context.Background for a CallbackContext that the package
// did not build.
func (c CallbackContext) Context() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

// withOwner returns c with the context of a callback that p runs.
func (c CallbackContext) withOwner(p *protocol.Protocol) CallbackContext {
	c.ctx = p.CallbackContext()
	return c
}

type (
	// ObjectIDsFunc supplies IDs in response to a client request, given the
	// acknowledged and requested counts. For a blocking request it must
	// answer within CanAwaitTimeout; with no ID available it returns none,
	// and the server sends MsgAwaitReply and calls AwaitObjectIDsFunc.
	ObjectIDsFunc func(CallbackContext, uint16, uint16) ([]VoteID, error)
	// AwaitObjectIDsFunc waits, until ctx ends, for up to the requested
	// number of IDs after the server sent MsgAwaitReply. The server sends
	// the IDs it returns, even after ctx has ended, or MsgServerIdle when it
	// returns none. An error returned before ctx ends fails the connection;
	// one returned after it does not. When it is nil the server waits
	// AwaitTimeout and then sends MsgServerIdle.
	AwaitObjectIDsFunc func(
		context.Context,
		CallbackContext,
		uint16,
	) ([]VoteID, error)
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
//
// Each side's Stop signals shutdown at once and waits for its goroutines
// unless ctx is the Context of a CallbackContext that side passed to a
// callback. A ctx that ends first cuts the waits short; Stop then returns
// ctx.Err() and shutdown still proceeds.
func (p *PerasVotes) Stop(ctx context.Context) error {
	var clientErr, serverErr error
	if p.Client != nil {
		clientErr = p.Client.Stop(ctx)
	}
	if p.Server != nil {
		serverErr = p.Server.Stop(ctx)
	}
	return errors.Join(clientErr, serverErr)
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
	if ret.AwaitTimeout == 0 {
		ret.AwaitTimeout = DefaultAwaitTimeout
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
		AwaitTimeout:             DefaultAwaitTimeout,
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
	if c.AwaitTimeout <= 0 || c.AwaitTimeout >= MustReplyTimeout {
		return fmt.Errorf(
			"await timeout must be positive and below %s",
			MustReplyTimeout,
		)
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

// WithAwaitTimeout sets how long the server waits for new IDs after
// MsgAwaitReply before it sends MsgServerIdle.
func WithAwaitTimeout(timeout time.Duration) PerasVotesOptionFunc {
	return func(c *Config) { c.AwaitTimeout = timeout }
}

// WithAwaitObjectIDsFunc sets the server's wait for new IDs after
// MsgAwaitReply.
func WithAwaitObjectIDsFunc(fn AwaitObjectIDsFunc) PerasVotesOptionFunc {
	return func(c *Config) { c.AwaitObjectIDsFunc = fn }
}

// WithObjectsFunc sets the server's object lookup callback.
func WithObjectsFunc(fn ObjectsFunc) PerasVotesOptionFunc {
	return func(c *Config) { c.ObjectsFunc = fn }
}

// WithVoteFunc sets the client's vote processing callback for Sync.
func WithVoteFunc(fn VoteFunc) PerasVotesOptionFunc {
	return func(c *Config) { c.VoteFunc = fn }
}
