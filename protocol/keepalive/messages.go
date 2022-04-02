package keepalive

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/utils"
)

const (
	MESSAGE_TYPE_KEEP_ALIVE          = 0
	MESSAGE_TYPE_KEEP_ALIVE_RESPONSE = 1
	MESSAGE_TYPE_DONE                = 2
)

func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MESSAGE_TYPE_KEEP_ALIVE:
		ret = &MsgKeepAlive{}
	case MESSAGE_TYPE_KEEP_ALIVE_RESPONSE:
		ret = &MsgKeepAliveResponse{}
	case MESSAGE_TYPE_DONE:
		ret = &MsgDone{}
	}
	if _, err := utils.CborDecode(data, ret); err != nil {
		return nil, fmt.Errorf("%s: decode error: %s", PROTOCOL_NAME, err)
	}
	if ret != nil {
		// Store the raw message CBOR
		ret.SetCbor(data)
	}
	return ret, nil
}

type MsgKeepAlive struct {
	protocol.MessageBase
	Cookie uint16
}

func NewMsgKeepAlive(cookie uint16) *MsgKeepAlive {
	msg := &MsgKeepAlive{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_KEEP_ALIVE,
		},
		Cookie: cookie,
	}
	return msg
}

type MsgKeepAliveResponse struct {
	protocol.MessageBase
	Cookie uint16
}

func NewMsgKeepAliveResponse(cookie uint16) *MsgKeepAliveResponse {
	msg := &MsgKeepAliveResponse{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_KEEP_ALIVE_RESPONSE,
		},
		Cookie: cookie,
	}
	return msg
}

type MsgDone struct {
	protocol.MessageBase
}

func NewMsgDone() *MsgDone {
	m := &MsgDone{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_DONE,
		},
	}
	return m
}
