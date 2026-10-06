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
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborwalk"
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

func (s *messageScanner) scan(
	data []byte,
	maxBytes int,
) (messageScanResult, error) {
	if s.complete {
		return s.result(maxBytes), nil
	}
	if s.offset > len(data) {
		return s.result(maxBytes), errors.New(
			"CBOR input shrank during incremental scan",
		)
	}
	for {
		if s.exceedsLimit(maxBytes) {
			return s.result(maxBytes), nil
		}
		if s.pendingBytes > 0 {
			// #nosec G115 -- offset is within the slice, so this is non-negative.
			available := uint64(len(data) - s.offset)
			consumed := min(available, s.pendingBytes)
			// #nosec G115 -- consumed is bounded by the slice length.
			s.offset += int(consumed)
			s.pendingBytes -= consumed
			// #nosec G115 -- consumed is bounded by the slice length.
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
			head, ok, err := cborwalk.ReadHead(data, s.offset)
			if err != nil {
				return s.result(maxBytes), err
			}
			if !ok {
				return s.result(maxBytes), nil
			}
			if head.Major != 4 || head.IsBreak {
				return s.result(maxBytes), errors.New(
					"protocol message is not a CBOR array",
				)
			}
			s.offset += head.EncodedSize
			s.processedBytes += head.EncodedSize
			if s.exceedsLimit(maxBytes) {
				return s.result(maxBytes), nil
			}
			s.started = true
			if head.Indefinite {
				if err := s.pushFrame(messageScanFrame{indefinite: true}); err != nil {
					return s.result(maxBytes), err
				}
			} else {
				if err := s.pushFrame(messageScanFrame{
					remaining: head.Argument,
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

		head, ok, err := cborwalk.ReadHead(data, s.offset)
		if err != nil {
			return s.result(maxBytes), err
		}
		if !ok {
			return s.result(maxBytes), nil
		}
		firstRootItem := len(s.frames) == 1 && s.rootItems == 0
		if firstRootItem {
			if head.Major != 0 || head.Indefinite || head.IsBreak {
				return s.result(maxBytes), errors.New(
					"protocol message type is not an unsigned integer",
				)
			}
			if head.Argument > uint64(^uint(0)) {
				return s.result(maxBytes), errors.New(
					"protocol message type exceeds uint range",
				)
			}
			s.messageType = uint(head.Argument)
			s.hasMessageType = true
		}
		if !parent.indefinite {
			parent.remaining--
		}
		if len(s.frames) == 1 {
			s.rootItems++
		}
		s.offset += head.EncodedSize
		s.processedBytes += head.EncodedSize
		if s.exceedsLimit(maxBytes) {
			return s.result(maxBytes), nil
		}

		switch head.Major {
		case 0, 1:
			if head.Indefinite || head.IsBreak {
				return s.result(maxBytes), errors.New("invalid CBOR integer")
			}
			s.finishItem()
		case 2, 3:
			if head.IsBreak {
				return s.result(maxBytes), errors.New("unexpected CBOR break")
			}
			if head.Indefinite {
				if err := s.pushFrame(messageScanFrame{indefinite: true}); err != nil {
					return s.result(maxBytes), err
				}
				break
			}
			s.pendingBytes = head.Argument
			if s.pendingBytes == 0 {
				s.finishItem()
			}
		case 4:
			if head.IsBreak {
				return s.result(maxBytes), errors.New("unexpected CBOR break")
			}
			if err := s.pushFrame(messageScanFrame{
				remaining:  head.Argument,
				indefinite: head.Indefinite,
			}); err != nil {
				return s.result(maxBytes), err
			}
			s.finishItem()
		case 5:
			if head.IsBreak {
				return s.result(maxBytes), errors.New("unexpected CBOR break")
			}
			if !head.Indefinite && head.Argument > ^uint64(0)/2 {
				return s.result(maxBytes), errors.New("CBOR map length overflows")
			}
			children := head.Argument * 2
			if err := s.pushFrame(messageScanFrame{
				remaining:  children,
				indefinite: head.Indefinite,
			}); err != nil {
				return s.result(maxBytes), err
			}
			s.finishItem()
		case 6:
			if head.Indefinite || head.IsBreak {
				return s.result(maxBytes), errors.New("invalid CBOR tag")
			}
			if err := s.pushFrame(messageScanFrame{remaining: 1}); err != nil {
				return s.result(maxBytes), err
			}
		case 7:
			if head.IsBreak {
				return s.result(maxBytes), errors.New("unexpected CBOR break")
			}
			s.finishItem()
		default:
			return s.result(maxBytes), fmt.Errorf(
				"invalid CBOR major type %d",
				head.Major,
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
	// #nosec G115 -- the remaining size is non-negative and bounded by maxBytes.
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
