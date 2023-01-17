package handshake

import (
	"time"

	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

const (
	PROTOCOL_NAME = "handshake"
	PROTOCOL_ID   = 0

	DIFFUSION_MODE_INITIATOR_ONLY          = false
	DIFFUSION_MODE_INITIATOR_AND_RESPONDER = true
)

var (
	STATE_PROPOSE = protocol.NewState(1, "Propose")
	STATE_CONFIRM = protocol.NewState(2, "Confirm")
	STATE_DONE    = protocol.NewState(3, "Done")
)

var StateMap = protocol.StateMap{
	STATE_PROPOSE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_CLIENT,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_PROPOSE_VERSIONS,
				NewState: STATE_CONFIRM,
			},
		},
	},
	STATE_CONFIRM: protocol.StateMapEntry{
		Agency:  protocol.AGENCY_SERVER,
		Timeout: 5 * time.Second,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_ACCEPT_VERSION,
				NewState: STATE_DONE,
			},
			{
				MsgType:  MESSAGE_TYPE_REFUSE,
				NewState: STATE_DONE,
			},
		},
	},
	STATE_DONE: protocol.StateMapEntry{
		Agency: protocol.AGENCY_NONE,
	},
}

type Handshake struct {
	Client *Client
	Server *Server
}

type Config struct {
	ProtocolVersions []uint16
	NetworkMagic     uint32
	ClientFullDuplex bool
	FinishedFunc     FinishedFunc
}

type FinishedFunc func(uint16, bool) error

func New(protoOptions protocol.ProtocolOptions, cfg *Config) *Handshake {
	h := &Handshake{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return h
}
