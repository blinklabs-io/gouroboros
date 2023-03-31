// Package localstatequery implements the Ouroboros local-state-query protocol
package localstatequery

import (
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// Protocol identifiers
const (
	protocolName        = "local-state-query"
	protocolId   uint16 = 7
)

var (
	stateIdle      = protocol.NewState(1, "Idle")
	stateAcquiring = protocol.NewState(2, "Acquiring")
	stateAcquired  = protocol.NewState(3, "Acquired")
	stateQuerying  = protocol.NewState(4, "Querying")
	stateDone      = protocol.NewState(5, "Done")
)

// LocalStateQuery protocol state machine
var StateMap = protocol.StateMap{
	stateIdle: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeAcquire,
				NewState: stateAcquiring,
			},
			{
				MsgType:  MessageTypeAcquireNoPoint,
				NewState: stateAcquiring,
			},
			{
				MsgType:  MessageTypeDone,
				NewState: stateDone,
			},
		},
	},
	stateAcquiring: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeFailure,
				NewState: stateIdle,
			},
			{
				MsgType:  MessageTypeAcquired,
				NewState: stateAcquired,
			},
		},
	},
	stateAcquired: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeQuery,
				NewState: stateQuerying,
			},
			{
				MsgType:  MessageTypeReacquire,
				NewState: stateAcquiring,
			},
			{
				MsgType:  MessageTypeReacquireNoPoint,
				NewState: stateAcquiring,
			},
			{
				MsgType:  MessageTypeRelease,
				NewState: stateIdle,
			},
		},
	},
	stateQuerying: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeResult,
				NewState: stateAcquired,
			},
		},
	},
	stateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

// LocalStateQuery is a wrapper object that holds the client and server instances
type LocalStateQuery struct {
	Client *Client
	Server *Server
}

// Config is used to configure the LocalStateQuery protocol instance
type Config struct {
	AcquireFunc    AcquireFunc
	QueryFunc      QueryFunc
	ReleaseFunc    ReleaseFunc
	ReAcquireFunc  ReAcquireFunc
	DoneFunc       DoneFunc
	AcquireTimeout time.Duration
	QueryTimeout   time.Duration
}

// Callback function types
// TODO: update callbacks
type AcquireFunc func(interface{}) error
type QueryFunc func(interface{}) error
type ReleaseFunc func() error
type ReAcquireFunc func(interface{}) error
type DoneFunc func() error

// New returns a new LocalStateQuery object
func New(protoOptions protocol.ProtocolOptions, cfg *Config) *LocalStateQuery {
	l := &LocalStateQuery{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return l
}

// LocalStateQueryOptionFunc represents a function used to modify the LocalStateQuery protocol config
type LocalStateQueryOptionFunc func(*Config)

// NewConfig returns a new LocalStateQuery config object with the provided options
func NewConfig(options ...LocalStateQueryOptionFunc) Config {
	c := Config{
		AcquireTimeout: 5 * time.Second,
		QueryTimeout:   180 * time.Second,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

// WithAcquireFunc specifies the Acquire callback function when acting as a server
func WithAcquireFunc(acquireFunc AcquireFunc) LocalStateQueryOptionFunc {
	return func(c *Config) {
		c.AcquireFunc = acquireFunc
	}
}

// WithQueryFunc specifies the Query callback function when acting as a server
func WithQueryFunc(queryFunc QueryFunc) LocalStateQueryOptionFunc {
	return func(c *Config) {
		c.QueryFunc = queryFunc
	}
}

// WithReleaseFunc specifies the Release callback function when acting as a server
func WithReleaseFunc(releaseFunc ReleaseFunc) LocalStateQueryOptionFunc {
	return func(c *Config) {
		c.ReleaseFunc = releaseFunc
	}
}

// WithReAcquireFunc specifies the ReAcquire callback function when acting as a server
func WithReAcquireFunc(reAcquireFunc ReAcquireFunc) LocalStateQueryOptionFunc {
	return func(c *Config) {
		c.ReAcquireFunc = reAcquireFunc
	}
}

// WithDoneFunc specifies the Done callback function when acting as a server
func WithDoneFunc(doneFunc DoneFunc) LocalStateQueryOptionFunc {
	return func(c *Config) {
		c.DoneFunc = doneFunc
	}
}

// WithAcquireTimeout specifies the timeout for the Acquire operation when acting as a client
func WithAcquireTimeout(timeout time.Duration) LocalStateQueryOptionFunc {
	return func(c *Config) {
		c.AcquireTimeout = timeout
	}
}

// WithQueryTimeout specifies the timeout for the Query operation when acting as a client
func WithQueryTimeout(timeout time.Duration) LocalStateQueryOptionFunc {
	return func(c *Config) {
		c.QueryTimeout = timeout
	}
}
