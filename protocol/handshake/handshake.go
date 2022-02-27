package handshake

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/muxer"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

const (
	PROTOCOL_NAME = "handshake"
	PROTOCOL_ID   = 0

	DIFFUSION_MODE_INITIATOR_ONLY          = true
	DIFFUSION_MODE_INITIATOR_AND_RESPONDER = false
)

var (
	STATE_PROPOSE = protocol.NewState(1, "Propose")
	STATE_CONFIRM = protocol.NewState(2, "Confirm")
	STATE_DONE    = protocol.NewState(3, "Done")
)

var stateMap = protocol.StateMap{
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
	proto      *protocol.Protocol
	nodeToNode bool
	Version    uint16
	Finished   chan bool
}

func New(m *muxer.Muxer, errorChan chan error, nodeToNode bool) *Handshake {
	h := &Handshake{
		nodeToNode: nodeToNode,
		Finished:   make(chan bool, 1),
	}
	protoConfig := protocol.ProtocolConfig{
		Name:                PROTOCOL_NAME,
		ProtocolId:          PROTOCOL_ID,
		Muxer:               m,
		ErrorChan:           errorChan,
		MessageHandlerFunc:  h.handleMessage,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            stateMap,
		InitialState:        STATE_PROPOSE,
	}
	h.proto = protocol.New(protoConfig)
	return h
}

func (h *Handshake) handleMessage(msg protocol.Message) error {
	var err error
	switch msg.Type() {
	case MESSAGE_TYPE_PROPOSE_VERSIONS:
		err = h.handleProposeVersions(msg)
	case MESSAGE_TYPE_ACCEPT_VERSION:
		err = h.handleAcceptVersion(msg)
	case MESSAGE_TYPE_REFUSE:
		err = h.handleRefuse(msg)
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", PROTOCOL_NAME, msg.Type())
	}
	return err
}

func (h *Handshake) ProposeVersions(versions []uint16, networkMagic uint32) error {
	// Create our request
	versionMap := make(map[uint16]interface{})
	for _, version := range versions {
		if h.nodeToNode {
			versionMap[version] = []interface{}{networkMagic, DIFFUSION_MODE_INITIATOR_ONLY}
		} else {
			versionMap[version] = networkMagic
		}
	}
	msg := newMsgProposeVersions(versionMap)
	return h.proto.SendMessage(msg, false)
}

func (h *Handshake) handleProposeVersions(msgGeneric protocol.Message) error {
	// TODO: implement me
	return fmt.Errorf("handshake request handling not yet implemented")
}

func (h *Handshake) handleAcceptVersion(msgGeneric protocol.Message) error {
	msg := msgGeneric.(*msgAcceptVersion)
	h.Version = msg.Version
	h.Finished <- true
	return nil
}

func (h *Handshake) handleRefuse(msgGeneric protocol.Message) error {
	msg := msgGeneric.(*msgRefuse)
	var err error
	switch msg.Reason[0].(uint64) {
	case REFUSE_REASON_VERSION_MISMATCH:
		err = fmt.Errorf("%s: version mismatch", PROTOCOL_NAME)
	case REFUSE_REASON_DECODE_ERROR:
		err = fmt.Errorf("%s: decode error: %s", PROTOCOL_NAME, msg.Reason[2].(string))
	case REFUSE_REASON_REFUSED:
		err = fmt.Errorf("%s: refused: %s", PROTOCOL_NAME, msg.Reason[2].(string))
	}
	return err
}
