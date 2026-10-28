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

package localstatequery

import (
	"fmt"
	"math"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborpreflight"
)

// growingArray decodes a CBOR array (optionally tagged, such as a tag-258 set)
// one entry at a time. Decoding straight into a []T sizes the slice from the
// peer-declared count, so an array of one-byte entries would reserve a whole T
// per byte before the first entry fails to decode. Here the slice grows only
// with entries that decode.
type growingArray[T any] []T

func (a *growingArray[T]) UnmarshalCBOR(data []byte) error {
	// A null or undefined decodes to a nil slice, as it does for a plain []T.
	if len(data) == 1 && (data[0] == 0xf6 || data[0] == 0xf7) {
		*a = nil
		return nil
	}
	items := make([]T, 0)
	err := cborpreflight.VisitArray(
		data,
		math.MaxInt,
		"array",
		func(idx int, raw []byte) error {
			var item T
			if _, err := cbor.Decode(raw, &item); err != nil {
				return fmt.Errorf("decode array item %d: %w", idx, err)
			}
			items = append(items, item)
			return nil
		},
	)
	if err != nil {
		return err
	}
	*a = items
	return nil
}
