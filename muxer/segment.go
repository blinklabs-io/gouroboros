package muxer

import (
	"time"
)

const (
	SEGMENT_PROTOCOL_ID_RESPONSE_FLAG = 0x8000
	SEGMENT_MAX_PAYLOAD_LENGTH        = 65535
)

type SegmentHeader struct {
	Timestamp     uint32
	ProtocolId    uint16
	PayloadLength uint16
}

type Segment struct {
	SegmentHeader
	Payload []byte
}

func NewSegment(protocolId uint16, payload []byte, isResponse bool) *Segment {
	header := SegmentHeader{
		Timestamp:  uint32(time.Now().UnixNano() & 0xffffffff),
		ProtocolId: protocolId,
	}
	if isResponse {
		header.ProtocolId = header.ProtocolId + SEGMENT_PROTOCOL_ID_RESPONSE_FLAG
	}
	header.PayloadLength = uint16(len(payload))
	segment := &Segment{
		SegmentHeader: header,
		Payload:       payload,
	}
	return segment
}

func (s *SegmentHeader) IsRequest() bool {
	return (s.ProtocolId & SEGMENT_PROTOCOL_ID_RESPONSE_FLAG) == 0
}

func (s *SegmentHeader) IsResponse() bool {
	return (s.ProtocolId & SEGMENT_PROTOCOL_ID_RESPONSE_FLAG) > 0
}

func (s *SegmentHeader) GetProtocolId() uint16 {
	if s.ProtocolId >= SEGMENT_PROTOCOL_ID_RESPONSE_FLAG {
		return s.ProtocolId - SEGMENT_PROTOCOL_ID_RESPONSE_FLAG
	}
	return s.ProtocolId
}
