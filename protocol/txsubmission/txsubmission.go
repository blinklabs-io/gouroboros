package txsubmission

import (
	"time"

	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

const (
	PROTOCOL_NAME        = "tx-submission"
	PROTOCOL_ID   uint16 = 4
)

var (
	STATE_INIT               = protocol.NewState(1, "Init")
	STATE_IDLE               = protocol.NewState(2, "Idle")
	STATE_TX_IDS_BLOCKING    = protocol.NewState(3, "TxIdsBlocking")
	STATE_TX_IDS_NONBLOCKING = protocol.NewState(4, "TxIdsNonBlocking")
	STATE_TXS                = protocol.NewState(5, "Txs")
	STATE_DONE               = protocol.NewState(6, "Done")
)

var StateMap = protocol.StateMap{
	STATE_INIT: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_INIT,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_IDLE: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_REQUEST_TX_IDS,
				NewState: STATE_TX_IDS_BLOCKING,
				// Match if blocking
				MatchFunc: func(msg protocol.Message) bool {
					msgRequestTxIds := msg.(*MsgRequestTxIds)
					return msgRequestTxIds.Blocking
				},
			},
			{
				MsgType:  MESSAGE_TYPE_REQUEST_TX_IDS,
				NewState: STATE_TX_IDS_NONBLOCKING,
				// Metch if non-blocking
				MatchFunc: func(msg protocol.Message) bool {
					msgRequestTxIds := msg.(*MsgRequestTxIds)
					return !msgRequestTxIds.Blocking
				},
			},
			{
				MsgType:  MESSAGE_TYPE_REQUEST_TXS,
				NewState: STATE_TXS,
			},
		},
	},
	STATE_TX_IDS_BLOCKING: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_REPLY_TX_IDS,
				NewState: STATE_IDLE,
			},
			{
				MsgType:  MESSAGE_TYPE_DONE,
				NewState: STATE_DONE,
			},
		},
	},
	STATE_TX_IDS_NONBLOCKING: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_REPLY_TX_IDS,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_TXS: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_REPLY_TXS,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_DONE: protocol.StateMapEntry{
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
type RequestTxIdsFunc func(bool, uint16, uint16) error
type ReplyTxIdsFunc func(interface{}) error
type RequestTxsFunc func(interface{}) error
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

func WithRequestTxIdsFunc(requestTxIdsFunc RequestTxIdsFunc) TxSubmissionOptionFunc {
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
