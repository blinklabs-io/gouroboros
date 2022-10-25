package handshake

import (
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
		Agency: protocol.AGENCY_SERVER,
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

/*
func (h *Handshake) Start() {
	h.Protocol.Start()
}

func (h *Handshake) ProposeVersions() error {
	// Create our request
	versionMap := make(map[uint16]interface{})
	diffusionMode := DIFFUSION_MODE_INITIATOR_ONLY
	if h.config.ClientFullDuplex {
		diffusionMode = DIFFUSION_MODE_INITIATOR_AND_RESPONDER
	}
	for _, version := range h.config.ProtocolVersions {
		if h.Mode() == protocol.ProtocolModeNodeToNode {
			versionMap[version] = []interface{}{h.config.NetworkMagic, diffusionMode}
		} else {
			versionMap[version] = h.config.NetworkMagic
		}
	}
	msg := NewMsgProposeVersions(versionMap)
	return h.SendMessage(msg)
}
*/
