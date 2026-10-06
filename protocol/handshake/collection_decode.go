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
)

const (
	// A version-map entry needs at least one byte each for its uint16 key and
	// raw version data. The message, type and map headers consume three bytes.
	maxHandshakeVersions = (MaxPendingMessageBytes - 3) / 2
	// Refusal reasons have two fields for version mismatch and three for a
	// decode error or explicit refusal.
	maxRefusalFields = 3
	// A version-mismatch refusal needs one byte each for the message, message
	// type, reason array, reason code and supported-version array headers. Each
	// uint16 version needs at least one byte.
	maxRefusalSupportedVersions = MaxPendingMessageBytes - 5
)

func validateVersionMapMessage(data []byte) error {
	collection, err := handshakeSecondField(data)
	if err != nil {
		return err
	}
	count, headerSize, indefinite := cbor.MapInfo(collection)
	if count < 0 {
		return errors.New("handshake versions are not a map")
	}
	if !indefinite && count > maxHandshakeVersions {
		return tooManyHandshakeVersions(count)
	}
	dec, err := cbor.NewStreamDecoder(collection)
	if err != nil {
		return err
	}
	if err := dec.Advance(int(headerSize)); err != nil {
		return err
	}
	var seen [1 << 13]byte
	for idx := 0; indefinite || idx < count; idx++ {
		position := dec.Position()
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
		var version uint16
		if _, _, err := dec.Decode(&version); err != nil {
			return fmt.Errorf("decode handshake version %d: %w", idx, err)
		}
		byteIndex := version >> 3
		bit := byte(1 << (version & 7))
		if seen[byteIndex]&bit != 0 {
			return fmt.Errorf("duplicate handshake version %d", version)
		}
		seen[byteIndex] |= bit
		if _, _, err := dec.Skip(); err != nil {
			return fmt.Errorf("decode handshake version data %d: %w", idx, err)
		}
	}
	return nil
}

func validateRefusalMessage(data []byte) error {
	collection, err := handshakeSecondField(data)
	if err != nil {
		return err
	}
	count, headerSize, indefinite := cbor.ArrayInfo(collection)
	if count < 0 {
		return errors.New("handshake refusal reason is not an array")
	}
	if !indefinite && count > maxRefusalFields {
		return tooManyRefusalFields(count)
	}
	dec, err := cbor.NewStreamDecoder(collection)
	if err != nil {
		return err
	}
	if err := dec.Advance(int(headerSize)); err != nil {
		return err
	}
	var reason uint64
	for idx := 0; indefinite || idx < count; idx++ {
		position := dec.Position()
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
			if _, _, err := dec.Decode(&reason); err != nil {
				return fmt.Errorf("decode handshake refusal reason: %w", err)
			}
			continue
		}
		if idx == 1 && reason == RefuseReasonVersionMismatch {
			if err := validateSupportedVersions(collection[position:]); err != nil {
				return err
			}
		}
		if _, _, err := dec.Skip(); err != nil {
			return fmt.Errorf("decode handshake refusal field %d: %w", idx, err)
		}
	}
	return nil
}

func validateSupportedVersions(data []byte) error {
	pos, err := skipHandshakeTags(data, 0)
	if err != nil {
		return err
	}
	count, headerSize, indefinite := cbor.ArrayInfo(data[pos:])
	if count < 0 {
		return errors.New("handshake supported versions are not an array")
	}
	if !indefinite && count > maxRefusalSupportedVersions {
		return tooManySupportedVersions(count)
	}
	pos += int(headerSize)
	dec, err := cbor.NewStreamDecoder(data[pos:])
	if err != nil {
		return err
	}
	for idx := 0; indefinite || idx < count; idx++ {
		position := dec.Position()
		if indefinite {
			if position >= len(data)-pos {
				return errors.New("unterminated handshake supported versions")
			}
			if data[pos+position] == 0xff {
				return nil
			}
			if idx >= maxRefusalSupportedVersions {
				return tooManySupportedVersions(idx + 1)
			}
		}
		if _, _, err := dec.Skip(); err != nil {
			return fmt.Errorf("decode handshake supported version %d: %w", idx, err)
		}
	}
	return nil
}

func handshakeSecondField(data []byte) ([]byte, error) {
	pos, err := skipHandshakeTags(data, 0)
	if err != nil {
		return nil, err
	}
	fieldCount, headerSize, indefinite := cbor.ArrayInfo(data[pos:])
	if fieldCount < 0 {
		return nil, errors.New("handshake message is not an array")
	}
	if !indefinite && fieldCount != 2 {
		return nil, fmt.Errorf(
			"handshake message has %d fields, expected 2",
			fieldCount,
		)
	}
	pos += int(headerSize)
	dec, err := cbor.NewStreamDecoder(data[pos:])
	if err != nil {
		return nil, err
	}
	var messageType uint
	if _, _, err := dec.Decode(&messageType); err != nil {
		return nil, fmt.Errorf("decode handshake message type: %w", err)
	}
	pos += dec.Position()
	pos, err = skipHandshakeTags(data, pos)
	if err != nil {
		return nil, err
	}
	return data[pos:], nil
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
