package keepalive

import (
	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

const (
	PROTOCOL_NAME        = "keep-alive"
	PROTOCOL_ID   uint16 = 8

	// Time between keep-alive probes, in seconds
	KEEP_ALIVE_PERIOD = 60
)

var (
	STATE_CLIENT = protocol.NewState(1, "Client")
	STATE_SERVER = protocol.NewState(2, "Server")
	STATE_DONE   = protocol.NewState(3, "Done")
)

var StateMap = protocol.StateMap{
	STATE_CLIENT: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_KEEP_ALIVE,
				NewState: STATE_SERVER,
			},
			{
				MsgType:  MESSAGE_TYPE_DONE,
				NewState: STATE_DONE,
			},
		},
	},
	STATE_SERVER: protocol.StateMapEntry{
		Agency: protocol.AGENCY_SERVER,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_KEEP_ALIVE_RESPONSE,
				NewState: STATE_CLIENT,
			},
		},
	},
	STATE_DONE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_NONE,
	},
}

type KeepAlive struct {
	Client *Client
	Server *Server
}

type Config struct {
	KeepAliveFunc         KeepAliveFunc
	KeepAliveResponseFunc KeepAliveResponseFunc
	DoneFunc              DoneFunc
}

// Callback function types
type KeepAliveFunc func(uint16) error
type KeepAliveResponseFunc func(uint16) error
type DoneFunc func() error

func New(protoOptions protocol.ProtocolOptions, cfg *Config) *KeepAlive {
	k := &KeepAlive{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return k
}
