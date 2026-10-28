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
)

// ErrItemBudget reports CBOR data holding more data items than the caller's
// budget allows.
var ErrItemBudget = errors.New("CBOR item budget exceeded")

// CheckItemBudget walks the CBOR headers of the first item in data and returns
// an error wrapping ErrItemBudget when it holds more than maxItems data items,
// or another error when it is truncated, malformed, or nests deeper than
// MaxUntrustedNestedLevels. Every array element, map key, map value, and tag
// content counts as one item, as does the first item itself. It allocates only
// the nesting stack, never a decoded value, and checks each declared
// collection length against the remaining budget before descending into it,
// so its cost is bounded by the budget rather than by declared lengths.
func CheckItemBudget(data []byte, maxItems int) error {
	_, err := countItems(data, maxItems)
	return err
}

// countItems returns the number of data items in the first item of data, or
// an error once that number would exceed maxItems.
func countItems(data []byte, maxItems int) (int, error) {
	// Each frame holds the number of children still owed by a definite
	// collection, or -1 for an indefinite one that ends at a break.
	stack := []int64{1}
	pos := 0
	items := 0
	for len(stack) > 0 {
		top := len(stack) - 1
		if stack[top] == 0 {
			stack = stack[:top]
			continue
		}
		if pos >= len(data) {
			return 0, errors.New("cbor: truncated CBOR")
		}
		first := data[pos]
		major := first >> 5
		info := first & 0x1f
		if first == 0xff {
			if stack[top] != -1 {
				return 0, errors.New("cbor: unexpected CBOR break")
			}
			pos++
			stack = stack[:top]
			continue
		}
		if stack[top] > 0 {
			stack[top]--
		}
		items++
		if items > maxItems {
			return 0, fmt.Errorf("%w: limit %d", ErrItemBudget, maxItems)
		}
		pos++
		var arg uint64
		switch {
		case info < 24:
			arg = uint64(info)
		case info <= 27:
			size := 1 << (info - 24)
			if len(data)-pos < size {
				return 0, errors.New("cbor: truncated CBOR")
			}
			for _, b := range data[pos : pos+size] {
				arg = arg<<8 | uint64(b)
			}
			pos += size
		case info == 31 && major >= 2 && major <= 5:
			// Indefinite length.
		default:
			return 0, errors.New("cbor: invalid CBOR header")
		}
		indefinite := info == 31
		var push int64
		switch major {
		case 0, 1, 7:
		case 2, 3:
			if indefinite {
				push = -1
			} else {
				// pos never exceeds len(data), so the conversion is non-negative.
				if arg > uint64(len(data)-pos) { //nolint:gosec
					return 0, errors.New("cbor: truncated CBOR")
				}
				pos += int(arg)
			}
		case 4, 5:
			if indefinite {
				push = -1
				break
			}
			if major == 5 {
				// A count past the budget is clamped so doubling it cannot
				// overflow.
				if arg > uint64(maxItems) { //nolint:gosec
					arg = uint64(maxItems) + 1 //nolint:gosec
				}
				arg *= 2
			}
			// items never exceeds maxItems here, so the difference is
			// non-negative.
			if arg > uint64(maxItems-items) { //nolint:gosec
				return 0, fmt.Errorf("%w: limit %d", ErrItemBudget, maxItems)
			}
			push = int64(arg)
		case 6:
			push = 1
		}
		if push != 0 {
			if len(stack) >= MaxUntrustedNestedLevels {
				return 0, errors.New("cbor: CBOR nesting too deep")
			}
			stack = append(stack, push)
		}
	}
	return items, nil
}
