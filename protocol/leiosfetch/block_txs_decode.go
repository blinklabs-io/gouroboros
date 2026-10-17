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
	"math/bits"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborpreflight"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborwalk"
)

const (
	// maxWireBlockTxs is the most transactions an endorser block can
	// reference; the reference node rejects any bitmaps or reply beyond it.
	maxWireBlockTxs = 17_142
	// maxWireBlockBitmaps is the number of 64-bit bitmaps covering
	// maxWireBlockTxs. A bitmap key is an index below it.
	maxWireBlockBitmaps = (maxWireBlockTxs + 63) / 64
)

// bitmapTxCount returns the number of transactions the bitmaps select.
func bitmapTxCount(bitmaps map[uint16]uint64) int {
	count := 0
	for _, bitmap := range bitmaps {
		count += bits.OnesCount64(bitmap)
	}
	return count
}

// validateBlockTxBitmaps applies the checks the reference node makes while
// decoding bitmaps, in wire order: keys strictly ascend, each below
// maxWireBlockBitmaps, and no bitmap is zero. Ascending keys also exclude a
// duplicate.
func validateBlockTxBitmaps(data []byte, maxCount int) error {
	maxCount = min(maxCount, maxWireBlockBitmaps)
	next := uint64(0)
	return cborpreflight.ValidateMap(
		data,
		maxCount,
		"block transaction bitmaps",
		func(idx int, key, value []byte) error {
			keyHead, ok, err := cborwalk.ReadHead(key, 0)
			if err != nil || !ok || keyHead.Major != 0 ||
				keyHead.EncodedSize != len(key) ||
				keyHead.Argument >= maxWireBlockBitmaps {
				return fmt.Errorf(
					"block transaction bitmap key %d must be an index below %d",
					idx,
					maxWireBlockBitmaps,
				)
			}
			if keyHead.Argument < next {
				return fmt.Errorf(
					"block transaction bitmap key %d is not strictly ascending",
					keyHead.Argument,
				)
			}
			next = keyHead.Argument + 1
			valueHead, ok, err := cborwalk.ReadHead(value, 0)
			if err != nil || !ok || valueHead.Major != 0 ||
				valueHead.EncodedSize != len(value) {
				return fmt.Errorf(
					"block transaction bitmap value %d must be uint64",
					idx,
				)
			}
			if valueHead.Argument == 0 {
				return fmt.Errorf(
					"block transaction bitmap %d selects no transaction",
					keyHead.Argument,
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

// unwrapBlockTx returns the block_transaction CBOR inside one
// #6.24(bytes .cbor block_transaction) entry. A plain byte string, any other
// tag, an indefinite-length byte string, and bytes that are not exactly one
// CBOR item are rejected.
func unwrapBlockTx(data []byte) ([]byte, error) {
	tagHead, ok, err := cborwalk.ReadHead(data, 0)
	if err != nil || !ok || tagHead.Major != 6 ||
		tagHead.Argument != cbor.CborTagCbor {
		return nil, errors.New("not a tag 24 wrapped transaction")
	}
	offset := tagHead.EncodedSize
	bytesHead, ok, err := cborwalk.ReadHead(data, offset)
	if err != nil || !ok || bytesHead.Major != 2 || bytesHead.Indefinite {
		return nil, errors.New("tag 24 does not wrap a definite byte string")
	}
	offset += bytesHead.EncodedSize
	//nolint:gosec // ReadHead bounds offset by len(data), so the difference is non-negative
	if bytesHead.Argument != uint64(len(data)-offset) {
		return nil, errors.New(
			"tag 24 byte string length does not match its entry",
		)
	}
	inner := data[offset:]
	if length, err := cborwalk.ItemLength(inner); err != nil ||
		length != len(inner) {
		return nil, errors.New("tag 24 bytes are not exactly one CBOR item")
	}
	return inner, nil
}
