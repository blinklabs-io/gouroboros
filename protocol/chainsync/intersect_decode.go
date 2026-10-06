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
)

// A FindIntersect message needs one byte each for its message array, type and
// point-array headers. An origin point is one byte, so the mode's existing byte
// budget is also the conservative maximum point count.
const findIntersectFrameBytes = 3

func maxFindIntersectPoints(modeLimit int) int {
	return modeLimit - findIntersectFrameBytes
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
			return nil
		},
	)
}
