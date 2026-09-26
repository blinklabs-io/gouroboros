// Copyright 2026 Blink Labs Software
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

package protocol

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

type messageScanFrame struct {
	remaining  uint64
	indefinite bool
}

type messageScanner struct {
	offset         int
	frames         []messageScanFrame
	started        bool
	complete       bool
	pendingBytes   uint64
	rootItems      uint64
	messageType    uint
	hasMessageType bool
	// processedBytes keeps the incremental scan work observable in tests.
	processedBytes int
}

type messageScanResult struct {
	messageType    uint
	hasMessageType bool
	started        bool
	messageLength  int
	complete       bool
	oversized      bool
}

type cborHead struct {
	major       byte
	additional  byte
	argument    uint64
	indefinite  bool
	isBreak     bool
	encodedSize int
}

func (s *messageScanner) scan(
	data []byte,
	maxBytes int,
) (messageScanResult, error) {
	if s.complete {
		return s.result(maxBytes), nil
	}
	for {
		if s.exceedsLimit(maxBytes) {
			return s.result(maxBytes), nil
		}
		if s.pendingBytes > 0 {
			available := uint64(len(data) - s.offset)
			consumed := min(available, s.pendingBytes)
			s.offset += int(consumed)
			s.pendingBytes -= consumed
			s.processedBytes += int(consumed)
			if s.pendingBytes > 0 {
				return s.result(maxBytes), nil
			}
			s.finishItem()
			if s.complete {
				return s.result(maxBytes), nil
			}
			continue
		}

		if !s.started {
			head, ok, err := readCBORHead(data, s.offset)
			if err != nil {
				return s.result(maxBytes), err
			}
			if !ok {
				return s.result(maxBytes), nil
			}
			if head.major != 4 || head.isBreak {
				return s.result(maxBytes), errors.New(
					"protocol message is not a CBOR array",
				)
			}
			s.offset += head.encodedSize
			s.processedBytes += head.encodedSize
			if s.exceedsLimit(maxBytes) {
				return s.result(maxBytes), nil
			}
			s.started = true
			if head.indefinite {
				if err := s.pushFrame(messageScanFrame{indefinite: true}); err != nil {
					return s.result(maxBytes), err
				}
			} else {
				if err := s.pushFrame(messageScanFrame{
					remaining: head.argument,
				}); err != nil {
					return s.result(maxBytes), err
				}
				s.finishItem()
				if s.complete {
					return s.result(maxBytes), nil
				}
			}
			continue
		}

		if len(s.frames) == 0 {
			s.complete = true
			return s.result(maxBytes), nil
		}
		if s.offset >= len(data) {
			return s.result(maxBytes), nil
		}

		parent := &s.frames[len(s.frames)-1]
		if parent.indefinite && data[s.offset] == 0xff {
			s.offset++
			s.frames = s.frames[:len(s.frames)-1]
			s.finishItem()
			if s.complete {
				return s.result(maxBytes), nil
			}
			continue
		}
		if !parent.indefinite && parent.remaining == 0 {
			s.frames = s.frames[:len(s.frames)-1]
			s.finishItem()
			if s.complete {
				return s.result(maxBytes), nil
			}
			continue
		}

		head, ok, err := readCBORHead(data, s.offset)
		if err != nil {
			return s.result(maxBytes), err
		}
		if !ok {
			return s.result(maxBytes), nil
		}
		firstRootItem := len(s.frames) == 1 && s.rootItems == 0
		if firstRootItem {
			if head.major != 0 || head.indefinite || head.isBreak {
				return s.result(maxBytes), errors.New(
					"protocol message type is not an unsigned integer",
				)
			}
			if head.argument > uint64(^uint(0)) {
				return s.result(maxBytes), errors.New(
					"protocol message type exceeds uint range",
				)
			}
			s.messageType = uint(head.argument)
			s.hasMessageType = true
		}
		if !parent.indefinite {
			parent.remaining--
		}
		if len(s.frames) == 1 {
			s.rootItems++
		}
		s.offset += head.encodedSize
		s.processedBytes += head.encodedSize
		if s.exceedsLimit(maxBytes) {
			return s.result(maxBytes), nil
		}

		switch head.major {
		case 0, 1:
			if head.indefinite || head.isBreak {
				return s.result(maxBytes), errors.New("invalid CBOR integer")
			}
			s.finishItem()
		case 2, 3:
			if head.isBreak {
				return s.result(maxBytes), errors.New("unexpected CBOR break")
			}
			if head.indefinite {
				if err := s.pushFrame(messageScanFrame{indefinite: true}); err != nil {
					return s.result(maxBytes), err
				}
				break
			}
			s.pendingBytes = head.argument
			if s.pendingBytes == 0 {
				s.finishItem()
			}
		case 4:
			if head.isBreak {
				return s.result(maxBytes), errors.New("unexpected CBOR break")
			}
			if err := s.pushFrame(messageScanFrame{
				remaining:  head.argument,
				indefinite: head.indefinite,
			}); err != nil {
				return s.result(maxBytes), err
			}
			s.finishItem()
		case 5:
			if head.isBreak {
				return s.result(maxBytes), errors.New("unexpected CBOR break")
			}
			if !head.indefinite && head.argument > ^uint64(0)/2 {
				return s.result(maxBytes), errors.New("CBOR map length overflows")
			}
			children := head.argument * 2
			if err := s.pushFrame(messageScanFrame{
				remaining:  children,
				indefinite: head.indefinite,
			}); err != nil {
				return s.result(maxBytes), err
			}
			s.finishItem()
		case 6:
			if head.indefinite || head.isBreak {
				return s.result(maxBytes), errors.New("invalid CBOR tag")
			}
			if err := s.pushFrame(messageScanFrame{remaining: 1}); err != nil {
				return s.result(maxBytes), err
			}
		case 7:
			if head.isBreak {
				return s.result(maxBytes), errors.New("unexpected CBOR break")
			}
			s.finishItem()
		default:
			return s.result(maxBytes), fmt.Errorf(
				"invalid CBOR major type %d",
				head.major,
			)
		}

		if firstRootItem {
			return s.result(maxBytes), nil
		}
		if s.complete {
			return s.result(maxBytes), nil
		}
	}
}

