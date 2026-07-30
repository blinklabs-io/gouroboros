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

package byron

import (
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// Byron merkle nodes are domain-separated by a leading tag byte so a leaf
// hash can never be mistaken for a branch hash.
const (
	merkleLeafTag   byte = 0
	merkleBranchTag byte = 1
)

// MerkleRoot computes the Byron merkle root over pre-encoded items, matching
// cardano-ledger's `Cardano.Chain.Common.Merkle`.
//
// Items are the original CBOR bytes of each element, never a re-encoding: the
// root has to match what the block producer hashed, and a round-trip through
// our encoder is not guaranteed to reproduce those bytes.
//
// The empty tree hashes the empty byte string, and the tree is built by
// splitting at the largest power of two below the item count, which makes it
// left-heavy in the same way as the reference implementation.
func MerkleRoot(items [][]byte) common.Blake2b256 {
	if len(items) == 0 {
		return common.Blake2b256Hash(nil)
	}
	return merkleNode(items)
}

func merkleNode(items [][]byte) common.Blake2b256 {
	if len(items) == 1 {
		return common.Blake2b256Hash(
			append([]byte{merkleLeafTag}, items[0]...),
		)
	}
	split := largestPowerOfTwoBelow(len(items))
	left := merkleNode(items[:split])
	right := merkleNode(items[split:])
	combined := make([]byte, 0, 1+len(left)+len(right))
	combined = append(combined, merkleBranchTag)
	combined = append(combined, left[:]...)
	combined = append(combined, right[:]...)
	return common.Blake2b256Hash(combined)
}

// largestPowerOfTwoBelow returns the largest power of two strictly less than
// n, which is where the reference implementation splits a node's items. n is
// always at least 2 here, so the result is at least 1.
func largestPowerOfTwoBelow(n int) int {
	power := 1
	for power*2 < n {
		power *= 2
	}
	return power
}
