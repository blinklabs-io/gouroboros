// Package localtxmonitor implements the Ouroboros local-tx-monitor protocol
package localtxmonitor

import (
	"time"

	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

// Protocol identifiers
const (
	protocolName        = "local-tx-monitor"
	protocolId   uint16 = 9
)

var (
	stateIdle      = protocol.NewState(1, "Idle")
	stateAcquiring = protocol.NewState(2, "Acquiring")
	stateAcquired  = protocol.NewState(3, "Acquired")
	stateBusy      = protocol.NewState(4, "Busy")
	stateDone      = protocol.NewState(5, "Done")
)

// LocalTxMonitor protocol state machine
var StateMap = protocol.StateMap{
	stateIdle: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeAcquire,
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
				MsgType:  MessageTypeAcquired,
				NewState: stateAcquired,
			},
		},
	},
	stateAcquired: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeAcquire,
				NewState: stateAcquiring,
			},
			{
				MsgType:  MessageTypeRelease,
				NewState: stateIdle,
			},
			{
				MsgType:  MessageTypeHasTx,
				NewState: stateBusy,
			},
			{
				MsgType:  MessageTypeNextTx,
				NewState: stateBusy,
			},
			{
				MsgType:  MessageTypeGetSizes,
				NewState: stateBusy,
			},
		},
	},
	stateBusy: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MessageTypeReplyHasTx,
				NewState: stateAcquired,
			},
			{
				MsgType:  MessageTypeReplyNextTx,
				NewState: stateAcquired,
			},
			{
				MsgType:  MessageTypeReplyGetSizes,
				NewState: stateAcquired,
			},
		},
	},
	stateDone: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

// LocalTxMonitor is a wrapper object that holds the client and server instances
type LocalTxMonitor struct {
	Client *Client
	Server *Server
}

// Config is used to configure the LocalTxMonitor protocol instance
type Config struct {
	AcquireTimeout time.Duration
	QueryTimeout   time.Duration
}

// New returns a new LocalTxMonitor object
func New(protoOptions protocol.ProtocolOptions, cfg *Config) *LocalTxMonitor {
	l := &LocalTxMonitor{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return l
}

// LocalTxMonitorOptionFunc represents a function used to modify the LocalTxMonitor protocol config
type LocalTxMonitorOptionFunc func(*Config)

// NewConfig returns a new LocalTxMonitor config object with the provided options
func NewConfig(options ...LocalTxMonitorOptionFunc) Config {
	c := Config{
		AcquireTimeout: 5 * time.Second,
		QueryTimeout:   30 * time.Second,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

// WithAcquireTimeout specifies the timeout for acquire operations when acting as a client
func WithAcquireTimeout(timeout time.Duration) LocalTxMonitorOptionFunc {
	return func(c *Config) {
		c.AcquireTimeout = timeout
	}
}

// WithQueryTimeout specifies the timeout for query operations when acting as a client
func WithQueryTimeout(timeout time.Duration) LocalTxMonitorOptionFunc {
	return func(c *Config) {
		c.QueryTimeout = timeout
	}
}
