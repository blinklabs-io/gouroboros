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

package cbor

import (
	"errors"
	"fmt"

	_cbor "github.com/fxamacker/cbor/v2"
)

// rawMapPresizePairs caps the capacity DecodeRawMap reserves from a declared
// pair count. It covers every transaction body and protocol parameter update
// field map without growth, while a larger declared count costs only the
// entries that actually decode.
const rawMapPresizePairs = 32

// DecodeRawMap decodes the CBOR map at the start of data into a map of
// undecoded values, applying the same well-formedness, collection limit, and
// duplicate-key rules as DecodeLedgerMap. It returns the map and the number of
// bytes read. A CBOR null yields a nil map. It stops at the first key that does
// not decode as K, so memory grows only with entries that decode, never with
// the pair count the input declares.
func DecodeRawMap[K comparable](data []byte) (map[K]RawMessage, int, error) {
	count, headerSize, indefinite := MapInfo(data)
	if count < 0 {
		// Without a map head there is no pair count to size from, so the
		// full decoder can report null, another type, or a bad head.
		var fields map[K]RawMessage
		n, err := DecodeLedgerMap(data, &fields)
		if err != nil {
			return nil, 0, err
		}
		return fields, n, nil
	}
	decMode, err := getDecMode()
	if err != nil {
		return nil, 0, err
	}
	if decMode == nil {
		return nil, 0, errors.New("CBOR decoder mode not initialized")
	}
	// Decoding into a Go map would not do: fxamacker sizes a nil map from
	// the declared count, and on a bad key records an error per pair and
	// keeps walking. Validating the whole item first keeps the limits a full
	// decode applies, and lets the walk below index without bounds checks.
	var item listItem
	rest, err := decMode.UnmarshalFirst(data, &item)
	if err != nil {
		return nil, 0, err
	}
	n := len(data) - len(rest)
	fields := make(map[K]RawMessage, min(count, rawMapPresizePairs))
	pos := int(headerSize)
	// Declared once: each pointer handed to UnmarshalFirst escapes, so
	// per-pair variables would cost two heap allocations per pair.
	var key, zeroKey K
	var value RawMessage
	for i := 0; indefinite || i < count; i++ {
		if indefinite && data[pos] == 0xff {
			break
		}
		key = zeroKey
		if rest, err = decMode.UnmarshalFirst(data[pos:n], &key); err != nil {
			return nil, 0, err
		}
		pos = n - len(rest)
		// RawMessage decodes by appending to its receiver, so it must start
		// nil to avoid sharing the previous value's backing array.
		value = nil
		if rest, err = decMode.UnmarshalFirst(data[pos:n], &value); err != nil {
			return nil, 0, err
		}
		pos = n - len(rest)
		if _, found := fields[key]; found {
			return nil, 0, &_cbor.DupMapKeyError{Key: key, Index: i}
		}
		fields[key] = value
	}
	return fields, n, nil
}

// ValidateMapFields checks required integer keys and rejects explicitly empty
// array, map, or set values for selected fields in a CBOR map.
func ValidateMapFields(
	data []byte,
	requiredKeys []uint64,
	nonEmptyKeys []uint64,
) error {
	fields, _, err := DecodeRawMap[uint64](data)
	if err != nil {
		return fmt.Errorf("decode CBOR integer-keyed map: %w", err)
	}
	for _, key := range requiredKeys {
		if _, found := fields[key]; !found {
			return fmt.Errorf("required CBOR map field %d is missing", key)
		}
	}
	for _, key := range nonEmptyKeys {
		field, found := fields[key]
		if !found {
			continue
		}
		empty, collection, err := emptyCollection(field)
		if err != nil {
			return fmt.Errorf("decode CBOR map field %d: %w", key, err)
		}
		if !collection {
			return fmt.Errorf(
				"CBOR map field %d is not an array, map, or set",
				key,
			)
		}
		if empty {
			return fmt.Errorf("CBOR map field %d must not be empty", key)
		}
	}
	return nil
}

func emptyCollection(data []byte) (empty, collection bool, err error) {
	if len(data) == 0 {
		return false, false, errors.New("empty CBOR value")
	}
	major := data[0] >> 5
	additional := data[0] & 31
	if major == 6 {
		headerSize := 1
		switch additional {
		case 24:
			headerSize += 1
		case 25:
			headerSize += 2
		case 26:
			headerSize += 4
		case 27:
			headerSize += 8
		case 31:
			return false, false, errors.New("indefinite CBOR tag")
		}
		if len(data) <= headerSize {
			return false, false, errors.New("truncated CBOR tag")
		}
		return emptyCollection(data[headerSize:])
	}
	if major != 4 && major != 5 {
		return false, false, nil
	}
	if additional == 31 {
		return len(data) > 1 && data[1] == 0xff, true, nil
	}
	if additional < 24 {
		return additional == 0, true, nil
	}
	var length uint64
	headerSize := 1
	var width int
	switch additional {
	case 24:
		width = 1
	case 25:
		width = 2
	case 26:
		width = 4
	case 27:
		width = 8
	default:
		return false, false, errors.New("invalid CBOR collection length")
	}
	headerSize += width
	if len(data) < headerSize {
		return false, false, errors.New("truncated CBOR collection")
	}
	for _, b := range data[1:headerSize] {
		length = length<<8 | uint64(b)
	}
	return length == 0, true, nil
}
