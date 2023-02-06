package chainsync

import (
	"time"

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
		Agency: protocol.AgencyClient,
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
		Agency: protocol.AgencyServer,
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
		Agency: protocol.AgencyServer,
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
		Agency: protocol.AgencyServer,
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
		Agency: protocol.AgencyNone,
	},
}

type ChainSync struct {
	Client *Client
	Server *Server
}

type Config struct {
	RollBackwardFunc RollBackwardFunc
	RollForwardFunc  RollForwardFunc
	IntersectTimeout time.Duration
	BlockTimeout     time.Duration
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
	c := Config{
		IntersectTimeout: 5 * time.Second,
		// We should really use something more useful like 30-60s, but we've seen 55s between blocks
		// in the preview network
		// https://preview.cexplorer.io/block/cb08a386363a946d2606e912fcd81ffed2bf326cdbc4058297b14471af4f67e9
		// https://preview.cexplorer.io/block/86806dca4ba735b233cbeee6da713bdece36fd41fb5c568f9ef5a3f5cbf572a3
		BlockTimeout: 180 * time.Second,
	}
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

func WithIntersectTimeout(timeout time.Duration) ChainSyncOptionFunc {
	return func(c *Config) {
		c.IntersectTimeout = timeout
	}
}

func WithBlockTimeout(timeout time.Duration) ChainSyncOptionFunc {
	return func(c *Config) {
		c.BlockTimeout = timeout
	}
}
