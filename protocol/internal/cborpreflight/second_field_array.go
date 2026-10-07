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

// Package cborpreflight validates protocol collection envelopes before typed
// decoding allocates from peer-controlled collection counts.
package cborpreflight

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborwalk"
)

// ValidateSecondFieldArray validates a two-field message whose second field is
// an array. validateItem may be nil when only the message-specific count bound
// is needed.
func ValidateSecondFieldArray(
	data []byte,
	maxCount int,
	label string,
	validateItem func(int, []byte) error,
) error {
	fields, err := arrayItems(data, 2, label+" message", nil, true)
	if err != nil {
		return err
	}
	if len(fields) != 2 {
		return fmt.Errorf(
			"%s message has %d fields, expected 2",
			label,
			len(fields),
		)
	}
	return validateArray(fields[1], maxCount, label, validateItem)
}

func validateArray(
	data []byte,
	maxCount int,
	label string,
	validateItem func(int, []byte) error,
) error {
	_, err := arrayItems(data, maxCount, label, validateItem, false)
	return err
}

// ValidateArray validates an array before a typed decoder can allocate from
// its peer-controlled count.
func ValidateArray(data []byte, maxCount int, label string, validateItem func(int, []byte) error) error {
	return validateArray(data, maxCount, label, validateItem)
}

// ArrayItems returns the encoded items of one validated array. The returned
// slices reference data and must not outlive or mutate it.
func ArrayItems(data []byte, maxCount int, label string) ([][]byte, error) {
	return arrayItems(data, maxCount, label, nil, true)
}

// ValidateItemDepth rejects an item whose nesting exceeds the immutable wire
// shape expected by its caller. It runs before typed decoding so malformed
// peer input cannot drive the recursive decoder to its general ledger limit.
func ValidateItemDepth(data []byte, maxDepth int, label string) error {
	length, err := cborwalk.ItemLengthWithin(data, maxDepth)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if length != len(data) {
		return fmt.Errorf("trailing data after %s", label)
	}
	return nil
}

func arrayItems(
	data []byte,
	maxCount int,
	label string,
	validateItem func(int, []byte) error,
	collect bool,
) ([][]byte, error) {
	pos, err := skipTags(data, 0, label)
	if err != nil {
		return nil, err
	}
	count, headerSize, indefinite := cbor.ArrayInfo(data[pos:])
	if count < 0 {
		return nil, fmt.Errorf("%s is not an array", label)
	}
	if !indefinite && count > maxCount {
		return nil, tooManyItems(label, count, maxCount)
	}
	pos += int(headerSize)
	var items [][]byte
	if collect {
		items = make([][]byte, 0, min(count, maxCount))
	}
	for idx := 0; indefinite || idx < count; idx++ {
		current := pos
		if indefinite {
			if current >= len(data) {
				return nil, fmt.Errorf("unterminated %s", label)
			}
			if data[current] == 0xff {
				pos++
				break
			}
			if idx >= maxCount {
				return nil, tooManyItems(label, idx+1, maxCount)
			}
		}
		length, err := cborwalk.ItemLength(data[current:])
		if err != nil {
			return nil, fmt.Errorf("decode %s item %d: %w", label, idx, err)
		}
		raw := data[current : current+length]
		pos += length
		if validateItem != nil {
			if err := validateItem(idx, raw); err != nil {
				return nil, err
			}
		}
		if collect {
			items = append(items, raw)
		}
	}
	if pos != len(data) {
		return nil, fmt.Errorf("trailing data after %s", label)
	}
	return items, nil
}

func skipTags(data []byte, pos int, label string) (int, error) {
	for depth := 0; pos < len(data) &&
		data[pos]&cbor.CborTypeMask == cbor.CborTypeTag; depth++ {
		if depth >= cbor.MaxNestedLevels {
			return 0, fmt.Errorf("%s tag nesting limit", label)
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
			return 0, fmt.Errorf("invalid %s tag", label)
		}
		if width >= len(data)-pos {
			return 0, fmt.Errorf("truncated %s tag", label)
		}
		pos += width + 1
	}
	if pos >= len(data) {
		return 0, errors.New("truncated CBOR message")
	}
	return pos, nil
}

func tooManyItems(label string, count, maxCount int) error {
	return fmt.Errorf(
		"%s has %d items, maximum is %d",
		label,
		count,
		maxCount,
	)
}
