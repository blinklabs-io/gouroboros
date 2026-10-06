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

package chainsync

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborpreflight"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborwalk"
)

const (
	// The two-field message and type need two bytes. A point count large enough
	// to reach this allocation ceiling uses a five-byte array header.
	findIntersectFrameBytes = 7
	// This is the largest established valid high-cardinality control. Bounding
	// the decoded Point slice here prevents a small origin-only wire message
	// from expanding into hundreds of megabytes of slice backing storage.
	maxFindIntersectDecodedPoints = 131_073
)

func maxFindIntersectPoints(modeLimit int) int {
	return max(0, min(modeLimit-findIntersectFrameBytes, maxFindIntersectDecodedPoints))
}

func validateFindIntersect(data []byte, maxCount int) error {
	return cborpreflight.ValidateSecondFieldArray(
		data,
		maxCount,
		"find-intersect point array",
		func(idx int, raw []byte) error {
			pointCount, _, indefinite := cbor.ArrayInfo(raw)
			if indefinite || (pointCount != 0 && pointCount != 2) {
				return fmt.Errorf(
					"find-intersect point %d must have 0 or 2 fields",
					idx,
				)
			}
			if pointCount == 0 {
				return nil
			}
			return cborpreflight.ValidateArray(raw, 2, "find-intersect point", func(field int, value []byte) error {
				head, ok, err := cborwalk.ReadHead(value, 0)
				if err != nil || !ok {
					return fmt.Errorf("find-intersect point %d field %d has invalid CBOR", idx, field)
				}
				switch field {
				case 0:
					if head.Major != 0 || head.Indefinite || head.EncodedSize != len(value) {
						return fmt.Errorf("find-intersect point %d slot must be an unsigned integer", idx)
					}
				case 1:
					if head.Major != 2 || head.Indefinite || head.Argument != 32 || head.EncodedSize+32 != len(value) {
						return fmt.Errorf("find-intersect point %d hash must be 32 bytes", idx)
					}
				}
				return nil
			})
		},
	)
}
