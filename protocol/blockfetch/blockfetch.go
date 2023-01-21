package blockfetch

import (
	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

const (
	PROTOCOL_NAME        = "block-fetch"
	PROTOCOL_ID   uint16 = 3
)

var (
	STATE_IDLE      = protocol.NewState(1, "Idle")
	STATE_BUSY      = protocol.NewState(2, "Busy")
	STATE_STREAMING = protocol.NewState(3, "Streaming")
	STATE_DONE      = protocol.NewState(4, "Done")
)

var StateMap = protocol.StateMap{
	STATE_IDLE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_REQUEST_RANGE,
				NewState: STATE_BUSY,
			},
			{
				MsgType:  MESSAGE_TYPE_CLIENT_DONE,
				NewState: STATE_DONE,
			},
		},
	},
	STATE_BUSY: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_START_BATCH,
				NewState: STATE_STREAMING,
			},
			{
				MsgType:  MESSAGE_TYPE_NO_BLOCKS,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_STREAMING: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_BLOCK,
				NewState: STATE_STREAMING,
			},
			{
				MsgType:  MESSAGE_TYPE_BATCH_DONE,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_DONE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_NONE,
	},
}

type BlockFetch struct {
	Client *Client
	Server *Server
}

type Config struct {
	StartBatchFunc StartBatchFunc
	NoBlocksFunc   NoBlocksFunc
	BlockFunc      BlockFunc
	BatchDoneFunc  BatchDoneFunc
}

// Callback function types
type StartBatchFunc func() error
type NoBlocksFunc func() error
type BlockFunc func(uint, interface{}) error
type BatchDoneFunc func() error

func New(protoOptions protocol.ProtocolOptions, cfg *Config) *BlockFetch {
	b := &BlockFetch{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return b
}

type BlockFetchOptionFunc func(*Config)

func NewConfig(options ...BlockFetchOptionFunc) Config {
	c := Config{}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

func WithStartBatchFunc(startBatchFunc StartBatchFunc) BlockFetchOptionFunc {
	return func(c *Config) {
		c.StartBatchFunc = startBatchFunc
	}
}

func WithNoBlocksFunc(noBlocksFunc NoBlocksFunc) BlockFetchOptionFunc {
	return func(c *Config) {
		c.NoBlocksFunc = noBlocksFunc
	}
}

func WithBlockFunc(blockFunc BlockFunc) BlockFetchOptionFunc {
	return func(c *Config) {
		c.BlockFunc = blockFunc
	}
}

func WithBatchDoneFunc(BatchDoneFunc BatchDoneFunc) BlockFetchOptionFunc {
	return func(c *Config) {
		c.BatchDoneFunc = BatchDoneFunc
	}
}
