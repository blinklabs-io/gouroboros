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

package ledger

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// maxRejectionItems bounds the total number of CBOR data items in one
// transaction rejection reason. The decoder's per-collection limit does not
// bound a payload made of many collections, and a rejection is peer-supplied.
// A rejection echoes at most a transaction's worth of data (tens of thousands
// of items for a maximum-size transaction), so this is far above any real
// ApplyTxError while keeping decoded memory in the tens of megabytes.
const maxRejectionItems = 131072

// checkRejectionBudget walks the CBOR headers of the first item in data and
// returns an error when it holds more than maxRejectionItems data items or
// nests deeper than cbor.MaxUntrustedNestedLevels. It allocates only the
// nesting stack, never a decoded value, and checks each declared collection
// length against the remaining budget before descending into it.
func checkRejectionBudget(data []byte) error {
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
			return errors.New("rejection: truncated CBOR")
		}
		first := data[pos]
		major := first >> 5
		info := first & 0x1f
		if first == 0xff {
			if stack[top] != -1 {
				return errors.New("rejection: unexpected CBOR break")
			}
			pos++
			stack = stack[:top]
			continue
		}
		if stack[top] > 0 {
			stack[top]--
		}
		items++
		if items > maxRejectionItems {
			return fmt.Errorf(
				"rejection exceeds %d CBOR items",
				maxRejectionItems,
			)
		}
		pos++
		var arg uint64
		switch {
		case info < 24:
			arg = uint64(info)
		case info <= 27:
			size := 1 << (info - 24)
			if len(data)-pos < size {
				return errors.New("rejection: truncated CBOR")
			}
			for _, b := range data[pos : pos+size] {
				arg = arg<<8 | uint64(b)
			}
			pos += size
		case info == 31 && major >= 2 && major <= 5:
			// Indefinite length.
		default:
			return errors.New("rejection: invalid CBOR header")
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
					return errors.New("rejection: truncated CBOR")
				}
				pos += int(arg)
			}
		case 4, 5:
			if indefinite {
				push = -1
				break
			}
			if major == 5 {
				if arg > maxRejectionItems {
					arg = maxRejectionItems + 1
				}
				arg *= 2
			}
			if arg > uint64(maxRejectionItems-items) {
				return fmt.Errorf(
					"rejection exceeds %d CBOR items",
					maxRejectionItems,
				)
			}
			push = int64(arg)
		case 6:
			push = 1
		}
		if push != 0 {
			if len(stack) >= cbor.MaxUntrustedNestedLevels {
				return errors.New("rejection: CBOR nesting too deep")
			}
			stack = append(stack, push)
		}
	}
	return nil
}
