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

package leiosfetch

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborpreflight"
)

// UnmarshalCBOR admits every vote-ID shape before typed slice allocation.
func (m *MsgVotesRequest) UnmarshalCBOR(data []byte) error {
	if err := cborpreflight.ValidateItemDepth(data, 4, "vote request"); err != nil {
		return err
	}
	if err := validateVoteRequest(data); err != nil {
		return err
	}
	type request MsgVotesRequest
	var tmp request
	if _, err := cbor.Decode(data, &tmp); err != nil {
		return err
	}
	*m = MsgVotesRequest(tmp)
	return nil
}

type cborCursor struct {
	data []byte
	pos  int
}

func (c *cborCursor) header(major byte) (uint64, bool, error) {
	if c.pos >= len(c.data) || c.data[c.pos]&0xe0 != major {
		return 0, false, errors.New("unexpected CBOR field type")
	}
	additional := c.data[c.pos] & 31
	c.pos++
	if additional < 24 {
		return uint64(additional), false, nil
	}
	if additional == 31 && major == 0x80 {
		return 0, true, nil
	}
	if additional > 27 {
		return 0, false, errors.New("invalid CBOR field header")
	}
	width := 1 << (additional - 24)
	if int(width) > len(c.data)-c.pos {
		return 0, false, errors.New("truncated CBOR field")
	}
	var value uint64
	for range width {
		value = value<<8 | uint64(c.data[c.pos])
		c.pos++
	}
	return value, false, nil
}

func (c *cborCursor) end(indefinite bool) error {
	if indefinite {
		if c.pos >= len(c.data) || c.data[c.pos] != 0xff {
			return errors.New("CBOR array has extra fields")
		}
		c.pos++
	}
	return nil
}

func (c *cborCursor) skipTags() error {
	for depth := 0; c.pos < len(c.data) && c.data[c.pos]&0xe0 == 0xc0; depth++ {
		if depth >= cbor.MaxNestedLevels {
			return errors.New("CBOR tag nesting limit")
		}
		if _, _, err := c.header(0xc0); err != nil {
			return err
		}
	}
	return nil
}

func (c *cborCursor) unsigned() (uint64, error) {
	start := c.pos
	if err := c.skipTags(); err != nil {
		return 0, err
	}
	tagged := c.pos != start
	if c.pos >= len(c.data) {
		return 0, errors.New("truncated CBOR scalar")
	}
	var value uint64
	switch c.data[c.pos] & 0xe0 {
	case 0:
		var err error
		value, _, err = c.header(0)
		if err != nil {
			return 0, err
		}
	case 0x40:
		// Positive bignum tags can still encode a uint64, including leading zeros.
		length, _, err := c.header(0x40)
		if err != nil {
			return 0, err
		}
		// #nosec G115 -- pos is within data, so the remainder is non-negative.
		if length > uint64(len(c.data)-c.pos) {
			return 0, errors.New("truncated CBOR bignum")
		}
		// #nosec G115 -- length is bounded by the remaining slice length.
		c.pos += int(length)
	default:
		return 0, errors.New("CBOR field is not unsigned")
	}
	if tagged || c.data[start]&0xe0 != 0 {
		return decodeTaggedUnsigned(c.data[start:c.pos])
	}
	return value, nil
}

func decodeTaggedUnsigned(data []byte) (uint64, error) {
	var value uint64
	if _, err := cbor.Decode(data, &value); err != nil {
		return 0, err
	}
	return value, nil
}

func validateVoteRequest(data []byte) error {
	c := cborCursor{data: data}
	if err := c.skipTags(); err != nil {
		return err
	}
	count, indefinite, err := c.header(0x80)
	if err != nil {
		return err
	}
	if !indefinite && count != 2 {
		return errors.New("vote request must have two fields")
	}
	messageType, err := c.unsigned()
	if err != nil {
		return err
	}
	if messageType != MessageTypeVotesRequest {
		return errors.New("vote request message type must be 4")
	}
	if err := c.skipTags(); err != nil {
		return err
	}
	// A nil VoteIds slice is encoded as null by NewMsgVotesRequest.
	if c.pos < len(data) && (data[c.pos] == 0xf6 || data[c.pos] == 0xf7) {
		c.pos++
		if err := c.end(indefinite); err != nil {
			return err
		}
		if c.pos != len(data) {
			return errors.New("trailing vote request data")
		}
		return nil
	}
	count, listIndefinite, err := c.header(0x80)
	if err != nil {
		return err
	}
	// Every two-scalar ID needs an array header and at least two scalar bytes.
	// #nosec G115 -- pos is within data, so the remainder is non-negative.
	if !listIndefinite && count > uint64((len(data)-c.pos)/3) {
		return errors.New("vote ID count exceeds encoded entries")
	}
	for idx := uint64(0); listIndefinite || idx < count; idx++ {
		if listIndefinite && c.pos < len(data) && data[c.pos] == 0xff {
			c.pos++
			break
		}
		if err := c.skipTags(); err != nil {
			return fmt.Errorf("vote ID %d: %w", idx, err)
		}
		fields, itemIndefinite, err := c.header(0x80)
		if err != nil {
			return fmt.Errorf("vote ID %d: %w", idx, err)
		}
		if !itemIndefinite && fields != 2 {
			return errors.New("vote ID must have two fields")
		}
		for range 2 {
			if _, err := c.unsigned(); err != nil {
				return fmt.Errorf("vote ID %d: %w", idx, err)
			}
		}
		if err := c.end(itemIndefinite); err != nil {
			return err
		}
	}
	if err := c.end(indefinite); err != nil {
		return err
	}
	if c.pos != len(data) {
		return errors.New("trailing vote request data")
	}
	return nil
}
