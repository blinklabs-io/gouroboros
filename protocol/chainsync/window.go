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
	"encoding/binary"

	"github.com/blinklabs-io/gouroboros/ledger"
)

// windowPlacement is what the sync window knows of the client's position
// on the server's chain, which decides how many requests it keeps
// outstanding.
type windowPlacement int

const (
	// windowUnplaced follows a new intersection or a MsgRollBackward, whose
	// point carries no block number: one request stays outstanding until a
	// roll forward places the client again.
	windowUnplaced windowPlacement = iota
	// windowPlaced means windowPosBlock holds the block number of the last
	// header or block delivered.
	windowPlaced
	// windowUnmeasured means the last delivery's block number could not be
	// read cheaply (a Byron header or block), so the window is kept full.
	windowUnmeasured
)

// windowTarget returns how many requests the sync loop keeps outstanding.
// A request beyond the server's tip is answered only once the server's chain
// grows, and Reintersect and StopAfterDrain must wait for every outstanding
// request, so the window extends at most one request past the server's tip:
// one per block the client is behind, between 1 and PipelineLimit. This is
// the reference client's pipelineDecisionMax. The caller holds
// lifecycleMutex.
func (c *Client) windowTarget() int {
	limit := max(c.config.PipelineLimit, 1)
	switch c.windowPlacement {
	case windowPlaced:
		if c.windowPosBlock >= c.windowTipBlock {
			return 1
		}
		behind := c.windowTipBlock - c.windowPosBlock
		if behind >= uint64(limit) {
			return limit
		}
		return int(behind) //nolint:gosec // below limit, an int
	case windowUnmeasured:
		return limit
	case windowUnplaced:
		return 1
	}
	return 1
}

// placeWindow records a delivered roll forward for the sync window. The
// caller holds lifecycleMutex.
func (c *Client) placeWindow(blockNumber uint64, measured bool, tip Tip) {
	c.windowTipBlock = tip.BlockNumber
	if !measured {
		c.windowPlacement = windowUnmeasured
		return
	}
	c.windowPlacement = windowPlaced
	c.windowPosBlock = blockNumber
}

// unplaceWindow records a delivered roll backward, or a new intersection,
// for the sync window. The caller holds lifecycleMutex.
func (c *Client) unplaceWindow(tip Tip) {
	c.windowTipBlock = tip.BlockNumber
	c.windowPlacement = windowUnplaced
}

// headerBlockNumber reads the block number from a post-Byron header, a CBOR
// array whose first item is the header body array, whose first item is the
// block number. It reports false for a Byron header or bytes of any other
// shape, without decoding the rest of the header.
func headerBlockNumber(blockType uint, headerCbor []byte) (uint64, bool) {
	if blockType == ledger.BlockTypeByronEbb ||
		blockType == ledger.BlockTypeByronMain {
		return 0, false
	}
	return firstUintInArrays(headerCbor, 2)
}

// blockBlockNumber reads the block number from a post-Byron block, a CBOR
// array whose first item is its header. See headerBlockNumber.
func blockBlockNumber(blockType uint, blockCbor []byte) (uint64, bool) {
	if blockType == ledger.BlockTypeByronEbb ||
		blockType == ledger.BlockTypeByronMain {
		return 0, false
	}
	return firstUintInArrays(blockCbor, 3)
}

// firstUintInArrays descends depth nested CBOR arrays, each the first item
// of the one before, and decodes the unsigned integer that is the first item
// of the innermost.
func firstUintInArrays(data []byte, depth int) (uint64, bool) {
	for range depth {
		if len(data) == 0 || data[0]>>5 != 4 {
			return 0, false
		}
		n, ok := cborHeadLen(data[0] & 0x1f)
		if !ok || len(data) < n {
			return 0, false
		}
		data = data[n:]
	}
	if len(data) == 0 || data[0]>>5 != 0 {
		return 0, false
	}
	info := data[0] & 0x1f
	n, ok := cborHeadLen(info)
	if !ok || info == 31 || len(data) < n {
		return 0, false
	}
	switch n {
	case 1:
		return uint64(info), true
	case 2:
		return uint64(data[1]), true
	case 3:
		return uint64(binary.BigEndian.Uint16(data[1:3])), true
	case 5:
		return uint64(binary.BigEndian.Uint32(data[1:5])), true
	default:
		return binary.BigEndian.Uint64(data[1:9]), true
	}
}

// cborHeadLen returns the length of a CBOR item's head, the initial byte
// and its argument, from the initial byte's additional information. 31 is
// an indefinite length, whose head is the initial byte alone.
func cborHeadLen(info byte) (int, bool) {
	switch {
	case info < 24, info == 31:
		return 1, true
	case info == 24:
		return 2, true
	case info == 25:
		return 3, true
	case info == 26:
		return 5, true
	case info == 27:
		return 9, true
	default:
		return 0, false
	}
}
