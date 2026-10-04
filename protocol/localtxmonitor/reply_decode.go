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

package localtxmonitor

import "errors"

type replyCursor struct {
	data []byte
	pos  int
}

func (c *replyCursor) header(major byte) (uint64, bool, error) {
	if c.pos >= len(c.data) || c.data[c.pos]&0xe0 != major {
		return 0, false, errors.New("unexpected transaction reply field type")
	}
	additional := c.data[c.pos] & 31
	c.pos++
	if additional < 24 {
		return uint64(additional), false, nil
	}
	if additional == 31 && (major == 0x80 || major == 0x40) {
		return 0, true, nil
	}
	if additional > 27 {
		return 0, false, errors.New("invalid transaction reply field header")
	}
	width := 1 << (additional - 24)
	if int(width) > len(c.data)-c.pos {
		return 0, false, errors.New("truncated transaction reply field")
	}
	var value uint64
	for range width {
		value = value<<8 | uint64(c.data[c.pos])
		c.pos++
	}
	return value, false, nil
}

func (c *replyCursor) end(indefinite bool) error {
	if indefinite {
		if c.pos >= len(c.data) || c.data[c.pos] != 0xff {
			return errors.New("transaction reply array has extra fields")
		}
		c.pos++
	}
	return nil
}

func (c *replyCursor) bytes() error {
	length, indefinite, err := c.header(0x40)
	if err != nil {
		return err
	}
	if !indefinite {
		// #nosec G115 -- pos is within data, so the remainder is non-negative.
		if length > uint64(len(c.data)-c.pos) {
			return errors.New("truncated transaction reply bytes")
		}
		// #nosec G115 -- length is bounded by the remaining slice length.
		c.pos += int(length)
		return nil
	}
	for {
		if c.pos >= len(c.data) {
			return errors.New("unterminated transaction reply bytes")
		}
		if c.data[c.pos] == 0xff {
			c.pos++
			return nil
		}
		length, chunkIndefinite, err := c.header(0x40)
		if err != nil {
			return err
		}
		if chunkIndefinite {
			return errors.New("indefinite transaction reply chunk")
		}
		// #nosec G115 -- pos is within data, so the remainder is non-negative.
		if length > uint64(len(c.data)-c.pos) {
			return errors.New("truncated transaction reply chunk")
		}
		// #nosec G115 -- length is bounded by the remaining slice length.
		c.pos += int(length)
	}
}

func validateReplyNextTx(data []byte) error {
	c := replyCursor{data: data}
	count, indefinite, err := c.header(0x80)
	if err != nil {
		return err
	}
	if !indefinite && count != 1 && count != 2 {
		return errors.New("transaction reply must have one or two fields")
	}
	kind, _, err := c.header(0)
	if err != nil {
		return err
	}
	if kind > 255 {
		return errors.New("message type integer overflow")
	}
	absent := !indefinite && count == 1 ||
		indefinite && c.pos < len(data) && data[c.pos] == 0xff
	if !absent {
		count, wrapperIndefinite, err := c.header(0x80)
		if err != nil {
			return err
		}
		if !wrapperIndefinite && count != 2 {
			return errors.New("transaction wrapper must have two fields")
		}
		era, _, err := c.header(0)
		if err != nil {
			return err
		}
		if era > 255 {
			return errors.New("era id integer overflow")
		}
		tag, _, err := c.header(0xc0)
		if err != nil {
			return err
		}
		if tag != 24 {
			return errors.New("transaction bytes must use tag 24")
		}
		if err := c.bytes(); err != nil {
			return err
		}
		if err := c.end(wrapperIndefinite); err != nil {
			return err
		}
	}
	if err := c.end(indefinite); err != nil {
		return err
	}
	if c.pos != len(data) {
		return errors.New("trailing transaction reply data")
	}
	return nil
}
