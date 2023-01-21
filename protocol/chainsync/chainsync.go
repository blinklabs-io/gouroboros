package chainsync

import (
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/protocol/common"
)

const (
	PROTOCOL_NAME          = "chain-sync"
	PROTOCOL_ID_NTN uint16 = 2
	PROTOCOL_ID_NTC uint16 = 5
)

var (
	STATE_IDLE       = protocol.NewState(1, "Idle")
	STATE_CAN_AWAIT  = protocol.NewState(2, "CanAwait")
	STATE_MUST_REPLY = protocol.NewState(3, "MustReply")
	STATE_INTERSECT  = protocol.NewState(4, "Intersect")
	STATE_DONE       = protocol.NewState(5, "Done")
)

var StateMap = protocol.StateMap{
	STATE_IDLE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_REQUEST_NEXT,
				NewState: STATE_CAN_AWAIT,
			},
			{
				MsgType:  MESSAGE_TYPE_FIND_INTERSECT,
				NewState: STATE_INTERSECT,
			},
			{
				MsgType:  MESSAGE_TYPE_DONE,
				NewState: STATE_DONE,
			},
		},
	},
	STATE_CAN_AWAIT: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_AWAIT_REPLY,
				NewState: STATE_MUST_REPLY,
			},
			{
				MsgType:  MESSAGE_TYPE_ROLL_FORWARD,
				NewState: STATE_IDLE,
			},
			{
				MsgType:  MESSAGE_TYPE_ROLL_BACKWARD,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_INTERSECT: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_INTERSECT_FOUND,
				NewState: STATE_IDLE,
			},
			{
				MsgType:  MESSAGE_TYPE_INTERSECT_NOT_FOUND,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_MUST_REPLY: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_ROLL_FORWARD,
				NewState: STATE_IDLE,
			},
			{
				MsgType:  MESSAGE_TYPE_ROLL_BACKWARD,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_DONE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_NONE,
	},
}

type ChainSync struct {
	Client *Client
	Server *Server
}

type Config struct {
	RollBackwardFunc RollBackwardFunc
	RollForwardFunc  RollForwardFunc
}

// Callback function types
type RollBackwardFunc func(common.Point, Tip) error
type RollForwardFunc func(uint, interface{}, Tip) error

func New(protoOptions protocol.ProtocolOptions, cfg *Config) *ChainSync {
	c := &ChainSync{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return c
}

type ChainSyncOptionFunc func(*Config)

func NewConfig(options ...ChainSyncOptionFunc) Config {
	c := Config{}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

func WithRollBackwardFunc(rollBackwardFunc RollBackwardFunc) ChainSyncOptionFunc {
	return func(c *Config) {
		c.RollBackwardFunc = rollBackwardFunc
	}
}

func WithRollForwardFunc(rollForwardFunc RollForwardFunc) ChainSyncOptionFunc {
	return func(c *Config) {
		c.RollForwardFunc = rollForwardFunc
	}
}
