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

package byron

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

func byronArrayFields(raw []byte, name string) ([]cbor.RawMessage, error) {
	length, headerSize, indefinite := cbor.ArrayInfo(raw)
	if indefinite || length < 0 {
		return nil, fmt.Errorf("%s must be a definite-length array", name)
	}
	if length > 32 {
		return nil, fmt.Errorf("%s has too many fields: %d", name, length)
	}
	pos := int(headerSize)
	fields := make([]cbor.RawMessage, 0, length)
	for i := 0; i < length; i++ {
		start := pos
		var err error
		pos, err = scanByronItem(raw, pos, 0)
		if err != nil {
			return nil, fmt.Errorf("decode %s field %d: %w", name, i, err)
		}
		fields = append(fields, cbor.RawMessage(raw[start:pos]))
	}
	if pos != len(raw) {
		return nil, fmt.Errorf("%s has trailing CBOR data", name)
	}
	return fields, nil
}

func requireByronArrayLength(raw []byte, name string, expected int) error {
	length, _, indefinite := cbor.ArrayInfo(raw)
	if indefinite || length < 0 {
		return fmt.Errorf("%s must be a definite-length array", name)
	}
	if length != expected {
		return fmt.Errorf("%s has %d fields, expected %d", name, length, expected)
	}
	return nil
}

