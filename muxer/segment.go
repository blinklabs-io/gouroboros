// Copyright 2025 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package muxer

import (
	"math"
	"time"
)

// Maximum segment payload length
const SegmentMaxPayloadLength = 65535

// Bit mask used to signify a response in the protocol ID field
const segmentProtocolIdResponseFlag = 0x8000

// SegmentHeader represents the header bytes on a segment
type SegmentHeader struct {
	Timestamp     uint32
	ProtocolId    uint16
	PayloadLength uint16
}

// Segment represents basic unit of data in the Ouroboros protocol.
//
// Each chunk of data exchanged by a particular mini-protocol is wrapped in a muxer segment.
// A segment consists of 4 bytes containing a timestamp, 2 bytes indicating which protocol the
// data is part of, 2 bytes indicating the size of the payload (up to 65535 bytes), and then
// the actual payload
type Segment struct {
	SegmentHeader
	Payload      []byte
	deliveryChan chan<- error
	writtenFunc  func()
	writeSpans   *writeSpan
}

// AddWriteSpan attaches a message fragment to its transport trace. offset is
// the first message byte's offset in this segment's payload. first and last
// identify the message boundaries, independently of other coalesced messages.
// It retains only the trace, not the protocol message or its CBOR buffer.
func (s *Segment) AddWriteSpan(
	trace *WriteTrace, first, last bool, offset int,
) {
	if trace != nil {
		s.writeSpans = &writeSpan{
			trace: trace, first: first, last: last, offset: offset,
			next: s.writeSpans,
		}
	}
}

// SetDeliveryChan registers a channel that receives the result of writing the
// segment to the underlying connection. The channel must be buffered so muxer
// shutdown cannot block on an abandoned receiver.
func (s *Segment) SetDeliveryChan(deliveryChan chan<- error) {
	s.deliveryChan = deliveryChan
}

// SetWrittenFunc registers fn to run once the segment has been written to the
// underlying connection. It does not run for a segment whose write failed or
// that was dropped because its protocol was unregistered. fn runs on the
// muxer's send goroutine for the segment's protocol, so it must not block.
func (s *Segment) SetWrittenFunc(fn func()) {
	s.writtenFunc = fn
}

func (s *Segment) reportDelivery(err error) {
	if err != nil {
		s.failWrite(WriteFailureAbandoned)
	}
	if err == nil && s.writtenFunc != nil {
		s.writtenFunc()
	}
	if s.deliveryChan == nil {
		return
	}
	select {
	case s.deliveryChan <- err:
	default:
	}
}

func (s *Segment) failWrite(reason WriteFailure) {
	for span := s.writeSpans; span != nil; span = span.next {
		span.trace.Fail(reason)
	}
}

// NewSegment returns a new Segment given a protocol ID, payload bytes, and whether the segment
// is a response
func NewSegment(protocolId uint16, payload []byte, isResponse bool) *Segment {
	// time since unix epoch even as nanoseconds will not overflow soon
	// #nosec G115
	header := SegmentHeader{
		Timestamp:  uint32(time.Now().UnixNano() & 0xffffffff),
		ProtocolId: protocolId,
	}
	if isResponse {
		header.ProtocolId = header.ProtocolId + segmentProtocolIdResponseFlag
	}
	size := len(payload)
	if size > SegmentMaxPayloadLength || size > math.MaxUint16 {
		return nil
	}
	// payload size fits within length
	header.PayloadLength = uint16(size)
	segment := &Segment{
		SegmentHeader: header,
		Payload:       payload,
	}
	return segment
}

// IsRequest returns true if the segment is not a response
func (s *SegmentHeader) IsRequest() bool {
	return (s.ProtocolId & segmentProtocolIdResponseFlag) == 0
}

// IsResponse returns true if the segment is a response
func (s *SegmentHeader) IsResponse() bool {
	return (s.ProtocolId & segmentProtocolIdResponseFlag) > 0
}

// GetProtocolId returns the protocol ID of the segment
func (s *SegmentHeader) GetProtocolId() uint16 {
	if s.ProtocolId >= segmentProtocolIdResponseFlag {
		return s.ProtocolId - segmentProtocolIdResponseFlag
	}
	return s.ProtocolId
}
