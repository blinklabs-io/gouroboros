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
	"fmt"
)

// ValidateMapFields checks required integer keys and rejects explicitly empty
// array, map, or set values for selected fields in a CBOR map.
func ValidateMapFields(
	data []byte,
	requiredKeys []uint64,
	nonEmptyKeys []uint64,
) error {
	var fields map[uint64]RawMessage
	if _, err := Decode(data, &fields); err != nil {
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
		return false, false, fmt.Errorf("empty CBOR value")
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
			return false, false, fmt.Errorf("indefinite CBOR tag")
		}
		if len(data) <= headerSize {
			return false, false, fmt.Errorf("truncated CBOR tag")
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
		return false, false, fmt.Errorf("invalid CBOR collection length")
	}
	headerSize += width
	if len(data) < headerSize {
		return false, false, fmt.Errorf("truncated CBOR collection")
	}
	for _, b := range data[1:headerSize] {
		length = length<<8 | uint64(b)
	}
	return length == 0, true, nil
}
