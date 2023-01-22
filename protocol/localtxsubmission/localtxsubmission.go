package localtxsubmission

import (
	"time"

	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

const (
	PROTOCOL_NAME        = "local-tx-submission"
	PROTOCOL_ID   uint16 = 6
)

var (
	STATE_IDLE = protocol.NewState(1, "Idle")
	STATE_BUSY = protocol.NewState(2, "Busy")
	STATE_DONE = protocol.NewState(3, "Done")
)

var StateMap = protocol.StateMap{
	STATE_IDLE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_SUBMIT_TX,
				NewState: STATE_BUSY,
			},
		},
	},
	STATE_BUSY: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_ACCEPT_TX,
				NewState: STATE_IDLE,
			},
			{
				MsgType:  MESSAGE_TYPE_REJECT_TX,
				NewState: STATE_IDLE,
			},
		},
	},
	STATE_DONE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_NONE,
	},
}

type LocalTxSubmission struct {
	Client *Client
	Server *Server
}

type Config struct {
	SubmitTxFunc SubmitTxFunc
	Timeout      time.Duration
}

// Callback function types
type SubmitTxFunc func(interface{}) error

func New(protoOptions protocol.ProtocolOptions, cfg *Config) *LocalTxSubmission {
	l := &LocalTxSubmission{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return l
}

type LocalTxSubmissionOptionFunc func(*Config)

func NewConfig(options ...LocalTxSubmissionOptionFunc) Config {
	c := Config{
		Timeout: 30 * time.Second,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

func WithSubmitTxFunc(submitTxFunc SubmitTxFunc) LocalTxSubmissionOptionFunc {
	return func(c *Config) {
		c.SubmitTxFunc = submitTxFunc
	}
}

func WithTimeout(timeout time.Duration) LocalTxSubmissionOptionFunc {
	return func(c *Config) {
		c.Timeout = timeout
	}
}
