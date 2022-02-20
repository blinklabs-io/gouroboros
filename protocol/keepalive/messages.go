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
		ret = &msgKeepAlive{}
	case MESSAGE_TYPE_KEEP_ALIVE_RESPONSE:
		ret = &msgKeepAliveResponse{}
	case MESSAGE_TYPE_DONE:
		ret = &msgDone{}
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

type msgKeepAlive struct {
	protocol.MessageBase
	Cookie uint16
}

func newMsgKeepAlive(cookie uint16) *msgKeepAlive {
	msg := &msgKeepAlive{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_KEEP_ALIVE,
		},
		Cookie: cookie,
	}
	return msg
}

type msgKeepAliveResponse struct {
	protocol.MessageBase
	Cookie uint16
}

func newMsgKeepAliveResponse(cookie uint16) *msgKeepAliveResponse {
	msg := &msgKeepAliveResponse{
		MessageBase: protocol.MessageBase{
			MessageType: MESSAGE_TYPE_KEEP_ALIVE_RESPONSE,
		},
		Cookie: cookie,
	}
	return msg
}

type msgDone struct {
	protocol.MessageBase
}
