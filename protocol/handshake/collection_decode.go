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

package handshake

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborwalk"
)

const (
	// A version-map entry needs at least one byte each for its uint16 key and
	// raw version data. The message and type consume two bytes; a count this
	// large uses a three-byte map header.
	maxHandshakeVersions = (MaxPendingMessageBytes - 5) / 2
	// Refusal reasons have two fields for version mismatch and three for a
	// decode error or explicit refusal.
	maxRefusalFields = 3
	// A version-mismatch refusal needs one byte each for the message, message
	// type, reason array and reason code. A count this large uses a three-byte
	// supported-version array header. Each uint16 version needs at least one byte.
	maxRefusalSupportedVersions = MaxPendingMessageBytes - 7
)

func validateVersionMapMessage(data []byte) error {
	collection, err := handshakeSecondField(data)
	if err != nil {
		return err
	}
	count, headerSize, indefinite, err := handshakeCollectionInfo(collection, 5, "handshake versions")
	if err != nil {
		return err
	}
	if !indefinite && count > maxHandshakeVersions {
		return tooManyHandshakeVersions(count)
	}
	pos := int(headerSize)
	var seen [1 << 13]byte
	for idx := 0; indefinite || idx < count; idx++ {
		position := pos
		if indefinite {
			if position >= len(collection) {
				return errors.New("unterminated handshake version map")
			}
			if collection[position] == 0xff {
				return nil
			}
			if idx >= maxHandshakeVersions {
				return tooManyHandshakeVersions(idx + 1)
			}
		}
		versionValue, consumed, err := decodeHandshakeUnsigned(
			collection[position:],
			uint64(^uint16(0)),
			"handshake version",
		)
		if err != nil {
			return fmt.Errorf("decode handshake version %d: %w", idx, err)
		}
		version := uint16(versionValue) // #nosec G115 -- bounded to uint16 above
		pos += consumed
		byteIndex := version >> 3
		bit := byte(1 << (version & 7))
		if seen[byteIndex]&bit != 0 {
			return fmt.Errorf("duplicate handshake version %d", version)
		}
		seen[byteIndex] |= bit
		if length, err := cborwalk.ItemLength(collection[pos:]); err != nil {
			return fmt.Errorf("decode handshake version data %d: %w", idx, err)
		} else {
			pos += length
		}
	}
	return nil
}

func validateRefusalMessage(data []byte) error {
	collection, err := handshakeSecondField(data)
	if err != nil {
		return err
	}
	count, headerSize, indefinite, err := handshakeCollectionInfo(collection, 4, "handshake refusal reason")
	if err != nil {
		return err
	}
	if !indefinite && count > maxRefusalFields {
		return tooManyRefusalFields(count)
	}
	pos := int(headerSize)
	var reason uint64
	for idx := 0; indefinite || idx < count; idx++ {
		position := pos
		if indefinite {
			if position >= len(collection) {
				return errors.New("unterminated handshake refusal reason")
			}
			if collection[position] == 0xff {
				break
			}
			if idx >= maxRefusalFields {
				return tooManyRefusalFields(idx + 1)
			}
		}
		if idx == 0 {
			var consumed int
			reason, consumed, err = decodeHandshakeUnsigned(
				collection[position:],
				^uint64(0),
				"handshake refusal reason",
			)
			if err != nil {
				return fmt.Errorf("decode handshake refusal reason: %w", err)
			}
			pos += consumed
			continue
		}
		if idx == 1 && reason == RefuseReasonVersionMismatch {
			if err := validateSupportedVersions(collection[position:]); err != nil {
				return err
			}
		}
		if length, err := cborwalk.ItemLength(collection[position:]); err != nil {
			return fmt.Errorf("decode handshake refusal field %d: %w", idx, err)
		} else {
			pos += length
		}
	}
	return nil
}

func validateSupportedVersions(data []byte) error {
	pos, err := skipHandshakeTags(data, 0)
	if err != nil {
		return err
	}
	count, headerSize, indefinite, err := handshakeCollectionInfo(data[pos:], 4, "handshake supported versions")
	if err != nil {
		return err
	}
	if !indefinite && count > maxRefusalSupportedVersions {
		return tooManySupportedVersions(count)
	}
	pos += int(headerSize)
	for idx := 0; indefinite || idx < count; idx++ {
		position := pos
		if indefinite {
			if position >= len(data) {
				return errors.New("unterminated handshake supported versions")
			}
			if data[position] == 0xff {
				return nil
			}
			if idx >= maxRefusalSupportedVersions {
				return tooManySupportedVersions(idx + 1)
			}
		}
		if length, err := cborwalk.ItemLength(data[position:]); err != nil {
			return fmt.Errorf("decode handshake supported version %d: %w", idx, err)
		} else {
			pos += length
		}
	}
	return nil
}

