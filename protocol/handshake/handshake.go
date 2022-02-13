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
	h.proto = protocol.New(PROTOCOL_NAME, PROTOCOL_ID, m, errorChan, h.handleMessage, NewMsgFromCbor)
	// Set initial state
	h.proto.SetState(STATE_PROPOSE)
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
	if err := h.proto.LockState([]protocol.State{STATE_PROPOSE}); err != nil {
		return fmt.Errorf("protocol not in expected state")
	}
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
	// Unlock and change state when we're done
	defer h.proto.UnlockState(STATE_CONFIRM)
	// Send request
	return h.proto.SendMessage(msg, false)
}

func (h *Handshake) handleProposeVersions(msgGeneric protocol.Message) error {
	if err := h.proto.LockState([]protocol.State{STATE_CONFIRM}); err != nil {
		return fmt.Errorf("received handshake request when protocol is in wrong state")
	}
	// TODO: implement me
	return fmt.Errorf("handshake request handling not yet implemented")
}

func (h *Handshake) handleAcceptVersion(msgGeneric protocol.Message) error {
	if err := h.proto.LockState([]protocol.State{STATE_CONFIRM}); err != nil {
		return fmt.Errorf("received handshake accept response when protocol is in wrong state")
	}
	msg := msgGeneric.(*msgAcceptVersion)
	h.Version = msg.Version
	h.Finished <- true
	// Unlock and change state when we're done
	defer h.proto.UnlockState(STATE_DONE)
	return nil
}

func (h *Handshake) handleRefuse(msgGeneric protocol.Message) error {
	if err := h.proto.LockState([]protocol.State{STATE_CONFIRM}); err != nil {
		return fmt.Errorf("received handshake refuse response when protocol is in wrong state")
	}
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
	// Unlock and change state when we're done
	defer h.proto.UnlockState(STATE_DONE)
	return err
}
