package handshake

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/muxer"
	"github.com/cloudstruct/go-ouroboros-network/protocol/handshake/messages"
	"github.com/cloudstruct/go-ouroboros-network/utils"
)

const (
	PROTOCOL_ID = 0

	STATE_PROPOSE = iota
	STATE_CONFIRM
	STATE_DONE
)

type Handshake struct {
	errorChan chan error
	sendChan  chan *muxer.Message
	recvChan  chan *muxer.Message
	state     uint8
	Version   uint16
	Finished  chan bool
}

func New(m *muxer.Muxer, errorChan chan error) *Handshake {
	sendChan, recvChan := m.RegisterProtocol(PROTOCOL_ID)
	h := &Handshake{
		sendChan: sendChan,
		recvChan: recvChan,
		state:    STATE_PROPOSE,
		Finished: make(chan bool, 1),
	}
	go h.recvLoop()
	return h
}

func (h *Handshake) Propose(versions []uint16, networkMagic uint32) error {
	if h.state != STATE_PROPOSE {
		return fmt.Errorf("protocol not in expected state")
	}
	// Create our request
	versionMap := make(map[uint16]uint32)
	for _, version := range versions {
		versionMap[version] = networkMagic
	}
	data := messages.NewRequest(versionMap)
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

func (h *Handshake) handleRequest(msg *muxer.Message) error {
	if h.state != STATE_CONFIRM {
		return fmt.Errorf("received handshake request when protocol is in wrong state")
	}
	// TODO: implement me
	return fmt.Errorf("handshake request handling not yet implemented")
}

func (h *Handshake) handleResponseAccept(msg *muxer.Message) error {
	if h.state != STATE_CONFIRM {
		return fmt.Errorf("received handshake accept response when protocol is in wrong state")
	}
	var respAccept messages.ResponseAccept
	if err := utils.CborDecode(msg.Payload, &respAccept); err != nil {
		return fmt.Errorf("handshake failed: decode error: %s", err)
	}
	h.Version = respAccept.Version
	h.Finished <- true
	h.state = STATE_DONE
	return nil
}

func (h *Handshake) handleResponseRefuse(msg *muxer.Message) error {
	if h.state != STATE_CONFIRM {
		return fmt.Errorf("received handshake refuse response when protocol is in wrong state")
	}
	var respRefuse messages.ResponseRefuse
	if err := utils.CborDecode(msg.Payload, &respRefuse); err != nil {
		return fmt.Errorf("handshake failed: decode error: %s", err)
	}
	var err error
	switch respRefuse.Reason[0].(uint64) {
	case messages.REFUSE_REASON_VERSION_MISMATCH:
		err = fmt.Errorf("handshake failed: version mismatch")
	case messages.REFUSE_REASON_DECODE_ERROR:
		err = fmt.Errorf("handshake failed: decode error: %s", respRefuse.Reason[2].(string))
	case messages.REFUSE_REASON_REFUSED:
		err = fmt.Errorf("handshake failed: refused: %s", respRefuse.Reason[2].(string))
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
		if err := utils.CborDecode(msg.Payload, &resp); err != nil {
			h.errorChan <- fmt.Errorf("handshake failed: decode error: %s", err)
		}
		switch resp[0].(uint64) {
		case messages.MESSAGE_TYPE_REQUEST:
			err = h.handleRequest(msg)
		case messages.MESSAGE_TYPE_RESPONSE_ACCEPT:
			err = h.handleResponseAccept(msg)
		case messages.MESSAGE_TYPE_RESPONSE_REFUSE:
			err = h.handleResponseRefuse(msg)
		default:
			err = fmt.Errorf("handshake failed: received unexpected message: %#v", resp)
		}
		if err != nil {
			h.errorChan <- err
		}
	}
}