func handshakeSecondField(data []byte) ([]byte, error) {
	pos, err := skipHandshakeTags(data, 0)
	if err != nil {
		return nil, err
	}
	fieldCount, headerSize, indefinite, err := handshakeCollectionInfo(data[pos:], 4, "handshake message")
	if err != nil {
		return nil, err
	}
	if !indefinite && fieldCount != 2 {
		return nil, fmt.Errorf(
			"handshake message has %d fields, expected 2",
			fieldCount,
		)
	}
	pos += int(headerSize)
	_, consumed, err := decodeHandshakeUnsigned(
		data[pos:],
		^uint64(0),
		"handshake message type",
	)
	if err != nil {
		return nil, fmt.Errorf("decode handshake message type: %w", err)
	}
	pos += consumed
	pos, err = skipHandshakeTags(data, pos)
	if err != nil {
		return nil, err
	}
	return data[pos:], nil
}

func decodeHandshakeUnsigned(
	data []byte,
	maxValue uint64,
	label string,
) (uint64, int, error) {
	head, ok, err := cborwalk.ReadHead(data, 0)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid %s: %w", label, err)
	}
	if !ok {
		return 0, 0, fmt.Errorf("truncated %s", label)
	}
	if head.Major != 0 || head.Indefinite || head.IsBreak {
		return 0, 0, fmt.Errorf("%s must be an unsigned integer", label)
	}
	if head.Argument > maxValue {
		return 0, 0, fmt.Errorf("%s exceeds its wire range", label)
	}
	return head.Argument, head.EncodedSize, nil
}

func handshakeCollectionInfo(data []byte, major byte, label string) (int, uint32, bool, error) {
	head, ok, err := cborwalk.ReadHead(data, 0)
	if err != nil {
		return 0, 0, false, fmt.Errorf("invalid %s header: %w", label, err)
	}
	if !ok {
		return 0, 0, false, fmt.Errorf("truncated %s header", label)
	}
	if head.Major != major || head.IsBreak {
		kind := "array"
		if major == 5 {
			kind = "map"
		}
		return 0, 0, false, fmt.Errorf("%s is not a %s", label, kind)
	}
	var count int
	var headerSize uint32
	var indefinite bool
	if major == 4 {
		count, headerSize, indefinite = cbor.ArrayInfo(data)
	} else {
		count, headerSize, indefinite = cbor.MapInfo(data)
	}
	if count < 0 {
		return 0, 0, false, fmt.Errorf("invalid %s header", label)
	}
	return count, headerSize, indefinite, nil
}

func skipHandshakeTags(data []byte, pos int) (int, error) {
	for depth := 0; pos < len(data) &&
		data[pos]&cbor.CborTypeMask == cbor.CborTypeTag; depth++ {
		if depth >= cbor.MaxNestedLevels {
			return 0, errors.New("handshake tag nesting limit")
		}
		additional := data[pos] & 0x1f
		width := 0
		switch additional {
		case 24:
			width = 1
		case 25:
			width = 2
		case 26:
			width = 4
		case 27:
			width = 8
		case 28, 29, 30, 31:
			return 0, errors.New("invalid handshake tag")
		}
		if width >= len(data)-pos {
			return 0, errors.New("truncated handshake tag")
		}
		pos += width + 1
	}
	if pos >= len(data) {
		return 0, errors.New("truncated handshake message")
	}
	return pos, nil
}

func tooManyHandshakeVersions(count int) error {
	return fmt.Errorf(
		"handshake version map has %d entries, maximum is %d",
		count,
		maxHandshakeVersions,
	)
}

func tooManyRefusalFields(count int) error {
	return fmt.Errorf(
		"handshake refusal reason has %d fields, maximum is %d",
		count,
		maxRefusalFields,
	)
}

func tooManySupportedVersions(count int) error {
	return fmt.Errorf(
		"handshake supported versions has %d entries, maximum is %d",
		count,
		maxRefusalSupportedVersions,
	)
}
