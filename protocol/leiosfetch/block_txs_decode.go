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

package leiosfetch

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborpreflight"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborwalk"
)

func blockTxsEnvelopeCount(data []byte) (int, error) {
	cursor := cborCursor{data: data}
	if err := cursor.skipTags(); err != nil {
		return 0, err
	}
	count, indefinite, err := cursor.header(cbor.CborTypeArray)
	if err != nil {
		return 0, err
	}
	if !indefinite {
		if count > 4 {
			return 0, errors.New("block transactions envelope has too many fields")
		}
		return int(count), nil
	}
	for count = 0; ; count++ {
		if cursor.pos >= len(data) {
			return 0, errors.New("unterminated block transactions envelope")
		}
		if data[cursor.pos] == 0xff {
			return int(count), nil
		}
		if count >= 5 {
			return 0, errors.New("block transactions envelope has too many fields")
		}
		if length, err := cborwalk.ItemLength(data[cursor.pos:]); err != nil {
			return 0, fmt.Errorf(
				"decode block transactions field %d: %w",
				count,
				err,
			)
		} else {
			cursor.pos += length
		}
	}
}

const maxWireBlockTxs = (int(^uint16(0)) + 1) * 64

func validateBlockTxsTransactions(data []byte, elementCount, maxCount int) error {
	return cborpreflight.ValidateArray(data, 4, "block transactions envelope", func(idx int, raw []byte) error {
		if idx != elementCount-1 {
			return nil
		}
		return cborpreflight.ValidateArray(raw, maxCount, "block transactions", nil)
	})
}
