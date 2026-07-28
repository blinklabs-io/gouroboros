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

package byron_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
)

// leaf and branch recompute the Byron merkle primitives directly from the
// specification, so these tests check the tree shape independently rather
// than by calling the implementation they are verifying.
func leaf(item []byte) common.Blake2b256 {
	return common.Blake2b256Hash(append([]byte{0}, item...))
}

func branch(l, r common.Blake2b256) common.Blake2b256 {
	combined := []byte{1}
	combined = append(combined, l[:]...)
	combined = append(combined, r[:]...)
	return common.Blake2b256Hash(combined)
}

// TestMerkleRootShape pins how items are combined. The split point is the
// largest power of two below the item count, which makes odd trees left-heavy;
// getting this wrong still produces a plausible-looking hash, so the structure
// is asserted explicitly for each size.
func TestMerkleRootShape(t *testing.T) {
	a := []byte("a")
	b := []byte("b")
	c := []byte("c")
	d := []byte("d")
	e := []byte("e")

	tests := []struct {
		name  string
		items [][]byte
		want  common.Blake2b256
	}{
		{
			name:  "empty hashes the empty string",
			items: nil,
			want:  common.Blake2b256Hash(nil),
		},
		{
			name:  "single item is a bare leaf",
			items: [][]byte{a},
			want:  leaf(a),
		},
		{
			name:  "two items split evenly",
			items: [][]byte{a, b},
			want:  branch(leaf(a), leaf(b)),
		},
		{
			name:  "three items split at two",
			items: [][]byte{a, b, c},
			want:  branch(branch(leaf(a), leaf(b)), leaf(c)),
		},
		{
			name:  "four items form a balanced tree",
			items: [][]byte{a, b, c, d},
			want:  branch(branch(leaf(a), leaf(b)), branch(leaf(c), leaf(d))),
		},
		{
			name:  "five items split at four",
			items: [][]byte{a, b, c, d, e},
			want: branch(
				branch(branch(leaf(a), leaf(b)), branch(leaf(c), leaf(d))),
				leaf(e),
			),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, byron.MerkleRoot(tc.items))
		})
	}
}

// TestMerkleRootOrderSensitive confirms the root depends on item order, which
// is what stops transactions being reordered without detection.
func TestMerkleRootOrderSensitive(t *testing.T) {
	forward := byron.MerkleRoot([][]byte{[]byte("a"), []byte("b")})
	reversed := byron.MerkleRoot([][]byte{[]byte("b"), []byte("a")})
	assert.NotEqual(t, forward, reversed)
}

// TestMerkleRootLeafBranchDomainSeparation confirms a leaf can never collide
// with a branch, which the differing tag bytes are there to guarantee.
func TestMerkleRootLeafBranchDomainSeparation(t *testing.T) {
	single := byron.MerkleRoot([][]byte{[]byte("x")})
	// A one-item tree is a leaf; feeding the same bytes as a two-item tree
	// must take the branch path and produce something different.
	pair := byron.MerkleRoot([][]byte{[]byte("x"), []byte("x")})
	assert.NotEqual(t, single, pair)
}