func (s *messageScanner) pushFrame(frame messageScanFrame) error {
	if len(s.frames) >= cbor.MaxNestedLevels {
		return fmt.Errorf(
			"CBOR nesting exceeds maximum depth %d",
			cbor.MaxNestedLevels,
		)
	}
	s.frames = append(s.frames, frame)
	return nil
}

func (s *messageScanner) finishItem() {
	for len(s.frames) > 0 {
		frame := s.frames[len(s.frames)-1]
		if frame.indefinite || frame.remaining > 0 {
			return
		}
		s.frames = s.frames[:len(s.frames)-1]
	}
	s.complete = true
}

func (s *messageScanner) exceedsLimit(maxBytes int) bool {
	if maxBytes <= 0 {
		return false
	}
	if s.offset > maxBytes {
		return true
	}
	return s.pendingBytes > uint64(maxBytes-s.offset)
}

func (s *messageScanner) result(maxBytes int) messageScanResult {
	return messageScanResult{
		messageType:    s.messageType,
		hasMessageType: s.hasMessageType,
		started:        s.started,
		messageLength:  s.offset,
		complete:       s.complete,
		oversized:      s.exceedsLimit(maxBytes),
	}
}

func readCBORHead(data []byte, offset int) (cborHead, bool, error) {
	if offset >= len(data) {
		return cborHead{}, false, nil
	}
	first := data[offset]
	head := cborHead{
		major:      first >> 5,
		additional: first & 0x1f,
	}
	switch {
	case head.additional < 24:
		head.argument = uint64(head.additional)
		head.encodedSize = 1
	case head.additional == 24:
		if len(data)-offset < 2 {
			return cborHead{}, false, nil
		}
		head.argument = uint64(data[offset+1])
		head.encodedSize = 2
	case head.additional == 25:
		if len(data)-offset < 3 {
			return cborHead{}, false, nil
		}
		head.argument = uint64(data[offset+1])<<8 | uint64(data[offset+2])
		head.encodedSize = 3
	case head.additional == 26:
		if len(data)-offset < 5 {
			return cborHead{}, false, nil
		}
		head.argument = uint64(data[offset+1])<<24 |
			uint64(data[offset+2])<<16 |
			uint64(data[offset+3])<<8 |
			uint64(data[offset+4])
		head.encodedSize = 5
	case head.additional == 27:
		if len(data)-offset < 9 {
			return cborHead{}, false, nil
		}
		head.argument = uint64(data[offset+1])<<56 |
			uint64(data[offset+2])<<48 |
			uint64(data[offset+3])<<40 |
			uint64(data[offset+4])<<32 |
			uint64(data[offset+5])<<24 |
			uint64(data[offset+6])<<16 |
			uint64(data[offset+7])<<8 |
			uint64(data[offset+8])
		head.encodedSize = 9
	case head.additional == 31:
		if head.major == 7 {
			head.isBreak = true
			head.encodedSize = 1
			return head, true, nil
		}
		if head.major < 2 || head.major > 5 {
			return cborHead{}, false, errors.New("invalid indefinite CBOR item")
		}
		head.indefinite = true
		head.encodedSize = 1
	case head.additional >= 28:
		return cborHead{}, false, errors.New("invalid CBOR additional information")
	}
	return head, true, nil
}
