package handshake

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/utils"
)

const (
	PROTOCOL_ID_SENDER             = 0x0000
	PROTOCOL_ID_RECEIVER           = 0x8000
	MESSAGE_TYPE_REQUEST           = 0
	MESSAGE_TYPE_RESPONSE_ACCEPT   = 1
	MESSAGE_TYPE_RESPONSE_REFUSE   = 2
	REFUSE_REASON_VERSION_MISMATCH = 0
	REFUSE_REASON_DECODE_ERROR     = 1
	REFUSE_REASON_REFUSED          = 2
)

type handshakeMessage struct {
	_           struct{} `cbor:",toarray"`
	MessageType uint8
}

type handshakeRequest struct {
	handshakeMessage
	VersionMap map[uint16]uint32
}

type handshakeResponseAccept struct {
	handshakeMessage
	Version      uint16
	NetworkMagic uint32
}

type handshakeResponseRefuse struct {
	handshakeMessage
	Reason []interface{}
}

type Handshake struct {
	sendChan chan []byte
	recvChan chan []byte
}

func New(sendChan chan []byte, recvChan chan []byte) *Handshake {
	h := &Handshake{
		sendChan: sendChan,
		recvChan: recvChan,
	}
	return h
}

func (h *Handshake) Start(versions []uint16, networkMagic uint32) (uint16, error) {
	// Create our request
	versionMap := make(map[uint16]uint32)
	for _, version := range versions {
		versionMap[version] = networkMagic
	}
	data := handshakeRequest{
		handshakeMessage: handshakeMessage{
			MessageType: MESSAGE_TYPE_REQUEST,
		},
		VersionMap: versionMap,
	}
	dataBytes, err := utils.CborEncode(data)
	if err != nil {
		return 0, err
	}
	// Send request
	h.sendChan <- dataBytes
	// Wait for response
	respBytes := <-h.recvChan
	// Decode response into generic list until we can determine what type of response it is
	var resp []interface{}
	if err := utils.CborDecode(respBytes, &resp); err != nil {
		return 0, err
	}
	switch resp[0].(uint64) {
	case MESSAGE_TYPE_RESPONSE_ACCEPT:
		var respAccept handshakeResponseAccept
		if err := utils.CborDecode(respBytes, &respAccept); err != nil {
			return 0, err
		}
		return respAccept.Version, nil
	case MESSAGE_TYPE_RESPONSE_REFUSE:
		var respRefuse handshakeResponseRefuse
		if err := utils.CborDecode(respBytes, &respRefuse); err != nil {
			return 0, err
		}
		switch respRefuse.Reason[0].(uint64) {
		case REFUSE_REASON_VERSION_MISMATCH:
			return 0, fmt.Errorf("handshake failed: version mismatch")
		case REFUSE_REASON_DECODE_ERROR:
			return 0, fmt.Errorf("handshake failed: decode error: %s", respRefuse.Reason[2].(string))
		case REFUSE_REASON_REFUSED:
			return 0, fmt.Errorf("handshake failed: refused: %s", respRefuse.Reason[2].(string))
		}
	}
	return 0, fmt.Errorf("unexpected failure")
}