func byronArrayField(raw []byte, name string, index, expected int) ([]byte, error) {
	length, headerSize, indefinite := cbor.ArrayInfo(raw)
	if indefinite || length != expected || index < 0 || index >= length {
		return nil, fmt.Errorf("%s must be a definite array of %d fields", name, expected)
	}
	pos := int(headerSize)
	var selected []byte
	for i := 0; i < length; i++ {
		start := pos
		var err error
		pos, err = scanByronItem(raw, pos, 0)
		if err != nil {
			return nil, err
		}
		if i == index {
			selected = raw[start:pos]
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("%s field %d was not found", name, index)
	}
	if pos != len(raw) {
		return nil, fmt.Errorf("%s has trailing CBOR data", name)
	}
	return selected, nil
}

func validateByronDefiniteStrings(raw []byte) error {
	consumed, err := scanByronItem(raw, 0, 0)
	if err != nil {
		return err
	}
	if consumed != len(raw) {
		return fmt.Errorf("trailing Byron CBOR data at byte %d", consumed)
	}
	return nil
}

func scanByronItem(raw []byte, pos, depth int) (int, error) {
	if depth > cbor.MaxNestedLevels {
		return 0, fmt.Errorf("byron CBOR nesting exceeds %d", cbor.MaxNestedLevels)
	}
	if pos >= len(raw) {
		return 0, errors.New("unexpected end of Byron CBOR")
	}
	first := raw[pos]
	major, additional := first>>5, first&0x1f
	if major == 7 && additional == 31 {
		return pos + 1, nil
	}
	if major == 7 {
		switch additional {
		case 24:
			if pos+2 > len(raw) {
				return 0, errors.New("truncated Byron CBOR simple value")
			}
			return pos + 2, nil
		case 25:
			if pos+3 > len(raw) {
				return 0, errors.New("truncated Byron CBOR half-float")
			}
			return pos + 3, nil
		case 26:
			if pos+5 > len(raw) {
				return 0, errors.New("truncated Byron CBOR float")
			}
			return pos + 5, nil
		case 27:
			if pos+9 > len(raw) {
				return 0, errors.New("truncated Byron CBOR double")
			}
			return pos + 9, nil
		case 28, 29, 30:
			return 0, fmt.Errorf("invalid Byron CBOR simple value at byte %d", pos)
		default:
			return pos + 1, nil
		}
	}
	var arg uint64
	headLen := 1
	var err error
	if additional == 31 {
		if major == 2 || major == 3 {
			return 0, fmt.Errorf("indefinite-length CBOR string at byte %d", pos)
		}
		if major != 4 && major != 5 {
			return 0, fmt.Errorf("invalid Byron CBOR additional information at byte %d", pos)
		}
	} else {
		arg, headLen, err = byronCBORArgument(raw[pos:], additional)
		if err != nil {
			return 0, fmt.Errorf("invalid Byron CBOR argument at byte %d: %w", pos, err)
		}
	}
	itemPos := pos + headLen
	switch major {
	case 0, 1:
		return itemPos, nil
	case 2, 3:
		if additional == 31 {
			return 0, fmt.Errorf("indefinite-length CBOR string at byte %d", pos)
		}
		// itemPos is within raw because the item header was parsed above;
		// converting the remaining slice length to uint64 is safe.
		if arg > uint64(len(raw)-itemPos) { //nolint:gosec
			return 0, fmt.Errorf("truncated Byron CBOR string at byte %d", pos)
		}
		// arg is bounded by the remaining slice length, so it fits int.
		return itemPos + int(arg), nil //nolint:gosec
	case 4, 5:
		indefinite := additional == 31
		items := arg
		if major == 5 && !indefinite {
			// len is bounded by MaxInt, so converting the remaining length is safe.
			if items > uint64((len(raw)-itemPos)/2) { //nolint:gosec
				return 0, fmt.Errorf("truncated Byron CBOR map at byte %d", pos)
			}
		} else if major == 4 && !indefinite && items > uint64(len(raw)-itemPos) { //nolint:gosec
			return 0, fmt.Errorf("truncated Byron CBOR array at byte %d", pos)
		}
		for i := uint64(0); indefinite || i < items; i++ {
			if itemPos >= len(raw) {
				return 0, fmt.Errorf("unterminated Byron CBOR collection at byte %d", pos)
			}
			if indefinite && raw[itemPos] == 0xff {
				return itemPos + 1, nil
			}
			itemPos, err = scanByronItem(raw, itemPos, depth+1)
			if err != nil {
				return 0, err
			}
			if major == 5 {
				if itemPos >= len(raw) || (indefinite && raw[itemPos] == 0xff) {
					return 0, fmt.Errorf("incomplete Byron CBOR map pair at byte %d", itemPos)
				}
				itemPos, err = scanByronItem(raw, itemPos, depth+1)
				if err != nil {
					return 0, err
				}
			}
		}
		return itemPos, nil
	case 6:
		return scanByronItem(raw, itemPos, depth+1)
	case 7:
		return 0, fmt.Errorf("invalid Byron CBOR simple value at byte %d", pos)
	default:
		return 0, fmt.Errorf("invalid Byron CBOR major type %d at byte %d", major, pos)
	}
}

func validateTransactionAttributes(raw []byte) error {
	length, headerSize, indefinite := cbor.MapInfo(raw)
	if indefinite || length < 0 {
		return errors.New("attributes must be a definite-length map")
	}
	pos := int(headerSize)
	var previous uint64
	for i := 0; i < length; i++ {
		keyStart := pos
		var err error
		pos, err = scanByronItem(raw, pos, 0)
		if err != nil {
			return fmt.Errorf("decode attribute key %d: %w", i, err)
		}
		var key uint64
		if consumed, decodeErr := cbor.Decode(raw[keyStart:pos], &key); decodeErr != nil || consumed != pos-keyStart || key > 0xff {
			return fmt.Errorf("attribute key %d must be a Word8", i)
		}
		if i > 0 && key <= previous {
			return errors.New("attribute keys must be strictly increasing")
		}
		previous = key
		valueStart := pos
		pos, err = scanByronItem(raw, pos, 0)
		if err != nil {
			return fmt.Errorf("decode attribute value %d: %w", i, err)
		}
		if _, err := decodeByronByteString(raw[valueStart:pos], false); err != nil {
			return fmt.Errorf("attribute %d value must be a definite CBOR byte string: %w", key, err)
		}
	}
	if pos != len(raw) {
		return errors.New("transaction attributes contain trailing CBOR data")
	}
	return nil
}

func requireByronByteString(raw []byte, name string) error {
	_, err := decodeByronByteString(raw, false)
	if err != nil {
		return fmt.Errorf("%s must be a definite CBOR byte string: %w", name, err)
	}
	return nil
}

func requireByronTextString(raw []byte, name string) error {
	if len(raw) == 0 || raw[0]&cbor.CborTypeMask != cbor.CborTypeTextString {
		return fmt.Errorf("%s must be a CBOR text string", name)
	}
	additional := raw[0] & 0x1f
	if additional == 31 {
		return fmt.Errorf("%s must use definite-length text framing", name)
	}
	length, headerLen, err := byronCBORArgument(raw, additional)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	// headerLen was bounds-checked by byronCBORArgument; len is MaxInt-bounded.
	if length > uint64(len(raw)-headerLen) || uint64(len(raw)-headerLen) != length { //nolint:gosec
		return fmt.Errorf("%s has truncated text or trailing CBOR data", name)
	}
	var value string
	consumed, err := cbor.Decode(raw, &value)
	if err != nil || consumed != len(raw) {
		return fmt.Errorf("%s is invalid UTF-8 or has trailing CBOR data", name)
	}
	return nil
}

func requireCanonicalByronByteString(raw []byte, name string) ([]byte, error) {
	value, err := decodeByronByteString(raw, true)
	if err != nil {
		return nil, fmt.Errorf("%s must use a canonical CBOR byte string: %w", name, err)
	}
	return value, nil
}

func requireByronVerificationKey(raw []byte, name string) error {
	key, err := requireCanonicalByronByteString(raw, name)
	if err != nil {
		return err
	}
	if len(key) != VerificationKeySize {
		return fmt.Errorf("%s is %d bytes, expected %d", name, len(key), VerificationKeySize)
	}
	return nil
}

func decodeByronByteString(raw []byte, canonical bool) ([]byte, error) {
	if len(raw) == 0 || raw[0]&cbor.CborTypeMask != cbor.CborTypeByteString {
		return nil, errors.New("wrong CBOR major type")
	}
	additional := raw[0] & 0x1f
	if additional == 31 {
		return nil, errors.New("indefinite-length string")
	}
	length, headerLen, err := byronCBORArgument(raw, additional)
	if err != nil {
		return nil, err
	}
	if canonical && !byronCBORArgumentIsShortest(additional, length) {
		return nil, errors.New("non-shortest string length")
	}
	// headerLen was bounds-checked by byronCBORArgument; len is MaxInt-bounded.
	if length > uint64(len(raw)-headerLen) || uint64(len(raw)-headerLen) != length { //nolint:gosec
		return nil, errors.New("truncated string or trailing CBOR data")
	}
	return raw[headerLen:], nil
}

func byronCBORArgument(raw []byte, additional byte) (uint64, int, error) {
	switch {
	case additional < 24:
		return uint64(additional), 1, nil
	case additional == 24:
		if len(raw) < 2 {
			return 0, 0, errors.New("truncated CBOR argument")
		}
		return uint64(raw[1]), 2, nil
	case additional == 25:
		if len(raw) < 3 {
			return 0, 0, errors.New("truncated CBOR argument")
		}
		return uint64(raw[1])<<8 | uint64(raw[2]), 3, nil
	case additional == 26:
		if len(raw) < 5 {
			return 0, 0, errors.New("truncated CBOR argument")
		}
		return uint64(raw[1])<<24 | uint64(raw[2])<<16 |
			uint64(raw[3])<<8 | uint64(raw[4]), 5, nil
	case additional == 27:
		if len(raw) < 9 {
			return 0, 0, errors.New("truncated CBOR argument")
		}
		var value uint64
		for _, b := range raw[1:9] {
			value = value<<8 | uint64(b)
		}
		return value, 9, nil
	default:
		return 0, 0, errors.New("reserved CBOR argument")
	}
}

func byronCBORArgumentIsShortest(additional byte, value uint64) bool {
	switch {
	case value < 24:
		return additional < 24
	case value <= 0xff:
		return additional == 24
	case value <= 0xffff:
		return additional == 25
	case value <= 0xffffffff:
		return additional == 26
	default:
		return additional == 27
	}
}
