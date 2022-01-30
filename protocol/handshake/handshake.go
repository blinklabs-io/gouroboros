package handshake

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/muxer"
	"github.com/cloudstruct/go-ouroboros-network/utils"
)

const (
	PROTOCOL_ID = 0

	STATE_PROPOSE = iota
	STATE_CONFIRM
	STATE_DONE

	DIFFUSION_MODE_INITIATOR_ONLY          = true
	DIFFUSION_MODE_INITIATOR_AND_RESPONDER = false
)

type Handshake struct {
	errorChan  chan error
	sendChan   chan *muxer.Message
	recvChan   chan *muxer.Message
	nodeToNode bool
	state      uint8
	Version    uint16
	Finished   chan bool
}

func New(m *muxer.Muxer, errorChan chan error, nodeToNode bool) *Handshake {
	sendChan, recvChan := m.RegisterProtocol(PROTOCOL_ID)
	h := &Handshake{
		sendChan:   sendChan,
		recvChan:   recvChan,
		nodeToNode: nodeToNode,
		state:      STATE_PROPOSE,
		Finished:   make(chan bool, 1),
	}
	go h.recvLoop()
	return h
}

func (h *Handshake) ProposeVersions(versions []uint16, networkMagic uint32) error {
	if h.state != STATE_PROPOSE {
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
	data := newMsgProposeVersions(versionMap)
	dataBytes, err := utils.CborEncode(data)
	if err != nil {
		return err
	}
	msg := muxer.NewMessage(PROTOCOL_ID, dataBytes, false)
	// Send request
	h.sendChan <- msg
	// Set the new state
	h.state = STATE_CONFIRM
	return nil
}

func (h *Handshake) handleProposeVersions(msg *muxer.Message) error {
	if h.state != STATE_CONFIRM {
		return fmt.Errorf("received handshake request when protocol is in wrong state")
	}
	// TODO: implement me
	return fmt.Errorf("handshake request handling not yet implemented")
}

func (h *Handshake) handleAcceptVersion(msg *muxer.Message) error {
	if h.state != STATE_CONFIRM {
		return fmt.Errorf("received handshake accept response when protocol is in wrong state")
	}
	var resp msgAcceptVersion
	if _, err := utils.CborDecode(msg.Payload, &resp); err != nil {
		return fmt.Errorf("handshake failed: decode error: %s", err)
	}
	h.Version = resp.Version
	h.Finished <- true
	h.state = STATE_DONE
	return nil
}

func (h *Handshake) handleRefuse(msg *muxer.Message) error {
	if h.state != STATE_CONFIRM {
		return fmt.Errorf("received handshake refuse response when protocol is in wrong state")
	}
	var resp msgRefuse
	if _, err := utils.CborDecode(msg.Payload, &resp); err != nil {
		return fmt.Errorf("handshake failed: decode error: %s", err)
	}
	var err error
	switch resp.Reason[0].(uint64) {
	case REFUSE_REASON_VERSION_MISMATCH:
		err = fmt.Errorf("handshake failed: version mismatch")
	case REFUSE_REASON_DECODE_ERROR:
		err = fmt.Errorf("handshake failed: decode error: %s", resp.Reason[2].(string))
	case REFUSE_REASON_REFUSED:
		err = fmt.Errorf("handshake failed: refused: %s", resp.Reason[2].(string))
	}
	h.state = STATE_DONE
	return err
}

func (h *Handshake) recvLoop() {
	for {
		var err error
		// Wait for response
		msg := <-h.recvChan
		// Decode response into generic list until we can determine what type of response it is
		var resp []interface{}
		if _, err := utils.CborDecode(msg.Payload, &resp); err != nil {
			h.errorChan <- fmt.Errorf("handshake failed: decode error: %s", err)
		}
		switch resp[0].(uint64) {
		case MESSAGE_TYPE_PROPOSE_VERSIONS:
			err = h.handleProposeVersions(msg)
		case MESSAGE_TYPE_ACCEPT_VERSION:
			err = h.handleAcceptVersion(msg)
		case MESSAGE_TYPE_REFUSE:
			err = h.handleRefuse(msg)
		default:
			err = fmt.Errorf("handshake failed: received unexpected message: %#v", resp)
		}
		if err != nil {
			h.errorChan <- err
		}
	}
}
