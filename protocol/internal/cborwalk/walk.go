// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cborwalk

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

type Head struct {
	Major       byte
	Additional  byte
	Argument    uint64
	Indefinite  bool
	IsBreak     bool
	EncodedSize int
}

func ReadHead(data []byte, offset int) (Head, bool, error) {
	if offset >= len(data) {
		return Head{}, false, nil
	}
	first := data[offset]
	head := Head{Major: first >> 5, Additional: first & 0x1f}
	switch {
	case head.Additional < 24:
		head.Argument, head.EncodedSize = uint64(head.Additional), 1
	case head.Additional == 24:
		if len(data)-offset < 2 {
			return Head{}, false, nil
		}
		head.Argument, head.EncodedSize = uint64(data[offset+1]), 2
	case head.Additional == 25:
		if len(data)-offset < 3 {
			return Head{}, false, nil
		}
		head.Argument = uint64(data[offset+1])<<8 | uint64(data[offset+2])
		head.EncodedSize = 3
	case head.Additional == 26:
		if len(data)-offset < 5 {
			return Head{}, false, nil
		}
		head.Argument = uint64(data[offset+1])<<24 | uint64(data[offset+2])<<16 | uint64(data[offset+3])<<8 | uint64(data[offset+4])
		head.EncodedSize = 5
	case head.Additional == 27:
		if len(data)-offset < 9 {
			return Head{}, false, nil
		}
		for i := 1; i <= 8; i++ {
			head.Argument = head.Argument<<8 | uint64(data[offset+i])
		}
		head.EncodedSize = 9
	case head.Additional == 31:
		if head.Major == 7 {
			head.IsBreak, head.EncodedSize = true, 1
			return head, true, nil
		}
		if head.Major < 2 || head.Major > 5 {
			return Head{}, false, errors.New("invalid indefinite CBOR item")
		}
		head.Indefinite, head.EncodedSize = true, 1
	case head.Additional >= 28:
		return Head{}, false, errors.New("invalid CBOR additional information")
	}
	return head, true, nil
}

type frame struct {
	remaining   uint64
	indefinite  bool
	stringMajor byte
}

// ItemLength validates one complete CBOR item without recursion and returns
// its encoded length. The explicit stack keeps peer-controlled nesting off the
// Go call stack.
func ItemLength(data []byte) (int, error) {
	frames := []frame{{remaining: 1}}
	pos := 0
	for len(frames) > 0 {
		top := &frames[len(frames)-1]
		if !top.indefinite && top.remaining == 0 {
			frames = frames[:len(frames)-1]
			continue
		}
		if pos >= len(data) {
			return 0, errors.New("truncated CBOR item")
		}
		if top.indefinite && data[pos] == 0xff {
			pos++
			frames = frames[:len(frames)-1]
			continue
		}
		head, ok, err := ReadHead(data, pos)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, errors.New("truncated CBOR item header")
		}
		if head.IsBreak {
			return 0, errors.New("unexpected CBOR break")
		}
		if top.stringMajor != 0 && (head.Major != top.stringMajor || head.Indefinite) {
			return 0, errors.New("invalid CBOR indefinite string chunk")
		}
		if !top.indefinite {
			top.remaining--
		}
		pos += head.EncodedSize
		push := func(f frame) error {
			if len(frames) >= cbor.MaxNestedLevels {
				return fmt.Errorf("CBOR nesting exceeds maximum depth %d", cbor.MaxNestedLevels)
			}
			frames = append(frames, f)
			return nil
		}
		switch head.Major {
		case 0, 1, 7:
		case 2, 3:
			if head.Indefinite {
				if err := push(frame{indefinite: true, stringMajor: head.Major}); err != nil {
					return 0, err
				}
			} else if head.Argument > uint64(len(data)-pos) { // #nosec G115 -- pos is within data
				return 0, errors.New("truncated CBOR string")
			} else {
				pos += int(head.Argument) // #nosec G115 -- bounded by remaining input
			}
		case 4:
			if err := push(frame{remaining: head.Argument, indefinite: head.Indefinite}); err != nil {
				return 0, err
			}
		case 5:
			if !head.Indefinite && head.Argument > ^uint64(0)/2 {
				return 0, errors.New("CBOR map length overflows")
			}
			if err := push(frame{remaining: head.Argument * 2, indefinite: head.Indefinite}); err != nil {
				return 0, err
			}
		case 6:
			if head.Indefinite {
				return 0, errors.New("invalid CBOR tag")
			}
			if err := push(frame{remaining: 1}); err != nil {
				return 0, err
			}
		default:
			return 0, fmt.Errorf("invalid CBOR major type %d", head.Major)
		}
	}
	return pos, nil
}
