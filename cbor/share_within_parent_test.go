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

package cbor_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func TestShareWithinParent(t *testing.T) {
	t.Parallel()
	elem := []byte{0x82, 0x01, 0x02}
	tests := []struct {
		name   string
		parent []byte
		shared bool
	}{
		{"definite array", []byte{0x82, 0x82, 0x01, 0x02, 0x03}, true},
		{"indefinite array", []byte{0x9f, 0x82, 0x01, 0x02, 0x03, 0xff}, true},
		{"different element bytes", []byte{0x82, 0x82, 0x01, 0x09, 0x03}, false},
		{"not an array head", []byte{0xa2, 0x82, 0x01, 0x02, 0x03}, false},
		{"multi-byte array head", []byte{0x98, 0x02, 0x82, 0x01, 0x02}, false},
		{"parent too short", []byte{0x82, 0x82, 0x01}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var d cbor.DecodeStoreCbor
			d.SetCbor(elem)
			before := &d.Cbor()[0]
			d.ShareWithinParent(tc.parent)
			require.Equal(t, elem, d.Cbor())
			if tc.shared {
				require.Same(t, &tc.parent[1], &d.Cbor()[0])
				require.Equal(t, len(elem), cap(d.Cbor()))
			} else {
				require.Same(t, before, &d.Cbor()[0])
			}
		})
	}
	t.Run("empty stored cbor", func(t *testing.T) {
		t.Parallel()
		var d cbor.DecodeStoreCbor
		d.ShareWithinParent([]byte{0x82, 0x01, 0x02})
		require.Nil(t, d.Cbor())
	})
}
