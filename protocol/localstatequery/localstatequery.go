package localstatequery

import (
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
	AcquireFunc   AcquireFunc
	AcquiredFunc  AcquiredFunc
	FailureFunc   FailureFunc
	QueryFunc     QueryFunc
	ResultFunc    ResultFunc
	ReleaseFunc   ReleaseFunc
	ReAcquireFunc ReAcquireFunc
	DoneFunc      DoneFunc
}

// Callback function types
// TODO: update callbacks
type AcquireFunc func(interface{}) error
type AcquiredFunc func() error
type FailureFunc func(interface{}) error
type QueryFunc func(interface{}) error
type ResultFunc func(interface{}) error
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
