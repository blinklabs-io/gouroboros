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
	"fmt"

	"github.com/blinklabs-io/gouroboros/protocol/internal/cborpreflight"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborwalk"
)

const (
	maxWireBlockBitmaps = int(^uint16(0)) + 1
	maxWireBlockTxs     = maxWireBlockBitmaps * 64
)

func validateBlockTxBitmaps(data []byte, maxCount int) error {
	var seen [maxWireBlockBitmaps / 8]byte
	return cborpreflight.ValidateMap(
		data,
		maxCount,
		"block transaction bitmaps",
		func(idx int, key, value []byte) error {
			keyHead, ok, err := cborwalk.ReadHead(key, 0)
			if err != nil || !ok || keyHead.Major != 0 ||
				keyHead.EncodedSize != len(key) ||
				keyHead.Argument > uint64(^uint16(0)) {
				return fmt.Errorf(
					"block transaction bitmap key %d must be uint16",
					idx,
				)
			}
			keyValue := uint16(keyHead.Argument)
			byteIndex := keyValue >> 3
			bit := byte(1 << (keyValue & 7))
			if seen[byteIndex]&bit != 0 {
				return fmt.Errorf(
					"duplicate block transaction bitmap key %d",
					keyValue,
				)
			}
			seen[byteIndex] |= bit
			valueHead, ok, err := cborwalk.ReadHead(value, 0)
			if err != nil || !ok || valueHead.Major != 0 ||
				valueHead.EncodedSize != len(value) {
				return fmt.Errorf(
					"block transaction bitmap value %d must be uint64",
					idx,
				)
			}
			return nil
		},
	)
}

func validateBlockTxsRequest(data []byte) error {
	fields, err := cborpreflight.ArrayItems(
		data,
		3,
		"block transactions request",
	)
	if err != nil {
		return err
	}
	if len(fields) != 3 {
		return fmt.Errorf(
			"block transactions request has %d fields, expected 3",
			len(fields),
		)
	}
	if err := cborpreflight.ValidateItemDepth(
		fields[0],
		0,
		"block transactions request message type",
	); err != nil {
		return err
	}
	if err := cborpreflight.ValidateItemDepth(
		fields[1],
		1,
		"block transactions request point",
	); err != nil {
		return err
	}
	return validateBlockTxBitmaps(fields[2], maxWireBlockBitmaps)
}
