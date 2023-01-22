package localstatequery

import (
	"time"

	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

const (
	PROTOCOL_NAME        = "local-state-query"
	PROTOCOL_ID   uint16 = 7
)

var (
	STATE_IDLE      = protocol.NewState(1, "Idle")
	STATE_ACQUIRING = protocol.NewState(2, "Acquiring")
	STATE_ACQUIRED  = protocol.NewState(3, "Acquired")
	STATE_QUERYING  = protocol.NewState(4, "Querying")
	STATE_DONE      = protocol.NewState(5, "Done")
)

var StateMap = protocol.StateMap{
	STATE_IDLE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_ACQUIRE,
				NewState: STATE_ACQUIRING,
			},
			{
				MsgType:  MESSAGE_TYPE_ACQUIRE_NO_POINT,
				NewState: STATE_ACQUIRING,
			},
			{
				MsgType:  MESSAGE_TYPE_DONE,
				NewState: STATE_DONE,
			},
		},
	},
	STATE_ACQUIRING: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_FAILURE,
				NewState: STATE_IDLE,
			},
			{
				MsgType:  MESSAGE_TYPE_ACQUIRED,
				NewState: STATE_ACQUIRED,
			},
		},
	},
	STATE_ACQUIRED: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_QUERY,
				NewState: STATE_QUERYING,
			},
			{
				MsgType:  MESSAGE_TYPE_REACQUIRE,
				NewState: STATE_ACQUIRING,
			},
			{
				MsgType:  MESSAGE_TYPE_REACQUIRE_NO_POINT,
				NewState: STATE_ACQUIRING,
			},
			{
				MsgType:  MESSAGE_TYPE_RELEASE,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_QUERYING: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_RESULT,
				NewState: STATE_ACQUIRED,
			},
		},
	},
	STATE_DONE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_NONE,
	},
}

type LocalStateQuery struct {
	Client *Client
	Server *Server
}

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

func New(protoOptions protocol.ProtocolOptions, cfg *Config) *LocalStateQuery {
	l := &LocalStateQuery{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return l
}

type LocalStateQueryOptionFunc func(*Config)

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

func WithAcquireFunc(acquireFunc AcquireFunc) LocalStateQueryOptionFunc {
	return func(c *Config) {
		c.AcquireFunc = acquireFunc
	}
}

func WithQueryFunc(queryFunc QueryFunc) LocalStateQueryOptionFunc {
	return func(c *Config) {
		c.QueryFunc = queryFunc
	}
}

func WithReleaseFunc(releaseFunc ReleaseFunc) LocalStateQueryOptionFunc {
	return func(c *Config) {
		c.ReleaseFunc = releaseFunc
	}
}

func WithReAcquireFunc(reAcquireFunc ReAcquireFunc) LocalStateQueryOptionFunc {
	return func(c *Config) {
		c.ReAcquireFunc = reAcquireFunc
	}
}

func WithDoneFunc(doneFunc DoneFunc) LocalStateQueryOptionFunc {
	return func(c *Config) {
		c.DoneFunc = doneFunc
	}
}

func WithAcquireTimeout(timeout time.Duration) LocalStateQueryOptionFunc {
	return func(c *Config) {
		c.AcquireTimeout = timeout
	}
}

func WithQueryTimeout(timeout time.Duration) LocalStateQueryOptionFunc {
	return func(c *Config) {
		c.QueryTimeout = timeout
	}
}
