package muxer

import (
	"time"
)

const (
	MESSAGE_PROTOCOL_ID_RESPONSE_FLAG = 0x8000
)

type MessageHeader struct {
	Timestamp     uint32
	ProtocolId    uint16
	PayloadLength uint16
}

type Message struct {
	MessageHeader
	Payload []byte
}

func NewMessage(protocolId uint16, payload []byte, isResponse bool) *Message {
	header := MessageHeader{
		Timestamp:  uint32(time.Now().UnixNano() & 0xffffffff),
		ProtocolId: protocolId,
	}
	if isResponse {
		header.ProtocolId = header.ProtocolId + MESSAGE_PROTOCOL_ID_RESPONSE_FLAG
	}
	header.PayloadLength = uint16(len(payload))
	msg := &Message{
		MessageHeader: header,
		Payload:       payload,
	}
	return msg
}

func (s *MessageHeader) IsRequest() bool {
	return (s.ProtocolId & MESSAGE_PROTOCOL_ID_RESPONSE_FLAG) == 0
}

func (s *MessageHeader) IsResponse() bool {
	return (s.ProtocolId & MESSAGE_PROTOCOL_ID_RESPONSE_FLAG) > 0
}

func (s *MessageHeader) GetProtocolId() uint16 {
	if s.ProtocolId >= MESSAGE_PROTOCOL_ID_RESPONSE_FLAG {
		return s.ProtocolId - MESSAGE_PROTOCOL_ID_RESPONSE_FLAG
	}
	return s.ProtocolId
}
